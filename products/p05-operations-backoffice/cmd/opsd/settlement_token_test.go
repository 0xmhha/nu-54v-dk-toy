package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/settlement"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func TestBranchUsesOneToken(t *testing.T) {
	won := commerce.Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}
	coin := commerce.Token{Name: "WKRC", Symbol: "WKRC", Label: "KRWC", Decimals: 18, BoardDecimals: 2}
	otherUnit := coin
	otherUnit.BoardDecimals = 3
	other, otherName, otherSymbol := won, won, won
	other.Decimals, otherName.Name, otherSymbol.Symbol = 18, "NU54 Test KRWC", "TKRW"
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
		{won, otherName, false},
		{won, otherSymbol, false},
		{coin, coin, true},
		{coin, won, false},
		{coin, otherUnit, false},
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
		`{"b":{"token":{"name":"NU54 Test KRW","symbol":"tKRW","label":"tKRW","decimals":2}}}`:            true,
		`{"b":{"token":{"name":"WKRC","symbol":"WKRC","label":"KRWC","decimals":18,"boardDecimals":2}}}`:  true,
		`{"b":{"token":{"name":"WKRC","symbol":"WKRC","label":"KRWC","decimals":18,"boardDecimals":-2}}}`: false,
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

func TestLabelIsOnlyShown(t *testing.T) {
	won := commerce.Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}
	shown := won
	shown.Label = "KRW"
	if err := matchContracts(map[string]commerce.Profile{"b": {Escrow: true, Token: won}},
		map[string]settlement.Target{"b": {ABI: settlement.EscrowABIHash, Token: shown}}); err != nil {
		t.Fatal(err)
	}
}

// Two branches on one chain: the one whose token differs is turned off, the other keeps running.
func TestOnlyTheWrongBranchIsTurnedOff(t *testing.T) {
	const right, wrong = "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"
	code := []byte{0xfe}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			ID     json.RawMessage
			Method string
			Params []json.RawMessage
		}
		_ = json.NewDecoder(r.Body).Decode(&call)
		var result any
		switch call.Method {
		case "eth_chainId":
			result = "0x205b"
		case "eth_getCode":
			result = hexutil.Encode(code)
		case "eth_call":
			var tx struct {
				To    common.Address
				Input hexutil.Bytes
			}
			_ = json.Unmarshal(call.Params[0], &tx)
			switch hexutil.Encode(tx.Input[:4]) {
			case "0x313ce567":
				result = hexutil.Encode(common.LeftPadBytes([]byte{6}, 32))
			case "0x3644e515":
				hash := func(text string) []byte { return crypto.Keccak256([]byte(text)) }
				result = hexutil.Encode(crypto.Keccak256(hash("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
					hash("NU54 Test USD"), hash("1"), common.LeftPadBytes([]byte{0x20, 0x5b}, 32), common.LeftPadBytes(tx.To.Bytes(), 32)))
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result})
	}))
	defer server.Close()
	client, err := ethclient.Dial(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, key := range []string{"P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY", "P05_SETTLEMENT_DEPLOYMENT_PROFILES", "P05_SETTLEMENT_DEPLOYMENT_SIGNATURES"} {
		t.Setenv(key, "")
	}
	target := settlement.Target{Stream: "00000000-0000-4000-8000-000000000003", Manifest: strings.Repeat("11", 32), Contract: common.HexToAddress("0xc0"),
		Asset: common.HexToAddress("0xa1"), CodeHash: crypto.Keccak256Hash(code), ABI: settlement.EscrowABIHash}
	won := target
	won.Token = commerce.Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}
	targets := map[string]settlement.Target{right: target, wrong: won}
	active, reviews := checkSettlementDeployments(context.Background(), client, targets, common.Address{})
	if _, ok := active[right]; !ok || len(active) != 1 || reviews[wrong].Status != "invalid" {
		t.Fatalf("active %v, reviews %+v", active, reviews)
	}
	forwarder := settlement.Forwarder{RentalEscrow: settlement.RentalEscrow{Targets: active, Reviews: reviews}}
	if !forwarder.Paused(wrong) || forwarder.Paused(right) {
		t.Fatalf("paused: %+v", reviews)
	}
}

// A start reads the settings first, so branches whose two settings differ stop it before anything else is needed.
func TestStartRefusesTwoTokensForOneBranch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "targets.json")
	if err := os.WriteFile(path, []byte(`{"b":{"abiSha256":"`+settlement.EscrowABIHash+`","token":{"name":"NU54 Test KRW","symbol":"tKRW","label":"tKRW","decimals":2}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("P05_SETTLEMENT_TARGETS", path)
	_, _, err := installSettlement(context.Background(), context.Background(), "", nil, map[string]commerce.Profile{"b": {Escrow: true}}, nil, http.NewServeMux(), nil)
	if err == nil || !strings.Contains(err.Error(), "different payment tokens") {
		t.Fatalf("start went on: %v", err)
	}
}
