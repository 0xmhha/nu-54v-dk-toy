package settlement

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/jackc/pgx/v5/pgxpool"
)

type cafePost func(int, string, any, int) []byte

func kitchenCommand(post cafePost, actor int, scope string, name string, body map[string]any, status int) []byte {
	if body == nil {
		body = make(map[string]any)
	}
	body["operation"], body["branch"] = name, scope
	if actor == 3 && (name == "pickup-queue" || name == "pickup-detail") {
		body["includeCafe"] = true
	}
	return post(actor, "/v2/commerce", map[string]any{"travel": body}, status)
}

type kioskSettlementDetail struct {
	Amount, Asset              string
	FinalizedAt, PickupReadyAt time.Time
}

func kioskCafeDetail(t *testing.T, order seededOrder, post cafePost) (string, *kioskSettlementDetail) {
	t.Helper()
	raw := kitchenCommand(post, 3, order.Branch, "pickup-detail", map[string]any{"sale": order.Sale}, 200)
	var response struct {
		Pickups []struct {
			SaleRef, EventRef string
			Settlement        *kioskSettlementDetail
		}
	}
	if err := json.Unmarshal(raw, &response); err != nil || len(response.Pickups) != 1 || response.Pickups[0].SaleRef != order.Sale {
		t.Fatal("cafe detail did not reopen the authorized original")
	}
	item := response.Pickups[0]
	return item.EventRef, item.Settlement
}

func verifyKioskCafeHold(t *testing.T, db *pgxpool.Pool, order seededOrder, post cafePost) {
	t.Helper()
	legacy := post(3, "/v2/commerce", map[string]any{"travel": map[string]string{
		"operation": "pickup-queue", "branch": order.Branch,
	}}, 200)
	if bytes.Contains(legacy, []byte(order.Sale)) || bytes.Contains(legacy, []byte(`"state":"checking"`)) {
		t.Fatal("legacy kitchen read received the new cafe order or waiting-state enum")
	}
	for range 2 {
		view := kitchenCommand(post, 3, order.Branch, "pickup-detail", map[string]any{"sale": order.Sale}, 200)
		var response struct {
			Pickups []struct{ SaleRef, State string }
		}
		if err := json.Unmarshal(view, &response); err != nil || len(response.Pickups) != 1 ||
			response.Pickups[0].SaleRef != order.Sale || response.Pickups[0].State != "checking" ||
			bytes.Contains(view, []byte("pickupNo")) || bytes.Contains(view, []byte("eventRef")) || bytes.Contains(view, []byte("settlement")) {
			t.Fatal("payment reconciliation did not retain the held original without pickup and fulfillment credentials")
		}
	}
	kitchenCommand(post, 3, order.Branch, "pickup-change", map[string]any{
		"sale": order.Sale, "requestKey": fixtureUUID(t, db), "expectedRevision": 0, "target": "preparing",
	}, 404)
	kitchenCommand(post, 0, order.Branch, "pickup-detail", map[string]any{"sale": order.Sale}, 404)
	kitchenCommand(post, 5, order.Branch, "pickup-detail", map[string]any{"sale": order.Sale}, 404)
	var printAttempts int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM ledger.print_attempt p
		JOIN ledger.fulfillment_outbox f ON f.event_id=p.event_id WHERE f.sale_id=$1`, order.Sale).Scan(&printAttempts); err != nil || printAttempts != 0 {
		t.Fatalf("held original had a print attempt: count=%d, err=%v", printAttempts, err)
	}
}

func verifyKioskCafeProgress(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn string, proof *EvidenceStore,
	order seededOrder, trust *x509.CertPool, cert tls.Certificate, original commerce.Order, post cafePost,
	paymentStatus func(int, seededOrder, int) (commerce.Order, []byte), owner int, finalStage string, native *cafeNativeLab) {
	t.Helper()
	verifyKioskCafeHold(t, db, order, post) // a chain payment alone cannot authorize preparation
	join := func(role string) *Store {
		t.Helper()
		_, restricted := testLogin(t, db, dsn, role)
		store, err := Open(ctx, restricted, role)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(store.Close)
		return store
	}
	mux := http.NewServeMux()
	Receiver{Store: join("nu54_p05_receiver"), Evidence: proof, Grants: map[string]ReceiverGrant{
		fingerprint(cert): {Branches: []string{order.Branch}, ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}}.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
	defer transport.CloseIdleConnections()
	dispatch := Dispatcher{Evidence: proof, Store: join("nu54_p05_dispatch"),
		Client:  &http.Client{Transport: transport, Timeout: 4 * time.Second},
		Targets: map[string]string{order.Branch: server.URL}}
	otherLeases := func() string {
		t.Helper()
		var leases string
		if err := db.QueryRow(ctx, `SELECT coalesce(jsonb_object_agg(event_id,lease_until),'{}'::jsonb)::text
			FROM ledger.fulfillment_outbox WHERE sale_id<>$1`, order.Sale).Scan(&leases); err != nil {
			t.Fatal(err)
		}
		return leases
	}
	before := otherLeases()
	holds, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holds.Rollback(ctx)
	// Exercise ordinary SKIP LOCKED dispatch without modifying any other sale.
	if _, err := holds.Exec(ctx, "SELECT event_id FROM ledger.fulfillment_outbox WHERE sale_id<>$1 AND state<>'delivered' FOR UPDATE", order.Sale); err != nil {
		t.Fatal(err)
	}
	if did, err := dispatch.Step(ctx); !did || err != nil {
		t.Fatalf("real receiver failed to acknowledge paid cafe order: %t %v", did, err)
	}
	if err := holds.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if after := otherLeases(); before != after {
		t.Fatal("unrelated original event lease changed during cafe fixture selection")
	}
	kitchen := kitchenCommand(post, 3, order.Branch, "pickup-queue", nil, 200)
	var queue struct {
		Pickups []struct {
			SaleRef, State, EventRef, Kind string
			PickupNo                       int64
		}
	}
	if err := json.Unmarshal(kitchen, &queue); err != nil {
		t.Fatal(err)
	}
	var event string
	var admissions, paid, delivered int
	if err := db.QueryRow(ctx, `SELECT e.event_id::text,
			(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=e.sale_id),
			(SELECT count(*) FROM ledger.payment WHERE sale_id=e.sale_id AND tx_hash=$2),
			(SELECT count(*) FROM ledger.consumer_receipt WHERE event_id=e.event_id AND effect_key=e.effect_key)
		FROM ledger.fulfillment_outbox e WHERE e.sale_id=$1 AND e.kind='paid' AND e.state='delivered'`,
		order.Sale, *original.Payment.TxHash).Scan(&event, &admissions, &paid, &delivered); err != nil ||
		admissions != 1 || paid != 1 || delivered != 1 {
		t.Fatalf("original cafe admission, payment and fulfillment did not converge once: %d/%d/%d, %v", admissions, paid, delivered, err)
	}
	found := 0
	for _, row := range queue.Pickups {
		if row.SaleRef != order.Sale {
			continue
		}
		if row.EventRef != event || row.Kind != "cafe" || row.State != "accepted" || row.PickupNo != *original.PickupNo {
			t.Fatal("wrong acknowledged cafe work")
		}
		found++
	}
	if found != 1 {
		t.Fatalf("expected one original pickup item; got %d", found)
	}
	stages := []string{"preparing"}
	if finalStage != "preparing" {
		stages = append(stages, "ready")
	}
	if finalStage == "collected" {
		stages = append(stages, "collected")
	}
	var included, journalConfirmed time.Time
	if err := db.QueryRow(ctx, `SELECT p.included_at,j.booked_at FROM ledger.journal j JOIN ledger.payment p
		ON p.sale_id=j.sale_id AND p.evidence_id=j.evidence_id WHERE j.sale_id=$1 AND j.refund_id IS NULL`, order.Sale).Scan(&included, &journalConfirmed); err != nil {
		t.Fatal("original payment journal confirmation unavailable")
	}
	if included.Before(order.Order.Issued) || !included.Before(order.Order.Expires) {
		t.Fatal("canonical inclusion fell outside the original signed quote lifetime")
	}
	inclusionAhead := finalStage == "ready"
	if inclusionAhead && !included.After(journalConfirmed) || !inclusionAhead && !included.Before(journalConfirmed) {
		t.Fatalf("expected the fixture's directed clock skew (journal minus inclusion: %s)", journalConfirmed.Sub(included))
	}
	t.Logf("journal minus inclusion: %s", journalConfirmed.Sub(included))
	if bytes.Contains(kitchen, []byte("settlement")) {
		t.Fatal("the queue carried detail-only cafe settlement")
	}
	if _, detail := kioskCafeDetail(t, order, post); detail != nil {
		t.Fatal("accepted but unprepared original exposed a readiness time")
	}
	accepted, _ := paymentStatus(owner, order, 200)
	if accepted.Payment == nil || accepted.Payment.PickupStage != nil || accepted.ReceiptRef == nil ||
		accepted.Payment.PickupReadyAt != nil || accepted.Payment.FinalizedAt == nil ||
		accepted.Payment.IncludedAt == nil || !accepted.Payment.IncludedAt.Equal(included) ||
		!accepted.Payment.FinalizedAt.Equal(journalConfirmed) ||
		*accepted.ReceiptRef != *original.ReceiptRef {
		t.Fatalf("legacy W11 must retain the original receipt before preparation without a new pickup enum (journal minus inclusion: %s)",
			journalConfirmed.Sub(included))
	}
	if native != nil && native.nearby != nil {
		lab := native
		lab.mark("accepted")
		// The staff UI displays minutes. Wait for the real SQL clock to
		// enter a different minute without rewriting either original time.
		nextMinute := journalConfirmed.Truncate(time.Minute).Add(time.Minute)
		for {
			var mayReady bool
			if err := db.QueryRow(ctx, "SELECT clock_timestamp() >= $1", nextMinute).Scan(&mayReady); err != nil {
				t.Fatal(err)
			}
			if mayReady {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(250 * time.Millisecond):
			}
		}
		lab.mark("may-ready")
		lab.wait(t, ctx, "ready")
		view, _ := paymentStatus(owner, order, 200)
		storeEvent, detail := kioskCafeDetail(t, order, post)
		var originalReady time.Time
		if err := db.QueryRow(ctx, `SELECT ready_at FROM ledger.travel_pickup_transition
			WHERE sale_id=$1 AND target='ready'`, order.Sale).Scan(&originalReady); err != nil {
			t.Fatal(err)
		}
		if originalReady.Before(nextMinute) || detail == nil || storeEvent != event || event == lab.nearby.publicEvent ||
			detail.Amount != order.Order.Amount || detail.Asset != lower(order.Target.Asset) ||
			view.ReceiptRef == nil || *view.ReceiptRef != *original.ReceiptRef ||
			view.PickupNo == nil || *view.PickupNo != *original.PickupNo || *view.PickupNo == lab.nearby.publicNumber ||
			view.Payment == nil || view.Payment.PickupStage == nil || *view.Payment.PickupStage != "ready" ||
			view.Payment.PickupReadyAt == nil || !view.Payment.PickupReadyAt.Equal(originalReady) ||
			view.Payment.FinalizedAt == nil || !view.Payment.FinalizedAt.Equal(journalConfirmed) ||
			view.Payment.TxHash == nil || *view.Payment.TxHash != *original.Payment.TxHash ||
			!detail.PickupReadyAt.Equal(originalReady) || !detail.FinalizedAt.Equal(journalConfirmed) {
			t.Fatal("paired nearby UI did not retain the distinct original paid/ready evidence")
		}
		ready, err := json.Marshal(map[string]any{"event": event, "pickup": *view.PickupNo,
			"paid": journalConfirmed, "ready": originalReady, "hash": *view.Payment.TxHash, "receipt": *view.ReceiptRef})
		if err != nil {
			t.Fatal(err)
		}
		lab.mu.Lock()
		lab.ready = ready
		lab.mu.Unlock()
		lab.mark("ready-seen")
		lab.wait(t, ctx, "buyer-done")
		lab.wait(t, ctx, "staff-done")
		t.Log("paired signed P04 and P02 reopened the same ready nearby original; the separate public original was observed")
	} else if native != nil {
		native.mark("accepted")
		native.wait(t, ctx, "collected-seen")
		done, _ := paymentStatus(owner, order, 200)
		if done.Payment == nil || done.Payment.PickupStage == nil || *done.Payment.PickupStage != "collected" ||
			done.ReceiptRef == nil || *done.ReceiptRef != *original.ReceiptRef {
			t.Fatal("installed lifecycle did not retain the original receipt")
		}
		t.Log("installed P04 staff moved the acknowledged original through preparation and handover; installed P02 confirmed the same W11 receipt")
	} else {
		var firstReady time.Time
		for revision, target := range stages {
			command := map[string]any{"sale": order.Sale, "target": target, "expectedRevision": revision, "requestKey": fixtureUUID(t, db)}
			result := kitchenCommand(post, 3, order.Branch, "pickup-change", command, 200)
			if replay := kitchenCommand(post, 3, order.Branch, "pickup-change", command, 200); !bytes.Equal(result, replay) {
				t.Fatal("staff retry changed the original transition result")
			}
			view, _ := paymentStatus(owner, order, 200)
			if view.Payment == nil || view.Payment.PickupStage == nil || *view.Payment.PickupStage != target ||
				view.ReceiptRef == nil || *view.ReceiptRef != *original.ReceiptRef || view.PickupNo == nil || *view.PickupNo != *original.PickupNo ||
				view.Payment.TxHash == nil || *view.Payment.TxHash != *original.Payment.TxHash {
				t.Fatalf("W11 lost original pickup/payment at %s: %+v", target, view)
			}
			if view.Payment.FinalizedAt == nil || !view.Payment.FinalizedAt.Equal(journalConfirmed) ||
				view.Payment.IncludedAt == nil || !view.Payment.IncludedAt.Equal(included) {
				t.Fatalf("original booking or inclusion changed at %s", target)
			}
			if target == "ready" {
				if err := db.QueryRow(ctx, `SELECT ready_at FROM ledger.travel_pickup_transition
					WHERE sale_id=$1 AND target='ready'`, order.Sale).Scan(&firstReady); err != nil {
					t.Fatal("authenticated ready transition did not record its time")
				}
			}
			if firstReady.IsZero() && view.Payment.PickupReadyAt != nil ||
				!firstReady.IsZero() && (view.Payment.PickupReadyAt == nil || !view.Payment.PickupReadyAt.Equal(firstReady)) {
				t.Fatalf("W11 returned premature or changed ready time at %s", target)
			}
			staffEvent, detail := kioskCafeDetail(t, order, post)
			if staffEvent != event {
				t.Fatal("staff detail changed the original fulfillment event")
			}
			if firstReady.IsZero() {
				if detail != nil {
					t.Fatal("preparing exposed ready details")
				}
			} else {
				if detail == nil || detail.Amount != original.Amount || detail.Asset != original.Asset ||
					!detail.FinalizedAt.Equal(journalConfirmed) || !detail.PickupReadyAt.Equal(firstReady) ||
					detail.FinalizedAt.Location() != time.UTC || detail.PickupReadyAt.Location() != time.UTC {
					t.Fatal("ready staff detail did not retain the original UTC payment and first ready time")
				}
				kitchenCommand(post, 5, order.Branch, "pickup-detail", map[string]any{"sale": order.Sale}, 404)
			}
			paymentStatus(4-owner, order, 404)
		}
		if inclusionAhead {
			// A clock mismatch within P05 is different from a difference with
			// the block clock. Withhold an impossible ready timestamp.
			fixtureExec(t, db, "UPDATE ledger.travel_pickup_transition SET ready_at=$2 WHERE sale_id=$1 AND target='ready'",
				order.Sale, journalConfirmed.Add(-time.Second))
			invalid, _ := paymentStatus(owner, order, 200)
			if invalid.Payment == nil || invalid.Payment.PickupReadyAt != nil || invalid.Payment.FinalizedAt == nil ||
				!invalid.Payment.FinalizedAt.Equal(journalConfirmed) {
				t.Fatal("a reversed local ready timestamp was exposed or masked the valid ledger booking")
			}
			fixtureExec(t, db, "UPDATE ledger.travel_pickup_transition SET ready_at=$2 WHERE sale_id=$1 AND target='ready'",
				order.Sale, firstReady)
			fixtureExec(t, db, "UPDATE ledger.travel_pickup_transition SET ready_at=NULL WHERE sale_id=$1 AND target='ready'", order.Sale)
			if _, detail := kioskCafeDetail(t, order, post); detail != nil {
				t.Fatal("legacy missing ready time was inferred")
			}
			fixtureExec(t, db, "UPDATE ledger.travel_pickup_transition SET ready_at=$2 WHERE sale_id=$1 AND target='ready'", order.Sale, firstReady)
			fixtureExec(t, db, "UPDATE ledger.payment SET reserved=1 WHERE sale_id=$1", order.Sale)
			if _, detail := kioskCafeDetail(t, order, post); detail != nil {
				t.Fatal("refund-affecting original exposed an unqualified paid detail")
			}
			fixtureExec(t, db, "UPDATE ledger.payment SET reserved=0 WHERE sale_id=$1", order.Sale)
			fixtureExec(t, db, "UPDATE ledger.consumer_receipt SET payload_digest=decode(repeat('ff',32),'hex') WHERE event_id=$1", event)
			if _, detail := kioskCafeDetail(t, order, post); detail != nil {
				t.Fatal("mismatched durable ACK exposed original times")
			}
			fixtureExec(t, db, "UPDATE ledger.consumer_receipt SET payload_digest=(SELECT payload_digest FROM ledger.fulfillment_outbox WHERE event_id=$1) WHERE event_id=$1", event)
		}
	}
	if queue := kitchenCommand(post, 3, order.Branch, "pickup-queue", nil, 200); bytes.Contains(queue, []byte(order.Sale)) == (finalStage == "collected") {
		t.Fatal("pickup queue did not reflect the original order state")
	}
	view := kitchenCommand(post, 3, order.Branch, "pickup-detail", map[string]any{"sale": order.Sale}, 200)
	if !bytes.Contains(view, []byte(`"state":"`+stages[len(stages)-1]+`"`)) {
		t.Fatalf("original pickup order is not available again: %s", view)
	}
	var payments, effects, deliveries, steps int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$1 AND kind='paid'),
		(SELECT count(*) FROM ledger.consumer_receipt c JOIN ledger.fulfillment_outbox e USING(event_id) WHERE e.sale_id=$1),
		(SELECT count(*) FROM ledger.travel_pickup_transition WHERE sale_id=$1)`, order.Sale).Scan(&payments, &effects, &deliveries, &steps); err != nil || [4]int{payments, effects, deliveries, steps} != [4]int{1, 1, 1, len(stages)} {
		t.Fatalf("unexpected payment/dispatch/progress rows: %d/%d/%d/%d %v", payments, effects, deliveries, steps, err)
	}
	t.Logf("K05 and W11 retain the same original receipt across %v; one payment and consumer effect", stages)
}
