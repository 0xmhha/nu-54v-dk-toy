package ops

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

// ReturnChain is what the return needs from the chain (core.Chain in opsctl, a fake in tests).
type ReturnChain interface {
	AccountOf(ctx context.Context, device common.Address) (core.Account, error)
	CloseAccount(ctx context.Context, device common.Address) (common.Hash, error)
	// AccountClosedBlock finds the finalized AccountClosed event of the device.
	AccountClosedBlock(ctx context.Context, device common.Address) (block uint64, tx common.Hash, ok bool, err error)
}

// ResetSession is the part of a device session the return uses.
type ResetSession interface {
	DeviceAddress() string
	SendDeviceReset(ctx context.Context, device string, nonce uint64, signature []byte) error
	Close() error
}

// ReturnParams are the operator's inputs to a return (P05-FR-07).
type ReturnParams struct {
	Domain   core.Domain
	Operator *ecdsa.PrivateKey
	Device   common.Address
	// FinalWait bounds the wait for the AccountClosed event to be finalized. Default 60 s.
	FinalWait time.Duration
	Poll      time.Duration
	Progress  func(step string)
}

// ReturnResult says how far the return got. Reset is false when the device was not reachable:
// the account is closed and run the return again when the device is at hand.
type ReturnResult struct {
	Device        common.Address `json:"device"`
	CloseTx       common.Hash    `json:"closeTx"`
	ClosedBlock   uint64         `json:"closedBlock"`
	WithdrawAfter uint64         `json:"withdrawAfter"`
	ResetNonce    uint64         `json:"resetNonce"`
	Reset         bool           `json:"reset"`
	ResetPending  string         `json:"resetPending,omitempty"`
}

// ErrNoAccount means the device never had a deposit; there is nothing to close.
var ErrNoAccount = errors.New("the device has no account (never deposited)")

// Return closes the device account, waits until AccountClosed is final, and only then sends the
// operator-signed DeviceReset (payment-protocol.md 5, P05 design 4). The order never changes:
// a device is wiped only after its account can no longer settle. Running it again after a
// partial return skips the close and repeats the same reset (the nonce is the close block).
func Return(ctx context.Context, chain ReturnChain, open func(context.Context) (ResetSession, error), p ReturnParams) (ReturnResult, error) {
	progress := p.Progress
	if progress == nil {
		progress = func(string) {}
	}
	if p.FinalWait == 0 {
		p.FinalWait = 60 * time.Second
	}
	if p.Poll == 0 {
		p.Poll = 2 * time.Second
	}
	res := ReturnResult{Device: p.Device}
	acct, err := chain.AccountOf(ctx, p.Device)
	if err != nil {
		return res, err
	}
	if !acct.Exists {
		return res, ErrNoAccount
	}
	if acct.ClosedAt == 0 {
		progress("closing the account")
		if res.CloseTx, err = chain.CloseAccount(ctx, p.Device); err != nil {
			return res, fmt.Errorf("closeAccount failed, no reset sent: %w", err)
		}
	}
	progress("waiting for AccountClosed to be final")
	deadline := time.Now().Add(p.FinalWait)
	for {
		block, tx, ok, err := chain.AccountClosedBlock(ctx, p.Device)
		if err != nil {
			return res, err
		}
		if ok {
			res.ClosedBlock, res.ResetNonce = block, block
			if res.CloseTx == (common.Hash{}) {
				res.CloseTx = tx
			}
			break
		}
		if time.Now().After(deadline) {
			return res, errors.New("AccountClosed is not final yet; no reset sent, run the return again")
		}
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(p.Poll):
		}
	}
	if acct, err = chain.AccountOf(ctx, p.Device); err == nil {
		res.WithdrawAfter = acct.WithdrawAfter
	}

	reset := DeviceReset{Device: p.Device, Nonce: new(big.Int).SetUint64(res.ResetNonce)}
	sig, err := SignDeviceReset(p.Domain, p.Operator, reset)
	if err != nil {
		return res, err
	}
	progress("connecting to the device")
	s, err := open(ctx)
	if err != nil {
		res.ResetPending = fmt.Sprintf("device not reachable (%v); the account is closed, run the return again with the device at hand", err)
		return res, nil
	}
	defer s.Close()
	// The nearest device may be another one: never send a reset to a device it was not signed for.
	if got := s.DeviceAddress(); !strings.EqualFold(got, p.Device.Hex()) {
		return res, fmt.Errorf("the connected device is %q, not %s: no reset sent", got, p.Device.Hex())
	}
	progress("sending device.reset")
	if err := s.SendDeviceReset(ctx, strings.ToLower(p.Device.Hex()), res.ResetNonce, sig); err != nil {
		return res, err
	}
	res.Reset = true
	return res, nil
}

// DeviceReset is the operator-signed wipe command {device, nonce}.
type DeviceReset struct {
	Device common.Address
	Nonce  *big.Int
}

// SignDeviceReset signs DeviceReset with the operator key.
func SignDeviceReset(domain core.Domain, operator *ecdsa.PrivateKey, r DeviceReset) ([]byte, error) {
	digest, err := core.Digest(domain, "DeviceReset", core.DeviceResetMessage(r.Device, r.Nonce))
	if err != nil {
		return nil, err
	}
	return core.SignDigest(digest, operator)
}
