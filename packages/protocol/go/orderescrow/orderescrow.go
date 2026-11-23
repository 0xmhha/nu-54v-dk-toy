// Package orderescrow builds what the board, the phone and the store sign for OrderEscrow.
// The board (nu54_order.c), the contract (OrderEscrow.sol) and the TypeScript SDK share its vectors.
package orderescrow

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"
	"strconv"
	"strings"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/quotev2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	ChainID      = 8283
	UnsignedSize = 672
	TokenName    = "NU54 Test USD"
	TokenVersion = "1"
)

// Word offsets inside the unsigned quote that the token authorization repeats.
const (
	chainOffset    = 0
	contractOffset = 32
	payerOffset    = 288
	assetOffset    = 320
	amountOffset   = 384
	expiresOffset  = 576
)

type Action string

const (
	Delivered Action = "OrderDelivered(bytes32 merchantId,bytes32 orderId)"
	Cancel    Action = "OrderCancel(bytes32 merchantId,bytes32 orderId)"
	Receipt   Action = "OrderReceipt(bytes32 merchantId,bytes32 orderId,uint64 claimableAt)"
	Dispute   Action = "OrderDispute(bytes32 merchantId,bytes32 orderId,uint64 claimableAt)"
	Withdraw  Action = "OrderDisputeWithdraw(bytes32 merchantId,bytes32 orderId,uint64 claimableAt)"
)

// BuyerActions maps the C3/order.answer action names to what the phone signs.
var BuyerActions = map[string]Action{"receipt": Receipt, "dispute": Dispute, "withdraw": Withdraw}

var (
	domainType  = text("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")
	receiveType = text("ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)")
	lockType    = text("OrderLock(bytes32 quoteDigest,address confirmer)")
)

var errInput = errors.New("invalid order escrow input")

func text(value string) []byte { return crypto.Keccak256([]byte(value)) }

// Lock is the authorization that pays one order. Digest is what the board signs.
type Lock struct {
	Nonce, Digest common.Hash
	Preimage      []byte
}

// NewLock builds the authorization for a 672-byte unsigned quote and a phone key.
func NewLock(unsigned []byte, confirmer common.Address) (Lock, error) {
	quote, err := quotev2.Digest(unsigned)
	if err != nil || confirmer == (common.Address{}) {
		return Lock{}, errInput
	}
	word := func(offset int) []byte { return unsigned[offset : offset+32] }
	nonce := crypto.Keccak256Hash(lockType, quote[:], common.LeftPadBytes(confirmer[:], 32))
	token := crypto.Keccak256(domainType, text(TokenName), text(TokenVersion), word(chainOffset), word(assetOffset))
	authorization := crypto.Keccak256(receiveType, word(payerOffset), word(contractOffset), word(amountOffset),
		make([]byte, 32), word(expiresOffset), nonce[:])
	preimage := append([]byte{0x19, 0x01}, append(token, authorization...)...)
	return Lock{Nonce: nonce, Digest: crypto.Keccak256Hash(preimage), Preimage: preimage}, nil
}

// ActionDigest is what the store signs (claimableAt 0) or the phone signs (claimableAt set).
func ActionDigest(action Action, escrow common.Address, merchantID, orderID common.Hash, claimableAt uint64) (common.Hash, error) {
	buyer := action == Receipt || action == Dispute || action == Withdraw
	if !buyer && action != Delivered && action != Cancel || buyer != (claimableAt != 0) ||
		escrow == (common.Address{}) || merchantID == (common.Hash{}) || orderID == (common.Hash{}) {
		return common.Hash{}, errInput
	}
	domain := crypto.Keccak256(domainType, text("NU54 Payment Settlement"), text("2"),
		common.LeftPadBytes(big.NewInt(ChainID).Bytes(), 32), common.LeftPadBytes(escrow[:], 32))
	fields := [][]byte{text(string(action)), merchantID[:], orderID[:]}
	if buyer {
		fields = append(fields, common.LeftPadBytes(binary.BigEndian.AppendUint64(nil, claimableAt), 32))
	}
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, domain, crypto.Keccak256(fields...)), nil
}

// SignedBy accepts a 65-byte low-s signature with v 27 or 28.
func SignedBy(digest common.Hash, signature []byte, signer common.Address) bool {
	if len(signature) != 65 || (signature[64] != 27 && signature[64] != 28) || signer == (common.Address{}) {
		return false
	}
	raw := append([]byte(nil), signature...)
	raw[64] -= 27
	if !crypto.ValidateSignatureValues(raw[64], new(big.Int).SetBytes(raw[:32]), new(big.Int).SetBytes(raw[32:64]), true) {
		return false
	}
	key, err := crypto.SigToPub(digest[:], raw)
	return err == nil && crypto.PubkeyToAddress(*key) == signer
}

// EnrollChallenge is what a board signs (0x79) to enroll its account with a store.
func EnrollChallenge(tenant, branch, subject string, payer common.Address, generation uint64, idempotencyKey string) [32]byte {
	return sha256.Sum256([]byte(strings.Join([]string{"NU54-payer-enroll-v1", tenant, branch, subject,
		strings.ToLower(payer.Hex()), strconv.FormatUint(generation, 10), idempotencyKey}, "\n")))
}

// AccountProofDigest is the EIP-712 digest of the board's account proof.
func AccountProofDigest(account common.Address, challenge [32]byte) common.Hash {
	domain := crypto.Keccak256(text("EIP712Domain(string name,string version,uint256 chainId)"), text("NU54 Wallet Account"), text("1"),
		common.LeftPadBytes(big.NewInt(ChainID).Bytes(), 32))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, domain, crypto.Keccak256(text("AccountProof(address account,bytes32 challenge)"),
		common.LeftPadBytes(account[:], 32), challenge[:]))
}

// ReturnDigest is the EIP-3009 TransferWithAuthorization that sends the board's balance home.
func ReturnDigest(token, from, to common.Address, amount *big.Int, validBefore uint64, nonce common.Hash) (common.Hash, error) {
	if amount == nil || amount.Sign() <= 0 || amount.BitLen() > 256 || token == (common.Address{}) || from == (common.Address{}) ||
		to == (common.Address{}) || nonce == (common.Hash{}) {
		return common.Hash{}, errInput
	}
	domain := crypto.Keccak256(domainType, text(TokenName), text(TokenVersion), common.LeftPadBytes(big.NewInt(ChainID).Bytes(), 32),
		common.LeftPadBytes(token[:], 32))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, domain, crypto.Keccak256(
		text("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"),
		common.LeftPadBytes(from[:], 32), common.LeftPadBytes(to[:], 32), common.LeftPadBytes(amount.Bytes(), 32), make([]byte, 32),
		common.LeftPadBytes(binary.BigEndian.AppendUint64(nil, validBefore), 32), nonce[:])), nil
}
