package settlement

import (
	"context"
	"encoding/json"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

func (h FinanceHTTP) operatorPaymentEvidence(ctx context.Context, actor service.Principal, branch, sale string, target Target, openPublic bool) (map[string]any, error) {
	out := map[string]any{"status": "pending", "chain": 8283, "contract": lower(target.Contract),
		"asset": lower(target.Asset), "manifest": target.Manifest, "abi": target.ABIHash(), "runtimeCodeHash": target.CodeHash.Hex()}
	var state string
	var cipher, digest, signer []byte
	var relayState, relayHash, finalHash, finalBlock *string
	var finalLog *int32
	err := h.Store.db.QueryRow(ctx, "SELECT * FROM ledger.operator_payment_evidence($1,$2,$3,$4)", actor.Tenant, branch, actor.Subject, sale).
		Scan(&state, &cipher, &digest, &signer, &relayState, &relayHash, &finalHash, &finalLog, &finalBlock)
	if err != nil {
		return nil, err
	}
	deployment := h.Deployments[branch]
	if deployment.Status == "invalid" {
		return map[string]any{"status": "invalid", "deploymentBlocked": true}, nil
	}
	out["orderState"] = state
	if relayHash != nil && relayState != nil &&
		(*relayState == "broadcast" || *relayState == "observed" || *relayState == "reverted") {
		out["attempt"] = *relayHash
	}
	if relayState != nil && *relayState == "reverted" {
		out["status"] = "held"
	}
	if h.QuoteCipher == nil || len(cipher) == 0 {
		return out, nil
	}
	unsigned, err := h.QuoteCipher.Open(nil, nil, cipher, quoteAAD(branch, sale))
	if err != nil {
		out["status"] = "invalid"
		return out, nil
	}
	quote, err := unpackQuote(unsigned, digest, signer, target)
	if err != nil {
		out["status"] = "invalid"
		return out, nil
	}
	out["beneficiary"] = lower(quote.Args.Beneficiary)
	out["beneficiaryVersion"] = quote.Args.BeneficiaryVersion
	out["amount"] = quote.Args.Amount.String()
	out["quoteIssuedAt"] = time.Unix(int64(quote.Args.IssuedAt), 0).UTC()
	out["quoteExpiresAt"] = quote.expiry()
	out["quoteDigest"] = common.Hash(quote.Digest).Hex()
	if relayHash != nil && (finalHash == nil || *relayHash != *finalHash) && deployment.Status == "verified" {
		if rejected := h.revertedPayment(ctx, target, quote, *relayHash); rejected != nil {
			out["attempt"] = *relayHash
			out["revertedAttempt"] = rejected
			out["status"] = "reverted-attempt"
		}
	}
	if target.Escrow() {
		var item []byte
		if err := h.Store.db.QueryRow(ctx, "SELECT ledger.finance_escrow_item($1,$2,$3,$4)", actor.Tenant, branch, actor.Subject, sale).Scan(&item); err != nil {
			return nil, err
		}
		if item != nil {
			out["escrow"] = json.RawMessage(item)
		}
	}
	if h.Evidence == nil || finalHash == nil || finalBlock == nil || finalLog == nil {
		return out, nil
	}
	if target.Escrow() {
		h.escrowEvidence(ctx, target, quote, *finalHash, *finalBlock, *finalLog, out)
		return out, nil
	}
	merchant := common.Hash(quote.Args.MerchantId).Hex()
	evidenceErr := h.Evidence.lockScope(ctx, target.Stream, merchant, func(tx pgx.Tx) error {
		var txHash, blockHash, amount, recipient string
		var logIndex int32
		var anchor int64
		var included, attested time.Time
		err := tx.QueryRow(ctx, `SELECT tx_hash,inclusion_hash,log_index,anchor_height,trim_scale(amount)::text,recipient_ref,included_at,attested_at
			FROM chainview.reconciliation_receipt($1,$2,$3)
			WHERE kind='paid' AND tx_hash=$4 AND log_index=$5
			  AND quote_digest=$6 AND chain_ref='8283' AND contract_ref=$7 AND abi_hash=$8 AND policy_ref=$9 AND asset_ref=$10`,
			target.Stream, merchant, common.Hash(quote.Args.OrderId).Hex(), *finalHash, *finalLog, common.Hash(quote.Digest).Hex(),
			lower(target.Contract), target.ABIHash(), target.Manifest, lower(target.Asset)).
			Scan(&txHash, &blockHash, &logIndex, &anchor, &amount, &recipient, &included, &attested)
		if err != nil {
			return err
		}
		if blockHash != *finalBlock || amount != quote.Args.Amount.String() || recipient != lower(quote.Args.Beneficiary) ||
			included.Unix() < int64(quote.Args.IssuedAt) || !included.Before(quote.expiry()) {
			out["status"] = "invalid"
			return nil
		}
		out["status"] = "verified"
		out["transaction"] = txHash
		out["block"] = blockHash
		out["log"] = logIndex
		out["finalizedHeight"] = anchor
		out["includedAt"] = included.UTC()
		out["attestedAt"] = attested.UTC()
		out["includedBeforeExpiry"] = true
		return nil
	})
	if evidenceErr != nil {
		out["status"] = "held"
	}
	if openPublic && h.PublicExplorer != nil && out["status"] == "verified" && h.Merchants[branch] == merchant {
		probe, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if h.publicDeploymentCurrent(probe, target, deployment) {
			link := h.PublicExplorer.verifiedLink(probe, target, explorerReceipt{
				Merchant: merchant, Order: common.Hash(quote.Args.OrderId).Hex(), Amount: quote.Args.Amount.String(),
				Transaction: *finalHash, Log: *finalLog,
			})
			if link != "" {
				out["explorerURL"] = link
			}
		}
	}
	return out, nil
}

func (h FinanceHTTP) notificationReview(ctx context.Context, actor service.Principal, branch, asset, event, key string, day time.Time, target Target) (map[string]any, error) {
	var requestID *string
	if key != "" {
		requestID = &key
	}
	var raw []byte
	err := h.Store.db.QueryRow(ctx, "SELECT ledger.finance_notification_review($1,$2,$3,$4,$5,$6,$7)",
		actor.Tenant, branch, actor.Subject, day, asset, event, requestID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var view map[string]any
	if err := json.Unmarshal(raw, &view); err != nil {
		return nil, err
	}
	sale, _ := view["saleId"].(string)
	payment, err := h.operatorPaymentEvidence(ctx, actor, branch, sale, target, true)
	if err != nil {
		return nil, err
	}
	// Compare the original payment with the event posting. The raw beneficiary
	// and delivery payload are unnecessary for deciding how to recover an event.
	original := map[string]any{
		"status": payment["status"], "beneficiaryVersion": payment["beneficiaryVersion"],
		"transaction": payment["transaction"], "includedAt": payment["includedAt"],
		"attestedAt": payment["attestedAt"], "finalizedHeight": payment["finalizedHeight"],
	}
	if link, ok := payment["explorerURL"].(string); ok {
		original["explorerURL"] = link
	}
	view["originalPayment"] = original
	return view, nil
}
