package commerce

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
)

func TestPostgresTerminalModeUsesActiveOperatorAndRevision(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a migrated disposable PostgreSQL database")
	}
	conf, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(conf.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(conf.Host, "/private/tmp/") || strings.HasPrefix(conf.Host, "/tmp/")) {
		t.Fatal("use a local nu54_commerce_* disposable PostgreSQL database")
	}
	ctx, stop := context.WithTimeout(context.Background(), 12*time.Second)
	defer stop()
	admin, err := pgx.ConnectConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	const site = "a1d906d0-4eb3-408c-b25c-03998b111b00"
	const point = "b1d906d0-4eb3-408c-b25c-03998b111b00"
	const operator = "c1d906d0-4eb3-408c-b25c-03998b111b00"
	const wrong = "d1d906d0-4eb3-408c-b25c-03998b111b00"
	const key = "e1d906d0-4eb3-408c-b25c-03998b111b00"
	const switchKey = "e2d906d0-4eb3-408c-b25c-03998b111b00"
	const source = "e3d906d0-4eb3-408c-b25c-03998b111b00"
	const origin = "e4d906d0-4eb3-408c-b25c-03998b111b00"
	const newOrigin = "e5d906d0-4eb3-408c-b25c-03998b111b00"
	const oldPOS = "K06-old"
	const newPOS = "K06-new"
	const asset = "0xaaaa000000000000000000000000000000000000"
	exec("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", site, tenant)
	exec("INSERT INTO ledger.command_rule VALUES ($1,'test terminal mode','synthetic','expire fixture',interval '2 minutes')", site)
	exec("INSERT INTO ledger.login_subject VALUES ('mode_fixture_operator',$1)", operator)
	exec("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", site, operator)
	exec("INSERT INTO ledger.commerce_grant (branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES ($1,$2,'terminal','pos',now()+interval '1 minute')", site, point)
	exec("INSERT INTO ledger.commerce_grant (branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES ($1,$2,'terminal','menu',now()+interval '2 minutes')", site, wrong)
	exec("INSERT INTO ledger.pos_source VALUES ($1,$2,$3,now()+interval '2 minutes')", site, source, point)
	exec("INSERT INTO ledger.menu_release VALUES ($1,1,now()-interval '1 minute',now()+interval '2 minutes')", site)
	exec("INSERT INTO ledger.price VALUES ($1,1,'tea','hot','Tea',$2,7)", site, asset)
	exec("INSERT INTO ledger.stock VALUES ($1,'tea',3)", site)
	exec("SET LOCAL ROLE nu54_p05_commerce")
	var priorPOS string
	if err := tx.QueryRow(ctx, "SELECT ledger.submit_pos_order($1,$2,$3,$4,$5,7)::text", tenant, site, source, oldPOS, asset).Scan(&priorPOS); err != nil {
		t.Fatal(err)
	}

	state := func(who, role string) (TerminalMode, error) {
		t.Helper()
		var got TerminalMode
		err := tx.QueryRow(ctx, "SELECT mode,revision FROM ledger.terminal_mode_state($1,$2,$3,$4,$5)",
			tenant, site, who, point, role).Scan(&got.Mode, &got.Revision)
		return got, domainError(err)
	}
	change := func(who, next string, revision int, requestKey string) (TerminalMode, error) {
		t.Helper()
		var got TerminalMode
		hash := []byte(strings.Repeat(next[:1], 32))
		err := tx.QueryRow(ctx, "SELECT mode,revision FROM ledger.change_terminal_mode($1,$2,$3,$4,$5,$6,$7,$8)",
			tenant, site, who, point, requestKey, revision, next, hash).Scan(&got.Mode, &got.Revision)
		return got, domainError(err)
	}
	denied := func(code service.Code, run func() error) {
		t.Helper()
		exec("SAVEPOINT denied")
		if err := run(); !errors.Is(err, code) {
			t.Fatalf("got %v, want %v", err, code)
		}
		exec("ROLLBACK TO denied")
		exec("RELEASE SAVEPOINT denied")
	}
	if got, err := state(point, "terminal"); err != nil || got != (TerminalMode{"pos", 1}) {
		t.Fatalf("initial %v %v", got, err)
	}
	denied(service.Forbidden, func() error { _, err := state(wrong, "terminal"); return err })
	denied(service.Forbidden, func() error { _, err := change(point, "menu", 1, key); return err })
	for range 2 {
		if got, err := change(operator, "menu", 1, key); err != nil || got != (TerminalMode{"menu", 2}) {
			t.Fatalf("first or retry %v %v", got, err)
		}
	}
	denied(service.Conflict, func() error { _, err := change(operator, "pos", 1, key); return err })
	denied(service.Conflict, func() error { _, err := change(operator, "pos", 1, wrong); return err })
	reserve := func(origin string) (string, error) {
		var sale string
		err := tx.QueryRow(ctx, `SELECT ledger.reserve_order($1,$2,$3,$4,'menu',$4,1,$5,
		  '[{"sku":"tea","option_key":"hot","quantity":1}]'::jsonb,'terminal')::text`,
			tenant, site, point, origin, asset).Scan(&sale)
		return sale, domainError(err)
	}
	oldSale, err := reserve(origin)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if got, err := change(operator, "pos", 2, switchKey); err != nil || got != (TerminalMode{"menu", 2}) {
			t.Fatalf("staged or retry %v %v", got, err)
		}
	}
	denied(service.Conflict, func() error { _, err := change(operator, "menu", 2, origin); return err })
	denied(service.Busy, func() error { _, err := reserve(newOrigin); return err })
	var found string
	if err := tx.QueryRow(ctx, `SELECT ledger.resume_reserve_order($1,$2,$3,$4,'menu',$4,1,$5,
	      '[{"sku":"tea","option_key":"hot","quantity":1}]'::jsonb,'terminal')::text`,
		tenant, site, point, origin, asset).Scan(&found); err != nil || found != oldSale {
		t.Fatalf("menu replay: %s %v", found, err)
	}
	if err := tx.QueryRow(ctx, "SELECT ledger.submit_pos_order($1,$2,$3,$4,$5,7)::text", tenant, site, source, oldPOS, asset).Scan(&found); err != nil || found != priorPOS {
		t.Fatalf("POS replay: %s %v", found, err)
	}
	denied(service.Forbidden, func() error {
		return domainError(tx.QueryRow(ctx, "SELECT ledger.submit_pos_order($1,$2,$3,$4,$5,7)::text", tenant, site, source, newPOS, asset).Scan(&found))
	})
	confirm := func(who, target, role, request string, version int) (TerminalSettings, error) {
		var got TerminalSettings
		err := tx.QueryRow(ctx, "SELECT ledger.confirm_terminal_transition($1,$2,$3,$4,$5,$6,$7)",
			tenant, site, who, target, role, request, version).Scan(&got)
		return got, domainError(err)
	}
	denied(service.Forbidden, func() error { _, err := confirm(wrong, point, "terminal", switchKey, 3); return err })
	denied(service.Forbidden, func() error { _, err := confirm(point, point, "terminal", switchKey, 9); return err })
	denied(service.Forbidden, func() error { _, err := confirm(point, point, "terminal", origin, 3); return err })
	denied(service.Forbidden, func() error { _, err := confirm(point, point, "operator", switchKey, 3); return err })
	got, err := confirm(point, point, "terminal", switchKey, 3)
	if err != nil || got.Transition == nil || got.Transition.State != "waiting" || !got.Transition.Ack || got.Transition.Handoff || got.Mode != "menu" || got.OrderCount != nil {
		t.Fatalf("ACK only: %+v %v", got, err)
	}
	denied(service.Busy, func() error { _, err := reserve(newOrigin); return err })
	got, err = confirm(operator, point, "operator", switchKey, 3)
	if err != nil || got.Transition == nil || got.Transition.State != "active" || !got.Transition.Handoff || got.Mode != "pos" || got.Revision != 3 || got.OrderCount == nil || *got.OrderCount != 2 || !slices.Contains(got.OrderRefs, oldSale) {
		t.Fatalf("both gates and retained queue: %+v %v", got, err)
	}
	got, err = confirm(point, point, "terminal", switchKey, 3)
	if err != nil || got.Revision != 3 {
		t.Fatalf("ACK replay: %+v %v", got, err)
	}
	denied(service.Forbidden, func() error { _, err := reserve(newOrigin); return err })
	if err := tx.QueryRow(ctx, "SELECT ledger.submit_pos_order($1,$2,$3,$4,$5,7)::text", tenant, site, source, newPOS, asset).Scan(&found); err != nil || found == priorPOS {
		t.Fatalf("new POS order: %s %v", found, err)
	}
	exec("RESET ROLE")
	exec("UPDATE ledger.staff_grant SET revoked_at=statement_timestamp() WHERE branch_id=$1", site)
	exec("SET LOCAL ROLE nu54_p05_commerce")
	denied(service.Forbidden, func() error { _, err := change(operator, "menu", 3, wrong); return err })
	if got, err := state(point, "terminal"); err != nil || got != (TerminalMode{"pos", 3}) {
		t.Fatalf("revocation disturbed retained mode: %v %v", got, err)
	}
}

func TestPostgresTerminalTransitionRejectsOperatorRevokedDuringLock(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a migrated disposable PostgreSQL database")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(config.Host, "/private/tmp/") || strings.HasPrefix(config.Host, "/tmp/")) {
		t.Fatal("use a local nu54_commerce_* disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	const site = "a1d906d0-4eb3-408c-b25c-03998b111b10"
	const operator = "c1d906d0-4eb3-408c-b25c-03998b111b10"
	const point = "b1d906d0-4eb3-408c-b25c-03998b111b10"
	const request = "e1d906d0-4eb3-408c-b25c-03998b111b10"
	run := func(q string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	run("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", site, tenant)
	run("INSERT INTO ledger.login_subject VALUES ('mode_revoke_lock',$1)", operator)
	run("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", site, operator)
	run("INSERT INTO ledger.commerce_grant (branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES ($1,$2,'terminal','menu',now()+interval '2 minutes')", site, point)
	run("INSERT INTO ledger.command_rule VALUES ($1,'K06 lock fixture','synthetic','dispose',interval '2 minutes')", site)
	revoke, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer revoke.Rollback(context.Background())
	if _, err = revoke.Exec(ctx, "SELECT FROM ledger.staff_grant WHERE branch_id=$1 AND subject_ref=$2 FOR UPDATE", site, operator); err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["application_name"] = "nu54_k06_blocked_mode"
	caller, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer caller.Close(context.Background())
	if _, err = caller.Exec(ctx, "SET ROLE nu54_p05_commerce"); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		var settings TerminalSettings
		result <- caller.QueryRow(ctx, `SELECT ledger.request_terminal_transition($1,$2,$3,$4,$5,1,'pos',sha256('fixture'))`,
			tenant, site, operator, point, request).Scan(&settings)
	}()
	for {
		var blocked bool
		if err = revoke.QueryRow(ctx, `SELECT EXISTS (SELECT FROM pg_stat_activity
		  WHERE application_name='nu54_k06_blocked_mode' AND wait_event_type='Lock')`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		if _, err = revoke.Exec(ctx, "SELECT pg_stat_clear_snapshot()"); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-result:
			t.Fatalf("request did not wait for the grant lock: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err = revoke.Exec(ctx, "UPDATE ledger.staff_grant SET revoked_at=clock_timestamp() WHERE branch_id=$1 AND subject_ref=$2", site, operator); err != nil {
		t.Fatal(err)
	}
	if err = revoke.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = domainError(<-result); !errors.Is(err, service.Forbidden) {
		t.Fatalf("revoked operator continued after the grant lock: %v", err)
	}
	var rows int
	if err = admin.QueryRow(ctx, "SELECT count(*) FROM ledger.terminal_transition WHERE branch_id=$1", site).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("revoked operator left a transition: %d %v", rows, err)
	}
}
