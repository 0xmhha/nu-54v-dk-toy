package records

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReviewInput struct {
	ID       string `json:"id"`
	Issuer   string `json:"issuer"`
	Proof    string `json:"proof"`
	Place    string `json:"place"`
	Revision int64  `json:"consentRevision"`
	Publish  *bool  `json:"publish"`
	Body     string `json:"body"`
	Inspect  bool   `json:"inspect,omitempty"`
}

func (s *Service) EnableReviewKey(key []byte) error {
	storage, err := newVault(key)
	if err != nil {
		return err
	}
	s.review = storage
	return nil
}
func (s *Service) reviewWrite(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	if !ok || actor.Role != "private-user" || s.review == nil || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	var req ReviewInput
	if !strictBody(w, r, 8192, &req) || (req.ID != "" && !uuid.MatchString(req.ID)) || !uuid.MatchString(req.Proof) || !recipientName.MatchString(req.Issuer) ||
		!placeCode.MatchString(req.Place) {
		reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_review"})
		return
	}
	if req.Inspect {
		s.reviewRead(w, r, actor, req)
		return
	}
	if req.Revision < 1 || req.Revision > 9007199254740991 || req.Publish == nil ||
		!utf8.ValidString(req.Body) || strings.TrimSpace(req.Body) == "" || len(req.Body) > 4000 {
		reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_review"})
		return
	}
	body := []byte(req.Body)
	defer clear(body)
	var published bool
	err := s.mutate(r, actor, s.pool, func(tx pgx.Tx) error {
		if req.ID == "" {
			var existing *string
			var environment string
			if err := tx.QueryRow(r.Context(), `SELECT id::text,environment FROM travel_private.review_intent($1,$2,$3,$4,$5)`,
				actor.Subject, req.Issuer, req.Proof, req.Place, req.Revision).Scan(&existing, &environment); err != nil {
				return err
			}
			if existing != nil {
				req.ID = *existing
			} else {
				id := s.review.dedup("review-identity:"+actor.Subject+":"+req.Issuer, []byte(req.Proof))
				id[6] = id[6]&15 | 64
				id[8] = id[8]&63 | 128
				req.ID = fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:16])
			}
			if environment == "synthetic" {
				value := false
				req.Publish = &value
			}
		}
		scope := "review:" + req.ID
		sealed, err := s.review.seal(scope, body)
		if err != nil {
			return err
		}
		return tx.QueryRow(r.Context(), `SELECT travel_private.write_review($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			actor.Subject, req.ID, req.Issuer, req.Proof, req.Place, req.Revision, *req.Publish, s.review.dedup(scope, body), sealed).Scan(&published)
	})
	if err != nil {
		authFailure(w, err)
		return
	}
	reply(w, http.StatusOK, struct {
		ID        string `json:"review"`
		Published bool   `json:"published"`
	}{req.ID, published})
}

func (s *Service) reviewRead(w http.ResponseWriter, r *http.Request, actor Principal, req ReviewInput) {
	var id string
	var packet []byte
	err := s.pool.QueryRow(r.Context(), `SELECT id::text,body FROM travel_private.own_review($1,$2,$3,$4)`,
		actor.Subject, req.Issuer, req.Proof, req.Place).Scan(&id, &packet)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		authFailure(w, err)
		return
	}
	body, err := s.review.open("review:"+id, packet)
	defer clear(body)
	if err != nil {
		authFailure(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]string{"review": id, "body": string(body)})
}

type ReviewReader struct {
	pool   *pgxpool.Pool
	review *vault
}

func OpenReviewReader(ctx context.Context, dsn string, key []byte) (*ReviewReader, error) {
	secret, err := newVault(key)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.New("published review source unavailable")
	}
	var allowed bool
	err = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p09_review_read','USAGE')
	    AND NOT pg_has_role(current_user,'nu54_p09_private_user','MEMBER')
	    AND NOT has_table_privilege('travel_private.record_object','SELECT,INSERT,UPDATE,DELETE')
	    AND NOT has_table_privilege('travel_private.review','SELECT,INSERT,UPDATE,DELETE')
	    AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed)
	if err != nil || !allowed {
		pool.Close()
		return nil, errors.New("published review reader is unapproved")
	}
	return &ReviewReader{pool, secret}, nil
}
func (r *ReviewReader) Close() { r.pool.Close() }
func (r *ReviewReader) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/reviews/{place}", r.read)
}
func (r *ReviewReader) RegisterGuarded(mux *http.ServeMux, guard *PublicationGuard) {
	mux.Handle("GET /v1/reviews/{place}", guard.Handler(http.HandlerFunc(r.read)))
}
func (reader *ReviewReader) read(w http.ResponseWriter, r *http.Request) {
	place := r.PathValue("place")
	if !placeCode.MatchString(place) || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	rows, err := reader.pool.Query(r.Context(), `SELECT review_id::text,body,badge FROM travel_private.public_review_details($1)`, place)
	if err != nil {
		authFailure(w, err)
		return
	}
	defer rows.Close()
	type review struct {
		ID    string `json:"id"`
		Body  string `json:"body"`
		Badge string `json:"badge"`
	}
	entries := []review{}
	for rows.Next() {
		var id, badge string
		var packet []byte
		if err := rows.Scan(&id, &packet, &badge); err != nil {
			authFailure(w, err)
			return
		}
		item, err := reader.review.open("review:"+id, packet)
		if err != nil {
			authFailure(w, err)
			return
		}
		entries = append(entries, review{id, string(item), badge})
		clear(item)
	}
	if err := rows.Err(); err != nil {
		authFailure(w, err)
		return
	}
	reply(w, http.StatusOK, map[string][]review{"reviews": entries})
}
