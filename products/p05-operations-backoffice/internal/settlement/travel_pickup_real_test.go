package settlement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

type travelPickupWire struct {
	SaleRef    string
	State      string
	PickupNo   *int64
	ReceiptRef *string
}

func testTravelPickupOriginal(t *testing.T, ctx context.Context, db *pgxpool.Pool, url string, base seededOrder,
	chain *ethclient.Client, token *testusdc.Testusdc, contract *v2.Settlementv2, clients [3]*http.Client, dispatch Dispatcher, native *nativeSubscriptionLab) {
	t.Helper()
	const amount = "3500000"
	const sku = "t06-travel-pickup"
	asset := lower(base.Target.Asset)
	starts := time.Now().UTC().Add(20 * time.Minute).Truncate(time.Second)
	window := fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,$2,'plain','시험 커피',$3,$4)", base.Branch, sku, asset, amount)
	fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,$2,1)", base.Branch, sku)
	fixtureExec(t, db, `INSERT INTO ledger.travel_pickup_window(branch_id,id,starts_at,ends_at,capacity)
		VALUES($1,$2,$3,$3::timestamptz+interval '15 minutes',1)`, base.Branch, window, starts)

	var sale string
	stage := func(name string) {
		t.Helper()
		if native != nil {
			waitNativeTravelStage(t, ctx, native, "t06-"+name)
		}
	}
	if native != nil {
		prepareNativeTravel(t, native, url, base, "", "", true)
		stage("reserved")
		if err := db.QueryRow(ctx, `SELECT o.sale_id::text FROM ledger.travel_pickup_order o
			JOIN ledger.order_checkout c ON (c.branch_id,c.sale_id)=(o.branch_id,o.sale_id)
			WHERE o.branch_id=$1 AND o.window_id=$2 AND c.creator=$3`, base.Branch, window, base.Actor).Scan(&sale); err != nil {
			t.Fatal("installed pickup did not reserve the exact original published window")
		}
	} else {
		cmd, cart := pilotReserveCart(t, db, base.Branch, asset, []commerce.Item{{SKU: sku, Option: "plain", Qty: 1}})
		cart.Pickup = &commerce.PickupChoice{Window: window, StartsAt: starts.Format(time.RFC3339)}
		raw, err := canonicalPilotCart(cart)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		cmd.Body["intentDigest"] = rawHex(digest[:])
		created, _, err := pilotCall(ctx, clients[0], url, cmd, map[string]any{"cart": cart})
		if err != nil || created.ResultRef == "" {
			t.Fatalf("T06 exact window reserve: %v", err)
		}
		sale = created.ResultRef
	}

	travel := func(client *http.Client, request commerce.TravelRequest, wantedStatus int) []travelPickupWire {
		t.Helper()
		request.Branch, request.Sale = base.Branch, sale
		raw, err := json.Marshal(map[string]commerce.TravelRequest{"travel": request})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", url+"/v2/commerce", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var reply struct{ Pickups []travelPickupWire }
		if res.StatusCode != wantedStatus {
			t.Fatalf("T06 %s HTTP status %d want %d", request.Operation, res.StatusCode, wantedStatus)
		}
		if wantedStatus != http.StatusOK {
			return nil
		}
		if json.NewDecoder(res.Body).Decode(&reply) != nil {
			t.Fatalf("T06 %s HTTP status %d or malformed response", request.Operation, res.StatusCode)
		}
		return reply.Pickups
	}
	var relayHash string
	check := func(state string, receipt, numbered bool) travelPickupWire {
		t.Helper()
		pickups := travel(clients[0], commerce.TravelRequest{Operation: "pickup-owned"}, http.StatusOK)
		if len(pickups) != 1 || pickups[0].SaleRef != sale || pickups[0].State != state ||
			(pickups[0].ReceiptRef != nil) != receipt || (pickups[0].PickupNo != nil) != numbered {
			t.Fatalf("T06 expected %s with receipt=%t number=%t on original", state, receipt, numbered)
		}
		view := newPilotCommand(t, db, base.Branch, sale, "order.status", map[string]string{}, false)
		status, _, err := pilotCall(ctx, clients[0], url, view, nil)
		if err != nil || status.Order == nil || status.Order.ID != sale ||
			(status.Order.ReceiptRef != nil) != receipt || (status.Order.PickupNo != nil) != numbered {
			t.Fatalf("T06 C3 original view inconsistent in %s: %v", state, err)
		}
		if receipt && (status.Order.State != "paid" || *status.Order.ReceiptRef != *pickups[0].ReceiptRef ||
			status.Order.Payment == nil || status.Order.Payment.Stage != "finalized") {
			t.Fatalf("T06 receipt or finality changed in %s", state)
		}
		if relayHash != "" && !receipt && (status.Order.State != "checking" || status.Order.Payment == nil ||
			status.Order.Payment.RelayTxHash == nil || *status.Order.Payment.RelayTxHash != relayHash) {
			t.Fatalf("T06 pending C3 did not return the original relay hash")
		}
		if receipt && (status.Order.Payment.TxHash == nil || *status.Order.Payment.TxHash != relayHash) {
			t.Fatal("T06 finalized C3 did not resolve to the same relay transaction")
		}
		if numbered && *status.Order.PickupNo != *pickups[0].PickupNo {
			t.Fatal("T06 number differs across original reads")
		}
		return pickups[0]
	}
	check("waiting-payment", false, false)

	key, err := crypto.ToECDSA(common.LeftPadBytes([]byte{100}, 32))
	if err != nil {
		t.Fatal(err)
	}
	operator, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	operator.Context, operator.GasLimit = ctx, 500_000
	confirm := func(tx *types.Transaction, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := bind.WaitMined(ctx, chain, tx)
		if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
			t.Fatalf("T06 escrow funding: %v", err)
		}
	}
	funds := big.NewInt(3500000)
	board := fixtureSigner(102)
	confirm(token.Mint(operator, operator.From, funds))
	confirm(token.Approve(operator, base.Target.Contract, funds))
	confirm(contract.DepositEscrow(operator, board.Address(), 1, funds))

	selected := base
	selected.Sale = sale
	owner := pilotIdentity{actor: base.Actor, board: board, client: clients[0]}
	if err := chain.Client().CallContext(ctx, nil, "evm_setAutomine", false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reset, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = chain.Client().CallContext(reset, nil, "evm_setAutomine", true)
	})
	if native == nil {
		item := issueTimedPilot(t, ctx, db, url, selected, owner, owner, amount)
		signature := pilotBoardApproval(t, ctx, item, board, "management")
		forward := newPilotCommand(t, db, base.Branch, sale, "payment.forward", map[string]string{
			"quoteDigest": rawHex(item.Quote.Digest[:]), "approvalDigest": rawHex(item.Quote.ApprovalHash[:]),
		}, true)
		reply, _, err := pilotCall(ctx, clients[0], url, forward, map[string]any{"approvalSignature": rawHex(signature)})
		if err != nil || reply.ResultRef != sale {
			t.Fatalf("T06 original approval submission: %v", err)
		}
	} else {
		stage("quote")
		var key, claim string
		var expected int64
		err := db.QueryRow(ctx, `SELECT r.request_key::text,encode(q.claim_digest,'hex'),q.expected_revision
			FROM ledger.command_receipt r JOIN ledger.quote_work q ON q.sale_id=r.result_ref
			WHERE r.branch_id=$1 AND r.scope_ref=$2 AND r.result_ref=$3 AND r.operation='issue_quote'`,
			base.Branch, base.Actor, sale).Scan(&key, &claim, &expected)
		if err != nil {
			t.Fatal("installed pickup signed quote was not committed")
		}
		resume := newPilotCommand(t, db, base.Branch, sale, "quote.issue", map[string]string{
			"claimDigest": claim, "orderRevision": strconv.FormatInt(expected, 10),
		}, false)
		resume.IdempotencyKey = key
		recovered, _, err := pilotCall(ctx, clients[0], url, resume, nil)
		if err != nil || recovered.Quote == nil {
			t.Fatalf("installed pickup same-key quote recovery: %v", err)
		}
		q := recovered.Quote
		signed, err := hex.DecodeString(q.Signed)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := hex.DecodeString(q.Digest)
		if err != nil {
			t.Fatal(err)
		}
		original, err := unpackQuote(signed, digest, fixtureSigner(101).Address().Bytes(), base.Target)
		if err != nil || q.SaleRef != sale || original.Args.Amount.String() != amount || original.Args.Payer != board.Address() ||
			!validPilotPresentation(q, original.Args.PayerGeneration) {
			t.Fatal("installed pickup quote or signed presentation changed")
		}
		selected.Signed, selected.Quote, selected.Presentation = signed, original, q.Presentation
		selected.Order.OrderID, selected.Order.Expires = original.Args.OrderId, time.Unix(q.Expires, 0)
		stage("board-approve")
		chainID := int32(8283)
		independentTravelDisplay(t, ctx, os.Getenv("NU54_TRAVEL_DISPLAY_PYTHON"), selected,
			commerce.TravelBooking{TravelTerms: commerce.TravelTerms{ChainID: &chainID, Asset: asset}}, amount, lower(original.Args.Beneficiary), filepath.Join(native.path, "board"), 1)
		stage("submitted")
	}

	waitUntil(t, func() bool {
		return db.QueryRow(ctx, `SELECT a.tx_hash FROM ledger.relay_attempt a JOIN ledger.settlement_work w ON w.id=a.work_id
			WHERE w.sale_id=$1 AND w.refund_id IS NULL AND a.state='broadcast'`, sale).Scan(&relayHash) == nil
	})
	counts := func() (c [10]int) {
		t.Helper()
		err := db.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM ledger.quote_work WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.relay_attempt a JOIN ledger.settlement_work w ON w.id=a.work_id WHERE w.sale_id=$1 AND w.refund_id IS NULL),
			(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.pickup WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.consumer_receipt c JOIN ledger.fulfillment_outbox o USING(event_id) WHERE o.sale_id=$1),
			(SELECT count(*) FROM ledger.travel_booking_leg WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.command_receipt WHERE result_ref=$1 AND operation='payment_forward'),
			(SELECT count(*) FROM ledger.command_receipt WHERE result_ref=$1 AND operation='issue_quote'),
			(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$1 AND kind='paid')`, sale).
			Scan(&c[0], &c[1], &c[2], &c[3], &c[4], &c[5], &c[6], &c[7], &c[8], &c[9])
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	stage("pending")
	for range 2 {
		clients[0].CloseIdleConnections()
		check("waiting-payment", false, false)
	}
	if c := counts(); c != ([10]int{1, 1, 1, 0, 0, 0, 0, 1, 1, 0}) {
		t.Fatalf("T06 pending counts: %v", c)
	}
	mine(t, ctx, chain, 6)
	if err := chain.Client().CallContext(ctx, nil, "evm_setAutomine", true); err != nil {
		t.Fatal(err)
	}
	receipt, err := chain.TransactionReceipt(ctx, common.HexToHash(relayHash))
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("T06 original relay did not succeed: %v", err)
	}
	waitUntil(t, func() bool {
		var principal string
		return db.QueryRow(ctx, "SELECT principal::text FROM ledger.payment WHERE sale_id=$1 AND tx_hash=$2", sale, relayHash).Scan(&principal) == nil && principal == amount
	})
	paid := check("awaiting-acceptance", true, false)
	stage("paid")
	travel(clients[1], commerce.TravelRequest{Operation: "pickup-owned"}, http.StatusNotFound)
	if c := counts(); c != ([10]int{1, 1, 1, 1, 1, 0, 0, 1, 1, 1}) {
		t.Fatalf("T06 pre-ACK counts: %v", c)
	}

	// The dispatcher uses SKIP LOCKED. Fence fixture siblings locally without
	// changing their leases or scheduling order.
	gate, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Rollback(context.Background())
	if _, err := gate.Exec(ctx, "SELECT event_id FROM ledger.fulfillment_outbox WHERE sale_id<>$1 AND state<>'delivered' FOR UPDATE", sale); err != nil {
		t.Fatal(err)
	}
	moved, err := dispatch.Step(ctx)
	if !moved || err != nil {
		t.Fatalf("T06 production paid ACK: %v", err)
	}
	if err := gate.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	accepted := check("accepted", true, true)
	for i, state := range []string{"preparing", "ready"} {
		expected := int64(i)
		step := commerce.TravelRequest{Operation: "pickup-change", Key: fixtureUUID(t, db), Expected: &expected, Target: state}
		for range 2 {
			travel(clients[2], step, http.StatusOK)
		}
		clients[0].CloseIdleConnections()
		current := check(state, true, true)
		stage(state)
		if *current.PickupNo != *accepted.PickupNo || *current.ReceiptRef != *paid.ReceiptRef {
			t.Fatal("T06 original receipt or number changed")
		}
	}
	stage("restarted")
	current := check("ready", true, true)
	if *current.PickupNo != *accepted.PickupNo || *current.ReceiptRef != *paid.ReceiptRef {
		t.Fatal("T06 reopened a different original receipt or number")
	}
	if c := counts(); c != ([10]int{1, 1, 1, 1, 1, 1, 0, 1, 1, 1}) {
		t.Fatalf("T06 final counts: %v", c)
	}
	t.Log("T06 original: quote/approval/relay/payment/number/merchant ACK = 1/1/1/1/1/1; seats = 0")
}
