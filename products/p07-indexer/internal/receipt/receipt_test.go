package receipt

import (
	"context"
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

func TestHandler(t *testing.T) {
	merchant := "0x" + strings.Repeat("a1", 20)
	order := "0x" + strings.Repeat("01", 32)
	h := Handler(memStore{merchant + "/" + order: {Merchant: merchant, OrderID: order, Amount: "4500000"}})

	cases := []struct {
		path string
		want int
	}{
		{"/receipts/" + merchant + "/" + order, http.StatusOK},
		{"/receipts/" + merchant + "/0x" + strings.Repeat("02", 32), http.StatusNotFound},
		{"/receipts/not-an-address/" + order, http.StatusBadRequest},
		{"/healthz", http.StatusOK},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.path, rec.Code, c.want)
		}
	}
}
