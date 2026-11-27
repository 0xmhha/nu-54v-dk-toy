package market

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"errors"
	"math/big"
	"regexp"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

var actions = []string{"swap-base", "swap-quote", "lp-add", "lp-remove", "fx-base", "fx-quote", "open-long", "open-short", "close"}
var integer = regexp.MustCompile(`^(0|[1-9][0-9]{0,77})$`)
var maxWord = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

func units(input string) (*big.Int, error) {
	if !integer.MatchString(input) {
		return nil, errors.New("expected canonical unsigned integer units")
	}
	value, _ := new(big.Int).SetString(input, 10)
	if value.Cmp(maxWord) > 0 {
		return nil, errors.New("number overflows unsigned word")
	}
	return value, nil
}

type Order struct {
	User         common.Address
	Action       uint8
	A            *big.Int
	B            *big.Int
	MinA         *big.Int
	MinB         *big.Int
	PriceVersion *big.Int
	Nonce        *big.Int
	Deadline     *big.Int
}

type QuoteRequest struct {
	User   common.Address `json:"user"`
	Action string         `json:"action"`
	A      string         `json:"a"`
	B      string         `json:"b"`
	MinA   string         `json:"minA"`
	MinB   string         `json:"minB"`
}

type Approval struct {
	QuoteRequest
	PriceVersion string         `json:"priceVersion"`
	Price        string         `json:"price"`
	ObservedAt   uint64         `json:"observedAt"`
	Deadline     uint64         `json:"deadline"`
	First        string         `json:"first"`
	Second       string         `json:"second"`
	Digest       common.Hash    `json:"digest"`
	To           common.Address `json:"to"`
	Data         hexutil.Bytes  `json:"data"`
	Nonce        string         `json:"nonce"`
}

func (r QuoteRequest) Order() (Order, error) {
	order := Order{User: r.User}
	if r.User == (common.Address{}) {
		return order, errors.New("missing test wallet")
	}
	found := false
	for i, label := range actions {
		if r.Action == label {
			order.Action = uint8(i)
			found = true
			break
		}
	}
	if !found {
		return order, errors.New("invalid market action")
	}
	for _, field := range []struct {
		in  string
		out **big.Int
	}{{r.A, &order.A}, {r.B, &order.B}, {r.MinA, &order.MinA}, {r.MinB, &order.MinB}} {
		value, err := units(field.in)
		if err != nil {
			return order, err
		}
		*field.out = value
	}
	if order.A.Sign() == 0 {
		return order, errors.New("primary quantity must be positive")
	}
	return order, nil
}

func (c *Chain) Quote(ctx context.Context, signer *ecdsa.PrivateKey, request QuoteRequest) (Approval, error) {
	order, err := request.Order()
	if err != nil {
		return Approval{}, err
	}
	head, err := c.Head(ctx)
	if err != nil {
		return Approval{}, err
	}
	if c.headClock(head.Time) != "current" {
		return Approval{}, errors.New("isolated chain head is not current")
	}
	at := head.Hash()
	expected, err := c.Venue(ctx, at, "quoteSigner")
	if err != nil || expected[0].(common.Address) != crypto.PubkeyToAddress(signer.PublicKey) {
		return Approval{}, errors.New("quote signer differs from market role")
	}
	price, err := c.Venue(ctx, at, "validPrice")
	if err != nil {
		return Approval{}, errors.New("price suspended or stale")
	}
	order.PriceVersion = price[0].(*big.Int)
	oldest := head.Time
	for i := int64(0); i < 2; i++ {
		observation, err := c.Venue(ctx, at, "observations", big.NewInt(i))
		if err != nil {
			return Approval{}, err
		}
		timestamp := observation[1].(*big.Int).Uint64()
		oldest = min(oldest, timestamp)
	}
	deadline := min(head.Time+60, oldest+120)
	if deadline <= head.Time {
		return Approval{}, errors.New("price has no remaining quote lifetime")
	}
	order.Deadline = new(big.Int).SetUint64(deadline)
	order.Nonce, err = rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return Approval{}, err
	}
	preview, err := c.Venue(ctx, at, "preview", order)
	if err != nil {
		return Approval{}, errors.New("order bounds, wallet state or market policy rejected")
	}
	digest := quoteDigest(c.Profile, order)
	signature, err := crypto.Sign(digest[:], signer)
	if err != nil {
		return Approval{}, err
	}
	signature[64] += 27
	data, err := venueABI.Pack("execute", order, signature)
	if err != nil {
		return Approval{}, err
	}
	return Approval{
		QuoteRequest: request, PriceVersion: order.PriceVersion.String(), Price: price[1].(*big.Int).String(),
		ObservedAt: oldest, Deadline: deadline, First: preview[0].(*big.Int).String(), Second: preview[1].(*big.Int).String(),
		Digest: digest, To: c.Profile.Venue.Address, Data: data, Nonce: order.Nonce.String(),
	}, nil
}

// Digest includes deployment bytecode as the EIP-712 domain salt.
func quoteDigest(profile Profile, order Order) common.Hash {
	hash := func(text string) []byte { return crypto.Keccak256([]byte(text)) }
	word := func(data []byte) []byte { return common.LeftPadBytes(data, 32) }
	domain := crypto.Keccak256(
		hash("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)"),
		hash("NU54 Isolated Market"), hash("1"), word(big.NewInt(ChainID).Bytes()), word(profile.Venue.Address.Bytes()),
		profile.Venue.CodeHash.Bytes(),
	)
	message := crypto.Keccak256(
		hash("Quote(address user,uint8 action,uint256 a,uint256 b,uint256 minA,uint256 minB,uint256 priceVersion,uint256 nonce,uint256 deadline)"),
		word(order.User.Bytes()), word([]byte{order.Action}), word(order.A.Bytes()), word(order.B.Bytes()),
		word(order.MinA.Bytes()), word(order.MinB.Bytes()), word(order.PriceVersion.Bytes()), word(order.Nonce.Bytes()),
		word(order.Deadline.Bytes()),
	)
	return crypto.Keccak256Hash([]byte{0x19, 1}, domain, message)
}

func abiOrder(value any) (Order, bool) {
	order, ok := abi.ConvertType(value, new(Order)).(*Order)
	if !ok || int(order.Action) >= len(actions) {
		return Order{}, false
	}
	return *order, true
}
