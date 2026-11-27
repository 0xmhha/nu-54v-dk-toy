package commerce

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresRentalEvidenceWriterScope(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires migrated disposable PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(config.Host, "/tmp/") || strings.HasPrefix(config.Host, "/private/tmp/")) {
		t.Fatal("requires named disposable PostgreSQL on a Unix socket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	db, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	const branch = "33a20b32-735b-40bd-bb62-597458360ef9"
	const tenant = "7400fa9c-5ada-4621-be94-eaa5b0dc20a3"
	const actor = "0f3b739c-fb09-4568-9862-e61e411b898b"
	const device = "443c843c-5b57-4696-ab18-bbaa7895e761"
	const stream = "55a20b32-735b-40bd-bb62-597458360ef9"
	const generation = "643c843c-5b57-4696-ab18-bbaa7895e761"
	const account = "0x1111111111111111111111111111111111111111"
	const payout = "0x2222222222222222222222222222222222222222"
	const signer = "0x3333333333333333333333333333333333333333"
	const settlement = "0x4444444444444444444444444444444444444444"
	const writer = "rental_070_writer_fixture"
	const other = "rental_070_other_fixture"
	const commerce = "rental_070_commerce_fixture"
	role := func(login, capability string) {
		t.Helper()
		_, err := db.Exec(ctx, "CREATE ROLE "+pgx.Identifier{login}.Sanitize()+" LOGIN IN ROLE "+pgx.Identifier{capability}.Sanitize())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, err := db.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{login}.Sanitize())
			if err != nil {
				t.Logf("fixture role removed with its scratch cluster: %v", err)
			}
		})
	}
	role(writer, "nu54_p05_rental_evidence")
	role(other, "nu54_p05_rental_evidence")
	role(commerce, "nu54_p05_commerce")
	login := func(user string) *pgx.Conn {
		t.Helper()
		params := config.Copy()
		params.User = user
		conn, err := pgx.ConnectConfig(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	auth, unrelated, staff, staffReplay := login(writer), login(other), login(commerce), login(commerce)
	defer auth.Close(context.Background())
	defer unrelated.Close(context.Background())
	defer staff.Close(context.Background())
	defer staffReplay.Close(context.Background())
	sum := func(n byte) []byte { return bytes.Repeat([]byte{n}, 32) }
	nonce := func(n byte) []byte { return append(make([]byte, 31), n) }
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	exec("INSERT INTO ledger.login_subject VALUES('rental_070_fixture',$1)", actor)
	exec("INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage', now()+interval '10 minutes')", branch, actor)
	exec(`INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
		VALUES($1,$2,1,$3,$4,'qualified',$5,now()+interval '10 minutes',now()-interval '1 minute')`,
		branch, device, account, payout, sum(1))
	const record = `SELECT ledger.record_rental_generation_proof($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,
		$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)`
	now := time.Now().Truncate(time.Second)
	args := []any{branch, device, int64(1), "72a20b32-735b-40bd-bb62-597458360ef9", sum(1), sum(2), signer, settlement, sum(3),
		stream, generation, sum(4), sum(5), sum(6), sum(7), nonce(0), nonce(2), 1, 1, 0, 1, 0,
		int64(100), sum(8), now.Add(-20 * time.Second), now.Add(90 * time.Second), now.Add(-10 * time.Second), now.Add(60 * time.Second), nil, nil}
	check := func(conn *pgx.Conn, parameters []any, want bool, code string) {
		t.Helper()
		var created bool
		err := conn.QueryRow(ctx, record, parameters...).Scan(&created)
		if code != "" {
			var postgres *pgconn.PgError
			if !errors.As(err, &postgres) || postgres.Code != code {
				t.Fatalf("wanted %s, got %t %v", code, created, err)
			}
		} else if err != nil || created != want {
			t.Fatalf("wanted committed=%t, got %t %v", want, created, err)
		}
	}
	// A dedicated role alone cannot register itself or write a count.
	check(auth, args, false, "42501")
	if _, err := auth.Exec(ctx, "SELECT * FROM ledger.rental_generation_source"); err == nil {
		t.Fatal("writer could read the source table")
	}
	exec(`INSERT INTO ledger.rental_generation_source VALUES($1,$2,1,$3,$4,$5,'qualified',$6,$7,$8,$9,$10,$11,$12,
		now()+interval '100 seconds',$13,now()+interval '3 minutes')`,
		branch, device, sum(1), account, payout, sum(2), signer, settlement, sum(3), stream, generation, sum(4), writer)
	check(unrelated, args, false, "42501")
	check(staff, args, false, "42501")
	mutate := func(index int, value any) []any {
		out := append([]any(nil), args...)
		out[index] = value
		return out
	}
	check(auth, mutate(10, "743c843c-5b57-4696-ab18-bbaa7895e761"), false, "P0501")
	check(auth, mutate(16, nonce(3)), false, "P0501")
	check(auth, args, true, "")
	check(auth, args, false, "")
	check(auth, mutate(20, 0), false, "P0501")
	check(auth, mutate(3, "82a20b32-735b-40bd-bb62-597458360ef9"), false, "P0501")
	exec(`UPDATE ledger.rental_generation_source SET challenge=$3,challenge_until=now()+interval '110 seconds'
		WHERE branch_id=$1 AND device_ref=$2`, branch, device, sum(10))
	nextPending := mutate(3, "92a20b32-735b-40bd-bb62-597458360ef9")
	nextPending[11], nextPending[12], nextPending[14] = sum(10), sum(11), sum(12)
	nextPending[28], nextPending[29] = int64(100), sum(8)
	for index, value := range map[int]any{13: sum(25), 22: int64(99), 23: sum(26), 26: now.Add(-11 * time.Second), 28: nil, 29: sum(19)} {
		conflicting := append([]any(nil), nextPending...)
		conflicting[index] = value
		check(auth, conflicting, false, "P0501")
	}
	resurrected := append([]any(nil), nextPending...)
	resurrected[20], resurrected[21] = 0, 1
	check(auth, resurrected, false, "P0501")
	check(auth, nextPending, true, "")
	type review struct {
		Ref     string `json:"returnReviewRef"`
		Pending struct {
			State    string `json:"state"`
			Count    *int   `json:"pendingCount"`
			Payments *int   `json:"pendingPayments"`
			Limits   *int   `json:"pendingLimits"`
		} `json:"generationPending"`
	}
	current := func() review {
		t.Helper()
		var raw []byte
		err := staff.QueryRow(ctx, "SELECT ledger.rental_generation_authorized_review($1,$2,$3,$4,1)",
			tenant, branch, actor, device).Scan(&raw)
		var result review
		if err != nil || json.Unmarshal(raw, &result) != nil {
			t.Fatalf("read review: %v", err)
		}
		return result
	}
	got := current()
	if got.Ref != "" || got.Pending.State != "pending" || got.Pending.Count == nil || *got.Pending.Count != 1 ||
		got.Pending.Payments == nil || *got.Pending.Payments != 1 || got.Pending.Limits == nil || *got.Pending.Limits != 0 {
		t.Fatalf("authorized view lost the exact pending payment: %+v", got)
	}
	var decisions string
	if err := db.QueryRow(ctx, `SELECT string_agg(pending_payments||'/'||pending_limits,',' ORDER BY request_key)
		FROM ledger.rental_generation_authorized_proof WHERE branch_id=$1`, branch).Scan(&decisions); err != nil || decisions != "1/0,1/0" {
		t.Fatalf("distinct pending counts were lost: %q %v", decisions, err)
	}
	const returnKey = "b2a20b32-735b-40bd-bb62-597458360ef9"
	closeWith := func(connection *pgx.Conn, ref string) (bool, error) {
		var created bool
		err := connection.QueryRow(ctx, `SELECT ledger.begin_rental_action($1,$2,$3,$4,1,'close',$5,NULL,$6)`,
			tenant, branch, actor, device, returnKey, ref).Scan(&created)
		return created, err
	}
	conflict := func(err error) {
		t.Helper()
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "P0501" {
			t.Fatalf("expected stale or held review, got %v", err)
		}
	}
	_, err = closeWith(staff, "unverified")
	conflict(err)

	// Refreshing readiness uses another challenge and receipt. It retains old
	// observations but never permits replaying their inputs under a new key.
	rotate := func(conn interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	}, challenge byte) {
		t.Helper()
		if _, err := conn.Exec(ctx, `UPDATE ledger.rental_generation_source SET challenge=$3
			WHERE branch_id=$1 AND device_ref=$2`, branch, device, sum(challenge)); err != nil {
			t.Fatal(err)
		}
	}
	nextProof := func(request string, challenge, freeze, assessment byte) []any {
		q := append([]any(nil), args...)
		q[3], q[11], q[12], q[14], q[20] = request, sum(challenge), sum(freeze), sum(assessment), 0
		q[22], q[23], q[26], q[28], q[29] = int64(101), sum(31), now.Add(-9*time.Second), int64(100), sum(8)
		return q
	}
	rotate(db, 21)
	if got := current(); got.Pending.State != "unknown" || got.Pending.Count != nil {
		t.Fatal("superseded challenge released a count")
	}
	zero := nextProof("c2a20b32-735b-40bd-bb62-597458360ef9", 21, 22, 23)
	check(auth, zero, true, "")
	ready := current()
	if ready.Ref == "" || ready.Pending.State != "verified" || ready.Pending.Count == nil || *ready.Pending.Count != 0 {
		t.Fatalf("fresh same-generation 0 was not available: %+v", ready)
	}

	// The review token is rechecked inside the same device lock as enrollment
	// and writer updates, even when the caller started with a valid token.
	refresh, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer refresh.Rollback(context.Background())
	if _, err := refresh.Exec(ctx, "SELECT ledger.lock_rental_evidence_device($1,$2)", branch, device); err != nil {
		t.Fatal(err)
	}
	rotate(refresh, 24)
	late := make(chan error, 1)
	go func() { _, err := closeWith(staff, ready.Ref); late <- err }()
	select {
	case <-late:
		t.Fatal("close bypassed the device fence")
	case <-time.After(50 * time.Millisecond):
	}
	if err := refresh.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	conflict(<-late)
	newZero := nextProof("d2a20b32-735b-40bd-bb62-597458360ef9", 24, 25, 26)
	newZero[28], newZero[29] = int64(101), sum(31)
	check(auth, newZero, true, "")
	approved := current()
	if approved.Ref == "" || approved.Ref == ready.Ref {
		t.Fatal("fresh appraisal did not rotate close review")
	}
	type reservation struct {
		created bool
		err     error
	}
	closed := make(chan reservation, 2)
	for _, conn := range []*pgx.Conn{staff, staffReplay} {
		go func(c *pgx.Conn) { yes, err := closeWith(c, approved.Ref); closed <- reservation{yes, err} }(conn)
	}
	first, second := <-closed, <-closed
	if first.err != nil || second.err != nil || first.created == second.created {
		t.Fatalf("the same close dispatched more than once: %+v %+v", first, second)
	}

	// A normal next-generation insertion acquires the same fence by trigger.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,
		enrollment_digest,attested_until,attested_at)
		VALUES($1,$2,2,'0x5555555555555555555555555555555555555555',$3,'qualified',$4,now()+interval '1 hour',now())`,
		branch, device, payout, sum(15)); err != nil {
		t.Fatal(err)
	}
	rotate(tx, 45)
	next := nextProof("a2a20b32-735b-40bd-bb62-597458360ef9", 45, 16, 17)
	next[28], next[29] = int64(101), sum(31)
	wait := make(chan error, 1)
	go func() {
		var created bool
		wait <- auth.QueryRow(ctx, record, next...).Scan(&created)
	}()
	select {
	case <-wait:
		t.Fatal("writer raced past the new enrollment fence")
	case <-time.After(50 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var pg *pgconn.PgError
	if err := <-wait; !errors.As(err, &pg) || pg.Code != "P0501" {
		t.Fatalf("retired generation received a new proof: %v", err)
	}
	check(auth, nextPending, false, "")
	// A separate enrolled account holds a limit approval, not a payment.
	const limitDevice = "a43c843c-5b57-4696-ab18-bbaa7895e761"
	const limitAccount = "0x7777777777777777777777777777777777777777"
	exec(`INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,
		enrollment_digest,attested_until,attested_at) VALUES($1,$2,1,$3,$4,'qualified',$5,now()+interval '1 hour',now()-interval '1 minute')`,
		branch, limitDevice, limitAccount, payout, sum(32))
	exec(`INSERT INTO ledger.rental_generation_source VALUES($1,$2,1,$3,$4,$5,'qualified',$6,$7,$8,$9,$10,$11,$12,
		now()+interval '115 seconds',$13,now()+interval '3 minutes')`,
		branch, limitDevice, sum(32), limitAccount, payout, sum(2), signer, settlement, sum(3), stream, generation, sum(33), writer)
	onlyLimit := append([]any(nil), args...)
	onlyLimit[1], onlyLimit[3], onlyLimit[4] = limitDevice, "e2a20b32-735b-40bd-bb62-597458360ef9", sum(32)
	onlyLimit[11], onlyLimit[12], onlyLimit[13], onlyLimit[14] = sum(33), sum(34), sum(35), sum(36)
	onlyLimit[20], onlyLimit[21] = 0, 1
	check(auth, onlyLimit, true, "")
	var raw []byte
	if err := staff.QueryRow(ctx, "SELECT ledger.rental_generation_authorized_review($1,$2,$3,$4,1)", tenant, branch, actor, limitDevice).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var independent review
	if json.Unmarshal(raw, &independent) != nil || independent.Pending.State != "pending" || independent.Pending.Payments == nil ||
		*independent.Pending.Payments != 0 || independent.Pending.Limits == nil || *independent.Pending.Limits != 1 {
		t.Fatalf("separate limit proof lost kind: %+v", independent)
	}
	exec(`UPDATE ledger.rental_generation_source SET challenge=$3 WHERE (branch_id,device_ref)=($1,$2)`,
		branch, limitDevice, sum(37))
	onlyLimit[3], onlyLimit[11], onlyLimit[12], onlyLimit[14] = "f2a20b32-735b-40bd-bb62-597458360ef9", sum(37), sum(38), sum(39)
	onlyLimit[21], onlyLimit[22], onlyLimit[23], onlyLimit[26], onlyLimit[28], onlyLimit[29] = 0, int64(101), sum(31), now.Add(-9*time.Second), int64(100), sum(8)
	check(auth, onlyLimit, true, "")
	if err := staff.QueryRow(ctx, "SELECT ledger.rental_generation_authorized_review($1,$2,$3,$4,1)", tenant, branch, actor, limitDevice).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &independent) != nil || independent.Pending.State != "verified" ||
		independent.Pending.Count == nil || *independent.Pending.Count != 0 {
		t.Fatalf("separate limit did not retire at later anchor: %+v", independent)
	}

	// Removing a provisioned source before any proof cannot downgrade the
	// generation to the previous manual-clearance path.
	const newerAccount = "0x5555555555555555555555555555555555555555"
	exec(`UPDATE ledger.rental_enrollment SET no_pending_until=now()+interval '30 minutes'
		WHERE (branch_id,device_ref,generation)=($1,$2,2)`, branch, device)
	exec(`INSERT INTO ledger.rental_generation_source VALUES($1,$2,2,$3,$4,$5,'qualified',$6,$7,$8,$9,$10,$11,$12,
		now()+interval '115 seconds',$13,now()+interval '3 minutes')`,
		branch, device, sum(15), newerAccount, payout, sum(2), signer, settlement, sum(3), stream, generation, sum(50), writer)
	exec(`DELETE FROM ledger.rental_generation_source WHERE (branch_id,device_ref,generation)=($1,$2,2)`, branch, device)
	if _, err := db.Exec(ctx, `UPDATE ledger.rental_enrollment SET generation_evidence_required=false
		WHERE (branch_id,device_ref,generation)=($1,$2,2)`, branch, device); err == nil {
		t.Fatal("protected generation mode was downgraded")
	}
	var legacyRef string
	if err := staff.QueryRow(ctx, `SELECT ledger.rental_member($1,$2,$3,$4,2)->>'returnReviewRef'`,
		tenant, branch, actor, device).Scan(&legacyRef); err != nil || legacyRef == "" {
		t.Fatalf("legacy fixture did not have a clearance: %v", err)
	}
	var result bool
	err = staff.QueryRow(ctx, `SELECT ledger.begin_rental_action($1,$2,$3,$4,2,'close',$5,NULL,$6)`,
		tenant, branch, actor, device, returnKey, legacyRef).Scan(&result)
	conflict(err)
	if again, err := closeWith(staff, "old-review"); err != nil || again {
		t.Fatalf("existing close recovery blocked: %t %v", again, err)
	}
}
