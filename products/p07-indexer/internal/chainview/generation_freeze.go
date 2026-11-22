package chainview

import (
	"encoding/binary"
	"errors"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type GenerationReceiptAuthority struct {
	Binding               GenerationBinding
	BoardSigner, Operator common.Address
	Challenge             common.Hash
	ChallengeUntil        time.Time
	RegisteredAt          time.Time
	AuthorityUntil        time.Time
}

var ErrGenerationFreeze = errors.New("rental generation freeze could not be verified")

// GenerationBindingDigest encodes P01's fixed 208-byte deployment/registration binding.
func GenerationBindingDigest(b GenerationBinding) (common.Hash, error) {
	if !nonzeroUUID(b.Branch) || !nonzeroUUID(b.Board) || b.Generation == 0 ||
		b.Registration == (common.Hash{}) || b.Measurement == (common.Hash{}) ||
		b.Deployment == (common.Hash{}) || b.Chain == 0 ||
		b.Settlement == (common.Address{}) || b.Account == (common.Address{}) {
		return common.Hash{}, ErrGenerationFreeze
	}
	body := make([]byte, 208)
	binary.BigEndian.PutUint64(body[:8], b.Generation)
	copy(body[8:24], common.FromHex(strings.ReplaceAll(b.Branch, "-", "")))
	copy(body[24:40], common.FromHex(strings.ReplaceAll(b.Board, "-", "")))
	copy(body[40:72], b.Registration[:])
	copy(body[72:104], b.Measurement[:])
	copy(body[104:136], b.Deployment[:])
	binary.BigEndian.PutUint64(body[160:168], b.Chain)
	copy(body[168:188], b.Settlement[:])
	copy(body[188:], b.Account[:])
	return crypto.Keccak256Hash(body), nil
}

// VerifyGenerationReceipt checks the detached RFR1 transport of P01's
// RentalFreezeReceipt. The authority must come from current board enrollment.
func VerifyGenerationReceipt(raw []byte, authority GenerationReceiptAuthority, now time.Time) (VerifiedGenerationFreeze, error) {
	receipt, err := rentalv1.ParseFreezeReceipt(raw)
	if err != nil || now.Unix() <= 0 {
		return VerifiedGenerationFreeze{}, ErrGenerationFreeze
	}
	binding, err := GenerationBindingDigest(authority.Binding)
	if err != nil || binding != common.Hash(receipt.Binding) ||
		authority.Challenge == (common.Hash{}) || authority.Challenge != common.Hash(receipt.Challenge) ||
		authority.BoardSigner == (common.Address{}) || authority.BoardSigner == authority.Binding.Account ||
		authority.Operator == (common.Address{}) || authority.BoardSigner == authority.Operator ||
		authority.Binding.Chain != 8283 {
		return VerifiedGenerationFreeze{}, ErrGenerationFreeze
	}
	root, first, next := common.Hash(receipt.Root), common.Hash(receipt.FirstNonce), common.Hash(receipt.NextNonce)
	count, frozenAt, expiresAt := receipt.EntryCount, receipt.FrozenAt, receipt.ExpiresAt
	current := uint64(now.Unix())
	if root == (common.Hash{}) || count > 8 || first[31] != 0 || frozenAt == 0 || frozenAt > current ||
		expiresAt > math.MaxInt64 || expiresAt <= current || expiresAt-current > 120 ||
		time.Unix(int64(expiresAt), 0).After(authority.ChallengeUntil) {
		return VerifiedGenerationFreeze{}, ErrGenerationFreeze
	}
	start := new(big.Int).SetBytes(first[:])
	end := new(big.Int).SetBytes(next[:])
	if end.Sub(end, start).Cmp(new(big.Int).SetUint64(count)) != 0 {
		return VerifiedGenerationFreeze{}, ErrGenerationFreeze
	}
	frozen := time.Unix(int64(frozenAt), 0)
	valid := time.Unix(int64(expiresAt), 0)
	if !authority.RegisteredAt.IsZero() && frozen.Before(authority.RegisteredAt) {
		return VerifiedGenerationFreeze{}, ErrGenerationFreeze
	}
	if !authority.AuthorityUntil.IsZero() {
		valid = minTime(valid, authority.AuthorityUntil)
	}
	if !valid.After(now) {
		return VerifiedGenerationFreeze{}, ErrGenerationFreeze
	}
	digest := common.Hash(receipt.Digest(authority.Binding.Chain, authority.Binding.Settlement))
	if recoverV1Device(digest, receipt.BoardSignature[:]) != authority.BoardSigner {
		return VerifiedGenerationFreeze{}, ErrGenerationFreeze
	}
	return VerifiedGenerationFreeze{Binding: authority.Binding, Root: root, ReceiptDigest: digest,
		FirstNonce: first, NextNonce: next, EntryCount: count, FrozenAt: frozen, ValidUntil: valid}, nil
}
