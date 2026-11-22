package examples

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestVersionedSettlementReceiptConsumers(t *testing.T) {
	raw, err := os.ReadFile("../../abi/vectors/settlement-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ChainID           int
		Asset, Settlement common.Address
		Cases             []struct {
			Case           string
			Status         uint64
			SettlementLogs []types.Log
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ChainID != 8283 || len(fixture.Cases) != 5 {
		t.Fatal("fixture scope changed")
	}
	v1, err := settlement.SettlementMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v1.Events["PaymentSettled"]; !ok {
		t.Fatal("v1 event missing")
	}
	if _, ok := v1.Methods["cashOut"]; !ok {
		t.Fatal("v1 cashOut missing")
	}
	filter, err := settlementv2.NewSettlementv2Filterer(fixture.Settlement, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 4} {
		if test := fixture.Cases[i]; test.Status != 0 || len(test.SettlementLogs) != 0 {
			t.Fatalf("failed transaction contains a settlement event: %s", test.Case)
		}
	}
	first := fixture.Cases[1]
	if first.Status != 1 || len(first.SettlementLogs) != 1 {
		t.Fatal("payment receipt missing")
	}
	paid, err := filter.ParsePaid(first.SettlementLogs[0])
	if err != nil {
		t.Fatal(err)
	}
	if paid.Raw.Address != fixture.Settlement || paid.Asset != fixture.Asset || paid.Amount.Cmp(big.NewInt(30)) != 0 ||
		paid.Nonce.Cmp(big.NewInt(91)) != 0 || paid.PayerGeneration != 1 || paid.Mode != 1 || paid.QuoteDigest == [32]byte{} {
		t.Fatalf("incorrect paid log: %+v", paid)
	}
	for i, amount := range []int64{10, 20} {
		test := fixture.Cases[i+2]
		if test.Status != 1 || len(test.SettlementLogs) != 1 {
			t.Fatal("refund receipt missing")
		}
		got, err := filter.ParseRefunded(test.SettlementLogs[0])
		if err != nil {
			t.Fatal(err)
		}
		if got.Amount.Cmp(big.NewInt(amount)) != 0 || got.OrderId != paid.OrderId || got.Payer != paid.Payer ||
			got.Asset != fixture.Asset || got.MerchantId != paid.MerchantId || got.Raw.Address != fixture.Settlement {
			t.Fatalf("incorrect refund log: %+v", got)
		}
	}
}
