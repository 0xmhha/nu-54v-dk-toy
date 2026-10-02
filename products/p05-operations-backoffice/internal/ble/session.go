package ble

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
)

// Transport moves raw fragments to and from the device (BLE GATT RX write and TX notify).
type Transport interface {
	// MTU is the negotiated ATT_MTU; fragments carry at most MTU - 5 data bytes.
	MTU() int
	Write(fragment []byte) error
	// Notifications delivers TX fragments in order.
	Notifications() <-chan []byte
	Close() error
}

// Options fixes the session id and kiosk nonce (tests); zero values are random.
type Options struct {
	SessionID  [8]byte
	KioskNonce [32]byte
	// ButtonWait bounds the steps that wait for the renter (setup.operator, PIN). Default 3 min.
	ButtonWait time.Duration
}

// Session is one setup session (payment-protocol.md 5). It implements SetupSession.
type Session struct {
	t          Transport
	w          protocol.FrameWriter
	rx         protocol.Reassembler
	sid        string
	buttonWait time.Duration
	// From session.open.ok.
	State      string
	Device     string // empty while UNPROVISIONED
	LastAnchor uint64
}

// RefusedError is a setup step the device answered with accepted: false, or an error reply.
type RefusedError struct {
	Step   string
	Reason string
}

func (e *RefusedError) Error() string { return fmt.Sprintf("device refused %s: %s", e.Step, e.Reason) }

const replyWait = 5 * time.Second

// Open opens a setup session. A device that is READY refuses it (NOT_PERMITTED).
func Open(ctx context.Context, t Transport, o Options) (*Session, error) {
	var zeroSID [8]byte
	var zeroNonce [32]byte
	if o.SessionID == zeroSID {
		_, _ = rand.Read(o.SessionID[:])
	}
	if o.KioskNonce == zeroNonce {
		_, _ = rand.Read(o.KioskNonce[:])
	}
	if o.ButtonWait == 0 {
		o.ButtonWait = 3 * time.Minute
	}
	s := &Session{t: t, w: protocol.FrameWriter{MTU: t.MTU()}, sid: hex.EncodeToString(o.SessionID[:]), buttonWait: o.ButtonWait}
	ok, err := s.request(ctx, protocol.Message{"type": "session.open", "mode": "setup", "kioskNonce": "0x" + hex.EncodeToString(o.KioskNonce[:])}, replyWait)
	if err != nil {
		return nil, err
	}
	if ok["type"] != "session.open.ok" {
		return nil, &RefusedError{"session.open", fmt.Sprint(ok["reason"])}
	}
	s.State, _ = ok["state"].(string)
	s.Device, _ = ok["device"].(string)
	s.LastAnchor, _ = strconv.ParseUint(fmt.Sprint(ok["lastAnchor"]), 10, 64)
	return s, nil
}

func (s *Session) send(m protocol.Message) error {
	m["v"] = 1
	m["sessionId"] = s.sid
	body, err := protocol.EncodeMessage(m)
	if err != nil {
		return err
	}
	frags, err := s.w.Write(body)
	if err != nil {
		return err
	}
	for _, f := range frags {
		if err := s.t.Write(f); err != nil {
			return err
		}
	}
	return nil
}

// next returns the next message of this session (or with the zero session id: a frame error).
func (s *Session) next(ctx context.Context, wait time.Duration) (protocol.Message, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, errors.New("no reply from the device in time")
		case f, open := <-s.t.Notifications():
			if !open {
				return nil, errors.New("the link closed")
			}
			body, err := s.rx.Feed(f)
			if err != nil {
				return nil, err
			}
			if body == nil {
				continue
			}
			m, err := protocol.DecodeMessage(body)
			if err != nil {
				return nil, err
			}
			if m["sessionId"] == s.sid || m["sessionId"] == "0000000000000000" {
				return m, nil
			}
		}
	}
}

func (s *Session) request(ctx context.Context, m protocol.Message, wait time.Duration) (protocol.Message, error) {
	if err := s.send(m); err != nil {
		return nil, err
	}
	return s.next(ctx, wait)
}

// ack waits for setup.ack of `step`; an error reply or accepted: false is a *RefusedError.
func (s *Session) ack(ctx context.Context, step string, wait time.Duration) (protocol.Message, error) {
	m, err := s.next(ctx, wait)
	if err != nil {
		return nil, err
	}
	if m["type"] == "error" {
		return nil, &RefusedError{step, fmt.Sprint(m["reason"])}
	}
	if m["type"] != "setup.ack" || m["step"] != step {
		return nil, fmt.Errorf("expected setup.ack for %s, got %v %v", step, m["type"], m["step"])
	}
	if accepted, _ := m["accepted"].(bool); !accepted {
		return nil, &RefusedError{step, fmt.Sprint(m["reason"])}
	}
	return m, nil
}

// SendOperator sends setup.operator and waits for the renter to confirm on the device.
func (s *Session) SendOperator(ctx context.Context, operator, contract string, chainID uint64, passkey uint32) error {
	if passkey > 999999 {
		return errors.New("passkey has at most six digits")
	}
	err := s.send(protocol.Message{"type": "setup.operator", "operator": operator, "contract": contract,
		"chainId": strconv.FormatUint(chainID, 10), "passkey": strconv.FormatUint(uint64(passkey), 10)})
	if err != nil {
		return err
	}
	_, err = s.ack(ctx, "setup.operator", s.buttonWait)
	return err
}

// AwaitKeygen waits until the renter has set the PIN and returns the new device address.
func (s *Session) AwaitKeygen(ctx context.Context) (string, error) {
	m, err := s.ack(ctx, "keygen", s.buttonWait)
	if err != nil {
		return "", err
	}
	s.Device, _ = m["device"].(string)
	return s.Device, nil
}

// SendTimeAnchor sends the operator-signed TimeAnchor and returns lastAnchor from the ack.
func (s *Session) SendTimeAnchor(ctx context.Context, device string, timestamp uint64, signature []byte) (uint64, error) {
	err := s.send(protocol.Message{"type": "setup.timeAnchor", "device": device, "timestamp": strconv.FormatUint(timestamp, 10),
		"operatorSignature": "0x" + hex.EncodeToString(signature)})
	if err != nil {
		return 0, err
	}
	m, err := s.ack(ctx, "setup.timeAnchor", replyWait)
	if err != nil {
		return 0, err
	}
	last, _ := strconv.ParseUint(fmt.Sprint(m["lastAnchor"]), 10, 64)
	s.LastAnchor = last
	return last, nil
}

// SendDeviceReset sends the operator-signed DeviceReset.
func (s *Session) SendDeviceReset(ctx context.Context, device string, nonce uint64, signature []byte) error {
	err := s.send(protocol.Message{"type": "device.reset", "device": device, "nonce": strconv.FormatUint(nonce, 10),
		"operatorSignature": "0x" + hex.EncodeToString(signature)})
	if err != nil {
		return err
	}
	_, err = s.ack(ctx, "device.reset", replyWait)
	return err
}

// Close ends the session and the link.
func (s *Session) Close() error {
	_ = s.send(protocol.Message{"type": "session.cancel"})
	return s.t.Close()
}

var _ SetupSession = (*Session)(nil)
