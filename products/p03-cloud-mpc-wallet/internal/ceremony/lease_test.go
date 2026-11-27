package ceremony

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"testing"
	"time"
)

func TestCancelledParticipantLeaseCannotResumeConsumedCeremony(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	var key [32]byte
	rand.Read(key[:])
	coordPub, coord, _ := ed25519.GenerateKey(rand.Reader)
	approvalPub, approve, _ := ed25519.GenerateKey(rand.Reader)
	recovery, _, _ := ed25519.GenerateKey(rand.Reader)
	newParticipant := func(ctx context.Context) *Node {
		t.Helper()
		v, err := openVault(path, key[:])
		if err != nil {
			t.Fatal(err)
		}
		n, err := newNode(ctx, "a1", v, coordPub, approvalPub, recovery, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := newParticipant(ctx)
	s := Session{ID: "35f15c1c-6391-4fb8-8463-debcc37bbd91", Wallet: "b8eaa48d-0e6d-4721-af1f-00c2b606503a",
		Kind: DKG, Epoch: 1, Expires: time.Now().Add(time.Minute).Unix(),
		Peers: []Peer{{"a1", A, 1}, {"b1", B, 1}, {"c1", C, 1}}}
	digest, err := s.Fingerprint(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	msg := ApprovalMessage(s.Kind, digest)
	auth := Authorization{Session: s, Coordinator: ed25519.Sign(coord, msg), Independent: ed25519.Sign(approve, msg)}
	if got, err := p.Start(auth); err != nil || got.State != "running" {
		t.Fatalf("failed to reserve: %v", err)
	}
	cancel()
	if _, err := p.Start(auth); err == nil {
		t.Fatal("expired participant accepted a request")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := newParticipant(context.Background())
	defer reopened.Close()
	if got, err := reopened.Start(auth); err != nil || got.State != "interrupted" || len(got.Signature) != 0 {
		t.Fatalf("cancelled ceremony resumed after restart: %s %v", got.State, err)
	}
}
