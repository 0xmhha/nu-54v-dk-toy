// Package gateway implements the P03 identity and independent consent realms.
package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var encoding = base64.RawURLEncoding
var verifierFormat = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

type Account struct {
	Subject, Credential, Username, PasswordHash, SeedPath string
	Key                                                   ed25519.PrivateKey `json:"-"`
}

type Realm struct {
	MarketPolicy                                              *marketlab.Policy
	Accounts                                                  []Account
	Role, Origin, Subject, Credential, Username, PasswordHash string
	Key                                                       ed25519.PrivateKey
	SignRefusalSink                                           func(context.Context, wallet.SignedSignRefusal) error     `json:"-"`
	RecoveryRefusalSink                                       func(context.Context, wallet.SignedRecoveryRefusal) error `json:"-"`
	oidc                                                      *oidcRealm
	mu                                                        sync.Mutex
	flows                                                     map[string]*flow
}

type closeAction struct {
	Wallet  string
	Epoch   uint64
	Request string `json:",omitempty"`
}

func canonicalRequest(ref string) bool {
	id, err := uuid.Parse(ref)
	return err == nil && id != uuid.Nil && id.String() == ref
}

type challenge struct {
	Onboarding        *onboardingSelection `json:"onboarding,omitempty"`
	Owner             string               `json:"owner,omitempty"`
	Action            string               `json:"action"`
	Audience          string               `json:"audience"`
	Session           *ceremony.Session    `json:"session"`
	Transfer          *wallet.Transfer     `json:"transfer"`
	Close             *closeAction         `json:"close"`
	ConfirmAccessEnds bool                 `json:"confirmAccessEnds"`
}

type flow struct {
	browser            browserFlow
	market             *marketlab.Terms
	request            challenge
	fingerprint        [32]byte
	challenge          string
	expires            time.Time
	ready              bool
	denied             bool
	submitting         bool
	signRefusalStarted bool
	resolved           bool
	refusal            *wallet.SignedRecoveryRefusal
	attempts           int
	grant              map[string]string
}

func (r *Realm) Handler() http.Handler {
	r.flows = make(map[string]*flow)
	router := http.NewServeMux()
	router.HandleFunc("POST /p03/device/start", r.start)
	router.HandleFunc("POST /p03/device/poll", r.poll)
	router.HandleFunc("GET /p03/device/verify", r.show)
	router.HandleFunc("POST /p03/device/verify", r.confirm)
	if r.oidc != nil {
		for _, provider := range r.oidc.providers {
			method := "GET"
			if provider.config.ResponseMode == "form_post" {
				method = "POST"
			}
			router.HandleFunc(method+" "+provider.callbackPath, func(w http.ResponseWriter, req *http.Request) {
				if req.Method != method {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				r.oidcCallback(w, req, provider)
			})
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; frame-ancestors 'none'; style-src 'sha256-"+consentStyleHash+"'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		router.ServeHTTP(w, req)
	})
}

func decode(w http.ResponseWriter, req *http.Request, out any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, req.Body, 8192))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err == nil && d.Decode(new(any)) == io.EOF {
		return true
	}
	http.Error(w, "invalid request", 400)
	return false
}

func send(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func token() string { var raw [32]byte; rand.Read(raw[:]); return encoding.EncodeToString(raw[:]) }

func (r *Realm) start(w http.ResponseWriter, req *http.Request) {
	var in struct {
		Request struct {
			challenge
			Fingerprint [32]byte `json:"fingerprint"`
		} `json:"request"`
		CodeChallenge string `json:"codeChallenge"`
		Method        string `json:"codeChallengeMethod"`
	}
	if !decode(w, req, &in) {
		return
	}
	now := time.Now()
	action := in.Request.challenge
	fingerprint := in.Request.Fingerprint
	valid := false
	var marketTerms *marketlab.Terms
	expires := now.Add(5 * time.Minute)
	switch r.Role {
	case "identity", "operator":
		valid = (r.Role == "identity" && action.Action == "login" || r.Role == "operator" && action.Action == "operator") && action.Audience == wallet.Audience && action.Session == nil && action.Transfer == nil && action.Close == nil
	case "approval", "recovery":
		if action.Action == "close" && r.Role == "approval" && action.Close != nil && action.ConfirmAccessEnds {
			id, err := uuid.Parse(action.Close.Wallet)
			valid = err == nil && id != uuid.Nil && id.String() == action.Close.Wallet && action.Close.Epoch > 0 &&
				(action.Close.Request == "" || canonicalRequest(action.Close.Request)) &&
				sha256.Sum256(wallet.AppendClose(action.Close.Wallet, action.Close.Epoch, action.Close.Request)) == fingerprint
		} else if action.Session != nil && string(action.Session.Kind) == action.Action {
			s := action.Session
			actual, err := s.Fingerprint(now)
			valid = err == nil && actual == fingerprint && (r.Role == "approval" && (s.Kind == ceremony.DKG || s.Kind == ceremony.Sign) ||
				r.Role == "recovery" && s.Kind == ceremony.Refresh)
			if s.Kind == ceremony.Sign {
				if action.Transfer == nil {
					valid = false
				} else if r.MarketPolicy == nil {
					valid = valid && wallet.ReviewTransfer(*action.Transfer, s.Digest, s.Intent)
				} else {
					terms, intent, err := wallet.ReviewMarketTransfer(r.MarketPolicy, *action.Transfer, now)
					valid = valid && err == nil && terms.Digest == s.Digest && intent == s.Intent && s.Expires <= int64(terms.Deadline)
					marketTerms = &terms
				}
			}
			expires = minTime(expires, time.Unix(s.Expires, 0))
		}
	}
	if len(r.Accounts) > 0 && (r.Role == "approval" || r.Role == "recovery") {
		subject, err := uuid.Parse(action.Owner)
		valid = valid && err == nil && subject != uuid.Nil && subject.String() == action.Owner
	}
	if r.oidc != nil {
		valid = valid && r.registered(action.Onboarding)
	} else {
		valid = valid && action.Onboarding == nil
	}
	if !valid || in.Method != "S256" || !verifierFormat.MatchString(in.CodeChallenge) || expires.Sub(now) < 15*time.Second {
		http.Error(w, "challenge rejected", 400)
		return
	}
	code := token()
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, f := range r.flows {
		if !now.Before(f.expires) {
			delete(r.flows, key)
		}
	}
	if len(r.flows) >= 128 {
		http.Error(w, "challenge unavailable", 429)
		return
	}
	r.flows[code] = &flow{request: action, market: marketTerms, fingerprint: fingerprint, challenge: in.CodeChallenge, expires: expires}
	response := map[string]any{"deviceCode": code, "verificationURL": r.Origin + "/p03/device/verify?code=" + code,
		"expiresIn": int(expires.Sub(now).Seconds())}
	if action.Onboarding != nil {
		response["onboarding"] = action.Onboarding
	}
	send(w, response)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (r *Realm) poll(w http.ResponseWriter, req *http.Request) {
	var in struct {
		Code     string `json:"deviceCode"`
		Verifier string `json:"codeVerifier"`
	}
	if !decode(w, req, &in) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.flows[in.Code]
	digest := sha256.Sum256([]byte(in.Verifier))
	if f == nil || !time.Now().Before(f.expires) || !verifierFormat.MatchString(in.Verifier) ||
		subtle.ConstantTimeCompare([]byte(f.challenge), []byte(encoding.EncodeToString(digest[:]))) != 1 {
		http.Error(w, "invalid challenge", 403)
		return
	}
	if f.resolved {
		send(w, map[string]string{"state": "resolved"})
		return
	}
	if f.denied {
		send(w, map[string]string{"state": "denied"})
		return
	}
	if !f.ready {
		send(w, map[string]string{"state": "pending"})
		return
	}
	send(w, f.grant)
}

func (r *Realm) jwt(account Account, scope string, now, until time.Time) string {
	body, _ := json.Marshal(wallet.Claims{Issuer: r.Origin, Audience: wallet.Audience, Subject: account.Subject, Credential: account.Credential,
		Scope: scope, Issued: now.Unix(), Expires: until.Unix()})
	signed := encoding.EncodeToString([]byte(`{"typ":"JWT","alg":"EdDSA"}`)) + "." + encoding.EncodeToString(body)
	return signed + "." + encoding.EncodeToString(ed25519.Sign(r.Key, []byte(signed)))
}

const consentStyle = `
body { max-width: 40rem; margin: 1.25rem auto; padding: 0 1rem; font: 1rem/1.5 system-ui, sans-serif; overflow-wrap: anywhere; }
label, input:not([type=hidden]) { display: block; }
input:not([type=hidden]) { box-sizing: border-box; width: 100%; max-width: 24rem; min-height: 2.75rem; margin: .25rem 0 .75rem; padding: .5rem; font: inherit; }
button { min-height: 2.75rem; padding: .4rem 1rem; font: inherit; }
`

var consentStyleHash = func() string {
	digest := sha256.Sum256([]byte(consentStyle))
	return base64.StdEncoding.EncodeToString(digest[:])
}()

var page = template.Must(template.New("verify").Funcs(template.FuncMap{
	"marketAction": marketlab.ActionName,
	"deadline":     func(seconds int64) string { return time.Unix(seconds, 0).UTC().Format("2006-01-02 15:04:05 UTC") },
	"consentStyle": func() template.CSS { return template.CSS(consentStyle) },
}).Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>P03 test wallet</title>
<style>{{consentStyle}}</style>
<h1>{{.Role}} · P03 test wallet</h1><p>Action {{.Request.Action}}</p>
<form method="post" autocomplete="off"><input type="hidden" name="code" value="{{.Code}}">
<label>Test identity <input name="user" autocomplete="username" autocapitalize="none" autocorrect="off" spellcheck="false" maxlength="80" required></label>
<label>Independent credential <input type="password" name="password" autocomplete="current-password" maxlength="72" required></label>
{{with .Request.Session}}<p>Wallet {{.Wallet}} · epoch {{.Epoch}}</p><p>Request {{.ID}}</p>{{if eq .Kind "sign"}}<p>Participants{{range .Peers}} · {{.Role}}{{end}}</p><p>Expires {{deadline .Expires}}</p>{{end}}{{end}}
{{if .Market}}<p>Isolated StableNet lab 8283 · {{marketAction .Market.Action}} · {{.Request.Transfer.To}}</p>
<p>Input A {{.Market.A}} · input B {{.Market.B}} · minimum A {{.Market.MinA}} · minimum B {{.Market.MinB}} (atomic units)</p>
<p>Price version {{.Market.ExpectedPriceVersion}} · bound quote {{.Market.OfferDigest.Hex}} · deadline {{.Market.Deadline}}</p>
{{if .Market.Allowance}}<p>Approve exact token allowance {{.Market.Allowance}} atomic units to the pinned venue.</p>{{end}}
<p>Nonce {{.Request.Transfer.Nonce}} · gas price {{.Request.Transfer.MaxFeeWei}} wei · fixed gas {{.Market.Gas}}</p>
{{else}}{{with .Request.Transfer}}<p>Sepolia 11155111 · {{.To}} · {{.ValueWei}} wei</p><p>Nonce {{.Nonce}} · max gas price {{.MaxFeeWei}} wei · fixed gas 21000</p>{{end}}{{end}}
{{with .Request.Close}}<p>End signing access for wallet {{.Wallet}} at epoch {{.Epoch}}.</p>{{if .Request}}<p>Closure request {{.Request}} · no asset transfer.</p>{{end}}{{end}}
<button name="decision" value="allow">Confirm</button><button name="decision" value="deny">Deny</button></form></html>`))

func (r *Realm) show(w http.ResponseWriter, req *http.Request) {
	if r.oidc != nil {
		r.startOIDCBrowser(w, req)
		return
	}
	code := req.URL.Query().Get("code")
	r.mu.Lock()
	f := r.flows[code]
	if f == nil || !time.Now().Before(f.expires) || f.ready || f.denied || f.resolved || f.submitting {
		r.mu.Unlock()
		http.Error(w, "challenge unavailable", 404)
		return
	}
	view := struct {
		Role, Code string
		Request    challenge
		Market     *marketlab.Terms
	}{r.Role, code, f.request, f.market}
	r.mu.Unlock()
	// WebKit serializes the form's navigation Origin as null under no-referrer.
	// Keep the required same-origin POST while suppressing cross-origin referrers.
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, view)
}

func (r *Realm) confirm(w http.ResponseWriter, req *http.Request) {
	if r.oidc != nil {
		http.NotFound(w, req)
		return
	}
	req.Body = http.MaxBytesReader(w, req.Body, 4096)
	if req.Header.Get("Origin") != r.Origin || req.ParseForm() != nil {
		http.Error(w, "request denied", 403)
		return
	}
	code := req.PostForm.Get("code")
	decision := strings.TrimSpace(req.PostForm.Get("decision"))
	r.mu.Lock()
	f := r.flows[code]
	signDeny := f != nil && decision == "deny" && r.Role == "approval" && f.request.Action == string(ceremony.Sign)
	if f == nil || !time.Now().Before(f.expires) || f.ready || f.resolved || (f.denied && !signDeny) || f.submitting || f.attempts >= 5 {
		r.mu.Unlock()
		http.Error(w, "challenge unavailable", 403)
		return
	}
	f.attempts++
	r.mu.Unlock()
	user, password := req.PostForm.Get("user"), req.PostForm.Get("password")
	account := Account{Subject: r.Subject, Credential: r.Credential, Username: r.Username, PasswordHash: r.PasswordHash, Key: r.Key}
	if len(r.Accounts) > 0 {
		account = Account{}
		for _, candidate := range r.Accounts {
			if subtle.ConstantTimeCompare([]byte(user), []byte(candidate.Username)) == 1 {
				account = candidate
				break
			}
		}
	}
	if len(password) > 72 || bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)) != nil ||
		subtle.ConstantTimeCompare([]byte(user), []byte(account.Username)) != 1 ||
		(len(r.Accounts) > 0 && (r.Role == "approval" || r.Role == "recovery") && f.request.Owner != account.Subject) {
		http.Error(w, "confirmation rejected", 403)
		return
	}

	if signDeny {
		if r.confirmSignRefusal(w, req.Context(), f, account) {
			confirmed(w)
		}
		return
	}
	if decision == "deny" && r.Role == "recovery" {
		if r.confirmRefusal(w, req.Context(), f, account) {
			confirmed(w)
		}
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f.ready || f.denied || f.resolved || f.submitting || f.refusal != nil || f.signRefusalStarted || !time.Now().Before(f.expires) {
		http.Error(w, "challenge unavailable", 403)
		return
	}
	switch decision {
	case "allow":
		f.ready = true
		if r.Role == "identity" {
			r.finishIdentity(f, account, time.Now())
		} else if r.Role == "operator" {
			now := time.Now()
			f.grant = map[string]string{"state": "ready", "token": r.jwt(account, "wallet:recovery-ops", now, f.expires)}
		} else {
			proof := ed25519.Sign(account.Key, ceremony.ApprovalMessage(ceremony.Kind(f.request.Action), f.fingerprint))
			f.grant = map[string]string{"state": "ready", "proof": base64.StdEncoding.EncodeToString(proof)}
		}
	case "deny":
		f.denied = true
	default:
		http.Error(w, "confirmation rejected", 400)
		return
	}
	confirmed(w)
}

func confirmed(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><meta charset="utf-8"><title>Confirmation recorded</title><h1>Confirmation recorded.</h1><p>Return to the wallet and check the result.</p></html>`)
}

func (r *Realm) confirmRefusal(w http.ResponseWriter, ctx context.Context, f *flow, account Account) bool {
	r.mu.Lock()
	if f.ready || f.denied || f.submitting || !time.Now().Before(f.expires) {
		r.mu.Unlock()
		http.Error(w, "challenge unavailable", 403)
		return false
	}
	if r.RecoveryRefusalSink == nil || len(account.Key) != ed25519.PrivateKeySize ||
		f.request.Session == nil || f.request.Owner == "" || f.request.Owner != account.Subject {
		r.mu.Unlock()
		http.Error(w, "recovery refusal unavailable", 503)
		return false
	}
	session := f.request.Session
	if f.refusal == nil {
		vote := wallet.RecoveryRefusal{Subject: account.Subject, Wallet: session.Wallet,
			Session: session.ID, Fingerprint: f.fingerprint, Epoch: session.Epoch, Vote: uuid.NewString(), IssuedAt: time.Now().Unix()}
		f.refusal = &wallet.SignedRecoveryRefusal{Decision: vote, Signature: ed25519.Sign(account.Key, vote.Message())}
	}
	signed := *f.refusal
	f.submitting = true
	r.mu.Unlock()

	err := r.RecoveryRefusalSink(ctx, signed)
	r.mu.Lock()
	f.submitting = false
	f.denied = err == nil
	r.mu.Unlock()
	if err != nil {
		http.Error(w, "recovery refusal could not be recorded; retry the same confirmation", 503)
		return false
	}
	return true
}

func (r *Realm) confirmSignRefusal(w http.ResponseWriter, ctx context.Context, f *flow, account Account) bool {
	r.mu.Lock()
	if f.ready || f.resolved || f.submitting || !time.Now().Before(f.expires) {
		r.mu.Unlock()
		http.Error(w, "challenge unavailable", 403)
		return false
	}
	if f.denied {
		r.mu.Unlock()
		return true
	}
	owner := f.request.Owner
	if owner == "" {
		owner = account.Subject
	}
	if r.SignRefusalSink == nil || len(account.Key) != ed25519.PrivateKeySize ||
		f.request.Session == nil || !canonicalRequest(owner) || owner != account.Subject {
		r.mu.Unlock()
		http.Error(w, "sign refusal unavailable", 503)
		return false
	}
	session := f.request.Session
	vote := wallet.SignRefusal{Subject: owner, Wallet: session.Wallet,
		Session: session.ID, Fingerprint: f.fingerprint, Epoch: session.Epoch}
	signed := wallet.SignedSignRefusal{Decision: vote, Signature: ed25519.Sign(account.Key, vote.Message())}
	f.submitting = true
	f.signRefusalStarted = true
	r.mu.Unlock()

	err := r.SignRefusalSink(ctx, signed)
	r.mu.Lock()
	f.submitting = false
	f.denied = err == nil
	f.resolved = errors.Is(err, wallet.ErrConflict)
	r.mu.Unlock()
	if errors.Is(err, wallet.ErrConflict) {
		http.Error(w, "The request changed. Return to the wallet and check its current status.", http.StatusConflict)
		return false
	}
	if err != nil {
		http.Error(w, "sign refusal unavailable; retry or check the wallet request", 503)
		return false
	}
	return true
}
