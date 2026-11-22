// Package marketlab reviews one isolated 8283 transaction for an independent signer.
package marketlab

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const ChainID = 8283
const QuoteLength = 452
const BoundAllowanceLength = 100

type Policy struct {
	Venue, Base, Quote, Signer    common.Address
	CodeHash, Deployment, Genesis common.Hash
}

type Request struct {
	Deployment      common.Hash
	User, To        common.Address
	Nonce, GasPrice uint64
	Calldata, Offer []byte
}

type Terms struct {
	Chain                uint64
	Digest, OfferDigest  common.Hash
	Action               uint8
	A, B, MinA, MinB     string
	Deadline             uint64
	ExpectedPriceVersion string
	Gas                  uint64
	Allowance            string
}

var actions = [...]string{"swap-base", "swap-quote", "lp-add", "lp-remove", "fx-base", "fx-quote", "open-long", "open-short", "close"}
var capQuantity = new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)

func ActionName(action uint8) string {
	if int(action) >= len(actions) {
		return ""
	}
	return actions[action]
}

func selector(signature string) []byte { return crypto.Keccak256([]byte(signature))[:4] }
func zero(data []byte) bool {
	for _, word := range data {
		if word != 0 {
			return false
		}
	}
	return true
}
func offerDigest(p Policy, offer []byte) common.Hash {
	hash := func(s string) []byte { return crypto.Keccak256([]byte(s)) }
	word := func(n []byte) []byte { return common.LeftPadBytes(n, 32) }
	domain := crypto.Keccak256(hash("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)"),
		hash("NU54 Isolated Market"), hash("1"), word(big.NewInt(ChainID).Bytes()), word(p.Venue[:]), p.CodeHash[:])
	body := crypto.Keccak256(hash("Quote(address user,uint8 action,uint256 a,uint256 b,uint256 minA,uint256 minB,uint256 priceVersion,uint256 nonce,uint256 deadline)"), offer[4:292])
	return crypto.Keccak256Hash([]byte{25, 1}, domain, body)
}

// Valid checks the static isolated venue policy before publishing it.
func (p Policy) Valid() bool {
	addresses := [...]common.Address{p.Venue, p.Base, p.Quote, p.Signer}
	for i, addr := range addresses {
		if addr == (common.Address{}) {
			return false
		}
		for _, earlier := range addresses[:i] {
			if addr == earlier {
				return false
			}
		}
	}
	return p.CodeHash != (common.Hash{}) && p.Genesis != (common.Hash{}) && p.Deployment != (common.Hash{})
}

// Review verifies the signed offer, exact on-chain call and fee bounds before deriving the EIP-155 signing digest.
// Policy is installed independently from transport. The caller authenticates User against its enrolled signer.
func Review(p Policy, r Request, now time.Time) (Terms, error) {
	invalid := func() (Terms, error) { return Terms{}, errors.New("invalid isolated market signing request") }
	if !p.Valid() || r.Deployment != p.Deployment || r.User == (common.Address{}) || r.GasPrice == 0 || r.GasPrice > 100_000_000_000 {
		return invalid()
	}
	q := r.Offer
	if len(q) != QuoteLength || !bytes.Equal(q[:4], selector("execute((address,uint8,uint256,uint256,uint256,uint256,uint256,uint256,uint256),bytes)")) ||
		!zero(q[4:16]) || !bytes.Equal(q[16:36], r.User[:]) || !zero(q[36:67]) || ActionName(q[67]) == "" ||
		!zero(q[260:284]) || !zero(q[292:322]) || q[322] != 1 || q[323] != 64 || !zero(q[324:355]) || q[355] != 65 ||
		q[420] < 27 || q[420] > 28 || !zero(q[421:]) || zero(q[68:100]) || zero(q[196:228]) {
		return invalid()
	}
	deadline := binary.BigEndian.Uint64(q[284:292])
	if now.Unix() < 1 || deadline <= uint64(now.Unix()) || deadline-uint64(now.Unix()) > 60 {
		return invalid()
	}
	quoteDigest := offerDigest(p, q)
	signature := bytes.Clone(q[356:421])
	signature[64] -= 27
	if !crypto.ValidateSignatureValues(signature[64], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:64]), true) {
		return invalid()
	}
	pubkey, err := crypto.SigToPub(quoteDigest[:], signature)
	if err != nil || crypto.PubkeyToAddress(*pubkey) != p.Signer {
		return invalid()
	}
	a, b := new(big.Int).SetBytes(q[68:100]), new(big.Int).SetBytes(q[100:132])
	minA, minB := new(big.Int).SetBytes(q[132:164]), new(big.Int).SetBytes(q[164:196])
	act := q[67]
	if a.Cmp(capQuantity) > 0 || b.Cmp(capQuantity) > 0 {
		return invalid()
	}
	if !validActionTerms(act, a, b, minA, minB) {
		return invalid()
	}
	view := Terms{Chain: ChainID, OfferDigest: quoteDigest, Action: act, A: a.String(), B: b.String(), MinA: minA.String(), MinB: minB.String(), Deadline: deadline,
		ExpectedPriceVersion: new(big.Int).SetBytes(q[196:228]).String(), Gas: 600000}
	if r.To == p.Venue {
		if !bytes.Equal(r.Calldata, q) {
			return invalid()
		}
	} else {
		in := r.Calldata
		if (len(in) != 68 && len(in) != BoundAllowanceLength) || !bytes.Equal(in[:4], selector("approve(address,uint256)")) ||
			!zero(in[4:16]) || !bytes.Equal(in[16:36], p.Venue[:]) {
			return invalid()
		}
		if len(in) == BoundAllowanceLength && !bytes.Equal(in[68:], quoteDigest[:]) {
			return invalid()
		}
		switch {
		case r.To == p.Base && (act == 0 || act == 2) && bytes.Equal(in[36:68], q[68:100]):
			view.Allowance = a.String()
		case r.To == p.Quote && act == 2 && bytes.Equal(in[36:68], q[100:132]):
			view.Allowance = b.String()
		case r.To == p.Quote && (act == 1 || act == 6 || act == 7) && bytes.Equal(in[36:68], q[68:100]):
			view.Allowance = a.String()
		default:
			return invalid()
		}
		view.Gas = 65000
	}
	raw, err := rlp.EncodeToBytes([]any{r.Nonce, r.GasPrice, view.Gas, r.To, uint64(0), r.Calldata, uint64(ChainID), uint64(0), uint64(0)})
	if err != nil {
		return invalid()
	}
	view.Digest = crypto.Keccak256Hash(raw)
	return view, nil
}

func validActionTerms(action uint8, a, b, minA, minB *big.Int) bool {
	if a.Sign() == 0 {
		return false
	}
	switch action {
	case 0, 1, 4, 5:
		return b.Sign() == 0 && minA.Sign() > 0 && minB.Sign() == 0
	case 2:
		return b.Sign() > 0 && minA.Sign() > 0 && minB.Sign() == 0
	case 3:
		return b.Sign() == 0 && minA.Sign() > 0 && minB.Sign() > 0
	case 6, 7:
		return b.Sign() > 0 && minA.Sign() == 0 && minB.Sign() == 0
	case 8:
		return b.Sign() > 0 && minB.Sign() == 0
	}
	return false
}
