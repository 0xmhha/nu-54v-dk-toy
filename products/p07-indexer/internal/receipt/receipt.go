// Package receipt serves PaymentSettled receipts by (merchant, orderId) ([N19]).
// P07 is a lookup service only; the kiosk decides "paid" on its own ([N08]).
package receipt

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
)

// Receipt is one finalized PaymentSettled event.
type Receipt struct {
	Merchant    string `json:"merchant"`
	OrderID     string `json:"orderId"`
	Device      string `json:"device"`
	Amount      string `json:"amount"`
	Nonce       string `json:"nonce"`
	TxHash      string `json:"txHash"`
	BlockNumber uint64 `json:"blockNumber"`
}

// ErrNotFound means no finalized PaymentSettled exists for the key.
var ErrNotFound = errors.New("receipt not found")

// Store looks receipts up; the PostgreSQL implementation arrives with WBS2-P07-01.
type Store interface {
	Get(ctx context.Context, merchant, orderID string) (Receipt, error)
}

var (
	addrRe  = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	orderRe = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
)

// Handler serves GET /receipts/{merchant}/{orderId}.
func Handler(store Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /receipts/{merchant}/{orderId}", func(w http.ResponseWriter, r *http.Request) {
		merchant, orderID := r.PathValue("merchant"), r.PathValue("orderId")
		if !addrRe.MatchString(merchant) || !orderRe.MatchString(orderID) {
			http.Error(w, "bad merchant or orderId", http.StatusBadRequest)
			return
		}
		rec, err := store.Get(r.Context(), merchant, orderID)
		switch {
		case errors.Is(err, ErrNotFound):
			http.Error(w, "not found", http.StatusNotFound)
		case err != nil:
			http.Error(w, "internal error", http.StatusInternalServerError)
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rec)
		}
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}
