// Package evidencev2 adapts the authenticated P07 C7 read boundary to chainview.
package evidencev2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"reflect"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

type Reader interface {
	Finalized(context.Context, string, string) (*service.Evidence, error)
}

func TLSConfig(clientCA *x509.CertPool) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCA}
}

func Handler(credentials map[string]service.Principal, store Reader, clock func() time.Time) http.Handler {
	return HandlerWithLookup(func(key string) (service.Principal, bool) {
		principal, ok := credentials[key]
		return principal, ok
	}, store, clock)
}

func HandlerWithLookup(lookup func(string) (service.Principal, bool), store Reader, clock func() time.Time) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v2/evidence/read", func(w http.ResponseWriter, r *http.Request) {
		now := clock()
		principal, client, allowed := service.CurrentClientPrincipalFrom(r.TLS, lookup, now)
		if !allowed {
			http.NotFound(w, r)
			return
		}
		req, err := service.DecodeRequest(r.Body)
		if err != nil || req.Contract != "C7" || req.Operation != "evidence.read" {
			write(w, http.StatusBadRequest, map[string]service.Code{"error": service.BadRequest})
			return
		}
		now = clock()
		current, _, allowed := service.CurrentClientPrincipalFrom(r.TLS, lookup, now)
		if !allowed || !reflect.DeepEqual(current, principal) {
			http.NotFound(w, r)
			return
		}
		bound, err := service.Authorize(req, principal, now)
		if service.CodeOf(err) == service.Forbidden {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			write(w, http.StatusBadRequest, map[string]service.Code{"error": service.CodeOf(err)})
			return
		}
		uncertain := service.Outcome(bound, "error")
		uncertain.Error = service.Unknown
		kind := req.Body["kind"]
		if kind == "revoked" {
			// A missing or inactive projection row supplies no revocation proof.
			write(w, http.StatusServiceUnavailable, uncertain)
			return
		}
		deadline := client.ExpiresAt
		if expires := time.Unix(req.ExpiresAt, 0); expires.Before(deadline) {
			deadline = expires
		}
		ctx, cancel := context.WithTimeout(r.Context(), deadline.Sub(now))
		defer cancel()
		evidence, err := store.Finalized(ctx, req.Domain, req.Object)
		now = clock()
		current, _, allowed = service.CurrentClientPrincipalFrom(r.TLS, lookup, now)
		if !allowed || !reflect.DeepEqual(current, principal) {
			http.NotFound(w, r)
			return
		}
		if now.Unix() >= req.ExpiresAt || ctx.Err() != nil {
			expired := service.Outcome(bound, "error")
			expired.Error = service.Expired
			write(w, http.StatusBadRequest, expired)
			return
		}
		if err == nil && evidence == nil && kind == "unobserved" {
			// Unobserved is a report about this reader, never a payment absence proof.
			write(w, http.StatusOK, service.Outcome(bound, "unobserved"))
			return
		}
		if err != nil || evidence == nil || kind != "finalized" {
			write(w, http.StatusServiceUnavailable, uncertain)
			return
		}
		reply := service.Outcome(bound, "finalized")
		reply.Evidence = evidence
		if service.ValidateReply(bound, reply, now) != nil {
			write(w, http.StatusServiceUnavailable, uncertain)
			return
		}
		write(w, http.StatusOK, reply)
	})
	return mux
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
