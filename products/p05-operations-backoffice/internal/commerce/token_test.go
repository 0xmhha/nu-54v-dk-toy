package commerce

import (
	"context"
	"errors"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
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

func TestWonPricesAreWhole(t *testing.T) {
	for _, c := range []struct {
		base, tax, discount string
		ok                  bool
	}{
		{"6500", "", "", true},
		{"6500.00", "0", "500", true},
		{"6500.55", "", "", false},
		{"6500", "0.50", "", false},
		{"6500", "", "0.01", false},
		{"6500.50", "0.50", "", false}, // the total is whole, the parts are not
	} {
		row := operatorRow{DisplayPrice: c.base, Tax: c.tax, Discount: c.discount}
		if err := row.resolvePrice(2); err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		if row.wholePrices(testWon) != c.ok {
			t.Errorf("won %+v", c)
		}
		if !row.wholePrices(Token{}) {
			t.Errorf("dollar branch refused %+v", c)
		}
	}
	if (operatorRow{Price: "650055"}).wholePrices(testWon) || !(operatorRow{Price: "650000"}).wholePrices(testWon) {
		t.Error("a price given in base units is checked the same way")
	}
}

func TestCatalogRowsFollowTheBranch(t *testing.T) {
	asset := common.HexToAddress("0xa1")
	h := HTTP{Profiles: map[string]Profile{"usd": {Chain: 8283, Asset: asset}, "won": {Chain: 8283, Asset: asset, Token: testWon}}}
	for _, c := range []struct {
		branch string
		row    operatorRow
		price  string
		ok     bool
	}{
		{"won", operatorRow{DisplayPrice: "6500"}, "650000", true},
		{"won", operatorRow{DisplayPrice: "6500.55"}, "", false},
		{"won", operatorRow{Price: "650055"}, "", false},
		{"won", operatorRow{Price: "650000"}, "650000", true},
		{"usd", operatorRow{DisplayPrice: "6.50"}, "6500000", true},
		{"usd", operatorRow{DisplayPrice: "6.500001"}, "6500001", true},
		{"usd", operatorRow{Price: "650055"}, "650055", true},
		{"unknown", operatorRow{DisplayPrice: "6.50"}, "", false},
	} {
		row := c.row
		if err := h.resolveRow(c.branch, &row); (err == nil) != c.ok || (c.ok && row.Price != c.price) {
			t.Errorf("%s %+v: %s %v", c.branch, c.row, row.Price, err)
		}
	}
}

func TestPausedBranchTakesNothing(t *testing.T) {
	h := HTTP{Profiles: map[string]Profile{"won": {Chain: 8283, Asset: common.HexToAddress("0xa1"), Token: testWon}}, Paused: map[string]bool{"won": true}}
	bound := service.Bound{Request: service.Request{Domain: "won", Body: map[string]string{}}}
	if _, err := h.issueWithReview(context.Background(), bound, nil); !errors.Is(err, service.Capability) {
		t.Errorf("quote: %v", err)
	}
	row := operatorRow{DisplayPrice: "6500"}
	if err := h.resolveRow("won", &row); !errors.Is(err, service.Capability) {
		t.Errorf("price: %v", err)
	}
}

func TestForgottenDecimalsAreRefused(t *testing.T) {
	token := testWon
	token.Decimals = 0
	if token.Valid() {
		t.Fatal("a token without decimals was accepted")
	}
}

func TestPartsThatCancelOutAreStillRefused(t *testing.T) {
	row := operatorRow{DisplayPrice: "6500", Tax: "0.50", Discount: "0.50"}
	if err := row.resolvePrice(2); err != nil || row.Price != "650000" || row.wholePrices(testWon) {
		t.Fatalf("tax and discount of half a won: %+v %v", row, err)
	}
}
