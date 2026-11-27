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
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// TestPostgresStorePage walks the store page against a migrated database: the store writes it on the operations web,
// and the customer entrance lists and answers it without any proof, giving out only what the store published.
func TestPostgresStorePage(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
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
			t.Fatal(query, err)
		}
	}
	role := "nu54_page_" + strings.TrimPrefix(config.Database, "nu54_commerce_")
	execute("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	store, err := Open(ctx, config.ConnString()+" user="+role)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fresh := func() string { return uuidForCatalogFixture(t, ctx, admin) }
	cafe, bakery, unnamed, operator, stranger := fresh(), fresh(), fresh(), fresh(), fresh()
	for _, b := range []string{cafe, bakery, unnamed} {
		execute("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", b, tenant)
	}
	// Names that sort after and before each other, and that no other test's store shares.
	suffix := cafe[:8]
	execute("INSERT INTO ledger.operator_branch_label VALUES ($1,$2)", cafe, "여울 커피 "+suffix)
	execute("INSERT INTO ledger.operator_branch_label VALUES ($1,$2)", bakery, "모퉁이 베이커리 "+suffix)
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", operator, "page_"+operator)
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", stranger, "page_"+stranger)
	for _, b := range []string{cafe, unnamed} {
		execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", b, operator)
	}
	execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", bakery, stranger)

	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(10 * time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	ca, _ := x509.ParseCertificate(caDER)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	credentials := map[string]service.Principal{}
	leaf := func(serial int64, subject string) tls.Certificate {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, key.Public(), caKey)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(der)
		credentials[hex.EncodeToString(digest[:])] = service.Principal{Tenant: tenant, Subject: subject, Role: "operator",
			ExpiresAt: time.Now().Add(10 * time.Minute).Unix()}
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	}
	ownCert, strangerCert := leaf(2, operator), leaf(3, stranger)

	certified, entrance := http.NewServeMux(), http.NewServeMux()
	server := httptest.NewUnstartedServer(certified)
	defer server.Close()
	origin := "https://" + server.Listener.Addr().String()
	h := HTTP{Store: store, OperatorOrigin: origin, Credentials: credentials,
		Customer: &CustomerEntrance{Limiter: NewRateLimiter(600, 200, nil), OperatorContact: " help@operator.example "},
		Profiles: map[string]Profile{cafe: {Chain: 8283, Contract: common.HexToAddress("0x" + strings.Repeat("3", 40)),
			Asset: common.HexToAddress("0x" + strings.Repeat("2", 40)), Beneficiary: common.HexToAddress("0x" + strings.Repeat("4", 40)),
			Signer: common.HexToAddress("0x" + strings.Repeat("5", 40)), MerchantID: [32]byte{0: 0xe1, 31: 0x07}, KeyVersion: 2,
			BeneficiaryVersion: 3, Escrow: true, Token: wonForOptions}}}
	h.Register(certified)
	h.RegisterCustomer(entrance)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	server.StartTLS()
	customers := httptest.NewServer(entrance) // the entrance asks for no client certificate
	defer customers.Close()

	send := func(client *http.Client, method, url string, header map[string]string, body any, want int) map[string]any {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		req, _ := http.NewRequestWithContext(ctx, method, url, reader)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		reply, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		out, _ := io.ReadAll(reply.Body)
		if reply.StatusCode != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, url, reply.StatusCode, want, out)
		}
		var decoded map[string]any
		_ = json.Unmarshal(out, &decoded)
		return decoded
	}
	staff := func(cert tls.Certificate) *http.Client {
		client := server.Client()
		transport := client.Transport.(*http.Transport).Clone()
		transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
		return &http.Client{Transport: transport}
	}
	write := func(cert tls.Certificate, branch string, page map[string]any, want int) map[string]any {
		t.Helper()
		return send(staff(cert), "POST", server.URL+"/v2/operations/store-page/"+branch,
			map[string]string{"Origin": origin, "Content-Type": "application/json"}, page, want)
	}
	read := func(path string, want int) map[string]any {
		t.Helper()
		return send(http.DefaultClient, "GET", customers.URL+path, nil, nil, want)
	}
	listed := func() []map[string]any {
		t.Helper()
		var mine []map[string]any
		for _, row := range read("/v2/customer/stores", 200)["stores"].([]any) {
			if store := row.(map[string]any); slices.Contains([]string{cafe, bakery, unnamed}, store["branchRef"].(string)) {
				mine = append(mine, store)
			}
		}
		return mine
	}
	weekdays := func(opens, closes string) []any {
		day := map[string]string{"opens": opens, "closes": closes}
		return []any{day, day, day, day, day, day, nil}
	}
	page := func() map[string]any {
		return map[string]any{"listed": true, "kind": "카페", "intro": "  직접 볶은 원두로 내립니다  ", "address": "시험시 시험로 1",
			"directions": "", "phone": "02-555-0100", "amenities": "무선 인터넷", "parking": "", "closedNote": "매주 일요일",
			"hours": weekdays("10:00", "21:00")}
	}

	// --- before the store writes anything: a named store has a page with its name and order state only ---
	if got := listed(); len(got) != 0 {
		t.Fatalf("a store that published nothing is listed: %v", got)
	}
	bare := read("/v2/customer/stores/"+bakery, 200)
	status := bare["status"].(map[string]any)
	if bare["name"] != "모퉁이 베이커리 "+suffix || bare["kind"] != nil || bare["hours"] != nil || status["mode"] != "open" ||
		status["timezone"] != "Asia/Seoul" || bare["paymentToken"] != nil || bare["merchantId"] != nil || bare["operatorContact"] != "help@operator.example" {
		t.Fatalf("bare page: %v", bare)
	}
	read("/v2/customer/stores/"+unnamed, 404) // no registered name, no page
	read("/v2/customer/stores/"+fresh(), 404) // no such branch
	read("/v2/customer/stores/not-a-branch", 404)
	read("/v2/customer/stores?tenant="+tenant, 404)

	// --- the store's operator writes the page; others cannot ---
	saved := write(ownCert, cafe, page(), 200)
	if saved["saved"] != true || saved["intro"] != "직접 볶은 원두로 내립니다" || saved["changedBy"] != operator {
		t.Fatalf("saved: %v", saved)
	}
	write(strangerCert, cafe, page(), 404)
	write(ownCert, unnamed, page(), 400) // a store without a registered name cannot publish
	send(staff(ownCert), "POST", server.URL+"/v2/operations/store-page/"+cafe, map[string]string{"Content-Type": "application/json"}, page(), 404)
	for name, broken := range map[string]func(map[string]any){
		"missing field": func(p map[string]any) { delete(p, "parking") },
		"unknown field": func(p map[string]any) { p["rating"] = "5" },
		"no kind":       func(p map[string]any) { p["kind"] = " " },
		"long intro":    func(p map[string]any) { p["intro"] = strings.Repeat("가", 201) },
		"control char":  func(p map[string]any) { p["address"] = "a\nb" },
		"phone":         func(p map[string]any) { p["phone"] = "call me" },
		"six days":      func(p map[string]any) { p["hours"] = weekdays("10:00", "21:00")[:6] },
		"bad time":      func(p map[string]any) { p["hours"] = weekdays("10:00", "24:00") },
		"same time":     func(p map[string]any) { p["hours"] = weekdays("10:00", "10:00") },
		"extra hour key": func(p map[string]any) {
			p["hours"].([]any)[6] = map[string]string{"opens": "10:00", "closes": "11:00", "note": "x"}
		},
		"hours not array": func(p map[string]any) { p["hours"] = "always" },
	} {
		p := page()
		broken(p)
		if got := write(ownCert, cafe, p, 400); got["error"] != "BAD_REQUEST" {
			t.Fatalf("%s: %v", name, got)
		}
	}
	own := send(staff(ownCert), "GET", server.URL+"/v2/operations/store-page/"+cafe, nil, nil, 200)
	if own["kind"] != "카페" || own["listed"] != true || own["name"] != "여울 커피 "+suffix {
		t.Fatalf("a refused change altered the page: %v", own)
	}
	send(staff(strangerCert), "GET", server.URL+"/v2/operations/store-page/"+cafe, nil, nil, 404)
	if first := send(staff(strangerCert), "GET", server.URL+"/v2/operations/store-page/"+bakery, nil, nil, 200); first["saved"] != false || first["hours"] != nil {
		t.Fatalf("unsaved page: %v", first)
	}

	// --- what customers read: the published fields, empty ones left out, and nothing else ---
	got := read("/v2/customer/stores/"+cafe, 200)
	allowed := []string{"branchRef", "name", "kind", "intro", "address", "phone", "amenities", "closedNote", "hours", "status", "paymentToken", "merchantId", "operatorContact"}
	for key := range got {
		if !slices.Contains(allowed, key) {
			t.Fatalf("the page gives out %q: %v", key, got)
		}
	}
	if len(got) != len(allowed) || got["branchRef"] != cafe || got["kind"] != "카페" || got["phone"] != "02-555-0100" ||
		got["paymentToken"] != "tKRW" || got["merchantId"] != "e1"+strings.Repeat("0", 60)+"07" ||
		len(got["hours"].([]any)) != 7 || got["hours"].([]any)[6] != nil {
		t.Fatalf("page: %v", got)
	}
	// The page tells the store's contract number only: no address, chain, signer or version of the payment profile.
	if raw, _ := json.Marshal(got); strings.Contains(string(raw), strings.Repeat("3", 40)) || strings.Contains(string(raw), strings.Repeat("4", 40)) ||
		strings.Contains(string(raw), strings.Repeat("5", 40)) || strings.Contains(string(raw), strings.Repeat("2", 40)) || strings.Contains(string(raw), "8283") {
		t.Fatalf("the page names a payment address or chain: %s", raw)
	}
	status = got["status"].(map[string]any)
	if weekday, ok := status["weekday"].(float64); !ok || weekday < 0 || weekday > 6 || len(status["time"].(string)) != 5 {
		t.Fatalf("status: %v", status)
	}
	rows := listed()
	if len(rows) != 1 || rows[0]["name"] != "여울 커피 "+suffix || rows[0]["address"] != "시험시 시험로 1" || rows[0]["intro"] != nil ||
		rows[0]["phone"] != nil || len(rows[0]["hours"].([]any)) != 7 || len(rows[0]) != 6 {
		t.Fatalf("list: %v", rows)
	}

	// --- the list follows the store's choice and its order state, sorted by name ---
	other := page()
	other["kind"], other["hours"] = "베이커리", nil
	write(strangerCert, bakery, other, 200)
	if rows = listed(); len(rows) != 2 || rows[0]["branchRef"] != bakery || rows[1]["branchRef"] != cafe || rows[0]["hours"] != nil {
		t.Fatalf("two stores by name: %v", rows)
	}
	send(staff(ownCert), "POST", server.URL+"/v2/operations/state/"+cafe+"/mode", map[string]string{"Origin": origin, "Content-Type": "application/json"},
		map[string]any{"mode": "busy", "busyMinutes": 25}, 200)
	if status = read("/v2/customer/stores/"+cafe, 200)["status"].(map[string]any); status["mode"] != "busy" || status["busyMinutes"] != float64(25) {
		t.Fatalf("busy: %v", status)
	}
	execute("INSERT INTO ledger.branch_closed_day VALUES ($1, ledger.branch_today($1, clock_timestamp()), $2, clock_timestamp())", cafe, operator)
	if status = listed()[1]["status"].(map[string]any); status["mode"] != "closed" {
		t.Fatalf("closed day: %v", status)
	}
	unlisted := page()
	unlisted["listed"] = false
	write(ownCert, cafe, unlisted, 200)
	if rows = listed(); len(rows) != 1 || rows[0]["branchRef"] != bakery {
		t.Fatalf("an unlisted store is listed: %v", rows)
	}
	read("/v2/customer/stores/"+cafe, 200) // the shortcut QR still opens it

	// --- the entrance limits these reads like its other requests ---
	h.Customer.Limiter = NewRateLimiter(60, 2, nil)
	read("/v2/customer/stores", 200)
	read("/v2/customer/stores/"+cafe, 200)
	read("/v2/customer/stores", 429)
}
