package commerce

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type rentalCleanupView struct {
	State string `json:"state"`
	Key   string `json:"requestKey,omitempty"`
}

// The current opsd can serve historical close reads and retries during rollout.
// Missing cleanup capability holds new opening and retirement operations.
func (h HTTP) rentalCleanupInstalled(ctx context.Context) (bool, error) {
	var installed bool
	err := h.Store.db.QueryRow(ctx, `SELECT pg_catalog.to_regprocedure(
		'ledger.rental_successor_cleanup_blocked(uuid,uuid,uuid,uuid,bigint)') IS NOT NULL`).Scan(&installed)
	return installed, err
}

func (c *ProtectedRentalClose) pendingCleanup(ctx context.Context, req protectedCloseRequest) (rentalCleanupView, error) {
	var view rentalCleanupView
	var raw []byte
	err := c.store.db.QueryRow(ctx, "SELECT ledger.rental_cleanup_pending($1,$2,$3,$4,$5)", req.arguments()...).Scan(&raw)
	if err == nil && json.Unmarshal(raw, &view) != nil {
		err = service.Unknown
	}
	return view, err
}

func (h HTTP) executeRentalCleanup(w http.ResponseWriter, r *http.Request, actor service.Principal, req protectedCloseRequest, sameCredential func() bool) {
	client, valid := service.CurrentVerifiedClient(r.TLS, time.Now())
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
	installed, err := h.rentalCleanupInstalled(ctx)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if !installed || !h.ProtectedWithdrawal || h.ProtectedClose == nil {
		var member []byte
		if err := h.Store.db.QueryRow(ctx, "SELECT ledger.rental_member($1,$2,$3,$4,$5)", req.arguments()...).Scan(&member); err != nil {
			operatorResponse(w, nil, err)
			return
		}
		writeJSON(w, http.StatusConflict, map[string]any{"cleanup": rentalCleanupView{State: "protected-evidence-unavailable"}})
		return
	}
	controller := h.ProtectedClose
	view, err := controller.pendingCleanup(ctx, req)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if view.State == "board-receipt-required" || view.State == "receipt-authenticated-handoff-required" {
		if view.Key == "" || req.Key != view.Key {
			operatorResponse(w, nil, service.Conflict)
			return
		}
		if !sameCredential() {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"cleanup": view})
		return
	}
	snapshot, err := controller.withdrawalSnapshot(ctx, req)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	approval, err := controller.withdrawalApproval(ctx, req, snapshot)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"cleanup": view})
		return
	}
	proof := approval.evidence
	_, freeze, disposition, err := controller.cleanupDisposition(ctx, req, approval)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	writer := controller.anchor.CallerFingerprint()
	scope := approval.plan
	lease := min(approval.lease, deadline.Unix())
	if lease <= controller.now().Unix() {
		operatorResponse(w, nil, service.Conflict)
		return
	}
	if !sameCredential() {
		http.NotFound(w, r)
		return
	}
	statement := "SELECT ledger.admit_rental_cleanup($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)"
	args := req.arguments(req.Key, writer[:], scope.ScopeDigest, proof.CloseHash[:], freeze[:], disposition[:], lease)
	var anchored bool
	if err = controller.store.db.QueryRow(ctx, cleanupIssuanceInstalledSQL).Scan(&anchored); err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if anchored {
		statement = "SELECT ledger.admit_anchored_rental_cleanup($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)"
		args = append(args, proof.FinalizedHeight, proof.FinalizedHash[:])
	}
	_, err = controller.store.db.Exec(ctx, statement, args...)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"cleanup": rentalCleanupView{State: "board-receipt-required", Key: req.Key}})
}

func (c *ProtectedRentalClose) cleanupDisposition(ctx context.Context, req protectedCloseRequest, approval protectedWithdrawalApproval) ([32]byte, [32]byte, [32]byte, error) {
	proof := approval.evidence
	var kind byte
	switch proof.State {
	case "withdrawal-received":
		kind = 1
	case "withdrawal-no-payout":
		kind = 2
	default:
		return [32]byte{}, [32]byte{}, [32]byte{}, service.Conflict
	}
	if proof.Amount == nil || proof.Amount.BitLen() > 256 || proof.FinalizedHeight <= 0 ||
		(kind == 1 && (proof.PayoutHash == (common.Hash{}) || proof.Amount.Sign() <= 0)) ||
		(kind == 2 && proof.Amount.Sign() != 0) {
		return [32]byte{}, [32]byte{}, [32]byte{}, service.Conflict
	}
	writer := c.anchor.CallerFingerprint()
	scope := approval.plan
	var freeze []byte
	err := c.store.db.QueryRow(ctx, "SELECT ledger.rental_cleanup_freeze($1,$2,$3,$4,$5,$6,$7,$8)",
		req.arguments(writer[:], scope.ScopeDigest, proof.CloseHash[:])...).Scan(&freeze)
	if err != nil {
		return [32]byte{}, [32]byte{}, [32]byte{}, err
	}
	if len(freeze) != 32 {
		return [32]byte{}, [32]byte{}, [32]byte{}, service.Unknown
	}
	binding := rentalCollectorBinding(req.Branch, req.Device, req.Generation, c.deployment, scope.Account,
		common.FromHex(scope.Registration), common.FromHex(scope.Measurement), common.FromHex(scope.Deployment))
	var amount [32]byte
	proof.Amount.FillBytes(amount[:])
	var height [8]byte
	binary.BigEndian.PutUint64(height[:], uint64(proof.FinalizedHeight))
	evidence := crypto.Keccak256Hash([]byte("NU54-rental-withdrawal-evidence-v1\x00"), proof.PayoutHash[:],
		proof.AcceptanceHash[:], amount[:], height[:])
	disposition, err := rentalv1.CleanupDispositionDigest(crypto.Keccak256Hash(binding[:]), [32]byte(freeze),
		proof.CloseHash, proof.FinalizedHash, kind, evidence)
	return crypto.Keccak256Hash(binding[:]), [32]byte(freeze), disposition, err
}
