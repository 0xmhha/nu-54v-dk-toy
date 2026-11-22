package chainview

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLeaseUnavailable = errors.New("chain generation is frozen, fenced or leased")

type Repository struct{ db *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Repository, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	var allowed bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user, 'nu54_p07_ingest', 'USAGE')
		AND NOT pg_has_role(current_user, 'nu54_p07_switch', 'MEMBER')
		AND NOT pg_has_role(current_user, 'nu54_p07_legacy', 'MEMBER')
		AND NOT has_schema_privilege('chainview', 'CREATE')
		AND has_table_privilege('chainview.deployment_profile','INSERT')`).Scan(&allowed)
	if err != nil || !allowed {
		db.Close()
		return nil, errors.New("chainview requires a migrated database and restricted ingest login")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Close() { r.db.Close() }

func (r *Repository) Register(ctx context.Context, profile Profile) error {
	return r.register(ctx, profile, false)
}

func (r *Repository) RegisterSigned(ctx context.Context, profile Profile, registered Profile, key []byte, sig []byte) error {
	if !VerifyProfileApproval(registered, key, sig) {
		return ErrProfileApproval
	}
	// Only token units may have been filled in by the canonical RPC check.
	pinned := profile
	pinned.AssetDecimals = registered.AssetDecimals
	if pinned != registered || (registered.AssetDecimals != nil &&
		(profile.AssetDecimals == nil || *profile.AssetDecimals != *registered.AssetDecimals)) {
		return ErrProfileApproval
	}
	return r.register(ctx, profile, true)
}

func (r *Repository) register(ctx context.Context, profile Profile, approved bool) error {
	if err := profile.validate(); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockProfile(ctx, tx, profile); err != nil {
			return err
		}
		var held, isolated bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM chainview.profile_hold WHERE stream_id=$1 AND generation_id=$2),
			EXISTS(SELECT FROM chainview.replay_job j WHERE j.id=$2 AND NOT (
			 j.state='matched' AND EXISTS(SELECT FROM chainview.read_target t
			   WHERE t.stream_id=j.stream_id AND t.generation_id=j.id)))`, profile.StreamID, profile.GenerationID).
			Scan(&held, &isolated); err != nil {
			return err
		}
		if isolated {
			return errors.New("isolated replay candidates are not live producer profiles")
		}
		if held && !approved {
			return ErrProfileApproval
		}
		chain := strconv.FormatUint(profile.ChainID, 10)
		_, err := tx.Exec(ctx, `INSERT INTO chainview.stream VALUES($1,$2,$3,$4,$5) ON CONFLICT(id) DO NOTHING`,
			profile.StreamID, chain, lower(profile.Contract), profile.ABIHash, profile.Manifest)
		if err != nil {
			return err
		}
		var units *int16
		if profile.AssetDecimals != nil {
			value := int16(*profile.AssetDecimals)
			units = &value
		}
		_, err = tx.Exec(ctx, `INSERT INTO chainview.deployment_profile(stream_id,version,asset_ref,code_hash,start_height,start_hash,asset_decimals)
			VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(stream_id) DO UPDATE SET asset_decimals=EXCLUDED.asset_decimals
			WHERE chainview.deployment_profile.asset_decimals IS NULL AND EXCLUDED.asset_decimals IS NOT NULL
			  AND chainview.deployment_profile.version=EXCLUDED.version AND chainview.deployment_profile.asset_ref=EXCLUDED.asset_ref
			  AND chainview.deployment_profile.code_hash=EXCLUDED.code_hash AND chainview.deployment_profile.start_height=EXCLUDED.start_height
			  AND chainview.deployment_profile.start_hash=EXCLUDED.start_hash`, profile.StreamID, profile.Version,
			lower(profile.Asset), profile.CodeHash.Hex(), profile.StartHeight, profile.StartHash.Hex(), units)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO chainview.generation(id,stream_id,base_height,fence,state)
			VALUES($1,$2,$3,0,'building') ON CONFLICT(id) DO NOTHING`, profile.GenerationID, profile.StreamID, profile.StartHeight)
		if err != nil {
			return err
		}
		var exact bool
		err = tx.QueryRow(ctx, `SELECT s.chain_ref=$3 AND s.contract_ref=$4 AND s.abi_hash=$5 AND s.manifest_ref=$6
			AND p.version=$7 AND p.asset_ref=$8 AND p.code_hash=$9 AND p.start_height=$10 AND p.start_hash=$11
			AND g.base_height=p.start_height AND ($12::smallint IS NULL OR p.asset_decimals=$12)
			FROM chainview.generation g JOIN chainview.stream s ON s.id=g.stream_id
			JOIN chainview.deployment_profile p ON p.stream_id=s.id WHERE g.id=$1 AND s.id=$2`,
			profile.GenerationID, profile.StreamID, chain, lower(profile.Contract), profile.ABIHash, profile.Manifest,
			profile.Version, lower(profile.Asset), profile.CodeHash.Hex(), profile.StartHeight, profile.StartHash.Hex(), units).Scan(&exact)
		if err != nil || !exact {
			return errors.New("persisted deployment profile differs")
		}
		if held {
			_, err = tx.Exec(ctx, "DELETE FROM chainview.profile_hold WHERE stream_id=$1 AND generation_id=$2",
				profile.StreamID, profile.GenerationID)
			return err
		}
		return nil
	})
}

type lease struct {
	generation     string
	fence          int64
	tip, finalized *int64
	finalizedHash  *string
}

func (r *Repository) claim(ctx context.Context, generation string) (lease, error) {
	result := lease{generation: generation}
	err := r.db.QueryRow(ctx, `UPDATE chainview.generation
		SET fence=fence+1,lease_until=clock_timestamp()+interval '30 seconds'
		WHERE id=$1 AND state<>'frozen' AND (lease_until IS NULL OR lease_until<=clock_timestamp())
		RETURNING fence,tip_height,finalized_height,finalized_hash`, generation).
		Scan(&result.fence, &result.tip, &result.finalized, &result.finalizedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return lease{}, ErrLeaseUnavailable
	}
	return result, err
}

func (r *Repository) release(ctx context.Context, l lease) {
	_, _ = r.db.Exec(ctx, `UPDATE chainview.generation SET lease_until=NULL WHERE id=$1 AND fence=$2`, l.generation, l.fence)
}

func (r *Repository) canonical(ctx context.Context, generation string, height int64) (common.Hash, error) {
	var hash string
	err := r.db.QueryRow(ctx, `SELECT block_hash FROM chainview.block
		WHERE generation_id=$1 AND height=$2 AND canonical`, generation, height).Scan(&hash)
	return common.HexToHash(hash), err
}

func lockLease(ctx context.Context, tx pgx.Tx, l lease) error {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT fence=$2 AND state<>'frozen' AND COALESCE(lease_until>clock_timestamp(),false)
		FROM chainview.generation WHERE id=$1 FOR UPDATE`, l.generation, l.fence).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrLeaseUnavailable
	}
	return nil
}

func (r *Repository) freeze(ctx context.Context, l lease, height int64, oldHash, newHash string) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		// Hold the entire stream against switches and new absence proofs before locking a generation.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(stream_id::text,9))
			FROM chainview.generation WHERE id=$1`, l.generation); err != nil {
			return err
		}
		if err := lockLease(ctx, tx, l); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO chainview.incident(id,generation_id,disputed_height,old_hash,new_hash,opened_at)
			VALUES(gen_random_uuid(),$1,$2,$3,$4,clock_timestamp())`, l.generation, height, oldHash, newHash)
		if err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE chainview.generation SET state='frozen'
			WHERE id=$1 AND fence=$2 AND lease_until>clock_timestamp()`, l.generation, l.fence)
		if err == nil && result.RowsAffected() != 1 {
			return ErrLeaseUnavailable
		}
		return err
	})
}

func (r *Repository) observe(ctx context.Context, l lease, finalized int64) error {
	result, err := r.db.Exec(ctx, `UPDATE chainview.generation
		SET observed_at=clock_timestamp(),observed_finalized_height=$3
		WHERE id=$1 AND fence=$2 AND lease_until>clock_timestamp() AND state<>'frozen'`, l.generation, l.fence, finalized)
	if err == nil && result.RowsAffected() != 1 {
		return ErrLeaseUnavailable
	}
	return err
}

func (r *Repository) advance(ctx context.Context, l lease, ancestor int64, blocks []Block, final *Header, policy string, observedFinal int64) error {
	tip := ancestor
	if len(blocks) != 0 {
		tip = blocks[len(blocks)-1].Height
	}
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockLease(ctx, tx, l); err != nil {
			return err
		}
		if l.finalized != nil && ancestor < *l.finalized {
			return ErrFinalizedConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE chainview.block SET canonical=false
			WHERE generation_id=$1 AND height>$2 AND canonical`, l.generation, ancestor); err != nil {
			return err
		}
		for _, block := range blocks {
			if err := insertBlock(ctx, tx, l.generation, block); err != nil {
				return err
			}
		}
		var tipHeight *int64
		var tipHash *string
		if l.tip != nil || len(blocks) != 0 {
			var hash string
			err := tx.QueryRow(ctx, `SELECT block_hash FROM chainview.block WHERE generation_id=$1 AND height=$2 AND canonical`,
				l.generation, tip).Scan(&hash)
			if err != nil {
				return err
			}
			tipHeight, tipHash = &tip, &hash
		}
		var finalHeight *int64
		var finalHash *string
		if final != nil {
			if err := finalize(ctx, tx, l.generation, l.finalized, *final, policy); err != nil {
				return err
			}
			value := final.Hash.Hex()
			finalHeight, finalHash = &final.Height, &value
		}
		result, err := tx.Exec(ctx, `UPDATE chainview.generation SET tip_height=$3,tip_hash=$4,
			finalized_height=COALESCE($5,finalized_height),finalized_hash=COALESCE($6,finalized_hash),
			state=CASE WHEN $5 IS NOT NULL THEN 'ready' ELSE state END,
			observed_at=clock_timestamp(),observed_finalized_height=$7,
			fork_ancestor=CASE WHEN $8<tip_height THEN $8 ELSE fork_ancestor END,
			fork_observed_at=CASE WHEN $8<tip_height THEN clock_timestamp() ELSE fork_observed_at END
			WHERE id=$1 AND fence=$2 AND lease_until>clock_timestamp()`, l.generation, l.fence,
			tipHeight, tipHash, finalHeight, finalHash, observedFinal, ancestor)
		if err == nil && result.RowsAffected() != 1 {
			return ErrLeaseUnavailable
		}
		return err
	})
}

func insertBlock(ctx context.Context, tx pgx.Tx, generation string, block Block) error {
	hash := block.Hash.Hex()
	result, err := tx.Exec(ctx, `INSERT INTO chainview.block VALUES($1,$2,$3,$4,$5,true)
		ON CONFLICT(generation_id,block_hash) DO UPDATE SET canonical=true
		WHERE chainview.block.height=excluded.height AND chainview.block.parent_hash=excluded.parent_hash
			AND chainview.block.block_time=excluded.block_time`,
		generation, hash, block.Height, block.Parent.Hex(), block.Time)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("block identity differs")
	}
	for _, e := range block.Events {
		var mode, generationNumber string
		if e.Mode != 0 {
			mode = strconv.Itoa(int(e.Mode))
		}
		if e.PayerGeneration != 0 {
			generationNumber = strconv.FormatUint(e.PayerGeneration, 10)
		}
		_, err := tx.Exec(ctx, `INSERT INTO chainview.observed_event(
			generation_id,block_hash,tx_hash,log_index,kind,receipt_success,recipient_ref,signer_nonce,
			merchant_key,order_key,refund_key,asset_ref,amount,payer_ref,quote_digest,payment_mode,payer_generation,quote_issued_at,quote_expires_at)
			VALUES($1,$2,$3,$4,$5,true,$6,NULLIF($7,''),$8,$9,NULLIF($10,''),$11,$12::text::numeric,
				$13,NULLIF($14,''),NULLIF($15,'')::smallint,NULLIF($16,'')::numeric,$17,$18)
			ON CONFLICT(generation_id,block_hash,tx_hash,log_index) DO NOTHING`,
			generation, hash, e.TxHash, e.Index, e.Kind, e.Recipient, e.Nonce, e.Merchant, e.Order, e.Refund,
			e.Asset, e.Amount, e.Payer, e.QuoteDigest, mode, generationNumber, e.IssuedAt, e.ExpiresAt)
		if err != nil {
			return fmt.Errorf("insert accepted settlement log: %w", err)
		}
	}
	return nil
}

func finalize(ctx context.Context, tx pgx.Tx, generation string, prior *int64, final Header, policy string) error {
	hash := final.Hash.Hex()
	var canonical bool
	err := tx.QueryRow(ctx, `SELECT canonical FROM chainview.block
		WHERE generation_id=$1 AND height=$2 AND block_hash=$3`, generation, final.Height, hash).Scan(&canonical)
	if err != nil || !canonical {
		return errors.New("finalized checkpoint is outside the persisted branch")
	}
	_, err = tx.Exec(ctx, `INSERT INTO chainview.evidence(id,generation_id,merchant_key,order_key,kind,
		anchor_height,anchor_hash,policy_ref,log_block,tx_hash,log_index,created_at)
		SELECT gen_random_uuid(),ev.generation_id,ev.merchant_key,ev.order_key,ev.kind,
			$2,$3,$4,ev.block_hash,ev.tx_hash,ev.log_index,clock_timestamp()
		FROM chainview.observed_event ev JOIN chainview.block b
			ON (b.generation_id,b.block_hash)=(ev.generation_id,ev.block_hash)
		WHERE ev.generation_id=$1 AND b.canonical AND b.height BETWEEN COALESCE($5::bigint+1,0) AND $2
			AND ev.kind IN ('paid','refunded','invalidated')
		ON CONFLICT(generation_id,log_block,tx_hash,log_index) WHERE log_block IS NOT NULL DO NOTHING`,
		generation, final.Height, hash, policy, prior)
	return err
}
