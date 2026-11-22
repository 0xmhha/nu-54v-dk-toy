package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestRentalCloseTransactionIdentity(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	dep := Deployment{ChainID: 8283, Settlement: common.Address{19: 17}}
	device := common.Address{19: 18}
	method := crypto.Keccak256([]byte("closeAccount(address)"))
	memo := append(append([]byte(nil), method[:4]...), common.LeftPadBytes(device[:], 32)...)
	build := func(chain uint64, target common.Address, value int64, data []byte) []byte {
		t.Helper()
		tx := types.NewTx(&types.DynamicFeeTx{ChainID: new(big.Int).SetUint64(chain), Nonce: 42,
			GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2), Gas: 90000, To: &target, Value: big.NewInt(value), Data: data})
		ready, err := types.SignTx(tx, types.LatestSignerForChainID(tx.ChainId()), key)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := ready.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	operator := crypto.PubkeyToAddress(key.PublicKey)
	ok := build(8283, dep.Settlement, 0, memo)
	tx, err := parseRentalCloseTransaction(dep, operator, device, ok)
	if err != nil || tx.Nonce() != 42 || tx.Hash() == (common.Hash{}) {
		t.Fatalf("original unsigned intent not recovered: %v", err)
	}
	for _, raw := range [][]byte{build(1, dep.Settlement, 0, memo), build(8283, device, 0, memo), build(8283, dep.Settlement, 1, memo),
		build(8283, dep.Settlement, 0, append(append([]byte(nil), memo...), 0))} {
		if _, err := parseRentalCloseTransaction(dep, operator, device, raw); err == nil {
			t.Fatal("wrong protected close admitted")
		}
	}
	if _, err := parseRentalCloseTransaction(dep, device, device, ok); err == nil {
		t.Fatal("different signer admitted")
	}
}
