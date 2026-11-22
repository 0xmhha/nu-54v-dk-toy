// Package quotev2 encodes and validates the SDK v2 signed merchant quote.
package quotev2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
)

var (
	errMalformed      = errors.New("malformed v2 quote")
	domainTypeHash    = crypto.Keccak256([]byte(domainType))
	domainNameHash    = crypto.Keccak256([]byte(domainName))
	domainVersionHash = crypto.Keccak256([]byte(domainVersion))
	quoteTypeHash     = crypto.Keccak256([]byte(quoteType))
)

type Policy struct {
	Expected       []byte
	MerchantSigner []byte
	Now            uint64
	Consumed       bool
}

func valid(raw []byte) bool {
	if len(raw) != unsignedSize {
		return false
	}
	for index, width := range widths {
		word := raw[index*32 : (index+1)*32]
		if !zero(word[:32-width]) || zero(word[32-width:]) {
			return false
		}
	}
	mode, role := raw[modeOffset+31], raw[payerRoleOffset+31]
	return (mode == escrowMode || mode == directMode) &&
		(role == ownerRole || role == renterRole) &&
		(mode != directMode || role == ownerRole) &&
		uint64Word(raw, expiresAtOffset) > uint64Word(raw, issuedAtOffset)
}

func zero(raw []byte) bool {
	for _, b := range raw {
		if b != 0 {
			return false
		}
	}
	return true
}

func uint64Word(raw []byte, index int) uint64 {
	return binary.BigEndian.Uint64(raw[index+24 : index+32])
}

// Encode pads ordered, fixed-width fields from the v2 schema to their ABI words.
func Encode(fields [][]byte) ([]byte, error) {
	if len(fields) != len(widths) {
		return nil, errMalformed
	}
	raw := make([]byte, unsignedSize)
	for index, width := range widths {
		if len(fields[index]) != width {
			return nil, errMalformed
		}
		copy(raw[(index+1)*32-width:], fields[index])
	}
	if !valid(raw) {
		return nil, errMalformed
	}
	return raw, nil
}

// Digest validates the canonical unsigned representation and applies EIP-712.
func Digest(raw []byte) ([32]byte, error) {
	if !valid(raw) {
		return [32]byte{}, errMalformed
	}
	domain := crypto.Keccak256(domainTypeHash, domainNameHash, domainVersionHash, raw[:quoteIdOffset])
	quote := crypto.Keccak256(quoteTypeHash, raw[quoteIdOffset:])
	return [32]byte(crypto.Keccak256([]byte{0x19, 0x01}, domain, quote)), nil
}

// Verify compares authenticated order and merchant state before authorizing a quote.
func Verify(signed []byte, policy Policy) int {
	if len(signed) != unsignedSize+signatureSize || !valid(signed[:unsignedSize]) ||
		!valid(policy.Expected) || len(policy.MerchantSigner) != 20 || zero(policy.MerchantSigner) {
		return statusMalformed
	}
	raw := signed[:unsignedSize]
	if !zero(raw[:24]) || uint64Word(raw, chainIdOffset) != pilotChain || raw[modeOffset+31] != escrowMode {
		return statusUnsupportedProfile
	}
	if !bytes.Equal(raw, policy.Expected) {
		return statusWrongBinding
	}
	if policy.Now < uint64Word(raw, issuedAtOffset) || policy.Now >= uint64Word(raw, expiresAtOffset) {
		return statusExpired
	}
	digest, _ := Digest(raw)
	signature := append([]byte(nil), signed[unsignedSize:]...)
	signature[64] -= 27
	r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:64])
	if !crypto.ValidateSignatureValues(signature[64], r, s, true) {
		return statusUntrustedSigner
	}
	public, err := crypto.SigToPub(digest[:], signature)
	if err != nil || !bytes.Equal(crypto.PubkeyToAddress(*public).Bytes(), policy.MerchantSigner) {
		return statusUntrustedSigner
	}
	if policy.Consumed {
		return statusConsumed
	}
	return statusOk
}
