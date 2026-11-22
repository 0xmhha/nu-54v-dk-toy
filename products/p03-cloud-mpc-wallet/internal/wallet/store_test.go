package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	secpECDSA "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresWallet(t *testing.T) {
	dsn := os.Getenv("P03_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("run make db-test for the scratch PostgreSQL fixture")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := Store{Pool: pool, Profile: Profile{
		First:         []ceremony.Peer{{ID: "aa", Role: ceremony.A, Epoch: 1}, {ID: "bb", Role: ceremony.B, Epoch: 1}, {ID: "cc", Role: ceremony.C, Epoch: 1}},
		Replacement:   []ceremony.Peer{{ID: "a2", Role: ceremony.A, Epoch: 2}, {ID: "b2", Role: ceremony.B, Epoch: 2}, {ID: "c2", Role: ceremony.C, Epoch: 2}},
		RecoveryDelay: time.Minute,
	}}
	actor := Claims{Subject: uuid.NewString(), Credential: uuid.NewString(), Issued: time.Now().Unix(), Expires: time.Now().Add(5 * time.Minute).Unix()}
	other := Claims{Subject: uuid.NewString(), Credential: uuid.NewString(), Issued: actor.Issued, Expires: actor.Expires}
	second := actor
	second.Credential = uuid.NewString()
	for _, a := range []Claims{actor, other} {
		if err := store.Enroll(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Link(ctx, actor, second); err != nil {
		t.Fatal(err)
	}
	approvePublic, approve, _ := ed25519.GenerateKey(rand.Reader)
	recoveryPublic, recovery, _ := ed25519.GenerateKey(rand.Reader)
	proof := func(op Operation, key ed25519.PrivateKey) []byte {
		return ed25519.Sign(key, ceremony.ApprovalMessage(op.Session.Kind, op.Fingerprint))
	}
	id := uuid.NewString()
	dkg, err := store.Reserve(ctx, actor, id, ceremony.DKG, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			r, err := store.Reserve(ctx, actor, id, ceremony.DKG, nil)
			if err != nil || r.Fingerprint != dkg.Fingerprint {
				t.Errorf("idempotent DKG %v", err)
			}
		})
	}
	wg.Wait()
	if _, err := store.Operation(ctx, other, id); !errors.Is(err, ErrMissing) {
		t.Fatalf("cross-account lookup: %v", err)
	}
	if _, err := store.Approve(ctx, actor, id, nil, approvePublic, recoveryPublic); !errors.Is(err, ErrForbidden) {
		t.Fatalf("login-only approval: %v", err)
	}
	if _, err := store.Approve(ctx, actor, id, proof(dkg, approve), approvePublic, recoveryPublic); err != nil {
		t.Fatal(err)
	}
	private, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := private.PubKey().SerializeUncompressed()
	report := observation{results: map[string]ceremony.Result{}, fenced: map[string]bool{}}
	makeAck := func(op Operation, peer, state string) ceremony.Result {
		return ceremony.Result{Session: op.Fingerprint, Peer: peer, State: state, PublicKey: pub}
	}
	report.results["aa"], report.results["bb"] = makeAck(dkg, "aa", "ready"), makeAck(dkg, "bb", "ready")
	if err := store.apply(ctx, dkg, report); err != nil {
		t.Fatal(err)
	}
	w, err := store.Wallet(ctx, actor)
	if err != nil || w.Address != "" || w.State != "creating" {
		t.Fatalf("partial ACK exposed wallet: %+v %v", w, err)
	}
	report.results["cc"] = makeAck(dkg, "cc", "ready")
	if err := store.apply(ctx, dkg, report); err != nil {
		t.Fatal(err)
	}
	w, err = store.Wallet(ctx, actor)
	if err != nil || w.Address == "" || w.State != "active" || w.Epoch != 1 {
		t.Fatalf("DKG not activated: %+v %v", w, err)
	}
	transfer := &Transfer{Profile: TestProfile, Chain: TestChain, To: "0x1234567890123456789012345678901234567890", ValueWei: "1000", Nonce: "0", MaxFeeWei: "1000000000", PriorityFeeWei: "1000000"}
	denial := func(op Operation) SignRefusal {
		return SignRefusal{Subject: actor.Subject, Wallet: op.Session.Wallet, Session: op.Session.ID,
			Epoch: op.Session.Epoch, Fingerprint: op.Fingerprint}
	}
	signedDenial := func(vote SignRefusal, key ed25519.PrivateKey) SignedSignRefusal {
		return SignedSignRefusal{Decision: vote, Signature: ed25519.Sign(key, vote.Message())}
	}
	refused, err := store.Reserve(ctx, actor, uuid.NewString(), ceremony.Sign, transfer)
	if err != nil {
		t.Fatal(err)
	}
	vote := denial(refused)
	for _, check := range []struct {
		name string
		vote SignRefusal
		key  ed25519.PrivateKey
	}{
		{"wrong owner", SignRefusal{Subject: other.Subject, Wallet: vote.Wallet, Session: vote.Session, Epoch: vote.Epoch, Fingerprint: vote.Fingerprint}, approve},
		{"wrong session", SignRefusal{Subject: vote.Subject, Wallet: vote.Wallet, Session: uuid.NewString(), Epoch: vote.Epoch, Fingerprint: vote.Fingerprint}, approve},
		{"wrong epoch", SignRefusal{Subject: vote.Subject, Wallet: vote.Wallet, Session: vote.Session, Epoch: 2, Fingerprint: vote.Fingerprint}, approve},
		{"wrong intent fingerprint", SignRefusal{Subject: vote.Subject, Wallet: vote.Wallet, Session: vote.Session, Epoch: vote.Epoch, Fingerprint: dkg.Fingerprint}, approve},
		{"recovery key", vote, recovery},
		{"wrong kind", denial(dkg), approve},
	} {
		t.Run(check.name, func(t *testing.T) {
			if err := store.RecordSignRefusal(ctx, signedDenial(check.vote, check.key), approvePublic); !errors.Is(err, ErrForbidden) {
				t.Fatalf("invalid refusal returned %v", err)
			}
		})
	}
	refusal := signedDenial(vote, approve)
	for range 4 {
		wg.Go(func() {
			if err := store.RecordSignRefusal(ctx, refusal, approvePublic); err != nil {
				t.Errorf("concurrent refusal returned %v", err)
			}
		})
	}
	wg.Wait()
	retried, err := store.Reserve(ctx, actor, refused.RequestID, ceremony.Sign, transfer)
	if err != nil || retried.State != "refused" || retried.Fingerprint != refused.Fingerprint {
		t.Fatalf("same request submission changed terminal refusal: %s %v", retried.State, err)
	}
	reloaded, err := (Store{Pool: pool, Profile: store.Profile}).Operation(ctx, actor, refused.RequestID)
	if err != nil || reloaded.State != "refused" || len(reloaded.Signature) != 0 || reloaded.RecoveryID != nil ||
		len(reloaded.proof) != 0 || reloaded.Acknowledged != 0 {
		t.Fatalf("persisted refusal contains round output or authorization: state=%s error=%v", reloaded.State, err)
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM wallet_mpc.audit_outbox WHERE operation_id=$1 AND event_kind='sign_refused'`, refused.Session.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("refusal audit event count=%d error=%v", events, err)
	}
	if _, err := store.Approve(ctx, actor, refused.RequestID, proof(refused, approve), approvePublic, recoveryPublic); !errors.Is(err, ErrConflict) {
		t.Fatalf("approval reopened refused sign: %v", err)
	}
	racing, err := store.Reserve(ctx, actor, uuid.NewString(), ceremony.Sign, transfer)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var approveErr, refuseErr error
	wg.Go(func() {
		<-start
		_, approveErr = store.Approve(ctx, actor, racing.RequestID, proof(racing, approve), approvePublic, recoveryPublic)
	})
	wg.Go(func() {
		<-start
		refuseErr = store.RecordSignRefusal(ctx, signedDenial(denial(racing), approve), approvePublic)
	})
	close(start)
	wg.Wait()
	switch {
	case approveErr == nil && errors.Is(refuseErr, ErrConflict):
	case refuseErr == nil && errors.Is(approveErr, ErrConflict):
	default:
		t.Fatalf("expected one winning decision: approval=%v refusal=%v", approveErr, refuseErr)
	}
	sign, err := store.Reserve(ctx, actor, uuid.NewString(), ceremony.Sign, transfer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Approve(ctx, actor, sign.RequestID, proof(refused, approve), approvePublic, recoveryPublic); !errors.Is(err, ErrForbidden) {
		t.Fatalf("approved a new request using the refused session proof: %v", err)
	}
	if _, err := store.Approve(ctx, actor, sign.Session.ID, proof(dkg, approve), approvePublic, recoveryPublic); !errors.Is(err, ErrForbidden) {
		t.Fatalf("DKG proof replay: %v", err)
	}
	if _, err := store.Approve(ctx, actor, sign.Session.ID, proof(sign, approve), approvePublic, recoveryPublic); err != nil {
		t.Fatal(err)
	}
	der := secpECDSA.Sign(private, sign.Session.Digest[:]).Serialize()
	parity := secpECDSA.SignCompact(private, sign.Session.Digest[:], false)[0] - 27
	sigReport := observation{results: map[string]ceremony.Result{}}
	for _, p := range sign.Session.Peers {
		sigReport.results[p.ID] = ceremony.Result{Session: sign.Fingerprint, Peer: p.ID, State: "signed", Signature: der, RecoveryID: []byte{parity}}
	}
	t.Run("partial signing round remains unsigned", func(t *testing.T) {
		partial := observation{results: map[string]ceremony.Result{}}
		for _, p := range sign.Session.Peers {
			if p.Role == ceremony.B {
				partial.results[p.ID] = sigReport.results[p.ID]
			}
		}
		if err := store.apply(ctx, sign, partial); err != nil {
			t.Fatal(err)
		}
		got, err := store.Operation(ctx, actor, sign.RequestID)
		if err != nil || got.State != "running" || got.Acknowledged != 1 || len(got.Signature) != 0 {
			t.Fatalf("single participant exposed a signature: %s %v", got.State, err)
		}
		if err := store.RecordSignRefusal(ctx, signedDenial(denial(sign), approve), approvePublic); !errors.Is(err, ErrConflict) {
			t.Fatalf("a refusal rewrote a legitimately started round: %v", err)
		}
	})
	if err := store.apply(ctx, sign, sigReport); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordSignRefusal(ctx, signedDenial(denial(sign), approve), approvePublic); !errors.Is(err, ErrConflict) {
		t.Fatalf("completed signing accepted a late refusal: %v", err)
	}
	for range 2 {
		got, err := store.Operation(ctx, actor, sign.Session.ID)
		if err != nil || string(got.Signature) != string(der) || got.RecoveryID == nil || *got.RecoveryID != parity {
			t.Fatalf("signature requery failed: %s %v", got.State, err)
		}
	}
	modernized := store
	modernized.Bindings = &Bindings{LegacySubject: actor.Subject}
	legacyResult, err := modernized.Operation(ctx, actor, sign.Session.ID)
	if err != nil || legacyResult.RequestID != sign.Session.ID || string(legacyResult.Signature) != string(der) {
		t.Fatalf("enabling profiles invalidated a legacy result: %v", err)
	}

	t.Run("expiry while waiting for a row lock", func(t *testing.T) {
		approvalID := uuid.NewString()
		pending, err := store.Reserve(ctx, actor, approvalID, ceremony.Sign, transfer)
		if err != nil {
			t.Fatal(err)
		}
		issuer, key, _ := ed25519.GenerateKey(rand.Reader)
		api := API{Store: store, ProfileSubject: actor.Subject, ApprovalKey: approvePublic,
			Identity: IdentityVerifier{Issuer: "https://local-p03-identity.example", Key: issuer}}.Handler()
		for _, action := range []struct {
			name, method, path string
			body               any
			block              string
		}{
			{"private signed receipt", http.MethodGet, "/v1/operations/" + sign.Session.ID, nil, w.ID},
			{"approval after operation lock", http.MethodPut, "/v1/operations/" + approvalID + "/approval", map[string][]byte{"proof": proof(pending, approve)}, approvalID},
		} {
			t.Run(action.name, func(t *testing.T) {
				var blocked rowBlocker
				if action.block == w.ID {
					blocked = holdWalletRow(t, pool, w.ID)
				} else {
					blocked = holdRow(t, pool, "SELECT operation_id FROM wallet_mpc.operation WHERE operation_id=$1 FOR UPDATE", approvalID)
				}
				short := actor
				short.Issuer, short.Audience, short.Scope = "https://local-p03-identity.example", Audience, "wallet"
				short.Issued, short.Expires = time.Now().Unix(), time.Now().Add(2*time.Second).Unix()
				body, err := json.Marshal(action.body)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(action.method, action.path, bytes.NewReader(body))
				request.Header.Set("Authorization", "Bearer "+identityToken(t, key, short))
				result := httptest.NewRecorder()
				complete := make(chan struct{})
				go func() { api.ServeHTTP(result, request); close(complete) }()
				blocked.wait(t)
				waitExpiry(short.Expires)
				blocked.release(t)
				<-complete
				if result.Code != http.StatusForbidden || result.Body.String() != "wallet access denied\n" {
					t.Fatalf("expired authenticated request returned HTTP %d", result.Code)
				}
			})
		}
		got, err := store.Operation(ctx, actor, approvalID)
		if err != nil || got.State != "reserved" || len(got.proof) != 0 {
			t.Fatalf("expired approval mutated state: %v", err)
		}
	})

	t.Run("credential scope and expiry while waiting on identity", func(t *testing.T) {
		newCredential := actor
		newCredential.Credential = uuid.NewString()
		if err := store.Enroll(ctx, newCredential); !errors.Is(err, ErrForbidden) {
			t.Fatalf("enrollment-only attached additional credential: %v", err)
		}
		for _, action := range []string{"enrollment", "old link claim", "new link claim", "unlink"} {
			t.Run(action, func(t *testing.T) {
				original, next := actor, newCredential
				subject := actor.Subject
				if action == "enrollment" {
					subject = other.Subject
					next = other
				}
				blocked := holdRow(t, pool, "SELECT subject_ref FROM wallet_mpc.identity WHERE subject_ref=$1 FOR UPDATE", subject)
				expiry := time.Now().Add(2 * time.Second).Unix()
				if action == "old link claim" || action == "unlink" {
					original.Expires = expiry
				} else {
					next.Expires = expiry
				}
				result := make(chan error, 1)
				go func() {
					switch action {
					case "enrollment":
						result <- store.Enroll(ctx, next)
					case "unlink":
						result <- store.Unlink(ctx, original, second.Credential)
					default:
						result <- store.Link(ctx, original, next)
					}
				}()
				blocked.wait(t)
				waitExpiry(expiry)
				blocked.release(t)
				if err := <-result; !errors.Is(err, ErrForbidden) {
					t.Fatalf("expired credential effect: %v", err)
				}
			})
		}
		got, err := store.Credentials(ctx, actor)
		if err != nil || len(got) != 2 {
			t.Fatalf("expired credential actions changed links: %v", err)
		}
	})

	t.Run("market deadline and exact late retry", func(t *testing.T) { testMarketExpiry(t, store, other) })

	oldEpochSign, err := store.Reserve(ctx, actor, uuid.NewString(), ceremony.Sign, transfer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Approve(ctx, actor, oldEpochSign.RequestID, proof(oldEpochSign, approve), approvePublic, recoveryPublic); err != nil {
		t.Fatal(err)
	}
	blocked := holdWalletRow(t, pool, w.ID)
	refreshID := uuid.NewString()
	type reserveResult struct {
		op  Operation
		err error
	}
	refreshResult := make(chan reserveResult, 1)
	go func() {
		op, err := store.Reserve(ctx, actor, refreshID, ceremony.Refresh, nil)
		refreshResult <- reserveResult{op, err}
	}()
	blocked.wait(t)
	var releasedAt time.Time
	if err := blocked.tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&releasedAt); err != nil {
		t.Fatal(err)
	}
	blocked.release(t)
	reserved := <-refreshResult
	refresh, err := reserved.op, reserved.err
	if err != nil || refresh.NotBefore.Before(releasedAt.Add(store.Profile.RecoveryDelay)) {
		t.Fatalf("recovery deadline predates freeze plus delay: %v", err)
	}
	if retry, err := store.Reserve(ctx, actor, refreshID, ceremony.Refresh, nil); err != nil || !retry.NotBefore.Equal(refresh.NotBefore) {
		t.Fatalf("recovery retry changed cooldown: %v", err)
	}

	if _, err := store.Approve(ctx, actor, refresh.Session.ID, proof(refresh, recovery), approvePublic, recoveryPublic); !errors.Is(err, ErrForbidden) {
		t.Fatalf("recovery delay bypassed: %v", err)
	}
	if _, err := store.Reserve(ctx, actor, uuid.NewString(), ceremony.Sign, transfer); !errors.Is(err, ErrConflict) {
		t.Fatalf("old epoch sign still available: %v", err)
	}
	oldReport := observation{results: make(map[string]ceremony.Result)}
	for peer, result := range sigReport.results {
		result.Session = oldEpochSign.Fingerprint
		oldReport.results[peer] = result
	}
	if err := store.apply(ctx, oldEpochSign, oldReport); err != nil {
		t.Fatal(err)
	}
	discarded, err := store.Operation(ctx, actor, oldEpochSign.RequestID)
	if err != nil || discarded.State != "failed" || len(discarded.proof) != 0 || len(discarded.Signature) != 0 {
		t.Fatalf("old-epoch signing round not discarded during recovery: %s %v", discarded.State, err)
	}
	got, err := store.Operation(ctx, actor, sign.Session.ID)
	if err != nil || len(got.Signature) != 0 {
		t.Fatalf("blocked wallet leaked old signature: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE wallet_mpc.operation SET not_before=clock_timestamp()-interval '1 second' WHERE operation_id=$1`, refresh.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Approve(ctx, actor, refresh.Session.ID, proof(refresh, approve), approvePublic, recoveryPublic); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ordinary proof recovered wallet: %v", err)
	}
	if _, err := store.Approve(ctx, actor, refresh.Session.ID, proof(refresh, recovery), approvePublic, recoveryPublic); err != nil {
		t.Fatal(err)
	}
	refReport := observation{results: map[string]ceremony.Result{}, fenced: map[string]bool{"bb": true, "cc": false}}
	for _, p := range refresh.Session.Peers {
		state := "ready"
		if p.Epoch == 1 {
			state = "retired"
		}
		refReport.results[p.ID] = makeAck(refresh, p.ID, state)
	}
	if err := store.apply(ctx, refresh, refReport); err != nil {
		t.Fatal(err)
	}
	frozen, err := store.Wallet(ctx, actor)
	if err != nil || frozen.State != "blocked" || frozen.Epoch != 1 {
		t.Fatalf("missing fence activated new epoch: %+v %v", frozen, err)
	}
	refReport.fenced["cc"] = true
	if err := store.apply(ctx, refresh, refReport); err != nil {
		t.Fatal(err)
	}
	active, err := store.Wallet(ctx, actor)
	if err != nil || active.State != "active" || active.Epoch != 2 || active.Address != w.Address {
		t.Fatalf("invalid recovery: %+v %v", active, err)
	}
	outcomes := make(chan error, 2)
	wg.Go(func() { outcomes <- store.Unlink(ctx, actor, actor.Credential) })
	wg.Go(func() { outcomes <- store.Unlink(ctx, second, second.Credential) })
	wg.Wait()
	first, next := <-outcomes, <-outcomes
	if (first == nil) == (next == nil) {
		t.Fatalf("exactly one unlink must succeed: %v %v", first, next)
	}
	remaining := actor
	if _, err := store.Wallet(ctx, actor); errors.Is(err, ErrForbidden) {
		remaining = second
	}
	proofHash := sha256.Sum256(AppendClose(active.ID, active.Epoch))
	if err := store.Close(ctx, remaining, active.ID, active.Epoch, ed25519.Sign(approve, ceremony.ApprovalMessage("close", proofHash)), approvePublic); err != nil {
		t.Fatal(err)
	}
	closed, err := store.Wallet(ctx, remaining)
	if err != nil || closed.State != "closed" || closed.Address != "" || len(closed.PublicKey) != 0 {
		t.Fatalf("close drift: %+v %v", closed, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM wallet_mpc.auth_link WHERE subject_ref=$1 AND unlinked_at IS NULL`, actor.Subject).Scan(&count); err != nil || count != 1 {
		t.Fatalf("last credential count %d %v", count, err)
	}
}

type rowBlocker struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
	pid  uint32
}

func holdWalletRow(t *testing.T, pool *pgxpool.Pool, wallet string) rowBlocker {
	t.Helper()
	return holdRow(t, pool, "SELECT wallet_id FROM wallet_mpc.wallet WHERE wallet_id=$1 FOR UPDATE", wallet)
}

func holdRow(t *testing.T, pool *pgxpool.Pool, query, id string) rowBlocker {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err := tx.Exec(ctx, query, id); err != nil {
		t.Fatal(err)
	}
	return rowBlocker{pool, tx, tx.Conn().PgConn().PID()}
}

func (b rowBlocker) wait(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for {
		var waiting bool
		err := b.pool.QueryRow(ctx, "SELECT EXISTS(SELECT FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid)))", b.pid).Scan(&waiting)
		if err != nil {
			t.Fatalf("request did not wait on held transaction: %v", err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (b rowBlocker) release(t *testing.T) {
	t.Helper()
	if err := b.tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func waitExpiry(unix int64) {
	time.Sleep(time.Until(time.Unix(unix, 0)) + 20*time.Millisecond)
}
