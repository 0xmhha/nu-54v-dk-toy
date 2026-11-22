package settlement

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testKioskMenuFinalOutcomes(t *testing.T, ctx context.Context, db *pgxpool.Pool, url string, base seededOrder, terminalID string,
	lab *nativeSubscriptionLab, customer, terminal *http.Client, alternate pilotIdentity, chain *ethclient.Client, token *testusdc.Testusdc,
	contract *v2.Settlementv2, proveUnpaid func(*testing.T, seededOrder)) {
	t.Helper()
	branch, asset := base.Branch, lower(base.Target.Asset)
	owner := pilotIdentity{actor: base.Actor, board: fixtureSigner(102), client: customer}
	publish := func(version int, price string) {
		t.Helper()
		fixtureExec(t, db, "INSERT INTO ledger.menu_release VALUES($1,$2,now()-interval '1 second',now()+interval '15 minutes')", branch, version)
		fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,$2,'kiosk-test','plain','시험 크루아상',$3,$4)", branch, version, asset, price)
	}
	var version int
	if err := db.QueryRow(ctx, "SELECT coalesce(max(version),0)+1 FROM ledger.menu_release WHERE branch_id=$1", branch).Scan(&version); err != nil {
		t.Fatal(err)
	}
	fixtureExec(t, db, "INSERT INTO ledger.stock VALUES ($1,'kiosk-test',4)", branch)
	publish(version, "6500000")
	device := base
	device.Actor = terminalID
	terminalURL := url
	var recoverySale string
	var armLoss, responseLost *atomic.Bool
	if alternate.client != nil {
		terminalURL, armLoss, responseLost = travelAckTunnel(t, ctx, url, func(ctx context.Context) (bool, error) {
			var committed bool
			err := db.QueryRow(ctx, `SELECT EXISTS(SELECT FROM ledger.checkout_claim
				WHERE branch_id=$1 AND sale_id=$2 AND actor_id=$3)`, branch, recoverySale, owner.actor).Scan(&committed)
			return committed, err
		})
	}
	var registration map[string]any
	if err := json.Unmarshal(lab.enrollmentDocument(t, terminalURL, device, nil), &registration); err != nil {
		t.Fatal(err)
	}
	repricedMarker(t, lab, "enrollment.json", registration)
	defer os.Remove(filepath.Join(lab.path, "enrollment.json"))
	stage := func(name string, fields map[string]string) {
		if fields == nil {
			fields = make(map[string]string)
		}
		fields["stage"] = name
		repricedMarker(t, lab, "stage.json", fields)
		repricedWait(t, ctx, lab, name)
	}
	read := func(client *http.Client, id string) commerce.Order {
		t.Helper()
		cmd := newPilotCommand(t, db, branch, id, "order.status", map[string]string{}, false)
		reply, _, err := pilotCall(ctx, client, url, cmd, nil)
		if err != nil || reply.Order == nil || reply.Order.ID != id {
			t.Fatalf("original status unavailable: %v", err)
		}
		return *reply.Order
	}
	original := func(want int, amount string, previous ...string) string {
		t.Helper()
		var sale string
		var total int
		if err := db.QueryRow(ctx, `SELECT count(*),coalesce(min(s.id::text) FILTER (WHERE NOT (s.id::text=ANY(coalesce($3::text[], ARRAY[]::text[]))) AND s.amount=$4::text::numeric),'')
			FROM ledger.sale s JOIN ledger.order_checkout c ON c.sale_id=s.id
			WHERE c.branch_id=$1 AND c.creator=$2 AND s.origin='menu'`, branch, terminalID, previous, amount).Scan(&total, &sale); err != nil ||
			total != want || sale == "" {
			t.Fatalf("installed checkout did not preserve original order count: %d %v", total, err)
		}
		return sale
	}
	requestPair := func(client *http.Client, sale string) string {
		t.Helper()
		cmd := newPilotCommand(t, db, branch, sale, "checkout.request", map[string]string{}, true)
		reply, _, err := pilotCall(ctx, client, url, cmd, nil)
		if err != nil || reply.Comparison == nil {
			t.Fatalf("customer did not receive an independent comparison: %v", err)
		}
		return reply.Comparison.Code
	}
	issue := func(sale, amount string) seededOrder {
		t.Helper()
		claim := newPilotCommand(t, db, branch, sale, "checkout.claim", map[string]string{}, false)
		view := newPilotCommand(t, db, branch, sale, "order.status", map[string]string{}, false)
		quote := newPilotCommand(t, db, branch, sale, "quote.issue", nil, true)
		out, err := issuePilotOrder(ctx, url, owner, base, sale, claim, view, quote, asset, amount)
		if err != nil {
			t.Fatal(err)
		}
		if terminalOrder := read(terminal, sale); terminalOrder.State != "issued" ||
			terminalOrder.MenuCheckout == nil || terminalOrder.MenuCheckout.FinalOutcome != nil {
			t.Fatal("mere quote issuance was projected as a final result")
		}
		return out
	}

	key, err := crypto.ToECDSA(common.LeftPadBytes([]byte{100}, 32))
	if err != nil {
		t.Fatal(err)
	}
	operator, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	operator.Context, operator.GasLimit = ctx, 500000
	fund := func(transaction *types.Transaction, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := bind.WaitMined(ctx, chain, transaction)
		if err != nil || receipt.Status != 1 {
			t.Fatalf("escrow fixture funding failed: %v", err)
		}
	}
	submit := func(sale string, quote seededOrder, replay bool) {
		t.Helper()
		amount := quote.Quote.Args.Amount
		fund(token.Mint(operator, operator.From, amount))
		fund(token.Approve(operator, base.Target.Contract, amount))
		fund(contract.DepositEscrow(operator, owner.board.Address(), 1, amount))
		approval := pilotBoardApproval(t, ctx, quote, owner.board, "management")
		forward := newPilotCommand(t, db, branch, sale, "payment.forward", map[string]string{
			"quoteDigest": rawHex(quote.Quote.Digest[:]), "approvalDigest": rawHex(quote.Quote.ApprovalHash[:]),
		}, true)
		approvalPayload := map[string]any{"approvalSignature": rawHex(approval)}
		if reply, _, err := pilotCall(ctx, customer, url, forward, approvalPayload); err != nil || reply.ResultRef != sale {
			t.Fatalf("customer payment was not forwarded: %v", err)
		}
		if replay {
			if reply, _, err := pilotCall(ctx, customer, url, forward, approvalPayload); err != nil || reply.ResultRef != sale {
				t.Fatalf("same customer payment did not resolve to its original submission: %v", err)
			}
		}
		var relayTx string
		waitUntil(t, func() bool {
			if db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", sale).Scan(&relayTx) != nil {
				return false
			}
			receipt, err := chain.TransactionReceipt(ctx, common.HexToHash(relayTx))
			return err == nil && receipt.Status == 1 && receipt.GasUsed > 0 &&
				receipt.EffectiveGasPrice != nil && receipt.EffectiveGasPrice.Sign() > 0
		})
		tx, _, err := chain.TransactionByHash(ctx, common.HexToHash(relayTx))
		if err != nil {
			t.Fatal(err)
		}
		sender, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8283)), tx)
		if err != nil || sender != operator.From {
			t.Fatal("original payout gas was not paid by the merchant relay")
		}
		mine(t, ctx, chain, 5)
		waitUntil(t, func() bool {
			var state string
			return db.QueryRow(ctx, "SELECT state FROM ledger.sale WHERE id=$1", sale).Scan(&state) == nil && state == "paid"
		})
	}
	stage("old-price", nil)
	publish(version+1, "7000000")
	stage("changed-price", nil)
	repriced := original(1, "7000000")
	if alternate.client != nil {
		recoverySale = repriced
		code := requestPair(customer, repriced)
		other := requestPair(alternate.client, repriced)
		if other == code {
			t.Fatal("different customers received the same comparison")
		}
		armLoss.Store(true)
		stage("reprice-recovery-pair", map[string]string{"sale": repriced, "code": code, "other": other})
		if !responseLost.Load() {
			t.Fatal("binding was not committed before losing the response")
		}
		checkOriginalBinding := func(wantQuotes int) {
			t.Helper()
			var claims, quotes, receipts int
			var payer string
			if err := db.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM ledger.checkout_claim WHERE sale_id=$1),
				(SELECT coalesce(min(actor_id::text),'') FROM ledger.checkout_claim WHERE sale_id=$1),
				(SELECT count(*) FROM ledger.quote_work WHERE sale_id=$1),
				(SELECT count(*) FROM ledger.command_receipt WHERE result_ref=$1 AND operation='authorize_checkout')`, repriced).
				Scan(&claims, &payer, &quotes, &receipts); err != nil || claims != 1 || payer != owner.actor || quotes != wantQuotes || receipts != 1 {
				t.Fatalf("response loss changed original binding or preissued a quote: claims=%d quotes=%d binds=%d err=%v", claims, quotes, receipts, err)
			}
		}
		checkOriginalBinding(0)
		stage("reprice-recovery-check", map[string]string{"sale": repriced, "code": code})
		chosen := read(customer, repriced)
		public := read(terminal, repriced)
		if chosen.State != "preview" || public.MenuCheckout == nil || !public.MenuCheckout.PayerBound ||
			public.MenuCheckout.CustomerRef == nil || *public.MenuCheckout.CustomerRef != owner.actor || public.Payment != nil {
			t.Fatal("private and public recovery did not preserve the chosen original")
		}
		loserClaim := newPilotCommand(t, db, branch, repriced, "checkout.claim", map[string]string{}, false)
		if reply, _, _ := pilotCall(ctx, alternate.client, url, loserClaim, nil); reply.Claim != nil {
			t.Fatal("unselected phone received an original order claim")
		}
		recovered := issue(repriced, "7000000")
		before := read(customer, repriced)
		loserStatus := newPilotCommand(t, db, branch, repriced, "order.status", map[string]string{}, false)
		if reply, _, _ := pilotCall(ctx, alternate.client, url, loserStatus, nil); reply.Order != nil &&
			(reply.Order.QuoteDigest != nil || reply.Order.Payment != nil || reply.Order.ReceiptRef != nil) {
			t.Fatal("unselected phone received signed quote or payment evidence")
		}
		stage("reprice-recovery-issued", map[string]string{"sale": repriced})
		reopened := issue(repriced, "7000000")
		after := read(customer, repriced)
		if !bytes.Equal(reopened.Signed, recovered.Signed) || reopened.Quote.Digest != recovered.Quote.Digest ||
			before.QuoteDigest == nil || after.QuoteDigest == nil || *before.QuoteDigest != *after.QuoteDigest ||
			*before.QuoteDigest != rawHex(recovered.Quote.Digest[:]) || before.Revision != after.Revision {
			t.Fatal("public clearing changed the private original signed quote")
		}
		checkOriginalBinding(1)
		publish(version+2, "6500000")
		stage("baseline-order", nil)
		baseline := original(2, "6500000", repriced)
		stage("baseline-pair", map[string]string{"sale": baseline, "code": requestPair(customer, baseline)})
		baselineQuote := issue(baseline, "6500000")
		beforeBalance, err := token.BalanceOf(&bind.CallOpts{Context: ctx}, baselineQuote.Quote.Args.Beneficiary)
		if err != nil {
			t.Fatal(err)
		}
		submit(baseline, baselineQuote, true)
		afterBalance, err := token.BalanceOf(&bind.CallOpts{Context: ctx}, baselineQuote.Quote.Args.Beneficiary)
		if err != nil || new(big.Int).Sub(afterBalance, beforeBalance).Cmp(big.NewInt(6500000)) != 0 {
			t.Fatal("merchant balance did not increase by exactly one unchanged-price payment")
		}
		final, phone := read(terminal, baseline), read(customer, baseline)
		if final.MenuCheckout == nil || final.MenuCheckout.FinalOutcome == nil || *final.MenuCheckout.FinalOutcome != "relay-paid" ||
			final.ReceiptRef == nil || phone.ReceiptRef == nil || *final.ReceiptRef != *phone.ReceiptRef || final.PickupNo == nil ||
			phone.PickupNo == nil || *final.PickupNo != *phone.PickupNo || final.Amount != "6500000" || final.Payment != nil {
			t.Fatal("unchanged-price checkout did not return the original merchant and private pickup")
		}
		stage("baseline-paid", map[string]string{"sale": baseline, "pickup": strconv.FormatInt(*final.PickupNo, 10)})
		assertKioskOutcomeCounts(t, ctx, db, repriced, 1, 0)
		assertKioskOutcomeCounts(t, ctx, db, baseline, 1, 1)
		original(2, "6500000", repriced)
		return
	}
	paid := repriced
	stage("paid-pair", map[string]string{"sale": paid, "code": requestPair(customer, paid)})
	paidQuote := issue(paid, "7000000")
	submit(paid, paidQuote, false)
	terminalPaid, customerPaid := read(terminal, paid), read(customer, paid)
	if terminalPaid.MenuCheckout == nil || terminalPaid.MenuCheckout.FinalOutcome == nil ||
		*terminalPaid.MenuCheckout.FinalOutcome != "relay-paid" || terminalPaid.ReceiptRef == nil ||
		customerPaid.ReceiptRef == nil || *terminalPaid.ReceiptRef != *customerPaid.ReceiptRef || terminalPaid.PickupNo == nil ||
		terminalPaid.Amount != "7000000" || terminalPaid.Payment != nil {
		t.Fatal("terminal did not obtain scoped final payment and the original receipt")
	}
	stage("paid-read", map[string]string{"sale": paid})
	stage("restart-paid", map[string]string{"sale": paid})
	stage("unpaid-order", nil)
	unpaid := original(2, "7000000", paid)
	fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=15 WHERE branch_id=$1", branch)
	stage("unpaid-pair", map[string]string{"sale": unpaid, "code": requestPair(customer, unpaid)})
	oldQuote := issue(unpaid, "7000000")
	stage("unpaid-issued", map[string]string{"sale": unpaid})
	select {
	case <-time.After(time.Until(oldQuote.Order.Expires.Add(time.Second))):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	head, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if head.Time <= uint64(oldQuote.Order.Expires.Unix()) {
		forceTime(t, ctx, chain, oldQuote.Order.Expires.Unix()+1)
	}
	mine(t, ctx, chain, 8)
	waitUntil(t, func() bool {
		var reached bool
		return db.QueryRow(ctx, `SELECT b.block_time >= $2 FROM chainview.read_target r
			JOIN chainview.generation g ON g.id=r.generation_id
			JOIN chainview.block b ON b.generation_id=g.id AND b.block_hash=g.finalized_hash
			WHERE r.stream_id=$1`, base.Target.Stream, oldQuote.Order.Expires).Scan(&reached) == nil && reached
	})
	proveUnpaid(t, oldQuote)
	waitUntil(t, func() bool {
		var known bool
		return db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM ledger.sale_resolution WHERE sale_id=$1)", unpaid).Scan(&known) == nil && known
	})
	terminalUnpaid := read(terminal, unpaid)
	if terminalUnpaid.MenuCheckout == nil || terminalUnpaid.MenuCheckout.FinalOutcome == nil ||
		*terminalUnpaid.MenuCheckout.FinalOutcome != "expired-unpaid" || terminalUnpaid.ReceiptRef != nil ||
		terminalUnpaid.Payment != nil {
		t.Fatal("terminal did not obtain scoped original unpaid resolution")
	}
	stage("unpaid-read", map[string]string{"sale": unpaid})
	stage("restart-unpaid", map[string]string{"sale": unpaid})

	fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=120 WHERE branch_id=$1", branch)
	stage("unbound-order", nil)
	unbound := original(3, "7000000", paid, unpaid)
	stage("unbound-idle", map[string]string{"sale": unbound})
	unconnected := read(terminal, unbound)
	if unconnected.State != "preview" || unconnected.MenuCheckout == nil || unconnected.MenuCheckout.PayerBound ||
		unconnected.MenuCheckout.FinalOutcome != nil {
		t.Fatal("public connection expiry changed the unbound original order")
	}
	stage("signed-order", nil)
	signed := original(4, "7000000", paid, unpaid, unbound)
	stage("signed-pair", map[string]string{"sale": signed, "code": requestPair(customer, signed)})
	signedQuote := issue(signed, "7000000")
	beforeIdle := read(customer, signed)
	stage("signed-idle", map[string]string{"sale": signed})
	afterIdle := read(customer, signed)
	if beforeIdle.State != "issued" || afterIdle.State != "issued" || beforeIdle.QuoteDigest == nil ||
		afterIdle.QuoteDigest == nil || *beforeIdle.QuoteDigest != *afterIdle.QuoteDigest ||
		afterIdle.Revision != beforeIdle.Revision || afterIdle.ReceiptRef != nil {
		t.Fatal("public idle reset changed the linked customer's original signed quote")
	}
	submit(signed, signedQuote, true)
	phoneReceipt := read(customer, signed)
	if phoneReceipt.State != "paid" || phoneReceipt.ReceiptRef == nil {
		t.Fatal("linked customer could not recover the original paid receipt after the kiosk reset")
	}
	for _, test := range []struct {
		sale     string
		quotes   int
		payments int
	}{{paid, 1, 1}, {unpaid, 1, 0}, {unbound, 0, 0}, {signed, 1, 1}} {
		assertKioskOutcomeCounts(t, ctx, db, test.sale, test.quotes, test.payments)
	}
	original(4, "7000000", paid, unpaid, unbound)
}

func assertKioskOutcomeCounts(t *testing.T, ctx context.Context, db *pgxpool.Pool, sale string, quoteCount, paymentCount int) {
	t.Helper()
	var quotes, submissions, receipts, paidRows int
	err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.quote_work WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.journal WHERE sale_id=$1 AND refund_id IS NULL),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$1)`, sale).
		Scan(&quotes, &submissions, &receipts, &paidRows)
	if err != nil || quotes != quoteCount || submissions != paymentCount || receipts != paymentCount || paidRows != paymentCount {
		t.Fatalf("original ledger counts quote=%d submission=%d receipt=%d payment=%d: %v", quotes, submissions, receipts, paidRows, err)
	}
}
