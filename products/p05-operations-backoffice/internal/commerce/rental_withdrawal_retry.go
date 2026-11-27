package commerce

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func (terminal *protectedWithdrawalTerminal) retryReview(approval protectedWithdrawalApproval) string {
	if terminal == nil || (terminal.State != "reverted" && terminal.State != "superseded") ||
		approval.evidence.State != "withdrawal-ready" {
		return ""
	}
	digest := crypto.Keccak256Hash([]byte("nu54-withdrawal-next-review-v1:" + terminal.OriginalHash + ":" +
		terminal.EvidenceHash + ":" + terminal.State + ":" + strconv.FormatInt(terminal.FinalizedHeight, 10) + ":" + terminal.FinalizedHash + ":" + approval.ref))
	return digest.Hex()[2:]
}

func (c *ProtectedRentalClose) retryWithdrawalReview(ctx context.Context, terminal *protectedWithdrawalTerminal,
	approval protectedWithdrawalApproval) string {
	review := terminal.retryReview(approval)
	if review == "" || !c.withdrawalTerminalAnchorCurrent(ctx, terminal, approval) {
		return ""
	}
	return review
}

func (c *ProtectedRentalClose) withdrawalTerminalAnchorCurrent(ctx context.Context, terminal *protectedWithdrawalTerminal,
	approval protectedWithdrawalApproval) bool {
	chain, ok := c.chain.(protectedWithdrawalChain)
	return ok && terminal.FinalizedHeight <= approval.evidence.FinalizedHeight &&
		chain.ProtectedWithdrawalAnchorCurrent(ctx, terminal.FinalizedHeight, common.HexToHash(terminal.FinalizedHash))
}

func (c *ProtectedRentalClose) archivedWithdrawal(ctx context.Context, req protectedCloseRequest) (*protectedWithdrawalView, error) {
	var view protectedWithdrawalView
	var raw []byte
	err := c.store.db.QueryRow(ctx, "SELECT ledger.rental_withdrawal_archived($1,$2,$3,$4,$5,$6,$7)",
		req.arguments(req.Key, req.Review)...).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(raw, &view) != nil {
		return nil, service.Unknown
	}
	if view.State == "" {
		return nil, nil
	}
	return &view, nil
}

func (c *ProtectedRentalClose) precedingWithdrawal(ctx context.Context, req protectedCloseRequest,
	currentHash string) (*protectedWithdrawalTerminal, error) {
	var terminal protectedWithdrawalTerminal
	var raw []byte
	hash := common.HexToHash(currentHash)
	err := c.store.db.QueryRow(ctx, "SELECT ledger.rental_withdrawal_predecessor($1,$2,$3,$4,$5,$6)",
		req.arguments(hash[:])...).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(raw, &terminal) != nil {
		return nil, service.Unknown
	}
	if terminal.State == "" {
		return nil, nil
	}
	return &terminal, nil
}
