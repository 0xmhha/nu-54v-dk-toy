package settlement

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"io"
	"net/http"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
)

type PrintFrame struct {
	AttemptID  string `json:"attemptId"`
	ReceiptRef string `json:"receiptRef"`
	BranchID   string `json:"branchId"`
	PickupNo   int64  `json:"pickupNo"`
}

type PrintAck struct {
	AttemptID  string `json:"attemptId"`
	ReceiptRef string `json:"receiptRef"`
	Status     string `json:"status"`
}

type PrintDevice interface {
	Print(context.Context, PrintFrame) (PrintAck, error)
}

type Printer struct {
	Store    *Store
	Evidence deliveryEvidence
	Device   PrintDevice
}

func (p Printer) Step(ctx context.Context) (bool, error) {
	var frame PrintFrame
	var stream, merchant string
	err := p.Store.db.QueryRow(ctx, "SELECT attempt_id::text,stream_id::text,merchant_key FROM ledger.next_print()").
		Scan(&frame.AttemptID, &stream, &merchant)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var admitted string
	err = p.Evidence.deliveryEpoch(ctx, stream, merchant, func(ctx context.Context, epoch string) error {
		admitted = epoch
		return p.Store.db.QueryRow(ctx, "SELECT receipt_ref::text,branch_id::text,pickup_no FROM ledger.begin_print($1,$2)",
			frame.AttemptID, epoch).Scan(&frame.ReceiptRef, &frame.BranchID, &frame.PickupNo)
	})
	if err != nil {
		_, delayErr := p.Store.db.Exec(ctx, "SELECT ledger.delay_print($1)", frame.AttemptID)
		return true, errors.Join(err, delayErr)
	}
	ack, err := p.Device.Print(ctx, frame)
	if err != nil || ack.AttemptID != frame.AttemptID || ack.ReceiptRef != frame.ReceiptRef || ack.Status != "printed" {
		return true, errors.New("printer outcome unknown")
	}
	err = p.Evidence.deliveryEpoch(ctx, stream, merchant, func(ctx context.Context, epoch string) error {
		if epoch != admitted {
			return ErrEvidenceHeld
		}
		_, err := p.Store.db.Exec(ctx, "SELECT ledger.complete_print($1,$2)", frame.AttemptID, epoch)
		return err
	})
	return true, err
}

type PrintStaffHTTP struct {
	Store       *Store
	Evidence    deliveryEvidence
	Credentials map[string]service.Principal
}

func (h PrintStaffHTTP) Register(mux *http.ServeMux) {
	mux.Handle("GET /v2/fulfillment/print-attempts", authority.Protect(h.Credentials, http.HandlerFunc(h.serve)))
	mux.Handle("POST /v2/fulfillment/print-decisions", authority.Protect(h.Credentials, http.HandlerFunc(h.serve)))
}

func (h PrintStaffHTTP) serve(w http.ResponseWriter, r *http.Request) {
	principal, client, ok := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	deadline := client.ExpiresAt
	if !ok || principal.Role != "printer_staff" || !time.Now().Before(deadline) {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	if r.Method == http.MethodGet {
		h.attempts(ctx, w, r, principal)
		return
	}
	var decision struct {
		Branch  string `json:"branchId"`
		Attempt string `json:"attemptId"`
		Key     string `json:"idempotencyKey"`
		Action  string `json:"action"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decision) != nil || decoder.Decode(new(any)) != io.EOF || !financeRef.MatchString(decision.Branch) ||
		!financeRef.MatchString(decision.Attempt) || !financeRef.MatchString(decision.Key) ||
		(decision.Action != "confirm_printed" && decision.Action != "reprint") {
		http.Error(w, "invalid print decision", http.StatusBadRequest)
		return
	}
	ctx = authority.Scope(ctx, principal, decision.Branch)
	var stream, merchant, result string
	err := h.Store.db.QueryRow(ctx, "SELECT stream_id::text,merchant_key FROM ledger.print_staff_source($1,$2,$3,$4)",
		principal.Tenant, decision.Branch, principal.Subject, decision.Attempt).Scan(&stream, &merchant)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	err = h.Evidence.deliveryEpoch(ctx, stream, merchant, func(ctx context.Context, epoch string) error {
		return h.Store.db.QueryRow(ctx, "SELECT ledger.resolve_print($1,$2,$3,$4,$5,$6,$7)::text",
			principal.Tenant, decision.Branch, principal.Subject, decision.Attempt, decision.Key, decision.Action, epoch).Scan(&result)
	})
	if err != nil {
		if service.CodeOf(sqlError(err)) == service.Conflict {
			http.Error(w, "print decision conflicts with recorded status", http.StatusConflict)
		} else if errors.Is(err, ErrEvidenceHeld) {
			http.Error(w, "print decision held", http.StatusServiceUnavailable)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	if err := h.Store.db.CheckRead(ctx); err != nil {
		http.NotFound(w, r)
		return
	}
	writePrintJSON(w, map[string]string{"attemptId": result})
}

func (h PrintStaffHTTP) attempts(ctx context.Context, w http.ResponseWriter, r *http.Request, principal service.Principal) {
	query := r.URL.Query()
	branch, receipt, sale := query.Get("branchId"), query.Get("receiptRef"), query.Get("saleRef")
	if !financeRef.MatchString(branch) || (receipt != "" && (!financeRef.MatchString(receipt) || sale != "")) ||
		(sale != "" && !financeRef.MatchString(sale)) {
		http.NotFound(w, r)
		return
	}
	ctx = authority.Scope(ctx, principal, branch)
	var data json.RawMessage
	var err error
	if receipt != "" {
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.print_staff_attempt_view($1,$2,$3,$4)",
			principal.Tenant, branch, principal.Subject, receipt).Scan(&data)
	} else {
		var saleRef *string
		if sale != "" {
			saleRef = &sale
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.print_staff_queue($1,$2,$3,$4)",
			principal.Tenant, branch, principal.Subject, saleRef).Scan(&data)
	}
	if err != nil || (receipt != "" && string(data) == "[]") || h.Store.db.CheckRead(ctx) != nil {
		http.NotFound(w, r)
		return
	}
	writePrintJSON(w, data)
}

func writePrintJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}
