package rentalv1

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func cleanupVector(t *testing.T) [][]byte {
	t.Helper()
	raw, err := os.ReadFile("../../v2/testdata/rental-cleanup.vectors")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(raw))
	sizes := []int{CleanupIntentSize, 32, CleanupReceiptSize, 32, 20, 20, 20}
	if len(fields) != len(sizes) {
		t.Fatal("invalid vector count")
	}
	out := make([][]byte, len(sizes))
	for i, field := range fields {
		out[i], err = hex.DecodeString(field)
		if err != nil || len(out[i]) != sizes[i] {
			t.Fatalf("invalid vector field %d", i)
		}
	}
	return out
}

func TestRentalCleanupProof(t *testing.T) {
	raw := cleanupVector(t)
	scope := CleanupScope{
		ChainID: 8283, Settlement: common.Address(raw[4]), Operator: common.Address(raw[5]), Board: common.Address(raw[6]),
		Binding: [32]byte{1}, Disposition: [32]byte{2}, Request: [32]byte{3},
	}
	intent, err := ParseCleanupIntent(raw[0])
	if err != nil || !bytes.Equal(intent.Bytes(), raw[0]) || intent.Digest(scope.ChainID, scope.Settlement) != [32]byte(raw[1]) {
		t.Fatal("intent encoding or digest differs")
	}
	receipt, err := ParseCleanupReceipt(raw[2])
	if err != nil || !bytes.Equal(receipt.Bytes(), raw[2]) || receipt.Digest(scope.ChainID, scope.Settlement) != [32]byte(raw[3]) {
		t.Fatal("receipt encoding or digest differs")
	}
	for _, now := range []uint64{1000, 1119} {
		if scope.VerifyIntent(intent, now) != nil {
			t.Fatalf("live intent refused at %d", now)
		}
	}
	if scope.VerifyIntent(intent, 999) == nil || scope.VerifyIntent(intent, 1120) == nil ||
		scope.VerifyReceipt(intent, receipt, 1059) == nil || scope.VerifyReceipt(intent, receipt, 1200) != nil {
		t.Fatal("intent time window or historical receipt verification differs")
	}
	for name, change := range map[string]func(*CleanupScope){
		"another operator":           func(s *CleanupScope) { s.Operator[0] ^= 1 },
		"another generation binding": func(s *CleanupScope) { s.Binding[0] ^= 1 },
		"another disposition":        func(s *CleanupScope) { s.Disposition[0] ^= 1 },
		"another request":            func(s *CleanupScope) { s.Request[0] ^= 1 },
		"another chain":              func(s *CleanupScope) { s.ChainID++ },
		"another settlement":         func(s *CleanupScope) { s.Settlement[0] ^= 1 },
	} {
		t.Run(name, func(t *testing.T) {
			other := scope
			change(&other)
			if other.VerifyIntent(intent, 1000) == nil || other.VerifyReceipt(intent, receipt, 1200) == nil {
				t.Fatal("proof crossed scopes")
			}
		})
	}
	wrongBoard := scope
	wrongBoard.Board[0] ^= 1
	if wrongBoard.VerifyReceipt(intent, receipt, 1200) == nil {
		t.Fatal("receipt accepted for another enrolled board key")
	}
	operatorRaw, boardRaw := make([]byte, 32), make([]byte, 32)
	operatorRaw[31], boardRaw[31] = 1, 2
	operator, err := crypto.ToECDSA(operatorRaw)
	if err != nil {
		t.Fatal(err)
	}
	board, err := crypto.ToECDSA(boardRaw)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(digest [32]byte, key *ecdsa.PrivateKey) [65]byte {
		t.Helper()
		sig, err := crypto.Sign(digest[:], key)
		if err != nil {
			t.Fatal(err)
		}
		sig[64] += 27
		return [65]byte(sig)
	}
	other := scope
	other.Request[0] ^= 1
	otherIntent := intent
	otherIntent.Request = other.Request
	otherIntent.OperatorSignature = sign(otherIntent.Digest(other.ChainID, other.Settlement), operator)
	if other.VerifyIntent(otherIntent, 1000) != nil || other.VerifyReceipt(otherIntent, receipt, 1200) == nil {
		t.Fatal("receipt replayed against another validly signed request")
	}
	for _, kind := range []string{
		"RentalSlotCredentialReceipt(bytes32 credential,bytes32 challenge,uint64 checkedAt)",
		"RentalSlotOpenReceipt(bytes32 credential,bytes32 challenge,uint64 checkedAt)",
	} {
		// Even the enrolled board's real legacy typed signature, repacked
		// into the new length and magic, cannot substantiate retirement.
		legacyType := crypto.Keccak256([]byte(kind))
		var challenge, checkedAt [32]byte
		challenge[0], checkedAt[30], checkedAt[31] = 7, 4, 36
		legacyDigest := cleanupDomain(scope.ChainID, scope.Settlement, crypto.Keccak256Hash(legacyType, receipt.Intent[:], challenge[:], checkedAt[:]))
		repacked := receipt
		repacked.BoardSignature = sign(legacyDigest, board)
		if scope.VerifyReceipt(intent, repacked, 1200) == nil {
			t.Fatalf("legacy typed signature accepted for %s", kind)
		}
	}
	for _, invalid := range [][]byte{[]byte("setup.ack"), raw[2][:len(raw[2])-1], append(receipt.Bytes(), 0)} {
		if _, err := ParseCleanupReceipt(invalid); err == nil {
			t.Fatal("malformed or legacy response accepted")
		}
	}
}
