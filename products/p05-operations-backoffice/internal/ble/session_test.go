package ble

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
)

type commissioningTransport struct {
	writes [][]byte
	rx     chan []byte
}

func (t *commissioningTransport) MTU() int                     { return 517 }
func (t *commissioningTransport) Close() error                 { return nil }
func (t *commissioningTransport) Notifications() <-chan []byte { return t.rx }
func (t *commissioningTransport) Write(fragment []byte) error {
	t.writes = append(t.writes, append([]byte(nil), fragment...))
	return nil
}

func TestProtectedOperatorSetupDoesNotFallBack(t *testing.T) {
	for _, protected := range []bool{false, true} {
		const sid = "0123456789abcdef"
		ack := protocol.Message{"v": 1, "type": "setup.ack", "sessionId": sid, "step": "setup.operator", "accepted": !protected}
		if protected {
			ack["reason"] = "NOT_PERMITTED"
		}
		body, err := protocol.EncodeMessage(ack)
		if err != nil {
			t.Fatal(err)
		}
		wire := &commissioningTransport{rx: make(chan []byte, 2)}
		session := &Session{t: wire, w: protocol.FrameWriter{MTU: wire.MTU()}, sid: sid, buttonWait: time.Second}
		fragments, err := session.w.Write(body)
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range fragments {
			wire.rx <- fragment
		}
		send := session.SendOperator
		if protected {
			send = session.SendProtectedRentalOperator
		}
		err = send(context.Background(), "0x"+strings.Repeat("11", 20), "0x"+strings.Repeat("22", 20), 8283, 123456)
		var refusal *RefusedError
		if protected && (!errors.As(err, &refusal) || refusal.Reason != "NOT_PERMITTED") || !protected && err != nil {
			t.Fatalf("protected=%v: %v", protected, err)
		}
		if len(wire.writes) != 1 {
			t.Fatalf("protected=%v sent %d setup attempts", protected, len(wire.writes))
		}
		reader := protocol.Reassembler{}
		decoded, err := reader.Feed(wire.writes[0])
		if err != nil {
			t.Fatal(err)
		}
		message, err := protocol.DecodeMessage(decoded)
		if err != nil {
			t.Fatal(err)
		}
		value, set := message["rentalJournal"]
		if set != protected || protected && value != true {
			t.Fatalf("protected=%v: marker set=%v value=%v", protected, set, value)
		}
	}
}

func TestFreezeProtectedRentalValidatesStandaloneResponse(t *testing.T) {
	for _, matches := range []bool{true, false} {
		var challenge [32]byte
		challenge[0] = 0x11
		replyChallenge := "0x11" + strings.Repeat("00", 31)
		if !matches {
			replyChallenge = "0x22" + strings.Repeat("00", 31)
		}
		digest := "0x" + strings.Repeat("11", 32)
		msg := protocol.Message{
			"v": 1, "type": "rental.freeze.result", "sessionId": "0000000000000000",
			"binding": digest, "challenge": replyChallenge, "projectionRoot": digest,
			"firstNonce": "256", "nextNonce": "257", "entryCount": "1", "frozenAt": "1000", "expiresAt": "1090",
			"boardSignature": "0x" + strings.Repeat("00", 65),
		}
		encoded, err := protocol.EncodeMessage(msg)
		if err != nil {
			t.Fatal(err)
		}
		wire := &commissioningTransport{rx: make(chan []byte, 2)}
		writer := protocol.FrameWriter{MTU: wire.MTU()}
		regressionFragments, err := writer.Write(encoded)
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range regressionFragments {
			wire.rx <- fragment
		}
		reply, err := FreezeProtectedRental(context.Background(), wire, challenge, [32]byte{42}, 1090, make([]byte, 65))
		if matches && (err != nil || reply["projectionRoot"] != digest) || !matches && err == nil {
			t.Fatalf("matches=%v: response=%v err=%v", matches, reply, err)
		}
		if len(wire.writes) != 1 {
			t.Fatalf("sent %d requests", len(wire.writes))
		}
		reader := protocol.Reassembler{}
		sent, err := reader.Feed(wire.writes[0])
		if err != nil {
			t.Fatal(err)
		}
		request, err := protocol.DecodeMessage(sent)
		if err != nil || request["type"] != "rental.freeze.v2" || request["channelNonce"] != "0x2a"+strings.Repeat("00", 31) || request["sessionId"] != "0000000000000000" {
			t.Fatalf("unexpected standalone freeze request: %v %v", request, err)
		}
	}
}
