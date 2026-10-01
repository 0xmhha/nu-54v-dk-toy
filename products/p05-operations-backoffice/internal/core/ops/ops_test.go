package ops

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

// Foundry public test mnemonic keys (test-only): index 1 operator, index 2 merchant.
const (
	operatorKey = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	merchantKey = "5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"
)

type vector struct {
	ID        string            `json:"id"`
	Message   map[string]string `json:"message"`
	Signature string            `json:"signature"`
	Signer    string            `json:"signer"`
}

func vectors(t *testing.T) (core.Domain, map[string]vector) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "docs", "content", "specifications", "protocol", "eip712-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Domain struct {
			ChainID           uint64 `json:"chainId"`
			VerifyingContract string `json:"verifyingContract"`
		} `json:"domain"`
		Vectors []json.RawMessage `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]vector{}
	for _, r := range doc.Vectors {
		var v vector
		_ = json.Unmarshal(r, &v) // vectors with non-string fields are not used here
		out[v.ID] = v
	}
	return core.Domain{ChainID: doc.Domain.ChainID, VerifyingContract: common.HexToAddress(doc.Domain.VerifyingContract)}, out
}

func key(t *testing.T, hex string) *ecdsa.PrivateKey {
	k, err := crypto.HexToECDSA(hex)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

type fakeRegistry struct {
	active      bool
	payout      common.Address
	pending     common.Address
	effectiveAt uint64
}

func (f fakeRegistry) MerchantStatus(common.Address) (bool, common.Address, error) {
	return f.active, f.payout, nil
}
func (f fakeRegistry) PendingPayoutOf(common.Address) (common.Address, uint64, error) {
	return f.pending, f.effectiveAt, nil
}

func u64(t *testing.T, s string) uint64 {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("not a number: %s", s)
	}
	return n.Uint64()
}

// P05-FR-02: with now = validFrom and the register validity, MA-01 is reproduced byte for byte.
func TestIssueAttestationReproducesVector(t *testing.T) {
	domain, vs := vectors(t)
	v := vs["MA-01"]
	merchant, payout := common.HexToAddress(v.Message["merchant"]), common.HexToAddress(v.Message["payout"])
	from, until := u64(t, v.Message["validFrom"]), u64(t, v.Message["validUntil"])
	a, err := IssueAttestation(fakeRegistry{active: true, payout: payout}, domain, key(t, operatorKey), merchant, payout, v.Message["name"], from, until-from)
	if err != nil {
		t.Fatal(err)
	}
	if hexutil.Encode(a.OperatorSignature) != v.Signature {
		t.Fatalf("signature %x, vector %s", a.OperatorSignature, v.Signature)
	}
}

// P05-FR-03 and FR-09.
func TestIssueAttestationRules(t *testing.T) {
	domain, _ := vectors(t)
	op := key(t, operatorKey)
	m, p, next := common.HexToAddress("0x01"), common.HexToAddress("0x02"), common.HexToAddress("0x03")
	cases := []struct {
		name   string
		reg    fakeRegistry
		payout common.Address
		want   error
		until  uint64
	}{
		{"revoked merchant", fakeRegistry{active: false, payout: p}, p, ErrNotActive, 0},
		{"payout differs from registry", fakeRegistry{active: true, payout: p}, next, ErrPayoutMismatch, 0},
		{"queued payout before it takes effect", fakeRegistry{active: true, payout: p, pending: next, effectiveAt: 1500}, next, ErrPayoutPending, 0},
		{"old payout while a change is queued ends at the effective time", fakeRegistry{active: true, payout: p, pending: next, effectiveAt: 1500}, p, nil, 1500},
		{"no change queued: full validity", fakeRegistry{active: true, payout: p}, p, nil, 1000 + 86400},
	}
	for _, c := range cases {
		a, err := IssueAttestation(c.reg, domain, op, m, c.payout, "Cafe", 1000, 86400)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err %v, want %v", c.name, err, c.want)
			continue
		}
		if err == nil {
			if a.ValidUntil != big.NewInt(int64(c.until)).String() {
				t.Errorf("%s: validUntil %s, want %d", c.name, a.ValidUntil, c.until)
			}
			d, _ := core.Digest(domain, "MerchantAttestation", map[string]any{"merchant": m.Hex(), "payout": c.payout.Hex(), "name": "Cafe", "validFrom": a.ValidFrom, "validUntil": a.ValidUntil})
			if signer, err := core.RecoverSigner(d, a.OperatorSignature); err != nil || signer != core.Address(op) {
				t.Errorf("%s: signer %s, err %v", c.name, signer.Hex(), err)
			}
		}
	}
}

// P05-FR-06: TA-01 is reproduced and a timestamp not after lastAnchor is refused.
func TestSignTimeAnchor(t *testing.T) {
	domain, vs := vectors(t)
	v := vs["TA-01"]
	ts := u64(t, v.Message["timestamp"])
	a, err := SignTimeAnchor(domain, key(t, operatorKey), common.HexToAddress(v.Message["device"]), ts, ts-1)
	if err != nil {
		t.Fatal(err)
	}
	if hexutil.Encode(a.OperatorSignature) != v.Signature {
		t.Fatalf("signature %x, vector %s", a.OperatorSignature, v.Signature)
	}
	if _, err := SignTimeAnchor(domain, key(t, operatorKey), common.HexToAddress(v.Message["device"]), ts, ts); !errors.Is(err, ErrAnchorNotLater) {
		t.Fatalf("equal timestamp: err %v", err)
	}
}

// P05-FR-04: the test merchant order reproduces MO-01.
func TestSignMerchantOrder(t *testing.T) {
	domain, vs := vectors(t)
	v := vs["MO-01"]
	amount, _ := new(big.Int).SetString(v.Message["amount"], 10)
	sig, err := SignMerchantOrder(domain, key(t, merchantKey), MerchantOrder{
		OrderID: common.HexToHash(v.Message["orderId"]), Token: common.HexToAddress(v.Message["token"]),
		Amount: amount, Payout: common.HexToAddress(v.Message["payout"]), Expiry: u64(t, v.Message["expiry"]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if hexutil.Encode(sig) != v.Signature {
		t.Fatalf("signature %x, vector %s", sig, v.Signature)
	}
}
