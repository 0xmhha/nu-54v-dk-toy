package commerce

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type protectedWithdrawalChain interface {
	ObserveProtectedWithdrawal(context.Context, common.Address, common.Address, common.Hash) (core.ProtectedWithdrawalEvidence, error)
	PrepareProtectedWithdrawal(context.Context, *ecdsa.PrivateKey, common.Address) (*types.Transaction, error)
	PublishProtectedWithdrawal(context.Context, *types.Transaction) error
	ProtectedWithdrawalUnseen(context.Context, common.Address, uint64, common.Hash) (bool, error)
	ProtectedWithdrawalAnchorCurrent(context.Context, int64, common.Hash) bool
	DecodeProtectedWithdrawal([]byte, common.Address, common.Address, uint64, common.Hash) (*types.Transaction, error)
}

type protectedWithdrawalSnapshot struct {
	Protected, Current                                                       bool
	CloseState, CloseHash, Account, Destination, WithdrawalHash, Key, Review string
}

type protectedWithdrawalView struct {
	State             string                       `json:"state"`
	CloseHash         string                       `json:"closeHash"`
	Amount            string                       `json:"amount,omitempty"`
	WithdrawAfter     uint64                       `json:"withdrawAfter,omitempty"`
	WithdrawalHash    string                       `json:"withdrawalHash,omitempty"`
	PayoutHash        string                       `json:"payoutHash,omitempty"`
	AcceptanceHash    string                       `json:"acceptanceHash,omitempty"`
	RequestKey        string                       `json:"requestKey,omitempty"`
	ReviewRef         string                       `json:"reviewRef,omitempty"`
	RecoveryReviewRef string                       `json:"recoveryReviewRef,omitempty"`
	Attempt           *protectedWithdrawalTerminal `json:"attempt,omitempty"`
}

func (c *ProtectedRentalClose) withdrawalSnapshot(ctx context.Context, req protectedCloseRequest) (protectedWithdrawalSnapshot, error) {
	var snapshot protectedWithdrawalSnapshot
	var raw []byte
	err := c.store.db.QueryRow(ctx, "SELECT ledger.rental_protected_withdrawal_snapshot($1,$2,$3,$4,$5)", req.arguments()...).Scan(&raw)
	if err == nil && json.Unmarshal(raw, &snapshot) != nil {
		err = service.Unknown
	}
	return snapshot, err
}

func (s protectedWithdrawalSnapshot) view() protectedWithdrawalView {
	state := "withdrawal-indeterminate"
	if s.CloseState != "confirmed" {
		state = "withdrawal-awaiting-close"
	}
	if s.WithdrawalHash != "" {
		state = "withdrawal-request-recorded"
	}
	return protectedWithdrawalView{State: state, CloseHash: s.CloseHash, RequestKey: s.Key, WithdrawalHash: s.WithdrawalHash}
}

type protectedWithdrawalApproval struct {
	plan     protectedClosePlan
	lease    int64
	evidence core.ProtectedWithdrawalEvidence
	ref      string
}

func (c *ProtectedRentalClose) withdrawalApproval(ctx context.Context, req protectedCloseRequest, scope protectedWithdrawalSnapshot) (protectedWithdrawalApproval, error) {
	chain, ok := c.chain.(protectedWithdrawalChain)
	if !ok || !scope.Current {
		return protectedWithdrawalApproval{}, service.Conflict
	}
	writer := c.anchor.CallerFingerprint()
	plan, err := c.store.plan(ctx, req, writer)
	if err != nil || plan.ExpiresAt <= c.now().Unix() || plan.Account != scope.Account || plan.Destination != scope.Destination ||
		plan.Deployment != common.Hash(c.manifest).Hex() || plan.Settlement != strings.ToLower(c.deployment.Settlement.Hex()) {
		return protectedWithdrawalApproval{}, service.Conflict
	}
	proof, err := chain.ObserveProtectedWithdrawal(ctx, common.HexToAddress(scope.Account), common.HexToAddress(scope.Destination), common.HexToHash(scope.CloseHash))
	if err != nil {
		return protectedWithdrawalApproval{}, service.Conflict
	}
	lease, err := c.confirm(ctx, req, plan)
	if err != nil || lease.Finalized.Height != proof.FinalizedHeight || lease.Finalized.Hash != proof.FinalizedHash.Hex() {
		return protectedWithdrawalApproval{}, service.Conflict
	}
	current, err := c.store.plan(ctx, req, writer)
	if err != nil || current.ScopeDigest != plan.ScopeDigest || lease.ExpiresAt <= c.now().Unix() {
		return protectedWithdrawalApproval{}, service.Conflict
	}
	reference, err := json.Marshal([]any{"rental-protected-withdrawal-v1", plan.ScopeDigest, req.Tenant, req.Branch, req.Device, req.Generation,
		scope.CloseHash, scope.Account, scope.Destination, proof.Amount.String(), proof.WithdrawAfter})
	if err != nil {
		return protectedWithdrawalApproval{}, service.Unknown
	}
	return protectedWithdrawalApproval{plan: plan, lease: lease.ExpiresAt, evidence: proof, ref: hex.EncodeToString(crypto.Keccak256(reference))}, nil
}

func (c *ProtectedRentalClose) withdrawalView(ctx context.Context, req protectedCloseRequest) (*protectedWithdrawalView, error) {
	snapshot, err := c.withdrawalSnapshot(ctx, req)
	if err != nil || !snapshot.Protected {
		return nil, err
	}
	view := snapshot.view()
	approval, err := c.withdrawalApproval(ctx, req, snapshot)
	if err != nil {
		return &view, nil
	}
	current, err := c.withdrawalSnapshot(ctx, req)
	if err != nil {
		return nil, err
	}
	if current != snapshot || approval.lease <= c.now().Unix() {
		view = current.view()
		return &view, nil
	}
	if snapshot.WithdrawalHash != "" {
		terminal, err := c.resolvedWithdrawal(ctx, req)
		if err != nil {
			return nil, err
		}
		if terminal != nil && terminal.OriginalHash == snapshot.WithdrawalHash {
			view.Attempt = terminal
		}
	}
	evidence := approval.evidence
	view.State, view.Amount, view.WithdrawAfter = evidence.State, evidence.Amount.String(), evidence.WithdrawAfter
	if evidence.PayoutHash != (common.Hash{}) {
		view.PayoutHash = evidence.PayoutHash.Hex()
	}
	if evidence.AcceptanceHash != (common.Hash{}) {
		view.AcceptanceHash = evidence.AcceptanceHash.Hex()
	}
	if snapshot.WithdrawalHash != "" && evidence.State == "withdrawal-ready" {
		view.State = "withdrawal-request-recorded"
		if snapshot.Key != "" {
			prior, err := c.precedingWithdrawal(ctx, req, snapshot.WithdrawalHash)
			if err != nil {
				return nil, err
			}
			if prior == nil || c.withdrawalTerminalAnchorCurrent(ctx, prior, approval) {
				view.ReviewRef = snapshot.Review
				if snapshot.Review != approval.ref && snapshot.Review != prior.retryReview(approval) {
					view.RecoveryReviewRef = approval.ref
				}
			}
		}
	}
	if retry := c.retryWithdrawalReview(ctx, view.Attempt, approval); retry != "" {
		view.State = "withdrawal-retry-review"
		view.ReviewRef, view.RecoveryReviewRef = retry, ""
	}
	if view.State == "withdrawal-ready" && snapshot.WithdrawalHash == "" {
		view.ReviewRef = approval.ref
	}
	return &view, nil
}

func (h HTTP) executeProtectedWithdrawal(w http.ResponseWriter, r *http.Request, actor service.Principal,
	req protectedCloseRequest, sameCredential func() bool) {
	if !sameCredential() || !rentalReviewRef.MatchString(req.Review) {
		operatorResponse(w, nil, service.Conflict)
		return
	}
	now := time.Now()
	client, valid := service.CurrentVerifiedClient(r.TLS, now)
	if !valid {
		http.NotFound(w, r)
		return
	}
	deadline := time.Unix(actor.ExpiresAt, 0)
	if client.ExpiresAt.Before(deadline) {
		deadline = client.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	c := h.ProtectedClose
	snapshot, err := c.withdrawalSnapshot(ctx, req)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	respond := func(current protectedWithdrawalSnapshot) {
		if !sameCredential() {
			http.NotFound(w, r)
		} else if current.Key != req.Key || current.Review != req.Review || current.WithdrawalHash == "" {
			operatorResponse(w, nil, service.Conflict)
		} else {
			writeJSON(w, http.StatusAccepted, map[string]any{"withdrawal": current.view()})
		}
	}
	archived, err := c.archivedWithdrawal(ctx, req)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if archived != nil {
		if !sameCredential() {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"withdrawal": archived})
		return
	}
	replacing := snapshot.WithdrawalHash != "" && snapshot.Key != req.Key
	if snapshot.WithdrawalHash != "" && !replacing && snapshot.Review != req.Review {
		respond(snapshot)
		return
	}
	approval, err := c.withdrawalApproval(ctx, req, snapshot)
	var predecessor *protectedWithdrawalTerminal
	if replacing {
		if err != nil || req.RecoveryReview != "" {
			operatorResponse(w, nil, service.Conflict)
			return
		}
		predecessor, err = c.resolvedWithdrawal(ctx, req)
		if err != nil || predecessor == nil || predecessor.OriginalHash != snapshot.WithdrawalHash ||
			c.retryWithdrawalReview(ctx, predecessor, approval) != req.Review {
			operatorResponse(w, nil, service.Conflict)
			return
		}
	}
	approvedRef := req.Review
	if replacing {
		approvedRef = approval.ref
	} else if snapshot.WithdrawalHash != "" && err == nil {
		prior, lookupErr := c.precedingWithdrawal(ctx, req, snapshot.WithdrawalHash)
		if lookupErr != nil {
			operatorResponse(w, nil, lookupErr)
			return
		}
		if prior != nil && !c.withdrawalTerminalAnchorCurrent(ctx, prior, approval) {
			if req.RecoveryReview != "" {
				operatorResponse(w, nil, service.Conflict)
			} else {
				respond(snapshot)
			}
			return
		}
		if prior.retryReview(approval) == req.Review {
			approvedRef = approval.ref
		}
	}
	if req.RecoveryReview != "" {
		if snapshot.WithdrawalHash == "" || err != nil || req.RecoveryReview != approval.ref {
			operatorResponse(w, nil, service.Conflict)
			return
		}
		approvedRef = req.RecoveryReview
	}
	if err != nil || approval.ref != approvedRef || approval.evidence.State != "withdrawal-ready" {
		if snapshot.WithdrawalHash != "" {
			if err == nil && (approval.evidence.State == "withdrawal-received" || approval.evidence.State == "withdrawal-held") {
				hash := common.HexToHash(snapshot.WithdrawalHash)
				_, _ = c.store.db.Exec(ctx, "SELECT ledger.rental_protected_withdrawal_clear($1,$2,$3,$4,$5,$6)", req.arguments(hash[:])...)
			}
			respond(snapshot)
			return
		}
		operatorResponse(w, nil, service.Conflict)
		return
	}
	chain := c.chain.(protectedWithdrawalChain)
	account, signer := common.HexToAddress(snapshot.Account), crypto.PubkeyToAddress(c.operator.PublicKey)
	writer, closeHash := c.anchor.CallerFingerprint(), common.HexToHash(snapshot.CloseHash)
	hash := common.HexToHash(snapshot.WithdrawalHash)
	contextFor := func(hash common.Hash) []byte {
		return append(append([]byte("nu54-protected-withdrawal-v1:"+closeHash.Hex()+":"), closeCandidateContext(req, hash)...), byte(0))
	}
	var tx *types.Transaction
	rechecked := approval
	if snapshot.WithdrawalHash == "" || replacing {
		tx, err = chain.PrepareProtectedWithdrawal(ctx, c.operator, account)
		if err != nil || tx == nil {
			operatorResponse(w, nil, service.Conflict)
			return
		}
		raw, err := tx.MarshalBinary()
		if err != nil {
			operatorResponse(w, nil, service.Unknown)
			return
		}
		defer clear(raw)
		hash = tx.Hash()
		rechecked, err = c.withdrawalApproval(ctx, req, snapshot)
		if err != nil || rechecked.ref != approval.ref || rechecked.evidence.State != "withdrawal-ready" || !sameCredential() ||
			(replacing && c.retryWithdrawalReview(ctx, predecessor, rechecked) != req.Review) {
			operatorResponse(w, nil, service.Conflict)
			return
		}
		sealed := c.cipher.Seal(nil, nil, raw, contextFor(hash))
		var admitted bool
		args := []any{req.Key, req.Review, closeHash[:], hash[:], writer[:], rechecked.plan.ScopeDigest,
			rechecked.lease, strings.ToLower(signer.Hex()), strconv.FormatUint(tx.Nonce(), 10), sealed}
		statement := "SELECT ledger.rental_protected_withdrawal_admit($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::text::numeric,$15)"
		if replacing {
			predecessorHash, evidenceHash := common.HexToHash(predecessor.OriginalHash), common.HexToHash(predecessor.EvidenceHash)
			args = append([]any{predecessorHash[:], evidenceHash[:]}, args...)
			statement = "SELECT ledger.rental_protected_withdrawal_retry($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::text::numeric,$17)"
		}
		err = c.store.db.QueryRow(ctx, statement, req.arguments(args...)...).Scan(&admitted)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		current, err := c.withdrawalSnapshot(ctx, req)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		snapshot = current
		if !admitted {
			respond(snapshot)
			return
		}
	} else {
		var storedSigner, storedNonce string
		var sealed []byte
		err = c.store.db.QueryRow(ctx, "SELECT signer,nonce::text,sealed_transaction FROM ledger.rental_protected_withdrawal_recover($1,$2,$3,$4,$5,$6,$7)",
			req.arguments(req.Key, req.Review)...).Scan(&storedSigner, &storedNonce, &sealed)
		nonce, parseErr := strconv.ParseUint(storedNonce, 10, 64)
		if err != nil || parseErr != nil || storedSigner != strings.ToLower(signer.Hex()) || len(sealed) == 0 {
			respond(snapshot)
			return
		}
		raw, err := c.cipher.Open(nil, nil, sealed, contextFor(hash))
		if err != nil {
			respond(snapshot)
			return
		}
		defer clear(raw)
		tx, err = chain.DecodeProtectedWithdrawal(raw, account, signer, nonce, hash)
		if err != nil {
			respond(snapshot)
			return
		}
	}
	unseen, err := chain.ProtectedWithdrawalUnseen(ctx, signer, tx.Nonce(), hash)
	if err != nil || !unseen || !sameCredential() {
		respond(snapshot)
		return
	}
	var claimed bool
	err = c.store.db.QueryRow(ctx, "SELECT ledger.rental_protected_withdrawal_claim($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)",
		req.arguments(req.Key, req.Review, closeHash[:], hash[:], writer[:], rechecked.plan.ScopeDigest, rechecked.lease, strings.ToLower(signer.Hex()))...).Scan(&claimed)
	if err == nil && claimed {
		sendCtx, stop := context.WithDeadline(ctx, time.Unix(rechecked.lease, 0))
		_ = chain.PublishProtectedWithdrawal(sendCtx, tx)
		stop()
	}
	respond(snapshot)
}
