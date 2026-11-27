package commerce

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
)

type Comparison struct {
	Customer  string `json:"customerRef,omitempty"`
	Code      string `json:"code"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (s *Store) RequestComparison(ctx context.Context, auth service.Bound) (Comparison, error) {
	var entropy [4]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return Comparison{}, service.Unknown
	}
	var result Comparison
	var expires time.Time
	err := s.db.QueryRow(ctx, `SELECT code,expiry FROM ledger.request_checkout_pair($1,$2,$3,$4,$5,$6)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, auth.Request.IdempotencyKey, hex.EncodeToString(entropy[:])).Scan(&result.Code, &expires)
	if err == nil {
		result.ExpiresAt = expires.Unix()
	}
	return result, domainError(err)
}

func (s *Store) PendingComparisons(ctx context.Context, auth service.Bound) ([]Comparison, error) {
	rows, err := s.db.Query(ctx, `SELECT customer_ref,code,expiry FROM ledger.pending_checkout_pairs($1,$2,$3,$4)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object)
	if err != nil {
		return nil, domainError(err)
	}
	defer rows.Close()
	var all []Comparison
	for rows.Next() {
		var result Comparison
		var expires time.Time
		if err := rows.Scan(&result.Customer, &result.Code, &expires); err != nil {
			return nil, domainError(err)
		}
		result.ExpiresAt = expires.Unix()
		all = append(all, result)
	}
	return all, domainError(rows.Err())
}

func (s *Store) ConfirmComparison(ctx context.Context, auth service.Bound) (Claim, error) {
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return Claim{}, service.Unknown
	}
	intent, err := hex.DecodeString(auth.IntentDigest)
	if err != nil {
		return Claim{}, service.BadRequest
	}
	var digest []byte
	var expires time.Time
	err = s.db.QueryRow(ctx, `SELECT claim_digest,expires_at FROM ledger.confirm_checkout_pair($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, auth.Request.Body["customerRef"], auth.Request.Body["code"],
		auth.Request.IdempotencyKey, intent, entropy[:]).Scan(&digest, &expires)
	if err != nil {
		return Claim{}, domainError(err)
	}
	return Claim{Digest: hex.EncodeToString(digest), ExpiresAt: expires.Unix()}, nil
}

func (s *Store) ReadClaim(ctx context.Context, auth service.Bound) (*Claim, error) {
	var digest []byte
	var expires time.Time
	err := s.db.QueryRow(ctx, `SELECT claim_digest,expires_at FROM ledger.read_checkout_claim($1,$2,$3,$4)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object).Scan(&digest, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, domainError(err)
	}
	return &Claim{Digest: hex.EncodeToString(digest), ExpiresAt: expires.Unix()}, nil
}

func (s *Store) POSNearbyCheckout(ctx context.Context, auth service.Bound) (*Comparison, *Claim, error) {
	var entropy [4]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, nil, service.Unknown
	}
	var code *string
	var digest []byte
	var expires time.Time
	err := s.db.QueryRow(ctx, `SELECT code,claim_digest,expiry FROM ledger.pos_nearby_checkout($1,$2,$3,$4,$5,$6,$7,$8)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, auth.Request.Body["customerRef"],
		auth.Request.IdempotencyKey, hex.EncodeToString(entropy[:]), auth.Request.Body["mode"] == "register").Scan(&code, &digest, &expires)
	if err != nil {
		return nil, nil, domainError(err)
	}
	if code != nil {
		return &Comparison{Code: *code, ExpiresAt: expires.Unix()}, nil, nil
	}
	return nil, &Claim{Digest: hex.EncodeToString(digest), ExpiresAt: expires.Unix()}, nil
}
