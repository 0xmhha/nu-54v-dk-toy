package commerce

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

var testWon = Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}

func TestTokenIsWholeOrAbsent(t *testing.T) {
	for _, good := range []Token{{}, DollarToken, testWon} {
		if !good.Valid() {
			t.Errorf("%+v refused", good)
		}
	}
	for _, change := range []func(*Token){
		func(v *Token) { v.Name = "" }, func(v *Token) { v.Name = " NU54" }, func(v *Token) { v.Name = "원" },
		func(v *Token) { v.Symbol = "" }, func(v *Token) { v.Symbol = "t KRW" }, func(v *Token) { v.Label = "" },
		func(v *Token) { v.Label = "<b>" }, func(v *Token) { v.Decimals = 19 },
	} {
		bad := testWon
		change(&bad)
		if bad.Valid() {
			t.Errorf("%+v accepted", bad)
		}
	}
	if (Token{}).OrDollar() != DollarToken || testWon.OrDollar() != testWon {
		t.Error("wrong default token")
	}
}

func TestTokenIsOutsideTheIssuanceDigest(t *testing.T) {
	profile := Profile{Chain: 8283, Asset: common.HexToAddress("0xa1")}
	named := profile
	named.Token = testWon
	if profile.Digest() != named.Digest() {
		t.Fatal("naming the token changed the issuance digest")
	}
}

func TestOperatorPricesUseTheBranchToken(t *testing.T) {
	asset := common.HexToAddress("0xa1")
	h := HTTP{Profiles: map[string]Profile{"usd": {Chain: 8283, Asset: asset}, "won": {Chain: 8283, Asset: asset, Token: testWon}}}
	usd, won := h.assetForOperations("usd"), h.assetForOperations("won")
	if usd == nil || usd.Symbol != "TestUSDC" || usd.Decimals != 6 {
		t.Fatalf("dollar branch: %+v", usd)
	}
	if won == nil || won.Symbol != "tKRW" || won.Decimals != 2 {
		t.Fatalf("won branch: %+v", won)
	}
	row := operatorRow{DisplayPrice: "6500", Tax: "0", Discount: "500"}
	if err := row.resolvePrice(won.Decimals); err != nil || row.Price != "600000" {
		t.Fatalf("6,000 won is %s units: %v", row.Price, err)
	}
	row = operatorRow{DisplayPrice: "6500.001"}
	if row.resolvePrice(won.Decimals) == nil {
		t.Fatal("a third decimal was accepted")
	}
}
