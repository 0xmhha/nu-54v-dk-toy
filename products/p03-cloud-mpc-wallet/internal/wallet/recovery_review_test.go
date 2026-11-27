package wallet

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRecoveryRefusalAndReview(t *testing.T) {
	dsn := os.Getenv("P03_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires the P03 scratch migration fixture")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE nu54_p03_service")
		return err
	}
	service, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	var canRewrite bool
	if err := service.QueryRow(ctx, `SELECT
		has_table_privilege(current_user, 'wallet_mpc.recovery_refusal', 'UPDATE,DELETE') OR
		has_column_privilege(current_user, 'wallet_mpc.recovery_review', 'operator_ref', 'UPDATE')`).Scan(&canRewrite); err != nil || canRewrite {
		t.Fatalf("restricted service can rewrite refusal or operator evidence: %v", err)
	}
	profile := Profile{
		First:       []ceremony.Peer{{ID: "pa", Role: ceremony.A, Epoch: 1}, {ID: "pb", Role: ceremony.B, Epoch: 1}, {ID: "pc", Role: ceremony.C, Epoch: 1}},
		Replacement: []ceremony.Peer{{ID: "p2a", Role: ceremony.A, Epoch: 2}, {ID: "p2b", Role: ceremony.B, Epoch: 2}, {ID: "p2c", Role: ceremony.C, Epoch: 2}},
	}
	store := Store{Pool: service, Profile: profile}
	actor := Claims{Subject: uuid.NewString(), Credential: uuid.NewString(), Expires: time.Now().Add(5 * time.Minute).Unix()}
	other := Claims{Subject: uuid.NewString(), Credential: uuid.NewString(), Expires: actor.Expires}
	operator := Claims{Subject: uuid.NewString(), Credential: uuid.NewString(), Expires: actor.Expires}
	for _, owner := range []Claims{actor, other} {
		if err := store.Enroll(ctx, owner); err != nil {
			t.Fatal(err)
		}
	}
	approvalKey, approval, _ := ed25519.GenerateKey(rand.Reader)
	recoveryKey, recovery, _ := ed25519.GenerateKey(rand.Reader)
	dkg, err := store.Reserve(ctx, actor, uuid.NewString(), ceremony.DKG, nil)
	if err != nil {
		t.Fatal(err)
	}
	approvalProof := func(op Operation, key ed25519.PrivateKey) []byte {
		return ed25519.Sign(key, ceremony.ApprovalMessage(op.Session.Kind, op.Fingerprint))
	}
	if _, err := store.Approve(ctx, actor, dkg.RequestID, approvalProof(dkg, approval), approvalKey, recoveryKey); err != nil {
		t.Fatal(err)
	}
	material, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	report := observation{results: map[string]ceremony.Result{}}
	for _, peer := range dkg.Session.Peers {
		report.results[peer.ID] = ceremony.Result{Session: dkg.Fingerprint, Peer: peer.ID, State: "ready", PublicKey: material.PubKey().SerializeUncompressed()}
	}
	if err := store.apply(ctx, dkg, report); err != nil {
		t.Fatal(err)
	}
	old, err := store.Reserve(ctx, actor, uuid.NewString(), ceremony.Refresh, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := approvalProof(old, recovery)
	if _, err := pool.Exec(ctx, `UPDATE wallet_mpc.operation SET not_before=clock_timestamp()-interval '1 second' WHERE operation_id=$1`, old.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Approve(ctx, actor, old.RequestID, before, approvalKey, recoveryKey); err != nil {
		t.Fatal(err)
	}
	refusal := RecoveryRefusal{Subject: actor.Subject, Wallet: old.Session.Wallet, Session: old.Session.ID,
		Fingerprint: old.Fingerprint, Epoch: old.Session.Epoch, Vote: uuid.NewString(), IssuedAt: time.Now().Unix()}
	sign := func(v RecoveryRefusal, key ed25519.PrivateKey) SignedRecoveryRefusal {
		return SignedRecoveryRefusal{Decision: v, Signature: ed25519.Sign(key, v.Message())}
	}
	cross := refusal
	cross.Subject = other.Subject
	wrongEpoch := refusal
	wrongEpoch.Epoch++
	wrongRequest := refusal
	wrongRequest.Session = uuid.NewString()
	for _, bad := range []SignedRecoveryRefusal{sign(cross, recovery), sign(wrongEpoch, recovery), sign(wrongRequest, recovery), sign(refusal, approval)} {
		if err := store.RecordRecoveryRefusal(ctx, bad, recoveryKey); err == nil {
			t.Fatal("invalid refusal accepted")
		}
	}
	vote := sign(refusal, recovery)
	for range 2 {
		if err := store.RecordRecoveryRefusal(ctx, vote, recoveryKey); err != nil {
			t.Fatal(err)
		}
	}
	restarted := Store{Pool: service, Profile: profile}
	late := observation{results: map[string]ceremony.Result{}, fenced: map[string]bool{}}
	for _, peer := range old.Session.Peers {
		if peer.Epoch == old.Session.Epoch {
			late.results[peer.ID] = ceremony.Result{Session: old.Fingerprint, Peer: peer.ID, State: "ready", PublicKey: material.PubKey().SerializeUncompressed()}
		} else {
			late.fenced[peer.ID] = true
		}
	}
	if err := restarted.apply(ctx, old, late); err != nil {
		t.Fatal(err)
	}
	history, err := restarted.RecoveryHistory(ctx, actor)
	if err != nil || len(history) != 1 || history[0].RequestID != old.RequestID {
		t.Fatalf("original refusal missing from restart-safe history: %v", err)
	}
	if items, err := restarted.RecoveryHistory(ctx, other); err != nil || len(items) != 0 {
		t.Fatalf("other account refusal history: %v", err)
	}

	got, err := restarted.Operation(ctx, actor, old.RequestID)
	if err != nil || got.State != "denied" || got.Refusal == nil || got.Refusal.Decision.Vote != refusal.Vote || !ed25519.Verify(recoveryKey, got.Refusal.Decision.Message(), got.Refusal.Signature) {
		t.Fatalf("refusal missing after restart: %v", err)
	}
	if _, err := restarted.Approve(ctx, actor, old.RequestID, before, approvalKey, recoveryKey); err == nil {
		t.Fatal("old positive proof released refusal")
	}
	if _, err := restarted.Reserve(ctx, actor, uuid.NewString(), ceremony.Refresh, nil); err == nil {
		t.Fatal("new request released refusal")
	}
	if _, err := restarted.Operation(ctx, other, old.RequestID); !errors.Is(err, ErrMissing) {
		t.Fatal("other subject obtained refusal")
	}
	newID := uuid.NewString()
	if _, err := restarted.ReviewRecovery(ctx, actor, old.RequestID, newID); err == nil {
		t.Fatal("review without operator permit")
	}
	// Exercise the actual incident transition. Restore only the isolated audit
	// fixture afterward to exercise the independent positive history in this case.
	for _, permitFirst := range []bool{false, true} {
		if permitFirst {
			if err := restarted.PermitRecoveryReview(ctx, actor.Subject, old.RequestID, operator); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := restarted.recoveryStatus(ctx, actor.Subject, true, operator); err != nil {
			t.Fatal(err)
		}
		if err := restarted.PermitRecoveryReview(ctx, actor.Subject, old.RequestID, operator); !errors.Is(err, ErrConflict) {
			t.Fatalf("quorum exposure allowed an authority permit: %v", err)
		}
		if _, err := restarted.ReviewRecovery(ctx, actor, old.RequestID, newID); err == nil {
			t.Fatal("quorum exposure allowed a new linked ceremony")
		}
		if record, err := restarted.Operation(ctx, actor, old.RequestID); err != nil || record.ReviewAvailable {
			t.Fatalf("quorum exposure advertised review: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM wallet_mpc.audit_outbox
			WHERE wallet_id=$1 AND event_kind='incident_hold'`, old.Session.Wallet); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := restarted.ReviewRecovery(ctx, actor, old.RequestID, newID)
	if err != nil || fresh.Fingerprint == old.Fingerprint || fresh.Session.Epoch != old.Session.Epoch {
		t.Fatalf("review did not reserve a fresh ceremony: %v", err)
	}
	if same, err := restarted.ReviewRecovery(ctx, actor, old.RequestID, newID); err != nil || same.Fingerprint != fresh.Fingerprint {
		t.Fatalf("review retry changed ceremony: %v", err)
	}
	if _, err := restarted.ReviewRecovery(ctx, actor, old.RequestID, uuid.NewString()); err == nil {
		t.Fatal("review permitted another successor")
	}
	if _, err := restarted.Approve(ctx, actor, newID, before, approvalKey, recoveryKey); err == nil {
		t.Fatal("old proof authorized fresh review")
	}
	if state, err := restarted.Wallet(ctx, actor); err != nil || state.State != "blocked" || state.Epoch != 1 {
		t.Fatalf("review activated candidate: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE wallet_mpc.operation SET state='held' WHERE operation_id=$1`, fresh.Session.ID); err != nil {
		t.Fatal(err)
	}
	if state, err := restarted.Wallet(ctx, actor); err != nil || state.RecoveryAvailable {
		t.Fatalf("a held successor advertised a new unlinked recovery: %v", err)
	}
	if _, err := restarted.Reserve(ctx, actor, uuid.NewString(), ceremony.Refresh, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("a held successor permitted an unlinked review: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE wallet_mpc.operation SET state='reserved' WHERE operation_id=$1`, fresh.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE wallet_mpc.operation SET not_before=clock_timestamp()-interval '1 second' WHERE operation_id=$1`, fresh.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Approve(ctx, actor, newID, approvalProof(fresh, recovery), approvalKey, recoveryKey); err != nil {
		t.Fatal(err)
	}
	verdict := observation{results: map[string]ceremony.Result{}, fenced: map[string]bool{}}
	for _, peer := range fresh.Session.Peers {
		state := "ready"
		if peer.Epoch < fresh.Session.Epoch {
			state = "retired"
			verdict.fenced[peer.ID] = true
		}
		verdict.results[peer.ID] = ceremony.Result{Session: fresh.Fingerprint, Peer: peer.ID, State: state, PublicKey: material.PubKey().SerializeUncompressed()}
	}
	if err := restarted.apply(ctx, fresh, verdict); err != nil {
		t.Fatal(err)
	}
	if state, err := restarted.Wallet(ctx, actor); err != nil || state.State != "active" || state.Epoch != 2 {
		t.Fatalf("fresh evidence did not activate authorized review: %v", err)
	}
	if original, err := restarted.Operation(ctx, actor, old.RequestID); err != nil || original.State != "denied" ||
		original.Refusal == nil || original.SuccessorRequest != newID {
		t.Fatalf("successful review changed original refusal evidence: %v", err)
	}

	request := uuid.NewString()
	closureProof := func(ref string) []byte {
		sum := sha256.Sum256(AppendClose(fresh.Session.Wallet, 2, ref))
		return ed25519.Sign(approval, ceremony.ApprovalMessage("close", sum))
	}
	accepted, err := restarted.CloseWithReceipt(ctx, actor, fresh.Session.Wallet, 2, request, closureProof(request), approvalKey)
	if err != nil || accepted.Request != request || accepted.CompletedAt != nil || accepted.AssetMoves != 0 {
		t.Fatalf("incorrect closure admission: %+v %v", accepted, err)
	}
	if _, err := restarted.CloseWithReceipt(ctx, actor, fresh.Session.Wallet, 2, uuid.NewString(), closureProof(request), approvalKey); !errors.Is(err, ErrForbidden) {
		t.Fatalf("request substitution accepted: %v", err)
	}
	if got, err := restarted.Closure(ctx, actor, request); err != nil || !got.AcceptedAt.Equal(accepted.AcceptedAt) || got.CompletedAt != nil {
		t.Fatalf("restarted receipt lookup: %+v %v", got, err)
	}
	if _, err := restarted.Closure(ctx, other, request); !errors.Is(err, ErrMissing) {
		t.Fatalf("another owner obtained closure receipt: %v", err)
	}
	fenced := &closureFences{missing: "p2c"}
	worker := Worker{Store: restarted, Control: fenced}
	if err := worker.fenceClosed(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := restarted.Closure(ctx, actor, request); err != nil || got.CompletedAt != nil {
		t.Fatalf("partial fencing falsely completed closure: %+v %v", got, err)
	}
	fenced.missing = ""
	if err := worker.fenceClosed(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := restarted.Closure(ctx, actor, request); err != nil || got.CompletedAt == nil || got.CompletedAt.Before(accepted.AcceptedAt) {
		t.Fatalf("all fences did not complete the original closure: %+v %v", got, err)
	}
	if got, err := restarted.CloseWithReceipt(ctx, actor, fresh.Session.Wallet, 2, request, closureProof(request), approvalKey); err != nil || !got.AcceptedAt.Equal(accepted.AcceptedAt) {
		t.Fatalf("idempotent close changed its receipt: %v", err)
	}
	var audits int
	var retainedKey []byte
	err = pool.QueryRow(ctx, `SELECT w.public_key,
		(SELECT count(*) FROM wallet_mpc.audit_outbox a WHERE a.wallet_id=w.wallet_id AND a.event_kind='closed')
		FROM wallet_mpc.wallet w WHERE w.wallet_id=$1`, fresh.Session.Wallet).Scan(&retainedKey, &audits)
	if err != nil || string(retainedKey) != string(material.PubKey().SerializeUncompressed()) || audits != 1 {
		t.Fatalf("closure evidence changed or duplicated: audits=%d %v", audits, err)
	}

}

type closureFences struct {
	Participants
	missing string
}

func (f *closureFences) Fence(_ context.Context, peer, _ string, _ uint64) error {
	if peer == f.missing {
		return errors.New("synthetic participant has not acknowledged")
	}
	return nil
}
