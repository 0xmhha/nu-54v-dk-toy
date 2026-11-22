package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
)

func TestOpsdRentalAuthorityReadAudience(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	program := os.Getenv("P05_074_TEST_OPSD")
	if dsn == "" || program == "" {
		t.Skip("requires disposable migrated PostgreSQL and an opsd build")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(config.Host, "/private/tmp/") || strings.HasPrefix(config.Host, "/tmp/")) {
		t.Fatal("use a disposable commerce database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	db, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	execSQL := func(statement string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, statement, args...); err != nil {
			t.Fatal(err)
		}
	}

	operator, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	operatorAddr := crypto.PubkeyToAddress(operator.PublicKey)
	dir := t.TempDir()
	write := func(file string, body []byte) string {
		t.Helper()
		path := filepath.Join(dir, file)
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	settings := func(file string, v any) string {
		t.Helper()
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return write(file, body)
	}
	pemFile := func(file, kind string, body []byte) string {
		return write(file, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: body}))
	}
	const (
		branch       = "adff5449-60f5-4d36-9dd2-267682faffd3"
		board        = "bdfc5449-60f5-4d36-9dd2-267682faffd3"
		tenant       = "ff435449-60f5-4d36-9dd2-267682faffd3"
		actor        = "aff65449-60f5-4d36-9dd2-267682faffd3"
		stream       = "f425ed7e-2365-4fbe-a6fe-c7dbacaef051"
		streamEpoch  = "8455ed7e-2365-4fbe-a6fe-c7dbacaef051"
		account      = "0x7171717171717171717171717171717171717171"
		settlement   = "0x7272727272727272727272727272727272727272"
		payout       = "0x7373737373737373737373737373737373737373"
		signer       = "0x7474747474747474747474747474747474747474"
		reader       = "rental_074_fixture_reader"
		writer       = "rental_074_fixture_writer"
		commerceRole = "rental_074_fixture_commerce"
	)
	manifest, err := json.Marshal(map[string]any{"chainId": 8283, "contracts": map[string]any{
		"PaymentSettlement": map[string]any{"address": settlement, "block": 1},
		"MerchantRegistry":  map[string]any{"address": payout}, "TestUSDC": map[string]any{"address": signer},
	}})
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(manifest)
	fill := func(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }
	challenge, registration, measurement, boardID, collector := fill(0x74), fill(0x41), fill(0x42), fill(0x43), fill(0x44)
	first, peer := make([]byte, 32), []byte{1, 2, 3, 4, 5, 6, 7}
	now := time.Now().Truncate(time.Second)
	registeredAt, expires := now.Add(-time.Minute), now.Add(time.Minute)
	for login, role := range map[string]string{reader: "nu54_p05_rental_authority_read", writer: "nu54_p05_rental_evidence", commerceRole: "nu54_p05_commerce"} {
		execSQL("CREATE ROLE " + login + " LOGIN IN ROLE " + role)
		defer db.Exec(context.Background(), "DROP ROLE "+login)
	}

	fixtureCert := func(serial int64, usage x509.ExtKeyUsage, issuer *x509.Certificate, issuerKey *ecdsa.PrivateKey) (tls.Certificate, *x509.Certificate) {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(3 * time.Minute),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
		if issuer == nil {
			template.IsCA, template.BasicConstraintsValid = true, true
			template.KeyUsage |= x509.KeyUsageCertSign
			template.ExtKeyUsage = nil
			issuer, issuerKey = template, key
		}
		raw, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := x509.ParseCertificate(raw)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key}, parsed
	}
	caChain, ca := fixtureCert(1, 0, nil, nil)
	caPrivate := caChain.PrivateKey.(*ecdsa.PrivateKey)
	server, _ := fixtureCert(2, x509.ExtKeyUsageServerAuth, ca, caPrivate)
	audience, _ := fixtureCert(3, x509.ExtKeyUsageClientAuth, ca, caPrivate)
	replacement, _ := fixtureCert(4, x509.ExtKeyUsageClientAuth, ca, caPrivate)
	writerCert, _ := fixtureCert(5, x509.ExtKeyUsageClientAuth, ca, caPrivate)
	operatorCert, _ := fixtureCert(6, x509.ExtKeyUsageClientAuth, ca, caPrivate)
	operatorFingerprint := sha256.Sum256(operatorCert.Certificate[0])
	pin, otherPin, writerPin := sha256.Sum256(audience.Certificate[0]), sha256.Sum256(replacement.Certificate[0]), sha256.Sum256(writerCert.Certificate[0])
	installKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	installation := elliptic.Marshal(installKey.Curve, installKey.X, installKey.Y)
	var binding [208]byte
	binary.BigEndian.PutUint64(binding[:8], 1)
	for i, v := range []string{branch, board} {
		binaryID, err := hex.DecodeString(strings.ReplaceAll(v, "-", ""))
		if err != nil {
			t.Fatal(err)
		}
		copy(binding[8+16*i:], binaryID)
	}
	copy(binding[40:], registration)
	copy(binding[72:], measurement)
	copy(binding[104:], manifestDigest[:])
	binary.BigEndian.PutUint64(binding[160:], 8283)
	copy(binding[168:], common.HexToAddress(settlement).Bytes())
	copy(binding[188:], common.HexToAddress(account).Bytes())
	registered := ops.RentalRegistrationScope{Challenge: [32]byte(challenge), Board: [32]byte(boardID), Binding: binding,
		FreezeSigner: common.HexToAddress(signer), CollectorCert: [32]byte(collector), CollectorPeer: [7]byte(peer), ExpiresAt: uint64(expires.Unix())}
	confirmation := sha256.Sum256(append(registered.Body(), installation...))
	execSQL("INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	execSQL("INSERT INTO ledger.login_subject VALUES('rental_074_fixture',$1)", actor)
	execSQL("INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',now()+interval '3 minutes')", branch, actor)
	execSQL(`INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
		VALUES($1,$2,1,$3,$4,'qualified',$5,now()+interval '2 minutes',$6)`, branch, board, account, payout, registration, registeredAt)
	execSQL(`INSERT INTO ledger.rental_generation_source VALUES($1,$2,1,$3,$4,$5,'qualified',$6,$7,$8,$9,$10,$11,$12,
		$13,$14,now()+interval '2 minutes')`, branch, board, registration, account, payout, measurement, signer, settlement, manifestDigest[:], stream, streamEpoch, challenge, expires, writer)
	execSQL("INSERT INTO ledger.firmware_member VALUES($1,$2,'qualified',$3,$4,2,'0.0.2',false,NULL)", branch, board, boardID, installation)
	execSQL(`INSERT INTO ledger.firmware_campaign(branch_id,device_ref,campaign_ref,actor_id,release_digest,version,counter,measurement,challenge,challenge_until,state,proof_digest)
		VALUES($1,$2,$3,$4,$5,'0.0.2',2,$6,$7,$8,'verified',$9)`, branch, board, streamEpoch, actor, hex.EncodeToString(fill(0x45)), measurement, fill(0x46), expires, fill(0x47))
	execSQL(`INSERT INTO ledger.rental_collector_registration(branch_id,device_ref,generation,operator_ref,collector_certificate,collector_peer,first_nonce,
		registration_proof,authority_source,valid_until)
		SELECT $1,$2,1,$3,$4,$5,$6,$7,ledger.rental_generation_source_digest(source),$8
		FROM ledger.rental_generation_source source WHERE (source.branch_id,source.device_ref,source.generation)=($1,$2,1)`,
		branch, board, actor, collector, peer, first, confirmation[:], expires)
	execSQL(`INSERT INTO ledger.rental_authority_audience
		SELECT $1,$2,1,$3,$4,$5,ledger.rental_generation_source_digest(source),$6
		FROM ledger.rental_generation_source source WHERE (source.branch_id,source.device_ref,source.generation)=($1,$2,1)`,
		branch, board, reader, pin[:], writerPin[:], expires)
	readonly := config.Copy()
	readonly.User = reader
	roleConn, err := pgx.ConnectConfig(ctx, readonly)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"SELECT * FROM ledger.rental_authority_audience", "SELECT * FROM ledger.rental_generation_source",
		"UPDATE ledger.rental_authority_audience SET valid_until=now()", "SELECT * FROM ledger.rental_collector_registration"} {
		if _, err := roleConn.Exec(ctx, q); err == nil {
			t.Fatal("read-only runtime had direct table privilege")
		}
	}
	var writerAccess bool
	if err := roleConn.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p05_rental_evidence','MEMBER')`).Scan(&writerAccess); err != nil || writerAccess {
		t.Fatal("reader inherited evidence writer")
	}
	_ = roleConn.Close(context.Background())

	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Method string
			ID     json.RawMessage
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Method != "eth_chainId" {
			t.Error("authority lookup attempted an unrelated chain operation")
			http.Error(w, "unavailable", 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": input.ID, "result": "0x205b"})
	}))
	defer rpc.Close()
	password := hex.EncodeToString(fill(0x48))
	keyStore := keystore.NewKeyStore(filepath.Join(dir, "key"), keystore.LightScryptN, keystore.LightScryptP)
	operatorAccount, err := keyStore.ImportECDSA(operator, password)
	if err != nil {
		t.Fatal(err)
	}
	signingProgram := write("fixture-signer", []byte("#!/bin/sh\nexit 1\n"))
	keyDER, err := x509.MarshalPKCS8PrivateKey(server.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	cmd := exec.CommandContext(ctx, program)
	cmd.Env = []string{"TMPDIR=" + dir, "P05_HTTP_ADDR=" + address, "P05_COMMERCE_DATABASE_URL=" + config.ConnString() + " user=" + commerceRole,
		"P05_COMMERCE_CA=" + pemFile("trust.pem", "CERTIFICATE", ca.Raw), "P05_COMMERCE_CERT=" + pemFile("server.pem", "CERTIFICATE", server.Certificate[0]),
		"P05_COMMERCE_KEY=" + pemFile("server-key.pem", "PRIVATE KEY", keyDER),
		"P05_COMMERCE_GRANTS=" + settings("grants.json", map[string]service.Principal{hex.EncodeToString(operatorFingerprint[:]): {Tenant: tenant, Subject: actor, Role: "operator", ExpiresAt: expires.Unix()}}),
		"P05_COMMERCE_PROFILES=" + settings("profiles.json", map[string]any{branch: map[string]any{"chainId": 8283, "contract": settlement, "asset": signer,
			"beneficiary": payout, "signer": account, "merchantId": strings.Repeat("12", 32), "keyVersion": 1, "beneficiaryVersion": 1}}),
		"P05_QUOTE_SIGNER=" + signingProgram, "P05_QUOTE_SIGNER_SHA256=" + common.Bytes2Hex(sha256Bytes([]byte("#!/bin/sh\nexit 1\n"))),
		"P05_QUOTE_STORAGE_KEY_FILE=" + write("storage", []byte(strings.Repeat("11", 32))),
		"P05_QUOTE_COMMITMENT_KEY_FILE=" + write("commitment", []byte(strings.Repeat("22", 32))),
		"P05_V1_RENTAL_RPC=" + rpc.URL, "P05_V1_RENTAL_MANIFEST=" + write("manifest.json", manifest),
		"P05_V1_RENTAL_MANIFEST_SHA256=" + hex.EncodeToString(manifestDigest[:]), "P05_V1_RENTAL_KEYSTORE=" + operatorAccount.URL.Path,
		"P05_V1_RENTAL_SECRET_REF=file:" + write("password", []byte(password)),
		"P05_RENTAL_AUTHORITY_READ_DATABASE_URL=" + config.ConnString() + " user=" + reader,
	}
	var logs bytes.Buffer
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Log(logs.String())
		}
	}()
	trust := x509.NewCertPool()
	trust.AddCert(ca)
	clientFor := func(cert tls.Certificate) *http.Client {
		tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: trust, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}}
		t.Cleanup(tr.CloseIdleConnections)
		return &http.Client{Transport: tr, Timeout: 5 * time.Second}
	}
	for start := time.Now(); ; {
		c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			break
		}
		if time.Since(start) > 7*time.Second {
			t.Fatal("opsd did not open its normal TLS listener")
		}
		time.Sleep(25 * time.Millisecond)
	}
	good, bad := clientFor(audience), clientFor(replacement)
	url := "https://" + address + rentalv1.AuthorityPath
	scope := rentalv1.AuthorityRequest{Stream: stream, StreamGeneration: streamEpoch, Branch: branch, Board: board, Generation: "1",
		Challenge: common.Hash(challenge).Hex(), CallerFingerprint: hex.EncodeToString(writerPin[:])}
	ask := func(client *http.Client, input rentalv1.AuthorityRequest) (int, []byte, error) {
		payload, err := json.Marshal(input)
		if err != nil {
			return 0, nil, err
		}
		resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
		if resp.Header.Get("Cache-Control") != "no-store" {
			return 0, nil, errors.New("missing cache fence")
		}
		return resp.StatusCode, body, err
	}
	check := func(client *http.Client, input rentalv1.AuthorityRequest, want int) {
		t.Helper()
		status, body, err := ask(client, input)
		if err != nil || status != want {
			t.Fatalf("wanted %d, got %d (%v)", want, status, err)
		}
		if want != http.StatusOK {
			if bytes.Contains(body, registration) || bytes.Contains(body, []byte(signer)) {
				t.Fatal("denial disclosed a private scope")
			}
			return
		}
		var output rentalv1.AuthorityResponse
		if json.Unmarshal(body, &output) != nil || output.AuthorityRequest != input || output.Registration != common.Hash(registration).Hex() ||
			output.Measurement != common.Hash(measurement).Hex() || output.Deployment != common.Hash(manifestDigest).Hex() ||
			output.ChainID != 8283 || output.Settlement != settlement || output.Account != account || output.BoardSigner != signer ||
			output.Operator != strings.ToLower(operatorAddr.Hex()) || output.RegisteredAt != registeredAt.Unix() ||
			output.ValidUntil != expires.Unix() || output.ChallengeUntil != expires.Unix() || bytes.Contains(body, []byte("collector")) {
			t.Fatal("authority projection differed from the current registered scope")
		}
	}
	check(good, scope, 200)
	check(bad, scope, 404)
	check(clientFor(operatorCert), scope, 404)
	check(clientFor(writerCert), scope, 404)
	for _, mutate := range []func(*rentalv1.AuthorityRequest){
		func(p *rentalv1.AuthorityRequest) { p.Stream = streamEpoch },
		func(p *rentalv1.AuthorityRequest) { p.StreamGeneration = stream },
		func(p *rentalv1.AuthorityRequest) { p.Branch = board },
		func(p *rentalv1.AuthorityRequest) { p.Board = branch },
		func(p *rentalv1.AuthorityRequest) { p.Generation = "2" },
		func(p *rentalv1.AuthorityRequest) { p.CallerFingerprint = hex.EncodeToString(otherPin[:]) },
		func(p *rentalv1.AuthorityRequest) { p.Challenge = common.Hash(fill(0x49)).Hex() },
	} {
		changed := scope
		mutate(&changed)
		check(good, changed, 404)
	}
	execSQL("REVOKE nu54_p05_rental_evidence FROM " + writer)
	check(good, scope, 404)
	execSQL("GRANT nu54_p05_rental_evidence TO " + writer)
	execSQL("GRANT nu54_p05_rental_authority_read TO " + writer)
	check(good, scope, 404)
	execSQL("REVOKE nu54_p05_rental_authority_read FROM " + writer)
	execSQL("UPDATE ledger.firmware_member SET withdrawn=true WHERE branch_id=$1", branch)
	check(good, scope, 404)
	execSQL("UPDATE ledger.firmware_member SET withdrawn=false WHERE branch_id=$1", branch)
	check(good, scope, 200)
	execSQL("UPDATE ledger.rental_authority_audience SET valid_until=clock_timestamp() WHERE branch_id=$1", branch)
	check(good, scope, 404)
	execSQL("UPDATE ledger.rental_authority_audience SET valid_until=$2 WHERE branch_id=$1", branch, expires)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "UPDATE ledger.rental_authority_audience SET audience_certificate=$2 WHERE branch_id=$1", branch, otherPin[:]); err != nil {
		t.Fatal(err)
	}
	reply := make(chan int, 1)
	go func() { status, _, _ := ask(good, scope); reply <- status }()
	select {
	case <-reply:
		t.Fatal("read returned during audience replacement lock")
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if status := <-reply; status != 404 {
		t.Fatalf("stale audience survived the replacement: %d", status)
	}
	check(bad, scope, 200)
	execSQL("DELETE FROM ledger.finance_grant WHERE branch_id=$1 AND actor_id=$2", branch, actor)
	check(bad, scope, 404)
}

func sha256Bytes(input []byte) []byte {
	sum := sha256.Sum256(input)
	return sum[:]
}
