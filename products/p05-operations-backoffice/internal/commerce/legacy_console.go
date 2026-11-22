package commerce

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type LegacyConsole interface {
	View(context.Context, string) (core.LegacyLedger, error)
	Preflight(context.Context, string) (core.LegacyLedger, error)
	Prepare(context.Context, string) ([]byte, error)
	Observe(context.Context, string, []byte, bool) (core.LegacyCashOutObservation, error)
}

type legacyReview struct {
	Reference string `json:"reference"`
	Version   string `json:"version"`
	Contract  string `json:"contract"`
	Asset     string `json:"asset"`
	Payout    string `json:"payout"`
	Balance   string `json:"balance"`
}

func (r legacyReview) matches(ledger core.LegacyLedger) bool {
	return r.Version == "v1" && strings.EqualFold(ledger.Contract.Hex(), r.Contract) &&
		strings.EqualFold(ledger.Asset.Hex(), r.Asset) && strings.EqualFold(ledger.Payout.Hex(), r.Payout) &&
		ledger.Balance == r.Balance
}

func (r legacyReview) compare(proof core.LegacyCashOutObservation) core.LegacyCashOutObservation {
	if (proof.Status == "verified" || proof.Status == "transfer-held") &&
		(proof.Amount != r.Balance || !strings.EqualFold(proof.Payout, r.Payout)) {
		proof.Status = "different-execution"
	}
	return proof
}

func (h HTTP) registerLegacy(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/operations/legacy/{branch}", h.operateLegacy)
	mux.HandleFunc("POST /v2/operations/legacy/{branch}/{action}", h.operateLegacy)
}

func (h HTTP) operateLegacy(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.operatorCredential(r)
	branch := r.PathValue("branch")
	if !ok || h.Store == nil || h.Legacy == nil || !operatorRef.MatchString(branch) {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	switch r.PathValue("action") {
	case "", "cashout", "resume":
	default:
		http.NotFound(w, r)
		return
	}
	commit, resume := r.PathValue("action") == "cashout", r.PathValue("action") == "resume"
	if (commit || resume) && (h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || r.Header.Get("Content-Type") != "application/json") {
		http.NotFound(w, r)
		return
	}
	var authorized, canResume bool
	if err := h.Store.db.QueryRow(ctx, "SELECT ledger.legacy_cashout_scope($1,$2,$3,$4),ledger.can_resume_legacy_cashout($1,$2,$3)", actor.Tenant, branch, actor.Subject, commit).Scan(&authorized, &canResume); err != nil {
		operatorResponse(w, nil, err)
		return
	}
	view, err := h.Legacy.View(ctx, branch)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	out := map[string]any{"ledger": view, "canWithdraw": authorized, "canResume": canResume, "version": "v1"}
	key := r.URL.Query().Get("reference")
	if key != "" && !operatorRef.MatchString(key) {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	var reviewed legacyReview
	if commit || resume {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&reviewed) != nil || decoder.Decode(new(any)) != io.EOF || !operatorRef.MatchString(reviewed.Reference) {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		key = reviewed.Reference
		if resume {
			if reviewed != (legacyReview{Reference: key}) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			if _, err := h.Store.db.Exec(ctx, "SELECT ledger.resume_legacy_cashout($1,$2,$3,$4)", actor.Tenant, branch, actor.Subject, key); err != nil {
				operatorResponse(w, nil, err)
				return
			}
		} else if reviewed.Version != "v1" || !strings.EqualFold(view.Contract.Hex(), reviewed.Contract) ||
			!strings.EqualFold(view.Asset.Hex(), reviewed.Asset) || !catalogAsset.MatchString(strings.ToLower(reviewed.Payout)) ||
			!catalogUnits.MatchString(reviewed.Balance) {
			operatorResponse(w, nil, service.Conflict)
			return
		}
	}
	var reference *string
	if key != "" {
		reference = &key
	}
	var candidate, stored, archive []byte
	err = h.Store.db.QueryRow(ctx, "SELECT * FROM ledger.find_legacy_cashout($1,$2,$3,$4)",
		actor.Tenant, branch, actor.Subject, reference).Scan(&candidate, &stored, &archive)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		operatorResponse(w, nil, err)
		return
	}
	var original legacyReview
	if len(archive) != 0 && json.Unmarshal(archive, &original) != nil {
		operatorResponse(w, nil, service.Unknown)
		return
	}
	if resume && len(candidate) == 0 {
		operatorResponse(w, nil, service.Conflict)
		return
	}
	if commit {
		if len(candidate) == 0 && len(stored) == 0 && !reviewed.matches(view) {
			operatorResponse(w, nil, service.Conflict)
			return
		}
		wire, _ := json.Marshal(reviewed)
		intent := sha256.Sum256(append([]byte(view.Merchant.Hex()), wire...))
		if err = h.Store.db.QueryRow(ctx, "SELECT * FROM ledger.open_legacy_cashout($1,$2,$3,$4,$5,$6)",
			actor.Tenant, branch, actor.Subject, key, intent[:], wire).Scan(&candidate, &stored); err != nil {
			var conflict *pgconn.PgError
			if errors.As(err, &conflict) && conflict.Code == "23505" &&
				conflict.ConstraintName == "legacy_cashout_signer_slot" {
				err = service.Busy
			}
			operatorResponse(w, nil, err)
			return
		}
		original = reviewed
		if len(candidate) == 0 && len(stored) == 0 {
			preflight, err := h.Legacy.Preflight(ctx, branch)
			if err != nil || !original.matches(preflight) {
				operatorResponse(w, nil, service.Conflict)
				return
			}
			prepared, err := h.Legacy.Prepare(ctx, branch)
			if err != nil {
				operatorResponse(w, nil, service.Unknown)
				return
			}
			if err = h.Store.db.QueryRow(ctx, "SELECT ledger.save_legacy_cashout($1,$2,$3,$4,$5)",
				actor.Tenant, branch, actor.Subject, key, prepared).Scan(&candidate); err != nil {
				operatorResponse(w, nil, err)
				return
			}
		}
	}
	if original.Reference != "" {
		out["review"] = original
	}
	var persisted core.LegacyCashOutObservation
	if len(stored) != 0 {
		if err := json.Unmarshal(stored, &persisted); err != nil {
			operatorResponse(w, nil, service.Unknown)
			return
		}
		out["outcome"] = json.RawMessage(stored)
	}
	if len(candidate) == 0 || (len(stored) != 0 && persisted.Status != "transfer-held") {
		writeJSON(w, http.StatusOK, out)
		return
	}

	observation, observeErr := h.Legacy.Observe(ctx, branch, candidate, false)
	if observeErr != nil && observation.Hash == "" {
		operatorResponse(w, nil, observeErr)
		return
	}
	if (commit || resume) && observeErr == nil && observation.Status == "pending" {
		preflight, err := h.Legacy.Preflight(ctx, branch)
		if err != nil {
			operatorResponse(w, nil, service.Unknown)
			return
		}
		matched := original.matches(preflight)
		if !matched {
			observation.Status = "review-mismatch"
		}
		wire, _ := json.Marshal(observation)
		var dispatch bool
		if err := h.Store.db.QueryRow(ctx, "SELECT ledger.dispatch_legacy_cashout($1,$2,$3,$4,$5,$6)",
			actor.Tenant, branch, actor.Subject, key, matched, wire).Scan(&dispatch); err != nil {
			operatorResponse(w, nil, err)
			return
		}
		if dispatch {
			observation, observeErr = h.Legacy.Observe(ctx, branch, candidate, true)
		}
	}
	observation = original.compare(observation)
	switch observation.Status {
	case "verified", "different-execution", "reverted", "no-withdrawal", "nonce-superseded", "transfer-held":
		if (commit || resume) && observeErr == nil {
			wire, _ := json.Marshal(observation)
			if _, err := h.Store.db.Exec(ctx, "SELECT ledger.finish_legacy_cashout($1,$2,$3,$4,$5)",
				actor.Tenant, branch, actor.Subject, key, wire); err != nil {
				operatorResponse(w, nil, err)
				return
			}
		}
	}
	out["outcome"] = observation
	writeJSON(w, http.StatusOK, out)
}
