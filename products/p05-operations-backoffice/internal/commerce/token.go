package commerce

import (
	"math/big"
	"regexp"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
)

// Token is how a branch's payment token signs and is shown. It stays outside Profile.Digest, so the
// quotes of a branch do not change when its token is named.
type Token struct {
	Name     string `json:"name"`   // in the signing domain
	Symbol   string `json:"symbol"` // on chain
	Label    string `json:"label"`  // on screens
	Decimals uint8  `json:"decimals"`
}

// DollarToken is the token of every branch that names none.
var DollarToken = Token{Name: orderescrow.TokenName, Symbol: "tUSDC", Label: "TestUSDC", Decimals: 6}

var tokenSymbol = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,15}$`)

// OrDollar is the token, or DollarToken when the branch names none.
func (t Token) OrDollar() Token {
	if t == (Token{}) {
		return DollarToken
	}
	return t
}

// Valid accepts no token or a complete one. Zero decimals is what a forgotten field reads as, so it is refused.
func (t Token) Valid() bool {
	return t == (Token{}) || (orderescrow.ValidTokenName(t.Name) && tokenSymbol.MatchString(t.Symbol) &&
		tokenSymbol.MatchString(t.Label) && t.Decimals >= 1 && t.Decimals <= 18)
}

// wholePrice accepts an amount only in whole won at a won branch, and anything elsewhere.
func (t Token) wholePrice(baseUnits string) bool {
	return t.OrDollar().Symbol != wonSymbol || multiple(baseUnits, t.Decimals)
}

// shownPrice accepts an operator's price only when every screen shows all of it: whole won, or dollars and cents.
func (t Token) shownPrice(baseUnits string) bool {
	token := t.OrDollar()
	if token.Symbol == wonSymbol {
		return t.wholePrice(baseUnits)
	}
	return token.Decimals <= 2 || multiple(baseUnits, token.Decimals-2)
}

// multiple reports whole units of ten to the given power.
func multiple(baseUnits string, power uint8) bool {
	amount, ok := new(big.Int).SetString(baseUnits, 10)
	unit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(power)), nil)
	return ok && new(big.Int).Mod(amount, unit).Sign() == 0
}

const wonSymbol = "tKRW"
