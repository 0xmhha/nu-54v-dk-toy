package market

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkStatus struct {
	Position  string       `json:"position"`
	Kind      string       `json:"kind"`
	Period    string       `json:"period"`
	Revision  string       `json:"revision"`
	Version   string       `json:"priceVersion"`
	Hash      common.Hash  `json:"txHash"`
	State     string       `json:"state"`
	Block     *int64       `json:"blockHeight,omitempty"`
	BlockHash *common.Hash `json:"blockHash,omitempty"`
}

func OpenInspector(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.New("invalid inspector database")
	}
	var authorized bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p08_inspector','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p08_keeper','MEMBER')
		AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND
			(rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication))
		AND NOT has_schema_privilege('market','CREATE')
		AND NOT has_column_privilege(current_user,'market.keeper_attempt','raw_tx','SELECT')
		AND NOT has_column_privilege(current_user,'market.keeper_attempt','state','UPDATE')`).Scan(&authorized)
	if err != nil || !authorized {
		db.Close()
		return nil, errors.New("P08 operator requires its read-only inspector database login")
	}
	return db, nil
}

func WorkHistory(ctx context.Context, db *pgxpool.Pool, p Profile) ([]WorkStatus, error) {
	rows, err := db.Query(ctx, `SELECT position::text,kind,period::text,revision::text,price_version::text,
		tx_hash,state,block_height,block_hash FROM market.keeper_attempt WHERE `+venueFilter+
		" ORDER BY created_at DESC, tx_hash DESC LIMIT 50", p.Genesis.Bytes(), p.Venue.Address.Bytes(), p.BlockHash.Bytes())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	work := make([]WorkStatus, 0)
	for rows.Next() {
		var event WorkStatus
		var tx, block []byte
		if err := rows.Scan(&event.Position, &event.Kind, &event.Period, &event.Revision, &event.Version, &tx, &event.State, &event.Block, &block); err != nil {
			return nil, err
		}
		event.Hash = common.BytesToHash(tx)
		if len(block) == common.HashLength {
			hash := common.BytesToHash(block)
			event.BlockHash = &hash
		}
		work = append(work, event)
	}
	return work, rows.Err()
}
