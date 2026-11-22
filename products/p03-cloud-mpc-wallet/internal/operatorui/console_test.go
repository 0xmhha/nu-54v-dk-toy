package operatorui

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOperatorPortalScopeAndExplicitFreeze(t *testing.T) {
	const id = "d44b1209-88c4-497a-b901-f9d805b05228"
	var reports int
	var probe, revoked atomic.Bool
	enter, release := make(chan struct{}), make(chan struct{})
	realm := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/p03/device/start" {
			_, _ = io.WriteString(w, `{"deviceCode":"test-local-operator-code-001","verificationURL":"https://`+r.Host+`/verify","expiresIn":120}`)
			return
		}
		_, _ = io.WriteString(w, `{"state":"ready","token":"synthetic-role-token"}`)
	})
	remote := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		res := httptest.NewRecorder()
		realm.ServeHTTP(res, req)
		return res.Result(), nil
	})}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if probe.Load() && r.Method == "GET" {
			close(enter)
			<-release
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-role-token" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Query().Get("wallet") != "" && r.URL.Query().Get("wallet") != id {
			w.WriteHeader(403)
			return
		}
		if r.Method == "POST" {
			reports++
		}
		_, _ = io.WriteString(w, `{"wallet":"`+id+`","epoch":1,"state":"active","policy":"synthetic","assetMoves":0}`)
	})
	handler, err := New("https://console.example", "https://realm.example", remote, api, func(string) (time.Time, error) {
		if revoked.Load() {
			return time.Time{}, errors.New("revoked")
		}
		return time.Now().Add(time.Minute), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	call := func(method, path string, form url.Values, want int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		if method == "POST" {
			req.Header.Set("Origin", "https://console.example")
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s status %d want %d", method, path, res.Code, want)
		}
		if cookies := res.Result().Cookies(); len(cookies) > 0 {
			cookie = cookies[0]
		}
		return res.Body.String()
	}
	call("GET", "/status?wallet="+id, nil, 401)
	call("POST", "/login", nil, 200)
	call("POST", "/session", nil, 303)
	call("GET", "/status?wallet="+id, nil, 200)
	call("GET", "/status?wallet=d44b1209-88c4-497a-b901-f9d805b05229", nil, 403)
	call("POST", "/incident", url.Values{"wallet": {id}}, 400)
	if reports != 0 {
		t.Fatal("unconfirmed freeze was issued")
	}
	call("POST", "/incident", url.Values{"wallet": {id}, "confirm": {"yes"}}, 303)
	if reports != 1 {
		t.Fatal("confirmed freeze was not sent")
	}
	probe.Store(true)
	delayed := httptest.NewRequest(http.MethodGet, "/status?wallet="+id, nil)
	delayed.AddCookie(cookie)
	result := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { handler.ServeHTTP(result, delayed); close(done) }()
	<-enter
	revoked.Store(true)
	close(release)
	<-done
	if result.Code != 401 || strings.Contains(result.Body.String(), id) {
		t.Fatal("late response showed wallet after authority removal")
	}

}

type roundTrip func(*http.Request) (*http.Response, error)

func (r roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return r(req) }
