package core

import (
	"context"
	"errors"
	"math/big"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type ProtectedWithdrawalAttempt struct {
	State       string
	Original    common.Hash
	Consuming   common.Hash
	FinalHeight int64
	FinalHash   common.Hash
}

// InspectProtectedWithdrawalAttempt classifies terminal failure of the exact
// original. Positive receipt/payout attribution is checked separately against
// the original close, fixed amount and registered token destination.
func (c *Chain) InspectProtectedWithdrawalAttempt(ctx context.Context, signer common.Address, nonce uint64, original common.Hash) (ProtectedWithdrawalAttempt, error) {
	unknown := ProtectedWithdrawalAttempt{State: "unknown", Original: original}
	final, err := c.client.HeaderByNumber(ctx, c.read)
	if err != nil || final.Number == nil || !final.Number.IsInt64() || signer == (common.Address{}) || original == (common.Hash{}) {
		return unknown, errProtectedWithdrawalEvidence
	}
	finish := func(state string, consuming common.Hash) (ProtectedWithdrawalAttempt, error) {
		current, err := c.client.HeaderByNumber(ctx, final.Number)
		if err != nil || current.Hash() != final.Hash() {
			return unknown, errProtectedWithdrawalEvidence
		}
		return ProtectedWithdrawalAttempt{State: state, Original: original, Consuming: consuming,
			FinalHeight: final.Number.Int64(), FinalHash: final.Hash()}, nil
	}
	hint, err := c.client.TransactionReceipt(ctx, original)
	if err != nil && !errors.Is(err, ethereum.NotFound) {
		return unknown, err
	}
	if hint != nil {
		if hint.TxHash != original || hint.BlockNumber == nil || hint.BlockHash == (common.Hash{}) {
			return unknown, errProtectedWithdrawalEvidence
		}
		if hint.BlockNumber.Cmp(final.Number) > 0 {
			return unknown, nil
		}
		block, err := c.client.HeaderByNumber(ctx, hint.BlockNumber)
		if err != nil {
			return unknown, err
		}
		if block.Hash() != hint.BlockHash {
			return unknown, errProtectedWithdrawalEvidence
		}
		checked, err := c.finalizedRentalCloseReceipt(ctx, block, RentalCloseTransaction{Hash: original, Nonce: nonce})
		if err != nil {
			return unknown, err
		}
		if checked.Status == types.ReceiptStatusFailed {
			return finish("reverted", original)
		}
		// A successful receipt alone cannot establish a payout.
		return finish("observed", original)
	}
	tx, _, err := c.client.TransactionByHash(ctx, original)
	if err == nil && tx != nil && tx.Hash() == original {
		return unknown, nil
	}
	if !errors.Is(err, ethereum.NotFound) {
		return unknown, errProtectedWithdrawalEvidence
	}
	currentNonce, err := c.rentalCloseNonceAt(ctx, signer, final)
	if err != nil || currentNonce <= nonce {
		return unknown, err
	}
	consuming, err := c.finalizedRentalCloseNonceUse(ctx, signer, nonce, final)
	if err != nil {
		return unknown, err
	}
	if consuming == original {
		// An index omission is not a failure proof, even if nonce advanced.
		return unknown, nil
	}
	return finish("superseded", consuming)
}

// ProtectedWithdrawalAnchorCurrent checks the stored terminal commitment
// against the canonical chain bounded by the caller's active P07 final head.
func (c *Chain) ProtectedWithdrawalAnchorCurrent(ctx context.Context, height int64, hash common.Hash) bool {
	if height <= 0 || hash == (common.Hash{}) {
		return false
	}
	header, err := c.client.HeaderByNumber(ctx, big.NewInt(height))
	return err == nil && header != nil && header.Hash() == hash
}

// ProtectedRentalOperatorAt resolves PaymentSettlement's immutable operator
// from the original canonical disposition block, independently of opsd's key.
func (c *Chain) ProtectedRentalOperatorAt(ctx context.Context, height int64, hash common.Hash) (common.Address, error) {
	if height <= 0 || hash == (common.Hash{}) {
		return common.Address{}, errProtectedWithdrawalEvidence
	}
	contract, err := settlement.NewSettlementCaller(c.dep.Settlement, c.client)
	if err != nil {
		return common.Address{}, err
	}
	operator, err := contract.Operator(&bind.CallOpts{Context: ctx, BlockHash: hash})
	if err != nil || operator == (common.Address{}) || !c.ProtectedWithdrawalAnchorCurrent(ctx, height, hash) {
		return common.Address{}, errProtectedWithdrawalEvidence
	}
	return operator, nil
}
