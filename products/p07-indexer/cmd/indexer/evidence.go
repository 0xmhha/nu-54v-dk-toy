package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/evidencev2"
)

// runEvidence serves only the C7 evidence projection over dedicated mutual TLS.
func runEvidence(gate *service.RecoveryGate) error {
	for _, key := range []string{"P07_EVIDENCE_DATABASE_URL", "P07_EVIDENCE_CA", "P07_EVIDENCE_CERT", "P07_EVIDENCE_KEY", "P07_EVIDENCE_GRANTS"} {
		if os.Getenv(key) == "" {
			return fmt.Errorf("%s is required", key)
		}
	}
	ca, err := os.ReadFile(os.Getenv("P07_EVIDENCE_CA"))
	if err != nil {
		return err
	}
	trust := x509.NewCertPool()
	if !trust.AppendCertsFromPEM(ca) {
		return errors.New("P07_EVIDENCE_CA has no certificates")
	}
	source := grantFile[service.Principal](os.Getenv("P07_EVIDENCE_GRANTS"))
	if _, err := source.read(); err != nil {
		return errors.New("P07_EVIDENCE_GRANTS must contain a certificate fingerprint grant map")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := evidencev2.Open(ctx, os.Getenv("P07_EVIDENCE_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer store.Close()
	server := &http.Server{
		Addr:              env("P07_EVIDENCE_HTTP_ADDR", "127.0.0.1:8443"),
		Handler:           gate.Handler(evidencev2.HandlerWithLookup(source.lookup, store, time.Now)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		TLSConfig:         evidencev2.TLSConfig(trust),
	}
	return server.ListenAndServeTLS(os.Getenv("P07_EVIDENCE_CERT"), os.Getenv("P07_EVIDENCE_KEY"))
}

func runExplorerOperations(gate *service.RecoveryGate) error {
	for _, key := range []string{"P07_EXPLORER_DATABASE_URL", "P07_EXPLORER_CA", "P07_EXPLORER_CERT", "P07_EXPLORER_KEY", "P07_EXPLORER_GRANTS"} {
		if os.Getenv(key) == "" {
			return fmt.Errorf("%s is required", key)
		}
	}
	ca, err := os.ReadFile(os.Getenv("P07_EXPLORER_CA"))
	if err != nil {
		return err
	}
	trust := x509.NewCertPool()
	if !trust.AppendCertsFromPEM(ca) {
		return errors.New("P07_EXPLORER_CA has no certificates")
	}
	source := grantFile[chainview.OperatorGrant](os.Getenv("P07_EXPLORER_GRANTS"))
	if _, err := source.read(); err != nil {
		return errors.New("P07_EXPLORER_GRANTS must contain a certificate fingerprint grant map")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := chainview.OpenOperator(ctx, os.Getenv("P07_EXPLORER_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer store.Close()
	if url := os.Getenv("P07_QUOTE_ORIGIN"); url != "" {
		reader, cleanup, err := quoteOriginReader(url)
		if err != nil {
			return err
		}
		defer cleanup()
		store.QuoteOrigin = reader
	}
	if replayURL := os.Getenv("P07_REPLAY_REQUEST_DATABASE_URL"); replayURL != "" {
		if err := store.EnableReplayRequests(ctx, replayURL); err != nil {
			return err
		}
	}
	server := &http.Server{Addr: env("P07_EXPLORER_HTTP_ADDR", "127.0.0.1:8444"),
		Handler: gate.Handler(store.HandlerWithLookup(source.lookup)), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
		TLSConfig: evidencev2.TLSConfig(trust)}
	return server.ListenAndServeTLS(os.Getenv("P07_EXPLORER_CERT"), os.Getenv("P07_EXPLORER_KEY"))
}

func quoteOriginReader(origin string) (*chainview.QuoteOriginReader, func(), error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, nil, errors.New("P07_QUOTE_ORIGIN must be the HTTPS quote issuer origin")
	}
	u.Path = "/v2/finance/quote-boundary-origin"
	trust := x509.NewCertPool()
	ca, err := os.ReadFile(os.Getenv("P07_QUOTE_ORIGIN_CA"))
	if err != nil || !trust.AppendCertsFromPEM(ca) {
		return nil, nil, errors.New("P07_QUOTE_ORIGIN_CA must contain the issuer CA")
	}
	cert, err := tls.LoadX509KeyPair(os.Getenv("P07_QUOTE_ORIGIN_CERT"), os.Getenv("P07_QUOTE_ORIGIN_KEY"))
	if err != nil {
		return nil, nil, errors.New("P07 quote issuer reader certificate and key required")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: trust, Certificates: []tls.Certificate{cert},
	}}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &chainview.QuoteOriginReader{Client: client, URL: u.String()}, transport.CloseIdleConnections, nil
}
