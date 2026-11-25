package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/settlement"
)

func TestBranchUsesOneToken(t *testing.T) {
	won := commerce.Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}
	other := won
	other.Decimals = 18
	for _, c := range []struct {
		profile, target commerce.Token
		ok              bool
	}{
		{commerce.Token{}, commerce.Token{}, true},
		{won, won, true},
		{commerce.Token{}, commerce.DollarToken, true},
		{won, commerce.Token{}, false},
		{commerce.Token{}, won, false},
		{won, other, false},
	} {
		err := matchContracts(map[string]commerce.Profile{"b": {Escrow: true, Token: c.profile}},
			map[string]settlement.Target{"b": {ABI: settlement.EscrowABIHash, Token: c.target}})
		if (err == nil) != c.ok {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

func TestTokenSettingsAreStrict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "targets.json")
	for text, ok := range map[string]bool{
		`{"b":{"token":{"name":"NU54 Test KRW","symbol":"tKRW","label":"tKRW","decimals":2}}}`: true,
		`{"b":{}}`: true,
		`{"b":{"token":{"name":"NU54 Test KRW","symbol":"tKRW","label":"tKRW","decimals":2,"extra":true}}}`: false,
		`{"b":{"token":{"name":"NU54 Test KRW","symbol":"tKRW","label":"tKRW","decimals":-2}}}`:             false,
	} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("P05_SETTLEMENT_TARGETS", path)
		targets := map[string]settlement.Target{}
		if err := configJSON("P05_SETTLEMENT_TARGETS", &targets); (err == nil) != ok {
			t.Errorf("%s: %v", text, err)
		}
	}
}

func TestIssuanceProfileChecksItsToken(t *testing.T) {
	won := commerce.Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}
	good := enrolledProfile{Chain: 8283, Contract: "0x00000000000000000000000000000000000000c0", Asset: "0x00000000000000000000000000000000000000a1",
		Beneficiary: "0x00000000000000000000000000000000000000b1", Signer: "0x00000000000000000000000000000000000000d1",
		Merchant: strings.Repeat("07", 32), KeyVersion: 1, BeneficiaryVersion: 1, ABI: settlement.EscrowABIHash, Token: won}
	if profile, err := good.issuance([32]byte{1}); err != nil || profile.Token != won || !profile.Escrow {
		t.Fatalf("%+v %v", profile, err)
	}
	plain := good
	plain.Token, plain.ABI = commerce.Token{}, ""
	if profile, err := plain.issuance([32]byte{1}); err != nil || profile.Token.OrDollar() != commerce.DollarToken {
		t.Fatalf("%+v %v", profile, err)
	}
	for label, change := range map[string]func(*enrolledProfile){
		"half a token":                 func(p *enrolledProfile) { p.Token.Name = "" },
		"token on a settlement branch": func(p *enrolledProfile) { p.ABI = "" },
		"unknown contract":             func(p *enrolledProfile) { p.ABI = strings.Repeat("0", 64) },
		"another chain":                func(p *enrolledProfile) { p.Chain = 1 },
		"no merchant":                  func(p *enrolledProfile) { p.Merchant = strings.Repeat("00", 32) },
		"no token address":             func(p *enrolledProfile) { p.Asset = "" },
	} {
		bad := good
		change(&bad)
		if _, err := bad.issuance([32]byte{1}); err == nil {
			t.Errorf("%s accepted", label)
		}
	}
}
