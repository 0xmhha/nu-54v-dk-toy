package settlement

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func verifyPrintGrantWait(t *testing.T, ctx context.Context, db *pgxpool.Pool, client *http.Client, origin, branch, actor, receipt, attempt string) {
	t.Helper()
	key := fixtureUUID(t, db)
	readURL := origin + "/v2/fulfillment/print-attempts?branchId=" + branch + "&receiptRef=" + receipt
	decisionURL := origin + "/v2/fulfillment/print-decisions"
	wire, _ := json.Marshal(map[string]string{
		"branchId": branch, "attemptId": attempt, "idempotencyKey": key, "action": "confirm_printed",
	})
	type response struct {
		code int
		body string
		err  error
	}
	send := func(method, url string) response {
		var payload io.Reader
		if method == "POST" {
			payload = bytes.NewReader(wire)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, payload)
		if err != nil {
			return response{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		reply, err := client.Do(req)
		if err != nil {
			return response{err: err}
		}
		defer reply.Body.Close()
		result, err := io.ReadAll(reply.Body)
		return response{reply.StatusCode, string(result), err}
	}
	for _, revoke := range []bool{false, true} {
		due := time.Now().Add(2500 * time.Millisecond)
		if revoke {
			due = time.Now().Add(time.Hour)
		}
		fixtureExec(t, db, "INSERT INTO ledger.print_staff_grant VALUES($1,$2,$3) ON CONFLICT(branch_id,actor_id) DO UPDATE SET valid_until=excluded.valid_until",
			branch, actor, due)
		busy, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer busy.Rollback(ctx)
		if _, err := busy.Exec(ctx, "SELECT 1 FROM ledger.print_staff_grant WHERE branch_id=$1 AND actor_id=$2 FOR UPDATE", branch, actor); err != nil {
			t.Fatal(err)
		}
		results := make(chan response, 2)
		go func() { results <- send("GET", readURL) }()
		go func() { results <- send("POST", decisionURL) }()
		waitUntil(t, func() bool {
			var waiting int
			err := db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database()
				AND wait_event_type='Lock' AND (query LIKE 'SELECT ledger.request_authority_live(%'
				   OR query LIKE 'SELECT ledger.print_staff_%')`).Scan(&waiting)
			return err == nil && waiting == 2
		})
		if revoke {
			if _, err := busy.Exec(ctx, "DELETE FROM ledger.print_staff_grant WHERE branch_id=$1 AND actor_id=$2", branch, actor); err != nil {
				t.Fatal(err)
			}
		} else {
			select {
			case <-time.After(time.Until(due) + 50*time.Millisecond):
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
		if err := busy.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			result := <-results
			if result.err != nil || result.code != 404 || strings.Contains(result.body, attempt) || strings.Contains(result.body, receipt) {
				t.Fatalf("print authority released blocked request after grant loss (revoke=%t): %d %v", revoke, result.code, result.err)
			}
		}
		var count int
		var state string
		if err := db.QueryRow(ctx, `SELECT a.state,(SELECT count(*) FROM ledger.print_decision WHERE request_key=$2)
			FROM ledger.print_attempt a WHERE a.id=$1`, attempt, key).Scan(&state, &count); err != nil || count != 0 || state != "unknown" {
			t.Fatalf("print changed after grant loss: %d %s %v", count, state, err)
		}
	}
	fixtureExec(t, db, "INSERT INTO ledger.print_staff_grant VALUES($1,$2,clock_timestamp()+interval '1 hour')", branch, actor)
	for _, result := range []response{send("GET", readURL), send("POST", decisionURL), send("POST", decisionURL)} {
		if result.err != nil || result.code != 200 || !strings.Contains(result.body, attempt) {
			t.Fatalf("renewed print credential failed exact receipt/decision replay: %d %v", result.code, result.err)
		}
	}
	t.Log("printer_staff GET and decision denied after grant row wait and expiry/revocation; no writes or private response, renewed exact replay succeeds")
}
