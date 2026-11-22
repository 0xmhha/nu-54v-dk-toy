package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var issuerPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var proofPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

type evidence struct {
	Issuer string `json:"issuer"`
	Proof  string `json:"proof"`
}
type status struct {
	Credited   int64 `json:"credited"`
	Adjusted   int64 `json:"adjusted"`
	Uncredited int64 `json:"uncredited"`
}

func handler(pool *pgxpool.Pool, reader []byte) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/visit-benefits/status", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		fingerprint := sha256.Sum256(r.TLS.VerifiedChains[0][0].Raw)
		if subtle.ConstantTimeCompare(fingerprint[:], reader) != 1 {
			http.NotFound(w, r)
			return
		}
		var items []evidence
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&items) != nil || len(items) == 0 || len(items) > 100 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := decoder.Token(); err != io.EOF {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var batch pgx.Batch
		for _, item := range items {
			if !issuerPattern.MatchString(item.Issuer) || !proofPattern.MatchString(item.Proof) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			batch.Queue(`SELECT credited,adjusted,uncredited FROM ledger.visit_benefit_status($1,$2)`, item.Issuer, item.Proof)
		}
		results := pool.SendBatch(r.Context(), &batch)
		defer results.Close()
		states := make([]status, len(items))
		for i := range states {
			if err := results.QueryRow().Scan(&states[i].Credited, &states[i].Adjusted, &states[i].Uncredited); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		if err := results.Close(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(states)
	})
	return mux
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	addr := flag.String("listen", "127.0.0.1:8108", "P05 benefit status mTLS listener")
	cert := flag.String("cert", "", "server certificate PEM")
	key := flag.String("key", "", "server private key PEM")
	clientCA := flag.String("client-ca", "", "P09 reader certificate authority PEM")
	pin := flag.String("reader-sha256", "", "enrolled P09 reader client certificate DER digest in hex")
	flag.Parse()
	reader, err := hex.DecodeString(*pin)
	if err != nil || len(reader) != sha256.Size || *cert == "" || *key == "" {
		return errors.New("benefit TLS identity and enrolled reader required")
	}
	ca, err := os.ReadFile(*clientCA)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		return errors.New("benefit client authority unavailable")
	}
	dsn := os.Getenv("P05_BENEFIT_READ_DATABASE_URL")
	if dsn == "" {
		return errors.New("benefit reader database capability required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("benefit reader database unavailable")
	}
	defer pool.Close()
	var allowed bool
	err = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p05_benefit_read','USAGE')
        AND NOT pg_has_role(current_user,'nu54_p05_benefit_worker','MEMBER')
        AND NOT has_schema_privilege('ledger','CREATE')
        AND NOT has_table_privilege('ledger.visit_benefit_enrollment','SELECT,INSERT,UPDATE,DELETE')
        AND NOT has_table_privilege('ledger.visit_benefit_state','SELECT,INSERT,UPDATE,DELETE')
        AND NOT has_table_privilege('ledger.visit_benefit_effect','SELECT,INSERT,UPDATE,DELETE')
        AND NOT has_table_privilege('ledger.payment','SELECT,INSERT,UPDATE,DELETE')
        AND NOT EXISTS(SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed)
	if err != nil || !allowed {
		return errors.New("independent benefit reader capability unavailable")
	}
	server := &http.Server{Addr: *addr, Handler: handler(pool, reader), ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}}
	return server.ListenAndServeTLS(*cert, *key)
}
