package commerce

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// The explicit profile pin must identify a board adapter qualified to guard
// all protected slot and journal writers. A credential-only provider or pin
// is insufficient. No ordinary server setting supplies this provider.
type renterSlotOpenCollector struct {
	authority *RentalAuthorityReader
	tls       *tls.ConnectionState
	writer    [32]byte
	profile   [32]byte
	source    rentalCredentialSource
}

type renterSlotOpenObservation struct {
	checkedAt, validUntil int64
	scope                 renterCredentialScope
}

type renterSlotScopeRead func(context.Context, service.Principal, string, [32]byte, [32]byte) (renterCredentialScope, error)

// readRenterSlotOpen is deliberately detached from the HTTP response. It gets
// the customer principal from verified TLS and uses the existing scoped 091 DB read.
func (h HTTP) readRenterSlotOpen(ctx context.Context, tlsState *tls.ConnectionState, assignment string,
	collector *renterSlotOpenCollector) (renterSlotOpenObservation, bool) {
	if h.Store == nil {
		return renterSlotOpenObservation{}, false
	}
	actor, customer, ok := service.CurrentClientPrincipal(tlsState, h.Credentials, time.Now())
	if !ok || actor.Role != "customer" || !operatorRef.MatchString(actor.Tenant) ||
		!operatorRef.MatchString(actor.Subject) || !operatorRef.MatchString(assignment) {
		return renterSlotOpenObservation{}, false
	}
	observation, known := collector.verify(ctx, actor, assignment, customer,
		func(ctx context.Context, principal service.Principal, ref string, audience, writer [32]byte) (renterCredentialScope, error) {
			_, scope, _, err := h.readRenterCredentialScopeForWriter(ctx, principal, ref, audience, writer, nil)
			return scope, err
		})
	if !known {
		return renterSlotOpenObservation{}, false
	}
	latest, latestCustomer, ok := service.CurrentClientPrincipal(tlsState, h.Credentials, time.Now())
	if !ok || latest.Tenant != actor.Tenant || latest.Subject != actor.Subject || latest.Role != actor.Role ||
		latestCustomer.Fingerprint != customer.Fingerprint ||
		time.Now().Unix() >= min(observation.validUntil, latest.ExpiresAt, latestCustomer.ExpiresAt.Unix()) {
		return renterSlotOpenObservation{}, false
	}
	observation.validUntil = min(observation.validUntil, latest.ExpiresAt, latestCustomer.ExpiresAt.Unix())
	return observation, true
}

// verify uses a reader that resolves the accepted handover and collection authority together.
func (c *renterSlotOpenCollector) verify(ctx context.Context, actor service.Principal, assignment string,
	customer service.VerifiedClient, readScope renterSlotScopeRead) (renterSlotOpenObservation, bool) {
	if c == nil || c.authority == nil || c.writer == ([32]byte{}) || c.profile == ([32]byte{}) || c.source == nil {
		return renterSlotOpenObservation{}, false
	}
	client, verified := service.CurrentVerifiedClient(c.tls, time.Now())
	if !verified {
		return renterSlotOpenObservation{}, false
	}
	return verifyRenterSlotOpen(ctx, actor, assignment, customer, client, c.writer, readScope,
		func(ctx context.Context, scope renterCredentialScope) (rentalObservation, bool) {
			return c.authority.verifyRentalObservation(ctx, client, scope.AuthorityRequest,
				c.profile, c.source, c.authority.read, time.Now, slotOpenReceiptType)
		}, time.Now)
}

func verifyRenterSlotOpen(ctx context.Context, actor service.Principal, assignment string,
	customer, collector service.VerifiedClient, writer [32]byte, readScope renterSlotScopeRead,
	verify func(context.Context, renterCredentialScope) (rentalObservation, bool),
	now func() time.Time) (renterSlotOpenObservation, bool) {
	var unknown renterSlotOpenObservation
	if assignment == "" || customer.Fingerprint == ([32]byte{}) || collector.Fingerprint == ([32]byte{}) ||
		writer == ([32]byte{}) ||
		customer.Fingerprint == collector.Fingerprint || customer.Fingerprint == writer ||
		collector.Fingerprint == writer || !customer.ExpiresAt.After(now()) {
		return unknown, false
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	first, err := readScope(ctx, actor, assignment, collector.Fingerprint, writer)
	if err != nil || first.ScopeDigest == "" || first.ValidUntil <= now().Unix() {
		return unknown, false
	}
	observation, verified := verify(ctx, first)
	if !verified {
		return unknown, false
	}
	final, err := readScope(ctx, actor, assignment, collector.Fingerprint, writer)
	finished := now().Unix()
	validUntil := min(observation.validUntil, final.ValidUntil, customer.ExpiresAt.Unix())
	if err != nil || final != first || ctx.Err() != nil || finished >= validUntil || finished < observation.checkedAt {
		return unknown, false
	}
	return renterSlotOpenObservation{observation.checkedAt, validUntil, first}, true
}

func projectRenterSlotOpen(raw json.RawMessage, observation renterSlotOpenObservation,
	current renterCredentialScope, customerUntil int64, now time.Time) json.RawMessage {
	validUntil := min(observation.validUntil, customerUntil)
	if observation.checkedAt <= 0 || observation.scope != current ||
		now.Unix() < observation.checkedAt || now.Unix() >= validUntil {
		return raw
	}
	var projected map[string]json.RawMessage
	if json.Unmarshal(raw, &projected) != nil || projected == nil {
		return raw
	}
	projected["slot"] = json.RawMessage(`"open"`)
	projected["slotCheckedAt"], _ = json.Marshal(time.Unix(observation.checkedAt, 0).UTC())
	projected["slotEvidenceUntil"], _ = json.Marshal(time.Unix(validUntil, 0).UTC())
	encoded, err := json.Marshal(projected)
	if err != nil {
		return raw
	}
	return encoded
}
