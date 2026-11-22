package chainview

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strconv"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/jackc/pgx/v5"
)

var ErrDeploymentIdentity = errors.New("deployment identity cannot be verified")
var ErrProfileApproval = errors.New("held deployment requires signed approval")

// VerifyProfileApproval binds the exact normalized profile to the deployment
// authority without storing a signature or a key in the observation database.
func VerifyProfileApproval(profile Profile, key ed25519.PublicKey, signature []byte) bool {
	return deployment.Verify(deployment.Profile(profile), key, signature)
}

func lockProfile(ctx context.Context, tx pgx.Tx, profile Profile) error {
	_, err := tx.Exec(ctx, "SELECT pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended($1,52))",
		profile.StreamID+profile.GenerationID)
	return err
}

func (r *Repository) HoldProfile(ctx context.Context, profile Profile, reason string) error {
	if !uuidPattern.MatchString(profile.StreamID) || !uuidPattern.MatchString(profile.GenerationID) {
		return errors.New("invalid profile scope")
	}
	var abi *string
	if hashPattern.MatchString(profile.ABIHash) {
		abi = &profile.ABIHash
	}
	var base *int64
	if profile.StartHeight >= 0 {
		base = &profile.StartHeight
	}
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockProfile(ctx, tx, profile); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO chainview.profile_hold(stream_id,generation_id,chain_ref,contract_ref,version,asset_ref,abi_hash,base_height,reason)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT(stream_id,generation_id) DO UPDATE SET chain_ref=excluded.chain_ref,contract_ref=excluded.contract_ref,
		  version=excluded.version,asset_ref=excluded.asset_ref,abi_hash=excluded.abi_hash,base_height=excluded.base_height,
		  reason=excluded.reason,observed_at=clock_timestamp()`, profile.StreamID, profile.GenerationID,
			strconv.FormatUint(profile.ChainID, 10), lower(profile.Contract), profile.Version,
			lower(profile.Asset), abi, base, reason)
		return err
	})
}
