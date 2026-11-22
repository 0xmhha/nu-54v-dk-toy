package settlement

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/jackc/pgx/v5/pgxpool"
)

const nearbyCafeAmount = "8000000"

func cafePaymentProfile(profile commerce.Profile, presentation ed25519.PublicKey) map[string]any {
	return map[string]any{
		"chain": profile.Chain, "contract": lower(profile.Contract), "asset": lower(profile.Asset),
		"beneficiary": lower(profile.Beneficiary), "merchantId": rawHex(profile.MerchantID[:]),
		"signer": lower(profile.Signer), "keyVersion": profile.KeyVersion, "beneficiaryVersion": profile.BeneficiaryVersion,
		"maxBaseUnits": "30000000", "presentationKey": rawHex(presentation),
	}
}

func admitNearbyCafe(t *testing.T, ctx context.Context, db *pgxpool.Pool, server *httptest.Server,
	terminalCert tls.Certificate, terminal, otherCustomer string, base seededOrder, profile commerce.Profile, presentation ed25519.PublicKey, payer fixtureSigner) seededOrder {
	t.Helper()
	privateKey, err := x509.MarshalPKCS8PrivateKey(terminalCert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	origin, requestKey := fixtureUUID(t, db), fixtureUUID(t, db)
	wire := map[string]any{
		"endpoint": server.URL + "/v2/commerce",
		"ca":       string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
		"cert":     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: terminalCert.Certificate[0]})),
		"key":      string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey})),
		"tenant":   base.Tenant, "branch": base.Branch, "customer": base.Actor, "otherCustomer": otherCustomer, "terminal": terminal,
		"origin": origin, "reserveKey": requestKey, "sale": "",
		"profile": cafePaymentProfile(profile, presentation),
	}
	call := func() pilotQuote {
		t.Helper()
		input, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, "node", "--experimental-transform-types", "../../test/settlement/k05-nearby.mts")
		cmd.Stdin = bytes.NewReader(input)
		var diagnostic bytes.Buffer
		cmd.Stderr = &diagnostic
		out, err := cmd.Output()
		var result pilotQuote
		if err != nil || json.Unmarshal(out, &result) != nil {
			t.Fatalf("nearby C2 fixture stopped before returning the original quote: %v\n%s", err, diagnostic.String())
		}
		return result
	}
	q := call()
	base.Sale = q.SaleRef
	digest, err := hex.DecodeString(q.Digest)
	if err != nil {
		t.Fatal(err)
	}
	base.Signed, err = hex.DecodeString(q.Signed)
	if err != nil {
		t.Fatal(err)
	}
	base.Quote, err = unpackQuote(base.Signed, digest, profile.Signer.Bytes(), base.Target)
	if err != nil || q.BranchRef != base.Branch || q.Presentation == nil ||
		q.Presentation.Customer != base.Actor || q.Presentation.Origin != origin ||
		base.Quote.Args.Payer != payer.Address() || base.Quote.Args.Amount.String() != nearbyCafeAmount ||
		!validPilotPresentation(&q, base.Quote.Args.PayerGeneration) {
		t.Fatalf("nearby C2 quote verification failed: %v", err)
	}
	base.Order = commerce.QuoteOrder{
		Payer: payer.Address(), Amount: nearbyCafeAmount, OrderID: base.Quote.Args.OrderId,
		Issued: time.Unix(int64(base.Quote.Args.IssuedAt), 0), Expires: time.Unix(q.Expires, 0),
	}
	base.Presentation = q.Presentation
	var exact bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM ledger.menu_reservation_outcome
		WHERE branch_id=$1 AND sale_id=$2 AND origin_ref=$3 AND nearby_customer=$4)`,
		base.Branch, base.Sale, origin, base.Actor).Scan(&exact); err != nil || !exact {
		t.Fatalf("nearby C2 admission did not durably bind the original cart: %v", err)
	}
	approval := pilotBoardApproval(t, ctx, base, payer, "payment")
	wire["sale"] = base.Sale
	wire["forward"] = map[string]string{"key": fixtureUUID(t, db), "signature": rawHex(approval)}
	again := call()
	if again.Signed != q.Signed || again.Digest != q.Digest {
		t.Fatal("reconnecting C2 issued a different quote")
	}
	return base
}
