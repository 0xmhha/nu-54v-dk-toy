package ops

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

func rentalVector(t *testing.T, name string) [][]byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "packages", "protocol", "v2", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var out [][]byte
	for _, token := range bytes.Fields(raw) {
		decoded, err := hex.DecodeString(string(token))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, decoded)
	}
	if len(out) != 2 {
		t.Fatalf("%s: unexpected number of fields", name)
	}
	return out
}

func TestProtectedRentalFreezeV2VectorAndIssueGate(t *testing.T) {
	projection := rentalVector(t, "rjp1.vectors")[0]
	vector := rentalVector(t, "rfq2.vectors")
	wire := vector[0]
	if len(projection) < 277 || len(wire) != 76 || string(wire[:4]) != "RFQ2" {
		t.Fatal("invalid fixture layout")
	}
	domain := core.Domain{
		ChainID: binary.BigEndian.Uint64(projection[164:172]), VerifyingContract: common.BytesToAddress(projection[172:192]),
	}
	request := RentalFreezeRequestV2{
		Binding: crypto.Keccak256Hash(projection[4:212]), Challenge: common.BytesToHash(wire[4:36]),
		ChannelNonce: common.BytesToHash(wire[36:68]), ExpiresAt: binary.BigEndian.Uint64(wire[68:76]),
	}
	digest, err := request.Digest(domain)
	if err != nil || !bytes.Equal(digest, vector[1]) {
		t.Fatalf("typed request digest mismatch: %x %v", digest, err)
	}
	operator, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	channel := ble.ProtectedRentalChallenge{Nonce: request.ChannelNonce, ExpiresAt: request.ExpiresAt}
	denied := errors.New("denied by the commissioning authority")
	if sig, err := IssueRentalFreezeV2(domain, operator, request, channel, request.ExpiresAt-1,
		func(core.Domain, RentalFreezeRequestV2) error { return denied }); sig != nil || !errors.Is(err, denied) {
		t.Fatalf("bypassed the registration authority: %v", err)
	}
	allowed := func(d core.Domain, r RentalFreezeRequestV2) error {
		if d != domain || r != request {
			return denied
		}
		return nil
	}
	wrongLink := channel
	wrongLink.Nonce[0] ^= 1
	if sig, err := IssueRentalFreezeV2(domain, operator, request, wrongLink, request.ExpiresAt-1, allowed); sig != nil || err == nil {
		t.Fatal("issuer accepted a proof for another link")
	}
	signature, err := IssueRentalFreezeV2(domain, operator, request, channel, request.ExpiresAt-1, allowed)
	if err != nil || len(signature) != 65 {
		t.Fatalf("issue failed: %v", err)
	}
	signature[64] -= 27
	pub, err := crypto.SigToPub(digest, signature)
	if err != nil || crypto.PubkeyToAddress(*pub) != crypto.PubkeyToAddress(operator.PublicKey) {
		t.Fatalf("typed request signer mismatch: %v", err)
	}
}
