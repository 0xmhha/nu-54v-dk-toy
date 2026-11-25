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
	"fmt"
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

func text(ko, en string) map[string]string { return map[string]string{"ko": ko, "en": en} }

func latte() []skuOptions {
	milk := []string{"milk"}
	return []skuOptions{{SKU: "latte", Groups: []optionGroup{
		{Key: "temp", Labels: text("온도", "Temperature"), Values: []optionValue{{Key: "hot", Labels: text("뜨거움", "Hot")}, {Key: "ice", Labels: text("차가움", "Iced"), Price: "500"}}},
		{Key: "milk", Labels: map[string]string{"ko": "우유"}, Values: []optionValue{{Key: "milk", Labels: text("우유", "Milk"), Allergens: &milk},
			{Key: "oat", Labels: map[string]string{"ko": "귀리"}, Price: "1000"}}},
	}}}
}

func latteRow() operatorRow {
	soy := []string{"soy"}
	return operatorRow{SKU: "latte", Label: "라떼", Price: "450000", DisplayPrice: "450000", Tax: "0", Discount: "0",
		Metadata: &menuMetadata{Labels: text("라떼", "Latte"), Allergens: &soy}}
}

var wonForOptions = Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}

func TestMenuOptionCombinations(t *testing.T) {
	options := latte()
	rows, err := expandOptions([]operatorRow{{SKU: "cookie", Option: "standard", Price: "1"}, latteRow()}, options, 2, wonForOptions)
	if err != nil || len(rows) != 5 || rows[0].SKU != "cookie" {
		t.Fatal(rows, err)
	}
	for index, want := range []struct {
		option, price, ko, en string
		allergens             []string
	}{
		{"hot-milk", "450000", "뜨거움 · 우유", "Hot · Milk", []string{"soy", "milk"}},
		{"hot-oat", "550000", "뜨거움 · 귀리", "Hot · 귀리", []string{"soy"}},
		{"ice-milk", "500000", "차가움 · 우유", "Iced · Milk", []string{"soy", "milk"}},
		{"ice-oat", "600000", "차가움 · 귀리", "Iced · 귀리", []string{"soy"}},
	} {
		row := rows[index+1]
		if row.Option != want.option || row.Price != want.price || row.DisplayPrice != want.price || !row.Required ||
			row.Metadata.OptionLabels["ko"] != want.ko || row.Metadata.OptionLabels["en"] != want.en ||
			row.Metadata.OptionGroupLabels["ko"] != "온도 · 우유" || row.Metadata.OptionGroupLabels["en"] != "Temperature · 우유" ||
			!slices.Equal(*row.Metadata.Allergens, want.allergens) || !row.Metadata.valid() {
			t.Fatalf("%+v %+v", row, row.Metadata)
		}
	}
	twice, listed := latte(), latteRow()
	listed.Metadata.Allergens = &[]string{"milk"}
	if rows, err := expandOptions([]operatorRow{listed}, twice, 2, wonForOptions); err != nil || !slices.Equal(*rows[0].Metadata.Allergens, []string{"milk"}) {
		t.Fatal("an ingredient listed twice", err)
	}
	if base := options[0].Base; base == nil || base.DisplayPrice != "450000" || base.Label != "라떼" || len(base.Metadata.OptionLabels) != 0 ||
		options[0].Groups[0].Values[1].Price != "50000" || options[0].Groups[0].Values[0].Price != "0" {
		t.Fatalf("%+v", options[0])
	}
	if shown, _ := json.Marshal(options[0].public()); strings.Contains(string(shown), "price") || strings.Contains(string(shown), "allergens") {
		t.Fatal(string(shown))
	}
}

func TestMenuOptionRefusals(t *testing.T) {
	plain := func(*operatorRow, *skuOptions) {}
	for name, change := range map[string]func(*operatorRow, *skuOptions){
		"":                                   plain,
		"a part of a won":                    func(_ *operatorRow, o *skuOptions) { o.Groups[0].Values[0].Price = "0.5" },
		"a lower price":                      func(_ *operatorRow, o *skuOptions) { o.Groups[0].Values[0].Price = "-500" },
		"a dearer choice on a taxed product": func(r *operatorRow, _ *skuOptions) { r.Tax = "100" },
		"an ingredient on a product without a list": func(r *operatorRow, _ *skuOptions) { r.Metadata.Allergens = nil },
		"its own option text":                       func(r *operatorRow, _ *skuOptions) { r.Metadata.OptionLabels = text("뜨거움", "Hot") },
		"a price in base units only":                func(r *operatorRow, _ *skuOptions) { r.DisplayPrice = "" },
		"a key above 32 characters": func(_ *operatorRow, o *skuOptions) {
			o.Groups[0].Values[0].Key, o.Groups[1].Values[0].Key = strings.Repeat("a", 16), strings.Repeat("b", 16)
		},
		"the reserved key":             func(_ *operatorRow, o *skuOptions) { o.Groups = o.Groups[:1]; o.Groups[0].Values[0].Key = "standard" },
		"a product that is not listed": func(r *operatorRow, _ *skuOptions) { r.SKU = "mocha" },
	} {
		row, options := latteRow(), latte()
		if change(&row, &options[0]); !options[0].valid() {
			t.Fatal(name, "refused too early")
		}
		if _, err := expandOptions([]operatorRow{row}, options, 2, wonForOptions); (err == nil) != (name == "") {
			t.Error(name, err)
		}
	}
	if _, err := expandOptions([]operatorRow{latteRow(), latteRow()}, latte(), 2, wonForOptions); err == nil {
		t.Error("two rows for one product")
	}
	if _, err := expandOptions([]operatorRow{latteRow()}, append(latte(), latte()...), 2, wonForOptions); err == nil {
		t.Error("two definitions for one product")
	}
	for name, change := range map[string]func(*skuOptions){
		"no group": func(o *skuOptions) { o.Groups = nil }, "five groups": func(o *skuOptions) {
			for len(o.Groups) < 5 {
				more := o.Groups[0]
				more.Key = fmt.Sprint("g", len(o.Groups))
				o.Groups = append(o.Groups, more)
			}
		},
		"one value":        func(o *skuOptions) { o.Groups[0].Values = o.Groups[0].Values[:1] },
		"same group":       func(o *skuOptions) { o.Groups[1].Key = "temp" },
		"same value":       func(o *skuOptions) { o.Groups[0].Values[1].Key = "hot" },
		"a dash":           func(o *skuOptions) { o.Groups[0].Values[1].Key = "i-e" },
		"no Korean":        func(o *skuOptions) { o.Groups[0].Labels = map[string]string{"en": "Temperature"} },
		"long text":        func(o *skuOptions) { o.Groups[0].Labels["ko"] = strings.Repeat("가", 25) },
		"bidi text":        func(o *skuOptions) { o.Groups[0].Labels["en"] = "Hot‮" },
		"unknown language": func(o *skuOptions) { o.Groups[0].Labels["fr"] = "Chaud" },
		"unknown allergen": func(o *skuOptions) { o.Groups[0].Values[0].Allergens = &[]string{"dust"} },
		"base from client": func(o *skuOptions) { o.Base = &optionBase{} },
		"thirteen values": func(o *skuOptions) {
			for len(o.Groups[0].Values) < 13 {
				o.Groups[0].Values = append(o.Groups[0].Values, optionValue{Key: fmt.Sprint("v", len(o.Groups[0].Values)), Labels: text("값", "v")})
			}
		},
	} {
		options := latte()
		if change(&options[0]); options[0].valid() {
			t.Error(name)
		}
	}
}

func bigOptions(sku string, counts ...int) skuOptions {
	out := skuOptions{SKU: sku}
	for g, count := range counts {
		group := optionGroup{Key: fmt.Sprint("g", g), Labels: text(fmt.Sprint("묶음", g), fmt.Sprint("Group ", g))}
		for v := range count {
			group.Values = append(group.Values, optionValue{Key: fmt.Sprint("v", v), Labels: text(fmt.Sprint("고르기", v), fmt.Sprint("Choice ", v))})
		}
		out.Groups = append(out.Groups, group)
	}
	return out
}

func TestMenuOptionLimits(t *testing.T) {
	row := func(sku string) operatorRow {
		return operatorRow{SKU: sku, Label: sku, Price: "100", DisplayPrice: "100", Tax: "0", Discount: "0"}
	}
	rows, err := expandOptions([]operatorRow{row("a")}, []skuOptions{bigOptions("a", 10, 10, 6)}, 2, wonForOptions)
	if err != nil || len(rows) != 600 {
		t.Fatal(len(rows), err)
	}
	for _, options := range [][]skuOptions{{bigOptions("a", 12, 12, 5)}, {bigOptions("a", 10, 10, 6), bigOptions("b", 2)}} {
		products := []operatorRow{row("a")}
		if len(options) == 2 {
			products = append(products, row("b"))
		}
		if _, err := expandOptions(products, options, 2, wonForOptions); err == nil {
			t.Error("more than 600 rows")
		}
	}
	if !readableCatalog(rows, []skuOptions{bigOptions("a", 10, 10, 6)}, nil, nil) {
		t.Error("an ordinary menu of 600 rows was refused")
	}
	wide := operatorRow{SKU: "a", Option: "b", Label: strings.Repeat("가", 120), Metadata: &menuMetadata{
		Labels: text(strings.Repeat("가", 120), strings.Repeat("나", 120)), OptionLabels: text(strings.Repeat("가", 120), strings.Repeat("나", 120))}}
	if readableCatalog([]operatorRow{wide}, nil, nil, nil) {
		t.Error("a row above the nearby limit")
	}
	var page []operatorRow
	for index := range 50 {
		page = append(page, operatorRow{SKU: fmt.Sprintf("s%02d", index), Option: "b", Label: "a", Price: "1", Asset: "0x" + strings.Repeat("2", 40),
			Metadata: &menuMetadata{Labels: text(strings.Repeat("가", 80), strings.Repeat("나", 80))}})
	}
	if readableCatalog(page, nil, nil, nil) || !readableCatalog(page[:30], nil, nil, nil) {
		t.Error("the page limit")
	}
}

func TestPostgresMenuOptions(t *testing.T) {
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
	role := "nu54_options_" + strings.TrimPrefix(config.Database, "nu54_commerce_")
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
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", operator, "options_"+operator)
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
		req, _ := http.NewRequestWithContext(ctx, method, server.URL+"/v2/operations/catalog/"+branch+path, bytes.NewReader(raw))
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
	row := func() operatorRow {
		row := latteRow()
		row.Asset, row.DisplayPrice, row.Price, row.Tax, row.Discount = asset, "4500", "", "", ""
		return row
	}
	publication := func(version int64, rows []operatorRow, options []skuOptions) map[string]any {
		return map[string]any{"requestKey": fresh(), "version": version, "expiresAt": time.Now().Add(time.Hour).UTC(), "rows": rows, "options": options}
	}
	window := func(page int32) ([]Price, []CatalogOptions) {
		t.Helper()
		rows, _, options, err := store.catalogMenuWindow(ctx, authorize(t, actor{customer, "customer"}, "catalog.read", branch, branch, "",
			map[string]string{"page": fmt.Sprint(page)}), page)
		if err != nil {
			t.Fatal(err)
		}
		return rows, options
	}

	first := publication(0, []operatorRow{row(), {SKU: "cookie", Option: "standard", Label: "쿠키", Asset: asset, DisplayPrice: "3000"}}, latte())
	if got := call("POST", "/publish", first, 200)["version"]; got != float64(1) {
		t.Fatal(got)
	}
	call("POST", "/publish", first, 200) // the same request again
	changed := latte()
	changed[0].Groups[0].Labels = text("온도 선택", "Temperature")
	first["options"] = changed
	call("POST", "/publish", first, 409) // the same key with another definition
	if _, err := store.db.Exec(ctx, "SELECT ledger.catalog_menu_window($1,$2,$3,0)", tenant, branch, operator); err == nil {
		t.Error("an operator without a customer grant read the menu")
	}
	rows, options := window(0)
	if len(rows) != 5 || len(options) != 1 || options[0].SKU != "latte" || len(options[0].Groups) != 2 ||
		options[0].Groups[0].Values[1].Key != "ice" || options[0].Groups[1].Labels["en"] != "" {
		t.Fatalf("%+v %+v", rows, options)
	}
	if last := rows[4]; last.Option != "ice-oat" || last.UnitPrice != "600000" || !last.Metadata.OptionRequired || last.Metadata.OptionLabels["ko"] != "차가움 · 귀리" {
		t.Fatalf("%+v", last)
	}
	home := call("GET", "", nil, 200)["catalog"].(map[string]any)
	saved := home["options"].([]any)[0].(map[string]any)
	if saved["base"].(map[string]any)["displayPrice"] != "450000" || strings.Contains(fmt.Sprint(home["breakdown"]), "Intent") {
		t.Fatal(saved, home["breakdown"])
	}
	stored, err := json.Marshal(saved["groups"])
	var groups []optionGroup
	if err != nil || json.Unmarshal(stored, &groups) != nil || groups[0].Values[1].Price != "50000" || (*groups[1].Values[0].Allergens)[0] != "milk" {
		t.Fatal(string(stored))
	}

	call("POST", "/stock", map[string]any{"requestKey": fresh(), "version": 1, "sku": "latte", "delta": 5}, 200)
	call("POST", "/option-stock", map[string]any{"requestKey": fresh(), "version": 1, "sku": "latte", "option": "ice-oat", "delta": 1}, 200)
	order := func(option string) error {
		ref := fresh()
		_, err := store.Reserve(ctx, authorize(t, actor{customer, "customer"}, "order.create", branch, ref, fresh(), map[string]string{
			"asset": asset, "quantity": "1", "intentDigest": strings.Repeat("a", 64)}),
			Cart{Origin: "menu", OriginRef: ref, MenuVersion: 1, Items: []Item{{SKU: "latte", Option: option, Qty: 1}}})
		return err
	}
	if err := order("ice-oat"); err != nil {
		t.Fatal(err)
	}
	if err := order("ice-oat"); err == nil {
		t.Fatal("sold a combination above its own stock")
	}
	if err := order("hot-oat"); err != nil {
		t.Fatal(err)
	}
	var amounts string
	if err := admin.QueryRow(ctx, "SELECT string_agg(amount::text,',' ORDER BY amount) FROM ledger.sale WHERE branch_id=$1", branch).Scan(&amounts); err != nil || amounts != "550000,600000" {
		t.Fatal(amounts, err)
	}

	for name, body := range map[string]map[string]any{
		"rows missing":          publication(1, []operatorRow{{SKU: "cookie", Option: "standard", Label: "쿠키", Asset: asset, DisplayPrice: "3000"}}, latte()),
		"an option on the base": publication(1, []operatorRow{func() operatorRow { r := row(); r.Option = "hot"; return r }()}, latte()),
		"no option at all":      publication(1, []operatorRow{{SKU: "cookie", Label: "쿠키", Asset: asset, DisplayPrice: "3000"}}, nil),
	} {
		if call("POST", "/publish", body, 400); name == "" {
			t.Fatal(name)
		}
	}
	// The database refuses what a caller other than the server's own code could send.
	direct := func(lines []operatorRow, options any) error {
		rawLines, _ := json.Marshal(lines)
		rawOptions, _ := json.Marshal(options)
		_, err := store.db.Exec(ctx, "SELECT ledger.publish_operator_catalog_options($1,$2,$3,$4,1,now()+interval '1 hour',$5,null,'[]','[]',$6)",
			tenant, branch, operator, fresh(), rawLines, rawOptions)
		return err
	}
	expanded := func() ([]operatorRow, []skuOptions) {
		options, base := latte(), row()
		base.DisplayPrice, base.Price, base.Tax, base.Discount = "450000", "450000", "0", "0"
		lines, err := expandOptions([]operatorRow{base}, options, 2, wonForOptions)
		if err != nil {
			t.Fatal(err)
		}
		return lines, options
	}
	for name, change := range map[string]func([]operatorRow, []skuOptions) ([]operatorRow, []skuOptions){
		"a missing combination": func(l []operatorRow, o []skuOptions) ([]operatorRow, []skuOptions) { return l[1:], o },
		"another combination": func(l []operatorRow, o []skuOptions) ([]operatorRow, []skuOptions) {
			l[0].Option = "warm-milk"
			return l, o
		},
		"an optional row": func(l []operatorRow, o []skuOptions) ([]operatorRow, []skuOptions) {
			for i := range l {
				l[i].Required = false
			}
			return l, o
		},
		"long text": func(l []operatorRow, o []skuOptions) ([]operatorRow, []skuOptions) {
			o[0].Groups[0].Labels["ko"] = strings.Repeat("가", 25)
			return l, o
		},
		"two definitions": func(l []operatorRow, o []skuOptions) ([]operatorRow, []skuOptions) { return l, append(o, o...) },
	} {
		lines, options := change(expanded())
		if direct(lines, options) == nil {
			t.Error(name)
		}
	}
	for _, function := range []string{"publish_operator_catalog_pricing($1,$2,$3,$4,1,now()+interval '1 hour',$5,null)",
		"publish_operator_catalog_categories($1,$2,$3,$4,1,now()+interval '1 hour',$5,null,'[]','[]')"} {
		if _, err := store.db.Exec(ctx, "SELECT ledger."+function, tenant, branch, operator, fresh(),
			`[{"sku":"a","option":"b","label":"c","asset":"`+asset+`","price":"1","_optionIntent":"x"}]`); err == nil {
			t.Error("took the private field", function)
		}
	}
	if lines, options := expanded(); direct(lines, options) != nil {
		t.Error("the same rows unchanged were refused")
	}

	big := row()
	big.SKU, big.Metadata = "big", nil
	published := publication(2, []operatorRow{big}, []skuOptions{bigOptions("big", 10, 10, 6)})
	call("POST", "/publish", published, 200)
	started := time.Now()
	total, widest := 0, 0
	for page := int32(0); ; page++ {
		rows, options := window(page)
		raw, _ := json.Marshal(result{Catalog: rows, CatalogOptions: options})
		widest = max(widest, len(raw))
		if total += len(rows); len(rows) < 50 {
			break
		}
		if len(options) != 1 {
			t.Fatal("a page without its choices")
		}
	}
	t.Logf("600 rows: widest page %d bytes, all pages read in %s", widest, time.Since(started))
	if total != 600 || widest > 32768 {
		t.Fatal(total, widest)
	}
	second := big
	second.SKU, second.Option = "extra", "standard"
	call("POST", "/publish", publication(3, []operatorRow{big, second}, []skuOptions{bigOptions("big", 10, 10, 6)}), 400)
}
