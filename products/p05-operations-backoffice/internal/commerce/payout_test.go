package commerce

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestPayoutProofBindsNewAddressRequestAndVersion(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	profile := FixtureProfile()
	now := time.Unix(1_800_000_000, 0)
	input := payoutProposal{
		Request: "5dc8d4be-c24d-4553-ab79-9c30f630e477",
		Address: strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()),
		Reason:  "custody_rotation",
		Expires: now.Add(10 * time.Minute).Unix(),
	}
	const branch = "b7d3377e-7c01-4a53-b9d6-fa5584557fae"
	message, err := payoutMessage(branch, profile, input, now)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(accounts.TextHash([]byte(message)), key)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	input.Proof = hex.EncodeToString(sig)
	if err := payoutOwnership(branch, profile, input, now); err != nil {
		t.Fatal(err)
	}
	profile.BeneficiaryVersion++
	if err := payoutOwnership(branch, profile, input, now); err == nil {
		t.Fatal("accepted a proof from an old profile")
	}
	profile.BeneficiaryVersion--
	input.Reason = "governance"
	if err := payoutOwnership(branch, profile, input, now); err == nil {
		t.Fatal("accepted changed reason")
	}
	input.Reason = "custody_rotation"
	if err := payoutOwnership(branch, profile, input, time.Unix(input.Expires, 0)); err == nil {
		t.Fatal("accepted stale proof")
	}
}
