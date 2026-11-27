package chainview

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

const absenceTTL = 15 * time.Minute

type absenceQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func checkedCoverage(ctx context.Context, db absenceQuery, profile Profile, merchant, order string,
	expiry time.Time) (Header, error) {
	var anchor Header
	var hash string
	var continuous, locallyPaid bool
	err := db.QueryRow(ctx, `SELECT g.finalized_height,g.finalized_hash,anchor.block_time,
		COALESCE(chainview.complete_prefix(g.id,g.finalized_height),false),
		EXISTS (SELECT FROM chainview.observed_event e JOIN chainview.block b
		    ON b.generation_id=e.generation_id AND b.block_hash=e.block_hash
		    WHERE e.generation_id=g.id AND b.canonical AND b.height<=g.finalized_height
		        AND e.kind IN ('paid','locked') AND e.merchant_key=$3 AND e.order_key=$4)
		FROM chainview.generation g JOIN chainview.read_target t ON t.generation_id=g.id
		JOIN chainview.block anchor ON anchor.generation_id=g.id AND anchor.block_hash=g.finalized_hash
		  AND anchor.height=g.finalized_height AND anchor.canonical
		WHERE g.id=$1 AND t.stream_id=$2 AND g.state='ready'
		AND NOT EXISTS (SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
		    WHERE affected.stream_id=t.stream_id AND i.resolved_at IS NULL)`,
		profile.GenerationID, profile.StreamID, merchant, order).
		Scan(&anchor.Height, &hash, &anchor.Time, &continuous, &locallyPaid)
	if err != nil {
		return Header{}, fmt.Errorf("active finalized checkpoint not available: %w", err)
	}
	if !continuous || locallyPaid || anchor.Time.Before(expiry) {
		return Header{}, errors.New("chain coverage, expiry, or unconsumed order check failed")
	}
	anchor.Hash = common.HexToHash(hash)
	return anchor, nil
}

func (r *Reader) ProveAbsence(ctx context.Context, merchant, order string, expiry time.Time) (string, error) {
	if !validOrderKeys(merchant, order) || (r.Profile.Version == 1 && len(merchant) != 42) || (r.Profile.Version >= 2 && len(merchant) != 66) ||
		expiry.Unix() <= 0 || expiry.Nanosecond() != 0 {
		return "", errors.New("invalid scoped order reference or expiry")
	}
	anchor, err := checkedCoverage(ctx, r.DB.db, r.Profile, merchant, order, expiry)
	if err != nil {
		return "", err
	}
	_, latestFinal, err := r.Source.Heads(ctx)
	if err != nil || latestFinal.Height < anchor.Height {
		return "", errors.New("RPC finalized checkpoint unavailable or regressed")
	}
	if err := verifyAnchor(ctx, r.Source, anchor); err != nil {
		return "", err
	}
	paid, err := r.Source.Paid(ctx, merchant, order, anchor)
	if err != nil {
		return "", err
	}
	if paid {
		return "", errors.New("contract reports order consumed")
	}
	if err := verifyAnchor(ctx, r.Source, anchor); err != nil {
		return "", err
	}
	proof := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(fmt.Sprintf("nu54-absence-v2\n%s\n%s\n%s\n%s\n%s\n%d\nfalse",
		r.Profile.Manifest, r.Profile.StreamID, merchant, order, anchor.Hash.Hex(), expiry.Unix()))))
	var id string
	err = pgx.BeginFunc(ctx, r.DB.db, func(tx pgx.Tx) error {
		// This locks the read target and generation against a concurrent switch or freeze.
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT COALESCE(chainview.lock_absence_anchor($1,$2,$3,$4),false)`,
			r.Profile.StreamID, r.Profile.GenerationID, anchor.Height, anchor.Hash.Hex()).Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return errors.New("active checkpoint changed before proof commit")
		}
		current, err := checkedCoverage(ctx, tx, r.Profile, merchant, order, expiry)
		if err != nil {
			return err
		}
		if current.Hash != anchor.Hash || current.Height != anchor.Height {
			return errors.New("active checkpoint advanced before proof commit")
		}
		return tx.QueryRow(ctx, `INSERT INTO chainview.evidence(id,generation_id,merchant_key,order_key,kind,
			anchor_height,anchor_hash,policy_ref,expires_at,coverage_start,state_proof_ref,created_at)
			VALUES(gen_random_uuid(),$1,$2,$3,'absent',$4,$5,$6,statement_timestamp()+make_interval(secs=>$9),$7,$8,statement_timestamp())
			RETURNING id::text`, r.Profile.GenerationID, merchant, order, anchor.Height, anchor.Hash.Hex(),
			r.Profile.Manifest, r.Profile.StartHeight, proof, absenceTTL.Seconds()).Scan(&id)
	})
	return id, err
}

func verifyAnchor(ctx context.Context, source Source, anchor Header) error {
	canonical, err := source.Header(ctx, anchor.Height)
	if err != nil {
		return err
	}
	if canonical.Hash != anchor.Hash || !canonical.Time.Equal(anchor.Time) {
		return ErrFinalizedConflict
	}
	return nil
}

func validOrderKeys(merchant, order string) bool {
	return (hexKey(merchant, 40) || hexKey(merchant, 64)) && hexKey(order, 64)
}

func hexKey(value string, width int) bool {
	if len(value) != width+2 || !strings.HasPrefix(value, "0x") {
		return false
	}
	var nonzero bool
	for _, char := range value[2:] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
		if char != '0' {
			nonzero = true
		}
	}
	return nonzero
}
