package settlement

import (
	"context"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgconn"
)

type FinanceHTTP struct {
	Store          *Store
	Credentials    map[string]service.Principal
	OperatorOrigin string
	Targets        map[string]Target
	Merchants      map[string]string
	Evidence       *EvidenceStore
	CloseStore     *Store
	Funds          Chain
	QuoteCipher    cipher.AEAD
	Deployments    map[string]DeploymentReview
	PublicExplorer *PublicExplorer
	GasAccount     common.Address
	Resolver       common.Address // settles disputes; zero turns those operations off
}

var financeCapabilities = map[string][]string{
	"report": {"report_read"}, "report-export": {"report_export"}, "order": {"report_read"},
	"reconciliation": {"reconcile_read"}, "fulfillment-review": {"reconcile_read"}, "payment-evidence": {"reconcile_read"}, "fulfillment-retry": {"reconcile_retry"},
	"report-close": {"day_close"}, "day-close": {"day_close"}, "refund-reserve": {"refund_request"},
	"deployment-profile": {"reconcile_read"},
	"refund-approve":     {"refund_approve"}, "refund-reject": {"refund_approve"},
	"refund-status": {"refund_request", "refund_approve", "refund_submit"},
	"refund-queue":  {"refund_request", "refund_approve", "refund_submit"},
	"refund-submit": {"refund_submit"},
	"escrow-orders": {"report_read"}, "escrow-claims": {"report_read"}, "escrow-claim": {"escrow_claim"},
	"escrow-resolve":         {"escrow_dispute_request"},
	"escrow-resolve-approve": {"escrow_dispute_approve"}, "escrow-resolve-reject": {"escrow_dispute_approve"},
}

var financeRef = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var financeAsset = regexp.MustCompile(`^0x[0-9a-f]{40}$`)

func refundRejectionAllowed(code string) bool {
	return code == "route_review" || code == "amount_review" || code == "policy_review"
}

func (h FinanceHTTP) Register(mux *http.ServeMux) {
	mux.Handle("POST /v2/finance/{operation}", authority.Protect(h.Credentials, http.HandlerFunc(h.serve)))
	mux.Handle("GET /v2/finance/scopes", authority.Protect(h.Credentials, http.HandlerFunc(h.scopes)))
}

func (h FinanceHTTP) credential(r *http.Request) (service.Principal, [32]byte, time.Time, bool) {
	principal, client, ok := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	return principal, client.Fingerprint, client.ExpiresAt, ok && principal.Role == "finance"
}

func (h FinanceHTTP) scopes(w http.ResponseWriter, r *http.Request) {
	principal, _, deadline, ok := h.credential(r)
	if !ok || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	var scopes []byte
	if err := h.Store.db.QueryRow(ctx, "SELECT ledger.operator_scopes($1,$2,'finance')",
		principal.Tenant, principal.Subject).Scan(&scopes); err != nil {
		http.Error(w, "finance access unavailable", http.StatusServiceUnavailable)
		return
	}
	scopes, err := scopesWithTokens(scopes, h.Targets)
	if err != nil {
		http.Error(w, "finance access unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(scopes)
}

// withToken adds the token of a branch that names one, so a page never shows its amounts as dollars.
func withToken[V any](view map[string]V, target Target) any {
	if view == nil || target.Token == (commerce.Token{}) {
		return view
	}
	out := make(map[string]any, len(view)+1)
	for key, value := range view {
		out[key] = value
	}
	out["token"] = target.Token
	return out
}

// scopesWithTokens adds the asset and token of every listed branch that names one. Other rows stay as they are.
func scopesWithTokens(raw []byte, targets map[string]Target) ([]byte, error) {
	named := false
	for _, target := range targets {
		named = named || target.Token != (commerce.Token{})
	}
	if !named {
		return raw, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		id, _ := row["id"].(string)
		if target := targets[id]; target.Token != (commerce.Token{}) {
			row["asset"], row["token"] = lower(target.Asset), target.Token
		}
	}
	return json.Marshal(rows)
}

func (h FinanceHTTP) serve(w http.ResponseWriter, r *http.Request) {
	principal, fp, deadline, ok := h.credential(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var in struct {
		Branch         string `json:"branchId"`
		Cursor         string `json:"cursor,omitempty"`
		Sale           string `json:"saleId,omitempty"`
		Event          string `json:"eventId,omitempty"`
		Key            string `json:"idempotencyKey,omitempty"`
		Amount         string `json:"amount,omitempty"`
		Refund         string `json:"refundId,omitempty"`
		Reason         string `json:"reason,omitempty"`
		Rejection      string `json:"rejectionCode,omitempty"`
		Day            string `json:"businessDay,omitempty"`
		Asset          string `json:"asset,omitempty"`
		Gross          string `json:"gross,omitempty"`
		Returns        string `json:"refunds,omitempty"`
		Evidence       string `json:"evidenceRef,omitempty"`
		ReviewRevision int64  `json:"reviewRevision,omitempty"`
		escrowInput
	}
	limit := int64(4096)
	if r.PathValue("operation") == "escrow-claim" {
		limit = 8192 // 100 order references
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF || !financeRef.MatchString(in.Branch) {
		http.Error(w, "invalid finance command", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	if ctx.Err() != nil || !time.Now().Before(deadline) {
		http.NotFound(w, r)
		return
	}
	var output any
	var err error
	operation := r.PathValue("operation")
	if capabilities, ok := financeCapabilities[operation]; ok {
		ctx = authority.Scope(ctx, principal, in.Branch, capabilities...)
	}
	refundOperation := operation == "refund-queue" || operation == "refund-status" || operation == "refund-reject" || operation == "refund-submit" ||
		operation == "refund-reserve" || operation == "refund-approve"
	if refundOperation {
		if (h.OperatorOrigin != "" && r.Header.Get("Origin") != h.OperatorOrigin) || r.Header.Get("Content-Type") != "application/json" {
			http.NotFound(w, r)
			return
		}
		target, configured := h.Targets[in.Branch]
		if !configured {
			http.NotFound(w, r)
			return
		}
		if target.Escrow() && (operation == "refund-reserve" || operation == "refund-approve" || operation == "refund-submit") {
			h.refundBlocked(ctx, w, "REFUND_DESTINATION_HELD")
			return
		}
	}
	switch r.PathValue("operation") {
	case "escrow-orders", "escrow-claims", "escrow-claim", "escrow-resolve", "escrow-resolve-approve", "escrow-resolve-reject":
		if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || r.Header.Get("Content-Type") != "application/json" ||
			!h.Targets[in.Branch].Escrow() || (strings.HasPrefix(operation, "escrow-resolve") && h.Resolver == (common.Address{})) {
			http.NotFound(w, r)
			return
		}
		var valid bool
		output, valid, err = h.escrow(ctx, principal, operation, fp, in.Branch, in.Cursor, in.Sale, in.Key, in.Reason, in.escrowInput)
		if !valid {
			http.Error(w, "invalid escrow request", http.StatusBadRequest)
			return
		}
	case "report", "report-export", "order", "reconciliation", "report-close", "fulfillment-retry", "fulfillment-review":
		if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || r.Header.Get("Content-Type") != "application/json" {
			http.NotFound(w, r)
			return
		}
		target, configured := h.Targets[in.Branch]
		if !configured {
			http.NotFound(w, r)
			return
		}
		operation := r.PathValue("operation")
		if operation == "fulfillment-retry" {
			if !financeRef.MatchString(in.Event) || !financeRef.MatchString(in.Key) {
				http.Error(w, "invalid event scope", http.StatusBadRequest)
				return
			}
			var state string
			err = h.Store.db.QueryRow(ctx, "SELECT ledger.finance_retry_fulfillment($1,$2,$3,$4,$5)",
				principal.Tenant, in.Branch, principal.Subject, in.Key, in.Event).Scan(&state)
			output = map[string]string{"eventId": in.Event, "requestId": in.Key, "state": state}
			break
		}
		if operation == "order" {
			if !financeRef.MatchString(in.Sale) {
				http.Error(w, "invalid order reference", http.StatusBadRequest)
				return
			}
			var detail map[string]json.RawMessage
			detail, err = h.financeOrder(ctx, principal, in.Branch, in.Sale, target)
			output = withToken(detail, target)
			break
		}
		day, parseErr := time.Parse("2006-01-02", in.Day)
		mutation := operation == "report-export" || operation == "report-close"
		if parseErr != nil || (in.Asset != "" && in.Asset != lower(target.Asset)) ||
			(mutation && !financeRef.MatchString(in.Key)) || (!mutation && in.Key != "" && (operation != "fulfillment-review" || !financeRef.MatchString(in.Key))) {
			http.Error(w, "invalid report scope", http.StatusBadRequest)
			return
		}
		in.Asset = lower(target.Asset)
		switch operation {
		case "report", "report-export":
			var report dayReport
			report, err = h.report(ctx, principal.Tenant, in.Branch, principal.Subject, in.Asset, day, in.Key)
			if err == nil && operation == "report-export" {
				writeFinanceCSV(w, report)
				return
			}
			scope := sha256.Sum256([]byte("nu54-finance-console-v1:" + hex.EncodeToString(fp[:]) + ":" + in.Branch + ":" + in.Day))
			report.ReceiptScope = hex.EncodeToString(scope[:])
			report.Token = target.Token
			output = report
		case "fulfillment-review":
			if !financeRef.MatchString(in.Event) {
				http.Error(w, "invalid event reference", http.StatusBadRequest)
				return
			}
			var review map[string]any
			review, err = h.notificationReview(ctx, principal, in.Branch, in.Asset, in.Event, in.Key, day, target)
			output = withToken(review, target)
		case "reconciliation":
			var raw, receipts json.RawMessage
			err = h.Store.db.QueryRow(ctx, "SELECT ledger.finance_cases($1,$2,$3,$4,$5),ledger.finance_notification_history($1,$2,$3,$4,$5)",
				principal.Tenant, in.Branch, principal.Subject, day, in.Asset).Scan(&raw, &receipts)
			status := "held"
			summary := map[string]any{"events": raw, "receipts": receipts}
			if err == nil && h.Evidence != nil && h.Merchants[in.Branch] != "" {
				report, reportErr := h.report(ctx, principal.Tenant, in.Branch, principal.Subject, in.Asset, day, "")
				if reportErr != nil {
					err = reportErr
					break
				}
				_ = h.corroborateDay(ctx, target, report, func(proof verifiedDay) error {
					summary["anchor"] = proof.Anchor
					summary["height"] = proof.Height
					summary["chainGross"] = proof.Gross
					summary["chainRefunds"] = proof.Refunds
					if proof.matches(report) && report.OpenFulfillment == 0 && report.ActiveRefunds == 0 {
						status = "matched"
					}
					return nil
				})
			}
			summary["status"] = status
			output = summary
		case "report-close":
			var recorded *int64
			err = h.Store.db.QueryRow(ctx, "SELECT ledger.finance_close_status($1,$2,$3,$4,$5,$6)",
				principal.Tenant, in.Branch, principal.Subject, in.Key, day, in.Asset).Scan(&recorded)
			if err != nil {
				break
			}
			if recorded != nil {
				output = map[string]any{"revision": *recorded, "idempotencyKey": in.Key}
				break
			}
			if h.CloseStore == nil {
				http.Error(w, "closing evidence unavailable", http.StatusConflict)
				return
			}
			report, reportErr := h.report(ctx, principal.Tenant, in.Branch, principal.Subject, in.Asset, day, "")
			if reportErr != nil {
				err = reportErr
				break
			}
			var revision int64
			err = h.corroborateDay(ctx, target, report, func(proof verifiedDay) error {
				if !proof.matches(report) || report.OpenFulfillment != 0 || report.ActiveRefunds != 0 {
					return ErrEvidenceHeld
				}
				return h.CloseStore.db.QueryRow(ctx, `SELECT ledger.finance_close_verified($1,$2,$3,$4,$5,$6,$7::text::numeric,$8::text::numeric,$9,$10,$11)`,
					principal.Tenant, in.Branch, principal.Subject, in.Key, day, in.Asset, report.Gross, report.Refunds, proof.Digest, proof.Anchor, proof.Height).Scan(&revision)
			})
			if errors.Is(err, ErrEvidenceHeld) {
				http.Error(w, "closing evidence held", http.StatusConflict)
				return
			}
			output = map[string]any{"revision": revision, "idempotencyKey": in.Key}
		}
	case "deployment-profile":
		target, configured := h.Targets[in.Branch]
		if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin ||
			r.Header.Get("Content-Type") != "application/json" || !configured {
			http.NotFound(w, r)
			return
		}
		var allowed bool
		err = h.Store.db.QueryRow(ctx, `SELECT EXISTS (
			SELECT FROM jsonb_array_elements(ledger.operator_scopes($1,$2,'finance')) scope
			WHERE scope->>'id'=$3 AND (scope->'capabilities') ? 'reconcile_read')`,
			principal.Tenant, principal.Subject, in.Branch).Scan(&allowed)
		if err == nil {
			if !allowed {
				http.NotFound(w, r)
				return
			}
			output = h.deploymentReview(ctx, in.Branch, target)
		}
	case "payment-evidence":
		target, configured := h.Targets[in.Branch]
		if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin ||
			r.Header.Get("Content-Type") != "application/json" || !configured || !financeRef.MatchString(in.Sale) {
			http.NotFound(w, r)
			return
		}
		output, err = h.operatorPaymentEvidence(ctx, principal, in.Branch, in.Sale, target, false)
	case "refund-queue":
		var cursor *string
		if in.Cursor != "" {
			if !financeRef.MatchString(in.Cursor) {
				http.Error(w, "invalid refund cursor", http.StatusBadRequest)
				return
			}
			cursor = &in.Cursor
		}
		var data []byte
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.console_refund_queue($1,$2,$3,$4)",
			principal.Tenant, in.Branch, principal.Subject, cursor).Scan(&data)
		output = json.RawMessage(data)
	case "refund-status":
		if !financeRef.MatchString(in.Sale) {
			http.Error(w, "invalid original order", 400)
			return
		}
		status, statusErr := h.consoleRefund(ctx, principal, in.Branch, in.Sale)
		if statusErr != nil {
			err = statusErr
			break
		}
		status["destination"] = h.refundRoute(ctx, in.Branch, status)
		merchant, _ := status["merchant"].(string)
		committed, _ := status["fundsCommitted"].(string)
		delete(status, "merchant")
		delete(status, "payer")
		delete(status, "rentalWithdrawal")
		delete(status, "rentalPayer")
		delete(status, "payerGeneration")
		delete(status, "fundsCommitted")
		status["funds"] = "unknown"
		if h.Targets[in.Branch].Escrow() {
			// An escrow branch has no SettlementV2 relay, so nothing it records could be sent.
			status["canRequest"] = false
			if refunds, ok := status["refunds"].([]any); ok {
				for _, refund := range refunds {
					if row, ok := refund.(map[string]any); ok {
						row["canDecide"], row["canSubmit"] = false, false
					}
				}
			}
		}
		if status["canManageFunds"] == true {
			if available, err := h.currentRefundFunds(ctx, in.Branch, merchant); err == nil && available != nil {
				status["funds"] = "observed"
				status["fundsAvailable"] = unreservedRefundFunds(available, committed)
			}
		}
		receiptScope := sha256.Sum256([]byte("nu54-refund-console:" + hex.EncodeToString(fp[:]) + ":" + in.Branch + ":" + in.Sale))
		status["receiptScope"] = hex.EncodeToString(receiptScope[:])
		output = status
	case "refund-reject":
		if !financeRef.MatchString(in.Sale) || !financeRef.MatchString(in.Refund) || in.ReviewRevision < 0 || !refundRejectionAllowed(in.Rejection) {
			http.Error(w, "invalid refund decision", 400)
			return
		}
		_, err = h.Store.db.Exec(ctx, "SELECT ledger.reject_console_refund($1,$2,$3,$4,$5,$6,$7)",
			principal.Tenant, in.Branch, principal.Subject, in.Sale, in.Refund, in.Rejection, in.ReviewRevision)
		output = map[string]string{"refundId": in.Refund, "state": "cancelled", "rejectionCode": in.Rejection}
	case "refund-reserve":
		if !financeRef.MatchString(in.Sale) || !financeRef.MatchString(in.Key) || !validUnits(in.Amount, false) {
			http.Error(w, "invalid refund request", http.StatusBadRequest)
			return
		}
		var reason *string
		if in.Reason != "" {
			if in.Reason != "partial_service" && in.Reason != "merchant_cancel" && in.Reason != "duplicate_charge" {
				http.Error(w, "invalid refund reason", http.StatusBadRequest)
				return
			}
			reason = &in.Reason
		}
		var prior *string
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.replay_staff_refund($1,$2,$3,$4,$5,$6::text::numeric,$7)::text",
			principal.Tenant, in.Branch, principal.Subject, in.Sale, in.Key, in.Amount, reason).Scan(&prior)
		if err != nil {
			break
		}
		if prior != nil {
			output = map[string]string{"refundId": *prior}
			break
		}
		if reason == nil {
			http.Error(w, "refund reason required", http.StatusBadRequest)
			return
		}
		status, statusErr := h.consoleRefund(ctx, principal, in.Branch, in.Sale)
		if statusErr != nil {
			err = statusErr
			break
		}
		if h.refundRoute(ctx, in.Branch, status) == "rental-held" {
			h.refundBlocked(ctx, w, "REFUND_DESTINATION_HELD")
			return
		}
		var id string
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.reserve_console_refund($1,$2,$3,$4,$5,$6::text::numeric,$7)::text",
			principal.Tenant, in.Branch, principal.Subject, in.Sale, in.Key, in.Amount, in.Reason).Scan(&id)
		output = map[string]string{"refundId": id}
	case "refund-approve":
		if !financeRef.MatchString(in.Sale) || !financeRef.MatchString(in.Refund) || in.ReviewRevision < 0 {
			http.Error(w, "invalid refund approval", http.StatusBadRequest)
			return
		}
		{
			status, statusErr := h.consoleRefund(ctx, principal, in.Branch, in.Sale)
			if statusErr != nil {
				err = statusErr
				break
			}
			if h.refundRoute(ctx, in.Branch, status) == "rental-held" {
				h.refundBlocked(ctx, w, "REFUND_DESTINATION_HELD")
				return
			}
		}
		var changed bool
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.approve_refund($1,$2,$3,$4,$5,$6)", principal.Tenant, in.Branch,
			principal.Subject, in.Sale, in.Refund, in.ReviewRevision).Scan(&changed)
		output = map[string]string{"refundId": in.Refund, "state": "approved"}
	case "refund-submit":
		if h.Deployments[in.Branch].Status == "invalid" {
			http.NotFound(w, r)
			return
		}
		if !financeRef.MatchString(in.Sale) || !financeRef.MatchString(in.Refund) || in.ReviewRevision < 0 {
			http.Error(w, "invalid refund submission", http.StatusBadRequest)
			return
		}
		status, statusErr := h.consoleRefund(ctx, principal, in.Branch, in.Sale)
		if statusErr != nil {
			err = statusErr
			break
		}
		routeHeld := h.refundRoute(ctx, in.Branch, status) == "rental-held"
		merchant, _ := status["merchant"].(string)
		var units *string
		if !routeHeld {
			if available, probeErr := h.currentRefundFunds(ctx, in.Branch, merchant); probeErr == nil && available != nil && available.Sign() >= 0 {
				value := available.String()
				units = &value
			}
		}
		_, err = h.Store.db.Exec(ctx, "SELECT ledger.submit_console_refund($1,$2,$3,$4,$5,$6,$7::text::numeric,$8)",
			principal.Tenant, in.Branch, principal.Subject, in.Sale, in.Refund, merchant, units, in.ReviewRevision)
		var denied *pgconn.PgError
		if routeHeld && errors.As(err, &denied) && denied.Code == "P1032" {
			h.refundBlocked(ctx, w, "REFUND_DESTINATION_HELD")
			return
		}
		output = map[string]string{"refundId": in.Refund, "state": "submitted"}
	case "day-close":
		if h.OperatorOrigin != "" {
			http.NotFound(w, r)
			return
		}
		day, parseErr := time.Parse("2006-01-02", in.Day)
		if parseErr != nil || !financeAsset.MatchString(in.Asset) || !validUnits(in.Gross, true) || !validUnits(in.Returns, true) ||
			!financeRef.MatchString(in.Evidence) {
			http.Error(w, "invalid closing command", http.StatusBadRequest)
			return
		}
		var rev int64
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.close_business_day($1,$2,$3,$4,$5,$6::text::numeric,$7::text::numeric,$8)",
			principal.Tenant, in.Branch, principal.Subject, day, in.Asset, in.Gross, in.Returns, in.Evidence).Scan(&rev)
		gross, _ := new(big.Int).SetString(in.Gross, 10)
		refunds, _ := new(big.Int).SetString(in.Returns, 10)
		output = struct {
			Revision int64  `json:"revision"`
			Gross    string `json:"gross"`
			Refunds  string `json:"refunds"`
			Net      string `json:"net"`
		}{
			rev, in.Gross, in.Returns, gross.Sub(gross, refunds).String(),
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err == nil {
		err = h.Store.db.CheckRead(ctx)
	}
	if err != nil {
		var dbError *pgconn.PgError
		if refundOperation && errors.As(err, &dbError) {
			switch {
			case operation == "refund-reserve" && dbError.Code == "P1030":
				h.refundBlocked(ctx, w, "REFUND_LIMIT_CHANGED")
				return
			case operation == "refund-submit" && dbError.Code == "P1032":
				h.refundBlocked(ctx, w, "REFUND_FUNDS_HELD")
				return
			}
		}
		switch service.CodeOf(sqlError(err)) {
		case service.Forbidden:
			http.NotFound(w, r)
		case service.Conflict:
			http.Error(w, "finance conflict", http.StatusConflict)
		default:
			http.Error(w, "finance service unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(output)
}

func validUnits(value string, allowZero bool) bool {
	if len(value) == 0 || len(value) > 78 {
		return false
	}
	n, ok := new(big.Int).SetString(value, 10)
	return ok && n.String() == value && (n.Sign() > 0 || (allowZero && n.Sign() == 0)) && n.BitLen() <= 256
}
