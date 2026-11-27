package commerce

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// fakeFunds answers a balance the test sets, or a failure, and records what was asked.
type fakeFunds struct {
	asset   common.Address
	balance *big.Int
	err     error
	wait    bool // do not answer before the caller gives up
	asked   []string
}

func (f *fakeFunds) PayerBalance(ctx context.Context, branch string, payer common.Address) (common.Address, *big.Int, error) {
	f.asked = append(f.asked, branch+"/"+strings.ToLower(payer.Hex()))
	if f.wait {
		<-ctx.Done()
		return common.Address{}, nil, ctx.Err()
	}
	return f.asset, f.balance, f.err
}

// A cart holds stock only when the chain says its account can pay it now: enough, short, and a chain that cannot be read.
func TestBalanceBeforeHold(t *testing.T) {
	token := common.HexToAddress("0x" + strings.Repeat("2", 40))
	asset := strings.ToLower(token.Hex())
	payer := common.HexToAddress("0x" + strings.Repeat("7", 40))
	cart := big.NewInt(300_000)
	check := func(funds PayerFunds) error {
		return checkFunds(context.Background(), funds, branch, payer, asset, cart)
	}
	refusal := func(err error) *holdRefusal {
		t.Helper()
		var refused *holdRefusal
		if !errors.As(err, &refused) {
			t.Fatalf("not a hold refusal: %v", err)
		}
		return refused
	}
	// Enough, to the base unit.
	for _, balance := range []int64{300_000, 300_001, 9_000_000} {
		funds := &fakeFunds{asset: token, balance: big.NewInt(balance)}
		if err := check(funds); err != nil {
			t.Fatalf("a balance of %d was refused for a cart of %s: %v", balance, cart, err)
		}
		if len(funds.asked) != 1 || funds.asked[0] != branch+"/"+strings.ToLower(payer.Hex()) {
			t.Fatalf("asked %v, want the proven account at its branch once", funds.asked)
		}
	}
	// Short, by one base unit or by all of it.
	for _, balance := range []int64{299_999, 1, 0} {
		short := refusal(check(&fakeFunds{asset: token, balance: big.NewInt(balance)}))
		if short.status != http.StatusConflict || short.code != service.CheckoutConflict || short.reason != "balance_short" ||
			short.required != "300000" || short.balance != big.NewInt(balance).String() || short.retry != 0 {
			t.Fatalf("a balance of %d: %+v", balance, short)
		}
	}
	// The chain cannot be read: no reader, a failure, no answer in time, or an answer that is no balance.
	slow := &fakeFunds{wait: true}
	started := time.Now()
	for name, funds := range map[string]PayerFunds{
		"no reader": nil, "failure": &fakeFunds{err: errors.New("rpc down")}, "no answer in time": slow,
		"no balance": &fakeFunds{asset: token}, "negative": &fakeFunds{asset: token, balance: big.NewInt(-1)},
		// A failure is a failure even when a balance came with it.
		"failure with a balance": &fakeFunds{asset: token, balance: big.NewInt(9_000_000), err: errors.New("stale")},
	} {
		unread := refusal(check(funds))
		if unread.status != http.StatusServiceUnavailable || unread.code != service.Busy || unread.reason != "balance_unavailable" ||
			unread.retry != balanceRetry || unread.required != "" || unread.balance != "" {
			t.Fatalf("%s: %+v", name, unread)
		}
	}
	if waited := time.Since(started); waited < balanceReadTimeout || waited > balanceReadTimeout+2*time.Second {
		t.Fatalf("a chain that does not answer was waited for %v, want about %v", waited, balanceReadTimeout)
	}
	// A balance of another asset than the cart is priced in proves nothing about the cart.
	other := &fakeFunds{asset: common.HexToAddress("0x" + strings.Repeat("3", 40)), balance: big.NewInt(9_000_000)}
	if err := check(other); !errors.Is(err, service.Mismatch) {
		t.Fatalf("a balance of another asset: %v", err)
	}
}

// What the app reads of a refused hold: the request's own envelope, the reason, and the wait when there is one.
func TestHoldRefusalReplies(t *testing.T) {
	bound := service.Bound{RequestDigest: strings.Repeat("ab", 32), Request: service.Request{RequestID: "6f1c1a52-4c39-4d1e-9d36-1f0b9b4d2a10"}}
	const envelope = `{"requestId":"6f1c1a52-4c39-4d1e-9d36-1f0b9b4d2a10","requestDigest":"` + "abababababababababababababababababababababababababababababababab" + `","status":"error","error":`
	for name, want := range map[string]struct {
		refusal *holdRefusal
		status  int
		retry   string
		body    string
	}{
		"short": {&holdRefusal{status: 409, code: service.CheckoutConflict, reason: "balance_short", required: "300000", balance: "1200"}, 409, "",
			`"CHECKOUT_CONFLICT","reason":"balance_short","required":"300000","balance":"1200"}`},
		"unread": {&holdRefusal{status: 503, code: service.Busy, reason: "balance_unavailable", retry: balanceRetry}, 503, "3",
			`"BUSY","reason":"balance_unavailable"}`},
		"limit": {&holdRefusal{status: 429, code: service.Busy, reason: "hold_limit", retry: 9500 * time.Millisecond}, 429, "10",
			`"BUSY","reason":"hold_limit"}`},
	} {
		reply := httptest.NewRecorder()
		errorResponse(reply, want.refusal, &bound)
		if reply.Code != want.status || reply.Header().Get("Retry-After") != want.retry || strings.TrimSpace(reply.Body.String()) != envelope+want.body {
			t.Fatalf("%s: %d, Retry-After %q, %s", name, reply.Code, reply.Header().Get("Retry-After"), reply.Body)
		}
	}
}
