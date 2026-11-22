package settlement

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	settlementProbeTimeout    = 500 * time.Millisecond
	settlementCustomerTimeout = 8 * time.Second
)

func settlementFixtureTLSClient(t *testing.T, roots *x509.CertPool, timeout time.Duration, certs ...tls.Certificate) *http.Client {
	t.Helper()
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: roots, Certificates: certs, MinVersion: tls.VersionTLS13,
	}}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: timeout}
}

func TestSettlementBuyerResponseCanOutlastReadinessProbe(t *testing.T) {
	const responseDelay = 650 * time.Millisecond
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		delay := time.NewTimer(responseDelay)
		defer delay.Stop()
		select {
		case <-delay.C:
			w.WriteHeader(http.StatusNoContent)
		case <-req.Context().Done():
		}
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())

	probe := settlementFixtureTLSClient(t, roots, settlementProbeTimeout)
	if resp, err := probe.Get(server.URL); err == nil {
		resp.Body.Close()
		t.Fatal("readiness probe waited for a delayed response")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("readiness probe did not expire at its deadline: %v", err)
	}
	buyer := settlementFixtureTLSClient(t, roots, settlementCustomerTimeout)
	start := time.Now()
	resp, err := buyer.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || time.Since(start) <= settlementProbeTimeout {
		t.Fatalf("buyer did not receive the deliberately delayed response: HTTP %d", resp.StatusCode)
	}
}
