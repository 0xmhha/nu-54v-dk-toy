package core

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type LegacyLedger struct {
	Contract common.Address `json:"contract"`
	Asset    common.Address `json:"asset"`
	Merchant common.Address `json:"merchant"`
	Payout   common.Address `json:"registeredPayout"`
	Balance  string         `json:"balance"`
}

func (c *Chain) LegacyLedger(ctx context.Context, merchant common.Address) (LegacyLedger, error) {
	return c.legacyLedgerAt(ctx, merchant, c.read)
}
func (c *Chain) LegacyLedgerLatest(ctx context.Context, merchant common.Address) (LegacyLedger, error) {
	return c.legacyLedgerAt(ctx, merchant, nil)
}
func (c *Chain) legacyLedgerAt(ctx context.Context, merchant common.Address, block *big.Int) (LegacyLedger, error) {
	result := LegacyLedger{Contract: c.dep.Settlement, Asset: c.dep.Token, Merchant: merchant}
	reader, err := settlement.NewSettlement(c.dep.Settlement, c.client)
	if err != nil {
		return result, err
	}
	header, err := c.client.HeaderByNumber(ctx, block)
	if err != nil {
		return result, err
	}
	opts := &bind.CallOpts{Context: ctx, BlockNumber: header.Number}
	balance, err := reader.MerchantBalance(opts, merchant)
	if err != nil {
		return result, err
	}
	registry, err := c.reg.MerchantStatus(opts, merchant)
	if err != nil {
		return result, err
	}
	result.Balance, result.Payout = balance.String(), registry.Payout
	return result, nil
}

func (c *Chain) PrepareLegacyCashOut(ctx context.Context, signer *ecdsa.PrivateKey, merchant common.Address) ([]byte, error) {
	contract, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
	if err != nil {
		return nil, err
	}
	opts, err := c.Transactor(ctx, signer)
	if err != nil {
		return nil, err
	}
	opts.NoSend = true
	tx, err := contract.CashOutFor(opts, merchant)
	if err != nil {
		return nil, err
	}
	return tx.MarshalBinary()
}

type LegacyCashOutObservation struct {
	Status              string `json:"status"`
	Hash                string `json:"transaction,omitempty"`
	Block               string `json:"block,omitempty"`
	Payout              string `json:"payout,omitempty"`
	Amount              string `json:"amount,omitempty"`
	AcceptedTransaction string `json:"acceptedTransaction,omitempty"`
}

func (c *Chain) ObserveLegacyCashOut(ctx context.Context, signer, merchant common.Address, raw []byte, submit bool) (LegacyCashOutObservation, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return LegacyCashOutObservation{}, err
	}
	abi, err := settlementext.SettlementextMetaData.GetAbi()
	if err != nil {
		return LegacyCashOutObservation{}, err
	}
	data, err := abi.Pack("cashOutFor", merchant)
	if err != nil {
		return LegacyCashOutObservation{}, err
	}
	from, err := types.Sender(types.LatestSignerForChainID(new(big.Int).SetUint64(c.dep.ChainID)), &tx)
	if err != nil || !tx.Protected() || from != signer || tx.To() == nil || *tx.To() != c.dep.Settlement ||
		tx.Value().Sign() != 0 || !bytes.Equal(tx.Data(), data) {
		return LegacyCashOutObservation{}, errors.New("legacy cash-out identity differs")
	}
	view := LegacyCashOutObservation{Status: "pending", Hash: tx.Hash().Hex()}
	receipt, err := c.client.TransactionReceipt(ctx, tx.Hash())
	if errors.Is(err, ethereum.NotFound) {
		final, headErr := c.client.HeaderByNumber(ctx, c.read)
		if headErr != nil {
			return view, headErr
		}
		nonce, nonceErr := c.client.NonceAt(ctx, signer, final.Number)
		if nonceErr != nil {
			return view, nonceErr
		}
		if nonce > tx.Nonce() {
			view.Status = "nonce-superseded"
			view.Block = final.Hash().Hex()
			return view, nil
		}
		if !submit {
			return view, nil
		}
		if _, _, err = c.client.TransactionByHash(ctx, tx.Hash()); errors.Is(err, ethereum.NotFound) {
			err = c.client.SendTransaction(ctx, &tx)
		}
		return view, err
	}
	if err != nil {
		return view, err
	}
	final, err := c.client.HeaderByNumber(ctx, c.read)
	if err != nil || final.Number.Cmp(receipt.BlockNumber) < 0 {
		return view, err
	}
	canonical, err := c.client.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil || canonical.Hash() != receipt.BlockHash {
		return view, err
	}
	view.Block = receipt.BlockHash.Hex()
	if receipt.Status != types.ReceiptStatusSuccessful {
		view.Status = "reverted"
		return view, nil
	}
	contract, err := settlement.NewSettlement(c.dep.Settlement, c.client)
	if err != nil {
		return view, err
	}
	for _, log := range receipt.Logs {
		if log.Address != c.dep.Settlement {
			continue
		}
		cashout, err := contract.ParseCashedOut(*log)
		if err != nil || cashout.Merchant != merchant {
			continue
		}
		view.Payout, view.Amount = cashout.Payout.Hex(), cashout.Amount.String()
		view.Status = "no-withdrawal"
		direct := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
		held := crypto.Keccak256Hash([]byte("TransferHeld(uint256,address,address,uint256)"))
		source, recipient := common.BytesToHash(c.dep.Settlement.Bytes()), common.BytesToHash(cashout.Payout.Bytes())
		for _, transfer := range receipt.Logs {
			if transfer.Address != c.dep.Token || len(transfer.Data) != 32 ||
				new(big.Int).SetBytes(transfer.Data).Cmp(cashout.Amount) != 0 {
				continue
			}
			if len(transfer.Topics) == 3 && transfer.Topics[0] == direct &&
				transfer.Topics[1] == source && transfer.Topics[2] == recipient {
				view.Status = "verified"
				break
			}
			if len(transfer.Topics) == 4 && transfer.Topics[0] == held &&
				transfer.Topics[2] == source && transfer.Topics[3] == recipient {
				view.Status = "transfer-held"
				accepted, matched, err := c.verifyAcceptedWithdrawal(ctx, transfer.Topics[1].Big(), cashout.Payout,
					cashout.Amount, receipt.BlockNumber.Uint64(), final.Number.Uint64(), direct)
				if err != nil {
					return view, err
				}
				if matched {
					view.Status, view.AcceptedTransaction = "verified", accepted.Hex()
					break
				}
			}
		}
		return view, nil
	}
	view.Status = "no-withdrawal"
	return view, nil
}
