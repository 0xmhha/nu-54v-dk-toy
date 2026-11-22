package commerce

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// The private provider must use its own verified TLS identity. The customer
// certificate authorizes the handover read and cannot act as a reader audience.
type renterCredentialCollector struct {
	authority *RentalAuthorityReader
	tls       *tls.ConnectionState
	writer    [32]byte
	profile   [32]byte
	source    rentalCredentialSource
}

type renterCredentialScope struct {
	rentalv1.AuthorityRequest
	ScopeDigest string `json:"scopeDigest"`
	ValidUntil  int64  `json:"validUntil"`
}

type renterCredentialLookup struct {
	audience [32]byte
	scope    renterCredentialScope
	term     rentalCredentialTerm
	known    bool
}

func (h HTTP) collectRenterCredential(ctx context.Context, actor service.Principal, assignment string, customer [32]byte) *renterCredentialLookup {
	collector := h.renterCredentialCollector
	if collector == nil || collector.authority == nil || collector.writer == ([32]byte{}) ||
		collector.profile == ([32]byte{}) || collector.source == nil {
		return nil
	}
	client, verified := service.CurrentVerifiedClient(collector.tls, time.Now())
	if !verified || collector.writer == client.Fingerprint || customer == client.Fingerprint || customer == collector.writer {
		return nil
	}
	_, scope, _, err := h.readRenterCredentialScope(ctx, actor, assignment, client.Fingerprint, nil)
	lookup := &renterCredentialLookup{audience: client.Fingerprint}
	if err != nil || scope.ScopeDigest == "" || scope.ValidUntil <= time.Now().Unix() {
		return lookup
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	lookup.scope = scope
	lookup.term, lookup.known = collector.authority.verifyCredential(bounded, collector.tls, scope.AuthorityRequest, collector.profile, collector.source)
	return lookup
}

func (h HTTP) readRenterCredentialScope(ctx context.Context, actor service.Principal, assignment string,
	audience [32]byte, settlementWriter []byte) (json.RawMessage, renterCredentialScope, *string, error) {
	return h.readRenterCredentialScopeForWriter(ctx, actor, assignment, audience,
		h.renterCredentialCollector.writer, settlementWriter)
}

func (h HTTP) readRenterCredentialScopeForWriter(ctx context.Context, actor service.Principal, assignment string,
	audience, writer [32]byte, settlementWriter []byte) (json.RawMessage, renterCredentialScope, *string, error) {
	var raw, authority json.RawMessage
	var scope renterCredentialScope
	var settlementScope *string
	err := h.Store.db.QueryRow(ctx, `
		WITH current AS MATERIALIZED (SELECT ledger.renter_credential_scope($1,$2,$3,$4,$5) value)
		SELECT ledger.read_renter_return($1,$2,$3), current.value,
			CASE WHEN $6::bytea IS NOT NULL THEN ledger.renter_return_assessment($1,$2,$3,$6)->>'scopeDigest' END
			FROM current`,
		actor.Tenant, actor.Subject, assignment, audience[:], writer[:], settlementWriter).Scan(&raw, &authority, &settlementScope)
	if err == nil && len(authority) != 0 {
		err = json.Unmarshal(authority, &scope)
	}
	return raw, scope, settlementScope, err
}

func projectRenterCredential(raw json.RawMessage, attempt *renterCredentialLookup, current renterCredentialScope, customerUntil int64, now time.Time) json.RawMessage {
	if !attempt.known || current != attempt.scope {
		return raw
	}
	validUntil := min(current.ValidUntil, attempt.term.validUntil, customerUntil)
	if now.Unix() >= validUntil || validUntil > attempt.term.endsAt {
		return raw
	}
	// Keep existing JSON values byte-for-byte as numbers and nested raw fields.
	var projected map[string]json.RawMessage
	if json.Unmarshal(raw, &projected) != nil || projected == nil {
		return raw
	}
	projected["credentialExpiresAt"], _ = json.Marshal(time.Unix(attempt.term.endsAt, 0).UTC())
	projected["credentialEvidenceUntil"], _ = json.Marshal(time.Unix(validUntil, 0).UTC())
	encoded, err := json.Marshal(projected)
	if err != nil {
		return raw
	}
	return encoded
}

// Revalidate all returned evidence within the final scoped database read.
func (h HTTP) readRenterProjectionScopes(ctx context.Context, actor service.Principal, assignment string,
	credentialAudience, credentialWriter, slotAudience, slotWriter, settlementWriter []byte,
) (json.RawMessage, renterCredentialScope, renterCredentialScope, *string, error) {
	var raw, credential, slot json.RawMessage
	var credentialScope, slotScope renterCredentialScope
	var assessmentScope *string
	err := h.Store.db.QueryRow(ctx, `
		WITH current AS MATERIALIZED (
			SELECT ledger.renter_credential_scope($1,$2,$3,$4,$5) credential,
				ledger.renter_credential_scope($1,$2,$3,$6,$7) slot)
		SELECT ledger.read_renter_return($1,$2,$3), current.credential, current.slot,
			CASE WHEN $8::bytea IS NOT NULL THEN ledger.renter_return_assessment($1,$2,$3,$8)->>'scopeDigest' END
		FROM current`, actor.Tenant, actor.Subject, assignment, credentialAudience, credentialWriter,
		slotAudience, slotWriter, settlementWriter).Scan(&raw, &credential, &slot, &assessmentScope)
	if err == nil && len(credential) != 0 {
		err = json.Unmarshal(credential, &credentialScope)
	}
	if err == nil && len(slot) != 0 {
		err = json.Unmarshal(slot, &slotScope)
	}
	return raw, credentialScope, slotScope, assessmentScope, err
}
