package commerce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func exercisePickupAssignmentResponse(t *testing.T, ctx context.Context, dsn, origin string, kiosk *http.Client, branch, actor string) {
	t.Helper()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	execute := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	const lock int64 = 540208124
	// Pause only the top-level final authority read, after the pickup SQL
	// has projected the previously authorized result and released its locks.
	execute("ALTER FUNCTION ledger.request_authority_live(uuid,uuid,uuid,text,text[]) RENAME TO original_pickup_scope")
	execute(fmt.Sprintf(`CREATE FUNCTION ledger.request_authority_live(uuid,uuid,uuid,text,text[]) RETURNS boolean
		LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $body$
		BEGIN
		  IF $4='terminal' AND current_query() LIKE 'SELECT ledger.request_authority_live(%%' THEN
		    PERFORM pg_advisory_xact_lock(%d);
		  END IF;
		  RETURN ledger.original_pickup_scope($1,$2,$3,$4,$5);
		END
		$body$`, lock))
	execute("REVOKE ALL ON FUNCTION ledger.request_authority_live(uuid,uuid,uuid,text,text[]) FROM PUBLIC")
	execute("GRANT EXECUTE ON FUNCTION ledger.request_authority_live(uuid,uuid,uuid,text,text[]) TO nu54_p05_commerce")
	defer execute("ALTER FUNCTION ledger.original_pickup_scope(uuid,uuid,uuid,text,text[]) RENAME TO request_authority_live")
	defer execute("DROP FUNCTION ledger.request_authority_live(uuid,uuid,uuid,text,text[])")
	payload, _ := json.Marshal(map[string]any{"travel": map[string]string{"operation": "pickup-queue", "branch": branch}})
	type response struct {
		status int
		body   []byte
		err    error
	}
	read := func() response {
		req, err := http.NewRequestWithContext(ctx, "POST", origin+"/v2/commerce", bytes.NewReader(payload))
		if err != nil {
			return response{err: err}
		}
		resp, err := kiosk.Do(req)
		if err != nil {
			return response{err: err}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return response{resp.StatusCode, body, err}
	}
	assign := func() {
		execute("INSERT INTO ledger.travel_pickup_terminal VALUES($1,$2)", branch, actor)
	}
	assign()
	if result := read(); result.err != nil || result.status != 200 {
		t.Fatalf("ordinary authorized terminal pickup: %d %v", result.status, result.err)
	}
	execute("SELECT pg_advisory_lock($1)", lock)
	defer admin.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", lock)
	got := make(chan response, 1)
	go func() { got <- read() }()
	for deadline := time.Now().Add(2 * time.Second); ; {
		var blocked bool
		err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_stat_activity
			WHERE datname=current_database() AND wait_event='advisory' AND query LIKE 'SELECT ledger.request_authority_live(%')`).Scan(&blocked)
		if err == nil && blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pickup was not held at final response authority: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	execute("DELETE FROM ledger.travel_pickup_terminal WHERE branch_id=$1 AND actor_id=$2", branch, actor)
	execute("SELECT pg_advisory_unlock($1)", lock)
	result := <-got
	if result.err != nil || result.status != 404 || bytes.Contains(result.body, []byte("pickups")) {
		t.Fatalf("pickup payload released after station assignment withdrawal: %d %v", result.status, result.err)
	}
	assign()
	if result := read(); result.err != nil || result.status != 200 {
		t.Fatalf("re-enrolled terminal pickup was unavailable: %d %v", result.status, result.err)
	}
	execute("DELETE FROM ledger.travel_pickup_terminal WHERE branch_id=$1 AND actor_id=$2", branch, actor)
	t.Log("actual mTLS terminal pickup response held at last read, withheld after assignment revocation; re-enrollment allowed")
}
