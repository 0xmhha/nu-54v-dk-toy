package settlement

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func verifyProtectedWithdrawalReceipts(t *testing.T, ctx context.Context, client *ethclient.Client, chain *core.Chain,
	deployment core.Deployment, operator *ecdsa.PrivateKey) {
	t.Helper()
	owner := crypto.PubkeyToAddress(operator.PublicKey)
	if _, err := chain.MintTestToken(ctx, operator, owner, big.NewInt(42)); err != nil {
		t.Fatal(err)
	}
	token, err := testusdc.NewTestusdc(deployment.Token, client)
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range []struct {
		amount              int64
		device, destination byte
		hold                bool
	}{{0, 200, 201, false}, {17, 202, 203, false}, {25, 204, 205, true}} {
		deviceKey, _ := crypto.ToECDSA(append(make([]byte, 31), sc.device))
		destinationKey, _ := crypto.ToECDSA(append(make([]byte, 31), sc.destination))
		device, destination := crypto.PubkeyToAddress(deviceKey.PublicKey), crypto.PubkeyToAddress(destinationKey.PublicKey)
		if _, err := chain.Deposit(ctx, operator, device, big.NewInt(sc.amount), destination); err != nil {
			t.Fatal(err)
		}
		var heldID *big.Int
		if sc.hold {
			if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", destination, "0x56BC75E2D63100000"); err != nil {
				t.Fatal(err)
			}
			recipient, err := chain.Transactor(ctx, destinationKey)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := token.SetRefusalMode(recipient, true); err != nil {
				t.Fatal(err)
			}
			heldID, err = token.NextHeldId(nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		closeHash, err := chain.CloseAccount(ctx, operator, device)
		if err != nil {
			t.Fatal(err)
		}
		show := func(want string) core.ProtectedWithdrawalEvidence {
			t.Helper()
			view, err := chain.ObserveProtectedWithdrawal(ctx, device, destination, closeHash)
			if err != nil || view.State != want || view.CloseHash != closeHash || view.Amount == nil || view.Amount.Int64() != sc.amount {
				t.Fatalf("protected %s: %+v %v", want, view, err)
			}
			return view
		}
		mine(t, ctx, client, 5)
		wrong, err := chain.ObserveProtectedWithdrawal(ctx, device, owner, closeHash)
		if err == nil || wrong.State != "withdrawal-indeterminate" || wrong.Amount != nil {
			t.Fatalf("wrong destination produced a withdrawal: %+v %v", wrong, err)
		}
		if sc.amount == 0 {
			show("withdrawal-no-payout")
			continue
		}
		delay := show("withdrawal-delay")
		forceTime(t, ctx, client, int64(delay.WithdrawAfter)+1)
		show("withdrawal-ready")
		payer := operator
		if !sc.hold {
			// executeWithdrawal is permissionless. A canonically finalized
			// third-party execution still settles the exact original close.
			payer = destinationKey
			if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", destination, "0x56BC75E2D63100000"); err != nil {
				t.Fatal(err)
			}
		}
		payout, err := chain.PrepareProtectedWithdrawal(ctx, payer, device)
		if err != nil {
			t.Fatal(err)
		}
		before, err := client.TransactionReceipt(ctx, payout.Hash())
		if before != nil || err == nil {
			t.Fatal("preparing withdrawal sent a transaction")
		}
		if err := chain.PublishProtectedWithdrawal(ctx, payout); err != nil {
			t.Fatal(err)
		}
		mine(t, ctx, client, 5)
		var acceptance *types.Transaction
		if sc.hold {
			if observed := show("withdrawal-held"); observed.PayoutHash != payout.Hash() {
				t.Fatal("held receipt lost original payout")
			}
			recipient, err := chain.Transactor(ctx, destinationKey)
			if err != nil {
				t.Fatal(err)
			}
			acceptance, err = token.AcceptTransfer(recipient, heldID)
			if err != nil {
				t.Fatal(err)
			}
			mine(t, ctx, client, 5)
		}
		view := show("withdrawal-received")
		if view.PayoutHash != payout.Hash() || (acceptance != nil && view.AcceptanceHash != acceptance.Hash()) {
			t.Fatal("protected view differs from its exact canonical payout/acceptance")
		}
		if !sc.hold {
			verifyProtectedWithdrawalAttemptFailure(t, ctx, client, chain, deployment, payer, device)
		}
	}
}

func verifyProtectedWithdrawalAttemptFailure(t *testing.T, ctx context.Context, client *ethclient.Client, chain *core.Chain,
	deployment core.Deployment, signer *ecdsa.PrivateKey, device common.Address) {
	t.Helper()
	ext, err := settlementext.NewSettlementext(deployment.Settlement, client)
	if err != nil {
		t.Fatal(err)
	}
	opts, err := chain.Transactor(ctx, signer)
	if err != nil {
		t.Fatal(err)
	}
	// The permissionless execution already cleared the original balance.
	// Bypass gas estimation so the deliberate retry produces a real reverted receipt.
	opts.GasLimit = 150000
	reverted, err := ext.ExecuteWithdrawal(opts, device)
	if err != nil {
		t.Fatal(err)
	}
	mine(t, ctx, client, 5)
	failure, err := chain.InspectProtectedWithdrawalAttempt(ctx, opts.From, reverted.Nonce(), reverted.Hash())
	if err != nil || failure.State != "reverted" || failure.Consuming != reverted.Hash() || failure.FinalHeight <= 0 {
		t.Fatalf("canonical original revert was not proven: %+v %v", failure, err)
	}
	replacement := types.NewTx(&types.DynamicFeeTx{ChainID: reverted.ChainId(), Nonce: reverted.Nonce(),
		GasTipCap: reverted.GasTipCap(), GasFeeCap: reverted.GasFeeCap(), Gas: reverted.Gas() + 1,
		To: reverted.To(), Data: reverted.Data(), Value: reverted.Value()})
	other, err := types.SignTx(replacement, types.LatestSignerForChainID(reverted.ChainId()), signer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := chain.InspectProtectedWithdrawalAttempt(ctx, opts.From, other.Nonce(), other.Hash())
	if err != nil || result.State != "superseded" || result.Original != other.Hash() || result.Consuming != reverted.Hash() {
		t.Fatalf("canonical different nonce consumer was not proven: %+v %v", result, err)
	}
}
