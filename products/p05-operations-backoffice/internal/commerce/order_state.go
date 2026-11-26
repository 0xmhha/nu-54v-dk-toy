package commerce

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
)

var (
	wallClock = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	calendar  = regexp.MustCompile(`^20[0-9]{2}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)
)

// orderStateChange is one change from the operations page: the store's state, one closed day, or products sold out.
type orderStateChange struct {
	Mode        string   `json:"mode,omitempty"`
	BusyMinutes *int     `json:"busyMinutes,omitempty"`
	ResumeAt    *string  `json:"resumeAt,omitempty"`
	Day         string   `json:"day,omitempty"`
	Closed      *bool    `json:"closed,omitempty"`
	Products    []string `json:"products,omitempty"`
	SoldOut     *bool    `json:"soldOut,omitempty"`
	// The tax-free shop designation; an empty Designation removes it.
	Designation    *string `json:"designation,omitempty"`
	DesignatedOn   string  `json:"designatedOn,omitempty"`
	RefundOperator string  `json:"refundOperator,omitempty"`
	Confirmed      bool    `json:"confirmed,omitempty"`
	// Cancelling one order that cannot be prepared: the order, the stage seen, and a key for repeating the same request.
	Sale     string `json:"sale,omitempty"`
	Revision *int64 `json:"revision,omitempty"`
	Key      string `json:"key,omitempty"`
}

// CustomerOrderState is what customers and terminals read with the first menu page.
type CustomerOrderState struct {
	Mode        string `json:"mode"`
	BusyMinutes int    `json:"busyMinutes,omitempty"`
	ResumeAt    string `json:"resumeAt,omitempty"`
	// AutoKitchen says a test store's automatic kitchen records the stages, not a person.
	AutoKitchen bool `json:"autoKitchen,omitempty"`
	// TaxFreeShop is the designation number of a tax-free shop for foreign visitors, as the store entered it.
	TaxFreeShop string `json:"taxFreeShop,omitempty"`
	// SoldOut is applied to the menu rows (their counts read as zero), not sent, so a page keeps its size.
	SoldOut []string `json:"-"`
}

func (h HTTP) registerOrderState(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/operations/state/{branch}", h.operateOrderState)
	mux.HandleFunc("POST /v2/operations/state/{branch}/{action}", h.operateOrderState)
	mux.HandleFunc("GET /v2/operations/state/{branch}/order", h.findOrder)
}

// operateOrderState reads or changes whether the store takes orders, its closed days and sold-out products.
func (h HTTP) operateOrderState(w http.ResponseWriter, r *http.Request) {
	branch := r.PathValue("branch")
	actor, ok := h.operatorCredential(r)
	if !operatorRef.MatchString(branch) || !ok || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	ctx := authority.Scope(r.Context(), actor, branch)
	var reply json.RawMessage
	if r.Method == http.MethodGet {
		err := h.Store.db.QueryRow(ctx, "SELECT ledger.operator_order_state($1,$2,$3)", actor.Tenant, branch, actor.Subject).Scan(&reply)
		operatorResponse(w, h.withMe(reply, actor.Subject), err)
		return
	}
	if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || strings.TrimSpace(r.Header.Get("Content-Type")) != "application/json" {
		http.NotFound(w, r)
		return
	}
	var change orderStateChange
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&change) != nil {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	var err error
	switch r.PathValue("action") {
	case "mode":
		if change.Mode == "" || (change.ResumeAt != nil && !wallClock.MatchString(*change.ResumeAt)) {
			err = service.BadRequest
			break
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.set_order_state($1,$2,$3,$4,$5,$6)",
			actor.Tenant, branch, actor.Subject, change.Mode, change.BusyMinutes, change.ResumeAt).Scan(&reply)
	case "closed-day":
		if !calendar.MatchString(change.Day) || change.Closed == nil {
			err = service.BadRequest
			break
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.set_closed_day($1,$2,$3,$4::date,$5)",
			actor.Tenant, branch, actor.Subject, change.Day, *change.Closed).Scan(&reply)
	case "sold-out":
		if len(change.Products) == 0 || len(change.Products) > 200 || change.SoldOut == nil {
			err = service.BadRequest
			break
		}
		products, _ := json.Marshal(change.Products)
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.set_sold_out($1,$2,$3,$4,$5)",
			actor.Tenant, branch, actor.Subject, products, *change.SoldOut).Scan(&reply)
	case "cancel":
		// Only someone with the kitchen-manage grant; the change records who it was.
		if !operatorRef.MatchString(change.Sale) || !operatorRef.MatchString(change.Key) || change.Revision == nil {
			err = service.BadRequest
			break
		}
		var revision int64
		manage := authority.Scope(ctx, actor, branch, "pickup_manage")
		if err = h.Store.db.QueryRow(manage, "SELECT ledger.kiosk_pickup_change($1,$2,$3,$4,$5,$6,'unfulfillable','operator')",
			actor.Tenant, branch, actor.Subject, change.Sale, change.Key, *change.Revision).Scan(&revision); err == nil {
			reply, _ = json.Marshal(map[string]int64{"revision": revision})
		}
		if err == nil {
			operatorResponse(w, reply, nil)
			return
		}
	case "tax-free":
		switch {
		case change.Designation == nil:
			err = service.BadRequest
		case *change.Designation == "":
			err = h.Store.db.QueryRow(ctx, "SELECT ledger.set_tax_free_shop($1,$2,$3,NULL,NULL,NULL)", actor.Tenant, branch, actor.Subject).Scan(&reply)
		case !change.Confirmed || !calendar.MatchString(change.DesignatedOn) || strings.TrimSpace(change.RefundOperator) == "":
			err = service.BadRequest // the operator confirms the designation they read
		default:
			err = h.Store.db.QueryRow(ctx, "SELECT ledger.set_tax_free_shop($1,$2,$3,$4,$5::date,$6)",
				actor.Tenant, branch, actor.Subject, *change.Designation, change.DesignatedOn, change.RefundOperator).Scan(&reply)
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	operatorResponse(w, h.withMe(reply, actor.Subject), nil)
}

// withMe adds who is asking, so the page can say "you" instead of a reference.
func (h HTTP) withMe(reply json.RawMessage, me string) json.RawMessage {
	var state map[string]json.RawMessage
	if reply == nil || json.Unmarshal(reply, &state) != nil {
		return reply
	}
	state["me"], _ = json.Marshal(me)
	out, _ := json.Marshal(state)
	return out
}

// OrderState reads the store's state for a customer or terminal.
func (s *Store) OrderState(ctx context.Context, auth service.Bound) (*CustomerOrderState, error) {
	var state CustomerOrderState
	var raw []byte
	if err := s.db.QueryRow(ctx, "SELECT ledger.customer_order_state($1,$2,$3)", auth.Tenant, auth.Request.Domain, auth.Subject).Scan(&raw); err != nil {
		return nil, domainError(err)
	}
	var wire struct {
		CustomerOrderState
		SoldOut []string `json:"soldOut"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, err
	}
	state = wire.CustomerOrderState
	state.SoldOut = wire.SoldOut
	return &state, nil
}

// markSoldOut shows the products a store marked sold out as having none left.
func markSoldOut(rows []Price, soldOut []string) {
	for index := range rows {
		if slices.Contains(soldOut, rows[index].SKU) {
			rows[index].Available, rows[index].SKUAvailable = 0, 0
		}
	}
}

// ReadyBoard is today's pickup numbers by stage, for the board on the store's tablet. Nothing else about an order.
type ReadyBoard struct {
	CheckedAt   string  `json:"checkedAt"`
	AutoKitchen bool    `json:"autoKitchen,omitempty"`
	Ready       []int64 `json:"ready"`
	Making      []int64 `json:"making"`
	Received    []int64 `json:"received"`
}

// ReadyBoard reads the board for the store's own tablet; any other credential is refused.
func (s *Store) ReadyBoard(ctx context.Context, auth service.Bound) (*ReadyBoard, error) {
	var raw []byte
	if err := s.db.QueryRow(ctx, "SELECT ledger.ready_board($1,$2,$3)", auth.Tenant, auth.Request.Domain, auth.Subject).Scan(&raw); err != nil {
		return nil, domainError(err)
	}
	var board ReadyBoard
	if err := json.Unmarshal(raw, &board); err != nil {
		return nil, err
	}
	return &board, nil
}

// findOrder finds one of today's orders by its pickup number, for the person who may cancel it.
func (h HTTP) findOrder(w http.ResponseWriter, r *http.Request) {
	branch := r.PathValue("branch")
	actor, ok := h.operatorCredential(r)
	number, err := strconv.ParseInt(r.URL.Query().Get("number"), 10, 64)
	if !operatorRef.MatchString(branch) || !ok || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	if err != nil || number < 1 || number > 99999 {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	ctx := authority.Scope(authority.Scope(r.Context(), actor, branch), actor, branch, "pickup_manage")
	var reply json.RawMessage
	if err := h.Store.db.QueryRow(ctx, "SELECT ledger.operator_find_order($1,$2,$3,$4)", actor.Tenant, branch, actor.Subject, number).Scan(&reply); err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if reply == nil || string(reply) == "null" {
		http.NotFound(w, r)
		return
	}
	operatorResponse(w, reply, nil)
}

// StepAutoKitchen moves the due orders of every test store whose kitchen is on, but only on a chain testChain accepts.
func (s *Store) StepAutoKitchen(ctx context.Context, testChain func(branch string) bool) (int, error) {
	rows, err := s.db.Query(ctx, "SELECT branch::text FROM ledger.auto_kitchen_branches() branch")
	if err != nil {
		return 0, err
	}
	var branches []string
	for rows.Next() {
		var branch string
		if err := rows.Scan(&branch); err != nil {
			rows.Close()
			return 0, err
		}
		branches = append(branches, branch)
	}
	rows.Close()
	moved := 0
	for _, branch := range branches {
		if !testChain(branch) {
			continue // a store marked by mistake on another chain is never run
		}
		var n int
		if err := s.db.QueryRow(ctx, "SELECT ledger.auto_kitchen_step($1)", branch).Scan(&n); err != nil {
			return moved, err
		}
		moved += n
	}
	return moved, nil
}
