package quotev2

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestPrivateCommitments(t *testing.T) {
	key := [32]byte{31: 2}
	quote := [32]byte{31: 3}
	tenant := []byte{15: 5}
	got, err := CommitPrivateContext(key, quote, TenantCommitment, tenant)
	if err != nil || hex.EncodeToString(got[:]) != "cc68d3735ae531e88f7c2f9f249aa8048533a95c393b8e2afec2fd698fe38026" {
		t.Fatalf("tenant commitment = %x, %v", got, err)
	}
	branch, err := CommitPrivateContext(key, quote, BranchCommitment, tenant)
	if err != nil || bytes.Equal(got[:], branch[:]) {
		t.Fatal("commitment did not separate purposes")
	}
	quote[31]++
	next, err := CommitPrivateContext(key, quote, TenantCommitment, tenant)
	if err != nil || bytes.Equal(got[:], next[:]) {
		t.Fatal("commitment did not separate quotes")
	}
	if _, err := CommitPrivateContext(key, quote, ClaimCommitment, tenant); err == nil {
		t.Fatal("accepted a malformed private claim")
	}
}
