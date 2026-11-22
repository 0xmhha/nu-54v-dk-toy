// mpc-oidc-gateway issues P03 login grants for registered OIDC subjects.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/gateway"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/privatefile"
)

type config struct {
	gateway.OIDCConfig
	Origin, Listen, Certificate, CertificateKey, Seed, LookupKey string
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil && err != flag.ErrHelp {
		log.Fatal(err)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("mpc-oidc-gateway", flag.ContinueOnError)
	flags.SetOutput(stdout)
	path := flags.String("config", "", "registered identity realm configuration")
	digestProvider := flags.String("subject-digest", "", "provider ID for offline enrollment digest from standard input")
	if err := flags.Parse(args); err != nil {
		return err
	}
	c, err := readConfig(*path)
	if err != nil {
		return err
	}
	lookup, err := privateSeed(c.LookupKey)
	if err != nil {
		return err
	}
	defer clear(lookup)
	if *digestProvider != "" {
		for _, provider := range c.Providers {
			if provider.Identity.ID == *digestProvider {
				raw, err := io.ReadAll(io.LimitReader(stdin, 258))
				subject := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
				clear(raw)
				if err != nil || len(subject) < 1 || len(subject) > 255 || strings.ContainsAny(subject, "\r\n\x00") {
					return errors.New("invalid provider subject")
				}
				_, err = fmt.Fprintln(stdout, gateway.OIDCSubjectDigest(lookup, provider, subject))
				return err
			}
		}
		return errors.New("provider is unavailable")
	}
	seed, err := privateSeed(c.Seed)
	if err != nil {
		return err
	}
	private := ed25519.NewKeyFromSeed(seed)
	clear(seed)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	realm, err := gateway.NewOIDCRealm(ctx, c.Origin, private, lookup, c.OIDCConfig, &http.Client{Timeout: 4 * time.Second})
	cancel()
	clear(lookup)
	if err != nil {
		return err
	}
	server := http.Server{
		Addr: c.Listen, Handler: realm.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 6 * time.Second,
		WriteTimeout: 12 * time.Second, IdleTimeout: 15 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13},
	}
	return server.ListenAndServeTLS(c.Certificate, c.CertificateKey)
}

func readConfig(path string) (config, error) {
	var c config
	unavailable := errors.New("registered identity configuration unavailable")
	raw, err := privatefile.Read(path, 1, 1<<20)
	if err != nil {
		return c, unavailable
	}
	defer clear(raw)
	if !utf8.Valid(raw) {
		return c, unavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	// Decode through a pointer so a JSON null is rejected as well.
	value := &c
	if decoder.Decode(&value) != nil || value == nil || decoder.Decode(new(any)) != io.EOF {
		return config{}, unavailable
	}
	return c, nil
}

func privateSeed(path string) ([]byte, error) {
	raw, err := privatefile.Read(path, ed25519.SeedSize, ed25519.SeedSize)
	if err != nil {
		return nil, errors.New("private identity key unavailable")
	}
	return raw, nil
}
