package commerce

import (
	"context"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
)

var k10FixturePayer = common.HexToAddress("0x1111111111111111111111111111111111111111")

// InstalledW10Settlement connects the installed menu to the local settlement
// fixture. The real forwarder and signer use the same archived quote.
type InstalledW10Settlement struct {
	Rental                 *InstalledW10Rental
	Branch                 string
	Profile                Profile
	Signer                 Signer
	QuoteCipher            cipher.AEAD
	Payer                  *ecdsa.PrivateKey
	RelayAccount           common.Address
	LookupSponsor          GasSponsorLookup
	Forwarder              PaymentForwarder
	ExpireOriginal         func(context.Context, string) (bool, error)
	FinalizeOriginal       func(context.Context, string) (string, error)
	RegisterPickupReceiver func(*http.ServeMux, string)
	DispatchOriginal       func(context.Context, *http.Client, string, string) error
	RegisterFinance        func(*http.ServeMux, map[string]service.Principal, string)
}

type installedMenuSigner struct {
	Signer
	calls atomic.Int32
}

func (s *installedMenuSigner) SignDigest(ctx context.Context, digest [32]byte) ([]byte, error) {
	s.calls.Add(1)
	return s.Signer.SignDigest(ctx, digest)
}

type installedSignedMenu struct {
	profile      Profile
	presentation ed25519.PublicKey
	signer       *installedMenuSigner
	mu           sync.Mutex
	initial      quoteReply
	retrieved    bool
	forwards     atomic.Int32
	gasAccount   common.Address
}

func prepareInstalledSignedMenu(t *testing.T, ctx context.Context, db *pgx.Conn, handler *HTTP, branch, asset string, settlement *InstalledW10Settlement) *installedSignedMenu {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	public, presenter, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var cipherKey [32]byte
	if _, err := rand.Read(cipherKey[:]); err != nil {
		t.Fatal(err)
	}
	cipher, err := NewCipher(cipherKey[:])
	if err != nil {
		t.Fatal(err)
	}
	profile := FixtureProfile()
	profile.Asset = common.HexToAddress(asset)
	profile.Signer = crypto.PubkeyToAddress(key.PublicKey)
	signer := &installedMenuSigner{Signer: transientSigner{key: key}}
	if settlement != nil {
		profile, cipher = settlement.Profile, settlement.QuoteCipher
		signer = &installedMenuSigner{Signer: settlement.Signer}
		handler.Payment, handler.GasSponsors = settlement.Forwarder, settlement.LookupSponsor
	}
	handler.Profiles[branch] = profile
	handler.PresentationKey, handler.Signer, handler.Cipher = presenter, signer, cipher
	digest := profile.Digest()
	if _, err := db.Exec(ctx, "INSERT INTO ledger.issuance_profile VALUES($1,1,$2,120,true)", branch, digest[:]); err != nil {
		t.Fatal(err)
	}
	signed := &installedSignedMenu{profile: profile, presentation: public, signer: signer}
	if settlement != nil {
		signed.gasAccount = settlement.RelayAccount
	}
	return signed
}

func (s *installedSignedMenu) sponsorFixture(t *testing.T, h *HTTP, branch string) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s.gasAccount = crypto.PubkeyToAddress(key.PublicKey)
	authority, sign, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(8 * time.Minute).Unix()
	// The installed fixture provides its own expiring sponsorship authority.
	// The order still traverses production scope, signature and SQL pinning.
	message := func(current string, profile Profile) []byte {
		digest := profile.Digest()
		return []byte(fmt.Sprintf("NU54-installed-test-merchant-relay-v1\x00%s/%x/%x/%d",
			current, digest[:], s.gasAccount.Bytes(), until))
	}
	approved := message(branch, s.profile)
	proof := ed25519.Sign(sign, approved)
	grant := sha256.Sum256(approved)
	h.GasSponsors = func(_ context.Context, current string, profile Profile, payer common.Address, now time.Time) (GasSponsorship, bool) {
		return GasSponsorship{Account: s.gasAccount, Grant: grant, ExpiresAt: until},
			now.Unix() < until && payer != s.gasAccount && ed25519.Verify(authority, message(current, profile), proof)
	}
}

func (s *installedSignedMenu) profileDocument() (map[string]any, string) {
	fingerprint := sha256.Sum256(s.presentation)
	p := s.profile
	return map[string]any{
		"chain": p.Chain, "contract": hexAddress(p.Contract), "asset": hexAddress(p.Asset),
		"beneficiary": hexAddress(p.Beneficiary), "merchantId": hex.EncodeToString(p.MerchantID[:]),
		"signer": hexAddress(p.Signer), "keyVersion": p.KeyVersion, "beneficiaryVersion": p.BeneficiaryVersion,
		"maxBaseUnits": "30000000", "presentationKey": hex.EncodeToString(s.presentation),
	}, hex.EncodeToString(fingerprint[:])
}

func hexAddress(value common.Address) string {
	return "0x" + hex.EncodeToString(value[:])
}

func (s *installedSignedMenu) capture(quote *quoteReply, lost bool, buyer string) bool {
	if quote == nil || quote.Presentation == nil || quote.Presentation.Customer != buyer ||
		quote.Digest == "" || quote.Signed == "" || quote.SaleRef == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if lost {
		s.initial = *quote
		return true
	}
	s.retrieved = s.initial.Digest == quote.Digest && s.initial.Signed == quote.Signed &&
		s.initial.SaleRef == quote.SaleRef && s.initial.OrderKey == quote.OrderKey &&
		s.initial.ExpiresAt == quote.ExpiresAt && sameInstalledPresentation(s.initial.Presentation, quote.Presentation)
	return s.retrieved
}

func sameInstalledPresentation(first, second *Presentation) bool {
	left, right := *first, *second
	left.GasSponsor, right.GasSponsor = nil, nil
	if !reflect.DeepEqual(left, right) {
		return false
	}
	if first.GasSponsor == nil || second.GasSponsor == nil {
		return first.GasSponsor == second.GasSponsor
	}
	return first.GasSponsor.Grant == second.GasSponsor.Grant && first.GasSponsor.Account == second.GasSponsor.Account
}

func (s *installedSignedMenu) assertLedger(t *testing.T, ctx context.Context, db *pgx.Conn, branch, stateExpected string) {
	t.Helper()
	s.mu.Lock()
	quote, retrieved := s.initial, s.retrieved
	s.mu.Unlock()
	effects, admissions := 0, 0
	if stateExpected == "paid" {
		effects, admissions = 1, 1
	} else if stateExpected == "closed" {
		admissions = 1
	}
	if !retrieved || s.signer.calls.Load() != 1 || s.forwards.Load() != int32(admissions) {
		t.Fatalf("quote replay/sign/forward mismatch: retrieved=%t sign=%d forward=%d",
			retrieved, s.signer.calls.Load(), s.forwards.Load())
	}
	var original, eventCount, paymentCount, relayCount, quoteCommands, lineCount, matchingLines int
	var submissions, journals, fulfillment int
	var state, amount, digest string
	if err := db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ledger.menu_reservation_outcome WHERE branch_id=$1 AND sale_id=$2),
		(SELECT count(*) FROM ledger.quote_outbox WHERE sale_id=$2),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$2),
		`+countOrderRelayAttempts+`,
		(SELECT count(*) FROM ledger.command_receipt WHERE branch_id=$1 AND result_ref=$2 AND operation='issue_quote'),
		(SELECT count(*) FROM ledger.order_line WHERE sale_id=$2),
		(SELECT count(*) FROM ledger.order_line WHERE sale_id=$2 AND sku='tea' AND quantity=1
			AND (option_key='hot' AND price=7000000 OR option_key='cold' AND price=8000000)),
		(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$2),
		(SELECT count(*) FROM ledger.journal WHERE sale_id=$2),
		(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$2),
		s.state,s.amount::text,encode(q.signed_digest,'hex')
		FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id WHERE s.id=$2 AND s.branch_id=$1`,
		branch, quote.SaleRef).Scan(&original, &eventCount, &paymentCount, &relayCount, &quoteCommands,
		&lineCount, &matchingLines, &submissions, &journals, &fulfillment, &state, &amount, &digest); err != nil ||
		state != stateExpected || amount != "15000000" || digest != quote.Digest ||
		original != 1 || eventCount != 1 || quoteCommands != 1 || paymentCount != effects || relayCount != effects ||
		submissions != admissions || journals != effects || fulfillment != effects ||
		lineCount != 2 || matchingLines != 2 {
		t.Fatalf("signed installed ledger mismatch original=%d events=%d issue commands=%d payments=%d relay=%d lines=%d/%d state=%s amount=%s error=%v",
			original, eventCount, quoteCommands, paymentCount, relayCount, matchingLines, lineCount, state, amount, err)
	}
	if stateExpected == "paid" {
		t.Log("installed W10 SW1: explicit submission, one payment, one relay, one journal and one fulfillment event")
	}
	t.Logf("installed W10 ledger proof: original=%d signed=%d outbox=%d issue commands=%d payments=%d relay=%d", original, s.signer.calls.Load(),
		eventCount, quoteCommands, paymentCount, relayCount)
}
