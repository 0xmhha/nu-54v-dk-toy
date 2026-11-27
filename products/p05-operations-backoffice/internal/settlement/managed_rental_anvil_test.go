package settlement

import (
	"bytes"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1registry "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/registry"
	v1 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func TestLoopbackManagedRentalFinalWithdrawal(t *testing.T) {
	url := os.Getenv("P05_SETTLEMENT_TEST_RPC")
	if url == "" {
		t.Skip("requires test/settlement/anvil.sh")
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatal("only a loopback fixture is allowed")
	}
	ctx := t.Context()
	client, err := ethclient.DialContext(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	operatorKey, err := crypto.ToECDSA(append(make([]byte, 31), 100))
	if err != nil {
		t.Fatal(err)
	}
	operator, err := bind.NewKeyedTransactorWithChainID(operatorKey, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	operator.Context = ctx
	operator.GasLimit = 10_000_000
	if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", operator.From, "0x56BC75E2D63100000"); err != nil {
		t.Fatal(err)
	}
	artifacts := os.Getenv("NU54_RENTAL_CONTRACT_OUT")
	if artifacts == "" {
		artifacts = "../../../p06-stablenet-contracts/out"
	}
	deploy := func(name string, values ...any) (common.Address, uint64) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(artifacts, name+".sol", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var artifact struct {
			ABI      json.RawMessage
			Bytecode struct{ Object hexutil.Bytes }
		}
		if err := json.Unmarshal(raw, &artifact); err != nil {
			t.Fatal(err)
		}
		contract, err := abi.JSON(bytes.NewReader(artifact.ABI))
		if err != nil {
			t.Fatal(err)
		}
		address, transaction, _, err := bind.DeployContract(operator, contract, artifact.Bytecode.Object, client, values...)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := bind.WaitMined(ctx, client, transaction)
		if err != nil || receipt.Status != 1 {
			t.Fatalf("legacy fixture deployment failed: %v", err)
		}
		return address, receipt.BlockNumber.Uint64()
	}
	token, _ := deploy("TestUSDC", operator.From)
	registry, _ := deploy("MerchantRegistry", operator.From, big.NewInt(300))
	settlement, block := deploy("PaymentSettlement", token, registry, operator.From, big.NewInt(20), big.NewInt(40), big.NewInt(300), big.NewInt(60))
	deployment := core.Deployment{ChainID: 8283, Registry: registry, Settlement: settlement, Token: token, SettlementBlock: block}
	chain, err := core.Dial(ctx, url, deployment)
	if err != nil {
		t.Fatal(err)
	}
	mine(t, ctx, client, 5)
	if dist := os.Getenv("NU54_RENTAL_OPENING_BROWSER_DIST"); dist != "" {
		t.Run("ProtectedReturnBrowser", func(t *testing.T) {
			verifyRentalOpeningBrowser(t, url, chain, client, operatorKey, deployment, dist, true)
		})
		t.Run("OpeningReviewBrowser", func(t *testing.T) {
			verifyRentalOpeningBrowser(t, url, chain, client, operatorKey, deployment, dist, false)
		})
	}
	if os.Getenv("NU54_PROTECTED_WITHDRAWAL_RECEIPTS") == "1" {
		t.Run("ProtectedWithdrawalReceipts", func(t *testing.T) {
			verifyProtectedWithdrawalReceipts(t, ctx, client, chain, deployment, operatorKey)
		})
		return
	}
	rental := ops.ManagedRental{Device: fixtureSigner(105).Address(), Destination: fixtureSigner(106).Address(),
		Chain: chain, Key: operatorKey}
	show := func(want string, hold bool) ops.RentalObservation {
		t.Helper()
		view, err := rental.Observe(ctx, hold)
		if err != nil || view.Status != want {
			t.Fatalf("%s: %+v %v", want, view, err)
		}
		return view
	}
	show("unopened", false)
	perPayment, daily, err := chain.RentalOpenLimits(ctx)
	if err != nil || perPayment.String() != "20" || daily.String() != "40" {
		t.Fatalf("pre-opening caps disagreed with finalized contract: %v %v %v", perPayment, daily, err)
	}
	if _, err := chain.MintTestToken(ctx, operatorKey, operator.From, big.NewInt(50)); err != nil {
		t.Fatal(err)
	}
	amount := big.NewInt(25)
	deposit, err := chain.Deposit(ctx, operatorKey, rental.Device, amount, rental.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if pending, err := chain.RentalOpeningBlock(ctx, deposit, rental.Device, rental.Destination, amount); pending != nil || err != nil {
		t.Fatalf("deposit presented as final before confirmation: %v %v", pending, err)
	}
	mine(t, ctx, client, 5)
	opening, err := chain.RentalOpeningBlock(ctx, deposit, rental.Device, rental.Destination, amount)
	receipt, receiptErr := client.TransactionReceipt(ctx, deposit)
	if err != nil || receiptErr != nil || opening == nil || opening.Height != receipt.BlockNumber.Uint64() || opening.Hash != receipt.BlockHash {
		t.Fatalf("original deposit proof unavailable: %v %v %v", opening, err, receiptErr)
	}
	for _, input := range []struct {
		amount *big.Int
		dest   common.Address
	}{{big.NewInt(26), rental.Destination}, {amount, fixtureSigner(107).Address()}} {
		if wrong, err := chain.RentalOpeningBlock(ctx, deposit, rental.Device, input.dest, input.amount); wrong != nil || err != nil {
			t.Fatalf("proved a different deposit: %v %v", wrong, err)
		}
	}
	t.Logf("original open deposit %s proven in canonical block %s at %d", deposit.Hex(), opening.Hash.Hex(), opening.Height)
	if view := show("active", false); view.Balance != "25" || view.DailyLimit != "40" {
		t.Fatal(view)
	}
	if _, err := rental.Submit(ctx, "close", true); err == nil {
		t.Fatal("close accepted despite pending payment")
	}
	closeView, err := rental.Submit(ctx, "close", false)
	if err != nil || closeView.Submitted == nil {
		t.Fatalf("close not broadcast: %+v %v", closeView, err)
	}
	mine(t, ctx, client, 5)
	delay := show("withdrawal-delay", false)
	if _, err := rental.Submit(ctx, "withdraw", false); err == nil {
		t.Fatal("early withdrawal broadcast")
	}
	forceTime(t, ctx, client, int64(delay.WithdrawAfter)+1)
	show("withdrawal-ready", false)
	routed, err := rental.Submit(ctx, "withdraw", false)
	if err != nil || routed.Submitted == nil {
		t.Fatalf("withdrawal missing: %+v %v", routed, err)
	}
	mine(t, ctx, client, 5)
	final := show("cleanup-ready", false)
	if final.WithdrawalEvidence == nil || *final.WithdrawalEvidence != *routed.Submitted {
		t.Fatal("net receipt not seen")
	}
	show("payment-hold", true)
	rental.Destination = fixtureSigner(107).Address()
	show("wrong-destination", false)
	t.Run("LegacyCashOutChangedAmount", func(t *testing.T) {
		payer, err := crypto.ToECDSA(append(make([]byte, 31), 109))
		if err != nil {
			t.Fatal(err)
		}
		merchant, payout := fixtureSigner(110).Address(), fixtureSigner(111).Address()
		contract, err := v1.NewSettlement(settlement, client)
		if err != nil {
			t.Fatal(err)
		}
		registrar, err := v1registry.NewRegistry(registry, client)
		if err != nil {
			t.Fatal(err)
		}
		mined := func(tx *types.Transaction, err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := bind.WaitMined(ctx, client, tx)
			if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
				t.Fatalf("legacy transaction failed: %v", err)
			}
		}
		mined(registrar.RegisterMerchant(operator, merchant, payout))
		if _, err := chain.Deposit(ctx, operatorKey, crypto.PubkeyToAddress(payer.PublicKey), big.NewInt(20), payout); err != nil {
			t.Fatal(err)
		}
		domain, err := contract.DomainSeparator(&bind.CallOpts{Context: ctx})
		if err != nil {
			t.Fatal(err)
		}
		functions, err := v1.SettlementMetaData.GetAbi()
		if err != nil {
			t.Fatal(err)
		}
		pay := func(nonce byte, amount int64) {
			t.Helper()
			head, err := client.HeaderByNumber(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			auth := v1.PaymentTypesPaymentAuthorization{ChainId: big.NewInt(8283), ContractAddress: settlement,
				Merchant: merchant, Payout: payout, Token: token, Amount: big.NewInt(amount),
				OrderId: [32]byte{31: nonce}, Nonce: big.NewInt(int64(nonce)), Expiry: head.Time + 45}
			packed, err := (functions.Methods["settle"].Inputs[:1]).Pack(auth)
			if err != nil {
				t.Fatal(err)
			}
			typeHash := crypto.Keccak256([]byte(protocol.EncodeType["PaymentAuthorization"]))
			digest := crypto.Keccak256([]byte{0x19, 0x01}, domain[:], crypto.Keccak256(typeHash, packed))
			signature, err := crypto.Sign(digest, payer)
			if err != nil {
				t.Fatal(err)
			}
			signature[64] += 27
			mined(contract.Settle(operator, auth, signature))
		}
		pay(1, 7)
		review, err := chain.LegacyLedgerLatest(ctx, merchant)
		if err != nil || review.Balance != "7" || review.Payout != payout {
			t.Fatalf("expected original review 7: %+v %v", review, err)
		}
		cashoutKey, err := crypto.ToECDSA(append(make([]byte, 31), 112))
		if err != nil {
			t.Fatal(err)
		}
		cashoutSigner := crypto.PubkeyToAddress(cashoutKey.PublicKey)
		if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", cashoutSigner, "0x56BC75E2D63100000"); err != nil {
			t.Fatal(err)
		}
		candidate, err := chain.PrepareLegacyCashOut(ctx, cashoutKey, merchant)
		if err != nil {
			t.Fatal(err)
		}
		pending, err := chain.ObserveLegacyCashOut(ctx, cashoutSigner, merchant, candidate, false)
		if err != nil || pending.Status != "pending" {
			t.Fatalf("expected permitted read without broadcasting: %+v %v", pending, err)
		}
		// A separate settlement can arrive after the review read and before mining.
		pay(2, 2)
		if _, err := chain.ObserveLegacyCashOut(ctx, cashoutSigner, merchant, candidate, true); err != nil {
			t.Fatal(err)
		}
		mine(t, ctx, client, 5)
		confirmed, err := chain.ObserveLegacyCashOut(ctx, cashoutSigner, merchant, candidate, false)
		if err != nil || confirmed.Status != "verified" || confirmed.Amount != "9" || confirmed.Payout != payout.Hex() {
			t.Fatalf("expected differing amount recorded in the actual finalized payout: %+v %v", confirmed, err)
		}
		if os.Getenv("NU54_LEGACY_BROWSER_SCRIPT") != "" {
			pay(3, 3)
			mine(t, ctx, client, 5)
			runLegacyCashoutBrowser(t, legacyBrowserChain{chain, cashoutKey, merchant}, func() { mine(t, ctx, client, 5) })
		}

	})

}

// Assert the exact interface used by the normal operator controller.
var _ ops.RentalChain = (*core.Chain)(nil)
