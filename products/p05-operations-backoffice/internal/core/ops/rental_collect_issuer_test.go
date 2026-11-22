package ops

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/ethereum/go-ethereum/common"
)

func TestRentalCollectorClientPinsSeparateMutualTLSIssuer(t *testing.T) {
	const branch = "4a56441c-008a-45ed-b1be-8764c92d2b9a"
	const device = "8d250324-c9b9-4460-bce5-8e33f1e83512"
	key, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	cert, err := x509.CreateCertificate(rand.Reader, template, template, key, private)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert)
	if err != nil {
		t.Fatal(err)
	}
	clientRoots := x509.NewCertPool()
	clientRoots.AddCert(leaf)
	channel := ble.ProtectedRentalChallenge{Nonce: [32]byte{1}, ExpiresAt: uint64(time.Now().Unix()) + 60}
	var revoked, redirect atomic.Bool
	var called atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		if revoked.Load() {
			http.NotFound(w, r)
			return
		}
		if redirect.Load() {
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
			return
		}
		var input struct {
			ChannelNonce string
			ExpiresAt    uint64
		}
		if r.URL.Path != "/v2/private/rental-collector/"+branch+"/"+device+"/5/freeze-challenge" ||
			r.Method != http.MethodPost || r.TLS == nil || len(r.TLS.VerifiedChains) != 1 ||
			r.Header.Get("Cache-Control") != "no-store" || json.NewDecoder(r.Body).Decode(&input) != nil ||
			input.ChannelNonce != hex.EncodeToString(channel.Nonce[:]) || input.ExpiresAt != channel.ExpiresAt {
			t.Error("issuer received a different certificate, route or board challenge")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"binding": strings.Repeat("01", 32), "challenge": strings.Repeat("02", 32),
			"signature": strings.Repeat("03", 65), "expiresAt": channel.ExpiresAt})
	})
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientRoots}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	pin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	identity := tls.Certificate{Certificate: [][]byte{cert}, PrivateKey: private}
	issuer, err := NewRentalCollectorClient(server.URL, roots, identity, pin, branch, device, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer issuer.Close()
	issued, err := issuer.Issue(context.Background(), channel)
	if err != nil || issued.Binding != common.HexToHash(strings.Repeat("01", 32)) || issued.ExpiresAt != channel.ExpiresAt {
		t.Fatalf("valid pinned request failed: %v", err)
	}
	pin[0] ^= 1
	wrong, err := NewRentalCollectorClient(server.URL, roots, identity, pin, branch, device, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	if _, err := wrong.Issue(context.Background(), channel); err == nil || called.Load() != 1 {
		t.Fatal("wrong origin key received a challenge")
	}
	redirect.Store(true)
	if _, err := issuer.Issue(context.Background(), channel); err == nil || called.Load() != 2 {
		t.Fatal("issuer allowed a redirect")
	}
	redirect.Store(false)
	revoked.Store(true)
	if _, err := issuer.Issue(context.Background(), channel); err == nil || called.Load() != 3 {
		t.Fatal("revoked issuer replayed a grant")
	}
}
