package chainview_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

type renterTokenMetadataGate struct {
	armed           atomic.Bool
	entered, resume chan struct{}
}

func blockRenterTokenMetadata(t *testing.T, origin string) (*renterTokenMetadataGate, string) {
	t.Helper()
	target, err := url.Parse(origin)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	gate := &renterTokenMetadataGate{entered: make(chan struct{}), resume: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "invalid fixture RPC", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if bytes.Contains(raw, []byte("0x313ce567")) && gate.armed.CompareAndSwap(true, false) {
			select {
			case gate.entered <- struct{}{}:
			case <-r.Context().Done():
				return
			}
			select {
			case <-gate.resume:
			case <-r.Context().Done():
				return
			}
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return gate, server.URL
}

func (s *combinedGenerationOpsd) readWhileTokenMetadataChanges(t *testing.T, path string, change func()) ([]byte, int) {
	t.Helper()
	gate := s.tokenMetadataGate
	if gate == nil {
		t.Fatal("token metadata fixture gate unavailable")
	}
	type result struct {
		value  []byte
		status int
		err    error
	}
	results := make(chan result, 1)
	gate.armed.Store(true)
	go func() {
		response, err := s.renter.Get(s.origin + path)
		if err != nil {
			results <- result{err: err}
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		results <- result{value: body, status: response.StatusCode, err: err}
	}()
	select {
	case <-gate.entered:
	case early := <-results:
		t.Fatalf("scope read did not block at live token metadata: status=%d err=%v", early.status, early.err)
	case <-time.After(5 * time.Second):
		t.Fatal("token metadata was not requested")
	}
	change()
	select {
	case gate.resume <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("token metadata fixture could not resume")
	}
	reply := <-results
	if reply.err != nil {
		t.Fatal(reply.err)
	}
	return reply.value, reply.status
}
