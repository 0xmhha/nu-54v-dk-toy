package chainview

import (
	"math/big"
	"testing"
	"time"

	extended "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestV1GenerationSameLimitsDistinctTypedNonces(t *testing.T) {
	profile := Profile{ChainID: 8283, Version: 1, ABIHash: v1ABIHash,
		Contract: common.Address{19: 7}, Asset: common.Address{19: 8}}
	extension, err := extended.SettlementextMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	device := crypto.PubkeyToAddress(key.PublicKey)
	arguments := extension.Methods["setLimits"].Inputs
	event := extension.Events["LimitsChanged"]
	anchor := Header{Height: 11, Hash: common.Hash{31: 10}, Time: time.Unix(1600, 0)}
	separator := crypto.Keccak256Hash(
		crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")),
		crypto.Keccak256([]byte("NU54 Payment Settlement")), crypto.Keccak256([]byte("1")),
		common.LeftPadBytes(big.NewInt(8283).Bytes(), 32), common.LeftPadBytes(profile.Contract[:], 32))
	fixed, err := abi.NewType("bytes32", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[common.Hash]bool)
	for _, nonce := range []int64{0, 1} {
		change := extended.PaymentTypesLimitChange{
			ChainId: big.NewInt(8283), ContractAddress: profile.Contract, Expiry: 1600,
			PerPaymentLimit: big.NewInt(70), DailyLimit: big.NewInt(120), Nonce: big.NewInt(nonce),
		}
		encoded, err := (abi.Arguments{{Type: fixed}, arguments[0]}).Pack(
			crypto.Keccak256Hash([]byte("LimitChange(uint256 chainId,address contract,uint256 perPaymentLimit,uint256 dailyLimit,uint256 nonce,uint64 expiry)")), change)
		if err != nil {
			t.Fatal(err)
		}
		digest := crypto.Keccak256Hash([]byte{0x19, 0x01}, separator[:], crypto.Keccak256(encoded))
		signature, err := crypto.Sign(digest[:], key)
		if err != nil {
			t.Fatal(err)
		}
		signature[64] += 27
		data, err := extension.Pack("setLimits", change, signature)
		if err != nil {
			t.Fatal(err)
		}
		tx := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(8283), To: &profile.Contract, Data: data})
		emitted, err := event.Inputs.NonIndexed().Pack(change.PerPaymentLimit, change.DailyLimit)
		if err != nil {
			t.Fatal(err)
		}
		log := types.Log{Address: profile.Contract, Topics: []common.Hash{event.ID, common.BytesToHash(device[:])}, Data: emitted,
			TxHash: tx.Hash(), BlockHash: anchor.Hash, BlockNumber: uint64(anchor.Height)}
		got := decodeV1GenerationCall(profile, tx, log, types.ReceiptStatusSuccessful, anchor)
		if got == nil || got.kind != "limit" || got.device != device || got.nonce != common.BytesToHash(change.Nonce.Bytes()) ||
			got.digest != digest || seen[got.digest] {
			t.Fatalf("different nonce did not bind this limit occurrence: %v", got)
		}
		seen[got.digest] = true
		if got := decodeV1GenerationCall(profile, tx, log, types.ReceiptStatusFailed, anchor); got != nil {
			t.Fatal("failed receipt projected")
		}
		relay := common.Address{19: 97}
		indirect := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(8283), To: &relay, Data: data})
		relayLog := log
		relayLog.TxHash = indirect.Hash()
		if got := decodeV1GenerationCall(profile, indirect, relayLog, types.ReceiptStatusSuccessful, anchor); got != nil {
			t.Fatal("indirect settlement event projected as a direct approval")
		}
		suffix := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(8283), To: &profile.Contract,
			Data: append(append([]byte(nil), data...), 0)})
		suffixLog := log
		suffixLog.TxHash = suffix.Hash()
		if got := decodeV1GenerationCall(profile, suffix, suffixLog, types.ReceiptStatusSuccessful, anchor); got != nil {
			t.Fatal("noncanonical transaction suffix accepted")
		}
		log.Data[31] ^= 1
		if got := decodeV1GenerationCall(profile, tx, log, types.ReceiptStatusSuccessful, anchor); got != nil {
			t.Fatal("unmatched event projected")
		}
	}
}
