package ceremony_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/operatorui"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/jackc/pgx/v5/pgxpool"
)

type operatorProbe struct {
	wallet.Participants
	outage string
}

func (p operatorProbe) Status(ctx context.Context, id, activity string) (ceremony.Result, error) {
	if _, err := os.Stat(p.outage); err == nil && id == "b1" {
		return ceremony.Result{}, errors.New("synthetic B response missing")
	}
	return p.Participants.Status(ctx, id, activity)
}

func (l *browserLab) runOperator(t *testing.T, ctx context.Context, config wallet.API, control wallet.Participants, pool *pgxpool.Pool, wallets [2]wallet.Wallet, gates []testGate) {
	t.Helper()
	staging := t.TempDir()
	revoked := filepath.Join(staging, "revoked")
	outage := filepath.Join(staging, "outage")
	config.OperatorParticipants = operatorProbe{Participants: control, outage: outage}
	config.OperatorGrant = func(actor wallet.Claims) bool {
		_, err := os.Stat(revoked)
		return errors.Is(err, os.ErrNotExist) && actor.Subject == gates[0].subject && actor.Credential == gates[0].credential
	}
	actual := config.Handler()
	var lost atomic.Bool
	interruption := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/recovery/incidents" || lost.Load() {
			actual.ServeHTTP(w, r)
			return
		}
		rec := httptest.NewRecorder()
		actual.ServeHTTP(rec, r)
		if rec.Code == 200 && lost.CompareAndSwap(false, true) {
			http.Error(w, "synthetic reply lost after commit", 503)
			return
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	})
	server := httptest.NewUnstartedServer(nil)
	defer server.Close()
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	origin := "https://localhost:" + port
	roots := x509.NewCertPool()
	roots.AddCert(l.root)
	client := &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}}}
	defer client.CloseIdleConnections()
	h, err := operatorui.New(origin, gates[0].origin, client, interruption, func(token string) (time.Time, error) {
		actor, err := config.VerifyRecoveryOperator(token, time.Now())
		return time.Unix(actor.Expires, 0), err
	})
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = h
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{l.certificate(t)}}
	server.StartTLS()
	fixture := map[string]string{"origin": origin, "wallet": wallets[0].ID, "other": wallets[1].ID,
		"username": gates[0].username, "password": gates[0].password, "revoked": revoked, "outage": outage}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(staging, "operator.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	run, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(run, "node", filepath.Join("..", "..", "test", "operator-browser.cjs"), path)
	if raw, err := cmd.CombinedOutput(); err != nil {
		stage := regexp.MustCompile(`W24_STAGE [a-z0-9_]+|Cannot find module|SyntaxError|EACCES|Cannot find package|MODULE_NOT_FOUND|ERR_DLOPEN_FAILED`).Find(raw)
		t.Fatalf("operator browser interaction failed: %v diagnostics=%s capturedBytes=%d", err, stage, len(raw))
	}
	if !lost.Load() {
		t.Fatal("committed operator reply was not interrupted")
	}
	var frozen, otherActive bool
	var events, signed int
	err = pool.QueryRow(ctx, `SELECT w.state='blocked', (SELECT state='active' FROM wallet_mpc.wallet WHERE wallet_id=$2),
		(SELECT count(*) FROM wallet_mpc.audit_outbox WHERE wallet_id=w.wallet_id AND event_kind='incident_hold'),
		(SELECT count(*) FROM wallet_mpc.operation WHERE wallet_id=w.wallet_id AND kind='sign' AND state='succeeded')
		FROM wallet_mpc.wallet w WHERE w.wallet_id=$1`, wallets[0].ID, wallets[1].ID).Scan(&frozen, &otherActive, &events, &signed)
	if err != nil || !frozen || !otherActive || events != 1 || signed != 0 {
		t.Fatalf("audit effects: freeze %t, other active %t, incident %d, signatures %d: %v", frozen, otherActive, events, signed, err)
	}
	t.Log("P03 W24 Chrome: outage read-only, isolated role, lost reply recovered, one frozen wallet, zero signatures and transfers")
}
