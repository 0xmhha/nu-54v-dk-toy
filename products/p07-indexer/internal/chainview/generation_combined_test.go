package chainview_test

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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
)

type combinedGenerationInputs struct {
	rpc, p07Origin, p07Pin, caFile                                                string
	p05Cert, p05Key, p07WriterCert, p07WriterKey                                  string
	operatorCertFile, operatorKeyFile                                             string
	audienceFP, collectorFP, operatorFP, renterFP, otherRenterFP, foreignRenterFP string
	roots                                                                         *x509.CertPool
	collector, operatorCert, renterCert, otherRenterCert, foreignRenterCert       tls.Certificate
	expected                                                                      rentalv1.AuthorityRequest
	profile                                                                       chainview.Profile
	projection, manifest                                                          []byte
	boardSigner                                                                   common.Address
	cleanupBoardKey                                                               *ecdsa.PrivateKey
	credential                                                                    *w12CredentialFixture
	operator                                                                      *ecdsa.PrivateKey
	terminal                                                                      tls.Certificate
	terminalFP                                                                    string
	challenge                                                                     common.Hash
	t0                                                                            uint64
}

type combinedGenerationOpsd struct {
	confirmInstalledCredentialRegistration                              func(*testing.T)
	ctx                                                                 context.Context
	db                                                                  *pgx.Conn
	origin, registrationPath, writePath, reviewPath                     string
	input                                                               combinedGenerationInputs
	actor                                                               string
	setIdentity                                                         func(string, string)
	enableV1Origins                                                     func()
	setInstalledRenter                                                  func(string)
	closeEnv                                                            []string
	operator, collector, renter, otherRenter, foreignRenter, v1Terminal *http.Client
	restart                                                             func(*testing.T)
	withdrawalFault                                                     *withdrawalPublishFault
	tokenMetadataGate                                                   *renterTokenMetadataGate
	closeFault                                                          *withdrawalPublishFault
}

const combinedV1Terminal = "64050674-1234-4f41-bc63-9927115e2d77"

const combinedRenter = "f89bf39d-485a-4cdf-8ed7-1135acc4e59d"

const combinedWriterRequest = "8f44d55c-c471-48d4-bbbc-6e65872c8346"

func startCombinedGenerationOpsd(t *testing.T, ctx context.Context, input combinedGenerationInputs) *combinedGenerationOpsd {
	t.Helper()
	dsn, binary := os.Getenv("P05_O09_COMBINED_DATABASE_URL"), os.Getenv("P05_O09_COMBINED_OPSD")
	config, err := pgx.ParseConfig(dsn)
	if dsn == "" || binary == "" || err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(config.Host, "/tmp/") || strings.HasPrefix(config.Host, "/private/tmp/")) {
		t.Fatal("combined fixture requires a separate disposable P05 PostgreSQL database")
	}
	db, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	s := &combinedGenerationOpsd{ctx: ctx, db: db, input: input, actor: "17e8d55c-c471-48d4-bbbc-6e65872c8346"}
	const tenant = "8744d55c-c471-48d4-bbbc-6e65872c8346"
	const commerceRole = "o09_combo_p05_commerce"
	const authorityRole = "o09_combo_p05_authority"
	const writerRole = "o09_combo_p05_writer"
	const readerRole = "o09_combo_p05_reader"
	const closeRole = "o09_combo_p05_close"
	if os.Getenv("P05_O09_PROTECTED_CLOSE_EXECUTOR") != "" || os.Getenv("P05_O09_SUPERVISED_CLOSE") != "" {
		s.sql(t, "CREATE ROLE "+closeRole+" LOGIN IN ROLE nu54_p05_rental_close")
	}
	for login, role := range map[string]string{commerceRole: "nu54_p05_commerce", authorityRole: "nu54_p05_rental_authority",
		writerRole: "nu54_p05_rental_evidence", readerRole: "nu54_p05_rental_authority_read"} {
		s.sql(t, "CREATE ROLE "+login+" LOGIN IN ROLE "+role)
	}
	binding, _, err := chainview.ParseGenerationProjection(input.projection)
	if err != nil || binding.Branch != input.expected.Branch || binding.Board != input.expected.Board ||
		sha256.Sum256(input.manifest) != binding.Deployment {
		t.Fatal("fixture manifest and protected journal differ")
	}
	hexFP := func(v string) []byte {
		t.Helper()
		d, err := hex.DecodeString(v)
		if err != nil || len(d) != 32 {
			t.Fatal("invalid fixture certificate")
		}
		return d
	}
	fill := func(x byte) []byte { return bytes.Repeat([]byte{x}, 32) }
	lower := func(a common.Address) string { return strings.ToLower(a.Hex()) }
	operatorAddr := crypto.PubkeyToAddress(input.operator.PublicKey)
	installKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	install := elliptic.Marshal(installKey.Curve, installKey.X, installKey.Y)
	registered, deadline := time.Unix(int64(input.t0), 0), time.Unix(int64(input.t0)+108, 0)
	if installedW12Credential() {
		deadline = time.Unix(int64(input.t0)+150, 0)
	}
	if time.Now().Before(registered) {
		select {
		case <-time.After(time.Until(registered)):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	s.sql(t, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", binding.Branch, tenant)
	s.sql(t, "INSERT INTO ledger.login_subject VALUES('o09_combined_fixture_operator',$1)", s.actor)
	s.sql(t, "INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',$3)", binding.Branch, s.actor, deadline.Add(time.Minute))
	s.sql(t, `INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
		VALUES($1,$2,$3,$4,$5,'fixture',$6,$7,$8)`, binding.Branch, binding.Board, binding.Generation, lower(binding.Account), lower(operatorAddr),
		binding.Registration[:], deadline.Add(time.Minute), registered)
	s.sql(t, `INSERT INTO ledger.rental_generation_source VALUES($1,$2,$3,$4,$5,$6,'fixture',$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		binding.Branch, binding.Board, binding.Generation, binding.Registration[:], lower(binding.Account), lower(operatorAddr), binding.Measurement[:],
		lower(input.boardSigner), lower(binding.Settlement), binding.Deployment[:], input.profile.StreamID, input.profile.GenerationID, input.challenge[:], deadline, writerRole, deadline)
	s.sql(t, "INSERT INTO ledger.firmware_member VALUES($1,$2,'fixture',$3,$4,2,'0.0.2',false,NULL)", binding.Branch, binding.Board, fill(0x45), install)
	s.sql(t, `INSERT INTO ledger.firmware_campaign(branch_id,device_ref,campaign_ref,actor_id,release_digest,version,counter,measurement,challenge,challenge_until,state,proof_digest)
		VALUES($1,$2,$3,$4,$5,'0.0.2',2,$6,$7,$8,'verified',$9)`,
		binding.Branch, binding.Board, input.profile.GenerationID, s.actor, hex.EncodeToString(fill(0x46)), binding.Measurement[:], fill(0x47), deadline, fill(0x48))
	s.sql(t, `INSERT INTO ledger.rental_collector_registration(branch_id,device_ref,generation,operator_ref,collector_certificate,
		collector_peer,first_nonce,authority_source,valid_until)
		SELECT $1,$2,$3,$4,$5,$6,$7,ledger.rental_generation_source_digest(p),$8 FROM ledger.rental_generation_source p
		WHERE (p.branch_id,p.device_ref,p.generation)=($1,$2,$3)`, binding.Branch, binding.Board, binding.Generation, s.actor,
		hexFP(input.collectorFP), []byte{1, 2, 3, 4, 5, 6, 7}, input.projection[212:244], deadline)
	s.sql(t, `INSERT INTO ledger.rental_authority_audience
		SELECT $1,$2,$3,$4,$5,$6,ledger.rental_generation_source_digest(p),$7 FROM ledger.rental_generation_source p
		WHERE (p.branch_id,p.device_ref,p.generation)=($1,$2,$3)`, binding.Branch, binding.Board, binding.Generation, readerRole,
		hexFP(input.audienceFP), hexFP(input.expected.CallerFingerprint), deadline)

	dir := t.TempDir()
	save := func(name string, raw []byte) string {
		t.Helper()
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	settings := func(name string, v any) string {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return save(name, raw)
	}
	private := hex.EncodeToString(fill(0x49))
	k := keystore.NewKeyStore(filepath.Join(dir, "keys"), keystore.LightScryptN, keystore.LightScryptP)
	account, err := k.ImportECDSA(input.operator, private)
	if err != nil {
		t.Fatal(err)
	}
	socket, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := socket.Addr().String()
	_ = socket.Close()
	s.origin = "https://" + address
	signProgram := []byte("#!/bin/sh\nexit 1\n")
	signHash := sha256.Sum256(signProgram)
	var opsRPC string
	opsRPC = input.rpc
	if os.Getenv("P05_O09_PROTECTED_WITHDRAWAL") == "recover" {
		s.withdrawalFault, opsRPC = interceptFirstRentalSend(t, input.rpc, "executeWithdrawal")
	}
	if os.Getenv("P05_O09_ROLLOVER_CLOSE") != "" {
		s.closeFault, opsRPC = interceptFirstRentalSend(t, input.rpc, "closeAccount")
	}
	if os.Getenv("P05_O09_RENTER_V1_ORIGIN") == "1" && os.Getenv("P05_O09_RENTER_ASSESSMENT") == "mixed" {
		s.tokenMetadataGate, opsRPC = blockRenterTokenMetadata(t, opsRPC)
	}
	program := exec.CommandContext(ctx, binary)
	if os.Getenv("P05_O09_RENTER_V1_ORIGIN") == "1" {
		s.sql(t, "INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES($1,$2,'terminal','pos',$3)",
			binding.Branch, combinedV1Terminal, deadline)
	}
	program.Env = []string{"TMPDIR=" + dir, "P05_HTTP_ADDR=" + address, "P05_COMMERCE_DATABASE_URL=" + dsn + " user=" + commerceRole,
		"P05_COMMERCE_CA=" + input.caFile, "P05_COMMERCE_CERT=" + input.p05Cert, "P05_COMMERCE_KEY=" + input.p05Key,
		"P05_COMMERCE_GRANTS=" + settings("grants", map[string]service.Principal{
			input.terminalFP:      {Tenant: tenant, Subject: combinedV1Terminal, Role: "terminal", ExpiresAt: deadline.Unix()},
			input.operatorFP:      {Tenant: tenant, Subject: s.actor, Role: "operator", ExpiresAt: deadline.Add(time.Minute).Unix()},
			input.renterFP:        {Tenant: tenant, Subject: combinedRenter, Role: "customer", ExpiresAt: deadline.Add(time.Minute).Unix()},
			input.otherRenterFP:   {Tenant: tenant, Subject: "094eab18-5ea6-4d32-b751-5ce067559873", Role: "customer", ExpiresAt: deadline.Add(time.Minute).Unix()},
			input.foreignRenterFP: {Tenant: "abee7f3d-485a-4cdf-8ed7-1135acc4e59d", Subject: combinedRenter, Role: "customer", ExpiresAt: deadline.Add(time.Minute).Unix()},
			input.collectorFP:     {Tenant: tenant, Subject: s.actor, Role: "rental-collector", ExpiresAt: deadline.Add(time.Minute).Unix()},
		}),
		"P05_COMMERCE_PROFILES=" + settings("profiles", map[string]any{binding.Branch: map[string]any{"chainId": 8283, "contract": binding.Settlement,
			"asset": input.profile.Asset, "beneficiary": operatorAddr, "signer": binding.Account, "merchantId": strings.Repeat("42", 32), "keyVersion": 1, "beneficiaryVersion": 1}}),
		"P05_QUOTE_SIGNER=" + save("signer", signProgram), "P05_QUOTE_SIGNER_SHA256=" + hex.EncodeToString(signHash[:]),
		"P05_QUOTE_STORAGE_KEY_FILE=" + save("storage", []byte(strings.Repeat("31", 32))),
		"P05_QUOTE_COMMITMENT_KEY_FILE=" + save("commitment", []byte(strings.Repeat("32", 32))),
		"P05_OPERATOR_ORIGIN=" + s.origin,
		"P05_V1_RENTAL_RPC=" + opsRPC, "P05_V1_RENTAL_MANIFEST=" + save("manifest", input.manifest),
		"P05_V1_RENTAL_MANIFEST_SHA256=" + input.profile.Manifest, "P05_V1_RENTAL_KEYSTORE=" + account.URL.Path,
		"P05_V1_RENTAL_SECRET_REF=file:" + save("password", []byte(private)),
		"P05_RENTAL_AUTHORITY_DATABASE_URL=" + dsn + " user=" + authorityRole,
		"P05_RENTAL_AUTHORITY_READ_DATABASE_URL=" + dsn + " user=" + readerRole,
		"P05_RENTAL_WRITER_DATABASE_URL=" + dsn + " user=" + writerRole,
		"P05_RENTAL_WRITER_P07_CONFIG=" + settings("writer", map[string]string{"Origin": input.p07Origin, "Roots": input.caFile,
			"Certificate": input.p07WriterCert, "KeyFile": input.p07WriterKey, "SPKI": input.p07Pin}),
	}
	if fixture := input.credential; fixture != nil {
		program.Env = append(program.Env, "P05_W12_TEST_FIXTURE="+settings("w12-credential", map[string]string{
			"Bridge": fixture.endpoint, "Token": fixture.token, "CA": input.caFile,
			"ServerCertificate": input.p05Cert, "ServerKey": input.p05Key,
			"AudienceCertificate": fixture.audienceCert, "AudienceKey": fixture.audienceKey,
			"Writer": input.expected.CallerFingerprint, "Measurement": hex.EncodeToString(binding.Measurement[:]),
			"SlotOpen": strconv.FormatBool(w12SlotOpenFixture()),
		}))
	}
	if dist := os.Getenv("P05_O09_CHROME_DIST"); dist != "" {
		program.Env = append(program.Env, "P05_CONSOLE_DIR="+dist)
	}
	if os.Getenv("P05_O09_PROTECTED_CLOSE_EXECUTOR") != "" {
		s.closeEnv = []string{"P05_TEST_CLOSE_ADMIN=" + dsn, "P05_TEST_CLOSE_DATABASE=" + dsn + " user=" + closeRole,
			"P05_TEST_CLOSE_RPC=" + input.rpc, "P05_TEST_CLOSE_TENANT=" + tenant, "P05_TEST_CLOSE_BRANCH=" + binding.Branch,
			"P05_TEST_CLOSE_DEVICE=" + binding.Board, "P05_TEST_CLOSE_ACTOR=" + s.actor, "P05_TEST_CLOSE_GENERATION=" + input.expected.Generation,
			"P05_TEST_CLOSE_P07=" + input.p07Origin, "P05_TEST_CLOSE_P07_PIN=" + input.p07Pin, "P05_TEST_CLOSE_CA=" + input.caFile,
			"P05_TEST_CLOSE_CERT=" + input.p07WriterCert, "P05_TEST_CLOSE_KEY=" + input.p07WriterKey,
			"P05_TEST_CLOSE_KEYSTORE=" + account.URL.Path, "P05_TEST_CLOSE_PASSWORD_FILE=" + save("close-password", []byte(private)), "P05_TEST_CLOSE_MANIFEST=" + save("close-manifest", input.manifest)}
	}
	if os.Getenv("P05_O09_SUPERVISED_CLOSE") != "" {
		program.Env = append(program.Env, "P05_RENTAL_PROTECTED_CLOSE_DATABASE_URL="+dsn+" user="+closeRole)
	}
	if os.Getenv("P05_O09_PROTECTED_WITHDRAWAL") != "" {
		program.Env = append(program.Env, "P05_RENTAL_PROTECTED_WITHDRAWAL=1")
	}
	if os.Getenv("P05_O09_WITHDRAWAL_RECONCILIATION") == "1" {
		const login = "o09_combo_p05_reconcile"
		s.sql(t, "CREATE ROLE "+login+" LOGIN IN ROLE nu54_p05_rental_reconcile")
		program.Env = append(program.Env, "P05_RENTAL_WITHDRAWAL_RECONCILER_DATABASE_URL="+dsn+" user="+login)
	}
	if os.Getenv("P05_O09_RENTER_V1_ORIGIN") == "1" {
		setting := "P05_RENTAL_V1_ORDER_TERMINALS=" + settings("v1issuers", map[string]any{input.terminalFP: map[string]any{
			"Tenant": tenant, "Branch": binding.Branch, "Terminal": combinedV1Terminal, "Merchant": lower(operatorAddr), "Payout": lower(operatorAddr), "ExpiresAt": deadline.Unix(),
		}})
		s.enableV1Origins = func() { program.Env = append(program.Env, setting) }
		if os.Getenv("P05_O09_RENTER_V1_UPGRADE") != "1" {
			s.enableV1Origins()
		}
	}
	if os.Getenv("P05_O09_RENTER_ASSESSMENT") != "" {
		program.Env = append(program.Env, "P05_RENTAL_RENTER_EVIDENCE=1")
	}
	launch := func(t *testing.T) *exec.Cmd {
		t.Helper()
		current := exec.CommandContext(ctx, binary)
		current.Env = program.Env
		logs, err := os.CreateTemp(dir, "opsd-*.log")
		if err != nil {
			t.Fatal(err)
		}
		current.Stderr = logs
		if err := current.Start(); err != nil {
			_ = logs.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = current.Process.Kill()
			_ = current.Wait()
			_ = logs.Close()
			if t.Failed() {
				body, _ := os.ReadFile(logs.Name())
				t.Log(string(body))
			}
		})
		for stop := time.Now().Add(8 * time.Second); ; {
			c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
			if err == nil {
				_ = c.Close()
				return current
			}
			if time.Now().After(stop) {
				t.Fatal("actual P05 did not start")
			}
			time.Sleep(30 * time.Millisecond)
		}
	}
	s.setIdentity = func(tenant, subject string) {
		settings("grants", map[string]service.Principal{input.operatorFP: {Tenant: tenant, Subject: subject, Role: "operator", ExpiresAt: deadline.Add(time.Minute).Unix()}})
	}
	s.setInstalledRenter = func(fingerprint string) {
		grants := map[string]service.Principal{
			input.operatorFP: {Tenant: tenant, Subject: s.actor, Role: "operator", ExpiresAt: deadline.Add(time.Minute).Unix()},
			fingerprint:      {Tenant: tenant, Subject: combinedRenter, Role: "customer", ExpiresAt: deadline.Add(time.Minute).Unix()},
		}
		if os.Getenv("P05_O09_RENTER_V1_ORIGIN") == "1" {
			grants[input.terminalFP] = service.Principal{Tenant: tenant, Subject: combinedV1Terminal, Role: "terminal", ExpiresAt: deadline.Unix()}
			grants[input.collectorFP] = service.Principal{Tenant: tenant, Subject: s.actor, Role: "rental-collector", ExpiresAt: deadline.Unix()}
		}
		settings("grants", grants)
	}
	active := launch(t)
	s.restart = func(t *testing.T) {
		t.Helper()
		_ = active.Process.Kill()
		_ = active.Wait()
		active = launch(t)
	}
	newClient := func(cert tls.Certificate) *http.Client {
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: input.roots, Certificates: []tls.Certificate{cert}}}
		t.Cleanup(transport.CloseIdleConnections)
		return &http.Client{Transport: transport, Timeout: 12 * time.Second}
	}
	s.operator, s.collector, s.renter, s.otherRenter, s.foreignRenter = newClient(input.operatorCert), newClient(input.collector), newClient(input.renterCert), newClient(input.otherRenterCert), newClient(input.foreignRenterCert)
	s.v1Terminal = newClient(input.terminal)
	scopePath := fmt.Sprintf("/%s/%s/%d", binding.Branch, binding.Board, binding.Generation)
	s.registrationPath = "/v2/private/rental-registration" + scopePath
	s.writePath = "/v2/private/rental-collector" + scopePath + "/record"
	s.reviewPath = fmt.Sprintf("/v2/operations/rental/%s/%s?generation=%d", binding.Branch, binding.Board, binding.Generation)
	// Use the real 072 confirmation handler and a generated installation key.
	// Firmware campaign qualification remains controlled test setup; no
	// application attempt installs, signs or rotates an actual device.
	confirmRegistration := func(t *testing.T) {
		t.Helper()
		raw := s.call(t, s.operator, http.MethodGet, s.registrationPath, "", nil, http.StatusOK)
		var enrollment struct{ InstallationRequest string }
		if json.Unmarshal(raw, &enrollment) != nil {
			t.Fatal("missing protected registration request")
		}
		statement, err := hex.DecodeString(enrollment.InstallationRequest)
		if err != nil || len(statement) != 375 || string(statement[:4]) != "N54G" ||
			!bytes.Equal(statement[68:276], input.projection[4:212]) {
			t.Fatal("registered P05 statement and genuine chain/board fixture differed")
		}
		digest := sha256.Sum256(statement)
		r, signature, err := ecdsa.Sign(rand.Reader, installKey, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		ack := append(statement, make([]byte, 64)...)
		r.FillBytes(ack[375:407])
		signature.FillBytes(ack[407:])
		s.call(t, s.operator, http.MethodPost, s.registrationPath, "application/octet-stream", ack, http.StatusOK)
	}
	if installedW12Credential() {
		s.confirmInstalledCredentialRegistration = confirmRegistration
	} else {
		confirmRegistration(t)
	}
	return s
}

func (s *combinedGenerationOpsd) sql(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := s.db.Exec(s.ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func (s *combinedGenerationOpsd) call(t *testing.T, client *http.Client, method, path, kind string, body []byte, want int) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(s.ctx, method, s.origin+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", kind)
	req.Header.Set("Origin", s.origin)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil || resp.StatusCode != want {
		if s.input.credential != nil {
			t.Fatalf("actual W12 P05 request: wanted %d got %d", want, resp.StatusCode)
		}
		t.Fatalf("actual P05 %s: wanted %d got %d (%v): %.400s", method, want, resp.StatusCode, err, raw)
	}
	return raw
}

func combinedProof(t *testing.T, key string, projection, receipt []byte) []byte {
	t.Helper()
	body, err := json.Marshal(struct {
		Key        string `json:"requestKey"`
		Projection []byte `json:"projection"`
		Receipt    []byte `json:"freezeReceipt"`
	}{key, projection, receipt})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func (s *combinedGenerationOpsd) recordAndReview(t *testing.T, projection, receipt []byte) {
	t.Helper()
	s.recordWithExpectedPending(t, projection, receipt, 1)
	s.assertHeldReview(t)
}

func (s *combinedGenerationOpsd) recordWithExpectedPending(t *testing.T, projection, receipt []byte, want int) {
	t.Helper()
	original := combinedProof(t, combinedWriterRequest, projection, receipt)
	s.call(t, s.collector, http.MethodPost, s.writePath, "application/json", original, http.StatusOK)
	s.call(t, s.collector, http.MethodPost, s.writePath, "application/json", original, http.StatusOK)
	var count, pending int
	err := s.db.QueryRow(s.ctx, `SELECT count(*),coalesce(sum(pending_payments),0)
		FROM ledger.rental_generation_authorized_proof WHERE branch_id=$1`, s.input.expected.Branch).Scan(&count, &pending)
	if err != nil || count != 1 || pending != want {
		t.Fatalf("actual P05 did not retain one proof and the expected pending count: %d %d %v", count, pending, err)
	}
}

func (s *combinedGenerationOpsd) assertHeldReview(t *testing.T) {
	t.Helper()
	body := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
	var review struct {
		ReturnReview struct {
			ReviewRef         string
			GenerationPending struct {
				State        string
				PendingCount *int
			}
		}
	}
	if json.Unmarshal(body, &review) != nil || review.ReturnReview.GenerationPending.State != "unknown" ||
		review.ReturnReview.GenerationPending.PendingCount != nil || review.ReturnReview.ReviewRef != "" {
		t.Fatal("guarded actual operations review exposed prematurely admitted evidence")
	}
}

func (s *combinedGenerationOpsd) revoke(t *testing.T) {
	t.Helper()
	s.sql(t, "DELETE FROM ledger.rental_authority_audience WHERE branch_id=$1", s.input.expected.Branch)
}

func (s *combinedGenerationOpsd) verifyRevocation(t *testing.T, projection, receipt []byte) {
	t.Helper()
	s.call(t, s.collector, http.MethodPost, s.writePath, "application/json",
		combinedProof(t, "9f44d55c-c471-48d4-bbbc-6e65872c8346", projection, receipt), http.StatusConflict)
	var count int
	if err := s.db.QueryRow(s.ctx, "SELECT count(*) FROM ledger.rental_proof_submission WHERE branch_id=$1",
		s.input.expected.Branch).Scan(&count); err != nil || count != 1 {
		t.Fatal("revoked authority created a new P05 proof", err)
	}
	s.assertHeldReview(t)
}

func (s *combinedGenerationOpsd) executeProtectedClose(t *testing.T) {
	t.Helper()
	command := exec.CommandContext(s.ctx, os.Getenv("P05_O09_PROTECTED_CLOSE_EXECUTOR"), "-test.run", "^TestProtectedCloseActualDaemons$", "-test.v")
	command.Env = append(os.Environ(), s.closeEnv...)
	body, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("staged production executor: %v\n%s", err, body)
	}
	if !bytes.Contains(body, []byte("\n--- PASS: TestProtectedCloseActualDaemons (")) ||
		!strings.HasSuffix(strings.TrimSpace(string(body)), "\nPASS") {
		t.Fatalf("required protected close child did not pass:\n%s", body)
	}
	t.Logf("%s", body)
}

func (s *combinedGenerationOpsd) supervisedClose(t *testing.T, projection, receipt []byte) {
	t.Helper()
	s.recordWithExpectedPending(t, projection, receipt, 0)
	var review struct {
		ReturnReview struct {
			ReviewRef         string `json:"reviewRef"`
			GenerationPending struct {
				State        string
				PendingCount *int
			}
		}
	}
	raw := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
	if json.Unmarshal(raw, &review) != nil || review.ReturnReview.ReviewRef == "" ||
		review.ReturnReview.GenerationPending.State != "verified" || review.ReturnReview.GenerationPending.PendingCount == nil ||
		*review.ReturnReview.GenerationPending.PendingCount != 0 {
		t.Fatal("opt-in operations GET did not expose the verified same-generation close review")
	}
	var untouched int
	if err := s.db.QueryRow(s.ctx, "SELECT count(*) FROM ledger.rental_close_candidate").Scan(&untouched); err != nil || untouched != 0 {
		t.Fatal("GET changed candidate count", err)
	}
	anchor := review.ReturnReview.ReviewRef
	path, query, _ := strings.Cut(s.reviewPath, "?")
	closePath := path + "/close?" + query
	key := "e39c66cd-adb5-4534-9100-a2018f7bbf05"
	wire := func(ref, key string) []byte {
		body, _ := json.Marshal(map[string]string{"reviewRef": ref, "requestKey": key})
		return body
	}
	s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(strings.Repeat("a", 64), key), http.StatusConflict)
	var originalAuthorityDeadline time.Time
	if err := s.db.QueryRow(s.ctx, "SELECT valid_until FROM ledger.rental_authority_audience WHERE branch_id=$1", s.input.expected.Branch).Scan(&originalAuthorityDeadline); err != nil {
		t.Fatal(err)
	}
	s.sql(t, "UPDATE ledger.rental_authority_audience SET valid_until=$2 WHERE branch_id=$1", s.input.expected.Branch, time.Now().Add(-time.Second))
	s.assertHeldReview(t)
	s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(anchor, key), http.StatusConflict)
	var candidates int
	if err := s.db.QueryRow(s.ctx, "SELECT count(*) FROM ledger.rental_close_candidate WHERE branch_id=$1", s.input.expected.Branch).Scan(&candidates); err != nil || candidates != 0 {
		t.Fatalf("expired authority materialized a close candidate: %d %v", candidates, err)
	}
	s.sql(t, "UPDATE ledger.rental_authority_audience SET valid_until=$2 WHERE branch_id=$1", s.input.expected.Branch, originalAuthorityDeadline)
	// Changing a still-valid audience deadline preserves the old 070 preview,
	// but invalidates the operator's old protected approval before admission.
	legacyReview := func() string {
		t.Helper()
		var out string
		const tenant = "8744d55c-c471-48d4-bbbc-6e65872c8346"
		err := s.db.QueryRow(s.ctx, `SELECT ledger.rental_generation_authorized_review($1,$2,$3,$4,$5)->>'returnReviewRef'`,
			tenant, s.input.expected.Branch, s.actor, s.input.expected.Board, s.input.expected.Generation).Scan(&out)
		if err != nil || out == "" {
			t.Fatalf("missing legacy review: %v", err)
		}
		return out
	}
	legacy := legacyReview()
	s.sql(t, "UPDATE ledger.rental_authority_audience SET valid_until=$2 WHERE branch_id=$1", s.input.expected.Branch, originalAuthorityDeadline.Add(time.Minute))
	if legacyReview() != legacy {
		t.Fatal("audience-only change unexpectedly changed the old preview")
	}
	s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(anchor, key), http.StatusConflict)
	newRaw := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
	if json.Unmarshal(newRaw, &review) != nil || review.ReturnReview.ReviewRef == "" || review.ReturnReview.ReviewRef == anchor {
		t.Fatal("protected review did not change with a still-valid authority basis")
	}
	if err := s.db.QueryRow(s.ctx, "SELECT count(*) FROM ledger.rental_close_candidate").Scan(&untouched); err != nil || untouched != 0 {
		t.Fatal("stale basis created a protected candidate", err)
	}
	anchor = review.ReturnReview.ReviewRef
	first := s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(anchor, key), http.StatusAccepted)
	var strict struct {
		Operations []struct {
			Action, RequestKey, Status, ProtectedReviewRef string
			Evidence                                       struct{ TransactionHash string }
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(first))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&strict); err != nil || len(strict.Operations) != 1 || strict.Operations[0].RequestKey != key || strict.Operations[0].ProtectedReviewRef != anchor || strict.Operations[0].Action != "close" || len(strict.Operations[0].Evidence.TransactionHash) != 66 {
		t.Fatal("public close response schema changed", err)
	}
	var hash []byte
	var phase string
	if err := s.db.QueryRow(s.ctx, "SELECT tx_hash,state FROM ledger.rental_close_candidate WHERE branch_id=$1", s.input.expected.Branch).Scan(&hash, &phase); err != nil || len(hash) != 32 || phase != "submitted" {
		t.Fatalf("operation did not retain one submitted original: %s %v", phase, err)
	}
	if !bytes.Contains(first, []byte(common.Hash(hash).Hex())) {
		t.Fatal("operator response did not identify original submitted transaction")
	}
	s.restart(t)
	s.revoke(t)
	s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(anchor, "073c66cd-adb5-4534-9100-a2018f7bbf05"), http.StatusConflict)
	s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(anchor, key), http.StatusAccepted)
	client, err := ethclient.DialContext(s.ctx, s.input.rpc)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Client().CallContext(s.ctx, nil, "anvil_mine", 4); err != nil {
		t.Fatal(err)
	}
	final := s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(anchor, key), http.StatusAccepted)
	if !bytes.Contains(final, []byte(common.Hash(hash).Hex())) {
		t.Fatal("same key did not re-query the original final hash")
	}
	var sealed []byte
	if err := s.db.QueryRow(s.ctx, "SELECT state,sealed_transaction FROM ledger.rental_close_candidate WHERE branch_id=$1", s.input.expected.Branch).Scan(&phase, &sealed); err != nil || phase != "confirmed" || sealed != nil {
		t.Fatalf("finalized exact HTTP close not retained: %s %v", phase, err)
	}
	binding, _, err := chainview.ParseGenerationProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	settle, err := settlementext.NewSettlementext(binding.Settlement, client)
	if err != nil {
		t.Fatal(err)
	}
	logs, err := settle.FilterAccountClosed(&bind.FilterOpts{Start: uint64(s.input.profile.StartHeight), Context: s.ctx}, []common.Address{binding.Account})
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	if !logs.Next() || logs.Event.Raw.TxHash != common.Hash(hash) || logs.Next() {
		t.Fatal("expected exactly one canonical original close")
	}
	const tenant = "8744d55c-c471-48d4-bbbc-6e65872c8346"
	const other = "abcdef12-0000-4000-8000-000000000090"
	const alien = "abcdef12-0000-4000-8000-000000000091"
	raw = s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
	if !bytes.Contains(raw, []byte("payment-hold")) {
		t.Fatal("post-close hold missing")
	}
	s.sql(t, "INSERT INTO ledger.login_subject VALUES('audit_peer',$1)", other)
	s.sql(t, "INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',clock_timestamp()+interval '1 hour')", s.input.expected.Branch, other)
	s.setIdentity(tenant, other)
	s.restart(t)
	peer := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
	if bytes.Contains(peer, []byte(key)) || bytes.Contains(peer, []byte(anchor)) || !bytes.Contains(peer, []byte(common.Hash(hash).Hex())) {
		t.Fatal("peer evidence boundary")
	}
	s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(anchor, key), http.StatusConflict)
	s.setIdentity(alien, s.actor)
	s.restart(t)
	s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusNotFound)
	s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(anchor, key), http.StatusNotFound)
	t.Log("read-only preview, exact POST schema, confirmed payment hold, peer and tenant HTTP isolation passed")

}
