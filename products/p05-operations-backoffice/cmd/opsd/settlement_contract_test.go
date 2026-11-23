package main

import (
	"context"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/settlement"
	"github.com/ethereum/go-ethereum/common"
)

func TestBranchUsesOnePaymentContract(t *testing.T) {
	escrow, old := settlement.Target{ABI: settlement.EscrowABIHash}, settlement.Target{}
	for _, c := range []struct {
		profile commerce.Profile
		target  settlement.Target
		ok      bool
	}{
		{commerce.Profile{}, old, true},
		{commerce.Profile{Escrow: true}, escrow, true},
		{commerce.Profile{}, escrow, false},
		{commerce.Profile{Escrow: true}, old, false},
	} {
		err := matchContracts(map[string]commerce.Profile{"b": c.profile}, map[string]settlement.Target{"b": c.target})
		if (err == nil) != c.ok {
			t.Fatalf("%+v: %v", c, err)
		}
	}
	if err := matchContracts(nil, map[string]settlement.Target{"b": escrow}); err != nil {
		t.Fatal(err)
	}
}

func TestDisputeAccountIsSeparate(t *testing.T) {
	gas := common.HexToAddress("0x1001")
	if account, signer, err := loadResolver(context.Background(), nil, gas, nil); err != nil || signer != nil || account != (common.Address{}) {
		t.Fatalf("an absent dispute account must turn the feature off: %v", err)
	}
	for _, value := range []string{"", "0x", gas.Hex(), (common.Address{}).Hex()} {
		t.Setenv("P05_ESCROW_RESOLVER", value)
		if _, _, err := loadResolver(context.Background(), nil, gas, nil); err == nil {
			t.Fatalf("dispute account %q accepted", value)
		}
	}
}
