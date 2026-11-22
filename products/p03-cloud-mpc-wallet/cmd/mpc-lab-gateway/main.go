package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/gateway"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	path := flag.String("config", "", "isolated gateway configuration")
	flag.Parse()
	raw, err := os.ReadFile(*path)
	if err != nil {
		log.Fatal("gateway configuration unavailable")
	}
	var c struct {
		gateway.Realm
		Listen, Certificate, CertificateKey, Seed                               string
		RefusalURL, RefusalServerCA, RefusalClientCertificate, RefusalClientKey string
	}
	if json.Unmarshal(raw, &c) != nil {
		log.Fatal("invalid gateway configuration")
	}
	if c.MarketPolicy != nil && (c.Role != "approval" || !c.MarketPolicy.Valid()) {
		log.Fatal("market policy requires a dedicated independent approval realm")
	}
	origin, err := url.Parse(c.Origin)
	_, hashErr := bcrypt.Cost([]byte(c.PasswordHash))
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.Path != "" || origin.User != nil ||
		origin.RawQuery != "" || origin.Fragment != "" || (len(c.Accounts) == 0 && (hashErr != nil || c.Username == "")) ||
		(c.Role != "identity" && c.Role != "approval" && c.Role != "recovery" && c.Role != "operator") {
		log.Fatal("invalid dedicated gateway realm")
	}
	validID := func(s string) bool { id, err := uuid.Parse(s); return err == nil && id != uuid.Nil && id.String() == s }
	if len(c.Accounts) == 0 {
		if (c.Role == "identity" || c.Role == "operator") && (!validID(c.Subject) || !validID(c.Credential)) {
			log.Fatal("invalid identity reference")
		}
		c.Key = keyFromSeed(c.Seed)
	} else {
		if c.Role == "identity" || c.Role == "operator" {
			c.Key = keyFromSeed(c.Seed)
		}
		users, subjects, credentials := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for i := range c.Accounts {
			profile := &c.Accounts[i]
			_, err := bcrypt.Cost([]byte(profile.PasswordHash))
			if err != nil || profile.Username == "" || users[profile.Username] || (c.Role != "identity" && subjects[profile.Subject]) ||
				!validID(profile.Subject) {
				log.Fatal("invalid dedicated fixture account")
			}
			users[profile.Username], subjects[profile.Subject] = true, true
			if c.Role == "identity" || c.Role == "operator" {
				if !validID(profile.Credential) || credentials[profile.Credential] {
					log.Fatal("invalid fixture credential")
				}
				credentials[profile.Credential] = true
			} else {
				profile.Key = keyFromSeed(profile.SeedPath)
			}
		}
	}
	if c.RefusalURL != "" || c.RefusalServerCA != "" || c.RefusalClientCertificate != "" || c.RefusalClientKey != "" {
		switch c.Role {
		case "recovery":
			sink, err := refusalSink[wallet.SignedRecoveryRefusal](c.RefusalURL, "/v1/recovery/refusals", c.RefusalServerCA, c.RefusalClientCertificate, c.RefusalClientKey)
			if err != nil {
				log.Fatal("recovery refusal transport unavailable")
			}
			c.RecoveryRefusalSink = sink
		case "approval":
			if len(c.Accounts) == 0 && !validID(c.Subject) {
				log.Fatal("sign refusal requires an owner reference")
			}
			sink, err := refusalSink[wallet.SignedSignRefusal](c.RefusalURL, "/v1/sign/refusals", c.RefusalServerCA, c.RefusalClientCertificate, c.RefusalClientKey)
			if err != nil {
				log.Fatal("sign refusal transport unavailable")
			}
			c.SignRefusalSink = sink
		default:
			log.Fatal("refusal transport requires a consent realm")
		}
	}
	server := http.Server{Addr: c.Listen, Handler: c.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 6 * time.Second,
		WriteTimeout: 6 * time.Second, IdleTimeout: 15 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13}}
	log.Fatal(server.ListenAndServeTLS(c.Certificate, c.CertificateKey))
}

func keyFromSeed(path string) ed25519.PrivateKey {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != ed25519.SeedSize {
		log.Fatal("private gateway seed unavailable")
	}
	seed, err := os.ReadFile(path)
	if err != nil {
		log.Fatal("private gateway seed unavailable")
	}
	defer clear(seed)
	return ed25519.NewKeyFromSeed(seed)
}

func refusalSink[T any](endpoint, path, authority, certificate, key string) (func(context.Context, T) error, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		u.Path != path || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid refusal endpoint")
	}
	ca, err := os.ReadFile(authority)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid refusal server authority")
	}
	identity, err := tls.LoadX509KeyPair(certificate, key)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 4 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{identity}}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return func(ctx context.Context, vote T) error {
		data, err := json.Marshal(vote)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode == http.StatusConflict {
			return wallet.ErrConflict
		}
		if response.StatusCode != http.StatusNoContent {
			return errors.New("refusal delivery was not committed")
		}
		return nil
	}, nil
}
