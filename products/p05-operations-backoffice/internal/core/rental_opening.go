package core

import (
	"bytes"
	"context"
	"errors"
	"math/big"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
)

func (c *Chain) rentalLimit(ctx context.Context, getter string) (*big.Int, error) {
	selector := crypto.Keccak256([]byte(getter))[:4]
	raw, err := c.client.CallContract(ctx, ethereum.CallMsg{To: &c.dep.Settlement, Data: selector}, c.read)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("finalized legacy account limit unavailable")
	}
	return new(big.Int).SetBytes(raw), nil
}

// RentalOpenLimits reads the immutable v1 limits before a device account exists.
func (c *Chain) RentalOpenLimits(ctx context.Context) (perPayment, daily *big.Int, err error) {
	perPayment, err = c.rentalLimit(ctx, "perPaymentCap()")
	if err != nil {
		return nil, nil, err
	}
	daily, err = c.rentalLimit(ctx, "dailyCap()")
	if err != nil {
		return nil, nil, err
	}
	if perPayment.Sign() <= 0 || perPayment.Cmp(daily) > 0 {
		return nil, nil, errors.New("invalid finalized legacy rental limits")
	}
	return perPayment, daily, nil
}

type RentalDepositBlock struct {
	Height uint64      `json:"height"`
	Hash   common.Hash `json:"hash"`
}

// RentalOpeningBlock verifies the exact original deposit and its canonical finality.
func (c *Chain) RentalOpeningBlock(ctx context.Context, txHash common.Hash, device, destination common.Address, amount *big.Int) (*RentalDepositBlock, error) {
	receipt, err := c.client.TransactionReceipt(ctx, txHash)
	if errors.Is(err, ethereum.NotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if receipt.TxHash != txHash || receipt.Status != types.ReceiptStatusSuccessful ||
		receipt.BlockNumber == nil || !receipt.BlockNumber.IsUint64() || receipt.BlockNumber.Uint64() < c.dep.SettlementBlock {
		return nil, nil
	}
	final, err := c.client.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil {
		return nil, err
	}
	if receipt.BlockNumber.Cmp(final.Number) > 0 {
		return nil, nil
	}
	block, err := c.client.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return nil, err
	}
	if block.Hash() != receipt.BlockHash {
		return nil, nil
	}
	tx, pending, err := c.client.TransactionByHash(ctx, txHash)
	if err != nil {
		return nil, err
	}
	if pending || tx.Hash() != txHash || tx.To() == nil || *tx.To() != c.dep.Settlement || tx.Value().Sign() != 0 {
		return nil, nil
	}
	codec, err := settlement.SettlementMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	expected, err := codec.Pack("depositFor", device, amount, destination)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(tx.Data(), expected) {
		return nil, nil
	}
	filter, err := settlement.NewSettlementFilterer(c.dep.Settlement, c.client)
	if err != nil {
		return nil, err
	}
	for _, log := range receipt.Logs {
		if log == nil || log.Address != c.dep.Settlement || log.TxHash != txHash || log.BlockHash != receipt.BlockHash || log.Removed {
			continue
		}
		deposit, err := filter.ParseDeposited(*log)
		if err == nil && deposit.Device == device && deposit.Amount.Cmp(amount) == 0 {
			return &RentalDepositBlock{Height: receipt.BlockNumber.Uint64(), Hash: receipt.BlockHash}, nil
		}
	}
	return nil, nil
}
