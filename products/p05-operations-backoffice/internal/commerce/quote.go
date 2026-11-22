package commerce

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/quotev2"
)

var ErrSigning = errors.New("quote signature unavailable")

type Profile struct {
	Chain                                uint64
	Contract, Asset, Beneficiary, Signer common.Address
	MerchantID                           [32]byte
	KeyVersion, BeneficiaryVersion       uint64
	CommitmentKey                        [32]byte
}

type QuoteOrder struct {
	Tenant, Branch, Sale string
	QuoteID, OrderID     [32]byte // independent public random identifiers
	Claim, Intent, Nonce [32]byte
	Payer                common.Address
	PayerGeneration      uint64
	PayerRole            byte
	Amount               string
	Issued, Expires      time.Time
}

type Signer interface {
	// SignDigest signs only the provided merchant EIP-712 digest; it never broadcasts.
	SignDigest(context.Context, [32]byte) ([]byte, error)
}

func uuidBytes(id string) []byte {
	if len(id) != 36 {
		return nil
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	if err != nil || len(raw) != 16 {
		return nil
	}
	return raw
}

func BuildUnsigned(order QuoteOrder, profile Profile) ([]byte, error) {
	if profile.Chain != 8283 {
		return nil, errors.New("unsupported settlement profile")
	}
	amount, ok := new(big.Int).SetString(order.Amount, 10)
	if !ok || amount.Sign() <= 0 || amount.BitLen() > 256 || amount.String() != order.Amount ||
		order.Issued.Unix() <= 0 || !order.Expires.After(order.Issued) {
		return nil, errors.New("invalid authorized amount or time")
	}
	makeCommitment := func(purpose quotev2.CommitmentPurpose, value []byte) ([]byte, error) {
		commitment, err := quotev2.CommitPrivateContext(profile.CommitmentKey, order.QuoteID, purpose, value)
		return commitment[:], err
	}
	tenant, err := makeCommitment(quotev2.TenantCommitment, uuidBytes(order.Tenant))
	if err != nil {
		return nil, err
	}
	branch, err := makeCommitment(quotev2.BranchCommitment, uuidBytes(order.Branch))
	if err != nil {
		return nil, err
	}
	claim, err := makeCommitment(quotev2.ClaimCommitment, order.Claim[:])
	if err != nil {
		return nil, err
	}
	intent, err := makeCommitment(quotev2.IntentCommitment, order.Intent[:])
	if err != nil {
		return nil, err
	}
	u64 := func(value uint64) []byte { return binary.BigEndian.AppendUint64(nil, value) }
	return quotev2.Encode([][]byte{
		new(big.Int).SetUint64(profile.Chain).FillBytes(make([]byte, 32)),
		profile.Contract.Bytes(), order.QuoteID[:], tenant, branch, order.OrderID[:], profile.MerchantID[:],
		claim, intent, order.Payer.Bytes(), profile.Asset.Bytes(), profile.Beneficiary.Bytes(),
		amount.FillBytes(make([]byte, 32)), order.Nonce[:], u64(profile.KeyVersion), u64(profile.BeneficiaryVersion),
		u64(order.PayerGeneration), u64(uint64(order.Issued.Unix())), u64(uint64(order.Expires.Unix())), {1}, {order.PayerRole},
	})
}

func SignQuote(ctx context.Context, order QuoteOrder, profile Profile, signer Signer, now time.Time) ([]byte, [32]byte, error) {
	unsigned, err := BuildUnsigned(order, profile)
	if err != nil {
		return nil, [32]byte{}, err
	}
	digest, err := quotev2.Digest(unsigned)
	if err != nil {
		return nil, [32]byte{}, err
	}
	signature, err := signer.SignDigest(ctx, digest)
	if err != nil || len(signature) != crypto.SignatureLength {
		return nil, [32]byte{}, ErrSigning
	}
	signed := append(bytes.Clone(unsigned), signature...)
	if quotev2.Verify(signed, quotev2.Policy{Expected: unsigned, MerchantSigner: profile.Signer.Bytes(), Now: uint64(now.Unix())}) != 0 {
		return nil, [32]byte{}, ErrSigning
	}
	return signed, digest, nil
}

func NewCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 || bytes.Equal(key, make([]byte, 32)) {
		return nil, errors.New("AES-256 quote storage key required")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func quoteContext(branch, sale string) []byte {
	return []byte("nu54-quote-store-v2:" + branch + ":" + sale)
}

func freshId() ([32]byte, error) {
	var out [32]byte
	_, err := rand.Read(out[:])
	return out, err
}
