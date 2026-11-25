package settlement

import (
	"cmp"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/jackc/pgx/v5"
)

type dayReport struct {
	ReceiptScope    string    `json:"receiptScope,omitempty"`
	Branch          string    `json:"branch"`
	Day             string    `json:"day"`
	Asset           string    `json:"asset"`
	Zone            string    `json:"zone"`
	From            time.Time `json:"from"`
	Until           time.Time `json:"until"`
	Gross           string    `json:"gross"`
	Refunds         string    `json:"refunds"`
	Net             string    `json:"net"`
	Digest          string    `json:"digest"`
	Postings        int64     `json:"postings"`
	ActiveRefunds   int64     `json:"activeRefunds"`
	OpenFulfillment int64     `json:"openFulfillment"`
	CloseRevision   *int64    `json:"closeRevision"`
	// Deposits that entered and left the escrow that day. Present only for a day that has any.
	EscrowLocked   string `json:"escrowLocked,omitempty"`
	EscrowReturned string `json:"escrowReturned,omitempty"`
	// Present only for a branch that names its token.
	Token commerce.Token `json:"token,omitzero"`
}

func (h FinanceHTTP) report(ctx context.Context, tenant, branch, actor, asset string, day time.Time, key string) (dayReport, error) {
	var report dayReport
	var raw []byte
	var err error
	if key == "" {
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.finance_day_report($1,$2,$3,$4,$5,false)", tenant, branch, actor, day, asset).Scan(&raw)
	} else {
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.finance_export_day($1,$2,$3,$4,$5,$6)", tenant, branch, actor, key, day, asset).Scan(&raw)
	}
	if err == nil {
		err = json.Unmarshal(raw, &report)
	}
	return report, err
}

func writeFinanceCSV(w http.ResponseWriter, report dayReport) {
	revision := ""
	if report.CloseRevision != nil {
		revision = strconv.FormatInt(*report.CloseRevision, 10)
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=settlement-"+report.Day+".csv")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writer := csv.NewWriter(w)
	header := []string{"branch", "business_day", "zone", "asset", "gross_units", "refund_units", "net_units", "postings", "close_revision"}
	row := []string{report.Branch, report.Day, report.Zone, report.Asset, report.Gross, report.Refunds,
		report.Net, strconv.FormatInt(report.Postings, 10), revision}
	if report.EscrowLocked != "" {
		header, row = append(header, "escrow_locked_units", "escrow_returned_units"), append(row, report.EscrowLocked, report.EscrowReturned)
	}
	_ = writer.Write(header)
	_ = writer.Write(row)
	writer.Flush()
}

type verifiedDay struct {
	Anchor  string `json:"anchor"`
	Height  int64  `json:"height"`
	Gross   string `json:"gross"`
	Refunds string `json:"refunds"`
	Count   int64  `json:"count"`
	Digest  string `json:"digest"`
	// An escrow day reports these instead of gross and refunds.
	Locked    *string `json:"locked,omitempty"`
	Released  *string `json:"released,omitempty"`
	Cancelled *string `json:"cancelled,omitempty"`
}

func (proof verifiedDay) matches(report dayReport) bool {
	if proof.Released != nil && proof.Locked != nil && proof.Cancelled != nil {
		return proof.Anchor != "" && proof.Digest == report.Digest && proof.Count == report.Postings &&
			*proof.Released == report.Gross && report.Refunds == "0" &&
			*proof.Locked == cmp.Or(report.EscrowLocked, "0") && *proof.Cancelled == cmp.Or(report.EscrowReturned, "0")
	}
	return proof.Anchor != "" && proof.Digest == report.Digest && proof.Count == report.Postings &&
		proof.Gross == report.Gross && proof.Refunds == report.Refunds
}

func (h FinanceHTTP) corroborateDay(ctx context.Context, target Target, report dayReport, verify func(verifiedDay) error) error {
	merchant := h.Merchants[report.Branch]
	if h.Evidence == nil || merchant == "" {
		return ErrEvidenceHeld
	}
	return h.Evidence.lockScope(ctx, target.Stream, merchant, func(tx pgx.Tx) error {
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT chainview.finance_day_evidence($1,$2,$3,$4,$5,'8283',$6,$7,$8)`,
			target.Stream, merchant, report.Asset, report.From, report.Until, lower(target.Contract), target.ABIHash(), target.Manifest).Scan(&raw)
		if err != nil || len(raw) == 0 {
			return ErrEvidenceHeld
		}
		var proof verifiedDay
		if json.Unmarshal(raw, &proof) != nil || proof.Anchor == "" || (proof.Released != nil) != target.Escrow() {
			return ErrEvidenceHeld
		}
		if proof.Released != nil {
			proof.Gross, proof.Refunds = *proof.Released, "0"
		}
		return verify(proof)
	})
}
