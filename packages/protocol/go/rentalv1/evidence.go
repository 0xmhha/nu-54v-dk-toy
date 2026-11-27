// Package rentalv1 defines the private cross-service rental evidence response.
package rentalv1

import (
	"errors"
	"math/bits"
	"regexp"
	"time"
)

const ReconcilePath = "/v2/private/rental-generation/reconcile"

type Binding struct {
	Stream           string `json:"streamId,omitempty"`
	StreamGeneration string `json:"streamGeneration,omitempty"`
	Deployment       string `json:"deploymentDigest,omitempty"`
	FreezeReceipt    string `json:"freezeReceiptDigest,omitempty"`
	JournalRoot      string `json:"journalRoot,omitempty"`
}

type Anchor struct {
	Height int64  `json:"height"`
	Hash   string `json:"hash"`
}

func (a Anchor) Valid() bool { return a.Height > 0 && hashPattern.MatchString(a.Hash) }

type Request struct {
	Projection                   []byte  `json:"projection"`
	Receipt                      []byte  `json:"freezeReceipt"`
	Previous                     *Anchor `json:"previousAnchor,omitempty"`
	IncludePendingPaymentEntries bool    `json:"includePendingPaymentEntries,omitempty"`
}

type Assessment struct {
	Status string `json:"status"`
	Binding
	PendingCount          *int    `json:"pendingCount,omitempty"`
	PendingPayments       *int    `json:"pendingPayments,omitempty"`
	PendingLimits         *int    `json:"pendingLimits,omitempty"`
	PendingPaymentEntries *uint8  `json:"pendingPaymentEntries,omitempty"`
	Previous              *Anchor `json:"previousAnchor,omitempty"`
	AnchorHeight          *int64  `json:"anchorHeight,omitempty"`
	AnchorHash            string  `json:"anchorHash,omitempty"`
	AnchorTime            int64   `json:"anchorTime,omitempty"`
	Digest                string  `json:"digest,omitempty"`
	ValidUntil            int64   `json:"validUntil,omitempty"`
}

var (
	refPattern           = regexp.MustCompile(`^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)
	hashPattern          = regexp.MustCompile(`^0x[0-9a-f]{64}$`)
	proofPattern         = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	ErrInvalidAssessment = errors.New("rental assessment did not match the authenticated request")
)

func (b Binding) valid() bool {
	return refPattern.MatchString(b.Stream) && refPattern.MatchString(b.StreamGeneration) &&
		hashPattern.MatchString(b.Deployment) && hashPattern.MatchString(b.FreezeReceipt) && hashPattern.MatchString(b.JournalRoot)
}

// Validate pins a conclusive response to data obtained from both authorities.
func (a Assessment) Validate(expected Binding, count int, frozenAt, freezeExpiry, now time.Time) error {
	switch a.Status {
	case "unknown", "incomplete":
		if a != (Assessment{Status: a.Status}) {
			return ErrInvalidAssessment
		}
		return nil
	case "pending", "verified":
		if !expected.valid() || a.Binding != expected || a.PendingCount == nil || a.PendingPayments == nil || a.PendingLimits == nil ||
			a.AnchorHeight == nil || *a.AnchorHeight < 1 ||
			(a.Previous != nil && (!a.Previous.Valid() || a.Previous.Height > *a.AnchorHeight ||
				(a.Previous.Height == *a.AnchorHeight && a.Previous.Hash != a.AnchorHash))) || !hashPattern.MatchString(a.AnchorHash) || !proofPattern.MatchString(a.Digest) ||
			a.AnchorTime < frozenAt.Unix() || a.AnchorTime > now.Unix() || a.ValidUntil <= now.Unix() ||
			a.ValidUntil > freezeExpiry.Unix() || count < 0 || count > 8 {
			return ErrInvalidAssessment
		}
		payments, limits := *a.PendingPayments, *a.PendingLimits
		if payments < 0 || payments > count || limits < 0 || limits > count || *a.PendingCount != payments+limits ||
			*a.PendingCount > count ||
			(a.PendingPaymentEntries != nil && (bits.OnesCount8(*a.PendingPaymentEntries) != payments || uint16(*a.PendingPaymentEntries) >= 1<<count)) ||
			(a.Status == "verified") != (*a.PendingCount == 0) {
			return ErrInvalidAssessment
		}
		return nil
	default:
		return ErrInvalidAssessment
	}
}
