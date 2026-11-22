package wallet

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/jackc/pgx/v5"
)

type SignRefusal struct {
	Subject     string   `json:"subject"`
	Wallet      string   `json:"wallet"`
	Session     string   `json:"session"`
	Fingerprint [32]byte `json:"fingerprint"`
	Epoch       uint64   `json:"epoch"`
}

func (v SignRefusal) Message() []byte {
	payload, _ := json.Marshal(v)
	return append([]byte("nu54/p03/sign-refusal/v1\x00wallet:sign-refusal\x00sign\x00"), payload...)
}

type SignedSignRefusal struct {
	Decision  SignRefusal `json:"decision"`
	Signature []byte      `json:"signature"`
}

func (s Store) RecordSignRefusal(ctx context.Context, signed SignedSignRefusal, legacy ed25519.PublicKey) error {
	v := signed.Decision
	if !canonicalUUID(v.Subject) || !canonicalUUID(v.Wallet) || !canonicalUUID(v.Session) ||
		v.Epoch == 0 || len(signed.Signature) != ed25519.SignatureSize {
		return ErrForbidden
	}
	_, err := withTx(ctx, s.Pool, func(tx pgx.Tx) (bool, error) {
		// Use the same wallet-then-operation lock order as approval and worker completion.
		w, err := readWallet(ctx, tx, v.Subject)
		if errors.Is(err, ErrMissing) || err == nil && w.ID != v.Wallet {
			return false, ErrForbidden
		}
		if err != nil {
			return false, err
		}
		op, err := existingOperation(ctx, tx, w.ID, v.Session)
		if errors.Is(err, ErrMissing) {
			return false, ErrForbidden
		}
		if err != nil {
			return false, err
		}
		if op.Session.Kind != ceremony.Sign || op.Fingerprint != v.Fingerprint || op.Session.Epoch != v.Epoch {
			return false, ErrForbidden
		}
		key := legacy
		if s.Bindings != nil && v.Subject != s.Bindings.LegacySubject {
			profile, err := readBinding(ctx, tx, v.Subject)
			if err != nil {
				return false, ErrForbidden
			}
			key = profile.ApprovalKey
		}
		if len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, v.Message(), signed.Signature) {
			return false, ErrForbidden
		}
		if op.State == "refused" {
			return true, nil
		}
		now := time.Now()
		if op.State != "reserved" || w.State != "active" || w.Epoch != v.Epoch || now.Unix() >= op.Session.Expires {
			return false, ErrConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.operation SET state='refused' WHERE operation_id=$1`, v.Session); err != nil {
			return false, err
		}
		return true, audit(ctx, tx, w.ID, v.Session, "sign_refused")
	})
	return err
}
