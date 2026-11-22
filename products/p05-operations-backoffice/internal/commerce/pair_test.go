package commerce

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
)

func TestPostgresQRPairingRequiresMatchingTerminalAndCustomer(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	conf, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(conf.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(conf.Host, "/private/tmp/") || strings.HasPrefix(conf.Host, "/tmp/")) {
		t.Fatal("use a local nu54_commerce_* fixture")
	}
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	conn, err := pgx.ConnectConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	run := func(q string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	const s = "dc624ff0-dbd4-4d8c-9815-5edf72b71443"
	const s2 = "a62c3820-95e4-4559-9cce-218707b51478"
	const shop = "72fe2ad0-8aac-453c-a506-df9ea9dc3fee"
	const guest1 = "1728d4b7-f136-4b87-89cd-26baec26804e"
	const guest2 = "22572997-f880-4c99-adbb-7dea0db07cfa"
	const point = "cfe07e14-af6f-44f9-ad36-cafc8e5f999e"
	const notOwner = "67ce6fb2-d0c5-4181-9126-56805ec73b75"
	asset := "0x" + strings.Repeat("1", 40)
	digest := []byte(strings.Repeat("d", 32))
	run("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", shop, tenant)
	run("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES ($1,$2,'terminal','pos',now()+interval '2 minutes'),($1,$3,'terminal','pos',now()+interval '2 minutes'),($1,$4,'customer',null,now()+interval '2 minutes'),($1,$5,'customer',null,now()+interval '2 minutes')", shop, point, notOwner, guest1, guest2)
	run("INSERT INTO ledger.payer_enrollment VALUES ($1,$2,$4,1,1,now()+interval '2 minutes'),($1,$3,$5,2,1,now()+interval '2 minutes')", shop, guest1, guest2, []byte(strings.Repeat("a", 20)), []byte(strings.Repeat("b", 20)))
	run("INSERT INTO ledger.menu_release VALUES($1,1,now()-interval '1 min',now()+interval '2 min')", shop)
	run("INSERT INTO ledger.sale (id,branch_id,origin,origin_ref,asset_ref,amount) VALUES ($1,$3,'menu',$1,$4,7),($2,$3,'pos',$2,$4,7)", s, s2, shop, asset)
	run("INSERT INTO ledger.order_checkout VALUES($1,$3,$4,1,now()+interval '2 min',$5),($2,$3,$4,1,now()+interval '2 min',$5)", s, s2, shop, point, digest)
	run("INSERT INTO ledger.operator_branch_label VALUES ($1,'Test coffee branch')", shop)
	run("SET LOCAL ROLE nu54_p05_commerce")
	readMenu := func(who, sale string) *MenuCheckout {
		t.Helper()
		var result *MenuCheckout
		if err := tx.QueryRow(ctx, "SELECT ledger.menu_checkout_context($1,$2,$3,$4)", tenant, shop, who, sale).Scan(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if observed := readMenu(point, s); observed == nil || observed.PayerBound ||
		observed.CustomerRef != nil || observed.BranchLabel != "Test coffee branch" {
		t.Fatalf("unexpected menu preview: %+v", observed)
	}
	if readMenu(notOwner, s) != nil || readMenu(point, s2) != nil {
		t.Fatal("menu checkout projection escaped its creator or menu scope")
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM ledger.order_status($1,$2,$3,$4)", tenant, shop, guest1, s).Scan(&count); err != nil || count != 0 {
		t.Fatalf("public sale leaked: %d %v", count, err)
	}
	for idx, user := range []string{guest1, guest2} {
		var code string
		want := []string{"a1b2c3d4", "1234fedc"}[idx]
		if err := tx.QueryRow(ctx, "SELECT code FROM ledger.request_checkout_pair($1,$2,$3,$4,$5,$6)", tenant, shop, user, s, user, want).Scan(&code); err != nil || code != want {
			t.Fatalf("pair: %s %v", code, err)
		}
	}
	confirm := func(who, guest, code string) error {
		var result []byte
		err := tx.QueryRow(ctx, "SELECT claim_digest FROM ledger.confirm_checkout_pair($1,$2,$3,$4,$5,$6,$7,$8,$9)", tenant, shop, who, s, guest, code, guest, digest, digest).Scan(&result)
		return domainError(err)
	}
	denied := func(who, guest, code string) {
		t.Helper()
		run("SAVEPOINT denied")
		if err := confirm(who, guest, code); !errors.Is(err, service.Forbidden) {
			t.Fatalf("comparison not denied: %v", err)
		}
		run("ROLLBACK TO denied")
		run("RELEASE SAVEPOINT denied")
	}
	denied(notOwner, guest1, "a1b2c3d4")
	denied(point, guest1, "1234fedc")
	if err := confirm(point, guest1, "a1b2c3d4"); err != nil {
		t.Fatal(err)
	}
	denied(point, guest2, "1234fedc")
	run("RESET ROLE")
	run("DELETE FROM ledger.checkout_pair WHERE sale_id=$1", s)
	run("SET LOCAL ROLE nu54_p05_commerce")
	if observed := readMenu(point, s); observed == nil || !observed.PayerBound ||
		observed.CustomerRef == nil || *observed.CustomerRef != guest1 {
		t.Fatalf("durable menu pairing missing after comparisons expire: %+v", observed)
	}
	if readMenu(guest1, s) != nil {
		t.Fatal("menu checkout projection leaked to the customer")
	}
	for _, check := range []struct {
		guest, sale string
		allowed     int
	}{{guest1, s, 1}, {guest2, s, 0}, {guest1, s2, 0}} {
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM ledger.read_checkout_claim($1,$2,$3,$4)", tenant, shop, check.guest, check.sale).Scan(&count); err != nil || count != check.allowed {
			t.Fatalf("wrong claim result %d %v", count, err)
		}
	}
}
