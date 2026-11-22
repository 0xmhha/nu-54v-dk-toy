package core

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func (c *Chain) ExecuteRentalWithdrawal(ctx context.Context, operator *ecdsa.PrivateKey, device common.Address) (common.Hash, error) {
	contract, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.Transactor(ctx, operator)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := contract.ExecuteWithdrawal(opts, device)
	return c.send(ctx, "executeWithdrawal", tx, err)
}

// RentalWithdrawalProof checks the finalized close and exact token transfer to the enrolled
// withdrawal destination. A zero balance is insufficient on its own.
func (c *Chain) RentalWithdrawalProof(ctx context.Context, device, destination common.Address) (common.Hash, bool, error) {
	first, closeHash, ok, err := c.AccountClosedBlock(ctx, device)
	if err != nil || !ok {
		return common.Hash{}, false, err
	}
	contract, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
	if err != nil {
		return common.Hash{}, false, err
	}
	header, err := c.client.HeaderByNumber(ctx, c.read)
	if err != nil {
		return common.Hash{}, false, err
	}
	last := header.Number.Uint64()
	requests, err := contract.FilterWithdrawalRequested(&bind.FilterOpts{Start: first, End: &last, Context: ctx}, []common.Address{device})
	if err != nil {
		return common.Hash{}, false, err
	}
	defer requests.Close()
	var amount *big.Int
	for requests.Next() {
		if requests.Event.Raw.TxHash == closeHash {
			amount = requests.Event.Amount
			break
		}
	}
	if err := requests.Error(); err != nil {
		return common.Hash{}, false, err
	}
	if amount == nil {
		return common.Hash{}, false, errors.New("closed rental lacks withdrawal intent")
	}
	if amount.Sign() == 0 {
		return closeHash, true, nil
	}
	withdrawals, err := contract.FilterWithdrawalExecuted(&bind.FilterOpts{Start: first, End: &last, Context: ctx}, []common.Address{device})
	if err != nil {
		return common.Hash{}, false, err
	}
	defer withdrawals.Close()
	transfer := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	held := crypto.Keccak256Hash([]byte("TransferHeld(uint256,address,address,uint256)"))
	source, recipient := common.BytesToHash(c.dep.Settlement.Bytes()), common.BytesToHash(destination.Bytes())
	for withdrawals.Next() {
		event := withdrawals.Event
		if event.Amount.Cmp(amount) != 0 {
			continue
		}
		receipt, err := c.client.TransactionReceipt(ctx, event.Raw.TxHash)
		if err != nil {
			return common.Hash{}, false, err
		}
		if receipt.Status != types.ReceiptStatusSuccessful || receipt.BlockHash != event.Raw.BlockHash {
			continue
		}
		for _, log := range receipt.Logs {
			if log.Address == c.dep.Token && len(log.Topics) == 3 && log.Topics[0] == transfer &&
				log.Topics[1] == source && log.Topics[2] == recipient && len(log.Data) == 32 &&
				new(big.Int).SetBytes(log.Data).Cmp(amount) == 0 {
				return receipt.TxHash, true, nil
			}
			if log.Address == c.dep.Token && len(log.Topics) == 4 && log.Topics[0] == held &&
				log.Topics[2] == source && log.Topics[3] == recipient && len(log.Data) == 32 &&
				new(big.Int).SetBytes(log.Data).Cmp(amount) == 0 {
				accepted, matched, err := c.verifyAcceptedWithdrawal(ctx, log.Topics[1].Big(), destination, amount,
					log.BlockNumber, last, transfer)
				if err != nil || matched {
					return accepted, matched, err
				}
			}
		}
	}
	return common.Hash{}, false, withdrawals.Error()
}

func (c *Chain) verifyAcceptedWithdrawal(ctx context.Context, id *big.Int, destination common.Address, amount *big.Int,
	first, last uint64, transfer common.Hash) (common.Hash, bool, error) {
	token, err := testusdc.NewTestusdc(c.dep.Token, c.client)
	if err != nil {
		return common.Hash{}, false, err
	}
	events, err := token.FilterTransferAccepted(&bind.FilterOpts{Start: first, End: &last, Context: ctx}, []*big.Int{id},
		[]common.Address{c.dep.Settlement}, []common.Address{destination})
	if err != nil {
		return common.Hash{}, false, err
	}
	defer events.Close()
	for events.Next() {
		event := events.Event
		if event.Amount.Cmp(amount) != 0 {
			continue
		}
		receipt, err := c.client.TransactionReceipt(ctx, event.Raw.TxHash)
		if err != nil {
			return common.Hash{}, false, err
		}
		if receipt.Status != types.ReceiptStatusSuccessful || receipt.BlockHash != event.Raw.BlockHash {
			continue
		}
		for _, log := range receipt.Logs {
			if log.Address == c.dep.Token && len(log.Topics) == 3 && log.Topics[0] == transfer &&
				log.Topics[1] == common.BytesToHash(c.dep.Token.Bytes()) && log.Topics[2] == common.BytesToHash(destination.Bytes()) &&
				len(log.Data) == 32 && new(big.Int).SetBytes(log.Data).Cmp(amount) == 0 {
				return receipt.TxHash, true, nil
			}
		}
	}
	return common.Hash{}, false, events.Error()
}
