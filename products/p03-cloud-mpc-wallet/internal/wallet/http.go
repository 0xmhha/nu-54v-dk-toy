package wallet

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	protocol "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
)

type API struct {
	Store                   Store
	Identity                IdentityVerifier
	ProfileSubject          string
	ApprovalKey             ed25519.PublicKey
	RecoveryKey             ed25519.PublicKey
	Mobile                  MobileRegistry
	RecoveryOperator        IdentityVerifier
	RecoveryOperatorSubject string
	OperatorParticipants    Participants
	OperatorGrant           func(Claims) bool
}

func bearer(req *http.Request) string {
	token, _ := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	return token
}

func (a API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/onboarding", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		network := TestProfile
		if a.Store.MarketPolicy != nil {
			network = MarketTestProfile
		}
		writeResponse(w, struct {
			Network  string `json:"network"`
			Identity string `json:"identity"`
		}{network, a.Identity.Issuer}, nil)
	})
	handle := func(pattern, scope string, fn func(http.ResponseWriter, *http.Request, Claims)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok {
				http.Error(w, "login required", http.StatusUnauthorized)
				return
			}
			actor, err := a.Identity.Verify(token, scope, time.Now())
			if err != nil {
				http.Error(w, "login required", http.StatusUnauthorized)
				return
			}
			if a.Store.Bindings == nil && actor.Subject != a.ProfileSubject {
				http.Error(w, "pilot profile unavailable", http.StatusForbidden)
				return
			}
			fn(w, r, actor)
		})
	}
	handle("POST /v1/enrollment", "wallet:enroll", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		writeResponse(w, nil, a.Store.Enroll(r.Context(), actor))
	})
	handle("POST /v1/mobile/roster", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		a.Mobile.Handle(a.Store, w, r, actor)
	})
	handle("GET /v1/session", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		subject, err := a.Store.Subject(r.Context(), actor)
		linked := err == nil
		if errors.Is(err, ErrForbidden) {
			pending, checkErr := a.Store.ProfilePending(r.Context(), actor)
			if checkErr != nil {
				writeResponse(w, nil, checkErr)
				return
			}
			if pending {
				subject, err = actor.Subject, nil
			}
		}
		writeClaimsResponse(w, struct {
			Subject string `json:"subject"`
			Expires int64  `json:"expires"`
			Linked  bool   `json:"linked"`
		}{subject, actor.Expires, linked}, err, actor)
	})
	handle("GET /v1/wallet", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		result, err := a.Store.Wallet(r.Context(), actor)
		writeClaimsResponse(w, result, err, actor)
	})
	handle("POST /v1/operations", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		var input struct {
			ID       string        `json:"id"`
			Kind     ceremony.Kind `json:"kind"`
			Transfer *Transfer     `json:"transfer,omitempty"`
		}
		if !requestJSON(w, r, &input) || !validID(w, input.ID) {
			return
		}
		result, err := a.Store.Reserve(r.Context(), actor, input.ID, input.Kind, input.Transfer)
		writeClaimsResponse(w, result, err, actor)
	})
	handle("GET /v1/operations/{id}", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		id := r.PathValue("id")
		if !validID(w, id) {
			return
		}
		result, err := a.Store.Operation(r.Context(), actor, id)
		writeClaimsResponse(w, result, err, actor)
	})
	handle("PUT /v1/operations/{id}/approval", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		var input struct {
			Proof []byte `json:"proof"`
		}
		id := r.PathValue("id")
		if !validID(w, id) || !requestJSON(w, r, &input) {
			return
		}
		result, err := a.Store.Approve(r.Context(), actor, id, input.Proof, a.ApprovalKey, a.RecoveryKey)
		writeClaimsResponse(w, result, err, actor)
	})
	mux.HandleFunc("POST /v1/sign/refusals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if _, ok := protocol.CurrentVerifiedClient(r.TLS, time.Now()); !ok {
			http.NotFound(w, r)
			return
		}
		var refusal SignedSignRefusal
		if !requestJSON(w, r, &refusal) {
			return
		}
		writeResponse(w, nil, a.Store.RecordSignRefusal(r.Context(), refusal, a.ApprovalKey))
	})
	mux.HandleFunc("POST /v1/recovery/refusals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if _, ok := protocol.CurrentVerifiedClient(r.TLS, time.Now()); !ok {
			http.NotFound(w, r)
			return
		}
		var refusal SignedRecoveryRefusal
		if !requestJSON(w, r, &refusal) {
			return
		}
		writeResponse(w, nil, a.Store.RecordRecoveryRefusal(r.Context(), refusal, a.RecoveryKey))
	})
	handle("GET /v1/recovery/history", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		history, err := a.Store.RecoveryHistory(r.Context(), actor)
		writeClaimsResponse(w, history, err, actor)
	})
	handle("POST /v1/operations/{id}/review", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		var input struct {
			Successor string `json:"successor"`
		}
		id := r.PathValue("id")
		if !validID(w, id) || !requestJSON(w, r, &input) || !validID(w, input.Successor) {
			return
		}
		result, err := a.Store.ReviewRecovery(r.Context(), actor, id, input.Successor)
		writeClaimsResponse(w, result, err, actor)
	})
	handle("GET /v1/credentials", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		out, err := a.Store.Credentials(r.Context(), actor)
		writeClaimsResponse(w, out, err, actor)
	})
	handle("POST /v1/credentials", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		var input struct {
			EnrollmentToken string `json:"enrollmentToken"`
		}
		if !requestJSON(w, r, &input) {
			return
		}
		enrollment, err := a.Identity.Verify(input.EnrollmentToken, "wallet:enroll", time.Now())
		if err != nil {
			writeResponse(w, nil, ErrForbidden)
			return
		}
		writeClaimsResponse(w, nil, a.Store.Link(r.Context(), actor, enrollment), actor, enrollment)
	})
	handle("DELETE /v1/credentials/{id}", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		id := r.PathValue("id")
		if !validID(w, id) {
			return
		}
		writeResponse(w, nil, a.Store.Unlink(r.Context(), actor, id))
	})
	handle("GET /v1/close", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		out, err := a.Store.Closure(r.Context(), actor, "")
		writeClaimsResponse(w, out, err, actor)
	})
	handle("GET /v1/close/{request}", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		request := r.PathValue("request")
		if !validID(w, request) {
			return
		}
		out, err := a.Store.Closure(r.Context(), actor, request)
		writeClaimsResponse(w, out, err, actor)
	})
	handle("POST /v1/close", "wallet", func(w http.ResponseWriter, r *http.Request, actor Claims) {
		var input struct {
			Wallet            string `json:"wallet"`
			Request           string `json:"request,omitempty"`
			Epoch             uint64 `json:"epoch"`
			Proof             []byte `json:"proof"`
			ConfirmAccessEnds bool   `json:"confirmAccessEnds"`
		}
		if !requestJSON(w, r, &input) || !validID(w, input.Wallet) {
			return
		}
		if !input.ConfirmAccessEnds || input.Epoch == 0 {
			http.Error(w, "confirmation required", http.StatusBadRequest)
			return
		}
		if input.Request != "" {
			if !validID(w, input.Request) {
				return
			}
			out, err := a.Store.CloseWithReceipt(r.Context(), actor, input.Wallet, input.Epoch, input.Request, input.Proof, a.ApprovalKey)
			writeClaimsResponse(w, out, err, actor)
			return
		}
		writeResponse(w, nil, a.Store.Close(r.Context(), actor, input.Wallet, input.Epoch, input.Proof, a.ApprovalKey))
	})
	a.recoveryHandlers(mux)
	return mux
}

func validID(w http.ResponseWriter, id string) bool {
	if canonicalUUID(id) {
		return true
	}
	http.Error(w, "invalid identifier", http.StatusBadRequest)
	return false
}

func requestJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err == nil && decodeStrict(raw, out) == nil {
		return true
	}
	http.Error(w, "invalid request", http.StatusBadRequest)
	return false
}

func writeClaimsResponse(w http.ResponseWriter, result any, err error, actors ...Claims) {
	var payload []byte
	if err == nil && result != nil {
		payload, err = json.Marshal(result)
	}
	if expiry := claimsCurrent(time.Now(), actors...); expiry != nil {
		err = expiry
	}
	if payload == nil || err != nil {
		writeResponse(w, nil, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(payload, '\n'))
}

func writeResponse(w http.ResponseWriter, result any, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		http.Error(w, "wallet access denied", http.StatusForbidden)
	case errors.Is(err, ErrMissing):
		http.Error(w, "wallet request unavailable", http.StatusNotFound)
	case errors.Is(err, ErrConflict):
		http.Error(w, "wallet state changed", http.StatusConflict)
	case err != nil:
		http.Error(w, "wallet service unavailable", http.StatusServiceUnavailable)
	case result == nil:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}
