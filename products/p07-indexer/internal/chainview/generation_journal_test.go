package chainview

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestSharedRJP1Projection(t *testing.T) {
	file, err := os.ReadFile("../../../../packages/protocol/v2/testdata/rjp1.vectors")
	if err != nil {
		t.Fatal(err)
	}
	tokens := strings.Fields(string(file))
	if len(tokens) != 2 {
		t.Fatal("invalid shared vector")
	}
	wire, err := hex.DecodeString(tokens[0])
	if err != nil {
		t.Fatal(err)
	}
	j, err := readGenerationJournal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if j.root.Hex()[2:] != tokens[1] || len(j.entries) != 3 || j.binding.Chain != 8283 ||
		j.entries[0].nonce != common.HexToHash("0xff00") || j.entries[1].nonce != common.HexToHash("0xff01") ||
		j.entries[0].kind != "payment" || j.entries[0].expiry != 1700000121 ||
		j.entries[1].kind != "limit" || !j.entries[2].burned {
		t.Fatal("RJP1 parser differed from the cross-product public fixture")
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { b[243] = 1 },
		func(b []byte) { b[275] = 4 },
		func(b []byte) { b[276] = 2 },
		func(b []byte) { b[generationHeaderBytes+1] = 0 },
		func(b []byte) { b[generationHeaderBytes+2] = 1 },
		func(b []byte) { b[generationHeaderBytes+generationEntryBytes+42] = 1 },
	} {
		copy := append([]byte(nil), wire...)
		mutate(copy)
		if _, err := readGenerationJournal(copy); err == nil {
			t.Fatal("invalid projection accepted")
		}
	}
	if _, err := readGenerationJournal(append(wire, make([]byte, 65)...)); err == nil {
		t.Fatal("projection accepted extra signature bytes")
	}
}

func TestSharedRFR1Receipt(t *testing.T) {
	read := func(name string) []string {
		t.Helper()
		file, err := os.ReadFile("../../../../packages/protocol/v2/testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Fields(string(file))
	}
	projection, err := hex.DecodeString(read("rjp1.vectors")[0])
	if err != nil {
		t.Fatal(err)
	}
	j, err := readGenerationJournal(projection)
	if err != nil {
		t.Fatal(err)
	}
	vector := read("rfr1.vectors")
	if len(vector) != 3 {
		t.Fatal("invalid shared board receipt vector")
	}
	raw, err := hex.DecodeString(vector[0])
	if err != nil {
		t.Fatal(err)
	}
	var approvedChallenge common.Hash
	for index := range approvedChallenge {
		approvedChallenge[index] = 0x72
	}
	scope := GenerationReceiptAuthority{
		Binding: j.binding, BoardSigner: common.HexToAddress(vector[2]),
		Operator: common.Address{19: 70}, Challenge: approvedChallenge, ChallengeUntil: time.Unix(1800000090, 0),
	}
	now := time.Unix(1800000001, 0)
	got, err := VerifyGenerationReceipt(raw, scope, now)
	if err != nil || got.ReceiptDigest.Hex()[2:] != vector[1] || got.Root != j.root || got.FirstNonce != j.first ||
		got.NextNonce != j.next || got.EntryCount != 3 || got.FrozenAt.Unix() != 1800000000 {
		t.Fatalf("board receipt disagrees with shared fixture: %v", err)
	}
	for _, mutate := range []func(*GenerationReceiptAuthority){
		func(s *GenerationReceiptAuthority) { s.Challenge[0] ^= 1 },
		func(s *GenerationReceiptAuthority) { s.BoardSigner = s.Binding.Account },
		func(s *GenerationReceiptAuthority) { s.BoardSigner = s.Operator },
		func(s *GenerationReceiptAuthority) { s.Binding.Deployment[0] ^= 1 },
		func(s *GenerationReceiptAuthority) { s.Binding.Settlement[0] ^= 1 },
		func(s *GenerationReceiptAuthority) { s.ChallengeUntil = now },
	} {
		wrong := scope
		mutate(&wrong)
		if _, err := VerifyGenerationReceipt(raw, wrong, now); err == nil {
			t.Fatal("different registered authority accepted the receipt")
		}
	}
	for _, offset := range []int{68, 108, 162, 171, 178, 187, 190, 252} {
		altered := append([]byte(nil), raw...)
		altered[offset] ^= 1
		if _, err := VerifyGenerationReceipt(altered, scope, now); err == nil {
			t.Fatal("modified detached receipt accepted")
		}
	}
	if _, err := VerifyGenerationReceipt(raw, scope, time.Unix(1800000090, 0)); err == nil {
		t.Fatal("expired detached receipt accepted")
	}
}
