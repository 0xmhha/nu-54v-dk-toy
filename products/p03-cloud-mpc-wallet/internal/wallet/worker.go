package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/jackc/pgx/v5"
)

type Participants interface {
	Status(context.Context, string, string) (ceremony.Result, error)
	Start(context.Context, string, ceremony.Authorization) (ceremony.Result, error)
	Fence(context.Context, string, string, uint64) error
}

type Worker struct {
	Store      Store
	Control    Participants
	SigningKey ed25519.PrivateKey
}

type observation struct {
	results map[string]ceremony.Result
	fenced  map[string]bool
}

func (w Worker) Run(ctx context.Context) error {
	if len(w.SigningKey) != ed25519.PrivateKeySize || w.Control == nil {
		return errors.New("worker configuration is incomplete")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := w.Step(ctx); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w Worker) Step(ctx context.Context) error {
	rows, err := w.Store.Pool.Query(ctx, `SELECT `+operationColumns+` FROM wallet_mpc.operation o
	WHERE o.state='running' OR o.state='reserved' AND (o.kind='refresh' OR o.expires_at<=clock_timestamp())
	ORDER BY o.created_at LIMIT 64`)
	if err != nil {
		return err
	}
	var pending []Operation
	for rows.Next() {
		op, err := readOperation(rows)
		if err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, op)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, op := range pending {
		report := w.collect(ctx, op)
		if err := w.Store.apply(ctx, op, report); err != nil {
			return err
		}
	}
	_, err = w.Store.Pool.Exec(ctx, `DELETE FROM wallet_mpc.signature_result WHERE expires_at<=clock_timestamp()`)
	if err != nil {
		return err
	}
	_, err = w.Store.Pool.Exec(ctx, `UPDATE wallet_mpc.operation SET intent=NULL WHERE intent IS NOT NULL AND expires_at+interval '24 hours'<=clock_timestamp()`)
	if err != nil {
		return err
	}
	return w.fenceClosed(ctx)
}

func (w Worker) collect(ctx context.Context, op Operation) observation {
	s := op.Session
	report := observation{results: make(map[string]ceremony.Result), fenced: make(map[string]bool)}
	if s.Kind == ceremony.Refresh {
		for _, peer := range s.Peers {
			if peer.Epoch == s.Epoch-1 {
				report.fenced[peer.ID] = w.Control.Fence(ctx, peer.ID, s.Wallet, peer.Epoch) == nil
			}
		}
	}
	if op.State != "running" || len(op.proof) != ed25519.SignatureSize || time.Now().Unix() >= s.Expires {
		return report
	}
	for _, ok := range report.fenced {
		if !ok {
			return report
		}
	}
	message := ceremony.ApprovalMessage(s.Kind, op.Fingerprint)
	auth := ceremony.Authorization{Session: s, Coordinator: ed25519.Sign(w.SigningKey, message), Independent: op.proof}
	for _, peer := range s.Peers {
		r, err := w.Control.Status(ctx, peer.ID, s.ID)
		if errors.Is(err, os.ErrNotExist) {
			r, err = w.Control.Start(ctx, peer.ID, auth)
		}
		if err == nil && r.Peer == peer.ID && r.Session == op.Fingerprint {
			report.results[peer.ID] = r
		}
	}
	return report
}

func (s Store) apply(ctx context.Context, candidate Operation, observed observation) error {
	_, err := withTx(ctx, s.Pool, func(tx pgx.Tx) (bool, error) {
		var wallet Wallet
		err := tx.QueryRow(ctx, `SELECT wallet_id,state,coalesce(active_epoch,0),public_key FROM wallet_mpc.wallet
			WHERE wallet_id=$1 FOR UPDATE`, candidate.Session.Wallet).Scan(&wallet.ID, &wallet.State, &wallet.Epoch, &wallet.PublicKey)
		if err != nil {
			return false, err
		}
		op, err := existingOperation(ctx, tx, wallet.ID, candidate.Session.ID)
		if err != nil {
			return false, err
		}
		if candidate.Fingerprint != op.Fingerprint || (op.State != "reserved" && op.State != "running") {
			return false, nil
		}
		for peer, ok := range observed.fenced {
			if !ok {
				continue
			}
			proof := sha256.Sum256(append(append([]byte("nu54/p03/fence/v1/"+peer+"\x00"), op.Fingerprint[:]...), byte(wallet.Epoch)))
			if err := addEvidence(ctx, tx, op.Session.ID, peer, "fenced", proof); err != nil {
				return false, err
			}
		}
		if time.Now().Unix() >= op.Session.Expires {
			return false, s.finishFailure(ctx, tx, wallet, op, "expired")
		}
		if op.State != "running" {
			return false, nil
		}
		full := true
		var result ceremony.Result
		for _, peer := range op.Session.Peers {
			got, ok := observed.results[peer.ID]
			ok = ok && got.Session == op.Fingerprint && got.Peer == peer.ID
			if ok && got.State == "interrupted" {
				return false, s.finishFailure(ctx, tx, wallet, op, "failed")
			}
			expected := "ready"
			if op.Session.Kind == ceremony.Sign {
				expected = "signed"
			}
			if op.Session.Kind == ceremony.Refresh && peer.Epoch < op.Session.Epoch {
				expected = "retired"
			}
			if !ok || got.State != expected {
				full = false
				continue
			}
			if op.Session.Kind == ceremony.Sign {
				if len(got.RecoveryID) != 1 || !ceremony.VerifySignature(wallet.PublicKey, op.Session.Digest, got.Signature) ||
					!matchesRecovery(wallet.PublicKey, op.Session.Digest, got.Signature, got.RecoveryID[0]) {
					return false, s.finishFailure(ctx, tx, wallet, op, "failed")
				}
			} else {
				if len(got.PublicKey) != 65 {
					return false, s.finishFailure(ctx, tx, wallet, op, "failed")
				}
				if _, err := secp256k1.ParsePubKey(got.PublicKey); err != nil {
					return false, s.finishFailure(ctx, tx, wallet, op, "failed")
				}
			}
			if result.Peer != "" && (!bytes.Equal(result.PublicKey, got.PublicKey) || !bytes.Equal(result.Signature, got.Signature) ||
				!bytes.Equal(result.RecoveryID, got.RecoveryID)) {
				return false, s.finishFailure(ctx, tx, wallet, op, "failed")
			}
			result = got
			raw, err := json.Marshal(got)
			if err != nil {
				return false, err
			}
			if err := addEvidence(ctx, tx, op.Session.ID, peer.ID, expected, sha256.Sum256(raw)); err != nil {
				return false, err
			}
		}
		if !full {
			return false, nil
		}
		if op.Session.Kind == ceremony.Refresh {
			for _, peer := range op.Session.Peers {
				if peer.Epoch < op.Session.Epoch && !observed.fenced[peer.ID] {
					return false, nil
				}
			}
		}
		return true, s.finishSuccess(ctx, tx, wallet, op, result)
	})
	return err
}

func addEvidence(ctx context.Context, tx pgx.Tx, id, peer, kind string, proof [32]byte) error {
	tag, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.participant_ack AS a (operation_id,participant_ref,evidence_kind,evidence_digest)
		VALUES ($1,$2,$3,$4) ON CONFLICT (operation_id,participant_ref,evidence_kind)
		DO UPDATE SET evidence_digest=excluded.evidence_digest WHERE a.evidence_digest=excluded.evidence_digest`, id, peer, kind, proof[:])
	if err == nil && tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return err
}

func (s Store) finishFailure(ctx context.Context, tx pgx.Tx, w Wallet, op Operation, state string) error {
	if op.Session.Kind == ceremony.Refresh {
		state = "held"
	}
	_, err := tx.Exec(ctx, `UPDATE wallet_mpc.operation SET state=$2,approval_proof=NULL WHERE operation_id=$1`, op.Session.ID, state)
	if err != nil {
		return err
	}
	if op.Session.Kind == ceremony.DKG && w.State == "creating" {
		_, err = tx.Exec(ctx, `UPDATE wallet_mpc.wallet SET state='failed' WHERE wallet_id=$1`, w.ID)
	}
	if op.Session.Kind == ceremony.Refresh {
		err = audit(ctx, tx, w.ID, op.Session.ID, "recovery_held")
	}
	return err
}

func (s Store) finishSuccess(ctx context.Context, tx pgx.Tx, w Wallet, op Operation, result ceremony.Result) error {
	id := op.Session.ID
	var event string
	var digest []byte
	switch op.Session.Kind {
	case ceremony.Sign:
		if w.State != "active" || w.Epoch != op.Session.Epoch {
			return s.finishFailure(ctx, tx, w, op, "failed")
		}
		h := sha256.Sum256(result.Signature)
		digest = h[:]
		if _, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.signature_result(operation_id,signature,recovery_id,expires_at)
			VALUES($1,$2,$3,$4)`, id, result.Signature, int16(result.RecoveryID[0]), time.Unix(op.Session.Expires, 0).Add(ceremony.ResultTTL)); err != nil {
			return err
		}
		event = "sign_finished"
	case ceremony.DKG, ceremony.Refresh:
		if op.Session.Kind == ceremony.DKG {
			if w.State != "creating" {
				return s.finishFailure(ctx, tx, w, op, "failed")
			}
			event = "created"
		} else {
			if w.State != "blocked" || w.Epoch+1 != op.Session.Epoch || !bytes.Equal(w.PublicKey, result.PublicKey) {
				return s.finishFailure(ctx, tx, w, op, "held")
			}
			tag, err := tx.Exec(ctx, `UPDATE wallet_mpc.key_epoch SET retired_at=clock_timestamp()
				WHERE wallet_id=$1 AND epoch=$2 AND retired_at IS NULL`, w.ID, w.Epoch)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrConflict
			}
			event = "epoch_activated"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.key_epoch(wallet_id,epoch,operation_id,activated_at)
			VALUES($1,$2,$3,clock_timestamp())`, w.ID, op.Session.Epoch, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.wallet SET active_epoch=$2,public_key=$3,state='active' WHERE wallet_id=$1`, w.ID, op.Session.Epoch, result.PublicKey); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.operation SET state='succeeded',approval_proof=NULL,result_digest=$2 WHERE operation_id=$1`, id, digest); err != nil {
		return err
	}
	return audit(ctx, tx, w.ID, id, event)
}

func (w Worker) fenceClosed(ctx context.Context) error {
	rows, err := w.Store.Pool.Query(ctx, `SELECT k.wallet_id,k.epoch,k.operation_id
	    FROM wallet_mpc.key_epoch k JOIN wallet_mpc.wallet v ON v.wallet_id=k.wallet_id AND v.active_epoch=k.epoch
		WHERE (v.state='closed' OR v.state='blocked' AND EXISTS (
		    SELECT 1 FROM wallet_mpc.audit_outbox a WHERE a.wallet_id=v.wallet_id AND a.event_kind='incident_hold'))
		AND k.retired_at IS NULL ORDER BY v.created_at LIMIT 32`)
	if err != nil {
		return err
	}
	type target struct {
		wallet, id string
		epoch      uint64
	}
	var pending []target
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.wallet, &item.epoch, &item.id); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range pending {
		var op Operation
		op, err = readOperation(w.Store.Pool.QueryRow(ctx, `SELECT `+operationColumns+` FROM wallet_mpc.operation o WHERE operation_id=$1`, item.id))
		if err != nil {
			return err
		}
		complete := true
		for _, peer := range op.Session.Peers {
			if peer.Epoch == item.epoch && w.Control.Fence(ctx, peer.ID, item.wallet, item.epoch) != nil {
				complete = false
			}
		}
		if complete {
			_, err = w.Store.Pool.Exec(ctx, `UPDATE wallet_mpc.key_epoch SET retired_at=clock_timestamp()
				WHERE wallet_id=$1 AND epoch=$2 AND retired_at IS NULL`, item.wallet, item.epoch)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
