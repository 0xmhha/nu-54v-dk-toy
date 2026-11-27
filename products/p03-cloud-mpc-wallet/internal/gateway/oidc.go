package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/privatefile"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const oidcCallbackPath = "/p03/oidc/callback"
const oidcCookie = "__Host-nu54-p03-oidc"
const appleIssuer = "https://appleid.apple.com"
const googleIssuer = "https://accounts.google.com"

var providerID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
var providerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 .&_-]{0,47}$`)
var subjectKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

type ProviderIdentity struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Issuer string `json:"issuer"`
}

type onboardingSelection struct {
	Network  string           `json:"network"`
	Provider ProviderIdentity `json:"provider"`
}

type ProviderConfig struct {
	Identity         ProviderIdentity
	ClientID         string
	ClientSecretFile string
	ResponseMode     string
	PKCE             bool
}

type OIDCBinding struct {
	Provider, SubjectDigest, Subject, Credential string
}

type OIDCConfig struct {
	Network        string
	Providers      []ProviderConfig
	Bindings       []OIDCBinding
	LegacyCallback bool
}

type oidcProvider struct {
	config         ProviderConfig
	oauth          oauth2.Config
	verify         *oidc.IDTokenVerifier
	callbackPath   string
	requiresIssuer bool
}

type oidcRealm struct {
	network   string
	providers map[string]oidcProvider
	bindings  map[string]map[string]Account
	lookupKey []byte
	client    *http.Client
}

type browserFlow struct {
	state, nonce, cookie, verifier string
}

func httpsEndpoint(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}

func readClientSecret(path string) (string, error) {
	raw, err := privatefile.Read(path, 1, 4096)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	secret := strings.TrimSpace(string(raw))
	if secret == "" || strings.ContainsAny(secret, "\r\n\x00") {
		return "", errors.New("invalid client secret")
	}
	return secret, nil
}

// OIDCSubjectDigest indexes an explicitly registered upstream account without
// storing the issuer's subject identifier in the identity realm.
func OIDCSubjectDigest(lookup []byte, provider ProviderConfig, subject string) string {
	mac := hmac.New(sha256.New, lookup)
	// The deployment's lookup key and issuer+client scope separate registrations.
	mac.Write([]byte("nu54-p03/oidc-subject/v1\x00" + provider.Identity.Issuer + "\x00" + provider.ClientID + "\x00" + subject))
	return hex.EncodeToString(mac.Sum(nil))
}

func NewOIDCRealm(ctx context.Context, origin string, key ed25519.PrivateKey, lookup []byte, config OIDCConfig, client *http.Client) (*Realm, error) {
	if !httpsEndpoint(origin) || strings.Contains(strings.TrimPrefix(origin, "https://"), "/") ||
		len(key) != ed25519.PrivateKeySize || len(lookup) != 32 ||
		(config.Network != wallet.TestProfile && config.Network != wallet.MarketTestProfile) ||
		len(config.Providers) < 1 || len(config.Providers) > 8 || (config.LegacyCallback && len(config.Providers) != 1) || len(config.Bindings) == 0 || client == nil {
		return nil, errors.New("invalid registered identity realm")
	}
	remote := *client
	remote.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx = oidc.ClientContext(ctx, &remote)
	settings := &oidcRealm{network: config.Network, providers: make(map[string]oidcProvider),
		bindings: make(map[string]map[string]Account), lookupKey: append([]byte(nil), lookup...), client: &remote}
	names := make(map[string]bool)
	issuers := make(map[string]bool)
	for _, provider := range config.Providers {
		id := provider.Identity
		if !providerID.MatchString(id.ID) || !providerName.MatchString(id.Name) ||
			names[strings.ToLower(id.Name)] || issuers[id.Issuer] || settings.providers[id.ID].verify != nil || len(id.Issuer) > 256 ||
			!httpsEndpoint(id.Issuer) || id.Issuer == origin || provider.ClientID == "" ||
			strings.ContainsAny(provider.ClientID, "\r\n\x00") ||
			(provider.ResponseMode != "query" && provider.ResponseMode != "form_post") ||
			(id.Issuer == appleIssuer && (provider.ResponseMode != "form_post" || provider.PKCE)) ||
			(id.Issuer == googleIssuer && provider.ResponseMode != "query") ||
			(id.Issuer != appleIssuer && !provider.PKCE) ||
			(strings.EqualFold(id.Name, "Google") && id.Issuer != googleIssuer) ||
			(strings.EqualFold(id.Name, "Apple") && id.Issuer != appleIssuer) {
			return nil, errors.New("invalid registered identity provider")
		}
		if _, err := readClientSecret(provider.ClientSecretFile); err != nil {
			return nil, err
		}
		upstream, err := oidc.NewProvider(ctx, id.Issuer)
		if err != nil {
			return nil, errors.New("registered OIDC discovery failed")
		}
		var discovery struct {
			Keys           string   `json:"jwks_uri"`
			ResponseTypes  []string `json:"response_types_supported"`
			ResponseModes  []string `json:"response_modes_supported"`
			ClientAuth     []string `json:"token_endpoint_auth_methods_supported"`
			PKCEMethods    []string `json:"code_challenge_methods_supported"`
			ResponseIssuer bool     `json:"authorization_response_iss_parameter_supported"`
		}
		endpoint := upstream.Endpoint()
		if upstream.Claims(&discovery) != nil || !httpsEndpoint(discovery.Keys) ||
			!httpsEndpoint(endpoint.AuthURL) || !httpsEndpoint(endpoint.TokenURL) ||
			!slices.Contains(discovery.ResponseTypes, "code") ||
			!slices.Contains(discovery.ResponseModes, provider.ResponseMode) ||
			!slices.Contains(discovery.ClientAuth, "client_secret_post") ||
			(provider.PKCE && !slices.Contains(discovery.PKCEMethods, "S256")) ||
			(id.Issuer == googleIssuer && !discovery.ResponseIssuer) {
			return nil, errors.New("invalid registered OIDC endpoints")
		}
		endpoint.AuthStyle = oauth2.AuthStyleInParams
		path := oidcCallbackPath + "/" + id.ID
		if config.LegacyCallback {
			path = oidcCallbackPath
		}
		login := oauth2.Config{
			ClientID: provider.ClientID, Endpoint: endpoint, RedirectURL: origin + path,
			Scopes: []string{oidc.ScopeOpenID},
		}
		settings.providers[id.ID] = oidcProvider{config: provider, oauth: login, callbackPath: path, requiresIssuer: discovery.ResponseIssuer,
			verify: upstream.VerifierContext(ctx, &oidc.Config{ClientID: provider.ClientID, SupportedSigningAlgs: []string{oidc.RS256}})}
		settings.bindings[id.ID] = make(map[string]Account)
		names[strings.ToLower(id.Name)] = true
		issuers[id.Issuer] = true
	}
	credentials := make(map[string]bool)
	for _, binding := range config.Bindings {
		subjects := settings.bindings[binding.Provider]
		if subjects == nil || !subjectKey.MatchString(binding.SubjectDigest) || !canonicalRequest(binding.Subject) ||
			!canonicalRequest(binding.Credential) || credentials[binding.Credential] || subjects[binding.SubjectDigest].Subject != "" {
			return nil, errors.New("invalid registered OIDC subject binding")
		}
		subjects[binding.SubjectDigest] = Account{Subject: binding.Subject, Credential: binding.Credential}
		credentials[binding.Credential] = true
	}
	return &Realm{Role: "identity", Origin: origin, Key: key, oidc: settings}, nil
}

func (r *Realm) registered(selection *onboardingSelection) bool {
	return selection != nil && selection.Network == r.oidc.network &&
		r.oidc.providers[selection.Provider.ID].config.Identity == selection.Provider &&
		selection.Provider.ID != ""
}

func (r *Realm) startOIDCBrowser(w http.ResponseWriter, req *http.Request) {
	device := req.URL.Query().Get("code")
	r.mu.Lock()
	f := r.flows[device]
	if f == nil || !time.Now().Before(f.expires) || f.ready || f.denied || f.submitting || f.browser.state != "" {
		r.mu.Unlock()
		http.Error(w, "challenge unavailable", http.StatusForbidden)
		return
	}
	f.browser = browserFlow{state: token(), nonce: token(), cookie: token()}
	provider := r.oidc.providers[f.request.Onboarding.Provider.ID]
	options := []oauth2.AuthCodeOption{oidc.Nonce(f.browser.nonce), oauth2.SetAuthURLParam("response_mode", provider.config.ResponseMode)}
	if provider.config.PKCE {
		f.browser.verifier = token()
		options = append(options, oauth2.S256ChallengeOption(f.browser.verifier))
	}
	expires := f.expires
	browser := f.browser
	r.mu.Unlock()
	sameSite := http.SameSiteLaxMode
	if provider.config.ResponseMode == "form_post" {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Path: "/", Value: browser.cookie, Secure: true, HttpOnly: true, SameSite: sameSite, Expires: expires})
	http.Redirect(w, req, provider.oauth.AuthCodeURL(browser.state, options...), http.StatusSeeOther)
}

func (r *Realm) oidcCallback(w http.ResponseWriter, req *http.Request, provider oidcProvider) {
	// ServeMux matches escaped path segments as well. Accept only the registered URI.
	if req.URL.EscapedPath() != provider.callbackPath {
		http.NotFound(w, req)
		return
	}
	req.Body = http.MaxBytesReader(w, req.Body, 8192)
	if req.ParseForm() != nil {
		http.Error(w, "confirmation rejected", http.StatusForbidden)
		return
	}
	params := req.Form
	if req.Method == "POST" {
		if req.URL.RawQuery != "" {
			http.Error(w, "confirmation rejected", http.StatusForbidden)
			return
		}
		params = req.PostForm
	}
	issuers := params["iss"]
	if (provider.requiresIssuer && (len(issuers) != 1 || issuers[0] != provider.config.Identity.Issuer)) ||
		(!provider.requiresIssuer && len(issuers) != 0) {
		http.Error(w, "confirmation rejected", http.StatusForbidden)
		return
	}
	state := params.Get("state")
	cookie, err := req.Cookie(oidcCookie)
	if err != nil || !verifierFormat.MatchString(state) || len(params["state"]) != 1 ||
		!verifierFormat.MatchString(cookie.Value) {
		http.Error(w, "confirmation rejected", http.StatusForbidden)
		return
	}
	r.mu.Lock()
	var f *flow
	for _, candidate := range r.flows {
		if hmac.Equal([]byte(candidate.browser.state), []byte(state)) {
			f = candidate
			break
		}
	}
	if f == nil || f.ready || f.denied || f.submitting || !time.Now().Before(f.expires) ||
		!hmac.Equal([]byte(f.browser.cookie), []byte(cookie.Value)) ||
		f.request.Onboarding.Provider.ID != provider.config.Identity.ID {
		r.mu.Unlock()
		http.Error(w, "confirmation rejected", http.StatusForbidden)
		return
	}
	browser := f.browser
	f.browser = browserFlow{}
	f.submitting = true
	r.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteNoneMode})

	var account Account
	code := params.Get("code")
	if len(params["code"]) == 1 && len(code) > 0 && len(code) <= 2048 && len(params["error"]) == 0 {
		account = r.oidc.exchange(req.Context(), provider, code, browser)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	f.submitting = false
	now := time.Now()
	if account.Subject == "" || !now.Before(f.expires) {
		f.denied = true
		http.Error(w, "confirmation rejected", http.StatusForbidden)
		return
	}
	r.finishIdentity(f, account, now)
	confirmed(w)
}

func (r *Realm) finishIdentity(f *flow, account Account, now time.Time) {
	f.ready = true
	f.grant = map[string]string{"state": "ready", "token": r.jwt(account, "wallet", now, f.expires),
		"enrollmentToken": r.jwt(account, "wallet:enroll", now, f.expires)}
}

func (o *oidcRealm) exchange(ctx context.Context, p oidcProvider, code string, browser browserFlow) Account {
	ctx = oidc.ClientContext(ctx, o.client)
	conf := p.oauth
	var err error
	conf.ClientSecret, err = readClientSecret(p.config.ClientSecretFile)
	if err != nil {
		return Account{}
	}
	var options []oauth2.AuthCodeOption
	if p.config.PKCE {
		options = append(options, oauth2.VerifierOption(browser.verifier))
	}
	grant, err := conf.Exchange(ctx, code, options...)
	if err != nil {
		return Account{}
	}
	raw, ok := grant.Extra("id_token").(string)
	if !ok || len(raw) > 16384 {
		return Account{}
	}
	id, err := p.verify.Verify(ctx, raw)
	now := time.Now()
	if err != nil || id.Issuer != p.config.Identity.Issuer || len(id.Audience) != 1 ||
		id.Audience[0] != p.config.ClientID || !hmac.Equal([]byte(id.Nonce), []byte(browser.nonce)) || len(id.Subject) < 1 ||
		len(id.Subject) > 255 || id.IssuedAt.Before(now.Add(-10*time.Minute)) || id.IssuedAt.After(now.Add(time.Minute)) {
		return Account{}
	}
	var authorized struct {
		Party string `json:"azp"`
	}
	if id.Claims(&authorized) != nil || authorized.Party != "" && authorized.Party != p.config.ClientID ||
		id.AccessTokenHash != "" && id.VerifyAccessToken(grant.AccessToken) != nil {
		return Account{}
	}
	return o.bindings[p.config.Identity.ID][OIDCSubjectDigest(o.lookupKey, p.config, id.Subject)]
}
