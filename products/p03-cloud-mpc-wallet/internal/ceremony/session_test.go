package ceremony

import (
	"bytes"

	"testing"
	"time"
)

func TestSessionCanonicalAndBound(t *testing.T) {
	start := time.Unix(100, 0)
	session := Session{
		ID: "53f230e2-8d75-4bb2-9930-10e28f2cb628", Wallet: "eb44182b-71b0-485c-9d0a-580b4a161f40",
		Kind: DKG, Epoch: 1, Expires: start.Add(time.Minute).Unix(),
		Peers: []Peer{{"a1", A, 1}, {"b1", B, 1}, {"c1", C, 1}},
	}
	id, err := session.Fingerprint(start)
	if err != nil {
		t.Fatal(err)
	}
	session.Peers[0], session.Peers[2] = session.Peers[2], session.Peers[0]
	permuted, err := session.Fingerprint(start)
	if err != nil || !bytes.Equal(id[:], permuted[:]) {
		t.Fatalf("peer permutation changed fingerprint: %v", err)
	}
	session.Peers[2].Epoch = 2
	if _, err := session.Fingerprint(start); err == nil {
		t.Fatal("accepted mismatched epoch")
	}
}

func TestBroadcastRequiresAgreement(t *testing.T) {
	s := NewInbox("b1", []string{"a1", "b1", "c1"})
	m := Frame{From: "a1", To: "b1", Round: "kg1", Broadcast: true, Data: []byte("payload")}
	if _, done, err := s.Receive("a1", m); err != nil || done {
		t.Fatalf("completed without echoes: %v", err)
	}
	e := m
	e.From, e.Source, e.Data, e.Digest = "c1", "a1", nil, m.Hash()
	if _, done, err := s.Receive("c1", e); err != nil || !done {
		t.Fatalf("did not complete with matching echo: %v", err)
	}
	if _, done, err := s.Receive("a1", m); err != nil || done {
		t.Fatalf("duplicate delivery: %v", err)
	}
	e.Digest[0] ^= 0xff
	if _, _, err := s.Receive("c1", e); err == nil {
		t.Fatal("accepted conflicting echo")
	}
}
