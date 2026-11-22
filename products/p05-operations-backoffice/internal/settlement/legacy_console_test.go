package settlement

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

func TestPostgresLegacyCashoutDispatchFence(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := t.Context()
	tenant, branch, maker, reconcile := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES('legacy-maker',$1),('legacy-reconciler',$2)", maker, reconcile)
	fixtureExec(t, db, `INSERT INTO ledger.finance_grant VALUES
		($1,$2,'legacy_ledger',now()+interval '1 hour'),($1,$2,'legacy_cashout',now()+interval '1 hour'),
		($1,$3,'legacy_ledger',now()+interval '1 hour'),($1,$3,'legacy_reconcile',now()+interval '1 hour')`, branch, maker, reconcile)
	_, scopedURL := testLogin(t, db, dsn, "nu54_p05_commerce")
	scoped, err := pgxpool.New(ctx, scopedURL)
	if err != nil {
		t.Fatal(err)
	}
	defer scoped.Close()
	open := func(key string) string {
		t.Helper()
		var result string
		err := scoped.QueryRow(ctx, `SELECT coalesce(outcome->>'status','open') FROM
			ledger.open_legacy_cashout($1,$2,$3,$4,decode(repeat('ab',32),'hex'),'{}')`, tenant, branch, maker, key).Scan(&result)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	save := func(key string) {
		t.Helper()
		if _, err := scoped.Exec(ctx, `SELECT ledger.save_legacy_cashout($1,$2,$3,$4,decode(repeat('ef',65),'hex'))`, tenant, branch, maker, key); err != nil {
			t.Fatal(err)
		}
	}
	dispatch := func(actor, key string, matched bool) bool {
		t.Helper()
		var result bool
		err := scoped.QueryRow(ctx, `SELECT ledger.dispatch_legacy_cashout($1,$2,$3,$4,$5,'{"status":"review-mismatch"}')`, tenant, branch, actor, key, matched).Scan(&result)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	expired, abandoned, uncertain := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	open(expired)
	fixtureExec(t, db, "UPDATE ledger.legacy_cashout_request SET created_at=now()-interval '2 minutes' WHERE key=$1", expired)
	if got := open(expired); got != "expired-before-signing" {
		t.Fatalf("unsigned expiry returned %q", got)
	}
	open(abandoned)
	save(abandoned)
	if dispatch(maker, abandoned, false) || open(abandoned) != "review-mismatch" || dispatch(maker, abandoned, true) {
		t.Fatal("changed review must release the unused slot without dispatching archived bytes")
	}
	open(uncertain)
	save(uncertain)
	if !dispatch(maker, uncertain, true) || dispatch(reconcile, uncertain, false) || open(uncertain) != "open" {
		t.Fatal("uncertain dispatch must retain its signer slot through later review mismatch")
	}
	fixtureExec(t, db, "DELETE FROM ledger.finance_grant WHERE branch_id=$1 AND actor_id=$2 AND capability='legacy_cashout'", branch, maker)
	if _, err := scoped.Exec(ctx, `SELECT ledger.dispatch_legacy_cashout($1,$2,$3,$4,true,'{}')`, tenant, branch, maker, uncertain); err == nil {
		t.Fatal("revoked maker resumed broadcast")
	}
	if !dispatch(reconcile, uncertain, true) {
		t.Fatal("assigned independent reconciler could not resume")
	}
}
