// Command v2fixture produces receipt vectors from a disposable, loopback-only Anvil.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type result struct {
	Case   string       `json:"case"`
	Status uint64       `json:"status"`
	Call   string       `json:"call"`
	Logs   []*types.Log `json:"settlementLogs"`
}

const (
	chain = 8283
	start = 1_800_000_000
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: v2fixture http://127.0.0.1:<isolated-anvil-port>")
	}
	endpoint, err := url.Parse(os.Args[1])
	must(err)
	if endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" ||
		endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" {
		panic("fixture requires a dedicated loopback HTTP endpoint")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(ctx, endpoint.String())
	must(err)
	defer client.Close()
	chainID, err := client.ChainID(ctx)
	must(err)
	require(chainID.Cmp(units(chain)) == 0, "expected chain 8283")
	height, err := client.BlockNumber(ctx)
	must(err)
	require(height == 0, "expected an untouched local chain")

	// These fixed synthetic scalars exist only to reproduce golden logs.
	governor, merchantSigner, payer, refundSigner, beneficiary := account(100), account(101),
		account(102), account(103), account(104)
	for _, address := range []common.Address{governor.From, beneficiary.From} {
		must(client.Client().CallContext(ctx, nil, "anvil_setBalance", address, "0x56BC75E2D63100000"))
	}
	now := uint64(start)
	mined := func(tx *types.Transaction, err error) *types.Receipt {
		must(err)
		receipt, err := bind.WaitMined(ctx, client, tx)
		must(err)
		return receipt
	}
	next := func() {
		now++
		must(client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", now))
	}
	do := func(call func() (*types.Transaction, error)) *types.Receipt {
		next()
		return mined(call())
	}
	ok := func(receipt *types.Receipt) {
		require(receipt.Status == types.ReceiptStatusSuccessful, "transaction failed")
	}

	tokenABI, err := testusdc.TestusdcMetaData.GetAbi()
	must(err)
	next()
	asset, tokenTx, _, err := bind.DeployContract(governor, *tokenABI, bytecode("TestUSDC"), client, governor.From)
	ok(mined(tokenTx, err))
	token, err := testusdc.NewTestusdc(asset, client)
	must(err)
	settlementABI, err := v2.Settlementv2MetaData.GetAbi()
	must(err)
	next()
	address, deployment, _, err := bind.DeployContract(governor, *settlementABI, bytecode("SettlementV2"), client, asset,
		governor.From, false)
	ok(mined(deployment, err))
	settlement, err := v2.NewSettlementv2(address, client)
	must(err)

	merchant := word(50)
	ok(do(func() (*types.Transaction, error) {
		return settlement.RegisterMerchant(governor, merchant, merchantSigner.From, beneficiary.From, refundSigner.From)
	}))
	ok(do(func() (*types.Transaction, error) {
		return settlement.RegisterPayer(governor, payer.From, 1, 2, payer.From)
	}))
	ok(do(func() (*types.Transaction, error) { return token.Mint(governor, governor.From, units(60)) }))
	ok(do(func() (*types.Transaction, error) { return token.Approve(governor, address, units(60)) }))
	ok(do(func() (*types.Transaction, error) {
		return settlement.DepositEscrow(governor, payer.From, 1, units(30))
	}))
	ok(do(func() (*types.Transaction, error) { return settlement.FundRefundReserve(governor, merchant, units(30)) }))

	record := func(label, method string, receipt *types.Receipt, expected uint64) result {
		require(receipt.Status == expected, label+" unexpected receipt status")
		out := result{Case: label, Status: receipt.Status, Call: method, Logs: []*types.Log{}}
		for _, log := range receipt.Logs {
			if log.Address == address {
				out.Logs = append(out.Logs, log)
			}
		}
		return out
	}

	q := v2.PaymentTypesV2MerchantQuote{
		QuoteId: word(1), TenantCommitment: word(2), BranchCommitment: word(3), OrderId: word(4), MerchantId: merchant,
		ClaimCommitment: word(5), IntentCommitment: word(6), Payer: payer.From, Asset: asset, Beneficiary: beneficiary.From,
		Amount: units(30), Nonce: units(91), MerchantKeyVersion: 1, BeneficiaryVersion: 1, PayerGeneration: 1,
		IssuedAt: now, ExpiresAt: now + 120, Mode: 1, PayerRole: 2,
	}
	hash, err := settlement.QuoteDigest(nil, q)
	must(err)
	approval, err := settlement.ApprovalDigest(nil, hash)
	must(err)
	merchantSig, payerSig := sign(101, hash), sign(102, approval)
	pay := func() (*types.Transaction, error) { return settlement.Settle(governor, q, merchantSig, payerSig) }
	ok(do(func() (*types.Transaction, error) { return token.SetRefusalMode(beneficiary, true) }))
	held := record("held-payment-atomic-rollback", "settle", do(pay), 0)
	credit, err := settlement.EscrowBalance(nil, payer.From, 1)
	must(err)
	order, err := settlement.Payments(nil, merchant, q.OrderId)
	must(err)
	require(credit.Cmp(units(30)) == 0 && order.Amount.Sign() == 0 && len(held.Logs) == 0, "held transfer mutated state")
	ok(do(func() (*types.Transaction, error) { return token.SetRefusalMode(beneficiary, false) }))
	paid := record("paid-exact", "settle", do(pay), 1)
	require(len(paid.Logs) == 1, "missing Paid")
	credit, err = settlement.EscrowBalance(nil, payer.From, 1)
	must(err)
	received, err := token.BalanceOf(nil, beneficiary.From)
	must(err)
	require(credit.Sign() == 0 && received.Cmp(units(30)) == 0, "payment is not exact")
	refund := func(amount int64, id byte, status uint64, label string) result {
		a := v2.PaymentTypesV2RefundAuthorization{
			MerchantId: merchant, OrderId: q.OrderId, RefundId: word(id), Amount: units(amount), KeyVersion: 1,
			ExpiresAt: now + 120,
		}
		digest, err := settlement.RefundDigest(nil, a)
		must(err)
		return record(label, "refund", do(func() (*types.Transaction, error) {
			return settlement.Refund(governor, a, sign(103, digest))
		}), status)
	}
	all := []result{held, paid, refund(10, 10, 1, "refund-10"), refund(20, 11, 1, "refund-20"),
		refund(1, 12, 0, "refund-excess-1")}
	released, err := settlement.EscrowBalance(nil, payer.From, 1)
	must(err)
	reserve, err := settlement.RefundReserve(nil, merchant)
	must(err)
	order, err = settlement.Payments(nil, merchant, q.OrderId)
	must(err)
	require(released.Cmp(units(30)) == 0 && reserve.Sign() == 0 && order.Refunded.Cmp(units(30)) == 0,
		"refund and reserve mismatch")
	require(len(all[2].Logs) == 1 && len(all[3].Logs) == 1 && len(all[4].Logs) == 0, "unexpected refund logs")
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	must(encoder.Encode(struct {
		ChainID    int            `json:"chainId"`
		Asset      common.Address `json:"asset"`
		Settlement common.Address `json:"settlement"`
		Cases      []result       `json:"cases"`
	}{chain, asset, address, all}))
}

func account(value byte) *bind.TransactOpts {
	key, err := crypto.ToECDSA(append(make([]byte, 31), value))
	must(err)
	auth, err := bind.NewKeyedTransactorWithChainID(key, units(chain))
	must(err)
	auth.GasLimit = 10_000_000
	return auth
}

func sign(value byte, digest common.Hash) []byte {
	key, err := crypto.ToECDSA(append(make([]byte, 31), value))
	must(err)
	sig, err := crypto.Sign(digest[:], key)
	must(err)
	sig[64] += 27
	return sig
}

func bytecode(name string) []byte {
	file := filepath.Join(cmp.Or(os.Getenv("FOUNDRY_OUT"), "products/p06-stablenet-contracts/out"), name+".sol", name+".json")
	raw, err := os.ReadFile(file)
	must(err)
	var artifact struct {
		Bytecode struct{ Object hexutil.Bytes }
	}
	must(json.Unmarshal(raw, &artifact))
	require(len(artifact.Bytecode.Object) > 0, "empty creation bytecode")
	return artifact.Bytecode.Object
}

func word(value byte) [32]byte   { return [32]byte{31: value} }
func units(value int64) *big.Int { return big.NewInt(value) }
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func require(condition bool, message string) {
	if !condition {
		panic(message)
	}
}
