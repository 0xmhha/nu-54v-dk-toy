package ceremony_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/gateway"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
)

var fixtureOIDCPage = template.Must(template.New("fixture").Parse(`<!doctype html><html lang="en">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Registered QA Identity</title>
<h1>Registered QA Identity</h1>
<form method="post" action="/authorize">
<input type="hidden" name="request" value="{{.}}">
<button>Continue with QA account</button>
</form></html>`))

func (l *browserLab) registerOIDC(t *testing.T, login testGate) (gateway.ProviderIdentity, func()) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "qa"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	const providerID, clientID, subject = "qa-identity", "p03-browser-qa", "qa-registered-subject"
	clientSecret := uuid.NewString()
	callback := login.origin + "/p03/oidc/callback/" + providerID
	type approvedRequest struct {
		nonce, challenge string
	}
	var mu sync.Mutex
	requests, codes := make(map[string]url.Values), make(map[string]approvedRequest)
	var issuer *httptest.Server
	mux := http.NewServeMux()
	jsonResponse := func(w http.ResponseWriter, payload any) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, map[string]any{
			"issuer": issuer.URL, "authorization_endpoint": issuer.URL + "/authorize",
			"token_endpoint": issuer.URL + "/token", "jwks_uri": issuer.URL + "/keys",
			"response_types_supported": []string{"code"}, "response_modes_supported": []string{"query"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
			"code_challenge_methods_supported":      []string{"S256"}, "id_token_signing_alg_values_supported": []string{"RS256"},
			"authorization_response_iss_parameter_supported": true,
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "qa", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("client_id") != clientID || query.Get("redirect_uri") != callback || query.Get("scope") != "openid" ||
			query.Get("response_type") != "code" || query.Get("response_mode") != "query" ||
			query.Get("code_challenge_method") != "S256" || len(query.Get("code_challenge")) != 43 ||
			len(query.Get("state")) != 43 || len(query.Get("nonce")) != 43 {
			http.Error(w, "Invalid authorization", 400)
			return
		}
		request := uuid.NewString()
		mu.Lock()
		requests[request] = query
		mu.Unlock()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = fixtureOIDCPage.Execute(w, request)
	})
	mux.HandleFunc("POST /authorize", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		if r.ParseForm() != nil {
			http.Error(w, "Invalid approval", 400)
			return
		}
		request := r.PostForm.Get("request")
		code := uuid.NewString()
		mu.Lock()
		query := requests[request]
		delete(requests, request)
		if query != nil {
			codes[code] = approvedRequest{nonce: query.Get("nonce"), challenge: query.Get("code_challenge")}
		}
		mu.Unlock()
		if query == nil {
			http.Error(w, "Invalid approval", 400)
			return
		}
		values := url.Values{"state": {query.Get("state")}, "iss": {issuer.URL}, "code": {code}}
		http.Redirect(w, r, callback+"?"+values.Encode(), http.StatusSeeOther)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if r.ParseForm() != nil || r.PostForm.Get("client_id") != clientID || r.PostForm.Get("client_secret") != clientSecret ||
			r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("redirect_uri") != callback {
			http.Error(w, "Invalid exchange", 400)
			return
		}
		code := r.PostForm.Get("code")
		mu.Lock()
		approved := codes[code]
		delete(codes, code)
		mu.Unlock()
		digest := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if approved.challenge != base64.RawURLEncoding.EncodeToString(digest[:]) {
			http.Error(w, "Invalid exchange", 400)
			return
		}
		now := time.Now()
		token, err := jwt.Signed(signer).Claims(map[string]any{
			"iss": issuer.URL, "sub": subject, "aud": clientID,
			"nonce": approved.nonce, "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
		}).Serialize()
		if err != nil {
			http.Error(w, "Unable to sign test grant", 500)
			return
		}
		jsonResponse(w, map[string]any{"access_token": "unused-qa-token", "token_type": "Bearer", "id_token": token})
	})
	issuer = httptest.NewUnstartedServer(mux)
	issuer.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{l.certificate(t)}}
	issuer.StartTLS()
	t.Cleanup(issuer.Close)
	roots := x509.NewCertPool()
	roots.AddCert(l.root)
	remote := &http.Client{Timeout: 4 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}}
	secretPath := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secretPath, []byte(clientSecret), 0600); err != nil {
		t.Fatal(err)
	}
	var lookup [32]byte
	rand.Read(lookup[:])
	identity := gateway.ProviderIdentity{ID: providerID, Name: "Registered QA Identity", Issuer: issuer.URL}
	provider := gateway.ProviderConfig{Identity: identity, ClientID: clientID, ClientSecretFile: secretPath, ResponseMode: "query", PKCE: true}
	config := gateway.OIDCConfig{Network: wallet.TestProfile, Providers: []gateway.ProviderConfig{provider},
		Bindings: []gateway.OIDCBinding{{Provider: providerID, SubjectDigest: gateway.OIDCSubjectDigest(lookup[:], provider, subject),
			Subject: login.subject, Credential: login.credential}}}
	realm, err := gateway.NewOIDCRealm(context.Background(), login.origin, login.realm.Key, lookup[:], config, remote)
	clear(lookup[:])
	if err != nil {
		t.Fatal(fmt.Errorf("start registered fixture: %w", err))
	}
	login.replace(realm)
	return identity, remote.CloseIdleConnections
}
