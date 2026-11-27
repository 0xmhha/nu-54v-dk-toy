package records

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"time"

	protocol "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GrantFile = protocol.ClientGrantFile[Principal]

func validPrivateGrant(digest string, principal Principal) bool {
	return fingerprint.MatchString(digest) && uuid.MatchString(principal.Subject) && principal.ExpiresAt > 0 &&
		((principal.Role == "private-user" && principal.Issuer == "") ||
			((principal.Role == "visit-issuer" || principal.Role == "benefit-worker") && recipientName.MatchString(principal.Issuer)))
}

// UseGrantFile selects the published authority before the listener is started.
func (s *Service) UseGrantFile(path string, restore bool) {
	valid := validPrivateGrant
	if restore {
		valid = validRestoreGrant
	}
	file := GrantFile(path)
	s.lookup = func(key string) (Principal, bool) {
		grants, err := file.Read()
		if err != nil {
			return Principal{}, false
		}
		for digest, grant := range grants {
			if !valid(digest, grant) {
				return Principal{}, false
			}
		}
		grant, ok := grants[key]
		return grant, ok
	}
}

func (s *Service) principalAt(r *http.Request, now time.Time) (Principal, time.Time, bool) {
	client, ok := protocol.CurrentVerifiedClient(r.TLS, now)
	if !ok {
		return Principal{}, time.Time{}, false
	}
	key := client.FingerprintHex()
	actor, ok := s.grants[key]
	if s.lookup != nil {
		actor, ok = s.lookup(key)
	}
	if expected, bound := r.Context().Value(boundPrincipal{}).(Principal); !ok || (bound && actor != expected) {
		return Principal{}, time.Time{}, false
	}
	deadline := client.ExpiresAt
	if expiry := time.Unix(actor.ExpiresAt, 0); expiry.Before(deadline) {
		deadline = expiry
	}
	return actor, deadline, now.Before(deadline)
}

func (s *Service) stillAuthorized(r *http.Request, actor Principal) bool {
	current, ok := s.principal(r)
	return ok && current == actor && r.Context().Err() == nil
}

type boundPrincipal struct{}

type guardedResponse struct {
	underlying http.ResponseWriter
	header     http.Header
	body       bytes.Buffer
	status     int
}

func (response *guardedResponse) Unwrap() http.ResponseWriter { return response.underlying }

func (response *guardedResponse) Header() http.Header { return response.header }
func (response *guardedResponse) WriteHeader(status int) {
	if response.status == 0 {
		response.status = status
	}
}
func (response *guardedResponse) Write(data []byte) (int, error) {
	response.WriteHeader(http.StatusOK)
	return response.body.Write(data)
}

func (s *Service) authorized(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, deadline, ok := s.principalAt(r, s.now())
		if !ok {
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithDeadline(r.Context(), deadline)
		defer cancel()
		r = r.WithContext(context.WithValue(ctx, boundPrincipal{}, actor))
		response := &guardedResponse{underlying: w, header: make(http.Header)}
		handler(response, r)
		if !s.stillAuthorized(r, actor) {
			http.NotFound(w, r)
			return
		}
		for key, value := range response.header {
			w.Header()[key] = value
		}
		if response.status != 0 {
			w.WriteHeader(response.status)
			_, _ = w.Write(response.body.Bytes())
		}
	}
}

var errGrantChanged = errors.New("private travel grant changed")

// Run the mutation and its authority checks in one transaction so a newer
// published grant observed while SQL waited can still prevent the commit.
func (s *Service) mutate(r *http.Request, actor Principal, pool *pgxpool.Pool, effect func(pgx.Tx) error) error {
	if !s.stillAuthorized(r, actor) {
		return errGrantChanged
	}
	return pgx.BeginFunc(r.Context(), pool, func(tx pgx.Tx) error {
		if !s.stillAuthorized(r, actor) {
			return errGrantChanged
		}
		if err := effect(tx); err != nil {
			return err
		}
		if !s.stillAuthorized(r, actor) {
			return errGrantChanged
		}
		return nil
	})
}

type mutationScan func(...any) error

func (scan mutationScan) Scan(dest ...any) error { return scan(dest...) }

func (s *Service) mutationRow(r *http.Request, actor Principal, pool *pgxpool.Pool, sql string, args ...any) pgx.Row {
	return mutationScan(func(dest ...any) error {
		return s.mutate(r, actor, pool, func(tx pgx.Tx) error {
			return tx.QueryRow(r.Context(), sql, args...).Scan(dest...)
		})
	})
}
