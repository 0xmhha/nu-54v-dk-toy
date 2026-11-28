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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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

	// A customer's menu read is a statement of milliseconds, and nothing may make the server treat it as a heavy one:
	// opsd's connections run without JIT, and the functions that answer one page say that they return one row, so
	// the plan of a read is not costed a thousand times over (migration 125). Either alone keeps the read from being
	// compiled; a first read that was compiled on a busy host ran into the statement timeout.
	var jit, timeout string
	if err := store.db.QueryRow(ctx, "SELECT current_setting('jit'), current_setting('statement_timeout')").Scan(&jit, &timeout); err != nil || jit != "off" || timeout != "7s" {
		t.Fatalf("the commerce pool runs with jit %q and statement_timeout %q: %v", jit, timeout, err)
	}
	var planned int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='ledger'
		AND p.proname IN ('catalog_category_window','catalog_menu_window','catalog_menu_photo_window','public_menu_window') AND p.proretset AND p.prorows=1`).Scan(&planned); err != nil || planned != 4 {
		t.Fatalf("%d of the 4 menu page functions are planned as one row: %v", planned, err)
	}

	fresh := func() string { return uuidForCatalogFixture(t, ctx, admin) }
	shop, elsewhere, operator, stranger, colleague := fresh(), fresh(), fresh(), fresh(), fresh()
	for _, b := range []string{shop, elsewhere} {
		execute("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", b, tenant)
		execute("INSERT INTO ledger.command_rule VALUES ($1,'proof test','fixture','dispose',interval '2 minutes')", b)
	}
	// A store takes customers only while its page is published (migration 126), so the shop has one from the start.
	publish := func(branch, name string) {
		t.Helper()
		execute("INSERT INTO ledger.operator_branch_label VALUES ($1,$2)", branch, name)
		execute(`INSERT INTO ledger.branch_store_page VALUES ($1,true,false,'카페','','','','','','','',NULL,$2,now())`, branch, operator)
	}
	publish(shop, "여울 커피 "+shop[:8])
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", operator, "proof_"+operator)
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", stranger, "proof_"+stranger)
	execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", shop, operator)
	execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", elsewhere, stranger)
	execute("INSERT INTO ledger.finance_grant VALUES ($1,$2,'pickup_manage',now()+interval '10 minutes')", shop, operator)
	execute("INSERT INTO ledger.finance_grant VALUES ($1,$2,'pickup_manage',now()+interval '10 minutes')", elsewhere, stranger)
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", colleague, "proof_"+colleague)
	execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", shop, colleague)
	execute("INSERT INTO ledger.finance_grant VALUES ($1,$2,'pickup_manage',now()+interval '10 minutes')", shop, colleague)

	// Three operator certificates: two of this shop, one of another shop.
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
	ownCert, strangerCert, colleagueCert := leaf(2, operator), leaf(3, stranger), leaf(4, colleague)
	// A pickup terminal of this shop with its assignment: the certificate that hands orders over at the counter's own
	// device. It does not scan a pickup QR.
	counter := fresh()
	execute("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES($1,$2,'terminal','menu',now()+interval '10 minutes')", shop, counter)
	execute("INSERT INTO ledger.travel_pickup_terminal VALUES ($1,$2)", shop, counter)
	counterCert := leaf(5, counter)
	counterDigest := sha256.Sum256(counterCert.Certificate[0])
	credentials[hex.EncodeToString(counterDigest[:])] = service.Principal{Tenant: tenant, Subject: counter, Role: "terminal", ExpiresAt: time.Now().Add(10 * time.Minute).Unix()}

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
	// The board's registration number, as the phone reads it from the board and sends it with a checkout proof.
	const boardGeneration = "7"
	prove := func(signer *ecdsa.PrivateKey, claimed common.Address, value [32]byte, want int) map[string]any {
		t.Helper()
		body := map[string]string{"challenge": hex.EncodeToString(value[:]), "account": strings.ToLower(claimed.Hex()),
			"signature": hex.EncodeToString(signAccountProof(t, signer, value)), "generation": boardGeneration}
		return call(customer, customers.URL+"/v2/customer/proof", nil, body, want)
	}
	command := func(session, operation, domain, object string, fields map[string]string, cart *Cart, want int) map[string]any {
		t.Helper()
		key := ""
		if operation == "order.create" || operation == "checkout.authorize" {
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
	// A challenge is kept nowhere until it is used, so there is no row to age: the entrance's clock is put past the
	// challenge's two minutes instead. The database refuses a challenge past its time by its own clock as well.
	late := challenge("checkout", shop, "")
	h.Customer.clock = func() time.Time { return time.Now().Add(121 * time.Second) }
	if got := prove(board, account, late, 400); got["error"] != string(service.Expired) {
		t.Fatal("an expired challenge was accepted", got)
	}
	h.Customer.clock = nil
	var taken bool
	if err := admin.QueryRow(ctx, "SELECT ledger.use_customer_challenge($1,'checkout',$2,NULL,clock_timestamp()-interval '1 second')",
		hashOf(late[:]), shop).Scan(&taken); err != nil || taken {
		t.Fatalf("the database took a challenge past its time: %v %v", taken, err)
	}
	var kept int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM ledger.customer_challenge WHERE branch_id=$1", shop).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("%d challenges of the store are kept after two were given and one was used: %v", kept, err)
	}
	call(customer, customers.URL+"/v2/customer/challenge", nil, map[string]string{"purpose": "checkout", "branchRef": fresh()}, 400)

	// --- before payment: the session covers this branch, and one unpaid hold per account ---
	session := checkout["session"].(string)
	// The same account's checkout session of the other branch, proven when a check first needs it.
	elsewhereOf := func() string {
		t.Helper()
		return prove(board, account, challenge("checkout", elsewhere, ""), 200)["session"].(string)
	}
	command(session, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 200)
	command(session, "catalog.read", elsewhere, elsewhere, map[string]string{"page": "0"}, nil, 404) // another branch
	command(session, "payer.status", shop, CustomerSubject(account), map[string]string{}, nil, 404)
	command(strings.Repeat("ab", 32), "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 404)

	// --- the menu is public: read without a proof, the rows a session reads, and nothing held or made by reading ---
	read := func(path string, want int) map[string]any {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, "GET", customers.URL+path, nil)
		reply, err := customer.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		out, _ := io.ReadAll(reply.Body)
		if reply.StatusCode != want {
			t.Fatalf("%s: got %d, want %d: %s", path, reply.StatusCode, want, out)
		}
		var decoded map[string]any
		_ = json.Unmarshal(out, &decoded)
		return decoded
	}
	picture, small := bytes.Repeat([]byte{0x5a}, 20_000), bytes.Repeat([]byte{0x3c}, 900)
	photo, thumb := hex.EncodeToString(hashOf(picture)), hex.EncodeToString(hashOf(small))
	execute("INSERT INTO ledger.menu_category VALUES ($1,1,'warm',1,'{\"ko\":\"음료\",\"en\":\"Drinks\"}')", shop)
	execute("INSERT INTO ledger.menu_sku_category VALUES ($1,1,'tea','warm')", shop)
	execute("INSERT INTO ledger.menu_photo(branch_id,digest,thumb_digest,photo,thumb,uploaded_by,rights_confirmed_at) VALUES ($1,$2,$3,$4,$5,$6,now())",
		shop, photo, thumb, picture, small, operator)
	execute("INSERT INTO ledger.menu_sku_photo VALUES ($1,1,'tea',$2)", shop, photo)
	footprint := func() string {
		t.Helper()
		var text string
		if err := admin.QueryRow(ctx, `SELECT concat_ws('/',(SELECT available FROM ledger.stock WHERE branch_id=$1 AND sku='tea'),
			(SELECT count(*) FROM ledger.order_checkout WHERE branch_id=$1),(SELECT count(*) FROM ledger.sale WHERE branch_id=$1),
			(SELECT count(*) FROM ledger.commerce_grant WHERE branch_id=$1),(SELECT count(*) FROM ledger.customer_account),
			(SELECT count(*) FROM ledger.customer_session),(SELECT count(*) FROM ledger.customer_challenge))`, shop).Scan(&text); err != nil {
			t.Fatal(err)
		}
		return text
	}
	// A registered name and a published menu open nothing: the store has to publish its store page first. Until
	// then the menu and the photos answer like a store that does not exist, to a session's customer too.
	storePage := func(published bool) {
		t.Helper()
		operate(ownCert, "/v2/operations/store-page/"+shop, map[string]any{"published": published, "listed": false, "kind": "카페", "intro": "",
			"address": "", "directions": "", "phone": "", "amenities": "", "parking": "", "closedNote": "", "hours": nil}, 200)
	}
	// Without a published page the store takes no customer either: the session that was given while it had one is
	// not live, and a new challenge is answered like one for a branch that does not exist.
	nowhere := call(customer, customers.URL+"/v2/customer/challenge", nil, map[string]string{"purpose": "checkout", "branchRef": fresh()}, 400)
	for _, closed := range []func(){func() { execute("DELETE FROM ledger.branch_store_page WHERE branch_id=$1", shop) }, func() { storePage(false) }} {
		closed()
		for _, path := range []string{"", "/menu/0", "/photos/" + photo + "/0", "/photos/" + thumb + "/0"} {
			read("/v2/customer/stores/"+shop+path, 404)
		}
		command(session, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 404)
		if got := call(customer, customers.URL+"/v2/customer/challenge", nil, map[string]string{"purpose": "checkout", "branchRef": shop}, 400); !reflect.DeepEqual(got, nowhere) {
			t.Fatalf("a store that is not public answers a challenge %v, a branch that does not exist %v", got, nowhere)
		}
	}
	storePage(true)
	before := footprint()
	menuKeys := []string{"catalog", "catalogVersion", "catalogOptions", "catalogPhotos", "shop", "orderState"}
	// The public menu is the session's menu, but for the counts of stock: it says only sold out or not.
	sameMenu := func(when string) map[string]any {
		t.Helper()
		public, proven := read("/v2/customer/stores/"+shop+"/menu/0", 200), command(session, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 200)
		for key := range public {
			if !slices.Contains(menuKeys, key) {
				t.Fatalf("%s: the public menu gives out %q: %v", when, key, public)
			}
		}
		for _, item := range proven["catalog"].([]any) {
			for _, count := range []string{"available", "skuAvailable"} {
				if row := item.(map[string]any); row[count] != float64(0) {
					row[count] = float64(1000)
				}
			}
		}
		for _, key := range menuKeys {
			if !reflect.DeepEqual(public[key], proven[key]) {
				t.Fatalf("%s: %s differs\npublic: %v\nsession: %v", when, key, public[key], proven[key])
			}
		}
		return public
	}
	menu := sameMenu("published")
	row := menu["catalog"].([]any)[0].(map[string]any)
	counted := command(session, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 200)["catalog"].([]any)[0].(map[string]any)
	if counted["available"] != float64(5) || counted["skuAvailable"] != float64(5) {
		t.Fatalf("a session does not read the counts: %v", counted)
	}
	if raw := mustJSON(t, menu); strings.Contains(raw, ":5,") || strings.Contains(raw, ":5}") {
		t.Fatalf("the public menu tells a count of stock: %s", raw)
	}
	if len(menu["catalog"].([]any)) != 1 || row["sku"] != "tea" || row["available"] != float64(1000) || row["skuAvailable"] != float64(1000) || row["unitPrice"] != "300000" || row["category"] == nil ||
		menu["shop"].(map[string]any)["branchLabel"] != "여울 커피 "+shop[:8] || menu["orderState"].(map[string]any)["mode"] != "open" ||
		len(menu["catalogPhotos"].([]any)) != 1 || menu["catalogPhotos"].([]any)[0].(map[string]any)["thumb"] != thumb {
		t.Fatalf("public menu: %v", menu)
	}
	// What the store marks sold out, and that it stopped taking orders, read the same without a proof.
	execute("INSERT INTO ledger.menu_sold_out VALUES ($1,'tea',$2,now())", shop, operator)
	execute("INSERT INTO ledger.branch_order_state(branch_id,mode,busy_minutes,changed_by,changed_at) VALUES ($1,'busy',20,$2,now())", shop, operator)
	stopped := sameMenu("sold out and busy")
	if stopped["catalog"].([]any)[0].(map[string]any)["available"] != float64(0) || stopped["orderState"].(map[string]any)["mode"] != "busy" {
		t.Fatalf("sold out and busy: %v", stopped)
	}
	execute("DELETE FROM ledger.menu_sold_out WHERE branch_id=$1", shop)
	execute("DELETE FROM ledger.branch_order_state WHERE branch_id=$1", shop)
	// A count that ran out reads as sold out as well, and the last one left reads like any other.
	for left, want := range map[int]float64{0: 0, 1: 1000} {
		execute("UPDATE ledger.stock SET available=$2 WHERE branch_id=$1 AND sku='tea'", shop, left)
		if row := sameMenu("stock left")["catalog"].([]any)[0].(map[string]any); row["available"] != want || row["skuAvailable"] != want {
			t.Fatalf("%d left reads %v", left, row)
		}
	}
	execute("UPDATE ledger.stock SET available=5 WHERE branch_id=$1 AND sku='tea'", shop)
	// A store that takes no order now, because it paused or today is its closed day, still shows its menu to read
	// (migration 128): the same rows, on sale or sold out and never a count, with the state that says why no order
	// is taken. Ordering stays closed: the session's own menu has no row to order then, and a cart is not held.
	for _, stop := range []struct{ mode, set, clear string }{
		{"paused", "INSERT INTO ledger.branch_order_state(branch_id,mode,resume_note,changed_by,changed_at) VALUES ($1,'paused','15:30',$2,now())",
			"DELETE FROM ledger.branch_order_state WHERE branch_id=$1"},
		{"closed", "INSERT INTO ledger.branch_closed_day VALUES ($1,ledger.branch_today($1,clock_timestamp()),$2,now())",
			"DELETE FROM ledger.branch_closed_day WHERE branch_id=$1"},
	} {
		execute(stop.set, shop, operator)
		shown := read("/v2/customer/stores/"+shop+"/menu/0", 200)
		for key := range shown {
			if !slices.Contains(menuKeys, key) {
				t.Fatalf("%s: the public menu gives out %q: %v", stop.mode, key, shown)
			}
		}
		rows, _ := shown["catalog"].([]any)
		if len(rows) != 1 || !reflect.DeepEqual(rows[0], row) || shown["orderState"].(map[string]any)["mode"] != stop.mode ||
			shown["catalogVersion"] != menu["catalogVersion"] || !reflect.DeepEqual(shown["catalogPhotos"], menu["catalogPhotos"]) {
			t.Fatalf("the menu of a store that is %s: %v", stop.mode, shown)
		}
		if raw := mustJSON(t, shown); strings.Contains(raw, ":5,") || strings.Contains(raw, ":5}") {
			t.Fatalf("the menu of a store that is %s tells a count of stock: %s", stop.mode, raw)
		}
		execute("UPDATE ledger.stock SET available=0 WHERE branch_id=$1 AND sku='tea'", shop)
		if gone := read("/v2/customer/stores/"+shop+"/menu/0", 200)["catalog"].([]any)[0].(map[string]any); gone["available"] != float64(0) || gone["skuAvailable"] != float64(0) {
			t.Fatalf("a sold out row of a store that is %s reads %v", stop.mode, gone)
		}
		execute("UPDATE ledger.stock SET available=5 WHERE branch_id=$1 AND sku='tea'", shop)
		proven := command(session, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 200)
		if held, _ := proven["catalog"].([]any); len(held) != 0 || mustJSON(t, proven["orderState"]) != mustJSON(t, shown["orderState"]) {
			t.Fatalf("the session's menu of a store that is %s offers rows to order, or says another state: %v", stop.mode, proven)
		}
		closed := footprint()
		if refused := order(session, 409); refused["error"] != string(service.CheckoutConflict) || footprint() != closed {
			t.Fatalf("a store that is %s held a cart: %v (%s, was %s)", stop.mode, refused, footprint(), closed)
		}
		execute(stop.clear, shop)
	}
	// A menu that is not on sale at this hour, or not on sale at all, has no row to show, as before.
	var onSaleFrom, onSaleUntil time.Time
	if err := admin.QueryRow(ctx, "SELECT published_at,expires_at FROM ledger.menu_release WHERE branch_id=$1", shop).Scan(&onSaleFrom, &onSaleUntil); err != nil {
		t.Fatal(err)
	}
	execute("SET session_replication_role=replica")
	execute("UPDATE ledger.menu_release SET published_at=$2,expires_at=$3 WHERE branch_id=$1", shop, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Minute))
	if none := read("/v2/customer/stores/"+shop+"/menu/0", 200); len(none["catalog"].([]any)) != 0 || none["catalogVersion"] != nil {
		t.Fatalf("a menu that ran out is shown: %v", none)
	}
	execute("UPDATE ledger.menu_release SET published_at=$2,expires_at=$3 WHERE branch_id=$1", shop, onSaleFrom, onSaleUntil)
	execute("SET session_replication_role=origin")
	// A page past the end is empty and says nothing more; a store without a store page has no public menu.
	if past := read("/v2/customer/stores/"+shop+"/menu/1", 200); len(past["catalog"].([]any)) != 0 || past["shop"] != nil || past["orderState"] != nil {
		t.Fatalf("page past the end: %v", past)
	}
	read("/v2/customer/stores/"+elsewhere+"/menu/0", 404)
	read("/v2/customer/stores/"+fresh()+"/menu/0", 404)
	// A photo comes in parts of at most 18,000 bytes, the same bytes a session reads; only a published one of this store.
	var whole []byte
	for offset := 0; offset < len(picture); {
		public := read(fmt.Sprintf("/v2/customer/stores/%s/photos/%s/%d", shop, photo, offset), 200)["photo"].(map[string]any)
		proven := command(session, "catalog.photo", shop, shop, map[string]string{"photo": photo, "offset": strconv.Itoa(offset)}, nil, 200)["photo"]
		part, _ := base64.StdEncoding.DecodeString(public["data"].(string))
		if !reflect.DeepEqual(map[string]any(public), proven) || public["size"] != float64(len(picture)) || len(part) == 0 || len(part) > 18_000 {
			t.Fatalf("photo part at %d: %v", offset, public)
		}
		whole, offset = append(whole, part...), offset+len(part)
	}
	if !bytes.Equal(whole, picture) {
		t.Fatal("the public photo is not the published one")
	}
	if small := read(fmt.Sprintf("/v2/customer/stores/%s/photos/%s/0", shop, thumb), 200)["photo"].(map[string]any); small["size"] != float64(900) {
		t.Fatalf("thumb: %v", small)
	}
	read(fmt.Sprintf("/v2/customer/stores/%s/photos/%s/%d", shop, photo, len(picture)), 400)    // past the end
	read(fmt.Sprintf("/v2/customer/stores/%s/photos/%s/0", shop, strings.Repeat("a", 64)), 400) // not a photo of this store
	read(fmt.Sprintf("/v2/customer/stores/%s/photos/%s/0", elsewhere, photo), 404)              // no store page, no photos
	// The store takes its page back: the menu and the photos close with it, and open again when it is published.
	storePage(false)
	read("/v2/customer/stores/"+shop+"/menu/0", 404)
	read(fmt.Sprintf("/v2/customer/stores/%s/photos/%s/0", shop, photo), 404)
	command(session, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 404) // the checkout session closes with the page
	storePage(true)
	read("/v2/customer/stores/"+shop+"/menu/0", 200)
	// Reading held no stock and made no order, customer, session or challenge.
	if after := footprint(); after != before {
		t.Fatalf("the public reads left a mark: %s, was %s", after, before)
	}
	// Without a session the cart, the claim, the quote and the status stay out of reach.
	for _, operation := range []string{"order.create", "checkout.claim", "checkout.authorize", "quote.issue", "order.lock", "order.status"} {
		envelope := map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: operation, RequestID: fresh(),
			Domain: shop, Object: shop, ExpiresAt: time.Now().Unix() + 30, Body: map[string]string{}}}
		call(customer, customers.URL+"/v2/customer/commerce", nil, envelope, 404)
		call(customer, customers.URL+"/v2/customer/commerce", map[string]string{"Authorization": "Bearer "}, envelope, 404)
	}
	if after := footprint(); after != before {
		t.Fatalf("requests without a session left a mark: %s, was %s", after, before)
	}

	// --- a cart holds stock only when its proven account can pay it as the chain stands now ---
	tryOrder := func(session, ref, key string) (int, string, map[string]any) {
		t.Helper()
		cart := Cart{Origin: "menu", OriginRef: ref, MenuVersion: 1, Items: []Item{{SKU: "tea", Option: "standard", Qty: 1}}}
		packed, _ := json.Marshal(cart)
		digest := sha256.Sum256(packed)
		raw, _ := json.Marshal(map[string]any{"cart": cart, "command": service.Request{Version: 2, Contract: "C3", Operation: "order.create",
			RequestID: fresh(), Domain: shop, Object: ref, ExpiresAt: time.Now().Unix() + 30, IdempotencyKey: key,
			Body: map[string]string{"asset": asset, "quantity": "1", "intentDigest": hex.EncodeToString(digest[:])}}})
		req, _ := http.NewRequestWithContext(ctx, "POST", customers.URL+"/v2/customer/commerce", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+session)
		reply, err := customer.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		var decoded map[string]any
		_ = json.NewDecoder(reply.Body).Decode(&decoded)
		return reply.StatusCode, reply.Header.Get("Retry-After"), decoded
	}
	unheld := footprint()
	unread := func(when string) {
		t.Helper()
		status, retry, got := tryOrder(session, fresh(), fresh())
		if status != 503 || retry != "3" || got["status"] != "error" || got["error"] != string(service.Busy) || got["reason"] != "balance_unavailable" ||
			got["requestId"] == "" || got["required"] != nil || got["balance"] != nil {
			t.Fatalf("%s: %d, Retry-After %q, %v", when, status, retry, got)
		}
		if after := footprint(); after != unheld {
			t.Fatalf("%s: a refused cart left a mark: %s, was %s", when, after, unheld)
		}
	}
	// No chain reader, or none for this store: a cart is not held on trust, and the answer does not say "try again
	// shortly" about something that does not pass by waiting.
	closedOrders := func(when string) {
		t.Helper()
		status, retry, got := tryOrder(session, fresh(), fresh())
		if status != 409 || retry != "" || got["status"] != "error" || got["error"] != string(service.CheckoutConflict) || got["reason"] != "orders_closed" ||
			got["requestId"] == "" || got["required"] != nil || got["balance"] != nil || footprint() != unheld {
			t.Fatalf("%s: %d, Retry-After %q, %v", when, status, retry, got)
		}
	}
	closedOrders("no chain reader")
	funds := &fakeFunds{asset: common.HexToAddress(asset), err: service.Capability}
	h.Customer.Funds = funds
	closedOrders("no settlement target for the store")
	funds.err = fmt.Errorf("rpc down")
	unread("the chain cannot be read")
	funds.err, funds.balance = nil, big.NewInt(299_999) // the tea costs 300000 base units
	status, retry, short := tryOrder(session, fresh(), fresh())
	if status != 409 || retry != "" || short["error"] != string(service.CheckoutConflict) || short["reason"] != "balance_short" ||
		short["required"] != "300000" || short["balance"] != "299999" || short["requestDigest"] == "" || short["resultRef"] != nil {
		t.Fatalf("a balance one unit short: %d %v", status, short)
	}
	if asked := funds.asked[len(funds.asked)-1]; asked != shop+"/"+strings.ToLower(account.Hex()) || footprint() != unheld {
		t.Fatalf("asked %s; footprint %s, was %s", asked, footprint(), unheld)
	}
	// An account asks for a limited number of carts in a time window, whatever the balance says.
	h.Customer.HoldAttempts = NewRateLimiter(6, 1, nil)
	if status, _, got := tryOrder(session, fresh(), fresh()); status != 409 || got["reason"] != "balance_short" {
		t.Fatalf("the first cart of the window: %d %v", status, got)
	}
	asks := len(funds.asked)
	if status, retry, got := tryOrder(session, fresh(), fresh()); status != 429 || retry != "10" || got["error"] != string(service.Busy) ||
		got["reason"] != "hold_limit" || len(funds.asked) != asks {
		t.Fatalf("a second cart in the window: %d, Retry-After %q, %v", status, retry, got)
	}
	if counts := h.Customer.Limited.Drain(); counts["hold"] != 1 || len(counts) != 1 {
		t.Fatalf("refusals by kind: %v", counts)
	}
	h.Customer.HoldAttempts = nil
	// Enough, to the unit: the cart is held. The same request again reads its first answer without another look at
	// the balance, so a retry after the payment is not refused for the money that payment took.
	funds.balance = big.NewInt(300_000)
	firstRef, firstKey := fresh(), fresh()
	status, _, held := tryOrder(session, firstRef, firstKey)
	if status != 200 || held["resultRef"] == nil || footprint() == unheld {
		t.Fatalf("a cart its account can pay: %d %v", status, held)
	}
	funds.balance, asks = big.NewInt(0), len(funds.asked)
	if status, _, again := tryOrder(session, firstRef, firstKey); status != 200 || again["resultRef"] != held["resultRef"] || len(funds.asked) != asks {
		t.Fatalf("the same request again: %d %v (%d balance reads)", status, again, len(funds.asked)-asks)
	}
	if status, _, got := tryOrder(session, fresh(), fresh()); status != 409 || got["reason"] != "balance_short" || got["balance"] != "0" {
		t.Fatalf("a new cart of an emptied account: %d %v", status, got)
	}
	funds.balance = big.NewInt(300_000)

	// --- the customer takes the unpaid cart back, and can hold another at once ---
	cancelHold := func(session, saleRef string, want int) map[string]any {
		t.Helper()
		return call(customer, customers.URL+"/v2/customer/checkout/cancel", map[string]string{"Authorization": "Bearer " + session}, map[string]string{"saleRef": saleRef}, want)
	}
	firstCart := held["resultRef"].(string)
	if got := order(session, 409); got["error"] != string(service.CheckoutConflict) { // one unpaid cart at a time
		t.Fatal("a second cart while the first is held", got)
	}
	outsider := prove(otherBoard, crypto.PubkeyToAddress(otherBoard.PublicKey), challenge("checkout", shop, ""), 200)["session"].(string)
	publish(elsewhere, "건너편 "+elsewhere[:8]) // the other branch takes customers from here on
	away := elsewhereOf()
	holding := footprint()
	cancelHold(outsider, firstCart, 404) // another account's cart
	cancelHold(away, firstCart, 404)     // the same account's session of another branch
	cancelHold(session, fresh(), 404)    // no such order
	cancelHold(strings.Repeat("ab", 32), firstCart, 404)
	call(customer, customers.URL+"/v2/customer/checkout/cancel", map[string]string{"Authorization": "Bearer " + session}, map[string]string{"saleRef": "x"}, 400)
	call(customer, customers.URL+"/v2/customer/checkout/cancel", map[string]string{"Authorization": "Bearer " + session},
		map[string]string{"saleRef": firstCart, "branchRef": shop}, 400)
	if after := footprint(); after != holding {
		t.Fatalf("a refused cancel left a mark: %s, was %s", after, holding)
	}
	for i := 0; i < 2; i++ { // the same again is the same answer
		if got := cancelHold(session, firstCart, 200); got["status"] != "ok" || got["saleRef"] != firstCart || got["state"] != "closed" || len(got) != 3 {
			t.Fatalf("cancel %d: %v", i+1, got)
		}
	}
	var closed string
	var teaLeft, lines int
	if err := admin.QueryRow(ctx, `SELECT (SELECT state FROM ledger.sale WHERE id=$1),(SELECT available FROM ledger.stock WHERE branch_id=$2 AND sku='tea'),
		(SELECT count(*) FROM ledger.order_line WHERE sale_id=$1)`, firstCart, shop).Scan(&closed, &teaLeft, &lines); err != nil || closed != "closed" || teaLeft != 5 || lines != 1 {
		t.Fatalf("after the cancel the order is %q with %d tea in stock and %d lines kept: %v", closed, teaLeft, lines, err)
	}
	if own := command(session, "order.status", shop, firstCart, map[string]string{}, nil, 200)["order"].(map[string]any); own["state"] != "closed" {
		t.Fatalf("the cancelled cart reads %v", own)
	}

	// At once: the cancel freed the account's one hold.
	sale := order(session, 200)["resultRef"].(string)
	if got := order(session, 409); got["error"] != string(service.CheckoutConflict) {
		t.Fatal("a second unpaid hold for one account", got)
	}
	// The customer's claim on its own order: none yet, then the one the session authorizes. The quote is issued for
	// the claim's payer, so the generation of the quote is the one the phone sent with the checkout proof.
	if got := command(session, "checkout.claim", shop, sale, map[string]string{}, nil, 200); got["claim"] != nil {
		t.Fatal("a claim before the customer asked for one", got)
	}
	claimed := command(session, "checkout.authorize", shop, sale, map[string]string{"customerRef": CustomerSubject(account)}, nil, 200)["claim"]
	if read := command(session, "checkout.claim", shop, sale, map[string]string{}, nil, 200)["claim"]; claimed == nil ||
		read.(map[string]any)["claimDigest"] != claimed.(map[string]any)["claimDigest"] {
		t.Fatal("the session does not read back its own claim", claimed, read)
	}
	var claimPayer []byte
	var claimGeneration, enrolledGeneration int64
	if err := admin.QueryRow(ctx, `SELECT c.payer,c.payer_generation,e.payer_generation FROM ledger.checkout_claim c
		JOIN ledger.payer_enrollment e USING (branch_id,actor_id) WHERE c.sale_id=$1`, sale).Scan(&claimPayer, &claimGeneration, &enrolledGeneration); err != nil ||
		!bytes.Equal(claimPayer, account[:]) || strconv.FormatInt(claimGeneration, 10) != boardGeneration || claimGeneration != enrolledGeneration {
		t.Fatal("the claim is not for the proven account and the generation of its proof", claimPayer, claimGeneration, enrolledGeneration, err)
	}
	// An order that is not this account's reads like an order without a claim, as on the certificate route.
	if got := command(session, "checkout.claim", shop, fresh(), map[string]string{}, nil, 200); got["claim"] != nil {
		t.Fatal("a claim on an order of nobody", got)
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

	// --- once a quote was issued the cart is not taken back: it ends with that quote (seeded state) ---
	execute("SET session_replication_role=replica")
	execute(`UPDATE ledger.sale SET state='issued' WHERE id=$1`, sale)
	execute("SET session_replication_role=origin")
	quoted := footprint()
	if got := cancelHold(session, sale, 409); got["error"] != string(service.CheckoutConflict) || got["reason"] != "quote_issued" || footprint() != quoted {
		t.Fatalf("cancel after a quote was issued: %v (%s, was %s)", got, footprint(), quoted)
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
	// The ordered combination has a name of its own (seeded: the test menu has one plain row).
	execute(`INSERT INTO ledger.menu_sku_options(branch_id,version,sku,base,groups) VALUES($1,1,'tea','{}',
		'[{"values":[{"key":"standard","labels":{"ko":"따뜻하게"}}]}]')`, shop)
	execute("SET session_replication_role=origin")

	// --- after payment: only the paying account gets the order session, and it covers that order only ---
	used := challenge("order", shop, sale)
	if got := prove(otherBoard, crypto.PubkeyToAddress(otherBoard.PublicKey), used, 400); got["error"] != string(service.Mismatch) {
		t.Fatal("another board with a valid signature got the order", got)
	}
	prove(board, account, used, 400) // that challenge is used up
	// An order is followed only at its own store: a challenge that names another store with it is not given at all.
	call(customer, customers.URL+"/v2/customer/challenge", nil, map[string]string{"purpose": "order", "branchRef": elsewhere, "saleRef": sale}, 400)
	ordered := prove(board, account, challenge("order", shop, sale), 200)
	if ordered["saleRef"] != sale || ordered["purpose"] != "order" {
		t.Fatal(ordered)
	}
	orderSession := ordered["session"].(string)
	// A paid order is not cancelled on this route, by either session of its account.
	paidFor := footprint()
	if got := cancelHold(session, sale, 409); got["error"] != string(service.CheckoutConflict) || got["reason"] != "paid" {
		t.Fatalf("cancel of a paid order: %v", got)
	}
	cancelHold(orderSession, sale, 404)
	var still string
	if err := admin.QueryRow(ctx, "SELECT state FROM ledger.sale WHERE id=$1", sale).Scan(&still); err != nil || still != "paid" || footprint() != paidFor {
		t.Fatalf("a refused cancel changed the paid order: %q %v (%s, was %s)", still, err, footprint(), paidFor)
	}
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 200)
	command(orderSession, "order.status", shop, fresh(), map[string]string{}, nil, 404)
	command(orderSession, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 404)
	order(orderSession, 404)
	// The checkout session of the same account reads its paid order too, on purpose: the page that just paid follows
	// the order to its pickup number with the session it has. It reads no order of another account or another store.
	if own := command(session, "order.status", shop, sale, map[string]string{}, nil, 200)["order"].(map[string]any); own["id"] != sale || own["state"] != "paid" {
		t.Fatalf("the paying account's checkout session does not read its paid order: %v", own)
	}
	otherSession := prove(otherBoard, crypto.PubkeyToAddress(otherBoard.PublicKey), challenge("checkout", shop, ""), 200)["session"].(string)
	command(otherSession, "order.status", shop, sale, map[string]string{}, nil, 404)
	command(elsewhereSession, "order.status", elsewhere, sale, map[string]string{}, nil, 404)
	command(elsewhereSession, "order.status", shop, sale, map[string]string{}, nil, 404)
	// Each session has its own request allowance, counted before anything of its address: one session using its up
	// does not limit another, though both come from one address, and a request its own session refuses takes
	// nothing from the address. A token the server does not know has an allowance of its own, apart from the proofs
	// of its address. Checkout sessions share the ceiling of their address; the session of a paid order is not
	// under it, so the customer who paid is answered while the address is at its ceiling.
	h.Customer.SessionLimiter, h.Customer.UnknownLimiter = NewRateLimiter(60, 2, nil), NewRateLimiter(60, 2, nil)
	h.Customer.AddressLimiter, h.Customer.Limiter, h.Customer.StartLimiter = NewRateLimiter(60, 7, nil), NewRateLimiter(60, 1, nil), NewRateLimiter(60, 1, nil)
	command(session, "order.status", shop, sale, map[string]string{}, nil, 200)
	command(session, "order.status", shop, sale, map[string]string{}, nil, 200)
	for i := 0; i < 20; i++ { // its own limit, and none of these is a request of the address
		command(session, "order.status", shop, sale, map[string]string{}, nil, 429)
	}
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 200)
	command(otherSession, "order.status", shop, sale, map[string]string{}, nil, 404)
	for i, want := range []int{404, 404, 429} {
		if got := command(strings.Repeat("cd", 32), "order.status", shop, sale, map[string]string{}, nil, want); want == 429 && got["error"] != string(service.Busy) {
			t.Fatalf("unknown token %d: %v", i, got)
		}
	}
	challenge("pickup", shop, sale)                                                           // the unknown tokens left the address its proof
	command(elsewhereSession, "order.status", elsewhere, sale, map[string]string{}, nil, 404) // the seventh request of the address
	command(otherSession, "order.status", shop, sale, map[string]string{}, nil, 429)          // its ceiling, though the session has an allowance left
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 200)          // the paid order is still answered
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 429)          // until its own allowance is used
	if counts := h.Customer.Limited.Drain(); counts["session"] != 21 || counts["unknown"] != 1 || counts["address"] != 1 || len(counts) != 3 {
		t.Fatalf("refusals by kind: %v", counts)
	}
	// One session start in the window for this account at this address, counted when a proof gives the session.
	// A challenge is no start, however many are asked for; a pickup proof gives no session; and another account
	// behind the same address has its own allowance, so the first one using its up does not stop it.
	h.Customer.Limiter = nil
	for i := 0; i < 3; i++ {
		challenge("checkout", shop, "")
	}
	prove(board, account, challenge("checkout", shop, ""), 200)
	unspent := challenge("checkout", shop, "")
	if got := prove(board, account, unspent, 429); got["error"] != string(service.Busy) {
		t.Fatalf("a second session start of the account in the window: %v", got)
	}
	prove(otherBoard, crypto.PubkeyToAddress(otherBoard.PublicKey), challenge("checkout", shop, ""), 200)
	prove(board, account, challenge("pickup", shop, sale), 409) // not ready yet, and not counted as a start
	if counts := h.Customer.Limited.Drain(); counts["start"] != 1 || len(counts) != 1 {
		t.Fatalf("refusals by kind: %v", counts)
	}
	// The refused proof left its challenge unused: once the account has a start again, the same challenge gives one.
	h.Customer.StartLimiter = nil
	prove(board, account, unspent, 200)
	h.Customer.SessionLimiter, h.Customer.UnknownLimiter, h.Customer.AddressLimiter, h.Customer.StartLimiter = nil, nil, nil, nil

	// --- pickup: only when ready, only the payer, one use, short-lived, this branch and this order ---
	if got := prove(board, account, challenge("pickup", shop, sale), 409); got["error"] != string(service.CheckoutConflict) {
		t.Fatal("a pickup token for an order that is not ready", got)
	}
	if got := prove(board, account, challenge("pickup", shop, sale), 409); got["reason"] != "not_ready" || got["status"] != "error" || got["recordedAt"] != nil {
		t.Fatal("an order that is not ready does not say so", got)
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
	// Every refusal says which it is, in the service's error envelope.
	refused := func(got map[string]any, code service.Code, reason string) {
		t.Helper()
		if got["status"] != "error" || got["error"] != string(code) || got["reason"] != reason {
			t.Fatalf("want %s/%s: %v", code, reason, got)
		}
		if _, timed := got["recordedAt"]; timed != (reason == "already_used") {
			t.Fatalf("the handover time of a %s refusal: %v", reason, got)
		}
	}
	expired := pickup()
	execute("UPDATE ledger.pickup_token SET expires_at=now()-interval '1 second' WHERE token_hash=$1", hashOfHex(t, expired))
	refused(scan(ownCert, shop, sale, expired, 400), service.Expired, "expired")
	refused(scan(ownCert, shop, sale, strings.Repeat("ab", 32), 400), service.Expired, "expired") // never issued, or long gone
	token := pickup()
	refused(scan(ownCert, shop, fresh(), token, 400), service.Mismatch, "other_order")
	// Another branch's staff on their own branch learn that the QR is not theirs, and nothing else about it.
	refused(scan(strangerCert, elsewhere, sale, token, 400), service.Mismatch, "other_store")
	refused(scan(strangerCert, elsewhere, sale, expired, 400), service.Mismatch, "other_store")
	scan(strangerCert, shop, sale, token, 404)   // another branch's staff, on this branch
	scan(strangerCert, shop, sale, expired, 404) // and no reason either
	call(staff(ownCert), server.URL+"/v2/operations/pickup-scan/"+shop+"/"+sale, map[string]string{"Origin": "https://elsewhere.example"},
		map[string]string{"token": token}, 404) // not the operations web
	// A pickup terminal holds the kitchen's handover authority and still does not scan a QR: the route is not found
	// for it, the database refuses its role, and the token stays unused.
	var assigned bool
	if err := admin.QueryRow(ctx, "SELECT ledger.request_authority_live($1,$2,$3,'terminal',ARRAY['pickup_assignment'])", tenant, shop, counter).Scan(&assigned); err != nil || !assigned {
		t.Fatal("the pickup terminal of this test has no assignment", assigned, err)
	}
	scan(counterCert, shop, sale, token, 404)
	var denied *pgconn.PgError
	if _, err := admin.Exec(ctx, "SELECT ledger.scan_pickup_token($1,$2,$3,'terminal',$4,$5,120)", tenant, shop, counter, sale, hashOfHex(t, token)); !errors.As(err, &denied) ||
		denied.Code != "42501" || denied.Message != "a pickup QR is scanned by a store operator" {
		t.Fatal("the database let a terminal scan a pickup QR", err)
	}
	var unused bool
	if err := admin.QueryRow(ctx, "SELECT used_at IS NULL FROM ledger.pickup_token WHERE token_hash=$1", hashOfHex(t, token)).Scan(&unused); err != nil || !unused {
		t.Fatal("a refused terminal used the token up", unused, err)
	}
	execute(`UPDATE ledger.travel_pickup_progress SET state='preparing' WHERE sale_id=$1`, sale)
	refused(scan(ownCert, shop, sale, token, 409), service.CheckoutConflict, "not_ready")
	execute(`UPDATE ledger.travel_pickup_progress SET state='ready' WHERE sale_id=$1`, sale)
	handed := scan(ownCert, shop, sale, token, 200)
	recordedAt, _ := handed["recordedAt"].(float64)
	if handed["saleRef"] != sale || handed["pickupNo"] != float64(7) || handed["revision"] != float64(3) ||
		mustJSON(t, handed["items"]) != `[{"label":"tea","option":"standard","optionLabel":"따뜻하게","quantity":1}]` ||
		time.Since(time.Unix(int64(recordedAt), 0)).Abs() > time.Minute || handed["alreadyRecorded"] != nil {
		t.Fatal(handed)
	}

	// --- the reply was lost: the same staff member scans the same QR again and gets the first answer, marked ---
	again := scan(ownCert, shop, sale, token, 200)
	if again["alreadyRecorded"] != true {
		t.Fatal("a repeated scan is not marked", again)
	}
	delete(again, "alreadyRecorded")
	if first, second := mustJSON(t, handed), mustJSON(t, again); first != second {
		t.Fatalf("a repeated scan answered %s, first %s", second, first)
	}
	// Any other reuse stays refused; a used QR on its own order's page comes with the time of the handover.
	byColleague := scan(colleagueCert, shop, sale, token, 409)
	refused(byColleague, service.Conflict, "already_used")
	if byColleague["recordedAt"] != recordedAt {
		t.Fatal("a colleague's repeated scan", byColleague)
	}
	refused(scan(ownCert, shop, fresh(), token, 400), service.Mismatch, "other_order")
	refused(scan(strangerCert, elsewhere, sale, token, 400), service.Mismatch, "other_store")
	execute("UPDATE ledger.pickup_token SET used_at=used_at-interval '10 minutes' WHERE token_hash=$1", hashOfHex(t, token))
	afterwards := scan(ownCert, shop, sale, token, 409)
	refused(afterwards, service.Conflict, "already_used")
	if afterwards["recordedAt"] != recordedAt-600 {
		t.Fatal("a late repeated scan", afterwards)
	}
	var uses, steps int
	if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.pickup_token WHERE sale_id=$1 AND used_at IS NOT NULL),
		(SELECT count(*) FROM ledger.travel_pickup_transition WHERE sale_id=$1)`, sale).Scan(&uses, &steps); err != nil || uses != 1 || steps != 1 {
		t.Fatalf("%d tokens used and %d pickup steps: %v", uses, steps, err)
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

	// --- the order was handed over: a new pickup proof says so, with the time, and gives no QR; only to its payer ---
	tokens := func() (count int) {
		t.Helper()
		if err := admin.QueryRow(ctx, "SELECT count(*) FROM ledger.pickup_token WHERE sale_id=$1", sale).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	issued := tokens()
	collected := prove(board, account, challenge("pickup", shop, sale), 409)
	if at, _ := collected["recordedAt"].(float64); collected["status"] != "error" || collected["error"] != string(service.CheckoutConflict) ||
		collected["reason"] != "collected" || at < recordedAt-1 || at > recordedAt+1 || collected["pickupToken"] != nil || tokens() != issued {
		t.Fatalf("a pickup proof for an order already handed over: %v (handed over at %v)", collected, recordedAt)
	}
	if got := prove(otherBoard, crypto.PubkeyToAddress(otherBoard.PublicKey), challenge("pickup", shop, sale), 400); got["error"] != string(service.Mismatch) || got["reason"] != nil {
		t.Fatal("another account learned that the order was handed over", got)
	}

	// --- two scans of one QR at once record one handover: the same staff member gets the answer twice, once marked ---
	execute("SET session_replication_role=replica") // the order is ready once more, as if it had not been handed over
	execute(`UPDATE ledger.travel_pickup_progress SET state='ready' WHERE sale_id=$1`, sale)
	execute(`DELETE FROM ledger.escrow_action WHERE sale_id=$1 AND kind='deliver'`, sale)
	execute("SET session_replication_role=origin")
	raced := pickup()
	type answer struct {
		status int
		body   map[string]any
	}
	answers := make(chan answer, 2)
	for _, client := range []*http.Client{staff(ownCert), staff(ownCert)} {
		go func() {
			req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/v2/operations/pickup-scan/"+shop+"/"+sale,
				strings.NewReader(`{"token":"`+raced+`"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", origin)
			var got answer
			if reply, err := client.Do(req); err == nil {
				got.status = reply.StatusCode
				_ = json.NewDecoder(reply.Body).Decode(&got.body)
				reply.Body.Close()
			}
			answers <- got
		}()
	}
	marked := 0
	for range 2 {
		got := <-answers
		if got.status != 200 || got.body["saleRef"] != sale {
			t.Fatal("a scan at the same time", got)
		}
		if got.body["alreadyRecorded"] == true {
			marked++
		}
	}
	if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.travel_pickup_transition WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.escrow_action WHERE sale_id=$1 AND kind='deliver')`, sale).Scan(&steps, &delivers); err != nil ||
		marked != 1 || steps != 2 || delivers != 1 {
		t.Fatalf("two scans at once: %d marked, %d pickup steps, %d delivery marks: %v", marked, steps, delivers, err)
	}

	// --- the order ended: its session still reads the final state, for a day, and takes no answer any more ---
	// The ledger writes the end of an order as record_escrow_event does: settled, at its own clock (check_at).
	settle := func(ago string) {
		t.Helper()
		execute("UPDATE ledger.order_escrow SET settled=true, check_at=clock_timestamp()-$2::interval WHERE sale_id=$1", sale, ago)
	}
	h.Customer.SessionLimiter, h.Customer.UnknownLimiter, h.Customer.AddressLimiter = nil, nil, nil
	// The session that followed the order was proven long before the order ended.
	execute("UPDATE ledger.customer_session SET created_at=created_at-interval '3 hours' WHERE token_hash=$1", hashOfHex(t, orderSession))
	settle("0")
	if ended := command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 200)["order"].(map[string]any); ended["id"] != sale || ended["state"] != "paid" {
		t.Fatalf("the session of an order that just ended does not read it: %v", ended)
	}
	// The session of an order is for that order alone, ended or not.
	command(orderSession, "order.status", shop, fresh(), map[string]string{}, nil, 404)
	command(orderSession, "catalog.read", shop, shop, map[string]string{"page": "0"}, nil, 404)
	order(orderSession, 404)
	cancelHold(orderSession, sale, 404)
	settle("23 hours 59 minutes")
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 200)
	// A day after the end the session that was there is over, like any session that ended.
	settle("24 hours 1 minute")
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 404)
	// The paying board can still prove its account for the order, and the session it gets reads the final state for
	// ten minutes: a phone that was away when the order ended learns its end with one proof. Another account gets
	// nothing, as before.
	if got := prove(otherBoard, crypto.PubkeyToAddress(otherBoard.PublicKey), challenge("order", shop, sale), 400); got["error"] != string(service.Mismatch) {
		t.Fatal("another account got a session for an order that ended", got)
	}
	afterEnd := prove(board, account, challenge("order", shop, sale), 200)["session"].(string)
	if ended := command(afterEnd, "order.status", shop, sale, map[string]string{}, nil, 200)["order"].(map[string]any); ended["id"] != sale || ended["state"] != "paid" {
		t.Fatalf("a session proven after the order ended does not read it: %v", ended)
	}
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 404) // the old one stays over
	execute("UPDATE ledger.customer_session SET created_at=created_at-interval '9 minutes 50 seconds' WHERE token_hash=$1", hashOfHex(t, afterEnd))
	command(afterEnd, "order.status", shop, sale, map[string]string{}, nil, 200)
	execute("UPDATE ledger.customer_session SET created_at=created_at-interval '20 seconds' WHERE token_hash=$1", hashOfHex(t, afterEnd))
	command(afterEnd, "order.status", shop, sale, map[string]string{}, nil, 404)
	// An order that has not ended is followed as long as its session lives, however old the session is.
	execute("UPDATE ledger.order_escrow SET settled=false WHERE sale_id=$1", sale)
	command(afterEnd, "order.status", shop, sale, map[string]string{}, nil, 200)
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 200)
	// A sale that was closed has no session to follow it, as before.
	execute("SET session_replication_role=replica")
	execute("UPDATE ledger.sale SET state='closed' WHERE id=$1", sale)
	execute("SET session_replication_role=origin")
	command(orderSession, "order.status", shop, sale, map[string]string{}, nil, 404)
	command(afterEnd, "order.status", shop, sale, map[string]string{}, nil, 404)
}

func mustKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
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
