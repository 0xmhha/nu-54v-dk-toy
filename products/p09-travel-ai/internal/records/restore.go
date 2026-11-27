package records

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const fenceDomain = "NU54-P09-fences-v1\x00"

type Fence struct {
	Sequence    string    `json:"sequence"`
	Kind        string    `json:"kind"`
	Subject     string    `json:"subject"`
	Purpose     *string   `json:"purpose,omitempty"`
	Revision    *string   `json:"throughRevision,omitempty"`
	ObjectKind  *string   `json:"objectKind,omitempty"`
	ObjectToken *string   `json:"objectToken,omitempty"`
	Cutoff      time.Time `json:"cutoff"`
	Expires     time.Time `json:"expiresAt"`
}

type Page struct {
	Namespace string  `json:"namespace"`
	HighWater string  `json:"highWater"`
	After     string  `json:"after"`
	Through   string  `json:"through"`
	Entries   []Fence `json:"entries"`
}

type PageEnvelope struct {
	Page
	Signature string `json:"signature"`
}

type RestoreStatus struct {
	Namespace      string `json:"namespace"`
	AppliedThrough string `json:"appliedThrough"`
	HighWater      string `json:"highWater"`
	Ready          bool   `json:"ready"`
}

func sequence(text string) (int64, error) {
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n < 0 || strconv.FormatInt(n, 10) != text {
		return 0, errors.New("invalid monotone cursor")
	}
	return n, nil
}

func (f Fence) valid() bool {
	if !uuid.MatchString(f.Subject) || f.Cutoff.IsZero() || f.Expires.Sub(f.Cutoff) < 90*24*time.Hour {
		return false
	}
	switch f.Kind {
	case "purpose":
		if f.Purpose == nil || !purposes[*f.Purpose] || f.Revision == nil || f.ObjectKind != nil || f.ObjectToken != nil {
			return false
		}
		r, err := sequence(*f.Revision)
		return err == nil && r > 0
	case "object":
		if f.Purpose != nil || f.Revision != nil || f.ObjectKind == nil || f.ObjectToken == nil || !uuid.MatchString(*f.ObjectToken) {
			return false
		}
		switch *f.ObjectKind {
		case "record", "job", "review", "itinerary", "visit-claim":
			return true
		}
	}
	return false
}

func (p Page) signedBytes() ([]byte, error) {
	raw, err := json.Marshal(p)
	return append([]byte(fenceDomain), raw...), err
}

type Exporter struct {
	pool   *pgxpool.Pool
	key    ed25519.PrivateKey
	grants *Service
}

func OpenExporter(ctx context.Context, dsn string, seed []byte, grants map[string]Principal) (*Exporter, error) {
	if len(seed) != ed25519.SeedSize || len(grants) == 0 {
		return nil, errors.New("invalid journal signer or enrollment")
	}
	for digest, actor := range grants {
		if !validRestoreGrant(digest, actor) {
			return nil, errors.New("invalid restore grant")
		}
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.New("journal database unavailable")
	}
	var allowed bool
	err = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p09_erasure_export','USAGE')
	    AND NOT pg_has_role(current_user,'nu54_p09_erasure_append','MEMBER')
	    AND NOT has_schema_privilege('travel_private','USAGE')
	    AND NOT has_table_privilege('travel_erasure.fence','INSERT,UPDATE,DELETE')
	    AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed)
	if err != nil || !allowed {
		pool.Close()
		return nil, errors.New("journal export capability unavailable")
	}
	return &Exporter{pool: pool, key: ed25519.NewKeyFromSeed(seed), grants: &Service{grants: grants}}, nil
}
func (e *Exporter) Close() { e.pool.Close() }

func validRestoreGrant(digest string, actor Principal) bool {
	return fingerprint.MatchString(digest) && uuid.MatchString(actor.Subject) && actor.Role == "restore" && actor.ExpiresAt > 0 && actor.Issuer == ""
}

func (e *Exporter) UseGrantFile(path string) { e.grants.UseGrantFile(path, true) }

func (e *Exporter) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/restore/fences", e.grants.authorized(e.fences))
}
func (e *Exporter) fences(w http.ResponseWriter, r *http.Request) {
	actor, ok := e.grants.principal(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query()
	after, err := sequence(query.Get("after"))
	if err != nil || len(query) != 1 || len(query["after"]) != 1 {
		http.NotFound(w, r)
		return
	}
	p, err := e.export(r.Context(), after)
	if err != nil {
		authFailure(w, err)
		return
	}
	raw, err := p.signedBytes()
	if err != nil {
		authFailure(w, err)
		return
	}
	if !e.grants.stillAuthorized(r, actor) {
		http.NotFound(w, r)
		return
	}
	reply(w, http.StatusOK, PageEnvelope{Page: p, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(e.key, raw))})
}

func (e *Exporter) export(ctx context.Context, after int64) (Page, error) {
	p := Page{After: strconv.FormatInt(after, 10), Entries: []Fence{}}
	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	var high int64
	if err := tx.QueryRow(ctx, `SELECT namespace::text,high_water FROM travel_erasure.namespace`).Scan(&p.Namespace, &high); err != nil {
		return p, err
	}
	if after > high {
		return p, errors.New("cursor outside journal")
	}
	p.HighWater = strconv.FormatInt(high, 10)
	rows, err := tx.Query(ctx, `SELECT sequence::text,kind,subject_ref::text,purpose,through_revision::text,
		object_kind,object_token::text,cutoff,expire_after FROM travel_erasure.fence WHERE sequence>$1 ORDER BY sequence LIMIT 500`, after)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	through := after
	for rows.Next() {
		var f Fence
		if err := rows.Scan(&f.Sequence, &f.Kind, &f.Subject, &f.Purpose, &f.Revision, &f.ObjectKind, &f.ObjectToken, &f.Cutoff, &f.Expires); err != nil {
			return p, err
		}
		through++
		if strconv.FormatInt(through, 10) != f.Sequence || !f.valid() {
			return p, errors.New("journal discontinuity")
		}
		f.Cutoff, f.Expires = f.Cutoff.UTC(), f.Expires.UTC()
		p.Entries = append(p.Entries, f)
	}
	if err := rows.Err(); err != nil {
		return p, err
	}
	if len(p.Entries) < 500 && through != high {
		return p, errors.New("journal discontinuity")
	}
	p.Through = strconv.FormatInt(through, 10)
	return p, tx.Commit(ctx)
}

type Applier struct {
	pool      *pgxpool.Pool
	key       ed25519.PublicKey
	namespace string
	high      int64
	grants    *Service
}

func OpenApplier(ctx context.Context, dsn, namespace, high string, key ed25519.PublicKey, grants map[string]Principal) (*Applier, error) {
	h, err := sequence(high)
	if err != nil || !uuid.MatchString(namespace) || len(key) != ed25519.PublicKeySize || len(grants) == 0 {
		return nil, errors.New("invalid restore gate")
	}
	for digest, actor := range grants {
		if !validRestoreGrant(digest, actor) {
			return nil, errors.New("invalid restore grant")
		}
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.New("restore database unavailable")
	}
	var permitted bool
	err = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p09_restore_apply','USAGE')
	    AND NOT pg_has_role(current_user,'nu54_p09_private_user','MEMBER')
	    AND NOT has_schema_privilege('travel_erasure','USAGE')
	    AND NOT has_table_privilege('travel_private.consent_decision','SELECT,INSERT,UPDATE,DELETE')
	    AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&permitted)
	if err != nil || !permitted {
		pool.Close()
		return nil, errors.New("restore apply capability unavailable")
	}
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(900022)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO travel_private.restore_checkpoint(singleton,namespace) VALUES(true,$1) ON CONFLICT DO NOTHING`, namespace); err != nil {
			return err
		}
		var stored string
		var applied int64
		if err := tx.QueryRow(ctx, `SELECT namespace::text,applied_through FROM travel_private.restore_checkpoint FOR UPDATE`).Scan(&stored, &applied); err != nil {
			return err
		}
		if stored != namespace || applied > h {
			return errors.New("restore startup checkpoint differs")
		}
		return nil
	})
	if err != nil {
		pool.Close()
		return nil, errors.New("cannot establish restore quarantine")
	}
	return &Applier{pool: pool, key: key, namespace: namespace, high: h, grants: &Service{grants: grants}}, nil
}
func (a *Applier) Close()                   { a.pool.Close() }
func (a *Applier) UseGrantFile(path string) { a.grants.UseGrantFile(path, true) }

func (a *Applier) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/restore/apply", a.grants.authorized(a.httpApply))
}
func (a *Applier) httpApply(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.grants.principal(r)
	if !ok || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	var payload PageEnvelope
	if !strictBody(w, r, 580_000, &payload) {
		reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_fence_batch"})
		return
	}
	status, err := a.apply(r.Context(), payload, func() bool { return a.grants.stillAuthorized(r, actor) })
	if err != nil {
		reply(w, http.StatusConflict, map[string]string{"error": "restore_gate_conflict"})
		return
	}
	reply(w, http.StatusOK, status)
}
func (a *Applier) apply(ctx context.Context, p PageEnvelope, authorized func() bool) (RestoreStatus, error) {
	high, e1 := sequence(p.HighWater)
	after, e2 := sequence(p.After)
	through, e3 := sequence(p.Through)
	message, e4 := p.Page.signedBytes()
	signature, e5 := base64.StdEncoding.Strict().DecodeString(p.Signature)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || p.Namespace != a.namespace || high != a.high || through > high ||
		through < after || through-after != int64(len(p.Entries)) || len(p.Entries) > 500 || !ed25519.Verify(a.key, message, signature) {
		return RestoreStatus{}, errors.New("invalid authenticated restore batch")
	}
	for index, entry := range p.Entries {
		if !entry.valid() || entry.Sequence != strconv.FormatInt(after+int64(index)+1, 10) {
			return RestoreStatus{}, errors.New("invalid ordered restore fence")
		}
	}
	var actual int64
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if !authorized() {
			return errGrantChanged
		}
		if _, err := tx.Exec(ctx, `INSERT INTO travel_private.restore_checkpoint(singleton,namespace) VALUES(true,$1) ON CONFLICT DO NOTHING`, a.namespace); err != nil {
			return err
		}
		var stored string
		if err := tx.QueryRow(ctx, `SELECT namespace::text,applied_through FROM travel_private.restore_checkpoint FOR UPDATE`).Scan(&stored, &actual); err != nil {
			return err
		}
		if stored != a.namespace || after > actual || actual > high {
			return errors.New("restore cursor discontinuity")
		}
		for index, entry := range p.Entries {
			cursor := after + int64(index) + 1
			if cursor <= actual {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO travel_private.restore_fence VALUES($1,$2,$3,$4,$5::text::bigint,$6,$7,$8,$9)`,
				cursor, entry.Subject, entry.Kind, entry.Purpose, entry.Revision, entry.ObjectKind, entry.ObjectToken, entry.Cutoff, entry.Expires); err != nil {
				return err
			}
		}
		if actual < through {
			actual = through
			if _, err := tx.Exec(ctx, `UPDATE travel_private.restore_checkpoint SET applied_through=$1`, actual); err != nil {
				return err
			}
		}
		if !authorized() {
			return errGrantChanged
		}
		return nil
	})
	return RestoreStatus{Namespace: a.namespace, AppliedThrough: strconv.FormatInt(actual, 10), HighWater: p.HighWater, Ready: err == nil && actual == high}, err
}
