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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// accountFunds answers each account the balance the test gave it; an account it does not know has none.
type accountFunds struct {
	mu       sync.Mutex
	asset    common.Address
	balances map[common.Address]*big.Int
	asked    int
	// hold, when set, keeps every read waiting until it is closed.
	hold chan struct{}
}

func (f *accountFunds) PayerBalance(ctx context.Context, _ string, payer common.Address) (common.Address, *big.Int, error) {
	f.mu.Lock()
	f.asked++
	balance, hold := f.balances[payer], f.hold
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return common.Address{}, nil, ctx.Err()
		}
	}
	if balance == nil {
		balance = new(big.Int)
	}
	return f.asset, balance, nil
}

// entranceFixture is one published store with a menu (tea, 3000 won, plenty of it) behind an entrance with the
// request limits opsd serves under, counted by a clock the test moves. Every request of a test comes from one
// address (the loopback), which is what customers behind a store's wireless network look like.
type entranceFixture struct {
	t         *testing.T
	ctx       context.Context
	admin     *pgx.Conn
	h         HTTP
	customers *httptest.Server
	shop      string
	operator  string
	asset     string
	funds     *accountFunds
	now       time.Time
	clock     sync.Mutex
	lookups   int
	fresh     func() string
	publish   func(published bool)
}

type entranceReply struct {
	status int
	retry  string
	body   map[string]any
	raw    string
}

func (f *entranceFixture) pass(d time.Duration) {
	f.clock.Lock()
	f.now = f.now.Add(d)
	f.clock.Unlock()
}

func (f *entranceFixture) send(method, path, session string, body any) entranceReply {
	f.t.Helper()
	var reader io.Reader
	switch value := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(value)
	default:
		raw, _ := json.Marshal(value)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequestWithContext(f.ctx, method, f.customers.URL+path, reader)
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	got, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer got.Body.Close()
	raw, _ := io.ReadAll(got.Body)
	out := entranceReply{status: got.StatusCode, retry: got.Header.Get("Retry-After"), raw: string(raw)}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

// challenge asks for a challenge of purpose for the fixture's shop (or for no store, for `account`).
func (f *entranceFixture) challenge(purpose, sale string) entranceReply {
	f.t.Helper()
	asked := map[string]string{"purpose": purpose}
	if purpose != "account" {
		asked["branchRef"] = f.shop
	}
	if sale != "" {
		asked["saleRef"] = sale
	}
	return f.send("POST", "/v2/customer/challenge", "", asked)
}

// prove signs the challenge of issued with key and sends the proof.
func (f *entranceFixture) prove(key *ecdsa.PrivateKey, issued entranceReply, generation string) entranceReply {
	f.t.Helper()
	if issued.status != 200 {
		f.t.Fatalf("no challenge to prove: %d %s", issued.status, issued.raw)
	}
	value, err := hex.DecodeString(issued.body["challenge"].(string))
	if err != nil || len(value) != 32 {
		f.t.Fatalf("challenge: %s", issued.raw)
	}
	proof := map[string]string{"challenge": hex.EncodeToString(value), "account": strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()),
		"signature": hex.EncodeToString(signAccountProof(f.t, key, [32]byte(value)))}
	if generation != "" {
		proof["generation"] = generation
	}
	return f.send("POST", "/v2/customer/proof", "", proof)
}

// checkout is a customer's whole session start: a challenge for the shop and the board's proof over it.
func (f *entranceFixture) checkout(key *ecdsa.PrivateKey) (entranceReply, entranceReply) {
	f.t.Helper()
	issued := f.challenge("checkout", "")
	if issued.status != 200 {
		return issued, entranceReply{}
	}
	return issued, f.prove(key, issued, "7")
}

func (f *entranceFixture) command(session, operation, object, key string, fields map[string]string, cart *Cart) entranceReply {
	f.t.Helper()
	envelope := map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: operation, RequestID: f.fresh(),
		Domain: f.shop, Object: object, ExpiresAt: time.Now().Unix() + 30, IdempotencyKey: key, Body: fields}}
	if cart != nil {
		envelope["cart"] = cart
	}
	return f.send("POST", "/v2/customer/commerce", session, envelope)
}

// order asks to hold one tea as the cart `ref` under the request key `key`.
func (f *entranceFixture) order(session, ref, key string) entranceReply {
	f.t.Helper()
	return f.orderAs(session, ref, key, f.fresh())
}

// orderAs is order with the request's own id given, for a request sent from beside the test's goroutine.
func (f *entranceFixture) orderAs(session, ref, key, request string) entranceReply {
	cart := Cart{Origin: "menu", OriginRef: ref, MenuVersion: 1, Items: []Item{{SKU: "tea", Option: "standard", Qty: 1}}}
	packed, _ := json.Marshal(cart)
	digest := sha256.Sum256(packed)
	envelope := map[string]any{"cart": cart, "command": service.Request{Version: 2, Contract: "C3", Operation: "order.create", RequestID: request,
		Domain: f.shop, Object: ref, ExpiresAt: time.Now().Unix() + 30, IdempotencyKey: key,
		Body: map[string]string{"asset": f.asset, "quantity": "1", "intentDigest": hex.EncodeToString(digest[:])}}}
	return f.send("POST", "/v2/customer/commerce", session, envelope)
}

func (f *entranceFixture) execute(query string, args ...any) {
	f.t.Helper()
	if _, err := f.admin.Exec(f.ctx, query, args...); err != nil {
		f.t.Fatal(query, err)
	}
}

// paid makes the unpaid order `sale` of key's account a paid one with an escrow that is not settled, as the
// payment worker would (seeded, as in TestPostgresCustomerContract).
func (f *entranceFixture) paid(sale string, key *ecdsa.PrivateKey) {
	f.t.Helper()
	account := crypto.PubkeyToAddress(key.PublicKey)
	f.execute("SET session_replication_role=replica")
	f.execute(`UPDATE ledger.sale SET state='paid' WHERE id=$1`, sale)
	f.execute(`INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,
		payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
		VALUES($1,$2,decode(md5(random()::text)||md5(random()::text),'hex'),decode(md5(random()::text)||md5(random()::text),'hex'),decode(md5(random()::text)||md5(random()::text),'hex'),decode(md5(random()::text)||md5(random()::text),'hex'),0,1,substring(decode(md5(random()::text)||md5(random()::text),'hex') FROM 1 FOR 20),
		$3,7,1,now(),now()+interval '1 minute',1,now(),'x',decode(md5(random()::text)||md5(random()::text),'hex'))`, sale, CustomerSubject(account), account[:])
	f.execute(`INSERT INTO ledger.payment(sale_id,stream_id,merchant_key,evidence_id,tx_hash,log_index,inclusion_hash,included_at,asset_ref,payer,
		payer_generation,payment_mode,principal) VALUES($1,gen_random_uuid(),'0x'||repeat('ab',32),gen_random_uuid(),$4,0,
		'0x'||repeat('ef',32),now(),$2,$3,7,1,300000)`, sale, f.asset, strings.ToLower(account.Hex()), "0x"+hex.EncodeToString(hashOf([]byte(sale))))
	f.execute(`INSERT INTO ledger.pickup(sale_id,branch_id,business_day,number) VALUES($1,$2,current_date,(SELECT count(*)+1 FROM ledger.pickup WHERE branch_id=$2))`, sale, f.shop)
	f.execute(`INSERT INTO ledger.order_escrow(sale_id,contract_ref,confirmer_ref,deliver_by,confirm_window) VALUES($1,$2,$2,$3,172800)`,
		sale, "0x"+strings.Repeat("4", 40), time.Now().Add(24*time.Hour).Unix())
	f.execute(`INSERT INTO ledger.travel_pickup_progress(sale_id,state,revision,updated_at) VALUES($1,'preparing',1,now())`, sale)
	f.execute("SET session_replication_role=origin")
}

func newEntranceFixture(t *testing.T, name string) *entranceFixture {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") {
		t.Fatal("use disposable commerce database")
	}
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(stop)
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(context.Background()) })
	f := &entranceFixture{t: t, ctx: ctx, admin: admin, asset: "0x" + strings.Repeat("2", 40), now: time.Unix(1_800_000_000, 0)}
	role := "nu54_" + name + "_" + strings.TrimPrefix(config.Database, "nu54_commerce_")
	f.execute("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " LOGIN IN ROLE nu54_p05_commerce")
	t.Cleanup(func() { admin.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize()) })
	store, err := Open(ctx, config.ConnString()+" user="+role)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	f.fresh = func() string { return uuidForCatalogFixture(t, ctx, admin) }
	f.shop, f.operator = f.fresh(), f.fresh()
	f.execute("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", f.shop, tenant)
	f.execute("INSERT INTO ledger.command_rule VALUES ($1,$2,'fixture','dispose',interval '2 minutes')", f.shop, name+" test")
	f.execute("INSERT INTO ledger.operator_branch_label VALUES ($1,$2)", f.shop, "여울 커피 "+f.shop[:8])
	f.execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", f.operator, name+"_"+f.operator)
	f.execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", f.shop, f.operator)
	f.execute(`INSERT INTO ledger.branch_store_page VALUES ($1,true,false,'카페','','','','','','','',NULL,$2,now())`, f.shop, f.operator)
	f.publish = func(published bool) {
		t.Helper()
		f.execute("UPDATE ledger.branch_store_page SET published=$2 WHERE branch_id=$1", f.shop, published)
	}

	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(10 * time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	ca, _ := x509.ParseCertificate(caDER)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, leafKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	leafDigest := sha256.Sum256(leafDER)
	credentials := map[string]service.Principal{hex.EncodeToString(leafDigest[:]): {Tenant: tenant, Subject: f.operator, Role: "operator",
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix()}}

	certified, entrance := http.NewServeMux(), http.NewServeMux()
	server := httptest.NewUnstartedServer(certified)
	t.Cleanup(server.Close)
	origin := "https://" + server.Listener.Addr().String()
	limits := NewCustomerLimits(func() time.Time {
		f.clock.Lock()
		defer f.clock.Unlock()
		return f.now
	})
	f.funds = &accountFunds{asset: common.HexToAddress(f.asset), balances: map[common.Address]*big.Int{}}
	limits.Funds = f.funds
	limits.lookup = func(ctx context.Context, token []byte) (customerSession, error) {
		f.clock.Lock()
		f.lookups++
		f.clock.Unlock()
		return store.customerSession(ctx, token)
	}
	f.h = HTTP{Store: store, OperatorOrigin: origin, Credentials: credentials, Customer: limits,
		Profiles: map[string]Profile{f.shop: {Chain: 8283, Asset: common.HexToAddress(f.asset), Token: wonForOptions}}}
	f.h.Register(certified)
	f.h.RegisterCustomer(entrance)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	server.StartTLS()
	f.customers = httptest.NewServer(entrance)
	t.Cleanup(f.customers.Close)
	staff := server.Client()
	staff.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}}
	operate := func(path string, body any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		reply, err := staff.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		if out, _ := io.ReadAll(reply.Body); reply.StatusCode != 200 {
			t.Fatalf("%s: %d %s", path, reply.StatusCode, out)
		}
	}
	operate("/v2/operations/catalog/"+f.shop+"/publish", map[string]any{"requestKey": f.fresh(), "version": 0, "expiresAt": time.Now().Add(time.Hour).UTC(),
		"rows": []operatorRow{{SKU: "tea", Option: "standard", Label: "tea", Asset: f.asset, DisplayPrice: "3000"}}})
	operate("/v2/operations/catalog/"+f.shop+"/stock", map[string]any{"requestKey": f.fresh(), "version": 1, "sku": "tea", "delta": 500})
	return f
}

// The probes of the second review, each from the one address every customer of the test shares, against the
// limits opsd serves under. In each a sender that pays nothing keeps asking, and a customer who does what the app
// does is served all the while.

// Session starts. The sender asks for challenges of a branch that does not exist (33 in half a minute, then six a
// minute), sends bodies that do not read, proofs with a signature that is not the account's and proofs over
// challenges nobody gave. Customers who open the store and start a session are not refused once.
func TestPostgresJunkDoesNotStopSessionStarts(t *testing.T) {
	f := newEntranceFixture(t, "starts")
	stranger := mustKey(t)
	junk := func(i int) entranceReply {
		switch i % 4 {
		case 0: // the review's probe: a branch that does not exist
			return f.send("POST", "/v2/customer/challenge", "", map[string]string{"purpose": "checkout", "branchRef": f.fresh()})
		case 1:
			return f.send("POST", "/v2/customer/challenge", "", `{"purpose":"checkout"`)
		case 2: // a signature of another key than the account named
			value := [32]byte{0: byte(i), 1: byte(i >> 8), 31: 1}
			return f.send("POST", "/v2/customer/proof", "", map[string]string{"challenge": hex.EncodeToString(value[:]),
				"account": "0x" + strings.Repeat("7", 40), "signature": hex.EncodeToString(signAccountProof(t, stranger, value))})
		default: // the sender's own valid signature over a challenge the server never gave
			return f.prove(stranger, entranceReply{status: 200, body: map[string]any{"challenge": hex.EncodeToString(hashOf([]byte{byte(i), byte(i >> 8)}))}}, "7")
		}
	}
	customers := 0
	customer := func(when string) {
		t.Helper()
		customers++
		if page := f.send("GET", "/v2/customer/stores/"+f.shop, "", nil); page.status != 200 {
			t.Fatalf("%s: the store page: %d %s", when, page.status, page.raw)
		}
		issued, proven := f.checkout(mustKey(t))
		if issued.status != 200 || proven.status != 200 || proven.body["session"] == nil {
			t.Fatalf("%s: customer %d was refused a session: challenge %d %s, proof %d %s", when, customers, issued.status, issued.raw, proven.status, proven.raw)
		}
	}
	answered, refused := 0, 0
	count := func(reply entranceReply) {
		t.Helper()
		switch reply.status {
		case 400:
			answered++
		case 429:
			refused++
			if reply.retry == "" || reply.body["error"] != string(service.Busy) {
				t.Fatalf("a refused attempt: %s, Retry-After %q", reply.raw, reply.retry)
			}
		default:
			t.Fatalf("a failed attempt was answered %d %s", reply.status, reply.raw)
		}
	}
	customer("before any junk")
	for i := 0; i < 33; i++ { // 33 in thirty seconds
		count(junk(i))
		f.pass(900 * time.Millisecond)
		if i%8 == 7 {
			customer("in the first half minute")
		}
	}
	for minute := 0; minute < 5; minute++ { // then six a minute
		for i := 0; i < 6; i++ {
			count(junk(33 + minute*6 + i))
			f.pass(10 * time.Second)
		}
		customer("while six failed attempts a minute go on")
		customer("while six failed attempts a minute go on")
	}
	// The same at a hundred a second: nothing more of it is answered, and customers still start sessions.
	for i := 0; i < 300; i++ {
		count(junk(i))
		f.pass(10 * time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		customer("under a hundred failed attempts a second")
	}
	if customers != 20 {
		t.Fatalf("%d customers were served", customers)
	}
	// Ten failed attempts at once and one every five seconds were answered as what they are; the rest was refused
	// before the database was asked.
	if answered > 10+(33*900+5*60_000+3_000)/5000+1 || refused < 300 {
		t.Fatalf("%d failed attempts were answered and %d refused", answered, refused)
	}
	limited := f.h.Customer.Limited.Drain()
	if limited["failed"] != int64(refused) || len(limited) != 1 {
		t.Fatalf("refusals by kind: %v, want only %d failed attempts", limited, refused)
	}
	// What the sender can still do is what a customer does, and it is counted for its own account: one key starting
	// session after session is held at its own starts, and the next customer is not.
	greedy := mustKey(t)
	starts, held := 0, 0
	for i := 0; i < 12; i++ {
		switch _, proven := f.checkout(greedy); proven.status {
		case 200:
			starts++
		case 429:
			held++
		default:
			t.Fatalf("session start %d of one account: %d %s", i+1, proven.status, proven.raw)
		}
		f.pass(1500 * time.Millisecond) // the address's challenges and proofs refill meanwhile
	}
	if starts < 6 || starts > 8 || held != 12-starts {
		t.Fatalf("one account started %d sessions in 18 seconds and was held %d times", starts, held)
	}
	f.pass(2 * time.Second)
	customer("after one account used its own starts up")
	if limited = f.h.Customer.Limited.Drain(); limited["start"] != int64(held) || len(limited) != 1 {
		t.Fatalf("refusals by kind: %v", limited)
	}
}

// Session requests. The sender sends ten made-up tokens a second for half a minute. A customer who is paying is
// served every time, and the made-up tokens are looked up no more often than their own allowance lets them.
func TestPostgresMadeUpTokensDoNotStopPayingCustomers(t *testing.T) {
	f := newEntranceFixture(t, "tokens")
	key := mustKey(t)
	_, proven := f.checkout(key)
	if proven.status != 200 {
		t.Fatalf("the customer's session: %d %s", proven.status, proven.raw)
	}
	session := proven.body["session"].(string)
	f.funds.balances[crypto.PubkeyToAddress(key.PublicKey)] = big.NewInt(300_000)
	paying := func(when string) {
		t.Helper()
		if menu := f.command(session, "catalog.read", f.shop, "", map[string]string{"page": "0"}, nil); menu.status != 200 {
			t.Fatalf("%s: the paying customer's request: %d %s (Retry-After %q)", when, menu.status, menu.raw, menu.retry)
		}
	}
	paying("before any made-up token")
	before := f.lookups
	found, refused := 0, 0
	for i := 0; i < 300; i++ {
		token := hex.EncodeToString(hashOf([]byte{0xcd, byte(i >> 8), byte(i)}))
		switch reply := f.command(token, "catalog.read", f.shop, "", map[string]string{"page": "0"}, nil); reply.status {
		case 404:
			found++
		case 429:
			refused++
		default:
			t.Fatalf("made-up token %d: %d %s", i, reply.status, reply.raw)
		}
		f.pass(100 * time.Millisecond)
		if i%10 == 9 {
			before++ // the customer's own lookup
			paying("among ten made-up tokens a second")
		}
	}
	// Ten at once and one every five seconds are answered as unknown; none is looked up once the address's
	// allowance for tokens the server has not seen live is gone (60 at once, one a second).
	if looked := f.lookups - before; looked != found+refusedAfterLookup(f, refused) || looked > 60+30 {
		t.Fatalf("%d lookups for made-up tokens: %d answered as unknown, %d refused", looked, found, refused)
	}
	if found > 10+30/5+1 {
		t.Fatalf("%d made-up tokens were answered as unknown", found)
	}
	// The customer then orders and takes the cart back, all with the made-up tokens still coming.
	held := f.order(session, f.fresh(), f.fresh())
	if held.status != 200 {
		t.Fatalf("the customer's cart: %d %s", held.status, held.raw)
	}
	if cancelled := f.send("POST", "/v2/customer/checkout/cancel", session, map[string]string{"saleRef": held.body["resultRef"].(string)}); cancelled.status != 200 {
		t.Fatalf("the customer's cancel: %d %s", cancelled.status, cancelled.raw)
	}
	limited := f.h.Customer.Limited.Drain()
	if limited["live"] != 0 || limited["session"] != 0 || limited["address"]+limited["unknown"] != int64(refused) {
		t.Fatalf("refusals by kind: %v (%d made-up tokens refused)", limited, refused)
	}
}

// refusedAfterLookup is how many of the refused made-up tokens were refused after their lookup (the allowance of
// unknown tokens) rather than before it (the address's): those were looked up.
func refusedAfterLookup(f *entranceFixture, refused int) int {
	f.t.Helper()
	counts := f.h.Customer.Limited.Drain()
	for kind, count := range counts { // put back what was read
		for i := int64(0); i < count; i++ {
			f.h.Customer.Limited.add(kind)
		}
	}
	if counts["address"]+counts["unknown"] != int64(refused) {
		f.t.Fatalf("refusals by kind: %v, want %d of made-up tokens", counts, refused)
	}
	return int(counts["unknown"])
}

// Carts. The sender holds one session of an account with no balance and asks for a cart six times a minute, after
// 33 in the first half minute. Customers whose accounts can pay hold their carts all the while. A customer's own
// request sent again reads its first answer even when that customer has no cart attempt left, and a cart that was
// taken back says so to the request that made it.
func TestPostgresCartAttemptsOfOneAccountDoNotStopOthers(t *testing.T) {
	f := newEntranceFixture(t, "carts")
	session := func(key *ecdsa.PrivateKey) string {
		t.Helper()
		_, proven := f.checkout(key)
		if proven.status != 200 {
			t.Fatalf("a session: %d %s", proven.status, proven.raw)
		}
		return proven.body["session"].(string)
	}
	sender := session(mustKey(t))
	customers := 0
	customer := func(when string) (string, string, string, string) {
		t.Helper()
		customers++
		key := mustKey(t)
		f.funds.balances[crypto.PubkeyToAddress(key.PublicKey)] = big.NewInt(300_000) // one tea, to the unit
		own := session(key)
		ref, request := f.fresh(), f.fresh()
		held := f.order(own, ref, request)
		if held.status != 200 || held.body["resultRef"] == nil {
			t.Fatalf("%s: customer %d's cart was not held: %d %s (Retry-After %q)", when, customers, held.status, held.raw, held.retry)
		}
		return own, ref, request, held.body["resultRef"].(string)
	}
	short, limited := 0, 0
	attempt := func() {
		t.Helper()
		switch reply := f.order(sender, f.fresh(), f.fresh()); {
		case reply.status == 409 && reply.body["reason"] == "balance_short":
			short++
		case reply.status == 429 && reply.body["reason"] == "hold_limit" && reply.retry != "":
			limited++
		default:
			t.Fatalf("the sender's cart: %d %s", reply.status, reply.raw)
		}
	}
	customer("before the sender asks")
	for i := 0; i < 33; i++ {
		attempt()
		f.pass(900 * time.Millisecond)
		if i%11 == 10 {
			customer("in the first half minute")
		}
	}
	for minute := 0; minute < 5; minute++ {
		for i := 0; i < 6; i++ {
			attempt()
			f.pass(10 * time.Second)
		}
		customer("while six cart attempts a minute go on")
	}
	// Six at once and one every twenty seconds reached the chain; the rest was refused before it was read.
	if reads := f.funds.asked - customers; reads != short || short > 6+(33*900+5*60_000)/20_000+1 || limited < 33+30-short {
		t.Fatalf("the sender's %d attempts read the chain %d times (%d short, %d limited)", 63, reads, short, limited)
	}
	if counts := f.h.Customer.Limited.Drain(); counts["hold"] != int64(limited) || len(counts) != 1 {
		t.Fatalf("refusals by kind: %v", counts)
	}

	// One customer uses its own cart attempts up, and then sends the request of its held cart again: that one is
	// recognised before anything is counted, and reads its first answer without another look at the chain.
	own, ref, request, sale := customer("the customer who retries")
	for i := 0; i < 12; i++ {
		f.order(own, f.fresh(), f.fresh()) // the account already holds a cart: refused, and each one counted
	}
	if spent := f.order(own, f.fresh(), f.fresh()); spent.status != 429 || spent.body["reason"] != "hold_limit" {
		t.Fatalf("the customer's own attempts are not used up: %d %s", spent.status, spent.raw)
	}
	asked := f.funds.asked
	if again := f.order(own, ref, request); again.status != 200 || again.body["resultRef"] != sale || f.funds.asked != asked {
		t.Fatalf("the same request again: %d %s (%d balance reads)", again.status, again.raw, f.funds.asked-asked)
	}
	// The cart is taken back. The request that made it is told so, instead of being handed the closed order as if
	// it were held; a new request then holds a new cart at once.
	if cancelled := f.send("POST", "/v2/customer/checkout/cancel", own, map[string]string{"saleRef": sale}); cancelled.status != 200 {
		t.Fatalf("the cancel: %d %s", cancelled.status, cancelled.raw)
	}
	released := f.order(own, ref, request)
	if released.status != 409 || released.retry != "" || released.body["error"] != string(service.CheckoutConflict) || released.body["reason"] != "cart_released" ||
		released.body["resultRef"] != nil || released.body["requestId"] == "" {
		t.Fatalf("the request of a cart that was taken back: %d %s", released.status, released.raw)
	}
	f.pass(25 * time.Second)
	if next := f.order(own, f.fresh(), f.fresh()); next.status != 200 || next.body["resultRef"] == sale {
		t.Fatalf("a new cart after the first was taken back: %d %s", next.status, next.raw)
	}
	// A cart whose hold ran out, unpaid, reads the same way to the request that made it.
	other, otherRef, otherRequest, expired := customer("the customer whose hold runs out")
	f.execute("UPDATE ledger.order_checkout SET expires_at=clock_timestamp()-interval '1 second' WHERE sale_id=$1", expired)
	for reclaimed := true; reclaimed; { // every hold that ran out, this one among them
		if err := f.admin.QueryRow(f.ctx, "SELECT ledger.reclaim_expired_order()").Scan(&reclaimed); err != nil {
			t.Fatal(err)
		}
	}
	if late := f.order(other, otherRef, otherRequest); late.status != 409 || late.body["reason"] != "cart_released" {
		t.Fatalf("the request of a cart whose hold ran out: %d %s", late.status, late.raw)
	}
	// A paid order is not a released cart: the request that made it still reads the order.
	payer := mustKey(t)
	f.funds.balances[crypto.PubkeyToAddress(payer.PublicKey)] = big.NewInt(300_000)
	paying := session(payer)
	paidRef, paidRequest := f.fresh(), f.fresh()
	paidSale := f.order(paying, paidRef, paidRequest).body["resultRef"].(string)
	f.paid(paidSale, payer)
	f.funds.balances[crypto.PubkeyToAddress(payer.PublicKey)] = big.NewInt(0) // the payment took the money
	if again := f.order(paying, paidRef, paidRequest); again.status != 200 || again.body["resultRef"] != paidSale {
		t.Fatalf("the request of a paid order again: %d %s", again.status, again.raw)
	}
}

// A chain that answers slowly holds only the places set aside for balance reads: a further cart is told at once to
// come back, and menus are read meanwhile.
func TestPostgresSlowBalanceReadsLeaveTheMenu(t *testing.T) {
	f := newEntranceFixture(t, "slow")
	f.h.Customer.BalanceReads = make(chan struct{}, 2)
	f.h.Customer.InFlight = make(chan struct{}, 4)
	f.h.Customer.InFlightWait = 50 * time.Millisecond
	sessions := make([]string, 5)
	for i := range sessions {
		key := mustKey(t)
		f.funds.balances[crypto.PubkeyToAddress(key.PublicKey)] = big.NewInt(300_000)
		_, proven := f.checkout(key)
		if proven.status != 200 {
			t.Fatalf("session %d: %d %s", i, proven.status, proven.raw)
		}
		sessions[i] = proven.body["session"].(string)
	}
	release := make(chan struct{})
	f.funds.mu.Lock()
	f.funds.hold = release
	f.funds.mu.Unlock()
	waiting := make(chan entranceReply, 2)
	for i := 0; i < 2; i++ {
		go func(session, ref, key, request string) { waiting <- f.orderAs(session, ref, key, request) }(sessions[i], f.fresh(), f.fresh(), f.fresh())
	}
	for deadline := time.Now().Add(5 * time.Second); len(f.h.Customer.BalanceReads) != 2; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d balance reads are waiting for the chain", len(f.h.Customer.BalanceReads))
		}
	}
	// Three more carts while the chain is silent: each is answered at once, and takes no place.
	for i := 2; i < 5; i++ {
		started := time.Now()
		refused := f.order(sessions[i], f.fresh(), f.fresh())
		if refused.status != 503 || refused.retry != "3" || refused.body["error"] != string(service.Busy) || refused.body["reason"] != "balance_unavailable" ||
			time.Since(started) > time.Second {
			t.Fatalf("cart %d while the chain is slow: %d %s after %v", i+1, refused.status, refused.raw, time.Since(started))
		}
	}
	// The other two places are free: the menu and the store page are read, and a session's menu too.
	for i := 0; i < 6; i++ {
		if menu := f.send("GET", "/v2/customer/stores/"+f.shop+"/menu/0", "", nil); menu.status != 200 {
			t.Fatalf("menu read %d while the chain is slow: %d %s", i+1, menu.status, menu.raw)
		}
		if page := f.send("GET", "/v2/customer/stores/"+f.shop, "", nil); page.status != 200 {
			t.Fatalf("store page read %d while the chain is slow: %d %s", i+1, page.status, page.raw)
		}
		if own := f.command(sessions[4], "catalog.read", f.shop, "", map[string]string{"page": "0"}, nil); own.status != 200 {
			t.Fatalf("session menu read %d while the chain is slow: %d %s", i+1, own.status, own.raw)
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		if held := <-waiting; held.status != 200 {
			t.Fatalf("a cart whose balance read came back: %d %s", held.status, held.raw)
		}
	}
	if counts := f.h.Customer.Limited.Drain(); counts["balance"] != 3 || len(counts) != 1 {
		t.Fatalf("refusals by kind: %v", counts)
	}
}

// A store takes customers only while its page is published: no challenge for a new session, no session from a
// challenge given before, no request of a session given before. A customer who paid is not shut out by that: the
// order is still followed, answered for and collected. And the account alone can ask how many of its orders are
// unfinished, which is what a phone that kept no record needs before a board is reset.
func TestPostgresPublishedGateAndOpenOrders(t *testing.T) {
	f := newEntranceFixture(t, "gate")
	f.h.Customer = &CustomerEntrance{Funds: f.funds} // the gate itself, without request limits
	entrance := http.NewServeMux()
	f.h.RegisterCustomer(entrance)
	f.customers.Config.Handler = entrance
	key, other := mustKey(t), mustKey(t)
	account := crypto.PubkeyToAddress(key.PublicKey)
	f.funds.balances[account] = big.NewInt(300_000)
	openOrders := func(who *ecdsa.PrivateKey) float64 {
		t.Helper()
		answer := f.prove(who, f.challenge("account", ""), "")
		if answer.status != 200 || answer.body["purpose"] != "account" || len(answer.body) != 2 {
			t.Fatalf("the account's unfinished orders: %d %s", answer.status, answer.raw)
		}
		return answer.body["openOrders"].(float64)
	}
	if open := openOrders(key); open != 0 {
		t.Fatalf("an account that never ordered has %v unfinished orders", open)
	}
	_, proven := f.checkout(key)
	session := proven.body["session"].(string)
	if open := openOrders(key); open != 0 { // a cart without a quote takes no money
		t.Fatalf("a session alone counts as %v unfinished orders", open)
	}
	sale := f.order(session, f.fresh(), f.fresh()).body["resultRef"].(string)
	f.paid(sale, key)
	if open := openOrders(key); open != 1 {
		t.Fatalf("a paid order whose escrow is not settled counts as %v", open)
	}
	if open := openOrders(other); open != 0 { // another account learns nothing of it
		t.Fatalf("another account is told %v unfinished orders", open)
	}
	// The answer needs the account's own signature over a challenge given for it, once.
	asked := f.challenge("account", "")
	value, _ := hex.DecodeString(asked.body["challenge"].(string))
	forged := f.send("POST", "/v2/customer/proof", "", map[string]string{"challenge": hex.EncodeToString(value), "account": strings.ToLower(account.Hex()),
		"signature": hex.EncodeToString(signAccountProof(t, other, [32]byte(value)))})
	if forged.status != 400 || forged.body["error"] != string(service.Mismatch) || forged.body["openOrders"] != nil {
		t.Fatalf("another key asked for the account: %d %s", forged.status, forged.raw)
	}
	if first := f.prove(key, asked, ""); first.status != 200 || first.body["openOrders"] != float64(1) {
		t.Fatalf("the account's own proof after a forged one: %d %s", first.status, first.raw)
	}
	if again := f.prove(key, asked, ""); again.status != 400 || again.body["error"] != string(service.Expired) {
		t.Fatalf("an account challenge was accepted twice: %d %s", again.status, again.raw)
	}
	// An account challenge gives no session, and a store's challenge gives no count.
	if got := f.prove(key, f.challenge("account", ""), "7"); got.body["session"] != nil || got.body["pickupToken"] != nil || got.body["openOrders"] != float64(1) {
		t.Fatalf("an account challenge proven like a checkout: %d %s", got.status, got.raw)
	}
	for _, bad := range []map[string]string{{"purpose": "account", "branchRef": f.shop}, {"purpose": "account", "saleRef": sale},
		{"purpose": "checkout"}, {"purpose": "orders"}} {
		if got := f.send("POST", "/v2/customer/challenge", "", bad); got.status != 400 {
			t.Fatalf("challenge %v: %d %s", bad, got.status, got.raw)
		}
	}

	// The store takes its page back.
	nowhere := f.send("POST", "/v2/customer/challenge", "", map[string]string{"purpose": "checkout", "branchRef": f.fresh()})
	pending := f.challenge("checkout", "") // given while the store was public
	f.publish(false)
	if closed := f.challenge("checkout", ""); closed.status != 400 || closed.status != nowhere.status || closed.raw != nowhere.raw {
		t.Fatalf("a checkout challenge for a store that is not public: %d %s; for no store: %d %s", closed.status, closed.raw, nowhere.status, nowhere.raw)
	}
	if late := f.prove(other, pending, "7"); late.status != 400 || late.body["error"] != string(service.Expired) || late.body["session"] != nil {
		t.Fatalf("a challenge given before the store closed gave a session: %d %s", late.status, late.raw)
	}
	if menu := f.command(session, "catalog.read", f.shop, "", map[string]string{"page": "0"}, nil); menu.status != 404 {
		t.Fatalf("a checkout session of a store that is not public: %d %s", menu.status, menu.raw)
	}
	if cart := f.order(session, f.fresh(), f.fresh()); cart.status != 404 {
		t.Fatalf("a cart at a store that is not public: %d %s", cart.status, cart.raw)
	}
	// An order of another store's customer, or one that does not exist, opens nothing here either.
	for _, purpose := range []string{"order", "pickup"} {
		if unknown := f.challenge(purpose, f.fresh()); unknown.status != 400 || unknown.raw != nowhere.raw {
			t.Fatalf("a %s challenge for no order at a store that is not public: %d %s", purpose, unknown.status, unknown.raw)
		}
	}
	// The customer who paid still follows the order, is told it is not ready, collects it and answers for it.
	following := f.prove(key, f.challenge("order", sale), "")
	if following.status != 200 || following.body["purpose"] != "order" || following.body["saleRef"] != sale {
		t.Fatalf("the paying customer's order session after the store closed: %d %s", following.status, following.raw)
	}
	orderSession := following.body["session"].(string)
	if status := f.command(orderSession, "order.status", sale, "", map[string]string{}, nil); status.status != 200 || status.body["order"].(map[string]any)["state"] != "paid" {
		t.Fatalf("the paid order read after the store closed: %d %s", status.status, status.raw)
	}
	if waiting := f.prove(key, f.challenge("pickup", sale), ""); waiting.status != 409 || waiting.body["reason"] != "not_ready" {
		t.Fatalf("a pickup proof before the order is ready: %d %s", waiting.status, waiting.raw)
	}
	f.execute("SET session_replication_role=replica")
	f.execute(`UPDATE ledger.travel_pickup_progress SET state='ready',revision=2,updated_at=now() WHERE sale_id=$1`, sale)
	f.execute("SET session_replication_role=origin")
	if ready := f.prove(key, f.challenge("pickup", sale), ""); ready.status != 200 || ready.body["pickupToken"] == nil {
		t.Fatalf("the pickup QR of a paid order after the store closed: %d %s", ready.status, ready.raw)
	}
	if stranger := f.prove(other, f.challenge("order", sale), ""); stranger.status != 400 || stranger.body["session"] != nil {
		t.Fatalf("another account followed the order: %d %s", stranger.status, stranger.raw)
	}
	if open := openOrders(key); open != 1 {
		t.Fatalf("the unfinished order of a store that closed counts as %v", open)
	}
	// Once the escrow is settled nothing of the account's is unfinished, and the board may be reset.
	f.execute("SET session_replication_role=replica")
	f.execute(`UPDATE ledger.order_escrow SET settled=true WHERE sale_id=$1`, sale)
	f.execute("SET session_replication_role=origin")
	if open := openOrders(key); open != 0 {
		t.Fatalf("a settled order counts as %v unfinished", open)
	}
	// Published again, the store takes customers again, and the session given before is live while it lasts.
	f.publish(true)
	if menu := f.command(session, "catalog.read", f.shop, "", map[string]string{"page": "0"}, nil); menu.status != 200 {
		t.Fatalf("the checkout session after the store opened again: %d %s", menu.status, menu.raw)
	}
	if _, again := f.checkout(other); again.status != 200 {
		t.Fatalf("a new session after the store opened again: %d %s", again.status, again.raw)
	}
}

// restart puts a new entrance in the place of the fixture's, as a restarted opsd has one: every allowance full and
// nothing remembered.
func (f *entranceFixture) restart() {
	lookup := f.h.Customer.lookup
	limits := NewCustomerLimits(func() time.Time {
		f.clock.Lock()
		defer f.clock.Unlock()
		return f.now
	})
	limits.Funds, limits.lookup = f.funds, lookup
	f.h.Customer = limits
	entrance := http.NewServeMux()
	f.h.RegisterCustomer(entrance)
	f.customers.Config.Handler = entrance
}

// The probes of the third review, with the limits opsd serves under and every request from the one address.

// Challenges cost a sender nothing, so they must cost the customers of its address nothing either, and neither
// must proof requests that are never looked at twice. A sender without any key asks for an `account` challenge five
// times a second for two minutes (the review's probe), then nineteen times a second, which is just under what an
// address may ask (twenty a second); then it sends bodies that do not read to the proof route at that rate.
// Twenty-five customers start a session meanwhile, one about every 4.8 seconds.
func TestPostgresFreeChallengesDoNotStopCustomers(t *testing.T) {
	free := func(f *entranceFixture) entranceReply { return f.challenge("account", "") }
	junk := func(f *entranceFixture) entranceReply {
		return f.send("POST", "/v2/customer/proof", "", `{"challenge":`)
	}
	for _, probe := range []struct {
		name, what string
		every      time.Duration
		send       func(*entranceFixture) entranceReply
		answers    []int
	}{
		{"free5", "account challenges at 5 a second", 200 * time.Millisecond, free, []int{200}},
		{"free19", "account challenges at 19 a second", 53 * time.Millisecond, free, []int{200}},
		{"junk19", "proof bodies that do not read at 19 a second", 53 * time.Millisecond, junk, []int{400, 429}},
	} {
		t.Run(probe.name, func(t *testing.T) {
			f := newEntranceFixture(t, probe.name)
			served, asked, sent := 0, 0, 0
			between := int(4800 * time.Millisecond / probe.every)
			for passed := time.Duration(0); passed < 2*time.Minute; passed += probe.every {
				sent++
				if reply := probe.send(f); !slices.Contains(probe.answers, reply.status) {
					t.Fatalf("request %d of the sender: %d %s", sent, reply.status, reply.raw)
				}
				f.pass(probe.every)
				if sent%between == 0 && asked < 25 {
					asked++
					if issued, proven := f.checkout(mustKey(t)); issued.status == 200 && proven.status == 200 && proven.body["session"] != nil {
						served++
					}
				}
			}
			limited := f.h.Customer.Limited.Drain()
			t.Logf("%s for 2 minutes (%d requests): %d of %d customers started a session; refusals by kind %v", probe.what, sent, served, asked, limited)
			if served != asked || asked != 25 {
				t.Fatalf("%d of %d customers started a session during %s", served, asked, probe.what)
			}
			// Nothing a customer needs was refused; only the sender's own failed attempts were, past their allowance.
			delete(limited, "failed")
			if len(limited) != 0 {
				t.Fatalf("refusals by kind: %v", limited)
			}
		})
	}
}

// A customer the server has no memory of pays nothing for the failed attempts of someone else. The sender keeps
// the failed attempts of the address used up: twelve bodies that do not read at once, then one every five seconds,
// each just before a customer asks. Twenty-four customers start a session right after a restart, at a store nobody
// has read; then the store takes its page back, the server restarts again, and the customer who paid asks twelve
// times for the order session and the pickup QR of that order.
func TestPostgresFailedAttemptsOfOthersDoNotStopCustomers(t *testing.T) {
	f := newEntranceFixture(t, "strict")
	payer := mustKey(t)
	f.funds.balances[crypto.PubkeyToAddress(payer.PublicKey)] = big.NewInt(300_000)
	_, first := f.checkout(payer)
	if first.status != 200 {
		t.Fatalf("the paying customer's session: %d %s", first.status, first.raw)
	}
	held := f.order(first.body["session"].(string), f.fresh(), f.fresh())
	if held.status != 200 {
		t.Fatalf("the paying customer's cart: %d %s", held.status, held.raw)
	}
	sale := held.body["resultRef"].(string)
	f.paid(sale, payer)
	junk := func() {
		t.Helper()
		if reply := f.send("POST", "/v2/customer/challenge", "", `{"purpose":`); reply.status != 400 && reply.status != 429 {
			t.Fatalf("a body that does not read: %d %s", reply.status, reply.raw)
		}
	}
	f.restart()
	for i := 0; i < 12; i++ {
		junk()
	}
	started := 0
	for i := 0; i < 24; i++ {
		f.pass(5 * time.Second)
		junk()
		if issued, proven := f.checkout(mustKey(t)); issued.status == 200 && proven.status == 200 && proven.body["session"] != nil {
			started++
		}
	}
	f.publish(false)
	f.restart()
	for i := 0; i < 12; i++ {
		junk()
	}
	followed := 0
	for i := 0; i < 12; i++ {
		f.pass(5 * time.Second)
		junk()
		purpose := []string{"order", "pickup"}[i%2]
		issued := f.challenge(purpose, sale)
		if issued.status != 200 {
			continue
		}
		// The order is still being made: its session is given, and its pickup proof says "not ready".
		if proven := f.prove(payer, issued, ""); (purpose == "order" && proven.status == 200 && proven.body["session"] != nil) ||
			(purpose == "pickup" && proven.status == 409 && proven.body["reason"] == "not_ready") {
			followed++
		}
	}
	t.Logf("one body that does not read every 5 seconds: %d of 24 customers started a session after a restart; the customer who paid was answered %d of 12 times at a store that took its page back; refusals by kind %v",
		started, followed, f.h.Customer.Limited.Drain())
	if started != 24 || followed != 12 {
		t.Fatalf("%d of 24 customers started a session and the paid order was answered %d of 12 times", started, followed)
	}
}
