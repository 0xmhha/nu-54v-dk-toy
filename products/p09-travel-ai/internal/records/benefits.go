package records

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BenefitAssertion struct {
	Issuer         string    `json:"issuer"`
	Proof          string    `json:"proof"`
	Revision       int64     `json:"revision"`
	Classification string    `json:"classification"`
	Expires        time.Time `json:"expiresAt"`
}
type BenefitSigner struct {
	pool *pgxpool.Pool
	key  ed25519.PrivateKey
}

func (s *Service) EnableBenefitIssuer(ctx context.Context, dsn string, seed []byte) error {
	if dsn == "" || len(seed) != ed25519.SeedSize {
		return errors.New("independent benefit authority required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	var allowed bool
	err = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p09_benefit_assert','USAGE')
	 AND NOT pg_has_role(current_user,'nu54_p09_private_user','MEMBER')
	 AND NOT has_table_privilege('travel_private.visit_evidence','SELECT,INSERT,UPDATE,DELETE')
	 AND NOT EXISTS(SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed)
	if err != nil || !allowed {
		pool.Close()
		return errors.New("independent benefit assertion role required")
	}
	s.benefit = &BenefitSigner{pool, ed25519.NewKeyFromSeed(seed)}
	return nil
}
func (s *Service) benefitAssertion(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	proof := r.PathValue("proof")
	if !ok || actor.Role != "benefit-worker" || s.benefit == nil || !uuid.MatchString(proof) || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	a := BenefitAssertion{Issuer: actor.Issuer, Proof: proof}
	err := s.benefit.pool.QueryRow(r.Context(), `SELECT revision,classification FROM travel_private.assert_visit_benefit($1,$2)`,
		a.Issuer, proof).Scan(&a.Revision, &a.Classification)
	if err != nil {
		authFailure(w, err)
		return
	}
	signedAt := s.now()
	current, deadline, ok := s.principalAt(r, signedAt)
	if !ok || current != actor {
		http.NotFound(w, r)
		return
	}
	a.Expires = signedAt.Add(20 * time.Second)
	if deadline.Before(a.Expires) {
		a.Expires = deadline
	}
	// The ledger stores timestamptz at microsecond precision. Round down to preserve the deadline.
	a.Expires = a.Expires.UTC().Truncate(time.Microsecond)
	if !s.now().Before(a.Expires) {
		http.NotFound(w, r)
		return
	}
	doc, err := json.Marshal(a)
	if err != nil {
		authFailure(w, err)
		return
	}
	packed := append([]byte("NU54-visit-benefit-v1\x00"), doc...)
	signature := ed25519.Sign(s.benefit.key, packed)
	if !s.now().Before(a.Expires) {
		http.NotFound(w, r)
		return
	}
	reply(w, http.StatusOK, signedVisit{Document: base64.StdEncoding.EncodeToString(doc),
		Signature: base64.StdEncoding.EncodeToString(signature)})
}
