package commerce

import (
	"context"
	"crypto/cipher"
	"crypto/ecdsa"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/common"
)

type ProtectedRentalClose struct {
	*protectedCloseExecutor
	store *postgresProtectedCloseStore
}

func OpenProtectedRentalClose(ctx context.Context, dsn string, chain *core.Chain, anchor *core.RentalEvidenceReader,
	seal cipher.AEAD, operator *ecdsa.PrivateKey, deployment core.Deployment, manifest [32]byte) (*ProtectedRentalClose, error) {
	executor, err := openProtectedCloseExecutor(ctx, dsn, chain, anchor, seal, operator, deployment, manifest)
	if err != nil {
		return nil, err
	}
	return &ProtectedRentalClose{protectedCloseExecutor: executor, store: executor.store.(*postgresProtectedCloseStore)}, nil
}

func (c *ProtectedRentalClose) Close() { c.store.db.Close() }

type protectedCloseSnapshot struct {
	Protected                bool
	Key, Review, State, Hash string
}

func (c *ProtectedRentalClose) snapshot(ctx context.Context, req protectedCloseRequest) (protectedCloseSnapshot, error) {
	var snapshot protectedCloseSnapshot
	var raw []byte
	err := c.store.db.QueryRow(ctx, "SELECT ledger.rental_close_operator_snapshot($1,$2,$3,$4,$5)", req.arguments()...).Scan(&raw)
	if err == nil && json.Unmarshal(raw, &snapshot) != nil {
		err = service.Unknown
	}
	return snapshot, err
}

func (s protectedCloseSnapshot) action() rentalAction {
	state := s.State
	if s.Key == "" {
		state = "recorded"
	}
	evidence, _ := json.Marshal(struct {
		Hash string `json:"transactionHash"`
	}{s.Hash})
	return rentalAction{Action: "close", Key: s.Key, Status: state, Evidence: evidence, ProtectedReviewRef: s.Review}
}

func (h HTTP) executeProtectedRentalClose(w http.ResponseWriter, r *http.Request, actor service.Principal,
	req protectedCloseRequest, sameCredential func() bool) {
	if !rentalReviewRef.MatchString(req.Review) {
		operatorResponse(w, nil, service.Conflict)
		return
	}
	client, valid := service.CurrentVerifiedClient(r.TLS, time.Now())
	if !valid || !sameCredential() {
		http.NotFound(w, r)
		return
	}
	deadline := time.Unix(actor.ExpiresAt, 0)
	if client.ExpiresAt.Before(deadline) {
		deadline = client.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	_, runErr := h.ProtectedClose.execute(ctx, req)
	current, err := h.ProtectedClose.snapshot(ctx, req)
	if !sameCredential() {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if req.RecoveryReview != "" && runErr != nil {
		operatorResponse(w, nil, runErr)
		return
	}
	if current.Key != req.Key || current.Review != req.Review || current.State == "" {
		if runErr == nil {
			runErr = service.Conflict
		}
		operatorResponse(w, nil, runErr)
		return
	}
	// A durable original is recoverable even if approval for a new send is now unavailable.
	writeJSON(w, http.StatusAccepted, map[string]any{"operations": []rentalAction{current.action()}})
}

func (h HTTP) protectedRentalReview(ctx context.Context, req protectedCloseRequest, member RentalMember,
	observation ops.RentalObservation) (RentalMember, ops.RentalObservation, error) {
	c := h.ProtectedClose
	snapshot, err := c.snapshot(ctx, req)
	if err != nil || !snapshot.Protected {
		return member, observation, err
	}
	if snapshot.State != "" {
		action := snapshot.action()
		if snapshot.Key != "" && (snapshot.State == "prepared" || snapshot.State == "attempting" || snapshot.State == "submitted") {
			plan, err := c.store.plan(ctx, req, c.anchor.CallerFingerprint())
			if err == nil && plan.ReviewRef != snapshot.Review && plan.ExpiresAt > c.now().Unix() &&
				plan.Account == member.Account && plan.Destination == member.Destination {
				action.ProtectedRecoveryReviewRef = plan.ReviewRef
			}
		}
		member.Actions = append(member.Actions, action)
		return member, observation, nil
	}
	writer := c.anchor.CallerFingerprint()
	plan, err := c.store.plan(ctx, req, writer)
	if err != nil || plan.ExpiresAt <= c.now().Unix() || plan.Account != member.Account || plan.Destination != member.Destination ||
		plan.Deployment != common.Hash(c.manifest).Hex() || plan.Settlement != strings.ToLower(c.deployment.Settlement.Hex()) {
		return member, observation, nil
	}
	preview := member
	preview.Hold = false
	live, err := h.Rental.Observe(ctx, preview)
	if err != nil || live.Status != "active" {
		return member, observation, nil
	}
	lease, err := c.confirm(ctx, req, plan)
	if err != nil {
		return member, observation, nil
	}
	current, err := c.store.plan(ctx, req, writer)
	if err != nil || current.ScopeDigest != plan.ScopeDigest || lease.ExpiresAt <= c.now().Unix() {
		return member, observation, nil
	}
	snapshot, err = c.snapshot(ctx, req)
	if err != nil {
		return member, observation, err
	}
	if snapshot.State != "" {
		member.Actions = append(member.Actions, snapshot.action())
		return member, observation, nil
	}
	if !snapshot.Protected || lease.ExpiresAt <= c.now().Unix() {
		return member, observation, nil
	}
	deadline, zero := time.Unix(plan.ExpiresAt, 0).UTC(), 0
	preview.ReturnReviewRef, preview.PendingEvidenceUntil = plan.ReviewRef, &deadline
	preview.GenerationPending = rentalGenerationPending{State: "verified", PendingCount: &zero, PendingPayments: &zero,
		PendingLimits: &zero, ValidUntil: &deadline}
	return preview, live, nil
}
