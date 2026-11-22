package ceremony_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/gateway"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func runWalletAPI(t *testing.T, dsn string, control *ceremony.Controller, coordinator, approval, recovery ed25519.PrivateKey, restartNative func(), mobile *mobileTest, endpoints map[string]ceremony.Endpoint, actor string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	profile := wallet.Profile{
		First:         []ceremony.Peer{{ID: "a1", Role: ceremony.A, Epoch: 1}, {ID: "b1", Role: ceremony.B, Epoch: 1}, {ID: "c1", Role: ceremony.C, Epoch: 1}},
		Replacement:   []ceremony.Peer{{ID: "a2", Role: ceremony.A, Epoch: 2}, {ID: "b2", Role: ceremony.B, Epoch: 2}, {ID: "c2", Role: ceremony.C, Epoch: 2}},
		Future:        [][]ceremony.Peer{{{ID: "a3", Role: ceremony.A, Epoch: 3}, {ID: "b3", Role: ceremony.B, Epoch: 3}, {ID: "c3", Role: ceremony.C, Epoch: 3}}},
		RecoveryDelay: time.Millisecond,
	}
	marketPolicy := installedMarket(t)
	store := wallet.Store{Pool: pool, Profile: profile, MarketPolicy: marketPolicy}
	worker := wallet.Worker{Store: store, Control: control, SigningKey: coordinator}
	public, key, _ := ed25519.GenerateKey(rand.Reader)
	credential := uuid.NewString()
	issuer := "https://p03-id.synthetic.example"
	registry := wallet.MobileRegistry{}
	if mobile != nil {
		registry = wallet.MobileRegistry{Requests: mobile.requests, Approved: mobile.approved}
	}
	opsPublic, opsKey, _ := ed25519.GenerateKey(rand.Reader)
	opsSubject := uuid.NewString()
	opsIssuer := "https://p03-operator.synthetic.example"
	handler := wallet.API{Store: store, Mobile: registry, RecoveryOperatorSubject: opsSubject,
		RecoveryOperator: wallet.IdentityVerifier{Issuer: opsIssuer, Key: opsPublic}, ProfileSubject: actor, Identity: wallet.IdentityVerifier{Issuer: issuer, Key: public},
		ApprovalKey: approval.Public().(ed25519.PublicKey), RecoveryKey: recovery.Public().(ed25519.PublicKey)}.Handler()
	login := newGate(t, "identity", issuer, actor, credential, key)
	owner := newGate(t, "approval", "https://p03-approval.synthetic.example", actor, credential, approval, marketPolicy)
	independent := newGate(t, "recovery", "https://p03-recovery.synthetic.example", actor, credential, recovery)
	creds := login.confirm(t, map[string]string{"action": "login", "audience": wallet.Audience})
	operator := newGate(t, "operator", opsIssuer, opsSubject, uuid.NewString(), opsKey)
	opsToken := operator.confirm(t, map[string]string{"action": "operator", "audience": wallet.Audience})["token"]

	token := func(scope string) string {
		if scope == "wallet:recovery-ops" {
			return opsToken
		}
		if scope == "wallet:enroll" {
			return creds["enrollmentToken"]
		}
		return creds["token"]
	}
	call := func(method, path, scope string, request, result any, expected int) {
		t.Helper()
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token(scope))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != expected {
			t.Fatalf("%s %s got HTTP %d, expected %d: %s", method, path, response.Code, expected, response.Body.String())
		}
		if result != nil {
			if err := json.Unmarshal(response.Body.Bytes(), result); err != nil {
				t.Fatal(err)
			}
		}
	}
	call(http.MethodPost, "/v1/enrollment", "wallet", nil, nil, 401)
	call(http.MethodPost, "/v1/enrollment", "wallet:enroll", nil, nil, 204)
	if mobile != nil {
		var got struct {
			State  string
			Roster json.RawMessage
		}
		payload := map[string]string{"csr": mobile.csr}
		call(http.MethodPost, "/v1/mobile/roster", "wallet", payload, &got, 200)
		if got.State != "pending" {
			t.Fatal("unapproved mobile identity activated")
		}
		mobile.publish(t, uint64(mobile.role[1]-'0'))
		call(http.MethodPost, "/v1/mobile/roster", "wallet", payload, &got, 200)
		if got.State != "ready" || len(got.Roster) == 0 {
			t.Fatal("approved mobile roster unavailable")
		}
		mobile.launch(t, endpoints, got.Roster, control)
	}

	wait := func(op wallet.Operation) wallet.Operation {
		t.Helper()
		for time.Now().Unix() < op.Session.Expires {
			if err := worker.Step(ctx); err != nil {
				t.Fatal(err)
			}
			var r wallet.Operation
			call(http.MethodGet, "/v1/operations/"+op.Session.ID, "wallet", nil, &r, 200)
			if r.State == "succeeded" {
				return r
			}
			if r.State != "running" {
				t.Fatalf("%s stopped at %s", op.Session.Kind, r.State)
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("%s expired", op.Session.Kind)
		return wallet.Operation{}
	}
	transfer := wallet.Transfer{Profile: wallet.TestProfile, Chain: wallet.TestChain,
		To: "0x1234567890123456789012345678901234567890", ValueWei: "1000", Nonce: "0", MaxFeeWei: "1000000000", PriorityFeeWei: "1000000"}
	reserve := func(kind ceremony.Kind, details *wallet.Transfer) wallet.Operation {
		t.Helper()
		var op wallet.Operation
		body := map[string]any{"id": uuid.NewString(), "kind": kind}
		if details != nil {
			body["transfer"] = details
		}
		call(http.MethodPost, "/v1/operations", "wallet", body, &op, 200)
		return op
	}
	approve := func(op wallet.Operation, key ed25519.PrivateKey) {
		t.Helper()
		path := "/v1/operations/" + op.Session.ID + "/approval"
		call(http.MethodPut, path, "wallet", map[string][]byte{}, nil, 403)
		proofRealm := owner
		if key.Equal(recovery) {
			proofRealm = independent
		}
		response := proofRealm.confirm(t, map[string]any{"action": op.Session.Kind, "session": op.Session, "fingerprint": op.Fingerprint, "transfer": op.Transfer})
		call(http.MethodPut, path, "wallet", map[string]string{"proof": response["proof"]}, nil, 200)
	}
	dkg := reserve(ceremony.DKG, nil)
	var initial wallet.Wallet
	call(http.MethodGet, "/v1/wallet", "wallet", nil, &initial, 200)
	if initial.Address != "" || initial.State != "creating" {
		t.Fatal("prepared wallet exposed a receive address")
	}
	approve(dkg, approval)
	wait(dkg)
	call(http.MethodGet, "/v1/wallet", "wallet", nil, &initial, 200)
	if initial.Address == "" || initial.Epoch != 1 {
		t.Fatal("initial key was not activated")
	}
	if restartNative != nil {
		restartNative()
		deadline := time.Now().Add(20 * time.Second)
		for {
			got, err := control.Status(ctx, "a1", dkg.Session.ID)
			if err == nil && got.State == "ready" && bytes.Equal(got.PublicKey, initial.PublicKey) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("mobile participant did not recover its committed share after restart")
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	call(http.MethodPost, "/v1/operations", "wallet", map[string]any{"id": uuid.NewString(), "kind": "sign", "digest": "opaque"}, nil, 400)
	verifyTransfer := func() {
		t.Helper()
		sign := reserve(ceremony.Sign, &transfer)
		approve(sign, approval)
		got := wait(sign)
		if !ceremony.VerifySignature(initial.PublicKey, got.Session.Digest, got.Signature) {
			t.Fatal("normal wallet API signature failed verification")
		}
		var repeat wallet.Operation
		call(http.MethodGet, "/v1/operations/"+got.Session.ID, "wallet", nil, &repeat, 200)
		if !bytes.Equal(got.Signature, repeat.Signature) || repeat.RecoveryID == nil || got.RecoveryID == nil ||
			*repeat.RecoveryID != *got.RecoveryID {
			t.Fatal("wallet API lost signature result")
		}
	}
	if marketPolicy != nil {
		call(http.MethodPost, "/v1/operations", "wallet", map[string]any{"id": uuid.NewString(), "kind": "sign", "transfer": transfer}, nil, 409)
		transfer = quoteMarket(t, initial.Address, marketPolicy)
		altered := transfer
		market := *transfer.Market
		market.Deployment[0] ^= 1
		altered.Market = &market
		call(http.MethodPost, "/v1/operations", "wallet", map[string]any{"id": uuid.NewString(), "kind": "sign", "transfer": altered}, nil, 409)
		sign := reserve(ceremony.Sign, &transfer)
		approve(sign, approval)
		result := wait(sign)
		sendMarket(t, transfer, result, marketPolicy)
		return
	}
	verifyTransfer()
	for epoch := uint64(2); epoch <= 3; epoch++ {
		refresh := reserve(ceremony.Refresh, nil)
		time.Sleep(10 * time.Millisecond)
		approve(refresh, recovery)
		wait(refresh)
		var replacement wallet.Wallet
		call(http.MethodGet, "/v1/wallet", "wallet", nil, &replacement, 200)
		if replacement.Address != initial.Address || replacement.Epoch != epoch || !bytes.Equal(replacement.PublicKey, initial.PublicKey) {
			t.Fatalf("epoch %d changed the wallet public key", epoch)
		}
		if mobile != nil && mobile.role[1] == byte('0'+epoch) {
			mobile.restart()
			deadline := time.Now().Add(20 * time.Second)
			for {
				got, err := control.Status(ctx, mobile.role, refresh.Session.ID)
				if err == nil && got.State == "ready" && bytes.Equal(got.PublicKey, initial.PublicKey) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("replacement mobile lost its committed share after restart")
				}
				time.Sleep(200 * time.Millisecond)
			}
		}
		transfer.Nonce = string(rune('0' + epoch - 1))
		verifyTransfer()
	}
	call(http.MethodGet, "/v1/recovery/status", "wallet", nil, nil, 401)
	var status wallet.RecoveryStatus
	call(http.MethodGet, "/v1/recovery/status", "wallet:recovery-ops", nil, &status, 200)
	if status.QuorumExposure || status.IndependentSignPath || status.State != "active" {
		t.Fatal("unverified alternate path was enabled")
	}
	call(http.MethodPost, "/v1/recovery/incidents", "wallet:recovery-ops", map[string]string{"kind": "quorum_exposure"}, &status, 200)
	if !status.QuorumExposure || status.State != "blocked" || status.IndependentSignPath {
		t.Fatal("declared compromised quorum did not enter hold")
	}
	call(http.MethodPost, "/v1/operations", "wallet", map[string]any{"id": uuid.NewString(), "kind": "refresh"}, nil, 409)
	call(http.MethodPost, "/v1/operations", "wallet", map[string]any{"id": uuid.NewString(), "kind": "sign", "transfer": transfer}, nil, 409)
}

type testGate struct {
	origin, password, username, subject, credential string
	client                                          *http.Client
	handler                                         http.Handler
	realm                                           *gateway.Realm
	replace                                         func(*gateway.Realm)
}

func newGate(t *testing.T, role, origin, subject, credential string, key ed25519.PrivateKey, market ...*marketlab.Policy) testGate {
	t.Helper()
	var entropy [32]byte
	rand.Read(entropy[:])
	password := base64.RawURLEncoding.EncodeToString(entropy[:])
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	realm := &gateway.Realm{Role: role, Origin: origin, Subject: subject, Credential: credential,
		Username: "local-fixture", PasswordHash: string(hash), Key: key}
	if len(market) > 0 {
		realm.MarketPolicy = market[0]
	}
	return testGate{origin: origin, password: password, username: "local-fixture", subject: subject, handler: realm.Handler()}
}

func tlsGate(t *testing.T, role string, subjects []string, issuer ed25519.PrivateKey, proofs []ed25519.PrivateKey, browser *browserLab) []testGate {
	t.Helper()
	server := httptest.NewUnstartedServer(nil)
	t.Cleanup(server.Close)
	origin := "https://" + server.Listener.Addr().String()
	r := &gateway.Realm{Role: role, Origin: origin, Key: issuer}
	if browser != nil && role == "approval" {
		r.MarketPolicy = browser.market
	}
	result := make([]testGate, len(subjects))
	for i, subject := range subjects {
		var entropy [32]byte
		rand.Read(entropy[:])
		password := base64.RawURLEncoding.EncodeToString(entropy[:])
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		username := "fixture-" + string(rune('a'+i))
		var proof ed25519.PrivateKey
		if len(proofs) != 0 {
			proof = proofs[i]
		}
		credential := uuid.NewString()
		r.Accounts = append(r.Accounts, gateway.Account{Username: username, PasswordHash: string(hash), Subject: subject,
			Credential: credential, Key: proof})
		result[i] = testGate{origin: origin, password: password, username: username, subject: subject, credential: credential}
	}
	type realmHandler struct{ http.Handler }
	var current atomic.Pointer[realmHandler]
	replace := func(realm *gateway.Realm) { current.Store(&realmHandler{realm.Handler()}) }
	replace(r)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		current.Load().ServeHTTP(w, req)
	})
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	if browser != nil {
		server.TLS.Certificates = []tls.Certificate{browser.certificate(t)}
	}
	server.StartTLS()
	for i := range result {
		result[i].client, result[i].realm, result[i].replace = server.Client(), r, replace
	}
	return result
}

func (g testGate) send(t *testing.T, req *http.Request) (int, []byte) {
	t.Helper()
	if g.client == nil {
		rec := httptest.NewRecorder()
		g.handler.ServeHTTP(rec, req)
		return rec.Code, rec.Body.Bytes()
	}
	uri, err := url.Parse(g.origin + req.URL.RequestURI())
	if err != nil {
		t.Fatal(err)
	}
	req.URL, req.RequestURI, req.Host = uri, "", uri.Host
	resp, err := g.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32768))
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func (g testGate) confirm(t *testing.T, action any) map[string]string {
	t.Helper()
	var entropy [32]byte
	rand.Read(entropy[:])
	verifier := base64.RawURLEncoding.EncodeToString(entropy[:])
	digest := sha256.Sum256([]byte(verifier))
	jsonCall := func(path string, body any) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		status, response := g.send(t, req)
		var result map[string]any
		if status != 200 || json.Unmarshal(response, &result) != nil {
			t.Fatalf("isolated credential realm failed: %d", status)
		}
		return result
	}
	start := jsonCall("/p03/device/start", map[string]any{
		"request": action, "codeChallenge": base64.RawURLEncoding.EncodeToString(digest[:]), "codeChallengeMethod": "S256"})
	device, ok := start["deviceCode"].(string)
	if !ok {
		t.Fatal("credential challenge not created")
	}
	if jsonCall("/p03/device/poll", map[string]string{"deviceCode": device, "codeVerifier": verifier})["state"] != "pending" {
		t.Fatal("credential realm issued an unconfirmed result")
	}
	verifyPath := "/p03/device/verify?code=" + device
	if status, body := g.send(t, httptest.NewRequest("GET", verifyPath, nil)); status != 200 || !bytes.Contains(body, []byte("<form")) {
		t.Fatalf("browser confirmation page unavailable: %d", status)
	}
	form := url.Values{"code": {device}, "user": {g.username}, "password": {g.password}, "decision": {"allow"}}
	req := httptest.NewRequest("POST", "/p03/device/verify", strings.NewReader(form.Encode()))
	req.Header.Set("Origin", g.origin)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if status, _ := g.send(t, req); status != 200 {
		t.Fatalf("isolated browser confirmation failed: %d", status)
	}
	r := jsonCall("/p03/device/poll", map[string]string{"deviceCode": device, "codeVerifier": verifier})
	if r["state"] != "ready" {
		t.Fatal("confirmed credential remains unavailable")
	}
	result := make(map[string]string)
	for k, v := range r {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}
