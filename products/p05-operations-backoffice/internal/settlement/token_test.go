package settlement

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var testWon = commerce.Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}

var tokenBlock = &types.Header{Number: big.NewInt(9), Difficulty: new(big.Int), Time: 1_800_000_000, UncleHash: types.EmptyUncleHash}

// tokenNode answers as a chain with one payment contract and one token.
func tokenNode(t *testing.T, name string, decimals int64, code []byte) *ethclient.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&call)
		var result any
		switch call.Method {
		case "eth_chainId":
			result = "0x205b"
		case "eth_getBlockByNumber":
			result = tokenBlock
		case "eth_getCode":
			result = hexutil.Encode(code)
		case "eth_call":
			var tx struct {
				To    common.Address
				Input hexutil.Bytes
				Data  hexutil.Bytes
			}
			_ = json.Unmarshal(call.Params[0], &tx)
			input := append(tx.Input, tx.Data...)
			switch hexutil.Encode(input[:4]) {
			case "0xf698da25": // domainSeparator()
				fields := append(common.LeftPadBytes(big.NewInt(8283).Bytes(), 32), common.LeftPadBytes(tx.To.Bytes(), 32)...)
				domain := domainHash(fields)
				result = hexutil.Encode(domain[:])
			case "0x38d52e0f": // asset()
				result = hexutil.Encode(common.LeftPadBytes(common.HexToAddress("0xa1").Bytes(), 32))
			case "0x313ce567": // decimals()
				result = hexutil.Encode(common.LeftPadBytes(big.NewInt(decimals).Bytes(), 32))
			case "0x3644e515": // DOMAIN_SEPARATOR()
				domain := crypto.Keccak256(
					crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")),
					crypto.Keccak256([]byte(name)), crypto.Keccak256([]byte("1")),
					common.LeftPadBytes(big.NewInt(8283).Bytes(), 32), common.LeftPadBytes(tx.To.Bytes(), 32))
				result = hexutil.Encode(domain)
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

func TestDeploymentNeedsTheTokenItNames(t *testing.T) {
	code := []byte{0xfe}
	target := Target{Contract: common.HexToAddress("0xc0"), Asset: common.HexToAddress("0xa1"), CodeHash: crypto.Keccak256Hash(code), ABI: EscrowABIHash}
	won := target
	won.Token = testWon
	const dollarName, wonName = "NU54 Test USD", "NU54 Test KRW"
	for _, c := range []struct {
		label    string
		target   Target
		name     string
		decimals int64
		ok       bool
	}{
		{"dollar branch, dollar token", target, dollarName, 6, true},
		{"won branch, won token", won, wonName, 2, true},
		{"dollar branch, won token", target, wonName, 2, false},
		{"won branch, dollar token", won, dollarName, 6, false},
		{"won branch, same decimals, other name", won, "NU54 Test KRWC", 2, false},
		{"won branch, right name, 18 decimals", won, wonName, 18, false},
		{"dollar branch, right name, 18 decimals", target, dollarName, 18, false},
	} {
		view := VerifyOperatorDeployment(context.Background(), tokenNode(t, c.name, c.decimals, code), c.target, common.Address{}, nil, nil, nil, false)
		if (view.Status != "invalid") != c.ok {
			t.Errorf("%s: %s", c.label, view.Status)
		}
	}
	// A settlement branch keeps its old rules and cannot name a token.
	old := target
	old.ABI = ""
	if view := VerifyOperatorDeployment(context.Background(), tokenNode(t, "", 0, code), old, common.Address{}, nil, nil, nil, false); view.Status != "unverified" {
		t.Errorf("settlement branch: %s", view.Status)
	}
	old.Token = testWon
	if view := VerifyOperatorDeployment(context.Background(), tokenNode(t, wonName, 2, code), old, common.Address{}, nil, nil, nil, false); view.Status != "invalid" {
		t.Errorf("settlement branch with a token: %s", view.Status)
	}
}

func TestLockIsCheckedAgainstTheBranchToken(t *testing.T) {
	target := Target{Contract: common.HexToAddress("0x1002"), Asset: common.HexToAddress("0x1003"), ABI: EscrowABIHash}
	quote, err := escrowQuote(t, target, 1)
	if err != nil {
		t.Fatal(err)
	}
	board, confirmer := fixtureSigner(102), fixtureSigner(120).Address()
	won, err := orderescrow.NewLockFor(testWon.Name, quote.Unsigned, confirmer)
	if err != nil {
		t.Fatal(err)
	}
	dollar, err := orderescrow.NewLock(quote.Unsigned, confirmer)
	if err != nil {
		t.Fatal(err)
	}
	wonTarget := target
	wonTarget.Token = testWon
	for _, c := range []struct {
		label  string
		target Target
		digest common.Hash
		ok     bool
	}{
		{"won signature at a won branch", wonTarget, won.Digest, true},
		{"dollar signature at a won branch", wonTarget, dollar.Digest, false},
		{"won signature at a dollar branch", target, won.Digest, false},
		{"dollar signature at a dollar branch", target, dollar.Digest, true},
	} {
		signature, err := board.SignDigest(context.Background(), c.digest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := verifyLock(quote, c.target, confirmer, signature); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.label, err)
		}
	}
}

func TestApprovedDeploymentUsesTheBranchUnits(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const stream, generation = "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"
	code := []byte{0xfe}
	dollar := Target{Stream: stream, Manifest: strings.Repeat("11", 32), Contract: common.HexToAddress("0xc0"),
		Asset: common.HexToAddress("0xa1"), CodeHash: crypto.Keccak256Hash(code), ABI: EscrowABIHash}
	won := dollar
	won.Token = testWon
	units := func(n uint8) *uint8 { return &n }
	for _, c := range []struct {
		label    string
		target   Target
		name     string
		decimals int64
		approved *uint8
		ok       bool
	}{
		{"dollar", dollar, "NU54 Test USD", 6, units(6), true},
		{"dollar without approved units", dollar, "NU54 Test USD", 6, nil, true},
		{"won", won, testWon.Name, 2, units(2), true},
		{"won without approved units", won, testWon.Name, 2, nil, true},
		{"won approved as six", won, testWon.Name, 2, units(6), false},
		{"dollar approved as two", dollar, "NU54 Test USD", 6, units(2), false},
	} {
		signed := deployment.Profile{StreamID: stream, GenerationID: generation, Version: 3, ChainID: 8283, Contract: c.target.Contract,
			Asset: c.target.Asset, AssetDecimals: c.approved, ABIHash: c.target.ABIHash(), Manifest: c.target.Manifest,
			CodeHash: c.target.CodeHash, StartHeight: 9, StartHash: tokenBlock.Hash()}
		body, _ := json.Marshal(signed)
		signature := ed25519.Sign(private, append([]byte("NU54-chainview-profile-v1\x00"), body...))
		view := VerifyOperatorDeployment(context.Background(), tokenNode(t, c.name, c.decimals, code), c.target, common.Address{},
			[]deployment.Profile{signed}, public, map[string]string{generation: rawHex(signature)}, true)
		if (view.Status == "verified") != c.ok {
			t.Errorf("%s: %s", c.label, view.Status)
		}
	}
}

func TestReturnIsCheckedAgainstTheBranchToken(t *testing.T) {
	board, home, nonce := fixtureSigner(102), common.HexToAddress("0x1005"), common.HexToHash("0x77")
	target := Target{Contract: common.HexToAddress("0x1002"), Asset: common.HexToAddress("0x1003"), ABI: EscrowABIHash}
	wonTarget := target
	wonTarget.Token = testWon
	relay, err := commerce.NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	bound := service.Bound{IntentDigest: strings.Repeat("11", 32), Request: service.Request{Body: map[string]string{
		"payer": lower(board.Address()), "to": lower(home), "amount": "3420000", "validBefore": "1800000500", "nonce": nonce.Hex()}}}
	for _, c := range []struct {
		label, name string
		target      Target
		ok          bool
	}{
		{"won at a won branch", testWon.Name, wonTarget, true},
		{"dollar at a won branch", orderescrow.TokenName, wonTarget, false},
		{"won at a dollar branch", testWon.Name, target, false},
		{"dollar at a dollar branch", orderescrow.TokenName, target, true},
	} {
		digest, err := orderescrow.ReturnDigestFor(c.name, c.target.Asset, board.Address(), home, big.NewInt(3_420_000), 1_800_000_500, nonce)
		if err != nil {
			t.Fatal(err)
		}
		signature, _ := board.SignDigest(context.Background(), digest)
		stored := false
		err = Forwarder{RelayCipher: relay}.storeTransfer(context.Background(), bound, c.target, signature, func([]any) error {
			stored = true
			return nil
		})
		if stored != c.ok || (err == nil) != c.ok {
			t.Errorf("%s: stored %v, %v", c.label, stored, err)
		}
	}
}

func TestFinancePagesLearnTheBranchToken(t *testing.T) {
	won := Target{Asset: common.HexToAddress("0xa1"), Token: testWon}
	raw := []byte(`[{"id":"won","label":"a","capabilities":["reconcile_read"]},{"id":"usd","label":"b","capabilities":[]}]`)
	if same, err := scopesWithTokens(raw, map[string]Target{"usd": {}}); err != nil || !bytes.Equal(same, raw) {
		t.Fatalf("dollar scopes changed: %s %v", same, err)
	}
	mixed, err := scopesWithTokens(raw, map[string]Target{"won": won, "usd": {}})
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID, Asset string
		Token     *commerce.Token
	}
	if err := json.Unmarshal(mixed, &rows); err != nil || len(rows) != 2 || rows[0].Token == nil || *rows[0].Token != testWon ||
		rows[0].Asset != lower(won.Asset) || rows[1].Token != nil || rows[1].Asset != "" {
		t.Fatalf("%s %v", mixed, err)
	}
	if body, _ := json.Marshal(withToken(map[string]any{"amount": "1"}, Target{})); string(body) != `{"amount":"1"}` {
		t.Fatalf("dollar view changed: %s", body)
	}
	if body, _ := json.Marshal(withToken(map[string]any{"amount": "1"}, won)); !strings.Contains(string(body), `"token":{"name":"NU54 Test KRW"`) {
		t.Fatalf("won view has no token: %s", body)
	}
	if body, _ := json.Marshal(dayReport{}); strings.Contains(string(body), "token") {
		t.Fatalf("dollar report changed: %s", body)
	}
	if body, _ := json.Marshal(dayReport{Token: testWon}); !strings.Contains(string(body), `"decimals":2`) {
		t.Fatalf("won report has no token: %s", body)
	}
}
