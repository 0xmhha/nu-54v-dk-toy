package commerce

import (
	"strings"
	"testing"
)

func exerciseExperienceHTTP(t *testing.T, execute func(string, ...any),
	call func(string, string, string, map[string]any, int) map[string]any, branch, foreign, actor, customer, origin, asset string) {
	t.Helper()
	const offer = "003971e2-b961-4ec4-ad9c-efdbfdb8cbb1"
	const subscription = "1c53f2fc-9166-47e2-81fc-c8b803497d10"
	const invoice = "0080f163-ea90-4db8-8c65-90bf8e92f18e"
	const key = "936cad97-8ab7-4f19-9b4c-e6c6f523a7b6"
	path := "/v2/operations/experience/" + branch
	call("GET", path, origin, nil, 404)
	execute("INSERT INTO ledger.finance_grant VALUES($1,$2,'experience_read',now()+interval '1 hour'),($1,$2,'experience_cancel',now()+interval '1 hour')", branch, actor)
	execute(`INSERT INTO ledger.subscription_offer
         (id,branch_id,label,terms_version,terms_digest,cadence,asset_ref,amount,fulfillment_kind)
         VALUES($1,$2,'Scoped fixture','V1',sha256('test'::bytea),'monthly',$3,10000000,'digital')`, offer, branch, asset)
	execute(`INSERT INTO ledger.subscription
         (id,branch_id,customer_scope,offer_id,terms_version,terms_digest,accepted_at,cadence,timezone,anchor_day,due_days,asset_ref,amount,next_cycle,next_due_at)
         VALUES($1,$2,$3,$4,'V1',sha256('test'::bytea),clock_timestamp(),'monthly','America/New_York',1,7,$5,10000000,'2027-01-01',now()+interval '1 month')`,
		subscription, branch, customer, offer, asset)
	execute(`WITH p AS (INSERT INTO ledger.sale(branch_id,origin,origin_ref,state,asset_ref,amount)
        VALUES($1,'subscription',$2,'issued',$3,10000000) RETURNING id)
        INSERT INTO ledger.invoice(id,branch_id,schedule_ref,cycle_key,due_at,sale_id,fulfillment_kind)
        SELECT $2,$1,$4,'2026-12-01', now()+interval '7 days',p.id,'digital' FROM p`, branch, invoice, asset, subscription)
	call("GET", "/v2/operations/experience/"+foreign+"?reference="+subscription, origin, nil, 404)
	before := call("GET", path+"?reference="+subscription, origin, nil, 200)
	subBefore := before["subscriptions"].([]any)[0].(map[string]any)
	if subBefore["timezone"] != "America/New_York" {
		t.Fatal("subscription time zone missing")
	}
	priorOrder := subBefore["invoices"].([]any)[0].(map[string]any)["order"]
	if strings.Contains(priorOrder.(string), customer) {
		t.Fatal("customer identity leaked into order projection")
	}
	cancel := map[string]any{"subscription": subscription, "requestKey": key, "expectedRevision": 1}
	call("POST", path+"/cancel", origin, cancel, 409)
	cancel["expectedRevision"] = 0
	reply := call("POST", path+"/cancel", origin, cancel, 200)
	call("POST", path+"/cancel", origin, cancel, 200)
	subAfter := reply["subscriptions"].([]any)[0].(map[string]any)
	existing := subAfter["invoices"].([]any)[0].(map[string]any)
	if subAfter["state"] != "cancelled" || subAfter["nextCycle"] != nil || existing["order"] != priorOrder || existing["state"] != "issued" {
		t.Fatalf("cancellation changed the existing issued order: %v", reply)
	}
}
