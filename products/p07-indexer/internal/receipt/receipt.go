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
	Merchant    string
	OrderID     string
	Device      string
	Amount      string
	Nonce       string
	TxHash      string
	BlockNumber uint64
	BlockTime   uint64
	// Duplicate: more than one log for (merchant, orderId); the earliest is returned (P07-FR-05).
	Duplicate bool
}

// view is the response body (P07-FR-06, design 4): addresses in full, the tx hash shortened [N16].
type view struct {
	Merchant    string `json:"merchant"`
	OrderID     string `json:"orderId"`
	Device      string `json:"device"`
	Amount      string `json:"amount"`
	BlockNumber uint64 `json:"blockNumber"`
	BlockTime   uint64 `json:"blockTime"`
	TxHashShort string `json:"txHashShort"`
	Duplicate   bool   `json:"duplicate"`
}

// Short is a hash as 0x + 6 … last 4 [N16].
func Short(h string) string {
	if len(h) < 12 {
		return h
	}
	return h[:8] + "…" + h[len(h)-4:]
}

// ErrNotFound means no finalized PaymentSettled exists for the key.
var ErrNotFound = errors.New("receipt not found")

// Store looks receipts up.
type Store interface {
	Get(ctx context.Context, merchant, orderID string) (Receipt, error)
}

// Freshness reports how far ingestion is behind the finalized head.
type Freshness interface {
	// Lag is finalized minus cursor in blocks, and the cursor; ok is false before the first poll.
	Lag() (lag, cursor uint64, ok bool)
}

// StaleBlocks: answer 503 when the cursor is more than this many blocks behind (design 4).
const StaleBlocks = 60

var (
	addrRe  = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	orderRe = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Handler serves GET /receipts/{merchant}/{orderId} (read only, P07-NFR-03).
func Handler(store Store, fresh Freshness) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /receipts/{merchant}/{orderId}", func(w http.ResponseWriter, r *http.Request) {
		merchant, orderID := r.PathValue("merchant"), r.PathValue("orderId")
		if !addrRe.MatchString(merchant) || !orderRe.MatchString(orderID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "BAD_REQUEST"})
			return
		}
		rec, err := store.Get(r.Context(), merchant, orderID)
		switch {
		case errors.Is(err, ErrNotFound):
			// Not indexed yet may also mean ingestion is behind: say so instead of a plain 404.
			if lag, cursor, ok := fresh.Lag(); ok && lag > StaleBlocks {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "RPC_STALE", "cursor": cursor})
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "NOT_INDEXED"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "INTERNAL"})
		default:
			writeJSON(w, http.StatusOK, view{rec.Merchant, rec.OrderID, rec.Device, rec.Amount, rec.BlockNumber, rec.BlockTime, Short(rec.TxHash), rec.Duplicate})
		}
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		lag, cursor, ok := fresh.Lag()
		writeJSON(w, http.StatusOK, map[string]any{"cursor": cursor, "lag": lag, "polled": ok})
	})
	return mux
}
