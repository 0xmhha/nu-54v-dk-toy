package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The ledger's own whole-won rule (migration 127, owner decision 2026-11-27): new booking terms, subscription
// amounts and refunds of a won asset are whole won; what a cancelled booking gets back is rounded up to a whole won
// and never passes what was paid; rows stored before keep their amounts; a dollar asset keeps every rule it had.
func TestPostgresWholeWonAmounts(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a migrated disposable PostgreSQL database")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(cfg.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(cfg.Host, "/private/tmp/") || strings.HasPrefix(cfg.Host, "/tmp/")) {
		t.Fatal("use a local nu54_commerce_* disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	text := func(sql string, args ...any) string {
		t.Helper()
		var value string
		if err := admin.QueryRow(ctx, sql, args...).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	code := func(err error) string {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			return pg.Code
		}
		if err != nil {
			return err.Error()
		}
		return ""
	}
	fresh := func() string { return text("SELECT gen_random_uuid()::text") }
	// The whole unit of an asset is kept for the whole database, so each run names assets of its own.
	address := func() string { return text("SELECT '0x'||substr(md5(random()::text)||md5(random()::text),1,40)") }

	login := "whole_" + strings.TrimPrefix(cfg.Database, "nu54_commerce_")
	if strings.ContainsAny(login, " -;/") {
		t.Fatal("unsafe scratch database name")
	}
	role := pgx.Identifier{login}.Sanitize()
	exec("CREATE ROLE " + role + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+role)
	store, err := Open(ctx, dsn+" user="+login)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, shop, guest, staff := fresh(), fresh(), fresh(), fresh()
	won, coin, dollar := address(), address(), address()
	exec("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", shop, owner)
	exec("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES ($1,$2,'customer',now()+interval '5 minutes')", shop, guest)
	exec("INSERT INTO ledger.finance_grant VALUES ($1,$2,'refund_request',now()+interval '5 minutes')", shop, staff)

	slot := func(asset, deposit, balance string, depositBPS int) (string, error) {
		id := fresh()
		_, err := admin.Exec(ctx, `INSERT INTO ledger.travel_slot(branch_id,id,starts_at,capacity,terms_version,cancellation_terms,asset_ref,
			deposit,balance,deposit_refund_bps,balance_refund_bps) VALUES ($1,$2,now()+interval '2 hours',5,1,'Refund follows the terms',$3,
			$4::text::numeric,$5::text::numeric,$6,10000)`, shop, id, asset, deposit, balance, depositBPS)
		return id, err
	}
	hold := func(slot, asset, deposit, balance string) (string, error) {
		var booking string
		err := store.db.QueryRow(ctx, `SELECT ledger.travel_hold($1,$2,$3,$4,$5,1,1,$6,$7,$8)::text`,
			owner, shop, guest, fresh(), slot, deposit, balance, asset).Scan(&booking)
		return booking, err
	}
	// A paid deposit, seeded: the leg and its order as the customer starts them, then a quote and its payment.
	pay := func(booking, asset, principal string) string {
		t.Helper()
		var sale string
		if err := store.db.QueryRow(ctx, `SELECT ledger.travel_start_leg($1,$2,$3,$4,'deposit')::text`, owner, shop, guest, booking).Scan(&sale); err != nil {
			t.Fatal(err)
		}
		exec("SET session_replication_role=replica")
		exec(`UPDATE ledger.sale SET state='paid' WHERE id=$1`, sale)
		exec(`INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,
			payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
			VALUES($1,$2,sha256(random()::text::bytea),sha256(random()::text::bytea),sha256(random()::text::bytea),sha256(random()::text::bytea),0,1,
			substring(sha256(random()::text::bytea) FROM 1 FOR 20),substring(sha256(random()::text::bytea) FROM 1 FOR 20),1,1,now(),
			now()+interval '1 minute',1,now(),'x',sha256(random()::text::bytea))`, sale, guest)
		exec(`INSERT INTO ledger.payment(sale_id,stream_id,merchant_key,evidence_id,tx_hash,log_index,inclusion_hash,included_at,asset_ref,payer,
			payer_generation,payment_mode,principal) VALUES($1,gen_random_uuid(),'0x'||repeat('ab',32),gen_random_uuid(),
			'0x'||encode(sha256(random()::text::bytea),'hex'),0,'0x'||repeat('ef',32),now(),$2,'0x'||repeat('12',20),1,1,$3::text::numeric)`, sale, asset, principal)
		exec("SET session_replication_role=origin")
		return sale
	}
	expected := func(booking string) string {
		t.Helper()
		var raw []byte
		if err := store.db.QueryRow(ctx, `SELECT ledger.travel_cancellation_preview($1,$2,$3,$4)`, owner, shop, guest, booking).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var preview TravelCancellationPreview
		if err := json.Unmarshal(raw, &preview); err != nil || len(preview.Legs) != 1 || preview.Legs[0].RefundExpected == nil {
			t.Fatalf("preview %s: %v", raw, err)
		}
		return *preview.Legs[0].RefundExpected
	}
	progress := func(booking string) (string, string) {
		t.Helper()
		var due, state string
		if err := store.db.QueryRow(ctx, `SELECT refund_expected,refund_state FROM ledger.travel_legs_with_refunds($1,$2,$3,$4)`,
			owner, shop, guest, booking).Scan(&due, &state); err != nil {
			t.Fatal(err)
		}
		return due, state
	}
	cancelBooking := func(booking string) {
		t.Helper()
		if _, err := store.db.Exec(ctx, `SELECT ledger.travel_cancel($1,$2,$3,$4)`, owner, shop, guest, booking); err != nil {
			t.Fatal(err)
		}
	}
	// The operator's refund request, as the finance console makes it.
	refund := func(sale, amount string) string {
		_, err := admin.Exec(ctx, `SELECT ledger.reserve_console_refund($1,$2,$3,$4,gen_random_uuid(),$5::text::numeric,'merchant_cancel')`,
			owner, shop, staff, sale, amount)
		return code(err)
	}
	offer := func(asset, amount string) (string, error) {
		id := fresh()
		_, err := admin.Exec(ctx, `INSERT INTO ledger.subscription_offer(id,branch_id,label,terms_version,terms_digest,cadence,asset_ref,amount,fulfillment_kind)
			VALUES ($1,$2,'Weekly',$5,sha256('terms'),'weekly',$3,$4::text::numeric,'physical')`, id, shop, asset, amount, id)
		return id, err
	}
	accept := func(offer string) error {
		_, err := store.db.Exec(ctx, `SELECT ledger.accept_subscription($1,$2,$3,$4,sha256('terms'),$5)`, owner, shop, guest, offer, fresh())
		return err
	}
	must := func(id string, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	// --- stored before the rule: terms and a payment with a part of a won, and one subscription already accepted ---
	halfSlot := must(slot(won, "3333300", "0", 5000)) // 33,333 won, half back
	oldSlot := must(slot(won, "3333350", "0", 10000)) // 33,333.50 won, all back
	coinSlot := must(slot(coin, "33333"+strings.Repeat("0", 18), "0", 5000))
	dollarSlot := must(slot(dollar, "33333", "1", 5000)) // 0.033333 dollars, half back
	half := must(hold(halfSlot, won, "3333300", "0"))
	parts := must(hold(halfSlot, won, "3333300", "0"))
	old := must(hold(oldSlot, won, "3333350", "0"))
	coins := must(hold(coinSlot, coin, "33333"+strings.Repeat("0", 18), "0"))
	dollars := must(hold(dollarSlot, dollar, "33333", "1"))
	halfSale, partsSale := pay(half, won, "3333300"), pay(parts, won, "3333300")
	oldSale, coinSale, dollarSale := pay(old, won, "3333350"), pay(coins, coin, "33333"+strings.Repeat("0", 18)), pay(dollars, dollar, "33333")
	oldOffer := must(offer(won, "990050")) // 9,900.50 won each week
	keptOffer := must(offer(won, "990025"))
	if err := accept(keptOffer); err != nil {
		t.Fatal(err)
	}

	// --- opsd tells the ledger what one won is: once, the same again, and never another unit ---
	wonToken := Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}
	coinToken := Token{Name: "WKRC", Symbol: "WKRC", Label: "KRWC", Decimals: 18, BoardDecimals: 2}
	profiles := map[string]Profile{"won": {Asset: common.HexToAddress(won), Token: wonToken},
		"coin": {Asset: common.HexToAddress(coin), Token: coinToken}, "dollar": {Asset: common.HexToAddress(dollar)}}
	for range 2 {
		if err := store.DeclareWholeAmounts(ctx, profiles); err != nil {
			t.Errorf("declare: %v", err)
		}
	}
	defer admin.Exec(context.Background(), "DELETE FROM ledger.whole_amount_asset WHERE asset_ref IN ($1,$2)", won, coin)
	var units string
	if err := admin.QueryRow(ctx, `SELECT string_agg(unit::text,',' ORDER BY unit) FROM ledger.whole_amount_asset WHERE asset_ref IN ($1,$2,$3)`,
		won, coin, dollar).Scan(&units); err != nil || units != "100,1"+strings.Repeat("0", 18) {
		t.Errorf("declared units %q: %v", units, err)
	}
	if err := store.DeclareWholeAmounts(ctx, map[string]Profile{"won": {Asset: common.HexToAddress(won), Token: coinToken}}); code(err) != "P1001" {
		t.Errorf("another unit for a declared asset: %v", err)
	}
	if _, err := store.db.Exec(ctx, "INSERT INTO ledger.whole_amount_asset VALUES ($1,100)", address()); code(err) != "42501" {
		t.Errorf("the commerce role wrote the table itself: %v", err)
	}

	// --- new amounts are whole won; a dollar asset takes what it took ---
	const refusedAmount = "23514"
	if _, err := slot(won, "3333350", "0", 5000); code(err) != refusedAmount {
		t.Errorf("a new slot with a deposit of 33,333.50 won: %v", err)
	}
	if _, err := slot(won, "3333300", "1000001", 5000); code(err) != refusedAmount {
		t.Errorf("a new slot with a balance of 10,000.01 won: %v", err)
	}
	if _, err := slot(coin, "33333"+strings.Repeat("0", 17)+"1", "0", 5000); code(err) != refusedAmount {
		t.Errorf("a new slot with a part of a won of the coin: %v", err)
	}
	if _, err := slot(won, "3333300", "1000000", 5000); err != nil {
		t.Errorf("a new slot in whole won: %v", err)
	}
	if _, err := slot(dollar, "3333350", "1000001", 5000); err != nil {
		t.Errorf("a dollar slot: %v", err)
	}
	// The stored slot keeps its terms and may change what is not an amount; its amounts may only become whole.
	if _, err := admin.Exec(ctx, "UPDATE ledger.travel_slot SET capacity=6 WHERE id=$1", oldSlot); err != nil {
		t.Errorf("a stored slot could not change its capacity: %v", err)
	}
	if _, err := admin.Exec(ctx, "UPDATE ledger.travel_slot SET deposit=3333360,terms_version=2 WHERE id=$1", oldSlot); code(err) != refusedAmount {
		t.Errorf("a stored slot changed to another part of a won: %v", err)
	}
	// No new booking copies a part of a won; the one already made is still there.
	if _, err := hold(oldSlot, won, "3333350", "0"); code(err) != refusedAmount {
		t.Errorf("a new hold of 33,333.50 won: %v", err)
	}
	if got := text("SELECT deposit::text FROM ledger.travel_booking WHERE id=$1", old); got != "3333350" {
		t.Errorf("the booking made before reads %s", got)
	}
	if _, err := offer(won, "990050"); code(err) != refusedAmount {
		t.Errorf("a new offer of 9,900.50 won: %v", err)
	}
	wholeOffer, err := offer(won, "990000")
	if err != nil {
		t.Errorf("a new offer in whole won: %v", err)
	}
	if _, err := offer(dollar, "990050"); err != nil {
		t.Errorf("a dollar offer: %v", err)
	}
	if err := accept(oldOffer); code(err) != refusedAmount {
		t.Errorf("a new acceptance of 9,900.50 won: %v", err)
	}
	if err := accept(wholeOffer); err != nil {
		t.Errorf("an acceptance in whole won: %v", err)
	}
	if got := text("SELECT amount::text FROM ledger.subscription WHERE offer_id=$1", keptOffer); got != "990025" {
		t.Errorf("the subscription accepted before reads %s", got)
	}

	// --- what a cancelled booking gets back: rounded up to a whole won, never more than was paid ---
	for _, c := range []struct {
		principal string
		bps       int
		asset     string
		due       string
	}{
		{"3333300", 5000, won, "1666700"},  // half of 33,333 won is 16,666.50: 16,667 won
		{"3333300", 10000, won, "3333300"}, // all of it
		{"3333300", 0, won, "0"},
		{"100", 1, won, "100"},             // a ten-thousandth of one won: one won
		{"1000000", 3333, won, "333300"},   // 3,333 won exactly
		{"3333350", 5000, won, "1666700"},  // half of a stored 33,333.50 won is 16,666.75: 16,667 won
		{"3333350", 10000, won, "3333350"}, // 33,334 won would pass what was paid
		{"3333350", 9999, won, "3333100"},  // 33,330.17 won: 33,331 won
		{"50", 10000, won, "50"},           // half a won was paid: no more comes back
		{"33333" + strings.Repeat("0", 18), 5000, coin, "16667" + strings.Repeat("0", 18)},
		{"33333", 5000, dollar, "16666"}, // dollars are cut off at the base unit, as before
		{"3333350", 9999, dollar, "3333016"},
	} {
		if got := text("SELECT ledger.travel_refund_due($1::text::numeric,$2::integer,$3::text)::text", c.principal, c.bps, c.asset); got != c.due {
			t.Errorf("%d/10000 of %s: %s, want %s", c.bps, c.principal, got, c.due)
		}
	}
	for _, c := range []struct{ booking, due string }{{half, "1666700"}, {parts, "1666700"}, {old, "3333350"},
		{coins, "16667" + strings.Repeat("0", 18)}, {dollars, "16666"}} {
		if got := expected(c.booking); got != c.due {
			t.Errorf("the customer is shown %s before cancelling, want %s", got, c.due)
		}
		cancelBooking(c.booking)
		if got, state := progress(c.booking); got != c.due || state != "waiting-refund" {
			t.Errorf("the customer is shown %s (%s) after cancelling, want %s", got, state, c.due)
		}
	}

	// --- the refund the operator asks for is that number: one request, or several that never pass it ---
	const notWhole, pastTerms, pastPaid = "P1030", "P1003", "P1030"
	if got := refund(halfSale, "1666650"); got != notWhole {
		t.Errorf("a refund of 16,666.50 won: %q", got)
	}
	if got := refund(halfSale, "1666800"); got != pastTerms {
		t.Errorf("a refund of 16,668 won: %q", got)
	}
	if got := refund(halfSale, "1666700"); got != "" {
		t.Errorf("the refund of 16,667 won: %q", got)
	}
	if got := refund(halfSale, "100"); got != pastTerms {
		t.Errorf("one won more than the terms give: %q", got)
	}
	for _, c := range []struct{ amount, want string }{{"1000000", ""}, {"666650", notWhole}, {"666700", ""}, {"100", pastTerms}} {
		if got := refund(partsSale, c.amount); got != c.want {
			t.Errorf("a part of %s: %q, want %q", c.amount, got, c.want)
		}
	}
	if got := text("SELECT (reserved+in_flight+refunded)::text FROM ledger.payment WHERE sale_id=$1", partsSale); got != "1666700" {
		t.Errorf("the parts add up to %s", got)
	}
	// All of a payment stored with a part of a won: nothing above it, and the rest of it only in one piece.
	if got := refund(oldSale, "3333400"); got != pastPaid {
		t.Errorf("33,334 won of 33,333.50 paid: %q", got)
	}
	if got := refund(oldSale, "50"); got != notWhole {
		t.Errorf("half a won that is not the rest: %q", got)
	}
	if got := refund(oldSale, "3333300"); got != "" {
		t.Errorf("33,333 won of 33,333.50: %q", got)
	}
	if got := refund(oldSale, "50"); got != "" {
		t.Errorf("the half won that is left: %q", got)
	}
	if got := refund(coinSale, "16667"+strings.Repeat("0", 18)); got != "" {
		t.Errorf("16,667 won of the coin: %q", got)
	}
	if got := refund(dollarSale, "16667"); got != pastTerms {
		t.Errorf("a dollar refund above the cut-off amount: %q", got)
	}
	if got := refund(dollarSale, "16666"); got != "" {
		t.Errorf("the dollar refund: %q", got)
	}

	// --- the booking is over when exactly that amount went back ---
	released := func(booking, sale, refunded string) bool {
		t.Helper()
		exec("UPDATE ledger.payment SET reserved=0,in_flight=0,refunded=$2::text::numeric WHERE sale_id=$1", sale, refunded)
		if _, err := store.db.Exec(ctx, `SELECT ledger.travel_reconcile_owned($1,$2,$3,$4)`, owner, shop, guest, booking); err != nil {
			t.Fatal(err)
		}
		return text("SELECT (released_at IS NOT NULL)::text FROM ledger.travel_booking WHERE id=$1", booking) == "true"
	}
	if released(half, halfSale, "1666600") {
		t.Error("the booking ended with 16,666 won back")
	}
	if !released(half, halfSale, "1666700") {
		t.Error("the booking did not end with 16,667 won back")
	}
	if got, state := progress(half); got != "1666700" || state != "settled" {
		t.Errorf("after the refund the customer is shown %s (%s)", got, state)
	}
	if !released(dollars, dollarSale, "16666") {
		t.Error("the dollar booking did not end with its cut-off refund")
	}
}
