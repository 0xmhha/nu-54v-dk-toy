package rentalv1

import (
	"encoding/binary"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const FreezeReceiptSize = 253

var (
	ErrFreezeReceipt     = errors.New("invalid RFR1 transport")
	freezeDomainType     = crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	freezeDomainName     = crypto.Keccak256([]byte("NU54 Payment Settlement"))
	freezeDomainRevision = crypto.Keccak256([]byte("1"))
	freezeReceiptType    = crypto.Keccak256([]byte("RentalFreezeReceipt(bytes32 binding,bytes32 challenge,bytes32 projectionRoot,uint256 firstNonce,uint256 nextNonce,uint64 entryCount,uint64 frozenAt,uint64 expiresAt)"))
)

// FreezeReceipt is the public P01 attestation transport. Parsing and hashing
// do not grant authority or establish that any signed field is current.
type FreezeReceipt struct {
	Binding, Challenge, Root, FirstNonce, NextNonce [32]byte
	EntryCount, FrozenAt, ExpiresAt                 uint64
	BoardSignature                                  [65]byte
}

func ParseFreezeReceipt(raw []byte) (FreezeReceipt, error) {
	if len(raw) != FreezeReceiptSize || string(raw[:4]) != "RFR1" {
		return FreezeReceipt{}, ErrFreezeReceipt
	}
	return FreezeReceipt{
		Binding: [32]byte(raw[4:36]), Challenge: [32]byte(raw[36:68]), Root: [32]byte(raw[68:100]),
		FirstNonce: [32]byte(raw[100:132]), NextNonce: [32]byte(raw[132:164]),
		EntryCount: binary.BigEndian.Uint64(raw[164:172]), FrozenAt: binary.BigEndian.Uint64(raw[172:180]),
		ExpiresAt: binary.BigEndian.Uint64(raw[180:188]), BoardSignature: [65]byte(raw[188:]),
	}, nil
}

// Bytes returns a detached canonical RFR1 transport.
func (receipt FreezeReceipt) Bytes() []byte {
	raw := make([]byte, FreezeReceiptSize)
	copy(raw, "RFR1")
	copy(raw[4:36], receipt.Binding[:])
	copy(raw[36:68], receipt.Challenge[:])
	copy(raw[68:100], receipt.Root[:])
	copy(raw[100:132], receipt.FirstNonce[:])
	copy(raw[132:164], receipt.NextNonce[:])
	binary.BigEndian.PutUint64(raw[164:172], receipt.EntryCount)
	binary.BigEndian.PutUint64(raw[172:180], receipt.FrozenAt)
	binary.BigEndian.PutUint64(raw[180:188], receipt.ExpiresAt)
	copy(raw[188:], receipt.BoardSignature[:])
	return raw
}

// Digest returns the EIP-712 digest signed by the registered board identity.
// The detached signature is deliberately excluded.
func (receipt FreezeReceipt) Digest(chainID uint64, settlement [20]byte) [32]byte {
	var chain, count, frozen, expiry [32]byte
	binary.BigEndian.PutUint64(chain[24:], chainID)
	binary.BigEndian.PutUint64(count[24:], receipt.EntryCount)
	binary.BigEndian.PutUint64(frozen[24:], receipt.FrozenAt)
	binary.BigEndian.PutUint64(expiry[24:], receipt.ExpiresAt)
	domain := crypto.Keccak256(freezeDomainType, freezeDomainName, freezeDomainRevision, chain[:], common.LeftPadBytes(settlement[:], 32))
	structure := crypto.Keccak256(freezeReceiptType, receipt.Binding[:], receipt.Challenge[:], receipt.Root[:],
		receipt.FirstNonce[:], receipt.NextNonce[:], count[:], frozen[:], expiry[:])
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, domain, structure)
}
