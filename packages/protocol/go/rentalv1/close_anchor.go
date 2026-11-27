package rentalv1

import (
	"strconv"
	"strings"
	"time"
)

// CloseAnchorPath is the private P07 confirmation of an existing review anchor.
const CloseAnchorPath = "/v2/private/rental-generation/close-anchor"

type CloseAnchorRequest struct {
	Stream              string `json:"streamId"`
	StreamGeneration    string `json:"streamGeneration"`
	Branch              string `json:"branch"`
	Board               string `json:"board"`
	Generation          string `json:"generation"`
	Challenge           string `json:"challenge"`
	RegistrationBinding string `json:"registrationBinding"`
	Previous            Anchor `json:"previousAnchor"`
	ReviewDigest        string `json:"reviewDigest"`
	ValidUntil          int64  `json:"validUntil"`
}

type CloseAnchorConfirmation struct {
	CloseAnchorRequest
	Finalized Anchor `json:"finalized"`
	ExpiresAt int64  `json:"expiresAt"`
}

// Valid checks the complete, bounded reference to one persisted assessment.
func (r CloseAnchorRequest) Valid(now time.Time) bool {
	const zeroUUID = "00000000-0000-0000-0000-000000000000"
	for _, ref := range []string{r.Stream, r.StreamGeneration, r.Branch, r.Board} {
		if ref == zeroUUID || !refPattern.MatchString(ref) {
			return false
		}
	}
	generation, err := strconv.ParseUint(r.Generation, 10, 64)
	zeroHash := "0x" + strings.Repeat("0", 64)
	return err == nil && generation > 0 && r.Generation == strconv.FormatUint(generation, 10) &&
		hashPattern.MatchString(r.Challenge) && r.Challenge != zeroHash &&
		hashPattern.MatchString(r.RegistrationBinding) && r.RegistrationBinding != zeroHash &&
		r.Previous.Valid() && r.Previous.Hash != zeroHash && proofPattern.MatchString(r.ReviewDigest) &&
		r.ValidUntil > now.Unix() && r.ValidUntil <= now.Add(120*time.Second).Unix()
}

// ValidFor checks the echoed scope and live confirmation received over pinned
// mutual TLS. It cannot validate the underlying review or permit a transaction.
func (c CloseAnchorConfirmation) ValidFor(expected CloseAnchorRequest, now time.Time) bool {
	return expected.Valid(now) && c.CloseAnchorRequest == expected && c.Finalized.Valid() &&
		c.Finalized.Height >= expected.Previous.Height &&
		(c.Finalized.Height != expected.Previous.Height || c.Finalized.Hash == expected.Previous.Hash) &&
		c.ExpiresAt > now.Unix() && c.ExpiresAt <= expected.ValidUntil && c.ExpiresAt <= now.Add(5*time.Second).Unix()
}
