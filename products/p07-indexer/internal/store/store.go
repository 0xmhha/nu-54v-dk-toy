// Package store keeps P07 receipts and the ingest cursor in PostgreSQL ([N25]).
package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/receipt"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/migrations"
)

// Row is one PaymentSettled log to store.
type Row struct {
	TxHash      string
	LogIndex    uint
	Merchant    string
	OrderID     string
	Device      string
	Amount      string // decimal
	Nonce       string // decimal
	BlockNumber uint64
	BlockTime   uint64
}

// Store is the PostgreSQL store.
type Store struct{ db *pgxpool.Pool }

// Open connects and applies the schema.
func Open(ctx context.Context, url string) (*Store, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(ctx, migrations.Receipts); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}

func (s *Store) Close() { s.db.Close() }

// Cursor is the last block whose logs are stored, or `start` before the first ingest.
func (s *Store) Cursor(ctx context.Context, start uint64) (uint64, error) {
	var n int64
	err := s.db.QueryRow(ctx, `SELECT block_number FROM cursor WHERE id = 1`).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return start, nil
	}
	return uint64(n), err
}

// Commit stores the rows of a block range and moves the cursor to `to`, in one transaction
// (P07-FR-02, FR-03): a log stored twice is ignored, and nothing moves if anything fails.
func (s *Store) Commit(ctx context.Context, rows []Row, to uint64) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		for _, r := range rows {
			_, err := tx.Exec(ctx, `INSERT INTO receipts (tx_hash, log_index, merchant, order_id, device, amount, nonce, block_number, block_time)
				VALUES ($1, $2, $3, $4, $5, $6::numeric, $7::numeric, $8, $9) ON CONFLICT (tx_hash, log_index) DO NOTHING`,
				strings.ToLower(r.TxHash), r.LogIndex, strings.ToLower(r.Merchant), strings.ToLower(r.OrderID), strings.ToLower(r.Device),
				r.Amount, r.Nonce, int64(r.BlockNumber), int64(r.BlockTime))
			if err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO cursor (id, block_number) VALUES (1, $1)
			ON CONFLICT (id) DO UPDATE SET block_number = EXCLUDED.block_number`, int64(to))
		return err
	})
}

// Get returns the earliest receipt of (merchant, orderId); a second log marks it duplicate (P07-FR-04, FR-05).
func (s *Store) Get(ctx context.Context, merchant, orderID string) (receipt.Receipt, error) {
	rows, err := s.db.Query(ctx, `SELECT merchant, order_id, device, amount::text, nonce::text, tx_hash, block_number, block_time
		FROM receipts WHERE merchant = $1 AND order_id = $2 ORDER BY block_number, log_index LIMIT 2`,
		strings.ToLower(merchant), strings.ToLower(orderID))
	if err != nil {
		return receipt.Receipt{}, err
	}
	defer rows.Close()
	var out []receipt.Receipt
	for rows.Next() {
		var r receipt.Receipt
		var bn, bt int64
		if err := rows.Scan(&r.Merchant, &r.OrderID, &r.Device, &r.Amount, &r.Nonce, &r.TxHash, &bn, &bt); err != nil {
			return receipt.Receipt{}, err
		}
		r.BlockNumber, r.BlockTime = uint64(bn), uint64(bt)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return receipt.Receipt{}, err
	}
	if len(out) == 0 {
		return receipt.Receipt{}, receipt.ErrNotFound
	}
	out[0].Duplicate = len(out) > 1
	return out[0], nil
}

// Exec runs a statement (tests).
func (s *Store) Exec(ctx context.Context, sql string) error {
	_, err := s.db.Exec(ctx, sql)
	return err
}

// Count is the number of stored logs (tests and health).
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM receipts`).Scan(&n)
	return n, err
}
