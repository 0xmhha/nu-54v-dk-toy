package commerce

import (
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

// Valid accepts no token or a complete one.
func (t Token) Valid() bool {
	return t == (Token{}) || (orderescrow.ValidTokenName(t.Name) && tokenSymbol.MatchString(t.Symbol) &&
		tokenSymbol.MatchString(t.Label) && t.Decimals <= 18)
}
