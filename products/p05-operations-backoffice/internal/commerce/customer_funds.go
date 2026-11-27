package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// The balance check before a hold (owner decision). Proving an account costs nothing, so a cart of a proven account
// holds stock only when that account can pay it as the chain stands now: after the account proof and immediately
// before the hold, opsd reads the account's balance of the asset the branch is paid in and compares it with the
// cart's amount. A balance that cannot be read refuses the cart too (fail closed) and asks the app to try again
// shortly. It is a check before the hold, not a reservation of the money: the payment itself still decides.
//
// Store-issued customers (a certificate, no account proof) are not checked here.

// PayerFunds reads from the chain what an account holds of the asset a branch is paid in.
type PayerFunds interface {
	// PayerBalance answers the branch's settlement asset and the account's balance of it in base units, as the
	// chain has it now. A branch that has no asset to be paid in answers service.Capability.
	PayerBalance(ctx context.Context, branch string, payer common.Address) (asset common.Address, balance *big.Int, err error)
}

const (
	balanceReadTimeout = 3 * time.Second
	// balanceRetry is the Retry-After of a cart refused because the balance could not be read.
	balanceRetry = 3 * time.Second
)

// holdRefusal is why the cart of a proven account held no stock. The reply names the reason for the app:
//
//	balance_short        409 CHECKOUT_CONFLICT, with the cart's amount (required) and the account's balance, both in
//	                     base units of the asset
//	balance_unavailable  503 BUSY with Retry-After: the chain could not be read now, or too many balance reads are
//	                     waiting for it; nothing is said about the balance
//	hold_limit           429 BUSY with Retry-After: the account asked for too many carts in a short time, or the
//	                     accounts of its address came to hold too many
//	orders_closed        409 CHECKOUT_CONFLICT: the store has no asset a proven account's balance could be read in
//	                     (no settlement target is enrolled for it), so no cart of one is held until the operator
//	                     changes that; asking again does not help
//	cart_released        409 CHECKOUT_CONFLICT: the cart this request key made earlier is closed without a payment
//	                     (its customer took it back, or its hold ran out); nothing is held, and a new cart needs a
//	                     new request key
type holdRefusal struct {
	status   int
	code     service.Code
	reason   string
	retry    time.Duration
	required string
	balance  string
}

func (e *holdRefusal) Error() string { return "cart not held: " + e.reason }

// write answers the refusal in the envelope of the request it refuses.
func (e *holdRefusal) write(w http.ResponseWriter, bound service.Bound) {
	reply := service.Outcome(bound, "error")
	reply.Error = e.code
	if e.retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int((e.retry+time.Second-1)/time.Second))))
	}
	writeJSON(w, e.status, struct {
		service.Reply
		Reason   string `json:"reason"`
		Required string `json:"required,omitempty"`
		Balance  string `json:"balance,omitempty"`
	}{reply, e.reason, e.required, e.balance})
}

// checkFunds refuses a cart its account cannot pay now. asset is the asset the cart is priced in, as the request
// names it (lower-case hex); required is the cart's amount in its base units.
func checkFunds(ctx context.Context, funds PayerFunds, branch string, payer common.Address, asset string, required *big.Int) error {
	// No chain reader at all, or none for this store: that does not pass by waiting, so it is not said as if it did.
	closed := &holdRefusal{status: http.StatusConflict, code: service.CheckoutConflict, reason: "orders_closed"}
	unread := &holdRefusal{status: http.StatusServiceUnavailable, code: service.Busy, reason: "balance_unavailable", retry: balanceRetry}
	if funds == nil {
		return closed // no chain to ask: the cart is not held on trust
	}
	read, cancel := context.WithTimeout(ctx, balanceReadTimeout)
	defer cancel()
	settled, balance, err := funds.PayerBalance(read, branch, payer)
	if errors.Is(err, service.Capability) {
		return closed
	}
	if err != nil || balance == nil || balance.Sign() < 0 {
		return unread
	}
	if strings.ToLower(settled.Hex()) != asset {
		return service.Mismatch // the cart is priced in another asset than the branch is paid in
	}
	if balance.Cmp(required) < 0 {
		return &holdRefusal{status: http.StatusConflict, code: service.CheckoutConflict, reason: "balance_short",
			required: required.String(), balance: balance.String()}
	}
	return nil
}

// beforeHold is what a session's cart passes before it holds stock, in this order:
//
//  1. A request the reservation already answered is not checked or counted again, so a retry after the payment
//     reads its first answer. When that answer is an order closed without a payment, the cart was released.
//  2. The limit of the account, at its address, on carts asked for (HoldAttempts).
//  3. The balance of the proven account, read from the chain inside the cap on balance reads (BalanceReads).
//  4. The limit of the address on carts its accounts come to hold (HoldLimiter): counted here, for an account that
//     has the balance, and given back by the function returned when the reservation then held nothing.
//
// The function returned is nil when nothing was counted for the address.
func (h HTTP) beforeHold(ctx context.Context, client string, bound service.Bound, cart Cart) (func(held bool), error) {
	lines, err := json.Marshal(cart.Items)
	if err != nil {
		return nil, service.BadRequest
	}
	var payer []byte
	var amount *string
	var released bool
	if err = h.Store.db.QueryRow(ctx, "SELECT payer_,amount_::text,released_ FROM ledger.customer_hold_check($1,$2,$3,$4,$5,$6,$7)",
		bound.Tenant, bound.Request.Domain, bound.Subject, bound.Request.IdempotencyKey, cart.MenuVersion,
		bound.Request.Body["asset"], lines).Scan(&payer, &amount, &released); err != nil {
		return nil, domainError(err)
	}
	if released {
		return nil, &holdRefusal{status: http.StatusConflict, code: service.CheckoutConflict, reason: "cart_released"}
	}
	if len(payer) != common.AddressLength || amount == nil {
		return nil, nil
	}
	required, ok := new(big.Int).SetString(*amount, 10)
	if !ok || required.Sign() <= 0 {
		return nil, service.Unknown
	}
	limited := func(wait time.Duration) error {
		h.Customer.Limited.add("hold")
		return &holdRefusal{status: http.StatusTooManyRequests, code: service.Busy, reason: "hold_limit", retry: wait}
	}
	if limiter := h.Customer.HoldAttempts; limiter != nil {
		if allowed, wait := limiter.Take(client + "/" + bound.Subject); !allowed {
			return nil, limited(wait)
		}
	}
	if reads := h.Customer.BalanceReads; reads != nil {
		select {
		case reads <- struct{}{}:
			defer func() { <-reads }()
		default:
			// The chain is answering slowly: this cart does not wait for it in a place a menu read needs.
			h.Customer.Limited.add("balance")
			return nil, &holdRefusal{status: http.StatusServiceUnavailable, code: service.Busy, reason: "balance_unavailable", retry: balanceRetry}
		}
	}
	if err = checkFunds(ctx, h.Customer.Funds, bound.Request.Domain, common.BytesToAddress(payer), bound.Request.Body["asset"], required); err != nil {
		return nil, err
	}
	limiter := h.Customer.HoldLimiter
	if limiter == nil {
		return nil, nil
	}
	if allowed, wait := limiter.Take(client); !allowed {
		return nil, limited(wait)
	}
	return func(held bool) {
		if !held {
			limiter.Refund(client) // only a cart that holds stock counts for its address
		}
	}, nil
}
