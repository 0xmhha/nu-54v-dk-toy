package protocol

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// roleAddress is the address each signer role must use in the vectors: indexes 0, 1 and 2
// of Foundry's public test mnemonic ("test test ... junk"). A vector signed with the wrong
// role's key fails here even when its digest is right ([N21]).
var roleAddress = map[string]common.Address{
	"device":   common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"),
	"operator": common.HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8"),
	"merchant": common.HexToAddress("0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC"),
}

// halfOrder is secp256k1n / 2; the contract and the device accept low-s signatures only.
var halfOrder = hexutil.MustDecodeBig("0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0")

// TestSignersMatchRoles recovers the signer of every vector from its digest and signature
// and checks it against the vector's signer and the expected role address.
func TestSignersMatchRoles(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "content", "specifications", "protocol", "eip712-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Vectors []struct {
			ID         string `json:"id"`
			SignerRole string `json:"signerRole"`
			Signer     string `json:"signer"`
			Digest     string `json:"digest"`
			Signature  string `json:"signature"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, v := range doc.Vectors {
		want, ok := roleAddress[v.SignerRole]
		if !ok {
			t.Errorf("%s: unknown signer role %q", v.ID, v.SignerRole)
			continue
		}
		seen[v.SignerRole] = true
		if common.HexToAddress(v.Signer) != want || !strings.EqualFold(v.Signer, want.Hex()) {
			t.Errorf("%s: signer %s, role %s expects %s", v.ID, v.Signer, v.SignerRole, want.Hex())
		}
		sig := hexutil.MustDecode(v.Signature)
		if len(sig) != 65 {
			t.Fatalf("%s: signature is %d bytes, want 65", v.ID, len(sig))
		}
		if s := new(big.Int).SetBytes(sig[32:64]); s.Cmp(halfOrder) > 0 {
			t.Errorf("%s: high-s signature", v.ID)
		}
		if sig[64] != 27 && sig[64] != 28 {
			t.Errorf("%s: v = %d, want 27 or 28", v.ID, sig[64])
		}
		rsv := append([]byte{}, sig...)
		rsv[64] -= 27 // go-ethereum expects a recovery id of 0 or 1
		pub, err := crypto.SigToPub(hexutil.MustDecode(v.Digest), rsv)
		if err != nil {
			t.Errorf("%s: recover: %v", v.ID, err)
			continue
		}
		if got := crypto.PubkeyToAddress(*pub); got != want {
			t.Errorf("%s: recovered %s, role %s expects %s", v.ID, got.Hex(), v.SignerRole, want.Hex())
		}
	}
	for role := range roleAddress {
		if !seen[role] {
			t.Errorf("no vector is signed by the %s role", role)
		}
	}
}
