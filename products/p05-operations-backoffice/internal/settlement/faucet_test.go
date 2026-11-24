package settlement

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc3009"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type noSigner struct{}

func (noSigner) SignDigest(context.Context, [32]byte) ([]byte, error) {
	return nil, errors.New("unused")
}

func faucetFixture() (Faucet, map[string]Target) {
	token := common.HexToAddress("0xa1")
	return Faucet{Account: common.HexToAddress("0xfa"), Signer: noSigner{}, Tokens: map[string][]FaucetToken{
			"shop": {{Address: token, Symbol: "tUSDC", Decimals: 6, Amount: "5000000", PerDay: 2, DailyTotal: "20000000"}}}},
		map[string]Target{"shop": {Contract: common.HexToAddress("0xc0"), ABI: EscrowABIHash}, "old": {Contract: common.HexToAddress("0xc1")}}
}

func TestFaucetNeedsEveryLimit(t *testing.T) {
	faucet, targets := faucetFixture()
	if err := CheckFaucet(faucet, targets, common.HexToAddress("0x6a5")); err != nil {
		t.Fatal(err)
	}
	if err := CheckFaucet(faucet, targets, faucet.Account); err == nil {
		t.Fatal("the gas account was accepted as the faucet")
	}
	for name, change := range map[string]func(*FaucetToken){
		"no amount":       func(f *FaucetToken) { f.Amount = "" },
		"zero amount":     func(f *FaucetToken) { f.Amount = "0" },
		"fraction":        func(f *FaucetToken) { f.Amount = "1.5" },
		"no daily claims": func(f *FaucetToken) { f.PerDay = 0 },
		"no total":        func(f *FaucetToken) { f.DailyTotal = "" },
		"total too small": func(f *FaucetToken) { f.DailyTotal = "4999999" },
		"no address":      func(f *FaucetToken) { f.Address = common.Address{} },
		"symbol":          func(f *FaucetToken) { f.Symbol = "t USDC" },
	} {
		row := faucet.Tokens["shop"][0]
		change(&row)
		if CheckFaucet(Faucet{Account: faucet.Account, Signer: noSigner{}, Tokens: map[string][]FaucetToken{"shop": {row}}}, targets) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	row := faucet.Tokens["shop"][0]
	other := row
	other.DailyTotal = "30000000"
	two := map[string]Target{"shop": targets["shop"], "second": targets["shop"]}
	for name, tokens := range map[string]map[string][]FaucetToken{
		"unknown branch": {"none": {row}}, "old payment contract": {"old": {row}}, "same token twice": {"shop": {row, row}},
		"empty list": {"shop": {}}, "two totals": {"shop": {row}, "second": {other}}, "no branch": {},
	} {
		if CheckFaucet(Faucet{Account: faucet.Account, Signer: noSigner{}, Tokens: tokens}, two) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestFaucetIsOffWithoutTokens(t *testing.T) {
	faucet, targets := faucetFixture()
	bound := service.Bound{Subject: "me", Request: service.Request{Domain: "shop", Object: "me", Body: map[string]string{"token": "0x00000000000000000000000000000000000000a2"}}}
	ctx := context.Background()
	if err := (Forwarder{Targets: targets}).ClaimFaucet(ctx, bound); !errors.Is(err, service.Capability) {
		t.Fatalf("no configuration: %v", err)
	}
	if _, err := (Forwarder{Targets: targets}).FaucetStatus(ctx, bound); !errors.Is(err, service.Capability) {
		t.Fatalf("no configuration: %v", err)
	}
	if err := (Forwarder{Targets: targets, Faucet: &faucet}).ClaimFaucet(ctx, bound); !errors.Is(err, service.Capability) {
		t.Fatalf("unlisted token: %v", err)
	}
	bound.Request.Object = "another customer"
	if err := (Forwarder{Targets: targets, Faucet: &faucet}).ClaimFaucet(ctx, bound); !errors.Is(err, service.Forbidden) {
		t.Fatalf("another customer: %v", err)
	}
}

func TestFaucetTransferIsTheStoredOne(t *testing.T) {
	faucet, targets := faucetFixture()
	target := targets["shop"]
	target.Sponsor = common.HexToAddress("0x5b")
	claim := &faucetTransfer{token: common.HexToAddress("0xa1"), sender: faucet.Account, to: common.HexToAddress("0xb0a2d"), amount: big.NewInt(5_000_000)}
	job := work{id: "job", branch: "shop", action: &escrowAction{kind: "faucet", faucet: claim}}
	w := &Worker{Faucet: &faucet, GasAccount: common.HexToAddress("0x6a5"), Now: time.Now}
	account, _, err := w.account(job)
	if err != nil || account != faucet.Account {
		t.Fatalf("sender %s %v", account, err)
	}
	if target.sponsored(job) || destination(job, target) != claim.token || gasFor(job) != faucetGas {
		t.Fatal("a claim must be a direct transfer of the token")
	}
	first, err := claim.call(job.id, nil, w.Now())
	if err != nil {
		t.Fatal(err)
	}
	job.intent, job.expires = first.intent[:], &first.expiry
	if err := w.validateAction(job, target, first.data); err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), first.data...)
	changed[len(changed)-1] ^= 1
	if w.validateAction(job, target, changed) == nil {
		t.Fatal("another amount accepted")
	}
	job.intent = make([]byte, 32)
	if w.validateAction(job, target, first.data) == nil {
		t.Fatal("another claim's intent accepted")
	}
	claim.sender = common.HexToAddress("0xfb")
	if _, _, err := w.account(job); err == nil {
		t.Fatal("a claim of another faucet account was signed")
	}
}

func TestFaucetPaysOnlyWithItsTransferLog(t *testing.T) {
	abi, err := testusdc3009.Testusdc3009MetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	claim := faucetTransfer{token: common.HexToAddress("0xa1"), sender: common.HexToAddress("0xfa"), to: common.HexToAddress("0xb0"), amount: big.NewInt(7)}
	log := func(token, from, to common.Address, amount int64) *types.Receipt {
		return &types.Receipt{Logs: []*types.Log{{Address: token, Topics: []common.Hash{abi.Events["Transfer"].ID, common.BytesToHash(from[:]), common.BytesToHash(to[:])},
			Data: common.LeftPadBytes(big.NewInt(amount).Bytes(), 32)}}}
	}
	if !claim.moved(log(claim.token, claim.sender, claim.to, 7)) {
		t.Fatal("the transfer was not recognized")
	}
	for _, receipt := range []*types.Receipt{{}, log(claim.to, claim.sender, claim.to, 7), log(claim.token, claim.sender, claim.sender, 7), log(claim.token, claim.sender, claim.to, 6)} {
		if claim.moved(receipt) {
			t.Fatal("another transfer was accepted")
		}
	}
}
