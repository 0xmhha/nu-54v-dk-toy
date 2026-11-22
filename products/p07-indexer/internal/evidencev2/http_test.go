package evidencev2

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

type absent struct{}

func (absent) Finalized(context.Context, string, string) (*service.Evidence, error) { return nil, nil }

func TestAuthenticatedReadBoundary(t *testing.T) {
	now := time.Unix(1600, 0)
	rawCert := []byte("test mTLS verified client")
	fingerprint := sha256.Sum256(rawCert)
	const generation = "987e50a6-a874-487c-8d85-1451edacfe99"
	const event = "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb"
	const other = "23a6f801-e925-42b9-b4fa-05f5d4f11cee"
	principal := service.Principal{Tenant: "80d39491-fa9b-4f35-9c2a-f6a258dd45d6", Role: "evidence-reader",
		Subject: "0fb9757b-d626-4fb8-b02e-68a9fc8930f0", ExpiresAt: 1800,
		Grants: []service.Grant{{Domain: generation, Object: event, Operations: []string{"C7/evidence.read"}}}}
	handler := Handler(map[string]service.Principal{hex.EncodeToString(fingerprint[:]): principal}, absent{}, func() time.Time { return now })
	body, _ := json.Marshal(service.Request{Version: 2, Contract: "C7", Operation: "evidence.read", RequestID: other,
		Domain: generation, Object: event, ExpiresAt: 1630, Body: map[string]string{"kind": "unobserved"}})
	request := func(payload string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v2/evidence/read", strings.NewReader(payload))
		if authorized {
			req.TLS = verifiedFixture(rawCert, 1700)
		}
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		return out
	}
	if out := request(string(body), false); out.Code != http.StatusNotFound {
		t.Fatalf("missing client credential: %d", out.Code)
	}
	if out := request(strings.Replace(string(body), event, other, 1), true); out.Code != http.StatusNotFound {
		t.Fatalf("other object leaked: %d", out.Code)
	}
	if out := request(string(body), true); out.Code != http.StatusOK || !strings.Contains(out.Body.String(), "unobserved") {
		t.Fatalf("accepted read failed: %d %s", out.Code, out.Body.String())
	}
}

type delayed struct{ clock *time.Time }

func (d delayed) Finalized(context.Context, string, string) (*service.Evidence, error) {
	*d.clock = d.clock.Add(31 * time.Second)
	return nil, nil
}

func TestRejectsResultAfterReaderOutlivesRequest(t *testing.T) {
	now := time.Unix(1600, 0)
	raw := []byte("synthetic mTLS peer")
	fp := sha256.Sum256(raw)
	const generation = "987e50a6-a874-487c-8d85-1451edacfe99"
	const event = "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb"
	p := service.Principal{Tenant: "80d39491-fa9b-4f35-9c2a-f6a258dd45d6", Role: "evidence-reader",
		Subject: "0fb9757b-d626-4fb8-b02e-68a9fc8930f0", ExpiresAt: 1630,
		Grants: []service.Grant{{Domain: generation, Object: event, Operations: []string{"C7/evidence.read"}}}}
	handler := Handler(map[string]service.Principal{hex.EncodeToString(fp[:]): p}, delayed{&now}, func() time.Time { return now })
	body, _ := json.Marshal(service.Request{Version: 2, Contract: "C7", Operation: "evidence.read",
		RequestID: "23a6f801-e925-42b9-b4fa-05f5d4f11cee", Domain: generation, Object: event, ExpiresAt: 1630,
		Body: map[string]string{"kind": "unobserved"}})
	req := httptest.NewRequest(http.MethodPost, "/v2/evidence/read", strings.NewReader(string(body)))
	req.TLS = verifiedFixture(raw, 1630)
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, req)
	if out.Code == http.StatusOK || strings.Contains(out.Body.String(), "unobserved") {
		t.Fatalf("expired read returned an observation: %d %s", out.Code, out.Body.String())
	}
}

func verifiedFixture(der []byte, intermediateDeadline int64) *tls.ConnectionState {
	leaf := &x509.Certificate{Raw: der, NotBefore: time.Unix(1400, 0), NotAfter: time.Unix(1900, 0)}
	intermediate := &x509.Certificate{Raw: []byte("test issuer"), NotBefore: time.Unix(1400, 0), NotAfter: time.Unix(intermediateDeadline, 0)}
	return &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf, intermediate}}}
}

type observedDuringRead func()

func (f observedDuringRead) Finalized(context.Context, string, string) (*service.Evidence, error) {
	f()
	return nil, nil
}

func TestRechecksAuthorityBeforeReleasingObservation(t *testing.T) {
	const domain = "987e50a6-a874-487c-8d85-1451edacfe99"
	const object = "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb"
	const id = "23a6f801-e925-42b9-b4fa-05f5d4f11cee"
	p := service.Principal{Tenant: "80d39491-fa9b-4f35-9c2a-f6a258dd45d6", Role: "evidence-reader",
		Subject: "0fb9757b-d626-4fb8-b02e-68a9fc8930f0", ExpiresAt: 1800,
		Grants: []service.Grant{{Domain: domain, Object: object, Operations: []string{"C7/evidence.read"}}}}
	body, _ := json.Marshal(service.Request{Version: 2, Contract: "C7", Operation: "evidence.read",
		RequestID: id, Domain: domain, Object: object, ExpiresAt: 1630, Body: map[string]string{"kind": "unobserved"}})
	raw := []byte("test client")
	fp := sha256.Sum256(raw)
	key := hex.EncodeToString(fp[:])
	for _, option := range []struct {
		name      string
		at        int64
		configure func(map[string]service.Principal)
	}{
		{name: "intermediate expires during read", at: 1620},
		{name: "credential revoked during read", at: 1600, configure: func(grants map[string]service.Principal) { delete(grants, key) }},
	} {
		t.Run(option.name, func(t *testing.T) {
			now := time.Unix(1600, 0)
			credentials := map[string]service.Principal{key: p}
			seen := false
			handler := Handler(credentials, observedDuringRead(func() {
				seen = true
				now = time.Unix(option.at, 0)
				if option.configure != nil {
					option.configure(credentials)
				}
			}), func() time.Time { return now })
			req := httptest.NewRequest(http.MethodPost, "/v2/evidence/read", strings.NewReader(string(body)))
			req.TLS = verifiedFixture(raw, 1620)
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, req)
			if !seen || out.Code != http.StatusNotFound || strings.Contains(out.Body.String(), "unobserved") {
				t.Fatalf("expected denied release after read; called=%t, status=%d, response=%s", seen, out.Code, out.Body)
			}
		})
	}
}
