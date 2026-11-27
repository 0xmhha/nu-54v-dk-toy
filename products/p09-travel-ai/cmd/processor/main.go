package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/records"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	endpoint := flag.String("endpoint", "", "approved HTTPS recipient work endpoint")
	recipient := flag.String("recipient", "", "approved recipient ID matching consent")
	pin := flag.String("pin", "", "recipient TLS leaf SPKI SHA-256")
	ca := flag.String("ca", "", "approved recipient CA")
	cert := flag.String("cert", "", "P09 client certificate")
	key := flag.String("key", "", "P09 client TLS private key")
	flag.Parse()
	if os.Getenv("P09_PROCESSOR_DATABASE_URL") == "" {
		return errors.New("processing database capability required")
	}
	caRaw, err := os.ReadFile(*ca)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(caRaw) {
		return errors.New("approved processor CA unavailable")
	}
	identity, err := tls.LoadX509KeyPair(*cert, *key)
	if err != nil {
		return errors.New("P09 processing identity unavailable")
	}
	secret, err := records.LoadKey(os.Getenv("P09_CONTENT_KEY"))
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	worker, err := records.OpenWorker(ctx, os.Getenv("P09_PROCESSOR_DATABASE_URL"), *endpoint, *recipient, *pin, roots, identity, secret)
	clear(secret)
	if err != nil {
		return err
	}
	defer worker.Close()
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for ctx.Err() == nil {
		if err := worker.Step(ctx); err != nil {
			log.Print("processing remains pending")
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
		}
	}
	return nil
}
