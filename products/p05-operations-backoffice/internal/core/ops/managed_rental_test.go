package ops

import (
	"math/big"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
)

func TestManagedRentalRequiresFinalizedDelayAndWithdrawal(t *testing.T) {
	destination := common.HexToAddress("0x606")
	account := core.Account{Exists: true, ClosedAt: 10, Balance: big.NewInt(20),
		PendingWithdrawal: big.NewInt(20), WithdrawAddress: destination, WithdrawAfter: 50}
	if status := managedRentalStatus(account, destination, 49, false, false); status != "withdrawal-delay" {
		t.Fatal(status)
	}
	if status := managedRentalStatus(account, destination, 50, false, false); status != "withdrawal-ready" {
		t.Fatal(status)
	}
	account.Balance.SetInt64(0)
	account.PendingWithdrawal.SetInt64(0)
	if status := managedRentalStatus(account, destination, 52, false, false); status != "withdrawal-evidence" {
		t.Fatal(status)
	}
	if status := managedRentalStatus(account, destination, 52, true, true); status != "payment-hold" {
		t.Fatal(status)
	}
	if status := managedRentalStatus(account, destination, 52, true, false); status != "cleanup-ready" {
		t.Fatal(status)
	}
	account.WithdrawAddress = common.HexToAddress("0xff")
	if status := managedRentalStatus(account, destination, 52, true, false); status != "wrong-destination" {
		t.Fatal(status)
	}
}
