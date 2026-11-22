package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/jackc/pgx/v5"
)

type RecoveryRefusal struct {
	Subject     string   `json:"subject"`
	Wallet      string   `json:"wallet"`
	Session     string   `json:"session"`
	Fingerprint [32]byte `json:"fingerprint"`
	Epoch       uint64   `json:"epoch"`
	Vote        string   `json:"vote"`
	IssuedAt    int64    `json:"issuedAt"`
}

func (refusal RecoveryRefusal) Message() []byte {
	payload, _ := json.Marshal(refusal)
	return append([]byte("nu54/p03/recovery-refusal/v1\x00wallet:recovery-refusal\x00refresh\x00"), payload...)
}

type SignedRecoveryRefusal struct {
	Decision  RecoveryRefusal `json:"decision"`
	Signature []byte          `json:"signature"`
}

type RecoveryReceipt struct {
	SignedRecoveryRefusal
	Authority []byte    `json:"authority"`
	Recorded  time.Time `json:"recordedAt"`
}

type RecoveryHistoryItem struct {
	RequestID string    `json:"requestId"`
	Recorded  time.Time `json:"recordedAt"`
}

func (s Store) RecordRecoveryRefusal(ctx context.Context, signed SignedRecoveryRefusal, legacy ed25519.PublicKey) error {
	v := signed.Decision
	if !canonicalUUID(v.Subject) || !canonicalUUID(v.Wallet) || !canonicalUUID(v.Session) || !canonicalUUID(v.Vote) ||
		v.Epoch <= 1 || v.IssuedAt <= 0 || len(signed.Signature) != ed25519.SignatureSize {
		return ErrForbidden
	}
	_, err := withTx(ctx, s.Pool, func(tx pgx.Tx) (bool, error) {
		w, err := readWallet(ctx, tx, v.Subject)
		if err != nil || w.ID != v.Wallet {
			return false, ErrForbidden
		}
		op, err := existingOperation(ctx, tx, w.ID, v.Session)
		if err != nil || op.Session.Kind != ceremony.Refresh || op.Fingerprint != v.Fingerprint || op.Session.Epoch != v.Epoch {
			return false, ErrForbidden
		}
		key := legacy
		if s.Bindings != nil && v.Subject != s.Bindings.LegacySubject {
			profile, err := readBinding(ctx, tx, v.Subject)
			if err != nil {
				return false, ErrForbidden
			}
			key = profile.RecoveryKey
		}
		if len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, v.Message(), signed.Signature) {
			return false, ErrForbidden
		}
		existing, err := recoveryReceipt(ctx, tx, op.Session.ID)
		if err == nil {
			if existing.Decision == v && bytes.Equal(existing.Signature, signed.Signature) {
				return true, nil
			}
			return false, ErrConflict
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return false, err
		}
		var created time.Time
		if err := tx.QueryRow(ctx, `SELECT created_at FROM wallet_mpc.operation WHERE operation_id=$1`, v.Session).Scan(&created); err != nil {
			return false, err
		}
		now := time.Now().Unix()
		if w.State != "blocked" || w.Epoch+1 != v.Epoch || (op.State != "reserved" && op.State != "running") ||
			now >= op.Session.Expires || v.IssuedAt > now || now-v.IssuedAt > 90 || v.IssuedAt < created.Unix() {
			return false, ErrConflict
		}
		out, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.recovery_refusal(operation_id,vote_id,decision,signature,authority_key)
			VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, v.Session, v.Vote, v, signed.Signature, []byte(key))
		if err != nil {
			return false, err
		}
		if out.RowsAffected() != 1 {
			return false, ErrConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.operation SET state='denied',approval_proof=NULL WHERE operation_id=$1`, v.Session); err != nil {
			return false, err
		}
		return true, audit(ctx, tx, w.ID, v.Session, "recovery_denied")
	})
	return err
}

func recoveryReceipt(ctx context.Context, tx pgx.Tx, session string) (RecoveryReceipt, error) {
	var r RecoveryReceipt
	err := tx.QueryRow(ctx, `SELECT decision,signature,authority_key,recorded_at FROM wallet_mpc.recovery_refusal
		WHERE operation_id=$1`, session).Scan(&r.Decision, &r.Signature, &r.Authority, &r.Recorded)
	return r, err
}

func (s Store) PermitRecoveryReview(ctx context.Context, subject, request string, operator Claims) error {
	_, err := withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (bool, error) {
		if !canonicalUUID(operator.Subject) || operator.Subject == subject {
			return false, ErrForbidden
		}
		w, err := readWallet(ctx, tx, subject)
		if err != nil {
			return false, err
		}
		op, err := requestOperation(ctx, tx, w.ID, request)
		if err != nil {
			return false, err
		}
		if err := requireReviewable(ctx, tx, w, op); err != nil {
			return false, err
		}
		out, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.recovery_review(operation_id,operator_ref)
			VALUES ($1,$2) ON CONFLICT DO NOTHING`, op.Session.ID, operator.Subject)
		if err != nil || out.RowsAffected() == 0 {
			return false, err
		}
		return true, audit(ctx, tx, w.ID, op.Session.ID, "recovery_review_permitted")
	}, operator)
	return err
}

func ordinaryRecoveryRetryAvailable(ctx context.Context, tx pgx.Tx, wallet string) (bool, error) {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT coalesce((
		SELECT o.state='held' AND NOT EXISTS (
			SELECT 1 FROM wallet_mpc.recovery_review r WHERE r.successor_operation=o.operation_id)
		FROM wallet_mpc.operation o WHERE o.wallet_id=$1 AND o.kind='refresh'
		ORDER BY o.created_at DESC,o.operation_id DESC LIMIT 1),false)
		AND NOT EXISTS (SELECT 1 FROM wallet_mpc.audit_outbox WHERE wallet_id=$1 AND event_kind='incident_hold')`, wallet).Scan(&allowed)
	return allowed, err
}

func requireReviewable(ctx context.Context, tx pgx.Tx, w Wallet, op Operation) error {
	if w.State != "blocked" || w.Epoch+1 != op.Session.Epoch || op.State != "denied" {
		return ErrConflict
	}
	exposed, err := quorumExposure(ctx, tx, w.ID)
	if err != nil {
		return err
	}
	if exposed {
		return ErrConflict
	}
	latest, err := latestRefresh(ctx, tx, w.ID)
	if err == nil && latest != op.Session.ID {
		return ErrConflict
	}
	return err
}

func latestRefresh(ctx context.Context, tx pgx.Tx, wallet string) (string, error) {
	var session string
	err := tx.QueryRow(ctx, `SELECT operation_id FROM wallet_mpc.operation WHERE wallet_id=$1 AND kind='refresh'
		ORDER BY created_at DESC,operation_id DESC LIMIT 1`, wallet).Scan(&session)
	return session, err
}

func (s Store) ReviewRecovery(ctx context.Context, actor Claims, original, successor string) (Operation, error) {
	if !canonicalUUID(original) || !canonicalUUID(successor) || original == successor {
		return Operation{}, ErrConflict
	}
	return s.reserve(ctx, actor, successor, ceremony.Refresh, nil, original)
}

func checkPriorReview(ctx context.Context, tx pgx.Tx, w Wallet, request string) (string, *string, error) {
	op, err := requestOperation(ctx, tx, w.ID, request)
	if err != nil {
		return "", nil, err
	}
	if op.State != "denied" {
		return "", nil, ErrConflict
	}
	var successor *string
	err = tx.QueryRow(ctx, `SELECT successor_operation FROM wallet_mpc.recovery_review WHERE operation_id=$1 FOR UPDATE`, op.Session.ID).Scan(&successor)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrForbidden
	}
	if err != nil {
		return "", nil, err
	}
	if successor == nil {
		if err := requireReviewable(ctx, tx, w, op); err != nil {
			return "", nil, err
		}
	}
	return op.Session.ID, successor, nil
}

func (s Store) RecoveryHistory(ctx context.Context, actor Claims) ([]RecoveryHistoryItem, error) {
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) ([]RecoveryHistoryItem, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, `SELECT o.request_ref::text,r.recorded_at
			FROM wallet_mpc.recovery_refusal r JOIN wallet_mpc.operation o USING(operation_id)
			JOIN wallet_mpc.wallet w USING(wallet_id) WHERE w.subject_ref=$1
			ORDER BY r.recorded_at DESC,r.operation_id DESC LIMIT 20`, actor.Subject)
		if err != nil {
			return nil, err
		}
		history, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (RecoveryHistoryItem, error) {
			var item RecoveryHistoryItem
			err := row.Scan(&item.RequestID, &item.Recorded)
			return item, err
		})
		if history == nil {
			history = []RecoveryHistoryItem{}
		}
		return history, err
	}, actor)
}

func attachRecoveryDisposition(ctx context.Context, tx pgx.Tx, w Wallet, op *Operation) error {
	if op.State != "denied" {
		return nil
	}
	receipt, err := recoveryReceipt(ctx, tx, op.Session.ID)
	if err != nil {
		return err
	}
	op.Refusal = &receipt
	err = tx.QueryRow(ctx, `SELECT r.successor_operation IS NULL,coalesce(o.request_ref::text,'')
		FROM wallet_mpc.recovery_review r LEFT JOIN wallet_mpc.operation o ON o.operation_id=r.successor_operation
		WHERE r.operation_id=$1`, op.Session.ID).Scan(&op.ReviewAvailable, &op.SuccessorRequest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	op.ReviewAvailable = op.ReviewAvailable && w.State == "blocked" && op.Session.Epoch == w.Epoch+1
	if op.ReviewAvailable {
		exposed, err := quorumExposure(ctx, tx, w.ID)
		op.ReviewAvailable = !exposed && err == nil
		return err
	}
	return nil
}
