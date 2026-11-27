package records

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (s *Service) erase(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	if !ok || actor.Role != "private-user" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	if r.Method == http.MethodGet && id == "" {
		s.listErasureReceipts(w, r, actor, nil)
		return
	}
	if r.Method == http.MethodPost {
		var input struct {
			Object   string `json:"object,omitempty"`
			Kind     string `json:"kind,omitempty"`
			Request  string `json:"requestKey,omitempty"`
			LocalAck bool   `json:"localDeviceErased,omitempty"`
			Before   string `json:"before,omitempty"`
			Proof    string `json:"installationProof,omitempty"`
		}
		if !strictBody(w, r, 1024, &input) || (input.Kind != "" &&
			((input.Kind != "review" && input.Kind != "visit-claim") || id != "" || input.Before != "" || input.LocalAck || input.Proof != "")) {
			reply(w, 400, map[string]string{"error": "invalid_erasure"})
			return
		}
		var commitment []byte
		if input.Proof != "" {
			proof, err := hex.DecodeString(input.Proof)
			if err != nil || !fingerprint.MatchString(input.Proof) {
				reply(w, 400, map[string]string{"error": "invalid_erasure"})
				return
			}
			digest := sha256.Sum256(proof)
			clear(proof)
			commitment = digest[:]
		}
		if input.Before != "" {
			if id != "" || !uuid.MatchString(input.Before) || input.Object != "" || input.Request != "" || input.LocalAck || commitment != nil {
				reply(w, 400, map[string]string{"error": "invalid_erasure"})
				return
			}
			s.listErasureReceipts(w, r, actor, &input.Before)
			return
		}
		if id != "" {
			if !uuid.MatchString(id) || !input.LocalAck || input.Object != "" || input.Request != "" {
				reply(w, 400, map[string]string{"error": "invalid_erasure"})
				return
			}
			var accepted bool
			var err error
			if commitment != nil {
				err = s.mutationRow(r, actor, s.pool, `SELECT travel_private.mark_device_erased($1,$2,$3)`,
					actor.Subject, id, commitment).Scan(&accepted)
			} else {
				err = s.mutationRow(r, actor, s.pool, `SELECT travel_private.mark_device_erased($1,$2)`,
					actor.Subject, id).Scan(&accepted)
			}
			if err != nil {
				authFailure(w, err)
				return
			}
			if !accepted {
				http.NotFound(w, r)
				return
			}
		} else {
			if !uuid.MatchString(input.Object) || !uuid.MatchString(input.Request) || input.LocalAck {
				reply(w, 400, map[string]string{"error": "invalid_erasure"})
				return
			}
			var err error
			if input.Kind != "" {
				err = s.mutationRow(r, actor, s.pool, `SELECT travel_private.request_visit_erasure($1,$2,$3,$4)::text`,
					actor.Subject, input.Request, input.Object, input.Kind).Scan(&id)
			} else if commitment != nil {
				err = s.mutationRow(r, actor, s.pool, `SELECT travel_private.request_selected_erasure($1,$2,$3,$4)::text`,
					actor.Subject, input.Request, input.Object, commitment).Scan(&id)
			} else {
				err = s.mutationRow(r, actor, s.pool, `SELECT travel_private.request_selected_erasure($1,$2,$3)::text`,
					actor.Subject, input.Request, input.Object).Scan(&id)
			}
			if err != nil {
				authFailure(w, err)
				return
			}
		}
	}
	if !uuid.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	var status struct {
		ID              string    `json:"request"`
		State           string    `json:"state"`
		LocalAck        bool      `json:"localDeviceAck"`
		LocalEvidence   string    `json:"localDeviceEvidence"`
		ProviderPending bool      `json:"providerPending"`
		BackupExpiry    time.Time `json:"backupExpiry"`
	}
	status.ID = id
	err := s.pool.QueryRow(ctx, `SELECT status,local_device_ack,provider_pending,backup_expiry,local_device_evidence
		FROM travel_private.selected_erasure_status_v2($1,$2)
		WHERE ($3 OR backup_expiry>statement_timestamp())`, actor.Subject, id, r.Method == http.MethodPost).
		Scan(&status.State, &status.LocalAck, &status.ProviderPending, &status.BackupExpiry, &status.LocalEvidence)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		authFailure(w, err)
		return
	}
	reply(w, http.StatusOK, status)
}

func (s *Service) listErasureReceipts(w http.ResponseWriter, r *http.Request, actor Principal, before *string) {
	type receipt struct {
		Request   string    `json:"request"`
		Requested time.Time `json:"requestedAt"`
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text,requested_at FROM travel_private.list_erasure_receipts($1,$2)`, actor.Subject, before)
	if err != nil {
		authFailure(w, err)
		return
	}
	history, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (receipt, error) {
		var item receipt
		err := row.Scan(&item.Request, &item.Requested)
		return item, err
	})
	if err != nil {
		authFailure(w, err)
		return
	}
	if current, ok := s.principal(r); !ok || current != actor {
		http.NotFound(w, r)
		return
	}
	var next string
	if len(history) > 50 {
		history = history[:50]
		next = history[len(history)-1].Request
	}
	if history == nil {
		history = []receipt{}
	}
	reply(w, http.StatusOK, struct {
		Receipts []receipt `json:"receipts"`
		Next     string    `json:"next,omitempty"`
	}{history, next})
}

type ErasureWorker struct{ workload, journal *pgxpool.Pool }

func OpenErasureWorker(ctx context.Context, primary, owner string) (*ErasureWorker, error) {
	if primary == "" || owner == "" || primary == owner {
		return nil, errors.New("distinct workload and erasure database capabilities required")
	}
	w, err := pgxpool.New(ctx, primary)
	if err != nil {
		return nil, errors.New("erasure workload unavailable")
	}
	j, err := pgxpool.New(ctx, owner)
	if err != nil {
		w.Close()
		return nil, errors.New("independent erasure journal unavailable")
	}
	var localDB, remoteDB, localService, remoteService string
	var allowed, appendOnly bool
	err = w.QueryRow(ctx, `SELECT current_database(),inline.server_id,
		pg_has_role(current_user,'nu54_p09_erasure_worker','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p09_private_user','MEMBER')
		AND NOT has_schema_privilege('travel_erasure','USAGE')
		AND NOT has_table_privilege('travel_private.record_object','SELECT,DELETE')
		AND NOT EXISTS(SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))
		FROM (SELECT coalesce(inet_server_addr()::text,'socket') server_id) inline`).Scan(&localDB, &localService, &allowed)
	e2 := j.QueryRow(ctx, `SELECT current_database(),coalesce(inet_server_addr()::text,'socket'),
		pg_has_role(current_user,'nu54_p09_erasure_append','USAGE')
		AND NOT has_schema_privilege('travel_private','USAGE')
		AND NOT has_table_privilege('travel_erasure.fence','SELECT,INSERT,DELETE')
		AND NOT EXISTS(SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).
		Scan(&remoteDB, &remoteService, &appendOnly)
	if err != nil || e2 != nil || !allowed || !appendOnly || (localDB == remoteDB && localService == remoteService) {
		w.Close()
		j.Close()
		return nil, errors.New("separated erasure capabilities unavailable")
	}
	return &ErasureWorker{w, j}, nil
}
func (w *ErasureWorker) Close() { w.workload.Close(); w.journal.Close() }
func (w *ErasureWorker) Step(ctx context.Context) error {
	type target struct {
		actor, id, kind, key string
		cutoff               time.Time
	}
	rows, err := w.workload.Query(ctx, `SELECT subject_ref::text,request_id::text,kind,target::text,cutoff FROM travel_private.pending_erasure_targets()`)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (target, error) {
		var item target
		err := row.Scan(&item.actor, &item.id, &item.kind, &item.key, &item.cutoff)
		return item, err
	})
	if err != nil || len(items) == 0 {
		return err
	}
	var namespace string
	var through int64
	seq := make([]int64, 0, len(items))
	err = pgx.BeginFunc(ctx, w.journal, func(tx pgx.Tx) error {
		for _, item := range items {
			var pos int64
			err := tx.QueryRow(ctx, `SELECT travel_erasure.append_fence($1,'object',NULL,NULL,$2,$3,$4,$5)`,
				item.actor, item.kind, item.key, item.cutoff, item.cutoff.AddDate(0, 0, 91)).Scan(&pos)
			if err != nil {
				return err
			}
			seq = append(seq, pos)
		}
		return tx.QueryRow(ctx, `SELECT namespace::text,high_water FROM travel_erasure.writer_cursor()`).Scan(&namespace, &through)
	})
	if err != nil {
		return err
	}
	var accepted bool
	return w.workload.QueryRow(ctx, `SELECT travel_private.finish_selected_erasure($1,$2,$3,$4,$5)`,
		items[0].actor, items[0].id, namespace, seq, through).Scan(&accepted)
}
