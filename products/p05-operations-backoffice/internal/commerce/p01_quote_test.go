package commerce

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// The fixture statements both scope attempts through settlement work to the outer sale.
const countOrderRelayAttempts = `(SELECT count(*) FROM ledger.relay_attempt r
	JOIN ledger.settlement_work w ON w.id=r.work_id WHERE w.sale_id=s.id)`

type transientSigner struct{ key *ecdsa.PrivateKey }

func (signer transientSigner) SignDigest(_ context.Context, digest [32]byte) ([]byte, error) {
	sig, err := crypto.Sign(digest[:], signer.key)
	if err == nil {
		sig[64] += 27
	}
	return sig, err
}

type p01PaymentFixture struct {
	signed []byte
	secret []byte
}

func p01TestPayers(t *testing.T) (common.Address, common.Address, []byte, []byte) {
	t.Helper()
	// Same public account recovered by the separately attested P01 HD restore test.
	hd := common.HexToAddress("0x919538116b4f25f1ce01429fd9ed7964556bf565")
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	renter, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return hd, crypto.PubkeyToAddress(renter.PublicKey), seed, crypto.FromECDSA(renter)
}

func runP01Quotes(t *testing.T, runner string, profile Profile, fixtures []p01PaymentFixture) {
	t.Helper()
	dir := t.TempDir()
	args := make([]string, 0, len(fixtures))
	for index, fixture := range fixtures {
		artifact := append(append(append([]byte(nil), fixture.signed...), profile.Signer.Bytes()...), fixture.secret...)
		path := filepath.Join(dir, fmt.Sprintf("quote-%d.bin", index))
		if err := os.WriteFile(path, artifact, 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, path)
	}
	if output, err := exec.Command(runner, args...).CombinedOutput(); err != nil {
		t.Fatalf("P01 handler rejected a P05 quote: %v\n%s", err, output)
	}
	refused, err := exec.Command(runner, "--refuse-review", args[0], args[2]).CombinedOutput()
	if err != nil || len(refused) != 0 {
		t.Fatalf("P01 refusal unexpectedly failed or emitted bytes: %v (%d bytes)", err, len(refused))
	}
}

func issueP01LedgerQuotes(t *testing.T, ctx context.Context, h HTTP, runSQL func(string, ...any), readLedger func(string) (string, int, int), profile Profile) {
	t.Helper()
	runner := os.Getenv("NU54_P01_INSTALL_TEST")
	if runner == "" {
		return
	}
	payerA, payerB, seed, standalone := p01TestPayers(t)
	runSQL("UPDATE ledger.payer_enrollment SET payer=$3,payer_role=1,expires_at=now()+interval '2 minutes' WHERE branch_id=$1 AND actor_id=$2",
		branch, customerA, payerA.Bytes())
	runSQL("UPDATE ledger.payer_enrollment SET payer=$3,payer_role=2,expires_at=now()+interval '2 minutes' WHERE branch_id=$1 AND actor_id=$2",
		branch, customerB, payerB.Bytes())
	const sku = "p01-quote"
	runSQL(`INSERT INTO ledger.price(branch_id,version,sku,option_key,label,asset_ref,unit_price)
		SELECT branch_id,version,$2,option_key,label,asset_ref,unit_price FROM ledger.price
		WHERE branch_id=$1 AND version=1 AND sku='pos' AND option_key='plain'`, branch, sku)
	runSQL("INSERT INTO ledger.stock VALUES($1,$2,3)", branch, sku)
	kiosk := actor{terminal, "terminal"}
	asset := fmt.Sprintf("0x%x", profile.Asset.Bytes())
	fixtures := make([]p01PaymentFixture, 0, 3)
	var original, other string
	for index, secret := range [][]byte{seed, standalone, seed} {
		subject := actor{customerA, "customer"}
		if index == 1 {
			subject.id = customerB
		}
		origin := fmt.Sprintf("f8000000-0000-4000-8000-%012x", index+1)
		bound := authorize(t, kiosk, "order.create", branch, origin, origin,
			map[string]string{"asset": asset, "quantity": "1", "intentDigest": fmt.Sprintf("%064x", index+1)})
		sale, err := h.Store.Reserve(ctx, bound, Cart{Origin: "pos", OriginRef: origin, MenuVersion: 1, Items: []Item{{sku, "plain", 1}}})
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			original = sale
		} else if index == 2 {
			other = sale
		}
		checkout := authorize(t, kiosk, "checkout.authorize", branch, sale, origin, map[string]string{"customerRef": subject.id})
		claim, err := h.Store.AuthorizeCheckout(ctx, checkout)
		if err != nil {
			t.Fatal(err)
		}
		issuer := subject
		if index == 1 {
			issuer = kiosk
		}
		issue := authorize(t, issuer, "quote.issue", branch, sale, origin,
			map[string]string{"claimDigest": claim.Digest, "orderRevision": "0"})
		quote := readAuthenticatedQuote(t, h, issue, nil)
		signed, err := hex.DecodeString(quote.Signed)
		if err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, p01PaymentFixture{signed, secret})
	}
	runP01Quotes(t, runner, profile, fixtures)
	// Seed a valid reserved attempt for a different issued sale. The SW2 assertion must
	// count only the original sale's attempts while the database contains a real positive row.
	runSQL(`INSERT INTO ledger.relay_attempt(work_id,chain_id,sender,nonce,intent_digest,expires_at)
		SELECT id,8283,'0x0000000000000000000000000000000000000001',1,decode(repeat('ab',32),'hex'),now()+interval '1 minute'
		FROM ledger.settlement_work WHERE sale_id=$1 AND refund_id IS NULL`, other)
	state, payments, relays := readLedger(original)
	if state != "issued" || payments != 0 || relays != 0 {
		t.Fatalf("P01 SW2 scope changed: state=%s payments=%d relays=%d", state, payments, relays)
	}
	otherState, otherPayments, otherRelays := readLedger(other)
	if otherState != "issued" || otherPayments != 0 || otherRelays != 1 {
		t.Fatalf("P01 control sale: state=%s payments=%d relays=%d", otherState, otherPayments, otherRelays)
	}
	t.Log("P01 SW2 original issued, 0 payments, 0 relay attempts; separate issued sale 1 reserved attempt")
}

func TestP01AcceptsTwoP05Quotes(t *testing.T) {
	runner := os.Getenv("NU54_P01_INSTALL_TEST")
	if runner == "" {
		t.Skip("set NU54_P01_INSTALL_TEST to the P01 host test_install executable")
	}
	merchant, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	profile := FixtureProfile()
	profile.Signer = crypto.PubkeyToAddress(merchant.PublicKey)
	fixtures := make([]p01PaymentFixture, 0, 3)
	firstPayer, secondPayer, seed, standalone := p01TestPayers(t)
	for index, payer := range []common.Address{firstPayer, secondPayer, firstPayer} {
		secret := seed
		i := byte(index + 1)
		order := FixtureOrder()
		order.QuoteID[1], order.OrderID[1], order.Nonce[1] = i, i, i
		order.Payer = payer
		if index == 1 {
			order.PayerRole, secret = 2, standalone
		}
		artifact, _, err := SignQuote(context.Background(), order, profile, transientSigner{merchant}, time.Unix(1601, 0))
		if err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, p01PaymentFixture{artifact, secret})
	}
	runP01Quotes(t, runner, profile, fixtures)
}
