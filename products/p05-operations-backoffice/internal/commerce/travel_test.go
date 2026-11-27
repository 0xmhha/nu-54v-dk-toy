package commerce

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

func TestTravelRejectsWrongModeAndMixedCommerceEnvelope(t *testing.T) {
	leaf := &x509.Certificate{Raw: []byte("scope-test"), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Minute)}
	digest := sha256.Sum256(leaf.Raw)
	mux := http.NewServeMux()
	HTTP{Credentials: map[string]service.Principal{
		hex.EncodeToString(digest[:]): {Tenant: "11111111-1111-4111-8111-111111111111",
			Subject: "22222222-2222-4222-8222-222222222222", Role: "terminal", ExpiresAt: time.Now().Unix() + 60},
	}}.Register(mux)
	for _, body := range []string{
		`{"travel":{"operation":"options","branch":"11111111-1111-4111-8111-111111111111"}}`,
		`{"travel":{"operation":"options","branch":"11111111-1111-4111-8111-111111111111"},"command":null}`,
	} {
		request := httptest.NewRequest("POST", "/v2/commerce", bytes.NewBufferString(body))
		request.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, request)
		if reply.Code != http.StatusNotFound && reply.Code != http.StatusForbidden {
			t.Fatalf("untrusted travel scope: %d", reply.Code)
		}
	}
}

func TestTravelLegRequestsBindExistingBookingOnly(t *testing.T) {
	req := TravelRequest{Operation: "start-deposit", Branch: "d6d10219-1c27-492c-9bdf-ec14d7d092bd", Booking: "5eb86d05-907a-4602-ab91-f85dd71d845c"}
	if !req.valid() {
		t.Fatal("existing booking cannot open its deposit checkout")
	}
	req.Operation = "start-balance"
	if !req.valid() {
		t.Fatal("existing booking cannot open its balance checkout")
	}
	req.Operation = "cancel-preview"
	if !req.valid() {
		t.Fatal("the preview must accept an existing booking scope")
	}
	withKey := req
	withKey.Key = req.Booking
	if withKey.valid() {
		t.Fatal("read-only preview must not accept a write request key")
	}
	req.Deposit = "10"
	if req.valid() {
		t.Fatal("caller supplied an amount for an already accepted booking")
	}
}
