package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/settlement"
	"github.com/ethereum/go-ethereum/common"
)

func TestMerchantGasSponsorPublicationReloadsWithoutRestart(t *testing.T) {
	branch, other := "bc83e0c5-f9ad-4bbd-aaaa-0123456789ab", "bc83e0c5-f9ad-4bbd-bbbb-0123456789ab"
	contract, asset := common.HexToAddress("0x11"), common.HexToAddress("0x12")
	payer, relay := common.HexToAddress("0x13"), common.HexToAddress("0x14")
	manifest, merchant := strings.Repeat("1", 64), common.HexToHash("0xaa")
	target := settlement.Target{Contract: contract, Asset: asset, Manifest: manifest}
	profile := commerce.Profile{Chain: 8283, Contract: contract, Asset: asset, MerchantID: merchant}
	review := settlement.DeploymentReview{Status: "verified", Chain: 8283, Contract: strings.ToLower(contract.Hex()),
		Asset: strings.ToLower(asset.Hex()), Manifest: manifest, GasPayer: strings.ToLower(relay.Hex())}
	public, signer, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "merchant-gas.json")
	t.Setenv(merchantGasPublication, path)
	t.Setenv("P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY", hex.EncodeToString(public))
	t.Setenv("P05_PRESENTATION_KEY_FILE", "configured")
	publish := func(entries []settlement.GasSponsorGrant) {
		t.Helper()
		content, err := json.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+".new", content, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".new", path); err != nil {
			t.Fatal(err)
		}
	}
	grant := settlement.GasSponsorGrant{Branch: branch, Chain: 8283, Contract: contract, Asset: asset,
		Manifest: manifest, Merchant: merchant, Relay: relay, StartsAt: 190, ExpiresAt: 220}
	body, valid := settlement.GasSponsorGrantMessage(grant)
	if !valid {
		t.Fatal("could not encode the enrolled branch")
	}
	grant.Signature = hex.EncodeToString(ed25519.Sign(signer, body))
	publish([]settlement.GasSponsorGrant{grant})
	current := true
	var duringRuntimeCheck func()
	lookup, err := merchantGasSponsors(map[string]settlement.Target{branch: target, other: target},
		map[string]settlement.DeploymentReview{branch: review, other: review},
		func(context.Context, settlement.Target, settlement.DeploymentReview) bool {
			if duringRuntimeCheck != nil {
				duringRuntimeCheck()
			}
			return current
		})
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(200, 0)
	hasSponsor := func(branch string) bool {
		_, usable := lookup(context.Background(), branch, profile, payer, clock)
		return usable
	}
	if !hasSponsor(branch) || hasSponsor(other) {
		t.Fatal("only the funded merchant branch can retain the gas commitment")
	}
	current = false
	if hasSponsor(branch) {
		t.Fatal("runtime deployment no longer matches the approved target")
	}
	current = true
	clock = time.Unix(grant.ExpiresAt-1, 999_900_000)
	duringRuntimeCheck = func() { time.Sleep(time.Millisecond) }
	if hasSponsor(branch) {
		t.Fatal("a gas commitment expiring during the deployment check remained usable")
	}
	clock = time.Unix(200, 0)
	duringRuntimeCheck = func() { publish([]settlement.GasSponsorGrant{}) }
	if hasSponsor(branch) {
		t.Fatal("a grant removed while checking the deployment was returned as current")
	}
	duringRuntimeCheck = nil
	publish([]settlement.GasSponsorGrant{})
	if hasSponsor(branch) {
		t.Fatal("revoked gas commitment was retained after atomic publication")
	}
	publish([]settlement.GasSponsorGrant{grant, grant})
	if hasSponsor(branch) {
		t.Fatal("ambiguous duplicate commitments were accepted")
	}
}
