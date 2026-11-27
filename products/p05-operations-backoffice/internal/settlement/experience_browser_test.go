package settlement

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func seedExperienceBrowser(t *testing.T, db *pgxpool.Pool, order seededOrder, operator string) string {
	t.Helper()
	offer, subscription, invoice := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES($1,$2)", "experience-"+operator, operator)
	fixtureExec(t, db, `INSERT INTO ledger.finance_grant VALUES
	   ($1,$2,'experience_read',now()+interval '1 hour'),($1,$2,'experience_cancel',now()+interval '1 hour')`, order.Branch, operator)
	fixtureExec(t, db, `INSERT INTO ledger.operator_branch_label VALUES($1,'시험 예약 공급 지점')`, order.Branch)
	asset := lower(order.Target.Asset)
	fixtureExec(t, db, `INSERT INTO ledger.subscription_offer
         (id,branch_id,label,terms_version,terms_digest,cadence,asset_ref,amount,fulfillment_kind)
         VALUES($1,$2,'Scoped fixture','V1',sha256('test'::bytea),'monthly',$3,10000000,'digital')`, offer, order.Branch, asset)
	fixtureExec(t, db, `INSERT INTO ledger.subscription
         (id,branch_id,customer_scope,offer_id,terms_version,terms_digest,accepted_at,cadence,timezone,anchor_day,due_days,asset_ref,amount,next_cycle,next_due_at)
         VALUES($1,$2,$3,$4,'V1',sha256('test'::bytea),clock_timestamp(),'monthly','America/New_York',1,7,$5,10000000,'2027-01-01',now()+interval '1 month')`,
		subscription, order.Branch, fixtureUUID(t, db), offer, asset)
	fixtureExec(t, db, `WITH p AS (INSERT INTO ledger.sale(branch_id,origin,origin_ref,state,asset_ref,amount)
        VALUES($1,'subscription',$2,'issued',$3,10000000) RETURNING id)
        INSERT INTO ledger.invoice(id,branch_id,schedule_ref,cycle_key,due_at,sale_id,fulfillment_kind)
        SELECT $2,$1,$4,'2026-12-01',now()+interval '7 days',p.id,'digital' FROM p`, order.Branch, invoice, asset, subscription)
	return subscription
}
