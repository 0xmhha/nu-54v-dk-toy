package settlement

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/jackc/pgx/v5/pgxpool"
)

func verifyNativePrint(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn string, proof *EvidenceStore,
	order seededOrder, frame PrintFrame, unknown, dir string, worker Printer, printer *recordingPrinter) {
	t.Helper()
	staff := fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES($1,$2)", "native_print_fixture", staff)
	fixtureExec(t, db, "INSERT INTO ledger.staff_grant VALUES($1,$2,'operator',clock_timestamp()-interval '1 second',NULL)", order.Branch, staff)
	fixtureExec(t, db, "INSERT INTO ledger.print_staff_grant VALUES($1,$2,clock_timestamp()+interval '1 hour')", order.Branch, staff)
	serverCert := newNativeLoopbackTLS(t)
	printerGrants := map[string]service.Principal{}
	principal := service.Principal{Tenant: order.Tenant, Subject: staff, Role: "printer_staff", ExpiresAt: time.Now().Add(10 * time.Minute).Unix()}
	clientPEM, lab := enrollNativeDevices(t, dir, serverCert, nil, printerGrants, principal)
	operatorGrants := map[string]service.Principal{}
	for fingerprint, staffGrant := range printerGrants {
		staffGrant.Role = "operator"
		operatorGrants[fingerprint] = staffGrant
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(clientPEM) {
		t.Fatal("fixture device CA unavailable")
	}
	_, printDSN := testLogin(t, db, dsn, "nu54_p05_print_staff")
	staffStore, err := Open(ctx, printDSN, "nu54_p05_print_staff")
	if err != nil {
		t.Fatal(err)
	}
	defer staffStore.Close()
	_, commerceDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	commerceStore, err := commerce.Open(ctx, commerceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer commerceStore.Close()
	mux := http.NewServeMux()
	PrintStaffHTTP{Store: staffStore, Evidence: proof, Credentials: printerGrants}.Register(mux)
	commerce.HTTP{Store: commerceStore, Credentials: operatorGrants}.Register(mux)
	var accepted, replayed atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v2/fulfillment/print-decisions" {
			mux.ServeHTTP(w, r)
			return
		}
		// Disconnect after the real handler committed the first valid decision.
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, r)
		if response.Code == 200 && accepted.Add(1) == 1 {
			panic(http.ErrAbortHandler)
		}
		if response.Code == 200 {
			replayed.Add(1)
		}
		for key, values := range response.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
		Certificates: []tls.Certificate{serverCert}}
	server.StartTLS()
	defer server.Close()
	leaf, err := x509.ParseCertificate(lab.server)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	for platform, der := range lab.leaves {
		profile := map[string]any{"endpoint": server.URL + "/v2/commerce", "pin": hex.EncodeToString(pin[:]),
			"ca": base64.StdEncoding.EncodeToString(lab.anchor), "certificates": []string{base64.StdEncoding.EncodeToString(der)},
			"scope": map[string]any{"branchRef": order.Branch, "tenantRef": order.Tenant, "subjectRef": staff,
				"role": "operator", "expiresAt": principal.ExpiresAt}}
		encoded, err := json.Marshal(map[string]any{"profile": profile, "staffPermissions": []string{"terminal_settings", "print_receipts"},
			"receiptRef": frame.ReceiptRef, "saleRef": order.Sale, "pickupNo": frame.PickupNo})
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, platform+".json")
		defer os.Remove(file)
		if err := os.WriteFile(file+".tmp", encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(file+".tmp", file); err != nil {
			t.Fatal(err)
		}
	}
	timer := time.NewTimer(180 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-timer.C:
			t.Fatal("K05 native UI fixture did not finish")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "failed")); err == nil {
				t.Fatal("K05 native UI reported failure")
			}
			if _, err := os.Stat(filepath.Join(dir, "done")); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				continue
			}
			var newAttempt string
			var decisions int
			if err := db.QueryRow(ctx, `SELECT count(*),min(result_id::text) FROM ledger.print_decision
				WHERE actor_id=$1 AND attempt_id=$2 AND action='reprint'`, staff, unknown).Scan(&decisions, &newAttempt); err != nil || decisions != 1 || accepted.Load() < 2 || replayed.Load() < 1 || len(printer.attempts) != 3 {
				t.Fatalf("expected one new decision despite committed response loss: %d/%d/%d/%d %v",
					decisions, accepted.Load(), replayed.Load(), len(printer.attempts), err)
			}
			if did, _ := worker.Step(ctx); !did || len(printer.attempts) != 4 ||
				printer.attempts[3].AttemptID != newAttempt || printer.attempts[3].ReceiptRef != frame.ReceiptRef {
				t.Fatal("native explicit copy did not preserve the original receipt")
			}
			if did, err := worker.Step(ctx); did || err != nil {
				t.Fatal("unknown native copy automatically reprinted")
			}
			t.Log("native mTLS operator resumed the original decision; one durable new attempt, same receipt, zero unsolicited copies")
			return
		}
	}
}
