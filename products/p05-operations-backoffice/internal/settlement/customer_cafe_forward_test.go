package settlement

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
)

func TestPostgresCustomerCafeForwardReceipt(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := context.Background()
	buyer := fixtureSigner(102)
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x2222")}
	issued := time.Now().UTC().Truncate(time.Second).Add(-time.Second)
	cafe := seedPayerPreview(t, db, target, "2000000", buyer)
	fixtureExec(t, db, "UPDATE ledger.sale SET origin='menu' WHERE id=$1", cafe.Sale)
	cafe = seedPayerIssue(t, db, cafe, issued, "2000000", 98, 1, 120*time.Second)
	merchant := fixtureUUID(t, db)
	fixtureExec(t, db, `INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,terminal_mode,valid_until)
		VALUES($1,$2,'terminal','pos',now()+interval '1 hour')`, cafe.Branch, merchant)
	fixtureExec(t, db, `INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,expires_at,intent_digest)
		VALUES($1,$2,$3,$4,$5)`, cafe.Sale, cafe.Branch, merchant, cafe.Order.Expires, cafe.Order.Intent[:])
	fixtureExec(t, db, `INSERT INTO ledger.menu_release VALUES($1,1,now()-interval '1 minute',now()+interval '15 minutes')`, cafe.Branch)
	fixtureExec(t, db, `INSERT INTO ledger.price VALUES($1,1,'cafe','plain','Cafe',$2,2000000)`, cafe.Branch, lower(target.Asset))
	fixtureExec(t, db, `INSERT INTO ledger.stock VALUES($1,'cafe',1)`, cafe.Branch)
	fixtureExec(t, db, `INSERT INTO ledger.order_line VALUES($1,$2,1,'cafe','plain',1,2000000)`, cafe.Sale, cafe.Branch)
	fixtureExec(t, db, `INSERT INTO ledger.payer_enrollment VALUES($1,$2,$3,1,1,$4)`,
		cafe.Branch, cafe.Actor, buyer.Address().Bytes(), cafe.Order.Expires)
	legacy := seedPayerOrder(t, db, target, issued, "2000000", 99, buyer, 1, cafe)
	fixtureExec(t, db, `INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,expires_at,intent_digest)
		VALUES($1,$2,$3,$4,$5)`, legacy.Sale, legacy.Branch, merchant, legacy.Order.Expires, legacy.Order.Intent[:])
	for _, order := range []seededOrder{cafe, legacy} {
		fixtureExec(t, db, `INSERT INTO ledger.checkout_claim VALUES($1,$2,$3,$4,$5,1,1,$6)`,
			order.Sale, order.Branch, order.Actor, order.Order.Claim[:], buyer.Address().Bytes(), order.Order.Expires)
	}

	grant := [32]byte{31: 90}
	fixtureExec(t, db, `UPDATE ledger.quote_work SET gas_sponsor_digest=$2 WHERE sale_id=$1`, cafe.Sale, grant[:])
	var lookups atomic.Int32
	lookup := func(_ context.Context, branch string, profile commerce.Profile, payer common.Address, now time.Time) (commerce.GasSponsorship, bool) {
		lookups.Add(1)
		until := cafe.Order.Expires
		return commerce.GasSponsorship{Account: fixtureSigner(110).Address(), Grant: grant, ExpiresAt: until.Unix(),
				CurrentPublication: func(at time.Time) bool { return at.Before(until) }},
			branch == cafe.Branch && profile.Asset == target.Asset && payer == buyer.Address() && now.Before(until)
	}
	_, commerceDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	commerceDB, err := commerce.Open(ctx, commerceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer commerceDB.Close()
	settlementDB, err := Open(ctx, commerceDSN, "nu54_p05_commerce")
	if err != nil {
		t.Fatal(err)
	}
	defer settlementDB.Close()
	qCipher, err := commerce.NewCipher(cafe.CipherKey[:])
	if err != nil {
		t.Fatal(err)
	}
	relayKey := [32]byte{31: 95}
	relayCipher, err := commerce.NewCipher(relayKey[:])
	if err != nil {
		t.Fatal(err)
	}
	trust, certificates, _ := mtlsClients(t, 1)
	cert := certificates[0]
	mux := http.NewServeMux()
	commerce.HTTP{Store: commerceDB,
		Credentials: map[string]service.Principal{fingerprint(cert): {
			Role: "customer", Tenant: cafe.Tenant, Subject: cafe.Actor, ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}},
		Payment: Forwarder{Store: settlementDB, QuoteCipher: qCipher, RelayCipher: relayCipher, GasSponsors: lookup,
			Targets: map[string]Target{cafe.Branch: target}},
	}.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 7 * time.Second}

	type response struct {
		service.Reply
		Order *commerce.Order `json:"order"`
	}
	send := func(t *testing.T, command service.Request, signature string) response {
		t.Helper()
		envelope := struct {
			Command   service.Request `json:"command"`
			Signature string          `json:"approvalSignature,omitempty"`
		}{command, signature}
		raw, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.Post(server.URL+"/v2/commerce", "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var reply response
		if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
			t.Fatalf("HTTP %d: %v", res.StatusCode, err)
		}
		if res.StatusCode != http.StatusOK || reply.Status != "ok" || reply.ResultRef != command.Object || reply.RequestID != command.RequestID {
			t.Fatalf("HTTP %d with C3 status %s/%s, match sale %t, match request %t",
				res.StatusCode, reply.Status, reply.Error, reply.ResultRef == command.Object, reply.RequestID == command.RequestID)
		}
		return reply
	}
	command := func(order seededOrder, operation string, body map[string]string) service.Request {
		return service.Request{Version: 2, Contract: "C3", Operation: operation, Domain: order.Branch, Object: order.Sale,
			RequestID: fixtureUUID(t, db), ExpiresAt: time.Now().Add(30 * time.Second).Unix(), Body: body}
	}
	forward := func(t *testing.T, order seededOrder, key string) response {
		t.Helper()
		signature, err := buyer.SignDigest(ctx, order.Quote.ApprovalHash)
		if err != nil {
			t.Fatal(err)
		}
		req := command(order, "payment.forward", map[string]string{
			"quoteDigest": rawHex(order.Quote.Digest[:]), "approvalDigest": rawHex(order.Quote.ApprovalHash[:]),
		})
		req.IdempotencyKey = key
		return send(t, req, rawHex(signature))
	}
	check := func(t *testing.T, order seededOrder, key string, wantCafe bool) {
		t.Helper()
		view := send(t, command(order, "order.status", map[string]string{}), "").Order
		if view == nil || view.ID != order.Sale || view.State != "checking" ||
			view.Payment == nil || view.Payment.Stage != "received" || (view.CafeReview != nil) != wantCafe || view.MenuCheckout != nil {
			t.Fatal("customer status did not reopen the original order and payment receipt")
		}
		var receipts, submissions, paid int
		err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.command_receipt
			WHERE result_ref=$1 AND operation='payment_forward'),
			(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.payment WHERE sale_id=$1)
			WHERE EXISTS (SELECT FROM ledger.command_receipt
				WHERE result_ref=$1 AND operation='payment_forward' AND request_key=$2 AND scope_ref=$3)`, order.Sale, key, order.Actor).
			Scan(&receipts, &submissions, &paid)
		if err != nil || receipts != 1 || submissions != 1 || paid != 0 {
			t.Fatalf("command receipts %d, submissions %d, payments %d: %v", receipts, submissions, paid, err)
		}
	}

	key := fixtureUUID(t, db)
	var first response
	if t.Run("initial_sponsored_submission", func(t *testing.T) {
		first = forward(t, cafe, key)
		if lookups.Load() != 1 {
			t.Fatal("new admission did not consult the current publication once")
		}
		check(t, cafe, key, true)
	}) {
		t.Run("same_key_receipt", func(t *testing.T) {
			replay := forward(t, cafe, key)
			if first.ResultRef != replay.ResultRef {
				t.Fatal("same payment key resolved to a different sale")
			}
			if lookups.Load() != 1 {
				t.Fatal("receipt recovery unexpectedly depended on the sponsor publication")
			}
			check(t, cafe, key, true)
		})
	}
	t.Run("legacy_unsponsored_customer", func(t *testing.T) {
		key := fixtureUUID(t, db)
		forward(t, legacy, key)
		check(t, legacy, key, false)
	})
}
