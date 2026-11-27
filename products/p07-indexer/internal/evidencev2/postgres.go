package evidencev2

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

type Store struct{ db *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	var allowed bool
	err = db.QueryRow(ctx, `SELECT current_user <> 'postgres'
		AND pg_has_role(current_user, 'nu54_p07_read', 'USAGE')
		AND NOT pg_has_role(current_user, 'nu54_p07_ingest', 'MEMBER')
		AND NOT pg_has_role(current_user, 'nu54_p07_switch', 'MEMBER')
        AND NOT pg_has_role(current_user, 'nu54_p07_legacy', 'MEMBER')
		AND NOT (SELECT rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb
		  FROM pg_roles WHERE rolname = current_user)
		AND has_table_privilege('chainview.active_evidence', 'SELECT')
		AND NOT has_schema_privilege('chainview', 'CREATE')`).Scan(&allowed)
	if err != nil || !allowed {
		db.Close()
		return nil, errors.New("C7 requires a migrated database and restricted evidence reader role")
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() { s.db.Close() }

func (s *Store) Finalized(ctx context.Context, generation, event string) (*service.Evidence, error) {
	return finalized(ctx, s.db, generation, event)
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func finalized(ctx context.Context, db queryer, generation, event string) (*service.Evidence, error) {
	var evidence service.Evidence
	var observed time.Time
	var ceiling *time.Time
	err := db.QueryRow(ctx, `SELECT id::text, generation_id::text, anchor_hash, kind, merchant_key, order_key, tx_hash, created_at, expires_at
		FROM chainview.active_evidence
		WHERE generation_id = $1 AND id = $2 AND kind IN ('paid','refunded','invalidated')`,
		generation, event).Scan(&evidence.EventID, &evidence.Generation, &evidence.AnchorHash, &evidence.Fact, &evidence.Merchant, &evidence.Order, &evidence.TxHash, &observed, &ceiling)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	evidence.Kind = "finalized"
	evidence.AnchorHash = strings.TrimPrefix(strings.ToLower(evidence.AnchorHash), "0x")
	evidence.Merchant = strings.ToLower(evidence.Merchant)
	if len(evidence.Merchant) == 66 {
		evidence.Merchant = strings.TrimPrefix(evidence.Merchant, "0x")
	}
	evidence.Order = strings.TrimPrefix(strings.ToLower(evidence.Order), "0x")
	evidence.TxHash = strings.TrimPrefix(strings.ToLower(evidence.TxHash), "0x")
	expires := observed.Add(service.EvidenceTTL())
	if ceiling != nil && ceiling.Before(expires) {
		expires = *ceiling
	}
	evidence.ObservedAt, evidence.ExpiresAt = observed.Unix(), expires.Unix()
	return &evidence, nil
}
