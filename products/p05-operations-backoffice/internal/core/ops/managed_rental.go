package ops

import (
	"context"
	"crypto/ecdsa"
	"errors"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
)

type RentalChain interface {
	AccountOf(context.Context, common.Address) (core.Account, error)
	FinalizedTime(context.Context) (uint64, error)
	CloseAccount(context.Context, *ecdsa.PrivateKey, common.Address) (common.Hash, error)
	ExecuteRentalWithdrawal(context.Context, *ecdsa.PrivateKey, common.Address) (common.Hash, error)
	RentalWithdrawalProof(context.Context, common.Address, common.Address) (common.Hash, bool, error)
}

type ManagedRental struct {
	Device      common.Address
	Destination common.Address
	Key         *ecdsa.PrivateKey
	Chain       RentalChain
}

type RentalOpening struct {
	RequestKey  string                   `json:"requestKey"`
	Amount      string                   `json:"amount"`
	TxHash      common.Hash              `json:"txHash"`
	Finalized   *core.RentalDepositBlock `json:"finalized,omitempty"`
	Device      common.Address           `json:"-"`
	Destination common.Address           `json:"-"`
}

type RentalObservation struct {
	Status             string         `json:"status"`
	Device             common.Address `json:"device"`
	Destination        common.Address `json:"destination"`
	Balance            string         `json:"balance,omitempty"`
	PerPaymentLimit    string         `json:"perPaymentLimit,omitempty"`
	DailyLimit         string         `json:"dailyLimit,omitempty"`
	WithdrawAfter      uint64         `json:"withdrawAfter,omitempty"`
	WithdrawalEvidence *common.Hash   `json:"withdrawalEvidence,omitempty"`
	Submitted          *common.Hash   `json:"submitted,omitempty"`
	Opening            *RentalOpening `json:"opening,omitempty"`
}

func managedRentalStatus(account core.Account, destination common.Address, chainTime uint64, withdrawalProven, openPayment bool) string {
	switch {
	case !account.Exists:
		return "unopened"
	case account.WithdrawAddress != destination:
		return "wrong-destination"
	case openPayment:
		return "payment-hold"
	case account.ClosedAt == 0:
		return "active"
	case account.Balance == nil || account.PendingWithdrawal == nil:
		return "withdrawal-evidence"
	case account.PendingWithdrawal.Sign() > 0 && account.WithdrawAfter > chainTime:
		return "withdrawal-delay"
	case account.PendingWithdrawal.Sign() > 0:
		return "withdrawal-ready"
	case account.Balance.Sign() == 0 && withdrawalProven:
		return "cleanup-ready"
	default:
		return "withdrawal-evidence"
	}
}

// Observe recomputes authorization conditions from the finalized legacy account.
func (r ManagedRental) Observe(ctx context.Context, paymentHold bool) (RentalObservation, error) {
	observation := RentalObservation{Device: r.Device, Destination: r.Destination}
	if r.Device == (common.Address{}) || r.Destination == (common.Address{}) {
		return observation, errors.New("rental enrollment required")
	}
	account, err := r.Chain.AccountOf(ctx, r.Device)
	if err != nil {
		return observation, err
	}
	clock, err := r.Chain.FinalizedTime(ctx)
	if err != nil {
		return observation, err
	}
	observation.WithdrawAfter = account.WithdrawAfter
	if account.Balance != nil {
		observation.Balance = account.Balance.String()
	}
	if account.PerPaymentLimit != nil {
		observation.PerPaymentLimit = account.PerPaymentLimit.String()
	}
	if account.DailyLimit != nil {
		observation.DailyLimit = account.DailyLimit.String()
	}
	proven := false
	if account.ClosedAt != 0 && account.WithdrawAddress == r.Destination && account.Balance != nil && account.Balance.Sign() == 0 {
		var reference common.Hash
		reference, proven, err = r.Chain.RentalWithdrawalProof(ctx, r.Device, r.Destination)
		if err != nil {
			return observation, err
		}
		if proven {
			observation.WithdrawalEvidence = &reference
		}
	}
	observation.Status = managedRentalStatus(account, r.Destination, clock, proven, paymentHold)
	return observation, nil
}

// Submit can start a close or execute its already-enrolled withdrawal. The
// caller persists its command identity before entering this spending boundary.
func (r ManagedRental) Submit(ctx context.Context, action string, paymentHold bool) (RentalObservation, error) {
	view, err := r.Observe(ctx, paymentHold)
	if err != nil {
		return view, err
	}
	var hash common.Hash
	switch {
	case action == "close" && view.Status == "active":
		hash, err = r.Chain.CloseAccount(ctx, r.Key, r.Device)
	case action == "withdraw" && view.Status == "withdrawal-ready":
		hash, err = r.Chain.ExecuteRentalWithdrawal(ctx, r.Key, r.Device)
	default:
		return view, errors.New("rental transition held")
	}
	if hash != (common.Hash{}) {
		view.Submitted = &hash
	}
	return view, err
}
