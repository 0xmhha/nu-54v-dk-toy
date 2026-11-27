package wallet

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/jackc/pgx/v5"
)

type ClosureReceipt struct {
	Request     string     `json:"request"`
	Wallet      string     `json:"wallet"`
	Epoch       uint64     `json:"epoch"`
	AcceptedAt  time.Time  `json:"acceptedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	AssetMoves  int        `json:"assetMoves"`
}

func readClosure(ctx context.Context, tx pgx.Tx, wallet, request string) (ClosureReceipt, error) {
	var out ClosureReceipt
	err := tx.QueryRow(ctx, `SELECT r.request_ref,r.wallet_id,r.epoch,r.accepted_at,CASE WHEN e.retired_at IS NOT NULL THEN greatest(e.retired_at,r.accepted_at) END
		FROM wallet_mpc.closure_receipt r JOIN wallet_mpc.key_epoch e USING(wallet_id,epoch)
		WHERE r.wallet_id=$1 AND ($2='' OR r.request_ref::text=$2)`, wallet, request).
		Scan(&out.Request, &out.Wallet, &out.Epoch, &out.AcceptedAt, &out.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrMissing
	}
	return out, err
}

func (s Store) Closure(ctx context.Context, actor Claims, request string) (ClosureReceipt, error) {
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (ClosureReceipt, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return ClosureReceipt{}, err
		}
		w, err := readWallet(ctx, tx, actor.Subject)
		if err != nil {
			return ClosureReceipt{}, err
		}
		return readClosure(ctx, tx, w.ID, request)
	}, actor)
}

func (s Store) Close(ctx context.Context, actor Claims, walletID string, epoch uint64, proof []byte, key ed25519.PublicKey) error {
	_, err := s.closeAccess(ctx, actor, walletID, epoch, "", proof, key)
	return err
}

func (s Store) CloseWithReceipt(ctx context.Context, actor Claims, walletID string, epoch uint64, request string, proof []byte, key ed25519.PublicKey) (ClosureReceipt, error) {
	if !canonicalUUID(request) {
		return ClosureReceipt{}, ErrConflict
	}
	return s.closeAccess(ctx, actor, walletID, epoch, request, proof, key)
}

func (s Store) closeAccess(ctx context.Context, actor Claims, walletID string, epoch uint64, request string, proof []byte, key ed25519.PublicKey) (ClosureReceipt, error) {
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (ClosureReceipt, error) {
		var receipt ClosureReceipt
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return receipt, err
		}
		w, err := readWallet(ctx, tx, actor.Subject)
		if err != nil {
			return receipt, err
		}
		key, _, err = s.approvalFor(ctx, tx, actor, key, nil)
		if err != nil {
			return receipt, err
		}
		proofHash := sha256.Sum256(AppendClose(w.ID, epoch, request))
		if claimsCurrent(time.Now(), actor) != nil || w.ID != walletID || w.Epoch != epoch || !ed25519.Verify(key, ceremony.ApprovalMessage("close", proofHash), proof) {
			return receipt, ErrForbidden
		}
		if w.State == "closed" {
			if request == "" {
				return receipt, nil
			}
			return readClosure(ctx, tx, w.ID, request)
		}
		if request != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.closure_receipt(wallet_id,request_ref,epoch)
				VALUES($1,$2,$3)`, w.ID, request, epoch); err != nil {
				return receipt, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.wallet SET state='closed' WHERE wallet_id=$1`, w.ID); err != nil {
			return receipt, err
		}
		if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.operation SET state='failed',approval_proof=NULL
			WHERE wallet_id=$1 AND state IN ('reserved','running','held')`, w.ID); err != nil {
			return receipt, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM wallet_mpc.signature_result r USING wallet_mpc.operation o
			WHERE r.operation_id=o.operation_id AND o.wallet_id=$1`, w.ID); err != nil {
			return receipt, err
		}
		if err := audit(ctx, tx, w.ID, "", "closed"); err != nil {
			return receipt, err
		}
		if request != "" {
			return readClosure(ctx, tx, w.ID, request)
		}
		return receipt, nil
	}, actor)
}
