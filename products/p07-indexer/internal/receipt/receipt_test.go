package receipt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type memStore map[string]Receipt

func (m memStore) Get(_ context.Context, merchant, orderID string) (Receipt, error) {
	if r, ok := m[merchant+"/"+orderID]; ok {
		return r, nil
	}
	return Receipt{}, ErrNotFound
}

type lag struct{ lag, cursor uint64 }

func (l lag) Lag() (uint64, uint64, bool) { return l.lag, l.cursor, true }

func TestHandler(t *testing.T) {
	merchant := "0x" + strings.Repeat("a1", 20)
	order := "0x" + strings.Repeat("01", 32)
	tx := "0x" + strings.Repeat("ab", 31) + "cdef"
	store := memStore{merchant + "/" + order: {Merchant: merchant, OrderID: order, Device: "0x" + strings.Repeat("d1", 20),
		Amount: "4500000", TxHash: tx, BlockNumber: 7, BlockTime: 1790000000, Duplicate: true}}
	h := Handler(store, lag{0, 100})

	cases := []struct {
		path string
		want int
		body string
	}{
		{"/receipts/" + merchant + "/" + order, http.StatusOK, `"txHashShort":"0xababab…cdef"`},
		{"/receipts/" + merchant + "/0x" + strings.Repeat("02", 32), http.StatusNotFound, `"NOT_INDEXED"`},
		{"/receipts/not-an-address/" + order, http.StatusBadRequest, `"BAD_REQUEST"`},
		{"/healthz", http.StatusOK, `"cursor":100`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != c.want || !strings.Contains(rec.Body.String(), c.body) {
			t.Errorf("%s: %d %s, want %d with %s", c.path, rec.Code, rec.Body.String(), c.want, c.body)
		}
	}

	// P07-FR-06: the response carries exactly the design's fields.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/receipts/"+merchant+"/"+order, nil))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	for _, k := range []string{"merchant", "orderId", "device", "amount", "blockNumber", "blockTime", "txHashShort", "duplicate"} {
		if _, ok := body[k]; !ok {
			t.Errorf("response lacks %s", k)
		}
	}
	if len(body) != 8 || body["duplicate"] != true {
		t.Errorf("response %v", body)
	}

	// Write methods do not exist (P07-NFR-03).
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/receipts/"+merchant+"/"+order, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}

func TestNotIndexedWhileFarBehindIsStale(t *testing.T) {
	h := Handler(memStore{}, lag{StaleBlocks + 1, 40})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/receipts/0x"+strings.Repeat("a1", 20)+"/0x"+strings.Repeat("01", 32), nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"RPC_STALE"`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}
