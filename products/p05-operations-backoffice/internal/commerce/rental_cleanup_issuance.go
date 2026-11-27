package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
)

const cleanupIssuanceInstalledSQL = "SELECT pg_catalog.to_regprocedure('ledger.rental_cleanup_issue_scope(uuid,uuid,uuid,bigint,uuid,bytea)') IS NOT NULL"
const cleanupReceiptInstalledSQL = "SELECT pg_catalog.to_regprocedure('ledger.rental_cleanup_receipt_authority(uuid,uuid,uuid,bigint,uuid,bytea)') IS NOT NULL"
const cleanupIssueScopeSQL = "SELECT * FROM ledger.rental_cleanup_issue_scope($1,$2,$3,$4,$5,$6)"

type rentalCleanupIssueAuthority struct {
	actor       string
	disposition [32]byte
	height      int64
	anchor      common.Hash
	envelope    []byte
	board       common.Address
	until       time.Time
}

func readCleanupIssueAuthority(row pgx.Row) (rentalCleanupIssueAuthority, error) {
	var out rentalCleanupIssueAuthority
	var disposition, anchor []byte
	var board string
	if err := row.Scan(&out.actor, &disposition, &out.height, &anchor, &out.envelope, &board, &out.until); err != nil {
		return out, err
	}
	if len(disposition) != 32 || len(anchor) != 32 || !common.IsHexAddress(board) || out.height <= 0 {
		return out, service.Conflict
	}
	out.disposition, out.anchor, out.board = [32]byte(disposition), common.Hash(anchor), common.HexToAddress(board)
	return out, nil
}

func (h HTTP) rentalCleanupIssue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	auth, client, ok := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	req, valid := cleanupStationRequest(r, auth)
	if !ok || !valid || r.ContentLength != 0 || !h.ProtectedWithdrawal || h.ProtectedClose == nil {
		http.NotFound(w, r)
		return
	}
	deadline := min(auth.ExpiresAt, client.ExpiresAt.Unix())
	ctx, cancel := context.WithDeadline(r.Context(), time.Unix(deadline, 0))
	defer cancel()
	var installed bool
	if err := h.ProtectedClose.store.db.QueryRow(ctx, cleanupIssuanceInstalledSQL).Scan(&installed); err != nil || !installed {
		operatorResponse(w, nil, service.Conflict)
		return
	}
	raw, err := h.ProtectedClose.issueRentalCleanup(ctx, req, client.Fingerprint, deadline)
	current, currentClient, ok := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	if !ok || current.Subject != auth.Subject || current.Tenant != auth.Tenant || current.Role != auth.Role || currentClient.Fingerprint != client.Fingerprint {
		http.NotFound(w, r)
		return
	}
	if domainError(err) == service.Forbidden {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	intent, err := rentalv1.ParseCleanupIntent(raw)
	if err != nil || time.Now().Unix() >= int64(intent.ExpiresAt) || ctx.Err() != nil {
		operatorResponse(w, nil, service.Conflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"intent": hex.EncodeToString(raw)})
}

func (c *ProtectedRentalClose) issueRentalCleanup(ctx context.Context, req protectedCloseRequest, cert [32]byte, clientUntil int64) ([]byte, error) {
	args := []any{req.Tenant, req.Branch, req.Device, req.Generation, req.Key, cert[:]}
	prior, err := readCleanupIssueAuthority(c.store.db.QueryRow(ctx, cleanupIssueScopeSQL, args...))
	if err != nil {
		return nil, err
	}
	scope, financial, err := c.cleanupFinancialAuthority(ctx, req, prior)
	if err != nil {
		return nil, err
	}
	tx, err := c.store.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	locked, err := readCleanupIssueAuthority(tx.QueryRow(ctx, cleanupIssueScopeSQL, args...))
	if err != nil {
		return nil, err
	}
	if locked.actor != prior.actor || locked.disposition != prior.disposition || locked.height != prior.height ||
		locked.anchor != prior.anchor || locked.board != prior.board {
		return nil, service.Conflict
	}
	issued := c.now().Unix()
	expires := min(issued+rentalv1.CleanupIntentLifetime, financial.lease, locked.until.Unix(), clientUntil)
	if issued <= 0 || expires <= issued || ctx.Err() != nil {
		return nil, service.Conflict
	}
	envelope := locked.envelope
	if envelope == nil {
		envelope, err = signRentalCleanup(scope, c.operator, uint64(issued), uint64(expires))
		if err != nil {
			return nil, err
		}
	}
	intent, err := rentalv1.ParseCleanupIntent(envelope)
	if err != nil || intent.ExpiresAt > uint64(expires) || scope.VerifyIntent(intent, uint64(issued)) != nil {
		return nil, service.Conflict
	}
	args = append(financial.arguments(args), envelope, int64(intent.ExpiresAt))
	const pinSQL = "SELECT ledger.pin_rental_cleanup_issuance($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)"
	var pinSigner bool
	if err := tx.QueryRow(ctx, cleanupReceiptInstalledSQL).Scan(&pinSigner); err != nil {
		return nil, err
	}
	pinStatement, pinArgs := pinSQL, args
	if pinSigner {
		pinStatement = "SELECT ledger.pin_rental_cleanup_issuance_with_signer($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)"
		pinArgs = append(args, scope.Board[:])
	}
	var persisted []byte
	if err := tx.QueryRow(ctx, pinStatement, pinArgs...).Scan(&persisted); err != nil {
		return nil, err
	}
	if !bytes.Equal(persisted, envelope) {
		return nil, service.Conflict
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	// External chain checks never run while a transaction holds a device
	// or registry lock. Recheck local authority after this final external wait.
	if !financial.canonical(ctx, prior) {
		return nil, service.Conflict
	}
	var deliverable []byte
	args[11] = nil // At delivery, never recreate an absent committed envelope.
	if err := c.store.db.QueryRow(ctx, pinSQL, args...).
		Scan(&deliverable); err != nil {
		return nil, err
	}
	if !bytes.Equal(deliverable, persisted) || c.now().Unix() >= int64(intent.ExpiresAt) || ctx.Err() != nil {
		return nil, service.Conflict
	}
	return deliverable, nil
}

func signRentalCleanup(scope rentalv1.CleanupScope, key *ecdsa.PrivateKey, issued, expires uint64) ([]byte, error) {
	intent := rentalv1.CleanupIntent{Binding: scope.Binding, Disposition: scope.Disposition, Request: scope.Request, IssuedAt: issued, ExpiresAt: expires}
	digest := intent.Digest(scope.ChainID, scope.Settlement)
	sig, err := crypto.Sign(digest[:], key)
	if err != nil {
		return nil, service.Unknown
	}
	sig[64] += 27
	copy(intent.OperatorSignature[:], sig)
	if scope.VerifyIntent(intent, issued) != nil {
		return nil, service.Conflict
	}
	return intent.Bytes(), nil
}

func cleanupStationRequest(r *http.Request, auth service.Principal) (protectedCloseRequest, bool) {
	branch, device, key := r.PathValue("branch"), r.PathValue("device"), r.PathValue("key")
	generation, err := strconv.ParseInt(r.PathValue("generation"), 10, 64)
	if auth.Role != "rental-collector" || !operatorRef.MatchString(auth.Tenant) ||
		!operatorRef.MatchString(branch) || !operatorRef.MatchString(device) || !operatorRef.MatchString(key) ||
		err != nil || generation < 1 || strconv.FormatInt(generation, 10) != r.PathValue("generation") {
		return protectedCloseRequest{}, false
	}
	return protectedCloseRequest{Tenant: auth.Tenant, Branch: branch, Device: device, Generation: generation, Key: key}, true
}

type cleanupFinancialAuthority struct {
	chain                       protectedWithdrawalChain
	currentHeight, lease        int64
	currentAnchor, closeHash    common.Hash
	scopeDigest                 string
	writer, freeze, disposition [32]byte
}

func (f cleanupFinancialAuthority) arguments(args []any) []any {
	return append(args, f.writer[:], f.scopeDigest, f.closeHash[:], f.freeze[:], f.disposition[:])
}

func (f cleanupFinancialAuthority) canonical(ctx context.Context, prior rentalCleanupIssueAuthority) bool {
	return f.chain.ProtectedWithdrawalAnchorCurrent(ctx, prior.height, prior.anchor) &&
		(f.currentAnchor == prior.anchor || f.chain.ProtectedWithdrawalAnchorCurrent(ctx, f.currentHeight, f.currentAnchor))
}

type cleanupOperatorAuthority interface {
	ProtectedRentalOperatorAt(context.Context, int64, common.Hash) (common.Address, error)
}

func (c *ProtectedRentalClose) cleanupFinancialAuthority(ctx context.Context, req protectedCloseRequest, prior rentalCleanupIssueAuthority) (rentalv1.CleanupScope, cleanupFinancialAuthority, error) {
	req.Actor = prior.actor
	snapshot, err := c.withdrawalSnapshot(ctx, req)
	if err != nil {
		return rentalv1.CleanupScope{}, cleanupFinancialAuthority{}, err
	}
	approval, err := c.withdrawalApproval(ctx, req, snapshot)
	if err != nil {
		return rentalv1.CleanupScope{}, cleanupFinancialAuthority{}, err
	}
	chain, ok := c.chain.(protectedWithdrawalChain)
	operatorChain, registered := c.chain.(cleanupOperatorAuthority)
	if !ok || !registered || approval.evidence.FinalizedHeight < prior.height {
		return rentalv1.CleanupScope{}, cleanupFinancialAuthority{}, service.Conflict
	}
	operator, err := operatorChain.ProtectedRentalOperatorAt(ctx, prior.height, prior.anchor)
	if err != nil || operator != crypto.PubkeyToAddress(c.operator.PublicKey) {
		return rentalv1.CleanupScope{}, cleanupFinancialAuthority{}, service.Conflict
	}
	currentHeight, currentAnchor := approval.evidence.FinalizedHeight, approval.evidence.FinalizedHash
	// The new observation can have a newer final head. Reconstruct the
	// commitment using the original still-canonical height and block hash.
	approval.evidence.FinalizedHeight, approval.evidence.FinalizedHash = prior.height, prior.anchor
	binding, freeze, disposition, err := c.cleanupDisposition(ctx, req, approval)
	if err != nil {
		return rentalv1.CleanupScope{}, cleanupFinancialAuthority{}, err
	}
	if disposition != prior.disposition {
		return rentalv1.CleanupScope{}, cleanupFinancialAuthority{}, service.Conflict
	}
	var uuid [16]byte
	putCollectorUUID(uuid[:], req.Key)
	scope := rentalv1.CleanupScope{ChainID: c.deployment.ChainID, Settlement: c.deployment.Settlement,
		Operator: operator, Board: prior.board, Binding: binding, Disposition: disposition,
		Request: crypto.Keccak256Hash([]byte("NU54-rental-cleanup-request-v1\x00"), binding[:], uuid[:])}
	return scope, cleanupFinancialAuthority{chain: chain, currentHeight: currentHeight, currentAnchor: currentAnchor,
		closeHash: approval.evidence.CloseHash, scopeDigest: approval.plan.ScopeDigest, lease: approval.lease,
		writer: c.anchor.CallerFingerprint(), freeze: freeze, disposition: disposition}, nil
}
