package commerce

import (
	"context"
	"errors"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

func TestQuoteAdmissionExpiryDoesNotAcquireSigningLease(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := HTTP{QuoteAdmission: slots}
	if reply, err := h.issue(ctx, service.Bound{}); reply != nil || !errors.Is(err, service.Busy) {
		t.Fatalf("full issuer did not return a safe retry: %v", err)
	}
	if len(slots) != 1 {
		t.Fatal("cancelled request stole an issuer slot")
	}
}
