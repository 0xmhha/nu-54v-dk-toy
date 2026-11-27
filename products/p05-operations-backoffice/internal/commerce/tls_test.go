package commerce

import (
	"bytes"
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
	"reflect"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// exerciseAuthenticatedRoute calls the production registry through a real TLS server.
func exerciseAuthenticatedRoute(t *testing.T, h HTTP, auth service.Bound, expected quoteReply) {
	t.Helper()
	readAuthenticatedQuote(t, h, auth, &expected)
}

func readAuthenticatedQuote(t *testing.T, h HTTP, auth service.Bound, expected *quoteReply) quoteReply {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Minute),
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	clientCAs := x509.NewCertPool()
	clientCAs.AddCert(cert)
	fingerprint := sha256.Sum256(der)
	h.Credentials = map[string]service.Principal{hex.EncodeToString(fingerprint[:]): {
		Tenant: auth.Tenant, Subject: auth.Subject, Role: auth.Role, ExpiresAt: time.Now().Unix() + 60,
		Grants: []service.Grant{{Domain: auth.Request.Domain, Object: auth.Request.Object, Operations: []string{"C3/quote.issue"}}},
	}}
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs}
	srv.StartTLS()
	defer srv.Close()
	clientTLS := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	clientTLS.Certificates = []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: clientTLS}}
	defer client.CloseIdleConnections()
	post := func(client *http.Client, command service.Request) (*http.Response, error) {
		payload, err := json.Marshal(map[string]any{"command": command})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v2/commerce", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant", other) // must never override the verified certificate authority
		return client.Do(req)
	}
	if resp, err := post(srv.Client(), auth.Request); err == nil {
		resp.Body.Close()
		t.Fatal("request without certificate reached commerce")
	}
	response, err := post(client, auth.Request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var reply result
	if err := json.NewDecoder(response.Body).Decode(&reply); response.StatusCode != http.StatusOK || err != nil ||
		reply.Quote == nil || (expected != nil && !reflect.DeepEqual(reply.Quote, expected)) {
		t.Fatalf("normal response %d, decode %v, quote %+v", response.StatusCode, err, reply.Quote)
	}
	if err := service.ValidateReply(auth, reply.Reply, time.Now()); err != nil {
		t.Fatal(err)
	}
	otherBranch := auth.Request
	otherBranch.Domain = other
	forbidden, err := post(client, otherBranch)
	if err != nil {
		t.Fatal(err)
	}
	defer forbidden.Body.Close()
	if forbidden.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign branch did not get hidden response: %d", forbidden.StatusCode)
	}
	return *reply.Quote
}
