package settlement

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
)

type refundFinanceProbe struct {
	Chain
	funds      atomic.Int64
	routeReady atomic.Bool
	withdrawal common.Address
}

func (f *refundFinanceProbe) RefundReserve(context.Context, Target, [32]byte) (*big.Int, error) {
	return big.NewInt(f.funds.Load()), nil
}
func (f *refundFinanceProbe) RefundWithdrawal(context.Context, Target, common.Address, uint64) (common.Address, error) {
	if f.routeReady.Load() {
		return f.withdrawal, nil
	}
	return common.Address{}, nil
}

func TestPostgresFinanceConsoleScopesAndReplay(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := t.Context()
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x2222"), Manifest: strings.Repeat("a", 64)}
	issued := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC)
	order := seedPayerOrder(t, db, target, issued, "22000000", 132, fixtureSigner(102), 1)
	merchant := common.Hash(order.Quote.Args.MerchantId).Hex()
	otherOrder := seedPayerOrder(t, db, target, issued, "22000000", 133, fixtureSigner(103), 1)
	txHash, blockHash := (common.Hash{31: 12}).Hex(), (common.Hash{31: 13}).Hex()
	fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),$4,0,$5,$6,$7,22000000,$8,1,1,$9)`,
		order.Sale, order.Quote.Digest[:], target.Stream, txHash, blockHash, issued.Add(time.Second),
		lower(target.Asset), lower(order.Order.Payer), merchant)
	fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),$4,0,$5,$6,$7,22000000,$8,1,1,$9)`,
		otherOrder.Sale, otherOrder.Quote.Digest[:], target.Stream, (common.Hash{31: 14}).Hex(), blockHash, issued.Add(time.Second),
		lower(target.Asset), lower(otherOrder.Order.Payer), common.Hash(otherOrder.Quote.Args.MerchantId).Hex())
	reader, exporter, foreign, treasury := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	for _, grant := range [][2]string{{reader, "report_read"}, {reader, "reconcile_read"}, {reader, "reconcile_retry"}, {reader, "day_close"}, {exporter, "report_export"}} {
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,$3,now()+interval '1 hour')", order.Branch, grant[0], grant[1])
	}
	fixtureExec(t, db, `INSERT INTO ledger.finance_grant VALUES
      ($1,$2,'refund_request',now()+interval '1 hour'),
      ($1,$2,'refund_approve',now()+interval '1 hour'),
      ($1,$3,'refund_approve',now()+interval '1 hour')`, order.Branch, reader, exporter)
	_, financeDSN := testLogin(t, db, dsn, "nu54_p05_finance")
	store, err := Open(ctx, financeDSN, "nu54_p05_finance")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	proofLogin, proofDSN := testLogin(t, db, dsn, "nu54_p07_reconcile")
	proof, err := OpenEvidence(ctx, proofDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer proof.Close()
	_, closeDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	closer, err := Open(ctx, closeDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	fixtureExec(t, db, `INSERT INTO chainview.stream VALUES($1,'8283',$2,$3,$4)`, target.Stream, lower(target.Contract), ABIHash, target.Manifest)
	fixtureExec(t, db, `INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '1 hour')`, proofLogin, target.Stream, merchant)
	trust, certificates, _ := mtlsClients(t, 4)
	identities := map[string]service.Principal{}
	for i, actor := range []string{reader, exporter, foreign, treasury} {
		identities[fingerprint(certificates[i])] = service.Principal{Tenant: order.Tenant, Subject: actor,
			Role: "finance", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	}
	const origin = "https://console.example.invalid"
	funds := &refundFinanceProbe{withdrawal: common.HexToAddress("0x4567")}
	mux := http.NewServeMux()
	FinanceHTTP{Store: store, CloseStore: closer, Evidence: proof, Credentials: identities, OperatorOrigin: origin, Funds: funds,
		Targets: map[string]Target{order.Branch: target, otherOrder.Branch: target}, Merchants: map[string]string{order.Branch: merchant}}.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	clients := make([]*http.Client, len(certificates))
	for i, certificate := range certificates {
		cfg := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		cfg.Certificates = []tls.Certificate{certificate}
		clients[i] = &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 5 * time.Second}
		defer clients[i].CloseIdleConnections()
	}
	call := func(who int, operation, callerOrigin string, body map[string]string, status int) string {
		t.Helper()
		wire, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v2/finance/"+operation, bytes.NewReader(wire))
		req.Header.Set("Origin", callerOrigin)
		req.Header.Set("Content-Type", "application/json")
		response, err := clients[who].Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != status {
			t.Fatalf("%s %d %q %v", operation, response.StatusCode, data, err)
		}
		return string(data)
	}
	call(0, "deployment-profile", origin, map[string]string{"branchId": order.Branch}, 200)
	call(1, "deployment-profile", origin, map[string]string{"branchId": order.Branch}, 404)
	saleScope := map[string]string{"branchId": order.Branch, "saleId": order.Sale}
	call(2, "refund-status", origin, saleScope, 404)
	legacyKey := fixtureUUID(t, db)
	if _, err := store.db.Exec(ctx, "SELECT ledger.reserve_refund($1,$2,$3,$4,$5,1)",
		order.Tenant, order.Branch, reader, order.Sale, legacyKey); !errors.Is(sqlError(err), service.Conflict) {
		t.Fatalf("unknown legacy reference should reject a new staff refund: %v", err)
	}
	requested := map[string]string{"branchId": order.Branch, "saleId": order.Sale,
		"idempotencyKey": fixtureUUID(t, db), "amount": "3000000", "reason": "partial_service"}
	call(0, "refund-reserve", "https://foreign.example.invalid", requested, 404)
	result := call(0, "refund-reserve", origin, requested, 200)
	if got := call(0, "refund-reserve", origin, requested, 200); got != result {
		t.Fatal(got, result)
	}
	var receipt map[string]string
	if err := json.Unmarshal([]byte(result), &receipt); err != nil {
		t.Fatal(err)
	}
	requested["reason"] = "duplicate_charge"
	call(0, "refund-reserve", origin, requested, 409)
	decision := map[string]string{"branchId": order.Branch, "saleId": order.Sale, "refundId": receipt["refundId"]}
	otherRefund := fixtureUUID(t, db)
	fixtureExec(t, db, `INSERT INTO ledger.refund(id,sale_id,request_key,chain_refund_id,amount,requester,requested_at,state,reason_code)
          VALUES($1,$2,gen_random_uuid(),sha256(convert_to($1::uuid::text,'UTF8')),1,$3,clock_timestamp(),'cancelled','merchant_cancel')`,
		otherRefund, otherOrder.Sale, reader)
	queueScope := map[string]string{"branchId": order.Branch}
	call(2, "refund-queue", origin, queueScope, 404)
	call(1, "refund-queue", origin, map[string]string{"branchId": otherOrder.Branch}, 404)
	call(1, "refund-queue", origin, map[string]string{"branchId": order.Branch, "cursor": "malformed"}, 400)
	call(1, "refund-queue", "https://foreign.example.invalid", queueScope, 404)
	for role, next := range []string{"requester_cannot_decide", "decide"} {
		got := call(role, "refund-queue", origin, queueScope, 200)
		for _, field := range []string{order.Sale, receipt["refundId"], `"next":"` + next + `"`, `"principal":"22000000"`} {
			if !strings.Contains(got, field) || strings.Contains(got, lower(order.Order.Payer)) || strings.Contains(got, reader) || strings.Contains(got, otherRefund) {
				t.Fatalf("unexpected refund queue projection: %s", got)
			}
		}
	}
	call(0, "refund-approve", origin, decision, 404)
	call(1, "refund-reject", origin, decision, 400)
	decision["rejectionCode"] = "route_review"
	call(0, "refund-reject", origin, decision, 404)
	if got := call(1, "refund-status", origin, saleScope, 200); !strings.Contains(got, `"canDecide":true`) ||
		!strings.Contains(got, `"reason":"partial_service"`) {
		t.Fatal(got)
	}
	call(1, "refund-reject", origin, decision, 200)
	call(1, "refund-reject", origin, decision, 200)
	decision["rejectionCode"] = "amount_review"
	call(1, "refund-reject", origin, decision, 409)
	if got := call(0, "refund-status", origin, saleScope, 200); !strings.Contains(got, `"available":"22000000"`) ||
		!strings.Contains(got, `"state":"cancelled"`) || !strings.Contains(got, `"rejectionCode":"route_review"`) {
		t.Fatal(got)
	}
	requested["idempotencyKey"], requested["amount"], requested["reason"] = fixtureUUID(t, db), "19000000", "partial_service"
	winner := call(0, "refund-reserve", origin, requested, 200)
	if result := call(0, "refund-reserve", origin, requested, 200); result != winner {
		t.Fatalf("committed request key lost after headroom changed: %s %s", winner, result)
	}
	requested["idempotencyKey"], requested["amount"] = fixtureUUID(t, db), "5000000"
	if conflict := call(0, "refund-reserve", origin, requested, 409); !strings.Contains(conflict, `"code":"REFUND_LIMIT_CHANGED"`) {
		t.Fatalf("insufficient current principal did not identify the limit review: %s", conflict)
	}
	if got := call(0, "refund-status", origin, saleScope, 200); !strings.Contains(got, `"available":"3000000"`) {
		t.Fatalf("competing request moved the current principal: %s", got)
	}
	if err := json.Unmarshal([]byte(winner), &receipt); err != nil {
		t.Fatal(err)
	}
	decision["refundId"] = receipt["refundId"]
	call(1, "refund-reject", origin, decision, 200)
	day := map[string]string{"branchId": order.Branch, "businessDay": "2026-11-20"}
	got := call(0, "report", origin, day, 200)
	for _, part := range []string{`"gross":"22000000"`, `"net":"22000000"`, `"zone":"Asia/Seoul"`} {
		if !strings.Contains(got, part) {
			t.Fatalf("report omitted %s: %s", part, got)
		}
	}
	call(1, "report", origin, day, 404)
	call(2, "report", origin, day, 404)
	call(0, "report", "https://foreign.example.invalid", day, 404)
	call(0, "order", origin, map[string]string{"branchId": order.Branch, "saleId": order.Sale}, 200)
	call(0, "order", origin, map[string]string{"branchId": order.Branch, "saleId": fixtureUUID(t, db)}, 404)
	cases := call(0, "reconciliation", origin, day, 200)
	if !strings.Contains(cases, `"status":"held"`) || !strings.Contains(cases, order.Sale) {
		t.Fatalf("missing hold and original order: %s", cases)
	}
	day["idempotencyKey"] = fixtureUUID(t, db)
	call(0, "report-export", origin, day, 404)
	first := call(1, "report-export", origin, day, 200)
	if second := call(1, "report-export", origin, day, 200); first != second || !strings.Contains(second, "22000000,0,22000000") {
		t.Fatalf("CSV was not a stable scoped export: %q vs %q", first, second)
	}
	day["businessDay"] = "2026-11-21"
	call(1, "report-export", origin, day, 409)
	var receipts int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.finance_export e WHERE e.branch_id=$1", order.Branch).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("duplicate or absent audit snapshot: %d %v", receipts, err)
	}
	day["businessDay"] = "2026-11-20"
	closeKey := fixtureUUID(t, db)
	day["idempotencyKey"] = closeKey
	call(0, "report-close", origin, day, 409)
	generation := fixtureUUID(t, db)
	finalHash := (common.Hash{31: 21}).Hex()
	baseHash := (common.Hash{31: 22}).Hex()
	fixtureExec(t, db, `INSERT INTO chainview.generation(id,stream_id,base_height,fence,state)
       VALUES($1,$2,10,0,'building')`, generation, target.Stream)
	fixtureExec(t, db, `INSERT INTO chainview.block VALUES
       ($1,$2,10,$2,'2026-11-19T18:00:00Z',true),($1,$3,11,$2,$5,true),($1,$4,12,$3,'2026-11-20T19:30:00Z',true)`,
		generation, baseHash, blockHash, finalHash, issued.Add(time.Second))
	fixtureExec(t, db, `UPDATE chainview.generation SET state='ready',tip_height=12,tip_hash=$2,finalized_height=12,finalized_hash=$2
       WHERE id=$1`, generation, finalHash)
	fixtureExec(t, db, `INSERT INTO chainview.read_target VALUES($1,$2,0)`, target.Stream, generation)
	finalized := func(merchantKey, hash string, want bool) {
		t.Helper()
		var valid bool
		err := proof.db.QueryRow(ctx, "SELECT chainview.reconciliation_finalized_block($1,$2,11,$3)",
			target.Stream, merchantKey, hash).Scan(&valid)
		if err != nil || valid != want {
			t.Fatalf("failed transaction anchor visible outside its final stream scope: %v %v", valid, err)
		}
	}
	finalized(merchant, blockHash, true)
	finalized(merchant, finalHash, false)
	finalized((common.Hash{31: 99}).Hex(), blockHash, false)
	fixtureExec(t, db, `INSERT INTO chainview.observed_event
       (generation_id,block_hash,tx_hash,log_index,kind,receipt_success,recipient_ref,signer_nonce,merchant_key,order_key,asset_ref,amount)
       VALUES($1,$2,$3,0,'paid',true,$4,'7',$5,$6,$7,22000000)`,
		generation, blockHash, txHash, lower(order.Quote.Args.Beneficiary), merchant, common.Hash(order.Quote.Args.OrderId).Hex(), lower(target.Asset))
	var eventID, effectKey string
	if err := db.QueryRow(ctx, `UPDATE ledger.fulfillment_outbox SET state='unknown',lease_until=now()+interval '10 minutes'
       WHERE sale_id=$1 RETURNING event_id::text,effect_key::text`, order.Sale).Scan(&eventID, &effectKey); err != nil {
		t.Fatal(err)
	}
	retry := map[string]string{"branchId": order.Branch, "idempotencyKey": fixtureUUID(t, db), "eventId": eventID}
	review := map[string]string{"branchId": order.Branch, "eventId": eventID, "businessDay": day["businessDay"]}
	call(1, "fulfillment-review", origin, review, 404)
	if got := call(0, "fulfillment-review", origin, review, 200); !strings.Contains(got, order.Sale) || !strings.Contains(got, "merchant-fulfillment") || strings.Contains(got, effectKey) {
		t.Fatalf("unexpected notification preview: %s", got)
	}
	call(1, "fulfillment-retry", origin, retry, 404)
	call(0, "fulfillment-retry", origin, retry, 200)
	call(0, "fulfillment-retry", origin, retry, 200)
	var preservedState, preservedEffect string
	var stillLeased bool
	if err := db.QueryRow(ctx, `SELECT state,effect_key::text,lease_until IS NOT NULL FROM ledger.fulfillment_outbox
       WHERE event_id=$1`, eventID).Scan(&preservedState, &preservedEffect, &stillLeased); err != nil ||
		preservedState != "unknown" || preservedEffect != effectKey || stillLeased {
		t.Fatalf("retry rewrote effect: %s %s %v %v", preservedState, preservedEffect, stillLeased, err)
	}
	call(0, "report-close", origin, day, 409) // kitchen has not acknowledged
	fixtureExec(t, db, `UPDATE ledger.fulfillment_outbox SET state='delivered',delivered_at=now() WHERE sale_id=$1`, order.Sale)
	review["idempotencyKey"] = retry["idempotencyKey"]
	if got := call(0, "fulfillment-review", origin, review, 200); !strings.Contains(got, retry["idempotencyKey"]) || !strings.Contains(got, "delivered") {
		t.Fatalf("original notification receipt is unavailable: %s", got)
	}
	queueRead := map[string]string{"branchId": order.Branch, "businessDay": day["businessDay"]}
	if got := call(0, "reconciliation", origin, queueRead, 200); !strings.Contains(got, retry["idempotencyKey"]) {
		t.Fatalf("completed retry disappeared from investigation: %s", got)
	}
	fixtureExec(t, db, `INSERT INTO ledger.finance_grant VALUES
		($1,$2,'reconcile_read',now()+interval '1 hour'),($1,$2,'report_read',now()+interval '1 hour')`, order.Branch, exporter)
	call(1, "fulfillment-review", origin, review, 404)
	if got := call(1, "reconciliation", origin, queueRead, 200); strings.Contains(got, retry["idempotencyKey"]) {
		t.Fatalf("another employee's lookup ID was exposed: %s", got)
	}
	foreignReview := map[string]string{"branchId": otherOrder.Branch, "eventId": eventID, "businessDay": day["businessDay"]}
	call(0, "fulfillment-review", origin, foreignReview, 404)
	review["idempotencyKey"] = fixtureUUID(t, db)
	call(0, "fulfillment-review", origin, review, 404)
	call(0, "fulfillment-retry", origin, retry, 200)
	retry["idempotencyKey"] = fixtureUUID(t, db)
	call(0, "fulfillment-retry", origin, retry, 409)
	fixtureExec(t, db, `UPDATE chainview.observed_event SET amount=22000001 WHERE generation_id=$1`, generation)
	call(0, "report-close", origin, day, 409) // active checkpoint differs from journal
	fixtureExec(t, db, `UPDATE chainview.observed_event SET amount=22000000 WHERE generation_id=$1`, generation)
	call(1, "report-close", origin, day, 404)
	recorded := call(0, "report-close", origin, day, 200)
	if !strings.Contains(recorded, `"revision":1`) {
		t.Fatal(recorded)
	}
	fixtureExec(t, db, `UPDATE chainview.generation SET state='frozen' WHERE id=$1`, generation)
	finalized(merchant, blockHash, false)
	if replay := call(0, "report-close", origin, day, 200); replay != recorded {
		t.Fatalf("lost close receipt: %s", replay)
	}
	day["idempotencyKey"] = fixtureUUID(t, db)
	call(0, "report-close", origin, day, 409)

	fixtureExec(t, db, `INSERT INTO ledger.finance_grant VALUES
       ($1,$2,'refund_submit',clock_timestamp()+interval '1 hour'),
       ($1,$3,'refund_submit',clock_timestamp()+interval '1 hour'),
       ($1,$4,'refund_submit',clock_timestamp()+interval '1 hour')`, order.Branch, treasury, reader, exporter)
	fixtureExec(t, db, `INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,
        enrollment_digest,attested_until) VALUES($1,gen_random_uuid(),1,$2,$3,'test',decode(repeat('01',32),'hex'),clock_timestamp()+interval '1 hour')`,
		order.Branch, lower(order.Order.Payer), lower(funds.withdrawal))
	requested["idempotencyKey"], requested["amount"] = fixtureUUID(t, db), "1100000"
	if body := call(0, "refund-status", origin, saleScope, 200); !strings.Contains(body, `"destination":"rental-held"`) || strings.Contains(body, lower(funds.withdrawal)) {
		t.Fatalf("unverified destination was released: %s", body)
	}
	if body := call(0, "refund-reserve", origin, requested, 409); !strings.Contains(body, "REFUND_DESTINATION_HELD") {
		t.Fatal(body)
	}
	funds.routeReady.Store(true)
	if body := call(0, "refund-status", origin, saleScope, 200); !strings.Contains(body, `"destination":"rental-verified"`) {
		t.Fatal(body)
	}
	if err := json.Unmarshal([]byte(call(0, "refund-reserve", origin, requested, 200)), &receipt); err != nil {
		t.Fatal(err)
	}
	approval := map[string]string{"branchId": order.Branch, "saleId": order.Sale, "refundId": receipt["refundId"]}
	call(3, "refund-approve", origin, approval, 404)
	call(1, "refund-approve", origin, approval, 200)
	for role, next := range map[int]string{0: "independent_treasury_required", 1: "independent_treasury_required", 3: "review_funds"} {
		got := call(role, "refund-queue", origin, queueScope, 200)
		var page struct {
			Items []struct{ RefundID, Next string }
		}
		if err := json.Unmarshal([]byte(got), &page); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range page.Items {
			if row.RefundID == approval["refundId"] && row.Next == next {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("approved refund returned the wrong role hand-off: %s", got)
		}
	}
	var queued int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.settlement_work WHERE refund_id=$1", receipt["refundId"]).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("checker auto-queued money: %d %v", queued, err)
	}
	call(0, "refund-submit", origin, approval, 404)
	call(1, "refund-submit", origin, approval, 404)
	call(2, "refund-submit", origin, approval, 404)
	if body := call(3, "refund-submit", origin, approval, 409); !strings.Contains(body, "REFUND_FUNDS_HELD") {
		t.Fatal(body)
	}
	funds.funds.Store(5000000)
	submitted := call(3, "refund-submit", origin, approval, 200)
	funds.funds.Store(0)
	if replay := call(3, "refund-submit", origin, approval, 200); replay != submitted {
		t.Fatal(replay, submitted)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.settlement_work WHERE refund_id=$1", receipt["refundId"]).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("treasury did not durably queue exactly one refund: %d %v", queued, err)
	}
	// Model a request already classified by the deployment migration. The
	// restricted finance role may recover this existing row but cannot mint it.
	prior := fixtureUUID(t, db)
	fixtureExec(t, db, "UPDATE ledger.payment SET reserved=reserved+1 WHERE sale_id=$1", order.Sale)
	fixtureExec(t, db, `INSERT INTO ledger.refund(id,sale_id,request_key,chain_refund_id,amount,requester,requested_at,state,decision_path)
		VALUES($1,$2,$3,sha256(convert_to($1::uuid::text,'UTF8')),1,$4,clock_timestamp(),'requested','prior')`,
		prior, order.Sale, legacyKey, reader)
	historical := map[string]string{"branchId": order.Branch, "saleId": order.Sale, "idempotencyKey": legacyKey, "amount": "1"}
	if body := call(0, "refund-reserve", origin, historical, 200); !strings.Contains(body, prior) {
		t.Fatalf("original historical request was not recovered exactly: %s", body)
	}
	historical["amount"] = "2"
	call(0, "refund-reserve", origin, historical, 409)
	call(1, "refund-approve", origin, map[string]string{"branchId": order.Branch, "saleId": order.Sale, "refundId": prior}, 200)
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.settlement_work WHERE refund_id=$1", prior).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("existing liability did not retain its original worker lane: %d %v", queued, err)
	}
	// Retained rows verify fixed-size keyset traversal without creating new work.
	fixtureExec(t, db, `INSERT INTO ledger.refund(sale_id,request_key,chain_refund_id,amount,requester,requested_at,state,reason_code)
        SELECT $1,gen_random_uuid(),sha256(convert_to(gen_random_uuid()::text,'UTF8')),1,$2,clock_timestamp(),'cancelled','merchant_cancel'
        FROM generate_series(1,52)`, order.Sale, reader)
	type queuePage struct {
		Items []struct{ RefundID string } `json:"items"`
		Next  string                      `json:"nextCursor"`
	}
	var initialPage, last queuePage
	if err := json.Unmarshal([]byte(call(1, "refund-queue", origin, queueScope, 200)), &initialPage); err != nil ||
		len(initialPage.Items) != 50 || initialPage.Next != initialPage.Items[49].RefundID {
		t.Fatalf("invalid initial queue page: %d %v", len(initialPage.Items), err)
	}
	queueScope["cursor"] = initialPage.Next
	if err := json.Unmarshal([]byte(call(1, "refund-queue", origin, queueScope, 200)), &last); err != nil ||
		len(last.Items) == 0 || last.Next != "" || last.Items[0].RefundID <= initialPage.Next {
		t.Fatalf("invalid continuation queue page: %d %v", len(last.Items), err)
	}
}
