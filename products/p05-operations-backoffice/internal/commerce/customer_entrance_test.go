package commerce

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
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
	// The whole allowance of a checkout session, by name: adding to it is a decision, not a side effect.
	if got := append(slices.Clone(checkoutOperations), checkoutReads...); !slices.Equal(got, []string{"order.create", "checkout.claim",
		"checkout.authorize", "quote.issue", "order.lock", "payment.forward", "catalog.read", "catalog.photo", "order.status"}) {
		t.Fatalf("a checkout session is allowed %v", got)
	}
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
	// The health route is counted like a public read, so asking it again and again keeps no connection open for
	// nothing; another address is not held back by that. Each refusal was counted by its kind.
	now = now.Add(time.Second)
	for i, want := range []int{http.StatusOK, http.StatusTooManyRequests, http.StatusTooManyRequests} {
		if code := send(apart, "/healthz", "198.51.100.7"); code != want {
			t.Fatalf("health request %d: %d, want %d", i+1, code, want)
		}
	}
	if code := send(apart, "/healthz", "198.51.100.8"); code != http.StatusOK {
		t.Fatalf("health for another address: %d", code)
	}
	if counts := apartLimits.Limited.Drain(); counts["read"] != 4 || counts["photo"] != 2 || counts["proof"] != 1 || counts["session"] != 0 {
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

// A store page tells the store's number in the order contract and nothing else of the payment profile: no token,
// address, chain or signer. A store that is not paid through the order contract has nothing to tell.
func TestStorePagePaymentFields(t *testing.T) {
	profile := Profile{Chain: 8283, Contract: common.HexToAddress("0x" + strings.Repeat("3", 40)), Asset: common.HexToAddress("0x" + strings.Repeat("2", 40)),
		Beneficiary: common.HexToAddress("0x" + strings.Repeat("4", 40)), Signer: common.HexToAddress("0x" + strings.Repeat("5", 40)),
		MerchantID: [32]byte{0: 0xe1, 31: 0x07}, KeyVersion: 2, BeneficiaryVersion: 3, Escrow: true, Token: wonForOptions}
	got := storePayment(profile)
	if len(got) != 1 || got["merchantId"] != "e1"+strings.Repeat("0", 60)+"07" {
		t.Fatalf("escrow store: %v", got)
	}
	profile.Escrow = false
	if got := storePayment(profile); len(got) != 0 {
		t.Fatalf("a store outside the order contract told something of its payment: %v", got)
	}
	profile.Escrow, profile.MerchantID = true, [32]byte{}
	if got := storePayment(profile); len(got) != 0 {
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

// sessionRequests is an entrance whose session reads are counted instead of reaching a database: tokens that start
// with 0xaa are live sessions of their own customer, every other token is unknown.
func sessionRequests(limits *CustomerEntrance) (send func(remote, token string) (int, string), reads *int) {
	reads = new(int)
	limits.lookup = func(_ context.Context, token []byte) (customerSession, error) {
		*reads++
		if token[0] != 0xaa {
			return customerSession{}, service.Forbidden
		}
		return customerSession{Branch: branch, Actor: hex.EncodeToString(token), Purpose: "checkout", ExpiresAt: time.Now().Add(time.Minute)}, nil
	}
	mux := http.NewServeMux()
	HTTP{Customer: limits}.RegisterCustomer(mux)
	return func(remote, token string) (int, string) {
		// The envelope is not a command, so a live session ends as a bad request before anything else is read.
		req := httptest.NewRequest("POST", "/v2/customer/commerce", strings.NewReader(`{"command":{}}`))
		req.RemoteAddr = remote
		req.Header.Set("Authorization", "Bearer "+token)
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, req)
		return reply.Code, reply.Header().Get("Retry-After")
	}, reads
}

// An address is counted before its session token is looked up: once it is limited, no request of it reads the
// database, whatever tokens it sends.
func TestSessionTokenIsNotLookedUpOnceTheAddressIsLimited(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }
	limits := &CustomerEntrance{AddressLimiter: NewRateLimiter(60, 3, clock)}
	send, reads := sessionRequests(limits)
	unknown := func(i int) string { return strings.Repeat("0", 62) + hex.EncodeToString([]byte{byte(i)}) }
	for i := 0; i < 3; i++ {
		if code, _ := send("198.51.100.7:4000", unknown(i)); code != http.StatusNotFound {
			t.Fatalf("unknown token %d: %d", i, code)
		}
	}
	if *reads != 3 {
		t.Fatalf("%d session reads for 3 allowed requests", *reads)
	}
	for i := 3; i < 40; i++ {
		if code, wait := send("198.51.100.7:"+hex.EncodeToString([]byte{byte(i)}), unknown(i)); code != http.StatusTooManyRequests || wait != "1" {
			t.Fatalf("request %d of a limited address: %d, Retry-After %q", i, code, wait)
		}
	}
	// A live session of the limited address is refused the same way, without a read.
	if code, _ := send("198.51.100.7:4000", "aa"+strings.Repeat("0", 62)); code != http.StatusTooManyRequests {
		t.Fatalf("a live session passed its limited address: %d", code)
	}
	if *reads != 3 {
		t.Fatalf("a limited address reached the session read: %d reads", *reads)
	}
	// Another address is read, and so is the first again once it has a request back.
	if code, _ := send("198.51.100.8:4000", unknown(0)); code != http.StatusNotFound || *reads != 4 {
		t.Fatalf("another address: %d, %d reads", code, *reads)
	}
	now = now.Add(time.Second)
	if code, _ := send("198.51.100.7:4000", "aa"+strings.Repeat("0", 62)); code != http.StatusBadRequest || *reads != 5 {
		t.Fatalf("after the wait: %d, %d reads", code, *reads)
	}
	if counts := limits.Limited.Drain(); counts["address"] != 38 || len(counts) != 1 {
		t.Fatalf("refusals by kind: %v", counts)
	}
	// A deployment that sets only the one limit keeps session requests under it, as before the limits were split.
	shared := &CustomerEntrance{Limiter: NewRateLimiter(60, 1, clock)}
	send, reads = sessionRequests(shared)
	if first, _ := send("198.51.100.9:4000", unknown(0)); first != http.StatusNotFound {
		t.Fatalf("first request: %d", first)
	}
	if second, _ := send("198.51.100.9:4000", unknown(1)); second != http.StatusTooManyRequests || *reads != 1 {
		t.Fatalf("session requests escaped the entrance limit: %d, %d reads", second, *reads)
	}
}

// Each live session has its own allowance and all checkout sessions of one address share that address's ceiling,
// so opening more sessions gives an address no more requests. A request its own session refuses is not counted for
// the address: one session that asks too often uses up its own allowance and nobody else's.
func TestSessionsShareTheCeilingOfTheirAddress(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }
	limits := &CustomerEntrance{AddressLimiter: NewRateLimiter(60, 6, clock), SessionLimiter: NewRateLimiter(60, 2, clock)}
	send, reads := sessionRequests(limits)
	live := func(i int) string { return "aa" + strings.Repeat("0", 60) + hex.EncodeToString([]byte{byte(i)}) }
	const home, other = "198.51.100.7:4000", "198.51.100.8:4000"
	// One session uses its own allowance up; another session of the same address is not held back by that.
	for i, want := range []int{http.StatusBadRequest, http.StatusBadRequest, http.StatusTooManyRequests} {
		if code, _ := send(home, live(1)); code != want {
			t.Fatalf("request %d of one session: %d, want %d", i+1, code, want)
		}
	}
	// The refused request was answered from memory: it read nothing and took nothing from its address, however
	// often it is sent.
	before := *reads
	for i := 0; i < 50; i++ {
		if code, _ := send(home, live(1)); code != http.StatusTooManyRequests {
			t.Fatalf("request %d of a session over its own limit: %d", i+4, code)
		}
	}
	if *reads != before {
		t.Fatalf("requests a session's own limit refused were read: %d, was %d", *reads, before)
	}
	if code, _ := send(home, live(2)); code != http.StatusBadRequest {
		t.Fatalf("a second session shares the first one's limit: %d", code)
	}
	// The same session from another address still has no allowance left: it is the session's, not the address's.
	if code, _ := send(other, live(1)); code != http.StatusTooManyRequests {
		t.Fatalf("a limited session got another allowance from another address: %d", code)
	}
	// Six requests of the address are used: the two the first session was answered, one of the second, and three
	// here. The 51 its own session refused are not among them. Sessions that never asked anything are refused now,
	// before any read: the address is at its ceiling.
	send(home, live(3))
	send(home, live(4))
	if code, _ := send(home, live(5)); code != http.StatusBadRequest {
		t.Fatalf("the sixth request of the address: %d", code)
	}
	before = *reads
	for i := 6; i < 31; i++ {
		if code, _ := send(home, live(i)); code != http.StatusTooManyRequests {
			t.Fatalf("session %d took its address past the ceiling: %d", i, code)
		}
	}
	if *reads != before {
		t.Fatalf("sessions over the ceiling were read: %d, was %d", *reads, before)
	}
	// A session the address's ceiling refused keeps its own allowance: it is served as soon as the address is.
	if code, _ := send(home, live(5)); code != http.StatusTooManyRequests {
		t.Fatalf("a known session passed the ceiling of its address: %d", code)
	}
	now = now.Add(time.Second)
	if code, _ := send(home, live(5)); code != http.StatusBadRequest {
		t.Fatalf("the session lost its own allowance to a refusal of its address: %d", code)
	}
	if code, _ := send(other, live(6)); code != http.StatusBadRequest {
		t.Fatalf("another address is under the first one's ceiling: %d", code)
	}
	if counts := limits.Limited.Drain(); counts["session"] != 52 || counts["address"] != 26 || len(counts) != 2 {
		t.Fatalf("refusals by kind: %v", counts)
	}
}

// Session tokens the server does not know have an allowance of their own for each address: using it up leaves the
// address its proofs, so phones with expired sessions behind one address can still get new ones.
func TestUnknownSessionTokensLeaveTheProofAllowance(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }
	limits := &CustomerEntrance{Limiter: NewRateLimiter(60, 2, clock), AddressLimiter: NewRateLimiter(600, 100, clock),
		UnknownLimiter: NewRateLimiter(6, 2, clock)}
	send, reads := sessionRequests(limits)
	const home = "198.51.100.7:4000"
	stale := strings.Repeat("cd", 32)
	for i, want := range []int{http.StatusNotFound, http.StatusNotFound, http.StatusTooManyRequests, http.StatusTooManyRequests} {
		if code, wait := send(home, stale); code != want || (want == http.StatusTooManyRequests && wait != "10") {
			t.Fatalf("stale token %d: %d (Retry-After %q), want %d", i+1, code, wait, want)
		}
	}
	// A header that is no token at all is counted the same way and reads nothing.
	if code, _ := send(home, "not-a-token"); code != http.StatusTooManyRequests || *reads != 4 {
		t.Fatalf("a malformed token: %d, %d reads", code, *reads)
	}
	// A live session of the same address is still served, and the address can still ask for a challenge.
	if code, _ := send(home, "aa"+strings.Repeat("0", 62)); code != http.StatusBadRequest {
		t.Fatalf("stale tokens of one phone stopped another phone's live session: %d", code)
	}
	mux := http.NewServeMux()
	HTTP{Customer: limits}.RegisterCustomer(mux)
	prove := func() int {
		req := httptest.NewRequest("POST", "/v2/customer/challenge", strings.NewReader("{"))
		req.RemoteAddr = home
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, req)
		return reply.Code
	}
	if first, second, third := prove(), prove(), prove(); first != http.StatusBadRequest || second != http.StatusBadRequest || third != http.StatusTooManyRequests {
		t.Fatalf("stale tokens used up the proofs of their address: %d %d %d", first, second, third)
	}
	if counts := limits.Limited.Drain(); counts["unknown"] != 3 || counts["proof"] != 1 || len(counts) != 2 {
		t.Fatalf("refusals by kind: %v", counts)
	}
}

// proofRequest is a proof request with a signature of key over challenge, as an app sends it.
func proofRequest(t *testing.T, remote string, key *ecdsa.PrivateKey, challenge [32]byte) *http.Request {
	t.Helper()
	body := `{"challenge":"` + hex.EncodeToString(challenge[:]) + `","account":"` + strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()) +
		`","signature":"` + hex.EncodeToString(signAccountProof(t, key, challenge)) + `","generation":"7"}`
	req := httptest.NewRequest("POST", "/v2/customer/proof", strings.NewReader(body))
	req.RemoteAddr = remote
	return req
}

// A session start is counted for the proven account at its address, and only when a proof gave a session. A
// challenge is never a start, so asking for challenges cannot use the starts of an address up; one account cannot
// use up another's; a proof that gave no session gives its start back.
func TestSessionStartsPerAccount(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }
	limits := &CustomerEntrance{StartLimiter: NewRateLimiter(6, 2, clock), clock: clock} // two at once, then one every ten seconds
	mux := http.NewServeMux()
	HTTP{Customer: limits}.RegisterCustomer(mux) // no store behind it: an allowed challenge ends as not found
	sale := "6f1c1a52-4c39-4d1e-9d36-1f0b9b4d2a10"
	const home = "[2001:db8:1:2::1]:4000"
	for i := 0; i < 40; i++ {
		for _, purpose := range []string{"checkout", "order", "pickup"} {
			body := `{"purpose":"` + purpose + `","branchRef":"` + branch + `"`
			if purpose != "checkout" {
				body += `,"saleRef":"` + sale + `"`
			}
			req := httptest.NewRequest("POST", "/v2/customer/challenge", strings.NewReader(body+"}"))
			req.RemoteAddr = home
			reply := httptest.NewRecorder()
			mux.ServeHTTP(reply, req)
			if reply.Code != http.StatusNotFound {
				t.Fatalf("%s challenge %d was counted as a session start: %d", purpose, i+1, reply.Code)
			}
		}
	}
	first, second := mustKey(t), mustKey(t)
	// proveFor counts one proof of key over a challenge of the entrance the way the route does, and settles it as
	// the handler would.
	proveFor := func(purpose, remote string, key *ecdsa.PrivateKey, gaveSession bool) (int, string) {
		t.Helper()
		given, err := limits.keys.mint(now, purpose, branch, now.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		reply := httptest.NewRecorder()
		try, allowed := limits.askedProof(reply, proofRequest(t, remote, key, given), true)
		if !allowed {
			return reply.Code, reply.Header().Get("Retry-After")
		}
		try.reached = true
		try.settle(gaveSession)
		return http.StatusOK, ""
	}
	prove := func(remote string, key *ecdsa.PrivateKey, gaveSession bool) (int, string) {
		t.Helper()
		return proveFor("checkout", remote, key, gaveSession)
	}
	for i := 0; i < 2; i++ {
		if code, _ := prove(home, first, true); code != http.StatusOK {
			t.Fatalf("session start %d of the account: %d", i+1, code)
		}
	}
	// The same /64 is the same address: the account has no start left there, and is told when it has one again.
	if code, wait := prove("[2001:db8:1:2::2]:4000", first, true); code != http.StatusTooManyRequests || wait != "10" {
		t.Fatalf("a third start of the account in the window: %d, Retry-After %q", code, wait)
	}
	// Another account behind the same address is not held back by that, however often the first one asks.
	for i := 0; i < 20; i++ {
		prove(home, first, true)
	}
	for i := 0; i < 2; i++ {
		if code, _ := prove(home, second, true); code != http.StatusOK {
			t.Fatalf("another account shares the first one's starts: %d", code)
		}
	}
	// The same account from another address has its own count there.
	if code, _ := prove("198.51.100.7:4000", first, true); code != http.StatusOK {
		t.Fatalf("the account at another address: %d", code)
	}
	// A proof that gave no session (a pickup QR, a refusal) is not a start: any number of them leaves the starts.
	third := mustKey(t)
	for i := 0; i < 30; i++ {
		if code, _ := prove(home, third, false); code != http.StatusOK {
			t.Fatalf("proof %d without a session was counted as a start: %d", i+1, code)
		}
	}
	// A pickup proof and the question about an account are never starts, even for an account that has none left.
	for _, purpose := range []string{"pickup", "account"} {
		if code, _ := proveFor(purpose, home, first, false); code != http.StatusOK {
			t.Fatalf("a %s proof of an account without a start left: %d", purpose, code)
		}
	}
	now = now.Add(10 * time.Second)
	if again, _ := prove(home, first, true); again != http.StatusOK {
		t.Fatalf("after the window: %d", again)
	}
	if counts := limits.Limited.Drain(); counts["start"] != 21 || len(counts) != 1 {
		t.Fatalf("refusals by kind: %v", counts)
	}
}

// What costs a sender nothing uses up nothing the customers behind the same address need. Bodies that do not read,
// challenges the server did not give and signatures that are not the account's use up the address's failed attempts
// and nothing else; no request of a customer is counted there before it was looked at, so a challenge for any store
// and a proof with the account's own signature pass all the while, whatever the entrance remembers of them.
func TestFailedAttemptsLeaveCustomersTheirAllowance(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }
	limits := &CustomerEntrance{IssueLimiter: NewRateLimiter(6000, 1000, clock), AttemptLimiter: NewRateLimiter(6000, 1000, clock),
		FailLimiter: NewRateLimiter(12, 2, clock), AccountLimiter: NewRateLimiter(12, 2, clock), Limiter: NewRateLimiter(60, 3, clock),
		StartLimiter: NewRateLimiter(6, 2, clock), clock: clock}
	mux := http.NewServeMux()
	HTTP{Customer: limits}.RegisterCustomer(mux) // no store behind it: a request that is let through ends as not found
	const home = "198.51.100.7:4000"
	post := func(path, body string) (int, string) {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.RemoteAddr = home
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, req)
		return reply.Code, reply.Header().Get("Retry-After")
	}
	given := func(purpose string) [32]byte {
		t.Helper()
		value, err := limits.keys.mint(now, purpose, branch, now.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	prove := func(key *ecdsa.PrivateKey, purpose string) (int, string) {
		t.Helper()
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, proofRequest(t, home, key, given(purpose)))
		return reply.Code, reply.Header().Get("Retry-After")
	}
	key := mustKey(t)
	account := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	// A challenge the server did not give, and one it gave with a signature that is not the account's.
	forged := `{"challenge":"` + strings.Repeat("ab", 32) + `","account":"` + account + `","signature":"` + strings.Repeat("11", 65) + `"}`
	own := given("checkout")
	stolen := `{"challenge":"` + hex.EncodeToString(own[:]) + `","account":"` + account + `","signature":"` +
		hex.EncodeToString(signAccountProof(t, mustKey(t), own)) + `"}`
	// Two failed attempts are answered as what they are; from the third on the address is told to wait.
	for i, want := range []int{http.StatusBadRequest, http.StatusBadRequest, http.StatusTooManyRequests} {
		if code, wait := post("/v2/customer/challenge", "{"); code != want || (want == http.StatusTooManyRequests && wait != "5") {
			t.Fatalf("challenge %d that does not read: %d, Retry-After %q", i+1, code, wait)
		}
	}
	for i := 0; i < 100; i++ {
		for _, attempt := range [][2]string{{"/v2/customer/challenge", `{"purpose":"checkout"}`}, {"/v2/customer/proof", "{"},
			{"/v2/customer/proof", forged}, {"/v2/customer/proof", stolen}} {
			if code, _ := post(attempt[0], attempt[1]); code != http.StatusTooManyRequests {
				t.Fatalf("failed attempt %d on %s got through: %d", i, attempt[0], code)
			}
		}
	}
	// A challenge is an app's request for any store, and for no store: the entrance remembers no store, and the
	// 401 requests above took nothing a challenge is counted under. Fifty of them pass at once.
	asks := []string{`{"purpose":"checkout","branchRef":"` + branch + `"}`, `{"purpose":"checkout","branchRef":"6f1c1a52-4c39-4d1e-9d36-1f0b9b4d2a10"}`,
		`{"purpose":"pickup","branchRef":"` + branch + `","saleRef":"6f1c1a52-4c39-4d1e-9d36-1f0b9b4d2a10"}`, `{"purpose":"account"}`}
	for i := 0; i < 50; i++ {
		if code, _ := post("/v2/customer/challenge", asks[i%len(asks)]); code != http.StatusNotFound {
			t.Fatalf("a customer's challenge %d while failed attempts are used up: %d", i+1, code)
		}
	}
	// A proof over a challenge of the entrance, with the account's own signature, is counted for that account
	// and passes while the failed attempts are used up. The account has two proofs at once, then one every five
	// seconds, and is told when it has one again.
	for i, want := range []int{http.StatusNotFound, http.StatusNotFound, http.StatusTooManyRequests} {
		if code, wait := prove(key, "pickup"); code != want || (want == http.StatusTooManyRequests && wait != "5") {
			t.Fatalf("pickup proof %d of one account: %d, Retry-After %q", i+1, code, wait)
		}
	}
	// Another account behind the same address has its own. Proofs that start a checkout session or ask about an
	// account are counted for the address as well: three at once here, then one a second.
	for i, purpose := range []string{"checkout", "account", "checkout"} {
		if code, _ := prove(mustKey(t), purpose); code != http.StatusNotFound {
			t.Fatalf("%s proof %d of a new account: %d", purpose, i+1, code)
		}
	}
	late := mustKey(t)
	if code, wait := prove(late, "checkout"); code != http.StatusTooManyRequests || wait != "1" {
		t.Fatalf("a fourth session start of the address at once: %d, Retry-After %q", code, wait)
	}
	// A proof for an order is not counted for the address: the order's own customer is served while the session
	// starts of the address are used up.
	for i := 0; i < 20; i++ {
		if code, _ := prove(mustKey(t), []string{"order", "pickup"}[i%2]); code != http.StatusNotFound {
			t.Fatalf("order proof %d while the address has no session start left: %d", i+1, code)
		}
	}
	// The account the address refused was not counted for it: it still has both of its own proofs.
	now = now.Add(2 * time.Second)
	for i := 0; i < 2; i++ {
		if code, _ := prove(late, "checkout"); code != http.StatusNotFound {
			t.Fatalf("proof %d of the account once its address had a start again: %d", i+1, code)
		}
	}
	// A challenge that ran out before the board answered is the account's own business too: it is answered as
	// what it is, and is no failed attempt of the address.
	slow := mustKey(t)
	ran := given("pickup")
	now = now.Add(61 * time.Second)
	for i := 0; i < 2; i++ {
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, proofRequest(t, home, slow, ran))
		if reply.Code != http.StatusBadRequest || !strings.Contains(reply.Body.String(), string(service.Expired)) {
			t.Fatalf("a proof over a challenge that ran out: %d %s", reply.Code, reply.Body)
		}
	}
	counts := limits.Limited.Drain()
	if counts["failed"] != 401 || counts["account"] != 1 || counts["proof"] != 1 || len(counts) != 3 {
		t.Fatalf("refusals by kind: %v", counts)
	}
}

// Made-up session tokens use up the allowance of tokens the server has not seen live, and nothing else: a session
// that was lately live is counted under the ceiling of live sessions, so a customer who is paying is still served,
// and a made-up token is still not looked up once its allowance is gone.
func TestMadeUpTokensDoNotStopLiveSessions(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }
	limits := &CustomerEntrance{AddressLimiter: NewRateLimiter(60, 3, clock), LiveLimiter: NewRateLimiter(60, 5, clock)}
	send, reads := sessionRequests(limits)
	const home = "198.51.100.7:4000"
	paying := "aa" + strings.Repeat("0", 62)
	if code, _ := send(home, paying); code != http.StatusBadRequest || *reads != 1 { // found live: remembered
		t.Fatalf("the customer's first request: %d, %d reads", code, *reads)
	}
	for i := 0; i < 300; i++ { // ten made-up tokens a second for half a minute
		token := hex.EncodeToString([]byte{0xcd, byte(i >> 8), byte(i)}) + strings.Repeat("0", 58)
		want := http.StatusNotFound
		if i >= 2 {
			want = http.StatusTooManyRequests // two were left of three; the rest is refused before any lookup
		}
		if code, _ := send(home, token); code != want {
			t.Fatalf("made-up token %d: %d, want %d", i+1, code, want)
		}
	}
	if *reads != 3 {
		t.Fatalf("%d session reads: made-up tokens were looked up past their allowance", *reads)
	}
	for i := 0; i < 5; i++ {
		if code, _ := send(home, paying); code != http.StatusBadRequest {
			t.Fatalf("request %d of the paying customer among made-up tokens: %d", i+1, code)
		}
	}
	// Live sessions still share a ceiling of their own, so more sessions give an address no more requests.
	if code, wait := send(home, paying); code != http.StatusTooManyRequests || wait != "1" {
		t.Fatalf("past the ceiling of live sessions: %d, Retry-After %q", code, wait)
	}
	// A session the server has not seen yet starts under the stricter allowance, which the made-up tokens emptied:
	// it waits until that has a request again, and from its first answer on it is a live one.
	late := "aa" + strings.Repeat("1", 62)
	if code, _ := send(home, late); code != http.StatusTooManyRequests {
		t.Fatalf("a session not seen before, with the allowance used up: %d", code)
	}
	now = now.Add(time.Second)
	for i := 0; i < 2; i++ {
		if code, _ := send(home, late); code != http.StatusBadRequest {
			t.Fatalf("request %d of the session once it was seen live: %d", i+1, code)
		}
	}
	// A session that ended is forgotten at its first refusal.
	limits.sawLive(sha256.Sum256(bytes.Repeat([]byte{0xcd}, 32)), customerSession{Purpose: "checkout", ExpiresAt: time.Now().Add(time.Minute)})
	ended := strings.Repeat("cd", 32)
	now = now.Add(time.Minute)
	if code, _ := send(home, ended); code != http.StatusNotFound || limits.live.has(string(hashOf(bytes.Repeat([]byte{0xcd}, 32))), time.Now()) {
		t.Fatalf("an ended session: %d, still remembered", code)
	}
	if counts := limits.Limited.Drain(); counts["address"] != 299 || counts["live"] != 1 || len(counts) != 2 {
		t.Fatalf("refusals by kind: %v", counts)
	}
}

// A full table of clients never refuses a newcomer: it forgets a client that is back at its full allowance, or
// the one closest to it, and a client that is being held back stays held back.
func TestRateLimiterKeepsABoundedTable(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter := NewRateLimiter(6, 2, func() time.Time { return now }) // two at once, then one every ten seconds
	limiter.Keys = 500
	// One client is held back and keeps asking, while a thousand requests a second arrive from new clients: no
	// newcomer is refused its first request for the table being full, and the client stays held back.
	limiter.Allow("held")
	limiter.Allow("held")
	refused := 0
	for i := 0; i < 5000; i++ {
		now = now.Add(time.Millisecond)
		key := "new-" + strconv.Itoa(i)
		if !limiter.Allow(key) {
			refused++
		}
		limiter.Allow(key)
		limiter.Allow(key) // each newcomer is then held back too, so no bucket in the table is full
		if limiter.Allow("held") {
			t.Fatalf("the client that is held back was let through at newcomer %d", i)
		}
		if len(limiter.clients) > 500 {
			t.Fatalf("the table holds %d clients, past its bound of 500", len(limiter.clients))
		}
	}
	if refused != 0 {
		t.Fatalf("%d of 5000 newcomers were refused by a full table", refused)
	}
	if _, kept := limiter.clients["held"]; !kept {
		t.Fatal("the client that is held back was forgotten")
	}
	// Once clients are back at their full allowance they are the ones forgotten.
	now = now.Add(time.Minute)
	limiter.Allow("drained")
	limiter.Allow("drained")
	for i := 0; i < 300; i++ {
		limiter.Allow("later-" + strconv.Itoa(i))
		limiter.Allow("later-" + strconv.Itoa(i))
	}
	if _, kept := limiter.clients["drained"]; !kept || limiter.Allow("drained") {
		t.Fatalf("a client that is held back was forgotten for newcomers (kept %v)", kept)
	}
	// A request given back counts again, up to the full allowance and no further.
	limiter.Refund("drained")
	if !limiter.Allow("drained") || limiter.Allow("drained") {
		t.Fatal("a request given back was not there to take once")
	}
	limiter.Refund("never-seen")
	for i := 0; i < 5; i++ {
		limiter.Refund("fresh")
	}
	if !limiter.Allow("fresh") || !limiter.Allow("fresh") || limiter.Allow("fresh") {
		t.Fatal("requests given back took a client past its full allowance")
	}
	// The memory of what the entrance recognises is bounded the same way.
	seen := recentSet{keys: 100}
	for i := 0; i < 1000; i++ {
		seen.put("key-"+strconv.Itoa(i), now, now.Add(time.Minute), "note")
		if len(seen.entries) > 100 {
			t.Fatalf("the memory holds %d keys, past its bound of 100", len(seen.entries))
		}
	}
	if note, known := seen.get("key-999", now); !known || note != "note" {
		t.Fatal("the newest key was not kept")
	}
	seen.put("gone", now, now.Add(-time.Second), "")
	if seen.has("gone", now) {
		t.Fatal("a key past its time is still known")
	}
}

// The recovery gate holds a control-database connection for as long as a request is inside it. A request enters it
// only after its rate limit, with its body read and inside the cap on requests served at once; the health route
// never does.
func TestEntranceTakesTheGateLast(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var inside, entered atomic.Int32
	hold := make(chan struct{})
	gate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			entered.Add(1)
			inside.Add(1)
			defer inside.Add(-1)
			if r.Header.Get("X-Hold") != "" {
				<-hold
			}
			next.ServeHTTP(w, r)
		})
	}
	limits := &CustomerEntrance{Limiter: NewRateLimiter(60, 2, func() time.Time { return now }), Gate: gate,
		InFlight: make(chan struct{}, 2), InFlightWait: 20 * time.Millisecond}
	mux := http.NewServeMux()
	HTTP{Customer: limits}.RegisterCustomer(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	get := func(path string, hold bool) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", server.URL+path, nil)
		if hold {
			req.Header.Set("X-Hold", "1")
		}
		reply, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		reply.Body.Close()
		return reply
	}
	menu := "/v2/customer/stores/" + branch + "/menu/0"

	// Health is answered outside the gate, and is counted like a public read; a limited request is refused outside
	// the gate too.
	for i, want := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests, http.StatusTooManyRequests} {
		if reply := get("/healthz", false); reply.StatusCode != want {
			t.Fatalf("health request %d: %d, want %d", i+1, reply.StatusCode, want)
		}
	}
	if entered.Load() != 0 {
		t.Fatalf("the health route entered the gate %d times", entered.Load())
	}
	now = now.Add(time.Minute)
	if first, second := get(menu, false), get(menu, false); first.StatusCode != http.StatusNotFound || second.StatusCode != http.StatusNotFound {
		t.Fatalf("allowed reads: %d %d", first.StatusCode, second.StatusCode)
	}
	if entered.Load() != 2 {
		t.Fatalf("%d of 2 allowed requests entered the gate", entered.Load())
	}
	for i := 0; i < 20; i++ {
		if reply := get(menu, false); reply.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("a limited read: %d", reply.StatusCode)
		}
	}
	if entered.Load() != 2 {
		t.Fatalf("limited requests entered the gate: %d entries", entered.Load())
	}

	// A body that arrives slowly is waited for outside the gate and outside the cap.
	limits.Limiter = nil
	connection, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	body := `{"purpose":"checkout","branchRef":"` + branch + `"}`
	fmt.Fprintf(connection, "POST /v2/customer/challenge HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body[:10])
	time.Sleep(100 * time.Millisecond)
	if entered.Load() != 2 || len(limits.InFlight) != 0 {
		t.Fatalf("a request with half a body holds the gate (%d entries) or a place (%d)", entered.Load(), len(limits.InFlight))
	}
	fmt.Fprint(connection, body[10:])
	answer, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil || answer.StatusCode != http.StatusNotFound || entered.Load() != 3 { // no store behind this entrance
		t.Fatalf("the completed request: %v %v, %d entries", answer, err, entered.Load())
	}
	// A body longer than any route reads is still refused by the route, as before.
	long, err := http.Post(server.URL+"/v2/customer/challenge", "application/json", strings.NewReader(`{"purpose":"`+strings.Repeat("x", 20_000)+`"}`))
	if err != nil || long.StatusCode != http.StatusBadRequest {
		t.Fatalf("a long body: %v %v", long, err)
	}

	// Two requests fill the cap; the third waits its short time and is told to come back, without entering the gate.
	held := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() { held <- get(menu, true).StatusCode }()
	}
	for deadline := time.Now().Add(2 * time.Second); inside.Load() != 2; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the held requests did not enter the gate")
		}
	}
	before := entered.Load()
	refused := get(menu, false)
	if refused.StatusCode != http.StatusServiceUnavailable || refused.Header.Get("Retry-After") != "1" || entered.Load() != before {
		t.Fatalf("over the cap: %d, Retry-After %q, %d new entries", refused.StatusCode, refused.Header.Get("Retry-After"), entered.Load()-before)
	}
	if reply := get("/healthz", false); reply.StatusCode != http.StatusOK { // health needs no place
		t.Fatalf("health at the cap: %d", reply.StatusCode)
	}
	close(hold)
	if first, second := <-held, <-held; first != http.StatusNotFound || second != http.StatusNotFound {
		t.Fatalf("the held requests: %d %d", first, second)
	}
	if reply := get(menu, false); reply.StatusCode != http.StatusNotFound || len(limits.InFlight) != 0 {
		t.Fatalf("after the cap cleared: %d, %d places taken", reply.StatusCode, len(limits.InFlight))
	}
	if counts := limits.Limited.Drain(); counts["busy"] != 1 || counts["read"] != 22 || len(counts) != 2 {
		t.Fatalf("refusals by kind: %v", counts)
	}
}

// The operator's contact is one visible line: a value with a hidden or control character is not shown at all.
func TestOperatorContactIsOneVisibleLine(t *testing.T) {
	for value, want := range map[string]string{
		" help@operator.example ": "help@operator.example", "02-555-0100 (평일 10~18시)": "02-555-0100 (평일 10~18시)",
		"": "", "a\nb": "", "a\u0085b": "", "a\u200bb": "", "a\u202eb": "", "a\u2028b": "", "a\u2029b": "", "a\ufeffb": "", "a\u00adb": "",
		"a\xffb": "", strings.Repeat("가", 121): "",
		// What does not show although it is no format or control character, and blanks other than the space.
		"\u3164": "", "help\u3164desk": "", "\u2800\u2800": "", "a\u00a0b": "", "a\ufe0fb": "", "a\u3000b": "", "a\u115fb": "", "a\uffa0b": "",
		"a\u034fb": "", "a\u2009b": "", "a\U000E0100b": "", "a\u180bb": "", "   ": "",
		"Café №1 ☕ 営業中": "Café №1 ☕ 営業中",
	} {
		if got := (&CustomerEntrance{OperatorContact: value}).contact(); got != want {
			t.Fatalf("contact %q is shown as %q, want %q", value, got, want)
		}
	}
	if got := (*CustomerEntrance)(nil).contact(); got != "" {
		t.Fatalf("no entrance has a contact: %q", got)
	}
}

// A pickup proof that gives no QR says whether the order is still being made or was already handed over, and when.
func TestPickupProofRefusalsAreDistinct(t *testing.T) {
	const envelope = `{"requestId":"","requestDigest":"","status":"error","error":"CHECKOUT_CONFLICT",`
	handed := time.Unix(1_764_207_060, 900_000_000)
	for outcome, want := range map[string]string{"not_ready": `"reason":"not_ready"}`, "collected": `"reason":"collected","recordedAt":1764207060}`} {
		status, refusal, known := proofRefusal(outcome, &handed)
		if outcome == "not_ready" {
			status, refusal, known = proofRefusal(outcome, nil)
		}
		raw, err := json.Marshal(refusal)
		if err != nil || !known || status != http.StatusConflict || string(raw) != envelope+want {
			t.Fatalf("%s: %d %s (%v)", outcome, status, raw, err)
		}
	}
	for _, outcome := range []string{"", "ok", "mismatch", "already_used"} {
		if _, _, known := proofRefusal(outcome, &handed); known {
			t.Fatalf("%q is a pickup proof refusal", outcome)
		}
	}
}

// The cancel route takes a checkout session and nothing else, and its requests are counted like a session's others.
func TestCustomerCancelNeedsACheckoutSession(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	reads := 0
	limits := &CustomerEntrance{AddressLimiter: NewRateLimiter(60, 4, func() time.Time { return now })}
	limits.lookup = func(_ context.Context, token []byte) (customerSession, error) {
		reads++
		switch token[0] {
		case 0xaa:
			return customerSession{Branch: branch, Actor: customerA, Purpose: "checkout", ExpiresAt: time.Now().Add(time.Minute)}, nil
		case 0xbb:
			return customerSession{Branch: branch, Actor: customerA, Purpose: "order", Sale: branch, ExpiresAt: time.Now().Add(time.Minute)}, nil
		}
		return customerSession{}, service.Forbidden
	}
	mux := http.NewServeMux()
	HTTP{Customer: limits}.RegisterCustomer(mux)
	cancel := func(token, body string) int {
		req := httptest.NewRequest("POST", "/v2/customer/checkout/cancel", strings.NewReader(body))
		req.RemoteAddr = "198.51.100.7:4000"
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, req)
		return reply.Code
	}
	order := `{"saleRef":"6f1c1a52-4c39-4d1e-9d36-1f0b9b4d2a10"}`
	if code := cancel("", order); code != http.StatusNotFound || reads != 0 {
		t.Fatalf("no session: %d, %d session reads", code, reads)
	}
	if code := cancel(strings.Repeat("cd", 32), order); code != http.StatusNotFound {
		t.Fatalf("an unknown session: %d", code)
	}
	if code := cancel("bb"+strings.Repeat("0", 62), order); code != http.StatusNotFound { // a paid order's session cancels nothing
		t.Fatalf("an order session: %d", code)
	}
	// A checkout session without a store behind it ends as not found too; it was read, so it was let through.
	if code := cancel("aa"+strings.Repeat("0", 62), order); code != http.StatusNotFound || reads != 3 {
		t.Fatalf("a checkout session: %d, %d session reads", code, reads)
	}
	// The fifth request of the address is refused before its session is read.
	if code := cancel("aa"+strings.Repeat("0", 62), order); code != http.StatusTooManyRequests || reads != 3 {
		t.Fatalf("over the ceiling of the address: %d, %d session reads", code, reads)
	}
	if reply := httptest.NewRecorder(); true {
		mux.ServeHTTP(reply, httptest.NewRequest("GET", "/v2/customer/checkout/cancel", nil))
		if reply.Code != http.StatusMethodNotAllowed && reply.Code != http.StatusNotFound {
			t.Fatalf("GET on the cancel route: %d", reply.Code)
		}
	}
}
