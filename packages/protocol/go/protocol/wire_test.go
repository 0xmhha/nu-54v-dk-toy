package protocol

// The Go CBOR and framing against the shared vectors: CBOR message vectors, fragment vectors
// (valid splits and the frames a receiver must refuse) and every body in the session vectors.

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func readVectors(t *testing.T, name string, v any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "content", "specifications", "protocol", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatal(err)
	}
}

// fromJSON turns encoding/json's generic values into the Message form (v as int, maps as Message).
func fromJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := Message{}
		for k, e := range x {
			m[k] = fromJSON(e)
		}
		return m
	case float64:
		return int(x)
	}
	return v
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCBORVectors(t *testing.T) {
	var doc struct {
		Vectors []struct {
			ID          string         `json:"id"`
			Message     map[string]any `json:"message"`
			CborHex     string         `json:"cborHex"`
			EnvelopeHex string         `json:"envelopeHex"`
		} `json:"vectors"`
	}
	readVectors(t, "cbor-vectors.json", &doc)
	if len(doc.Vectors) == 0 {
		t.Fatal("no vectors")
	}
	for _, v := range doc.Vectors {
		msg := fromJSON(v.Message).(Message)
		body, err := EncodeMessage(msg)
		if err != nil {
			t.Fatalf("%s: %v", v.ID, err)
		}
		if hex.EncodeToString(body) != v.CborHex {
			t.Errorf("%s: encoded %x, want %s", v.ID, body, v.CborHex)
		}
		env, _ := Envelope(body)
		if hex.EncodeToString(env) != v.EnvelopeHex {
			t.Errorf("%s: envelope differs", v.ID)
		}
		// Decoding gives hex in lower case (the vectors use checksummed addresses), so the
		// round trip is compared on the bytes.
		back, err := DecodeMessage(body)
		if err != nil || back["type"] != msg["type"] {
			t.Fatalf("%s: decoded %v (%v)", v.ID, back, err)
		}
		if again, _ := EncodeMessage(back); hex.EncodeToString(again) != v.CborHex {
			t.Errorf("%s: decode then encode differs", v.ID)
		}
	}
}

func TestFrameVectors(t *testing.T) {
	var doc struct {
		Valid []struct {
			ID           string   `json:"id"`
			Message      string   `json:"message"`
			AttMTU       int      `json:"attMtu"`
			Sequence     int      `json:"sequence"`
			FragmentsHex []string `json:"fragmentsHex"`
		} `json:"valid"`
		Invalid []struct {
			ID           string   `json:"id"`
			FragmentsHex []string `json:"fragmentsHex"`
		} `json:"invalid"`
	}
	readVectors(t, "frame-vectors.json", &doc)
	var cbor struct {
		Vectors []struct {
			ID      string `json:"id"`
			CborHex string `json:"cborHex"`
		} `json:"vectors"`
	}
	readVectors(t, "cbor-vectors.json", &cbor)
	bodies := map[string]string{}
	for _, v := range cbor.Vectors {
		bodies[v.ID] = v.CborHex
	}
	rx := &Reassembler{} // one receiver for every message in a row, as on a link
	for _, v := range doc.Valid {
		env, _ := Envelope(mustHex(t, bodies[v.Message]))
		frags, err := Fragments(env, byte(v.Sequence), v.AttMTU)
		if err != nil || len(frags) != len(v.FragmentsHex) {
			t.Fatalf("%s: %d fragments (%v), want %d", v.ID, len(frags), err, len(v.FragmentsHex))
		}
		var body []byte
		for i, f := range frags {
			if hex.EncodeToString(f) != v.FragmentsHex[i] {
				t.Errorf("%s: fragment %d differs", v.ID, i)
			}
			if body, err = rx.Feed(f); err != nil {
				t.Fatalf("%s: %v", v.ID, err)
			}
		}
		if hex.EncodeToString(body) != bodies[v.Message] {
			t.Errorf("%s: reassembled body differs", v.ID)
		}
	}
	for _, v := range doc.Invalid {
		rx := &Reassembler{}
		var err error
		for _, f := range v.FragmentsHex {
			if _, err = rx.Feed(mustHex(t, f)); err != nil {
				break
			}
		}
		var pe *ProtocolError
		if !errors.As(err, &pe) || pe.Reason != ReasonBadFrame {
			t.Errorf("%s: got %v, want BAD_FRAME", v.ID, err)
		}
	}
}

func TestSessionVectorBodiesRoundTrip(t *testing.T) {
	var doc struct {
		Scenarios []struct {
			ID    string `json:"id"`
			Steps []struct {
				SendType string   `json:"sendType"`
				Send     string   `json:"send"`
				Expect   []string `json:"expect"`
				Phone    []string `json:"phone"`
			} `json:"steps"`
		} `json:"scenarios"`
	}
	readVectors(t, "session-vectors.json", &doc)
	n := 0
	for _, sc := range doc.Scenarios {
		for _, st := range sc.Steps {
			bodies := append(append([]string{}, st.Expect...), st.Phone...)
			if st.SendType == "(outside the schema)" {
				// A refusal test's body: decoding it must refuse it as UNSUPPORTED_TYPE.
				_, err := DecodeMessage(mustHex(t, st.Send))
				var pe *ProtocolError
				if !errors.As(err, &pe) || pe.Reason != ReasonUnsupportedType {
					t.Errorf("%s: outside-schema body decoded as %v", sc.ID, err)
				}
			} else if st.Send != "" {
				bodies = append(bodies, st.Send)
			}
			for _, h := range bodies {
				m, err := DecodeMessage(mustHex(t, h))
				if err != nil {
					t.Fatalf("%s: %v", sc.ID, err)
				}
				again, err := EncodeMessage(m)
				if err != nil || hex.EncodeToString(again) != h {
					t.Errorf("%s: %s does not re-encode to the same bytes (%v)", sc.ID, m["type"], err)
				}
				n++
			}
		}
	}
	if n < 100 {
		t.Fatalf("only %d bodies checked", n)
	}
}

func TestDecodeRefusesNonDeterministicCBOR(t *testing.T) {
	ok, _ := EncodeMessage(Message{"v": 1, "type": "session.cancel", "sessionId": "0102030405060708"})
	cases := map[string][]byte{
		"trailing byte":    append(append([]byte{}, ok...), 0),
		"non-shortest int": {0xa1, 0x61, 0x76, 0x18, 0x01},
		"not a map":        {0x01},
		"unknown type":     mustHex(t, "a2647479706563666f6f617601"),
	}
	for name, b := range cases {
		if _, err := DecodeMessage(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
