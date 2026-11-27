package settlement

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
)

func TestMerchantGasSponsorRequiresAnExplicitLiveBranchGrant(t *testing.T) {
	public, signer, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(200, 0)
	branch := "bc83e0c5-f9ad-4bbd-aaaa-0123456789ab"
	other := "bc83e0c5-f9ad-4bbd-bbbb-0123456789ab"
	contract := common.HexToAddress("0x11")
	asset := common.HexToAddress("0x12")
	payer := common.HexToAddress("0x13")
	relay := common.HexToAddress("0x14")
	merchant := common.HexToHash("0xaa")
	manifest := hex.EncodeToString(make([]byte, 32))
	target := Target{Contract: contract, Asset: asset, Manifest: manifest}
	profile := commerce.Profile{Chain: 8283, Contract: contract, Asset: asset, MerchantID: merchant}
	approved := DeploymentReview{Status: "verified", Chain: 8283, Contract: lower(contract),
		Asset: lower(asset), Manifest: manifest, GasPayer: lower(relay)}
	grant := GasSponsorGrant{Branch: branch, Chain: 8283, Contract: contract, Asset: asset, Merchant: merchant,
		Relay: relay, Manifest: manifest, StartsAt: 190, ExpiresAt: 220}
	message, ok := GasSponsorGrantMessage(grant)
	if !ok {
		t.Fatal("valid sponsorship enrollment message rejected")
	}
	grant.Signature = hex.EncodeToString(ed25519.Sign(signer, message))

	check := func(grant GasSponsorGrant, requestedBranch string, proof DeploymentReview, buyer common.Address, when time.Time) bool {
		_, ok := VerifyMerchantGasSponsor(grant, requestedBranch, profile, target, proof, buyer, public, when)
		return ok
	}
	if !check(grant, branch, approved, payer, clock) {
		t.Fatal("the configured relay with the signed sponsorship grant was rejected")
	}
	changed := grant
	changed.Signature = ""
	for _, candidate := range []struct {
		grant      GasSponsorGrant
		branch     string
		deployment DeploymentReview
		payer      common.Address
		now        time.Time
	}{
		{GasSponsorGrant{}, branch, approved, payer, clock},
		{changed, branch, approved, payer, clock},
		{grant, other, approved, payer, clock},
		{grant, branch, DeploymentReview{Status: "unverified"}, payer, clock},
		{grant, branch, approved, relay, clock},
		{grant, branch, approved, payer, time.Unix(220, 0)},
	} {
		if check(candidate.grant, candidate.branch, candidate.deployment, candidate.payer, candidate.now) {
			t.Fatal("invalid or expired gas sponsorship was accepted")
		}
	}
}
