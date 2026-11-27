package commerce

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func (h HTTP) rentalCleanupReceipt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	auth, client, ok := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	req, valid := cleanupStationRequest(r, auth)
	if !ok || !valid || (r.Method != http.MethodPost && (r.Method != http.MethodGet || r.ContentLength != 0)) {
		http.NotFound(w, r)
		return
	}
	var proposed []byte
	if r.Method == http.MethodPost {
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		var err error
		proposed, err = io.ReadAll(http.MaxBytesReader(w, r.Body, rentalv1.CleanupReceiptSize))
		if _, parseErr := rentalv1.ParseCleanupReceipt(proposed); err != nil || parseErr != nil {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
	}
	deadline := min(auth.ExpiresAt, client.ExpiresAt.Unix())
	ctx, cancel := context.WithDeadline(r.Context(), time.Unix(deadline, 0))
	defer cancel()
	var installed bool
	if err := h.ProtectedClose.store.db.QueryRow(ctx, cleanupReceiptInstalledSQL).Scan(&installed); err != nil || !installed {
		operatorResponse(w, nil, service.Conflict)
		return
	}
	raw, validUntil, err := h.ProtectedClose.recordRentalCleanupReceipt(ctx, req, client.Fingerprint, deadline, proposed)
	current, currentClient, ok := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	if !ok || current.Subject != auth.Subject || current.Tenant != auth.Tenant || current.Role != auth.Role ||
		currentClient.Fingerprint != client.Fingerprint || ctx.Err() != nil || domainError(err) == service.Forbidden {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if raw == nil || !time.Now().Before(validUntil) {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"receipt": hex.EncodeToString(raw)})
}

func (c *ProtectedRentalClose) recordRentalCleanupReceipt(ctx context.Context, req protectedCloseRequest, cert [32]byte, clientUntil int64, proposed []byte) ([]byte, time.Time, error) {
	args := []any{req.Tenant, req.Branch, req.Device, req.Generation, req.Key, cert[:]}
	var envelope, board, disposition, anchor, registration, measurement, deployment, source []byte
	var height int64
	var settlement, account string
	var authorityUntil time.Time
	err := c.store.db.QueryRow(ctx, "SELECT * FROM ledger.rental_cleanup_receipt_authority($1,$2,$3,$4,$5,$6)", args...).
		Scan(&envelope, &board, &disposition, &height, &anchor, &registration, &measurement, &deployment, &settlement, &account, &authorityUntil, &source)
	if err != nil {
		return nil, time.Time{}, err
	}
	intent, err := rentalv1.ParseCleanupIntent(envelope)
	if err != nil || !common.IsHexAddress(account) || c.deployment.Settlement != common.HexToAddress(settlement) ||
		!bytes.Equal(deployment, c.manifest[:]) || len(board) != 20 || len(disposition) != 32 || len(anchor) != 32 || height <= 0 {
		return nil, time.Time{}, service.Conflict
	}
	operatorChain, ok := c.chain.(cleanupOperatorAuthority)
	if !ok {
		return nil, time.Time{}, service.Conflict
	}
	// The immutable contract operator is independent of opsd's running key.
	// This lookup also verifies the original disposition block is canonical.
	operator, err := operatorChain.ProtectedRentalOperatorAt(ctx, height, common.Hash(anchor))
	if err != nil {
		return nil, time.Time{}, service.Conflict
	}
	binding := rentalCollectorBinding(req.Branch, req.Device, req.Generation, c.deployment, account, registration, measurement, deployment)
	bindingHash := crypto.Keccak256Hash(binding[:])
	var uuid [16]byte
	putCollectorUUID(uuid[:], req.Key)
	scope := rentalv1.CleanupScope{ChainID: c.deployment.ChainID, Settlement: c.deployment.Settlement,
		Operator: operator, Board: common.Address(board), Binding: bindingHash,
		Disposition: [32]byte(disposition), Request: crypto.Keccak256Hash([]byte("NU54-rental-cleanup-request-v1\x00"), bindingHash[:], uuid[:])}
	verify := func(raw []byte) bool {
		receipt, err := rentalv1.ParseCleanupReceipt(raw)
		now := c.now().Unix()
		return err == nil && now > 0 && scope.VerifyReceipt(intent, receipt, uint64(now)) == nil
	}
	if proposed != nil && !verify(proposed) {
		return nil, time.Time{}, service.Conflict
	}
	if min(clientUntil, authorityUntil.Unix()) <= c.now().Unix() || ctx.Err() != nil {
		return nil, time.Time{}, service.Conflict
	}
	args = append(args, envelope, board, source, proposed, clientUntil)
	var committed []byte
	var validUntil time.Time
	if err := c.store.db.QueryRow(ctx, "SELECT * FROM ledger.record_rental_cleanup_receipt($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)", args...).Scan(&committed, &validUntil); err != nil {
		return nil, time.Time{}, err
	}
	if committed != nil && (!verify(committed) || (proposed != nil && !bytes.Equal(committed, proposed))) {
		return nil, time.Time{}, service.Conflict
	}
	return committed, validUntil, nil
}
