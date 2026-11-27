package commerce

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
)

func (h HTTP) registerExperience(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/operations/experience/{branch}", h.operateExperience)
	mux.HandleFunc("POST /v2/operations/experience/{branch}/cancel", h.operateExperience)
}

func (h HTTP) operateExperience(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.operatorCredential(r)
	branch := r.PathValue("branch")
	if !ok || h.Store == nil || !operatorRef.MatchString(branch) {
		http.NotFound(w, r)
		return
	}
	var raw json.RawMessage
	var err error
	if r.Method == http.MethodGet {
		var ref *string
		if query := r.URL.Query().Get("reference"); query != "" {
			if !operatorRef.MatchString(query) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			ref = &query
		}
		ctx := authority.Scope(r.Context(), actor, branch, "experience_read")
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.operator_local_experience($1,$2,$3,$4)",
			actor.Tenant, branch, actor.Subject, ref).Scan(&raw)
	} else {
		if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || r.Header.Get("Content-Type") != "application/json" {
			http.NotFound(w, r)
			return
		}
		reader := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		reader.DisallowUnknownFields()
		var command struct {
			Subscription string `json:"subscription"`
			Key          string `json:"requestKey"`
			Revision     int64  `json:"expectedRevision"`
		}
		if reader.Decode(&command) != nil || reader.Decode(new(any)) != io.EOF ||
			!operatorRef.MatchString(command.Subscription) || !operatorRef.MatchString(command.Key) || command.Revision < 0 {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		ctx := authority.Scope(r.Context(), actor, branch, "experience_cancel")
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.operator_local_cancel_subscription($1,$2,$3,$4,$5,$6)",
			actor.Tenant, branch, actor.Subject, command.Subscription, command.Key, command.Revision).Scan(&raw)
	}
	if err == nil {
		var portfolio map[string]json.RawMessage
		if err = json.Unmarshal(raw, &portfolio); err == nil {
			scope := sha256.Sum256([]byte("nu54-experience-receipt:" + actor.Tenant + ":" + branch + ":" + actor.Subject))
			portfolio["receiptScope"], _ = json.Marshal(hex.EncodeToString(scope[:]))
			raw, err = json.Marshal(portfolio)
		}
	}
	operatorResponse(w, raw, err)
}
