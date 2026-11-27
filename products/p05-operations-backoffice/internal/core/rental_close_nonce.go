package core

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
)

var errRentalCloseNonceEvidence = errors.New("finalized protected close nonce use could not be verified")

func (c *Chain) rentalCloseNonceAt(ctx context.Context, signer common.Address, header *types.Header) (uint64, error) {
	var result hexutil.Uint64
	err := c.client.Client().CallContext(ctx, &result, "eth_getTransactionCount", signer, rpc.BlockNumberOrHashWithHash(header.Hash(), true))
	return uint64(result), err
}

// A consumed nonce is only a search hint. Verify the full canonical
// transaction body and recover the exact sender before classifying the use.
func (c *Chain) finalizedRentalCloseNonceUse(ctx context.Context, signer common.Address, nonce uint64, final *types.Header) (common.Hash, error) {
	if final.Number == nil || !final.Number.IsUint64() {
		return common.Hash{}, errRentalCloseNonceEvidence
	}
	low, high := uint64(0), final.Number.Uint64()
	for low < high {
		middle := low + (high-low)/2
		header, err := c.client.HeaderByNumber(ctx, new(big.Int).SetUint64(middle))
		if err != nil {
			return common.Hash{}, errRentalCloseNonceEvidence
		}
		used, err := c.rentalCloseNonceAt(ctx, signer, header)
		if err != nil {
			return common.Hash{}, errRentalCloseNonceEvidence
		}
		if used > nonce {
			high = middle
		} else {
			low = middle + 1
		}
	}
	number := new(big.Int).SetUint64(low)
	origin, err := c.client.HeaderByNumber(ctx, number)
	if err != nil {
		return common.Hash{}, errRentalCloseNonceEvidence
	}
	if consumed, err := c.rentalCloseNonceAt(ctx, signer, origin); err != nil || consumed <= nonce {
		return common.Hash{}, errRentalCloseNonceEvidence
	}
	block, err := c.client.BlockByHash(ctx, origin.Hash())
	if err != nil || block.Hash() != origin.Hash() || block.Number().Cmp(number) != 0 || block.TxHash() != origin.TxHash ||
		types.DeriveSha(block.Transactions(), trie.NewStackTrie(nil)) != origin.TxHash {
		return common.Hash{}, errRentalCloseNonceEvidence
	}
	recoverSigner := types.LatestSignerForChainID(new(big.Int).SetUint64(c.dep.ChainID))
	for _, tx := range block.Transactions() {
		if tx.Nonce() != nonce {
			continue
		}
		from, err := types.Sender(recoverSigner, tx)
		if err != nil || from != signer {
			continue
		}
		current, err := c.client.HeaderByNumber(ctx, number)
		if err != nil || current.Hash() != origin.Hash() {
			return common.Hash{}, errRentalCloseNonceEvidence
		}
		return tx.Hash(), nil
	}
	return common.Hash{}, errRentalCloseNonceEvidence
}
