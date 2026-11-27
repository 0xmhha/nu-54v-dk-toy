package settlement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/jackc/pgx/v5/pgxpool"
)

type orderTiming struct {
	Request string  `json:"requestId"`
	Sale    string  `json:"saleId,omitempty"`
	Code    string  `json:"code"`
	MS      float64 `json:"ms"`
}

func measurePilotOrders(t *testing.T, ctx context.Context, db *pgxpool.Pool, client *http.Client, url string, order seededOrder, target Target, out string) []string {
	t.Helper()
	began := time.Now().UTC()
	branch := order.Branch
	asset := lower(target.Asset)
	type payload struct {
		Command service.Request `json:"command"`
		Cart    commerce.Cart   `json:"cart"`
	}
	newOrder := func() payload {
		origin := fixtureUUID(t, db)
		cart := commerce.Cart{Origin: "menu", OriginRef: origin, MenuVersion: 1,
			Items: []commerce.Item{{SKU: "sample", Option: "plain", Qty: 1}}}
		canonical, err := json.Marshal(cart)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(canonical)
		return payload{Command: service.Request{Version: 2, Contract: "C3", Operation: "order.create", Domain: branch,
			Object: origin, RequestID: fixtureUUID(t, db), ExpiresAt: time.Now().Add(35 * time.Second).Unix(),
			IdempotencyKey: fixtureUUID(t, db), Body: map[string]string{"asset": asset, "quantity": "1", "intentDigest": rawHex(digest[:])}}, Cart: cart}
	}
	post := func(input payload) (orderTiming, error) {
		timing := orderTiming{Request: input.Command.RequestID}
		body, err := json.Marshal(input)
		if err != nil {
			return timing, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v2/commerce", bytes.NewReader(body))
		if err != nil {
			return timing, err
		}
		req.Header.Set("Content-Type", "application/json")
		start := time.Now()
		response, err := client.Do(req)
		if err != nil {
			return timing, err
		}
		defer response.Body.Close()
		reply, err := service.DecodeReply(io.LimitReader(response.Body, 8192))
		timing.MS = float64(time.Since(start).Microseconds()) / 1000
		if err != nil || reply.RequestID != timing.Request {
			return timing, fmt.Errorf("unbound C3 response HTTP %d: %v", response.StatusCode, err)
		}
		timing.Code, timing.Sale = string(reply.Error), reply.ResultRef
		if response.StatusCode == http.StatusOK && reply.Status == "ok" && financeRef.MatchString(timing.Sale) {
			timing.Code = "ok"
			return timing, nil
		}
		if response.StatusCode == http.StatusServiceUnavailable && reply.Error == service.Busy {
			return timing, nil
		}
		return timing, fmt.Errorf("unexpected HTTP %d C3 status %s/%s", response.StatusCode, reply.Status, reply.Error)
	}
	requests := make([]payload, 100)
	for i := range requests {
		requests[i] = newOrder()
	}
	start := make(chan struct{})
	timings := make([]orderTiming, len(requests))
	failures := make([]error, len(requests))
	var group sync.WaitGroup
	for i := range requests {
		group.Go(func() {
			<-start
			timings[i], failures[i] = post(requests[i])
		})
	}
	close(start)
	group.Wait()
	distinct := map[string]bool{}
	latencies := make([]float64, len(timings))
	for i, timing := range timings {
		if failures[i] != nil || timing.Code != "ok" || distinct[timing.Sale] {
			t.Fatalf("concurrent order %d did not create one distinct sale: %s %v", i, timing.Code, failures[i])
		}
		distinct[timing.Sale] = true
		latencies[i] = timing.MS
	}
	slices.Sort(latencies)
	var stock int
	if err := db.QueryRow(ctx, "SELECT available FROM ledger.stock WHERE branch_id=$1 AND sku='sample'", branch).Scan(&stock); err != nil || stock != 5 {
		t.Fatalf("100 concurrent reservations yielded stock %d: %v", stock, err)
	}
	locked := newOrder()
	lock, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(context.Background())
	if _, err := lock.Exec(ctx, "SELECT available FROM ledger.stock WHERE branch_id=$1 AND sku='sample' FOR UPDATE", branch); err != nil {
		t.Fatal(err)
	}
	lockStart := time.Now()
	lockTimings := make([]orderTiming, 0, 3)
	for range 2 {
		timing, err := post(locked)
		if err != nil || timing.Code != string(service.Busy) || timing.MS < 1800 || timing.MS > 2800 {
			t.Fatalf("expected two-second lock wait: %+v %v", timing, err)
		}
		lockTimings = append(lockTimings, timing)
	}
	if err := lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	retry, err := post(locked)
	lockTimings = append(lockTimings, retry)
	elapsed := float64(time.Since(lockStart).Microseconds()) / 1000
	if err != nil || retry.Code != "ok" || elapsed >= 8000 {
		t.Fatalf("bounded lock recovery failed after %.2fms: %+v %v", elapsed, retry, err)
	}
	// This workload measures reservation only. Those rows have no payment or cooking authority.
	var effects int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM ledger.fulfillment_outbox e
		JOIN ledger.sale s ON s.id=e.sale_id JOIN ledger.order_line l ON l.sale_id=s.id
		WHERE s.branch_id=$1 AND l.sku='sample'`, branch).Scan(&effects); err != nil || effects != 0 {
		t.Fatalf("reservation unexpectedly created fulfillment: %d %v", effects, err)
	}
	report := struct {
		Mode        string        `json:"mode"`
		StartedAt   time.Time     `json:"startedAt"`
		FinishedAt  time.Time     `json:"finishedAt"`
		Requests    []orderTiming `json:"orderCreate"`
		P95MS       float64       `json:"p95MS"`
		LockRetries []orderTiming `json:"lockAttempts"`
		RecoveryMS  float64       `json:"lockRecoveryMS"`
	}{"local_opsd_mtls_postgres_reservation", began, time.Now().UTC(), timings, latencies[94], lockTimings, elapsed}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "reservation-load.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("measured 100 concurrent reservations, p95 %.2fms; lock retries and sale reference in %.2fms", report.P95MS, elapsed)
	if report.P95MS >= 1000 {
		t.Fatal("local order creation exceeded the pilot p95 threshold")
	}
	ids := make([]string, len(timings))
	for i, observed := range timings {
		ids[i] = observed.Sale
	}
	return ids

}

func writePilotTrace(t *testing.T, ctx context.Context, db *pgxpool.Pool, out string, orders [2]seededOrder,
	chain [3]*types.Receipt, refundID string, legacy map[string]any) {
	t.Helper()
	type trace struct {
		Sale       string `json:"saleId"`
		Order      string `json:"publicOrderKey"`
		Quote      string `json:"quoteDigest"`
		Approval   string `json:"approvalDigest"`
		BoardPath  string `json:"hostBoardPath"`
		Tx         string `json:"transactionHash"`
		Block      string `json:"inclusionBlockHash"`
		Evidence   string `json:"p07EvidenceId"`
		Journal    string `json:"journalId"`
		Units      string `json:"assetBaseUnits"`
		Pickup     int64  `json:"pickupNo"`
		Receipt    string `json:"receiptRef"`
		Consumer   string `json:"consumerEventId"`
		GasUnits   uint64 `json:"gasUsed"`
		NativeCost string `json:"gasCostNativeBaseUnits"`
	}
	traces := make([]trace, 2)
	for i, order := range orders {
		item := trace{Sale: order.Sale, Order: rawHex(order.Order.OrderID[:]), Quote: rawHex(order.Quote.Digest[:]),
			Approval: rawHex(order.Quote.ApprovalHash[:]), BoardPath: [2]string{"management", "payment"}[i]}
		if err := db.QueryRow(ctx, `SELECT p.tx_hash,p.inclusion_hash,p.evidence_id::text,j.id::text,p.principal::text,
			f.number,f.receipt_ref::text,c.event_id::text FROM ledger.payment p JOIN ledger.journal j
			ON j.sale_id=p.sale_id AND j.refund_id IS NULL JOIN ledger.pickup f ON f.sale_id=p.sale_id
			JOIN ledger.fulfillment_outbox e ON e.sale_id=p.sale_id AND e.kind='paid'
			JOIN ledger.consumer_receipt c ON c.event_id=e.event_id JOIN ledger.fulfillment_inbox i ON i.event_id=c.event_id
			WHERE p.sale_id=$1`, order.Sale).Scan(&item.Tx, &item.Block, &item.Evidence, &item.Journal,
			&item.Units, &item.Pickup, &item.Receipt, &item.Consumer); err != nil {
			t.Fatal(err)
		}
		if item.Tx != chain[i].TxHash.Hex() || item.Block != chain[i].BlockHash.Hex() || item.Units != order.Order.Amount {
			t.Fatal("trace ledger facts do not match actual transaction")
		}
		item.GasUnits = chain[i].GasUsed
		item.NativeCost = new(big.Int).Mul(chain[i].EffectiveGasPrice, new(big.Int).SetUint64(item.GasUnits)).String()
		traces[i] = item
	}
	var refunded, refundTx, refundEvidence, refundJournal string
	if err := db.QueryRow(ctx, `SELECT r.amount::text,c.tx_hash,c.evidence_id::text,j.id::text
		FROM ledger.refund r JOIN ledger.refund_confirmation c ON c.refund_id=r.id
		JOIN ledger.journal j ON j.refund_id=r.id WHERE r.id=$1`, refundID).
		Scan(&refunded, &refundTx, &refundEvidence, &refundJournal); err != nil {
		t.Fatal(err)
	}
	if refundTx != chain[2].TxHash.Hex() || refunded != pilotUnits(3) {
		t.Fatal("refund trace disagrees with actual chain")
	}
	report := map[string]any{
		"recordedAt": time.Now().UTC(), "mode": "synthetic_p01_host_core_loopback_mtls_anvil_8283",
		"testChainId": 8283, "orders": traces, "legacyPendingEscrow": legacy, "beneficiaryCreditBaseUnits": pilotUnits(30),
		"refundEscrowCreditBaseUnits": pilotUnits(3), "remainingRefundReserveBaseUnits": pilotUnits(27), "netBaseUnits": pilotUnits(27), "testUSDCDecimals": 6,
		"sameOrderQRAuthenticatedMTLSApprovalRequests": 10, "sameOrderNearbyWireApprovalRequests": 10, "bothReceiptsPrivate": true, "refund": map[string]any{"refundId": refundID, "transactionHash": refundTx,
			"p07EvidenceId": refundEvidence, "journalId": refundJournal, "gasUsed": chain[2].GasUsed,
			"gasCostNativeBaseUnits": new(big.Int).Mul(chain[2].EffectiveGasPrice, new(big.Int).SetUint64(chain[2].GasUsed)).String()},
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "settlement-trace.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

type pilotPublicSlot struct {
	Started        int64    `json:"started"`
	ForwardedAt    int64    `json:"forwardedAt"`
	Ended          int64    `json:"ended"`
	PublicSale     string   `json:"publicSale"`
	PrivateSale    string   `json:"privateSale"`
	Routes         []string `json:"routes"`
	MenuActivities []int64  `json:"menuActivities"`
}

func pilotHoldDuration(t *testing.T) time.Duration {
	t.Helper()
	setting := os.Getenv("P05_PILOT_PUBLIC_HOLD_SECONDS")
	if setting == "" {
		return 0
	}
	seconds, err := strconv.Atoi(setting)
	if err != nil || seconds < 1 || seconds > 300 || os.Getenv("P05_PILOT_OUTPUT") == "" {
		t.Fatal("set a 1–300 second hold only with a pilot evidence directory")
	}
	return time.Duration(seconds) * time.Second
}

func forwardThroughPilotClients(t *testing.T, ctx context.Context, server *httptest.Server, order, publicOrder seededOrder,
	terminal string, customerCert, terminalCert tls.Certificate, signature []byte, key string, hold time.Duration) func() pilotPublicSlot {
	t.Helper()
	pemValue := func(kind string, der []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}))
	}
	identity := func(c tls.Certificate, actor string) map[string]string {
		t.Helper()
		der, err := x509.MarshalPKCS8PrivateKey(c.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]string{"subject": actor, "cert": pemValue("CERTIFICATE", c.Certificate[0]), "key": pemValue("PRIVATE KEY", der)}
	}
	readyFile := filepath.Join(t.TempDir(), "forwarded")
	input := map[string]any{"forward": map[string]any{
		"holdMS": hold.Milliseconds(), "readyFile": readyFile, "publicSale": publicOrder.Sale, "asset": lower(order.Target.Asset),
		"url": server.URL, "server": pemValue("CERTIFICATE", server.TLS.Certificates[0].Certificate[0]), "tenant": order.Tenant,
		"branch": order.Branch, "sale": order.Sale, "requestKey": key, "quoteDigest": rawHex(order.Quote.Digest[:]),
		"approvalDigest": rawHex(order.Quote.ApprovalHash[:]), "signature": rawHex(signature), "claimDigest": rawHex(order.Order.Claim[:]),
		"customer": identity(customerCert, order.Actor), "terminal": identity(terminalCert, terminal),
	}}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "node", "../../test/settlement/pilot-client-wire.mjs")
	command.Stdin = bytes.NewReader(raw)
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	var output bytes.Buffer
	command.Stdout = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	finished := false
	t.Cleanup(func() {
		if !finished {
			_ = command.Process.Kill()
			<-done
		}
	})
	var routes []string
	startupDeadline := time.NewTimer(8 * time.Second)
	defer startupDeadline.Stop()
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	for routes == nil {
		select {
		case <-startupDeadline.C:
			t.Fatal("app client did not finish submitting on both paths")
		case <-poll.C:
			data, err := os.ReadFile(readyFile)
			if err == nil {
				_ = json.Unmarshal(data, &routes)
			}
		case err := <-done:
			finished = true
			if err != nil {
				t.Fatalf("dual route payment replay: %s %v", diagnostic.String(), err)
			}
			data, err := os.ReadFile(readyFile)
			if err != nil || json.Unmarshal(data, &routes) != nil {
				t.Fatal("finished app client did not record the submitted routes")
			}
		}
	}
	if len(routes) != 20 {
		t.Fatal("did not observe twenty app route submissions")
	}
	for i, route := range routes {
		if route != [2]string{"mTLS", "nearby-wire"}[i%2] {
			t.Fatal("duplicate submissions did not alternate both product routes")
		}
	}
	return func() pilotPublicSlot {
		t.Helper()
		if !finished {
			err := <-done
			finished = true
			if err != nil {
				t.Fatalf("public display fixture: %s %v", diagnostic.String(), err)
			}
		}
		var result pilotPublicSlot
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
}

func writePilotPublicSlot(t *testing.T, ctx context.Context, db *pgxpool.Pool, out string, hold time.Duration, state pilotPublicSlot, publicPaymentEntered time.Time) {
	t.Helper()
	paidAt := func(sale string) time.Time {
		t.Helper()
		var booked time.Time
		if err := db.QueryRow(ctx, "SELECT booked_at FROM ledger.journal WHERE sale_id=$1 AND refund_id IS NULL", sale).Scan(&booked); err != nil {
			t.Fatal(err)
		}
		return booked
	}
	var privateAccepted time.Time
	if err := db.QueryRow(ctx, "SELECT received_at FROM ledger.settlement_submission WHERE sale_id=$1", state.PrivateSale).
		Scan(&privateAccepted); err != nil {
		t.Fatal(err)
	}
	privatePaid, publicPaid := paidAt(state.PrivateSale), paidAt(state.PublicSale)
	if state.Ended-state.Started < hold.Milliseconds() || !privateAccepted.Before(privatePaid) ||
		privateAccepted.UnixMilli() < state.Started || publicPaymentEntered.UnixMilli() < state.Ended || !privatePaid.Before(publicPaymentEntered) || !publicPaymentEntered.Before(publicPaid) ||
		privatePaid.UnixMilli() < state.ForwardedAt || (hold >= 5*time.Minute && privatePaid.UnixMilli() >= state.Ended) {
		t.Fatalf("public-slot/payment timing invalid: %+v, private=%s public=%s", state, privatePaid, publicPaid)
	}
	if hold >= 5*time.Minute {
		if len(state.MenuActivities) < 6 || state.MenuActivities[len(state.MenuActivities)-1]-state.Started < 250_000 {
			t.Fatalf("five-minute session did not contain continuing menu activity: %v", state.MenuActivities)
		}
		for i := 1; i < len(state.MenuActivities); i++ {
			if state.MenuActivities[i]-state.MenuActivities[i-1] >= 60_000 {
				t.Fatal("public menu activity did not renew the idle interval")
			}
		}
	}
	raw, err := json.MarshalIndent(map[string]any{"mode": "synthetic_app_public_slot_loopback_chain", "publicSlot": state,
		"privateCustomerAcceptedAt": privateAccepted.UTC(), "privateCustomerFinalizedJournalAt": privatePaid.UTC(), "publicCustomerFinalizedJournalAt": publicPaid.UTC(), "publicCustomerFirstApprovalStartedAt": publicPaymentEntered,
		"observedDurationMS": state.Ended - state.Started}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "public-slot.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writePilotConvergence(t *testing.T, out string, eligible, converged time.Time, event string) {
	t.Helper()
	elapsed := converged.Sub(eligible)
	if elapsed < 0 || elapsed > time.Minute {
		t.Fatalf("single pending refund event did not converge within 60s: %s", elapsed)
	}
	raw, err := json.MarshalIndent(map[string]any{
		"mode": "actual_opsd_worker_and_mtls_consumer_after_event_release", "eventId": event,
		"eligibleAt": eligible.UTC(), "convergedAt": converged.UTC(), "elapsedMS": elapsed.Milliseconds(),
		"scope": "one already reconciled refund notice; both consumer and sender inboxes durable",
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "notice-convergence.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
