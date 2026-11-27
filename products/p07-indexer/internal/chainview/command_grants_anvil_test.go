package chainview

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

func verifyLiveGrantCommands(t *testing.T, ctx context.Context, database string, owner *pgxpool.Pool, stream, generation, merchant string) {
	t.Helper()
	binary := os.Getenv("P07_CHAINVIEW_TEST_INDEXER")
	if binary == "" {
		t.Fatal("actual indexer executable required")
	}
	root := newExplorerClient(t)
	certificate, err := tls.LoadX509KeyPair(root.cert, root.key)
	if err != nil {
		t.Fatal(err)
	}
	var event, order, transaction string
	if err := owner.QueryRow(ctx, "SELECT id::text,order_key,tx_hash FROM chainview.active_evidence WHERE generation_id=$1 AND kind='paid' LIMIT 1", generation).Scan(&event, &order, &transaction); err != nil {
		t.Fatal(err)
	}
	for _, option := range []struct {
		name, command, prefix, role, path, lock string
	}{
		{"evidence", "evidence", "P07_EVIDENCE_", "nu54_p07_read", "/v2/evidence/read", "chainview.evidence"},
		{"explorer-operations", "explorer-operations", "P07_EXPLORER_", "nu54_p07_explorer", "/v2/explorer/monitor", "chainview.generation"},
		{"explorer-boundary", "explorer-operations", "P07_EXPLORER_", "nu54_p07_explorer", "/v2/explorer/boundary", "chainview.evidence"},
	} {
		t.Run(option.name, func(t *testing.T) {
			role, dsn := fixtureLogin(t, database, owner, option.role)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := listener.Addr().String()
			listener.Close()
			dir := t.TempDir()
			file := filepath.Join(dir, "grants.json")
			publish := func(raw []byte) {
				t.Helper()
				tmp := file + ".next"
				if err := os.WriteFile(tmp, raw, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(tmp, file); err != nil {
					t.Fatal(err)
				}
			}
			var allowed, wrong, retained any
			var payload []byte
			if option.command == "evidence" {
				principal := service.Principal{Subject: "232d3f54-a436-43c9-aab7-b737541d27d6", Tenant: "458a99cd-fc10-43b7-8104-177272121c02",
					Role: "evidence-reader", ExpiresAt: time.Now().Add(time.Minute).Unix(),
					Grants: []service.Grant{{Domain: generation, Object: event, Operations: []string{"C7/evidence.read"}}}}
				allowed = principal
				principal.Grants = nil
				wrong = principal
				payload, err = json.Marshal(service.Request{Version: 2, Contract: "C7", Operation: "evidence.read",
					RequestID: "232d3f54-a436-43c9-aab7-b737541d27d5", Domain: generation, Object: event,
					ExpiresAt: time.Now().Add(50 * time.Second).Unix(), Body: map[string]string{"kind": "finalized"}})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				grant := OperatorGrant{Streams: []string{stream}, Merchants: map[string][]string{stream: {merchant}},
					ExpiresAt: time.Now().Add(time.Minute).Unix()}
				allowed = grant
				if option.name == "explorer-boundary" {
					unrelated := common.Hash{31: 222}.Hex()
					grant.Merchants = map[string][]string{stream: {unrelated}}
					wrong = grant
					grant.Merchants = map[string][]string{stream: {unrelated, merchant}}
					grant.ExpiresAt += 60
					retained = grant
				} else {
					grant.Streams, grant.Merchants = nil, nil
					wrong = grant
				}
			}
			good, _ := json.Marshal(map[string]any{root.fingerprint: allowed})
			narrow, _ := json.Marshal(map[string]any{root.fingerprint: wrong})
			publish(good)

			log, err := os.Create(filepath.Join(dir, "indexer.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			proc := exec.CommandContext(ctx, binary, option.command)
			proc.Env = append(os.Environ(), option.prefix+"DATABASE_URL="+dsn, option.prefix+"CA="+root.caFile,
				option.prefix+"CERT="+root.serverCert, option.prefix+"KEY="+root.serverKey,
				option.prefix+"GRANTS="+file, option.prefix+"HTTP_ADDR="+addr)
			proc.Stderr = log
			if err := proc.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = proc.Process.Kill()
				_ = proc.Wait()
			}()

			transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: root.ca, Certificates: []tls.Certificate{certificate}}}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 6 * time.Second}
			type response struct {
				code   int
				body   string
				reused bool
				err    error
			}
			request := func() response {
				method := http.MethodGet
				if payload != nil {
					method = http.MethodPost
				}
				endpoint := "https://" + addr + option.path
				if option.name == "explorer-boundary" {
					endpoint += "?" + (url.Values{"stream": {stream}, "merchant": {merchant}, "order": {order}}).Encode()
				}
				req, _ := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
				var out response
				req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
					GotConn: func(info httptrace.GotConnInfo) { out.reused = info.Reused },
				}))
				resp, err := client.Do(req)
				if err != nil {
					out.err = err
					return out
				}
				defer resp.Body.Close()
				data, err := io.ReadAll(resp.Body)
				out.code, out.body, out.err = resp.StatusCode, string(data), err
				return out
			}
			expect := func(out response, code int, private bool) {
				t.Helper()
				token := stream
				if payload != nil {
					token = event
				} else if option.name == "explorer-boundary" {
					token = transaction
				}
				if out.err != nil || out.code != code || strings.Contains(out.body, token) != private {
					t.Fatalf("unexpected %s result: %d reused=%t, %v, %s", option.command, out.code, out.reused, out.err, out.body)
				}
			}
			readyBy := time.Now().Add(5 * time.Second)
			for {
				out := request()
				if out.err == nil {
					expect(out, http.StatusOK, true)
					break
				}
				if time.Now().After(readyBy) {
					logText, _ := os.ReadFile(log.Name())
					t.Fatalf("indexer did not start: %v %s", out.err, logText)
				}
				time.Sleep(35 * time.Millisecond)
			}

			// Exercise an ordinary SELECT stalled in PostgreSQL while the
			// existing certificate's authorized scope is atomically replaced.
			changeWhileBlocked := func(update []byte) response {
				t.Helper()
				tx, err := owner.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				if _, err := tx.Exec(ctx, "LOCK TABLE "+option.lock+" IN ACCESS EXCLUSIVE MODE"); err != nil {
					t.Fatal(err)
				}
				finished := make(chan response, 1)
				go func() { finished <- request() }()
				waitBy := time.Now().Add(3 * time.Second)
				for {
					var blocked bool
					err := owner.QueryRow(ctx, "SELECT EXISTS(SELECT FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock')", role).Scan(&blocked)
					if err != nil || (!blocked && time.Now().After(waitBy)) {
						t.Fatalf("read did not reach normal database lock: %v", err)
					}
					if blocked {
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
				publish(update)
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				return <-finished
			}
			if retained != nil {
				updated, _ := json.Marshal(map[string]any{root.fingerprint: retained})
				expect(changeWhileBlocked(updated), http.StatusOK, true)
			}
			waited := changeWhileBlocked(narrow)
			expect(waited, http.StatusNotFound, false)
			if !waited.reused {
				t.Fatal("blocked request did not reuse the enrolled connection")
			}

			publish([]byte("{}"))
			reused := request()
			expect(reused, http.StatusNotFound, false)
			if !reused.reused {
				t.Fatal("withdrawal was not exercised on the same connection")
			}
			transport.CloseIdleConnections()
			fresh := request()
			expect(fresh, http.StatusNotFound, false)
			if fresh.reused {
				t.Fatal("expected a new TLS connection")
			}
			publish([]byte("{broken"))
			expect(request(), http.StatusNotFound, false)
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			expect(request(), http.StatusNotFound, false)
			publish(good)
			expect(request(), http.StatusOK, true)
		})
	}
}

func verifyReadRevision(t *testing.T, ctx context.Context, database string, owner *pgxpool.Pool, stream, merchant, order string) {
	t.Helper()
	role, dsn := fixtureLogin(t, database, owner, "nu54_p07_public")
	store, err := OpenPublic(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	request := func() *httptest.ResponseRecorder {
		query := url.Values{"stream": {stream}, "merchant": {merchant}, "order": {order}}
		out := httptest.NewRecorder()
		store.PublicHandler().ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/v2/receipts?"+query.Encode(), nil).WithContext(ctx))
		return out
	}
	if release := request(); release.Code != http.StatusOK || !strings.Contains(release.Body.String(), order) {
		t.Fatalf("initial canonical receipt unavailable: %d %s", release.Code, release.Body.String())
	}

	writer, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback(ctx)
	if _, err := writer.Exec(ctx, "LOCK TABLE chainview.observed_event IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	waiting := make(chan *httptest.ResponseRecorder, 1)
	go func() { waiting <- request() }()
	waitBy := time.Now().Add(2 * time.Second)
	for {
		var blocked bool
		err := owner.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_stat_activity
			WHERE usename=$1 AND wait_event_type='Lock' AND query LIKE '%explorer_receipt_detail%')`, role).Scan(&blocked)
		if err != nil || (!blocked && time.Now().After(waitBy)) {
			t.Fatalf("canonical read did not reach the second statement lock: %v", err)
		}
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := writer.Exec(ctx, "UPDATE chainview.read_target SET revision=revision+1 WHERE stream_id=$1", stream); err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	stale := <-waiting
	if stale.Code != http.StatusServiceUnavailable || strings.Contains(stale.Body.String(), order) {
		t.Fatalf("stale canonical result was released: %d %s", stale.Code, stale.Body)
	}
	if next := request(); next.Code != http.StatusOK || !strings.Contains(next.Body.String(), order) {
		t.Fatalf("ordinary read did not recover from revision change: %d %s", next.Code, next.Body)
	}
}
