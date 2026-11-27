package wallet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
)

const TestProfile = "sepolia-native-test-v1"
const TestChain = "11155111"

type Transfer struct {
	Profile        string          `json:"profile"`
	Chain          string          `json:"chain"`
	To             string          `json:"to"`
	ValueWei       string          `json:"valueWei"`
	Nonce          string          `json:"nonce"`
	MaxFeeWei      string          `json:"maxFeeWei"`
	PriorityFeeWei string          `json:"priorityFeeWei"`
	Market         *MarketTransfer `json:"market,omitempty"`
}

var decimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,37})$`)

func (t Transfer) hashes() ([32]byte, [32]byte, error) {
	var digest, intent [32]byte
	raw, err := t.unsigned()
	if err != nil {
		return digest, intent, err
	}
	h := sha3.NewLegacyKeccak256()
	h.Write(raw)
	h.Sum(digest[:0])
	data, err := json.Marshal(t)
	if err != nil {
		return digest, intent, err
	}
	intent = sha256.Sum256(append([]byte("nu54/p03/transfer/v1\x00"), data...))
	return digest, intent, nil
}

func (t Transfer) unsigned() ([]byte, error) {
	invalid := errors.New("invalid Sepolia native transfer")
	if t.Market != nil || t.Profile != TestProfile || t.Chain != TestChain || !strings.HasPrefix(t.To, "0x") || len(t.To) != 42 || t.To != strings.ToLower(t.To) {
		return nil, invalid
	}
	to, err := hex.DecodeString(t.To[2:])
	if err != nil || new(big.Int).SetBytes(to).Sign() == 0 {
		return nil, invalid
	}
	num := func(value string, bits int) *big.Int {
		if !decimal.MatchString(value) {
			return nil
		}
		n, _ := new(big.Int).SetString(value, 10)
		if n.BitLen() > bits {
			return nil
		}
		return n
	}
	value, nonce, maxFee, priority := num(t.ValueWei, 128), num(t.Nonce, 64), num(t.MaxFeeWei, 128), num(t.PriorityFeeWei, 128)
	if value == nil || value.Sign() == 0 || nonce == nil || maxFee == nil || maxFee.Sign() == 0 || priority == nil || priority.Cmp(maxFee) > 0 {
		return nil, invalid
	}
	chain, _ := new(big.Int).SetString(TestChain, 10)
	return append([]byte{2}, rlpList(
		rlpBytes(chain.Bytes()), rlpBytes(nonce.Bytes()), rlpBytes(priority.Bytes()), rlpBytes(maxFee.Bytes()),
		rlpBytes(big.NewInt(21000).Bytes()), rlpBytes(to), rlpBytes(value.Bytes()), rlpBytes(nil), rlpList(),
	)...), nil
}

func rlpBytes(v []byte) []byte {
	if len(v) == 1 && v[0] < 0x80 {
		return append([]byte{}, v...)
	}
	return append([]byte{0x80 + byte(len(v))}, v...)
}

func rlpList(items ...[]byte) []byte {
	var payload []byte
	for _, item := range items {
		payload = append(payload, item...)
	}
	if len(payload) < 56 {
		return append([]byte{0xc0 + byte(len(payload))}, payload...)
	}
	return append([]byte{0xf8, byte(len(payload))}, payload...)
}

func address(pub []byte) string {
	if len(pub) != 65 || pub[0] != 4 {
		return ""
	}
	d := sha3.NewLegacyKeccak256()
	d.Write(pub[1:])
	return "0x" + hex.EncodeToString(d.Sum(nil)[12:])
}

func matchesRecovery(pub []byte, digest [32]byte, der []byte, recoveryID byte) bool {
	signature, err := ecdsa.ParseDERSignature(der)
	if err != nil || recoveryID > 1 {
		return false
	}
	r, s := signature.R(), signature.S()
	if s.IsOverHalfOrder() {
		return false
	}
	compact := [65]byte{27 + recoveryID}
	r.PutBytesUnchecked(compact[1:33])
	s.PutBytesUnchecked(compact[33:])
	key, _, err := ecdsa.RecoverCompact(compact[:], digest[:])
	return err == nil && string(key.SerializeUncompressed()) == string(pub)
}

// ReviewTransfer recomputes the canonical wallet intent for an independent
// approval authority before it signs a ceremony fingerprint.
func ReviewTransfer(t Transfer, digest, intent [32]byte) bool {
	d, i, err := t.hashes()
	return err == nil && d == digest && i == intent
}
