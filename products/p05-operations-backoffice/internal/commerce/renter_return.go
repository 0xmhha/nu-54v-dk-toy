package commerce

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"github.com/ethereum/go-ethereum/common"
)

var renterAssetSymbol = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,11}$`)

var renterAccount = regexp.MustCompile(`^0x[0-9a-f]{40}$`)

func (h HTTP) registerRenterStatus(mux, operators *http.ServeMux) {
	renter := http.NewServeMux()
	renter.HandleFunc("GET /v2/renter/returns/{assignment}", h.renterReturn)
	renter.HandleFunc("POST /v2/renter/returns/{assignment}/accept", h.renterReturn)
	mux.Handle("/v2/renter/returns/", authority.Protect(h.Credentials, renter))
	operators.HandleFunc("POST /v2/operations/renter-handover/{branch}/{device}", h.offerRenterReturn)
	operators.HandleFunc("POST /v2/operations/renter-handover/{assignment}/revoke", h.revokeRenterReturn)
}

func renterBody(w http.ResponseWriter, r *http.Request, body any) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	return decoder.Decode(body) == nil && decoder.Decode(new(any)) == io.EOF
}

func (h HTTP) offerRenterReturn(w http.ResponseWriter, r *http.Request) {
	actor, valid := h.operatorCredential(r)
	branch, device := r.PathValue("branch"), r.PathValue("device")
	if !valid || h.Store == nil || h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin ||
		!operatorRef.MatchString(branch) || !operatorRef.MatchString(device) {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Customer      string    `json:"customer"`
		Account       string    `json:"account"`
		Generation    string    `json:"generation"`
		Until         time.Time `json:"assignmentUntil"`
		PaymentEscrow bool      `json:"paymentEscrow,omitempty"`
	}
	if !renterBody(w, r, &input) || !operatorRef.MatchString(input.Customer) || !renterAccount.MatchString(input.Account) {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	gen, err := strconv.ParseInt(input.Generation, 10, 64)
	if err != nil || gen <= 0 {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	var assignment string
	if input.PaymentEscrow {
		if h.RentalEscrow == nil {
			operatorResponse(w, nil, service.Capability)
			return
		}
		manifest, available := h.RentalEscrow.Deployment(r.Context(), branch)
		if !available {
			operatorResponse(w, nil, service.Capability)
			return
		}
		err = h.Store.db.QueryRow(r.Context(), `SELECT ledger.offer_renter_handover($1,$2,$3,$4,$5,$6,$7,$8,$9)::text`,
			actor.Tenant, branch, actor.Subject, device, gen, input.Customer, input.Account, input.Until, manifest).Scan(&assignment)
	} else {
		err = h.Store.db.QueryRow(r.Context(), `SELECT ledger.offer_renter_return($1,$2,$3,$4,$5,$6,$7,$8)::text`,
			actor.Tenant, branch, actor.Subject, device, gen, input.Customer, input.Account, input.Until).Scan(&assignment)
	}
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"assignment": assignment, "account": input.Account, "generation": input.Generation})
}

func (h HTTP) revokeRenterReturn(w http.ResponseWriter, r *http.Request) {
	actor, valid := h.operatorCredential(r)
	assignment := r.PathValue("assignment")
	if !valid || h.Store == nil || h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || !operatorRef.MatchString(assignment) {
		http.NotFound(w, r)
		return
	}
	var changed bool
	err := h.Store.db.QueryRow(r.Context(), `SELECT ledger.change_renter_return($1,$2,$3,'revoke','')`,
		actor.Tenant, actor.Subject, assignment).Scan(&changed)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h HTTP) renterReturn(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, customerClient, valid := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	assignment := r.PathValue("assignment")
	if !valid || h.Store == nil || actor.Role != "customer" || !operatorRef.MatchString(actor.Tenant) ||
		!operatorRef.MatchString(actor.Subject) || !operatorRef.MatchString(assignment) {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		var input struct {
			Account string `json:"account"`
		}
		if !renterBody(w, r, &input) || !renterAccount.MatchString(input.Account) {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		var changed bool
		err := h.Store.db.QueryRow(r.Context(), `SELECT ledger.change_renter_return($1,$2,$3,'accept',$4)`,
			actor.Tenant, actor.Subject, assignment, input.Account).Scan(&changed)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		if !changed {
			http.NotFound(w, r)
			return
		}
	}
	var raw json.RawMessage
	err := h.Store.db.QueryRow(r.Context(), `SELECT ledger.read_renter_return($1,$2,$3)`, actor.Tenant, actor.Subject, assignment).Scan(&raw)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if len(raw) == 0 {
		http.NotFound(w, r)
		return
	}
	var credential *renterCredentialLookup
	if requested := r.URL.Query()["credential"]; r.Method == http.MethodGet && len(requested) == 1 && requested[0] == "1" {
		credential = h.collectRenterCredential(r.Context(), actor, assignment, customerClient.Fingerprint)
	}
	var slot renterSlotOpenObservation
	var slotAudience, slotWriter []byte
	if requested := r.URL.Query()["slot"]; r.Method == http.MethodGet && len(requested) == 1 &&
		requested[0] == "1" && h.renterSlotOpenCollector != nil {
		collector := h.renterSlotOpenCollector
		if client, ok := service.CurrentVerifiedClient(collector.tls, time.Now()); ok {
			slotAudience, slotWriter = client.Fingerprint[:], collector.writer[:]
			slot, _ = h.readRenterSlotOpen(r.Context(), r.TLS, assignment, collector)
		}
	}
	base := raw
	var publishedAssessment *renterReturnAssessment
	if r.Method == http.MethodGet && h.RenterEvidence != nil {
		assessment := h.addRenterReturnEvidence(r.Context(), actor, assignment)
		var decimals uint8
		var symbol string
		if assessment != nil && h.RenterV1Origins != nil && *assessment.PendingPayments > 0 {
			if lookup := h.RenterV1Origins.TokenMetadata; lookup != nil {
				ctx, cancel := context.WithTimeout(r.Context(), time.Second)
				n, label, lookupErr := lookup(ctx, common.HexToHash(assessment.Hash))
				cancel()
				current := h.addRenterReturnEvidence(r.Context(), actor, assignment)
				if current == nil || current.ScopeDigest != assessment.ScopeDigest {
					assessment = nil
				} else {
					assessment = current
					if lookupErr == nil && n <= 36 && renterAssetSymbol.MatchString(label) {
						decimals, symbol = n, label
					}
				}
			}
		}
		// A revoke is distinct from losing the evidence. Recheck the active
		// handover after the chain RPC, even when it failed.
		previous := bytes.Clone(raw)
		var orders json.RawMessage
		if assessment != nil && h.RenterV1Origins != nil && *assessment.PendingPayments > 0 {
			fingerprint := h.RenterEvidence.Reader.CallerFingerprint()
			var scope *string
			err = h.Store.db.QueryRow(r.Context(), `
				WITH current AS MATERIALIZED (SELECT ledger.renter_return_assessment($1,$2,$3,$4) value)
				SELECT ledger.read_renter_return($1,$2,$3), current.value->>'scopeDigest',
					CASE WHEN current.value->>'scopeDigest'=$5
						THEN ledger.read_renter_v1_pending_orders($1,$2,$3,$4,$5) END
					FROM current`, actor.Tenant, actor.Subject, assignment, fingerprint[:], assessment.ScopeDigest).
				Scan(&raw, &scope, &orders)
			if scope == nil || *scope != assessment.ScopeDigest {
				assessment = nil
			}
		} else {
			err = h.Store.db.QueryRow(r.Context(), `SELECT ledger.read_renter_return($1,$2,$3)`,
				actor.Tenant, actor.Subject, assignment).Scan(&raw)
		}
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		if len(raw) == 0 {
			http.NotFound(w, r)
			return
		}
		base = raw
		if assessment != nil && bytes.Equal(previous, raw) {
			var projected map[string]any
			if json.Unmarshal(raw, &projected) == nil {
				projected["settlement"] = "verified"
				projected["pendingCount"] = *assessment.PendingCount
				projected["pendingPayments"] = *assessment.PendingPayments
				projected["pendingLimits"] = *assessment.PendingLimits
				projected["basisAt"] = assessment.AsOf
				projected["basisUntil"] = time.Unix(assessment.ExpiresAt, 0).UTC()
				if len(orders) > 0 {
					projected["pendingOrders"] = orders
					if symbol != "" && !bytes.Equal(orders, []byte("[]")) {
						projected["tokenDecimals"], projected["tokenSymbol"] = decimals, symbol
					}
				}
				if time.Now().Unix() < assessment.ExpiresAt {
					raw, _ = json.Marshal(projected)
					publishedAssessment = assessment
				}
			}
		}
	}
	var currentCredentialScope, currentSlotScope renterCredentialScope
	if credential != nil || slotAudience != nil {
		var settlementWriter []byte
		if publishedAssessment != nil {
			fingerprint := h.RenterEvidence.Reader.CallerFingerprint()
			settlementWriter = fingerprint[:]
		}
		var credentialAudience, credentialWriter []byte
		if credential != nil {
			credentialAudience, credentialWriter = credential.audience[:], h.renterCredentialCollector.writer[:]
		}
		latest, scope, slotScope, settlementScope, recheckErr := h.readRenterProjectionScopes(
			r.Context(), actor, assignment, credentialAudience, credentialWriter, slotAudience, slotWriter, settlementWriter)
		if recheckErr != nil {
			// An unavailable optional projection cannot authorize stale handover data.
			latest = nil
			recheckErr = h.Store.db.QueryRow(r.Context(), `SELECT ledger.read_renter_return($1,$2,$3)`,
				actor.Tenant, actor.Subject, assignment).Scan(&latest)
			if recheckErr != nil {
				operatorResponse(w, nil, recheckErr)
				return
			}
			scope, slotScope, settlementScope = renterCredentialScope{}, renterCredentialScope{}, nil
		}
		if len(latest) == 0 {
			http.NotFound(w, r)
			return
		}
		if !bytes.Equal(latest, base) || (publishedAssessment != nil &&
			(settlementScope == nil || *settlementScope != publishedAssessment.ScopeDigest ||
				time.Now().Unix() >= publishedAssessment.ExpiresAt)) {
			raw = latest
		}
		currentCredentialScope, currentSlotScope = scope, slotScope
	}
	current, customerClient, stillAuthorized := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	if !stillAuthorized || current.Tenant != actor.Tenant || current.Subject != actor.Subject || current.Role != actor.Role {
		http.NotFound(w, r)
		return
	}
	if credential != nil {
		raw = projectRenterCredential(raw, credential, currentCredentialScope, customerClient.ExpiresAt.Unix(), time.Now())
	}
	raw = projectRenterSlotOpen(raw, slot, currentSlotScope, customerClient.ExpiresAt.Unix(), time.Now())
	operatorResponse(w, raw, err)
}
