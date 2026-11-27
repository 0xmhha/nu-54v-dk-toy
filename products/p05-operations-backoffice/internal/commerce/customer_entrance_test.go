package commerce

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
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
	for _, operation := range []string{"order.create", "checkout.authorize", "quote.issue", "order.lock", "payment.forward", "order.status"} {
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
	for _, operation := range []string{"order.create", "quote.issue", "order.lock", "catalog.read"} {
		if order.allows(request(operation, branch, sale)) {
			t.Fatalf("order session allowed %s", operation)
		}
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
