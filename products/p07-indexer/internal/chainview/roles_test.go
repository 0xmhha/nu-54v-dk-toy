package chainview

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func fixtureLogin(t *testing.T, database string, owner *pgxpool.Pool, group string) (string, string) {
	t.Helper()
	ctx := context.Background()
	dbconfig, err := pgxpool.ParseConfig(database)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("%s_fixture_%d", strings.TrimPrefix(group, "nu54_"), time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := owner.Exec(ctx, "CREATE ROLE "+quoted+" LOGIN"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// A separate connection handles cleanup after the scoped reader closes.
		conn, err := pgx.Connect(ctx, database)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "DROP ROLE "+quoted); err != nil {
			t.Error(err)
		}
	})
	if _, err := owner.Exec(ctx, "GRANT "+pgx.Identifier{group}.Sanitize()+" TO "+quoted); err != nil {
		t.Fatal(err)
	}
	settings := url.Values{
		"host": {dbconfig.ConnConfig.Host}, "port": {strconv.Itoa(int(dbconfig.ConnConfig.Port))}, "sslmode": {"disable"},
	}
	uri := url.URL{Scheme: "postgres", User: url.User(name), Path: "/" + dbconfig.ConnConfig.Database, RawQuery: settings.Encode()}
	return name, uri.String()
}

func verifyReceiptRoles(t *testing.T, database string, owner *pgxpool.Pool, stream string, event Event) {
	t.Helper()
	ctx := context.Background()
	_, publicURL := fixtureLogin(t, database, owner, "nu54_p07_public")
	public, err := OpenPublic(ctx, publicURL)
	if err != nil {
		t.Fatal(err)
	}
	defer public.Close()
	h := public.PublicHandler()
	req := httptest.NewRequest(http.MethodGet,
		"/v2/receipts?stream="+stream+"&merchant="+event.Merchant+"&order="+event.Order, nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	var body struct{ Receipts []map[string]any }
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != 200 ||
		len(body.Receipts) != 1 || body.Receipts[0]["amount"] != "30" {
		t.Fatalf("public projection: %d, %s, %v", response.Code, response.Body.String(), err)
	}
	for _, hidden := range []string{"payer", "recipient", "signerNonce", "quoteDigest", "payerGeneration", "paymentMode"} {
		if _, present := body.Receipts[0][hidden]; present {
			t.Fatalf("public projection leaked %s", hidden)
		}
	}
	if _, err := public.db.Exec(ctx, "SELECT payer_ref FROM chainview.settlement_receipt"); err == nil {
		t.Fatal("public database role can read restricted receipt")
	}

	reconciler, restrictedURL := fixtureLogin(t, database, owner, "nu54_p07_reconcile")
	restricted, err := pgx.Connect(ctx, restrictedURL)
	if err != nil {
		t.Fatal(err)
	}
	defer restricted.Close(ctx)
	var matched int
	if err := restricted.QueryRow(ctx, "SELECT count(*) FROM chainview.reconciliation_receipt($1,$2,$3)",
		stream, event.Merchant, event.Order).Scan(&matched); err != nil || matched != 0 {
		t.Fatalf("ungranted merchant scope: %d %v", matched, err)
	}
	if _, err := owner.Exec(ctx, "INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '1 minute')",
		reconciler, stream, event.Merchant); err != nil {
		t.Fatal(err)
	}
	var payer, quoteDigest string
	if err := restricted.QueryRow(ctx,
		"SELECT payer_ref,quote_digest FROM chainview.reconciliation_receipt($1,$2,$3)", stream, event.Merchant, event.Order).
		Scan(&payer, &quoteDigest); err != nil || payer != event.Payer || quoteDigest != event.QuoteDigest {
		t.Fatalf("restricted granted projection: %s %s %v", payer, quoteDigest, err)
	}
	if _, err := restricted.Exec(ctx, "SELECT * FROM chainview.settlement_receipt"); err == nil {
		t.Fatal("restricted account can bypass scoped projection")
	}
	if _, err := owner.Exec(ctx, "UPDATE chainview.receipt_scope SET valid_until=now() WHERE login_role=$1", reconciler); err != nil {
		t.Fatal(err)
	}
	if err := restricted.QueryRow(ctx, "SELECT count(*) FROM chainview.reconciliation_receipt($1,$2,$3)",
		stream, event.Merchant, event.Order).Scan(&matched); err != nil || matched != 0 {
		t.Fatalf("expired merchant scope: %d %v", matched, err)
	}
}
