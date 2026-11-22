// Package chainview ingests pinned deployment profiles into generation-scoped projections.
package chainview

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/quotev2"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"math"
	"math/big"
	"strings"
	"time"

	v1 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	v1ABIHash = "05bbfd86dc59243e5456ce494036f33870e319d66aa96a91ee82e7054670ca4d"
	v2ABIHash = "06f5bad7c462de2835bb3054583d7d962b179424bd67e411360313d47da02814"
)

var ErrUnknownABI = errors.New("unapproved settlement ABI")

type Profile deployment.Profile

type Event struct {
	Kind, Merchant, Order, Refund, Asset, Amount, Nonce, Payer, Recipient string
	QuoteDigest, TxHash                                                   string
	Mode                                                                  uint8
	PayerGeneration                                                       uint64
	Index                                                                 int32
	IssuedAt, ExpiresAt                                                   *time.Time
}

type Decoder struct {
	profile Profile
	legacy  *v1.SettlementFilterer
	modern  *v2.Settlementv2Filterer
	paid    common.Hash
	refund  common.Hash
	settle  abi.Method
}

func NewDecoder(profile Profile) (*Decoder, error) {
	if profile.ChainID != 8283 || profile.Contract == (common.Address{}) || profile.Asset == (common.Address{}) {
		return nil, ErrDeploymentIdentity
	}
	d := &Decoder{profile: profile}
	switch {
	case profile.Version == 1 && profile.ABIHash == v1ABIHash:
		abi, err := v1.SettlementMetaData.GetAbi()
		if err != nil {
			return nil, err
		}
		d.legacy, err = v1.NewSettlementFilterer(profile.Contract, nil)
		d.paid = abi.Events["PaymentSettled"].ID
		return d, err
	case profile.Version == 2 && profile.ABIHash == v2ABIHash:
		abi, err := v2.Settlementv2MetaData.GetAbi()
		if err != nil {
			return nil, err
		}
		d.modern, err = v2.NewSettlementv2Filterer(profile.Contract, nil)
		d.paid, d.refund = abi.Events["Paid"].ID, abi.Events["Refunded"].ID
		d.settle = abi.Methods["settle"]
		return d, err
	default:
		return nil, ErrUnknownABI
	}
}

func (d *Decoder) matches(log types.Log) bool {
	return log.Address == d.profile.Contract && len(log.Topics) != 0 &&
		(log.Topics[0] == d.paid || (d.modern != nil && log.Topics[0] == d.refund))
}

func (d *Decoder) Decode(log types.Log, status uint64) (Event, error) {
	if !d.matches(log) || status != types.ReceiptStatusSuccessful || log.Removed || log.Index > math.MaxInt32 ||
		log.TxHash == (common.Hash{}) || log.BlockHash == (common.Hash{}) || len(log.Topics) != 4 {
		return Event{}, errors.New("log is outside the successful pinned settlement receipt")
	}
	switch {
	case d.legacy != nil:
		if len(log.Data) != 64 || !wordAddress(log.Topics[1][:]) || !wordAddress(log.Topics[3][:]) {
			return Event{}, errors.New("noncanonical v1 receipt encoding")
		}
		decoded, err := d.legacy.ParsePaymentSettled(log)
		if err != nil {
			return Event{}, err
		}
		if decoded.Merchant == (common.Address{}) || decoded.Device == (common.Address{}) ||
			decoded.OrderId == ([32]byte{}) || !positive(decoded.Amount) || decoded.Nonce == nil {
			return Event{}, errors.New("invalid legacy settlement event")
		}
		// A v1 event credits the merchant ledger; it does not attest a beneficiary transfer.
		merchant := lower(decoded.Merchant)
		return Event{Kind: "paid", Merchant: merchant, Order: common.Hash(decoded.OrderId).Hex(), Asset: lower(d.profile.Asset),
			Amount: decoded.Amount.String(), Nonce: decoded.Nonce.String(), Payer: lower(decoded.Device), Recipient: merchant,
			TxHash: log.TxHash.Hex(), Index: int32(log.Index)}, nil
	case log.Topics[0] == d.paid:
		if len(log.Data) != 224 || !wordAddress(log.Topics[3][:]) || !wordAddress(log.Data[:32]) ||
			!wordAddress(log.Data[32:64]) {
			return Event{}, errors.New("noncanonical v2 paid receipt encoding")
		}
		decoded, err := d.modern.ParsePaid(log)
		if err != nil {
			return Event{}, err
		}
		if decoded.MerchantId == ([32]byte{}) || decoded.OrderId == ([32]byte{}) || decoded.QuoteDigest == ([32]byte{}) ||
			decoded.Payer == (common.Address{}) || decoded.Beneficiary == (common.Address{}) || decoded.Asset != d.profile.Asset ||
			!positive(decoded.Amount) || !positive(decoded.Nonce) || decoded.PayerGeneration == 0 || !validMode(decoded.Mode) {
			return Event{}, errors.New("invalid v2 paid event")
		}
		return Event{Kind: "paid", Merchant: common.Hash(decoded.MerchantId).Hex(), Order: common.Hash(decoded.OrderId).Hex(),
			Asset: lower(decoded.Asset), Amount: decoded.Amount.String(), Nonce: decoded.Nonce.String(),
			Payer: lower(decoded.Payer), Recipient: lower(decoded.Beneficiary), QuoteDigest: common.Hash(decoded.QuoteDigest).Hex(),
			Mode: decoded.Mode, PayerGeneration: decoded.PayerGeneration, TxHash: log.TxHash.Hex(), Index: int32(log.Index)}, nil
	default:
		if len(log.Data) != 128 || !wordAddress(log.Data[:32]) || !wordAddress(log.Data[32:64]) {
			return Event{}, errors.New("noncanonical v2 refund receipt encoding")
		}
		decoded, err := d.modern.ParseRefunded(log)
		if err != nil {
			return Event{}, err
		}
		if decoded.MerchantId == ([32]byte{}) || decoded.OrderId == ([32]byte{}) || decoded.RefundId == ([32]byte{}) ||
			decoded.Payer == (common.Address{}) || decoded.Asset != d.profile.Asset || !positive(decoded.Amount) || !validMode(decoded.Mode) {
			return Event{}, errors.New("invalid v2 refund event")
		}
		payer := lower(decoded.Payer)
		return Event{Kind: "refunded", Merchant: common.Hash(decoded.MerchantId).Hex(), Order: common.Hash(decoded.OrderId).Hex(),
			Refund: common.Hash(decoded.RefundId).Hex(), Asset: lower(decoded.Asset), Amount: decoded.Amount.String(), Payer: payer,
			Recipient: payer, Mode: decoded.Mode, TxHash: log.TxHash.Hex(), Index: int32(log.Index)}, nil
	}
}

// bindQuoteWindow verifies that the canonical direct transaction's quote
// calldata hashes to the same digest the settlement contract accepted.
func (d *Decoder) bindQuoteWindow(event *Event, tx *types.Transaction, included time.Time) error {
	if d.modern == nil || event.Kind != "paid" || tx.To() == nil || *tx.To() != d.profile.Contract {
		return nil
	}
	data := tx.Data()
	if len(data) < 4 || !bytes.Equal(data[:4], d.settle.ID) {
		return nil
	}
	const tupleLength = 19 * 32
	if len(data) < 4+tupleLength {
		return errors.New("missing signed quote tuple in successful settlement")
	}
	fields := data[4 : 4+tupleLength]
	raw := make([]byte, 64+tupleLength)
	binary.BigEndian.PutUint64(raw[24:32], d.profile.ChainID)
	copy(raw[44:64], d.profile.Contract.Bytes())
	copy(raw[64:], fields)
	digest, err := quotev2.Digest(raw)
	if err != nil || common.Hash(digest).Hex() != event.QuoteDigest {
		return errors.New("accepted settlement event and quote digest differ")
	}
	decoded, err := (abi.Arguments{d.settle.Inputs[0]}).Unpack(fields)
	if err != nil || len(decoded) != 1 {
		return errors.New("invalid settlement quote timing")
	}
	q := *abi.ConvertType(decoded[0], new(v2.PaymentTypesV2MerchantQuote)).(*v2.PaymentTypesV2MerchantQuote)
	if q.IssuedAt > math.MaxInt64 || q.ExpiresAt > math.MaxInt64 ||
		included.Unix() < int64(q.IssuedAt) || included.Unix() >= int64(q.ExpiresAt) {
		return errors.New("settlement inclusion is outside the authenticated quote window")
	}
	issued, expires := time.Unix(int64(q.IssuedAt), 0).UTC(), time.Unix(int64(q.ExpiresAt), 0).UTC()
	event.IssuedAt, event.ExpiresAt = &issued, &expires
	return nil
}

func lower(address common.Address) string { return strings.ToLower(address.Hex()) }
func positive(value *big.Int) bool        { return value != nil && value.Sign() > 0 && value.BitLen() <= 256 }
func validMode(mode uint8) bool           { return mode == 1 || mode == 2 }

func wordAddress(word []byte) bool {
	for _, b := range word[:12] {
		if b != 0 {
			return false
		}
	}
	return true
}
