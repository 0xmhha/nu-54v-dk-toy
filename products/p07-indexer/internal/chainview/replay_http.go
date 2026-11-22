package chainview

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type replayStatus struct {
	ID          string  `json:"id"`
	State       string  `json:"state"`
	Comparison  *string `json:"comparison,omitempty"`
	Reason      *string `json:"reason,omitempty"`
	Anchor      int64   `json:"anchor"`
	AnchorHash  string  `json:"anchorHash"`
	Revision    int64   `json:"revision"`
	Replayed    *int64  `json:"replayedHeight,omitempty"`
	Difference  *int64  `json:"differingLogs,omitempty"`
	PaidDelta   *string `json:"paidDelta,omitempty"`
	RefundDelta *string `json:"refundDelta,omitempty"`
}

func (s *OperatorStore) EnableReplayRequests(ctx context.Context, url string) error {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	var allowed bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p07_replay_request','USAGE')
		AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND
		  (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication))
		AND NOT EXISTS (SELECT FROM unnest(ARRAY['nu54_p07_switch','nu54_p07_ingest','nu54_p07_incident',
		  'nu54_p07_explorer','nu54_p07_read','nu54_p07_legacy','nu54_p07_reconcile','nu54_p07_public']) role_name
		  WHERE pg_has_role(current_user,role_name,'MEMBER'))
		AND NOT has_schema_privilege('chainview','CREATE')
		AND NOT has_table_privilege('chainview.generation','INSERT,UPDATE,DELETE')`).Scan(&allowed)
	if err != nil || !allowed {
		db.Close()
		return errors.New("replay request requires its isolated database role")
	}
	s.requests = db
	return nil
}

func (s *OperatorStore) listReplays(w http.ResponseWriter, r *http.Request, lookup func(string) (OperatorGrant, bool)) {
	stream := r.URL.Query().Get("stream")
	grant, expiry, ok := operatorAt(r, lookup, time.Now())
	if !ok || !uuidPattern.MatchString(stream) || !slices.Contains(grant.Streams, stream) {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), expiry)
	defer cancel()
	var revision *int64
	err := s.db.QueryRow(ctx, `SELECT revision FROM chainview.explorer_read_revision WHERE stream_id=$1`, stream).Scan(&revision)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "replay unavailable", http.StatusServiceUnavailable)
		return
	}
	rows, err := s.db.Query(ctx, `SELECT id::text,state,comparison_id::text,reason,anchor_height,anchor_hash,expected_revision,
		replayed_height,differing_logs,paid_delta::text,refund_delta::text
		FROM chainview.explorer_replay_status WHERE stream_id=$1 ORDER BY requested_at DESC,id LIMIT 20`, stream)
	if err != nil {
		http.Error(w, "replay unavailable", http.StatusServiceUnavailable)
		return
	}
	jobs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (replayStatus, error) {
		var item replayStatus
		err := row.Scan(&item.ID, &item.State, &item.Comparison, &item.Reason, &item.Anchor, &item.AnchorHash, &item.Revision,
			&item.Replayed, &item.Difference, &item.PaidDelta, &item.RefundDelta)
		return item, err
	})
	current, _, ok := operatorAt(r, lookup, time.Now())
	if !ok || !slices.Contains(current.Streams, stream) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "replay unavailable", http.StatusServiceUnavailable)
		return
	}
	explorerJSON(w, struct {
		Jobs           []replayStatus `json:"jobs"`
		Revision       *int64         `json:"revision"`
		RequestAllowed bool           `json:"requestAllowed"`
	}{jobs, revision, s.requests != nil && slices.Contains(current.Replay, stream)})
}

func (s *OperatorStore) startReplay(w http.ResponseWriter, r *http.Request, lookup func(string) (OperatorGrant, bool)) {
	var request struct {
		Stream   string `json:"stream"`
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if s.requests == nil {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" ||
		(r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "https://"+r.Host) {
		http.Error(w, "invalid replay request", http.StatusBadRequest)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF ||
		!uuidPattern.MatchString(request.Stream) || !uuidPattern.MatchString(request.ID) || request.Revision < 1 {
		http.Error(w, "invalid replay request", http.StatusBadRequest)
		return
	}
	grant, expiry, ok := operatorAt(r, lookup, time.Now())
	if !ok || !slices.Contains(grant.Streams, request.Stream) || !slices.Contains(grant.Replay, request.Stream) {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), expiry)
	defer cancel()
	tx, err := s.requests.Begin(ctx)
	if err != nil {
		http.Error(w, "replay unavailable", http.StatusServiceUnavailable)
		return
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	_, err = tx.Exec(ctx, "SELECT chainview.start_replay($1,$2,$3,$4)", request.Stream, request.ID, request.Revision, expiry)
	grant, _, ok = operatorAt(r, lookup, time.Now())
	if !ok || !slices.Contains(grant.Streams, request.Stream) || !slices.Contains(grant.Replay, request.Stream) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "replay cannot start at this revision", http.StatusConflict)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "replay result uncertain", http.StatusServiceUnavailable)
		return
	}
	explorerJSON(w, map[string]string{"id": request.ID})
}
