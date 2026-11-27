package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/places"
	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/records"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	addr := flag.String("listen", "127.0.0.1:8099", "private travel mTLS listener")
	cert := flag.String("cert", "", "server certificate PEM")
	key := flag.String("key", "", "server private key PEM")
	root := flag.String("client-ca", "", "independently approved private travel client CA PEM")
	grantFile := flag.String("grants", "", "certificate DER SHA-256 mapped to P09-only grants")
	flag.Parse()
	database := os.Getenv("P09_PRIVATE_DATABASE_URL")
	if *cert == "" || *key == "" || *root == "" || *grantFile == "" {
		return errors.New("private travel database, TLS identity, client authority and grants are required")
	}
	clientCA, err := os.ReadFile(*root)
	if err != nil {
		return errors.New("cannot read approved private travel client CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(clientCA) {
		return errors.New("invalid private travel client CA")
	}
	grants, err := records.GrantFile(*grantFile).Read()
	if err != nil {
		return errors.New("invalid private travel grants")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mux := http.NewServeMux()
	var handler http.Handler = mux
	if os.Getenv("P09_RESTORE_QUARANTINE") == "true" {
		encoded, err := os.ReadFile(os.Getenv("P09_RESTORE_JOURNAL_KEY"))
		if err != nil {
			return errors.New("restore journal public key unavailable")
		}
		key, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(encoded)))
		if err != nil {
			return errors.New("restore journal public key invalid")
		}
		applier, err := records.OpenApplier(ctx, os.Getenv("P09_RESTORE_DATABASE_URL"),
			os.Getenv("P09_RESTORE_NAMESPACE"), os.Getenv("P09_RESTORE_HIGH_WATER"), ed25519.PublicKey(key), grants)
		if err != nil {
			return err
		}
		defer applier.Close()
		applier.UseGrantFile(*grantFile)
		applier.Register(mux)
	} else {
		service, err := records.Open(ctx, database, grants)
		if err != nil {
			return err
		}
		defer service.Close()
		service.UseGrantFile(*grantFile, false)
		if trustFile := os.Getenv("P09_VISIT_TRUST"); trustFile != "" {
			data, err := os.ReadFile(trustFile)
			if err != nil {
				return errors.New("visit trust unavailable")
			}
			var trust map[string]records.VisitIssuer
			if len(data) > 1<<20 || json.Unmarshal(data, &trust) != nil {
				return errors.New("invalid visit trust")
			}
			if err := service.EnableVisitIngest(ctx, os.Getenv("P09_VISIT_DATABASE_URL"), trust); err != nil {
				return err
			}
		}
		if signingKey := os.Getenv("P09_BENEFIT_SIGNING_KEY"); signingKey != "" {
			key, err := records.LoadKey(signingKey)
			if err != nil {
				return err
			}
			err = service.EnableBenefitIssuer(ctx, os.Getenv("P09_BENEFIT_DATABASE_URL"), key)
			clear(key)
			if err != nil {
				return err
			}
		}
		if keyPath := os.Getenv("P09_CONTENT_KEY"); keyPath != "" {
			info, err := os.Stat(keyPath)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
				return errors.New("private record key file required")
			}
			blob, err := os.ReadFile(keyPath)
			if err != nil {
				return errors.New("content key unavailable")
			}
			key, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(blob)))
			clear(blob)
			if err != nil {
				return errors.New("content key invalid")
			}
			err = service.EnableVault(key)
			clear(key)
			if err != nil {
				return err
			}
		}
		if reviewKey := os.Getenv("P09_REVIEW_KEY"); reviewKey != "" {
			key, err := records.LoadKey(reviewKey)
			if err != nil {
				return err
			}
			err = service.EnableReviewKey(key)
			clear(key)
			if err != nil {
				return err
			}
		}
		if catalogDB := os.Getenv("P09_APPROVED_CATALOG_DATABASE_URL"); catalogDB != "" {
			keyText, err := os.ReadFile(os.Getenv("P09_APPROVED_CATALOG_TRUST_KEY"))
			if err != nil {
				return errors.New("approved public catalog anchor required")
			}
			key, err := hex.DecodeString(strings.TrimSpace(string(keyText)))
			if err != nil || len(key) != ed25519.PublicKeySize {
				return errors.New("invalid public catalog anchor")
			}
			catalog, err := places.Connect(ctx, catalogDB, false)
			if err != nil {
				return err
			}
			defer catalog.Close()
			service.EnableCatalog(func(ctx context.Context) (places.Catalog, error) {
				return catalog.Published(ctx, ed25519.PublicKey(key))
			})
		}
		if localEndpoint := os.Getenv("P09_LOCAL_AI_ENDPOINT"); localEndpoint != "" {
			ranker, err := records.OpenLocalRanker(localEndpoint, os.Getenv("P09_LOCAL_AI_MODEL"))
			if err != nil {
				return err
			}
			service.EnableLocalRanker(ranker)
		}
		if endpoint := os.Getenv("P09_BENEFIT_STATUS_ENDPOINT"); endpoint != "" {
			err := service.EnableBenefitReader(endpoint, os.Getenv("P09_BENEFIT_STATUS_CA"),
				os.Getenv("P09_BENEFIT_STATUS_CERT"), os.Getenv("P09_BENEFIT_STATUS_KEY"))
			if err != nil {
				return err
			}
		}
		service.Register(mux)
		guard, err := service.RestorePublication(ctx, records.RestorePublicationConfig{
			ControlDSN:      os.Getenv("P09_RECOVERY_DATABASE_URL"),
			ErasureGuardDSN: os.Getenv("P09_LIVE_ERASURE_GUARD_DATABASE_URL"),
			Scope:           os.Getenv("P09_RECOVERY_SCOPE"), Manifest: os.Getenv("P09_RECOVERY_MANIFEST"),
		})
		if err != nil {
			return err
		}
		defer guard.Close()
		handler = guard.Handler(mux)
	}
	srv := &http.Server{Addr: *addr, Handler: handler, ReadTimeout: 5 * time.Second, ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}}
	return srv.ListenAndServeTLS(*cert, *key)
}
