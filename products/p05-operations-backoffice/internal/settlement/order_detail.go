package settlement

import (
	"context"
	"encoding/json"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

type orderChainScope struct {
	OrderKey    string
	QuoteDigest string
	Stream      string
	Candidate   string
	Merchant    string
	Block       *string
	Log         *int32
}

func (h FinanceHTTP) financeOrder(ctx context.Context, actor service.Principal, branch, sale string, target Target) (map[string]json.RawMessage, error) {
	var raw, scopeRaw []byte
	if err := h.Store.db.QueryRow(ctx, "SELECT detail,chain_scope FROM ledger.finance_order_snapshot($1,$2,$3,$4)",
		actor.Tenant, branch, actor.Subject, sale).Scan(&raw, &scopeRaw); err != nil {
		return nil, err
	}
	var detail map[string]json.RawMessage
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, err
	}
	detail["observedHeight"] = json.RawMessage("null")
	detail["confirmedHeight"] = json.RawMessage("null")

	merchant := h.Merchants[branch]
	if h.Evidence == nil || merchant == "" {
		return detail, nil
	}
	var scope orderChainScope
	if err := json.Unmarshal(scopeRaw, &scope); err != nil {
		return nil, err
	}
	if scope.Stream != target.Stream || (scope.Merchant != "" && scope.Merchant != merchant) {
		return detail, nil
	}
	tx := scope.Candidate
	if scope.Block != nil {
		if err := json.Unmarshal(detail["transaction"], &tx); err != nil {
			return nil, err
		}
	}
	if tx == "" {
		return detail, nil
	}
	var amount string
	if err := json.Unmarshal(detail["amount"], &amount); err != nil {
		return nil, err
	}
	var observed, confirmed *int64
	err := h.Evidence.db.QueryRow(ctx, `SELECT * FROM chainview.finance_order_observation(
		$1,$2,$3,$4,$5,$6,$7,$8,$9,$10::text::numeric,$11,$12)`,
		target.Stream, merchant, scope.OrderKey, scope.QuoteDigest, tx,
		lower(target.Contract), target.ABIHash(), target.Manifest, lower(target.Asset), amount, scope.Block, scope.Log).
		Scan(&observed, &confirmed)
	if err != nil || observed == nil {
		// The saved order remains available when the independent evidence reader is unavailable or held.
		return detail, nil
	}
	detail["transaction"], _ = json.Marshal(tx)
	detail["observedHeight"], _ = json.Marshal(observed)
	detail["confirmedHeight"], _ = json.Marshal(confirmed)
	return detail, nil
}
