package chainview

import (
	"context"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/jackc/pgx/v5"
)

// ConfirmStoredGenerationAnchor checks only ancestry and deployment identity.
// The caller must separately authenticate and interpret the stored review.
func (r *Reader) ConfirmStoredGenerationAnchor(ctx context.Context, previous rentalv1.Anchor) (rentalv1.Anchor, error) {
	profile := r.Profile
	source, ok := r.Source.(generationStateSource)
	if !ok || profile.Version != 1 || r.DB == nil || !previous.Valid() || previous.Height < profile.StartHeight {
		return rentalv1.Anchor{}, errGenerationUnresolved
	}
	current, err := generationAnchor(ctx, r.DB.db, profile)
	if err != nil || previous.Height > current.Height {
		return rentalv1.Anchor{}, errGenerationUnresolved
	}
	err = pgx.BeginFunc(ctx, r.DB.db, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT chainview.lock_absence_anchor($1,$2,$3,$4)`,
			profile.StreamID, profile.GenerationID, current.Height, current.Hash.Hex()).Scan(&locked); err != nil || !locked {
			return errGenerationUnresolved
		}
		check, err := generationAnchor(ctx, tx, profile)
		if err != nil || check.Height != current.Height || check.Hash != current.Hash || !check.Time.Equal(current.Time) {
			return errGenerationUnresolved
		}
		var canonical bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM chainview.block
			WHERE generation_id=$1 AND height=$2 AND block_hash=$3 AND canonical)`,
			profile.GenerationID, previous.Height, previous.Hash).Scan(&canonical); err != nil || !canonical {
			return errGenerationUnresolved
		}
		origin, err := source.Header(ctx, previous.Height)
		if err != nil || origin.Hash.Hex() != previous.Hash {
			return errGenerationUnresolved
		}
		_, latest, err := source.Heads(ctx)
		if err != nil || latest.Height != current.Height || latest.Hash != current.Hash ||
			verifyAnchor(ctx, source, current) != nil || !source.generationDeploymentAt(ctx, profile, current) {
			return errGenerationUnresolved
		}
		return nil
	})
	if err != nil {
		return rentalv1.Anchor{}, err
	}
	return rentalv1.Anchor{Height: current.Height, Hash: current.Hash.Hex()}, nil
}
