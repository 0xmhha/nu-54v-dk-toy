package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

func TestPostgresOrderState(t *testing.T) {
	config, err := pgx.ParseConfig(os.Getenv("P05_COMMERCE_TEST_DATABASE_URL"))
	if os.Getenv("P05_COMMERCE_TEST_DATABASE_URL") == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") {
		t.Fatal("use disposable commerce database")
	}
	ctx, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	execute := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	role := "nu54_state_" + strings.TrimPrefix(config.Database, "nu54_commerce_")
	execute("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	store, err := Open(ctx, config.ConnString()+" user="+role)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fresh := func() string { return uuidForCatalogFixture(t, ctx, admin) }
	branch, operator, customer := fresh(), fresh(), fresh()
	execute("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", operator, "state_"+operator)
	execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", branch, operator)
	execute("INSERT INTO ledger.command_rule VALUES ($1,'options test','fixture','dispose',interval '2 minutes')", branch)
	execute("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES($1,$2,'customer',now()+interval '5 minutes')", branch, customer)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(5 * time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(authority)
	digest := sha256.Sum256(der)
	asset := "0x" + strings.Repeat("2", 40)
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	defer server.Close()
	origin := "https://" + server.Listener.Addr().String()
	HTTP{Store: store, OperatorOrigin: origin, Profiles: map[string]Profile{branch: {Chain: 8283, Asset: common.HexToAddress(asset), Token: wonForOptions}},
		Credentials: map[string]service.Principal{hex.EncodeToString(digest[:]): {Tenant: tenant, Subject: operator, Role: "operator",
			ExpiresAt: time.Now().Add(5 * time.Minute).Unix()}}}.Register(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	server.StartTLS()
	client := server.Client()
	client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(raw))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		reply, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		out, _ := io.ReadAll(reply.Body)
		if reply.StatusCode != want {
			t.Fatalf("%s: got %d, want %d: %s", path, reply.StatusCode, want, out)
		}
		var decoded map[string]any
		_ = json.Unmarshal(out, &decoded)
		return decoded
	}
	// The menu: a cookie and a cake, five of each.
	cookie := func(sku string) operatorRow {
		return operatorRow{SKU: sku, Option: "standard", Label: sku, Asset: asset, DisplayPrice: "3000"}
	}
	call("POST", "/v2/operations/catalog/"+branch+"/publish", map[string]any{"requestKey": fresh(), "version": 0,
		"expiresAt": time.Now().Add(time.Hour).UTC(), "rows": []operatorRow{cookie("cookie"), cookie("cake")}}, 200)
	for _, sku := range []string{"cookie", "cake"} {
		call("POST", "/v2/operations/catalog/"+branch+"/stock", map[string]any{"requestKey": fresh(), "version": 1, "sku": sku, "delta": 5}, 200)
	}
	order := func(sku string) error {
		ref := fresh()
		_, err := store.Reserve(ctx, authorize(t, actor{customer, "customer"}, "order.create", branch, ref, fresh(), map[string]string{
			"asset": asset, "quantity": "1", "intentDigest": strings.Repeat("a", 64)}),
			Cart{Origin: "menu", OriginRef: ref, MenuVersion: 1, Items: []Item{{SKU: sku, Option: "standard", Qty: 1}}})
		return err
	}
	seen := func() *CustomerOrderState {
		t.Helper()
		state, err := store.OrderState(ctx, authorize(t, actor{customer, "customer"}, "catalog.read", branch, branch, "", map[string]string{"page": "0"}))
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	state := "/v2/operations/state/" + branch
	set := func(action string, change map[string]any, want int) map[string]any {
		t.Helper()
		return call("POST", state+"/"+action, change, want)
	}

	if got := call("GET", state, nil, 200); got["mode"] != "open" || got["me"] != operator || len(got["log"].([]any)) != 0 {
		t.Fatal(got)
	}
	if s := seen(); s.Mode != "open" || s.BusyMinutes != 0 || len(s.SoldOut) != 0 {
		t.Fatalf("%+v", s)
	}
	// Busy: orders still come in, and customers see the minutes.
	busy := set("mode", map[string]any{"mode": "busy", "busyMinutes": 25}, 200)
	if busy["mode"] != "busy" || busy["busyMinutes"] != float64(25) || busy["changedBy"] != operator {
		t.Fatal(busy)
	}
	set("mode", map[string]any{"mode": "busy", "busyMinutes": 25}, 200) // the same again is not a second change
	if s := seen(); s.Mode != "busy" || s.BusyMinutes != 25 {
		t.Fatalf("%+v", s)
	}
	if err := order("cookie"); err != nil {
		t.Fatal("a busy store refused an order", err)
	}
	for name, change := range map[string]map[string]any{
		"minutes off the five-minute steps": {"mode": "busy", "busyMinutes": 7},
		"more than an hour":                 {"mode": "busy", "busyMinutes": 65},
		"busy without minutes":              {"mode": "busy"},
		"a time while open":                 {"mode": "open", "resumeAt": "11:20"},
		"a time that is not one":            {"mode": "paused", "resumeAt": "25:00"},
		"an unknown state":                  {"mode": "closed"},
	} {
		set("mode", change, 400)
		_ = name
	}
	// Paused: no new order, and the time to take them again stays a note.
	set("mode", map[string]any{"mode": "paused", "resumeAt": "11:20"}, 200)
	if s := seen(); s.Mode != "paused" || s.ResumeAt != "11:20" {
		t.Fatalf("%+v", s)
	}
	if err := order("cookie"); !errors.Is(err, service.CatalogChanged) {
		t.Fatal("a paused store took an order", err)
	}
	set("mode", map[string]any{"mode": "open"}, 200)
	if err := order("cookie"); err != nil {
		t.Fatal(err)
	}
	// A closed day: today refuses, and the day can be taken back. A past day cannot be set.
	var today string
	if err := admin.QueryRow(ctx, "SELECT ledger.branch_today($1, now())::text", branch).Scan(&today); err != nil {
		t.Fatal(err)
	}
	closed := true
	set("closed-day", map[string]any{"day": today, "closed": closed}, 200)
	if s := seen(); s.Mode != "closed" {
		t.Fatalf("%+v", s)
	}
	if err := order("cookie"); !errors.Is(err, service.CatalogChanged) {
		t.Fatal("a closed day took an order", err)
	}
	set("closed-day", map[string]any{"day": "2020-01-01", "closed": true}, 400)
	set("closed-day", map[string]any{"day": "next monday", "closed": true}, 400)
	closed = false
	if got := set("closed-day", map[string]any{"day": today, "closed": closed}, 200); len(got["closedDays"].([]any)) != 0 {
		t.Fatal(got)
	}
	// Sold out: several products at once, apart from their counts; a product off the menu is refused.
	sold := set("sold-out", map[string]any{"products": []string{"cookie", "cake"}, "soldOut": true}, 200)
	if len(sold["soldOut"].([]any)) != 2 {
		t.Fatal(sold)
	}
	if err := order("cake"); !errors.Is(err, ErrStock) {
		t.Fatal("a sold-out product was ordered", err)
	}
	rows, _, _, err := store.catalogMenuWindow(ctx, authorize(t, actor{customer, "customer"}, "catalog.read", branch, branch, "",
		map[string]string{"page": "0"}), 0)
	if err != nil {
		t.Fatal(err)
	}
	markSoldOut(rows, seen().SoldOut)
	for _, row := range rows {
		if row.Available != 0 || row.SKUAvailable != 0 {
			t.Fatalf("%+v", row)
		}
	}
	set("sold-out", map[string]any{"products": []string{"mocha"}, "soldOut": true}, 400)
	set("sold-out", map[string]any{"products": []string{"cake"}, "soldOut": false}, 200)
	if err := order("cake"); err != nil {
		t.Fatal("selling again did not sell", err)
	}
	var left int64
	if err := admin.QueryRow(ctx, "SELECT available FROM ledger.stock WHERE branch_id=$1 AND sku='cake'", branch).Scan(&left); err != nil || left != 4 {
		t.Fatal("the count changed with the sold-out mark", left, err)
	}
	// Every change is recorded with who and when, newest first; the record cannot be changed by the server's role.
	log := call("GET", state, nil, 200)["log"].([]any)
	if len(log) != 7 || log[0].(map[string]any)["by"] != operator ||
		fmt.Sprint(log[0].(map[string]any)["change"]) != "map[products:[cake] soldOut:false]" {
		t.Fatal(log)
	}
	if _, err := store.db.Exec(ctx, "DELETE FROM ledger.branch_state_log"); err == nil {
		t.Fatal("the server's role deleted the record")
	}
	// A refund mark needs a designated tax-free shop, confirmed by the operator, and a taxed take-away good.
	refund := func(tax string) map[string]any {
		r := cookie("beans")
		r.Metadata = &menuMetadata{Tax: tax, TakeAway: true, TaxRefund: true}
		r.Tax = "0"
		return map[string]any{"requestKey": fresh(), "version": 1, "expiresAt": time.Now().Add(time.Hour).UTC(),
			"rows": []operatorRow{cookie("cookie"), cookie("cake"), r}}
	}
	call("POST", "/v2/operations/catalog/"+branch+"/publish", refund("vat10"), 400)
	number := "123-45-67890-001"
	set("tax-free", map[string]any{"designation": number, "designatedOn": "2026-11-20", "refundOperator": "환급 창구"}, 400)
	set("tax-free", map[string]any{"designation": "번호", "designatedOn": "2026-11-20", "refundOperator": "환급 창구", "confirmed": true}, 400)
	set("tax-free", map[string]any{"designation": number, "designatedOn": "2999-01-01", "refundOperator": "환급 창구", "confirmed": true}, 400)
	shop := set("tax-free", map[string]any{"designation": number, "designatedOn": "2026-11-20", "refundOperator": "환급 창구", "confirmed": true}, 200)
	if shop["taxFreeShop"].(map[string]any)["designation"] != number || shop["taxFreeShop"].(map[string]any)["by"] != operator {
		t.Fatal(shop)
	}
	if s := seen(); s.TaxFreeShop != number {
		t.Fatalf("%+v", s)
	}
	call("POST", "/v2/operations/catalog/"+branch+"/publish", refund("exempt"), 400)
	call("POST", "/v2/operations/catalog/"+branch+"/publish", refund("vat10"), 200)
	set("tax-free", map[string]any{"designation": ""}, 200)
	if s := seen(); s.TaxFreeShop != "" {
		t.Fatal("the number stayed after it was removed")
	}

	// The ready board: today's numbers by stage for the store's tablet only, and nothing else about an order.
	terminal := fresh()
	execute("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until,terminal_mode) VALUES($1,$2,'terminal',now()+interval '5 minutes','menu')", branch, terminal)
	pickup := func(number int64, stage string, paid string) {
		t.Helper()
		sale := fresh()
		// Rows written directly, without the payment path that would make them; triggers and keys are off for this.
		tx, err := admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		for _, step := range []struct {
			query string
			args  []any
		}{
			{"SET LOCAL session_replication_role = replica", nil},
			{"INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,asset_ref,amount,state) VALUES($1,$2,'menu',$1,$3,300000,$4)", []any{sale, branch, asset, paid}},
			{"INSERT INTO ledger.pickup(sale_id,branch_id,business_day,number) VALUES($1,$2,ledger.ledger_business_day($2,clock_timestamp()),$3)", []any{sale, branch, number}},
		} {
			if _, err := tx.Exec(ctx, step.query, step.args...); err != nil {
				t.Fatal(step.query, err)
			}
		}
		if stage != "received" {
			if _, err := tx.Exec(ctx, "INSERT INTO ledger.travel_pickup_progress(sale_id,state,revision,updated_at) VALUES($1,$2,1,clock_timestamp()+make_interval(secs => $3))", sale, stage, number); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	pickup(11, "ready", "paid")
	pickup(12, "ready", "paid")
	pickup(13, "preparing", "paid")
	pickup(15, "received", "paid")
	pickup(16, "collected", "paid")
	pickup(17, "unfulfillable", "paid")
	board, err := store.ReadyBoard(ctx, authorize(t, actor{terminal, "terminal"}, "board.read", branch, branch, "", map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(board.Ready, board.Making, board.Received) != "[12 11] [13] [15]" || board.CheckedAt == "" {
		t.Fatalf("%+v", board)
	}
	if raw, _ := json.Marshal(board); strings.Contains(string(raw), "amount") || strings.Contains(string(raw), "sale") {
		t.Fatal("the board says more than numbers", string(raw))
	}
	// The service rules give the request to tablets only; the function refuses a customer on its own too.
	var leaked []byte
	if err := store.db.QueryRow(ctx, "SELECT ledger.ready_board($1,$2,$3)", tenant, branch, customer).Scan(&leaked); err == nil {
		t.Fatal("a customer read the board", string(leaked))
	}

	// The operations routes refuse a page from another origin.
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+state+"/mode", strings.NewReader(`{"mode":"open"}`))
	req.Header.Set("Origin", "https://elsewhere.invalid")
	req.Header.Set("Content-Type", "application/json")
	if reply, err := client.Do(req); err != nil || reply.StatusCode != 404 {
		t.Fatal("another origin changed the state", err)
	}
}
