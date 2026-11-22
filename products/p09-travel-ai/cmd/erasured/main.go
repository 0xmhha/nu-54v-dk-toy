package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/records"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	addr := flag.String("listen", "127.0.0.1:8101", "independent erasure mTLS listener")
	cert := flag.String("cert", "", "server TLS certificate PEM")
	key := flag.String("key", "", "server TLS private key PEM")
	ca := flag.String("client-ca", "", "restore client CA PEM")
	grants := flag.String("grants", "", "approved restore clients")
	signer := flag.String("journal-signer", "", "base64-encoded raw Ed25519 seed file")
	flag.Parse()
	database := os.Getenv("P09_ERASURE_DATABASE_URL")
	if database == "" || *cert == "" || *key == "" || *ca == "" || *grants == "" || *signer == "" {
		return errors.New("independent journal configuration required")
	}
	roots := x509.NewCertPool()
	root, err := os.ReadFile(*ca)
	if err != nil || !roots.AppendCertsFromPEM(root) {
		return errors.New("restore client CA unavailable")
	}
	enrollments, err := records.GrantFile(*grants).Read()
	if err != nil {
		return errors.New("restore enrollments unavailable")
	}
	info, err := os.Stat(*signer)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("private journal signer file required")
	}
	raw, err := os.ReadFile(*signer)
	if err != nil {
		return errors.New("journal signer unavailable")
	}
	seed, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return errors.New("invalid journal signer")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exporter, err := records.OpenExporter(ctx, database, seed, enrollments)
	clear(seed)
	if err != nil {
		return err
	}
	defer exporter.Close()
	exporter.UseGrantFile(*grants)
	mux := http.NewServeMux()
	exporter.Register(mux)
	server := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}}
	return server.ListenAndServeTLS(*cert, *key)
}
