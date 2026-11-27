package orderescrow

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func vectors(t *testing.T, name string) map[string][]byte {
	t.Helper()
	raw, err := os.ReadFile("../../v2/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		key, value, _ := strings.Cut(line, "=")
		out[key] = common.FromHex(value)
	}
	return out
}

func TestLockMatchesBoardAndContract(t *testing.T) {
	v := vectors(t, "order-lock.vectors")
	lock, err := NewLock(v["unsigned"], common.BytesToAddress(v["confirmer"]))
	if err != nil || !bytes.Equal(lock.Preimage, v["preimage"]) {
		t.Fatalf("preimage differs: %v", err)
	}
	payer := common.BytesToAddress(v["unsigned"][payerOffset : payerOffset+32])
	if !SignedBy(lock.Digest, v["payerSignature"], payer) {
		t.Fatal("board signature refused")
	}
	if other, _ := NewLock(v["unsigned"], common.HexToAddress("0x1005")); SignedBy(other.Digest, v["payerSignature"], payer) {
		t.Fatal("another phone key accepted")
	}
	tampered := bytes.Clone(v["unsigned"])
	tampered[amountOffset+31] ^= 1
	if other, err := NewLock(tampered, common.BytesToAddress(v["confirmer"])); err == nil && SignedBy(other.Digest, v["payerSignature"], payer) {
		t.Fatal("another amount accepted")
	}
	if _, err := NewLock(v["unsigned"], common.Address{}); err == nil {
		t.Fatal("no phone key accepted")
	}
	if _, err := NewLock(v["unsigned"][:671], common.HexToAddress("0x1")); err == nil {
		t.Fatal("short quote accepted")
	}
}

func TestActionsMatchContract(t *testing.T) {
	v := vectors(t, "order-actions.vectors")
	escrow, merchant, order := common.HexToAddress("0x1002"), common.HexToHash("0x7"), common.HexToHash("0x5")
	for name, action := range map[string]Action{"delivered": Delivered, "cancel": Cancel, "receipt": Receipt, "dispute": Dispute, "withdraw": Withdraw} {
		at := uint64(0)
		if _, buyer := BuyerActions[name]; buyer {
			at = 1_800_172_800
		}
		digest, err := ActionDigest(action, escrow, merchant, order, at)
		if err != nil || !bytes.Equal(digest[:], v[name]) {
			t.Errorf("%s: %x %v", name, digest, err)
		}
	}
	if _, err := ActionDigest(Receipt, escrow, merchant, order, 0); err == nil {
		t.Error("receipt before delivery accepted")
	}
	if _, err := ActionDigest(Delivered, escrow, merchant, order, 1); err == nil {
		t.Error("store action with a delivery time accepted")
	}
	if _, err := ActionDigest(Action("Other()"), escrow, merchant, order, 0); err == nil {
		t.Error("unknown action accepted")
	}
}

func TestBoardReturnAndAccountProofs(t *testing.T) {
	v := vectors(t, "wallet-return.vectors")
	digest, err := ReturnDigest(common.BytesToAddress(v["token"]), common.BytesToAddress(v["from"]), common.BytesToAddress(v["to"]),
		big.NewInt(9_500_000), 1_800_000_500, common.BytesToHash(v["nonce"]))
	if err != nil || digest != crypto.Keccak256Hash(v["preimage"]) {
		t.Fatalf("return digest differs: %x %v", digest, err)
	}
	if signature, ok := v["signature"]; ok && !SignedBy(digest, signature, common.BytesToAddress(v["from"])) {
		t.Fatal("board return signature refused")
	}
	// Pinned in products/p01-device-firmware/test/test_order.c.
	account := common.HexToAddress("0x3004df73ebac3f1a2d5a0ea01ed39503139b9dcb")
	if got := AccountProofDigest(account, [32]byte(bytes.Repeat([]byte{0x11}, 32))); hex.EncodeToString(got[:]) !=
		"6ac8be6e20f7c3971e894cfd18e30676ef1032d039b624d7a244cf7742b490c8" {
		t.Fatalf("account proof differs: %x", got)
	}
	challenge := EnrollChallenge("t", "b", "s", account, 3, "k")
	if hex.EncodeToString(challenge[:]) != enrollVector {
		t.Fatalf("enroll challenge differs: %x", challenge)
	}
}

// Also in packages/protocol/ts/test/order-escrow.test.ts.
const enrollVector = "abdcac8f60735d1edf9ef7ae849eec2a48aa839705e68386170e276043388427"
