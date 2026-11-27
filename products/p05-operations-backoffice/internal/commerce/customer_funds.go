package commerce

import (
	"context"
	"encoding/json"
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
	// chain has it now.
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
//	balance_unavailable  503 BUSY with Retry-After: the chain could not be read; nothing is said about the balance
//	hold_limit           429 BUSY with Retry-After: the address started too many carts in a short time
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
	unread := &holdRefusal{status: http.StatusServiceUnavailable, code: service.Busy, reason: "balance_unavailable", retry: balanceRetry}
	if funds == nil {
		return unread // no chain to ask: the cart is not held on trust
	}
	read, cancel := context.WithTimeout(ctx, balanceReadTimeout)
	defer cancel()
	settled, balance, err := funds.PayerBalance(read, branch, payer)
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

// beforeHold is what a session's cart passes before it holds stock: the limit of its address on new carts, and the
// balance of its proven account. A request the reservation already answered is not checked again, so a retry after
// the payment reads its first answer.
func (h HTTP) beforeHold(ctx context.Context, client string, bound service.Bound, cart Cart) error {
	if limiter := h.Customer.HoldLimiter; limiter != nil {
		if allowed, wait := limiter.Take(client); !allowed {
			h.Customer.Limited.add("hold")
			return &holdRefusal{status: http.StatusTooManyRequests, code: service.Busy, reason: "hold_limit", retry: wait}
		}
	}
	lines, err := json.Marshal(cart.Items)
	if err != nil {
		return service.BadRequest
	}
	var payer []byte
	var amount *string
	if err = h.Store.db.QueryRow(ctx, "SELECT payer_,amount_::text FROM ledger.customer_hold_check($1,$2,$3,$4,$5,$6,$7)",
		bound.Tenant, bound.Request.Domain, bound.Subject, bound.Request.IdempotencyKey, cart.MenuVersion,
		bound.Request.Body["asset"], lines).Scan(&payer, &amount); err != nil {
		return domainError(err)
	}
	if len(payer) != common.AddressLength || amount == nil {
		return nil
	}
	required, ok := new(big.Int).SetString(*amount, 10)
	if !ok || required.Sign() <= 0 {
		return service.Unknown
	}
	return checkFunds(ctx, h.Customer.Funds, bound.Request.Domain, common.BytesToAddress(payer), bound.Request.Body["asset"], required)
}
