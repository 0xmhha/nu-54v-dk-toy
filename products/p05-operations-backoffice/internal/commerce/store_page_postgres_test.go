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
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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
		return map[string]any{"published": true, "listed": true, "kind": "카페", "intro": "  직접 볶은 원두로 내립니다  ", "address": "시험시 시험로 1",
			"directions": "", "phone": "02-555-0100", "amenities": "무선 인터넷", "parking": "", "closedNote": "매주 일요일",
			"hours": weekdays("10:00", "21:00")}
	}

	// --- nothing of a store is public until the store says so: its reference and its name open nothing ---
	if got := listed(); len(got) != 0 {
		t.Fatalf("a store that published nothing is listed: %v", got)
	}
	// Every closed store answers exactly like a branch that does not exist.
	answer := func(path string) string {
		t.Helper()
		reply, err := http.Get(customers.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		body, _ := io.ReadAll(reply.Body)
		return reply.Status + " " + reply.Header.Get("Content-Type") + " " + string(body)
	}
	absent := fresh()
	closed := func(when, branch string) {
		t.Helper()
		for _, path := range []string{"", "/menu/0", "/photos/" + strings.Repeat("a", 64) + "/0"} {
			got, none := answer("/v2/customer/stores/"+branch+path), answer("/v2/customer/stores/"+absent+path)
			if got != none || !strings.HasPrefix(got, "404 ") {
				t.Fatalf("%s%s: %q, a branch that does not exist answers %q", when, path, got, none)
			}
		}
	}
	closed("a store with a registered name and no page", bakery)
	closed("a store without a registered name", unnamed)
	read("/v2/customer/stores/not-a-branch", 404)
	read("/v2/customer/stores?tenant="+tenant, 404)
	// A page that is saved without being published stays closed, whatever it says about the list.
	draft := page()
	draft["published"] = false
	if kept := write(ownCert, cafe, draft, 200); kept["saved"] != true || kept["published"] != false || kept["listed"] != true {
		t.Fatalf("a saved, unpublished page: %v", kept)
	}
	closed("a store that saved its page without publishing it", cafe)
	if got := listed(); len(got) != 0 {
		t.Fatalf("an unpublished store is listed: %v", got)
	}

	// --- the store's operator writes and publishes the page; others cannot ---
	saved := write(ownCert, cafe, page(), 200)
	if saved["saved"] != true || saved["published"] != true || saved["intro"] != "직접 볶은 원두로 내립니다" || saved["changedBy"] != operator {
		t.Fatalf("saved: %v", saved)
	}
	write(strangerCert, cafe, page(), 404)
	write(ownCert, unnamed, page(), 400) // a store without a registered name cannot publish
	send(staff(ownCert), "POST", server.URL+"/v2/operations/store-page/"+cafe, map[string]string{"Content-Type": "application/json"}, page(), 404)
	for name, broken := range map[string]func(map[string]any){
		"missing field":           func(p map[string]any) { delete(p, "parking") },
		"not said whether public": func(p map[string]any) { delete(p, "published") },
		"public is not yes or no": func(p map[string]any) { p["published"] = "yes" },
		"unknown field":           func(p map[string]any) { p["rating"] = "5" },
		"no kind":                 func(p map[string]any) { p["kind"] = " " },
		"long intro":              func(p map[string]any) { p["intro"] = strings.Repeat("가", 201) },
		"control char":            func(p map[string]any) { p["address"] = "a\nb" },
		// Characters that do not show: they hide text, reorder it or break the line on a customer's screen.
		"zero width space":      func(p map[string]any) { p["kind"] = "카\u200b페" },
		"zero width joiner":     func(p map[string]any) { p["intro"] = "a\u200db" },
		"direction override":    func(p map[string]any) { p["intro"] = "환불 불가 \u202e다니됩 불환" },
		"direction isolate":     func(p map[string]any) { p["directions"] = "a\u2066b\u2069" },
		"left-to-right mark":    func(p map[string]any) { p["parking"] = "a\u200eb" },
		"line separator":        func(p map[string]any) { p["address"] = "a\u2028b" },
		"paragraph separator":   func(p map[string]any) { p["amenities"] = "a\u2029b" },
		"soft hyphen":           func(p map[string]any) { p["closedNote"] = "a\u00adb" },
		"byte order mark":       func(p map[string]any) { p["intro"] = "a\ufeffb" },
		"word joiner":           func(p map[string]any) { p["intro"] = "a\u2060b" },
		"tag character":         func(p map[string]any) { p["intro"] = "a\U000E0041b" },
		"control above ASCII":   func(p map[string]any) { p["intro"] = "a\u0085b" },
		"delete character":      func(p map[string]any) { p["intro"] = "a\u007fb" },
		"format char at an end": func(p map[string]any) { p["kind"] = "카페\u200b" },
		// Characters that are neither format characters nor controls and still do not show, or show as a blank.
		"hangul filler":         func(p map[string]any) { p["intro"] = "a\u3164b" },
		"hangul filler alone":   func(p map[string]any) { p["kind"] = "\u3164" },
		"choseong filler":       func(p map[string]any) { p["intro"] = "a\u115fb" },
		"halfwidth filler":      func(p map[string]any) { p["intro"] = "a\uffa0b" },
		"blank braille":         func(p map[string]any) { p["kind"] = "\u2800\u2800" },
		"no-break space":        func(p map[string]any) { p["amenities"] = "100\u00a0석" },
		"no-break space alone":  func(p map[string]any) { p["kind"] = "\u00a0" },
		"ideographic space":     func(p map[string]any) { p["kind"] = "\u3000" },
		"thin space":            func(p map[string]any) { p["intro"] = "a\u2009b" },
		"variation selector":    func(p map[string]any) { p["intro"] = "a\ufe0fb" },
		"variation selector 17": func(p map[string]any) { p["intro"] = "a\U000E0100b" },
		"grapheme joiner":       func(p map[string]any) { p["intro"] = "a\u034fb" },
		"mongolian selector":    func(p map[string]any) { p["intro"] = "a\u180bb" },
		"spaces only":           func(p map[string]any) { p["kind"] = "   " },
		"phone":                 func(p map[string]any) { p["phone"] = "call me" },
		"six days":              func(p map[string]any) { p["hours"] = weekdays("10:00", "21:00")[:6] },
		"bad time":              func(p map[string]any) { p["hours"] = weekdays("10:00", "24:00") },
		"same time":             func(p map[string]any) { p["hours"] = weekdays("10:00", "10:00") },
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
	// What shows is kept as written: other scripts, accents, a symbol and a hyphen that does not break.
	shown := page()
	shown["intro"], shown["amenities"], shown["directions"] = "Café №1 ☕ 営業中", "Wi\u2011Fi · 100 석", "شارع ١"
	if kept := write(ownCert, cafe, shown, 200); kept["intro"] != "Café №1 ☕ 営業中" || kept["amenities"] != "Wi\u2011Fi · 100 석" || kept["directions"] != "شارع ١" {
		t.Fatalf("visible text was changed: %v", kept)
	}
	write(ownCert, cafe, page(), 200)
	own := send(staff(ownCert), "GET", server.URL+"/v2/operations/store-page/"+cafe, nil, nil, 200)
	if own["kind"] != "카페" || own["published"] != true || own["listed"] != true || own["name"] != "여울 커피 "+suffix {
		t.Fatalf("a refused change altered the page: %v", own)
	}
	send(staff(strangerCert), "GET", server.URL+"/v2/operations/store-page/"+cafe, nil, nil, 404)
	if first := send(staff(strangerCert), "GET", server.URL+"/v2/operations/store-page/"+bakery, nil, nil, 200); first["saved"] != false || first["published"] != false || first["hours"] != nil {
		t.Fatalf("unsaved page: %v", first)
	}

	// --- what customers read: the published fields, empty ones left out, and nothing else ---
	got := read("/v2/customer/stores/"+cafe, 200)
	allowed := []string{"branchRef", "name", "kind", "intro", "address", "phone", "amenities", "closedNote", "hours", "status", "merchantId", "operatorContact"}
	for key := range got {
		if !slices.Contains(allowed, key) {
			t.Fatalf("the page gives out %q: %v", key, got)
		}
	}
	if len(got) != len(allowed) || got["branchRef"] != cafe || got["kind"] != "카페" || got["phone"] != "02-555-0100" ||
		got["merchantId"] != "e1"+strings.Repeat("0", 60)+"07" || got["operatorContact"] != "help@operator.example" ||
		len(got["hours"].([]any)) != 7 || got["hours"].([]any)[6] != nil {
		t.Fatalf("page: %v", got)
	}
	// The page tells the store's contract number only: no token, address, chain, signer or version of the payment profile.
	if raw, _ := json.Marshal(got); strings.Contains(string(raw), strings.Repeat("3", 40)) || strings.Contains(string(raw), strings.Repeat("4", 40)) ||
		strings.Contains(string(raw), strings.Repeat("5", 40)) || strings.Contains(string(raw), strings.Repeat("2", 40)) || strings.Contains(string(raw), "8283") ||
		strings.Contains(string(raw), "paymentToken") || strings.Contains(string(raw), "KRW") {
		t.Fatalf("the page names a payment token, address or chain: %s", raw)
	}
	// The cafe has no menu on sale yet, so it does not read as open: no order can be placed with it.
	status := got["status"].(map[string]any)
	if weekday, ok := status["weekday"].(float64); !ok || weekday < 0 || weekday > 6 || len(status["time"].(string)) != 5 || status["timezone"] != "Asia/Seoul" ||
		status["mode"] != "closed" || status["orderable"] != false || status["reason"] != "no_menu" {
		t.Fatalf("status without a menu: %v", status)
	}
	// With a menu on sale it takes orders.
	taking := func(when, mode string, orderable bool, reason any) map[string]any {
		t.Helper()
		status := read("/v2/customer/stores/"+cafe, 200)["status"].(map[string]any)
		if status["mode"] != mode || status["orderable"] != orderable || status["reason"] != reason {
			t.Fatalf("%s: %v, want %s, orderable %v, reason %v", when, status, mode, orderable, reason)
		}
		// The menu a customer opens agrees: rows to order from exactly when an order can be placed.
		rows := len(read("/v2/customer/stores/"+cafe+"/menu/0", 200)["catalog"].([]any))
		if (rows != 0) != orderable {
			t.Fatalf("%s: the status says orderable %v and the menu has %d rows", when, orderable, rows)
		}
		return status
	}
	execute("INSERT INTO ledger.command_rule VALUES ($1,'page test','fixture','dispose',interval '2 minutes')", cafe)
	send(staff(ownCert), "POST", server.URL+"/v2/operations/catalog/"+cafe+"/publish", map[string]string{"Origin": origin, "Content-Type": "application/json"},
		map[string]any{"requestKey": fresh(), "version": 0, "expiresAt": time.Now().Add(time.Hour).UTC(),
			"rows": []operatorRow{{SKU: "tea", Option: "standard", Label: "tea", Asset: "0x" + strings.Repeat("2", 40), DisplayPrice: "3000"}}}, 200)
	taking("a menu on sale", "open", true, nil)
	// Outside the hours of the price list the store's switch still says open, and the status does not.
	execute(`UPDATE ledger.catalog_pricing SET hours=CASE WHEN (clock_timestamp() AT TIME ZONE 'Asia/Seoul')::time < '12:00'
		THEN '{"zone":"Asia/Seoul","opens":"12:30","closes":"23:59"}'::jsonb ELSE '{"zone":"Asia/Seoul","opens":"00:00","closes":"11:30"}'::jsonb END
		WHERE branch_id=$1`, cafe)
	taking("outside the hours of the price list", "closed", false, "outside_hours")
	execute("UPDATE ledger.catalog_pricing SET hours=NULL WHERE branch_id=$1", cafe)
	// A menu that is not on sale yet, and one that ran out.
	execute("UPDATE ledger.menu_release SET published_at=now()+interval '1 hour',expires_at=now()+interval '2 hours' WHERE branch_id=$1", cafe)
	taking("a menu that is not on sale yet", "closed", false, "no_menu")
	execute("UPDATE ledger.menu_release SET published_at=now()-interval '2 hours',expires_at=now()-interval '1 second' WHERE branch_id=$1", cafe)
	taking("a menu that ran out", "closed", false, "no_menu")
	execute("UPDATE ledger.menu_release SET published_at=now()-interval '1 minute',expires_at=now()+interval '1 hour' WHERE branch_id=$1", cafe)
	taking("the menu on sale again", "open", true, nil)
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
	if status = taking("busy", "busy", true, nil); status["busyMinutes"] != float64(25) {
		t.Fatalf("busy: %v", status)
	}
	send(staff(ownCert), "POST", server.URL+"/v2/operations/state/"+cafe+"/mode", map[string]string{"Origin": origin, "Content-Type": "application/json"},
		map[string]any{"mode": "paused", "resumeAt": "15:30"}, 200)
	if status = taking("paused", "paused", false, "paused"); status["resumeAt"] != "15:30" {
		t.Fatalf("paused: %v", status)
	}
	send(staff(ownCert), "POST", server.URL+"/v2/operations/state/"+cafe+"/mode", map[string]string{"Origin": origin, "Content-Type": "application/json"},
		map[string]any{"mode": "open"}, 200)
	execute("INSERT INTO ledger.branch_closed_day VALUES ($1, ledger.branch_today($1, clock_timestamp()), $2, clock_timestamp())", cafe, operator)
	taking("a closed day", "closed", false, "closed_day")
	if status = listed()[1]["status"].(map[string]any); status["mode"] != "closed" || status["orderable"] != false || status["reason"] != "closed_day" {
		t.Fatalf("closed day in the list: %v", status)
	}
	// The bakery published a page and no menu: listed, and not open.
	if status = listed()[0]["status"].(map[string]any); status["mode"] != "closed" || status["orderable"] != false || status["reason"] != "no_menu" {
		t.Fatalf("a listed store without a menu: %v", status)
	}
	unlisted := page()
	unlisted["listed"] = false
	write(ownCert, cafe, unlisted, 200)
	if rows = listed(); len(rows) != 1 || rows[0]["branchRef"] != bakery {
		t.Fatalf("an unlisted store is listed: %v", rows)
	}
	read("/v2/customer/stores/"+cafe, 200) // the shortcut QR still opens it

	// --- a list longer than a page says that it goes on, and where from ---
	write(ownCert, cafe, page(), 200) // listed again: the bakery and the cafe, in that order
	walk := func(size int) (mine []string, pages int) {
		t.Helper()
		h.Customer.ListPage = size
		defer func() { h.Customer.ListPage = 0 }()
		seen := map[string]bool{}
		for path := "/v2/customer/stores"; ; pages++ {
			reply := read(path, 200)
			rows := reply["stores"].([]any)
			if len(rows) > size || (len(rows) == 0 && pages != 0) || pages > 500 {
				t.Fatalf("page %d of %d-store pages holds %d stores", pages, size, len(rows))
			}
			last := ""
			for _, row := range rows {
				last = row.(map[string]any)["branchRef"].(string)
				if seen[last] {
					t.Fatalf("store %s is on two pages", last)
				}
				seen[last] = true
				if last == cafe || last == bakery {
					mine = append(mine, last)
				}
			}
			next, more := reply["next"].(string)
			if !more {
				if _, present := reply["next"]; present {
					t.Fatalf("the last page names a next one: %v", reply["next"])
				}
				return mine, pages + 1
			}
			if next != last { // the position is the last store of the page
				t.Fatalf("next is %s, the page ended with %s", next, last)
			}
			path = "/v2/customer/stores?after=" + next
		}
	}
	whole, one := walk(200)
	if one != 1 || !slices.Equal(whole, []string{bakery, cafe}) {
		t.Fatalf("the whole list in %d pages: %v", one, whole)
	}
	for _, size := range []int{1, 2, 3} {
		if paged, pages := walk(size); !slices.Equal(paged, whole) || (size == 1 && pages < 2) {
			t.Fatalf("pages of %d: %v in %d pages, want %v", size, paged, pages, whole)
		}
	}
	// A list that is cut short says so: with one store a page, the first page names the next.
	h.Customer.ListPage = 1
	if first := read("/v2/customer/stores", 200); len(first["stores"].([]any)) != 1 || first["next"] == nil {
		t.Fatalf("a cut list does not say that it goes on: %v", first)
	}
	h.Customer.ListPage = 0
	// The position is a store's reference and nothing else is taken as a query.
	read("/v2/customer/stores?after="+fresh(), 400) // not a store
	for _, query := range []string{"after=", "after=not-a-branch", "after=" + cafe + "&after=" + bakery, "after=" + cafe + "&tenant=" + tenant,
		"after=" + strings.ToUpper(cafe), "limit=1", "after=" + cafe + "%20"} {
		read("/v2/customer/stores?"+query, 404)
	}
	// A place in the list is a store of the list. A store that is unlisted, or that took its page back, is none,
	// and the answer is the one of a reference that is no store at all: it does not say that such a store exists.
	noPlace := read("/v2/customer/stores?after="+fresh(), 400)
	write(ownCert, cafe, unlisted, 200)
	if after := read("/v2/customer/stores?after="+cafe, 400); !reflect.DeepEqual(after, noPlace) {
		t.Fatalf("after an unlisted store: %v, after no store: %v", after, noPlace)
	}
	// Taking the page back closes it and takes it off the list, though it still says listed.
	hidden := page()
	hidden["published"] = false
	write(ownCert, cafe, hidden, 200)
	if after := read("/v2/customer/stores?after="+cafe, 400); !reflect.DeepEqual(after, noPlace) {
		t.Fatalf("after a store that took its page back: %v, after no store: %v", after, noPlace)
	}
	closed("a store that took its page back", cafe)
	if rows = listed(); len(rows) != 1 || rows[0]["branchRef"] != bakery {
		t.Fatalf("a store that took its page back is listed: %v", rows)
	}
	write(ownCert, cafe, unlisted, 200)

	// --- the store's registered name is shown to customers too, so it is one visible line like the page's text ---
	rename := func(label string) error {
		_, err := admin.Exec(ctx, "SELECT ledger.name_operator_branch($1,$2,$3,$4)", tenant, cafe, operator, label)
		return err
	}
	for name, label := range map[string]string{"hangul filler": "\u3164", "fillers": "\u3164\u3164\u3164", "blank braille": "\u2800",
		"zero width space inside": "여울\u200b커피", "no-break space": "여울\u00a0커피", "direction override": "여울 \u202e피커", "variation selector": "여울\ufe0f"} {
		var refused *pgconn.PgError
		if err := rename(label); !errors.As(err, &refused) || refused.Code != "23514" {
			t.Fatalf("a name with a %s was saved: %v", name, err)
		}
	}
	if err := rename("여울 커피 " + suffix); err != nil {
		t.Fatalf("the store's own name was refused: %v", err)
	}

	// --- a store page is deleted with its branch, as its retention record says ---
	gone := fresh()
	execute("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", gone, tenant)
	execute("INSERT INTO ledger.branch_store_page VALUES ($1,true,true,'카페','','','','','','','',NULL,$2,now())", gone, operator)
	execute("DELETE FROM ledger.branch WHERE id=$1", gone)
	var left int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM ledger.branch_store_page WHERE branch_id=$1", gone).Scan(&left); err != nil || left != 0 {
		t.Fatalf("%d store pages outlived their branch: %v", left, err)
	}

	// --- the entrance limits these reads like its other requests ---
	h.Customer.Limiter = NewRateLimiter(60, 2, nil)
	read("/v2/customer/stores", 200)
	read("/v2/customer/stores/"+cafe, 200)
	read("/v2/customer/stores", 429)
}
