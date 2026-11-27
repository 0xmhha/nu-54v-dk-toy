package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
)

func TestRentalEvidencePinnedMTLSAndExactGeneration(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageCertSign}
	root, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(root)
	if err != nil {
		t.Fatal(err)
	}
	clientPub, clientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: big.NewInt(2),
		NotBefore: template.NotBefore, NotAfter: template.NotAfter,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}, ca, clientPub, key)
	if err != nil {
		t.Fatal(err)
	}
	clientCA := x509.NewCertPool()
	clientCA.AddCert(ca)
	digest := func(c string) string { return "0x" + strings.Repeat(c, 64) }
	expected := rentalv1.Binding{
		Stream: "098cdd27-c6d0-46c4-914e-27810d349f28", StreamGeneration: "c9c0dc8c-d40c-4f13-8c38-b6e1d1130d79",
		Deployment: digest("a"), FreezeReceipt: digest("b"), JournalRoot: digest("c"),
	}
	pending, zero, height := 1, 0, int64(91)
	base := rentalv1.Assessment{Status: "pending", Binding: expected, PendingCount: &pending,
		PendingPayments: &pending, PendingLimits: &zero, AnchorHeight: &height, AnchorHash: digest("d"),
		AnchorTime: now.Add(-2 * time.Second).Unix(), Digest: "sha256:" + strings.Repeat("e", 64),
		ValidUntil: now.Add(time.Minute).Unix()}
	var scenario atomic.Int32
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != rentalv1.ReconcilePath || r.Method != "POST" || r.TLS == nil || len(r.TLS.VerifiedChains) == 0 ||
			r.Header.Get("Cache-Control") != "no-store" {
			http.Error(w, "wrong scope", http.StatusForbidden)
			return
		}
		result := base
		switch scenario.Load() {
		case 1:
			result.StreamGeneration = "c9c0dc8c-d40c-4f13-8c38-b6e1d1130d71"
		case 2:
			result.FreezeReceipt = digest("f")
		case 3:
			result.Status = "verified"
		case 4:
			result.PendingPayments, result.PendingLimits = &zero, &pending
		case 5:
			result = rentalv1.Assessment{Status: "incomplete"}
		case 6:
			result.Status = "incomplete"
		case 7:
			http.Redirect(w, r, "/ordinary-c7", http.StatusTemporaryRedirect)
			return
		case 8:
			result.ValidUntil = now.Unix()
		case 9:
			mask := uint8(1)
			result.PendingPaymentEntries = &mask
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientCAs: clientCA, ClientAuth: tls.RequireAndVerifyClientCert}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	pin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	identity := tls.Certificate{Certificate: [][]byte{leaf}, PrivateKey: clientKey}
	reader, err := NewRentalEvidenceReader(server.URL, roots, identity, pin)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	query := rentalv1.Request{Projection: make([]byte, 277), Receipt: make([]byte, 253)}
	read := func(reader *RentalEvidenceReader) (rentalv1.Assessment, error) {
		return reader.Read(context.Background(), query, expected, 3, now.Add(-time.Minute), now.Add(2*time.Minute), now)
	}
	for _, mode := range []int32{0, 4, 5} {
		scenario.Store(mode)
		got, err := read(reader)
		if err != nil {
			t.Fatalf("authorized assessment %d: %v", mode, err)
		}
		if mode == 5 && (got.Status != "incomplete" || got.PendingCount != nil) {
			t.Fatalf("unresolved assessment turned into count: %+v", got)
		}
		if (mode == 0 && (got.PendingPayments == nil || *got.PendingPayments != 1)) ||
			(mode == 4 && (got.PendingLimits == nil || *got.PendingLimits != 1)) {
			t.Fatalf("distinct signed pending counts did not survive: %+v", got)
		}
	}
	for _, mode := range []int32{1, 2, 3, 6, 7, 8} {
		scenario.Store(mode)
		before := requests.Load()
		if _, err := read(reader); err == nil || requests.Load() != before+1 {
			t.Fatalf("invalid proof accepted or forwarded: %d", mode)
		}
	}
	scenario.Store(9)
	if _, err := read(reader); err == nil {
		t.Fatal("unrequested entry enumeration entered the count-only consumer")
	}
	query.IncludePendingPaymentEntries = true
	query.Projection = append(query.Projection, make([]byte, 126)...)
	for _, test := range []struct {
		kind, state byte
		accepted    bool
	}{{1, 1, true}, {1, 2, false}, {2, 1, false}} {
		query.Projection[277], query.Projection[278] = test.kind, test.state
		if _, err := read(reader); (err == nil) != test.accepted {
			t.Fatalf("incorrect pending entry authorization for kind %v state %v: %v", test.kind, test.state, err)
		}
	}
	pin[0] ^= 1
	wrongPin, err := NewRentalEvidenceReader(server.URL, roots, identity, pin)
	if err != nil {
		t.Fatal(err)
	}
	defer wrongPin.Close()
	before := requests.Load()
	if _, err := read(wrongPin); err == nil || requests.Load() != before {
		t.Fatal("evidence reached a different server public key")
	}
	if _, err := NewRentalEvidenceReader(server.URL, roots, tls.Certificate{}, pin); err == nil {
		t.Fatal("provisionless customer could construct a reader")
	}
}
