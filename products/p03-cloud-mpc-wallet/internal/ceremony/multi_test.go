package ceremony_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/mobileconfig"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runMultiWallet(t *testing.T, dsn, dir string, endpoints map[string]ceremony.Endpoint, cert, key, ca string,
	coordinator ed25519.PrivateKey, approvals, recoveries [2]ed25519.PrivateKey) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	issuerPublic, issuer, _ := ed25519.GenerateKey(rand.Reader)
	profileAuthority, profileKey, _ := ed25519.GenerateKey(rand.Reader)
	operatorPublic, operatorKey, _ := ed25519.GenerateKey(rand.Reader)
	subjects := [2]string{uuid.NewString(), uuid.NewString()}
	operators := [2]string{uuid.NewString(), uuid.NewString()}
	browser := newBrowserLab(t)
	loginSubjects := subjects[:]
	if browser != nil && browser.recovery {
		loginSubjects = append(loginSubjects, subjects[0])
	}
	loginGates := tlsGate(t, "identity", loginSubjects, issuer, nil, browser)
	operatorGates := tlsGate(t, "operator", operators[:], operatorKey, nil, browser)
	ownerGates := tlsGate(t, "approval", subjects[:], nil, approvals[:], browser)
	recoveryGates := tlsGate(t, "recovery", subjects[:], nil, recoveries[:], browser)
	issuerURL, operatorURL := loginGates[0].origin, operatorGates[0].origin
	bindings := &wallet.Bindings{Directory: dir, Authority: profileAuthority, CoordinatorKey: coordinator.Public().(ed25519.PublicKey),
		IdentityKey: issuerPublic, OperatorKey: operatorPublic}
	store := wallet.Store{Pool: pool, Bindings: bindings}
	if browser != nil {
		store.MarketPolicy = browser.market
	}
	legacy, err := ceremony.NewController(cert, key, ca, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker := wallet.Worker{Store: store, SigningKey: coordinator, Control: &wallet.BoundParticipants{Store: store, Legacy: legacy, CertPath: cert, KeyPath: key, CAPath: ca}}
	mobileRequests, mobileApproved := filepath.Join(dir, "csrs"), filepath.Join(dir, "rosters")
	for _, path := range []string{mobileRequests, mobileApproved} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	api := wallet.API{Store: store, Mobile: wallet.MobileRegistry{Requests: mobileRequests, Approved: mobileApproved}, Identity: wallet.IdentityVerifier{Issuer: issuerURL, Key: issuerPublic},
		RecoveryOperator: wallet.IdentityVerifier{Issuer: operatorURL, Key: operatorPublic}}
	handler := api.Handler()
	type actor struct {
		login, join, inspect string
		own, recover         testGate
		profile              wallet.AccountProfile
	}
	var actors [2]actor
	for n, subject := range subjects {
		endpoint := make(map[string]ceremony.Endpoint)
		cohort := func(suffix byte, epoch uint64) []ceremony.Peer {
			var group []ceremony.Peer
			for _, role := range []ceremony.Role{ceremony.A, ceremony.B, ceremony.C} {
				id := string(byte(role[0])+('a'-'A')) + string(rune(suffix))
				endpoint[id] = endpoints[id]
				group = append(group, ceremony.Peer{ID: id, Role: role, Epoch: epoch})
			}
			return group
		}
		committees := [][]ceremony.Peer{cohort(byte('1'+n), 1)}
		if n == 0 {
			committees = append(committees, cohort('3', 2))
		}
		p := wallet.AccountProfile{Subject: subject, Issuer: issuerURL, Operator: operators[n], Committees: committees, Endpoints: endpoint,
			ApprovalKey: approvals[n].Public().(ed25519.PublicKey), RecoveryKey: recoveries[n].Public().(ed25519.PublicKey), DelaySeconds: 60}
		auth := loginGates[n]
		result := auth.confirm(t, map[string]string{"action": "login", "audience": wallet.Audience})
		inspect := operatorGates[n].confirm(t, map[string]string{"action": "operator", "audience": wallet.Audience})
		actors[n] = actor{login: result["token"], join: result["enrollmentToken"], inspect: inspect["token"], profile: p,
			own: ownerGates[n], recover: recoveryGates[n]}
	}
	register := func(p wallet.AccountProfile) {
		t.Helper()
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(struct{ Payload, Signature []byte }{raw, ed25519.Sign(profileKey, append([]byte(wallet.ProfileDomain), raw...))})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p.Subject+".json"), envelope, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if browser != nil && !browser.operator {
		for _, who := range actors {
			register(who.profile)
		}
		browser.run(t, api, worker, pool, subjects, loginGates, ownerGates, recoveryGates, operatorGates)
		return
	}
	call := func(token, method, path string, payload, out any, want int) {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		if result.Code != want {
			t.Fatalf("%s %s got %d expected %d: %s", method, path, result.Code, want, result.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(result.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	var lease struct {
		Subject string
		Linked  bool
	}
	call(actors[0].login, "GET", "/v1/session", nil, &lease, 200)
	if lease.Subject != subjects[0] || lease.Linked {
		t.Fatal("unbound subject received a native lease")
	}
	call(actors[0].login, "GET", "/v1/wallet", nil, nil, 403)
	var vaultKey [32]byte
	rand.Read(vaultKey[:])
	csr, err := mobileconfig.Prepare(t.TempDir(), vaultKey[:])
	if err != nil {
		t.Fatal(err)
	}
	clear(vaultKey[:])
	var registration struct{ State string }
	call(actors[0].login, "POST", "/v1/mobile/roster", map[string]string{"csr": csr}, &registration, 200)
	if registration.State != "pending" {
		t.Fatal("CSR bootstrap returned more than pending readiness")
	}
	for _, who := range actors {
		call(who.join, "POST", "/v1/enrollment", nil, nil, 403)
		register(who.profile)
		call(who.join, "POST", "/v1/enrollment", nil, nil, 204)
	}
	call(actors[0].login, "GET", "/v1/session", nil, &lease, 200)
	if !lease.Linked || lease.Subject != subjects[0] {
		t.Fatal("approved binding did not enable native lease")
	}
	shadow := actors[1].profile
	shadow.Subject = uuid.NewString()
	clone := newGate(t, "identity", issuerURL, shadow.Subject, uuid.NewString(), issuer)
	register(shadow)
	call(clone.confirm(t, map[string]string{"action": "login", "audience": wallet.Audience})["enrollmentToken"], "POST", "/v1/enrollment", nil, nil, 409)
	reserve := func(n int, kind ceremony.Kind, id string, transfer *wallet.Transfer) wallet.Operation {
		t.Helper()
		var op wallet.Operation
		request := map[string]any{"id": id, "kind": kind}
		if transfer != nil {
			request["transfer"] = transfer
		}
		call(actors[n].login, "POST", "/v1/operations", request, &op, 200)
		if op.RequestID != id || op.Session.ID == id {
			t.Fatal("client ID was not scoped to wallet")
		}
		return op
	}
	approve := func(n int, op wallet.Operation, which testGate, want int) {
		t.Helper()
		proof := which.confirm(t, map[string]any{"action": op.Session.Kind, "owner": which.subject, "session": op.Session,
			"fingerprint": op.Fingerprint, "transfer": op.Transfer})["proof"]
		call(actors[n].login, "PUT", "/v1/operations/"+op.RequestID+"/approval", map[string]string{"proof": proof}, nil, want)
	}
	wait := func(ops ...wallet.Operation) []wallet.Operation {
		t.Helper()
		completed := make([]wallet.Operation, len(ops))
		deadline := time.Now().Add(4 * time.Minute)
		for time.Now().Before(deadline) {
			if err := worker.Step(ctx); err != nil {
				t.Fatal(err)
			}
			var count int
			for n, op := range ops {
				call(actors[n].login, "GET", "/v1/operations/"+op.RequestID, nil, &completed[n], 200)
				if completed[n].State == "succeeded" {
					count++
				} else if completed[n].State != "running" {
					t.Fatalf("workflow stopped: %s", completed[n].State)
				}
			}
			if count == len(ops) {
				return completed
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatal("wallet operations timed out")
		return nil
	}
	same := uuid.NewString()
	first, second := reserve(0, ceremony.DKG, same, nil), reserve(1, ceremony.DKG, same, nil)
	if first.Session.Wallet == second.Session.Wallet || first.Session.ID == second.Session.ID || first.Fingerprint == second.Fingerprint {
		t.Fatal("shared wallet identity")
	}
	if retry := reserve(0, ceremony.DKG, same, nil); retry.Session.ID != first.Session.ID {
		t.Fatal("same owner changed ceremony")
	}
	approve(0, first, actors[1].own, 403)
	approve(0, first, actors[0].own, 200)
	approve(1, second, actors[1].own, 200)
	wait(first, second)
	var wallets [2]wallet.Wallet
	for n := range wallets {
		call(actors[n].login, "GET", "/v1/wallet", nil, &wallets[n], 200)
	}
	if wallets[0].Address == "" || wallets[0].Address == wallets[1].Address {
		t.Fatal("different DKGs exposed the same address")
	}
	if browser != nil && browser.operator {
		browser.runOperator(t, ctx, api, worker.Control, pool, wallets, operatorGates)
		fresh := loginGates[0].confirm(t, map[string]string{"action": "login", "audience": wallet.Audience})["token"]
		call(fresh, "POST", "/v1/operations", map[string]any{"id": uuid.NewString(), "kind": "sign",
			"transfer": wallet.Transfer{Profile: wallet.TestProfile, Chain: wallet.TestChain, To: "0x1234567890123456789012345678901234567890",
				ValueWei: "100", Nonce: "0", MaxFeeWei: "1000000000", PriorityFeeWei: "1000000"}}, nil, 409)
		return
	}
	tx := wallet.Transfer{Profile: wallet.TestProfile, Chain: wallet.TestChain, To: "0x1234567890123456789012345678901234567890",
		ValueWei: "100", Nonce: "0", MaxFeeWei: "1000000000", PriorityFeeWei: "1000000"}
	firstSign := reserve(0, ceremony.Sign, uuid.NewString(), &tx)
	secondSign := reserve(1, ceremony.Sign, uuid.NewString(), &tx)
	call(actors[1].login, "GET", "/v1/operations/"+firstSign.RequestID, nil, nil, 404)
	call(actors[1].login, "PUT", "/v1/operations/"+firstSign.RequestID+"/approval", map[string]string{"proof": ""}, nil, 404)
	approve(0, firstSign, actors[0].own, 200)
	approve(1, secondSign, actors[1].own, 200)
	signatures := wait(firstSign, secondSign)
	for n, sig := range signatures {
		if !ceremony.VerifySignature(wallets[n].PublicKey, sig.Session.Digest, sig.Signature) ||
			ceremony.VerifySignature(wallets[1-n].PublicKey, sig.Session.Digest, sig.Signature) {
			t.Fatal("signature not isolated")
		}
	}
	refresh := reserve(0, ceremony.Refresh, uuid.NewString(), nil)
	call(actors[1].login, "GET", "/v1/operations/"+refresh.RequestID, nil, nil, 404)
	call(actors[1].inspect, "GET", "/v1/recovery/status?wallet="+wallets[0].ID, nil, nil, 403)
	call(actors[0].inspect, "GET", "/v1/recovery/status?wallet="+wallets[0].ID, nil, nil, 200)
	// Advance the persisted delay in the isolated database to exercise production authorization without a test-only server option.
	if _, err := pool.Exec(ctx, `UPDATE wallet_mpc.operation SET not_before=clock_timestamp()-interval '1 second' WHERE operation_id=$1`, refresh.Session.ID); err != nil {
		t.Fatal(err)
	}
	approve(0, refresh, actors[1].recover, 403)
	approve(0, refresh, actors[0].recover, 200)
	wait(refresh)
	var recovered wallet.Wallet
	call(actors[0].login, "GET", "/v1/wallet", nil, &recovered, 200)
	if recovered.Epoch != 2 || recovered.Address != wallets[0].Address {
		t.Fatal("scoped recovery lost wallet identity")
	}
}
