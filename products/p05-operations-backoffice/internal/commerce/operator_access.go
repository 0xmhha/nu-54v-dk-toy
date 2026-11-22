package commerce

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

var accessReviewRef = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (h HTTP) registerAccess(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/operations/access/{branch}", h.operateAccess)
	mux.HandleFunc("GET /v2/operations/access/{branch}/handoffs", h.operateAccessHandoffs)
	mux.HandleFunc("POST /v2/operations/access/{branch}/{action}", h.operateAccess)
	mux.HandleFunc("GET /v2/operations/audit/{branch}", h.operateAudit)
}

func (h HTTP) operateAudit(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.operatorCredential(r)
	branch := r.PathValue("branch")
	if !ok || h.Store == nil || !operatorRef.MatchString(branch) {
		http.NotFound(w, r)
		return
	}
	first, firstErr := time.Parse("2006-01-02", r.URL.Query().Get("from"))
	last, lastErr := time.Parse("2006-01-02", r.URL.Query().Get("until"))
	if firstErr != nil || lastErr != nil || last.Before(first) {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	var result json.RawMessage
	err := h.Store.db.QueryRow(r.Context(), "SELECT ledger.operator_audit($1,$2,$3,$4,$5)",
		actor.Tenant, branch, actor.Subject, first, last).Scan(&result)
	operatorResponse(w, result, err)
}

func (h HTTP) operateAccessHandoffs(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.operatorCredential(r)
	branch := r.PathValue("branch")
	if !ok || h.Store == nil || !operatorRef.MatchString(branch) {
		http.NotFound(w, r)
		return
	}
	var raw json.RawMessage
	err := h.Store.db.QueryRow(r.Context(), "SELECT ledger.operator_access_handoffs($1,$2,$3)",
		actor.Tenant, branch, actor.Subject).Scan(&raw)
	operatorResponse(w, raw, err)
}

func (h HTTP) operateAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.operatorCredential(r)
	branch := r.PathValue("branch")
	if !ok || h.Store == nil || !operatorRef.MatchString(branch) {
		http.NotFound(w, r)
		return
	}
	var raw json.RawMessage
	ctx := r.Context()
	if r.Method == http.MethodGet {
		err := h.Store.db.QueryRow(ctx, "SELECT ledger.operator_access_status($1,$2,$3)", actor.Tenant, branch, actor.Subject).Scan(&raw)
		operatorResponse(w, raw, err)
		return
	}
	if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || r.Header.Get("Content-Type") != "application/json" {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Key       string `json:"requestKey"`
		Target    string `json:"target"`
		Role      string `json:"role"`
		Operation string `json:"operation"`
		Label     string `json:"label"`
		HandoffTo string `json:"handoffTo"`
		Review    string `json:"reviewKey"`
		Sale      string `json:"saleId"`
	}
	reader := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	reader.DisallowUnknownFields()
	if reader.Decode(&body) != nil || reader.Decode(new(any)) != io.EOF {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	if (body.HandoffTo != "" && !operatorRef.MatchString(body.HandoffTo)) ||
		(body.Review != "" && !accessReviewRef.MatchString(body.Review)) {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	if body.HandoffTo != "" && !h.enrollsAccessTarget(actor.Tenant, branch, body.HandoffTo, "operator") {
		http.NotFound(w, r)
		return
	}
	var err error
	switch r.PathValue("action") {
	case "name":
		if body.Label == "" || body.Key != "" || body.Target != "" || body.Role != "" || body.Operation != "" ||
			body.HandoffTo != "" || body.Review != "" || body.Sale != "" {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.name_operator_branch($1,$2,$3,$4)",
			actor.Tenant, branch, actor.Subject, body.Label).Scan(&raw)
	case "revoke-review":
		if !operatorRef.MatchString(body.Target) || (body.Role != "terminal" && body.Role != "operator") ||
			body.Key != "" || body.Operation != "" || body.Label != "" || body.Review != "" || body.Sale != "" {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.operator_access_revoke_review($1,$2,$3,$4,$5,NULLIF($6,'')::uuid)",
			actor.Tenant, branch, actor.Subject, body.Target, body.Role, body.HandoffTo).Scan(&raw)
	case "propose":
		if !operatorRef.MatchString(body.Key) || !operatorRef.MatchString(body.Target) || (body.Role != "terminal" && body.Role != "operator") ||
			(body.Operation != "register" && body.Operation != "revoke") || body.Label != "" || body.Sale != "" ||
			(body.Operation == "register" && (body.HandoffTo != "" || body.Review != "")) {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		if body.Operation == "register" && !h.enrollsAccessTarget(actor.Tenant, branch, body.Target, body.Role) {
			http.NotFound(w, r)
			return
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.propose_operator_access($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,NULLIF($9,''))",
			actor.Tenant, branch, actor.Subject, body.Key, body.Target, body.Role, body.Operation, body.HandoffTo, body.Review).Scan(&raw)
	case "approve":
		if !operatorRef.MatchString(body.Key) || body.Label != "" || body.Target != "" || body.Role != "" || body.Operation != "" ||
			body.HandoffTo != "" || body.Review != "" || body.Sale != "" {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.apply_operator_access($1,$2,$3,$4)",
			actor.Tenant, branch, actor.Subject, body.Key).Scan(&raw)
	case "handoff-ack":
		if !operatorRef.MatchString(body.Key) || !operatorRef.MatchString(body.Sale) || body.Label != "" ||
			body.Target != "" || body.Role != "" || body.Operation != "" || body.HandoffTo != "" || body.Review != "" {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.accept_operator_access_handoff($1,$2,$3,$4,$5)",
			actor.Tenant, branch, actor.Subject, body.Key, body.Sale).Scan(&raw)
	default:
		http.NotFound(w, r)
		return
	}
	operatorResponse(w, raw, err)
}

func (h HTTP) enrollsAccessTarget(tenant, branch, target, role string) bool {
	for _, device := range h.Credentials {
		if device.Tenant != tenant || device.Subject != target || device.Role != role || device.ExpiresAt <= time.Now().Unix() {
			continue
		}
		if role == "operator" {
			return true
		}
		for _, permit := range device.Grants {
			if permit.Domain == branch {
				return true
			}
		}
	}
	return false
}
