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
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// The answers of the customer entrance, as the phone app reads them, are kept in one file that both sides are held
// to: packages/protocol/v2/testdata/customer-entrance.json. The tests here ask the real routes and compare what they
// answer with that file; the phone app's test (p02 test/customerEntranceContract.test.ts) reads the same file with the
// app's own parsers. A change of an answer therefore fails here first, and the file cannot be changed to suit the
// app without failing here.
//
// What differs from run to run is replaced by the sample value of the file before the comparison: references, names,
// the store's clock, request ids and digests, tokens and times. Their kind (string or number) is still compared.
//
// NU54_WRITE_CUSTOMER_CONTRACT=1 writes the answers of a run into the file instead of comparing.

const customerContractFile = "../../../../packages/protocol/v2/testdata/customer-entrance.json"

// Every answer in the file, by the test that asks for it.
var (
	customerContractPlain    = []string{"busy-rate", "busy-in-flight"}
	customerContractDatabase = []string{"busy-rate-session", "challenge", "proof-checkout", "store-list-page", "store-list-last", "store-page", "store-page-busy",
		"store-page-no-menu", "store-page-closed-day", "store-page-paused", "store-page-unpublished", "menu-unpublished", "photo-unpublished",
		"menu-public", "menu-public-closed-day", "menu-public-paused", "menu-session", "hold-orders-closed", "hold-balance-unavailable", "hold-balance-short", "hold-limit", "hold-other-cart",
		"hold-cart-released", "cancel-released", "cancel-quote-issued", "cancel-paid", "cancel-not-found", "pickup-not-ready", "pickup-token",
		"pickup-collected", "challenge-not-public", "challenge-account", "proof-account"}
)

type customerContractAnswer struct {
	Status     int `json:"status"`
	RetryAfter int `json:"retryAfter,omitempty"`
	// Body is nil for an answer that is not JSON: a plain not found.
	Body any `json:"body"`
}

type customerContract struct {
	About   []string                          `json:"about"`
	Answers map[string]customerContractAnswer `json:"answers"`
}

// Sample values of the file, in the forms the phone app accepts.
const (
	contractCafe      = "0fbc54ef-2827-4ab5-bfcd-3a27611c3002"
	contractBakery    = "0fbc54ef-2827-4ab5-bfcd-3a27611c3003"
	contractSale      = "0fbc54ef-2827-4ab5-bfcd-3a27611c3001"
	contractTenant    = "0fbc54ef-2827-4ab5-bfcd-3a27611c39e0"
	contractSubject   = "0fbc54ef-2827-4ab5-bfcd-3a27611c3004"
	contractRequest   = "0fbc54ef-2827-4ab5-bfcd-3a27611c3005"
	contractCafeName  = "여울 커피"
	contractBakerName = "초록 베이커리"
	contractClock     = "10:31"
	contractWeekday   = float64(4)
	contractSeconds   = float64(1_764_201_600)
	contractExpiry    = "2026-11-27T12:00:00Z"
)

var contractDigest = strings.Repeat("ab", 32)

type contractRun struct {
	t       *testing.T
	names   []string
	answers map[string]customerContractAnswer
	replace *strings.Replacer
}

// setPath puts value at every place path names in doc ("a.b", with "*" for each element of a list), and reports how
// many places it found. The value there must be of the same kind as the sample.
func setPath(t *testing.T, name string, doc any, path []string, value any) int {
	t.Helper()
	if len(path) == 0 {
		return 0
	}
	switch node := doc.(type) {
	case map[string]any:
		child, ok := node[path[0]]
		if !ok {
			return 0
		}
		if len(path) == 1 {
			if reflect.TypeOf(child) != reflect.TypeOf(value) {
				t.Fatalf("%s: %s is %T, the sample is %T", name, path[0], child, value)
			}
			node[path[0]] = value
			return 1
		}
		return setPath(t, name, child, path[1:], value)
	case []any:
		if path[0] != "*" {
			return 0
		}
		found := 0
		for i, child := range node {
			if len(path) == 1 {
				if reflect.TypeOf(child) != reflect.TypeOf(value) {
					t.Fatalf("%s: element %d is %T, the sample is %T", name, i, child, value)
				}
				node[i] = value
				found++
				continue
			}
			found += setPath(t, name, child, path[1:], value)
		}
		return found
	}
	return 0
}

// take records one answer of the server under a name. varies names what differs from run to run, with its sample.
func (c *contractRun) take(name string, status int, header http.Header, raw []byte, varies map[string]any) {
	c.t.Helper()
	if !slices.Contains(c.names, name) {
		c.t.Fatalf("%s is not an answer this test is listed for", name)
	}
	answer := customerContractAnswer{Status: status}
	if wait := header.Get("Retry-After"); wait != "" {
		seconds, err := strconv.Atoi(wait)
		if err != nil || seconds < 1 {
			c.t.Fatalf("%s: Retry-After %q", name, wait)
		}
		answer.RetryAfter = seconds
	}
	text := string(raw)
	if c.replace != nil {
		text = c.replace.Replace(text)
	}
	var body any
	if json.Unmarshal([]byte(text), &body) == nil {
		if _, object := body.(map[string]any); !object {
			c.t.Fatalf("%s: the answer is JSON but no object: %s", name, text)
		}
		if header.Get("Content-Type") != "application/json" || header.Get("Cache-Control") != "no-store" {
			c.t.Fatalf("%s: headers %v", name, header)
		}
		for path, sample := range varies {
			if setPath(c.t, name, body, strings.Split(path, "."), sample) == 0 {
				c.t.Fatalf("%s: nothing at %s in %s", name, path, text)
			}
		}
		answer.Body = body
	}
	if earlier, again := c.answers[name]; again && !reflect.DeepEqual(earlier, answer) {
		c.t.Fatalf("%s was answered twice, differently: %v, then %v", name, earlier, answer)
	}
	c.answers[name] = answer
}

// settle holds every answer of the run against the file, or writes them into it.
func (c *contractRun) settle() {
	c.t.Helper()
	for _, name := range c.names {
		if _, asked := c.answers[name]; !asked {
			c.t.Fatalf("%s was not asked for", name)
		}
	}
	var file customerContract
	raw, err := os.ReadFile(customerContractFile)
	if err == nil {
		err = json.Unmarshal(raw, &file)
	}
	if os.Getenv("NU54_WRITE_CUSTOMER_CONTRACT") == "1" {
		if file.Answers == nil {
			file.Answers = map[string]customerContractAnswer{}
		}
		file.About = []string{
			"Answers of the customer entrance of opsd (p05 internal/commerce), as the phone app reads them.",
			"Written by the tests of p05 internal/commerce/customer_contract_test.go from what the routes answered (NU54_WRITE_CUSTOMER_CONTRACT=1) and compared with them on every run.",
			"Read by the phone app's parsers in p02 test/customerEntranceContract.test.ts. Do not edit by hand.",
			"References, names, the store's clock, request ids and digests, tokens and times are samples; everything else is what the server answered.",
		}
		for name, answer := range c.answers {
			file.Answers[name] = answer
		}
		var out bytes.Buffer
		encoder := json.NewEncoder(&out)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(file); err != nil {
			c.t.Fatal(err)
		}
		if err := os.WriteFile(customerContractFile, out.Bytes(), 0o644); err != nil {
			c.t.Fatal(err)
		}
		return
	}
	if err != nil {
		c.t.Fatalf("%s: %v", customerContractFile, err)
	}
	for _, name := range c.names {
		want, listed := file.Answers[name]
		if !listed {
			c.t.Errorf("%s is not in %s", name, customerContractFile)
			continue
		}
		if got := c.answers[name]; !reflect.DeepEqual(got, want) {
			have, _ := json.Marshal(got)
			kept, _ := json.Marshal(want)
			c.t.Errorf("%s: the server answers\n  %s\nthe file the phone app is held to says\n  %s", name, have, kept)
		}
	}
}

// What the entrance answers without a database: a request refused for its rate, and one that found no place among
// the requests served at once. It also holds the file to the list of answers the two tests ask for.
func TestCustomerContractWithoutDatabase(t *testing.T) {
	run := &contractRun{t: t, names: customerContractPlain, answers: map[string]customerContractAnswer{}}
	ask := func(mux *http.ServeMux) *httptest.ResponseRecorder {
		reply := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v2/customer/challenge", strings.NewReader(`{"purpose":"pickup"}`))
		req.RemoteAddr = "203.0.113.9:4000"
		mux.ServeHTTP(reply, req)
		return reply
	}
	at := time.Unix(1_764_201_600, 0)
	limited := http.NewServeMux()
	HTTP{Customer: &CustomerEntrance{Limiter: NewRateLimiter(20, 1, func() time.Time { return at })}}.RegisterCustomer(limited)
	if first := ask(limited); first.Code != http.StatusBadRequest {
		t.Fatalf("the first request: %d", first.Code)
	}
	refused := ask(limited) // 20 a minute: the next request is three seconds away
	run.take("busy-rate", refused.Code, refused.Header(), refused.Body.Bytes(), nil)

	full := http.NewServeMux()
	entrance := &CustomerEntrance{InFlight: make(chan struct{}, 1), InFlightWait: 5 * time.Millisecond}
	entrance.InFlight <- struct{}{}
	HTTP{Customer: entrance}.RegisterCustomer(full)
	// A challenge that reads, for a store: only a request an app sends waits for a place among the requests served
	// at once. (An `account` challenge asks nothing of the database and needs no place.)
	crowded := httptest.NewRecorder()
	full.ServeHTTP(crowded, httptest.NewRequest("POST", "/v2/customer/challenge", strings.NewReader(`{"purpose":"checkout","branchRef":"`+contractCafe+`"}`)))
	run.take("busy-in-flight", crowded.Code, crowded.Header(), crowded.Body.Bytes(), nil)
	run.settle()

	if os.Getenv("NU54_WRITE_CUSTOMER_CONTRACT") == "1" {
		return
	}
	raw, err := os.ReadFile(customerContractFile)
	if err != nil {
		t.Fatal(err)
	}
	var file customerContract
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	var kept []string
	for name := range file.Answers {
		kept = append(kept, name)
	}
	asked := append(slices.Clone(customerContractPlain), customerContractDatabase...)
	sort.Strings(kept)
	sort.Strings(asked)
	if !slices.Equal(kept, asked) {
		t.Fatalf("the file holds %v; the tests ask for %v", kept, asked)
	}
}

// TestPostgresCustomerContract asks a migrated database for every other answer the phone app reads: the store list
// and its pages, a store page in each state it can say, a store that is not public, the public menu and a session's
// menu, a cart that is not held, a cart taken back, and the pickup proof.
func TestPostgresCustomerContract(t *testing.T) {
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
	role := "nu54_contract_" + strings.TrimPrefix(config.Database, "nu54_commerce_")
	execute("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	store, err := Open(ctx, config.ConnString()+" user="+role)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fresh := func() string { return uuidForCatalogFixture(t, ctx, admin) }
	before, cafe, bakery, hidden, operator := fresh(), fresh(), fresh(), fresh(), fresh()
	// The three listed stores are named to come last in the list, in this order, whatever other tests listed: the
	// pages asked for below then hold these stores and nothing else. Stores an earlier run left listed are taken out.
	const last = "힣 계약 "
	unlist := func() {
		_, _ = admin.Exec(context.Background(), `UPDATE ledger.branch_store_page SET listed=false WHERE branch_id IN
			(SELECT branch_id FROM ledger.operator_branch_label WHERE label LIKE $1)`, last+"%")
	}
	unlist()
	defer unlist()
	names := map[string]string{before: last + "1 " + cafe[:8], cafe: last + "2 " + cafe[:8], bakery: last + "3 " + cafe[:8], hidden: last + "4 " + cafe[:8]}
	hours := `[{"opens":"10:00","closes":"21:00"},{"opens":"10:00","closes":"21:00"},{"opens":"10:00","closes":"21:00"},{"opens":"10:00","closes":"21:00"},{"opens":"10:00","closes":"21:00"},{"opens":"11:00","closes":"20:00"},null]`
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", operator, "contract_"+operator)
	for _, b := range []string{before, cafe, bakery, hidden} {
		execute("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", b, tenant)
		execute("INSERT INTO ledger.command_rule VALUES ($1,'contract test','fixture','dispose',interval '2 minutes')", b)
		execute("INSERT INTO ledger.operator_branch_label VALUES ($1,$2)", b, names[b])
		execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", b, operator)
	}
	page := func(branch string, published bool, kind, intro, address, directions, phone, amenities, parking, closedNote string, week *string) {
		t.Helper()
		execute(`INSERT INTO ledger.branch_store_page VALUES ($1,$2,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,now())`,
			branch, published, kind, intro, address, directions, phone, amenities, parking, closedNote, week, operator)
	}
	page(before, true, "분식", "", "", "", "", "", "", "", nil)
	page(cafe, true, "카페", "직접 볶은 원두로 내립니다", "시험시 시험로 1", "역 2번 출구에서 걸어서 3분", "02-555-0100", "무선 인터넷", "건물 뒤 2대", "매주 일요일", &hours)
	page(bakery, true, "베이커리", "", "시험시 초록로 2", "", "", "", "", "", nil)
	page(hidden, false, "카페", "", "", "", "", "", "", "", nil)

	// One operator certificate, to publish the cafe's menu the way a store does.
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
	credentials := map[string]service.Principal{hex.EncodeToString(leafDigest[:]): {Tenant: tenant, Subject: operator, Role: "operator",
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix()}}

	asset := "0x" + strings.Repeat("2", 40)
	merchant := [32]byte{0: 0xe1, 31: 0x07}
	certified, entrance := http.NewServeMux(), http.NewServeMux()
	server := httptest.NewUnstartedServer(certified)
	defer server.Close()
	origin := "https://" + server.Listener.Addr().String()
	h := HTTP{Store: store, OperatorOrigin: origin, Credentials: credentials,
		Customer: &CustomerEntrance{OperatorContact: "help@operator.example", ListPage: 1},
		Profiles: map[string]Profile{cafe: {Chain: 8283, Contract: common.HexToAddress("0x" + strings.Repeat("3", 40)), Asset: common.HexToAddress(asset),
			Beneficiary: common.HexToAddress("0x" + strings.Repeat("4", 40)), Signer: common.HexToAddress("0x" + strings.Repeat("5", 40)),
			MerchantID: merchant, KeyVersion: 2, BeneficiaryVersion: 3, Escrow: true, Token: wonForOptions}}}
	h.Register(certified)
	h.RegisterCustomer(entrance)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	server.StartTLS()
	customers := httptest.NewServer(entrance)
	defer customers.Close()
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
	// The cafe's menu: tea, five of it, and a cake that ran out.
	operate("/v2/operations/catalog/"+cafe+"/publish", map[string]any{"requestKey": fresh(), "version": 0, "expiresAt": time.Now().Add(time.Hour).UTC(),
		"rows": []operatorRow{{SKU: "cake", Option: "standard", Label: "cake", Asset: asset, DisplayPrice: "5500"},
			{SKU: "tea", Option: "standard", Label: "tea", Asset: asset, DisplayPrice: "3000"}}})
	operate("/v2/operations/catalog/"+cafe+"/stock", map[string]any{"requestKey": fresh(), "version": 1, "sku": "tea", "delta": 5})

	board := mustKey(t)
	account := crypto.PubkeyToAddress(board.PublicKey)
	run := &contractRun{t: t, names: customerContractDatabase, answers: map[string]customerContractAnswer{}}
	replacements := []string{names[cafe], contractCafeName, names[bakery], contractBakerName, cafe, contractCafe, bakery, contractBakery,
		tenant, contractTenant, CustomerSubject(account), contractSubject}
	run.replace = strings.NewReplacer(replacements...)

	type reply struct {
		status int
		header http.Header
		raw    []byte
		body   map[string]any
	}
	send := func(method, path, session string, body any) reply {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		req, _ := http.NewRequestWithContext(ctx, method, customers.URL+path, reader)
		if session != "" {
			req.Header.Set("Authorization", "Bearer "+session)
		}
		got, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer got.Body.Close()
		raw, _ := io.ReadAll(got.Body)
		out := reply{status: got.StatusCode, header: got.Header, raw: raw}
		_ = json.Unmarshal(raw, &out.body)
		return out
	}
	take := func(name string, got reply, varies map[string]any) reply {
		t.Helper()
		run.take(name, got.status, got.header, got.raw, varies)
		return got
	}
	clock := map[string]any{"status.time": contractClock, "status.weekday": contractWeekday}

	// --- the store list, a page at a time: the page after the first of the three, then the last ---
	listed := take("store-list-page", send("GET", "/v2/customer/stores?after="+before, "", nil),
		map[string]any{"stores.*.status.time": contractClock, "stores.*.status.weekday": contractWeekday})
	if rows := listed.body["stores"].([]any); len(rows) != 1 || rows[0].(map[string]any)["branchRef"] != cafe || listed.body["next"] != cafe {
		t.Fatalf("the page after the first store: %s", listed.raw)
	}
	end := take("store-list-last", send("GET", "/v2/customer/stores?after="+cafe, "", nil),
		map[string]any{"stores.*.status.time": contractClock, "stores.*.status.weekday": contractWeekday})
	if rows := end.body["stores"].([]any); len(rows) != 1 || rows[0].(map[string]any)["branchRef"] != bakery || end.body["next"] != nil {
		t.Fatalf("the last page: %s", end.raw)
	}

	// --- a store page, in each state it can say ---
	take("store-page", send("GET", "/v2/customer/stores/"+cafe, "", nil), clock)
	execute("INSERT INTO ledger.branch_order_state VALUES ($1,'busy',20,NULL,$2,now())", cafe, operator)
	take("store-page-busy", send("GET", "/v2/customer/stores/"+cafe, "", nil), clock)
	execute("DELETE FROM ledger.branch_order_state WHERE branch_id=$1", cafe)
	take("store-page-no-menu", send("GET", "/v2/customer/stores/"+bakery, "", nil), clock)
	execute("INSERT INTO ledger.branch_closed_day VALUES ($1,ledger.branch_today($1,clock_timestamp()),$2,now())", bakery, operator)
	take("store-page-closed-day", send("GET", "/v2/customer/stores/"+bakery, "", nil), clock)
	execute("DELETE FROM ledger.branch_closed_day WHERE branch_id=$1", bakery)
	execute("INSERT INTO ledger.branch_order_state VALUES ($1,'paused',NULL,'11:00',$2,now())", bakery, operator)
	take("store-page-paused", send("GET", "/v2/customer/stores/"+bakery, "", nil), clock)
	execute("DELETE FROM ledger.branch_order_state WHERE branch_id=$1", bakery)

	// --- a store that did not publish its page answers like a branch that does not exist: page, menu and photo ---
	photo := strings.Repeat("ab", 32)
	nowhere := fresh()
	for name, path := range map[string]string{"store-page-unpublished": "", "menu-unpublished": "/menu/0", "photo-unpublished": "/photos/" + photo + "/0"} {
		unpublished := take(name, send("GET", "/v2/customer/stores/"+hidden+path, "", nil), nil)
		if unknown := send("GET", "/v2/customer/stores/"+nowhere+path, "", nil); unknown.status != unpublished.status || !bytes.Equal(unknown.raw, unpublished.raw) {
			t.Fatalf("%s: a store that is not public answers %d %q, a branch that does not exist %d %q", name, unpublished.status, unpublished.raw, unknown.status, unknown.raw)
		}
	}

	// --- the public menu: on sale or sold out, never a count ---
	take("menu-public", send("GET", "/v2/customer/stores/"+cafe+"/menu/0", "", nil), map[string]any{"catalog.*.expiresAt": contractExpiry})
	// A store that takes no order now still shows its menu to read, with the state that says why (migration 128).
	execute("INSERT INTO ledger.branch_closed_day VALUES ($1,ledger.branch_today($1,clock_timestamp()),$2,now())", cafe, operator)
	take("menu-public-closed-day", send("GET", "/v2/customer/stores/"+cafe+"/menu/0", "", nil), map[string]any{"catalog.*.expiresAt": contractExpiry})
	execute("DELETE FROM ledger.branch_closed_day WHERE branch_id=$1", cafe)
	execute("INSERT INTO ledger.branch_order_state VALUES ($1,'paused',NULL,'11:00',$2,now())", cafe, operator)
	take("menu-public-paused", send("GET", "/v2/customer/stores/"+cafe+"/menu/0", "", nil), map[string]any{"catalog.*.expiresAt": contractExpiry})
	execute("DELETE FROM ledger.branch_order_state WHERE branch_id=$1", cafe)

	// --- the account proof, and the menu its session reads: the same rows with what is left ---
	prove := func(purpose, sale string) reply {
		t.Helper()
		asked := map[string]string{"purpose": purpose, "branchRef": cafe}
		if sale != "" {
			asked["saleRef"] = sale
		}
		issued := send("POST", "/v2/customer/challenge", "", asked)
		if purpose == "checkout" {
			take("challenge", issued, map[string]any{"challenge": contractDigest, "expiresAt": contractSeconds})
		}
		value, err := hex.DecodeString(issued.body["challenge"].(string))
		if err != nil || len(value) != 32 {
			t.Fatalf("challenge: %s", issued.raw)
		}
		proof := map[string]string{"challenge": hex.EncodeToString(value), "account": strings.ToLower(account.Hex()),
			"signature": hex.EncodeToString(signAccountProof(t, board, [32]byte(value)))}
		if purpose == "checkout" {
			proof["generation"] = "7"
		}
		return send("POST", "/v2/customer/proof", "", proof)
	}
	proven := take("proof-checkout", prove("checkout", ""), map[string]any{"session": contractDigest, "expiresAt": contractSeconds})
	session := proven.body["session"].(string)
	bound := map[string]any{"requestId": contractRequest, "requestDigest": contractDigest}
	command := func(operation, object, key string, fields map[string]string, cart *Cart) reply {
		t.Helper()
		envelope := map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: operation, RequestID: fresh(),
			Domain: cafe, Object: object, ExpiresAt: time.Now().Unix() + 30, IdempotencyKey: key, Body: fields}}
		if cart != nil {
			envelope["cart"] = cart
		}
		return send("POST", "/v2/customer/commerce", session, envelope)
	}
	take("menu-session", command("catalog.read", cafe, "", map[string]string{"page": "0"}, nil),
		map[string]any{"requestId": contractRequest, "requestDigest": contractDigest, "catalog.*.expiresAt": contractExpiry})

	// --- a cart that is not held: the balance cannot be read, the balance is short, too many carts ---
	order := func() reply {
		t.Helper()
		ref := fresh()
		cart := Cart{Origin: "menu", OriginRef: ref, MenuVersion: 1, Items: []Item{{SKU: "tea", Option: "standard", Qty: 1}}}
		packed, _ := json.Marshal(cart)
		digest := sha256.Sum256(packed)
		return command("order.create", ref, fresh(), map[string]string{"asset": asset, "quantity": "1", "intentDigest": hex.EncodeToString(digest[:])}, &cart)
	}
	take("hold-orders-closed", order(), bound) // no chain reader for the store: a cart is not held on trust, and waiting does not help
	funds := &fakeFunds{asset: common.HexToAddress(asset), err: errors.New("rpc down")}
	h.Customer.Funds = funds
	take("hold-balance-unavailable", order(), bound)    // the chain could not be read now
	funds.err, funds.balance = nil, big.NewInt(299_999) // the tea costs 300000 base units
	take("hold-balance-short", order(), bound)
	h.Customer.HoldAttempts = NewRateLimiter(6, 1, nil)
	order()
	take("hold-limit", order(), bound)
	h.Customer.HoldAttempts = nil
	funds.balance = big.NewInt(300_000)
	heldRef, heldKey := fresh(), fresh()
	orderAs := func(ref, key string) reply {
		t.Helper()
		cart := Cart{Origin: "menu", OriginRef: ref, MenuVersion: 1, Items: []Item{{SKU: "tea", Option: "standard", Qty: 1}}}
		packed, _ := json.Marshal(cart)
		digest := sha256.Sum256(packed)
		return command("order.create", ref, key, map[string]string{"asset": asset, "quantity": "1", "intentDigest": hex.EncodeToString(digest[:])}, &cart)
	}
	held := orderAs(heldRef, heldKey)
	if held.status != 200 {
		t.Fatalf("a cart its account can pay: %d %s", held.status, held.raw)
	}
	first := held.body["resultRef"].(string)
	take("hold-other-cart", order(), bound) // one unpaid cart for each account

	// --- the customer takes the unpaid cart back ---
	cancel := func(sale string) reply {
		t.Helper()
		return send("POST", "/v2/customer/checkout/cancel", session, map[string]string{"saleRef": sale})
	}
	inSale := func(sale string) *strings.Replacer {
		return strings.NewReplacer(append(slices.Clone(replacements), sale, contractSale)...)
	}
	run.replace = inSale(first)
	take("cancel-released", cancel(first), nil)
	take("cancel-not-found", cancel(fresh()), nil)
	// The same cart request again, after the cart was taken back: not the closed order as if it were held.
	take("hold-cart-released", orderAs(heldRef, heldKey), bound)
	second := order()
	if second.status != 200 {
		t.Fatalf("a cart after the first was taken back: %d %s", second.status, second.raw)
	}
	sale := second.body["resultRef"].(string)
	run.replace = inSale(sale)
	seed := func(statements func()) {
		t.Helper()
		execute("SET session_replication_role=replica")
		statements()
		execute("SET session_replication_role=origin")
	}
	seed(func() { execute(`UPDATE ledger.sale SET state='issued' WHERE id=$1`, sale) })
	take("cancel-quote-issued", cancel(sale), nil)
	// The order is paid (seeded, as in TestPostgresCustomerAccountProof): quote, payment, pickup number, escrow.
	seed(func() {
		execute(`UPDATE ledger.sale SET state='paid' WHERE id=$1`, sale)
		execute(`INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,
			payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
			VALUES($1,$2,decode(md5(random()::text)||md5(random()::text),'hex'),decode(md5(random()::text)||md5(random()::text),'hex'),decode(md5(random()::text)||md5(random()::text),'hex'),decode(md5(random()::text)||md5(random()::text),'hex'),0,1,substring(decode(md5(random()::text)||md5(random()::text),'hex') FROM 1 FOR 20),
			$3,1,1,now(),now()+interval '1 minute',1,now(),'x',decode(md5(random()::text)||md5(random()::text),'hex'))`, sale, CustomerSubject(account), account[:])
		execute(`INSERT INTO ledger.payment(sale_id,stream_id,merchant_key,evidence_id,tx_hash,log_index,inclusion_hash,included_at,asset_ref,payer,
			payer_generation,payment_mode,principal) VALUES($1,gen_random_uuid(),'0x'||repeat('ab',32),gen_random_uuid(),'0x'||repeat('cd',32),0,
			'0x'||repeat('ef',32),now(),$2,$3,1,1,300000)`, sale, asset, strings.ToLower(account.Hex()))
		execute(`INSERT INTO ledger.pickup(sale_id,branch_id,business_day,number) VALUES($1,$2,current_date,7)`, sale, cafe)
		execute(`INSERT INTO ledger.order_escrow(sale_id,contract_ref,confirmer_ref,deliver_by,confirm_window) VALUES($1,$2,$2,$3,172800)`,
			sale, "0x"+strings.Repeat("4", 40), time.Now().Add(24*time.Hour).Unix())
		execute(`INSERT INTO ledger.travel_pickup_progress(sale_id,state,revision,updated_at) VALUES($1,'preparing',1,now())`, sale)
	})
	take("cancel-paid", cancel(sale), nil)

	// --- the pickup proof: still being made, ready, and already handed over ---
	take("pickup-not-ready", prove("pickup", sale), nil)
	seed(func() {
		execute(`UPDATE ledger.travel_pickup_progress SET state='ready',revision=2,updated_at=now() WHERE sale_id=$1`, sale)
	})
	take("pickup-token", prove("pickup", sale), map[string]any{"pickupToken": contractDigest, "expiresAt": contractSeconds})
	seed(func() {
		execute(`UPDATE ledger.travel_pickup_progress SET state='collected',revision=3,updated_at=now() WHERE sale_id=$1`, sale)
	})
	take("pickup-collected", prove("pickup", sale), map[string]any{"recordedAt": contractSeconds})

	// --- a store that is not public gives no challenge, like a branch that does not exist ---
	closed := take("challenge-not-public", send("POST", "/v2/customer/challenge", "", map[string]string{"purpose": "checkout", "branchRef": hidden}), nil)
	if unknown := send("POST", "/v2/customer/challenge", "", map[string]string{"purpose": "checkout", "branchRef": nowhere}); unknown.status != closed.status || !bytes.Equal(unknown.raw, closed.raw) {
		t.Fatalf("a store that is not public answers a challenge %d %q, a branch that does not exist %d %q", closed.status, closed.raw, unknown.status, unknown.raw)
	}

	// --- the account alone: how many of its orders are unfinished, and nothing else ---
	asked := take("challenge-account", send("POST", "/v2/customer/challenge", "", map[string]string{"purpose": "account"}),
		map[string]any{"challenge": contractDigest, "expiresAt": contractSeconds})
	value, err := hex.DecodeString(asked.body["challenge"].(string))
	if err != nil || len(value) != 32 {
		t.Fatalf("account challenge: %s", asked.raw)
	}
	take("proof-account", send("POST", "/v2/customer/proof", "", map[string]string{"challenge": hex.EncodeToString(value), "account": strings.ToLower(account.Hex()),
		"signature": hex.EncodeToString(signAccountProof(t, board, [32]byte(value)))}), nil)

	// --- a session over its rate is refused like any other request: no word about the session ---
	h.Customer.SessionLimiter = NewRateLimiter(20, 1, nil)
	command("order.status", sale, "", map[string]string{}, nil)
	take("busy-rate-session", command("order.status", sale, "", map[string]string{}, nil), nil)
	run.settle()
}
