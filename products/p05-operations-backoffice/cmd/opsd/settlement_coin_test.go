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

type chainToken struct {
	name, symbol string
	decimals     byte
}

// tokenChain answers as StableNet with one order contract's code and the named tokens.
func tokenChain(t *testing.T, code []byte, tokens map[common.Address]chainToken) *ethclient.Client {
	t.Helper()
	hash := func(text string) []byte { return crypto.Keccak256([]byte(text)) }
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
			token, known := tokens[tx.To]
			if !known {
				break
			}
			switch hexutil.Encode(tx.Input[:4]) {
			case "0x313ce567": // decimals()
				result = hexutil.Encode(common.LeftPadBytes([]byte{token.decimals}, 32))
			case "0x95d89b41": // symbol()
				result = hexutil.Encode(append(append(common.LeftPadBytes([]byte{32}, 32), common.LeftPadBytes([]byte{byte(len(token.symbol))}, 32)...),
					common.RightPadBytes([]byte(token.symbol), 32)...))
			case "0x3644e515": // DOMAIN_SEPARATOR()
				result = hexutil.Encode(crypto.Keccak256(hash("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
					hash(token.name), hash("1"), common.LeftPadBytes([]byte{0x20, 0x5b}, 32), common.LeftPadBytes(tx.To.Bytes(), 32)))
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result})
	}))
	t.Cleanup(server.Close)
	client, err := ethclient.Dial(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

// The three stores of the commercial plan on one chain, read from settings as opsd reads them.
func TestStartChecksEachBranchOfThreeTokens(t *testing.T) {
	const usd, won, krwc = "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003"
	code := []byte{0xfe}
	usdAsset, wonAsset := common.HexToAddress("0xa1"), common.HexToAddress("0xa2")
	real := map[common.Address]chainToken{usdAsset: {"NU54 Test USD", "tUSDC", 6}, wonAsset: {"NU54 Test KRW", "tKRW", 2}, commerce.StableNetCoin: {"WKRC", "WKRC", 18}}
	for _, key := range []string{"P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY", "P05_SETTLEMENT_DEPLOYMENT_PROFILES", "P05_SETTLEMENT_DEPLOYMENT_SIGNATURES"} {
		t.Setenv(key, "")
	}
	const wonToken = `{"name":"NU54 Test KRW","symbol":"tKRW","label":"tKRW","decimals":2}`
	const coinToken = `{"name":"WKRC","symbol":"WKRC","label":"KRWC","decimals":18,"boardDecimals":2}`
	target := func(asset common.Address, token string) string {
		text := `{"streamId":"00000000-0000-4000-8000-000000000009","manifestSha256":"` + strings.Repeat("11", 32) + `","contract":"0x00000000000000000000000000000000000000c0","asset":"` +
			asset.Hex() + `","codeHash":"` + crypto.Keccak256Hash(code).Hex() + `","abiSha256":"` + settlement.EscrowABIHash + `"`
		if token != "" {
			text += `,"token":` + token
		}
		return text + "}"
	}
	for label, c := range map[string]struct {
		coin  string // the settings of the KRWC branch
		chain chainToken
		off   bool
	}{
		"as the chain has it":                 {target(commerce.StableNetCoin, coinToken), real[commerce.StableNetCoin], false},
		"shown under another label":           {target(commerce.StableNetCoin, strings.Replace(coinToken, `"KRWC"`, `"WKRC"`, 1)), real[commerce.StableNetCoin], true},
		"another name in the settings":        {target(commerce.StableNetCoin, strings.Replace(coinToken, `"WKRC"`, `"KRWC"`, 1)), real[commerce.StableNetCoin], true},
		"another name on chain":               {target(commerce.StableNetCoin, coinToken), chainToken{"KRWC", "WKRC", 18}, true},
		"another symbol on chain":             {target(commerce.StableNetCoin, coinToken), chainToken{"WKRC", "KRWC", 18}, true},
		"other decimals on chain":             {target(commerce.StableNetCoin, coinToken), chainToken{"WKRC", "WKRC", 2}, true},
		"no decimals of the board":            {target(commerce.StableNetCoin, strings.Replace(coinToken, `,"boardDecimals":2`, "", 1)), real[commerce.StableNetCoin], true},
		"no token named":                      {target(commerce.StableNetCoin, ""), real[commerce.StableNetCoin], true},
		"another token that calls itself so":  {target(wonAsset, coinToken), real[commerce.StableNetCoin], true},
		"the test won at the coin's address":  {target(commerce.StableNetCoin, wonToken), chainToken{"NU54 Test KRW", "tKRW", 2}, true},
		"a token the chain does not have yet": {target(commerce.StableNetCoin, coinToken), chainToken{}, true},
	} {
		path := filepath.Join(t.TempDir(), "targets.json")
		if err := os.WriteFile(path, []byte(`{"`+usd+`":`+target(usdAsset, "")+`,"`+won+`":`+target(wonAsset, wonToken)+`,"`+krwc+`":`+c.coin+`}`), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("P05_SETTLEMENT_TARGETS", path)
		targets := map[string]settlement.Target{}
		if err := configJSON("P05_SETTLEMENT_TARGETS", &targets); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		tokens := map[common.Address]chainToken{usdAsset: real[usdAsset], wonAsset: real[wonAsset]}
		if c.chain != (chainToken{}) {
			tokens[commerce.StableNetCoin] = c.chain
		}
		active, reviews := checkSettlementDeployments(context.Background(), tokenChain(t, code, tokens), targets, common.Address{})
		_, usdOn := active[usd]
		_, wonOn := active[won]
		_, coinOn := active[krwc]
		if !usdOn || !wonOn || coinOn == c.off || (reviews[krwc].Status == "invalid") != c.off {
			t.Errorf("%s: active %t %t %t, review %s", label, usdOn, wonOn, coinOn, reviews[krwc].Status)
		}
		paused := settlement.Forwarder{RentalEscrow: settlement.RentalEscrow{Targets: active, Reviews: reviews}}
		if paused.Paused(usd) || paused.Paused(won) || paused.Paused(krwc) != c.off {
			t.Errorf("%s: paused %t %t %t", label, paused.Paused(usd), paused.Paused(won), paused.Paused(krwc))
		}
	}
}
