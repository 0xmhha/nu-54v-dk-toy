package commerce

import (
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

func TestLegacyCashoutKeepsChangedExecutionDistinct(t *testing.T) {
	review := legacyReview{Payout: "0x100", Balance: "7000"}
	for _, test := range []struct {
		name, status, payout, amount, want string
	}{
		{"matched", "verified", "0x100", "7000", "verified"},
		{"new payout", "verified", "0x200", "7000", "different-execution"},
		{"new balance", "verified", "0x100", "8000", "different-execution"},
		{"held changed payout", "transfer-held", "0x200", "7000", "different-execution"},
		{"unknown", "pending", "", "", "pending"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := review.compare(core.LegacyCashOutObservation{
				Status: test.status, Hash: "transaction", Block: "confirmed-block",
				Payout: test.payout, Amount: test.amount,
			})
			if got.Status != test.want || got.Hash != "transaction" || got.Block != "confirmed-block" ||
				got.Payout != test.payout || got.Amount != test.amount {
				t.Fatalf("unexpected reviewed execution: %+v", got)
			}
		})
	}
}
