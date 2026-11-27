package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProtectedWithdrawalReconciler struct {
	db         *pgxpool.Pool
	chain      *core.Chain
	deployment [32]byte
	settlement string
}

func OpenProtectedWithdrawalReconciler(ctx context.Context, dsn string, chain *core.Chain, deployment [32]byte,
	settlement common.Address) (*ProtectedWithdrawalReconciler, error) {
	if dsn == "" || chain == nil || deployment == ([32]byte{}) || settlement == (common.Address{}) {
		return nil, errors.New("protected withdrawal reconciler requires a pinned deployment")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid withdrawal reconciliation database configuration")
	}
	config.ConnConfig.RuntimeParams["lock_timeout"] = "2s"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "5s"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("withdrawal reconciliation database unavailable")
	}
	var valid bool
	err = pool.QueryRow(ctx, `SELECT session_user=current_user AND pg_has_role(current_user,'nu54_p05_rental_reconcile','USAGE')
		AND NOT EXISTS(SELECT FROM pg_roles WHERE rolname IN ('nu54_p05_commerce','nu54_p05_rental_authority',
			'nu54_p05_rental_authority_read','nu54_p05_rental_evidence','nu54_p05_rental_close') AND pg_has_role(current_user,oid,'MEMBER'))
		AND NOT (SELECT rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls FROM pg_roles WHERE rolname=current_user)
		AND NOT has_schema_privilege('ledger','CREATE')
		AND NOT has_table_privilege('ledger.rental_withdrawal_candidate','SELECT,INSERT,UPDATE,DELETE,TRUNCATE')
		AND NOT has_table_privilege('ledger.rental_withdrawal_resolution','SELECT,INSERT,UPDATE,DELETE,TRUNCATE')`).Scan(&valid)
	if err != nil || !valid {
		pool.Close()
		return nil, errors.New("dedicated withdrawal reconciliation login required")
	}
	return &ProtectedWithdrawalReconciler{db: pool, chain: chain, deployment: deployment, settlement: strings.ToLower(settlement.Hex())}, nil
}

func (w *ProtectedWithdrawalReconciler) Close() { w.db.Close() }

type withdrawalReconciliationRow struct {
	branch, board, signer, nonce, account, destination string
	generation                                         int64
	original, closeHash                                []byte
}

func (w *ProtectedWithdrawalReconciler) Reconcile(ctx context.Context) error {
	rows, err := w.db.Query(ctx, `SELECT branch_id::text,device_ref::text,generation,original_hash,signer,
		nonce::text,account_ref,destination_ref,close_hash FROM ledger.rental_withdrawal_reconcile_next($1,$2)`,
		w.deployment[:], w.settlement)
	if err != nil {
		return err
	}
	batch, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (r withdrawalReconciliationRow, err error) {
		err = row.Scan(&r.branch, &r.board, &r.generation, &r.original, &r.signer, &r.nonce, &r.account, &r.destination, &r.closeHash)
		return
	})
	if err != nil {
		return err
	}
	for _, row := range batch {
		probe, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = w.reconcile(probe, row)
		cancel()
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func (w *ProtectedWithdrawalReconciler) reconcile(ctx context.Context, row withdrawalReconciliationRow) error {
	nonce, err := strconv.ParseUint(row.nonce, 10, 64)
	if err != nil || len(row.original) != 32 || len(row.closeHash) != 32 {
		return service.Conflict
	}
	original := common.Hash(row.original)
	attempt, err := w.chain.InspectProtectedWithdrawalAttempt(ctx, common.HexToAddress(row.signer), nonce, original)
	if err != nil {
		return err
	}
	status, transaction, height, block := attempt.State, attempt.Consuming, attempt.FinalHeight, attempt.FinalHash
	if status != "reverted" && status != "superseded" {
		payout, err := w.chain.ObserveProtectedWithdrawal(ctx, common.HexToAddress(row.account), common.HexToAddress(row.destination), common.Hash(row.closeHash))
		if err != nil || (payout.State != "withdrawal-held" && payout.State != "withdrawal-received") {
			return err
		}
		status, transaction, height, block = "executed", payout.PayoutHash, payout.FinalizedHeight, payout.FinalizedHash
		if payout.PayoutHash != original {
			status = "settled_other"
		}
	}
	if transaction == (common.Hash{}) || block == (common.Hash{}) || height <= 0 {
		return service.Conflict
	}
	_, err = w.db.Exec(ctx, "SELECT ledger.rental_withdrawal_reconcile_finish($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)", row.branch,
		row.board, row.generation, row.original, status, transaction[:], height, block[:], w.deployment[:], w.settlement)
	return err
}

type protectedWithdrawalTerminal struct {
	State           string    `json:"state"`
	OriginalHash    string    `json:"originalHash"`
	EvidenceHash    string    `json:"evidenceHash"`
	ConfirmedAt     time.Time `json:"confirmedAt"`
	FinalizedHeight int64     `json:"finalizedHeight,omitempty"`
	FinalizedHash   string    `json:"finalizedHash,omitempty"`
}

func (c *ProtectedRentalClose) resolvedWithdrawal(ctx context.Context, req protectedCloseRequest) (*protectedWithdrawalTerminal, error) {
	var terminal protectedWithdrawalTerminal
	var raw []byte
	err := c.store.db.QueryRow(ctx, "SELECT ledger.rental_withdrawal_resolution_view($1,$2,$3,$4,$5)", req.arguments()...).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(raw, &terminal) != nil {
		return nil, service.Unknown
	}
	if terminal.State == "" {
		return nil, nil
	}
	return &terminal, nil
}
