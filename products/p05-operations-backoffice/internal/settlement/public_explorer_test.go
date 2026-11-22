package settlement

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestPublicExplorerRestrictsConfiguredOrigin(t *testing.T) {
	for _, invalid := range []string{
		"javascript:alert(1)", "//explorer.example", "http://explorer.example",
		"https://operator:token@explorer.example", "https://explorer.example/ops/",
		"https://explorer.example/?sale=private", "https://explorer.example/#private",
		"https://explorer.example/%2f", "https://explorer.example/?",
	} {
		if _, err := NewPublicExplorer(invalid, "https://ops.example"); err == nil {
			t.Fatalf("accepted untrusted public explorer base %q", invalid)
		}
	}
	if _, err := NewPublicExplorer("https://OPS.example:443/", "https://ops.example"); err == nil {
		t.Fatal("accepted the private operator origin as public")
	}
	explorer, err := NewPublicExplorer("https://explorer.example", "https://ops.example")
	if err != nil {
		t.Fatal(err)
	}
	public, err := url.Parse(explorer.link(url.Values{
		"stream": {"pub"}, "merchant": {"public-key"}, "txHash": {"transaction"}, "logIndex": {"0"},
	}))
	if err != nil || public.Host != "explorer.example" || public.Path != "/" || len(public.Query()) != 4 ||
		public.Query().Get("logIndex") != "0" || public.User != nil || public.Fragment != "" {
		t.Fatalf("public reference was altered: %v (%v)", public, err)
	}
}

func TestPublicExplorerChecksActualPublicReceiptAndWithholdsMismatches(t *testing.T) {
	const stream = "80d39491-fa9b-4f35-9c2a-f6a258dd45d6"
	target := Target{Stream: stream, Contract: common.HexToAddress("0x412"), Asset: common.HexToAddress("0x531")}
	receipt := explorerReceipt{
		Merchant: "0x" + strings.Repeat("a", 64), Order: "0x" + strings.Repeat("c", 64),
		Transaction: "0x" + strings.Repeat("b", 64), Amount: "2", Log: 0,
	}
	for _, mode := range []string{"exact", "wrong-deployment", "held", "missing-log", "duplicate", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			explorerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/receipts" || r.Method != http.MethodGet {
					t.Error("unexpected explorer request or followed redirect")
					http.NotFound(w, r)
					return
				}
				want := url.Values{"stream": {stream}, "merchant": {receipt.Merchant}, "txHash": {receipt.Transaction}, "logIndex": {"0"}}
				if r.URL.RawQuery != want.Encode() || r.Header.Get("Referer") != "" ||
					r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("public request forwarded private context")
				}
				deployment := map[string]any{"stream": stream, "chain": "8283", "contract": lower(target.Contract),
					"asset": lower(target.Asset), "status": "current", "version": 2}
				line := map[string]any{"kind": "paid", "merchant": receipt.Merchant, "order": receipt.Order,
					"txHash": receipt.Transaction, "logIndex": receipt.Log, "amount": receipt.Amount}
				lines := []map[string]any{line}
				switch mode {
				case "wrong-deployment":
					deployment["contract"] = lower(target.Asset)
				case "held":
					deployment["status"] = "held"
				case "missing-log":
					delete(line, "logIndex")
				case "duplicate":
					lines = append(lines, line)
				case "redirect":
					http.Redirect(w, r, "/must-not-be-followed", http.StatusFound)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"deployment": deployment, "receipts": lines})
			}))
			defer explorerServer.Close()
			explorer, err := NewPublicExplorer(explorerServer.URL, "https://ops.example")
			if err != nil {
				t.Fatal(err)
			}
			if link := explorer.verifiedLink(t.Context(), target, receipt); (link != "") != (mode == "exact") {
				t.Fatalf("public receipt link %q for %s", link, mode)
			}
		})
	}
}
