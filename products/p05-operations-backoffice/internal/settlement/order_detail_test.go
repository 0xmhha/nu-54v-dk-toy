package settlement

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestPostgresFinancePendingOrderProjection(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := t.Context()
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x2222")}
	order := seedPayerOrder(t, db, target, time.Now().Truncate(time.Second), "6500000", 91, fixtureSigner(102), 1)
	actor := fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'report_read',now()+interval '1 hour')", order.Branch, actor)
	_, financeDSN := testLogin(t, db, dsn, "nu54_p05_finance")
	finance, err := Open(ctx, financeDSN, "nu54_p05_finance")
	if err != nil {
		t.Fatal(err)
	}
	defer finance.Close()
	read := func() map[string]any {
		t.Helper()
		var raw []byte
		err := finance.db.QueryRow(ctx, "SELECT ledger.finance_order($1,$2,$3,$4)", order.Tenant, order.Branch, actor, order.Sale).Scan(&raw)
		if err != nil {
			t.Fatal(err)
		}
		var detail map[string]any
		if err := json.Unmarshal(raw, &detail); err != nil {
			t.Fatal(err)
		}
		return detail
	}
	assertPending := func(detail map[string]any, tx any) {
		t.Helper()
		if detail["saleId"] != order.Sale || detail["quoteId"] != common.Hash(order.Order.QuoteID).Hex() ||
			detail["transaction"] != tx || detail["fundsStatus"] != "held" || detail["fulfillmentStatus"] != "held" ||
			detail["paidAt"] != nil || detail["confirmedHeight"] != nil {
			t.Fatalf("unexpected pending original order: %v", detail)
		}
	}
	assertPending(read(), nil)
	hash := (common.Hash{31: 77}).Hex()
	fixtureExec(t, db, `INSERT INTO ledger.settlement_submission
		(sale_id,stream_id,quote_digest,approval_digest,approval_cipher,received_at)
		VALUES($1,$2,$3,$3,decode(repeat('00',93),'hex'),now())`, order.Sale, target.Stream, order.Quote.Digest[:])
	fixtureExec(t, db, `INSERT INTO ledger.relay_attempt
		(work_id,chain_id,sender,nonce,intent_digest,expires_at,tx_hash,tx_cipher,state)
		VALUES($1,8283,$2,0,$3,now()+interval '1 minute',$4,decode(repeat('00',29),'hex'),'signed')`,
		order.Sale, lower(fixtureSigner(103).Address()), order.Quote.ApprovalHash[:], hash)
	assertPending(read(), nil)
	fixtureExec(t, db, "UPDATE ledger.relay_attempt SET state='unknown' WHERE work_id=$1", order.Sale)
	assertPending(read(), nil)
	fixtureExec(t, db, "UPDATE ledger.relay_attempt SET state='broadcast' WHERE work_id=$1", order.Sale)
	assertPending(read(), hash)
	fixtureExec(t, db, "UPDATE ledger.relay_attempt SET state='final' WHERE work_id=$1", order.Sale)
	assertPending(read(), nil)
}
