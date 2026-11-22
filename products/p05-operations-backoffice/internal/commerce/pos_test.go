package commerce

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

func TestPOSRejectsUntrustedAssetBeforeLedgerAccess(t *testing.T) {
	cert := &x509.Certificate{Raw: []byte("test POS source"), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Minute)}
	sum := sha256.Sum256(cert.Raw)
	h := HTTP{Credentials: map[string]service.Principal{
		hex.EncodeToString(sum[:]): {Tenant: tenant, Subject: customerA, Role: "pos", ExpiresAt: time.Now().Add(time.Minute).Unix(),
			Grants: []service.Grant{{Domain: branch, Object: branch, Operations: []string{"POS/submit"}}}},
	}, Profiles: map[string]Profile{branch: FixtureProfile()}}
	mux := http.NewServeMux()
	h.Register(mux)
	r := httptest.NewRequest(http.MethodPost, "/v2/commerce", strings.NewReader(`{"pos":{"operation":"submit","branch":"`+branch+`","reference":"S-0142","chainId":8283,"asset":"0x2222222222222222222222222222222222222222","amount":"6500000"}}`))
	r.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "POS_ASSET_UNSUPPORTED") {
		t.Fatalf("wrong authenticated POS asset rejected without publishing intent: %d %s", w.Code, w.Body.String())
	}
}
