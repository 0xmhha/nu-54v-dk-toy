package settlement

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func TestQuoteBoundaryOriginalArchive(t *testing.T) {
	endpoint, binary := os.Getenv("P05_SETTLEMENT_TEST_RPC"), os.Getenv("P05_SETTLEMENT_TEST_INDEXER")
	if endpoint == "" || binary == "" {
		t.Skip("requires the disposable settlement PostgreSQL and Anvil runner")
	}
	db, dsn := scratchDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	chain, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Close()
	target, deployed, _, _, _ := deployFixture(t, ctx, chain, db)
	stop, _, _, proveAbsence := startFixtureIndexer(t, ctx, db, dsn, endpoint, binary, target, deployed)
	defer stop()
	head, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := seedPayerOrder(t, db, target, time.Unix(int64(head.Time), 0), pilotUnits(10), 72, fixtureSigner(102), 1)
	otherBranch, operator := fixtureUUID(t, db), fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", otherBranch, order.Tenant)
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'reconcile_read',now()+interval '1 hour')", order.Branch, operator)
	_, financeDSN := testLogin(t, db, dsn, "nu54_p05_finance")
	finance, err := Open(ctx, financeDSN, "nu54_p05_finance")
	if err != nil {
		t.Fatal(err)
	}
	defer finance.Close()
	_, explorerDSN := testLogin(t, db, dsn, "nu54_p07_explorer")
	cipher, err := commerce.NewCipher(order.CipherKey[:])
	if err != nil {
		t.Fatal(err)
	}
	trust, identities, ca := mtlsClients(t, 2)
	explorerReader, browser := identities[0], identities[1]
	operatorFP := fingerprint(browser)
	merchant := common.Hash(order.Quote.Args.MerchantId).Hex()
	orderKey := common.Hash(order.Quote.Args.OrderId).Hex()
	expiry := time.Now().Add(time.Hour).Unix()
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	writeJSON := func(name string, data any) string {
		t.Helper()
		body, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		path := write(name+".new", body)
		destination := filepath.Join(dir, name)
		if err := os.Rename(path, destination); err != nil {
			t.Fatal(err)
		}
		return destination
	}
	writeCert := func(prefix string, cert tls.Certificate) (string, string) {
		t.Helper()
		key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		return write(prefix+".crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})),
			write(prefix+".key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
	}
	issuerGrants := service.ClientGrantFile[QuoteBoundaryReaderGrant](writeJSON("issuer-grants", map[string]QuoteBoundaryReaderGrant{
		fingerprint(explorerReader): {ExpiresAt: expiry},
	}))
	operatorGrants := map[string]service.Principal{operatorFP: {
		Role: "finance", Tenant: order.Tenant, Subject: operator, ExpiresAt: expiry,
	}}
	opFile := service.ClientGrantFile[service.Principal](writeJSON("finance-grants", operatorGrants))
	targets := map[string]Target{order.Branch: target, otherBranch: target}
	merchants := map[string]string{order.Branch: merchant, otherBranch: merchant}
	mux := http.NewServeMux()
	FinanceHTTP{Store: finance, QuoteCipher: cipher, Targets: targets}.
		RegisterQuoteBoundary(mux, issuerGrants.Lookup, opFile.Lookup, merchants)
	issuer := httptest.NewUnstartedServer(mux)
	issuer.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	issuer.StartTLS()
	defer issuer.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	caFile := write("ca.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca}))
	serverCert, serverKey := writeCert("server", issuer.TLS.Certificates[0])
	issuerCert, issuerKey := writeCert("issuer-reader", explorerReader)
	browserCert, browserKey := writeCert("browser", browser)
	grants := map[string]any{operatorFP: map[string]any{
		"expiresAt": expiry, "streams": []string{target.Stream}, "merchants": map[string][]string{target.Stream: {merchant}},
	}}
	grantFile := writeJSON("explorer-grants", grants)
	cmd := exec.CommandContext(ctx, binary, "explorer-operations")
	cmd.Env = append(os.Environ(), "P07_EXPLORER_HTTP_ADDR="+addr, "P07_EXPLORER_DATABASE_URL="+explorerDSN,
		"P07_EXPLORER_CA="+caFile, "P07_EXPLORER_CERT="+serverCert, "P07_EXPLORER_KEY="+serverKey, "P07_EXPLORER_GRANTS="+grantFile,
		"P07_QUOTE_ORIGIN="+issuer.URL, "P07_QUOTE_ORIGIN_CA="+serverCert, "P07_QUOTE_ORIGIN_CERT="+issuerCert, "P07_QUOTE_ORIGIN_KEY="+issuerKey)
	var output bytes.Buffer
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Log(output.String())
		}
	}()
	transport := issuer.Client().Transport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	transport.TLSClientConfig.Certificates = []tls.Certificate{browser}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	explorerURL := "https://" + addr
	waitUntil(t, func() bool {
		reply, err := client.Get(explorerURL + "/")
		if err != nil {
			return false
		}
		reply.Body.Close()
		return reply.StatusCode == 200
	})
	read := func(branch, sale, key string, want int) []byte {
		t.Helper()
		path := explorerURL + "/v2/explorer/boundary"
		var req *http.Request
		var err error
		if branch != "" || sale != "" {
			fields, err := json.Marshal(map[string]string{"stream": target.Stream, "merchant": merchant, "order": key, "branch": branch, "sale": sale})
			if err != nil {
				t.Fatal(err)
			}
			req, err = http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(fields))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
		} else {
			fields := url.Values{"stream": {target.Stream}, "merchant": {merchant}, "order": {key}}
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, path+"?"+fields.Encode(), nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		out, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer out.Body.Close()
		body, err := io.ReadAll(out.Body)
		if err != nil || out.StatusCode != want {
			t.Fatalf("boundary HTTP %d, want %d: %s %v", out.StatusCode, want, body, err)
		}
		return body
	}
	snapshot := func() [4]int {
		t.Helper()
		var out [4]int
		err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.sale_resolution WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.journal WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$1)`, order.Sale).
			Scan(&out[0], &out[1], &out[2], &out[3])
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	browse := func(phase string) {
		t.Helper()
		script := os.Getenv("P07_QUOTE_BOUNDARY_BROWSER_SCRIPT")
		if script == "" {
			return
		}
		cmd := exec.CommandContext(ctx, "node", script)
		cmd.Env = append(os.Environ(), "X06_PHASE="+phase, "X06_URL="+explorerURL, "X06_CERT="+browserCert, "X06_KEY="+browserKey,
			"X06_STREAM="+target.Stream, "X06_MERCHANT="+merchant, "X06_ORDER="+orderKey, "X06_BRANCH="+order.Branch, "X06_SALE="+order.Sale)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("X06 browser %s: %v\n%s", phase, err, out)
		}
		t.Logf("X06 browser accepted %s from the real issuer and active indexer", phase)
	}
	assertKind := func(kind string, short bool) {
		t.Helper()
		var result struct {
			Kind     string    `json:"kind"`
			Expiry   time.Time `json:"quoteExpiresAt"`
			Digest   string    `json:"quoteDigest"`
			Coverage struct {
				Hash string    `json:"hash"`
				Time time.Time `json:"time"`
			} `json:"coverage"`
			Evidence json.RawMessage `json:"evidence"`
		}
		if err := json.Unmarshal(read(order.Branch, order.Sale, orderKey, 200), &result); err != nil ||
			result.Kind != kind || !result.Expiry.Equal(order.Order.Expires) || result.Digest != common.Hash(order.Quote.Digest).Hex() ||
			result.Coverage.Time.Before(result.Expiry) != short || result.Coverage.Hash == "" ||
			(len(result.Evidence) > 0) != (kind == "absent") {
			t.Fatalf("original boundary %s: %+v %v", kind, result, err)
		}
	}
	before := snapshot()
	assertKind("pending", true)
	browse("pending")
	if old := read("", "", orderKey, 200); string(bytes.TrimSpace(old)) != `{"kind":"unobserved"}` {
		t.Fatalf("legacy unobserved changed: %s", old)
	}
	missing := read(order.Branch, fixtureUUID(t, db), orderKey, 404)
	for _, denied := range [][]byte{
		read(otherBranch, order.Sale, orderKey, 404),
		read(order.Branch, order.Sale, common.Hash{31: 9}.Hex(), 404),
	} {
		if !bytes.Equal(missing, denied) {
			t.Fatal("scope denial identified an existing archive")
		}
	}
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'reconcile_read',now()+interval '1 hour')", otherBranch, operator)
	if wrongBranch := read(otherBranch, order.Sale, orderKey, 404); !bytes.Equal(missing, wrongBranch) {
		t.Fatal("a second authorized branch identified the first branch's original")
	}
	writeJSON("explorer-grants", map[string]any{})
	read(order.Branch, order.Sale, orderKey, 404)
	writeJSON("explorer-grants", grants)
	writeJSON("finance-grants", map[string]service.Principal{})
	read(order.Branch, order.Sale, orderKey, 404)
	writeJSON("finance-grants", operatorGrants)
	writeJSON("issuer-grants", map[string]QuoteBoundaryReaderGrant{})
	read(order.Branch, order.Sale, orderKey, 404)
	writeJSON("issuer-grants", map[string]QuoteBoundaryReaderGrant{fingerprint(explorerReader): {ExpiresAt: expiry}})

	forceTime(t, ctx, chain, order.Order.Expires.Unix()+1)
	latest, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		var height int64
		return db.QueryRow(ctx, "SELECT finalized_height FROM chainview.generation WHERE stream_id=$1", target.Stream).Scan(&height) == nil &&
			height >= latest.Number.Int64()-2
	})
	assertKind("pending", false)
	wrongCutoff := order
	wrongCutoff.Order.Expires = order.Order.Expires.Add(-10 * time.Second)
	proveAbsence(t, wrongCutoff)
	assertKind("pending", false)
	proveAbsence(t, order)
	assertKind("absent", false)
	browse("absent")
	incident := recordFixtureIncident(t, ctx, db, target.Stream)
	read(order.Branch, order.Sale, orderKey, 503)
	fixtureExec(t, db, "UPDATE chainview.incident SET resolved_at=clock_timestamp() WHERE id=$1", incident)
	assertKind("absent", false)
	issuer.Close()
	read(order.Branch, order.Sale, orderKey, 503)
	if after := snapshot(); before != after || after != [4]int{} {
		t.Fatalf("read-only quote boundary changed financial or fulfillment rows: %v to %v", before, after)
	}
}
