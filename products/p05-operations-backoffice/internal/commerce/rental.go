package commerce

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

var rentalReviewRef = regexp.MustCompile(`^[0-9a-f]{64}$`)

type RentalMember struct {
	Account              string                  `json:"account"`
	Destination          string                  `json:"destination"`
	Revision             string                  `json:"revision"`
	Generation           int64                   `json:"generation"`
	CleanupBlocked       bool                    `json:"cleanupBlocked"`
	Hold                 bool                    `json:"hold"`
	AttestedAt           *time.Time              `json:"attestedAt"`
	AttestedUntil        time.Time               `json:"attestedUntil"`
	ReviewRef            string                  `json:"reviewRef"`
	ReturnReviewRef      string                  `json:"returnReviewRef"`
	PendingEvidenceUntil *time.Time              `json:"pendingEvidenceUntil"`
	GenerationPending    rentalGenerationPending `json:"generationPending"`
	Actions              []rentalAction          `json:"actions"`
}

type rentalGenerationPending struct {
	State           string     `json:"state"`
	PendingCount    *int       `json:"pendingCount,omitempty"`
	PendingPayments *int       `json:"pendingPayments,omitempty"`
	PendingLimits   *int       `json:"pendingLimits,omitempty"`
	ValidUntil      *time.Time `json:"validUntil,omitempty"`
}

type rentalReturnReview struct {
	Generation           string                  `json:"generation"`
	Account              string                  `json:"account"`
	Destination          string                  `json:"destination"`
	DestinationVerified  bool                    `json:"destinationVerified"`
	PendingState         string                  `json:"pendingState"`
	PendingEvidenceUntil *time.Time              `json:"pendingEvidenceUntil,omitempty"`
	ReviewRef            string                  `json:"reviewRef,omitempty"`
	GenerationPending    rentalGenerationPending `json:"generationPending"`
}

func (member RentalMember) returnReview(observation ops.RentalObservation, now time.Time) rentalReturnReview {
	verified := observation.Status != "wrong-destination" && observation.Status != "unopened" &&
		observation.Device == common.HexToAddress(member.Account) &&
		observation.Destination == common.HexToAddress(member.Destination)
	result := rentalReturnReview{
		Generation: strconv.FormatInt(member.Generation, 10), Account: member.Account, Destination: member.Destination,
		DestinationVerified: verified, PendingState: "reconciliation-required",
	}
	result.GenerationPending.State = "unknown"
	if verified {
		switch member.GenerationPending.State {
		case "frozen", "incomplete", "pending", "verified":
			result.GenerationPending = member.GenerationPending
		}
	}
	if !member.Hold && member.PendingEvidenceUntil != nil && member.PendingEvidenceUntil.After(now) {
		result.PendingState = "clearance-current"
		result.PendingEvidenceUntil = member.PendingEvidenceUntil
	}
	if verified && observation.Status == "active" && result.PendingState == "clearance-current" {
		result.ReviewRef = member.ReturnReviewRef
	}
	return result
}

type rentalAction struct {
	Action                     string          `json:"action"`
	Key                        string          `json:"requestKey"`
	Status                     string          `json:"status"`
	Evidence                   json.RawMessage `json:"evidence,omitempty"`
	Amount                     string          `json:"amount,omitempty"`
	ProtectedReviewRef         string          `json:"protectedReviewRef,omitempty"`
	ProtectedRecoveryReviewRef string          `json:"protectedRecoveryReviewRef,omitempty"`
}

func (member RentalMember) Opening() *ops.RentalOpening {
	for _, action := range member.Actions {
		if action.Action != "open" || action.Amount == "" {
			continue
		}
		var recorded struct {
			Submitted   common.Hash    `json:"submitted"`
			Device      common.Address `json:"device"`
			Destination common.Address `json:"destination"`
		}
		if json.Unmarshal(action.Evidence, &recorded) == nil && recorded.Submitted != (common.Hash{}) {
			return &ops.RentalOpening{
				RequestKey: action.Key, Amount: action.Amount, TxHash: recorded.Submitted,
				Device: recorded.Device, Destination: recorded.Destination,
			}
		}
	}
	return nil
}

type RentalControl interface {
	Observe(context.Context, RentalMember) (ops.RentalObservation, error)
	Submit(context.Context, RentalMember, string, string, *big.Int) (ops.RentalObservation, error)
}

func (h HTTP) registerRental(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/operations/rental/{branch}/{device}", h.operateRental)
	mux.HandleFunc("POST /v2/operations/rental/{branch}/{device}/{action}", h.operateRental)
}

func (h HTTP) operateRental(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.operatorCredential(r)
	branch, device := r.PathValue("branch"), r.PathValue("device")
	generation, parseErr := strconv.ParseInt(r.URL.Query().Get("generation"), 10, 64)
	if !ok || h.Rental == nil || h.Store == nil || !operatorRef.MatchString(branch) || !operatorRef.MatchString(device) ||
		parseErr != nil || generation <= 0 {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	sameCredential := func() bool {
		current, valid := h.operatorCredential(r)
		return valid && current.Tenant == actor.Tenant && current.Subject == actor.Subject
	}
	protectedRequest := protectedCloseRequest{Tenant: actor.Tenant, Branch: branch, Actor: actor.Subject, Device: device, Generation: generation}
	action := r.PathValue("action")
	var command struct {
		Key            string  `json:"requestKey"`
		Amount         string  `json:"amount,omitempty"`
		Review         *string `json:"reviewRef,omitempty"`
		RecoveryReview string  `json:"recoveryReviewRef,omitempty"`
	}
	decodeCommand := func() bool {
		if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || r.Header.Get("Content-Type") != "application/json" {
			http.NotFound(w, r)
			return false
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&command) != nil || decoder.Decode(new(any)) != io.EOF || !operatorRef.MatchString(command.Key) {
			operatorResponse(w, nil, service.BadRequest)
			return false
		}
		return true
	}
	if action == "cleanup" && r.Method == http.MethodPost {
		if !decodeCommand() {
			return
		}
		if command.Amount != "" || command.Review != nil || command.RecoveryReview != "" {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		if !sameCredential() {
			http.NotFound(w, r)
			return
		}
		protectedRequest.Key = command.Key
		h.executeRentalCleanup(w, r, actor, protectedRequest, sameCredential)
		return
	}
	if h.ProtectedClose != nil && action == "close" && r.Method == http.MethodPost {
		scope, err := h.ProtectedClose.snapshot(ctx, protectedRequest)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		if scope.Protected {
			if !decodeCommand() {
				return
			}
			if command.Amount != "" || (command.Review != nil && !rentalReviewRef.MatchString(*command.Review)) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			protectedRequest.Key = command.Key
			if command.Review != nil {
				protectedRequest.Review = *command.Review
			}
			if command.RecoveryReview != "" && !rentalReviewRef.MatchString(command.RecoveryReview) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			protectedRequest.RecoveryReview = command.RecoveryReview
			h.executeProtectedRentalClose(w, r, actor, protectedRequest, sameCredential)
			return
		}
	}
	if h.ProtectedWithdrawal && h.ProtectedClose != nil && action == "withdraw" && r.Method == http.MethodPost {
		scope, err := h.ProtectedClose.withdrawalSnapshot(ctx, protectedRequest)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		if scope.Protected {
			if !decodeCommand() {
				return
			}
			if command.Amount != "" || (command.RecoveryReview != "" && !rentalReviewRef.MatchString(command.RecoveryReview)) || (command.Review != nil && !rentalReviewRef.MatchString(*command.Review)) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			protectedRequest.Key = command.Key
			if command.Review != nil {
				protectedRequest.Review = *command.Review
			}
			protectedRequest.RecoveryReview = command.RecoveryReview
			h.executeProtectedWithdrawal(w, r, actor, protectedRequest, sameCredential)
			return
		}
	}
	cleanupInstalled, err := h.rentalCleanupInstalled(ctx)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	readMember := func(includePending bool) (RentalMember, error) {
		member := RentalMember{CleanupBlocked: !cleanupInstalled && generation > 1}
		var raw []byte
		query := "SELECT ledger.rental_member($1,$2,$3,$4,$5)"
		if includePending {
			query += " || jsonb_build_object('generationPending',ledger.rental_generation_pending($1,$2,$3,$4,$5))"
		}
		query += " || ledger.rental_generation_authorized_review($1,$2,$3,$4,$5)"
		if cleanupInstalled {
			query += " || jsonb_build_object('cleanupBlocked',ledger.rental_successor_cleanup_blocked($1,$2,$3,$4,$5))"
		}
		query += ` || CASE WHEN ledger.rental_runtime_close_hold($1,$2,$3,$4,$5)
			THEN '{"hold":true,"returnReviewRef":null,"pendingEvidenceUntil":null,"generationPending":{"state":"unknown"}}'::jsonb
			ELSE '{}'::jsonb END`
		err := h.Store.db.QueryRow(ctx, query,
			actor.Tenant, branch, actor.Subject, device, generation).Scan(&raw)
		if err == nil && json.Unmarshal(raw, &member) != nil {
			err = service.Unknown
		}
		return member, err
	}
	member, err := readMember(false)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	var cleanup *rentalCleanupView
	var withdrawal *protectedWithdrawalView
	if h.ProtectedWithdrawal && h.ProtectedClose != nil && r.Method == http.MethodGet {
		withdrawal, err = h.ProtectedClose.withdrawalView(ctx, protectedRequest)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
	}
	if cleanupInstalled && withdrawal != nil {
		pending, err := h.ProtectedClose.pendingCleanup(ctx, protectedRequest)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		if pending.State == "settlement-proof-required" && (withdrawal.State == "withdrawal-received" || withdrawal.State == "withdrawal-no-payout") {
			pending.State = "request-available"
		}
		cleanup = &pending
	}
	observation := ops.RentalObservation{Status: "payment-hold", Device: common.HexToAddress(member.Account),
		Destination: common.HexToAddress(member.Destination)}
	if withdrawal == nil {
		observation, err = h.Rental.Observe(ctx, member)
		if err != nil {
			operatorResponse(w, nil, service.Unknown)
			return
		}
	}
	response := func(status int) {
		reply := map[string]any{
			"observation": observation, "operations": member.Actions, "revision": member.Revision,
			"attestedAt": member.AttestedAt, "attestedUntil": member.AttestedUntil, "reviewRef": member.ReviewRef,
			"returnReview": member.returnReview(observation, time.Now()), "cleanupBlocked": member.CleanupBlocked,
		}
		if withdrawal != nil {
			reply["withdrawal"] = withdrawal
		}
		if cleanup != nil {
			reply["cleanup"] = cleanup
		}
		writeJSON(w, status, reply)
	}
	if r.Method == http.MethodGet {
		// The finalized chain observation can block. Re-read the grant and the
		// bound enrollment before returning evidence from that snapshot.
		current, err := readMember(true)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		if current.Account != member.Account || current.Destination != member.Destination || current.ReviewRef != member.ReviewRef || current.ReturnReviewRef != member.ReturnReviewRef || current.Hold != member.Hold {
			operatorResponse(w, nil, service.Conflict)
			return
		}
		if h.ProtectedClose != nil {
			current, observation, err = h.protectedRentalReview(ctx, protectedRequest, current, observation)
			if err != nil {
				operatorResponse(w, nil, err)
				return
			}
		}
		if !sameCredential() {
			http.NotFound(w, r)
			return
		}
		member = current
		response(http.StatusOK)
		return
	}
	if !decodeCommand() {
		return
	}
	if command.RecoveryReview != "" {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	required := map[string]string{"open": "unopened", "close": "active", "withdraw": "withdrawal-ready"}[action]
	if required == "" {
		http.NotFound(w, r)
		return
	}
	var amount *big.Int
	var amountWire *string
	if command.Review != nil && ((action != "open" && action != "close") || !rentalReviewRef.MatchString(*command.Review)) {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	if action == "open" {
		if !catalogUnits.MatchString(command.Amount) {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		amount, _ = new(big.Int).SetString(command.Amount, 10)
		if amount.BitLen() > 128 {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		amountWire = &command.Amount
	} else if command.Amount != "" {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	known := false
	for _, prior := range member.Actions {
		known = known || (prior.Action == action && prior.Key == command.Key)
	}
	review := member.ReviewRef
	if action == "close" {
		review = member.returnReview(observation, time.Now()).ReviewRef
	}
	if !known && ((action == "open" && member.CleanupBlocked) || observation.Status != required ||
		(action == "close" && command.Review == nil) || (command.Review != nil && *command.Review != review)) {
		operatorResponse(w, nil, service.Conflict)
		return
	}
	if !sameCredential() {
		http.NotFound(w, r)
		return
	}
	var dispatch bool
	var actions interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	} = h.Store.db
	historicalOpen := action == "open" && member.CleanupBlocked && !cleanupInstalled
	if historicalOpen {
		// An old schema can authenticate a known key, but cannot authorize a
		// new successor if the historical row disappears before the decision.
		tx, err := h.Store.db.Begin(ctx)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		defer tx.Rollback(context.Background())
		actions = tx
	}
	if err := actions.QueryRow(ctx, "SELECT ledger.begin_rental_action($1,$2,$3,$4,$5,$6,$7,$8::text::numeric,$9)",
		actor.Tenant, branch, actor.Subject, device, generation, action, command.Key, amountWire, command.Review).Scan(&dispatch); err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if !dispatch {
		response(http.StatusAccepted)
		return
	}
	if historicalOpen {
		operatorResponse(w, nil, service.Conflict)
		return
	}
	received, _ := h.Rental.Submit(ctx, member, action, command.Key, amount)
	status := "unknown"
	if received.Submitted != nil && *received.Submitted != (common.Hash{}) {
		status = "submitted"
	}
	raw, _ := json.Marshal(received)
	if _, err := h.Store.db.Exec(ctx, "SELECT ledger.record_rental_action($1,$2,$3,$4,$5,$6,$7,$8,$9)",
		actor.Tenant, branch, actor.Subject, device, generation, action, command.Key, status, raw); err == nil {
		observation = received
		member.Actions = append(member.Actions, rentalAction{Action: action, Key: command.Key, Status: status, Evidence: raw, Amount: command.Amount})
	}
	response(http.StatusAccepted)
}
