package settlement

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

func TestRentalEscrowProofLeavesClockMargin(t *testing.T) {
	source, err := os.ReadFile("../../../../packages/protocol/ts/src/private-presentation.ts")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`escrow\.expiresAt <= now \|\| escrow\.expiresAt - now > (\d+)`).FindSubmatch(source)
	if match == nil {
		t.Fatal("wallet escrow limit not found")
	}
	seconds, _ := strconv.Atoi(string(match[1]))
	limit := time.Duration(seconds) * time.Second
	if RentalEscrowProofLifetime <= 0 || limit-RentalEscrowProofLifetime < 2*time.Second {
		t.Fatalf("escrow proof lifetime %s leaves less than 2s under the wallet limit %s", RentalEscrowProofLifetime, limit)
	}
}
