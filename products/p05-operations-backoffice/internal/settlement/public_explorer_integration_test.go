package settlement

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func startFixturePublicExplorer(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn string) string {
	t.Helper()
	_, publicDSN := testLogin(t, db, dsn, "nu54_p07_public")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	binary := os.Getenv("P05_SETTLEMENT_TEST_INDEXER")
	command := exec.CommandContext(ctx, binary, "public-receipts")
	command.Env = []string{"P07_PUBLIC_DATABASE_URL=" + publicDSN, "P07_PUBLIC_HTTP_ADDR=" + address}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
		if t.Failed() {
			t.Log(stderr.String())
		}
	})
	public := "http://" + address
	client := http.Client{Timeout: 200 * time.Millisecond}
	waitUntil(t, func() bool {
		response, err := client.Get(public + "/v2/explorer/deployments")
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		return response.StatusCode == http.StatusOK
	})
	return public
}

func notificationExplorerEnvironment(t *testing.T, ctx context.Context, db *pgxpool.Pool, evidenceDSN, branch, stream, public string) []string {
	t.Helper()
	var expected []byte
	if err := db.QueryRow(ctx, `SELECT jsonb_object_agg(e.event_id,jsonb_build_object(
		'merchant',p.merchant_key,'txHash',p.tx_hash,'logIndex',p.log_index)) FROM ledger.fulfillment_outbox e
		JOIN ledger.sale s ON s.id=e.sale_id JOIN ledger.payment p ON p.sale_id=e.sale_id
		WHERE s.branch_id=$1 AND p.stream_id=$2`, branch, stream).Scan(&expected); err != nil || len(expected) == 0 {
		t.Fatalf("original transaction coordinates unavailable: %v", err)
	}
	evidence, err := pgx.ParseConfig(evidenceDSN)
	if err != nil {
		t.Fatal(err)
	}
	barrier := filepath.Join(t.TempDir(), "explorer-scope")
	holdAtExplorerClick(t, ctx, db, evidence.User, stream, barrier)
	return []string{
		"NU54_NOTIFICATION_HOLD_BARRIER=" + barrier,
		"NU54_NOTIFICATION_PUBLIC_ORIGIN=" + public,
		"NU54_NOTIFICATION_PUBLIC_STREAM=" + stream,
		"NU54_NOTIFICATION_ORIGINAL_PAID=" + string(expected),
	}
}

func holdAtExplorerClick(t *testing.T, ctx context.Context, db *pgxpool.Pool, role, stream, path string) {
	t.Helper()
	change, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil && !t.Failed() {
			t.Error(err)
		}
	})
	go func() {
		waitFile := func(suffix string) bool {
			tick := time.NewTicker(25 * time.Millisecond)
			defer tick.Stop()
			for {
				if _, err := os.Stat(path + suffix); err == nil {
					return true
				}
				select {
				case <-tick.C:
				case <-change.Done():
					return false
				}
			}
		}
		if !waitFile(".requested") {
			done <- nil
			return
		}
		if _, err := db.Exec(change, `UPDATE chainview.receipt_scope SET valid_until=now()-interval '1 second'
			WHERE login_role=$1 AND stream_id=$2`, role, stream); err != nil {
			done <- err
			return
		}
		err := os.WriteFile(path+".held", nil, 0600)
		if err == nil {
			waitFile(".response")
		}
		restore, stop := context.WithTimeout(context.WithoutCancel(change), 2*time.Second)
		defer stop()
		_, restoreErr := db.Exec(restore, `UPDATE chainview.receipt_scope SET valid_until=now()+interval '1 hour'
			WHERE login_role=$1 AND stream_id=$2`, role, stream)
		if err == nil {
			err = restoreErr
		}
		if err == nil {
			err = os.WriteFile(path+".restored", nil, 0600)
		}
		done <- err
	}()
}
