package authority

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommitRequiresCurrentAuthorityAfterDatabaseWait(t *testing.T) {
	url := os.Getenv("P05_AUTHORITY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set P05_AUTHORITY_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db := Pool{Pool: pool}
	table := pgx.Identifier{fmt.Sprintf("authority_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err = pool.Exec(ctx, "CREATE TABLE "+table+" (payload text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DROP TABLE "+table)

	for _, test := range []struct {
		name    string
		expiry  time.Duration
		revoke  bool
		allowed bool
	}{
		{name: "current", expiry: time.Second, allowed: true},
		{name: "expired while blocked", expiry: 60 * time.Millisecond},
		{name: "revoked while blocked", expiry: time.Second, revoke: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			busy, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer busy.Rollback(ctx)
			if _, err = busy.Exec(ctx, "LOCK TABLE "+table+" IN ACCESS EXCLUSIVE MODE"); err != nil {
				t.Fatal(err)
			}
			var active atomic.Bool
			active.Store(true)
			req := httptest.NewRequest("POST", "/", nil)
			w, r, cancel := Bound(req, time.Now().Add(test.expiry), active.Load, httptest.NewRecorder())
			defer cancel()

			result := make(chan error, 1)
			go func() {
				var saved string
				result <- db.QueryRow(r.Context(), "INSERT INTO "+table+" VALUES($1) RETURNING payload", test.name).Scan(&saved)
			}()
			time.Sleep(100 * time.Millisecond)
			if test.revoke {
				active.Store(false)
			}
			if err = busy.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-result
			if (err == nil) != test.allowed {
				t.Fatalf("commit allowed=%t: %v", test.allowed, err)
			}
			var count int
			if err = pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE payload=$1", test.name).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if (count == 1) != test.allowed {
				t.Fatalf("unauthorized row count=%d", count)
			}
			_, _ = io.WriteString(w, "private result")
		})
	}
}

func TestLiveGrantRecheckedAfterItsOwnRowLock(t *testing.T) {
	url := os.Getenv("P05_AUTHORITY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set P05_AUTHORITY_TEST_DATABASE_URL to a disposable migrated PostgreSQL database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	const tenant = "458a99cd-fc10-43b7-8104-177272121c02"
	const actor = "232d3f54-a436-43c9-aab7-b737541d27d6"
	var branch string
	if err := admin.QueryRow(ctx, "INSERT INTO ledger.branch(tenant_ref,timezone,close_local) VALUES($1,'UTC','00:00') RETURNING id", tenant).Scan(&branch); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DELETE FROM ledger.branch WHERE id=$1", branch)
	defer admin.Exec(ctx, "DELETE FROM ledger.finance_grant WHERE branch_id=$1", branch)
	tag := fmt.Sprint("authority_grant_", time.Now().UnixNano())
	table := pgx.Identifier{tag}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE TABLE "+table+" (payload text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP TABLE "+table)
	if _, err := admin.Exec(ctx, "GRANT INSERT, SELECT ON "+table+" TO nu54_p05_finance"); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["application_name"] = tag
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE nu54_p05_finance")
		return err
	}
	finance, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer finance.Close()

	for _, scenario := range []string{"current", "expired", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			var expires time.Time
			lifetime := "15 seconds"
			if scenario == "expired" {
				lifetime = "900 milliseconds"
			}
			err := admin.QueryRow(ctx, `INSERT INTO ledger.finance_grant(branch_id,actor_id,capability,valid_until)
				VALUES($1,$2,'refund_approve',clock_timestamp()+$3::text::interval)
				ON CONFLICT(branch_id,actor_id,capability) DO UPDATE SET valid_until=excluded.valid_until RETURNING valid_until`,
				branch, actor, lifetime).Scan(&expires)
			if err != nil {
				t.Fatal(err)
			}
			lock, err := admin.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Rollback(ctx)
			if _, err = lock.Exec(ctx, "SELECT 1 FROM ledger.finance_grant WHERE branch_id=$1 AND actor_id=$2 FOR UPDATE", branch, actor); err != nil {
				t.Fatal(err)
			}

			out := httptest.NewRecorder()
			w, r, cancel := Bound(httptest.NewRequest("POST", "/", nil), time.Now().Add(5*time.Second), func() bool { return true }, out)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				p := service.Principal{Tenant: tenant, Subject: actor, Role: "finance"}
				scoped := Scope(r.Context(), p, branch, "refund_approve")
				var value string
				err := (Pool{Pool: finance}).QueryRow(scoped, "INSERT INTO "+table+" VALUES($1) RETURNING payload", scenario).Scan(&value)
				if err == nil {
					_, _ = io.WriteString(w, "protected data "+value)
				} else {
					http.NotFound(w, r)
				}
				done <- err
			}()
			waitBy := time.Now().Add(time.Second)
			for {
				var blocked bool
				err = admin.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_stat_activity WHERE application_name=$1
					AND wait_event_type='Lock' AND query LIKE '%request_authority_live%')`, tag).Scan(&blocked)
				if err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				if time.Now().After(waitBy) {
					t.Fatal("ordinary finance query did not reach grant-row lock")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if scenario == "expired" {
				time.Sleep(time.Until(expires.Add(50 * time.Millisecond)))
			} else if scenario == "revoked" {
				if _, err := lock.Exec(ctx, "UPDATE ledger.finance_grant SET valid_until=clock_timestamp() WHERE branch_id=$1 AND actor_id=$2", branch, actor); err != nil {
					t.Fatal(err)
				}
			}
			if err = lock.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-done
			var count int
			if scanErr := admin.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE payload=$1", scenario).Scan(&count); scanErr != nil {
				t.Fatal(scanErr)
			}
			if scenario == "current" {
				if err != nil || count != 1 || out.Code != http.StatusOK || out.Body.String() != "protected data current" {
					t.Fatalf("current grant did not commit: %v rows=%d response=%d", err, count, out.Code)
				}
			} else if err == nil || count != 0 || out.Code != http.StatusNotFound || strings.Contains(out.Body.String(), "protected data") {
				t.Fatalf("stale grant committed or released data: %v rows=%d response=%d", err, count, out.Code)
			}
		})
	}
}
