package ops

// The return (P05-FR-07) against a fake chain and the shared session vector SV-17: the reset
// bytes opsctl sends equal the vector's, and the reset never goes out before the account is
// closed and AccountClosed is final, nor to another device.

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

// The vectors' device is Foundry public test key 0.
var vectorDevice = common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")

type fakeReturnChain struct {
	acct      core.Account
	closeErr  error
	closed    bool
	finalized bool // AccountClosed visible at finalized
	calls     []string
}

func (c *fakeReturnChain) AccountOf(context.Context, common.Address) (core.Account, error) {
	c.calls = append(c.calls, "accountOf")
	a := c.acct
	if c.closed {
		a.ClosedAt, a.WithdrawAfter = 1_790_000_000, 1_790_259_200
	}
	return a, nil
}

func (c *fakeReturnChain) CloseAccount(context.Context, common.Address) (common.Hash, error) {
	c.calls = append(c.calls, "closeAccount")
	if c.closeErr != nil {
		return common.Hash{}, c.closeErr
	}
	c.closed, c.finalized = true, true
	return common.Hash{0xc1}, nil
}

func (c *fakeReturnChain) AccountClosedBlock(context.Context, common.Address) (uint64, common.Hash, bool, error) {
	c.calls = append(c.calls, "closedEvent")
	return 1, common.Hash{0xc1}, c.finalized, nil // block 1: the vector's DeviceReset nonce
}

func paymentSession(t *testing.T, tr ble.Transport) *ble.Session {
	t.Helper()
	sid, _ := hex.DecodeString("0102030405060708")
	o := ble.Options{Mode: "payment"}
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

func returnParams(t *testing.T) ReturnParams {
	op, _ := crypto.HexToECDSA(operatorKey)
	return ReturnParams{
		Domain:   core.Domain{ChainID: 8283, VerifyingContract: common.HexToAddress("0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0")},
		Operator: op, Device: vectorDevice, FinalWait: 50 * time.Millisecond, Poll: 10 * time.Millisecond,
	}
}

func TestReturnClosesThenSendsTheVectorReset(t *testing.T) {
	sv := scenario(t, "SV-17")
	// SV-17: [2] session.open (payment, READY device), [5] device.reset by the operator for this device.
	tr := newReplay(t, []vectorStep{sv[2], sv[5]})
	chain := &fakeReturnChain{acct: core.Account{Exists: true, Balance: big.NewInt(9_000_000)}}
	res, err := Return(context.Background(), chain, func(context.Context) (ResetSession, error) { return paymentSession(t, tr), nil }, returnParams(t))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reset || res.ResetNonce != 1 || res.WithdrawAfter == 0 || tr.next != 2 {
		t.Fatalf("result %+v, %d vector messages sent", res, tr.next)
	}
	if strings.Join(chain.calls[:3], ",") != "accountOf,closeAccount,closedEvent" {
		t.Fatalf("order %v", chain.calls)
	}
}

func TestReturnAgainSkipsTheClose(t *testing.T) {
	sv := scenario(t, "SV-17")
	tr := newReplay(t, []vectorStep{sv[2], sv[5]})
	chain := &fakeReturnChain{acct: core.Account{Exists: true}, closed: true, finalized: true}
	res, err := Return(context.Background(), chain, func(context.Context) (ResetSession, error) { return paymentSession(t, tr), nil }, returnParams(t))
	if err != nil || !res.Reset {
		t.Fatalf("%+v %v", res, err)
	}
	for _, c := range chain.calls {
		if c == "closeAccount" {
			t.Fatal("closed an already closed account")
		}
	}
}

func TestReturnSendsNoResetWhenTheCloseIsNotDone(t *testing.T) {
	opened := false
	open := func(context.Context) (ResetSession, error) { opened = true; return nil, errors.New("unexpected") }

	failed := &fakeReturnChain{acct: core.Account{Exists: true}, closeErr: errors.New("execution reverted")}
	if _, err := Return(context.Background(), failed, open, returnParams(t)); err == nil || !strings.Contains(err.Error(), "no reset sent") {
		t.Fatalf("closeAccount failure: %v", err)
	}
	notFinal := &fakeReturnChain{acct: core.Account{Exists: true}, closed: true, finalized: false}
	if _, err := Return(context.Background(), notFinal, open, returnParams(t)); err == nil || !strings.Contains(err.Error(), "not final") {
		t.Fatalf("not final: %v", err)
	}
	if _, err := Return(context.Background(), &fakeReturnChain{}, open, returnParams(t)); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("no account: %v", err)
	}
	if opened {
		t.Fatal("a device session was opened before AccountClosed was final")
	}
}

func TestReturnRefusesAnotherDeviceAndKeepsAnUnreachableOnePending(t *testing.T) {
	sv := scenario(t, "SV-17")
	tr := newReplay(t, []vectorStep{sv[2]}) // only session.open: no reset may follow
	p := returnParams(t)
	p.Device = common.HexToAddress("0x" + strings.Repeat("ab", 20))
	chain := &fakeReturnChain{acct: core.Account{Exists: true}}
	_, err := Return(context.Background(), chain, func(context.Context) (ResetSession, error) { return paymentSession(t, tr), nil }, p)
	if err == nil || !strings.Contains(err.Error(), "no reset sent") || tr.next != 1 {
		t.Fatalf("other device: %v, %d sent", err, tr.next)
	}

	res, err := Return(context.Background(), &fakeReturnChain{acct: core.Account{Exists: true}},
		func(context.Context) (ResetSession, error) { return nil, errors.New("no device in setup reach") }, returnParams(t))
	if err != nil || res.Reset || res.ResetPending == "" || res.CloseTx == (common.Hash{}) {
		t.Fatalf("unreachable: %+v %v", res, err)
	}
}
