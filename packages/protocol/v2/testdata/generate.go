// Command generate emits synthetic quote fixtures using go-ethereum's typed-data implementation.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	gethmath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

type schema struct {
	Domain struct{ Name, Version string }
	Quote  struct {
		Type   string
		Fields [][2]string
	}
}

func main() {
	path := "packages/protocol/v2/schema.json"
	rawSchema, err := os.ReadFile(path)
	must(err)
	var s schema
	must(json.Unmarshal(rawSchema, &s))
	// Public scalar 1 is a deterministic synthetic fixture only.
	key, err := crypto.ToECDSA(append(make([]byte, 31), 1))
	must(err)
	signer := crypto.PubkeyToAddress(key.PublicKey)
	wordCount := len(s.Quote.Fields) + 2
	raw := make([]byte, wordCount*32)
	setWord(raw, 0, 8283)
	setWord(raw, 1, 0x1001)
	for i := 2; i < wordCount; i++ {
		setWord(raw, i, int64(i))
	}
	setWord(raw, 17, 1_800_000_000)
	setWord(raw, 18, 1_800_000_120)
	setWord(raw, 19, 1)
	setWord(raw, 20, 2)
	fields := make([]apitypes.Type, len(s.Quote.Fields))
	for i, f := range s.Quote.Fields {
		fields[i] = apitypes.Type{Name: f[0], Type: f[1]}
	}
	digest := func(payload []byte, version string) []byte {
		message := apitypes.TypedDataMessage{}
		for i, f := range fields {
			word := payload[(i+2)*32 : (i+3)*32]
			switch f.Type {
			case "address":
				message[f.Name] = common.BytesToAddress(word).Hex()
			case "bytes32":
				message[f.Name] = common.BytesToHash(word).Hex()
			default:
				message[f.Name] = new(big.Int).SetBytes(word)
			}
		}
		typed := apitypes.TypedData{
			Types: apitypes.Types{
				"EIP712Domain": {{Name: "name", Type: "string"}, {Name: "version", Type: "string"},
					{Name: "chainId", Type: "uint256"}, {Name: "verifyingContract", Type: "address"}},
				s.Quote.Type: fields,
			},
			PrimaryType: s.Quote.Type,
			Domain: apitypes.TypedDataDomain{Name: s.Domain.Name, Version: version,
				ChainId:           (*gethmath.HexOrDecimal256)(new(big.Int).SetBytes(payload[:32])),
				VerifyingContract: common.BytesToAddress(payload[32:64]).Hex()},
			Message: message,
		}
		hash, _, err := apitypes.TypedDataAndHash(typed)
		must(err)
		return hash
	}
	sign := func(payload []byte, version string) []byte {
		sig, err := crypto.Sign(digest(payload, version), key)
		must(err)
		sig[64] += 27
		return sig
	}
	emit := func(id string, payload, sig, expected []byte, merchant common.Address, now int64, consumed bool, hash []byte, status int) {
		fmt.Printf("%s|%x|%x|%x|%x|%d|%t|%s|%d\n", id, append(slices.Clone(payload), sig...),
			expected, payload, merchant, now, consumed, hex.EncodeToString(hash), status)
	}
	const clock = 1_800_000_001
	sig := sign(raw, s.Domain.Version)
	expectedHash := digest(raw, s.Domain.Version)
	fmt.Println("# case|signedHex|expectedHex|unsignedHex|merchantSignerHex|now|consumed|digestHex|status")
	emit("kiosk-selected", raw, sig, raw, signer, clock, false, expectedHash, 0)
	emit("phone-private", raw, sig, raw, signer, clock, false, expectedHash, 0)
	emit("at-issue", raw, sig, raw, signer, clock-1, false, expectedHash, 0)
	emit("short-signature", raw, sig[:64], raw, signer, clock, false, expectedHash, 1)
	emit("trailing-byte", raw, append(slices.Clone(sig), 0), raw, signer, clock, false, expectedHash, 1)
	emit("before-issue", raw, sig, raw, signer, clock-2, false, expectedHash, 4)
	emit("at-expiry", raw, sig, raw, signer, 1_800_000_120, false, expectedHash, 4)
	emit("already-consumed", raw, sig, raw, signer, clock, true, expectedHash, 6)
	malformedPolicy := slices.Clone(raw)
	setWord(malformedPolicy, 12, 0)
	emit("invalid-policy-amount", raw, sig, malformedPolicy, signer, clock, false, expectedHash, 1)
	emit("invalid-policy-signer", raw, sig, raw, common.Address{}, clock, false, expectedHash, 1)
	largest := slices.Clone(raw)
	for i := 12 * 32; i < 13*32; i++ {
		largest[i] = 0xff
	}
	emit("max-uint256", largest, sign(largest, s.Domain.Version), largest, signer, clock, false,
		digest(largest, s.Domain.Version), 0)
	direct := slices.Clone(raw)
	setWord(direct, 19, 2)
	setWord(direct, 20, 1)
	emit("direct-profile-disabled", direct, sign(direct, s.Domain.Version), direct, signer, clock, false,
		digest(direct, s.Domain.Version), 2)
	emit("wrong-signer", raw, sig, raw, common.HexToAddress("0x9876"), clock, false, expectedHash, 5)
	emit("v1-domain", raw, sign(raw, "1"), raw, signer, clock, false, expectedHash, 5)
	badV := slices.Clone(sig)
	badV[64] = 0
	emit("invalid-v", raw, badV, raw, signer, clock, false, expectedHash, 5)
	highS := slices.Clone(sig)
	new(big.Int).Sub(crypto.S256().Params().N, new(big.Int).SetBytes(sig[32:64])).FillBytes(highS[32:64])
	highS[64] = 55 - sig[64]
	emit("high-s", raw, highS, raw, signer, clock, false, expectedHash, 5)
	for _, v := range []struct {
		id     string
		word   int
		value  int64
		status int
	}{
		{"cross-chain", 0, 1, 2}, {"cross-contract", 1, 0x1002, 3}, {"cross-tenant", 3, 22, 3},
		{"wrong-role", 20, 1, 3}, {"old-generation", 16, 3, 3}, {"wrong-price", 12, 1000, 3},
		{"wrong-recipient", 11, 0x1002, 3}, {"invalid-role", 20, 3, 1}, {"zero-amount", 12, 0, 1},
		{"other-claim", 7, 55, 3}, {"other-intent", 8, 56, 3}, {"zero-nonce", 13, 0, 1},
		{"nonpositive-duration", 18, 1_800_000_000, 1},
		{"direct-renter", 19, 2, 1},
	} {
		different := slices.Clone(raw)
		setWord(different, v.word, v.value)
		if v.status == 1 {
			emit(v.id, different, sig, raw, signer, clock, false, nil, v.status)
		} else {
			emit(v.id, different, sign(different, s.Domain.Version), raw, signer, clock, false, digest(different, s.Domain.Version), v.status)
		}
	}
	overwide := slices.Clone(raw)
	overwide[32] = 1
	emit("overwide-address", overwide, sig, raw, signer, clock, false, nil, 1)
}

func setWord(raw []byte, index int, n int64) {
	big.NewInt(n).FillBytes(raw[index*32 : (index+1)*32])
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
