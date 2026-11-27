package commerce

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
)

type RenterReturnEvidence struct {
	Reader     *core.RentalEvidenceReader
	Deployment core.Deployment
	Manifest   [32]byte
}

type renterReturnAssessment struct {
	protectedClosePlan
	Branch, Device                               string
	Generation                                   int64
	AsOf                                         time.Time
	PendingCount, PendingPayments, PendingLimits *int
}

func (h HTTP) addRenterReturnEvidence(ctx context.Context, actor service.Principal, assignment string) *renterReturnAssessment {
	evidence := h.RenterEvidence
	writer := evidence.Reader.CallerFingerprint()
	load := func() (renterReturnAssessment, bool) {
		var record renterReturnAssessment
		var raw []byte
		err := h.Store.db.QueryRow(ctx, `SELECT ledger.renter_return_assessment($1,$2,$3,$4)`,
			actor.Tenant, actor.Subject, assignment, writer[:]).Scan(&raw)
		ok := err == nil && len(raw) > 0 && json.Unmarshal(raw, &record) == nil
		return record, ok
	}
	plan, ok := load()
	if !ok || plan.ScopeDigest == "" || plan.Deployment != common.Hash(evidence.Manifest).Hex() ||
		plan.Settlement != strings.ToLower(evidence.Deployment.Settlement.Hex()) ||
		plan.PendingCount == nil || plan.PendingPayments == nil || plan.PendingLimits == nil ||
		*plan.PendingPayments < 0 || *plan.PendingLimits < 0 || *plan.PendingCount > 8 ||
		*plan.PendingPayments+*plan.PendingLimits != *plan.PendingCount || plan.AsOf.IsZero() || plan.AsOf.After(time.Now()) {
		return nil
	}
	req := protectedCloseRequest{Branch: plan.Branch, Device: plan.Device, Generation: plan.Generation}
	expected := rentalAnchorRequest(req, evidence.Deployment, plan.protectedClosePlan)
	confirmed, err := evidence.Reader.ConfirmCloseAnchor(ctx, expected)
	if err != nil || !confirmed.ValidFor(expected, time.Now()) || confirmed.Finalized != expected.Previous {
		return nil
	}
	current, ok := load()
	if !ok || current.ScopeDigest != plan.ScopeDigest || !confirmed.ValidFor(expected, time.Now()) {
		return nil
	}
	return &plan
}
