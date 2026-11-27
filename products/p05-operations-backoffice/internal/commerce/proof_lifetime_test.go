package commerce

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// walletProofLimit reads the maximum proof lifetime the wallet accepts from the shared
// TypeScript verifier, so the issuer margin cannot silently drift from the verifier.
func walletProofLimit(t *testing.T, pattern string) time.Duration {
	t.Helper()
	source, err := os.ReadFile("../../../../packages/protocol/ts/src/private-presentation.ts")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(pattern).FindSubmatch(source)
	if match == nil {
		t.Fatalf("wallet limit %q not found", pattern)
	}
	seconds, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	return time.Duration(seconds) * time.Second
}

func TestGasSponsorProofLeavesClockMargin(t *testing.T) {
	limit := walletProofLimit(t, `sponsor\.expiresAt - now > (\d+)\) throw new Error\("GAS_SPONSOR_SHAPE"\)`)
	if GasSponsorProofLifetime <= 0 || limit-GasSponsorProofLifetime < 2*time.Second {
		t.Fatalf("sponsor proof lifetime %s leaves less than 2s under the wallet limit %s", GasSponsorProofLifetime, limit)
	}
}
