package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
)

func TestPickupCartCommitsExactApprovedTime(t *testing.T) {
	cart := Cart{
		Origin: "menu", OriginRef: customerA, MenuVersion: 1,
		Items:  []Item{{SKU: "drip", Option: "hot", Qty: 1}},
		Pickup: &PickupChoice{Window: other, StartsAt: "2027-01-02T09:00:00Z"},
	}
	auth := authorize(t, actor{customerA, "customer"}, "order.create", branch, customerA, customerA,
		map[string]string{"asset": "0x1111111111111111111111111111111111111111", "quantity": "1",
			"intentDigest": "c7735b29a273967cb0b629088b18c01be893c2cd5c9c92be8ce558bb81ae44ec"})
	if err := checkCart(auth, &cart); err != nil {
		t.Fatal(err)
	}
	cart.Pickup.StartsAt = "2027-01-02T10:00:00Z"
	if err := checkCart(auth, &cart); err == nil {
		t.Fatal("time changed without a new cart commitment")
	}
}

func TestPickupWindowSerializesLastPlaceAndRetainsTheOriginalSale(t *testing.T) {
	dsn := os.Getenv("P05_PICKUP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a migrated disposable P05 database")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(cfg.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(cfg.Host, "/tmp/") || strings.HasPrefix(cfg.Host, "/private/tmp/")) {
		t.Fatal("use a disposable local PostgreSQL socket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	login := "pickup_" + strings.TrimPrefix(cfg.Database, "nu54_commerce_")
	role := pgx.Identifier{login}.Sanitize()
	exec("CREATE ROLE " + role + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+role)
	store, err := Open(ctx, dsn+" user="+login)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(ctx, "SELECT * FROM ledger.travel_pickup_window"); err == nil {
		t.Fatal("runtime can read unrestricted pickup schedule")
	}

	const slot = "5eb86d05-907a-4602-ab91-f85dd71d845c"
	const foreign = "568299aa-19ef-492e-9bd3-eca7eec67d0b"
	const after = "d41513c7-8036-43e5-8339-724afb603b69"
	const asset = "0x1111111111111111111111111111111111111111"
	start := time.Now().UTC().Add(20 * time.Minute).Truncate(time.Second)
	exec("INSERT INTO ledger.branch VALUES ($1,$3,'Asia/Seoul','04:00'),($2,$3,'Asia/Seoul','04:00')", branch, other, tenant)
	exec("INSERT INTO ledger.command_rule SELECT id,'fixture pickup','fixture law','expire fixture',interval '2 minutes' FROM ledger.branch WHERE id IN ($1,$2)", branch, other)
	exec("INSERT INTO ledger.commerce_grant VALUES ($1,$2,'customer',NULL,now()+interval '2 minutes'),($1,$3,'customer',NULL,now()+interval '2 minutes')", branch, customerA, customerB)
	exec("INSERT INTO ledger.menu_release VALUES ($1,1,now()-interval '1 minute',now()+interval '2 minutes')", branch)
	exec("INSERT INTO ledger.price VALUES ($1,1,'coffee','plain','Coffee',$2,700)", branch, asset)
	exec("INSERT INTO ledger.stock VALUES ($1,'coffee',3)", branch)
	exec(`INSERT INTO ledger.travel_pickup_window(branch_id,id,starts_at,ends_at,capacity,active)
      VALUES($1,$2,$5,$5::timestamptz+interval '10 minutes',1,true),($3,$4,$5,$5::timestamptz+interval '10 minutes',1,true),
        ($1,$6,$5::timestamptz+interval '30 minutes',$5::timestamptz+interval '40 minutes',1,false)`, branch, slot, other, foreign, start, after)
	var selected string
	err = store.db.QueryRow(ctx, `SELECT id::text FROM ledger.travel_pickup_options($1,$2,$3)`, tenant, branch, customerA).Scan(&selected)
	if err != nil || selected != slot {
		t.Fatalf("wrong approved window: %s %v", selected, err)
	}
	if err := store.db.QueryRow(ctx, `SELECT id::text FROM ledger.travel_pickup_options($1,$2,$3)`, tenant, other, customerA).Scan(&selected); err == nil {
		t.Fatal("foreign branch window leaked")
	}

	order := func(who string, window string, at time.Time, version int64) (service.Bound, Cart) {
		bound := authorize(t, actor{who, "customer"}, "order.create", branch, who, who,
			map[string]string{"asset": asset, "quantity": "1", "intentDigest": strings.Repeat("a", 64)})
		return bound, Cart{Origin: "menu", OriginRef: who, MenuVersion: version, Items: []Item{{"coffee", "plain", 1}},
			Pickup: &PickupChoice{Window: window, StartsAt: at.Format(time.RFC3339Nano)}}
	}
	for _, input := range []struct {
		window  string
		start   time.Time
		version int64
	}{
		{foreign, start, 1}, {after, start.Add(30 * time.Minute), 1}, {slot, start.Add(time.Minute), 1}, {slot, start, 2},
	} {
		bound, cart := order(customerA, input.window, input.start, input.version)
		if _, err := store.Reserve(ctx, bound, cart); !errors.Is(err, service.CatalogChanged) {
			t.Fatalf("unapproved, changed, or outdated request was not re-reviewed: %v", err)
		}
	}

	type result struct {
		member, sale string
		err          error
	}
	gate := make(chan struct{})
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for _, member := range []string{customerA, customerB} {
		bound, cart := order(member, slot, start, 1)
		workers.Go(func() {
			<-gate
			sale, err := store.Reserve(ctx, bound, cart)
			results <- result{member, sale, err}
		})
	}
	close(gate)
	workers.Wait()
	close(results)
	var winner result
	for response := range results {
		if response.err == nil {
			if winner.sale != "" {
				t.Fatal("two orders reserved the final pickup window")
			}
			winner = response
		} else if !errors.Is(response.err, service.CatalogChanged) {
			t.Fatal(response.err)
		}
	}
	if winner.sale == "" {
		t.Fatal("no accepted pickup")
	}
	exec("UPDATE ledger.travel_pickup_window SET active=false WHERE branch_id=$1 AND id=$2", branch, slot)
	bound, cart := order(winner.member, slot, start, 1)
	sale, err := store.Reserve(ctx, bound, cart)
	if err != nil || sale != winner.sale {
		t.Fatalf("retry did not return original order: %s %v", sale, err)
	}
	cart.Pickup.StartsAt = start.Add(time.Minute).Format(time.RFC3339Nano)
	if _, err := store.Reserve(ctx, bound, cart); !errors.Is(err, service.Conflict) {
		t.Fatalf("changed replay not rejected: %v", err)
	}
	var sales, pick, seats, remaining int
	err = admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.sale WHERE branch_id=$1),
        (SELECT count(*) FROM ledger.travel_pickup_order WHERE branch_id=$1 AND sale_id=$2),
        (SELECT count(*) FROM ledger.travel_booking WHERE branch_id=$1),
        (SELECT available FROM ledger.stock WHERE branch_id=$1 AND sku='coffee')`, branch, sale).Scan(&sales, &pick, &seats, &remaining)
	if err != nil || sales != 1 || pick != 1 || seats != 0 || remaining != 2 {
		t.Fatalf("unexpected sale/pickup/booking/stock: %d %d %d %d %v", sales, pick, seats, remaining, err)
	}
	const fill = "ea5b823b-9f93-4cad-8b1d-ebd37db4fb5c"
	const prepareKey = "f39facb2-4585-4773-bee8-de04146b27bb"
	const readyKey = "360afe4b-d225-4205-9624-c65c63877fa0"
	const failKey = "262f5b76-6a92-4613-826e-07b333404306"
	const financeReq = "fde18be4-23a4-4467-b5f3-35ee0de6674a"
	const financeCheck = "35b3760c-af7e-456c-9cf7-7af75b389db5"
	const financeSubmit = "9507e11b-6269-41a3-b329-14149d7b217f"
	const refundKey = "b35fe694-9bbb-4d77-8c73-e582a29fde4d"
	const refundEvidence = "1662cf7d-ea03-46f4-9c4c-beccc4a2a3a9"
	badCustomer := customerA
	if winner.member == customerA {
		badCustomer = customerB
	}
	exec("INSERT INTO ledger.commerce_grant VALUES ($1,$2,'terminal','menu',now()+interval '2 minutes'),($1,$3,'terminal','menu',now()+interval '2 minutes')", branch, fill, terminal)
	exec("INSERT INTO ledger.travel_pickup_terminal VALUES ($1,$2)", branch, fill)
	snapshot := func(who string) (map[string]any, error) {
		var data []byte
		err := store.db.QueryRow(ctx, `SELECT * FROM ledger.travel_pickup_customer($1,$2,$3,$4)`, tenant, branch, who, sale).Scan(&data)
		if err != nil {
			return nil, err
		}
		var projection map[string]any
		err = json.Unmarshal(data, &projection)
		return projection, err
	}
	advance := func(who, key, target string, at int64) (int64, error) {
		var revision int64
		err := store.db.QueryRow(ctx, `SELECT ledger.change_travel_pickup($1,$2,$3,$4,$5,$6,$7)`, tenant, branch, who, sale, key, at, target).Scan(&revision)
		return revision, err
	}
	stage := func(want string) map[string]any {
		t.Helper()
		item, err := snapshot(winner.member)
		if err != nil || item["state"] != want {
			t.Fatalf("pickup state: got %v %v want %s", item, err, want)
		}
		return item
	}
	if _, err := snapshot(badCustomer); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign customer saw original order: %v", err)
	}
	readOrder := func(who string) (Order, error) {
		req := authorize(t, actor{who, "customer"}, "order.status", branch, sale, "", map[string]string{})
		return store.Status(ctx, req)
	}
	if current, err := readOrder(winner.member); err != nil || current.PickupNo != nil || current.ReceiptRef != nil {
		t.Fatalf("unpaid order status exposed evidence: %+v, %v", current, err)
	}
	stage("waiting-payment")
	if _, err := advance(fill, prepareKey, "preparing", 0); err == nil {
		t.Fatal("unpaid pickup progressed")
	}
	exec(`INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,
      payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
      VALUES($1,$2,decode(repeat('91',32),'hex'),decode(repeat('92',32),'hex'),decode(repeat('93',32),'hex'),
        decode(repeat('94',32),'hex'),0,1,decode(repeat('11',20),'hex'),decode(repeat('22',20),'hex'),1,1,
        now()-interval '10 seconds',now()+interval '30 seconds',1,now()+interval '30 seconds',decode('00','hex'),decode(repeat('99',32),'hex'))`, sale, winner.member)
	exec("UPDATE ledger.sale SET state='issued' WHERE id=$1", sale)
	const merchant = "0x1111111111111111111111111111111111111111111111111111111111111111"
	exec(`SELECT ledger.confirm_payment($1,decode(repeat('99',32),'hex'),$2,gen_random_uuid(),'0x'||repeat('b1',32),0,
        '0x'||repeat('b2',32),date_trunc('second',now()),$3,700,'0x'||repeat('22',20),1,1,$4)`, sale, branch, asset, merchant)
	if item := stage("awaiting-acceptance"); item["pickupNo"] != nil || item["receiptRef"] == nil {
		t.Fatalf("paid before ACK: %v", item)
	}
	beforeACK, err := readOrder(winner.member)
	if err != nil || beforeACK.State != "paid" || beforeACK.PickupNo != nil || beforeACK.ReceiptRef == nil ||
		beforeACK.ReceiptKind == nil || *beforeACK.ReceiptKind != "pickup-awaiting-acceptance" {
		t.Fatalf("paid C3 before same-sale ACK exposed a pickup number or lost payment: %+v, %v", beforeACK, err)
	}
	if _, err := readOrder(badCustomer); !errors.Is(err, service.Forbidden) {
		t.Fatalf("nonowner C3 obtained the original paid receipt: %v", err)
	}
	var projected *int64
	if err := store.db.QueryRow(ctx, `SELECT pickup_no FROM ledger.commerce_order_view($1,$2,$3,$4)`,
		tenant, branch, winner.member, sale).Scan(&projected); err != nil || projected != nil {
		t.Fatalf("underlying authorized projection exposed pre-ACK number: %v, %v", projected, err)
	}
	if _, err := advance(fill, prepareKey, "preparing", 0); err == nil {
		t.Fatal("unaccepted pickup progressed")
	}
	exec(`UPDATE ledger.fulfillment_outbox SET fence=1,state='sending',lease_until=now()+interval '2 minutes'
      WHERE sale_id=$1 AND kind='paid'`, sale)
	exec(`SELECT ledger.prepare_fulfillment(event_id,1,effect_key,repeat('b3',32),decode(repeat('b4',32),'hex'),true)
      FROM ledger.fulfillment_outbox WHERE sale_id=$1 AND kind='paid'`, sale)
	exec(`SELECT ledger.receive_fulfillment(event_id,effect_key,1,repeat('b3',32),decode(repeat('b4',32),'hex'),
        $2,$2,$3,true) FROM ledger.fulfillment_outbox WHERE sale_id=$1 AND kind='paid'`, sale, branch, merchant)
	accepted := stage("accepted")
	afterACK, err := readOrder(winner.member)
	if err != nil || afterACK.PickupNo == nil || float64(*afterACK.PickupNo) != accepted["pickupNo"] ||
		afterACK.ReceiptKind != nil || afterACK.ReceiptRef == nil || *afterACK.ReceiptRef != *beforeACK.ReceiptRef {
		t.Fatalf("paid C3 after ACK did not retain the original receipt and accepted number: %+v, %v", afterACK, err)
	}
	if _, err := advance(terminal, prepareKey, "preparing", 0); err == nil {
		t.Fatal("ordinary public terminal could prepare a pickup")
	}
	if _, err := advance(fill, readyKey, "ready", 0); err == nil {
		t.Fatal("invalid transition was accepted")
	}
	for _, step := range []struct {
		key, target string
		expected    int64
	}{{prepareKey, "preparing", 0}, {readyKey, "ready", 1}, {failKey, "unfulfillable", 2}} {
		for range 2 {
			next, err := advance(fill, step.key, step.target, step.expected)
			if err != nil || next != step.expected+1 {
				t.Fatalf("transition/replay failed: %v %d", err, next)
			}
		}
		stage(step.target)
	}

	getRefund := func(want string) map[string]any {
		t.Helper()
		refund, ok := stage("unfulfillable")["refund"].(map[string]any)
		if !ok || refund["state"] != want {
			t.Fatalf("refund state: got %v want %s", refund, want)
		}
		return refund
	}
	getRefund("awaiting-request")
	exec(`INSERT INTO ledger.finance_grant VALUES
      ($1,$2,'refund_request',now()+interval '2 minutes'),($1,$3,'refund_approve',now()+interval '2 minutes'),
      ($1,$4,'refund_submit',now()+interval '2 minutes')`, branch, financeReq, financeCheck, financeSubmit)
	var refund string
	for range 2 {
		var id string
		err := admin.QueryRow(ctx, `SELECT ledger.reserve_console_refund($1,$2,$3,$4,$5,700,'merchant_cancel')::text`,
			tenant, branch, financeReq, sale, refundKey).Scan(&id)
		if err != nil || (refund != "" && id != refund) {
			t.Fatalf("formal refund: %s %v", id, err)
		}
		refund = id
	}
	if item := getRefund("pending"); item["receiptRef"] != nil || item["amount"] != "700" {
		t.Fatalf("pending refund evidence: %v", item)
	}
	exec(`SELECT ledger.approve_refund($1,$2,$3,$4,$5,0)`, tenant, branch, financeCheck, sale, refund)
	exec(`SELECT ledger.submit_console_refund($1,$2,$3,$4,$5,$6,700,0)`, tenant, branch, financeSubmit, sale, refund, merchant)
	exec(`SELECT ledger.mark_refund_inflight($1)`, refund)
	getRefund("pending")
	exec(`SELECT ledger.confirm_refund(r.id,$2,$3,'0x'||repeat('c1',32),0,'0x'||repeat('c2',32),
        date_trunc('second',now()),$4,700,'0x'||repeat('22',20),1,r.chain_refund_id) FROM ledger.refund r WHERE r.id=$1`,
		refund, branch, refundEvidence, asset)
	if entry := getRefund("confirmed"); entry["receiptRef"] != refundEvidence {
		t.Fatalf("wrong linked refund evidence: %v", entry)
	}
	var transitions, refunds int
	err = admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.travel_pickup_transition WHERE sale_id=$1),
        (SELECT count(*) FROM ledger.refund WHERE sale_id=$1)`, sale).Scan(&transitions, &refunds)
	if err != nil || transitions != 3 || refunds != 1 {
		t.Fatalf("duplicate effects: %d %d %v", transitions, refunds, err)
	}

}

func TestPickupLifecycleEnvelopeCannotMixBookingAndTerminalChanges(t *testing.T) {
	revision := int64(0)
	change := TravelRequest{Operation: "pickup-change", Branch: branch, Sale: customerA,
		Key: customerB, Expected: &revision, Target: "preparing"}
	if !change.valid() || !pickupTerminalOperation(change.Operation) {
		t.Fatal("valid scoped first kitchen transition was rejected")
	}
	mixed := change
	mixed.Booking = other
	if mixed.valid() {
		t.Fatal("seat booking fields accepted on a pickup transition")
	}
	change.Target = "confirmed"
	if change.valid() {
		t.Fatal("terminal can claim a refund was confirmed")
	}
}
