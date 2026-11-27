package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
)

func TestPostgresRentalRuntimeWriterAuthenticatesBoardAndP07(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires migrated disposable PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(config.Host, "/private/tmp/") || strings.HasPrefix(config.Host, "/tmp/")) {
		t.Fatal("use a disposable commerce database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	const (
		tenant           = "17ee49c5-c96e-48cc-a2e7-261efd3ec983"
		branch           = "a66e84c6-dc18-4d57-bf86-da8e35b11aa8"
		actor            = "4375093b-f96d-4838-9cc0-ce7bbf9577f9"
		device           = "b207d53a-1b90-45c1-ac6d-ecedadf8d142"
		stream           = "aafe381f-ffe3-411e-b2bb-ef0158fc7bc8"
		streamGeneration = "bbfe381f-ffe3-411e-b2bb-ef0158fc7bc8"
		account          = "0x6161616161616161616161616161616161616161"
		payout           = "0x6262626262626262626262626262626262626262"
		settlement       = "0x6464646464646464646464646464646464646464"
		writer           = "rental_073_fixture_writer"
		request          = "a66084c6-dc18-4d57-bf86-da8e35b11aa8"
	)
	fill := func(v byte) []byte { return bytes.Repeat([]byte{v}, 32) }
	deployment := core.Deployment{ChainID: 8283, Settlement: common.HexToAddress(settlement)}
	manifest, registration, measurement, challenge, board := fill(0x74), fill(0x71), fill(0x72), fill(0x73), fill(0x75)
	holder, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	operator, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.PubkeyToAddress(holder.PublicKey)
	now := time.Now().Truncate(time.Second)
	deadline := now.Add(time.Minute)
	binding := rentalCollectorBinding(branch, device, 1, deployment, account, registration, measurement, manifest)
	first, peer := make([]byte, 32), []byte{1, 2, 3, 4, 5, 6, 7}
	collector := [32]byte(fill(6))
	installation, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	install := elliptic.Marshal(installation.Curve, installation.X, installation.Y)
	confirmation := confirmedRegistrationDigest(ops.RentalRegistrationScope{Binding: binding, Challenge: [32]byte(challenge), Board: [32]byte(board),
		FreezeSigner: signer, CollectorCert: collector, CollectorPeer: [7]byte(peer), FirstNonce: [32]byte(first), ExpiresAt: uint64(deadline.Unix())}, install)
	exec("CREATE ROLE " + writer + " LOGIN IN ROLE nu54_p05_rental_evidence")
	defer admin.Exec(context.Background(), "DROP ROLE "+writer)
	exec("INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	exec("INSERT INTO ledger.login_subject VALUES('rental_073_fixture',$1)", actor)
	exec("INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',now()+interval '3 minutes')", branch, actor)
	exec(`INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
		VALUES($1,$2,1,$3,$4,'qualified',$5,now()+interval '2 minutes',now()-interval '1 minute')`, branch, device, account, payout, registration)
	exec(`INSERT INTO ledger.rental_generation_source VALUES($1,$2,1,$3,$4,$5,'qualified',$6,$7,$8,$9,$10,$11,$12,
        $13,$14,now()+interval '2 minutes')`, branch, device, registration, account, payout, measurement, strings.ToLower(signer.Hex()), settlement, manifest, stream, streamGeneration, challenge, deadline, writer)
	exec("INSERT INTO ledger.firmware_member VALUES($1,$2,'qualified',$3,$4,2,'0.0.2',false,NULL)", branch, device, board, install)
	exec(`INSERT INTO ledger.firmware_campaign(branch_id,device_ref,campaign_ref,actor_id,release_digest,version,counter,measurement,challenge,challenge_until,state,proof_digest)
		VALUES($1,$2,$3,$4,$5,'0.0.2',2,$6,$7,$8,'verified',$9)`, branch, device, streamGeneration, actor, hex.EncodeToString(fill(7)), measurement, fill(8), deadline, fill(9))
	exec(`INSERT INTO ledger.rental_collector_registration(branch_id,device_ref,generation,operator_ref,collector_certificate,collector_peer,
		first_nonce,registration_proof,authority_source,valid_until)
		SELECT $1,$2,1,$3,$4,$5,$6,$7,ledger.rental_generation_source_digest(s),$8 FROM ledger.rental_generation_source s
        WHERE (s.branch_id,s.device_ref,s.generation)=($1,$2,1)`, branch, device, actor, collector[:], peer, first, confirmation[:], deadline)
	projection := make([]byte, 277)
	copy(projection, "RJP1")
	copy(projection[4:], binding[:])
	receipt := rentalv1.FreezeReceipt{Binding: crypto.Keccak256Hash(binding[:]), Challenge: [32]byte(challenge),
		Root: crypto.Keccak256Hash(projection), FrozenAt: uint64(now.Add(-time.Second).Unix()), ExpiresAt: uint64(deadline.Unix())}
	digest := receipt.Digest(8283, deployment.Settlement)
	signature, err := core.SignDigest(digest[:], holder)
	if err != nil {
		t.Fatal(err)
	}
	receipt.BoardSignature = [65]byte(signature)
	body := rentalSubmission{Key: request, Projection: projection, Receipt: receipt.Bytes()}
	clientKey, clientPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(2 * time.Minute), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, clientKey, clientPrivate)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}
	trustClient := x509.NewCertPool()
	trustClient.AddCert(cert)
	var called, scenario atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		var forwarded rentalv1.Request
		if r.URL.Path != rentalv1.ReconcilePath || r.TLS == nil || len(r.TLS.VerifiedChains) == 0 ||
			json.NewDecoder(r.Body).Decode(&forwarded) != nil || !bytes.Equal(forwarded.Projection, projection) || !bytes.Equal(forwarded.Receipt, body.Receipt) || forwarded.Previous != nil {
			t.Error("P07 request differed from authenticated board proof")
			w.WriteHeader(400)
			return
		}
		switch scenario.Load() {
		case 1:
			_ = json.NewEncoder(w).Encode(rentalv1.Assessment{Status: "incomplete"})
			return
		case 2:
			conn, err := pgx.ConnectConfig(r.Context(), config)
			if err == nil {
				_, err = conn.Exec(r.Context(), "DELETE FROM ledger.finance_grant WHERE branch_id=$1 AND actor_id=$2", branch, actor)
				_ = conn.Close(r.Context())
			}
			if err != nil {
				t.Error("concurrent operator grant could not be withdrawn", err)
			}
		}
		zero, height := 0, int64(100)
		_ = json.NewEncoder(w).Encode(rentalv1.Assessment{Status: "verified",
			Binding: rentalv1.Binding{Stream: stream, StreamGeneration: streamGeneration, Deployment: common.Hash(manifest).Hex(),
				FreezeReceipt: common.Hash(digest).Hex(), JournalRoot: common.Hash(receipt.Root).Hex()},
			PendingCount: &zero, PendingPayments: &zero, PendingLimits: &zero, AnchorHeight: &height, AnchorHash: common.Hash(fill(10)).Hex(),
			AnchorTime: now.Unix(), ValidUntil: deadline.Add(-time.Second).Unix(), Digest: "sha256:" + hex.EncodeToString(fill(0x7b))})
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trustClient}
	server.StartTLS()
	defer server.Close()
	trustServer := x509.NewCertPool()
	trustServer.AddCert(server.Certificate())
	reader, err := core.NewRentalEvidenceReader(server.URL, trustServer, tls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: clientPrivate},
		sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	runtime, err := OpenRentalProofWriter(ctx, config.ConnString()+" user="+writer, reader, deployment, [32]byte(manifest), crypto.PubkeyToAddress(operator.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	identity := service.VerifiedClient{Fingerprint: collector, ExpiresAt: deadline}
	record := func(input rentalSubmission) error {
		return runtime.record(ctx, tenant, branch, device, 1, identity, input)
	}
	tampered := body
	tampered.Receipt = bytes.Clone(body.Receipt)
	tampered.Receipt[200] ^= 1
	if record(tampered) == nil || called.Load() != 0 {
		t.Fatal("unsigned journal reached P07")
	}
	scenario.Store(1)
	if record(body) == nil || called.Load() != 1 {
		t.Fatal("incomplete P07 result was recorded")
	}
	scenario.Store(2)
	if record(body) == nil || called.Load() != 2 {
		t.Fatal("withdrawn live scope was recorded")
	}
	exec("INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',now()+interval '3 minutes')", branch, actor)
	scenario.Store(0)
	if err := record(body); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if err := record(body); err != nil || called.Load() != 3 {
		t.Fatal("committed retry unexpectedly needed P07", err)
	}
	if record(tampered) == nil {
		t.Fatal("same key accepted a different journal")
	}
	var records int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM ledger.rental_proof_submission WHERE branch_id=$1", branch).Scan(&records); err != nil || records != 1 {
		t.Fatalf("expected one stored retry commitment, got %d, %v", records, err)
	}
	var hold bool
	if err := admin.QueryRow(ctx, "SELECT ledger.rental_runtime_close_hold($1,$2,$3,$4,1)", tenant, branch, actor, device).Scan(&hold); err != nil || !hold {
		t.Fatal("a stored runtime proof did not retain the live authority hold", err)
	}
	var review string
	if err := admin.QueryRow(ctx, "SELECT ledger.rental_generation_authorized_review($1,$2,$3,$4,1)->>'returnReviewRef'",
		tenant, branch, actor, device).Scan(&review); err != nil || review == "" {
		t.Fatal("fixture did not create an otherwise valid stored zero", err)
	}
	if _, err := admin.Exec(ctx, "SELECT ledger.begin_rental_action($1,$2,$3,$4,1,'close',$5,NULL,$6)", tenant, branch, actor, device, request, review); err == nil {
		t.Fatal("stored zero alone reserved a close without live authority")
	}

	exec(`INSERT INTO ledger.rental_operation(branch_id,device_ref,generation,action,request_key,actor_id,status)
		VALUES($1,$2,1,'close',$3,$4,'dispatching')`, branch, device, request, actor)
	var dispatch bool
	if err := admin.QueryRow(ctx, "SELECT ledger.begin_rental_action($1,$2,$3,$4,1,'close',$5,NULL,NULL)",
		tenant, branch, actor, device, request).Scan(&dispatch); err != nil || dispatch {
		t.Fatal("committed close was dispatched again or could not be retrieved", err)
	}

}
