// Package settlement owns v2 approvals, relay custody and finalized owner reconciliation.
package settlement

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"math/big"
	"strings"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/quotev2"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const ABIHash = "06f5bad7c462de2835bb3054583d7d962b179424bd67e411360313d47da02814"

type Target struct {
	Stream        string         `json:"streamId"`
	Manifest      string         `json:"manifestSha256"`
	Contract      common.Address `json:"contract"`
	CodeHash      common.Hash    `json:"codeHash"`
	Asset         common.Address `json:"asset"`
	RefundSigner  common.Address `json:"refundSigner"`
	RefundVersion uint64         `json:"refundKeyVersion"`
}

type Quote struct {
	Args         v2.PaymentTypesV2MerchantQuote
	Digest       [32]byte
	MerchantSig  []byte
	ApprovalHash common.Hash
}

func unpackQuote(signed, issuedDigest, signer []byte, target Target) (Quote, error) {
	methods, err := v2.Settlementv2MetaData.GetAbi()
	if err != nil || len(signed) != 737 || len(signer) != 20 || len(issuedDigest) != 32 {
		return Quote{}, errors.New("invalid archived quote envelope")
	}
	unsigned := signed[:672]
	digest, err := quotev2.Digest(unsigned)
	if err != nil || !bytes.Equal(digest[:], issuedDigest) || new(big.Int).SetBytes(unsigned[:32]).Cmp(big.NewInt(8283)) != 0 ||
		common.BytesToAddress(unsigned[32:64]) != target.Contract {
		return Quote{}, errors.New("archived quote digest or domain differs")
	}
	fields, err := (abi.Arguments{methods.Methods["settle"].Inputs[0]}).Unpack(unsigned[64:])
	if err != nil || len(fields) != 1 {
		return Quote{}, errors.New("archived quote words not recognized")
	}
	q := *abi.ConvertType(fields[0], new(v2.PaymentTypesV2MerchantQuote)).(*v2.PaymentTypesV2MerchantQuote)
	// Verify archive authenticity at its original issue timestamp. New submissions
	// also enforce the live expiry with the owner lease inside PostgreSQL.
	if q.Asset != target.Asset || q.Mode != 1 || q.ExpiresAt > math.MaxInt64 || quotev2.Verify(signed, quotev2.Policy{
		Expected: unsigned, MerchantSigner: signer, Now: q.IssuedAt,
	}) != 0 {
		return Quote{}, errors.New("archived quote binding or signature differs")
	}
	separator := domainHash(unsigned[:64])
	approval := crypto.Keccak256Hash([]byte{0x19, 0x01}, separator[:],
		crypto.Keccak256(crypto.Keccak256([]byte("PaymentApproval(bytes32 quoteDigest)")), digest[:]))
	return Quote{Args: q, Digest: digest, MerchantSig: bytes.Clone(signed[672:]), ApprovalHash: approval}, nil
}

func domainHash(fields []byte) common.Hash {
	return crypto.Keccak256Hash(
		crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")),
		crypto.Keccak256([]byte("NU54 Payment Settlement")), crypto.Keccak256([]byte("2")), fields,
	)
}

func matchesSigner(hash common.Hash, signature []byte, address common.Address) bool {
	if len(signature) != 65 || (signature[64] != 27 && signature[64] != 28) {
		return false
	}
	payload := bytes.Clone(signature)
	payload[64] -= 27
	if !lowS(payload) {
		return false
	}
	key, err := crypto.SigToPub(hash[:], payload)
	return err == nil && crypto.PubkeyToAddress(*key) == address
}

func lower(address common.Address) string { return strings.ToLower(address.Hex()) }
func rawHex(data []byte) string           { return hex.EncodeToString(data) }

func (q Quote) expiry() time.Time { return time.Unix(int64(q.Args.ExpiresAt), 0).UTC() }
