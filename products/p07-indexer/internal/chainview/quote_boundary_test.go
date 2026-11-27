package chainview

import (
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

func TestOriginalQuoteRequiresExactInclusionWithinSignedWindow(t *testing.T) {
	origin := service.QuoteBoundaryOrigin{
		Manifest: "approved-deployment", Digest: "original-digest", IssuedAt: 100, ExpiresAt: 200,
	}
	coverage := quoteCoverage{Height: 10, Time: time.Unix(199, 0)}
	digest := origin.Digest
	evidence := boundaryEvidence{
		Kind: "paid", Policy: origin.Manifest, Anchor: 10, AnchorTime: &coverage.Time, EventQuoteDigest: &digest,
	}
	for _, tt := range []struct {
		name     string
		included int64
		digest   string
		kind     string
	}{
		{"inclusive issue", 100, digest, "paid"},
		{"before expiry", 199, digest, "paid"},
		{"exclusive expiry", 200, digest, "pending"},
		{"predates original issue", 99, digest, "pending"},
		{"different quote on same order", 199, "other-digest", "pending"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ev := evidence
			included := time.Unix(tt.included, 0)
			ev.IncludedAt, ev.EventQuoteDigest = &included, &tt.digest
			out := quoteBoundaryResult(coverage, origin, ev)
			if out.Kind != tt.kind || (out.Evidence != nil) != (tt.kind == "paid") {
				t.Fatalf("original signed quote: %+v", out)
			}
		})
	}
}
