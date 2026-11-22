package chainview

import (
	"bytes"
	"math/big"
	"slices"

	v1 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	extended "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	wire "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type generationUse struct {
	kind          string
	device        common.Address
	nonce, digest common.Hash
	merchant      common.Address
	order         common.Hash
	expiry        uint64
}

var (
	v1DomainType    = crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	v1DomainName    = crypto.Keccak256([]byte("NU54 Payment Settlement"))
	v1DomainVersion = crypto.Keccak256([]byte("1"))
	v1PaymentType   = crypto.Keccak256([]byte(wire.EncodeType["PaymentAuthorization"]))
	v1LimitType     = crypto.Keccak256([]byte(wire.EncodeType["LimitChange"]))
)

// decodeV1GenerationCall projects only canonical direct applications of the
// exact typed approval. A nonce-used bit without this evidence stays unresolved.
func decodeV1GenerationCall(profile Profile, tx *types.Transaction, log types.Log, status uint64, included Header) *generationUse {
	if profile.Version != 1 || profile.ABIHash != v1ABIHash || profile.ChainID != 8283 || tx == nil || tx.To() == nil ||
		*tx.To() != profile.Contract || log.Address != profile.Contract || log.Removed || status != types.ReceiptStatusSuccessful ||
		log.TxHash != tx.Hash() || log.BlockHash != included.Hash || log.BlockNumber != uint64(included.Height) ||
		len(log.Topics) == 0 || len(tx.Data()) < 4 || included.Time.Unix() < 0 {
		return nil
	}
	base, err := v1.SettlementMetaData.GetAbi()
	if err != nil {
		return nil
	}
	extra, err := extended.SettlementextMetaData.GetAbi()
	if err != nil {
		return nil
	}
	var method abi.Method
	var event abi.Event
	var typeHash []byte
	var staticWords int
	var use generationUse
	switch log.Topics[0] {
	case base.Events["PaymentSettled"].ID:
		method, event = base.Methods["settle"], base.Events["PaymentSettled"]
		use.kind, typeHash, staticWords = "payment", v1PaymentType, 9
	case extra.Events["LimitsChanged"].ID:
		method, event = extra.Methods["setLimits"], extra.Events["LimitsChanged"]
		use.kind, typeHash, staticWords = "limit", v1LimitType, 6
	default:
		return nil
	}
	args := tx.Data()[4:]
	staticSize := staticWords * 32
	if !bytes.Equal(tx.Data()[:4], method.ID) || len(args) < staticSize {
		return nil
	}
	values, err := method.Inputs.Unpack(args)
	if err != nil || len(values) != 2 {
		return nil
	}
	canonical, err := method.Inputs.PackValues(values)
	if err != nil || !bytes.Equal(canonical, args) {
		return nil
	}
	signature, ok := values[1].([]byte)
	if !ok {
		return nil
	}
	chainID := new(big.Int).SetUint64(profile.ChainID)
	use.digest = typedV1Digest(profile.ChainID, profile.Contract, typeHash, args[:staticSize])
	use.device = recoverV1Device(use.digest, signature)
	if use.device == (common.Address{}) {
		return nil
	}
	deviceTopic := common.BytesToHash(use.device[:])
	var topics []common.Hash
	var eventData []byte
	switch use.kind {
	case "payment":
		a := abi.ConvertType(values[0], new(v1.PaymentTypesPaymentAuthorization)).(*v1.PaymentTypesPaymentAuthorization)
		if a.ChainId.Cmp(chainID) != 0 || a.ContractAddress != profile.Contract || a.Token != profile.Asset ||
			a.Merchant == (common.Address{}) || a.Payout == (common.Address{}) || a.OrderId == ([32]byte{}) ||
			!positive(a.Amount) {
			return nil
		}
		use.nonce, use.expiry = common.BytesToHash(a.Nonce.Bytes()), a.Expiry
		use.merchant, use.order = a.Merchant, a.OrderId
		topics = []common.Hash{event.ID, common.BytesToHash(a.Merchant[:]), a.OrderId, deviceTopic}
		eventData, err = event.Inputs.NonIndexed().Pack(a.Amount, a.Nonce)
	case "limit":
		c := abi.ConvertType(values[0], new(extended.PaymentTypesLimitChange)).(*extended.PaymentTypesLimitChange)
		if c.ChainId.Cmp(chainID) != 0 || c.ContractAddress != profile.Contract {
			return nil
		}
		use.nonce, use.expiry = common.BytesToHash(c.Nonce.Bytes()), c.Expiry
		topics = []common.Hash{event.ID, deviceTopic}
		eventData, err = event.Inputs.NonIndexed().Pack(c.PerPaymentLimit, c.DailyLimit)
	}
	if err != nil || use.expiry < uint64(included.Time.Unix()) ||
		!slices.Equal(log.Topics, topics) || !bytes.Equal(log.Data, eventData) {
		return nil
	}
	return &use
}

func typedV1Digest(chainID uint64, contract common.Address, typeHash, words []byte) common.Hash {
	chain := new(big.Int).SetUint64(chainID)
	separator := crypto.Keccak256(v1DomainType, v1DomainName, v1DomainVersion,
		common.LeftPadBytes(chain.Bytes(), 32), common.LeftPadBytes(contract[:], 32))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, separator, crypto.Keccak256(typeHash, words))
}

func recoverV1Device(digest common.Hash, signature []byte) common.Address {
	if len(signature) != 65 || (signature[64] != 27 && signature[64] != 28) {
		return common.Address{}
	}
	var normalized [65]byte
	copy(normalized[:], signature)
	normalized[64] -= 27
	if !crypto.ValidateSignatureValues(normalized[64], new(big.Int).SetBytes(normalized[:32]),
		new(big.Int).SetBytes(normalized[32:64]), true) {
		return common.Address{}
	}
	key, err := crypto.SigToPub(digest[:], normalized[:])
	if err != nil {
		return common.Address{}
	}
	return crypto.PubkeyToAddress(*key)
}
