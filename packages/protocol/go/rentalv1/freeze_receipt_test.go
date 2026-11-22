package rentalv1

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestFreezeReceiptSharedVectorAndStrictWire(t *testing.T) {
	vector, err := os.ReadFile("../../v2/testdata/rfr1.vectors")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(vector))
	raw, err := hex.DecodeString(fields[0])
	if err != nil {
		t.Fatal(err)
	}
	wantDigest := fields[1]
	projectionVector, err := os.ReadFile("../../v2/testdata/rjp1.vectors")
	if err != nil {
		t.Fatal(err)
	}
	projected, err := hex.DecodeString(strings.Fields(string(projectionVector))[0])
	if err != nil {
		t.Fatal(err)
	}
	settlement := [20]byte(projected[172:192])
	receipt, err := ParseFreezeReceipt(raw)
	if err != nil || receipt.EntryCount != 3 || !bytes.Equal(receipt.Bytes(), raw) ||
		!bytes.Equal(receipt.Root[:], raw[68:100]) || !bytes.Equal(receipt.FirstNonce[:], projected[212:244]) ||
		!bytes.Equal(receipt.NextNonce[:], projected[244:276]) {
		t.Fatal("RFR1 encoding disagreed with the shared projection")
	}
	digest := receipt.Digest(8283, settlement)
	if hex.EncodeToString(digest[:]) != wantDigest {
		t.Fatal("RFR1 typed signature digest disagreed with the shared vector")
	}
	receipt.BoardSignature[0] ^= 1
	if receipt.Digest(8283, settlement) != digest {
		t.Fatal("detached signature changed the signed digest")
	}
	receipt.NextNonce[0] ^= 1
	if receipt.Digest(8283, settlement) == digest ||
		receipt.Digest(8284, settlement) == receipt.Digest(8283, settlement) {
		t.Fatal("typed digest failed to bind exact nonce and chain")
	}
	badMagic := bytes.Clone(raw)
	badMagic[3] = '2'
	for _, invalid := range [][]byte{nil, raw[:4], raw[:FreezeReceiptSize-1], append(bytes.Clone(raw), 0), badMagic} {
		if _, err := ParseFreezeReceipt(invalid); err == nil {
			t.Fatal("RFR1 accepted malformed wire")
		}
	}
}
