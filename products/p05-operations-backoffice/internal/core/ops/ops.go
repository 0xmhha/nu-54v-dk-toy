// Package ops holds the operations procedures opsctl runs now and opsd will expose next
// cycle (P05 design 1). Chain access goes through small interfaces so the rules are tested
// without a chain.
package ops

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

// Registry is what the procedures read from MerchantRegistry.
type Registry interface {
	MerchantStatus(merchant common.Address) (active bool, payout common.Address, err error)
	PendingPayoutOf(merchant common.Address) (payout common.Address, effectiveAt uint64, err error)
}

// Attestation is the signedAttestation of the protocol schema (payment.identify).
type Attestation struct {
	Merchant          common.Address `json:"merchant"`
	Payout            common.Address `json:"payout"`
	Name              string         `json:"name"`
	ValidFrom         string         `json:"validFrom"`
	ValidUntil        string         `json:"validUntil"`
	OperatorSignature hexutil.Bytes  `json:"operatorSignature"`
}

var (
	ErrNotActive      = errors.New("merchant is not active in the registry")
	ErrPayoutMismatch = errors.New("payout differs from the registry payout")
	ErrPayoutPending  = errors.New("payout change has not taken effect yet")
)

// IssueAttestation signs a MerchantAttestation with the operator key (P05-FR-02, 03, 09).
// The payout must be the registry's current payout. While a payout change is queued the
// attestation ends at the change's effective time, so no attestation for the old payout
// outlives it; an attestation for the queued payout is refused until it takes effect.
func IssueAttestation(reg Registry, domain core.Domain, operator *ecdsa.PrivateKey,
	merchant, payout common.Address, name string, now, validity uint64) (Attestation, error) {
	active, current, err := reg.MerchantStatus(merchant)
	if err != nil {
		return Attestation{}, err
	}
	if !active {
		return Attestation{}, ErrNotActive
	}
	pending, effectiveAt, err := reg.PendingPayoutOf(merchant)
	if err != nil {
		return Attestation{}, err
	}
	if pending != (common.Address{}) && payout == pending {
		return Attestation{}, ErrPayoutPending
	}
	if payout != current {
		return Attestation{}, ErrPayoutMismatch
	}
	until := now + validity
	if pending != (common.Address{}) && effectiveAt < until {
		until = effectiveAt
	}
	a := Attestation{Merchant: merchant, Payout: payout, Name: name,
		ValidFrom: fmt.Sprint(now), ValidUntil: fmt.Sprint(until)}
	digest, err := core.Digest(domain, "MerchantAttestation", map[string]any{
		"merchant": merchant.Hex(), "payout": payout.Hex(), "name": name,
		"validFrom": a.ValidFrom, "validUntil": a.ValidUntil,
	})
	if err != nil {
		return Attestation{}, err
	}
	if a.OperatorSignature, err = core.SignDigest(digest, operator); err != nil {
		return Attestation{}, err
	}
	return a, nil
}

// TimeAnchor is the setup.timeAnchor payload (P05-FR-06).
type TimeAnchor struct {
	Device            common.Address `json:"device"`
	Timestamp         string         `json:"timestamp"`
	OperatorSignature hexutil.Bytes  `json:"operatorSignature"`
}

var ErrAnchorNotLater = errors.New("timestamp must be strictly later than the device's last anchor")

// SignTimeAnchor signs {device, timestamp} with the operator key. The timestamp is the
// finalized block time the caller read, and must be later than the device's lastAnchor.
func SignTimeAnchor(domain core.Domain, operator *ecdsa.PrivateKey, device common.Address, timestamp, lastAnchor uint64) (TimeAnchor, error) {
	if timestamp <= lastAnchor {
		return TimeAnchor{}, ErrAnchorNotLater
	}
	digest, err := core.Digest(domain, "TimeAnchor", core.TimeAnchorMessage(device, timestamp))
	if err != nil {
		return TimeAnchor{}, err
	}
	sig, err := core.SignDigest(digest, operator)
	if err != nil {
		return TimeAnchor{}, err
	}
	return TimeAnchor{Device: device, Timestamp: fmt.Sprint(timestamp), OperatorSignature: sig}, nil
}

// MerchantOrder is the order the kiosk signs with the (test) merchant key (P05-FR-04).
type MerchantOrder struct {
	OrderID common.Hash    `json:"orderId"`
	Token   common.Address `json:"token"`
	Amount  *big.Int       `json:"amount"`
	Payout  common.Address `json:"payout"`
	Expiry  uint64         `json:"expiry"`
}

func SignMerchantOrder(domain core.Domain, merchant *ecdsa.PrivateKey, o MerchantOrder) ([]byte, error) {
	digest, err := core.Digest(domain, "MerchantOrder", map[string]any{
		"orderId": o.OrderID.Hex(), "token": o.Token.Hex(), "amount": o.Amount.String(),
		"payout": o.Payout.Hex(), "expiry": fmt.Sprint(o.Expiry),
	})
	if err != nil {
		return nil, err
	}
	return core.SignDigest(digest, merchant)
}
