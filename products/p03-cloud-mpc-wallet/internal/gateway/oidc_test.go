package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	jose "github.com/go-jose/go-jose/v4"
	jwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
)

func TestRegisteredOIDCLogin(t *testing.T) {
	for _, scenario := range []struct{ mode, failure string }{
		{"query", ""}, {"google", ""}, {"google", "issuer not advertised"}, {"legacy", ""}, {"dual", ""}, {"form_post", ""}, {"apple", ""},
		{"query", "missing iss"}, {"query", "duplicate iss"}, {"query", "wrong iss"}, {"query", "shared route"}, {"dual", "wrong provider"}, {"apple", "wrong method"}, {"apple", "unexpected iss"}, {"apple", "form query"}, {"query", "unknown subject"},
		{"query", "nonce"}, {"query", "audience"}, {"query", "extra audience"}, {"form_post", "azp"},
		{"query", "signature"}, {"query", "state"}, {"form_post", "browser"},
	} {
		t.Run(scenario.mode+"/"+scenario.failure, func(t *testing.T) {
			testRegisteredOIDCLogin(t, scenario.mode, scenario.failure)
		})
	}
}

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func testRegisteredOIDCLogin(t *testing.T, mode, failure string) {
	apple, google := mode == "apple", mode == "google"
	legacy, dual := mode == "legacy", mode == "dual"
	if legacy || dual || google {
		mode = "query"
	}
	if apple {
		mode = "form_post"
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	subject, credential := uuid.NewString(), uuid.NewString()
	var target, configuredIssuer string
	var upstreamMu sync.Mutex
	authorizations := make(map[string]url.Values)
	keyID := "current"
	var issuer *httptest.Server
	var exchanges atomic.Int32
	issuer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		upstreamMu.Lock()
		defer upstreamMu.Unlock()
		path := req.URL.Path
		upstreamIssuer := configuredIssuer
		if strings.HasPrefix(path, "/alternate/") {
			upstreamIssuer = issuer.URL + "/alternate"
			path = strings.TrimPrefix(path, "/alternate")
		}
		switch path {
		case "/.well-known/openid-configuration":
			meta := map[string]any{"issuer": upstreamIssuer, "authorization_endpoint": upstreamIssuer + "/authorize",
				"token_endpoint": upstreamIssuer + "/token", "jwks_uri": upstreamIssuer + "/keys",
				"id_token_signing_alg_values_supported": []string{"RS256"}, "response_types_supported": []string{"code"},
				"response_modes_supported": []string{mode}, "token_endpoint_auth_methods_supported": []string{"client_secret_post"}}
			if apple || google {
				meta["authorization_endpoint"] = issuer.URL + "/authorize"
				meta["token_endpoint"] = issuer.URL + "/token"
				meta["jwks_uri"] = issuer.URL + "/keys"
			}
			if !apple {
				meta["code_challenge_methods_supported"] = []string{"S256"}
				meta["authorization_response_iss_parameter_supported"] = failure != "issuer not advertised"
			}
			send(w, meta)
		case "/authorize":
			params := req.URL.Query()
			code := token()
			authorizations[code] = params
			callback := url.Values{"code": {code}, "state": {params.Get("state")}}
			if !apple {
				callback.Set("iss", upstreamIssuer)
			}
			if mode == "query" {
				http.Redirect(w, req, params.Get("redirect_uri")+"?"+callback.Encode(), http.StatusSeeOther)
			} else {
				// This fixture serializes the form fields for the simulated browser.
				send(w, callback)
			}
		case "/keys":
			send(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: keyID, Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			exchanges.Add(1)
			if err := req.ParseForm(); err != nil {
				t.Error(err)
			}
			code := req.Form.Get("code")
			auth, known := authorizations[code]
			delete(authorizations, code)
			verifier := req.Form.Get("code_verifier")
			hash := sha256.Sum256([]byte(verifier))
			if !known || req.Form.Get("client_id") != "p03-client" || req.Form.Get("client_secret") != "local-client-secret" ||
				req.Form.Get("redirect_uri") != auth.Get("redirect_uri") ||
				(!apple && encoding.EncodeToString(hash[:]) != auth.Get("code_challenge")) || (apple && verifier != "") {
				t.Error("invalid token exchange")
				http.Error(w, "rejected", 400)
				return
			}
			signingKey := key
			if failure == "signature" {
				signingKey = wrong
			}
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: signingKey, KeyID: keyID}}, nil)
			if err != nil {
				t.Error(err)
				return
			}
			upstream, returnedNonce, azp := "opaque-upstream-user", auth.Get("nonce"), "p03-client"
			var aud any = "p03-client"
			switch failure {
			case "unknown subject":
				upstream = "unknown"
			case "nonce":
				returnedNonce = "unknown"
			case "audience":
				aud = "unregistered-client"
			case "extra audience":
				aud = []string{"p03-client", "unregistered-client"}
			case "azp":
				azp = "unregistered-client"
			}
			idToken, err := jwt.Signed(signer).Claims(map[string]any{"iss": upstreamIssuer, "sub": upstream, "aud": aud, "azp": azp,
				"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(), "nonce": returnedNonce, "email": "ignored@example.test"}).Serialize()
			if err != nil {
				t.Error(err)
				return
			}
			send(w, map[string]any{"access_token": "unused", "token_type": "Bearer", "id_token": idToken})
		default:
			t.Errorf("unexpected upstream path: %s", req.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer issuer.Close()
	configuredIssuer = issuer.URL
	providerName := "Registered Test"
	upstreamClient := *issuer.Client()
	if apple || google {
		configuredIssuer, providerName = googleIssuer, "Google"
		if apple {
			configuredIssuer, providerName = appleIssuer, "Apple"
		}
		configuredURL, err := url.Parse(configuredIssuer)
		if err != nil {
			t.Fatal(err)
		}
		upstreamClient.Transport = testRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == configuredURL.Host {
				target, err := url.Parse(issuer.URL + req.URL.RequestURI())
				if err != nil {
					return nil, err
				}
				req = req.Clone(req.Context())
				req.URL = target
			}
			return issuer.Client().Transport.RoundTrip(req)
		})
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// The upstream TLS client trusts only this synthetic provider.
	var app http.Handler
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.ServeHTTP(w, r) }))
	defer server.Close()
	callbackBase := "/p03/oidc/callback"
	callbackPath := callbackBase + "/test"
	if legacy {
		callbackPath = callbackBase
	}
	target = server.URL + callbackPath
	secret := filepath.Join(t.TempDir(), "client-secret")
	if err := os.WriteFile(secret, []byte("local-client-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	lookup := make([]byte, 32)
	rand.Read(lookup)
	provider := ProviderConfig{Identity: ProviderIdentity{ID: "test", Name: providerName, Issuer: configuredIssuer}, ClientID: "p03-client",
		ClientSecretFile: secret, ResponseMode: mode, PKCE: !apple}
	config := OIDCConfig{Network: wallet.TestProfile, Providers: []ProviderConfig{provider}, LegacyCallback: legacy,
		Bindings: []OIDCBinding{{Provider: "test", SubjectDigest: OIDCSubjectDigest(lookup, provider, "opaque-upstream-user"), Subject: subject, Credential: credential}}}
	alternate := provider
	alternate.Identity = ProviderIdentity{ID: "alternate", Name: "Alternate Test", Issuer: issuer.URL + "/alternate"}
	if dual {
		config.Providers = append(config.Providers, alternate)
		config.Bindings = append(config.Bindings, OIDCBinding{Provider: "alternate",
			SubjectDigest: OIDCSubjectDigest(lookup, alternate, "opaque-upstream-user"), Subject: subject, Credential: uuid.NewString()})
		legacyMulti := config
		legacyMulti.LegacyCallback = true
		if _, err := NewOIDCRealm(context.Background(), server.URL, private, lookup, legacyMulti, &upstreamClient); err == nil {
			t.Fatal("legacy common callback allowed with multiple providers")
		}
		duplicate := config
		duplicate.Providers = append([]ProviderConfig(nil), config.Providers...)
		duplicate.Providers[1].Identity.Issuer = provider.Identity.Issuer
		duplicate.Providers[1].ClientID = "other-client"
		if _, err := NewOIDCRealm(context.Background(), server.URL, private, lookup, duplicate, &upstreamClient); err == nil {
			t.Fatal("duplicate issuer allowed under another provider ID")
		}
	}
	realm, err := NewOIDCRealm(context.Background(), server.URL, private, lookup, config, &upstreamClient)
	if failure == "issuer not advertised" {
		if err == nil {
			t.Fatal("Google discovery missing required issuer capability was accepted")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	app = realm.Handler()
	jar, _ := cookiejar.New(nil)
	browser := *server.Client()
	browser.Jar = jar
	browser.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	device := server.Client()
	request := func(client *http.Client, method, path, kind, body string) (int, []byte, http.Header) {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", kind)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw, resp.Header
	}
	verifier := token()
	digest := sha256.Sum256([]byte(verifier))
	launch := func(selected *onboardingSelection) (int, []byte) {
		body, _ := json.Marshal(map[string]any{"request": challenge{Action: "login", Audience: wallet.Audience, Onboarding: selected},
			"codeChallengeMethod": "S256", "codeChallenge": encoding.EncodeToString(digest[:])})
		status, data, _ := request(device, "POST", "/p03/device/start", "application/json", string(body))
		return status, data
	}
	if status, _ := launch(nil); status != 400 {
		t.Fatalf("unscoped login: %d", status)
	}
	unregistered := provider.Identity
	unregistered.Name = "Other provider"
	if status, _ := launch(&onboardingSelection{Network: config.Network, Provider: unregistered}); status != 400 {
		t.Fatalf("wrong provider: %d", status)
	}
	selected := onboardingSelection{Network: config.Network, Provider: provider.Identity}
	exerciseLogin := func() {
		status, raw := launch(&selected)
		var flow struct {
			DeviceCode, VerificationURL string
			Onboarding                  onboardingSelection
		}
		if status != 200 || json.Unmarshal(raw, &flow) != nil || flow.Onboarding != selected {
			t.Fatalf("login start: %d %s", status, raw)
		}
		poll := func(verifier string) (int, map[string]string) {
			body, _ := json.Marshal(map[string]string{"deviceCode": flow.DeviceCode, "codeVerifier": verifier})
			status, data, _ := request(device, "POST", "/p03/device/poll", "application/json", string(body))
			var reply map[string]string
			_ = json.Unmarshal(data, &reply)
			return status, reply
		}
		if status, initial := poll(verifier); status != 200 || initial["state"] != "pending" {
			t.Fatalf("initial poll: %d %v", status, initial)
		}
		status, _, header := request(&browser, "GET", strings.TrimPrefix(flow.VerificationURL, server.URL), "", "")
		auth, err := url.Parse(header.Get("Location"))
		if status != http.StatusSeeOther || err != nil || auth.Host != strings.TrimPrefix(issuer.URL, "https://") {
			t.Fatalf("authorization redirect: %d %v", status, auth)
		}
		query := auth.Query()
		pkce := query.Get("code_challenge")
		if len(query.Get("nonce")) != 43 || query.Get("response_type") != "code" ||
			query.Get("response_mode") != mode || query.Get("redirect_uri") != target || query.Get("client_id") != "p03-client" ||
			query.Get("scope") != "openid" || (!apple && (len(pkce) != 43 || query.Get("code_challenge_method") != "S256")) ||
			(apple && (query.Has("code_challenge") || query.Has("code_challenge_method"))) {
			t.Fatalf("invalid authorization request: %s", auth.RawQuery)
		}
		cookieHeader := header.Get("Set-Cookie")
		wantSameSite := "SameSite=Lax"
		if mode == "form_post" {
			wantSameSite = "SameSite=None"
		}
		if !strings.Contains(cookieHeader, wantSameSite) || !strings.Contains(cookieHeader, "Secure") || !strings.Contains(cookieHeader, "HttpOnly") {
			t.Fatal("invalid browser cookie policy")
		}
		upstreamBrowser := *issuer.Client()
		upstreamBrowser.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		upstreamResponse, err := upstreamBrowser.Get(auth.String())
		if err != nil {
			t.Fatal(err)
		}
		defer upstreamResponse.Body.Close()
		var callbackValues url.Values
		if mode == "query" {
			redirect, err := upstreamResponse.Location()
			if err != nil || redirect.Scheme+"://"+redirect.Host+redirect.Path != target {
				t.Fatal("invalid upstream callback")
			}
			callbackValues = redirect.Query()
		} else if err := json.NewDecoder(upstreamResponse.Body).Decode(&callbackValues); err != nil {
			t.Fatal(err)
		}
		if failure == "state" {
			callbackValues.Set("state", token())
		}
		switch failure {
		case "missing iss":
			callbackValues.Del("iss")
		case "duplicate iss":
			callbackValues.Add("iss", configuredIssuer)
		case "wrong iss":
			callbackValues.Set("iss", "https://wrong.example")
		case "wrong provider":
			callbackValues.Set("iss", alternate.Identity.Issuer)
		case "unexpected iss":
			callbackValues.Set("iss", configuredIssuer)
		}
		if failure == "browser" {
			browser.Jar = nil
		}
		callback := func() int {
			path, method, content := callbackPath, "POST", callbackValues.Encode()
			switch failure {
			case "wrong provider":
				path = callbackBase + "/alternate"
			case "shared route":
				path = callbackBase
			case "form query":
				path += "?iss=" + url.QueryEscape(configuredIssuer)
			}
			if mode == "query" || failure == "wrong method" {
				path += "?" + content
				method, content = "GET", ""
			}
			status, _, _ := request(&browser, method, path, "application/x-www-form-urlencoded", content)
			return status
		}
		baseline := exchanges.Load()
		status = callback()
		if failure != "" {
			untouched := map[string]int{"missing iss": 403, "duplicate iss": 403, "wrong iss": 403, "shared route": 404, "wrong provider": 403, "wrong method": 405, "unexpected iss": 403, "form query": 403}
			if expected, ok := untouched[failure]; ok {
				if status != expected || exchanges.Load() != baseline {
					t.Fatalf("invalid callback reached exchange: status=%d calls=%d", status, exchanges.Load()-baseline)
				}
			} else if status != 403 {
				t.Fatalf("invalid callback accepted: %d", status)
			}
			if _, reply := poll(verifier); reply["state"] == "ready" {
				t.Fatal("credential issued after rejected callback")
			}
			return
		}
		if status != 200 || callback() != 403 {
			t.Fatal("callback failed or was reusable")
		}
		if status, _ := poll(token()); status != 403 {
			t.Fatal("wrong device verifier accepted")
		}
		status, reply := poll(verifier)
		verify := wallet.IdentityVerifier{Issuer: server.URL, Key: public}
		actor, err := verify.Verify(reply["token"], "wallet", time.Now())
		if status != 200 || err != nil || actor.Subject != subject || actor.Credential != credential ||
			reply["enrollmentToken"] == "" {
			t.Fatalf("invalid P03 grant: %d %v", status, err)
		}
	}
	exerciseLogin()
	if dual && failure == "" {
		selected.Provider = alternate.Identity
		callbackPath = callbackBase + "/" + alternate.Identity.ID
		target = server.URL + callbackPath
		credential = config.Bindings[1].Credential
		exerciseLogin()
	}
	if mode == "query" && failure == "" {
		upstreamMu.Lock()
		key, keyID = wrong, "rotated"
		upstreamMu.Unlock()
		exerciseLogin() // the gateway's cached JWKS must refresh on the new signing key.
	}
}

func TestClientSecretFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("qa-secret\n"), 0400); err != nil {
		t.Fatal(err)
	}
	if got, err := readClientSecret(path); err != nil || got != "qa-secret" {
		t.Fatalf("failed to read owner-only secret: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := readClientSecret(path); err == nil || got != "" || strings.Contains(err.Error(), "qa-secret") {
		t.Fatalf("public secret file accepted or disclosed: %v", err)
	}
}
