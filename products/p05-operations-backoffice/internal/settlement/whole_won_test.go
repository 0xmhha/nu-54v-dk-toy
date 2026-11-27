package settlement

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
)

// An operator's refund request at a won branch is whole won (owner decision 2026-11-27). The one exception is all
// that is left of a payment that was stored with a part of a won: more than was paid never goes back. A dollar
// branch takes every base unit, as before.
//
// A won token is enrolled only with OrderEscrow, which gives a whole order back and takes no such request. The won
// branch here pays through SettlementV2 so that the rule is exercised where a request can be made at all.
func TestPostgresWonRefundRequestIsWholeWon(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := t.Context()
	var asset string
	if err := db.QueryRow(ctx, "SELECT '0x'||substr(md5(random()::text)||md5(random()::text),1,40)").Scan(&asset); err != nil {
		t.Fatal(err)
	}
	wonTarget := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress(asset), Contract: common.HexToAddress("0x2222"), Manifest: strings.Repeat("a", 64)}
	dollarTarget := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x2222"), Manifest: strings.Repeat("a", 64)}
	issued := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	paid := func(target Target, amount string, nonce byte, branch ...seededOrder) seededOrder {
		order := seedPayerOrder(t, db, target, issued, amount, nonce, fixtureSigner(102), 1, branch...)
		fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),'0x'||encode(sha256(random()::text::bytea),'hex'),0,$4,$5,$6,
			$7::text::numeric,$8,1,1,$9)`, order.Sale, order.Quote.Digest[:], target.Stream, (common.Hash{31: 13}).Hex(), issued.Add(time.Second),
			lower(target.Asset), amount, lower(order.Order.Payer), common.Hash(order.Quote.Args.MerchantId).Hex())
		return order
	}
	whole := paid(wonTarget, "3333300", 141)         // 33,333 won
	stored := paid(wonTarget, "3333350", 142, whole) // 33,333.50 won, from before the rule
	dollars := paid(dollarTarget, "3333350", 143)    // 3.33335 dollars
	fixtureExec(t, db, "SELECT ledger.declare_whole_amount($1,100)", asset)
	t.Cleanup(func() { fixtureExec(t, db, "DELETE FROM ledger.whole_amount_asset WHERE asset_ref=$1", asset) })
	staff := fixtureUUID(t, db)
	for _, branch := range []string{whole.Branch, dollars.Branch} {
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'refund_request',now()+interval '1 hour')", branch, staff)
	}
	_, financeDSN := testLogin(t, db, dsn, "nu54_p05_finance")
	store, err := Open(ctx, financeDSN, "nu54_p05_finance")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	trust, certificates, _ := mtlsClients(t, 2)
	identities := map[string]service.Principal{
		fingerprint(certificates[0]): {Tenant: whole.Tenant, Subject: staff, Role: "finance", ExpiresAt: time.Now().Add(time.Hour).Unix()},
		fingerprint(certificates[1]): {Tenant: dollars.Tenant, Subject: staff, Role: "finance", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}
	const origin = "https://console.example.invalid"
	wonTarget.Token = commerce.Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}
	mux := http.NewServeMux()
	FinanceHTTP{Store: store, Credentials: identities, OperatorOrigin: origin, Funds: &refundFinanceProbe{},
		Targets: map[string]Target{whole.Branch: wonTarget, dollars.Branch: dollarTarget}}.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	reserve := func(who int, order seededOrder, amount string) int {
		t.Helper()
		cfg := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		cfg.Certificates = []tls.Certificate{certificates[who]}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 5 * time.Second}
		defer client.CloseIdleConnections()
		wire, _ := json.Marshal(map[string]string{"branchId": order.Branch, "saleId": order.Sale, "idempotencyKey": fixtureUUID(t, db),
			"amount": amount, "reason": "partial_service"})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v2/finance/refund-reserve", bytes.NewReader(wire))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode
	}
	for _, c := range []struct {
		name   string
		who    int
		order  seededOrder
		amount string
		want   int
	}{
		{"16,666.50 won", 0, whole, "1666650", 400},
		{"16,667 won", 0, whole, "1666700", 200},
		{"half a won that is not the rest", 0, stored, "50", 400},
		{"33,334 won of 33,333.50 paid", 0, stored, "3333400", 409},
		{"33,333 won of 33,333.50 paid", 0, stored, "3333300", 200},
		{"the half won that is left", 0, stored, "50", 200},
		{"a dollar amount to the base unit", 1, dollars, "1666651", 200},
	} {
		if got := reserve(c.who, c.order, c.amount); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
	var held string
	if err := db.QueryRow(ctx, "SELECT string_agg(reserved::text,',' ORDER BY principal) FROM ledger.payment WHERE sale_id IN ($1,$2)",
		whole.Sale, stored.Sale).Scan(&held); err != nil || held != "1666700,3333350" {
		t.Errorf("held for refunds %q: %v", held, err)
	}
}
