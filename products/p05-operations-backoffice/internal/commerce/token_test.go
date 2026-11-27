package commerce

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	if won == nil || won.Symbol != "tKRW" || won.Decimals != 2 || won.Token != testWon || usd.Token != (Token{}) {
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
		if row.shownPrices(testWon) != c.ok {
			t.Errorf("won %+v", c)
		}
	}
	if (operatorRow{Price: "650055"}).shownPrices(testWon) || !(operatorRow{Price: "650000"}).shownPrices(testWon) {
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
		{"usd", operatorRow{DisplayPrice: "6.500001"}, "", false},
		{"usd", operatorRow{DisplayPrice: "6.50", Tax: "0.001"}, "", false},
		{"usd", operatorRow{DisplayPrice: "6.50", Discount: "0.001"}, "", false},
		{"usd", operatorRow{DisplayPrice: "6.50", Tax: "0.01", Discount: "0.02"}, "6490000", true},
		{"usd", operatorRow{Price: "650055"}, "650055", true},
		{"unknown", operatorRow{DisplayPrice: "6.50"}, "", false},
	} {
		row := c.row
		if err := h.resolveRow(c.branch, &row); (err == nil) != c.ok || (c.ok && row.Price != c.price) {
			t.Errorf("%s %+v: %s %v", c.branch, c.row, row.Price, err)
		}
	}
}

type pausedPayment struct {
	PaymentForwarder
	branch string
}

func (p pausedPayment) Paused(branch string) bool { return branch == p.branch }

func TestPausedBranchTakesNothing(t *testing.T) {
	h := HTTP{Profiles: map[string]Profile{"won": {Chain: 8283, Asset: common.HexToAddress("0xa1"), Token: testWon}}, Payment: pausedPayment{branch: "won"}}
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
	if err := row.resolvePrice(2); err != nil || row.Price != "650000" || row.shownPrices(testWon) {
		t.Fatalf("tax and discount of half a won: %+v %v", row, err)
	}
}

// The requests themselves, up to the point where the ledger would be asked.
func TestPausedBranchRefusesRequests(t *testing.T) {
	cert := &x509.Certificate{Raw: []byte("test store source"), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Minute)}
	sum := sha256.Sum256(cert.Raw)
	asset := "0x2222222222222222222222222222222222222222"
	ask := func(role, body string, payment PaymentForwarder) *httptest.ResponseRecorder {
		principal := service.Principal{Tenant: tenant, Subject: customerA, Role: role, ExpiresAt: time.Now().Add(time.Minute).Unix()}
		if role == "pos" {
			principal.Grants = []service.Grant{{Domain: branch, Object: branch, Operations: []string{"POS/submit"}}}
		}
		h := HTTP{Credentials: map[string]service.Principal{hex.EncodeToString(sum[:]): principal}, Payment: payment,
			Profiles: map[string]Profile{branch: {Chain: 8283, Asset: common.HexToAddress(asset), Token: testWon}}}
		mux := http.NewServeMux()
		h.Register(mux)
		r := httptest.NewRequest(http.MethodPost, "/v2/commerce", strings.NewReader(body))
		r.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	pos := func(amount string) string {
		return `{"pos":{"operation":"submit","branch":"` + branch + `","reference":"S-0142","chainId":8283,"asset":"` + asset + `","amount":"` + amount + `"}}`
	}
	if w := ask("pos", pos("650000"), pausedPayment{branch: branch}); !strings.Contains(w.Body.String(), "UNSUPPORTED_CAPABILITY") {
		t.Errorf("POS order at a paused branch: %d %s", w.Code, w.Body)
	}
	if w := ask("pos", pos("650055"), nil); w.Code != http.StatusBadRequest {
		t.Errorf("POS order of half a won: %d %s", w.Code, w.Body)
	}
	if w := ask("pos", pos("650000"), nil); w.Code != http.StatusServiceUnavailable {
		t.Errorf("a whole-won POS order must reach the ledger: %d %s", w.Code, w.Body)
	}
	auth := authorize(t, actor{customerA, "customer"}, "order.create", branch, customerA, customerA,
		map[string]string{"asset": asset, "quantity": "1", "intentDigest": strings.Repeat("11", 32)})
	command, err := json.Marshal(auth.Request)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"command":` + string(command) + `}`
	if w := ask("customer", body, pausedPayment{branch: branch}); !strings.Contains(w.Body.String(), "UNSUPPORTED_CAPABILITY") {
		t.Errorf("order at a paused branch: %d %s", w.Code, w.Body)
	}
}

func TestCoinCountsForItsBoard(t *testing.T) {
	coin := Token{Name: "WKRC", Symbol: "WKRC", Label: "KRWC", Decimals: 18, BoardDecimals: 2}
	if !coin.Valid() || !coin.IsCoin() || testWon.IsCoin() || DollarToken.IsCoin() {
		t.Fatal("the coin")
	}
	for _, change := range []func(*Token){
		func(v *Token) { v.BoardDecimals = 0 }, func(v *Token) { v.BoardDecimals = 1 }, func(v *Token) { v.BoardDecimals = 9 }, func(v *Token) { v.Decimals = 8 },
		func(v *Token) { v.Label = "WKRC" }, func(v *Token) { v.Symbol = "tKRW" },
	} {
		bad := coin
		change(&bad)
		if bad.Valid() {
			t.Errorf("%+v accepted", bad)
		}
	}
	for price, ok := range map[string]bool{"6500": true, "6500.00": true, "6500.5": false, "6500.000000000000000001": false, "0": false} {
		row := operatorRow{DisplayPrice: price}
		err := row.resolvePrice(18)
		if got := err == nil && row.shownPrices(coin); got != ok {
			t.Errorf("%s: %v %v", price, got, err)
		}
		if ok && row.Price != "6500000000000000000000" {
			t.Errorf("%s: %s", price, row.Price)
		}
	}
	if (operatorRow{Price: "6500000000000000000001"}).shownPrices(coin) || !(operatorRow{Price: "6500000000000000000000"}).shownPrices(coin) {
		t.Error("a price in base units")
	}
}
