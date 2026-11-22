package market

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNamedOperatorAuthorizationRechecksRevocationAndExpiry(t *testing.T) {
	now := time.Unix(2000, 0)
	leaf := &x509.Certificate{Raw: []byte("verified fixture certificate"), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(3 * time.Minute)}
	digest := sha256.Sum256(leaf.Raw)
	deployment := Profile{Genesis: common.Hash{1}, BlockHash: common.Hash{2}, Venue: Contract{Address: common.Address{3}}}
	grant := OperatorGrant{Subject: "16eed27d-a142-46d1-9a03-5d440cc228e2", Role: "market-operator",
		Genesis: deployment.Genesis, Deployment: deployment.BlockHash, Venue: deployment.Venue.Address, ExpiresAt: now.Add(time.Minute).Unix()}
	current := grant
	reads := 0
	revoke := false
	handler := operatorMTLS(deployment, func(key string) (OperatorGrant, bool) {
		return current, key == hex.EncodeToString(digest[:]) && current.Subject != ""
	}, func() time.Time { return now }, func(context.Context) operatorView {
		reads++
		if revoke {
			current = OperatorGrant{}
		}
		return operatorView{State: "private-fixture-data"}
	})
	check := func(path string, verified bool, want int, marker string) {
		t.Helper()
		req := httptest.NewRequest("GET", "https://127.0.0.1:8286"+path, nil)
		req.Header.Set("X-P08-Console", "observation")
		if verified {
			req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("got %d, want %d", w.Code, want)
		}
		body := w.Body.String()
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("operator response may be cached")
		}
		if marker == "" && body != "" || !strings.Contains(body, marker) ||
			(path != "/observation" || want != 200) && strings.Contains(body, "private-fixture-data") {
			t.Fatalf("unexpected response body for %s with status %d", path, want)
		}
	}
	check("/", false, 404, "404 page not found")
	check("/", true, 200, "Checking market operator access")
	check("/?state=denied", true, 404, "404 page not found")
	check("/access", true, 204, "")
	current.Role = "learner"
	check("/access", true, 403, "")
	check("/observation", true, 403, "")
	current = grant
	current.Deployment[0]++
	check("/observation", true, 403, "")
	current = grant
	current.ExpiresAt = now.Unix()
	check("/observation", true, 403, "")
	if reads != 0 {
		t.Fatal("authorization failure reached data reader")
	}
	current = grant
	check("/observation", true, 200, "private-fixture-data")
	revoke = true
	check("/observation", true, 403, "")

	if reads != 2 {
		t.Fatal("in-flight revocation test never read evidence")
	}
}

func TestLearningAPIHasNoOperatorRoutes(t *testing.T) {
	handler := (Handler{}).Routes()
	for _, path := range []string{"/", "/operator.js", "/access", "/observation"} {
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("X-P08-Console", "observation")
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, request)
		if result.Code != http.StatusNotFound || result.Body.String() != "404 page not found\n" {
			t.Fatalf("learning API disclosed an operator route at %s", path)
		}
	}
}

func checkMTLSOperatorFixture(t *testing.T, ctx context.Context, chain *Chain, db *pgxpool.Pool, effect common.Hash, state string) {
	t.Helper()
	now := time.Now()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	spec := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "P08 fixture operator"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(3 * time.Minute), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	raw, err := x509.CreateCertificate(rand.Reader, spec, spec, public, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	fingerprint := sha256.Sum256(raw)
	id := hex.EncodeToString(fingerprint[:])
	profile := chain.Profile
	grant := OperatorGrant{Subject: "16eed27d-a142-46d1-9a03-5d440cc228e2", Role: "market-operator",
		Genesis: profile.Genesis, Deployment: profile.BlockHash, Venue: profile.Venue.Address, ExpiresAt: now.Add(2 * time.Minute).Unix()}
	file := service.ClientGrantFile[OperatorGrant](filepath.Join(t.TempDir(), "grants.json"))
	publish := func(data map[string]OperatorGrant) {
		t.Helper()
		body, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		next := string(file) + ".next"
		if err := os.WriteFile(next, body, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(next, string(file)); err != nil {
			t.Fatal(err)
		}
	}
	publish(map[string]OperatorGrant{id: grant})
	host := httptest.NewUnstartedServer(OperatorMTLS(chain, db, file.Lookup))
	host.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	host.StartTLS()
	defer host.Close()
	base := host.Client().Transport.(*http.Transport)
	denied := host.Client()
	if response, err := denied.Get(host.URL); err == nil {
		response.Body.Close()
		t.Fatal("unauthenticated peer completed mTLS")
	}
	transport := base.Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{{Certificate: [][]byte{raw}, PrivateKey: key, Leaf: leaf}}
	client := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()
	load := func(code int) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, "GET", host.URL+"/observation", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-P08-Console", "observation")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		hasEffect := strings.Contains(string(body), effect.Hex())
		if err != nil || response.StatusCode != code || hasEffect != (code == 200) ||
			(code == 200 && !strings.Contains(string(body), state)) {
			t.Fatalf("mTLS operator response %d: %s %v", response.StatusCode, body, err)
		}
	}
	load(200)
	if browser := os.Getenv("P08_TEST_OPERATOR_MTLS_BROWSER"); browser != "" {
		dir := t.TempDir()
		certPath, keyPath := filepath.Join(dir, "operator.crt"), filepath.Join(dir, "operator.key")
		private, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600); err != nil {
			t.Fatal(err)
		}
		serverCertPath := filepath.Join(dir, "server.crt")
		if err := os.WriteFile(serverCertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: host.Certificate().Raw}), 0600); err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, browser, host.URL, effect.Hex(), certPath, keyPath, state)
		command.Env = append(os.Environ(), "NODE_EXTRA_CA_CERTS="+serverCertPath)
		if state == "paused" {
			command.Env = append(command.Env, "P08_OPERATOR_FIXTURE_GRANTS="+string(file))
		}
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("mTLS operator browser: %s %v", out, err)
		}
		t.Logf("mTLS operator browser: %s", out)
	}
	publish(map[string]OperatorGrant{})
	load(403)
	wrongRole := grant
	wrongRole.Role = "market-viewer"
	publish(map[string]OperatorGrant{id: wrongRole})
	load(403)
	expired := grant
	expired.ExpiresAt = now.Unix()
	publish(map[string]OperatorGrant{id: expired})
	load(403)
	grant.Deployment[0] ^= 1
	publish(map[string]OperatorGrant{id: grant})
	load(403)
}
