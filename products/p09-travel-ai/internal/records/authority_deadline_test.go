package records

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetainedTLSChainExpiry(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{Raw: []byte("prior verified leaf"), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Minute)}
	ca := &x509.Certificate{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Minute)}
	id := sha256.Sum256(leaf.Raw)
	svc := Service{grants: map[string]Principal{hex.EncodeToString(id[:]): {Role: "private-user", ExpiresAt: now.Add(time.Hour).Unix()}}}
	req := httptest.NewRequest("GET", "/v1/private/consents", nil)
	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf, ca}}}
	if _, ok := svc.principal(req); !ok {
		t.Fatal("currently valid verified chain was denied")
	}
	ca.NotAfter = now.Add(-time.Second)
	if _, ok := svc.principal(req); ok {
		t.Fatal("retained verified chain granted access after intermediate expiry")
	}
}

func TestTLS13KeepAliveCannotRetainExpiredAuthority(t *testing.T) {
	for _, bound := range []string{"peer", "grant"} {
		t.Run(bound, func(t *testing.T) {
			certificate, leaf := processorCertificate(t, false)
			id := sha256.Sum256(leaf.Raw)
			grant := leaf.NotAfter.Add(time.Minute)
			deadline := leaf.NotAfter
			if bound == "grant" {
				grant = leaf.NotAfter.Add(-time.Minute)
				deadline = grant
			}
			var instant atomic.Int64
			instant.Store(deadline.Add(-time.Nanosecond).UnixNano())
			svc := Service{
				grants: map[string]Principal{hex.EncodeToString(id[:]): {Role: "private-user", ExpiresAt: grant.Unix()}},
				clock:  func() time.Time { return time.Unix(0, instant.Load()) },
			}
			mux := http.NewServeMux()
			svc.Register(mux)
			roots := x509.NewCertPool()
			roots.AddCert(leaf)
			var first atomic.Bool
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Prime the production authority check without needing a DB for
				// the positive request. The subsequent request takes the normal route.
				if first.CompareAndSwap(false, true) {
					if _, ok := svc.principal(r); !ok {
						http.NotFound(w, r)
						return
					}
					w.WriteHeader(http.StatusNoContent)
					return
				}
				mux.ServeHTTP(w, r)
			}))
			server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
			server.StartTLS()
			defer server.Close()
			client := server.Client()
			client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{certificate}
			get := func(want int, wantReused bool) {
				t.Helper()
				var reused bool
				req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/private/consents", nil)
				if err != nil {
					t.Fatal(err)
				}
				req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
					GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
				}))
				res, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				_, err = io.Copy(io.Discard, res.Body)
				res.Body.Close()
				if err != nil || res.StatusCode != want || reused != wantReused {
					t.Fatalf("received status=%d reused=%v read=%v", res.StatusCode, reused, err)
				}
			}
			get(http.StatusNoContent, false)
			instant.Store(deadline.UnixNano())
			get(http.StatusNotFound, true)
		})
	}
}
