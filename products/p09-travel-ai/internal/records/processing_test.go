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
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func processorCertificate(t *testing.T, server bool) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	usage := x509.ExtKeyUsageClientAuth
	if server {
		usage = x509.ExtKeyUsageServerAuth
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{usage}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}, parsed
}

func TestProcessorFixtureChild(t *testing.T) {
	dir := os.Getenv("P09_ISOLATED_PROCESSOR_CHILD")
	if dir == "" {
		return
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "server.pem"), filepath.Join(dir, "server.key"))
	if err != nil {
		t.Fatal(err)
	}
	clientCA, err := os.ReadFile(filepath.Join(dir, "client.pem"))
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(clientCA) {
		t.Fatal("fixture client authority")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/work", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(dir, "unavailable")); err == nil {
			http.Error(w, "synthetic provider unavailable", 503)
			return
		}
		var item Work
		if !strictBody(w, r, 1_405_000, &item) || !uuid.MatchString(item.Handle) || len(r.TLS.VerifiedChains) == 0 {
			http.NotFound(w, r)
			return
		}
		result := WorkAck{Handle: item.Handle, Event: "accepted"}
		file := func(s string) string { return filepath.Join(dir, item.Handle+"."+s) }
		switch item.Action {
		case "send":
			sample, e := base64.StdEncoding.Strict().DecodeString(item.Sample)
			if e != nil || string(sample) != "disposable voice sample" {
				http.NotFound(w, r)
				return
			}
			if _, e := os.Stat(file("send")); !errors.Is(e, os.ErrNotExist) {
				http.Error(w, "duplicate work", 409)
				return
			}
			_ = os.WriteFile(file("send"), []byte("sent"), 0o600)
		case "cancel", "status", "delete":
			if item.Sample != "" {
				http.NotFound(w, r)
				return
			}
			_ = os.WriteFile(file(item.Action), []byte("observed"), 0o600)
			gate, e := os.ReadFile(filepath.Join(dir, "terminal"))
			if e == nil {
				at, e := time.Parse(time.RFC3339Nano, string(gate))
				if e != nil {
					http.Error(w, "invalid clock", 500)
					return
				}
				result.Event, result.Ended = "terminal", &at
				if item.Action == "status" {
					result.Transcript = "synthetic transcript"
				}
				if item.Action == "delete" {
					if _, e := os.Stat(filepath.Join(dir, "erasure")); e == nil {
						at2 := time.Now().UTC()
						result.Event, result.Deleted = "erased", &at2
					}
				}
			}
		default:
			http.NotFound(w, r)
			return
		}
		reply(w, 200, result)
	})
	srv := &http.Server{Addr: os.Getenv("P09_ISOLATED_PROCESSOR_ADDR"), Handler: mux,
		ReadHeaderTimeout: 3 * time.Second, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert},
			MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}}
	if err := srv.ListenAndServeTLS("", ""); err != nil {
		t.Fatal(err)
	}
}

func TestScopedProcessorRevocation(t *testing.T) {
	dsn := os.Getenv("P09_PRIVATE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL with P09 schema")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(cfg.Database, "nu54_p09_private") ||
		!(strings.HasPrefix(cfg.Host, "/tmp/") || strings.HasPrefix(cfg.Host, "/private/tmp/")) {
		t.Fatal("use private temporary PG socket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	run := func(stmt string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, stmt, args...); err != nil {
			t.Fatal(err)
		}
	}
	var salt [6]byte
	if _, err := rand.Read(salt[:]); err != nil {
		t.Fatal(err)
	}
	role := "visit_process_" + hex.EncodeToString(salt[:])
	quoted := pgx.Identifier{role}.Sanitize()
	run("CREATE ROLE " + quoted + " LOGIN IN ROLE nu54_p09_processor")
	run(`INSERT INTO travel_private.processing_recipient VALUES($1,'isolated-transcriber')`, role)
	defer admin.Exec(context.Background(), "DROP ROLE "+quoted)
	run(`INSERT INTO travel_private.policy VALUES
	  ('voice',2,'isolated-sync',gen_random_uuid(),600,false,true),
	  ('transcription',2,'isolated-transcriber',gen_random_uuid(),600,true,true)`)
	dir := t.TempDir()
	serverCert, serverLeaf := processorCertificate(t, true)
	clientCert, clientLeaf := processorCertificate(t, false)
	write := func(name string, body []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pemBytes := func(kind string, body []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: body}) }
	private, err := x509.MarshalPKCS8PrivateKey(serverCert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	write("server.pem", pemBytes("CERTIFICATE", serverLeaf.Raw))
	write("server.key", pemBytes("PRIVATE KEY", private))
	write("client.pem", pemBytes("CERTIFICATE", clientLeaf.Raw))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessorFixtureChild$")
	cmd.Env = []string{"P09_ISOLATED_PROCESSOR_CHILD=" + dir, "P09_ISOLATED_PROCESSOR_ADDR=" + addr}
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
		if t.Failed() {
			t.Log(log.String())
		}
	}()
	for i := 0; i < 50; i++ {
		if conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond); err == nil {
			conn.Close()
			break
		}
		if i == 49 {
			t.Fatal("test recipient unavailable")
		}
		time.Sleep(20 * time.Millisecond)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	storage, err := newVault(key)
	if err != nil {
		t.Fatal(err)
	}
	ca := x509.NewCertPool()
	ca.AddCert(serverLeaf)
	pin := sha256.Sum256(serverLeaf.RawSubjectPublicKeyInfo)
	worker, err := OpenWorker(ctx, dsn+" user="+role, "https://"+addr+"/v1/work", "isolated-transcriber", hex.EncodeToString(pin[:]), ca, clientCert, key)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	const actor = "6ed33366-97cd-43f6-b313-830a1ae8d93e"
	const source = "e9bdb8f5-de09-4bae-9878-48a43e23513d"
	const other = "0837ad25-baf3-4f5e-a537-8711d15baf33"
	const job = "9e70c8aa-4d94-4355-aab3-d5cec8b9994e"
	const never = "5834bae6-02d1-4334-b20e-b5b054c1cbbc"
	expiry := time.Now().Add(time.Minute).UTC()
	run(`SELECT travel_private.decide($1,'voice',0,2,'isolated-sync',null,true,$2,gen_random_uuid())`, actor, expiry)
	run(`SELECT travel_private.decide($1,'transcription',0,2,'isolated-transcriber',$2,true,$3,gen_random_uuid())`, actor, source, expiry)
	for _, id := range []string{source, other} {
		plain := []byte("disposable voice sample")
		scope := actor + ":record:" + id
		ciphertext, err := storage.seal(scope, plain)
		if err != nil {
			t.Fatal(err)
		}
		run(`SELECT travel_private.write_record($1,$2,'voice',1,'isolated-sync',null,$3,$4,$5)`,
			actor, id, expiry, storage.dedup(scope, plain), ciphertext)
	}
	jobRow := func(id string) (state, handle string) {
		t.Helper()
		if err := admin.QueryRow(ctx, `SELECT state,handle::text FROM travel_private.processing_job WHERE id=$1`, id).Scan(&state, &handle); err != nil {
			t.Fatal(err)
		}
		return
	}
	expect := func(id, target string) string {
		t.Helper()
		state, handle := jobRow(id)
		if state != target {
			t.Fatalf("wanted %s, got %s", target, state)
		}
		return handle
	}
	step := func() {
		t.Helper()
		if err := worker.Step(ctx); err != nil {
			t.Fatal(err)
		}
	}
	retry := func() {
		t.Helper()
		run(`UPDATE travel_private.processing_job SET retry_at=clock_timestamp() WHERE subject_ref=$1`, actor)
	}
	run(`SELECT travel_private.queue_transcription($1,$2,$3,1,'isolated-transcriber')`, actor, job, source)
	step()
	handle := expect(job, "processing")
	if _, err := os.Stat(filepath.Join(dir, handle+".send")); err != nil {
		t.Fatal("fixture did not receive one audio sample")
	}
	run(`SELECT travel_private.queue_transcription($1,$2,$3,1,'isolated-transcriber')`, actor, never, source)
	run(`SELECT travel_private.decide($1,'transcription',1,2,'isolated-transcriber',$2,false,null,gen_random_uuid())`, actor, source)
	neverHandle := expect(never, "erased")
	if _, err := os.Stat(filepath.Join(dir, neverHandle+".send")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("withdrawal sent a queued sample")
	}
	retry()
	step()
	expect(job, "cancel-pending")
	write("terminal", []byte(time.Now().UTC().Format(time.RFC3339Nano)))
	retry()
	step()
	expect(job, "delete-pending")
	retry()
	step()
	expect(job, "delete-pending")
	write("unavailable", []byte("isolated provider failure"))
	retry()
	if err := worker.Step(ctx); err == nil {
		t.Fatal("recipient HTTP failure was ignored")
	}
	expect(job, "delete-pending")
	if err := os.Remove(filepath.Join(dir, "unavailable")); err != nil {
		t.Fatal(err)
	}
	write("erasure", []byte("approve fixture action"))
	retry()
	step()
	expect(job, "erased")
	var retained int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM travel_private.my_records($1)`, actor).Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("transcription withdrawal damaged independent source records: %d %v", retained, err)
	}
	var ready bool
	if err := admin.QueryRow(ctx, `SELECT deleted_at>=ended_at FROM travel_private.processing_job WHERE id=$1`, job).Scan(&ready); err != nil || !ready {
		t.Fatal("in-flight work claimed completion without deletion proof")
	}
	const approvedJob = "f05857b5-852c-4966-82b6-f6aa3e1ccd07"
	run(`SELECT travel_private.decide($1,'transcription',2,2,'isolated-transcriber',$2,true,$3,gen_random_uuid())`, actor, other, expiry)
	run(`SELECT travel_private.queue_transcription($1,$2,$3,3,'isolated-transcriber')`, actor, approvedJob, other)
	step()
	expect(approvedJob, "processing")
	write("terminal", []byte(time.Now().UTC().Format(time.RFC3339Nano)))
	retry()
	step()
	expect(approvedJob, "delete-pending")
	var purpose string
	var sealed []byte
	if err := admin.QueryRow(ctx, `SELECT purpose,sealed FROM travel_private.read_record($1,$2)`, actor, approvedJob).
		Scan(&purpose, &sealed); err != nil || purpose != "transcription" {
		t.Fatalf("approved processor result was not available: %v", err)
	}
	plain, err := storage.open(actor+":record:"+approvedJob, sealed)
	if err != nil || string(plain) != "synthetic transcript" || bytes.Contains(sealed, plain) {
		t.Fatal("processor content was not sealed to the approved derived scope")
	}
	retry()
	step()
	expect(approvedJob, "erased")

}
