package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// TestPostgresCustomerAccountProof walks the customer entrance against a migrated database: challenges, the three
// purposes, the session scope, one unpaid hold per account, and the pickup QR scan on the order's page.
func TestPostgresCustomerAccountProof(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") {
		t.Fatal("use disposable commerce database")
	}
	ctx, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	execute := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(query, err)
		}
	}
	role := "nu54_proof_" + strings.TrimPrefix(config.Database, "nu54_commerce_")
	execute("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	store, err := Open(ctx, config.ConnString()+" user="+role)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fresh := func() string { return uuidForCatalogFixture(t, ctx, admin) }
	shop, elsewhere, operator, stranger := fresh(), fresh(), fresh(), fresh()
	for _, b := range []string{shop, elsewhere} {
		execute("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", b, tenant)
		execute("INSERT INTO ledger.command_rule VALUES ($1,'proof test','fixture','dispose',interval '2 minutes')", b)
	}
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", operator, "proof_"+operator)
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", stranger, "proof_"+stranger)
	execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", shop, operator)
	execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", elsewhere, stranger)
	execute("INSERT INTO ledger.finance_grant VALUES ($1,$2,'pickup_manage',now()+interval '10 minutes')", shop, operator)
	execute("INSERT INTO ledger.finance_grant VALUES ($1,$2,'pickup_manage',now()+interval '10 minutes')", elsewhere, stranger)

	// Two operator certificates: one of this shop, one of another shop.
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(10 * time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	ca, _ := x509.ParseCertificate(caDER)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	credentials := map[string]service.Principal{}
	leaf := func(serial int64, subject string) tls.Certificate {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, key.Public(), caKey)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(der)
		credentials[hex.EncodeToString(digest[:])] = service.Principal{Tenant: tenant, Subject: subject, Role: "operator",
			ExpiresAt: time.Now().Add(10 * time.Minute).Unix()}
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	}
	ownCert, strangerCert := leaf(2, operator), leaf(3, stranger)

	asset := "0x" + strings.Repeat("2", 40)
	certified, entrance := http.NewServeMux(), http.NewServeMux()
	server := httptest.NewUnstartedServer(certified)
	defer server.Close()
	origin := "https://" + server.Listener.Addr().String()
	h := HTTP{Store: store, OperatorOrigin: origin, Credentials: credentials, Customer: &CustomerEntrance{},
		Profiles: map[string]Profile{shop: {Chain: 8283, Asset: common.HexToAddress(asset), Token: wonForOptions}}}
	h.Register(certified)
	h.RegisterCustomer(entrance)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	server.StartTLS()
	customers := httptest.NewServer(entrance) // the entrance asks for no client certificate
	defer customers.Close()

	staff := func(cert tls.Certificate) *http.Client {
		client := server.Client()
		transport := client.Transport.(*http.Transport).Clone()
		transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
		return &http.Client{Transport: transport}
	}
	call := func(client *http.Client, url string, header map[string]string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range header {
			req.Header.Set(k, v)
		}
		reply, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		out, _ := io.ReadAll(reply.Body)
		if reply.StatusCode != want {
			t.Fatalf("%s: got %d, want %d: %s", url, reply.StatusCode, want, out)
		}
		var decoded map[string]any
		_ = json.Unmarshal(out, &decoded)
		return decoded
	}
	operate := func(cert tls.Certificate, path string, body any, want int) map[string]any {
		t.Helper()
		return call(staff(cert), server.URL+path, map[string]string{"Origin": origin}, body, want)
	}

	// The menu: tea, five of it.
	operate(ownCert, "/v2/operations/catalog/"+shop+"/publish", map[string]any{"requestKey": fresh(), "version": 0,
		"expiresAt": time.Now().Add(time.Hour).UTC(), "rows": []operatorRow{{SKU: "tea", Option: "standard", Label: "tea", Asset: asset, DisplayPrice: "3000"}}}, 200)
	operate(ownCert, "/v2/operations/catalog/"+shop+"/stock", map[string]any{"requestKey": fresh(), "version": 1, "sku": "tea", "delta": 5}, 200)

	board, otherBoard := mustKey(t), mustKey(t)
	account := crypto.PubkeyToAddress(board.PublicKey)
	customer := http.DefaultClient
	challenge := func(purpose, branchRef, sale string) [32]byte {
		t.Helper()
		body := map[string]string{"purpose": purpose, "branchRef": branchRef}
		if sale != "" {
			body["saleRef"] = sale
		}
		got := call(customer, customers.URL+"/v2/customer/challenge", nil, body, 200)
		raw, err := hex.DecodeString(got["challenge"].(string))
		if err != nil || len(raw) != 32 {
			t.Fatal(got)
		}
		return [32]byte(raw)
	}
	prove := func(signer *ecdsa.PrivateKey, claimed common.Address, value [32]byte, want int) map[string]any {
		t.Helper()
		body := map[string]string{"challenge": hex.EncodeToString(value[:]), "account": strings.ToLower(claimed.Hex()),
			"signature": hex.EncodeToString(signAccountProof(t, signer, value)), "generation": "1"}
		return call(customer, customers.URL+"/v2/customer/proof", nil, body, want)
	}
	command := func(session, operation, domain, object string, fields map[string]string, cart *Cart, want int) map[string]any {
		t.Helper()
		key := ""
		if operation == "order.create" {
			key = fresh()
		}
		envelope := map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: operation, RequestID: fresh(),
			Domain: domain, Object: object, ExpiresAt: time.Now().Unix() + 30, IdempotencyKey: key, Body: fields}}
		if cart != nil {
			envelope["cart"] = cart
		}
		return call(customer, customers.URL+"/v2/customer/commerce", map[string]string{"Authorization": "Bearer " + session}, envelope, want)
	}
	order := func(session string, want int) map[string]any {
		t.Helper()
		ref := fresh()
		cart := Cart{Origin: "menu", OriginRef: ref, MenuVersion: 1, Items: []Item{{SKU: "tea", Option: "standard", Qty: 1}}}
		packed, _ := json.Marshal(cart)
		digest := sha256.Sum256(packed)
		return command(session, "order.create", shop, ref, map[string]string{"asset": asset, "quantity": "1",
			"intentDigest": hex.EncodeToString(digest[:])}, &cart, want)
	}

	// --- challenges: single use, expiry, the board's own signature only ---
	first := challenge("checkout", shop, "")
	prove(otherBoard, account, first, 400) // another key cannot speak for this account, and does not use the challenge up
	checkout := prove(board, account, first, 200)
	if checkout["subjectRef"] != CustomerSubject(account) || checkout["tenantRef"] != tenant || checkout["branchRef"] != shop ||
		checkout["purpose"] != "checkout" || checkout["session"] == nil {
		t.Fatal(checkout)
	}
	if got := prove(board, account, first, 400); got["error"] != string(service.Expired) {
		t.Fatal("a challenge was accepted twice", got)
	}
	late := challenge("checkout", shop, "")
	execute("UPDATE ledger.customer_challenge SET expires_at=now()-interval '1 second' WHERE challenge_hash=$1", hashOf(late[:]))
	if got := prove(board, account, late, 400); got["error"] != string(service.Expired) {
		t.Fatal("an expired challenge was accepted", got)
	}
	call(customer, customers.URL+"/v2/customer/challenge", nil, map[string]string{"purpose": "checkout", "branchRef": fresh()}, 400)

	// --- before payment: the session covers this branch, and one unpaid hold per account ---
	session := checkout["session"].(string)
	command(session, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 200)
	command(session, "catalog.read", elsewhere, elsewhere, map[string]string{"page": "0"}, nil, 404) // another branch
	command(session, "payer.status", shop, CustomerSubject(account), map[string]string{}, nil, 404)
	command(strings.Repeat("ab", 32), "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 404)
	sale := order(session, 200)["resultRef"].(string)
	if got := order(session, 409); got["error"] != string(service.CheckoutConflict) {
		t.Fatal("a second unpaid hold for one account", got)
	}
	// The same account proven again elsewhere is the same customer: still one hold.
	elsewhereSession := prove(board, account, challenge("checkout", elsewhere, ""), 200)["session"].(string)
	if elsewhereSession == session {
		t.Fatal("sessions are not fresh")
	}
	var holdsOf string
	if err := admin.QueryRow(ctx, "SELECT creator::text FROM ledger.order_checkout WHERE sale_id=$1", sale).Scan(&holdsOf); err != nil ||
		holdsOf != CustomerSubject(account) {
		t.Fatal("the order is not the proven account's", holdsOf, err)
	}

	// --- the order is paid (seeded): quote signed for the account, payment, pickup number, escrow ---
	execute("SET session_replication_role=replica")
	execute(`UPDATE ledger.sale SET state='paid' WHERE id=$1`, sale)
	execute(`INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,
		payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
		VALUES($1,$2,decode(md5(random()::text)||md5(random()::text),'hex'),decode(md5(random()::text)||md5(random()::text),'hex'),decode(md5(random()::text)||md5(random()::text),'hex'),decode(md5(random()::text)||md5(random()::text),'hex'),0,1,substring(decode(md5(random()::text)||md5(random()::text),'hex') FROM 1 FOR 20),
		$3,1,1,now(),now()+interval '1 minute',1,now(),'x',decode(md5(random()::text)||md5(random()::text),'hex'))`, sale, CustomerSubject(account), account[:])
	execute(`INSERT INTO ledger.payment(sale_id,stream_id,merchant_key,evidence_id,tx_hash,log_index,inclusion_hash,included_at,asset_ref,payer,
		payer_generation,payment_mode,principal) VALUES($1,gen_random_uuid(),'0x'||repeat('ab',32),gen_random_uuid(),'0x'||repeat('cd',32),0,
		'0x'||repeat('ef',32),now(),$2,$3,1,1,300000)`, sale, asset, strings.ToLower(account.Hex()))
	execute(`INSERT INTO ledger.pickup(sale_id,branch_id,business_day,number) VALUES($1,$2,current_date,7)`, sale, shop)
	execute(`INSERT INTO ledger.order_escrow(sale_id,contract_ref,confirmer_ref,deliver_by,confirm_window) VALUES($1,$2,$2,$3,172800)`,
		sale, "0x"+strings.Repeat("4", 40), time.Now().Add(24*time.Hour).Unix())
	execute(`WITH e AS (INSERT INTO ledger.fulfillment_outbox(sale_id,kind,source_ref,revision) VALUES($1,'paid',$1,1) RETURNING event_id,effect_key)
		INSERT INTO ledger.consumer_receipt(event_id,effect_key,payload_digest,accepted_epoch) SELECT event_id,effect_key,decode(md5(random()::text)||md5(random()::text),'hex'),repeat('0',64) FROM e`, sale)
	execute(`INSERT INTO ledger.travel_pickup_progress(sale_id,state,revision,updated_at) VALUES($1,'preparing',1,now())`, sale)
	execute("SET session_replication_role=origin")

	// --- after payment: only the paying account gets the order session, and it covers that order only ---
	used := challenge("order", shop, sale)
	if got := prove(otherBoard, crypto.PubkeyToAddress(otherBoard.PublicKey), used, 400); got["error"] != string(service.Mismatch) {
		t.Fatal("another board with a valid signature got the order", got)
	}
	prove(board, account, used, 400) // that challenge is used up
	prove(board, account, challenge("order", elsewhere, sale), 400)
	ordered := prove(board, account, challenge("order", shop, sale), 200)
	if ordered["saleRef"] != sale || ordered["purpose"] != "order" {
		t.Fatal(ordered)
	}
	orderSession := ordered["session"].(string)
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 200)
	command(orderSession, "order.status", shop, fresh(), map[string]string{}, nil, 404)
	command(orderSession, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 404)
	order(orderSession, 404)

	// --- pickup: only when ready, only the payer, one use, short-lived, this branch and this order ---
	if got := prove(board, account, challenge("pickup", shop, sale), 409); got["error"] != string(service.CheckoutConflict) {
		t.Fatal("a pickup token for an order that is not ready", got)
	}
	execute(`UPDATE ledger.travel_pickup_progress SET state='ready',revision=2 WHERE sale_id=$1`, sale)
	prove(otherBoard, crypto.PubkeyToAddress(otherBoard.PublicKey), challenge("pickup", shop, sale), 400)
	pickup := func() string {
		t.Helper()
		got := prove(board, account, challenge("pickup", shop, sale), 200)
		if got["session"] != nil || got["pickupToken"] == nil || int64(got["expiresAt"].(float64)) > time.Now().Add(61*time.Second).Unix() {
			t.Fatal(got)
		}
		return got["pickupToken"].(string)
	}
	scan := func(cert tls.Certificate, branchRef, saleRef, token string, want int) map[string]any {
		t.Helper()
		return operate(cert, "/v2/operations/pickup-scan/"+branchRef+"/"+saleRef, map[string]string{"token": token}, want)
	}
	expired := pickup()
	execute("UPDATE ledger.pickup_token SET expires_at=now()-interval '1 second' WHERE token_hash=$1", hashOfHex(t, expired))
	if got := scan(ownCert, shop, sale, expired, 400); got["error"] != string(service.Expired) {
		t.Fatal("an expired pickup token", got)
	}
	token := pickup()
	if got := scan(ownCert, shop, fresh(), token, 400); got["error"] != string(service.Mismatch) {
		t.Fatal("the QR of another order was accepted", got)
	}
	scan(strangerCert, elsewhere, sale, token, 400) // another branch's staff, on their own branch
	scan(strangerCert, shop, sale, token, 404)      // another branch's staff, on this branch
	call(staff(ownCert), server.URL+"/v2/operations/pickup-scan/"+shop+"/"+sale, map[string]string{"Origin": "https://elsewhere.example"},
		map[string]string{"token": token}, 404) // not the operations web
	execute(`UPDATE ledger.travel_pickup_progress SET state='preparing' WHERE sale_id=$1`, sale)
	if got := scan(ownCert, shop, sale, token, 409); got["error"] != string(service.CheckoutConflict) {
		t.Fatal("handed over an order that is not ready", got)
	}
	execute(`UPDATE ledger.travel_pickup_progress SET state='ready' WHERE sale_id=$1`, sale)
	handed := scan(ownCert, shop, sale, token, 200)
	items, _ := handed["items"].([]any)
	if handed["saleRef"] != sale || handed["pickupNo"] != float64(7) || len(items) != 1 || items[0].(map[string]any)["label"] != "tea" {
		t.Fatal(handed)
	}
	if got := scan(ownCert, shop, sale, token, 400); got["error"] != string(service.Expired) {
		t.Fatal("a pickup token was used twice", got)
	}
	var state string
	var delivers, payouts int
	if err := admin.QueryRow(ctx, `SELECT (SELECT state FROM ledger.travel_pickup_progress WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.escrow_action WHERE sale_id=$1 AND kind='deliver'),
		(SELECT count(*) FROM ledger.escrow_action WHERE sale_id=$1 AND kind IN ('claim','receipt','resolve'))
			+(SELECT count(*) FROM ledger.order_escrow_event WHERE sale_id=$1 AND kind='released')`, sale).Scan(&state, &delivers, &payouts); err != nil ||
		state != "collected" || delivers != 1 || payouts != 0 {
		t.Fatalf("scan recorded %q, %d delivery marks and %d payouts: %v", state, delivers, payouts, err)
	}

	// --- the order session ends with the order ---
	execute("UPDATE ledger.order_escrow SET settled=true WHERE sale_id=$1", sale)
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 404)
}

func mustKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func hashOf(value []byte) []byte {
	sum := sha256.Sum256(value)
	return sum[:]
}

func hashOfHex(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return hashOf(raw)
}
