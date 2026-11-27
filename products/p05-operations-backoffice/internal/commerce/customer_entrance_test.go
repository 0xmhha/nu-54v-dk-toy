package commerce

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// signAccountProof signs like the board's NU54_ACCOUNT_PROOF_OP: the EIP-712 account digest, v as 27 or 28.
func signAccountProof(t *testing.T, key *ecdsa.PrivateKey, challenge [32]byte) []byte {
	t.Helper()
	digest := orderescrow.AccountProofDigest(crypto.PubkeyToAddress(key.PublicKey), challenge)
	signature, err := crypto.Sign(digest[:], key)
	if err != nil {
		t.Fatal(err)
	}
	signature[64] += 27
	return signature
}

func TestAccountProofDigestMatchesTheBoard(t *testing.T) {
	// The vector pinned in products/p01-device-firmware/test/test_order.c and packages/protocol (Go and TypeScript).
	account := common.HexToAddress("0x3004df73ebac3f1a2d5a0ea01ed39503139b9dcb")
	var challenge [32]byte
	copy(challenge[:], bytes.Repeat([]byte{0x11}, 32))
	if got := orderescrow.AccountProofDigest(account, challenge); hex.EncodeToString(got[:]) !=
		"6ac8be6e20f7c3971e894cfd18e30676ef1032d039b624d7a244cf7742b490c8" {
		t.Fatalf("account proof digest differs from the board: %x", got)
	}
}

func TestVerifyAccountProof(t *testing.T) {
	key, _ := crypto.GenerateKey()
	other, _ := crypto.GenerateKey()
	account := crypto.PubkeyToAddress(key.PublicKey)
	challenge := [32]byte{1, 2, 3}
	signature := signAccountProof(t, key, challenge)
	if !verifyAccountProof(account, challenge, signature) {
		t.Fatal("the board's own proof was refused")
	}
	if verifyAccountProof(crypto.PubkeyToAddress(other.PublicKey), challenge, signature) {
		t.Fatal("a proof was accepted for another account")
	}
	if verifyAccountProof(account, [32]byte{9}, signature) {
		t.Fatal("a proof was accepted for another challenge")
	}
	if verifyAccountProof(account, [32]byte{}, signAccountProof(t, key, [32]byte{})) {
		t.Fatal("a zero challenge was accepted; the board never signs one")
	}
	if verifyAccountProof(account, challenge, signature[:64]) || verifyAccountProof(account, challenge, make([]byte, 65)) {
		t.Fatal("a malformed signature was accepted")
	}
}

func TestCustomerSubject(t *testing.T) {
	a, b := common.HexToAddress("0x"+strings.Repeat("1", 40)), common.HexToAddress("0x"+strings.Repeat("2", 40))
	subject := CustomerSubject(a)
	if subject != CustomerSubject(a) || subject == CustomerSubject(b) {
		t.Fatal("the subject must follow the account, one to one")
	}
	if !operatorRef.MatchString(subject) {
		t.Fatalf("not a version 4 UUID the C3 boundary accepts: %s", subject)
	}
	// The phone derives the same subject before any proof (p02 storeAccess.ts, storeAccess.test.ts holds this value too).
	if subject != "1a07c82c-ec0d-4eca-98d2-6aec9f1bb156" {
		t.Fatalf("the derivation changed: %s", subject)
	}
	// The existing fixed subjects are random; the derivation is domain separated from them.
	for _, existing := range []string{customerA, customerB, terminal} {
		if subject == existing {
			t.Fatal("collision with a store-issued subject")
		}
	}
}

func TestRateLimiterPerClient(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter := NewRateLimiter(6, 3, func() time.Time { return now }) // 6 a minute, 3 at once
	for i := 0; i < 3; i++ {
		if !limiter.Allow("192.0.2.1") {
			t.Fatal("the burst was refused")
		}
	}
	if limiter.Allow("192.0.2.1") {
		t.Fatal("a fourth request at once was allowed")
	}
	if !limiter.Allow("192.0.2.2") {
		t.Fatal("another client shares the first one's limit")
	}
	now = now.Add(10 * time.Second) // one token back
	if !limiter.Allow("192.0.2.1") || limiter.Allow("192.0.2.1") {
		t.Fatal("the limit does not refill at its rate")
	}

	h := HTTP{Customer: &CustomerEntrance{Limiter: NewRateLimiter(60, 1, func() time.Time { return now })}}
	mux := http.NewServeMux()
	h.RegisterCustomer(mux)
	send := func() int {
		req := httptest.NewRequest("POST", "/v2/customer/challenge", strings.NewReader("{"))
		req.RemoteAddr = "198.51.100.7:4000"
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, req)
		return reply.Code
	}
	if code := send(); code != http.StatusBadRequest {
		t.Fatalf("first request: %d", code)
	}
	if code := send(); code != http.StatusTooManyRequests {
		t.Fatalf("second request at once: %d, want 429", code)
	}
}

func TestCustomerSessionScope(t *testing.T) {
	sale := "6f1c1a52-4c39-4d1e-9d36-1f0b9b4d2a10"
	other := "0c7e0e9e-8f5b-4b8e-a3f4-6a7f7b0e5d21"
	request := func(operation, domain, object string) service.Request {
		return service.Request{Contract: "C3", Operation: operation, Domain: domain, Object: object}
	}
	checkout := customerSession{Branch: branch, Purpose: "checkout"}
	for _, operation := range []string{"order.create", "checkout.claim", "checkout.authorize", "quote.issue", "order.lock", "payment.forward", "order.status"} {
		if !checkout.allows(request(operation, branch, sale)) {
			t.Fatalf("checkout session refused %s", operation)
		}
	}
	for _, operation := range []string{"catalog.read", "catalog.photo"} {
		if !checkout.allows(request(operation, branch, branch)) || checkout.allows(request(operation, branch, sale)) {
			t.Fatalf("checkout session reads %s outside its branch", operation)
		}
	}
	for _, refused := range []service.Request{
		request("order.create", other, sale), // another branch
		request("order.answer", branch, sale),
		request("payer.enroll", branch, sale),
		request("wallet.return", branch, sale),
		request("terminal.state", branch, branch),
		request("order.list", branch, sale),
		request("order.cancel-public", branch, sale), // the kiosk's cancel, never a customer's
		request("checkout.request", branch, sale),
		request("checkout.confirm", branch, sale),
	} {
		if checkout.allows(refused) {
			t.Fatalf("checkout session allowed %s on %s", refused.Operation, refused.Domain)
		}
	}
	order := customerSession{Branch: branch, Purpose: "order", Sale: sale}
	for _, operation := range []string{"order.status", "order.answer"} {
		if !order.allows(request(operation, branch, sale)) {
			t.Fatalf("order session refused %s on its order", operation)
		}
		if order.allows(request(operation, branch, other)) || order.allows(request(operation, other, sale)) {
			t.Fatalf("order session reached another order with %s", operation)
		}
	}
	for _, operation := range []string{"order.create", "checkout.claim", "quote.issue", "order.lock", "catalog.read", "order.cancel-public"} {
		if order.allows(request(operation, branch, sale)) {
			t.Fatalf("order session allowed %s", operation)
		}
	}
}

// The public menu and its photos are under the per-client limit, and photo parts have an allowance of their own.
func TestPublicMenuRateLimit(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }
	photo := "/v2/customer/stores/" + branch + "/photos/" + strings.Repeat("a", 64) + "/0"
	menu := "/v2/customer/stores/" + branch + "/menu/0"
	send := func(mux *http.ServeMux, path, client string) int {
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = client + ":4000"
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, req)
		return reply.Code
	}
	// Without a store behind it an allowed request ends as not found; a limited one never gets that far.
	apart := http.NewServeMux()
	apartLimits := &CustomerEntrance{Limiter: NewRateLimiter(60, 1, clock), ReadLimiter: NewRateLimiter(60, 2, clock), PhotoLimiter: NewRateLimiter(60, 1, clock)}
	HTTP{Customer: apartLimits}.RegisterCustomer(apart)
	for i, want := range []int{http.StatusNotFound, http.StatusNotFound, http.StatusTooManyRequests} {
		if code := send(apart, menu, "198.51.100.7"); code != want {
			t.Fatalf("menu request %d: %d, want %d", i+1, code, want)
		}
	}
	if code := send(apart, menu, "198.51.100.8"); code != http.StatusNotFound {
		t.Fatalf("another client shares the limit: %d", code)
	}
	// The same client's photo parts are counted apart from its other requests, and limited too.
	for i, want := range []int{http.StatusNotFound, http.StatusTooManyRequests} {
		if code := send(apart, photo, "198.51.100.7"); code != want {
			t.Fatalf("photo request %d: %d, want %d", i+1, code, want)
		}
	}
	now = now.Add(time.Second) // one request back in each allowance
	if send(apart, menu, "198.51.100.7") != http.StatusNotFound || send(apart, photo, "198.51.100.7") != http.StatusNotFound ||
		send(apart, menu, "198.51.100.7") != http.StatusTooManyRequests || send(apart, photo, "198.51.100.7") != http.StatusTooManyRequests {
		t.Fatal("the limits do not refill at their rates")
	}
	// Reading the menu did not use up what the same address needs to prove an account, and the reverse.
	proving := func(mux *http.ServeMux, client string) (int, string) {
		req := httptest.NewRequest("POST", "/v2/customer/challenge", strings.NewReader("{"))
		req.RemoteAddr = client + ":4000"
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, req)
		return reply.Code, reply.Header().Get("Retry-After")
	}
	if code, _ := proving(apart, "198.51.100.7"); code != http.StatusBadRequest {
		t.Fatalf("a limited reader cannot prove its account: %d", code)
	}
	// Retry-After is the real wait: one request a second here, so one second, not a fixed ten.
	if code, wait := proving(apart, "198.51.100.7"); code != http.StatusTooManyRequests || wait != "1" {
		t.Fatalf("second proof at once: %d, Retry-After %q", code, wait)
	}
	now = now.Add(time.Second)
	if send(apart, menu, "198.51.100.7") != http.StatusNotFound {
		t.Fatal("a limited prover cannot read the menu")
	}
	// The health route is outside every limit, and each refusal was counted by its kind.
	for i := 0; i < 5; i++ {
		if code := send(apart, "/healthz", "198.51.100.7"); code != http.StatusOK {
			t.Fatalf("health: %d", code)
		}
	}
	if counts := apartLimits.Limited.Drain(); counts["read"] != 2 || counts["photo"] != 2 || counts["proof"] != 1 || counts["session"] != 0 {
		t.Fatalf("refusals by kind: %v", counts)
	}
	if counts := apartLimits.Limited.Drain(); len(counts) != 0 {
		t.Fatalf("the count was not drained: %v", counts)
	}
	slow := NewRateLimiter(6, 1, clock) // one request every ten seconds
	if ok, _ := slow.Take("a"); !ok {
		t.Fatal("first request refused")
	}
	if ok, wait := slow.Take("a"); ok || wait != 10*time.Second {
		t.Fatalf("the wait is not the refill time: %v %v", ok, wait)
	}
	// A deployment that sets no photo limit keeps photos under the one limit.
	shared := http.NewServeMux()
	HTTP{Customer: &CustomerEntrance{Limiter: NewRateLimiter(60, 2, clock)}}.RegisterCustomer(shared)
	if send(shared, photo, "198.51.100.9") != http.StatusNotFound || send(shared, menu, "198.51.100.9") != http.StatusNotFound ||
		send(shared, photo, "198.51.100.9") != http.StatusTooManyRequests {
		t.Fatal("photos escaped the entrance limit")
	}
}

// Only the store list, a store page, a menu page and a menu photo are read without a proof. Everything a session
// does answers not found without one, and the public reads take no other method, page form or query.
func TestPublicMenuIsTheOnlyReadWithoutProof(t *testing.T) {
	mux := http.NewServeMux()
	HTTP{Customer: &CustomerEntrance{}}.RegisterCustomer(mux)
	send := func(method, path, body string) int {
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, httptest.NewRequest(method, path, strings.NewReader(body)))
		return reply.Code
	}
	for _, operation := range []string{"catalog.read", "order.create", "checkout.claim", "checkout.authorize", "quote.issue", "order.lock",
		"payment.forward", "order.status", "order.answer", "order.list"} {
		body := `{"command":{"version":2,"contract":"C3","operation":"` + operation + `","requestId":"` + branch + `","domainRef":"` + branch +
			`","objectRef":"` + branch + `","expiresAt":4000000000,"body":{}}}`
		if code := send("POST", "/v2/customer/commerce", body); code != http.StatusNotFound {
			t.Fatalf("%s without a session answered %d", operation, code)
		}
	}
	store := "/v2/customer/stores/" + branch
	for _, path := range []string{store + "/menu", store + "/menu/01", store + "/menu/-1", store + "/menu/x", store + "/menu/32768", store + "/menu/0?page=1",
		store + "/menu/0/0", store + "/orders", store + "/photos/" + strings.Repeat("a", 64), store + "/photos/" + strings.Repeat("A", 64) + "/0",
		store + "/photos/" + strings.Repeat("a", 64) + "/153601", store + "/photos/" + strings.Repeat("a", 64) + "/00", "/v2/customer/stores/x/menu/0"} {
		if code := send("GET", path, ""); code != http.StatusNotFound {
			t.Fatalf("GET %s answered %d", path, code)
		}
	}
	for _, path := range []string{store + "/menu/0", store + "/photos/" + strings.Repeat("a", 64) + "/0"} {
		for _, method := range []string{"POST", "PUT", "DELETE"} {
			if code := send(method, path, "{}"); code != http.StatusMethodNotAllowed && code != http.StatusNotFound {
				t.Fatalf("%s %s answered %d", method, path, code)
			}
		}
	}
}

// A store page tells the store's number in the order contract and the label of its token, and nothing else of the
// payment profile. A store that is not paid through the order contract has no number to tell.
func TestStorePagePaymentFields(t *testing.T) {
	profile := Profile{Chain: 8283, Contract: common.HexToAddress("0x" + strings.Repeat("3", 40)), Asset: common.HexToAddress("0x" + strings.Repeat("2", 40)),
		Beneficiary: common.HexToAddress("0x" + strings.Repeat("4", 40)), Signer: common.HexToAddress("0x" + strings.Repeat("5", 40)),
		MerchantID: [32]byte{0: 0xe1, 31: 0x07}, KeyVersion: 2, BeneficiaryVersion: 3, Escrow: true}
	got := storePayment(profile)
	if len(got) != 2 || got["paymentToken"] != "TestUSDC" || got["merchantId"] != "e1"+strings.Repeat("0", 60)+"07" {
		t.Fatalf("escrow store: %v", got)
	}
	profile.Escrow = false
	if got := storePayment(profile); len(got) != 1 || got["merchantId"] != "" {
		t.Fatalf("a store outside the order contract told a merchant number: %v", got)
	}
	profile.Escrow, profile.MerchantID = true, [32]byte{}
	if got := storePayment(profile); len(got) != 1 {
		t.Fatalf("an unset merchant number was told: %v", got)
	}
}

// The customer entrance adds only its own routes; nothing of the certificate side becomes reachable without a certificate.
func TestCustomerEntranceKeepsCertificateRoutesApart(t *testing.T) {
	h := HTTP{Customer: &CustomerEntrance{}}
	customer := http.NewServeMux()
	h.RegisterCustomer(customer)
	for _, path := range []string{"/v2/commerce", "/v2/operations/state/" + branch, "/v2/operations/pickup-scan/" + branch + "/" + branch} {
		reply := httptest.NewRecorder()
		customer.ServeHTTP(reply, httptest.NewRequest("POST", path, strings.NewReader("{}")))
		if reply.Code != http.StatusNotFound {
			t.Fatalf("%s answered %d on the customer entrance", path, reply.Code)
		}
	}
	certified := http.NewServeMux()
	h.Register(certified)
	for _, path := range []string{"/v2/commerce", "/v2/customer/commerce", "/v2/customer/challenge", "/v2/customer/proof",
		"/v2/operations/pickup-scan/" + branch + "/" + branch} {
		reply := httptest.NewRecorder()
		certified.ServeHTTP(reply, httptest.NewRequest("POST", path, strings.NewReader("{}")))
		if reply.Code != http.StatusNotFound {
			t.Fatalf("%s answered %d without a client certificate", path, reply.Code)
		}
	}
	// A session token on the customer entrance cannot carry a POS or travel body or reach other contracts.
	reply := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v2/customer/commerce", strings.NewReader(`{"command":{}}`))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("ab", 32))
	customer.ServeHTTP(reply, req)
	if reply.Code != http.StatusNotFound {
		t.Fatalf("an unknown session answered %d", reply.Code)
	}
}

// Each refused pickup scan names its reason in the service's error envelope; only a used QR carries the handover time.
func TestPickupScanRefusalsAreDistinct(t *testing.T) {
	const envelope = `{"requestId":"","requestDigest":"","status":"error","error":`
	for outcome, want := range map[string]struct {
		status int
		body   string
	}{
		"expired":      {400, `"EXPIRED","reason":"expired"}`},
		"already_used": {409, `"REPLAY_CONFLICT","reason":"already_used","recordedAt":1700000000}`},
		"other_store":  {400, `"MISMATCHED_EVIDENCE","reason":"other_store"}`},
		"other_order":  {400, `"MISMATCHED_EVIDENCE","reason":"other_order"}`},
		"not_ready":    {409, `"CHECKOUT_CONFLICT","reason":"not_ready"}`},
	} {
		status, refusal, known := pickupRefusal(outcome, 1700000000)
		raw, err := json.Marshal(refusal)
		if err != nil || !known || status != want.status || string(raw) != envelope+want.body {
			t.Fatalf("%s: %d %s (%v)", outcome, status, raw, err)
		}
	}
	for _, outcome := range []string{"", "ok", "elsewhere"} {
		if _, _, known := pickupRefusal(outcome, 0); known {
			t.Fatalf("%q is a refusal", outcome)
		}
	}
}

// A limit counts an IPv4 client by its address and an IPv6 client by its /64, whatever the port.
func TestClientKeyGroupsAnIPv6Network(t *testing.T) {
	key := func(remote string) string {
		return clientKey(&http.Request{RemoteAddr: remote})
	}
	for remote, want := range map[string]string{
		"198.51.100.7:4000":                   "198.51.100.7",
		"198.51.100.7:65000":                  "198.51.100.7",
		"[::ffff:198.51.100.7]:4000":          "198.51.100.7", // IPv4 through an IPv6 socket is still that IPv4 client
		"[2001:db8:1:2::1]:4000":              "2001:db8:1:2::/64",
		"[2001:db8:1:2:aaaa:bbbb:cccc:d]:443": "2001:db8:1:2::/64",
		"[2001:db8:1:3::1]:4000":              "2001:db8:1:3::/64",
		"[fe80::1%en0]:4000":                  "fe80::/64",
		"[::1]:4000":                          "::/64",
		"198.51.100.7":                        "198.51.100.7", // no port
		"not an address":                      "not an address",
	} {
		if got := key(remote); got != want {
			t.Fatalf("%s is counted as %q, want %q", remote, got, want)
		}
	}
	// Every address of one /64 shares one allowance; the next /64 and an IPv4 client have their own.
	now := time.Unix(1_800_000_000, 0)
	h := HTTP{Customer: &CustomerEntrance{Limiter: NewRateLimiter(60, 2, func() time.Time { return now })}}
	mux := http.NewServeMux()
	h.RegisterCustomer(mux)
	send := func(remote string) int {
		req := httptest.NewRequest("POST", "/v2/customer/challenge", strings.NewReader("{"))
		req.RemoteAddr = remote
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, req)
		return reply.Code
	}
	if send("[2001:db8:1:2::1]:4000") != http.StatusBadRequest || send("[2001:db8:1:2::2]:4001") != http.StatusBadRequest {
		t.Fatal("the allowance of a /64 was refused")
	}
	for _, remote := range []string{"[2001:db8:1:2::3]:4000", "[2001:db8:1:2:ffff:ffff:ffff:ffff]:4000"} {
		if code := send(remote); code != http.StatusTooManyRequests {
			t.Fatalf("%s got another allowance inside a limited /64: %d", remote, code)
		}
	}
	if send("[2001:db8:1:3::1]:4000") != http.StatusBadRequest || send("198.51.100.7:4000") != http.StatusBadRequest {
		t.Fatal("another network shares the limit")
	}
}
