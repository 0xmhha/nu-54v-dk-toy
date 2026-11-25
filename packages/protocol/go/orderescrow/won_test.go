package orderescrow

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const wonName = "NU54 Test KRW"

func TestWonLockMatchesTheContract(t *testing.T) {
	v := vectors(t, "order-lock-won.vectors")
	confirmer := common.BytesToAddress(v["confirmer"])
	lock, err := NewLockFor(wonName, v["unsigned"], confirmer)
	if err != nil || !bytes.Equal(lock.Preimage, v["preimage"]) {
		t.Fatalf("preimage differs: %v", err)
	}
	payer := common.BytesToAddress(v["unsigned"][payerOffset : payerOffset+32])
	if !SignedBy(lock.Digest, v["payerSignature"], payer) {
		t.Fatal("won signature refused")
	}
	// The quote and the token address are the same; only the name differs.
	for _, name := range []string{TokenName, "NU54 Test KRWC"} {
		if other, err := NewLockFor(name, v["unsigned"], confirmer); err != nil || SignedBy(other.Digest, v["payerSignature"], payer) {
			t.Fatalf("a signature for %s was accepted as another token: %v", wonName, err)
		}
	}
	if old, err := NewLock(v["unsigned"], confirmer); err != nil || SignedBy(old.Digest, v["payerSignature"], payer) {
		t.Fatal("the dollar check accepted a won signature")
	}
}

func TestWonReturnMatchesTheContract(t *testing.T) {
	v := vectors(t, "wallet-return-won.vectors")
	digest, err := ReturnDigestFor(wonName, common.BytesToAddress(v["token"]), common.BytesToAddress(v["from"]),
		common.BytesToAddress(v["to"]), big.NewInt(3_420_000), 1_800_000_500, common.BytesToHash(v["nonce"]))
	if err != nil || digest != crypto.Keccak256Hash(v["preimage"]) {
		t.Fatalf("return digest differs: %x %v", digest, err)
	}
}

func TestTokenNameIsChecked(t *testing.T) {
	v := vectors(t, "order-lock-won.vectors")
	for _, name := range []string{"", " NU54", "NU54 ", "a\nb", "\uc6d0", strings.Repeat("a", 65)} {
		if _, err := NewLockFor(name, v["unsigned"], common.HexToAddress("0x1004")); err == nil {
			t.Errorf("lock accepted the name %q", name)
		}
		if _, err := ReturnDigestFor(name, common.HexToAddress("0x1"), common.HexToAddress("0x2"), common.HexToAddress("0x3"),
			big.NewInt(1), 1, common.HexToHash("0x1")); err == nil {
			t.Errorf("return accepted the name %q", name)
		}
	}
}
