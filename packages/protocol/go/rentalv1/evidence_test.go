package rentalv1

import (
	"strings"
	"testing"
	"time"
)

func TestAssessmentPendingPaymentEntries(t *testing.T) {
	now := time.Unix(10000, 0)
	binding := Binding{
		Stream: "6717226b-a901-4b4d-b26c-f3b96efd06b4", StreamGeneration: "07d99c17-b42d-43bd-bab9-f86bd4272289",
		Deployment: "0x" + strings.Repeat("12", 32), FreezeReceipt: "0x" + strings.Repeat("34", 32),
		JournalRoot: "0x" + strings.Repeat("56", 32),
	}
	pending, payments, limits, height := 2, 1, 1, int64(50)
	assessment := Assessment{Status: "pending", Binding: binding, PendingCount: &pending, PendingPayments: &payments,
		PendingLimits: &limits, AnchorHeight: &height, AnchorHash: "0x" + strings.Repeat("11", 32),
		AnchorTime: now.Unix(), Digest: "sha256:" + strings.Repeat("aa", 32), ValidUntil: now.Unix() + 30}
	validate := func(value Assessment) error {
		return value.Validate(binding, 6, now.Add(-time.Second), now.Add(time.Minute), now)
	}
	if err := validate(assessment); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mask  uint8
		valid bool
	}{{0, false}, {1 << 3, true}, {1 << 6, false}, {(1 << 3) | (1 << 4), false}} {
		assessment.PendingPaymentEntries = &tc.mask
		if (validate(assessment) == nil) != tc.valid {
			t.Fatalf("unexpected validity for payment-entry mask %08b", tc.mask)
		}
	}
	if validate(Assessment{Status: "unknown", PendingPaymentEntries: assessment.PendingPaymentEntries}) == nil {
		t.Fatal("unknown response carried payment-entry positions")
	}
}
