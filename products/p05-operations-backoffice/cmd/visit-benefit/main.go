package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
var issuer = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type packet struct {
	Document  string `json:"document"`
	Signature string `json:"signature"`
}
type assertion struct {
	Issuer         string    `json:"issuer"`
	Proof          string    `json:"proof"`
	Revision       int64     `json:"revision"`
	Classification string    `json:"classification"`
	Expires        time.Time `json:"expiresAt"`
}

func verify(data []byte, key ed25519.PublicKey) (assertion, error) {
	var result assertion
	var envelope packet
	if json.Unmarshal(data, &envelope) != nil {
		return result, errors.New("invalid benefit envelope")
	}
	body, e1 := base64.StdEncoding.Strict().DecodeString(envelope.Document)
	sig, e2 := base64.StdEncoding.Strict().DecodeString(envelope.Signature)
	if e1 != nil || e2 != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, append([]byte("NU54-visit-benefit-v1\x00"), body...), sig) {
		return result, errors.New("benefit signature unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, errors.New("invalid benefit assertion")
	}
	if _, err := decoder.Token(); err != io.EOF || !issuer.MatchString(result.Issuer) || !uuid.MatchString(result.Proof) ||
		result.Revision < 1 || !result.Expires.After(time.Now()) {
		return result, errors.New("invalid benefit assertion")
	}
	return result, nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	authority := flag.String("authority", "", "independently approved P09 benefit verification key")
	campaign := flag.String("campaign", "", "opaque approved campaign identifier")
	flag.Parse()
	if !uuid.MatchString(*campaign) {
		return errors.New("approved benefit campaign required")
	}
	data, err := os.ReadFile(*authority)
	if err != nil || len(data) > 128 {
		return errors.New("benefit authority not enrolled")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("invalid benefit authority")
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 6145))
	if err != nil || len(raw) > 6144 {
		return errors.New("invalid benefit envelope")
	}
	proof, err := verify(raw, key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	dsn := os.Getenv("P05_BENEFIT_DATABASE_URL")
	if dsn == "" {
		return errors.New("independent benefit database capability required")
	}
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return errors.New("benefit database unavailable")
	}
	defer db.Close(ctx)
	var allowed bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p05_benefit_worker','USAGE')
	   AND NOT has_table_privilege('ledger.payment','SELECT,INSERT,UPDATE,DELETE')
	   AND NOT has_schema_privilege('ledger','CREATE')
	   AND NOT EXISTS(SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed)
	if err != nil || !allowed {
		return errors.New("independent benefit capability unavailable")
	}
	var applied bool
	err = db.QueryRow(ctx, `SELECT ledger.apply_visit_benefit($1,$2,$3,$4,$5,$6)`, proof.Issuer, proof.Proof, *campaign, proof.Revision, proof.Classification, proof.Expires).Scan(&applied)
	if err != nil {
		return errors.New("benefit assertion did not apply")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]bool{"effect": applied})
}
