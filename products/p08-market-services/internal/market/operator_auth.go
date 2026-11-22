package market

import (
	"bytes"
	"context"
	_ "embed"
	"net/http"
	"regexp"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OperatorGrant struct {
	Subject    string         `json:"subjectRef"`
	Role       string         `json:"role"`
	Genesis    common.Hash    `json:"genesis"`
	Deployment common.Hash    `json:"deploymentHash"`
	Venue      common.Address `json:"venue"`
	ExpiresAt  int64          `json:"expiresAt"`
}

var operatorSubject = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

//go:embed operator_console.js
var operatorConsoleJS string

func OperatorMTLS(chain *Chain, db *pgxpool.Pool, lookup func(string) (OperatorGrant, bool)) http.Handler {
	return operatorMTLS(chain.Profile, lookup, time.Now, func(ctx context.Context) operatorView {
		return buildOperatorView(ctx, chain, db)
	})
}

func operatorMTLS(profile Profile, lookup func(string) (OperatorGrant, bool), clock func() time.Time,
	read func(context.Context) operatorView) http.Handler {
	authorize := func(r *http.Request) (OperatorGrant, time.Time, bool) {
		now := clock()
		peer, ok := service.CurrentVerifiedClient(r.TLS, now)
		if !ok {
			return OperatorGrant{}, time.Time{}, false
		}
		grant, ok := lookup(peer.FingerprintHex())
		expiry := time.Unix(grant.ExpiresAt, 0)
		if !ok || grant.Role != "market-operator" || !operatorSubject.MatchString(grant.Subject) ||
			grant.Genesis != profile.Genesis || grant.Deployment != profile.BlockHash || grant.Venue != profile.Venue.Address ||
			!now.Before(expiry) {
			return OperatorGrant{}, time.Time{}, false
		}
		if peer.ExpiresAt.Before(expiry) {
			expiry = peer.ExpiresAt
		}
		return grant, expiry, true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		operatorHeaders(w)
		path := r.URL.Path
		if r.URL.RawQuery != "" || (path != "/" && path != "/operator.js" && path != "/access" && path != "/observation") {
			http.NotFound(w, r)
			return
		}
		if _, ok := service.CurrentVerifiedClient(r.TLS, clock()); !ok {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Read-only operator page", http.StatusMethodNotAllowed)
			return
		}
		switch path {
		case "/":
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; connect-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = operatorTemplate.ExecuteTemplate(w, "console", nil)
			return
		case "/operator.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = w.Write([]byte(operatorConsoleJS))
			return
		case "/observation":
			if r.Header.Get("X-P08-Console") != "observation" {
				http.NotFound(w, r)
				return
			}
		}
		grant, expires, ok := authorize(r)
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if path == "/access" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Compute a real relative timeout so tests can inject a deterministic authorization clock.
		wait := min(12*time.Second, expires.Sub(clock()))
		if wait <= 0 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), wait)
		defer cancel()
		view := read(ctx)
		var output bytes.Buffer
		if err := operatorTemplate.ExecuteTemplate(&output, "observation", view); err != nil {
			http.Error(w, "Operator evidence unavailable", 503)
			return
		}
		current, _, ok := authorize(r)
		if !ok || grant != current {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if ctx.Err() != nil {
			http.Error(w, "Operator evidence unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = output.WriteTo(w)
	})
}
