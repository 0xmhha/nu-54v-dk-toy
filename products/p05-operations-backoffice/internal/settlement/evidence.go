package settlement

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrEvidenceHeld = errors.New("active reconciliation evidence unavailable or held")

type EvidenceStore struct{ db *pgxpool.Pool }

func OpenEvidence(ctx context.Context, dsn string) (*EvidenceStore, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid scoped evidence DSN")
	}
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "5s"
	cfg.ConnConfig.RuntimeParams["jit"] = "off" // see commerce.Open
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "8s"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("scoped evidence unavailable")
	}
	var permitted bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p07_reconcile','USAGE')
		AND NOT has_schema_privilege('chainview','CREATE')
		AND NOT has_table_privilege('chainview.settlement_receipt','SELECT')
		AND NOT has_table_privilege('chainview.observed_event','SELECT')`).Scan(&permitted)
	if err != nil || !permitted {
		db.Close()
		return nil, errors.New("scoped P07 reconciliation login required")
	}
	return &EvidenceStore{db}, nil
}

func (e *EvidenceStore) Close() { e.db.Close() }

type observation struct {
	id, kind, merchant, order, chain, contract, abi, policy string
	asset, amount, tx, block                                string
	payer, recipient, nonce, digest, generation, refund     string
	mode, index                                             int32
	included                                                time.Time
}

func (e *EvidenceStore) lock(ctx context.Context, target Target, quote Quote, action func(pgx.Tx) error) error {
	return e.lockScope(ctx, target.Stream, common.Hash(quote.Args.MerchantId).Hex(), action)
}

func (e *EvidenceStore) lockScope(ctx context.Context, stream, merchant string, action func(pgx.Tx) error) error {
	return pgx.BeginTxFunc(ctx, e.db, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var ready bool
		err := tx.QueryRow(ctx, "SELECT chainview.reconciliation_ready($1,$2)", stream, merchant).Scan(&ready)
		if err != nil || !ready {
			return ErrEvidenceHeld
		}
		return action(tx)
	})
}

// deliveryEpoch holds the stream lock only during a bounded database operation.
func (e *EvidenceStore) deliveryEpoch(ctx context.Context, stream, merchant string, action func(context.Context, string) error) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return pgx.BeginTxFunc(ctx, e.db, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var epoch *string
		if err := tx.QueryRow(ctx, "SELECT chainview.fulfillment_epoch($1,$2)", stream, merchant).Scan(&epoch); err != nil || epoch == nil {
			return ErrEvidenceHeld
		}
		return action(ctx, *epoch)
	})
}

func (e *EvidenceStore) reconciliation(ctx context.Context, worker *Store, w work, target Target, quote Quote) (bool, error) {
	var committed bool
	err := e.lock(ctx, target, quote, func(tx pgx.Tx) error {
		if target.Escrow() {
			var err error
			committed, err = lockReconciliation(ctx, tx, worker, w, target, quote)
			return err
		}
		merchant, order := common.Hash(quote.Args.MerchantId).Hex(), common.Hash(quote.Args.OrderId).Hex()
		kind := "paid"
		refundKey := ""
		if w.refund != nil {
			kind = "refunded"
			refundKey = common.BytesToHash(w.refundKey).Hex()
		}
		var item observation
		err := tx.QueryRow(ctx, `SELECT id::text,kind,merchant_key,order_key,chain_ref,contract_ref,abi_hash,policy_ref,
			asset_ref,amount::text,tx_hash,inclusion_hash,COALESCE(payer_ref,''),COALESCE(recipient_ref,''),
			COALESCE(signer_nonce,''),COALESCE(quote_digest,''),COALESCE(payer_generation::text,''),
			COALESCE(refund_key,''),COALESCE(payment_mode,0),log_index,included_at
			FROM chainview.reconciliation_receipt($1,$2,$3)
			WHERE kind=$4 AND COALESCE(refund_key,'')=$5`, target.Stream, merchant, order, kind, refundKey).
			Scan(&item.id, &item.kind, &item.merchant, &item.order, &item.chain, &item.contract, &item.abi, &item.policy,
				&item.asset, &item.amount, &item.tx, &item.block, &item.payer, &item.recipient, &item.nonce, &item.digest,
				&item.generation, &item.refund, &item.mode, &item.index, &item.included)
		if errors.Is(err, pgx.ErrNoRows) {
			if w.refund == nil && !time.Now().Before(quote.expiry()) {
				committed, err = applyAbsence(ctx, tx, worker, w, target, quote)
				return err
			}
			return nil
		}
		if err != nil {
			return err
		}
		if item.chain != "8283" || item.contract != lower(target.Contract) || item.abi != ABIHash || item.policy != target.Manifest ||
			item.asset != lower(quote.Args.Asset) || item.payer != lower(quote.Args.Payer) || item.mode != int32(quote.Args.Mode) ||
			item.merchant != merchant || item.order != order || item.included.Unix() < int64(quote.Args.IssuedAt) {
			return errors.New("finalized fact outside enrolled quote scope")
		}
		if w.refund == nil {
			if item.amount != quote.Args.Amount.String() || item.recipient != lower(quote.Args.Beneficiary) ||
				item.nonce != quote.Args.Nonce.String() || item.digest != common.Hash(quote.Digest).Hex() ||
				item.generation != strconv.FormatUint(quote.Args.PayerGeneration, 10) || quote.Args.PayerGeneration > math.MaxInt64 ||
				!item.included.Before(quote.expiry()) {
				return errors.New("finalized payment differs from the exact signed quote")
			}
			_, err = worker.db.Exec(ctx, `SELECT ledger.confirm_payment($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::text::numeric,$11,$12,$13,$14)`,
				w.sale, quote.Digest[:], target.Stream, item.id, item.tx, item.index, item.block, item.included, item.asset, item.amount,
				item.payer, int64(quote.Args.PayerGeneration), item.mode, item.merchant)
		} else {
			if w.refundAmount == nil || item.amount != *w.refundAmount || item.refund != refundKey || item.recipient != item.payer ||
				w.expires == nil || !item.included.Before(*w.expires) {
				return errors.New("finalized refund differs from the signed original route")
			}
			_, err = worker.db.Exec(ctx, `SELECT ledger.confirm_refund($1,$2,$3,$4,$5,$6,$7,$8,$9::text::numeric,$10,$11,$12)`,
				*w.refund, target.Stream, item.id, item.tx, item.index, item.block, item.included, item.asset, item.amount, item.payer, item.mode, w.refundKey)
		}
		committed = err == nil
		return err
	})
	return committed, err
}

// requestAbsence asks the indexer to prove that nothing paid an expired order. The indexer keeps one
// request per order. Before its migration the operator's prove-absence command is the only way.
func (e *EvidenceStore) requestAbsence(ctx context.Context, target Target, quote Quote) error {
	_, err := e.db.Exec(ctx, "SELECT chainview.request_absence($1,$2,$3,$4)", target.Stream,
		common.Hash(quote.Args.MerchantId).Hex(), common.Hash(quote.Args.OrderId).Hex(), quote.expiry().Unix())
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "42883" {
		return nil
	}
	return err
}

func applyAbsence(ctx context.Context, tx pgx.Tx, worker *Store, w work, target Target, quote Quote) (bool, error) {
	merchant, order := common.Hash(quote.Args.MerchantId).Hex(), common.Hash(quote.Args.OrderId).Hex()
	var id, anchor, policy, proof string
	var anchored time.Time
	err := tx.QueryRow(ctx, `SELECT evidence_id::text,anchor_hash,anchored_at,policy_ref,state_proof_ref
		FROM chainview.reconciliation_absence($1,$2,$3)`, target.Stream, merchant, order).Scan(&id, &anchor, &anchored, &policy, &proof)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	expected := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(fmt.Sprintf("nu54-absence-v2\n%s\n%s\n%s\n%s\n%s\n%d\nfalse",
		target.Manifest, target.Stream, merchant, order, anchor, quote.Args.ExpiresAt))))
	if policy != target.Manifest || proof != expected || anchored.Before(quote.expiry()) {
		return false, errors.New("absence evidence differs from the signed order expiry")
	}
	_, err = worker.db.Exec(ctx, "SELECT ledger.close_unpaid($1,$2,$3,$4)", w.sale, quote.Digest[:], id, anchored)
	return err == nil, err
}
