package rentalv1

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const CleanupIntentSize = 181
const CleanupReceiptSize = 109
const CleanupIntentLifetime = 120

var (
	ErrCleanupProof    = errors.New("invalid rental cleanup proof")
	cleanupIntentType  = crypto.Keccak256([]byte("RentalCleanupIntentV1(bytes32 binding,bytes32 disposition,bytes32 request,uint64 issuedAt,uint64 expiresAt)"))
	cleanupReceiptType = crypto.Keccak256([]byte("RentalCleanupReceiptV1(bytes32 intent,uint64 retiredAt)"))
)

type CleanupIntent struct {
	Binding, Disposition, Request [32]byte
	IssuedAt, ExpiresAt           uint64
	OperatorSignature             [65]byte
}

type CleanupReceipt struct {
	Intent         [32]byte
	RetiredAt      uint64
	BoardSignature [65]byte
}

type CleanupScope struct {
	ChainID                       uint64
	Settlement, Operator, Board   common.Address
	Binding, Disposition, Request [32]byte
}

func (intent CleanupIntent) Bytes() []byte {
	raw := make([]byte, CleanupIntentSize)
	copy(raw, "RCI1")
	copy(raw[4:36], intent.Binding[:])
	copy(raw[36:68], intent.Disposition[:])
	copy(raw[68:100], intent.Request[:])
	binary.BigEndian.PutUint64(raw[100:108], intent.IssuedAt)
	binary.BigEndian.PutUint64(raw[108:116], intent.ExpiresAt)
	copy(raw[116:], intent.OperatorSignature[:])
	return raw
}

func ParseCleanupIntent(raw []byte) (CleanupIntent, error) {
	if len(raw) != CleanupIntentSize || string(raw[:4]) != "RCI1" {
		return CleanupIntent{}, ErrCleanupProof
	}
	return CleanupIntent{[32]byte(raw[4:36]), [32]byte(raw[36:68]), [32]byte(raw[68:100]),
		binary.BigEndian.Uint64(raw[100:108]), binary.BigEndian.Uint64(raw[108:116]), [65]byte(raw[116:])}, nil
}

func (receipt CleanupReceipt) Bytes() []byte {
	raw := make([]byte, CleanupReceiptSize)
	copy(raw, "RCR1")
	copy(raw[4:36], receipt.Intent[:])
	binary.BigEndian.PutUint64(raw[36:44], receipt.RetiredAt)
	copy(raw[44:], receipt.BoardSignature[:])
	return raw
}

func ParseCleanupReceipt(raw []byte) (CleanupReceipt, error) {
	if len(raw) != CleanupReceiptSize || string(raw[:4]) != "RCR1" {
		return CleanupReceipt{}, ErrCleanupProof
	}
	return CleanupReceipt{[32]byte(raw[4:36]), binary.BigEndian.Uint64(raw[36:44]), [65]byte(raw[44:])}, nil
}

func cleanupDomain(chainID uint64, settlement common.Address, message [32]byte) [32]byte {
	var chain [32]byte
	binary.BigEndian.PutUint64(chain[24:], chainID)
	domain := crypto.Keccak256(freezeDomainType, freezeDomainName, freezeDomainRevision, chain[:], common.LeftPadBytes(settlement[:], 32))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, domain, message[:])
}

func (intent CleanupIntent) Digest(chainID uint64, settlement common.Address) [32]byte {
	var issued, expires [32]byte
	binary.BigEndian.PutUint64(issued[24:], intent.IssuedAt)
	binary.BigEndian.PutUint64(expires[24:], intent.ExpiresAt)
	message := crypto.Keccak256Hash(cleanupIntentType, intent.Binding[:], intent.Disposition[:], intent.Request[:], issued[:], expires[:])
	return cleanupDomain(chainID, settlement, message)
}

func (receipt CleanupReceipt) Digest(chainID uint64, settlement common.Address) [32]byte {
	var retired [32]byte
	binary.BigEndian.PutUint64(retired[24:], receipt.RetiredAt)
	return cleanupDomain(chainID, settlement, crypto.Keccak256Hash(cleanupReceiptType, receipt.Intent[:], retired[:]))
}

func cleanupSigner(digest [32]byte, signature [65]byte, expected common.Address) bool {
	if signature[64] != 27 && signature[64] != 28 {
		return false
	}
	signature[64] -= 27
	if !crypto.ValidateSignatureValues(signature[64], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:64]), true) {
		return false
	}
	key, err := crypto.SigToPub(digest[:], signature[:])
	return err == nil && crypto.PubkeyToAddress(*key) == expected
}

func (scope CleanupScope) verifyIntent(intent CleanupIntent, at uint64) error {
	if scope.ChainID == 0 || scope.Settlement == (common.Address{}) || scope.Operator == (common.Address{}) ||
		scope.Board == (common.Address{}) || scope.Board == scope.Operator ||
		scope.Binding == ([32]byte{}) || scope.Disposition == ([32]byte{}) || scope.Request == ([32]byte{}) ||
		intent.Binding != scope.Binding || intent.Disposition != scope.Disposition || intent.Request != scope.Request ||
		intent.IssuedAt == 0 || at < intent.IssuedAt || at >= intent.ExpiresAt ||
		intent.ExpiresAt-intent.IssuedAt > CleanupIntentLifetime ||
		!cleanupSigner(intent.Digest(scope.ChainID, scope.Settlement), intent.OperatorSignature, scope.Operator) {
		return ErrCleanupProof
	}
	return nil
}

// VerifyIntent checks a new request at trusted time. It does not authorize erasure.
func (scope CleanupScope) VerifyIntent(intent CleanupIntent, now uint64) error {
	return scope.verifyIntent(intent, now)
}

// VerifyReceipt allows recovery of a completed request after intent expiry.
// The caller supplies the same independently resolved scope and immutable intent.
func (scope CleanupScope) VerifyReceipt(intent CleanupIntent, receipt CleanupReceipt, now uint64) error {
	if receipt.RetiredAt > now || scope.verifyIntent(intent, receipt.RetiredAt) != nil ||
		receipt.Intent != intent.Digest(scope.ChainID, scope.Settlement) ||
		!cleanupSigner(receipt.Digest(scope.ChainID, scope.Settlement), receipt.BoardSignature, scope.Board) {
		return ErrCleanupProof
	}
	return nil
}

// CleanupDispositionDigest binds terminal financial evidence to the original freeze.
// The caller must first verify the evidence independently with the ledger and P07.
func CleanupDispositionDigest(binding, freeze, close, anchor [32]byte, kind byte, evidence [32]byte) ([32]byte, error) {
	if binding == ([32]byte{}) || freeze == ([32]byte{}) || close == ([32]byte{}) ||
		anchor == ([32]byte{}) || evidence == ([32]byte{}) || (kind != 1 && kind != 2) {
		return [32]byte{}, ErrCleanupProof
	}
	return crypto.Keccak256Hash([]byte("NU54-rental-cleanup-disposition-v1\x00"), binding[:], freeze[:],
		close[:], anchor[:], []byte{kind}, evidence[:]), nil
}
