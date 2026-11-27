package settlement

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testTravelProcess(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn, evidenceDSN, url string, base seededOrder,
	customers [2]*http.Client, finance [3]*http.Client, chain *ethclient.Client, token *testusdc.Testusdc, contract *v2.Settlementv2, relayCipher cipher.AEAD, native *nativeSubscriptionLab, inspect func(*testing.T, string, string), proveUnpaid func(*testing.T, seededOrder)) {
	// Each step intentionally uses a new connection to verify re-open after a
	// disconnect and avoid borrowing a stale socket from another fixture step.
	for index := range customers {
		fresh := func(client *http.Client) *http.Client {
			transport := client.Transport.(*http.Transport).Clone()
			transport.DisableKeepAlives = true
			t.Cleanup(transport.CloseIdleConnections)
			return &http.Client{Transport: transport, Timeout: client.Timeout}
		}
		customers[index], finance[index] = fresh(customers[index]), fresh(finance[index])
	}
	hostMode := os.Getenv("NU54_TRAVEL_T07_HOST")
	t07Host := hostMode == "1" || hostMode == "full"
	t07Full := hostMode == "full" || os.Getenv("NU54_TRAVEL_NATIVE_T07_FULL") == "1"
	t07 := t07Host || t07Full || os.Getenv("NU54_TRAVEL_NATIVE_T07") == "1"
	const balance, pickupAmount = "20000000", "5000000"
	depositRefund := "3000000"
	deposit := "10000000"
	funding := int64(45_000_000)
	if native == nil && inspect == nil && !t07 {
		// A 30% policy yields 3000000.9 base units in the host case.
		// Rounding to the nearest unit would overstate the refund.
		deposit, funding = "10000003", 45_000_006
	}
	slot, fullSlot := fixtureUUID(t, db), fixtureUUID(t, db)
	asset := lower(base.Target.Asset)
	depositRefundBPS, cancellationTerms := 3000, "Deposit refund 30% and balance in full before cutoff"
	if t07 {
		depositRefundBPS, cancellationTerms = 10000, "Full deposit refund before cutoff"
		depositRefund = deposit
	}
	if t07Full {
		cancellationTerms = "Full deposit and balance refunds before cutoff"
	}
	fixtureExec(t, db, `INSERT INTO ledger.travel_slot(branch_id,id,starts_at,capacity,terms_version,cancellation_terms,asset_ref,deposit,balance,deposit_refund_bps,balance_refund_bps,
		price_version,no_show_terms,chain_id,cancellation_cutoff_seconds)
		VALUES($1,$2,now()+interval '2 hours',1,3,$7,$4,$5,$6,$8,10000,2,'Deposit retained for no-show',8283,1800),
		  ($1,$3,now()+interval '3 hours',1,1,'Deposit is full price',$4,$5,0,0,0,NULL,NULL,NULL,0)`, base.Branch, slot, fullSlot, asset, deposit, balance, cancellationTerms, depositRefundBPS)
	type travelReply struct {
		Booking             commerce.TravelBooking
		Bookings            []commerce.TravelBooking
		BookingRef, SaleRef string
		Legs                []commerce.TravelLeg
		CancellationPreview *commerce.TravelCancellationPreview
	}
	post := func(client *http.Client, route string, value any, status int) []byte {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", url+route, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		result, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer result.Body.Close()
		raw, err = io.ReadAll(result.Body)
		if err != nil || result.StatusCode != status {
			t.Fatalf("%s wanted %d, got %d: %s %v", route, status, result.StatusCode, raw, err)
		}
		return raw
	}
	travel := func(client *http.Client, req commerce.TravelRequest, status int) travelReply {
		t.Helper()
		req.Branch = base.Branch
		var reply travelReply
		if err := json.Unmarshal(post(client, "/v2/commerce", map[string]commerce.TravelRequest{"travel": req}, status), &reply); err != nil {
			t.Fatal(err)
		}
		return reply
	}
	hold := func(which, remainder string) commerce.TravelBooking {
		t.Helper()
		version := int64(1)
		if which == slot {
			version = 3
		}
		return travel(customers[0], commerce.TravelRequest{Operation: "hold", Slot: which,
			Key: fixtureUUID(t, db), Party: 1, Terms: version, Asset: asset, Deposit: deposit, Balance: remainder}, 200).Booking
	}
	leg := func(customer *http.Client, bookingID, op string, status int) travelReply {
		t.Helper()
		return travel(customer, commerce.TravelRequest{Operation: op, Booking: bookingID}, status)
	}
	uc1 := os.Getenv("NU54_TRAVEL_NATIVE_UC1") == "1"
	nativeBoard := native != nil && os.Getenv("NU54_TRAVEL_NATIVE_BOARD") == "1"
	if t07Host && native != nil {
		t.Fatal("T07 host driver cannot stand in for an installed customer")
	}
	if t07 && !t07Host && !uc1 {
		t.Fatal("T07 requires the installed original deposit fixture")
	}
	if uc1 && !nativeBoard {
		t.Fatal("UC1 requires the native board and enrollment fixtures")
	}
	var booking commerce.TravelBooking
	var depositSale string
	if uc1 {
		prepareNativeTravel(t, native, url, base, "", "", true)
		defer os.Remove(filepath.Join(native.path, "preview.json"))
		defer os.Remove(filepath.Join(native.path, "public-branch"))
		waitNativeTravelStage(t, ctx, native, "booking")
		bookings := travel(customers[0], commerce.TravelRequest{Operation: "my-bookings"}, 200).Bookings
		if len(bookings) != 1 || bookings[0].Slot != slot || bookings[0].Party != 1 ||
			bookings[0].Deposit != deposit || bookings[0].Balance != balance {
			t.Fatalf("expected one installed-client hold for the offered slot, got %+v", bookings)
		}
		booking = bookings[0]
		legs := leg(customers[0], booking.ID, "legs", 200).Legs
		if len(legs) != 1 || legs[0].Kind != "deposit" || legs[0].Amount != deposit || legs[0].Receipt != nil {
			t.Fatalf("expected a single original unpaid deposit order, got %+v", legs)
		}
		depositSale = legs[0].Sale
	} else {
		booking = hold(slot, balance)
		depositSale = leg(customers[0], booking.ID, "start-deposit", 200).SaleRef
	}
	if booking.Terms != 3 || booking.PriceVersion == nil || *booking.PriceVersion != 2 ||
		booking.ChainID == nil || *booking.ChainID != 8283 || booking.NoShow == nil ||
		*booking.NoShow != "Deposit retained for no-show" || booking.Starts == nil ||
		!booking.Cutoff.Equal(booking.Starts.Add(-30*time.Minute)) {
		t.Fatal("travel review did not preserve the agreed commercial terms")
	}
	publishCancellationTerms := func() {
		t.Helper()
		bookings := travel(customers[1], commerce.TravelRequest{Operation: "my-bookings"}, 200).Bookings
		for _, current := range bookings {
			if current.ID != booking.ID {
				continue
			}
			if !current.Cutoff.Equal(booking.Cutoff) || current.Cancellation != cancellationTerms ||
				current.NoShow == nil || current.PriceVersion == nil {
				t.Fatal("P05 current original policy did not match the published booking")
			}
			raw, err := json.Marshal(map[string]string{"booking": current.ID,
				"cutoff": current.Cutoff.UTC().Format(time.RFC3339), "policy": current.Cancellation,
				"noShow": *current.NoShow, "terms": strconv.FormatInt(current.Terms, 10),
				"price": strconv.FormatInt(*current.PriceVersion, 10)})
			if err != nil || os.WriteFile(filepath.Join(native.path, "cancellation-terms.json"), raw, 0o600) != nil {
				t.Fatal("could not publish the public policy comparison fixture")
			}
			return
		}
		t.Fatal("P05 did not return the original booking to its customer")
	}
	leg(customers[0], booking.ID, "start-balance", 409)
	if !uc1 {
		if same := leg(customers[1], booking.ID, "start-deposit", 200).SaleRef; depositSale == "" || same != depositSale {
			t.Fatal("separate TLS connection did not resume the deposit sale")
		}
	}
	board := fixtureSigner(102)
	owner := pilotIdentity{actor: base.Actor, board: board, client: customers[0]}
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
		result, err := bind.WaitMined(ctx, chain, tx)
		if err != nil || result.Status != types.ReceiptStatusSuccessful {
			t.Fatalf("escrow fixture transaction: %v", err)
		}
	}
	funds := big.NewInt(funding)
	confirm(token.Mint(operator, operator.From, funds))
	confirm(token.Approve(operator, base.Target.Contract, funds))
	confirm(contract.DepositEscrow(operator, board.Address(), 1, funds))
	refundReserve := big.NewInt(23_000_000)
	if t07Full {
		refundReserve.SetInt64(30_000_000)
	}
	confirm(token.Mint(operator, operator.From, refundReserve))
	confirm(token.Approve(operator, base.Target.Contract, refundReserve))
	confirm(contract.FundRefundReserve(operator, base.Quote.Args.MerchantId, refundReserve))
	confirmed := func(work string) {
		t.Helper()
		var receipt *types.Receipt
		waitUntil(t, func() bool {
			var tx string
			if db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", work).Scan(&tx) != nil {
				return false
			}
			receipt, err = chain.TransactionReceipt(ctx, common.HexToHash(tx))
			return err == nil
		})
		if receipt.Status != 1 {
			t.Fatal("settlement transaction reverted")
		}
		mine(t, ctx, chain, 5)
	}
	pythonDisplay := os.Getenv("NU54_TRAVEL_DISPLAY_PYTHON")
	t05 := nativeBoard && (os.Getenv("NU54_TRAVEL_NATIVE_T05") == "1" || t07Full)
	loseDepositAck := os.Getenv("NU54_TRAVEL_LOSE_DEPOSIT_ACK") == "1"
	if loseDepositAck && native != nil && !t05 {
		t.Fatal("lost deposit reply in an installed run requires the T05 journey")
	}
	var armAckLoss, lostAck *atomic.Bool
	appEndpoint := url
	if loseDepositAck {
		appEndpoint, armAckLoss, lostAck = travelAckTunnel(t, ctx, url, func(ctx context.Context) (bool, error) {
			var committed bool
			err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM ledger.settlement_submission WHERE sale_id=$1)", depositSale).Scan(&committed)
			return committed, err
		})
	}
	var depositApproval []byte
	challenges := make(map[string]bool)
	displayDecision := func(item seededOrder, amount, recipient string, button byte) {
		t.Helper()
		dir := filepath.Join(native.path, "board")
		independentTravelDisplay(t, ctx, pythonDisplay, item, booking, amount, recipient, dir, button)
		challenge, err := os.ReadFile(filepath.Join(dir, "display-challenge"))
		if err != nil || len(challenge) != 32 || challenges[string(challenge)] {
			t.Fatal("each local board decision requires a fresh challenge")
		}
		challenges[string(challenge)] = true
	}
	type paymentCounts struct{ quotes, submissions, receipts, forwards, issues int }
	counts := func(sale string) paymentCounts {
		t.Helper()
		var c paymentCounts
		err := db.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM ledger.quote_work WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.journal WHERE sale_id=$1 AND refund_id IS NULL),
			(SELECT count(*) FROM ledger.command_receipt WHERE result_ref=$1 AND operation='payment_forward'),
			(SELECT count(*) FROM ledger.command_receipt WHERE result_ref=$1 AND operation='issue_quote')`, sale).
			Scan(&c.quotes, &c.submissions, &c.receipts, &c.forwards, &c.issues)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	assertReadOnlyQuote := func(sale string) {
		t.Helper()
		c := counts(sale)
		_, staged := os.Stat(filepath.Join(native.path, "board", "display-request"))
		if c.quotes != 1 || c.issues != 1 || c.submissions != 0 || c.forwards != 0 || !os.IsNotExist(staged) {
			t.Fatal("reading the signed order changed the ledger or contacted the approval host")
		}
	}

	if nativeBoard {
		if pythonDisplay == "" {
			t.Fatal("native board review requires NU54_TRAVEL_DISPLAY_PYTHON")
		}
		if !uc1 {
			prepareNativeTravel(t, native, appEndpoint, base, depositSale, booking.ID, true)
			defer os.Remove(filepath.Join(native.path, "preview.json"))
			defer os.Remove(filepath.Join(native.path, "public-branch"))
		}
		waitNativeTravelStage(t, ctx, native, "deposit-quote")
		assertReadOnlyQuote(depositSale)
	}
	pay := func(sale, amount string) {
		t.Helper()
		selected := base
		selected.Sale = sale
		var item seededOrder
		label := "deposit"
		if amount == balance {
			label = "balance"
		}
		submittedByNative := nativeBoard && sale == depositSale || t05 && amount == balance
		if submittedByNative {
			claim := newPilotCommand(t, db, base.Branch, sale, "checkout.claim", map[string]string{}, false)
			view := newPilotCommand(t, db, base.Branch, sale, "order.status", map[string]string{}, false)
			issue := newPilotCommand(t, db, base.Branch, sale, "quote.issue", nil, true)
			recovered, err := issuePilotOrder(ctx, url, owner, selected, sale, claim, view, issue, asset, amount)
			if err != nil {
				t.Fatalf("could not recover the native application's original signed quote: %v", err)
			}
			item = recovered
		} else {
			item = issueTimedPilot(t, ctx, db, url, selected, owner, owner, amount)
		}
		if len(item.Signed) < 32 || new(big.Int).SetBytes(item.Signed[:32]).Uint64() != 8283 ||
			item.Quote.Args.Beneficiary != base.Quote.Args.Beneficiary ||
			item.Quote.Args.Amount.String() != amount || item.Quote.Args.Mode != 1 {
			t.Fatal("issued merchant relay quote changed the reviewed network, recipient, or amount")
		}
		if sale == depositSale {
			leg(customers[1], booking.ID, "start-balance", 409)
		}
		if inspect != nil && amount == balance {
			inspect(t, "mixed-paid", booking.ID)
		}
		var signature []byte
		if pythonDisplay != "" && (sale == depositSale || submittedByNative) {
			recipient := lower(base.Quote.Args.Beneficiary)
			if !t07 {
				if submittedByNative {
					if label == "balance" {
						cmd := newPilotCommand(t, db, base.Branch, sale, "payment.forward", map[string]string{
							"quoteDigest": rawHex(item.Quote.Digest[:]), "approvalDigest": rawHex(item.Quote.ApprovalHash[:]),
						}, true)
						reply, status, err := pilotCall(ctx, customers[1], url, cmd, map[string]any{"approvalSignature": rawHex(depositApproval)})
						if len(depositApproval) != 65 || err == nil || status != 400 || reply.Error != service.Mismatch || counts(sale).submissions != 0 {
							t.Fatal("the finalized deposit approval authorized a balance submission")
						}
						t.Log("Actual deposit approval was rejected for the separately signed balance")
					}
					waitNativeTravelStage(t, ctx, native, label+"-board-refuse")
					displayDecision(item, amount, recipient, 2)
					waitNativeTravelStage(t, ctx, native, label+"-refused")
				} else {
					env := independentTravelDisplay(t, ctx, pythonDisplay, item, booking, amount, recipient, "", 0)
					pilotBoardDecision(t, ctx, item, board, "management", append(env, "NU54_INSTALL_TEST_REFUSE=1"), true)
				}
				read := leg(customers[1], booking.ID, "legs", 200).Legs
				original := false
				for _, entry := range read {
					original = original || entry.Kind == label && entry.Sale == sale && entry.State == "issued" && entry.Receipt == nil
				}
				if !original || counts(sale).submissions != 0 {
					t.Fatal("refusal did not leave the original travel order unpaid and unsubmitted")
				}
				if label == "deposit" {
					t.Log("Independent travel display verified the original quote, local refusal created no submission or receipt")
				} else {
					t.Log("Independent travel display verified the original balance quote and local refusal left zero submissions")
				}
			}
			if submittedByNative {
				waitNativeTravelStage(t, ctx, native, label+"-board-approve")
				if loseDepositAck && sale == depositSale {
					armAckLoss.Store(true)
				}
				displayDecision(item, amount, recipient, 1)
				waitNativeTravelStage(t, ctx, native, label+"-approved")
				t.Logf("Installed travel app explicitly submitted the separately approved original %s", label)
			} else {
				env := independentTravelDisplay(t, ctx, pythonDisplay, item, booking, amount, recipient, "", 0)
				signature = pilotBoardDecision(t, ctx, item, board, "management", env, false)
			}
		} else {
			signature = pilotBoardApproval(t, ctx, item, board, "management")
		}
		if !submittedByNative {
			cmd := newPilotCommand(t, db, base.Branch, sale, "payment.forward", map[string]string{
				"quoteDigest": rawHex(item.Quote.Digest[:]), "approvalDigest": rawHex(item.Quote.ApprovalHash[:]),
			}, true)
			endpoint := url
			loseOriginalReply := loseDepositAck && sale == depositSale
			if loseOriginalReply {
				armAckLoss.Store(true)
				endpoint = appEndpoint
			}
			reply, _, err := pilotCall(ctx, customers[1], endpoint, cmd, map[string]any{"approvalSignature": rawHex(signature)})
			if loseOriginalReply {
				if err == nil || !lostAck.Load() {
					t.Fatal("original deposit reply was not lost after its durable commit")
				}
			} else if err != nil || reply.ResultRef != sale {
				t.Fatalf("forwarding exact approved sale: %v", err)
			}
		}
		if sale == depositSale || amount == balance {
			// A fresh authenticated read after acceptance can only follow the original.
			// Neither relay admission nor an observed transaction is finalized payment.
			kind := "balance"
			if sale == depositSale {
				kind = "deposit"
			}
			pending := leg(customers[0], booking.ID, "legs", 200).Legs
			hasOriginal := false
			for _, entry := range pending {
				if entry.Kind == kind {
					hasOriginal = entry.Sale == sale && entry.Amount == amount &&
						entry.State == "checking" && entry.Receipt == nil
				}
			}
			if !hasOriginal || leg(customers[1], booking.ID, "start-"+kind, 200).SaleRef != sale {
				t.Fatal("checking payment did not retain exactly its original order")
			}
			if kind == "deposit" {
				leg(customers[1], booking.ID, "start-balance", 409)
				if t05 {
					var stored []byte
					if err := db.QueryRow(ctx, "SELECT approval_cipher FROM ledger.settlement_submission WHERE sale_id=$1", sale).Scan(&stored); err != nil {
						t.Fatal(err)
					}
					var err error
					depositApproval, err = relayCipher.Open(nil, nil, stored, approvalAAD(sale))
					if err != nil || !matchesSigner(item.Quote.ApprovalHash, depositApproval, board.Address()) {
						t.Fatal("stored deposit did not contain the installed board approval")
					}
					if loseDepositAck && !lostAck.Load() {
						t.Fatal("installed deposit reply did not cross the committed response barrier")
					}
					before := counts(sale)
					waitNativeTravelStage(t, ctx, native, "deposit-checking")
					leg(customers[1], booking.ID, "start-balance", 409)
					held := leg(customers[0], booking.ID, "legs", 200).Legs
					if len(held) != 1 || held[0].Sale != sale || held[0].State != "checking" || held[0].Receipt != nil ||
						before.quotes != 1 || before.submissions != 1 || before.forwards != 1 || before.receipts != 0 || counts(sale) != before {
						t.Fatal("deposit restart changed the original ledger before finality")
					}
					t.Log("Installed deposit restart retained one quote, one submission, zero receipts and no balance order")
				}
			} else if native != nil {
				waitNativeTravelStage(t, ctx, native, "balance-checking")
			}
		}
		confirmed(sale)
		waitUntil(t, func() bool {
			var principal string
			return db.QueryRow(ctx, "SELECT principal::text FROM ledger.payment WHERE sale_id=$1", sale).Scan(&principal) == nil && principal == amount
		})
		if loseDepositAck && (sale == depositSale || amount == balance) {
			c := counts(sale)
			if c.quotes != 1 || c.submissions != 1 || c.forwards != 1 || c.receipts != 1 {
				t.Fatal("recovered original payment must have one quote, submission, forward and receipt")
			}
			var tx string
			if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.payment WHERE sale_id=$1", sale).Scan(&tx); err != nil {
				t.Fatal(err)
			}
			receipt, err := chain.TransactionReceipt(ctx, common.HexToHash(tx))
			if err != nil {
				t.Fatal(err)
			}
			events := 0
			for _, log := range receipt.Logs {
				if log.Address != base.Target.Contract {
					continue
				}
				event, err := contract.ParsePaid(*log)
				if err != nil {
					continue
				}
				if event.OrderId != item.Order.OrderID || event.QuoteDigest != item.Quote.Digest || event.Amount.String() != amount || event.Asset != base.Target.Asset || event.Beneficiary != item.Quote.Args.Beneficiary || event.Mode != 1 || event.Payer != board.Address() {
					t.Fatal("Paid event differs from the recovered original")
				}
				events++
			}
			if events != 1 {
				t.Fatal("expected one Paid event")
			}
		}
	}
	pay(depositSale, deposit)
	if uc1 || (t07Host && !t07Full) {
		paid := leg(customers[1], booking.ID, "legs", 200).Legs
		c := counts(depositSale)
		if len(paid) != 1 || paid[0].Kind != "deposit" || paid[0].Sale != depositSale ||
			paid[0].Amount != deposit || paid[0].Receipt == nil || c.submissions != 1 || c.receipts != 1 {
			t.Fatalf("expected one finalized original deposit and receipt: %+v, %+v", paid, c)
		}
		if uc1 {
			proof, err := json.Marshal(map[string]string{"booking": booking.ID, "order": depositSale, "receipt": *paid[0].Receipt, "amount": deposit})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(native.path, "original-receipt.json"), proof, 0o600); err != nil {
				t.Fatal(err)
			}
			waitNativeTravelStage(t, ctx, native, "deposit")
			t.Log("Original installed booking, one 10 TestUSDC deposit and one receipt verified")
		}
		if t07 && !t07Full {
			if t07Host {
				preview := leg(customers[1], booking.ID, "cancel-preview", 200).CancellationPreview
				if preview == nil || !preview.CanCancel || !preview.Deadline.Equal(booking.Cutoff) ||
					len(preview.Legs) != 1 || preview.Legs[0].Sale != depositSale ||
					preview.Legs[0].RefundExpected == nil || *preview.Legs[0].RefundExpected != deposit {
					t.Fatal("T07 customer did not receive the original deposit estimate")
				}
			} else {
				publishCancellationTerms()
				waitNativeTravelStage(t, ctx, native, "cancel-preview")
			}
			var untouched bool
			err := db.QueryRow(ctx, `SELECT b.cancel_requested_at IS NULL AND b.released_at IS NULL AND NOT EXISTS (
				SELECT FROM ledger.refund WHERE sale_id=$2
			) AND EXISTS (SELECT FROM ledger.payment WHERE sale_id=$2 AND reserved=0 AND in_flight=0 AND refunded=0)
			FROM ledger.travel_booking b WHERE b.id=$1`, booking.ID, depositSale).Scan(&untouched)
			if err != nil || !untouched || counts(depositSale) != c {
				t.Fatal("T07 viewing the estimate mutated the original reservation or payment")
			}
			if t07Host {
				if leg(customers[1], booking.ID, "cancel", 200).BookingRef != booking.ID {
					t.Fatal("T07 customer cancellation changed booking identity")
				}
			} else {
				waitNativeTravelStage(t, ctx, native, "cancel-requested")
			}
			pending := leg(customers[0], booking.ID, "legs", 200).Legs
			var refundCount int
			err = db.QueryRow(ctx, "SELECT count(*) FROM ledger.refund WHERE sale_id=$1", depositSale).Scan(&refundCount)
			if err != nil || refundCount != 0 || len(pending) != 1 || pending[0].Sale != depositSale ||
				pending[0].Receipt == nil || *pending[0].Receipt != *paid[0].Receipt ||
				pending[0].RefundState != "waiting-refund" || pending[0].RefundExpected == nil || *pending[0].RefundExpected != deposit {
				t.Fatal("T07 customer cancellation did not retain one original in refund-pending state")
			}
			reserve := map[string]string{"branchId": base.Branch, "saleId": depositSale, "idempotencyKey": fixtureUUID(t, db),
				"amount": deposit, "reason": "merchant_cancel"}
			var refunded map[string]string
			if err := json.Unmarshal(post(finance[0], "/v2/finance/refund-reserve", reserve, 200), &refunded); err != nil || refunded["refundId"] == "" {
				t.Fatal("T07 operator could not reserve the policy refund")
			}
			approval := map[string]string{"branchId": base.Branch, "saleId": depositSale, "refundId": refunded["refundId"]}
			post(finance[0], "/v2/finance/refund-approve", approval, 404)
			post(finance[1], "/v2/finance/refund-approve", approval, 200)
			post(finance[2], "/v2/finance/refund-submit", approval, 200)
			confirmed(refunded["refundId"])
			waitUntil(t, func() bool {
				var state string
				return db.QueryRow(ctx, "SELECT state FROM ledger.refund WHERE id=$1", refunded["refundId"]).Scan(&state) == nil && state == "final"
			})
			if !t07Host {
				waitNativeTravelStage(t, ctx, native, "deposit-refunded")
			}
			final := leg(customers[0], booking.ID, "legs", 200).Legs
			var attempts int
			err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.refund WHERE sale_id=$1),
				(SELECT count(*) FROM ledger.relay_attempt WHERE work_id=$2)`, depositSale, refunded["refundId"]).Scan(&refundCount, &attempts)
			if err != nil || refundCount != 1 || attempts != 1 || counts(depositSale) != c ||
				len(final) != 1 || final[0].Sale != depositSale || final[0].Receipt == nil ||
				*final[0].Receipt != *paid[0].Receipt || final[0].Refunded != deposit || final[0].RefundState != "settled" {
				t.Fatal("T07 final refund did not reconcile once with the original receipt")
			}
			if t07Host {
				t.Log("T07 host customer cancelled original booking; one separate 10 TestUSDC refund verified")
			} else {
				t.Log("T07 installed customer cancelled original booking; one separate 10 TestUSDC refund verified")
			}
		}
		if !t07Full {
			return
		}
	}
	if native != nil {
		if !nativeBoard {
			prepareNativeTravel(t, native, url, base, depositSale, booking.ID, false)
			defer os.Remove(filepath.Join(native.path, "preview.json"))
			defer os.Remove(filepath.Join(native.path, "public-branch"))
		}
		if !uc1 {
			waitNativeTravelStage(t, ctx, native, "deposit")
		}
	}
	verifyVisitIssuer(t, ctx, db, dsn, evidenceDSN, base, depositSale)
	verifyVisitBenefit(t, ctx, db, dsn, base, depositSale)
	balanceSale := leg(customers[1], booking.ID, "start-balance", 200).SaleRef
	if balanceSale == "" || balanceSale == depositSale {
		t.Fatal("balance must be a separate order")
	}
	if t05 {
		waitNativeTravelStage(t, ctx, native, "balance-quote")
		assertReadOnlyQuote(balanceSale)
	}
	preview := func() *commerce.TravelCancellationPreview {
		t.Helper()
		result := leg(customers[1], booking.ID, "cancel-preview", 200).CancellationPreview
		if result == nil || !result.CanCancel || !result.Deadline.Equal(booking.Cutoff) || len(result.Legs) != 2 ||
			result.Legs[0].Sale != depositSale || result.Legs[1].Sale != balanceSale ||
			result.Legs[0].RefundExpected == nil || *result.Legs[0].RefundExpected != depositRefund {
			t.Fatal("cancellation preview did not keep both originals and the current deposit policy")
		}
		return result
	}
	if view := preview(); view.Legs[1].RefundExpected != nil {
		t.Fatal("a balance payment that is not confirmed cannot have a refund estimate")
	}
	pay(balanceSale, balance)
	verifyPaidPreview := func() {
		t.Helper()
		if view := preview(); view.Legs[1].RefundExpected == nil || *view.Legs[1].RefundExpected != balance {
			t.Fatal("the paid balance lost its separate contractual refund estimate")
		}
	}
	if !t07Full || t07Host {
		verifyPaidPreview()
	}
	if t07Full && !t07Host {
		publishCancellationTerms()
		waitNativeTravelStage(t, ctx, native, "cancel-preview")
		verifyPaidPreview()
	}
	var untouched bool
	err = db.QueryRow(ctx, `SELECT b.cancel_requested_at IS NULL AND b.released_at IS NULL AND NOT EXISTS (
		SELECT FROM ledger.refund r WHERE r.sale_id IN ($2,$3)
	) AND (SELECT count(*) FROM ledger.payment p WHERE p.sale_id IN ($2,$3) AND p.reserved=0 AND p.in_flight=0 AND p.refunded=0)=2
	FROM ledger.travel_booking b WHERE b.id=$1`, booking.ID, depositSale, balanceSale).Scan(&untouched)
	if err != nil || !untouched {
		t.Fatal("viewing the cancellation estimate must leave the reservation and original payments untouched")
	}
	initial := leg(customers[1], booking.ID, "legs", 200).Legs
	if len(initial) != 2 || initial[0].Sale != depositSale || initial[1].Sale != balanceSale ||
		initial[0].Receipt == nil || initial[1].Receipt == nil || *initial[0].Receipt == *initial[1].Receipt || initial[0].RefundState != "not-requested" || initial[0].RefundExpected != nil {
		t.Fatalf("booking did not reopen with separate original receipts: %+v", initial)
	}
	if t05 {
		for _, sale := range []string{depositSale, balanceSale} {
			count := counts(sale)
			var payments int
			if db.QueryRow(ctx, "SELECT count(*) FROM ledger.payment WHERE sale_id=$1", sale).Scan(&payments) != nil ||
				payments != 1 || count.quotes != 1 || count.submissions != 1 || count.forwards != 1 || count.receipts != 1 {
				t.Fatal("original travel leg did not settle exactly once")
			}
		}
		if !t07Full {
			waitNativeTravelStage(t, ctx, native, "both-paid")
		}
	}
	if t07Full && !t07Host {
		waitNativeTravelStage(t, ctx, native, "cancel-requested")
	} else if leg(customers[1], booking.ID, "cancel", 200).BookingRef != booking.ID {
		t.Fatal("wrong cancellation binding")
	}
	pending := leg(customers[0], booking.ID, "legs", 200).Legs
	if len(pending) != 2 || pending[0].RefundState != "waiting-refund" || pending[0].RefundExpected == nil ||
		*pending[0].RefundExpected != depositRefund || pending[1].RefundState != "waiting-refund" ||
		pending[1].RefundExpected == nil || *pending[1].RefundExpected != balance {
		t.Fatalf("unexpected cancellation policy projection: %+v", pending)
	}
	reserve := map[string]string{"branchId": base.Branch, "saleId": depositSale,
		"idempotencyKey": fixtureUUID(t, db), "amount": "3000001", "reason": "merchant_cancel"}
	if t07Full {
		reserve["amount"] = "10000001"
		var accepted, replayed time.Time
		readCancellation := func(at *time.Time) error {
			return db.QueryRow(ctx, "SELECT cancel_requested_at FROM ledger.travel_booking WHERE id=$1", booking.ID).Scan(at)
		}
		if readCancellation(&accepted) != nil || leg(customers[0], booking.ID, "cancel", 200).BookingRef != booking.ID ||
			readCancellation(&replayed) != nil || !accepted.Equal(replayed) {
			t.Fatal("T07 cancellation replay did not recover the same accepted request")
		}
		var refundCount int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.refund WHERE sale_id IN ($1,$2)", depositSale, balanceSale).Scan(&refundCount); err != nil || refundCount != 0 {
			t.Fatal("T07 customer cancellation created an operations refund")
		}
	}
	post(finance[0], "/v2/finance/refund-reserve", reserve, 409)
	reserve["amount"] = depositRefund
	var refunded map[string]string
	if err := json.Unmarshal(post(finance[0], "/v2/finance/refund-reserve", reserve, 200), &refunded); err != nil {
		t.Fatal(err)
	}
	approval := map[string]string{"branchId": base.Branch, "saleId": depositSale, "refundId": refunded["refundId"]}
	post(finance[0], "/v2/finance/refund-approve", approval, 404)
	post(finance[1], "/v2/finance/refund-approve", approval, 200)
	post(finance[2], "/v2/finance/refund-submit", approval, 200)
	confirmed(refunded["refundId"])
	waitUntil(t, func() bool {
		var state string
		return db.QueryRow(ctx, "SELECT state FROM ledger.refund WHERE id=$1", refunded["refundId"]).Scan(&state) == nil && state == "final"
	})
	partlyRefunded := leg(customers[0], booking.ID, "legs", 200).Legs
	if len(partlyRefunded) != 2 || partlyRefunded[0].Receipt == nil || partlyRefunded[1].Receipt == nil ||
		*partlyRefunded[1].Receipt != *initial[1].Receipt || partlyRefunded[0].Refunded != depositRefund ||
		*partlyRefunded[0].Receipt != *initial[0].Receipt || partlyRefunded[1].Refunded != "0" ||
		partlyRefunded[0].RefundState != "settled" || partlyRefunded[1].RefundState != "waiting-refund" {
		t.Fatalf("partial refund changed or misbound the original receipt: %+v", partlyRefunded)
	}
	if native != nil {
		waitNativeTravelStage(t, ctx, native, "partial")
	}
	reserve = map[string]string{"branchId": base.Branch, "saleId": balanceSale,
		"idempotencyKey": fixtureUUID(t, db), "amount": balance, "reason": "merchant_cancel"}
	if err := json.Unmarshal(post(finance[0], "/v2/finance/refund-reserve", reserve, 200), &refunded); err != nil {
		t.Fatal(err)
	}
	approval = map[string]string{"branchId": base.Branch, "saleId": balanceSale, "refundId": refunded["refundId"]}
	post(finance[1], "/v2/finance/refund-approve", approval, 200)
	post(finance[2], "/v2/finance/refund-submit", approval, 200)
	confirmed(refunded["refundId"])
	waitUntil(t, func() bool {
		var state string
		return db.QueryRow(ctx, "SELECT state FROM ledger.refund WHERE id=$1", refunded["refundId"]).Scan(&state) == nil && state == "final"
	})
	final := leg(customers[1], booking.ID, "legs", 200).Legs
	if len(final) != 2 || final[0].Receipt == nil || *final[0].Receipt != *initial[0].Receipt ||
		final[1].Receipt == nil || final[1].Refunded != balance ||
		*final[1].Receipt != *initial[1].Receipt || final[0].Refunded != depositRefund ||
		final[0].RefundState != "settled" || final[1].RefundState != "settled" {
		t.Fatalf("paid refund did not reconcile against both original legs: %+v", final)
	}
	if native != nil {
		waitNativeTravelStage(t, ctx, native, "refunded")
	}
	var released, hasPickup bool
	if err := db.QueryRow(ctx, `SELECT released_at IS NOT NULL,
		EXISTS (SELECT FROM ledger.pickup WHERE sale_id IN ($2,$3)) FROM ledger.travel_booking WHERE id=$1`,
		booking.ID, depositSale, balanceSale).Scan(&released, &hasPickup); err != nil || !released || hasPickup {
		t.Fatalf("final cancellation did not release seat, or booking entered pickup: %v", err)
	}
	if t07Full {
		for _, sale := range []string{depositSale, balanceSale} {
			var refunds, finalized, attempts int
			err := db.QueryRow(ctx, `SELECT count(*), count(*) FILTER(WHERE state='final'),
				(SELECT count(*) FROM ledger.relay_attempt a JOIN ledger.refund r ON a.work_id=r.id WHERE r.sale_id=$1)
				FROM ledger.refund WHERE sale_id=$1`, sale).Scan(&refunds, &finalized, &attempts)
			if err != nil || refunds != 1 || finalized != 1 || attempts != 1 || counts(sale).receipts != 1 {
				t.Fatal("T07 did not settle exactly one refund for each preserved original")
			}
		}
		t.Log("T07 full paid booking: one 10 TestUSDC and one 20 TestUSDC refund verified")
		return
	}

	full := hold(fullSlot, "0")
	fullSale := leg(customers[0], full.ID, "start-deposit", 200).SaleRef
	pay(fullSale, deposit)
	leg(customers[1], full.ID, "start-balance", 404)
	if entries := leg(customers[0], full.ID, "legs", 200).Legs; len(entries) != 1 || entries[0].Kind != "deposit" {
		t.Fatalf("full deposit produced a second payment: %+v", entries)
	}
	fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,'travel-pickup','plain','Pickup fixture',$2,$3)", base.Branch, asset, pickupAmount)
	fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,'travel-pickup',1)", base.Branch)
	request, cart := pilotReserveCart(t, db, base.Branch, asset, []commerce.Item{{SKU: "travel-pickup", Option: "plain", Qty: 1}})
	created, _, err := pilotCall(ctx, customers[0], url, request, map[string]any{"cart": cart})
	if err != nil || created.ResultRef == "" {
		t.Fatalf("seatless pickup reserve: %v", err)
	}
	pay(created.ResultRef, pickupAmount)
	view := newPilotCommand(t, db, base.Branch, created.ResultRef, "order.status", map[string]string{}, false)
	resumed, _, err := pilotCall(ctx, customers[1], url, view, nil)
	var booked bool
	if err != nil || resumed.Order == nil || resumed.Order.PickupNo == nil || resumed.Order.ReceiptRef == nil ||
		db.QueryRow(ctx, "SELECT EXISTS (SELECT FROM ledger.travel_booking_leg WHERE sale_id=$1)", created.ResultRef).Scan(&booked) != nil || booked {
		t.Fatalf("seatless pickup did not reopen with pickup and receipt: %#v %v", resumed.Order, err)
	}
	if inspect == nil {
		return
	}
	abandoned := hold(slot, balance)
	abandonedSale := leg(customers[0], abandoned.ID, "start-deposit", 200).SaleRef
	base.Sale = abandonedSale
	fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=10 WHERE branch_id=$1", base.Branch)
	quote := issueTimedPilot(t, ctx, db, url, base, owner, owner, deposit)
	fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=120 WHERE branch_id=$1", base.Branch)
	leg(customers[0], abandoned.ID, "cancel", 200)
	select {
	case <-time.After(time.Until(quote.Order.Expires.Add(250 * time.Millisecond))):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	leg(customers[1], abandoned.ID, "legs", 200)
	inspect(t, "expired-held", abandoned.ID)
	head, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if head.Time < uint64(quote.Order.Expires.Unix()+1) {
		forceTime(t, ctx, chain, quote.Order.Expires.Unix()+1)
	}
	mine(t, ctx, chain, 8)
	waitUntil(t, func() bool {
		var reached bool
		return db.QueryRow(ctx, `SELECT b.block_time>=$2 FROM chainview.read_target r
          JOIN chainview.generation g ON g.id=r.generation_id
          JOIN chainview.block b ON b.generation_id=g.id AND b.block_hash=g.finalized_hash
          WHERE r.stream_id=$1`, base.Target.Stream, quote.Order.Expires).Scan(&reached) == nil && reached
	})
	proveUnpaid(t, quote)
	waitUntil(t, func() bool {
		var known bool
		return db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM ledger.sale_resolution WHERE sale_id=$1)", abandonedSale).Scan(&known) == nil && known
	})
	leg(customers[1], abandoned.ID, "legs", 200)
	inspect(t, "confirmed-release", abandoned.ID)
	var clean bool
	if err := db.QueryRow(ctx, `SELECT
        (SELECT count(*)=1 FROM ledger.sale_resolution WHERE sale_id=$1)
        AND NOT EXISTS(SELECT FROM ledger.payment WHERE sale_id=$1)
        AND NOT EXISTS(SELECT FROM ledger.relay_attempt WHERE work_id=$1)
        AND EXISTS(SELECT FROM ledger.travel_booking WHERE id=$2 AND released_at IS NOT NULL)`, abandonedSale, abandoned.ID).Scan(&clean); err != nil || !clean {
		t.Fatalf("resolution did not release the same unpaid booking: %v", err)
	}

}

func verifyVisitIssuer(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn, evidenceDSN string, base seededOrder, sale string) {
	t.Helper()
	binary := os.Getenv("P05_SETTLEMENT_TEST_VISIT_ISSUER")
	if binary == "" {
		t.Fatal("normal visit issuer executable missing")
	}
	_, issuerDSN := testLogin(t, db, dsn, "nu54_p05_visit_issuer")
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	defer clear(seed)
	signer := ed25519.NewKeyFromSeed(seed)
	defer clear(signer)
	path := t.TempDir()
	key := filepath.Join(path, "issuer")
	if err := os.WriteFile(key, []byte(base64.StdEncoding.EncodeToString(seed)), 0o600); err != nil {
		t.Fatal(err)
	}
	enrollment := filepath.Join(path, "enrolled.json")
	subject, proof := fixtureUUID(t, db), fixtureUUID(t, db)
	fields := map[string]any{"issuer": "fixture-p05", "tenant": base.Tenant, "branch": base.Branch, "customer": base.Actor,
		"subject": subject, "place": "travel-merchant", "proof": proof, "sale": sale, "expiresAt": time.Now().Add(time.Minute)}
	issue := func() ([]byte, error) {
		t.Helper()
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(enrollment, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, binary, "-enrollment", enrollment, "-signer", key)
		command.Env = append(os.Environ(), "P05_VISIT_DATABASE_URL="+issuerDSN, "P07_VISIT_EVIDENCE_DATABASE_URL="+evidenceDSN)
		return command.CombinedOutput()
	}
	out, err := issue()
	if err != nil {
		t.Fatalf("independently reconciled visit unavailable: %v %s", err, out)
	}
	var envelope map[string]string
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.Strict().DecodeString(envelope["document"])
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(envelope["signature"])
	if err != nil || !ed25519.Verify(signer.Public().(ed25519.PublicKey), append([]byte("NU54-visit-assertion-v1\x00"), body...), signature) {
		t.Fatal("bad visit signature")
	}
	var assertion map[string]any
	if err := json.Unmarshal(body, &assertion); err != nil {
		t.Fatal(err)
	}
	if len(assertion) != 7 || assertion["subject"] != subject || assertion["proof"] != proof || assertion["state"] != "settled" || assertion["place"] != "travel-merchant" {
		t.Fatalf("issuer disclosed excess data or misbound purpose: %v", assertion)
	}
	fields["customer"] = fixtureUUID(t, db)
	if out, err := issue(); err == nil {
		t.Fatalf("foreign customer acquired visit proof: %s", out)
	}
}

func verifyVisitBenefit(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn string, base seededOrder, sale string) {
	t.Helper()
	binary := os.Getenv("P05_SETTLEMENT_TEST_VISIT_BENEFIT")
	if binary == "" {
		t.Fatal("normal visit benefit executable missing")
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	authority := filepath.Join(t.TempDir(), "benefit-authority")
	if err := os.WriteFile(authority, []byte(base64.StdEncoding.EncodeToString(pub)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, restricted := testLogin(t, db, dsn, "nu54_p05_benefit_worker")
	live, synthetic := fixtureUUID(t, db), fixtureUUID(t, db)
	visit := fixtureUUID(t, db)
	fixtureExec(t, db, `INSERT INTO ledger.visit_benefit_enrollment VALUES
	 ($1,$2,$3,'fixture-p09',$5,'live'),($1,$2,$4,'fixture-p09',$5,'synthetic')`, base.Branch, sale, live, synthetic, visit)
	apply := func(campaign, stage string, revision int, expected bool, pass bool) {
		t.Helper()
		packet := map[string]any{"issuer": "fixture-p09", "proof": visit, "classification": stage, "revision": revision,
			"expiresAt": time.Now().UTC().Add(15 * time.Second)}
		raw, err := json.Marshal(packet)
		if err != nil {
			t.Fatal(err)
		}
		sig := ed25519.Sign(key, append([]byte("NU54-visit-benefit-v1\x00"), raw...))
		env, err := json.Marshal(map[string]string{"document": base64.StdEncoding.EncodeToString(raw), "signature": base64.StdEncoding.EncodeToString(sig)})
		if err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, binary, "-authority", authority, "-campaign", campaign)
		command.Env = append(os.Environ(), "P05_BENEFIT_DATABASE_URL="+restricted)
		command.Stdin = bytes.NewReader(env)
		out, err := command.CombinedOutput()
		if !pass {
			if err == nil {
				t.Fatalf("old authorization was accepted: %s", out)
			}
			return
		}
		var answer map[string]bool
		if err != nil || json.Unmarshal(out, &answer) != nil || answer["effect"] != expected {
			t.Fatalf("wrong benefit effect: %s %v", out, err)
		}
	}
	apply(synthetic, "eligible", 1, false, true)
	apply(live, "eligible", 1, true, true)
	apply(live, "eligible", 1, false, true)
	apply(live, "adjusted", 2, true, true)
	apply(live, "eligible", 1, false, false)
	var grants, net, syntheticEffects int
	if err := db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE campaign=$1),coalesce(sum(delta) FILTER(WHERE campaign=$1),0),
	    count(*) FILTER(WHERE campaign=$2) FROM ledger.visit_benefit_effect WHERE proof=$3`, live, synthetic, visit).
		Scan(&grants, &net, &syntheticEffects); err != nil || grants != 2 || net != 0 || syntheticEffects != 0 {
		t.Fatalf("grant/reversal must be independent and exactly once: %d/%d/%d %v", grants, net, syntheticEffects, err)
	}
}

func independentTravelDisplay(t *testing.T, ctx context.Context, python string, item seededOrder,
	booking commerce.TravelBooking, amount, recipient, boardDir string, button byte) []string {
	t.Helper()
	dir := t.TempDir()
	expected := map[string]any{
		"presentationKey": rawHex(pilotPresentationKey()), "branch": item.Branch, "customer": item.Actor,
		"sale": item.Sale, "recipient": recipient, "issuer": lower(fixtureSigner(101).Address()),
		"chain": booking.ChainID, "asset": booking.Asset, "amount": amount,
	}
	projection := map[string]any{
		"saleRef": item.Sale, "branchRef": item.Branch, "quoteDigest": rawHex(item.Quote.Digest[:]),
		"orderKey": rawHex(item.Order.OrderID[:]), "expiresAt": item.Order.Expires.Unix(),
		"signedQuote": rawHex(item.Signed), "presentation": item.Presentation,
	}
	for name, data := range map[string]any{"expected.json": expected, "projection.json": projection} {
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var challenge []byte
	if boardDir == "" {
		var err error
		challenge, err = exec.CommandContext(ctx, os.Getenv("NU54_P01_INSTALL_TEST"), "--display-challenge").Output()
		if err != nil {
			t.Fatalf("P01 host did not create an independent display challenge: %v", err)
		}
	} else {
		waitUntil(t, func() bool {
			_, err := os.Stat(filepath.Join(boardDir, "display-request"))
			return err == nil
		})
		request, err := os.ReadFile(filepath.Join(boardDir, "display-request"))
		if err != nil || !bytes.Equal(request, item.Signed) {
			t.Fatal("installed board transport staged a quote different from the authenticated P05 original")
		}
		challenge, err = os.ReadFile(filepath.Join(boardDir, "display-challenge"))
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(challenge) != 32 {
		t.Fatal("P01 host did not create a 32-byte display challenge")
	}
	if err := os.WriteFile(filepath.Join(dir, "challenge"), challenge, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"../../test/settlement/travel-display.py", dir}
	if boardDir != "" {
		args = append(args, filepath.Join(filepath.Dir(boardDir), "display", "key.pem"))
	}
	command := exec.CommandContext(ctx, python, args...)
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("independent travel display failed: %v: %s", err, out)
	}
	if boardDir != "" {
		proof, err := os.ReadFile(filepath.Join(dir, "proof"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(boardDir, "display-proof"), proof, 0o600); err != nil {
			t.Fatal(err)
		}
		decision := filepath.Join(boardDir, "display-decision")
		if err := os.WriteFile(decision+".tmp", []byte{button}, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(decision+".tmp", decision); err != nil {
			t.Fatal(err)
		}
		return nil
	}
	return []string{
		"NU54_INSTALL_DISPLAY_KEY=" + filepath.Join(dir, "display-public.pem"),
		"NU54_INSTALL_DISPLAY_PROOF=" + filepath.Join(dir, "proof"),
		"NU54_INSTALL_DISPLAY_CHALLENGE=" + filepath.Join(dir, "challenge"),
	}
}

// The synthetic mobile run derives read-only checkpoints from real chain and
// ledger transitions. Without the opted-in fixture the settlement test never pauses.
func prepareNativeTravel(t *testing.T, lab *nativeSubscriptionLab, url string, order seededOrder, sale, booking string, display bool) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(lab.path, "public-branch"), []byte(order.Branch), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]string{"saleRef": sale, "bookingRef": booking}
	if display {
		metadata["travelRecipient"] = lower(order.Quote.Args.Beneficiary)
	}
	data := lab.enrollmentDocument(t, url, order, metadata)
	pending := filepath.Join(lab.path, "preview.tmp")
	if err := os.WriteFile(pending, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(pending, filepath.Join(lab.path, "preview.json")); err != nil {
		t.Fatal(err)
	}
}

func waitNativeTravelStage(t *testing.T, ctx context.Context, lab *nativeSubscriptionLab, stage string) {
	t.Helper()
	deadline := time.NewTimer(120 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	if err := os.WriteFile(filepath.Join(lab.path, "travel-stage"), []byte(stage), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatalf("native travel did not report %s", stage)
		case <-poll.C:
			if _, err := os.Stat(filepath.Join(lab.path, "ui-aborted")); err == nil {
				t.Fatal("native travel UI did not complete")
			}
			if _, err := os.Stat(filepath.Join(lab.path, "travel-"+stage)); err == nil {
				return
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
}
