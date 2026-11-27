package commerce

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SubscriptionOffer struct {
	ID           string `json:"id"`
	Series       string `json:"seriesRef"`
	Label        string `json:"label"`
	TermsVersion string `json:"termsVersion"`
	TermsDigest  string `json:"termsDigest"`
	Cadence      string `json:"cadence"`
	Timezone     string `json:"timezone"`
	AnchorDay    int    `json:"anchorDay"`
	DueDays      int    `json:"dueDays"`
	Asset        string `json:"asset"`
	Amount       string `json:"amount"`
	Fulfillment  string `json:"fulfillment"`
}

type Subscription struct {
	SubscriptionOffer
	Offer        string     `json:"offerRef"`
	State        string     `json:"state"`
	Next         *time.Time `json:"nextAt,omitempty"`
	Revision     int64      `json:"revision"`
	NeedsConsent bool       `json:"needsConsent"`
}

type Invoice struct {
	ID           string    `json:"id"`
	Subscription string    `json:"subscriptionRef"`
	Sale         string    `json:"saleRef"`
	Cycle        string    `json:"cycle"`
	Due          time.Time `json:"dueAt"`
	State        string    `json:"state"`
	Asset        string    `json:"asset"`
	Amount       string    `json:"amount"`
	Fulfillment  string    `json:"fulfillment"`
	Stage        string    `json:"stage,omitempty"`
}

type InvoiceLifecycle struct {
	InvoiceRef     string     `json:"invoiceRef"`
	Cycle          string     `json:"cycle"`
	DueAt          time.Time  `json:"dueAt"`
	TermsVersion   string     `json:"termsVersion"`
	Stage          string     `json:"stage"`
	QuoteRef       string     `json:"quoteRef,omitempty"`
	QuoteExpiresAt *time.Time `json:"quoteExpiresAt,omitempty"`
}

func (s *Store) subscription(ctx context.Context, auth service.Bound, out *result) error {
	req := auth.Request
	if req.Operation == "subscription.offers" || req.Operation == "subscription.list" || req.Operation == "invoice.list" {
		if req.Object != auth.Subject {
			return service.Forbidden
		}
		return s.subscriptionRead(ctx, auth, out)
	}
	var err error
	switch req.Operation {
	case "invoice.skip":
		return domainError(s.db.QueryRow(ctx, "SELECT ledger.skip_subscription_invoice($1,$2,$3,$4,$5)::text",
			auth.Tenant, req.Domain, auth.Subject, req.Object, req.IdempotencyKey).Scan(&out.ResultRef))
	case "subscription.accept":
		digest, err := hex.DecodeString(req.Body["termsDigest"])
		if err != nil || len(digest) != 32 {
			return service.BadRequest
		}
		return domainError(s.db.QueryRow(ctx,
			"SELECT ledger.accept_subscription($1,$2,$3,$4,$5,$6)::text", auth.Tenant, req.Domain, auth.Subject,
			req.Object, digest, req.IdempotencyKey).Scan(&out.ResultRef))
	case "subscription.reconsent":
		digest, e := hex.DecodeString(req.Body["termsDigest"])
		revision, e2 := strconv.ParseInt(req.Body["expectedRevision"], 10, 64)
		if e != nil || e2 != nil || len(digest) != 32 {
			return service.BadRequest
		}
		err = s.db.QueryRow(ctx, "SELECT ledger.reconsent_subscription($1,$2,$3,$4,$5,$6,$7,$8)::text",
			auth.Tenant, req.Domain, auth.Subject, req.Object, revision, req.Body["offerRef"], digest, req.IdempotencyKey).
			Scan(&out.ResultRef)
	default:
		revision, e := strconv.ParseInt(req.Body["expectedRevision"], 10, 64)
		if e != nil {
			return service.BadRequest
		}
		err = s.db.QueryRow(ctx, "SELECT ledger.change_subscription($1,$2,$3,$4,$5,$6,$7)::text",
			auth.Tenant, req.Domain, auth.Subject, req.Object, revision,
			strings.TrimPrefix(req.Operation, "subscription."), req.IdempotencyKey).Scan(&out.ResultRef)
	}
	return domainError(err)
}

func (s *Store) subscriptionRead(ctx context.Context, auth service.Bound, out *result) error {
	tenant, branch, actor := auth.Tenant, auth.Request.Domain, auth.Subject
	switch auth.Request.Operation {
	case "subscription.offers":
		rows, err := s.db.Query(ctx, `SELECT id::text,series_ref::text,label,terms_version,terms_digest,cadence,timezone,anchor_day,due_days,asset_ref,amount,fulfillment_kind
			FROM ledger.subscription_offers($1,$2,$3)`, tenant, branch, actor)
		if err != nil {
			return domainError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var item SubscriptionOffer
			if err := rows.Scan(&item.ID, &item.Series, &item.Label, &item.TermsVersion, &item.TermsDigest, &item.Cadence, &item.Timezone,
				&item.AnchorDay, &item.DueDays, &item.Asset, &item.Amount, &item.Fulfillment); err != nil {
				return domainError(err)
			}
			out.Offers = append(out.Offers, item)
		}
		return domainError(rows.Err())
	case "subscription.list":
		rows, err := s.db.Query(ctx, `SELECT id::text,offer_id::text,series_ref::text,label,terms_version,terms_digest,cadence,timezone,anchor_day,due_days,
			asset_ref,amount,state,next_due_at,revision,needs_consent,fulfillment_kind FROM ledger.subscriptions($1,$2,$3)`, tenant, branch, actor)
		if err != nil {
			return domainError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var item Subscription
			if err := rows.Scan(&item.ID, &item.Offer, &item.Series, &item.Label, &item.TermsVersion, &item.TermsDigest, &item.Cadence, &item.Timezone,
				&item.AnchorDay, &item.DueDays, &item.Asset, &item.Amount, &item.State, &item.Next, &item.Revision, &item.NeedsConsent, &item.Fulfillment); err != nil {
				return domainError(err)
			}
			out.Subscriptions = append(out.Subscriptions, item)
		}
		return domainError(rows.Err())
	case "invoice.list":
		rows, err := s.db.Query(ctx, `SELECT id::text,subscription_id::text,sale_id::text,cycle_key::text,due_at,state,asset_ref,amount,fulfillment_kind,coalesce(lifecycle->>'stage','')
			FROM ledger.subscription_invoice_rows($1,$2,$3)`, tenant, branch, actor)
		if err != nil {
			return domainError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var item Invoice
			if err := rows.Scan(&item.ID, &item.Subscription, &item.Sale, &item.Cycle, &item.Due, &item.State, &item.Asset, &item.Amount, &item.Fulfillment, &item.Stage); err != nil {
				return domainError(err)
			}
			if item.Stage == item.State {
				item.Stage = ""
			}
			out.Invoices = append(out.Invoices, item)
		}
		return domainError(rows.Err())
	}
	return service.Capability
}

type SubscriptionWorker struct{ pool *pgxpool.Pool }

func OpenSubscriptionWorker(ctx context.Context, dsn string) (*SubscriptionWorker, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, service.Capability
	}
	config.ConnConfig.RuntimeParams["lock_timeout"] = "2s"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "7s"
	config.ConnConfig.RuntimeParams["jit"] = "off" // see commerce.Open
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, service.Capability
	}
	var role bool
	err = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p05_subscription_worker','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p05_commerce','MEMBER')
		AND NOT has_schema_privilege('ledger','CREATE') AND NOT has_table_privilege('ledger.subscription','SELECT,INSERT,UPDATE,DELETE')
		AND NOT (SELECT rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole FROM pg_roles WHERE rolname=current_user)`).Scan(&role)
	if err != nil || !role {
		pool.Close()
		return nil, service.Forbidden
	}
	return &SubscriptionWorker{pool}, nil
}

func (w *SubscriptionWorker) Close() { w.pool.Close() }

func (w *SubscriptionWorker) Step(ctx context.Context) (bool, error) {
	var id *string
	err := w.pool.QueryRow(ctx, "SELECT ledger.make_due_invoice()::text").Scan(&id)
	return id != nil, err
}
