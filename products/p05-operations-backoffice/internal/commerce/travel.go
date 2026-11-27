package commerce

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

var travelUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var travelBalance = regexp.MustCompile(`^(0|[1-9][0-9]{0,77})$`)
var travelAmount = regexp.MustCompile(`^[1-9][0-9]{0,77}$`)
var travelAsset = regexp.MustCompile(`^0x[0-9a-f]{40}$`)

type TravelRequest struct {
	Operation   string `json:"operation"`
	Branch      string `json:"branch"`
	Slot        string `json:"slot,omitempty"`
	Booking     string `json:"booking,omitempty"`
	Key         string `json:"requestKey,omitempty"`
	Party       int    `json:"partySize,omitempty"`
	Terms       int64  `json:"termsVersion,omitempty"`
	Asset       string `json:"asset,omitempty"`
	Deposit     string `json:"deposit,omitempty"`
	Balance     string `json:"balance,omitempty"`
	Sale        string `json:"sale,omitempty"`
	Expected    *int64 `json:"expectedRevision,omitempty"`
	Target      string `json:"target,omitempty"`
	IncludeCafe bool   `json:"includeCafe,omitempty"`
}

type TravelPickupWindow struct {
	ID       string    `json:"id"`
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
	Timezone string    `json:"timezone"`
}

type TravelTerms struct {
	Terms        int64     `json:"termsVersion"`
	Cancellation string    `json:"cancellation"`
	Asset        string    `json:"asset"`
	Deposit      string    `json:"deposit"`
	Balance      string    `json:"balance"`
	PriceVersion *int64    `json:"priceVersion,omitempty"`
	NoShow       *string   `json:"noShow,omitempty"`
	ChainID      *int32    `json:"chainId,omitempty"`
	Cutoff       time.Time `json:"cancellationDeadline"`
}

type TravelSlot struct {
	ID     string    `json:"id"`
	Starts time.Time `json:"startsAt"`
	TravelTerms
	Remaining int `json:"remaining"`
}

type TravelLeg struct {
	Kind           string  `json:"kind"`
	Sale           string  `json:"saleRef"`
	State          string  `json:"state"`
	Amount         string  `json:"amount"`
	Receipt        *string `json:"receiptRef,omitempty"`
	Refunded       string  `json:"refunded"`
	RefundExpected *string `json:"refundExpected,omitempty"`
	RefundState    string  `json:"refundState"`
}

type TravelCancellationPreview struct {
	CanCancel bool      `json:"canCancel"`
	Deadline  time.Time `json:"cancellationDeadline"`
	Legs      []struct {
		Kind           string  `json:"kind"`
		Sale           string  `json:"saleRef"`
		PaymentState   string  `json:"paymentState"`
		RefundExpected *string `json:"refundExpected,omitempty"`
	} `json:"legs"`
}

type TravelBooking struct {
	ID      string    `json:"id"`
	Slot    string    `json:"slot"`
	Party   int       `json:"partySize"`
	Expires time.Time `json:"expiresAt"`
	TravelTerms
	Starts *time.Time `json:"startsAt,omitempty"`
	State  string     `json:"state"`
}

func pickupTerminalOperation(operation string) bool {
	return operation == "pickup-queue" || operation == "pickup-detail" || operation == "pickup-change"
}

func (req TravelRequest) valid() bool {
	if !travelUUID.MatchString(req.Branch) || (req.IncludeCafe && req.Operation != "pickup-queue" && req.Operation != "pickup-detail") {
		return false
	}
	unset := req.Party == 0 && req.Terms == 0 && req.Asset == "" && req.Deposit == "" && req.Balance == ""
	if pickupTerminalOperation(req.Operation) || req.Operation == "pickup-orders" || req.Operation == "pickup-owned" {
		if !unset || req.Slot != "" || req.Booking != "" {
			return false
		}
		if req.Operation == "pickup-change" {
			return travelUUID.MatchString(req.Sale) && travelUUID.MatchString(req.Key) && req.Expected != nil &&
				*req.Expected >= 0 && *req.Expected < 9007199254740991 &&
				(req.Target == "preparing" || req.Target == "ready" || req.Target == "collected" || req.Target == "unfulfillable")
		}
		if req.Key != "" || req.Expected != nil || req.Target != "" {
			return false
		}
		if req.Operation == "pickup-queue" || req.Operation == "pickup-orders" {
			return req.Sale == ""
		}
		return travelUUID.MatchString(req.Sale)
	}
	if req.Sale != "" || req.Expected != nil || req.Target != "" {
		return false
	}
	switch req.Operation {
	case "options", "pickup-options":
		return unset && req.Slot == "" && req.Booking == "" && req.Key == ""
	case "hold":
		return req.Booking == "" && travelUUID.MatchString(req.Slot) && travelUUID.MatchString(req.Key) &&
			req.Party >= 1 && req.Party <= 20 && req.Terms > 0 && travelAsset.MatchString(req.Asset) && travelAmount.MatchString(req.Deposit) &&
			travelBalance.MatchString(req.Balance)
	case "reopen":
		return unset && req.Slot == "" && ((travelUUID.MatchString(req.Booking) && req.Key == "") ||
			(req.Booking == "" && travelUUID.MatchString(req.Key)))
	case "start-deposit", "start-balance", "legs", "cancel-preview", "cancel":
		return unset && req.Slot == "" && req.Key == "" && travelUUID.MatchString(req.Booking)
	case "my-bookings":
		return unset && req.Slot == "" && req.Booking == "" && req.Key == ""
	default:
		return false
	}
}

func travelFailure(w http.ResponseWriter, status int, label string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": label})
}

func failTravelStore(w http.ResponseWriter, err error) {
	if errors.Is(err, service.Forbidden) {
		travelFailure(w, http.StatusNotFound, "booking_unavailable")
		return
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "42501", "P0002":
			travelFailure(w, http.StatusNotFound, "booking_unavailable")
			return
		case "P0001", "P0501":
			travelFailure(w, http.StatusConflict, "request_conflict")
			return
		case "P0003":
			travelFailure(w, http.StatusConflict, "terms_changed")
			return
		case "P0004":
			travelFailure(w, http.StatusConflict, "sold_out")
			return
		}
	}
	travelFailure(w, http.StatusServiceUnavailable, "booking_unavailable")
}

func (h HTTP) travel(w http.ResponseWriter, r *http.Request, principal service.Principal, body json.RawMessage) {
	if (principal.Role != "customer" && principal.Role != "terminal" && principal.Role != "operator") || principal.ExpiresAt <= time.Now().Unix() ||
		!travelUUID.MatchString(principal.Tenant) || !travelUUID.MatchString(principal.Subject) {
		http.NotFound(w, r)
		return
	}
	var req TravelRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || !req.valid() {
		travelFailure(w, http.StatusBadRequest, "invalid_booking")
		return
	}
	if _, err := decoder.Token(); err != io.EOF {
		travelFailure(w, http.StatusBadRequest, "invalid_booking")
		return
	}
	if pickupTerminalOperation(req.Operation) != (principal.Role == "terminal" || principal.Role == "operator") ||
		(req.IncludeCafe && principal.Role != "operator") {
		travelFailure(w, http.StatusNotFound, "booking_unavailable")
		return
	}
	// Only a hold carries amounts. At a won branch they are whole won, as menu prices are.
	if token := h.Profiles[req.Branch].Token; req.Operation == "hold" && !(token.wholePrice(req.Deposit) && token.wholePrice(req.Balance)) {
		travelFailure(w, http.StatusBadRequest, "invalid_booking")
		return
	}

	if h.Store == nil {
		travelFailure(w, http.StatusServiceUnavailable, "booking_unavailable")
		return
	}

	ctx := authority.Scope(r.Context(), principal, req.Branch)
	switch principal.Role {
	case "operator":
		ctx = authority.Scope(ctx, principal, req.Branch, "pickup_manage")
	case "terminal":
		ctx = authority.Scope(ctx, principal, req.Branch, "pickup_assignment")
	}
	var result any
	switch req.Operation {
	case "pickup-change":
		var revision int64
		err := h.Store.db.QueryRow(ctx, `SELECT ledger.kiosk_pickup_change($1,$2,$3,$4,$5,$6,$7,$8)`,
			principal.Tenant, req.Branch, principal.Subject, req.Sale, req.Key, req.Expected, req.Target, principal.Role).Scan(&revision)
		if err != nil {
			failTravelStore(w, err)
			return
		}
		result = map[string]int64{"revision": revision}
	case "pickup-queue", "pickup-detail", "pickup-orders", "pickup-owned":
		query := `SELECT * FROM ledger.travel_pickup_customer($1,$2,$3,$4)`
		var sale *string
		if req.Sale != "" {
			sale = &req.Sale
		}
		args := []any{principal.Tenant, req.Branch, principal.Subject, sale}
		if pickupTerminalOperation(req.Operation) {
			query = `SELECT * FROM ledger.kiosk_pickup_queue($1,$2,$3,$4,$5)`
			args = append(args, principal.Role)
			if req.IncludeCafe {
				query = `SELECT * FROM ledger.kiosk_pickup_queue($1,$2,$3,$4,$5,$6)`
				args = append(args, true)
			}
		}
		rows, err := h.Store.db.Query(ctx, query, args...)
		if err != nil {
			failTravelStore(w, err)
			return
		}
		items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (json.RawMessage, error) {
			var item json.RawMessage
			err := row.Scan(&item)
			return item, err
		})
		if err != nil {
			failTravelStore(w, err)
			return
		}
		if sale != nil && len(items) != 1 {
			travelFailure(w, http.StatusNotFound, "booking_unavailable")
			return
		}
		result = map[string][]json.RawMessage{"pickups": items}

	case "pickup-options":
		rows, err := h.Store.db.Query(ctx, `SELECT id::text,starts_at,ends_at,timezone
			FROM ledger.travel_pickup_options($1,$2,$3)`, principal.Tenant, req.Branch, principal.Subject)
		if err != nil {
			failTravelStore(w, err)
			return
		}
		windows, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TravelPickupWindow, error) {
			var window TravelPickupWindow
			err := row.Scan(&window.ID, &window.StartsAt, &window.EndsAt, &window.Timezone)
			window.StartsAt, window.EndsAt = window.StartsAt.UTC(), window.EndsAt.UTC()
			return window, err
		})
		if err != nil {
			failTravelStore(w, err)
			return
		}
		result = map[string][]TravelPickupWindow{"pickupWindows": windows}

	case "start-deposit", "start-balance":
		var sale string
		err := h.Store.db.QueryRow(ctx, `SELECT ledger.travel_start_leg($1,$2,$3,$4,$5)::text`,
			principal.Tenant, req.Branch, principal.Subject, req.Booking, req.Operation[6:]).Scan(&sale)
		if err != nil {
			failTravelStore(w, err)
			return
		}
		result = map[string]string{"saleRef": sale}
	case "cancel-preview":
		var preview TravelCancellationPreview
		if err := h.Store.db.QueryRow(ctx, `SELECT ledger.travel_cancellation_preview($1,$2,$3,$4)`,
			principal.Tenant, req.Branch, principal.Subject, req.Booking).Scan(&preview); err != nil {
			failTravelStore(w, err)
			return
		}
		result = map[string]TravelCancellationPreview{"cancellationPreview": preview}
	case "cancel":
		var booking string
		err := h.Store.db.QueryRow(ctx, `SELECT ledger.travel_cancel($1,$2,$3,$4)::text`,
			principal.Tenant, req.Branch, principal.Subject, req.Booking).Scan(&booking)
		if err != nil {
			failTravelStore(w, err)
			return
		}
		result = map[string]string{"bookingRef": booking}
	case "legs":
		if _, err := h.Store.db.Exec(ctx, `SELECT ledger.travel_reconcile_owned($1,$2,$3,$4)`,
			principal.Tenant, req.Branch, principal.Subject, req.Booking); err != nil {
			failTravelStore(w, err)
			return
		}
		rows, err := h.Store.db.Query(ctx, `SELECT kind,sale_id::text,state,amount,receipt_ref::text,refunded,refund_expected,refund_state FROM ledger.travel_legs_with_refunds($1,$2,$3,$4)`,
			principal.Tenant, req.Branch, principal.Subject, req.Booking)
		if err != nil {
			failTravelStore(w, err)
			return
		}
		legs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TravelLeg, error) {
			var leg TravelLeg
			err := row.Scan(&leg.Kind, &leg.Sale, &leg.State, &leg.Amount, &leg.Receipt, &leg.Refunded, &leg.RefundExpected, &leg.RefundState)
			return leg, err
		})
		if err != nil {
			failTravelStore(w, err)
			return
		}
		result = map[string][]TravelLeg{"legs": legs}
	case "options":
		rows, err := h.Store.db.Query(ctx, `SELECT id::text, starts_at, terms_version, cancellation_terms, asset_ref, deposit, balance, remaining,
            price_version, no_show_terms, chain_id, cancellation_deadline
			 FROM ledger.travel_review_options($1,$2,$3)`, principal.Tenant, req.Branch, principal.Subject)
		if err != nil {
			failTravelStore(w, err)
			return
		}
		slots, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TravelSlot, error) {
			var slot TravelSlot
			err := row.Scan(&slot.ID, &slot.Starts, &slot.Terms, &slot.Cancellation, &slot.Asset, &slot.Deposit, &slot.Balance, &slot.Remaining,
				&slot.PriceVersion, &slot.NoShow, &slot.ChainID, &slot.Cutoff)
			return slot, err
		})
		if err != nil {
			failTravelStore(w, err)
			return
		}
		if slots == nil {
			slots = []TravelSlot{}
		}
		result = map[string]any{"slots": slots}
	default:
		booking, key := req.Booking, req.Key
		if req.Operation == "hold" {
			err := h.Store.db.QueryRow(ctx, `SELECT ledger.travel_hold($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)::text`,
				principal.Tenant, req.Branch, principal.Subject, key, req.Slot, req.Party, req.Terms, req.Deposit, req.Balance, req.Asset).Scan(&booking)
			if err != nil {
				failTravelStore(w, err)
				return
			}
			key = ""
		}
		rows, err := h.Store.db.Query(ctx, `SELECT id::text, slot_id::text, party_size, expires_at, terms_version, cancellation_terms, asset_ref,
			deposit, balance,state, price_version, no_show_terms, chain_id, cancellation_deadline, starts_at
        FROM ledger.travel_review_owned($1,$2,$3,NULLIF($4::text,'')::uuid,NULLIF($5::text,'')::uuid)`,
			principal.Tenant, req.Branch, principal.Subject, booking, key)
		if err != nil {
			failTravelStore(w, err)
			return
		}
		bookings, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TravelBooking, error) {
			var item TravelBooking
			err := row.Scan(&item.ID, &item.Slot, &item.Party, &item.Expires, &item.Terms, &item.Cancellation, &item.Asset, &item.Deposit, &item.Balance, &item.State,
				&item.PriceVersion, &item.NoShow, &item.ChainID, &item.Cutoff, &item.Starts)
			return item, err
		})
		if err != nil {
			failTravelStore(w, err)
			return
		}
		if req.Operation == "my-bookings" {
			if bookings == nil {
				bookings = []TravelBooking{}
			}
			result = map[string]any{"bookings": bookings}
		} else {
			if len(bookings) != 1 {
				travelFailure(w, http.StatusNotFound, "booking_unavailable")
				return
			}
			result = map[string]any{"booking": bookings[0]}
		}
	}
	if err := h.Store.db.CheckRead(ctx); err != nil {
		if service.CodeOf(err) == service.Forbidden {
			http.NotFound(w, r)
			return
		}
		failTravelStore(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(result)
}
