package settlement

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testNativeSubscriptionTerms(t *testing.T, ctx context.Context, db *pgxpool.Pool,
	url string, base seededOrder, customer *http.Client, native *nativeSubscriptionLab) {
	t.Helper()
	assert := func(query string, args ...any) {
		t.Helper()
		var ok bool
		if err := db.QueryRow(ctx, query, args...).Scan(&ok); err != nil || !ok {
			t.Fatalf("subscription terms state failed: %s: %v", query, err)
		}
	}
	series, oldOffer, newOffer := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	publish := func(id, version string, amount int) {
		digest := sha256.Sum256([]byte("local published terms " + version))
		fixtureExec(t, db, `INSERT INTO ledger.subscription_offer
			(id,branch_id,series_ref,label,terms_version,terms_digest,cadence,timezone,due_days,asset_ref,amount,fulfillment_kind)
			VALUES($1,$2,$3,'도시 산책 패스',$4,$5,'weekly','Asia/Seoul',7,$6,$7,'digital')`,
			id, base.Branch, series, version, digest[:], lower(base.Target.Asset), amount)
	}
	publish(oldOffer, "v2", 4_000_000)
	native.publish(t, "preview.json", string(native.enrollmentDocument(t, url, base, map[string]string{})))
	defer os.Remove(filepath.Join(native.path, "preview.json"))
	native.await(t, ctx, "accepted")
	var schedule, sale string
	waitUntil(t, func() bool {
		return db.QueryRow(ctx, `SELECT s.id::text,i.sale_id::text
			FROM ledger.subscription s JOIN ledger.invoice i ON i.schedule_ref=s.id
			WHERE s.offer_id=$1 AND s.state='active' AND s.amount=4000000`, oldOffer).Scan(&schedule, &sale) == nil
	})
	base.Sale = sale
	owner := pilotIdentity{actor: base.Actor, board: fixtureSigner(102), client: customer}
	original := issueTimedPilot(t, ctx, db, url, base, owner, owner, "4000000")
	if original.Quote.Args.Amount.String() != "4000000" {
		t.Fatal("original quote amount changed")
	}
	identity, err := json.Marshal(map[string]string{"sale": sale})
	if err != nil {
		t.Fatal(err)
	}
	native.publish(t, "original.json", string(identity))

	native.await(t, ctx, "paused")
	assert("SELECT state='paused' AND revision=1 FROM ledger.subscription WHERE id=$1", schedule)
	native.publish(t, "pause-verified", "ok")
	native.await(t, ctx, "resumed")
	assert("SELECT state='active' AND revision=2 FROM ledger.subscription WHERE id=$1", schedule)
	publish(newOffer, "v3", 5_000_000)
	fixtureExec(t, db, "UPDATE ledger.subscription_offer SET enabled=false WHERE id=$1", oldOffer)
	native.publish(t, "offered", "ok")

	native.await(t, ctx, "reviewing-change")
	assert("SELECT NOT EXISTS(SELECT FROM ledger.subscription WHERE offer_id=$1)", newOffer)
	native.publish(t, "consent-ready", "ok")
	native.await(t, ctx, "changed")
	assert("SELECT state='cancelled' AND next_cycle IS NULL FROM ledger.subscription WHERE id=$1", schedule)
	assert("SELECT count(*)=1 FROM ledger.subscription WHERE offer_id=$1 AND state='active' AND amount=5000000", newOffer)
	assert("SELECT count(*)=1 FROM ledger.invoice i JOIN ledger.sale s ON s.id=i.sale_id WHERE i.schedule_ref=$1 AND i.sale_id=$2 AND s.amount=4000000", schedule, sale)
	native.publish(t, "change-verified", "ok")

	native.await(t, ctx, "finished")
	assert("SELECT count(*)=2 FROM ledger.subscription WHERE offer_id IN ($1,$2) AND state='cancelled' AND next_cycle IS NULL", oldOffer, newOffer)
	assert(`SELECT count(*)=1 AND bool_and(i.sale_id=$3 AND s.amount=4000000 AND s.state IN ('issued','checking'))
		FROM ledger.invoice i JOIN ledger.sale s ON s.id=i.sale_id JOIN ledger.subscription r ON r.id=i.schedule_ref
		WHERE r.offer_id IN ($1,$2)`, oldOffer, newOffer, sale)
	assert(`SELECT
		(SELECT count(*)=1 FROM ledger.checkout_claim WHERE sale_id=$1)
		AND (SELECT count(*)=1 FROM ledger.quote_work WHERE sale_id=$1)
		AND NOT EXISTS(SELECT FROM ledger.settlement_submission WHERE sale_id=$1)
		AND NOT EXISTS(SELECT FROM ledger.payment WHERE sale_id=$1)`, sale)
	native.publish(t, "result.json", `{"accepted":1,"originalAmount":"4000000","replacementAmount":"5000000","paused":true,"resumed":true,"futureCancelled":true,"sameOriginalInvoice":true,"claims":1,"quotes":1,"submissions":0,"payments":0}`)
}
