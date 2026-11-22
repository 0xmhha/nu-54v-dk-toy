package records

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
var recipientName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var fingerprint = regexp.MustCompile(`^[a-f0-9]{64}$`)
var purposes = map[string]bool{
	"location": true, "voice": true, "transcription": true, "recommendation": true, "review": true, "challenge": true,
}

type Principal struct {
	Subject   string `json:"subject"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"expiresAt"`
	Issuer    string `json:"issuer,omitempty"`
}

type Decision struct {
	Purpose   string     `json:"purpose"`
	Expected  int64      `json:"expectedRevision"`
	Policy    int        `json:"policyVersion"`
	Recipient string     `json:"recipient"`
	Object    *string    `json:"object,omitempty"`
	Accept    *bool      `json:"accept"`
	Expires   *time.Time `json:"expiresAt,omitempty"`
	Request   string     `json:"requestKey"`
}

func (d Decision) valid() bool {
	return purposes[d.Purpose] && d.Expected >= 0 && d.Expected < 9007199254740991 && d.Policy > 0 &&
		recipientName.MatchString(d.Recipient) && (d.Object == nil || uuid.MatchString(*d.Object)) &&
		d.Accept != nil && *d.Accept == (d.Expires != nil) && uuid.MatchString(d.Request)
}

type Receipt struct {
	Purpose   string    `json:"purpose"`
	Revision  int64     `json:"revision"`
	Policy    int       `json:"policyVersion"`
	Recipient string    `json:"recipient"`
	Object    *string   `json:"object,omitempty"`
	Accepted  bool      `json:"accepted"`
	Decided   time.Time `json:"decidedAt"`
	Expires   time.Time `json:"expiresAt"`
	Ref       string    `json:"receipt"`
}

type Service struct {
	benefitRead *benefitReader
	pool        *pgxpool.Pool
	grants      map[string]Principal
	lookup      func(string) (Principal, bool)
	visitPool   *pgxpool.Pool
	visitTrust  map[string]VisitIssuer
	content     *vault
	review      *vault
	catalog     catalogLoader
	benefit     *BenefitSigner
	ranker      *Ranker
	clock       func() time.Time
}

func Open(ctx context.Context, database string, grants map[string]Principal) (*Service, error) {
	if len(grants) == 0 {
		return nil, errors.New("private travel enrollment required")
	}
	for digest, principal := range grants {
		if !validPrivateGrant(digest, principal) {
			return nil, errors.New("invalid private travel enrollment")
		}
	}
	config, err := pgxpool.ParseConfig(database)
	if err != nil {
		return nil, errors.New("private travel database unavailable")
	}
	config.ConnConfig.RuntimeParams["statement_timeout"] = "4s"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("private travel database unavailable")
	}
	var permitted bool
	err = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p09_private_user','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p09_public','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p09_publisher','MEMBER')
		AND NOT has_schema_privilege('travel_private','CREATE')
		AND NOT has_table_privilege('travel_private.consent_decision','SELECT,INSERT,UPDATE,DELETE')
		AND EXISTS (SELECT FROM pg_roles WHERE rolname=current_user
		AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls OR rolreplication))`).Scan(&permitted)
	if err != nil || !permitted {
		pool.Close()
		return nil, errors.New("private travel database capability unavailable")
	}
	return &Service{pool: pool, grants: grants}, nil
}

func (s *Service) Close() {
	if s.benefit != nil {
		s.benefit.pool.Close()
		clear(s.benefit.key)
	}
	if s.visitPool != nil {
		s.visitPool.Close()
	}
	s.pool.Close()
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

func (s *Service) principal(r *http.Request) (Principal, bool) {
	actor, _, ok := s.principalAt(r, s.now())
	return actor, ok
}

func (s *Service) Register(mux *http.ServeMux) {
	register := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, s.authorized(handler))
	}
	register("GET /v1/private/consents", s.consents)
	register("POST /v1/private/consents", s.consents)
	register("POST /v1/issuer/visit-evidence", s.ingestVisit)
	register("GET /v1/issuer/benefit/{proof}", s.benefitAssertion)
	register("POST /v1/private/visits", s.visits)
	register("GET /v1/private/visits", s.visits)
	register("GET /v1/private/records", s.records)
	register("GET /v1/private/records/{id}", s.records)
	register("POST /v1/private/records", s.records)
	register("POST /v1/private/transcriptions", s.queue)
	register("GET /v1/private/erasure", s.erase)
	register("POST /v1/private/erasure", s.erase)
	register("POST /v1/private/erasure/{id}", s.erase)
	register("GET /v1/private/erasure/{id}", s.erase)
	register("POST /v1/private/reviews", s.reviewWrite)
	register("GET /v1/private/itineraries/{id}", s.itinerary)
	register("POST /v1/private/itineraries/preview", s.itinerary)
	register("POST /v1/private/itineraries/apply", s.itinerary)
	register("POST /v1/private/itineraries/generate", s.generateItinerary)
}

func reply(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func authFailure(w http.ResponseWriter, err error) {
	var db *pgconn.PgError
	if errors.As(err, &db) {
		switch db.Code {
		case "P1801":
			reply(w, http.StatusConflict, map[string]string{"error": "stale_or_conflicting_intent"})
			return
		case "42501", "P0002":
			reply(w, http.StatusNotFound, map[string]string{"error": "private_scope_unavailable"})
			return
		}
	}
	reply(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
}

func (s *Service) consents(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	if !ok || actor.Role != "private-user" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	if r.Method == http.MethodPost {
		var command Decision
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6144))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&command); err != nil || !command.valid() {
			reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_consent"})
			return
		}
		if _, err := decoder.Token(); err != io.EOF {
			reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_consent"})
			return
		}
		if current, ok := s.principal(r); !ok || current != actor {
			http.NotFound(w, r)
			return
		}
		var ref string
		err := s.mutationRow(r, actor, s.pool, `SELECT travel_private.decide($1,$2,$3,$4,$5,$6,$7,$8,$9)::text`,
			actor.Subject, command.Purpose, command.Expected, command.Policy, command.Recipient, command.Object,
			*command.Accept, command.Expires, command.Request).Scan(&ref)
		if err != nil {
			authFailure(w, err)
			return
		}
		reply(w, http.StatusOK, map[string]string{"receipt": ref})
		return
	}
	rows, err := s.pool.Query(ctx, `SELECT purpose,revision,policy_version,recipient,selected_object::text,accepted,
		decided_at,expires_at,receipt::text FROM travel_private.current_decisions($1)`, actor.Subject)
	if err != nil {
		authFailure(w, err)
		return
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Receipt, error) {
		var value Receipt
		err := row.Scan(&value.Purpose, &value.Revision, &value.Policy, &value.Recipient, &value.Object, &value.Accepted,
			&value.Decided, &value.Expires, &value.Ref)
		return value, err
	})
	if err != nil {
		authFailure(w, err)
		return
	}
	if entries == nil {
		entries = []Receipt{}
	}
	reply(w, http.StatusOK, map[string][]Receipt{"consents": entries})
}
