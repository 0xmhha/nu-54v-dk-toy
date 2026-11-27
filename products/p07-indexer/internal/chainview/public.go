package chainview

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PublicStore struct {
	db              *pgxpool.Pool
	LegacyLedgerURL string
}

func OpenPublic(ctx context.Context, url string) (*PublicStore, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	var allowed bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user, 'nu54_p07_public', 'USAGE')
		AND NOT pg_has_role(current_user, 'nu54_p07_reconcile', 'MEMBER')
		AND NOT pg_has_role(current_user, 'nu54_p07_read', 'MEMBER')
		AND NOT pg_has_role(current_user, 'nu54_p07_ingest', 'MEMBER')
		AND NOT pg_has_role(current_user, 'nu54_p07_legacy', 'MEMBER')
		AND NOT pg_has_role(current_user, 'nu54_p07_switch', 'MEMBER')
		AND NOT pg_has_role(current_user, 'nu54_p07_incident', 'MEMBER')
		AND has_table_privilege('chainview.finalized_receipt','SELECT')
		AND NOT pg_has_role(current_user, 'nu54_p07_explorer', 'MEMBER')
		AND NOT has_schema_privilege('chainview', 'CREATE')`).Scan(&allowed)
	if err != nil || !allowed {
		db.Close()
		return nil, errors.New("public endpoint requires a migrated database and projection-only login")
	}
	return &PublicStore{db: db}, nil
}

func (s *PublicStore) Close() { s.db.Close() }

type Receipt struct {
	Kind                string    `json:"kind"`
	Merchant            string    `json:"merchant"`
	Order               string    `json:"order"`
	Refund              string    `json:"refund,omitempty"`
	Asset               string    `json:"asset"`
	Amount              string    `json:"amount"`
	TxHash              string    `json:"txHash"`
	LogIndex            int32     `json:"logIndex"`
	Height              int64     `json:"inclusionHeight"`
	IncludedAt          time.Time `json:"includedAt"`
	AttestedAt          time.Time `json:"attestedAt"`
	Anchor              int64     `json:"finalizedAnchorHeight"`
	RecipientCommitment *string   `json:"recipientCommitment,omitempty"`
}

type orderTotals struct {
	Order     string `json:"order"`
	Paid      string `json:"paid"`
	Refunded  string `json:"refunded"`
	Remaining string `json:"remaining"`
}

func (s *PublicStore) PublicHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/receipts", s.receipts)
	mux.HandleFunc("GET /v2/explorer/deployments", s.deployments)
	mux.HandleFunc("GET /{$}", explorerPage(false))
	mux.HandleFunc("GET /web/explorer.js", explorerStatic)
	mux.HandleFunc("GET /web/explorer.css", explorerStatic)
	return mux
}

func (s *PublicStore) receipts(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	stream, merchant, order, transaction := q.Get("stream"), q.Get("merchant"), q.Get("order"), q.Get("txHash")
	if !uuidPattern.MatchString(stream) || !(hexKey(merchant, 40) || hexKey(merchant, 64)) ||
		(order == "") == (transaction == "") || (order != "" && !hexKey(order, 64)) ||
		(transaction != "" && !hexKey(transaction, 64)) {
		http.Error(w, "invalid receipt reference", http.StatusBadRequest)
		return
	}
	var logIndex *int32
	if input := q.Get("logIndex"); input != "" {
		index, err := strconv.ParseInt(input, 10, 32)
		if transaction == "" || err != nil || index < 0 {
			http.Error(w, "invalid transaction log index", http.StatusBadRequest)
			return
		}
		value := int32(index)
		logIndex = &value
	}
	height, index, err := afterPosition(q.Get("after"))
	if err != nil {
		http.Error(w, "invalid receipt cursor", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		http.Error(w, "receipt projection unavailable", http.StatusServiceUnavailable)
		return
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	var deployment *Deployment
	var item Deployment
	err = tx.QueryRow(ctx, "SELECT "+deploymentFields+" FROM chainview.explorer_deployment WHERE stream_id=$1", stream).Scan(item.destinations()...)
	if err == nil {
		deployment = &item
	} else if !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "deployment view unavailable", http.StatusServiceUnavailable)
		return
	}
	before, revisionErr := captureReadRevision(ctx, tx, stream)
	if deployment != nil && revisionErr != nil {
		http.Error(w, "receipt observation unavailable", http.StatusServiceUnavailable)
		return
	}

	rows, err := tx.Query(ctx, `SELECT kind,merchant_key,order_key,COALESCE(refund_key,''),asset_ref,trim_scale(amount)::text,
		tx_hash,log_index,inclusion_height,included_at,attested_at,anchor_height,recipient_commitment
		FROM chainview.explorer_receipt_detail WHERE stream_id=$1 AND merchant_key=$2 AND ($3='' OR order_key=$3)
		  AND ($4='' OR tx_hash=$4) AND ($5::int IS NULL OR log_index=$5)
		  AND (inclusion_height,log_index)>($6,$7)
		ORDER BY inclusion_height,log_index LIMIT 101`, stream, merchant, order, transaction, logIndex, height, index)
	if err != nil {
		http.Error(w, "receipt projection unavailable", http.StatusServiceUnavailable)
		return
	}
	defer rows.Close()
	receipts := make([]Receipt, 0)
	for rows.Next() {
		var receipt Receipt
		err := rows.Scan(&receipt.Kind, &receipt.Merchant, &receipt.Order, &receipt.Refund, &receipt.Asset, &receipt.Amount,
			&receipt.TxHash, &receipt.LogIndex, &receipt.Height, &receipt.IncludedAt, &receipt.AttestedAt, &receipt.Anchor, &receipt.RecipientCommitment)
		if err != nil {
			http.Error(w, "receipt projection unavailable", http.StatusServiceUnavailable)
			return
		}
		receipts = append(receipts, receipt)
	}
	if rows.Err() != nil {
		http.Error(w, "receipt projection unavailable", http.StatusServiceUnavailable)
		return
	}
	next := ""
	if len(receipts) > 100 {
		receipts = receipts[:100]
		last := receipts[len(receipts)-1]
		next = strconv.FormatInt(last.Height, 10) + ":" + strconv.FormatInt(int64(last.LogIndex), 10)
	}
	var decimals *int16
	err = tx.QueryRow(ctx, "SELECT asset_decimals FROM chainview.explorer_units WHERE stream_id=$1", stream).Scan(&decimals)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "receipt amount metadata unavailable", http.StatusServiceUnavailable)
		return
	}
	var totals *orderTotals
	if order != "" && deployment != nil && deployment.Version == 2 && len(receipts) > 0 {
		item := orderTotals{Order: order}
		err = tx.QueryRow(ctx, `SELECT trim_scale(paid)::text,trim_scale(refunded)::text,trim_scale(paid-refunded)::text
			FROM (SELECT count(*) FILTER(WHERE kind='paid') AS payments,
			  sum(amount) FILTER(WHERE kind='paid') AS paid,
			  coalesce(sum(amount) FILTER(WHERE kind='refunded'),0) AS refunded
			  FROM chainview.explorer_receipt_detail WHERE stream_id=$1 AND merchant_key=$2 AND order_key=$3) AS balance
			WHERE payments=1 AND refunded<=paid`, stream, merchant, order).Scan(&item.Paid, &item.Refunded, &item.Remaining)
		if err == nil {
			totals = &item
		} else if !errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "receipt totals unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	legacyLedger := ""
	if deployment != nil && deployment.Version == 1 {
		legacyLedger = s.LegacyLedgerURL
	}
	if err = tx.Commit(ctx); err != nil {
		http.Error(w, "receipt projection unavailable", http.StatusServiceUnavailable)
		return
	}
	if deployment != nil && before.Status != "held" {
		after, err := captureReadRevision(ctx, s.db, stream)
		if err != nil || !sameReadRevision(before, after) {
			http.Error(w, "receipt observation changed", http.StatusServiceUnavailable)
			return
		}
	}

	webHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Receipts     []Receipt    `json:"receipts"`
		Next         string       `json:"next,omitempty"`
		Deployment   *Deployment  `json:"deployment,omitempty"`
		Decimals     *int16       `json:"assetDecimals,omitempty"`
		Totals       *orderTotals `json:"totals,omitempty"`
		LegacyLedger string       `json:"legacyLedger,omitempty"`
	}{receipts, next, deployment, decimals, totals, legacyLedger})
}

func afterPosition(value string) (int64, int32, error) {
	if value == "" {
		return -1, -1, nil
	}
	a, b, ok := strings.Cut(value, ":")
	height, errA := strconv.ParseInt(a, 10, 64)
	index, errB := strconv.ParseInt(b, 10, 32)
	if !ok || errA != nil || errB != nil || height < 0 || index < 0 {
		return 0, 0, errors.New("invalid cursor")
	}
	return height, int32(index), nil
}
