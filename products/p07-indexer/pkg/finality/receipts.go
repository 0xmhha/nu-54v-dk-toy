// Package finality validates canonical Ethereum receipt coverage for product readers.
package finality

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
)

// BlockReceipts fetches receipts and proves their integrity and ordering against the header.
func BlockReceipts(ctx context.Context, client *ethclient.Client, block *types.Block) ([]*types.Receipt, error) {
	txs := block.Transactions()
	hash, height := block.Hash(), block.Number()
	if !height.IsUint64() || types.DeriveSha(txs, trie.NewStackTrie(nil)) != block.TxHash() {
		return nil, errors.New("invalid block transaction coverage")
	}
	receipts, err := client.BlockReceipts(ctx, rpc.BlockNumberOrHashWithHash(hash, true))
	if err != nil {
		receipts = make([]*types.Receipt, len(txs))
		for index, tx := range txs {
			receipts[index], err = client.TransactionReceipt(ctx, tx.Hash())
			if err != nil {
				return nil, err
			}
		}
	}
	if len(receipts) != len(txs) {
		return nil, errors.New("missing or extra transaction receipts")
	}
	var logIndex uint
	var cumulative uint64
	for index, receipt := range receipts {
		if receipt == nil || receipt.TxHash != txs[index].Hash() || receipt.BlockHash != hash ||
			receipt.TransactionIndex != uint(index) || receipt.BlockNumber == nil ||
			receipt.BlockNumber.Cmp(height) != 0 || receipt.Type != txs[index].Type() || len(receipt.PostState) != 0 ||
			receipt.CumulativeGasUsed < cumulative || receipt.GasUsed != receipt.CumulativeGasUsed-cumulative ||
			(receipt.Status != types.ReceiptStatusSuccessful && receipt.Status != types.ReceiptStatusFailed) ||
			(receipt.Status == types.ReceiptStatusFailed && len(receipt.Logs) != 0) {
			return nil, errors.New("receipt contradicted block coverage")
		}
		cumulative = receipt.CumulativeGasUsed
		for _, log := range receipt.Logs {
			if log == nil || log.BlockHash != hash || log.BlockNumber != height.Uint64() ||
				log.TxHash != receipt.TxHash || log.TxIndex != uint(index) || log.Index != logIndex || log.Removed {
				return nil, errors.New("receipt log identity mismatch")
			}
			logIndex++
		}
	}
	if cumulative != block.GasUsed() || types.DeriveSha(types.Receipts(receipts), trie.NewStackTrie(nil)) != block.ReceiptHash() {
		return nil, errors.New("receipt root mismatch")
	}
	return receipts, nil
}

type Evidence struct {
	Transaction *types.Transaction
	Receipt     *types.Receipt
	Anchor      *types.Header
}

var ErrPending = errors.New("transaction finality pending")

// CanonicalReceipt checks the receipt's block at its height and the finalized tag.
// The caller pins its network and deployment and verifies its own event semantics.
func CanonicalReceipt(ctx context.Context, client *ethclient.Client, hash common.Hash) (Evidence, error) {
	hint, err := client.TransactionReceipt(ctx, hash)
	if err != nil {
		return Evidence{}, err
	}
	if hint == nil || hint.TxHash != hash || hint.BlockNumber == nil || !hint.BlockNumber.IsUint64() {
		return Evidence{}, errors.New("invalid receipt hint")
	}
	finalized, err := client.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil {
		return Evidence{}, err
	}
	if !finalized.Number.IsUint64() || hint.BlockNumber.Cmp(finalized.Number) > 0 {
		return Evidence{}, ErrPending
	}
	block, err := client.BlockByNumber(ctx, hint.BlockNumber)
	if err != nil {
		return Evidence{}, err
	}
	if block.Hash() != hint.BlockHash || block.Number().Cmp(hint.BlockNumber) != 0 {
		return Evidence{}, ErrPending
	}
	receipts, err := BlockReceipts(ctx, client, block)
	if err != nil {
		return Evidence{}, err
	}
	if hint.TransactionIndex >= uint(len(receipts)) || receipts[hint.TransactionIndex].TxHash != hash {
		return Evidence{}, errors.New("transaction not in canonical receipt coverage")
	}
	recheck, err := client.HeaderByNumber(ctx, finalized.Number)
	if err != nil {
		return Evidence{}, err
	}
	if finalized.Hash() != recheck.Hash() {
		return Evidence{}, ErrPending
	}
	recheck, err = client.HeaderByNumber(ctx, hint.BlockNumber)
	if err != nil {
		return Evidence{}, err
	}
	if block.Hash() != recheck.Hash() {
		return Evidence{}, ErrPending
	}
	return Evidence{block.Transactions()[hint.TransactionIndex], receipts[hint.TransactionIndex], finalized}, nil
}
