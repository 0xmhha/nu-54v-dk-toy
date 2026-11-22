package chainview

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

//go:embed web
var explorerAssets embed.FS

func explorerPage(operators bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file := "web/index.html"
		if operators {
			file = "web/operations.html"
		}
		webHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page, _ := explorerAssets.ReadFile(file)
		_, _ = w.Write(page)
	}
}

func explorerStatic(w http.ResponseWriter, r *http.Request) {
	webHeaders(w)
	http.FileServer(http.FS(explorerAssets)).ServeHTTP(w, r)
}

func webHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none';script-src 'self';style-src 'self';connect-src 'self';base-uri 'none';frame-ancestors 'none';form-action 'self'")
}

type Deployment struct {
	Stream           string     `json:"stream"`
	Chain            string     `json:"chain"`
	Contract         string     `json:"contract"`
	Version          int        `json:"version"`
	Asset            string     `json:"asset"`
	Tip              *int64     `json:"tip"`
	Finalized        *int64     `json:"finalized"`
	ReportedFinality *int64     `json:"reportedFinality"`
	ObservedAt       *time.Time `json:"observedAt"`
	Status           string     `json:"status"`
}

func (d *Deployment) destinations() []any {
	return []any{&d.Stream, &d.Chain, &d.Contract, &d.Version, &d.Asset,
		&d.Tip, &d.Finalized, &d.ReportedFinality, &d.ObservedAt, &d.Status}
}

const deploymentFields = "stream_id::text,chain_ref,contract_ref,version,asset_ref,tip_height,finalized_height,observed_finalized_height,observed_at,status"

func (s *PublicStore) deployments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	rows, err := s.db.Query(ctx, "SELECT "+deploymentFields+" FROM chainview.explorer_deployment ORDER BY chain_ref,version DESC,contract_ref LIMIT 64")
	if err != nil {
		http.Error(w, "deployment view unavailable", http.StatusServiceUnavailable)
		return
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Deployment, error) {
		var deployment Deployment
		err := row.Scan(deployment.destinations()...)
		return deployment, err
	})
	if err != nil {
		http.Error(w, "deployment view unavailable", http.StatusServiceUnavailable)
		return
	}
	explorerJSON(w, items)
}

type readRevision struct {
	Revision *int64
	Status   string
}

func captureReadRevision(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, stream string) (readRevision, error) {
	var out readRevision
	err := db.QueryRow(ctx, "SELECT revision,status FROM chainview.explorer_read_revision WHERE stream_id=$1", stream).
		Scan(&out.Revision, &out.Status)
	return out, err
}

func sameReadRevision(before, after readRevision) bool {
	if before.Status == "held" || after.Status == "held" || (before.Revision == nil) != (after.Revision == nil) {
		return false
	}
	return before.Revision == nil || *before.Revision == *after.Revision
}

type OperatorGrant struct {
	Streams   []string            `json:"streams"`
	Merchants map[string][]string `json:"merchants,omitempty"` // stream to permitted public merchant keys
	ExpiresAt int64               `json:"expiresAt"`
	Replay    []string            `json:"replay,omitempty"`
}

type OperatorStore struct {
	db          *pgxpool.Pool
	requests    *pgxpool.Pool
	QuoteOrigin *QuoteOriginReader
}

func OpenOperator(ctx context.Context, url string) (*OperatorStore, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	var safe bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p07_explorer','USAGE')
		AND NOT EXISTS(SELECT FROM pg_roles WHERE rolname=current_user AND
		     (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication))
		AND NOT EXISTS(SELECT FROM unnest(ARRAY['nu54_p07_public','nu54_p07_reconcile','nu54_p07_read',
		     'nu54_p07_ingest','nu54_p07_legacy','nu54_p07_switch','nu54_p07_incident','nu54_p07_replay_request']) r
		     WHERE pg_has_role(current_user,r,'MEMBER'))
		AND NOT has_schema_privilege('chainview','CREATE')
		AND NOT has_table_privilege('chainview.observed_event','SELECT,INSERT,UPDATE,DELETE')`).Scan(&safe)
	if err != nil || !safe {
		db.Close()
		return nil, errors.New("operations explorer requires its read-only database capability")
	}
	return &OperatorStore{db: db}, nil
}

func (s *OperatorStore) Close() {
	s.db.Close()
	if s.requests != nil {
		s.requests.Close()
	}
}

func (s *OperatorStore) Handler(grants map[string]OperatorGrant) http.Handler {
	return s.HandlerWithLookup(func(key string) (OperatorGrant, bool) {
		grant, ok := grants[key]
		return grant, ok
	})
}

func (s *OperatorStore) HandlerWithLookup(lookup func(string) (OperatorGrant, bool)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", explorerPage(true))
	mux.HandleFunc("GET /web/explorer.js", explorerStatic)
	mux.HandleFunc("GET /web/explorer.css", explorerStatic)
	mux.HandleFunc("GET /v2/explorer/monitor", func(w http.ResponseWriter, r *http.Request) {
		s.monitor(w, r, lookup)
	})
	boundary := func(w http.ResponseWriter, r *http.Request) { s.boundary(w, r, lookup) }
	mux.HandleFunc("GET /v2/explorer/boundary", boundary)
	mux.HandleFunc("POST /v2/explorer/boundary", boundary)
	mux.HandleFunc("GET /v2/explorer/replays", func(w http.ResponseWriter, r *http.Request) {
		s.listReplays(w, r, lookup)
	})
	mux.HandleFunc("POST /v2/explorer/replays", func(w http.ResponseWriter, r *http.Request) {
		s.startReplay(w, r, lookup)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := operatorAt(r, lookup, time.Now()); !ok {
			http.NotFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func operatorAt(r *http.Request, lookup func(string) (OperatorGrant, bool), now time.Time) (OperatorGrant, time.Time, bool) {
	peer, ok := service.CurrentVerifiedClient(r.TLS, now)
	if !ok {
		return OperatorGrant{}, time.Time{}, false
	}
	grant, ok := lookup(peer.FingerprintHex())
	expiry := time.Unix(grant.ExpiresAt, 0)
	if peer.ExpiresAt.Before(expiry) {
		expiry = peer.ExpiresAt
	}
	return grant, expiry, ok && len(grant.Streams) != 0 && now.Before(expiry)
}

type observationStatus struct {
	Deployment
	Proposed     *string    `json:"proposedGeneration,omitempty"`
	ABI          *string    `json:"abiHash,omitempty"`
	HoldReason   *string    `json:"holdReason,omitempty"`
	Base         *int64     `json:"base"`
	TipHash      *string    `json:"tipHash"`
	FinalHash    *string    `json:"finalHash"`
	Ancestor     *int64     `json:"ancestor"`
	ForkAt       *time.Time `json:"forkAt"`
	Incident     *string    `json:"incident"`
	Disputed     *int64     `json:"disputed"`
	PreviousHash *string    `json:"previousHash"`
	NextHash     *string    `json:"nextHash"`
	OpenedAt     *time.Time `json:"openedAt"`
}

func (s *OperatorStore) monitor(w http.ResponseWriter, r *http.Request, lookup func(string) (OperatorGrant, bool)) {
	grant, expiry, ok := operatorAt(r, lookup, time.Now())
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx, stop := context.WithDeadline(r.Context(), expiry)
	defer stop()
	rows, err := s.db.Query(ctx, "SELECT "+deploymentFields+`,base_height,tip_hash,finalized_hash,
		fork_ancestor,fork_observed_at,incident_ref::text,disputed_height,old_hash,new_hash,opened_at,
		NULL::text AS proposed_generation,NULL::text AS abi_hash,NULL::text AS hold_reason
		FROM chainview.explorer_monitor WHERE stream_id=ANY($1::text[]::uuid[])
		UNION ALL SELECT stream_id::text,chain_ref,contract_ref,version,asset_ref,
		  NULL,NULL,NULL,observed_at,'unmapped',base_height,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,
		  generation_id::text,abi_hash,reason
		FROM chainview.profile_hold WHERE stream_id=ANY($1::text[]::uuid[])
		ORDER BY chain_ref,version DESC,contract_ref,status LIMIT 64`, grant.Streams)
	if err != nil {
		http.Error(w, "monitor unavailable", http.StatusServiceUnavailable)
		return
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (observationStatus, error) {
		var item observationStatus
		err := row.Scan(append(item.destinations(), &item.Base, &item.TipHash, &item.FinalHash,
			&item.Ancestor, &item.ForkAt, &item.Incident, &item.Disputed, &item.PreviousHash,
			&item.NextHash, &item.OpenedAt, &item.Proposed, &item.ABI, &item.HoldReason)...)
		return item, err
	})
	active, _, ok := operatorAt(r, lookup, time.Now())
	if !ok || slices.ContainsFunc(items, func(item observationStatus) bool { return !slices.Contains(active.Streams, item.Stream) }) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "monitor unavailable", http.StatusServiceUnavailable)
		return
	}
	explorerJSON(w, items)
}

type boundaryEvidence struct {
	Kind             string     `json:"kind"`
	TxHash           *string    `json:"txHash"`
	LogIndex         *int32     `json:"logIndex"`
	Height           *int64     `json:"inclusionHeight"`
	IncludedAt       *time.Time `json:"includedAt"`
	ObservedAt       time.Time  `json:"attestedAt"`
	Anchor           int64      `json:"anchor"`
	AnchorHash       string     `json:"anchorHash"`
	Expires          *time.Time `json:"expiresAt"`
	CoverageStart    *int64     `json:"coverageStart"`
	Proof            *string    `json:"proof"`
	QuoteIssuedAt    *time.Time `json:"quoteIssuedAt,omitempty"`
	QuoteExpiresAt   *time.Time `json:"quoteExpiresAt,omitempty"`
	QuoteDigest      *string    `json:"quoteDigest,omitempty"`
	Policy           string     `json:"-"`
	AnchorTime       *time.Time `json:"-"`
	EventQuoteDigest *string    `json:"-"`
}

func (s *OperatorStore) boundary(w http.ResponseWriter, r *http.Request, lookup func(string) (OperatorGrant, bool)) {
	var in struct {
		Stream   string `json:"stream"`
		Merchant string `json:"merchant"`
		Order    string `json:"order"`
		Branch   string `json:"branch"`
		Sale     string `json:"sale"`
	}
	original := r.Method == http.MethodPost
	if original {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if r.Header.Get("Content-Type") != "application/json" || decoder.Decode(&in) != nil ||
			decoder.Decode(new(any)) != io.EOF || !uuidPattern.MatchString(in.Branch) ||
			!uuidPattern.MatchString(in.Sale) || s.QuoteOrigin == nil {
			http.NotFound(w, r)
			return
		}
	} else {
		q := r.URL.Query()
		in.Stream, in.Merchant, in.Order = q.Get("stream"), q.Get("merchant"), q.Get("order")
	}
	stream, merchant, order := in.Stream, in.Merchant, in.Order
	grant, expiry, ok := operatorAt(r, lookup, time.Now())
	if !ok || !slices.Contains(grant.Streams, stream) || !slices.Contains(grant.Merchants[stream], merchant) ||
		!uuidPattern.MatchString(stream) || !validOrderKeys(merchant, order) {
		http.NotFound(w, r)
		return
	}
	ctx, stop := context.WithDeadline(r.Context(), expiry)
	defer stop()
	var origin service.QuoteBoundaryOrigin
	if original {
		peer, ok := service.CurrentVerifiedClient(r.TLS, time.Now())
		if !ok {
			http.NotFound(w, r)
			return
		}
		var err error
		origin, err = s.QuoteOrigin.Read(ctx, service.QuoteBoundaryLookup{
			Operator: peer.FingerprintHex(), Branch: in.Branch, Sale: in.Sale, Stream: stream, Merchant: merchant, Order: order,
		})
		if errors.Is(err, errQuoteOriginDenied) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "issuer read unavailable", http.StatusServiceUnavailable)
			return
		}
	}

	before, err := captureReadRevision(ctx, s.db, stream)
	if err != nil || before.Status == "held" {
		http.Error(w, "active observation unavailable", http.StatusServiceUnavailable)
		return
	}

	var coverage quoteCoverage
	var coverageErr error
	if original {
		coverage, coverageErr = captureQuoteCoverage(ctx, s.db, stream)
	}

	var item boundaryEvidence
	if original {
		err = s.db.QueryRow(ctx, `SELECT kind,tx_hash,log_index,inclusion_height,included_at,created_at,
			anchor_height,anchor_hash,expires_at,coverage_start,state_proof_ref,policy_ref,anchored_at,event_quote_digest
			FROM chainview.explorer_boundary
			WHERE stream_id=$1 AND merchant_key=$2 AND order_key=$3
			  AND (kind<>'absent' OR state_proof_ref='sha256:'||encode(sha256(convert_to(
			      concat_ws(chr(10),'nu54-absence-v2',$4::text,stream_id::text,merchant_key,order_key,anchor_hash,$5::bigint::text,'false'),
			      'UTF8')), 'hex'))
			ORDER BY CASE WHEN kind='paid' THEN 0 WHEN kind='invalidated' THEN 1 ELSE 2 END,anchor_height DESC,id
			LIMIT 1`, stream, merchant, order, origin.Manifest, origin.ExpiresAt).
			Scan(&item.Kind, &item.TxHash, &item.LogIndex, &item.Height, &item.IncludedAt, &item.ObservedAt,
				&item.Anchor, &item.AnchorHash, &item.Expires, &item.CoverageStart, &item.Proof,
				&item.Policy, &item.AnchorTime, &item.EventQuoteDigest)
	} else {
		err = s.db.QueryRow(ctx, `SELECT kind,tx_hash,log_index,inclusion_height,included_at,created_at,
			anchor_height,anchor_hash,expires_at,coverage_start,state_proof_ref,quote_issued_at,quote_expires_at,quote_digest
			FROM chainview.explorer_boundary
			WHERE stream_id=$1 AND merchant_key=$2 AND order_key=$3
			ORDER BY CASE WHEN kind='paid' THEN 0 WHEN kind='invalidated' THEN 1 ELSE 2 END,anchor_height DESC,id
			LIMIT 1`, stream, merchant, order).
			Scan(&item.Kind, &item.TxHash, &item.LogIndex, &item.Height, &item.IncludedAt, &item.ObservedAt,
				&item.Anchor, &item.AnchorHash, &item.Expires, &item.CoverageStart, &item.Proof,
				&item.QuoteIssuedAt, &item.QuoteExpiresAt, &item.QuoteDigest)
	}

	after, revisionErr := captureReadRevision(ctx, s.db, stream)

	active, _, ok := operatorAt(r, lookup, time.Now())
	if !ok || !slices.Contains(active.Streams, stream) || !slices.Contains(active.Merchants[stream], merchant) {
		http.NotFound(w, r)
		return
	}
	if revisionErr != nil || !sameReadRevision(before, after) {
		http.Error(w, "active observation changed", http.StatusServiceUnavailable)
		return
	}

	if original {
		if coverageErr != nil {
			http.Error(w, "active contiguous coverage unavailable", http.StatusServiceUnavailable)
			return
		}
		if !coverage.matches(origin) {
			http.NotFound(w, r)
			return
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "evidence unavailable", http.StatusServiceUnavailable)
			return
		}
		explorerJSON(w, quoteBoundaryResult(coverage, origin, item))
		return
	}

	if errors.Is(err, pgx.ErrNoRows) {
		explorerJSON(w, map[string]string{"kind": "unobserved"})
		return
	}
	if err != nil {
		http.Error(w, "evidence unavailable", http.StatusServiceUnavailable)
		return
	}
	explorerJSON(w, item)
}

func explorerJSON(w http.ResponseWriter, data any) {
	webHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
