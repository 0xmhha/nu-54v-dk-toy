package commerce

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"github.com/jackc/pgx/v5"
)

var externalPOSReference = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$`)

type posCommand struct {
	Operation string `json:"operation"`
	Branch    string `json:"branch"`
	Reference string `json:"reference,omitempty"`
	Handoff   string `json:"handoff,omitempty"`
	ChainID   uint64 `json:"chainId,omitempty"`
	Asset     string `json:"asset,omitempty"`
	Amount    string `json:"amount,omitempty"`
}

type posOrder struct {
	Branch        string `json:"branch"`
	BranchLabel   string `json:"branchLabel"`
	Reference     string `json:"reference"`
	Handoff       string `json:"handoff"`
	ChainID       uint64 `json:"chainId"`
	Bound         bool   `json:"payerBound"`
	SelectedCode  string `json:"selectedCode,omitempty"`
	SelectionHeld bool   `json:"selectionHeld,omitempty"`
	Order         Order  `json:"order"`
}

func posError(w http.ResponseWriter, r *http.Request, err error) {
	switch code := service.CodeOf(domainError(err)); code {
	case service.Forbidden:
		http.NotFound(w, r)
	case service.Conflict:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "POS_REFERENCE_CONFLICT"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "POS_RESULT_UNAVAILABLE"})
	}
}

func (h HTTP) pos(w http.ResponseWriter, r *http.Request, actor service.Principal, data []byte) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var in posCommand
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || !operatorRef.MatchString(in.Branch) {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	if in.Operation == "submit" || in.Operation == "status" {
		if actor.Role != "pos" || in.Handoff != "" || !slices.ContainsFunc(actor.Grants, func(grant service.Grant) bool {
			return grant.Domain == in.Branch && grant.Object == in.Branch && slices.Contains(grant.Operations, "POS/submit")
		}) {
			http.NotFound(w, r)
			return
		}
		ctx := authority.Scope(r.Context(), actor, in.Branch)
		if in.Operation == "status" {
			if !externalPOSReference.MatchString(in.Reference) || in.ChainID != 0 || in.Asset != "" || in.Amount != "" || h.Store == nil {
				http.NotFound(w, r)
				return
			}
			var terminal, handoff string
			err := h.Store.db.QueryRow(ctx, "SELECT terminal_ref::text,handoff_ref::text FROM ledger.pos_source_order($1,$2,$3,$4)",
				actor.Tenant, in.Branch, actor.Subject, in.Reference).Scan(&terminal, &handoff)
			if errors.Is(err, pgx.ErrNoRows) {
				err = service.Forbidden
			}
			if err != nil {
				posError(w, r, err)
				return
			}
			// The enrollment owns this reference. The original terminal's
			// read scope remains authoritative for settled receipt details.
			owner := service.Principal{Tenant: actor.Tenant, Subject: terminal, Role: "terminal"}
			out, err := h.Store.posStatus(ctx, owner, in.Branch, handoff)
			if err != nil {
				posError(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
		amount, valid := new(big.Int).SetString(in.Amount, 10)
		if !externalPOSReference.MatchString(in.Reference) || !valid || amount.Sign() <= 0 ||
			amount.BitLen() > 256 || amount.String() != in.Amount {
			errorResponse(w, service.BadRequest, nil)
			return
		}
		profile, ok := h.Profiles[in.Branch]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if in.ChainID != 8283 || in.ChainID != profile.Chain || in.Asset != strings.ToLower(profile.Asset.Hex()) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "POS_ASSET_UNSUPPORTED"})
			return
		}
		if h.paused(in.Branch) {
			errorResponse(w, service.Capability, nil)
			return
		}
		if !profile.Token.wholePrice(in.Amount) {
			errorResponse(w, service.BadRequest, nil)
			return
		}
		if h.Store == nil {
			posError(w, r, service.Unknown)
			return
		}
		var handoff string
		err := h.Store.db.QueryRow(ctx, "SELECT ledger.submit_pos_order($1,$2,$3,$4,$5,$6::text::numeric)::text",
			actor.Tenant, in.Branch, actor.Subject, in.Reference, in.Asset, in.Amount).Scan(&handoff)
		if err != nil {
			posError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"branch": in.Branch, "reference": in.Reference, "handoff": handoff})
		return
	}
	if in.Operation != "lookup" || actor.Role != "terminal" || !operatorRef.MatchString(in.Handoff) ||
		in.Reference != "" || in.ChainID != 0 || in.Asset != "" || in.Amount != "" || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	ctx := authority.Scope(r.Context(), actor, in.Branch)
	out, err := h.Store.posStatus(ctx, actor, in.Branch, in.Handoff)
	if err != nil {
		posError(w, r, err)
		return
	}
	profile, ok := h.Profiles[in.Branch]
	if !ok || profile.Chain != 8283 || strings.ToLower(profile.Asset.Hex()) != out.Order.Asset {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "POS_ASSET_UNSUPPORTED"})
		return
	}
	if out.Bound && (out.Order.State == "preview" || out.Order.State == "issued") {
		var marker *string
		err := h.Store.db.QueryRow(ctx, "SELECT ledger.pos_selected_marker($1,$2,$3,$4),ledger.pos_selection_held($1,$2,$3,$4)",
			actor.Tenant, in.Branch, actor.Subject, in.Handoff).Scan(&marker, &out.SelectionHeld)
		if err != nil {
			posError(w, r, err)
			return
		}
		if marker != nil {
			out.SelectedCode = *marker
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Store) posStatus(ctx context.Context, actor service.Principal, branch, handoff string) (posOrder, error) {
	out := posOrder{Branch: branch, Handoff: handoff, ChainID: 8283}
	var sale string
	err := s.db.QueryRow(ctx, "SELECT sale_ref::text,external_ref,branch_label,payer_bound FROM ledger.lookup_pos_order($1,$2,$3,$4)",
		actor.Tenant, branch, actor.Subject, handoff).Scan(&sale, &out.Reference, &out.BranchLabel, &out.Bound)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, service.Forbidden
	}
	if err != nil {
		return out, err
	}
	out.Order, err = s.Status(ctx, service.Bound{Tenant: actor.Tenant, Subject: actor.Subject,
		Request: service.Request{Domain: branch, Object: sale}})
	return out, err
}
