package commerce

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/quotev2"
)

func TestBuildCanonicalEscrowQuote(t *testing.T) {
	profile := FixtureProfile()
	order := FixtureOrder()
	raw, err := BuildUnsigned(order, profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := quotev2.Digest(raw); err != nil {
		t.Fatal(err)
	}
	order.Amount = "9.5"
	if _, err := BuildUnsigned(order, profile); err == nil {
		t.Fatal("fractional amount entered signed quote")
	}
}

func FixtureProfile() Profile {
	return Profile{Chain: 8283,
		Contract: common.HexToAddress("0x1001"), Asset: common.HexToAddress("0x2001"), Beneficiary: common.HexToAddress("0x3001"),
		MerchantID: [32]byte{1}, CommitmentKey: [32]byte{2}, KeyVersion: 1, BeneficiaryVersion: 1}
}

func FixtureOrder() QuoteOrder {
	return QuoteOrder{Tenant: "80d39491-fa9b-4f35-9c2a-f6a258dd45d6", Branch: "987e50a6-a874-487c-8d85-1451edacfe99",
		Sale: "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb", QuoteID: [32]byte{3}, OrderID: [32]byte{4},
		Claim: [32]byte{5}, Intent: [32]byte{6}, Nonce: [32]byte{7}, Payer: common.HexToAddress("0x4001"),
		PayerGeneration: 1, PayerRole: 1, Amount: "9", Issued: time.Unix(1600, 0), Expires: time.Unix(1630, 0)}
}
