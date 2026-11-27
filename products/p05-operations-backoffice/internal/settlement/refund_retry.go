package settlement

import (
	"context"
	"errors"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/finality"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/jackc/pgx/v5"
)

var errRefundReopened = errors.New("refund awaits renewed independent approval")

type failedReceiptSource interface {
	verifiedFailure(context.Context, *types.Receipt) (bool, error)
}
type failedReceiptAuthority interface {
	reopenRefund(context.Context, *Store, work, Target, Quote, *types.Receipt) (bool, error)
}

func (c LiveChain) verifiedFailure(ctx context.Context, receipt *types.Receipt) (bool, error) {
	if receipt.Status != types.ReceiptStatusFailed || receipt.BlockNumber == nil || !receipt.BlockNumber.IsInt64() ||
		receipt.BlockNumber.Sign() < 0 || receipt.BlockHash == (common.Hash{}) {
		return false, nil
	}
	block, err := c.BlockByHash(ctx, receipt.BlockHash)
	if err != nil {
		return false, err
	}
	if block.Hash() != receipt.BlockHash || block.Number().Cmp(receipt.BlockNumber) != 0 ||
		receipt.TransactionIndex >= uint(len(block.Transactions())) ||
		block.Transactions()[receipt.TransactionIndex].Hash() != receipt.TxHash {
		return false, nil
	}
	receipts, err := finality.BlockReceipts(ctx, c.Client, block)
	if err != nil {
		return false, err
	}
	return receipts[receipt.TransactionIndex].Status == types.ReceiptStatusFailed, nil
}

func (e *EvidenceStore) reopenRefund(ctx context.Context, worker *Store, job work, target Target, quote Quote, receipt *types.Receipt) (bool, error) {
	var reopened bool
	err := e.lock(ctx, target, quote, func(tx pgx.Tx) error {
		merchant, order := common.Hash(quote.Args.MerchantId).Hex(), common.Hash(quote.Args.OrderId).Hex()
		var canonical bool
		err := tx.QueryRow(ctx, `SELECT chainview.reconciliation_finalized_block($1,$2,$3,$4)
			AND NOT EXISTS(SELECT FROM chainview.reconciliation_receipt($1,$2,$5)
			 WHERE kind='refunded' AND refund_key=$6)`,
			target.Stream, merchant, receipt.BlockNumber.Int64(), receipt.BlockHash.Hex(), order, common.BytesToHash(job.refundKey).Hex()).Scan(&canonical)
		if err != nil || !canonical {
			return err
		}
		return worker.db.QueryRow(ctx, "SELECT ledger.reopen_finalized_refund($1,$2,$3,$4,$5)",
			job.id, job.fence, receipt.TxHash.Hex(), receipt.BlockHash.Hex(), receipt.BlockNumber.Int64()).Scan(&reopened)
	})
	return reopened, err
}

func (w *Worker) establishRefundFailure(ctx context.Context, job work, target Target, quote Quote, receipt *types.Receipt) error {
	chain, hasChain := w.Chain.(failedReceiptSource)
	auth, hasAuthority := w.Evidence.(failedReceiptAuthority)
	if !hasChain || !hasAuthority || receipt.TxHash.Hex() != *job.hash {
		return nil
	}
	verified, err := chain.verifiedFailure(ctx, receipt)
	if err != nil || !verified {
		return err
	}
	reopened, err := auth.reopenRefund(ctx, w.Store, job, target, quote, receipt)
	if err != nil {
		return err
	}
	if reopened {
		return errRefundReopened
	}
	return nil
}
