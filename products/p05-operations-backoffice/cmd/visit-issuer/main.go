package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ref = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
var label = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)

type enrollment struct {
	Issuer   string    `json:"issuer"`
	Tenant   string    `json:"tenant"`
	Branch   string    `json:"branch"`
	Customer string    `json:"customer"`
	Subject  string    `json:"subject"`
	Place    string    `json:"place"`
	Proof    string    `json:"proof"`
	Sale     string    `json:"sale"`
	Expires  time.Time `json:"expiresAt"`
}
type visitAssertion struct {
	Issuer   string    `json:"issuer"`
	Proof    string    `json:"proof"`
	Subject  string    `json:"subject"`
	Place    string    `json:"place"`
	Revision int64     `json:"revision"`
	State    string    `json:"state"`
	Observed time.Time `json:"observedAt"`
}

func (e enrollment) valid(now time.Time) bool {
	return label.MatchString(e.Issuer) && label.MatchString(e.Place) && ref.MatchString(e.Tenant) && ref.MatchString(e.Branch) &&
		ref.MatchString(e.Customer) && ref.MatchString(e.Subject) && ref.MatchString(e.Proof) && ref.MatchString(e.Sale) && e.Expires.After(now)
}
func loadPrivate(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("private issuance file required")
	}
	return os.ReadFile(path)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	profilePath := flag.String("enrollment", "", "independently approved one-purpose customer, P09 subject, place and sale binding")
	signerPath := flag.String("signer", "", "private base64 Ed25519 seed file")
	flag.Parse()
	input, err := loadPrivate(*profilePath)
	if err != nil {
		return err
	}
	var profile enrollment
	dec := json.NewDecoder(strings.NewReader(string(input)))
	dec.DisallowUnknownFields()
	if len(input) > 4096 || dec.Decode(&profile) != nil || !profile.valid(time.Now()) {
		return errors.New("visit enrollment unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	dsn, source := os.Getenv("P05_VISIT_DATABASE_URL"), os.Getenv("P07_VISIT_EVIDENCE_DATABASE_URL")
	if dsn == "" || source == "" {
		return errors.New("both independent visit evidence capabilities are required")
	}
	p05, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return errors.New("P05 visit lookup unavailable")
	}
	defer p05.Close(ctx)
	p07, err := pgx.Connect(ctx, source)
	if err != nil {
		return errors.New("P07 active evidence unavailable")
	}
	defer p07.Close(ctx)
	assertion, err := issue(ctx, p05, p07, profile)
	if err != nil {
		return err
	}
	if !profile.Expires.After(time.Now()) {
		return errors.New("visit enrollment expired")
	}
	key, err := loadPrivate(*signerPath)
	if err != nil {
		return err
	}
	seed, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(key)))
	clear(key)
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("visit signer seed invalid")
	}
	private := ed25519.NewKeyFromSeed(seed)
	clear(seed)
	defer clear(private)
	body, err := json.Marshal(assertion)
	if err != nil {
		return err
	}
	data := append([]byte("NU54-visit-assertion-v1\x00"), body...)
	return json.NewEncoder(os.Stdout).Encode(map[string]string{
		"document":  base64.StdEncoding.EncodeToString(body),
		"signature": base64.StdEncoding.EncodeToString(ed25519.Sign(private, data)),
	})
}

func issue(ctx context.Context, p05, p07 *pgx.Conn, profile enrollment) (visitAssertion, error) {
	var allowed bool
	if err := p05.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p05_visit_issuer','USAGE')
		AND NOT has_table_privilege('ledger.payment','SELECT,UPDATE,DELETE')
		AND NOT has_schema_privilege('ledger','CREATE')
		AND NOT EXISTS(SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed); err != nil || !allowed {
		return visitAssertion{}, errors.New("restricted P05 issuer capability required")
	}
	a := visitAssertion{Issuer: profile.Issuer, Proof: profile.Proof, Subject: profile.Subject, Place: profile.Place}
	var stream, merchant, order, transaction string
	if err := p05.QueryRow(ctx, `SELECT stream::text,merchant,order_key,transaction_ref,state,source_revision,observed_at
       FROM ledger.visit_origin($1,$2,$3,$4)`, profile.Tenant, profile.Branch, profile.Customer, profile.Sale).
		Scan(&stream, &merchant, &order, &transaction, &a.State, &a.Revision, &a.Observed); err != nil {
		return visitAssertion{}, errors.New("finalized original payment unavailable")
	}
	var present bool
	if err := p07.QueryRow(ctx, `SELECT chainview.reconciliation_ready($1,$2), EXISTS(
       SELECT FROM chainview.reconciliation_receipt($1,$2,$3) WHERE kind='paid' AND tx_hash=$4)`, stream, merchant, order, transaction).Scan(&allowed, &present); err != nil || !allowed {
		return visitAssertion{}, errors.New("active finalized visit evidence unavailable")
	}
	if a.Revision < 1 || a.Revision >= 4503599627370495 {
		return visitAssertion{}, errors.New("visit revision unavailable")
	}
	a.Revision *= 2
	if !present {
		a.Revision++
		a.State = "reorg"
	}
	a.Observed = a.Observed.UTC()
	return a, nil
}
