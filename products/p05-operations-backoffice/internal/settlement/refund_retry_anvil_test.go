package settlement

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

func verifyFinalizedRefundRetry(t *testing.T, ctx context.Context, db *pgxpool.Pool, chain *ethclient.Client, w *Worker, order seededOrder,
	step func(string) error, indexerTip func() int64, post func(int, string, any, int) []byte, browser func(string, string, string, int)) {
	t.Helper()
	const reserve = "/v2/finance/refund-reserve"
	const approve = "/v2/finance/refund-approve"
	const submit = "/v2/finance/refund-submit"
	realTime := w.Now
	defer func() { w.Now = realTime }()
	alignClock := func() {
		t.Helper()
		head, err := chain.HeaderByNumber(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		offset := max(time.Duration(0), time.Until(time.Unix(int64(head.Time)+1, 0)))
		w.Now = func() time.Time { return time.Now().Add(offset) }
	}
	alignClock()
	input := map[string]string{"branchId": order.Branch, "saleId": order.Sale, "idempotencyKey": fixtureUUID(t, db), "amount": pilotUnits(1), "reason": "partial_service"}
	var request map[string]string
	if err := json.Unmarshal(post(1, reserve, input, 200), &request); err != nil {
		t.Fatal(err)
	}
	refund := request["refundId"]
	decision := map[string]any{"branchId": order.Branch, "saleId": order.Sale, "refundId": refund}
	post(2, approve, decision, 200)
	post(3, submit, decision, 200)
	// Make the actual first Anvil transaction fail its contract expiry. The
	// worker wall clock still permits transport of the original signed bytes.
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	var oldHash string
	var expires time.Time
	if err := db.QueryRow(ctx, "SELECT tx_hash,expires_at FROM ledger.relay_attempt WHERE work_id=$1", refund).Scan(&oldHash, &expires); err != nil {
		t.Fatal(err)
	}
	if err := chain.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", expires.Unix()+1); err != nil {
		t.Fatal(err)
	}
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	failed, err := bind.WaitMinedHash(ctx, chain, common.HexToHash(oldHash))
	if err != nil || failed.Status != types.ReceiptStatusFailed {
		t.Fatalf("real first refund did not revert: %v", err)
	}
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	assertReview := func(want string, revision int64, history int) {
		t.Helper()
		var status string
		var epoch int64
		var count int
		if err := db.QueryRow(ctx, `SELECT r.state,r.review_revision,
		  (SELECT count(*) FROM ledger.refund_failed_attempt WHERE refund_id=r.id)
		  FROM ledger.refund r WHERE r.id=$1`, refund).Scan(&status, &epoch, &count); err != nil || status != want || epoch != revision || count != history {
			t.Fatalf("refund retry boundary: %s/%d/history %d: %v", status, epoch, count, err)
		}
	}
	assertReview("inflight", 0, 0)
	if browser != nil {
		browser("pre-final", refund, oldHash, 2)
	}
	mine(t, ctx, chain, 5)
	waitUntil(t, func() bool { return indexerTip() >= failed.BlockNumber.Int64() })
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	assertReview("requested", 1, 1)
	post(2, approve, decision, 409)
	post(3, submit, decision, 409)
	decision["reviewRevision"] = 1
	post(1, approve, decision, 404)
	if browser != nil {
		browser("maker", refund, oldHash, 1)
		browser("approve", refund, oldHash, 2)
	} else {
		post(2, approve, decision, 200)
	}
	post(2, submit, decision, 404)
	if browser != nil {
		browser("treasury", refund, oldHash, 3)
	} else {
		post(3, submit, decision, 200)
	}
	post(3, submit, decision, 200)
	// Retain the original refund and chain idempotency key, with a fresh
	// authorization window after the deliberately advanced Anvil block.
	alignClock()
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	var nextHash string
	if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", refund).Scan(&nextHash); err != nil || nextHash == oldHash {
		t.Fatalf("retry did not receive a distinct signed attempt: %v", err)
	}
	header, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := chain.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", header.Time+1); err != nil {
		t.Fatal(err)
	}
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	paid, err := bind.WaitMinedHash(ctx, chain, common.HexToHash(nextHash))
	if err != nil || paid.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("renewed real refund did not settle: %v", err)
	}
	mine(t, ctx, chain, 5)
	waitUntil(t, func() bool { return indexerTip() >= paid.BlockNumber.Int64() })
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	assertReview("final", 1, 1)
	if browser != nil {
		browser("final", refund, nextHash, 2)
	}
	var confirmations int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM ledger.refund_confirmation c
	  JOIN ledger.refund r ON r.id=c.refund_id JOIN ledger.payment p ON p.sale_id=r.sale_id
	  WHERE r.id=$1 AND r.sale_id=$2 AND r.amount=$3::text::numeric AND p.payer=$4 AND c.tx_hash=$5`,
		refund, order.Sale, pilotUnits(1), lower(order.Order.Payer), nextHash).Scan(&confirmations); err != nil || confirmations != 1 {
		t.Fatalf("renewed receipt did not preserve the original refund once: %d %v", confirmations, err)
	}
	if receipt := post(1, reserve, input, 200); !json.Valid(receipt) {
		t.Fatal("original request receipt unavailable")
	}
	t.Log("actual failed Anvil refund remained held until P07 finality. Stale roles/revision denied; same amount, order and recipient settled once after renewed approval and an independent treasury request.")
}

func refundReviewBrowser(t *testing.T, ctx context.Context, script string, order seededOrder, certificates []tls.Certificate,
	trust *x509.CertPool, finance FinanceHTTP) func(string, string, string, int) {
	t.Helper()
	dist := os.Getenv("NU54_FINANCE_BROWSER_DIST")
	if !filepath.IsAbs(script) || !filepath.IsAbs(dist) || os.Getenv("P05_PILOT_HUMAN_UNITS") == "" {
		t.Fatal("refund review browser requires absolute script and dist paths and human fixture amounts")
	}
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	t.Cleanup(server.Close)
	origin := "https://" + server.Listener.Addr().String()
	finance.OperatorOrigin = origin
	finance.Register(mux)
	mux.Handle("/ops/", http.StripPrefix("/ops/", http.FileServer(http.Dir(dist))))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	dir := t.TempDir()
	args := make([][]string, len(certificates))
	for _, identity := range []int{1, 2, 3} {
		certPath := filepath.Join(dir, fmt.Sprintf("%d-cert.pem", identity))
		keyPath := filepath.Join(dir, fmt.Sprintf("%d-key.pem", identity))
		certificate := certificates[identity]
		key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
		if err != nil {
			t.Fatal("cannot serialize disposable fixture certificate key")
		}
		for _, part := range []struct {
			path, label string
			bytes       []byte
		}{
			{certPath, "CERTIFICATE", certificate.Certificate[0]}, {keyPath, "PRIVATE KEY", key},
		} {
			if err := os.WriteFile(part.path, pem.EncodeToMemory(&pem.Block{Type: part.label, Bytes: part.bytes}), 0600); err != nil {
				t.Fatal("cannot write disposable fixture certificate")
			}
		}
		args[identity] = []string{certPath, keyPath}
	}
	return func(stage, refund, hash string, identity int) {
		t.Helper()
		arguments := append([]string{script, origin, order.Branch, order.Sale, refund, hash, stage}, args[identity]...)
		arguments = append(arguments, args[2]...)
		out, err := exec.CommandContext(ctx, "node", arguments...).CombinedOutput()
		if err != nil {
			t.Fatalf("refund review browser stage %s: %v\n%s", stage, err, out)
		}
		t.Logf("refund review browser: %s", out)
	}
}
