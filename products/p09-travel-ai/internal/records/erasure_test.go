package records

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSelectedErasureAndRestore(t *testing.T) {
	dsn, journal := os.Getenv("P09_PRIVATE_TEST_DSN"), os.Getenv("P09_ERASURE_TEST_DSN")
	if dsn == "" || journal == "" {
		t.Skip("requires independent disposable P09 workload and journal databases")
	}
	for _, str := range []string{dsn, journal} {
		cfg, err := pgx.ParseConfig(str)
		if err != nil || !strings.HasPrefix(cfg.Database, "nu54_p09_") ||
			!(strings.HasPrefix(cfg.Host, "/tmp/") || strings.HasPrefix(cfg.Host, "/private/tmp/")) {
			t.Fatal("use private temporary PG databases")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	w, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close(context.Background())
	j, err := pgx.Connect(ctx, journal)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close(context.Background())
	run := func(conn *pgx.Conn, stmt string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, stmt, args...); err != nil {
			t.Fatal(err)
		}
	}
	createRole := func(conn *pgx.Conn, name, group string) string {
		t.Helper()
		var salt [6]byte
		if _, err := rand.Read(salt[:]); err != nil {
			t.Fatal(err)
		}
		login := name + "_" + hex.EncodeToString(salt[:])
		quoted := pgx.Identifier{login}.Sanitize()
		run(conn, "CREATE ROLE "+quoted+" LOGIN IN ROLE "+group)
		t.Cleanup(func() {
			conn2, err := pgx.Connect(context.Background(), dsn)
			if err == nil {
				defer conn2.Close(context.Background())
				conn2.Exec(context.Background(), "DROP ROLE "+quoted)
			}
		})
		return login
	}
	workRole := createRole(w, "erase_work", "nu54_p09_erasure_worker")
	journalRole := createRole(j, "erase_append", "nu54_p09_erasure_append")
	exportRole := createRole(j, "erase_export", "nu54_p09_erasure_export")
	applyRole := createRole(w, "erase_apply", "nu54_p09_restore_apply")
	worker, err := OpenErasureWorker(ctx, dsn+" user="+workRole, journal+" user="+journalRole)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	const namespace = "a7cc392e-d757-4351-9f60-34d966efd8e6"
	const actor = "522b4262-1aca-47b0-835a-72e2aedfaac3"
	const root = "e4730111-bcd4-43f2-8c12-accad0e737e0"
	const survivor = "fa54f8fe-603a-4651-8c4b-e36701dae6da"
	const child = "ad73fba7-c676-4ebc-ad90-3e60424d07ce"
	const job = "1c161a42-ded2-4727-a978-1c6c0567531b"
	const request = "e2d996ec-cf48-4274-bc9c-85ae71a6d0b3"
	run(j, `INSERT INTO travel_erasure.namespace(namespace) VALUES($1)`, namespace)
	run(w, `INSERT INTO travel_private.policy VALUES
	    ('voice',3,'erase-sync',gen_random_uuid(),500,false,true),
	    ('transcription',3,'erase-transcriber',gen_random_uuid(),500,true,true)`)
	deadline := time.Now().UTC().Add(time.Minute).Truncate(time.Microsecond)
	run(w, `SELECT travel_private.decide($1,'voice',0,3,'erase-sync',NULL,true,$2,gen_random_uuid())`, actor, deadline)
	run(w, `SELECT travel_private.decide($1,'transcription',0,3,'erase-transcriber',$2,true,$3,gen_random_uuid())`, actor, root, deadline)
	writeOriginal := func() {
		t.Helper()
		run(w, `SELECT travel_private.write_record($1,$2,'voice',1,'erase-sync',NULL,$3,decode(repeat('00',32),'hex'),decode(repeat('01',35),'hex'))`, actor, root, deadline)
		run(w, `SELECT travel_private.write_record($1,$2,'voice',1,'erase-sync',NULL,$3,decode(repeat('00',32),'hex'),decode(repeat('01',35),'hex'))`, actor, survivor, deadline)
		run(w, `SELECT travel_private.write_record($1,$2,'transcription',1,'erase-transcriber',$3,$4,decode(repeat('00',32),'hex'),decode(repeat('01',35),'hex'))`, actor, child, root, deadline)
	}
	writeOriginal()
	run(w, `SELECT travel_private.queue_transcription($1,$2,$3,1,'erase-transcriber')`, actor, job, root)
	run(w, `SELECT travel_private.request_selected_erasure($1,$2,$3)`, actor, request, root)
	run(w, `SELECT travel_private.request_selected_erasure($1,$2,$3)`, actor, request, root)
	check := func(stmt string, want int, args ...any) {
		t.Helper()
		var count int
		if err := w.QueryRow(ctx, stmt, args...).Scan(&count); err != nil || count != want {
			t.Fatalf("expected %d rows, got %d %v", want, count, err)
		}
	}
	check(`SELECT count(*) FROM travel_private.my_records($1) WHERE id=$2`, 1, actor, survivor)
	check(`SELECT count(*) FROM travel_private.my_records($1) WHERE id IN ($2,$3)`, 0, actor, root, child)
	if err := worker.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if err := worker.Step(ctx); err != nil {
		t.Fatal(err)
	}
	check(`SELECT count(*) FROM travel_private.record_object WHERE subject_ref=$1`, 1, actor)
	check(`SELECT count(*) FROM travel_private.processing_job WHERE id=$1 AND state='erased'`, 1, job)
	check(`SELECT count(*) FROM travel_private.erasure_request WHERE id=$1 AND journaled_at IS NOT NULL AND this_device_erased_at IS NULL`, 1, request)

	// Model restoring the pre-erasure workload rows, with the independent journal intact.
	run(w, `DELETE FROM travel_private.erasure_target WHERE request_id=$1`, request)
	run(w, `DELETE FROM travel_private.erasure_request WHERE id=$1`, request)
	run(w, `DELETE FROM travel_private.processing_job WHERE id=$1`, job)
	writeOriginal()
	run(w, `SELECT travel_private.queue_transcription($1,$2,$3,1,'erase-transcriber')`, actor, job, root)
	check(`SELECT count(*) FROM travel_private.record_object WHERE subject_ref=$1`, 3, actor)
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	certificate, leaf := processorCertificate(t, false)
	hash := sha256.Sum256(leaf.Raw)
	protected := map[string]Principal{hex.EncodeToString(hash[:]): {Subject: actor, Role: "restore", ExpiresAt: time.Now().Add(time.Minute).Unix()}}
	exporter, err := OpenExporter(ctx, journal+" user="+exportRole, seed, protected)
	if err != nil {
		t.Fatal(err)
	}
	defer exporter.Close()
	page, err := exporter.export(ctx, 0)
	if err != nil || len(page.Entries) != 3 || page.HighWater != "3" {
		t.Fatalf("unexpected independent archive %v %v", page.HighWater, err)
	}
	body, err := page.signedBytes()
	if err != nil {
		t.Fatal(err)
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(exporter.key, body))
	applier, err := OpenApplier(ctx, dsn+" user="+applyRole, namespace, page.HighWater, exporter.key.Public().(ed25519.PublicKey), protected)
	if err != nil {
		t.Fatal(err)
	}
	defer applier.Close()
	altered := page
	altered.Through = "2"
	if _, err := applier.apply(ctx, PageEnvelope{Page: altered, Signature: signature}, func() bool { return true }); err == nil {
		t.Fatal("tampered archive applied")
	}
	valid := PageEnvelope{Page: page, Signature: signature}
	checks := 0
	if _, err := applier.apply(ctx, valid, func() bool { checks++; return checks == 1 }); err != errGrantChanged || checks != 2 {
		t.Fatalf("restore transaction did not abort at authority recheck: %v (%d checks)", err, checks)
	}
	check(`SELECT count(*) FROM travel_private.record_object WHERE subject_ref=$1`, 3, actor)
	grantPath := filepath.Join(t.TempDir(), "published.json")
	current, err := json.Marshal(protected)
	if err != nil {
		t.Fatal(err)
	}
	publishGrantFixture(t, grantPath, current)
	exporter.UseGrantFile(grantPath)
	applier.UseGrantFile(grantPath)
	mux := http.NewServeMux()
	exporter.Register(mux)
	applier.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	httpClient := server.Client()
	httpClient.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{certificate}
	route := func(method, path string, data []byte, want int) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		res, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil || res.StatusCode != want {
			t.Fatalf("restore response status=%d want=%d read=%v", res.StatusCode, want, err)
		}
		return raw
	}
	const exportPath = "/v1/restore/fences?after=0"
	archive := route(http.MethodGet, exportPath, nil, http.StatusOK)
	var approved PageEnvelope
	if err := json.Unmarshal(archive, &approved); err != nil || approved.Signature != signature {
		t.Fatal("normal journal export did not sign the expected page")
	}
	publishGrantFixture(t, grantPath, []byte("{}"))
	if denied := route(http.MethodGet, exportPath, nil, http.StatusNotFound); bytes.Contains(denied, []byte("signature")) {
		t.Fatal("withdrawn restore reader received a signed page")
	}
	route(http.MethodPost, "/v1/restore/apply", archive, http.StatusNotFound)
	check(`SELECT count(*) FROM travel_private.record_object WHERE subject_ref=$1`, 3, actor)
	publishGrantFixture(t, grantPath, current)
	for range 2 {
		result := route(http.MethodPost, "/v1/restore/apply", archive, http.StatusOK)
		var restored RestoreStatus
		if err := json.Unmarshal(result, &restored); err != nil || !restored.Ready || restored.AppliedThrough != "3" {
			t.Fatalf("restore not idempotent: %v %v", restored, err)
		}
	}

	check(`SELECT count(*) FROM travel_private.my_records($1) WHERE id=$2`, 1, actor, survivor)
	check(`SELECT count(*) FROM travel_private.record_object WHERE subject_ref=$1 AND record_id IN($2,$3)`, 0, actor, root, child)
	check(`SELECT count(*) FROM travel_private.processing_job WHERE id=$1 AND state='erased'`, 1, job)
}
