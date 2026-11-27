package settlement

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/jackc/pgx/v5/pgxpool"
)

func repricedTestEnabled() bool {
	return os.Getenv("NU54_REPRICED_ORDER_TEST") != "" || os.Getenv("NU54_REPRICED_NATIVE_LAB") != ""
}

func logRepricedExpiryState(t *testing.T, db *pgxpool.Pool, sale, phase string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var state string
	var expired, finished, eligible, resolved bool
	var payments, submissions int
	err := db.QueryRow(ctx, `SELECT s.state,q.expires_at<=clock_timestamp(),w.finished,
		NOT w.finished AND w.retry_at<=clock_timestamp(),
		EXISTS (SELECT FROM ledger.sale_resolution WHERE sale_id=s.id),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=s.id),
		(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=s.id)
		FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
		JOIN ledger.settlement_work w ON w.sale_id=s.id WHERE s.id=$1 AND w.id=s.id`, sale).
		Scan(&state, &expired, &finished, &eligible, &resolved, &payments, &submissions)
	if err != nil {
		t.Logf("archived reprice phase=%s state=unavailable", phase)
		return
	}
	t.Logf("archived reprice phase=%s state=%s elapsed=%t jobFinished=%t retryDue=%t resolved=%t payments=%d submissions=%d",
		phase, state, expired, finished, eligible, resolved, payments, submissions)
}

func repricedMarker(t *testing.T, lab *nativeSubscriptionLab, name string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(lab.path, name)
	if err := os.WriteFile(file+".tmp", raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file+".tmp", file); err != nil {
		t.Fatal(err)
	}
}

func repricedWait(t *testing.T, ctx context.Context, lab *nativeSubscriptionLab, marker string) {
	t.Helper()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(filepath.Join(lab.path, marker)); err == nil {
			return
		}
		if _, err := os.Stat(filepath.Join(lab.path, "ui-aborted")); err == nil {
			t.Fatalf("native test stopped at %s", marker)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("native %s: %v", marker, ctx.Err())
		case <-tick.C:
		}
	}
}

func verifyRepricedOriginal(t *testing.T, ctx context.Context, db *pgxpool.Pool, base seededOrder, owner pilotIdentity,
	url string, buyer fixtureSigner, post cafePost, status func(int, seededOrder, int) (commerce.Order, []byte), lab *nativeSubscriptionLab, archive cipher.AEAD, settle func(seededOrder)) seededOrder {
	t.Helper()
	if os.Getenv("NU54_P01_INSTALL_TEST") == "" {
		t.Fatal("repriced receipt evidence requires the P01 host test_install")
	}
	branch, asset := base.Branch, lower(base.Target.Asset)
	fixtureExec(t, db, "INSERT INTO ledger.operator_branch_label VALUES($1,'Yeoul Coffee Seongsu') ON CONFLICT (branch_id) DO UPDATE SET label=EXCLUDED.label", branch)
	publish := func(version int, pastry int) {
		t.Helper()
		fixtureExec(t, db, "INSERT INTO ledger.menu_release VALUES($1,$2,now()-interval '1 minute',now()+interval '15 minutes')", branch, version)
		fixtureExec(t, db, "INSERT INTO ledger.price VALUES ($1,$2,'coffee','plain','Iced coffee',$3,3500000),($1,$2,'pastry','plain','Croissant',$3,$4)", branch, version, asset, pastry)
		fixtureExec(t, db, `INSERT INTO ledger.catalog_pricing
			SELECT $1,$2,NULL,jsonb_agg(jsonb_build_object('sku',sku,'option',option_key,
				'displayPrice',unit_price::text,'tax','0','discount','0'))
			FROM ledger.price WHERE branch_id=$1 AND version=$2`, branch, version)
	}
	fixtureExec(t, db, "INSERT INTO ledger.stock VALUES ($1,'coffee',2),($1,'pastry',2)", branch)
	publish(2, 3_000_000)
	reserve := func(origin, key string, version int, expectedCode service.Code) string {
		t.Helper()
		cart := commerce.Cart{Origin: "menu", OriginRef: origin, MenuVersion: int64(version),
			Items: []commerce.Item{{SKU: "coffee", Option: "plain", Qty: 1}, {SKU: "pastry", Option: "plain", Qty: 1}}}
		raw, err := json.Marshal(cart)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		command := newPilotCommand(t, db, branch, origin, "order.create", map[string]string{
			"asset": asset, "quantity": "2", "intentDigest": rawHex(digest[:]),
		}, true)
		command.IdempotencyKey = key
		reply, httpStatus, err := pilotCall(ctx, owner.client, url, command, map[string]any{"cart": cart})
		if expectedCode != "" {
			if httpStatus != 409 || reply.Error != expectedCode || reply.ResultRef != "" {
				t.Fatalf("reserve version %d HTTP %d code %s, expected 409 %s", version, httpStatus, reply.Error, expectedCode)
			}
			return ""
		}
		if httpStatus != 200 || err != nil || !financeRef.MatchString(reply.ResultRef) {
			t.Fatalf("reserve version %d HTTP %d, expected original sale: %v", version, httpStatus, err)
		}
		return reply.ResultRef
	}

	// The comparison order was separately issued before the menu changed.
	// Its signed price remains immutable and it cannot authorize the new sale.
	old := base
	old.Sale = reserve(fixtureUUID(t, db), fixtureUUID(t, db), 2, "")
	old = issueTimedPilot(t, ctx, db, url, old, owner, owner, "6500000")
	oldApproval := pilotBoardApproval(t, ctx, old, buyer, "management")
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=clock_timestamp()+interval '1 hour' WHERE sale_id=$1", old.Sale)

	original, request := fixtureUUID(t, db), fixtureUUID(t, db)
	next := base
	if lab == nil {
		publish(3, 3_500_000)
		reserve(original, request, 2, service.CatalogChanged)
		reserve(original, request, 2, service.CatalogChanged)
		reserve(original, request, 3, service.Conflict)
		var before int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.sale WHERE branch_id=$1 AND origin_ref=$2", branch, original).Scan(&before); err != nil || before != 0 {
			t.Fatalf("stale cart acquired a sale: %d %v", before, err)
		}
		// The customer explicitly confirmed the newly published price.
		reviewedOrigin, reviewedKey := fixtureUUID(t, db), fixtureUUID(t, db)
		next.Sale = reserve(reviewedOrigin, reviewedKey, 3, "")
		if replay := reserve(reviewedOrigin, reviewedKey, 3, ""); replay != next.Sale {
			t.Fatal("same reviewed cart changed sale")
		}
		var count int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.sale WHERE branch_id=$1 AND origin_ref=$2",
			branch, reviewedOrigin).Scan(&count); err != nil || count != 1 {
			t.Fatalf("confirmed repriced cart must produce one sale: %d %v", count, err)
		}
		next = issueTimedPilot(t, ctx, db, url, next, owner, owner, "7000000")
	} else {
		var enrollment map[string]any
		if err := json.Unmarshal(lab.enrollmentDocument(t, url, old, nil), &enrollment); err != nil {
			t.Fatal(err)
		}
		repricedMarker(t, lab, "enrollment.json", enrollment)
		repricedWait(t, ctx, lab, "shown-original")
		publish(3, 3_500_000)
		repricedMarker(t, lab, "published", true)
		repricedWait(t, ctx, lab, "review-shown")
		var unapproved int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM ledger.order_checkout
			WHERE (branch_id,creator)=($1,$2) AND menu_version IN (2,3) AND sale_id<>$3`,
			branch, owner.actor, old.Sale).Scan(&unapproved); err != nil || unapproved != 0 {
			t.Fatalf("cart before confirmation: %d %v", unapproved, err)
		}
		t.Log("unconfirmed 6.50→7.00 changes created zero sales or quotes")
		repricedMarker(t, lab, "review-empty", true)
		repricedWait(t, ctx, lab, "quote-rechecked")
		var sealed, digest []byte
		if err := db.QueryRow(ctx, `SELECT s.id::text,q.payload_cipher,q.signed_digest FROM ledger.sale s
			JOIN ledger.order_checkout c ON c.sale_id=s.id
			JOIN ledger.quote_work q ON q.sale_id=s.id
			WHERE (s.branch_id,c.creator,c.menu_version)=($1,$2,3)`, branch, owner.actor).Scan(&next.Sale, &sealed, &digest); err != nil {
			t.Fatal(err)
		}
		var err error
		next.Signed, err = archive.Open(nil, nil, sealed, quoteAAD(branch, next.Sale))
		if err != nil {
			t.Fatal(err)
		}
		next.Quote, err = unpackQuote(next.Signed, digest, fixtureSigner(101).Address().Bytes(), base.Target)
		if err != nil {
			t.Fatal(err)
		}
	}
	if next.Quote.Args.Amount.String() != "7000000" || next.Quote.Args.Beneficiary != old.Quote.Args.Beneficiary || next.Sale == old.Sale {
		t.Fatal("quote lost the reviewed amount, beneficiary or original sale")
	}
	forward := newPilotCommand(t, db, branch, next.Sale, "payment.forward", map[string]string{
		"quoteDigest": rawHex(next.Quote.Digest[:]), "approvalDigest": rawHex(next.Quote.ApprovalHash[:]),
	}, true)
	refused, code, err := pilotCall(ctx, owner.client, url, forward, map[string]any{"approvalSignature": rawHex(oldApproval)})
	if err == nil || code != 400 || refused.Error != service.Mismatch {
		t.Fatalf("old signed amount must fail exact binding: status %d, code %s, %v", code, refused.Error, err)
	}
	t.Log("old 6.50 approval rejected with MISMATCH before the new submission")
	if view, _ := status(0, next, 200); view.Payment != nil || view.ReceiptRef != nil {
		t.Fatal("stale approval changed the new order")
	}
	if lab == nil {
		approval := pilotBoardApproval(t, ctx, next, buyer, "management")
		if bytes.Equal(approval, oldApproval) {
			t.Fatal("different quotes yielded the same board approval")
		}
		post(0, "/v2/commerce", map[string]any{"command": forward, "approvalSignature": rawHex(approval)}, 200)
	} else {
		repricedMarker(t, lab, "old-rejected", map[string]string{"beneficiary": lower(next.Quote.Args.Beneficiary), "sale": next.Sale})
		repricedWait(t, ctx, lab, "explicitly-submitted")
	}
	settle(next)

	owner.client.CloseIdleConnections()
	final, _ := status(0, next, 200)
	if final.ID != next.Sale || final.Amount != "7000000" || final.Payment == nil ||
		final.Payment.TxHash == nil || final.Payment.PickupStage == nil || *final.Payment.PickupStage != "collected" ||
		final.ReceiptRef == nil || final.PickupNo == nil {
		t.Fatalf("missing repriced final receipt: %+v", final)
	}
	var submitted, paid, pickup, oldPaid int
	var amount string
	err = db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.pickup WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$2),
		(SELECT principal::text FROM ledger.payment WHERE sale_id=$1)`, next.Sale, old.Sale).Scan(&submitted, &paid, &pickup, &oldPaid, &amount)
	if err != nil || submitted != 1 || paid != 1 || pickup != 1 || oldPaid != 0 || amount != "7000000" {
		t.Fatalf("repriced effects: submit=%d paid=%d pickup=%d old paid=%d amount=%s err=%v", submitted, paid, pickup, oldPaid, amount, err)
	}
	if lab != nil {
		repricedMarker(t, lab, "final.json", map[string]any{"sale": final.ID, "hash": *final.Payment.TxHash, "receipt": *final.ReceiptRef, "pickup": *final.PickupNo})
		repricedWait(t, ctx, lab, "original-reopened")
	}
	t.Logf("repriced original %s, hash %s, receipt %s, exact payment 7000000", next.Sale, *final.Payment.TxHash, *final.ReceiptRef)
	return old
}
