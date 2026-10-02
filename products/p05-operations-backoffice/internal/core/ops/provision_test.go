package ops

// The rental setup from the operator tool's side against the shared session vector SV-13: every
// body opsctl sends must equal the vector's `send` bytes, and the device's replies come from
// the vector's `expect` bytes, framed as on BLE.

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

type vectorStep struct {
	At     uint64   `json:"at"`
	Send   string   `json:"send"`
	Expect []string `json:"expect"`
}

func scenario(t *testing.T, id string) []vectorStep {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "docs", "content", "specifications", "protocol", "session-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Scenarios []struct {
			ID    string       `json:"id"`
			Steps []vectorStep `json:"steps"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, sc := range doc.Scenarios {
		if sc.ID == id {
			return sc.Steps
		}
	}
	t.Fatalf("no scenario %s", id)
	return nil
}

// replay is a device that answers each central message with the vector's replies.
type replay struct {
	t     *testing.T
	steps []vectorStep
	next  int
	rx    protocol.Reassembler
	tx    protocol.FrameWriter
	out   chan []byte
}

func newReplay(t *testing.T, steps []vectorStep) *replay {
	return &replay{t: t, steps: steps, tx: protocol.FrameWriter{MTU: 185}, out: make(chan []byte, 64)}
}

func (r *replay) MTU() int                     { return 185 }
func (r *replay) Notifications() <-chan []byte { return r.out }
func (r *replay) Close() error                 { return nil }
func (r *replay) Write(fragment []byte) error {
	body, err := r.rx.Feed(fragment)
	if err != nil || body == nil {
		return err
	}
	if r.next >= len(r.steps) {
		return nil // session.cancel at the end
	}
	st := r.steps[r.next]
	r.next++
	want, _ := hex.DecodeString(st.Send)
	if !bytes.Equal(body, want) {
		m, _ := protocol.DecodeMessage(body)
		r.t.Errorf("step %d: sent %v\n  got  %x\n  want %s", r.next-1, m["type"], body, st.Send)
	}
	for _, e := range st.Expect {
		b, _ := hex.DecodeString(e)
		frags, _ := r.tx.Write(b)
		for _, f := range frags {
			r.out <- f
		}
	}
	return nil
}

func vectorSession(t *testing.T, tr ble.Transport) *ble.Session {
	t.Helper()
	sid, _ := hex.DecodeString("0102030405060708")
	o := ble.Options{}
	copy(o.SessionID[:], sid)
	for i := range o.KioskNonce {
		o.KioskNonce[i] = 0xa5
	}
	s, err := ble.Open(context.Background(), tr, o)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestProvisionMatchesSessionVectors(t *testing.T) {
	steps := scenario(t, "SV-13")[:3] // open, setup.operator, setup.timeAnchor
	tr := newReplay(t, steps)
	s := vectorSession(t, tr)
	if s.State != "UNPROVISIONED" || s.Device != "" {
		t.Fatalf("opened %s %q", s.State, s.Device)
	}
	op, _ := crypto.HexToECDSA(operatorKey)
	var deposited common.Address
	var progress []string
	res, err := Provision(context.Background(), s, ProvisionParams{
		Domain:   core.Domain{ChainID: 8283, VerifyingContract: common.HexToAddress("0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0")},
		Operator: op, Passkey: 42195, Amount: big.NewInt(10_000_000), Withdraw: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Progress: func(s string) { progress = append(progress, s) },
	},
		func(context.Context) (uint64, error) { return steps[2].At, nil },
		func(_ context.Context, d common.Address, _ *big.Int, _ common.Address) (common.Hash, error) {
			deposited = d
			return common.Hash{1}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if tr.next != 3 {
		t.Fatalf("sent %d of 3 vector messages", tr.next)
	}
	if res.Passkey != "042195" || res.LastAnchor != steps[2].At || deposited != res.Device {
		t.Fatalf("result %+v, deposited for %s", res, deposited.Hex())
	}
	if len(progress) != 3 {
		t.Fatalf("progress %v", progress)
	}
}

func TestProvisionStopsWhenTheRenterRefuses(t *testing.T) {
	steps := scenario(t, "SV-14")[:2]
	s := vectorSession(t, newReplay(t, steps))
	op, _ := crypto.HexToECDSA(operatorKey)
	_, err := Provision(context.Background(), s, ProvisionParams{
		Domain: core.Domain{ChainID: 8283, VerifyingContract: common.HexToAddress("0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0")}, Operator: op, Passkey: 42195,
	}, func(context.Context) (uint64, error) { t.Fatal("no anchor after a refusal"); return 0, nil }, nil)
	var refused *ble.RefusedError
	if !errors.As(err, &refused) || refused.Step != "setup.operator" || refused.Reason != "USER_REJECTED" {
		t.Fatalf("got %v", err)
	}
}

func TestProvisionStopsOnPinTimeout(t *testing.T) {
	steps := scenario(t, "SV-15")[:2]
	s := vectorSession(t, newReplay(t, steps))
	op, _ := crypto.HexToECDSA(operatorKey)
	_, err := Provision(context.Background(), s, ProvisionParams{
		Domain: core.Domain{ChainID: 8283, VerifyingContract: common.HexToAddress("0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0")}, Operator: op, Passkey: 42195,
	}, func(context.Context) (uint64, error) { t.Fatal("no anchor without a key"); return 0, nil }, nil)
	var refused *ble.RefusedError
	if !errors.As(err, &refused) || refused.Step != "keygen" || refused.Reason != "TIMEOUT" {
		t.Fatalf("got %v", err)
	}
}
