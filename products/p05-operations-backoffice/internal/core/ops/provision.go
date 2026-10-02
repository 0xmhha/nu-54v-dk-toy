package ops

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

// ProvisionParams are the operator's inputs to a rental setup (P05-FR-06, payment-protocol.md 5).
type ProvisionParams struct {
	Domain   core.Domain
	Operator *ecdsa.PrivateKey
	// Passkey is the device's pairing code (0-999999), printed on the label QR.
	Passkey  uint32
	Amount   *big.Int
	Withdraw common.Address
	// Progress tells the operator what the renter has to do next.
	Progress func(step string)
}

// ProvisionResult is what the label and the rental record need.
type ProvisionResult struct {
	Device     common.Address `json:"device"`
	Passkey    string         `json:"passkey"`
	Anchor     uint64         `json:"anchor"`
	DepositTx  common.Hash    `json:"depositTx"`
	LastAnchor uint64         `json:"lastAnchor"`
}

// Provision runs the rental setup on an UNPROVISIONED device: setup.operator (the renter
// confirms on the device), key and PIN (keygen ack), TimeAnchor from the finalized block time,
// and only after the device accepted the anchor, depositFor.
func Provision(ctx context.Context, s ble.SetupSession, p ProvisionParams,
	finalized func(context.Context) (uint64, error),
	deposit func(ctx context.Context, device common.Address, amount *big.Int, withdraw common.Address) (common.Hash, error),
) (ProvisionResult, error) {
	progress := p.Progress
	if progress == nil {
		progress = func(string) {}
	}
	if p.Passkey > 999999 {
		return ProvisionResult{}, errors.New("passkey has at most six digits")
	}
	operator := core.Address(p.Operator)
	progress("confirm the operator values with the device button")
	if err := s.SendOperator(ctx, strings.ToLower(operator.Hex()), strings.ToLower(p.Domain.VerifyingContract.Hex()), p.Domain.ChainID, p.Passkey); err != nil {
		return ProvisionResult{}, err
	}
	progress("set the PIN with the device buttons")
	dev, err := s.AwaitKeygen(ctx)
	if err != nil {
		return ProvisionResult{}, err
	}
	if !common.IsHexAddress(dev) {
		return ProvisionResult{}, fmt.Errorf("keygen ack without a device address (%q)", dev)
	}
	device := common.HexToAddress(dev)
	ts, err := finalized(ctx) // finalized block time, not the host clock
	if err != nil {
		return ProvisionResult{}, err
	}
	a, err := SignTimeAnchor(p.Domain, p.Operator, device, ts, 0)
	if err != nil {
		return ProvisionResult{}, err
	}
	last, err := s.SendTimeAnchor(ctx, strings.ToLower(device.Hex()), ts, a.OperatorSignature)
	if err != nil {
		return ProvisionResult{}, err
	}
	res := ProvisionResult{Device: device, Passkey: fmt.Sprintf("%06d", p.Passkey), Anchor: ts, LastAnchor: last}
	if p.Amount == nil || p.Amount.Sign() == 0 {
		return res, nil // no deposit asked for
	}
	progress("depositing for the device")
	res.DepositTx, err = deposit(ctx, device, p.Amount, p.Withdraw)
	return res, err
}
