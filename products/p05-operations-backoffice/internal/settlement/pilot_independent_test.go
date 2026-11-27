package settlement

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	pilotIndependentCustomers  = 100
	pilotReservationP95LimitMS = 1000
	pilotReservedStockLeft     = 5
	pilotOptionSKU             = "batch-option"
)

func independentCartItems(sameSKU bool) []commerce.Item {
	if sameSKU {
		return []commerce.Item{{SKU: pilotOptionSKU, Option: "a", Qty: 1}, {SKU: pilotOptionSKU, Option: "b", Qty: 1}}
	}
	return []commerce.Item{{SKU: "batch-a", Option: "plain", Qty: 1}, {SKU: "batch-b", Option: "plain", Qty: 1}}
}

type pilotIdentity struct {
	actor  string
	board  fixtureSigner
	cert   tls.Certificate
	client *http.Client
}

type pilotQuote struct {
	SaleRef      string                 `json:"saleRef"`
	BranchRef    string                 `json:"branchRef"`
	OrderKey     string                 `json:"orderKey"`
	Signed       string                 `json:"signedQuote"`
	Digest       string                 `json:"quoteDigest"`
	Expires      int64                  `json:"expiresAt"`
	Presentation *commerce.Presentation `json:"presentation,omitempty"`
}

type pilotReply struct {
	service.Reply
	Invoices    []commerce.Invoice
	Order       *commerce.Order
	Claim       *commerce.Claim
	Quote       *pilotQuote
	Comparison  *commerce.Comparison
	Comparisons []commerce.Comparison
}

func pilotCall(ctx context.Context, client *http.Client, url string, command service.Request, extras map[string]any) (pilotReply, int, error) {
	var reply pilotReply
	args := map[string]any{"command": command}
	for k, v := range extras {
		args[k] = v
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return reply, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v2/commerce", bytes.NewReader(payload))
	if err != nil {
		return reply, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return reply, 0, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32768))
	if res.StatusCode != 200 {
		_ = json.Unmarshal(raw, &reply)
	}
	if err != nil || res.StatusCode != 200 {
		return reply, res.StatusCode, fmt.Errorf("%s HTTP %d: %s", command.Operation, res.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return reply, res.StatusCode, err
	}
	if reply.Status != "ok" || reply.RequestID != command.RequestID {
		return reply, res.StatusCode, fmt.Errorf("unbound %s response", command.Operation)
	}
	return reply, res.StatusCode, nil
}

func newPilotCommand(t *testing.T, db *pgxpool.Pool, branch, object, op string, body map[string]string, write bool) service.Request {
	cmd := service.Request{Version: 2, Contract: "C3", Operation: op, RequestID: fixtureUUID(t, db), Domain: branch, Object: object,
		ExpiresAt: time.Now().Add(45 * time.Second).Unix(), Body: body}
	if write {
		cmd.IdempotencyKey = fixtureUUID(t, db)
	}
	return cmd
}

func pilotReserveCart(t *testing.T, db *pgxpool.Pool, branch, asset string, lines []commerce.Item) (service.Request, commerce.Cart) {
	origin := fixtureUUID(t, db)
	cart := commerce.Cart{Origin: "menu", OriginRef: origin, MenuVersion: 1, Items: lines}
	raw, err := canonicalPilotCart(cart)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	cmd := newPilotCommand(t, db, branch, origin, "order.create", map[string]string{
		"asset": asset, "quantity": strconv.Itoa(len(lines)), "intentDigest": rawHex(digest[:])}, true)
	return cmd, cart
}

// issuePilotOrder reaches the same order/claim/quote authority as the real client.

func canonicalPilotCart(cart commerce.Cart) ([]byte, error) {
	// The service canonicalizes by SKU and option independently of wire order.
	cart.Items = slices.Clone(cart.Items)
	slices.SortFunc(cart.Items, func(a, b commerce.Item) int {
		if bySKU := strings.Compare(a.SKU, b.SKU); bySKU != 0 {
			return bySKU
		}
		return strings.Compare(a.Option, b.Option)
	})
	return json.Marshal(cart)
}

func issuePilotOrder(ctx context.Context, url string, owner pilotIdentity, base seededOrder, sale string,
	claim, view, issue service.Request, asset string, amount string) (seededOrder, error) {
	out := base
	out.Actor, out.Sale, out.QuoteAttempts = owner.actor, sale, 0
	reply, _, err := pilotCall(ctx, owner.client, url, claim, nil)
	if err != nil || reply.Claim == nil {
		return out, fmt.Errorf("authorize checkout: %v", err)
	}
	ref := reply.Claim.Digest
	reply, _, err = pilotCall(ctx, owner.client, url, view, nil)
	if err != nil || reply.Order == nil || reply.Order.Asset != asset || reply.Order.Amount != amount {
		return out, fmt.Errorf("private order total: %v", err)
	}
	issue.Body = map[string]string{"claimDigest": ref, "orderRevision": strconv.FormatInt(reply.Order.Revision, 10)}
	// The first three attempts form one bounded request window. If admission
	// is still busy, reopen the private order before another user retry window.
	// The quote request retains its original body, expected revision and key.
	windows := 4
	for window := range windows {
		limit, cancel := context.WithTimeout(ctx, 8*time.Second)
		for range 3 {
			out.QuoteAttempts++
			reply, _, err = pilotCall(limit, owner.client, url, issue, nil)
			if err == nil || (reply.Error != service.Busy && reply.Error != service.Unknown && !errors.Is(err, context.DeadlineExceeded)) || limit.Err() != nil {
				break
			}
		}
		cancel()
		if err == nil || (reply.Error != service.Busy && reply.Error != service.Unknown && !errors.Is(err, context.DeadlineExceeded)) || window == windows-1 {
			break
		}
		resumed, _, resumeErr := pilotCall(ctx, owner.client, url, view, nil)
		if resumeErr != nil || resumed.Order == nil || resumed.Order.ID != sale || resumed.Order.Amount != amount {
			return out, fmt.Errorf("safe quote admission retry lost its private sale: %v", resumeErr)
		}
		if resumed.Order.State == "preview" {
			// The original response may have been lost after reserving its
			// five-second durable lease. Let that fence expire before resuming.
			select {
			case <-time.After(5200 * time.Millisecond):
			case <-ctx.Done():
				return out, ctx.Err()
			}
		}
	}
	if err != nil || reply.Quote == nil {
		return out, fmt.Errorf("genuine quote issue: %v", err)
	}
	q := reply.Quote
	if q.SaleRef != sale || q.BranchRef != base.Branch || q.Presentation == nil ||
		q.Presentation.Customer != owner.actor || q.Presentation.Claim != ref || len(q.Presentation.Signature) != 128 {
		return out, fmt.Errorf("issued quote does not bind private presentation")
	}
	signed, err := hex.DecodeString(q.Signed)
	if err != nil {
		return out, err
	}
	digest, err := hex.DecodeString(q.Digest)
	if err != nil {
		return out, err
	}
	out.Quote, err = unpackQuote(signed, digest, fixtureSigner(101).Address().Bytes(), base.Target)
	if err != nil || out.Quote.Args.Payer != owner.board.Address() || out.Quote.Args.Amount.String() != amount ||
		rawHex(out.Quote.Args.OrderId[:]) != q.OrderKey {
		return out, fmt.Errorf("issued quote identity differs: %v", err)
	}
	if !validPilotPresentation(q, out.Quote.Args.PayerGeneration) {
		return out, fmt.Errorf("issued presentation failed independent signature verification")
	}
	out.Signed, out.Presentation = signed, q.Presentation
	out.Order = commerce.QuoteOrder{Payer: owner.board.Address(), Amount: amount, OrderID: out.Quote.Args.OrderId,
		Issued: time.Unix(int64(out.Quote.Args.IssuedAt), 0), Expires: time.Unix(q.Expires, 0)}
	claimBytes, err := hex.DecodeString(ref)
	if err != nil {
		return out, err
	}
	copy(out.Order.Claim[:], claimBytes)
	return out, nil
}

func independentReservations(t *testing.T, ctx context.Context, db *pgxpool.Pool, url string, base seededOrder,
	owners []pilotIdentity, sameSKU bool) ([]string, []orderTiming) {
	t.Helper()
	if len(owners) != pilotIndependentCustomers {
		t.Fatalf("expected %d independent customers, got %d", pilotIndependentCustomers, len(owners))
	}
	var pending sync.WaitGroup
	start := make(chan struct{})
	ids := make([]string, len(owners))
	timings := make([]orderTiming, len(owners))
	errs := make([]error, len(owners))
	for i, owner := range owners {
		lines := independentCartItems(sameSKU)
		if i%2 == 1 {
			lines[0], lines[1] = lines[1], lines[0]
		}
		cmd, cart := pilotReserveCart(t, db, base.Branch, lower(base.Target.Asset), lines)
		pending.Go(func() {
			<-start
			began := time.Now()
			reply, _, err := pilotCall(ctx, owner.client, url, cmd, map[string]any{"cart": cart})
			ids[i], errs[i] = reply.ResultRef, err
			timings[i] = orderTiming{Request: cmd.RequestID, Sale: reply.ResultRef, Code: reply.Status, MS: float64(time.Since(began).Microseconds()) / 1000}
		})
	}
	close(start)
	pending.Wait()
	seen := map[string]bool{}
	for i, err := range errs {
		if err != nil || !financeRef.MatchString(ids[i]) || seen[ids[i]] {
			t.Fatalf("customer %d did not create independent sale: %v", i, err)
		}
		seen[ids[i]] = true
	}
	return ids, timings
}

func pilotDeadlocks(t *testing.T, ctx context.Context, db *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := db.QueryRow(ctx, "SELECT deadlocks FROM pg_stat_database WHERE datname=current_database()").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func seedIndependentInventory(t *testing.T, db *pgxpool.Pool, order seededOrder, sameSKU bool) {
	t.Helper()
	if sameSKU {
		fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,$2,$3)",
			order.Branch, pilotOptionSKU, 2*pilotIndependentCustomers+pilotReservedStockLeft)
	}
	for _, line := range independentCartItems(sameSKU) {
		fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,$2,$3,$2,$4,5)",
			order.Branch, line.SKU, line.Option, lower(order.Target.Asset))
		if sameSKU {
			fixtureExec(t, db, "INSERT INTO ledger.catalog_option_stock VALUES($1,$2,$3,$4)",
				order.Branch, line.SKU, line.Option, pilotIndependentCustomers+pilotReservedStockLeft)
		} else {
			fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,$2,$3)",
				order.Branch, line.SKU, pilotIndependentCustomers+pilotReservedStockLeft)
		}
	}
}

func saveIndependentReservations(t *testing.T, ctx context.Context, db *pgxpool.Pool, base seededOrder, out string, timings []orderTiming, before int, sameSKU bool) {
	t.Helper()
	if len(timings) != pilotIndependentCustomers {
		t.Fatalf("expected %d independent sales, got %d", pilotIndependentCustomers, len(timings))
	}
	if deadlocks := pilotDeadlocks(t, ctx, db) - before; deadlocks != 0 {
		t.Fatalf("reservation PostgreSQL deadlock delta=%d", deadlocks)
	}
	report := map[string]any{"recordedAt": time.Now().UTC(), "requests": timings,
		"independentCustomers": len(timings), "postgreSQLDeadlockDelta": 0}
	var a, b int
	if sameSKU {
		var stock, heldOptions, heldSales int
		err := db.QueryRow(ctx, `
			SELECT s.available,max(o.available) FILTER(WHERE o.option_key='a'),max(o.available) FILTER(WHERE o.option_key='b')
			FROM ledger.stock s JOIN ledger.catalog_option_stock o USING(branch_id,sku)
			WHERE s.branch_id=$1 AND s.sku=$2 GROUP BY s.available`, base.Branch, pilotOptionSKU).Scan(&stock, &a, &b)
		if err != nil {
			t.Fatal(err)
		}
		err = db.QueryRow(ctx, `
			SELECT count(*),count(DISTINCT sale_id) FROM ledger.catalog_option_hold WHERE branch_id=$1 AND sku=$2`,
			base.Branch, pilotOptionSKU).Scan(&heldOptions, &heldSales)
		if err != nil || stock != pilotReservedStockLeft || heldOptions != 2*pilotIndependentCustomers || heldSales != pilotIndependentCustomers {
			t.Fatalf("reservation SKU stock=%d held options=%d sales=%d: %v", stock, heldOptions, heldSales, err)
		}
		report["cartMode"], report["retainedSKUStock"] = "same-sku-two-options", stock
		report["retainedOptionStockA"], report["retainedOptionStockB"] = a, b
		report["heldOptions"], report["heldSales"] = heldOptions, heldSales
	} else {
		err := db.QueryRow(ctx, `
			SELECT max(available) FILTER(WHERE sku='batch-a'),max(available) FILTER(WHERE sku='batch-b')
			FROM ledger.stock WHERE branch_id=$1`, base.Branch).Scan(&a, &b)
		if err != nil {
			t.Fatal(err)
		}
		report["cartMode"], report["retainedStockA"], report["retainedStockB"] = "two-skus", a, b
	}
	if a != pilotReservedStockLeft || b != pilotReservedStockLeft {
		t.Fatalf("reservation retained stock=%d/%d", a, b)
	}
	latencies := make([]float64, len(timings))
	for i, timing := range timings {
		if timing.MS < 0 || math.IsNaN(timing.MS) || math.IsInf(timing.MS, 0) {
			t.Fatalf("invalid reservation latency for %s: %v", timing.Request, timing.MS)
		}
		latencies[i] = timing.MS
	}
	slices.Sort(latencies)
	p95 := latencies[(95*len(latencies)+99)/100-1]
	report["p95MS"], report["p95LimitMS"], report["p95Passed"] = p95, pilotReservationP95LimitMS, p95 < pilotReservationP95LimitMS
	saveBulkReport(t, out, "independent-reservations.json", report)
	checkIndependentReservationLatency(t, p95)
}

func checkIndependentReservationLatency(t *testing.T, p95 float64) {
	t.Helper()
	if p95 >= pilotReservationP95LimitMS {
		t.Errorf("reservation p95=%.2f ms must be below %d ms", p95, pilotReservationP95LimitMS)
	}
}

func TestIndependentReservationPerformanceGate(t *testing.T) {
	const latencyEnv = "NU54_PILOT_GATE_CHILD_LATENCY"
	const continued = "continued past the performance gate"
	if value := os.Getenv(latencyEnv); value != "" {
		p95, err := strconv.ParseFloat(value, 64)
		if err != nil {
			t.Fatal(err)
		}
		checkIndependentReservationLatency(t, p95)
		fmt.Println(continued)
		return
	}
	for _, scenario := range []struct {
		p95  string
		fail bool
	}{{"999.99", false}, {"1000", true}, {"1000.01", true}} {
		t.Run(scenario.p95, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=^TestIndependentReservationPerformanceGate$", "-test.v")
			child.Env = append(os.Environ(), latencyEnv+"="+scenario.p95)
			output, err := child.CombinedOutput()
			var failure *exec.ExitError
			if (scenario.fail && (!errors.As(err, &failure) || failure.ExitCode() != 1)) || (!scenario.fail && err != nil) {
				t.Fatalf("unexpected child exit %v: %s", err, output)
			}
			if !bytes.Contains(output, []byte(continued)) {
				t.Fatalf("payment and receipt phases were blocked: %s", output)
			}
			const latencyFailure = "must be below 1000 ms"
			if bytes.Contains(output, []byte(latencyFailure)) != scenario.fail {
				t.Fatalf("wrong performance result: %s", output)
			}
		})
	}
}

func TestIndependentPilotCartModes(t *testing.T) {
	for _, sameSKU := range []bool{false, true} {
		items := independentCartItems(sameSKU)
		if len(items) != 2 || (items[0].SKU == items[1].SKU) != sameSKU ||
			(items[0].Option != items[1].Option) != sameSKU || items[0].Qty != 1 || items[1].Qty != 1 {
			t.Fatalf("unexpected pilot cart for sameSKU=%v: %+v", sameSKU, items)
		}
		wanted := commerce.Cart{Origin: "menu", MenuVersion: 1, Items: items}
		raw, err := json.Marshal(wanted)
		if err != nil {
			t.Fatal(err)
		}
		reversed := wanted
		reversed.Items = []commerce.Item{items[1], items[0]}
		got, err := canonicalPilotCart(reversed)
		if err != nil || !bytes.Equal(raw, got) || reversed.Items[0] != items[1] {
			t.Fatalf("wire order changed or digest input is noncanonical: %s %v", got, err)
		}
	}
}

func genuineBulkQuotes(t *testing.T, ctx context.Context, db *pgxpool.Pool, url string, base seededOrder, ids []string, owners []pilotIdentity, directory string) []seededOrder {
	t.Helper()
	out := make([]seededOrder, len(ids))
	errors := make([]error, len(ids))
	timings := make([]float64, len(ids))
	start := make(chan struct{})
	var group sync.WaitGroup
	for i, owner := range owners {
		id := ids[i]
		claim := newPilotCommand(t, db, base.Branch, id, "checkout.authorize", map[string]string{"customerRef": owner.actor}, true)
		view := newPilotCommand(t, db, base.Branch, id, "order.status", map[string]string{}, false)
		issue := newPilotCommand(t, db, base.Branch, id, "quote.issue", nil, true)
		group.Go(func() {
			<-start
			began := time.Now()
			out[i], errors[i] = issuePilotOrder(ctx, url, owner, base, id, claim, view, issue, lower(base.Target.Asset), "10")
			timings[i] = float64(time.Since(began).Microseconds()) / 1000
		})
	}
	close(start)
	group.Wait()
	issued, failed := 0, 0
	for _, err := range errors {
		if err == nil {
			issued++
		} else {
			failed++
		}
	}
	if failed != 0 {
		var drafts, finished int
		_ = db.QueryRow(ctx, "SELECT count(*),count(signed_digest) FROM ledger.quote_work WHERE sale_id::text=ANY($1)", ids).Scan(&drafts, &finished)
		saveBulkReport(t, directory, "independent-issuance-incomplete.json", map[string]any{"drafts": drafts, "persisted": finished, "clientReceived": issued})
		for i, err := range errors {
			if err != nil {
				t.Fatalf("customer %d could not obtain normal issue (%d drafts, %d committed, %d read): %v", i, drafts, finished, issued, err)
			}
		}
	}
	slices.Sort(timings)
	resumed, attempts := 0, 0
	for _, item := range out {
		attempts += item.QuoteAttempts
		if item.QuoteAttempts > 1 {
			resumed++
		}
	}
	saveBulkReport(t, directory, "independent-issuance.json", map[string]any{"allConcurrent": len(ids), "claimStatusQuoteP95MS": timings[94], "maxMS": timings[len(timings)-1], "wallClockSamplesMS": timings, "sameKeyRetries": attempts - len(ids), "customersWithRetry": resumed, "firstPassCustomers": len(ids) - resumed})
	return out
}

func pilotPrivateReceipts(t *testing.T, ctx context.Context, db *pgxpool.Pool, url, branch string, ids []string, owners []pilotIdentity, baselineDeadlocks int, out string) {
	t.Helper()
	seen := map[string]bool{}
	for i, customer := range owners {
		request := newPilotCommand(t, db, branch, ids[i], "order.status", map[string]string{}, false)
		reply, _, err := pilotCall(ctx, customer.client, url, request, nil)
		if err != nil || reply.Order == nil || reply.Order.State != "paid" || reply.Order.PickupNo == nil ||
			reply.Order.ReceiptRef == nil || seen[*reply.Order.ReceiptRef] {
			t.Fatalf("owner receipt refused or duplicated: %v", err)
		}
		seen[*reply.Order.ReceiptRef] = true
		if _, status, _ := pilotCall(ctx, owners[(i+1)%len(owners)].client, url, request, nil); status != 404 {
			t.Fatalf("distinct customer receipt returned HTTP %d", status)
		}
	}
	var ownersInLedger, payers int
	err := db.QueryRow(ctx, `SELECT count(DISTINCT q.actor_id),count(DISTINCT p.payer)
		FROM ledger.payment p JOIN ledger.quote_work q ON q.sale_id=p.sale_id JOIN ledger.order_checkout c ON c.sale_id=p.sale_id
		WHERE p.sale_id::text=ANY($1) AND q.actor_id=c.creator`, ids).Scan(&ownersInLedger, &payers)
	delta := pilotDeadlocks(t, ctx, db) - baselineDeadlocks
	if err != nil || ownersInLedger != len(ids) || payers != len(ids) || delta != 0 {
		t.Fatalf("owner/payer/deadlocks mismatch: %d/%d/%d %v", ownersInLedger, payers, delta, err)
	}
	saveBulkReport(t, out, "independent-private-receipts.json", map[string]any{"ownReceipts": len(seen), "crossOwnerDenied": len(ids),
		"independentLedgerCustomers": ownersInLedger, "distinctOnchainPayers": payers, "fullFlowPostgreSQLDeadlockDelta": delta})
}

func startPilotIssuer(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn string, order, second seededOrder,
	terminal string, certs []tls.Certificate, ca []byte, native *tls.Certificate) (string, []pilotIdentity) {
	t.Helper()
	serverFixture := httptest.NewTLSServer(http.NewServeMux())
	serverFixture.Close()
	server := serverFixture.TLS.Certificates[0]
	dir := t.TempDir()
	write := func(tag string, data []byte, mode os.FileMode) string {
		t.Helper()
		file := filepath.Join(dir, tag)
		if err := os.WriteFile(file, data, mode); err != nil {
			t.Fatal(err)
		}
		return file
	}
	config := func(tag string, v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return write(tag, raw, 0o600)
	}
	pemValue := func(kind string, data []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: data}) }
	serverKey, err := x509.MarshalPKCS8PrivateKey(server.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	_, runtime := testLogin(t, db, dsn, "nu54_p05_commerce")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	var owners []pilotIdentity
	grants := map[string]service.Principal{}
	root := x509.NewCertPool()
	leaf, err := x509.ParseCertificate(server.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	root.AddCert(leaf)
	posSource := fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.pos_source VALUES($1,$2,$3,now()+interval '1 hour')", order.Branch, posSource, terminal)
	for i, actor := range []string{order.Actor, second.Actor, terminal, posSource} {
		role := "customer"
		if i == 2 {
			role = "terminal"
		}
		cert := certs[[]int{0, 4, 5, 3}[i]]
		grant := service.Principal{Role: role, Tenant: order.Tenant, Subject: actor, ExpiresAt: time.Now().Add(time.Hour).Unix()}
		if i == 3 {
			grant.Role = "pos"
			grant.Grants = []service.Grant{{Domain: order.Branch, Object: order.Branch, Operations: []string{"POS/submit"}}}
		}
		grants[fingerprint(cert)] = grant
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: root, Certificates: []tls.Certificate{cert}}}
		t.Cleanup(transport.CloseIdleConnections)
		owners = append(owners, pilotIdentity{actor: actor, board: []fixtureSigner{102, 105, 0, 0}[i], cert: cert, client: &http.Client{Transport: transport, Timeout: 8 * time.Second}})
	}
	if lab := os.Getenv("NU54_POS_NATIVE_LAB"); lab != "" {
		if native == nil {
			t.Fatal("native POS lab requires a terminal certificate")
		}
		expires := time.Now().Add(10 * time.Minute).Unix()
		grants[fingerprint(*native)] = service.Principal{Role: "terminal", Tenant: order.Tenant, Subject: terminal, ExpiresAt: expires}
		profile := map[string]any{"endpoint": "https://" + addr + "/v2/commerce", "pin": fmt.Sprintf("%x", sha256.Sum256(leaf.RawSubjectPublicKeyInfo)),
			"ca": base64.StdEncoding.EncodeToString(server.Certificate[0]), "certificates": []string{base64.StdEncoding.EncodeToString(native.Certificate[0])},
			"scope": map[string]any{"tenantRef": order.Tenant, "branchRef": order.Branch, "subjectRef": terminal, "role": "terminal", "expiresAt": expires}}
		raw, err := json.Marshal(profile)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(lab, "terminal-profile.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cast, err := exec.LookPath(os.Getenv("P05_SETTLEMENT_TEST_SIGNER"))
	if err != nil {
		t.Fatal(err)
	}
	signer := fixtureSignerProgram(cast, 101)
	if lab := os.Getenv("NU54_POS_NATIVE_LAB"); lab != "" {
		hold := filepath.Join(lab, "hold-quote")
		// Only the test signer is delayed. The real opsd still prepares and
		// commits or rejects the normal signed quote under its existing lease.
		hook := fmt.Sprintf("if [ -f %q ]; then /bin/sleep 2.8; fi\n", hold)
		signer = bytes.Replace(signer, []byte("exec "), []byte(hook+"exec "), 1)
	}
	hash := sha256.Sum256(signer)
	command := exec.CommandContext(ctx, os.Getenv("P05_SETTLEMENT_TEST_OPSD"))
	command.Env = []string{"P05_HTTP_ADDR=" + addr, "TMPDIR=" + dir, "P05_COMMERCE_DATABASE_URL=" + runtime,
		"P05_COMMERCE_CA=" + write("client-ca", pemValue("CERTIFICATE", ca), 0o600),
		"P05_COMMERCE_CERT=" + write("server", pemValue("CERTIFICATE", server.Certificate[0]), 0o600),
		"P05_COMMERCE_KEY=" + write("server-key", pemValue("PRIVATE KEY", serverKey), 0o600),
		"P05_COMMERCE_GRANTS=" + config("grants", grants),
		"P05_COMMERCE_PROFILES=" + config("profile", map[string]any{order.Branch: map[string]any{
			"chainId": 8283, "contract": order.Target.Contract, "asset": order.Target.Asset, "beneficiary": fixtureSigner(104).Address(),
			"merchantId": strings.Repeat("00", 31) + "32", "keyVersion": 1, "beneficiaryVersion": 1, "signer": fixtureSigner(101).Address(),
		}}),
		"P05_QUOTE_SIGNER=" + write("issuer", signer, 0o700), "P05_QUOTE_SIGNER_SHA256=" + rawHex(hash[:]),
		"P05_QUOTE_COMMITMENT_KEY_FILE=" + write("commitment", []byte(fmt.Sprintf("%064x", 9)), 0o600),
		"P05_QUOTE_STORAGE_KEY_FILE=" + write("storage", []byte(rawHex(order.CipherKey[:])), 0o600),
		"P05_PRESENTATION_KEY_FILE=" + write("presentation", []byte(strings.Repeat("31", 32)), 0o600),
	}
	var log bytes.Buffer
	command.Stderr = &log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
		if t.Failed() {
			t.Log(log.String())
		}
	})
	url := "https://" + addr
	waitUntil(t, func() bool {
		r, err := owners[0].client.Get(url + "/healthz")
		if err != nil {
			return false
		}
		r.Body.Close()
		return r.StatusCode == 200
	})
	return url, owners
}

func reserveTimedPilot(t *testing.T, ctx context.Context, db *pgxpool.Pool, url string, base seededOrder,
	owner pilotIdentity, sku string, posSource *pilotIdentity) seededOrder {
	t.Helper()
	origin := fixtureUUID(t, db)
	if posSource != nil {
		amount := pilotUnits(10)
		ref := "S-0142-" + origin[:8]
		const lookupPath = "/v2/commerce"
		body := map[string]any{"operation": "submit", "branch": base.Branch, "reference": ref, "chainId": 8283, "asset": lower(base.Target.Asset), "amount": amount}
		do := func(identity pilotIdentity, input map[string]any, want int) []byte {
			t.Helper()
			return postPOSFixture(t, ctx, identity.client, url+lookupPath, input, want)
		}
		bad := map[string]any{"operation": "submit", "branch": base.Branch, "reference": ref, "chainId": 8283, "asset": lower(common.HexToAddress("0xbad")), "amount": amount}
		do(*posSource, bad, 400)
		do(owner, body, 404)
		bad["branch"] = fixtureUUID(t, db)
		do(*posSource, bad, 404)
		legacyCart := commerce.Cart{Origin: "pos", OriginRef: origin, MenuVersion: 1, Items: []commerce.Item{{SKU: sku, Option: "plain", Qty: 1}}}
		if _, status, _ := pilotCall(ctx, owner.client, url, newPilotCommand(t, db, base.Branch, origin, "order.create", map[string]string{
			"asset": lower(base.Target.Asset), "quantity": "1", "intentDigest": strings.Repeat("f", 64),
		}, true), map[string]any{"cart": legacyCart}); status != 404 {
			t.Fatalf("legacy POS cart fabricated an order: HTTP %d", status)
		}
		var accepted struct {
			Handoff string `json:"handoff"`
		}
		if err := json.Unmarshal(do(*posSource, body, 200), &accepted); err != nil || !financeRef.MatchString(accepted.Handoff) {
			t.Fatalf("bad POS handoff: %v", err)
		}
		var dup struct {
			Handoff string `json:"handoff"`
		}
		if err := json.Unmarshal(do(*posSource, body, 200), &dup); err != nil || dup != accepted {
			t.Fatalf("POS duplicate changed handoff: %v", err)
		}
		body["amount"] = "9999999"
		do(*posSource, body, 409)
		var view struct {
			Order commerce.Order `json:"order"`
		}
		lookup := map[string]any{"operation": "lookup", "branch": base.Branch, "handoff": accepted.Handoff}
		if err := json.Unmarshal(do(owner, lookup, 200), &view); err != nil || view.Order.Amount != amount || !financeRef.MatchString(view.Order.ID) {
			t.Fatalf("bad original POS order: %+v %v", view, err)
		}
		base.Sale = view.Order.ID
		return base
	}
	cart := commerce.Cart{Origin: "menu", OriginRef: origin, MenuVersion: 1, Items: []commerce.Item{{SKU: sku, Option: "plain", Qty: 1}}}
	raw, err := json.Marshal(cart)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	cmd := newPilotCommand(t, db, base.Branch, origin, "order.create", map[string]string{
		"asset": lower(base.Target.Asset), "quantity": "1", "intentDigest": rawHex(digest[:])}, true)
	reply, _, err := pilotCall(ctx, owner.client, url, cmd, map[string]any{"cart": cart})
	if err != nil || !financeRef.MatchString(reply.ResultRef) {
		t.Fatalf("timed pilot reserve: %v", err)
	}
	base.Sale = reply.ResultRef
	return base
}

func postPOSFixture(t *testing.T, ctx context.Context, client *http.Client, url string, payload map[string]any, want int) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"pos": payload})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != want {
		t.Fatalf("POS %s: %d %s %v", payload["operation"], response.StatusCode, raw, err)
	}
	return raw
}

func issueTimedPilot(t *testing.T, ctx context.Context, db *pgxpool.Pool, url string, base seededOrder,
	owner, checkoutActor pilotIdentity, amount string, contender ...pilotIdentity) seededOrder {
	t.Helper()
	id := base.Sale
	if checkoutActor.actor != owner.actor {
		customers := append([]pilotIdentity{owner}, contender...)
		candidates := make([]*commerce.Comparison, 0, len(customers))
		for _, customer := range customers {
			request := newPilotCommand(t, db, base.Branch, id, "checkout.request", map[string]string{}, true)
			reply, _, err := pilotCall(ctx, customer.client, url, request, nil)
			if err != nil || reply.Comparison == nil {
				t.Fatalf("POS comparison request: %v", err)
			}
			candidates = append(candidates, reply.Comparison)
		}
		pending := newPilotCommand(t, db, base.Branch, id, "checkout.pending", map[string]string{}, false)
		reply, _, err := pilotCall(ctx, checkoutActor.client, url, pending, nil)
		if err != nil || len(reply.Comparisons) != len(candidates) {
			t.Fatalf("POS comparisons %v %v", reply.Comparisons, err)
		}
		choose := newPilotCommand(t, db, base.Branch, id, "checkout.confirm", map[string]string{"customerRef": owner.actor, "code": candidates[0].Code}, true)
		first, _, err := pilotCall(ctx, checkoutActor.client, url, choose, nil)
		if err != nil || first.Claim == nil {
			t.Fatalf("select matched payer: %v", err)
		}
		// Re-use the exact command key after seeking a response on a fresh connection.
		checkoutActor.client.CloseIdleConnections()
		resumed, _, err := pilotCall(ctx, checkoutActor.client, url, choose, nil)
		if err != nil || resumed.Claim == nil || resumed.Claim.Digest != first.Claim.Digest {
			t.Fatalf("POS payer selection retry changed binding: %v", err)
		}
		if len(contender) > 0 {
			other := newPilotCommand(t, db, base.Branch, id, "checkout.confirm", map[string]string{
				"customerRef": contender[0].actor, "code": candidates[1].Code}, true)
			if alternate, _, err := pilotCall(ctx, checkoutActor.client, url, other, nil); err == nil || alternate.Claim != nil {
				t.Fatalf("alternate POS payer admitted: %v", err)
			}
		}
	}
	op := "checkout.authorize"
	fields := map[string]string{"customerRef": owner.actor}
	write := true
	if checkoutActor.actor != owner.actor {
		op = "checkout.claim"
		fields = map[string]string{}
		write = false
	}
	claim := newPilotCommand(t, db, base.Branch, id, op, fields, write)
	view := newPilotCommand(t, db, base.Branch, id, "order.status", map[string]string{}, false)
	issue := newPilotCommand(t, db, base.Branch, id, "quote.issue", nil, true)
	r, err := issuePilotOrder(ctx, url, owner, base, id, claim, view, issue, lower(base.Target.Asset), amount)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func validPilotPresentation(q *pilotQuote, generation uint64) bool {
	p := q.Presentation
	data := []byte("NU54-private-presentation-v1\x00")
	for _, ref := range []string{q.BranchRef, q.SaleRef, p.Customer, p.Origin, p.Claim, q.OrderKey, q.Digest} {
		decoded, err := hex.DecodeString(strings.ReplaceAll(ref, "-", ""))
		if err != nil {
			return false
		}
		data = append(data, decoded...)
	}
	data = binary.BigEndian.AppendUint64(data, generation)
	data = binary.BigEndian.AppendUint64(data, uint64(q.Expires))
	sig, err := hex.DecodeString(p.Signature)
	if err != nil || !ed25519.Verify(pilotPresentationKey(), data, sig) {
		return false
	}
	if p.Invoice != nil {
		data = append(data, 1)
		data = append(data, p.Invoice.Cycle...)
		data = binary.BigEndian.AppendUint16(data, uint16(len(p.Invoice.TermsVersion)))
		data = append(data, p.Invoice.TermsVersion...)
		raw, err := hex.DecodeString(q.Signed)
		if err != nil {
			return false
		}
		wireHash := sha256.Sum256(raw)
		data = append(data, wireHash[:]...)
		sig, err := hex.DecodeString(p.Invoice.Signature)
		return err == nil && ed25519.Verify(pilotPresentationKey(), data, sig)
	}
	return true
}

func pilotPresentationKey() ed25519.PublicKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, 32)).Public().(ed25519.PublicKey)
}

func verifyPOSNativeLab(t *testing.T, ctx context.Context, db *pgxpool.Pool, lab, url string, order seededOrder, actors []pilotIdentity,
	settle func(seededOrder)) {
	t.Helper()
	const previewRef = "S-0142"
	const amount = "6500000"
	asset := lower(order.Target.Asset)
	payload := map[string]any{"operation": "submit", "branch": order.Branch, "reference": previewRef, "chainId": 8283, "asset": asset, "amount": amount}
	lookup := map[string]any{"operation": "lookup", "branch": order.Branch}
	var fresh struct{ Handoff string }
	if err := json.Unmarshal(postPOSFixture(t, ctx, actors[3].client, url+"/v2/commerce", payload, 200), &fresh); err != nil {
		t.Fatal(err)
	}
	var view struct{ Order commerce.Order }
	lookup["handoff"] = fresh.Handoff
	if err := json.Unmarshal(postPOSFixture(t, ctx, actors[2].client, url+"/v2/commerce", lookup, 200), &view); err != nil {
		t.Fatal(err)
	}
	if view.Order.State != "preview" || view.Order.Amount != amount {
		t.Fatal("expected new authenticated POS preview")
	}
	sale := view.Order.ID
	writeLab := func(name string, value map[string]string) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(lab, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	waitLab := func(name string) {
		t.Helper()
		deadline := time.NewTimer(2 * time.Minute)
		defer deadline.Stop()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-deadline.C:
				t.Fatalf("native POS harness did not finish %s", name)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-tick.C:
				if _, err := os.Stat(filepath.Join(lab, name)); err == nil {
					return
				}
			}
		}
	}
	writePairs := func(sale, file string) map[string]string {
		t.Helper()
		pairs := map[string]string{}
		for i, name := range []string{"chosen", "other"} {
			reply, _, err := pilotCall(ctx, actors[i].client, url, newPilotCommand(t, db, order.Branch, sale, "checkout.request", map[string]string{}, true), nil)
			if err != nil || reply.Comparison == nil {
				t.Fatalf("native POS candidate: %v", err)
			}
			pairs[name] = reply.Comparison.Code
		}
		writeLab(file, pairs)
		return pairs
	}
	nearby := func(sale string, candidate int, mode string) (pilotReply, int, error) {
		t.Helper()
		command := newPilotCommand(t, db, order.Branch, sale, "checkout.pos-nearby", map[string]string{
			"customerRef": actors[candidate].actor, "mode": mode,
		}, true)
		return pilotCall(ctx, actors[2].client, url, command, nil)
	}
	writeLab("orders.json", map[string]string{"handoff": fresh.Handoff})
	waitLab("preview-open")
	if err := os.WriteFile(filepath.Join(lab, "hold-quote"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	pairs := writePairs(sale, "pairs.json")

	if reply, _, err := nearby(sale, 0, "register"); err != nil || reply.Claim != nil ||
		reply.Comparison == nil || reply.Comparison.Code != pairs["chosen"] {
		t.Fatalf("registered terminal did not reuse the customer's QR candidate for BLE: %v", err)
	}

	waitLab("locking-seen")
	deadline := time.Now().Add(6 * time.Second)
	var selected, prepared, stored bool
	for time.Now().Before(deadline) {
		err := db.QueryRow(ctx, `SELECT EXISTS(SELECT FROM ledger.checkout_claim WHERE sale_id=$1),
			EXISTS(SELECT FROM ledger.quote_work WHERE sale_id=$1),
			EXISTS(SELECT FROM ledger.quote_work WHERE sale_id=$1 AND signed_digest IS NOT NULL)`, sale).
			Scan(&selected, &prepared, &stored)
		if err != nil {
			t.Fatal(err)
		}
		if stored {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !stored {
		t.Fatalf("Android did not commit its selected POS quote: selected=%t prepared=%t stored=%t", selected, prepared, stored)
	}
	waitLab("payer-bound")
	if err := os.Remove(filepath.Join(lab, "hold-quote")); err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		Order        commerce.Order
		SelectedCode string
	}
	if err := json.Unmarshal(postPOSFixture(t, ctx, actors[2].client, url+"/v2/commerce", lookup, 200), &persisted); err != nil ||
		persisted.Order.State != "issued" || persisted.Order.ID != sale || persisted.SelectedCode != pairs["chosen"] {
		t.Fatal("native terminal observed issuance before the original quote was committed")
	}

	if reply, _, err := nearby(sale, 0, "resume"); err != nil || reply.Claim == nil || reply.Comparison != nil {
		t.Fatalf("selected customer's nearby resume lost the original claim: %v", err)
	}
	if reply, status, err := nearby(sale, 1, "resume"); err == nil || status != 400 ||
		reply.Error != service.Forbidden || reply.Comparison != nil || reply.Claim != nil {
		t.Fatalf("losing nearby customer received another customer's scope: status %d", status)
	}

	claim := newPilotCommand(t, db, order.Branch, sale, "checkout.claim", map[string]string{}, false)
	refused, _, err := pilotCall(ctx, actors[1].client, url, claim, nil)
	if err != nil || refused.Claim != nil {
		t.Fatalf("losing native POS claimant should see an empty scoped claim: %v", err)
	}
	order.Sale = sale
	issued, err := issuePilotOrder(ctx, url, actors[0], order, sale, claim,
		newPilotCommand(t, db, order.Branch, sale, "order.status", map[string]string{}, false),
		newPilotCommand(t, db, order.Branch, sale, "quote.issue", nil, true), asset, amount)
	if err != nil {
		t.Fatal(err)
	}
	settle(issued)
	if err := json.Unmarshal(postPOSFixture(t, ctx, actors[2].client, url+"/v2/commerce", lookup, 200), &view); err != nil ||
		view.Order.ID != sale || view.Order.State != "paid" || view.Order.Amount != amount || view.Order.ReceiptRef == nil {
		t.Fatalf("paid POS lookup lost the original order: %v", err)
	}
	receipt := *view.Order.ReceiptRef
	var replay struct{ Handoff string }
	if err := json.Unmarshal(postPOSFixture(t, ctx, actors[3].client, url+"/v2/commerce", payload, 200), &replay); err != nil || replay != fresh {
		t.Fatalf("paid POS replay changed handoff: %v", err)
	}
	var counts [5]int
	err = db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ledger.pos_order WHERE branch_id=$1 AND external_ref=$2 AND sale_id=$3),
		(SELECT count(*) FROM ledger.checkout_claim WHERE sale_id=$3),
		(SELECT count(*) FROM ledger.quote_work WHERE sale_id=$3),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$3),
		(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$3 AND kind='paid')`,
		order.Branch, previewRef, sale).Scan(&counts[0], &counts[1], &counts[2], &counts[3], &counts[4])
	if err != nil || counts != [5]int{1, 1, 1, 1, 1} {
		t.Fatalf("native S-0142 expected one sale, claim, quote, payment and paid effect: %v %v", counts, err)
	}
	writeLab("receipt.json", map[string]string{"receipt": receipt})
	payload["reference"] = "S-0143"
	var competing struct{ Handoff string }
	if err := json.Unmarshal(postPOSFixture(t, ctx, actors[3].client, url+"/v2/commerce", payload, 200), &competing); err != nil {
		t.Fatal(err)
	}
	lookup["handoff"] = competing.Handoff
	var competingView struct{ Order commerce.Order }
	if err := json.Unmarshal(postPOSFixture(t, ctx, actors[2].client, url+"/v2/commerce", lookup, 200), &competingView); err != nil {
		t.Fatal(err)
	}
	writeLab("competing-order.json", map[string]string{"handoff": competing.Handoff})
	waitLab("competing-open")
	competingPairs := writePairs(competingView.Order.ID, "competing-pairs.json")
	waitLab("competing-visible")
	bind := newPilotCommand(t, db, order.Branch, competingView.Order.ID, "checkout.confirm", map[string]string{
		"customerRef": actors[1].actor, "code": competingPairs["other"],
	}, true)
	if reply, _, err := pilotCall(ctx, actors[2].client, url, bind, nil); err != nil || reply.Claim == nil {
		t.Fatalf("independent original was not bound: %v", err)
	}
	writeLab("external-bound.json", map[string]string{"state": "bound"})
	waitLab("competing-checked")
	var losing [4]int
	err = db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ledger.checkout_claim WHERE sale_id=$1 AND actor_id=$2),
		(SELECT count(*) FROM ledger.quote_work WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$1)`, competingView.Order.ID, actors[0].actor).
		Scan(&losing[0], &losing[1], &losing[2], &losing[3])
	if err != nil || losing != [4]int{} {
		t.Fatalf("losing Android selection produced a ledger effect: %v %v", losing, err)
	}

	waitLab("ui-completed")
	t.Log("native S-0142 selected payer, P01 approved 6.50 TestUSDC, settlement recorded five single effects, and restarted app reopened the same receipt")
}
