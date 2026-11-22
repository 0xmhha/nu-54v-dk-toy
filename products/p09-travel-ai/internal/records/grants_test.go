package records

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPublishedGrantsOnExistingTLSConnection(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{Raw: []byte("test certificate"), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	digest := sha256.Sum256(leaf.Raw)
	key := hex.EncodeToString(digest[:])
	path := filepath.Join(t.TempDir(), "grants.json")
	s := &Service{clock: func() time.Time { return now }}
	s.UseGrantFile(path, false)
	r := httptest.NewRequest("GET", "/v1/private/records", nil)
	r.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	actor := Principal{Subject: "52dbce50-6f60-4e12-9000-56e23c0c9082", Role: "private-user", ExpiresAt: now.Add(time.Minute).Unix()}
	for _, tc := range []struct {
		grant Principal
		ok    bool
	}{
		{actor, true},
		{Principal{Subject: actor.Subject, Role: "restore", ExpiresAt: actor.ExpiresAt}, false},
		{Principal{Subject: actor.Subject, Role: actor.Role, ExpiresAt: now.Unix()}, false},
	} {
		data, err := json.Marshal(map[string]Principal{key: tc.grant})
		if err != nil {
			t.Fatal(err)
		}
		publishGrantFixture(t, path, data)
		if got, ok := s.principal(r); ok != tc.ok || (ok && got != tc.grant) {
			t.Fatalf("principal = %+v, %v", got, ok)
		}
	}
	for _, data := range []string{"{}", "{", "null"} {
		publishGrantFixture(t, path, []byte(data))
		if _, ok := s.principal(r); ok {
			t.Fatalf("incomplete or malformed snapshot retained authority: %s", data)
		}
	}
}

func publishGrantFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path+".new", data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}
}
