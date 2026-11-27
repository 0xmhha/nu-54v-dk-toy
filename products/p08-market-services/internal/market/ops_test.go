package market

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestOperatorPageDeniesUnscopedRequestsBeforeReading(t *testing.T) {
	reads := 0
	handler, err := operatorRoutes("operator", strings.Repeat("a", 64), func(context.Context) operatorView {
		reads++
		return operatorView{State: "price-unavailable"}
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path, user, token string
		code                      int
	}{
		{"GET", "/", "", "", 401},
		{"GET", "/", "learner", strings.Repeat("a", 64), 401},
		{"GET", "/", "operator", "wrong", 401},
		{"GET", "/?deployment=elsewhere", "operator", strings.Repeat("a", 64), 404},
		{"GET", "/v1/wallet/me", "operator", strings.Repeat("a", 64), 404},
		{"POST", "/", "operator", strings.Repeat("a", 64), 405},
		{"GET", "/", "operator", strings.Repeat("a", 64), 200},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		if test.user != "" {
			request.SetBasicAuth(test.user, test.token)
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, request)
		if result.Code != test.code {
			t.Fatalf("%s %s got %d", test.method, test.path, result.Code)
		}
		if result.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cached operator page")
		}
	}
	if reads != 1 {
		t.Fatalf("%d privileged reads", reads)
	}
}

func TestOperatorSeparatesChainClockFromPriceAge(t *testing.T) {
	wall := time.Unix(2000, 0)
	for _, item := range []struct {
		time  uint64
		state string
	}{{2000, "current"}, {2010, "current"}, {2011, "ahead"}, {1970, "current"}, {1969, "stalled"}} {
		if got := tipClock(item.time, wall); got != item.state {
			t.Fatalf("clock at %d: %s", item.time, got)
		}
	}
}

func OperatorPage(user, token string, chain *Chain, db *pgxpool.Pool) (http.Handler, error) {
	return operatorRoutes(user, token, func(ctx context.Context) operatorView {
		return buildOperatorView(ctx, chain, db)
	})
}

func operatorRoutes(user, token string, read func(context.Context) operatorView) (http.Handler, error) {
	if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`).MatchString(user) || len(token) != 64 {
		return nil, errors.New("configure an operator name and a random 32-byte hex credential")
	}
	if _, err := hex.DecodeString(token); err != nil {
		return nil, errors.New("configure an operator name and a random 32-byte hex credential")
	}
	userHash, tokenHash := sha256.Sum256([]byte(user)), sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		operatorHeaders(w)
		name, password, exists := r.BasicAuth()
		gotUser, gotToken := sha256.Sum256([]byte(name)), sha256.Sum256([]byte(password))
		accepted := subtle.ConstantTimeCompare(gotUser[:], userHash[:]) & subtle.ConstantTimeCompare(gotToken[:], tokenHash[:])
		if !exists || accepted != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="P08 isolated operations", charset="UTF-8"`)
			http.Error(w, "Operator credentials required", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Read-only operator page", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = operatorTemplate.Execute(w, read(ctx))
	}), nil
}
