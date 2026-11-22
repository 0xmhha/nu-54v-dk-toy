package commerce

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestRentalReviewProofBindsOriginalQuoteAndFreshChallenge(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	x, y := elliptic.P256().ScalarBaseMult([]byte{3})
	board := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	spki, err := x509.MarshalPKIXPublicKey(board)
	if err != nil {
		t.Fatal(err)
	}
	commissioned := RentalReviewCommissioning{Signer: [65]byte(elliptic.Marshal(board.Curve, x, y)),
		BoardPin: sha256.Sum256(spki), TerminalPin: [32]byte{17}, ExpiresAt: now.Add(5 * time.Second)}
	original, scope := []byte("original signed quote"), []byte("verified original sale scope")
	challenge := RentalReviewChallenge{Request: strings.Repeat("a", 32), Challenge: strings.Repeat("1", 64)}
	proof := signRentalReview(key, scope, original, now, now.Add(15*time.Second).Unix(), challenge, commissioned)
	if proof == nil || proof.RentalReviewChallenge != challenge || proof.ExpiresAt != commissioned.ExpiresAt.Unix() {
		t.Fatal("fresh review did not use the actual commissioning lifetime and caller nonce")
	}
	hash, wire := sha256.Sum256(scope), sha256.Sum256(original)
	payload := append([]byte("NU54-rental-payment-review-authority-v1\x00"), hash[:]...)
	payload = append(payload, wire[:]...)
	payload = append(payload, commissioned.BoardPin[:]...)
	payload = append(payload, commissioned.TerminalPin[:]...)
	payload = append(payload, commissioned.Signer[:]...)
	request, _ := hex.DecodeString(challenge.Request)
	nonce, _ := hex.DecodeString(challenge.Challenge)
	payload = append(payload, request...)
	payload = append(payload, nonce...)
	payload = binary.BigEndian.AppendUint64(payload, uint64(proof.ExpiresAt))
	signature, err := hex.DecodeString(proof.Signature)
	if err != nil || !ed25519.Verify(key.Public().(ed25519.PublicKey), payload, signature) {
		t.Fatal("authority signature did not bind the exact original, terminal, board and phone nonce")
	}
	if signRentalReview(key, scope, original, now, now.Unix(), challenge, commissioned) != nil {
		t.Fatal("expired rental escrow authorized a new review")
	}
	commissioned.BoardPin[0] ^= 1
	if signRentalReview(key, scope, original, now, now.Add(time.Minute).Unix(), challenge, commissioned) != nil {
		t.Fatal("board key differed from registered canonical SPKI")
	}
}
