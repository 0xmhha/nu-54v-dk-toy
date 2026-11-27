package commerce

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestRenterCredentialProjectionPreservesOrderAndAssessment(t *testing.T) {
	now := time.Unix(1000, 0)
	raw := json.RawMessage(`{"assignment":"fixture","settlement":"verified","pendingCount":2,"pendingPayments":1,"pendingLimits":1,"pendingOrders":[{"amount":"123456789012345678901","orderRef":"original"}],"slot":"unconfirmed"}`)
	scope := renterCredentialScope{ScopeDigest: "first", ValidUntil: 1010}
	evidence := &renterCredentialLookup{scope: scope, known: true, term: rentalCredentialTerm{endsAt: 1100, validUntil: 1020}}
	received := projectRenterCredential(raw, evidence, scope, 1009, now)
	var baseline, enhanced map[string]json.RawMessage
	if json.Unmarshal(raw, &baseline) != nil || json.Unmarshal(received, &enhanced) != nil {
		t.Fatal("expected object projections")
	}
	if len(enhanced) != len(baseline)+2 || string(enhanced["credentialExpiresAt"]) != `"1970-01-01T00:18:20Z"` ||
		string(enhanced["credentialEvidenceUntil"]) != `"1970-01-01T00:16:49Z"` {
		t.Fatal("credential term and evidence deadline differed")
	}
	for key, expected := range baseline {
		if !bytes.Equal(expected, enhanced[key]) {
			t.Fatalf("previous projection field %q changed", key)
		}
	}
	for _, tc := range []struct {
		scope renterCredentialScope
		now   time.Time
	}{
		{renterCredentialScope{ScopeDigest: "changed", ValidUntil: 1010}, now},
		{scope, time.Unix(scope.ValidUntil, 0)},
	} {
		if got := projectRenterCredential(raw, evidence, tc.scope, 1009, tc.now); !bytes.Equal(got, raw) {
			t.Fatal("stale credential changed pre-existing projection")
		}
	}
}
