package chainview

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	legacy "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestDecodePinnedDeploymentProfiles(t *testing.T) {
	raw, err := os.ReadFile("../../../../packages/contracts-abi/abi/vectors/settlement-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		ChainID           uint64
		Asset, Settlement common.Address
		Cases             []struct {
			Status         uint64
			SettlementLogs []types.Log
		}
	}
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatal(err)
	}
	v2 := Profile{Version: 2, ChainID: captured.ChainID, Contract: captured.Settlement,
		Asset: captured.Asset, ABIHash: v2ABIHash}
	decoder, err := NewDecoder(v2)
	if err != nil {
		t.Fatal(err)
	}
	payment := captured.Cases[1].SettlementLogs[0]
	paid, err := decoder.Decode(payment, types.ReceiptStatusSuccessful)
	if err != nil || paid.Kind != "paid" || paid.Amount != "30" || paid.Nonce != "91" || paid.PayerGeneration != 1 || paid.Mode != 1 {
		t.Fatalf("wrong v2 Paid: %+v, %v", paid, err)
	}
	refund, err := decoder.Decode(captured.Cases[2].SettlementLogs[0], types.ReceiptStatusSuccessful)
	if err != nil || refund.Kind != "refunded" || refund.Amount != "10" || refund.Order != paid.Order ||
		refund.Merchant != paid.Merchant || refund.Payer != paid.Payer {
		t.Fatalf("wrong v2 Refunded: %+v, %v", refund, err)
	}
	if _, err := decoder.Decode(payment, types.ReceiptStatusFailed); err == nil {
		t.Fatal("reverted receipt accepted")
	}
	wrongContract := payment
	wrongContract.Address = common.HexToAddress("0x1234")
	if _, err := decoder.Decode(wrongContract, types.ReceiptStatusSuccessful); err == nil {
		t.Fatal("different deployment accepted")
	}

	v1 := v2
	v1.Version, v1.ABIHash = 1, v1ABIHash
	old, err := NewDecoder(v1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Decode(payment, types.ReceiptStatusSuccessful); err == nil {
		t.Fatal("v2 event accepted under v1 deployment profile")
	}
	abi, err := legacy.SettlementMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	event := abi.Events["PaymentSettled"]
	data, err := event.Inputs.NonIndexed().Pack(big.NewInt(35), big.NewInt(8))
	if err != nil {
		t.Fatal(err)
	}
	v1Log := payment
	v1Log.Topics = []common.Hash{
		event.ID, common.BytesToHash(common.HexToAddress("0x123").Bytes()), common.HexToHash("0x412"),
		common.BytesToHash(common.HexToAddress("0x845").Bytes()),
	}
	v1Log.Data = data
	observed, err := old.Decode(v1Log, types.ReceiptStatusSuccessful)
	if err != nil || observed.Kind != "paid" || observed.Amount != "35" || observed.Nonce != "8" || observed.Mode != 0 {
		t.Fatalf("wrong v1 Paid: %+v, %v", observed, err)
	}
	if _, err := decoder.Decode(v1Log, types.ReceiptStatusSuccessful); err == nil {
		t.Fatal("v1 event accepted under v2 deployment profile")
	}
	v2.ABIHash = v1ABIHash
	if _, err := NewDecoder(v2); err == nil {
		t.Fatal("version and ABI digest mismatch accepted")
	}
}
