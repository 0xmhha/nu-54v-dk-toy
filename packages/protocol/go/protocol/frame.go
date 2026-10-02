package protocol

// BLE framing (payment-protocol.md 4), the Go twin of ts/src/frame.ts:
//
//	envelope = length(u16 BE, whole envelope) | digest(8 = SHA-256(body)[0..8]) | CBOR body
//	fragment = sequence(u8) | index(u8) | up to ATT_MTU - 5 envelope bytes
//
// The receiver follows the rules the shared fragment vectors encode: index starts at 0 and
// grows by one, the sequence stays the same within a message (it is not compared between
// messages), every fragment carries data, the length is 10..2058, and the digest is checked
// once the envelope is complete.

import (
	"bytes"
	"crypto/sha256"
	"errors"
)

const (
	EnvelopeHeaderLen = 10
	MaxBodyLen        = 2048
)

// Envelope wraps a CBOR body with its length and digest.
func Envelope(body []byte) ([]byte, error) {
	if len(body) > MaxBodyLen {
		return nil, badFrame("body above 2048 bytes")
	}
	total := EnvelopeHeaderLen + len(body)
	d := sha256.Sum256(body)
	out := []byte{byte(total >> 8), byte(total)}
	out = append(out, d[:8]...)
	return append(out, body...), nil
}

// Fragments splits an envelope for a negotiated ATT_MTU.
func Fragments(env []byte, sequence byte, attMTU int) ([][]byte, error) {
	if attMTU <= 5 {
		return nil, errors.New("ATT_MTU too small")
	}
	size := attMTU - 5
	var out [][]byte
	for i, idx := 0, 0; i < len(env); i, idx = i+size, idx+1 {
		if idx > 255 {
			return nil, errors.New("more than 256 fragments")
		}
		end := min(i+size, len(env))
		out = append(out, append([]byte{sequence, byte(idx)}, env[i:end]...))
	}
	return out, nil
}

// FrameWriter numbers messages per direction and splits them.
type FrameWriter struct {
	MTU      int
	sequence byte
}

// Write frames one CBOR body.
func (w *FrameWriter) Write(body []byte) ([][]byte, error) {
	env, err := Envelope(body)
	if err != nil {
		return nil, err
	}
	out, err := Fragments(env, w.sequence, w.MTU)
	w.sequence++
	return out, err
}

// Reassembler is the receiver side; any rule violation is BAD_FRAME and resets it.
type Reassembler struct {
	buf       []byte
	want      int
	sequence  byte
	nextIndex int
	active    bool
}

func (r *Reassembler) reset() { *r = Reassembler{} }

func (r *Reassembler) bad(detail string) ([]byte, error) {
	r.reset()
	return nil, badFrame("%s", detail)
}

// Feed takes one fragment; it returns the body once the message is complete, else nil.
func (r *Reassembler) Feed(fragment []byte) ([]byte, error) {
	if len(fragment) <= 2 {
		return r.bad("fragment without data")
	}
	seq, idx := fragment[0], int(fragment[1])
	if !r.active {
		if idx != 0 {
			return r.bad("message does not start at index 0")
		}
		r.active, r.sequence = true, seq
	} else if seq != r.sequence || idx != r.nextIndex {
		return r.bad("fragment out of order")
	}
	data := fragment[2:]
	if len(r.buf)+len(data) > EnvelopeHeaderLen+MaxBodyLen {
		return r.bad("envelope too long")
	}
	r.buf = append(r.buf, data...)
	r.nextIndex++
	if r.want == 0 && len(r.buf) >= 2 {
		r.want = int(r.buf[0])<<8 | int(r.buf[1])
		if r.want < EnvelopeHeaderLen || r.want > EnvelopeHeaderLen+MaxBodyLen {
			return r.bad("length out of range")
		}
	}
	if r.want != 0 && len(r.buf) > r.want {
		return r.bad("bytes beyond the length")
	}
	if r.want != 0 && len(r.buf) == r.want {
		body := append([]byte(nil), r.buf[EnvelopeHeaderLen:]...)
		d := sha256.Sum256(body)
		if !bytes.Equal(d[:8], r.buf[2:EnvelopeHeaderLen]) {
			return r.bad("digest mismatch")
		}
		r.reset()
		return body, nil
	}
	return nil, nil
}
