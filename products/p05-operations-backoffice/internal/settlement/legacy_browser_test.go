package settlement

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
)

type legacyBrowserChain struct {
	chain    *core.Chain
	key      *ecdsa.PrivateKey
	merchant common.Address
}

func (p legacyBrowserChain) View(ctx context.Context, _ string) (core.LegacyLedger, error) {
	return p.chain.LegacyLedger(ctx, p.merchant)
}

func (p legacyBrowserChain) Preflight(ctx context.Context, _ string) (core.LegacyLedger, error) {
	return p.chain.LegacyLedgerLatest(ctx, p.merchant)
}

func (p legacyBrowserChain) Prepare(ctx context.Context, _ string) ([]byte, error) {
	return p.chain.PrepareLegacyCashOut(ctx, p.key, p.merchant)
}

func (p legacyBrowserChain) Observe(ctx context.Context, _ string, raw []byte, submit bool) (core.LegacyCashOutObservation, error) {
	return p.chain.ObserveLegacyCashOut(ctx, core.Address(p.key), p.merchant, raw, submit)
}

func runLegacyCashoutBrowser(t *testing.T, deployed legacyBrowserChain, finalize func()) {
	t.Helper()
	script, assets := os.Getenv("NU54_LEGACY_BROWSER_SCRIPT"), os.Getenv("NU54_LEGACY_BROWSER_DIST")
	if !filepath.IsAbs(script) || !filepath.IsAbs(assets) {
		t.Fatal("legacy browser requires absolute script and web dist")
	}
	ctx := t.Context()
	db, dsn := scratchDB(t)
	tenant, branch, reader, operator := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES($1,$2),($3,$4)",
		"legacy_operator_"+operator, operator, "legacy_review_"+reader, reader)
	fixtureExec(t, db, `INSERT INTO ledger.finance_grant VALUES
		($1,$2,'legacy_ledger',now()+interval '1 hour'),($1,$2,'legacy_cashout',now()+interval '1 hour'),
		($1,$3,'legacy_ledger',now()+interval '1 hour'),($1,$3,'legacy_reconcile',now()+interval '1 hour')`, branch, operator, reader)
	_, scoped := testLogin(t, db, dsn, "nu54_p05_commerce")
	store, err := commerce.Open(ctx, scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	trust, certs, _ := mtlsClients(t, 2)
	identities := map[string]service.Principal{}
	fixture := t.TempDir()
	for i, actor := range []string{operator, reader} {
		cert := certs[i]
		identities[fingerprint(cert)] = service.Principal{Role: "operator", Tenant: tenant, Subject: actor, ExpiresAt: time.Now().Add(10 * time.Minute).Unix()}
		raw, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(fixture, []string{"maker", "checker"}[i])
		for suffix, block := range map[string]pem.Block{
			".crt": {Type: "CERTIFICATE", Bytes: cert.Certificate[0]},
			".key": {Type: "PRIVATE KEY", Bytes: raw},
		} {
			if err := os.WriteFile(name+suffix, pem.EncodeToMemory(&block), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	origin := "https://" + server.Listener.Addr().String()
	mux.Handle("/ops/", http.StripPrefix("/ops/", http.FileServer(http.Dir(assets))))
	commerce.HTTP{Store: store, Credentials: identities, OperatorOrigin: origin, Legacy: deployed}.Register(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	run := func(phase string) {
		t.Helper()
		cli := exec.CommandContext(ctx, "sh", script, phase)
		cli.Env = append(os.Environ(), "NU54_ACCESS_ORIGIN="+origin, "NU54_ACCESS_BRANCH="+branch, "NU54_ACCESS_DIR="+fixture)
		if output, err := cli.CombinedOutput(); err != nil {
			t.Fatalf("legacy cashout browser %s: %v\n%s", phase, err, output)
		}
	}
	run("submit")
	finalize()
	run("verify")
	var reviewed, amount, observed, hash string
	if err := db.QueryRow(ctx, `SELECT review->>'balance',outcome->>'amount',outcome->>'status',outcome->>'transaction'
	    FROM ledger.legacy_cashout_request WHERE branch_id=$1 AND actor=$2`, branch, operator).
		Scan(&reviewed, &amount, &observed, &hash); err != nil {
		t.Fatal(err)
	}
	if reviewed != "3" || amount != reviewed || observed != "verified" || len(hash) != 66 {
		t.Fatalf("legacy proof: review=%s actual=%s status=%s tx=%s", reviewed, amount, observed, hash)
	}
}
