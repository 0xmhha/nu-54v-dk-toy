package settlement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

type escrowInput struct {
	Status   string   `json:"status,omitempty"`
	Sales    []string `json:"saleIds,omitempty"`
	Outcome  string   `json:"outcome,omitempty"`
	Decision string   `json:"decisionId,omitempty"`
}

var (
	escrowStatuses = []string{"", "held", "waiting", "claimable", "claiming", "disputed", "paid", "refunded"}
	escrowReasons  = []string{"not_delivered", "delivered_proof", "wrong_item", "goodwill"}
)

// escrow serves the order escrow operations. The second result is false for a malformed request.
func (h FinanceHTTP) escrow(ctx context.Context, actor service.Principal, operation string, fp [32]byte, branch, cursor, sale, key, reason string, in escrowInput) (any, bool, error) {
	var page *string
	if cursor != "" {
		if !financeRef.MatchString(cursor) {
			return nil, false, nil
		}
		page = &cursor
	}
	if operation != "escrow-orders" && operation != "escrow-claims" && !financeRef.MatchString(key) {
		return nil, false, nil
	}
	query, args := "", []any{actor.Tenant, branch, actor.Subject}
	switch operation {
	case "escrow-orders":
		if !slices.Contains(escrowStatuses, in.Status) {
			return nil, false, nil
		}
		query, args = "SELECT ledger.finance_escrow_orders($1,$2,$3,$4,$5)", append(args, in.Status, page)
	case "escrow-claims":
		query, args = "SELECT ledger.finance_escrow_claims($1,$2,$3,$4)", append(args, page)
	case "escrow-claim":
		seen := map[string]bool{}
		for _, id := range in.Sales {
			if !financeRef.MatchString(id) || seen[id] {
				return nil, false, nil
			}
			seen[id] = true
		}
		if len(seen) < 1 || len(seen) > 100 {
			return nil, false, nil
		}
		query, args = "SELECT ledger.finance_escrow_claim($1,$2,$3,$4,$5::text[]::uuid[])", append(args, key, in.Sales)
	case "escrow-resolve":
		if !financeRef.MatchString(sale) || (in.Outcome != "pay-merchant" && in.Outcome != "refund-buyer") || !slices.Contains(escrowReasons, reason) {
			return nil, false, nil
		}
		query, args = "SELECT ledger.finance_escrow_resolve($1,$2,$3,$4,$5,$6,$7)", append(args, key, sale, in.Outcome, reason)
	default:
		if !financeRef.MatchString(in.Decision) {
			return nil, false, nil
		}
		query, args = "SELECT ledger.finance_escrow_decide($1,$2,$3,$4,$5)", append(args, in.Decision, operation == "escrow-resolve-approve")
	}
	var raw []byte
	if err := h.Store.db.QueryRow(ctx, query, args...).Scan(&raw); err != nil || strings.HasPrefix(operation, "escrow-resolve") {
		return json.RawMessage(raw), true, err
	}
	// Request keys belong to this certificate and branch, like the other finance receipts.
	var view map[string]any
	if err := json.Unmarshal(raw, &view); err != nil {
		return nil, true, err
	}
	digest := sha256.Sum256([]byte("nu54-escrow-claim:" + hex.EncodeToString(fp[:]) + ":" + branch))
	scope := hex.EncodeToString(digest[:])
	view["receiptScope"] = scope
	if items, list := view["items"].([]any); list && operation == "escrow-claims" {
		for _, item := range items {
			item.(map[string]any)["receiptScope"] = scope
		}
	}
	return view, true, nil
}
