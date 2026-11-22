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
	"strconv"
	"sync"
	"testing"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/quotev2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testSubscriptionProcess(t *testing.T, ctx context.Context, db *pgxpool.Pool, url string, base seededOrder,
	devices [2]*http.Client, chain *ethclient.Client, token *testusdc.Testusdc, contract *v2.Settlementv2, native *nativeSubscriptionLab, proveUnpaid func(*testing.T, seededOrder)) {
	t.Helper()
	flow := os.Getenv("NU54_SUBSCRIPTION_NATIVE_FLOW")
	if flow != "" && (native == nil || (flow != "terms" && flow != "lifecycle" && flow != "review")) {
		t.Fatal("subscription native flow requires an enrolled test fixture and a known mode")
	}
	if flow == "terms" {
		testNativeSubscriptionTerms(t, ctx, db, url, base, devices[0], native)
		return
	}
	board := fixtureSigner(102)
	offer, newOffer, series := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	digest, newDigest := sha256.Sum256([]byte("weekly terms fixture")), sha256.Sum256([]byte("revised weekly terms fixture"))
	publish := func(id, version string, terms [32]byte, amount int) {
		fixtureExec(t, db, `INSERT INTO ledger.subscription_offer
			(id,branch_id,series_ref,label,terms_version,terms_digest,cadence,timezone,due_days,asset_ref,amount,fulfillment_kind)
			VALUES($1,$2,$3,'Private pilot subscription',$4,$5,'weekly','Asia/Seoul',7,$6,$7,'digital')`,
			id, base.Branch, series, version, terms[:], lower(base.Target.Asset), amount)
	}
	principal := int64(30)
	if flow == "review" {
		principal = 4_000_000
	}
	publish(offer, "v1", digest, int(principal))
	type replyError struct {
		reply pilotReply
		err   error
	}
	invoices := func(client *http.Client) []commerce.Invoice {
		req := newPilotCommand(t, db, base.Branch, base.Actor, "invoice.list", map[string]string{}, false)
		reply, _, err := pilotCall(ctx, client, url, req, nil)
		if err != nil {
			t.Fatal(err)
		}
		return reply.Invoices
	}
	var both [2]replyError
	var phones sync.WaitGroup
	for device, client := range devices {
		req := newPilotCommand(t, db, base.Branch, offer, "subscription.accept", map[string]string{"termsDigest": rawHex(digest[:])}, true)
		phones.Go(func() { both[device].reply, _, both[device].err = pilotCall(ctx, client, url, req, nil) })
	}
	phones.Wait()
	schedule := both[0].reply.ResultRef
	if schedule == "" || both[1].reply.ResultRef != schedule || both[0].err != nil || both[1].err != nil {
		t.Fatalf("two device grants did not converge on one subscription: %s / %s, errors %v / %v",
			schedule, both[1].reply.ResultRef, both[0].err, both[1].err)
	}
	var invoice commerce.Invoice
	waitUntil(t, func() bool {
		rows := invoices(devices[0])
		if len(rows) != 1 {
			return false
		}
		invoice = rows[0]
		return true
	})
	if rows := invoices(devices[1]); len(rows) != 1 || rows[0] != invoice || invoice.Subscription != schedule || invoice.Amount != strconv.FormatInt(principal, 10) {
		t.Fatalf("same due cycle was not returned on both phones: %#v, %#v", invoice, rows)
	}
	if native != nil && flow != "review" {
		native.verifyPreview(t, ctx, db, url, base, invoice)
	}
	apply := func(client *http.Client, id, operation string, fields map[string]string) string {
		req := newPilotCommand(t, db, base.Branch, id, operation, fields, true)
		reply, _, err := pilotCall(ctx, client, url, req, nil)
		if err != nil {
			t.Fatal(err)
		}
		return reply.ResultRef
	}
	for version, op := range []string{"subscription.pause", "subscription.resume"} {
		apply(devices[version], schedule, op, map[string]string{"expectedRevision": strconv.Itoa(version)})
	}
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
		r, err := bind.WaitMined(ctx, chain, tx)
		if err != nil || r.Status != types.ReceiptStatusSuccessful {
			t.Fatalf("subscription escrow fixture: %v", err)
		}
	}
	amount := big.NewInt(principal)
	before, err := token.BalanceOf(nil, base.Quote.Args.Beneficiary)
	if err != nil {
		t.Fatal(err)
	}
	confirm(token.Mint(operator, operator.From, amount))
	confirm(token.Approve(operator, base.Target.Contract, amount))
	confirm(contract.DepositEscrow(operator, board.Address(), 1, amount))
	base.Sale = invoice.Sale
	owner := pilotIdentity{actor: base.Actor, board: board, client: devices[0]}
	if flow == "review" {
		testNativeSubscriptionApproval(t, ctx, db, url, base, owner, native, invoice, chain, token, before, amount)
		return
	}
	item := issueTimedPilot(t, ctx, db, url, base, owner, owner, amount.String())
	signature := pilotBoardApproval(t, ctx, item, board, "management")
	// Only the beneficiary address differs from the archived quote. The original
	// board approval must not authorize this payment in the actual P05 endpoint.
	changed := bytes.Clone(item.Signed[:len(item.Signed)-65])
	const lastBeneficiaryByte = 383
	changed[lastBeneficiaryByte] ^= 1
	changedDigest, err := quotev2.Digest(changed)
	if err != nil || changedDigest == item.Quote.Digest {
		t.Fatalf("changed beneficiary did not produce a distinct quote digest: %v", err)
	}
	request := newPilotCommand(t, db, base.Branch, invoice.Sale, "payment.forward", map[string]string{
		"quoteDigest": rawHex(changedDigest[:]), "approvalDigest": rawHex(item.Quote.ApprovalHash[:]),
	}, true)
	refused, code, err := pilotCall(ctx, devices[0], url, request, map[string]any{"approvalSignature": rawHex(signature)})
	if err == nil || code != http.StatusBadRequest || refused.Error != service.Mismatch {
		t.Fatalf("changed beneficiary forward was not rejected: HTTP %d %s %v", code, refused.Error, err)
	}
	var attemptsBefore, paymentsBefore int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$1)`, invoice.Sale).Scan(&attemptsBefore, &paymentsBefore); err != nil ||
		attemptsBefore != 0 || paymentsBefore != 0 {
		t.Fatalf("wrong recipient changed the original invoice: %d submissions, %d payments: %v", attemptsBefore, paymentsBefore, err)
	}
	t.Log("recipient-only quote substitution rejected; zero P05 submissions or payments")

	for _, device := range devices {
		req := newPilotCommand(t, db, base.Branch, invoice.Sale, "payment.forward", map[string]string{
			"quoteDigest": rawHex(item.Quote.Digest[:]), "approvalDigest": rawHex(item.Quote.ApprovalHash[:]),
		}, true)
		reply, _, err := pilotCall(ctx, device, url, req, map[string]any{"approvalSignature": rawHex(signature)})
		if err != nil || reply.ResultRef != invoice.Sale {
			t.Fatalf("same signed cycle forwarded twice: %v", err)
		}
	}
	var txHash string
	var receipt *types.Receipt
	waitUntil(t, func() bool {
		err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", invoice.Sale).Scan(&txHash)
		if err != nil {
			return false
		}
		receipt, err = chain.TransactionReceipt(ctx, common.HexToHash(txHash))
		return err == nil
	})
	header, err := chain.HeaderByHash(ctx, receipt.BlockHash)
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful || header.Time >= uint64(item.Order.Expires.Unix()) {
		t.Fatalf("expected successful inclusion before expiry: %v", err)
	}
	var paid int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.payment WHERE sale_id=$1", invoice.Sale).Scan(&paid); err != nil || paid != 0 {
		t.Fatalf("included invoice finalized prematurely: %d %v", paid, err)
	}
	if flow == "lifecycle" {
		native.publish(t, "included", invoice.Sale)
		native.await(t, ctx, "included-seen")
	}
	publish(newOffer, "v2", newDigest, 40)
	fixtureExec(t, db, "UPDATE ledger.subscription_offer SET enabled=false WHERE id=$1", offer)
	replacement := apply(devices[1], schedule, "subscription.reconsent", map[string]string{
		"expectedRevision": "2", "offerRef": newOffer, "termsDigest": rawHex(newDigest[:]),
	})
	if replacement == "" || replacement == schedule {
		t.Fatal("new consent did not issue a new schedule")
	}
	apply(devices[0], replacement, "subscription.cancel", map[string]string{"expectedRevision": "0"})
	forceTime(t, ctx, chain, item.Order.Expires.Unix()+1)
	waitUntil(t, func() bool {
		return db.QueryRow(ctx, "SELECT count(*) FROM ledger.payment WHERE sale_id=$1 AND tx_hash=$2 AND principal=30", invoice.Sale, txHash).Scan(&paid) == nil && paid == 1
	})
	after, err := token.BalanceOf(nil, item.Quote.Args.Beneficiary)
	if err != nil || new(big.Int).Sub(after, before).Cmp(amount) != 0 {
		t.Fatalf("beneficiary received the wrong amount: %v", err)
	}
	var attempts, journals int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.relay_attempt WHERE work_id=$1),
		(SELECT count(*) FROM ledger.journal WHERE sale_id=$1)`, invoice.Sale).Scan(&attempts, &journals); err != nil || attempts != 1 || journals != 1 {
		t.Fatalf("duplicate subscription payment or journal: %d/%d %v", attempts, journals, err)
	}
	var entitlement, pickup, kitchen int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.subscription_entitlement WHERE invoice_id=$1),
		(SELECT count(*) FROM ledger.pickup WHERE sale_id=$2),
		(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$2)`, invoice.ID, invoice.Sale).Scan(&entitlement, &pickup, &kitchen); err != nil ||
		entitlement != 1 || pickup != 0 || kitchen != 0 {
		t.Fatalf("digital contract must issue one entitlement and no kitchen work: %d/%d/%d, %v", entitlement, pickup, kitchen, err)
	}
	for _, device := range devices {
		rows := invoices(device)
		var matched bool
		for _, row := range rows {
			if row.ID == invoice.ID {
				matched = row.State == "paid" && row.Amount == "30"
			}
		}
		if !matched {
			t.Fatalf("phone lost the original paid invoice after reconsent: %#v", rows)
		}
	}
	if flow == "lifecycle" {
		var receiptRef string
		if err := db.QueryRow(ctx, "SELECT receipt_ref::text FROM ledger.subscription_entitlement WHERE invoice_id=$1", invoice.ID).Scan(&receiptRef); err != nil {
			t.Fatal(err)
		}
		native.publish(t, "receipt-ref", receiptRef)
		native.publish(t, "confirmed", invoice.Sale)
		native.await(t, ctx, "confirmed-seen")
	}
	series = fixtureUUID(t, db)
	skipOffer := fixtureUUID(t, db)
	publish(skipOffer, "skippable", digest, 30)
	skipSchedule := apply(devices[0], skipOffer, "subscription.accept", map[string]string{"termsDigest": rawHex(digest[:])})
	var skipped commerce.Invoice
	waitUntil(t, func() bool {
		for _, row := range invoices(devices[1]) {
			if row.Subscription == skipSchedule {
				skipped = row
				return row.State == "preview"
			}
		}
		return false
	})
	if flow == "lifecycle" {
		native.publish(t, "skip-open", skipped.Sale)
		native.await(t, ctx, "skip-open-seen")
	} else {
		skip := newPilotCommand(t, db, base.Branch, skipped.ID, "invoice.skip", map[string]string{}, true)
		for _, device := range devices {
			reply, _, err := pilotCall(ctx, device, url, skip, nil)
			if err != nil || reply.ResultRef != skipped.ID {
				t.Fatalf("two phone skip did not preserve invoice reference: %v", err)
			}
		}
	}
	foundClosed := false
	for _, row := range invoices(devices[0]) {
		if row.ID == skipped.ID && row.State == "closed" && row.Stage == "" {
			foundClosed = true
		}
	}
	if !foundClosed {
		t.Fatal("skipped invoice remains open on the original phone")
	}
	checkExposure := func(id string, expectedClaimsAndQuotes int) {
		t.Helper()
		var claims, quotes, submissions, payments int
		err := db.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM ledger.checkout_claim WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.quote_work WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.payment WHERE sale_id=$1)`, id).Scan(&claims, &quotes, &submissions, &payments)
		if err != nil || claims != expectedClaimsAndQuotes || quotes != expectedClaimsAndQuotes || submissions != 0 || payments != 0 {
			t.Fatalf("unexpected claims/quotes/submissions/payments: %d/%d/%d/%d: %v", claims, quotes, submissions, payments, err)
		}
	}
	checkExposure(skipped.Sale, 0)

	series = fixtureUUID(t, db)
	unpaidOffer := fixtureUUID(t, db)
	publish(unpaidOffer, "unpaid", digest, 30)
	unpaidSchedule := apply(devices[0], unpaidOffer, "subscription.accept", map[string]string{"termsDigest": rawHex(digest[:])})
	var unpaid commerce.Invoice
	waitUntil(t, func() bool {
		for _, row := range invoices(devices[0]) {
			if row.Subscription == unpaidSchedule {
				unpaid = row
				return row.State == "preview"
			}
		}
		return false
	})
	base.Sale = unpaid.Sale
	fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=10 WHERE branch_id=$1", base.Branch)
	unspent := issueTimedPilot(t, ctx, db, url, base, owner, owner, amount.String())
	fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=120 WHERE branch_id=$1", base.Branch)
	select {
	case <-time.After(time.Until(unspent.Order.Expires.Add(250 * time.Millisecond))):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	read := func(stage string) {
		t.Helper()
		for _, row := range invoices(devices[1]) {
			if row.ID == unpaid.ID && row.Stage == stage && row.Due.Equal(unpaid.Due) {
				req := newPilotCommand(t, db, base.Branch, unpaid.Sale, "order.status", map[string]string{}, false)
				reply, _, err := pilotCall(ctx, devices[1], url, req, nil)
				if err == nil && reply.Order != nil && reply.Order.Subscription != nil &&
					reply.Order.Subscription.Stage == stage && reply.Order.Subscription.InvoiceRef == unpaid.ID &&
					reply.Order.Subscription.QuoteRef != "" && reply.Order.ReceiptRef == nil &&
					reply.Order.Subscription.QuoteExpiresAt != nil &&
					reply.Order.Subscription.QuoteExpiresAt.Equal(unspent.Order.Expires) {
					return
				}
			}
		}
		t.Fatalf("original signed unpaid invoice did not expose %s", stage)
	}
	read("reconcile")
	if flow == "lifecycle" {
		native.publish(t, "reconcile", unpaid.Sale)
		native.await(t, ctx, "reconcile-seen")
	}
	checkExposure(unpaid.Sale, 1)

	head, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if head.Time < uint64(unspent.Order.Expires.Unix()+1) {
		forceTime(t, ctx, chain, unspent.Order.Expires.Unix()+1)
	}
	mine(t, ctx, chain, 8)
	waitUntil(t, func() bool {
		var reached bool
		return db.QueryRow(ctx, `SELECT b.block_time >= $2 FROM chainview.read_target r
			JOIN chainview.generation g ON g.id=r.generation_id
			JOIN chainview.block b ON b.generation_id=g.id AND b.block_hash=g.finalized_hash
			WHERE r.stream_id=$1`, base.Target.Stream, unspent.Order.Expires).Scan(&reached) == nil && reached
	})
	proveUnpaid(t, unspent)
	waitUntil(t, func() bool {
		var resolved bool
		return db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM ledger.sale_resolution WHERE sale_id=$1)", unpaid.Sale).Scan(&resolved) == nil && resolved
	})
	read("expired")
	if flow == "lifecycle" {
		native.publish(t, "expired", unpaid.Sale)
		native.await(t, ctx, "expired-seen")
	}
	checkExposure(unpaid.Sale, 1)
	if flow == "lifecycle" {
		native.publish(t, "lifecycle-report.json", `{"previewReopened":true,"includedAwaitingFinality":true,"originalDigitalReceipt":true,"originalPayments":1,"originalJournals":1,"skippedClaims":0,"skippedQuotes":0,"skippedSubmissions":0,"skippedPayments":0,"signedReconcileRead":true,"signedAbsenceFinalized":true,"newClaimsAfterDeadline":0,"unpaidSubmissions":0}`)
	}
	t.Log("subscription: two enrolled phones, one invoice and one board-approved chain payment, late finality retained after reconsent and cancellation")
}

// The separate display obtains the quote from its own registered TLS client.
func testNativeSubscriptionApproval(t *testing.T, ctx context.Context, db *pgxpool.Pool, url string, base seededOrder,
	owner pilotIdentity, lab *nativeSubscriptionLab, invoice commerce.Invoice, chain *ethclient.Client, token *testusdc.Testusdc,
	balanceBefore, amount *big.Int) {
	t.Helper()
	lab.publish(t, "enrollment.json", string(lab.enrollmentDocument(t, url, base, invoice)))
	status := func() (pilotReply, error) {
		read := newPilotCommand(t, db, base.Branch, base.Sale, "order.status", map[string]string{}, false)
		reply, _, err := pilotCall(ctx, owner.client, url, read, nil)
		return reply, err
	}
	lab.await(t, ctx, "notification-opened")
	verify := func(state string, submissions, payments int) {
		t.Helper()
		var seen string
		var accepted, paid int
		err := db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=s.id),
			(SELECT count(*) FROM ledger.payment WHERE sale_id=s.id) FROM ledger.sale s WHERE id=$1`, base.Sale).Scan(&seen, &accepted, &paid)
		if err != nil || seen != state || accepted != submissions || paid != payments {
			t.Fatalf("unexpected subscription effects: state=%s submissions=%d payments=%d: %v", seen, accepted, paid, err)
		}
	}
	verify("preview", 0, 0)
	lab.publish(t, "notification-no-payment", invoice.ID)
	lab.await(t, ctx, "quote-reviewed")
	verify("issued", 0, 0)
	view, err := status()
	if err != nil || view.Order == nil || view.Order.Subscription == nil || view.Order.Subscription.InvoiceRef != invoice.ID || view.Order.Subscription.Cycle != invoice.Cycle {
		t.Fatalf("original authenticated invoice missing: %v", err)
	}
	claim := newPilotCommand(t, db, base.Branch, base.Sale, "checkout.claim", map[string]string{}, false)
	claimed, _, err := pilotCall(ctx, owner.client, url, claim, nil)
	if err != nil || claimed.Claim == nil {
		t.Fatalf("independent invoice claim read failed: %v", err)
	}
	issue := newPilotCommand(t, db, base.Branch, base.Sale, "quote.issue", map[string]string{
		"claimDigest": claimed.Claim.Digest, "orderRevision": strconv.FormatInt(view.Order.Revision-1, 10),
	}, true)
	replayed, _, err := pilotCall(ctx, owner.client, url, issue, nil)
	q := replayed.Quote
	if err != nil || q == nil || q.Presentation == nil || q.Presentation.Invoice == nil ||
		q.Presentation.Origin != invoice.ID || q.Presentation.Invoice.Cycle != invoice.Cycle ||
		q.Presentation.Invoice.TermsVersion != view.Order.Subscription.TermsVersion {
		t.Fatalf("independent TLS quote did not return the original signed invoice: %v", err)
	}
	raw, err := hex.DecodeString(q.Signed)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := hex.DecodeString(q.Digest)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := unpackQuote(raw, digest, fixtureSigner(101).Address().Bytes(), base.Target)
	if err != nil || quote.Args.Amount.Cmp(amount) != 0 || q.SaleRef != base.Sale ||
		!validPilotPresentation(q, quote.Args.PayerGeneration) {
		t.Fatalf("independent invoice quote binding differed: %v", err)
	}
	data, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	lab.publish(t, "display-projection.json", string(data))
	lab.await(t, ctx, "board-signed")
	verify("issued", 0, 0)
	head, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	blockTime := max(int64(head.Time)+1, int64(quote.Args.IssuedAt)+1)
	if blockTime >= q.Expires {
		t.Fatal("test chain advanced beyond the signed invoice window")
	}
	if err := chain.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", blockTime); err != nil {
		t.Fatal(err)
	}
	lab.publish(t, "board-signed-no-payment", q.Digest)
	lab.await(t, ctx, "explicitly-submitted")
	var hash string
	var receipt *types.Receipt
	waitUntil(t, func() bool {
		if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", base.Sale).Scan(&hash); err != nil {
			return false
		}
		receipt, err = chain.TransactionReceipt(ctx, common.HexToHash(hash))
		return err == nil
	})
	if receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("invoice transfer reverted: %s", hash)
	}
	mine(t, ctx, chain, 6)
	waitUntil(t, func() bool {
		var paid int
		return db.QueryRow(ctx, "SELECT count(*) FROM ledger.payment WHERE sale_id=$1 AND tx_hash=$2", base.Sale, hash).Scan(&paid) == nil && paid == 1
	})
	verify("paid", 1, 1)
	balanceAfter, err := token.BalanceOf(nil, quote.Args.Beneficiary)
	if err != nil || new(big.Int).Sub(balanceAfter, balanceBefore).Cmp(amount) != 0 {
		t.Fatalf("subscription beneficiary did not gain the exact reviewed amount: %v", err)
	}
	first, err := status()
	if err != nil || first.Order == nil || first.Order.Payment == nil || first.Order.Payment.TxHash == nil ||
		*first.Order.Payment.TxHash != hash || first.Order.ReceiptRef == nil || first.Order.Subscription == nil ||
		first.Order.Subscription.InvoiceRef != invoice.ID || first.Order.Subscription.Cycle != invoice.Cycle {
		t.Fatalf("paid invoice lost its original receipt: %v", err)
	}
	data, err = json.Marshal(map[string]string{"sale": base.Sale, "invoice": invoice.ID, "hash": hash, "receipt": *first.Order.ReceiptRef})
	if err != nil {
		t.Fatal(err)
	}
	lab.publish(t, "final.json", string(data))
	lab.await(t, ctx, "original-reopened")
	verify("paid", 1, 1)
	t.Logf("W19 original invoice %s cycle %s sale %s digest %s payment %s receipt %s", invoice.ID, invoice.Cycle, base.Sale, q.Digest, hash, *first.Order.ReceiptRef)
}
