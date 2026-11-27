package service

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"time"
)

type Evidence struct {
	EventID    string `json:"eventId"`
	Generation string `json:"generationRef"`
	Kind       string `json:"kind"`
	AnchorHash string `json:"anchorHash"`
	Fact       string `json:"fact"`
	Merchant   string `json:"merchantKey"`
	Order      string `json:"orderKey"`
	TxHash     string `json:"txHash"`
	ObservedAt int64  `json:"observedAt"`
	ExpiresAt  int64  `json:"expiresAt"`
}

type Reply struct {
	RequestID     string    `json:"requestId"`
	RequestDigest string    `json:"requestDigest"`
	Status        string    `json:"status"`
	Error         Code      `json:"error,omitempty"`
	ResultRef     string    `json:"resultRef,omitempty"`
	Evidence      *Evidence `json:"evidence,omitempty"`
}

// DecodeReply accepts one bounded JSON result from a C3–C9 owner.
func DecodeReply(input io.Reader) (Reply, error) {
	blob, err := io.ReadAll(io.LimitReader(input, rules.MaxEnvelopeBytes+1))
	if err != nil || int64(len(blob)) > rules.MaxEnvelopeBytes {
		return Reply{}, BadRequest
	}
	keys := json.NewDecoder(bytes.NewReader(blob))
	if !uniqueObject(keys, 0, "evidence") {
		return Reply{}, BadRequest
	}
	if _, err := keys.Token(); err != io.EOF {
		return Reply{}, BadRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(blob))
	decoder.DisallowUnknownFields()
	var reply Reply
	if err := decoder.Decode(&reply); err != nil {
		return Reply{}, BadRequest
	}
	return reply, nil
}

// Outcome binds an authenticated response envelope to an authorized request.
func Outcome(bound Bound, status string) Reply {
	return Reply{RequestID: bound.Request.RequestID, RequestDigest: bound.RequestDigest, Status: status}
}

// ValidateReply enforces result binding; domain state transitions stay with product owners.
func EvidenceTTL() time.Duration { return time.Duration(rules.MaxEvidenceAgeSeconds) * time.Second }

func ValidateReply(bound Bound, reply Reply, now time.Time) error {
	if bound.Request.ExpiresAt <= now.Unix() {
		return Expired
	}
	if !valid("digest", bound.RequestDigest) || reply.RequestID != bound.Request.RequestID ||
		reply.RequestDigest != bound.RequestDigest {
		return Mismatch
	}
	if reply.Status == "error" {
		if !slices.Contains([]Code{BadRequest, Version, Capability, Forbidden, Expired, Conflict, Busy, Unknown, Mismatch, OutOfStock, CatalogChanged}, reply.Error) ||
			reply.ResultRef != "" || reply.Evidence != nil {
			return BadRequest
		}
		return nil
	}
	if reply.Error != "" {
		return BadRequest
	}
	if bound.rule.Response != "evidence" {
		if !slices.Contains([]string{"ok", "accepted"}, reply.Status) || !valid("uuid", reply.ResultRef) || reply.Evidence != nil {
			return BadRequest
		}
		return nil
	}
	if reply.ResultRef != "" {
		return BadRequest
	}
	if reply.Status != bound.Request.Body["kind"] {
		return Mismatch
	}
	if reply.Status == "unobserved" {
		if reply.Evidence != nil {
			return Mismatch
		}
		return nil
	}
	e := reply.Evidence
	if e == nil || !valid("uuid", e.EventID) || e.EventID != bound.Request.Object ||
		e.Generation != bound.Request.Domain || e.Kind != reply.Status || !valid("digest", e.AnchorHash) ||
		(reply.Status == "finalized" && (!slices.Contains([]string{"paid", "refunded", "invalidated"}, e.Fact) ||
			(!valid("digest", e.Merchant) && !valid("address", e.Merchant)) || !valid("digest", e.Order) || !valid("digest", e.TxHash))) ||
		(reply.Status == "revoked" && (e.Fact != "revoked" || !valid("digest", e.Order) || e.TxHash != "")) ||
		e.ObservedAt <= 0 || e.ObservedAt > now.Unix() || e.ExpiresAt <= now.Unix() ||
		e.ExpiresAt-e.ObservedAt > rules.MaxEvidenceAgeSeconds {
		return Mismatch
	}
	return nil
}
