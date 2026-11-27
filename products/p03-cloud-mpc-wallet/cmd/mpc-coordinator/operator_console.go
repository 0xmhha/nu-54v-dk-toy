package main

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"time"

	protocol "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/operatorui"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
)

type operatorConsoleConfig struct {
	Listen, Origin, Certificate, Key, RealmCA string
}

type recoveryOperatorGrant struct {
	Subject    string    `json:"subject"`
	Credential string    `json:"credential"`
	Expires    time.Time `json:"expires"`
}

func grantSource(path string) func(wallet.Claims) bool {
	if path == "" {
		return nil
	}
	source := protocol.ClientGrantFile[recoveryOperatorGrant](path)
	return func(actor wallet.Claims) bool {
		if actor.Subject == "" || actor.Credential == "" {
			return false
		}
		digest := sha256.Sum256([]byte("nu54/p03/recovery-ops/v1\x00" + actor.Subject + "\x00" + actor.Credential))
		g, ok := source.Lookup(hex.EncodeToString(digest[:]))
		return ok && g.Subject == actor.Subject && g.Credential == actor.Credential && time.Now().Before(g.Expires)
	}
}

func newOperatorServer(config operatorConsoleConfig, issuer string, api wallet.API) (*http.Server, error) {
	if config.Listen == "" || config.Certificate == "" || config.Key == "" || api.OperatorGrant == nil {
		return nil, errors.New("operator listener requires TLS and live operator grants")
	}
	if _, err := tls.LoadX509KeyPair(config.Certificate, config.Key); err != nil {
		return nil, err
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13}}
	if config.RealmCA != "" {
		pem, err := os.ReadFile(config.RealmCA)
		roots := x509.NewCertPool()
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("invalid operator realm CA")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	h, err := operatorui.New(config.Origin, issuer, &http.Client{Transport: transport, Timeout: 4 * time.Second}, api.Handler(),
		func(token string) (time.Time, error) {
			actor, err := api.VerifyRecoveryOperator(token, time.Now())
			return time.Unix(actor.Expires, 0), err
		})
	if err != nil {
		return nil, err
	}
	return &http.Server{Addr: config.Listen, Handler: h, ReadHeaderTimeout: 4 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 8 * time.Second,
		IdleTimeout: 30 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13}}, nil
}
