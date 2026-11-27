package chainview

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func verifyReplayBrowser(t *testing.T, ctx context.Context, database, endpoint string, owner *pgxpool.Pool, profile Profile, legacy, merchant string) {
	t.Helper()
	script := os.Getenv("P07_EXPLORER_BROWSER_SCRIPT")
	if script == "" {
		return
	}
	profile.StreamID = "79014272-cee2-4280-ae7d-24bd2afd3901"
	profile.GenerationID = "79014272-cee2-4280-ae7d-24bd2afd3902"
	profile.Manifest = fmt.Sprintf("%064d", 52)
	source, err := Dial(ctx, endpoint, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	repo := &Repository{db: owner}
	head, err := source.Header(ctx, -2)
	if err != nil {
		t.Fatal(err)
	}
	reader := Reader{Profile: profile, Source: &controlledFinality{RPCSource: source, height: head.Height}, DB: repo}
	if err := repo.Register(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	activate(t, owner, profile.StreamID, profile.GenerationID)
	_, publicURL := fixtureLogin(t, database, owner, "nu54_p07_public")
	_, explorerURL := fixtureLogin(t, database, owner, "nu54_p07_explorer")
	_, requestURL := fixtureLogin(t, database, owner, "nu54_p07_replay_request")
	publicStore, err := OpenPublic(ctx, publicURL)
	if err != nil {
		t.Fatal(err)
	}
	defer publicStore.Close()
	root := newExplorerClient(t)
	public := httptest.NewServer(publicStore.PublicHandler())
	defer public.Close()
	dir := t.TempDir()
	grants, err := json.Marshal(map[string]OperatorGrant{root.fingerprint: {
		Streams: []string{profile.StreamID, legacy}, Replay: []string{profile.StreamID},
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	grantFile := filepath.Join(dir, "grants.json")
	if err := os.WriteFile(grantFile, grants, 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	opsURL := "https://" + addr
	operationsLog, err := os.Create(filepath.Join(dir, "operations.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer operationsLog.Close()
	operations := exec.CommandContext(ctx, os.Getenv("P07_CHAINVIEW_TEST_INDEXER"), "explorer-operations")
	operations.Env = append(os.Environ(), "P07_EXPLORER_DATABASE_URL="+explorerURL, "P07_REPLAY_REQUEST_DATABASE_URL="+requestURL,
		"P07_EXPLORER_HTTP_ADDR="+addr, "P07_EXPLORER_GRANTS="+grantFile, "P07_EXPLORER_CA="+root.caFile,
		"P07_EXPLORER_CERT="+root.serverCert, "P07_EXPLORER_KEY="+root.serverKey)
	operations.Stderr = operationsLog
	if err := operations.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = operations.Process.Kill()
		_ = operations.Wait()
		if t.Failed() {
			raw, _ := os.ReadFile(operationsLog.Name())
			t.Logf("operator executable: %s", raw)
		}
	}()
	cert, err := tls.LoadX509KeyPair(root.cert, root.key)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: root.ca, Certificates: []tls.Certificate{cert}}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	ready := false
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		response, err := client.Get(opsURL + "/")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !ready {
		t.Fatal("mTLS operator executable did not start")
	}
	log, err := os.Create(filepath.Join(dir, "browser.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	browser := exec.CommandContext(ctx, "node", script)
	browser.Stdout, browser.Stderr = log, log
	browser.Env = append(os.Environ(), "EXPLORER_PHASE=replay", "EXPLORER_PUBLIC="+public.URL,
		"EXPLORER_OPS="+opsURL, "EXPLORER_CERT="+root.cert, "EXPLORER_KEY="+root.key,
		"EXPLORER_STREAM="+profile.StreamID, "EXPLORER_LEGACY="+legacy, "EXPLORER_MERCHANT="+merchant,
		"EXPLORER_ORDER=0x"+fmt.Sprintf("%064x", 4), "EXPLORER_REPLAY_SYNC="+dir)
	if err := browser.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if browser.ProcessState == nil {
			_ = browser.Process.Kill()
			_ = browser.Wait()
		}
		if t.Failed() {
			raw, _ := os.ReadFile(log.Name())
			t.Logf("replay browser: %s", raw)
		}
	}()

	waitJob := func(signal string) (string, int64) {
		t.Helper()
		end := time.Now().Add(10 * time.Second)
		for time.Now().Before(end) {
			if _, err := os.Stat(filepath.Join(dir, signal)); err == nil {
				var job string
				var height int64
				err = owner.QueryRow(ctx, `SELECT id::text,anchor_height FROM chainview.replay_job
					WHERE stream_id=$1 AND state='running'`, profile.StreamID).Scan(&job, &height)
				if err != nil {
					t.Fatal(err)
				}
				return job, height
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("no replay browser signal %s", signal)
		return "", 0
	}
	matched, _ := waitJob("first")
	if err := reader.PollReplay(ctx); err != nil {
		t.Fatal(err)
	}
	failed, height := waitJob("second")
	anchor, err := source.Header(ctx, height)
	if err != nil {
		t.Fatal(err)
	}
	candidate := reader
	candidate.Profile.GenerationID = failed
	candidate.Source = replaySource{source, anchor}
	candidate.isolated = true
	if err := candidate.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `UPDATE chainview.observed_event
		SET amount=amount+1 WHERE generation_id=$1 AND kind='paid'`, failed); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, "SELECT chainview.finish_replay($1,false)", failed); err != nil {
		t.Fatal(err)
	}
	if err := browser.Wait(); err != nil {
		t.Fatal(err)
	}
	var live, proof, state, reason string
	var revision int64
	err = owner.QueryRow(ctx, `SELECT t.generation_id::text,t.revision,good.comparison_id::text,bad.state,bad.reason
		FROM chainview.read_target t CROSS JOIN chainview.replay_job good CROSS JOIN chainview.replay_job bad
		WHERE t.stream_id=$1 AND good.id=$2 AND bad.id=$3`, profile.StreamID, matched, failed).Scan(&live, &revision, &proof, &state, &reason)
	if err != nil || live != profile.GenerationID || state != "diff" || reason != "projection_diff" {
		t.Fatalf("failed replay changed active target or lacks difference: %s %s %s %v", live, state, reason, err)
	}
	_, switchURL := fixtureLogin(t, database, owner, "nu54_p07_switch")
	switchDB, err := pgxpool.New(ctx, switchURL)
	if err != nil {
		t.Fatal(err)
	}
	defer switchDB.Close()
	request := "79014272-cee2-4280-ae7d-24bd2afd3903"
	actor := "79014272-cee2-4280-ae7d-24bd2afd3904"
	if _, err := switchDB.Exec(ctx, `INSERT INTO chainview.switch_request (id,stream_id,target_id,expected_revision,operator_ref,compare_ref,requested_at)
		VALUES($1,$2,$3,$4,$5,$6,clock_timestamp())`, request, profile.StreamID, matched, revision, actor, proof); err != nil {
		t.Fatal(err)
	}
	if _, err := switchDB.Exec(ctx, "SELECT chainview.apply_switch($1)", request); err != nil {
		t.Fatal(err)
	}
	if err := owner.QueryRow(ctx, "SELECT generation_id::text FROM chainview.read_target WHERE stream_id=$1", profile.StreamID).Scan(&live); err != nil || live != matched {
		t.Fatalf("approved replay did not switch through the existing guard: %v", err)
	}
	profile.GenerationID = matched
	if err := repo.Register(ctx, profile); err != nil {
		t.Fatalf("promoted candidate could not enter the normal producer: %v", err)
	}
	t.Log("X05 Chrome running/reload/matched/diff; stable public payment through mismatch; matched comparison required explicit trusted switch")
}
