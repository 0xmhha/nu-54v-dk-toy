// Package ble is the operator-side BLE setup client ([N20][N23]): it opens a
// setup session and sends setup.operator, setup.timeAnchor and device.reset
// using the framing in payment-protocol.md §4. The transport is implemented in
// WBS2-P05-02 with tinygo-org/bluetooth; this file fixes the interface.
package ble

import "context"

// SetupSession is one BLE setup session with a device.
type SetupSession interface {
	// SendOperator records operator, contract, chainId and the per-device pairing passkey
	// (0-999999) after the renter confirms on the device; it returns once setup.ack{setup.operator}
	// arrives (payment-protocol.md 5).
	SendOperator(ctx context.Context, operator, contract string, chainID uint64, passkey uint32) error
	// AwaitKeygen waits for setup.ack{step: keygen} and returns the new device address.
	AwaitKeygen(ctx context.Context) (device string, err error)
	// SendTimeAnchor sends the operator-signed TimeAnchor and returns lastAnchor from the ack.
	SendTimeAnchor(ctx context.Context, device string, timestamp uint64, signature []byte) (lastAnchor uint64, err error)
	// SendDeviceReset sends the operator-signed DeviceReset.
	SendDeviceReset(ctx context.Context, device string, nonce uint64, signature []byte) error
	Close() error
}
