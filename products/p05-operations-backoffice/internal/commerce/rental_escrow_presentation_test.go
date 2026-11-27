package commerce

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"
)

func TestRentalEscrowPresentationSignsEarliestAuthorityDeadline(t *testing.T) {
	now := time.Unix(1800000000, 500_000_000)
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	scope, quote := []byte("private scope"), []byte("signed original")
	for _, test := range []struct {
		name                                             string
		scopeAuthority, finalizedEvidence, originalQuote time.Duration
		deadline                                         int64
	}{
		{"payer_enrollment_first", time.Second, 15 * time.Second, 30 * time.Second, now.Unix() + 1},
		{"chain_evidence_first", 7 * time.Second, 3 * time.Second, 20 * time.Second, now.Unix() + 3},
		{"original_quote_first", 7 * time.Second, 6 * time.Second, 2 * time.Second, now.Unix() + 2},
		{"rounded_deadline_already_reached", 100 * time.Millisecond, 15 * time.Second, 30 * time.Second, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := signRentalEscrowProof(key, scope, quote, now, now.Add(test.scopeAuthority),
				now.Add(test.finalizedEvidence), now.Add(test.originalQuote))
			if test.deadline == 0 {
				if got != nil {
					t.Fatal("signed an already expired deadline")
				}
				return
			}
			if got == nil || got.ExpiresAt != test.deadline {
				t.Fatalf("proof expiry = %+v, want %d", got, test.deadline)
			}
			scopeHash, quoteHash := sha256.Sum256(scope), sha256.Sum256(quote)
			body := append([]byte("NU54-rental-payment-escrow-v1\x00"), scopeHash[:]...)
			body = append(body, quoteHash[:]...)
			body = binary.BigEndian.AppendUint64(body, uint64(test.deadline))
			signature, err := hex.DecodeString(got.Signature)
			if err != nil || !ed25519.Verify(key.Public().(ed25519.PublicKey), body, signature) {
				t.Fatal("proof signature did not bind the earliest deadline")
			}
		})
	}
}
