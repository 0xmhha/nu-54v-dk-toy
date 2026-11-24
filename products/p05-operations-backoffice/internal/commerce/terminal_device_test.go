package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/networklist"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

func TestPostgresTerminalDeviceRecordsEveryChange(t *testing.T) {
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
	const site, point, other = "a1d906d0-4eb3-408c-b25c-03998b111c00", "b1d906d0-4eb3-408c-b25c-03998b111c00", "b2d906d0-4eb3-408c-b25c-03998b111c00"
	const operator, former = "c1d906d0-4eb3-408c-b25c-03998b111c00", "d1d906d0-4eb3-408c-b25c-03998b111c00"
	const first, second, third = "e1d906d0-4eb3-408c-b25c-03998b111c00", "e2d906d0-4eb3-408c-b25c-03998b111c00", "e3d906d0-4eb3-408c-b25c-03998b111c00"
	exec("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", site, tenant)
	exec("INSERT INTO ledger.login_subject VALUES ('device_fixture_operator',$1),('device_fixture_former',$2)", operator, former)
	exec("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", site, operator)
	exec("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '2 minutes',now()-interval '1 minute')", site, former)
	exec("INSERT INTO ledger.commerce_grant (branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES ($1,$2,'terminal','menu',now()+interval '1 hour'),($1,$3,'terminal','menu',now()+interval '1 hour')", site, point, other)
	exec("SET LOCAL ROLE nu54_p05_commerce")

	state := func(who, target, role string) (terminalDeviceRow, error) {
		var got terminalDeviceRow
		err := tx.QueryRow(ctx, "SELECT ledger.terminal_device_state($1,$2,$3,$4,$5)", tenant, site, who, target, role).Scan(&got)
		return got, domainError(err)
	}
	change := func(who, target, key string, expected int, name, value string) (terminalDeviceRow, error) {
		var got terminalDeviceRow
		err := tx.QueryRow(ctx, "SELECT ledger.change_terminal_device($1,$2,$3,$4,$5,$6,$7,$8,$9)", tenant, site, who, target, key, expected, name, value,
			[]byte((name + value + strings.Repeat("-", 32))[:32])).Scan(&got)
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
	if got, err := state(point, point, "terminal"); err != nil || got.Revision != 0 || got.Language != "ko" || got.IdleSeconds != 60 || got.ReadServer != "" || got.Changes != nil || got.RegistrationExpiresAt != 0 {
		t.Fatalf("first state %+v %v", got, err)
	}
	denied(service.Forbidden, func() error { _, err := state(point, other, "terminal"); return err })
	denied(service.Forbidden, func() error { _, err := change(point, point, first, 0, "language", "en"); return err })
	denied(service.Forbidden, func() error { _, err := change(former, point, first, 0, "language", "en"); return err })
	denied(service.Forbidden, func() error { _, err := change(operator, operator, first, 0, "language", "en"); return err })
	for _, bad := range [][2]string{{"language", "fr"}, {"idle-seconds", "59"}, {"idle-seconds", "x"}, {"read-server", "http://rpc.example"},
		{"read-server", "https://user@rpc.example"}, {"read-server", "https://rpc.example/?k=1"}, {"sound", "1"}} {
		denied(service.BadRequest, func() error { _, err := change(operator, point, first, 0, bad[0], bad[1]); return err })
	}
	for range 2 { // a repeated request changes nothing again
		got, err := change(operator, point, first, 0, "read-server", "https://rpc.example/stable")
		if err != nil || got.Revision != 1 || got.ReadServer != "https://rpc.example/stable" || len(got.Changes) != 1 {
			t.Fatalf("first change %+v %v", got, err)
		}
	}
	denied(service.Conflict, func() error { _, err := change(operator, point, first, 0, "language", "en"); return err })
	denied(service.Conflict, func() error {
		_, err := change(operator, other, first, 0, "read-server", "https://rpc.example/stable")
		return err
	})
	denied(service.Conflict, func() error { _, err := change(operator, point, second, 0, "language", "en"); return err })
	if got, err := change(operator, point, second, 1, "idle-seconds", "120"); err != nil || got.Revision != 2 || got.IdleSeconds != 120 || got.ReadServer == "" {
		t.Fatalf("second change %+v %v", got, err)
	}
	got, err := change(operator, point, third, 2, "read-server", "")
	if err != nil || got.Revision != 3 || got.ReadServer != "" || got.RegistrationExpiresAt < time.Now().Unix() || len(got.Changes) != 3 {
		t.Fatalf("back to the registered server %+v %v", got, err)
	}
	last := got.Changes[0]
	if last.Name != "read-server" || last.From != "https://rpc.example/stable" || last.To != "" || last.Actor != operator || last.At < time.Now().Add(-time.Minute).Unix() ||
		got.Changes[1].From != "60" || got.Changes[1].To != "120" || got.Changes[2].From != "" {
		t.Fatalf("record %+v", got.Changes)
	}
	if got, err := state(other, other, "terminal"); err != nil || got.Revision != 0 {
		t.Fatalf("another kiosk changed %+v %v", got, err)
	}
	if got, err := state(point, point, "terminal"); err != nil || got.Revision != 3 || got.IdleSeconds != 120 || got.Changes != nil {
		t.Fatalf("the kiosk reads %+v %v", got, err)
	}
	denied(service.Forbidden, func() error { _, err := state(former, point, "operator"); return err })
}

func TestNetworkListNeedsItsSignerAndTime(t *testing.T) {
	raw, err := os.ReadFile("../../../../packages/protocol/v2/testdata/network-list.vectors")
	if err != nil {
		t.Fatal(err)
	}
	v := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		key, value, _ := strings.Cut(line, "=")
		v[key] = value
	}
	path := filepath.Join(t.TempDir(), "list.json")
	publish := func(signature string) {
		t.Helper()
		body, _ := json.MarshalIndent(networklist.Envelope{Payload: v["payload"], Signature: signature}, "", "  ")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	publish(v["signature"])
	now := time.Unix(1_800_000_000, 0)
	list := NetworkList{Path: path, Operator: common.HexToAddress(v["operator"])}
	if got, err := list.Read(now); err != nil || got.Payload != v["payload"] {
		t.Fatalf("published list: %v", err)
	}
	if _, err := list.Read(now.AddDate(1, 0, 0)); err == nil {
		t.Fatal("an expired list is served")
	}
	if _, err := (NetworkList{Path: path, Operator: common.HexToAddress("0x1")}).Read(now); err == nil {
		t.Fatal("another signer's list is served")
	}
	publish("0x" + strings.Repeat("0", 130))
	if _, err := list.Read(now); err == nil {
		t.Fatal("an unsigned list is served")
	}
	bound := service.Bound{Subject: "me", Request: service.Request{Object: "me"}}
	if _, err := (HTTP{}).networkList(context.Background(), bound); !errors.Is(err, service.Capability) {
		t.Fatalf("no list: %v", err)
	}
	if _, err := (HTTP{NetworkList: &list}).networkList(context.Background(), bound); !errors.Is(err, service.Capability) {
		t.Fatalf("invalid list: %v", err)
	}
	if _, err := (HTTP{NetworkList: &list}).networkList(context.Background(), service.Bound{Subject: "you", Request: bound.Request}); !errors.Is(err, service.Forbidden) {
		t.Fatalf("another customer: %v", err)
	}
}
