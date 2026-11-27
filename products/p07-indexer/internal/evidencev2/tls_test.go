package evidencev2

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

func TestC7MutualTLS(t *testing.T) {
	now := time.Now().UTC()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	root, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(root)
	if err != nil {
		t.Fatal(err)
	}
	intermediatePub, intermediateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	intermediate := x509.Certificate{SerialNumber: big.NewInt(2), IsCA: true, BasicConstraintsValid: true,
		NotBefore: template.NotBefore, NotAfter: now.Add(45 * time.Second), KeyUsage: x509.KeyUsageCertSign}
	intermediateDER, err := x509.CreateCertificate(rand.Reader, &intermediate, ca, intermediatePub, key)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := x509.ParseCertificate(intermediateDER)
	if err != nil {
		t.Fatal(err)
	}
	leaf := x509.Certificate{SerialNumber: big.NewInt(3), NotBefore: template.NotBefore, NotAfter: template.NotAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	clientPub, clientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &leaf, issuer, clientPub, intermediateKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	fp := sha256.Sum256(der)
	const generation = "987e50a6-a874-487c-8d85-1451edacfe99"
	const event = "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb"
	claims := service.Principal{Tenant: "80d39491-fa9b-4f35-9c2a-f6a258dd45d6", Role: "evidence-reader",
		Subject: "0fb9757b-d626-4fb8-b02e-68a9fc8930f0", ExpiresAt: now.Unix() + 90,
		Grants: []service.Grant{{Domain: generation, Object: event, Operations: []string{"C7/evidence.read"}}}}
	var serviceTime atomic.Int64
	serviceTime.Store(now.Unix())
	srv := httptest.NewUnstartedServer(Handler(map[string]service.Principal{hex.EncodeToString(fp[:]): claims},
		absent{}, func() time.Time { return time.Unix(serviceTime.Load(), 0) }))
	srv.TLS = TLSConfig(roots)
	srv.StartTLS()
	defer srv.Close()

	req := service.Request{Version: 2, Contract: "C7", Operation: "evidence.read", RequestID: "23a6f801-e925-42b9-b4fa-05f5d4f11cee",
		Domain: generation, Object: event, ExpiresAt: now.Unix() + 30, Body: map[string]string{"kind": "unobserved"}}
	body, _ := json.Marshal(req)
	path := srv.URL + "/v2/evidence/read"
	if resp, err := srv.Client().Post(path, "application/json", bytes.NewReader(body)); err == nil {
		resp.Body.Close()
		t.Fatal("network transport accepted missing client certificate")
	}
	tlsSettings := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tlsSettings.Certificates = []tls.Certificate{{Certificate: [][]byte{der, intermediateDER}, PrivateKey: clientKey}}
	auth := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsSettings}}
	defer auth.CloseIdleConnections()
	resp, err := auth.Post(path, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var reply service.Reply
	if err := json.NewDecoder(resp.Body).Decode(&reply); resp.StatusCode != http.StatusOK || err != nil {
		t.Fatalf("mTLS response %d: %v", resp.StatusCode, err)
	}
	bound, err := service.Authorize(req, claims, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.ValidateReply(bound, reply, now); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// The new request and leaf are both unexpired. Only the intermediate
	// became stale, while the client remained on the authenticated socket.
	serviceTime.Store(now.Add(50 * time.Second).Unix())
	req.ExpiresAt = now.Add(80 * time.Second).Unix()
	body, _ = json.Marshal(req)
	followup, err := http.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var reused bool
	followup = followup.WithContext(httptrace.WithClientTrace(followup.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
	}))
	stale, err := auth.Do(followup)
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Body.Close()
	if !reused || stale.StatusCode != http.StatusNotFound {
		t.Fatalf("stale intermediate on existing connection: reused=%t status=%d", reused, stale.StatusCode)
	}

}
