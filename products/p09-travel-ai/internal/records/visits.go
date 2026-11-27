package records

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VisitIssuer struct {
	Key         string `json:"key"`
	Environment string `json:"environment"`
}

type visit struct {
	Issuer   string    `json:"issuer"`
	Proof    string    `json:"proof"`
	Subject  string    `json:"subject"`
	Place    string    `json:"place"`
	Version  int64     `json:"revision"`
	State    string    `json:"state"`
	Observed time.Time `json:"observedAt"`
}

var placeCode = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)

type signedVisit struct {
	Document  string `json:"document"`
	Signature string `json:"signature"`
}

func (i VisitIssuer) publicKey() (ed25519.PublicKey, error) {
	key, err := base64.StdEncoding.Strict().DecodeString(i.Key)
	if err != nil || len(key) != ed25519.PublicKeySize || (i.Environment != "synthetic" && i.Environment != "live") {
		return nil, errors.New("invalid visit issuer")
	}
	return ed25519.PublicKey(key), nil
}

func (i VisitIssuer) verify(envelope signedVisit) (visit, error) {
	var v visit
	key, err := i.publicKey()
	doc, e := base64.StdEncoding.Strict().DecodeString(envelope.Document)
	proof, s := base64.StdEncoding.Strict().DecodeString(envelope.Signature)
	if err != nil || e != nil || s != nil || len(doc) > 3072 || !ed25519.Verify(key, append([]byte("NU54-visit-assertion-v1\x00"), doc...), proof) {
		return v, errors.New("visit proof unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(doc))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&v); err != nil {
		return v, err
	}
	if _, err := decoder.Token(); err != io.EOF || !recipientName.MatchString(v.Issuer) || !uuid.MatchString(v.Proof) ||
		!uuid.MatchString(v.Subject) || !placeCode.MatchString(v.Place) || v.Version < 1 || v.Version > 9007199254740991 ||
		(v.State != "settled" && v.State != "reorg" && v.State != "refunded") || v.Observed.IsZero() {
		return v, errors.New("visit proof unavailable")
	}
	return v, nil
}

func (s *Service) EnableVisitIngest(ctx context.Context, dsn string, trust map[string]VisitIssuer) error {
	if dsn == "" || len(trust) == 0 {
		return errors.New("visit issuer trust and capability required")
	}
	for name, issuer := range trust {
		if !recipientName.MatchString(name) {
			return errors.New("invalid visit issuer name")
		}
		if _, err := issuer.publicKey(); err != nil {
			return err
		}
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("visit issuer database unavailable")
	}
	var allowed bool
	err = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p09_visit_issuer','USAGE')
	    AND NOT pg_has_role(current_user,'nu54_p09_private_user','MEMBER')
	    AND NOT has_schema_privilege('travel_private','CREATE')
	    AND NOT has_table_privilege('travel_private.visit_evidence','SELECT,INSERT,UPDATE,DELETE')
	    AND EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND NOT(rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed)
	if err != nil || !allowed {
		pool.Close()
		return errors.New("visit issuer capability unavailable")
	}
	s.visitPool, s.visitTrust = pool, trust
	return nil
}

func strictBody(w http.ResponseWriter, r *http.Request, limit int64, value any) bool {
	reader := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	reader.DisallowUnknownFields()
	if err := reader.Decode(value); err != nil {
		return false
	}
	_, err := reader.Token()
	return err == io.EOF
}

func (s *Service) ingestVisit(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	if !ok || actor.Role != "visit-issuer" || s.visitPool == nil || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	trust, ok := s.visitTrust[actor.Issuer]
	var payload signedVisit
	if !ok || !strictBody(w, r, 6144, &payload) {
		http.NotFound(w, r)
		return
	}
	item, err := trust.verify(payload)
	current, authorized := s.principal(r)
	if err != nil || item.Issuer != actor.Issuer || !authorized || current != actor {
		http.NotFound(w, r)
		return
	}
	var applied bool
	err = s.mutationRow(r, actor, s.visitPool, `SELECT travel_private.put_visit($1,$2,$3,$4,$5,$6,$7,$8)`,
		item.Issuer, item.Proof, item.Subject, item.Place, trust.Environment, item.Version, item.State, item.Observed).Scan(&applied)
	if err != nil {
		authFailure(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]bool{"applied": applied})
}

type VisitClaim struct {
	Issuer   string `json:"issuer"`
	Proof    string `json:"proof"`
	Place    string `json:"place"`
	Purpose  string `json:"purpose"`
	Revision int64  `json:"consentRevision"`
}
type VisitStatus struct {
	Issuer         string `json:"issuer"`
	Proof          string `json:"proof"`
	Place          string `json:"place"`
	Purpose        string `json:"purpose"`
	Claim          string `json:"claim"`
	Classification string `json:"classification"`
}

type VisitOption struct {
	continuation string
	ClaimRef     *string        `json:"claimRef,omitempty"`
	PlaceName    string         `json:"placeName,omitempty"`
	ReviewState  *string        `json:"reviewState,omitempty"`
	Benefit      *BenefitStatus `json:"benefit,omitempty"`
	Issuer       string         `json:"issuer"`
	Proof        string         `json:"proof"`
	Place        string         `json:"place"`
	Observed     time.Time      `json:"observedAt"`
	Environment  string         `json:"environment"`
	State        string         `json:"state"`
	Challenge    string         `json:"challenge"`
	Review       *string        `json:"review,omitempty"`
}

type VisitExperience struct {
	BenefitAvailable bool          `json:"benefitAvailable"`
	Next             string        `json:"next,omitempty"`
	Options          []VisitOption `json:"options"`
	ChallengeCount   int           `json:"challengeCount"`
}

func (s *Service) visits(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	if !ok || actor.Role != "private-user" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		var command struct {
			VisitClaim
			Projection      string `json:"projection,omitempty"`
			Cursor          string `json:"cursor,omitempty"`
			IncludeBenefits bool   `json:"includeBenefits,omitempty"`
		}
		if !strictBody(w, r, 4096, &command) {
			reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_claim"})
			return
		}
		if command.Projection != "" {
			if command.Projection != "options" || command.Issuer != "" || command.Proof != "" || command.Purpose != "" || command.Revision != 0 ||
				(command.Place != "" && !placeCode.MatchString(command.Place)) || (command.Cursor != "" && !fingerprint.MatchString(command.Cursor)) {
				reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_visit_projection"})
				return
			}
			s.visitPage(w, r, actor, command.Place, command.Cursor, command.IncludeBenefits)
			return
		}
		if command.Cursor != "" || command.IncludeBenefits || !recipientName.MatchString(command.Issuer) || !uuid.MatchString(command.Proof) ||
			!placeCode.MatchString(command.Place) || command.Revision < 1 || command.Revision > 9007199254740991 ||
			(command.Purpose != "review" && command.Purpose != "challenge") {
			reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_claim"})
			return
		}
		var id string
		err := s.mutationRow(r, actor, s.pool, `SELECT travel_private.claim_visit($1,$2,$3,$4,$5,$6)::text`,
			actor.Subject, command.Issuer, command.Proof, command.Purpose, command.Revision, command.Place).Scan(&id)
		if err != nil {
			authFailure(w, err)
			return
		}
		reply(w, http.StatusOK, map[string]string{"claim": id})
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT issuer,proof::text,place_id,purpose,claim::text,classification FROM travel_private.my_visits($1)`, actor.Subject)
	if err != nil {
		authFailure(w, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (VisitStatus, error) {
		var v VisitStatus
		err := row.Scan(&v.Issuer, &v.Proof, &v.Place, &v.Purpose, &v.Claim, &v.Classification)
		return v, err
	})
	if err != nil {
		authFailure(w, err)
		return
	}
	reply(w, http.StatusOK, map[string][]VisitStatus{"visits": list})
}

func (s *Service) visitPage(w http.ResponseWriter, r *http.Request, actor Principal, place, cursor string, includeBenefits bool) {
	const size = 16
	ctx := r.Context()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		authFailure(w, err)
		return
	}
	defer tx.Rollback(ctx)
	options, err := tx.Query(ctx, `SELECT issuer,proof::text,place_id,observed_at,environment,state,challenge,claim_id::text,review_id::text,review_state,continuation
		FROM travel_private.visit_option_page($1,nullif($2,''),nullif($3,''))`, actor.Subject, place, cursor)
	if err != nil {
		authFailure(w, err)
		return
	}
	choices, err := pgx.CollectRows(options, func(row pgx.CollectableRow) (VisitOption, error) {
		var v VisitOption
		err := row.Scan(&v.Issuer, &v.Proof, &v.Place, &v.Observed, &v.Environment, &v.State, &v.Challenge, &v.ClaimRef, &v.Review, &v.ReviewState, &v.continuation)
		return v, err
	})
	if err != nil {
		authFailure(w, err)
		return
	}
	var next string
	if len(choices) > size {
		choices = choices[:size]
		next = choices[size-1].continuation
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT travel_private.challenge_count($1)`, actor.Subject).Scan(&count); err != nil {
		authFailure(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		authFailure(w, err)
		return
	}
	if s.catalog != nil && len(choices) != 0 {
		if catalog, err := s.catalog(ctx); err == nil {
			names := make(map[string]string, len(catalog.Places))
			for _, place := range catalog.Places {
				names[place.ID] = place.Name
			}
			for i := range choices {
				choices[i].PlaceName = names[choices[i].Place]
			}
		}
	}
	reply(w, http.StatusOK, VisitExperience{Options: choices, Next: next, ChallengeCount: count, BenefitAvailable: includeBenefits && s.benefitRead.attach(r.Context(), choices)})
}
