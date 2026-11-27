package commerce

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
)

func TestPostgresRegisteredPOSSelection(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	conf, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(conf.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(conf.Host, "/private/tmp/") || strings.HasPrefix(conf.Host, "/tmp/")) {
		t.Fatal("use a local nu54_commerce_* fixture")
	}
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()
	admin, err := pgx.ConnectConfig(ctx, conf)
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
	newID := func() string {
		t.Helper()
		var s string
		if err := admin.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	branch, otherBranch, terminal, other, source := newID(), newID(), newID(), newID(), newID()
	guests := []string{newID(), newID(), newID()}
	exec("INSERT INTO ledger.branch VALUES ($1,$3,'Asia/Seoul','04:00'),($2,$3,'Asia/Seoul','04:00')", branch, otherBranch, tenant)
	exec("INSERT INTO ledger.commerce_grant (branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES ($1,$3,'terminal','pos',now()+interval '2 min'),($1,$4,'terminal','pos',now()+interval '2 min'),($2,$3,'terminal','pos',now()+interval '2 min')", branch, otherBranch, terminal, other)
	exec("INSERT INTO ledger.command_rule VALUES ($1,'registered POS test','synthetic','expire fixture',interval '2 min')", branch)
	exec("INSERT INTO ledger.pos_source VALUES ($1,$2,$3,now()+interval '2 min')", branch, source, terminal)
	for i, guest := range guests {
		exec("INSERT INTO ledger.commerce_grant (branch_id,actor_id,role_code,valid_until) VALUES ($1,$2,'customer',now()+interval '2 min')", branch, guest)
		exec("INSERT INTO ledger.payer_enrollment VALUES ($1,$2,$3,$4,1,now()+interval '2 min')", branch, guest, bytes.Repeat([]byte{byte(i + 1)}, 20), i+1)
	}
	connect := func() *pgx.Conn {
		t.Helper()
		c, err := pgx.ConnectConfig(ctx, conf)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close(context.Background()) })
		if _, err := c.Exec(ctx, "SET ROLE nu54_p05_commerce"); err != nil {
			t.Fatal(err)
		}
		return c
	}
	client := connect()
	saleFor := func(ref string) (sale, handoff string) {
		t.Helper()
		if err := client.QueryRow(ctx, "SELECT ledger.submit_pos_order($1,$2,$3,$4,$5,7)::text", tenant, branch, source, ref, "0x"+strings.Repeat("a", 40)).Scan(&handoff); err != nil {
			t.Fatal(err)
		}
		if err := client.QueryRow(ctx, "SELECT sale_ref::text FROM ledger.lookup_pos_order($1,$2,$3,$4)", tenant, branch, terminal, handoff).Scan(&sale); err != nil {
			t.Fatal(err)
		}
		return
	}
	sale, handoff := saleFor("S-0142")
	independent, _ := saleFor("S-0143")
	pair := func(sale, guest, code string) (string, error) {
		var observed string
		err := client.QueryRow(ctx, "SELECT code FROM ledger.request_checkout_pair($1,$2,$3,$4,$5,$6)", tenant, branch, guest, sale, newID(), code).Scan(&observed)
		return observed, domainError(err)
	}
	nearby := func(owner, order, guest string, register bool) (*string, []byte, error) {
		var code *string
		var claim []byte
		err := client.QueryRow(ctx, "SELECT code,claim_digest FROM ledger.pos_nearby_checkout($1,$2,$3,$4,$5,$6,$7,$8)",
			tenant, branch, owner, order, guest, newID(), "aaaa0000", register).Scan(&code, &claim)
		return code, claim, domainError(err)
	}

	codes := []string{"1234abcd", "abcd1234"}
	for i, code := range codes {
		if observed, err := pair(sale, guests[i], code); err != nil || observed != code {
			t.Fatalf("candidate %d: %q %v", i, observed, err)
		}
	}
	if code, err := pair(sale, guests[0], "aaaabbbb"); err != nil || code != codes[0] {
		t.Fatalf("duplicate scan changed candidate: %q %v", code, err)
	}
	if code, claim, err := nearby(terminal, sale, guests[0], true); err != nil || code == nil || *code != codes[0] || claim != nil {
		t.Fatalf("QR/BLE duplicate did not share its candidate: %v", err)
	}
	if _, _, err := nearby(other, sale, guests[0], true); !errors.Is(err, service.Forbidden) {
		t.Fatalf("wrong terminal relayed candidate: %v", err)
	}
	if _, _, err := nearby(terminal, sale, guests[0], false); !errors.Is(err, service.Forbidden) {
		t.Fatalf("nearby resumed before selection: %v", err)
	}

	if _, err := pair(sale, guests[2], codes[0]); !errors.Is(err, service.Conflict) {
		t.Fatalf("same mark must not bind two candidates: %v", err)
	}
	if code, err := pair(independent, guests[2], codes[0]); err != nil || code != codes[0] {
		t.Fatalf("independent order cannot use same mark: %q %v", code, err)
	}
	digest := bytes.Repeat([]byte{7}, 32)
	blockedKey := newID()
	var claim []byte
	if err := client.QueryRow(ctx, "SELECT claim_digest FROM ledger.authorize_checkout($1,$2,$3,$4,$5,$6,$7,$7)", tenant, branch, terminal, sale, guests[0], blockedKey, digest).Scan(&claim); !errors.Is(domainError(err), service.Forbidden) {
		t.Fatalf("direct POS authorization: %v", err)
	}
	confirm := func(c *pgx.Conn, owner, order, guest, code, key string) ([]byte, error) {
		var claim []byte
		err := c.QueryRow(ctx, "SELECT claim_digest FROM ledger.confirm_checkout_pair($1,$2,$3,$4,$5,$6,$7,$8,$8)", tenant, branch, owner, order, guest, code, key, digest).Scan(&claim)
		return claim, domainError(err)
	}
	if _, err := confirm(client, terminal, sale, guests[0], codes[1], newID()); !errors.Is(err, service.Forbidden) {
		t.Fatalf("unexpected matched label: %v", err)
	}
	if _, err := confirm(client, other, sale, guests[0], codes[0], newID()); !errors.Is(err, service.Forbidden) {
		t.Fatalf("wrong terminal selected: %v", err)
	}
	type outcome struct {
		index int
		claim []byte
		err   error
	}
	results := make(chan outcome, 2)
	keys := []string{newID(), newID()}
	for i, code := range codes {
		c := connect()
		go func(i int, code string) {
			claim, err := confirm(c, terminal, sale, guests[i], code, keys[i])
			results <- outcome{i, claim, err}
		}(i, code)
	}
	won, lost := <-results, <-results
	if won.err != nil {
		won, lost = lost, won
	}
	if won.err != nil || !errors.Is(lost.err, service.CheckoutConflict) {
		t.Fatalf("expected one winner and one scoped conflict, got %v / %v", won.err, lost.err)
	}
	if got, err := confirm(connect(), terminal, sale, guests[won.index], codes[won.index], keys[won.index]); err != nil || !bytes.Equal(got, won.claim) {
		t.Fatalf("restart lost original claim: %v", err)
	}
	if _, err := confirm(client, terminal, independent, guests[2], codes[0], newID()); err != nil {
		t.Fatalf("independent order blocked: %v", err)
	}
	if code, claim, err := nearby(terminal, sale, guests[won.index], false); err != nil || code != nil || !bytes.Equal(claim, won.claim) {
		t.Fatalf("nearby winner did not recover original claim: %v", err)
	}
	if _, _, err := nearby(terminal, sale, guests[lost.index], true); !errors.Is(err, service.Forbidden) {
		t.Fatalf("nearby loser got scoped result: %v", err)
	}

	var count int
	if err := client.QueryRow(ctx, "SELECT count(*) FROM ledger.pending_checkout_pairs($1,$2,$3,$4)", tenant, branch, terminal, sale).Scan(&count); err != nil || count != 0 {
		t.Fatalf("bound POS exposed candidate list: %d %v", count, err)
	}
	if _, err := pair(sale, guests[2], "aaaa0000"); !errors.Is(err, service.Forbidden) {
		t.Fatalf("bound POS accepted new candidate: %v", err)
	}
	for i, guest := range guests {
		if err := client.QueryRow(ctx, "SELECT count(*) FROM ledger.read_checkout_claim($1,$2,$3,$4)", tenant, branch, guest, sale).Scan(&count); err != nil || (count == 1) != (i == won.index) {
			t.Fatalf("claim escaped winning customer: %d %v", count, err)
		}
	}
	for _, test := range []struct{ branch, terminal, want string }{{branch, terminal, codes[won.index]}, {branch, other, ""}, {otherBranch, terminal, ""}} {
		var got string
		if err := client.QueryRow(ctx, "SELECT coalesce(ledger.pos_selected_marker($1,$2,$3,$4),'')", tenant, test.branch, test.terminal, handoff).Scan(&got); err != nil || got != test.want {
			t.Fatalf("incorrect marker scope: %q %v", got, err)
		}
	}
	exec("INSERT INTO ledger.issuance_profile VALUES ($1,1,$2,60,true)", branch, digest)
	signer := bytes.Repeat([]byte{9}, 20)
	quoteID := bytes.Repeat([]byte{5}, 32)
	orderID := bytes.Repeat([]byte{6}, 32)
	nonce := bytes.Repeat([]byte{8}, 32)
	prepare := func(c *pgx.Conn, who, request, order string) (int64, []byte, error) {
		var fence int64
		var signed []byte
		err := c.QueryRow(ctx, `SELECT fence,signed_digest FROM ledger.prepare_quote(
			$1,$2,$3,$4,0,$5,$6,$6,$6,$7,$8,$9,$10)`,
			tenant, branch, who, order, request, digest, signer, quoteID, orderID, nonce).Scan(&fence, &signed)
		return fence, signed, domainError(err)
	}
	issueKey := newID()
	fence, signed, err := prepare(client, terminal, issueKey, sale)
	if err != nil || signed != nil {
		t.Fatalf("new reservation returned an issued quote: %v", err)
	}
	if _, _, err := prepare(client, guests[lost.index], newID(), sale); !errors.Is(err, service.Forbidden) {
		t.Fatalf("loser entered signer: %v", err)
	}
	if _, _, err := prepare(client, guests[won.index], newID(), sale); !errors.Is(err, service.Busy) {
		t.Fatalf("winner saw quote before signer committed: %v", err)
	}
	var state string
	if err := admin.QueryRow(ctx, "SELECT state::text FROM ledger.sale WHERE id=$1", sale).Scan(&state); err != nil || state != "preview" {
		t.Fatalf("changed before quote commit: %q %v", state, err)
	}
	var finished bool
	if err := client.QueryRow(ctx, "SELECT ledger.finish_quote($1,$2,$3,$4,$5,$5,$6,$7,$5)", tenant, branch, terminal, sale, digest, fence, bytes.Repeat([]byte{4}, 765)).Scan(&finished); err != nil || !finished {
		t.Fatalf("quote did not commit: %v", err)
	}
	if _, signed, err := prepare(connect(), terminal, issueKey, sale); err != nil || !bytes.Equal(signed, digest) {
		t.Fatalf("new connection missed committed quote: %v", err)
	}
	var records [4]int
	if err := admin.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ledger.checkout_claim WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.quote_work WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$1)`, sale).
		Scan(&records[0], &records[1], &records[2], &records[3]); err != nil || records != [4]int{1, 1, 0, 0} {
		t.Fatalf("unexpected quote-stage ledger counts: %v %v", records, err)
	}

	exec("UPDATE ledger.checkout_pair SET expires_at=now()-interval '1 second' WHERE sale_id=$1", sale)
	if _, err := confirm(client, terminal, sale, guests[lost.index], codes[lost.index], keys[lost.index]); !errors.Is(err, service.Forbidden) {
		t.Fatalf("expired candidate exposed conflict: %v", err)
	}
	var afterExpiry string
	if err := client.QueryRow(ctx, "SELECT coalesce(ledger.pos_selected_marker($1,$2,$3,$4),'')", tenant, branch, terminal, handoff).Scan(&afterExpiry); err != nil || afterExpiry != "" {
		t.Fatalf("expired marker persisted: %q %v", afterExpiry, err)
	}
}
