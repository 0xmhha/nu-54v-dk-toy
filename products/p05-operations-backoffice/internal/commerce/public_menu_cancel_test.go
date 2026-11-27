package commerce

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
)

func TestPostgresPublicMenuCancellationFencesHandoff(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a disposable migrated PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(config.Host, "/tmp/") || strings.HasPrefix(config.Host, "/private/tmp/")) {
		t.Fatal("requires local scratch PostgreSQL")
	}
	ctx, finish := context.WithTimeout(context.Background(), 15*time.Second)
	defer finish()
	db, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	execute := func(statement string, values ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, statement, values...); err != nil {
			t.Fatal(err)
		}
	}
	const login = "k09_public_cancel_fixture"
	execute("CREATE ROLE " + pgx.Identifier{login}.Sanitize() + " LOGIN IN ROLE nu54_p05_commerce")
	defer db.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{login}.Sanitize())
	store, err := Open(ctx, config.ConnString()+" user="+login)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id := func() string { return uuidForCatalogFixture(t, ctx, db) }
	branch, wrongBranch := id(), id()
	owner, wrongOwner, phone := actor{id(), "terminal"}, actor{id(), "terminal"}, actor{id(), "customer"}
	const asset = "0x2222222222222222222222222222222222222222"
	execute("INSERT INTO ledger.branch VALUES($1,$3,'Asia/Seoul','04:00'),($2,$3,'Asia/Seoul','04:00')", branch, wrongBranch, tenant)
	execute("INSERT INTO ledger.command_rule VALUES($1,'preview','fixture basis','expire fixture',interval '15 minutes')", branch)
	execute(`INSERT INTO ledger.commerce_grant VALUES
          ($1,$2,'terminal','menu',now()+interval '2 minutes'),($1,$3,'terminal','menu',now()+interval '2 minutes'),
          ($1,$4,'customer',NULL,now()+interval '2 minutes')`, branch, owner.id, wrongOwner.id, phone.id)
	execute(`INSERT INTO ledger.payer_enrollment VALUES($1,$2,decode(repeat('11',20),'hex'),1,1,now()+interval '2 minutes')`, branch, phone.id)
	execute("INSERT INTO ledger.menu_release VALUES($1,1,now()-interval '1 minute',now()+interval '2 minutes')", branch)
	execute("INSERT INTO ledger.price VALUES($1,1,'tea','hot','Hot tea',$2,6),($1,1,'tea','iced','Iced tea',$2,7)", branch, asset)
	execute("INSERT INTO ledger.stock VALUES($1,'tea',12)", branch)
	execute("INSERT INTO ledger.catalog_option_stock VALUES($1,'tea','hot',6),($1,'tea','iced',6)", branch)
	reserve := func(session, key string) (string, error) {
		cart := Cart{Origin: "menu", OriginRef: session, MenuVersion: 1,
			Items: []Item{{SKU: "tea", Option: "hot", Qty: 1}, {SKU: "tea", Option: "iced", Qty: 1}}}
		bound := authorize(t, owner, "order.create", branch, session, key,
			map[string]string{"asset": asset, "quantity": "2", "intentDigest": strings.Repeat("a", 64)})
		return store.Reserve(ctx, bound, cart)
	}
	cancel := func(who actor, domain, session, key string) error {
		return store.CancelPublicOrder(ctx, authorize(t, who, "order.cancel-public", domain, session, key, map[string]string{}))
	}
	stock := func(want int) {
		t.Helper()
		var sku, hot, iced int
		if err := db.QueryRow(ctx, `SELECT available,(SELECT available FROM ledger.catalog_option_stock WHERE branch_id=$1 AND sku='tea' AND option_key='hot'),
          (SELECT available FROM ledger.catalog_option_stock WHERE branch_id=$1 AND sku='tea' AND option_key='iced')
          FROM ledger.stock WHERE branch_id=$1 AND sku='tea'`, branch).Scan(&sku, &hot, &iced); err != nil ||
			sku != want*2 || hot != want || iced != want {
			t.Fatalf("stock expected %d/%d/%d, got %d/%d/%d: %v", want*2, want, want, sku, hot, iced, err)
		}
	}
	session, key := id(), id()
	sale, err := reserve(session, key)
	if err != nil {
		t.Fatal(err)
	}
	stock(5)
	if err := cancel(wrongOwner, branch, session, key); !errors.Is(err, service.Forbidden) {
		t.Fatalf("wrong terminal could end the original: %v", err)
	}
	if err := cancel(owner, wrongBranch, session, key); !errors.Is(err, service.Forbidden) {
		t.Fatalf("wrong branch could end the original: %v", err)
	}
	if err := cancel(owner, branch, session, id()); !errors.Is(err, service.Forbidden) {
		t.Fatalf("otherwise valid caller used a different reserve key: %v", err)
	}
	if err := cancel(owner, branch, session, key); err != nil {
		t.Fatal(err)
	}
	if err := cancel(owner, branch, session, key); err != nil {
		t.Fatal(err)
	}
	stock(6)
	if original, err := reserve(session, key); err != nil || original != sale {
		t.Fatalf("prior receipt original was changed after cancellation: %v", err)
	}
	var state string
	if err := db.QueryRow(ctx, "SELECT state FROM ledger.sale WHERE id=$1", sale).Scan(&state); err != nil || state != "closed" {
		t.Fatal("old preview did not durably close")
	}
	session, key = id(), id()
	if err := cancel(owner, branch, session, key); err != nil {
		t.Fatal(err)
	}
	if _, err := reserve(session, key); !errors.Is(err, service.Expired) {
		t.Fatalf("late create reopened a cancelled session: %v", err)
	}
	if _, err := reserve(session, id()); !errors.Is(err, service.Expired) {
		t.Fatalf("new key reopened an ended public session: %v", err)
	}
	stock(6)

	// A real phone comparison races against cancellation on the same sale row.
	session, key = id(), id()
	sale, err = reserve(session, key)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := store.RequestComparison(ctx, authorize(t, phone, "checkout.request", branch, sale, id(), map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	confirm := authorize(t, owner, "checkout.confirm", branch, sale, id(),
		map[string]string{"customerRef": phone.id, "code": pair.Code})
	fence, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fence.Rollback(context.Background())
	if _, err := fence.Exec(ctx, "SELECT id FROM ledger.sale WHERE id=$1 FOR UPDATE", sale); err != nil {
		t.Fatal(err)
	}
	confirmed, released := make(chan error, 1), make(chan error, 1)
	go func() { _, err := store.ConfirmComparison(ctx, confirm); confirmed <- err }()
	go func() { released <- cancel(owner, branch, session, key) }()
	if err := fence.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	var linked bool
	if err := db.QueryRow(ctx, "SELECT state,EXISTS(SELECT FROM ledger.checkout_claim WHERE sale_id=$1) FROM ledger.sale WHERE id=$1", sale).Scan(&state, &linked); err != nil {
		t.Fatal(err)
	}
	level := 6
	if err := <-confirmed; err == nil {
		if !linked || state != "preview" {
			t.Fatal("committed phone handoff lost its original")
		}
		level = 5
		stock(level)
	} else {
		if linked || state != "closed" {
			t.Fatalf("unclaimed preview was not released: %v", err)
		}
		stock(6)
	}
	// A bound phone continues issuing the exact original even when the
	// kiosk ends its session during the external signing round trip.
	session, key = id(), id()
	sale, err = reserve(session, key)
	if err != nil {
		t.Fatal(err)
	}
	pair, err = store.RequestComparison(ctx, authorize(t, phone, "checkout.request", branch, sale, id(), map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := store.ConfirmComparison(ctx, authorize(t, owner, "checkout.confirm", branch, sale, id(),
		map[string]string{"customerRef": phone.id, "code": pair.Code}))
	if err != nil {
		t.Fatal(err)
	}
	signKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	profile := FixtureProfile()
	profile.Asset = common.HexToAddress(asset)
	profile.Signer = crypto.PubkeyToAddress(signKey.PublicKey)
	profileDigest := profile.Digest()
	execute("INSERT INTO ledger.issuance_profile VALUES($1,1,$2,60,true)", branch, profileDigest[:])
	sign := &barrierSigner{key: signKey, start: make(chan struct{}), release: make(chan struct{})}
	secret := [32]byte{8}
	cipher, err := NewCipher(secret[:])
	if err != nil {
		t.Fatal(err)
	}
	h := HTTP{Store: store, Profiles: map[string]Profile{branch: profile}, Signer: sign, Cipher: cipher,
		PresentationKey: ed25519.NewKeyFromSeed(make([]byte, 32))}
	issue := authorize(t, phone, "quote.issue", branch, sale, id(), map[string]string{"claimDigest": claim.Digest, "orderRevision": "0"})
	type issued struct {
		reply *quoteReply
		err   error
	}
	result := make(chan issued, 1)
	go func() { answer, err := h.issue(ctx, issue); result <- issued{answer, err} }()
	select {
	case <-sign.start:
	case failed := <-result:
		t.Fatalf("phone could not reach the signing step: %v", failed.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := cancel(owner, branch, session, key); err != nil {
		close(sign.release)
		t.Fatal(err)
	}
	close(sign.release)
	firstQuote := <-result
	if firstQuote.err != nil || firstQuote.reply == nil || firstQuote.reply.Signed == "" {
		t.Fatalf("kiosk ending interfered with protected original quote: %v", firstQuote.err)
	}
	if err := cancel(owner, branch, session, key); err != nil {
		t.Fatal(err)
	}
	stock(level - 1)
	mobile, err := store.Status(ctx, authorize(t, phone, "order.status", branch, sale, "", map[string]string{}))
	if err != nil || mobile.ID != sale || mobile.State != "issued" {
		t.Fatalf("bound phone lost access to its issued original: %+v %v", mobile, err)
	}

	// Expiration while waiting on the sale lock must leave the stock held.
	session, key = id(), id()
	sale, err = reserve(session, key)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(800 * time.Millisecond)
	execute("UPDATE ledger.commerce_grant SET valid_until=$3 WHERE branch_id=$1 AND actor_id=$2", branch, owner.id, expiry)
	waiting, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer waiting.Rollback(context.Background())
	if _, err := waiting.Exec(ctx, "SELECT id FROM ledger.sale WHERE id=$1 FOR UPDATE", sale); err != nil {
		t.Fatal(err)
	}
	ended := make(chan error, 1)
	go func() { ended <- cancel(owner, branch, session, key) }()
	select {
	case <-ended:
		t.Fatal("cancel did not wait for the original order's lock")
	case <-time.After(time.Until(expiry) + 30*time.Millisecond):
	}
	if err := waiting.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-ended; !errors.Is(err, service.Forbidden) {
		t.Fatalf("expired terminal mutated checkout: %v", err)
	}
	stock(level - 2)

}
