// Package authority carries a request's authenticated authority to the
// database commit and private response boundaries.
package authority

import (
	"context"
	"net/http"
	"reflect"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

type contextKey struct{}

type boundary struct {
	current func() bool
}

func Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	if guard, ok := ctx.Value(contextKey{}).(boundary); ok && !guard.current() {
		return service.Forbidden
	}
	return nil
}

// Bound uses the earlier of the existing context deadline and the verified
// certificate/grant deadline. Current must re-read any credential binding
// used to authorize this request.
func Bound(r *http.Request, deadline time.Time, current func() bool, w http.ResponseWriter) (http.ResponseWriter, *http.Request, context.CancelFunc) {
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	ctx = context.WithValue(ctx, contextKey{}, boundary{current: current})
	return &privateResponse{ResponseWriter: w, request: r, ctx: ctx}, r.WithContext(ctx), cancel
}

func Protect(credentials map[string]service.Principal, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, client, ok := service.CurrentClientPrincipal(r.TLS, credentials, time.Now())
		if !ok {
			http.NotFound(w, r)
			return
		}
		w, r, cancel := Bound(r, client.ExpiresAt, func() bool {
			active, _, ok := service.CurrentClientPrincipal(r.TLS, credentials, time.Now())
			return ok && reflect.DeepEqual(active, principal)
		}, w)
		defer cancel()
		next.ServeHTTP(w, r)
	})
}

type privateResponse struct {
	http.ResponseWriter
	request *http.Request
	ctx     context.Context
	started bool
	blocked bool
}

func (w *privateResponse) WriteHeader(status int) {
	if w.started {
		return
	}
	w.started = true
	if Check(w.ctx) != nil {
		w.blocked = true
		clear(w.Header())
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w.ResponseWriter, w.request)
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *privateResponse) Write(data []byte) (int, error) {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}
	if w.blocked || Check(w.ctx) != nil {
		return len(data), nil
	}
	return w.ResponseWriter.Write(data)
}
