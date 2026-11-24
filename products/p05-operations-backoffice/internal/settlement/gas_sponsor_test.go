package settlement

import (
	"bytes"
	"testing"

	sponsorabi "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/gassponsor"
	"github.com/ethereum/go-ethereum/common"
)

func TestSponsoredRelay(t *testing.T) {
	target := Target{Contract: common.Address{19: 1}, Asset: common.Address{19: 2}, Sponsor: common.Address{19: 3}, ABI: EscrowABIHash}
	lock, call := work{}, []byte{1, 2, 3, 4, 5}
	returned := work{action: &escrowAction{kind: "return"}}
	resolve := work{action: &escrowAction{kind: "resolve"}}
	for name, job := range map[string]work{"lock": lock, "return": returned} {
		to, data, err := relayTo(job, target, call)
		if err != nil || to != target.Sponsor || bytes.Equal(data, call) {
			t.Fatalf("%s is not sponsored: %v %v", name, to, err)
		}
		inner, err := relayed(job, target, to, data)
		if err != nil || !bytes.Equal(inner, call) {
			t.Fatalf("%s lost its call: %x %v", name, inner, err)
		}
		// Signed before the branch had a sponsor.
		if inner, err := relayed(job, target, destination(job, target), call); err != nil || !bytes.Equal(inner, call) {
			t.Fatalf("%s direct call refused: %v", name, err)
		}
		if _, err := relayed(job, target, common.Address{19: 9}, data); err == nil {
			t.Fatalf("%s accepted another destination", name)
		}
		if _, err := relayed(job, target, to, append(bytes.Clone(data), make([]byte, 32)...)); err == nil {
			t.Fatalf("%s accepted bytes after the call", name)
		}
		if _, err := relayed(job, target, to, call); err == nil {
			t.Fatalf("%s accepted a bare call to the sponsor", name)
		}
	}
	// A sponsored lock does not stand for a sponsored return.
	_, data, _ := relayTo(lock, target, call)
	if _, err := relayed(returned, target, target.Sponsor, data); err == nil {
		t.Fatal("a return accepted a call to the escrow")
	}
	// The arbiter decides in its own name, and a branch without a sponsor or an escrow relays directly.
	none, legacy := target, target
	none.Sponsor, legacy.ABI = common.Address{}, ""
	for name, item := range map[string]struct {
		job    work
		target Target
	}{"resolve": {resolve, target}, "no sponsor": {lock, none}, "settlement v2": {lock, legacy}} {
		to, data, err := relayTo(item.job, item.target, call)
		if err != nil || to != target.Contract || !bytes.Equal(data, call) {
			t.Fatalf("%s went through the sponsor: %v", name, err)
		}
		if _, err := relayed(item.job, item.target, target.Sponsor, call); err == nil {
			t.Fatalf("%s accepted the sponsor", name)
		}
	}
	lists, err := sponsoredSelectors()
	if err != nil || len(lists.escrow) != len(escrowCalls)+1 || len(lists.token) != 1 {
		t.Fatalf("selectors: %d %d %v", len(lists.escrow), len(lists.token), err)
	}
	if contract, err := sponsorabi.GassponsorMetaData.GetAbi(); err != nil || len(contract.Methods["sponsor"].Inputs) != 2 {
		t.Fatal("sponsor method")
	}
}
