package chainview_test

import (
	"bytes"
	"context"
	stdcrypto "crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/evidencev2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
)

func verifyGenerationDaemon(t *testing.T, ctx context.Context, db *pgx.Conn, dsn, rpc string, profile chainview.Profile,
	projection []byte, boardKey *ecdsa.PrivateKey, challenge common.Hash, t0 uint64, manifest []byte, fixtureOperator *ecdsa.PrivateKey, fixtureSignature []byte) {
	t.Helper()
	binary := os.Getenv("P07_GENERATION_TEST_BINARY")
	if binary == "" {
		t.Fatal("dedicated v1 fixture requires the prebuilt indexer daemon")
	}
	if _, err := db.Exec(ctx, "CREATE ROLE rental_o09_reader_fixture LOGIN IN ROLE nu54_p07_rental_reader"); err != nil {
		t.Fatal(err)
	}
	readerDSN := dsn + " user=rental_o09_reader_fixture"
	role, err := pgx.Connect(ctx, readerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer role.Close(context.Background())
	if _, err := role.Exec(ctx, `SELECT tx_hash FROM chainview.observed_event LIMIT 1`); err == nil {
		t.Fatal("rental credential could read unrelated commerce event data")
	}
	if _, err := role.Exec(ctx, `UPDATE chainview.generation SET state='frozen' WHERE false`); err == nil {
		t.Fatal("rental credential could mutate chainview state")
	}

	dir := t.TempDir()
	save := func(name string, data []byte) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	jsonBytes := func(v any) []byte {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	now := time.Now()
	newTLSKey := func() stdcrypto.Signer {
		t.Helper()
		if os.Getenv("P05_O09_W12_INSTALLED_RUNNER") != "" {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			return key
		}
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	caKey := newTLSKey()
	ca := x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Local rental fixture CA"}, IsCA: true, BasicConstraintsValid: true,
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageCertSign}
	caRaw, err := x509.CreateCertificate(rand.Reader, &ca, &ca, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	caParsed, err := x509.ParseCertificate(caRaw)
	if err != nil {
		t.Fatal(err)
	}
	pemBlock := func(kind string, raw []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: raw}) }
	caFile := save("ca.pem", pemBlock("CERTIFICATE", caRaw))
	pool := x509.NewCertPool()
	pool.AddCert(caParsed)
	leaf := func(name string, serial int64, usage x509.ExtKeyUsage) (tls.Certificate, string, string, string) {
		t.Helper()
		key := newTLSKey()
		template := x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, ExtKeyUsage: []x509.ExtKeyUsage{usage}, KeyUsage: x509.KeyUsageDigitalSignature}
		raw, err := x509.CreateCertificate(rand.Reader, &template, caParsed, key.Public(), caKey)
		if err != nil {
			t.Fatal(err)
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		fingerprint := sha256.Sum256(raw)
		pair := tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key}
		return pair, save(name+".pem", pemBlock("CERTIFICATE", raw)), save(name+".key", pemBlock("PRIVATE KEY", der)), hex.EncodeToString(fingerprint[:])
	}
	_, serverCert, serverKey, _ := leaf("p07", 2, x509.ExtKeyUsageServerAuth)
	writer, writerCert, writerKey, writerFP := leaf("writer", 3, x509.ExtKeyUsageClientAuth)
	authorityServer, authorityServerCert, authorityServerKey, _ := leaf("authority", 4, x509.ExtKeyUsageServerAuth)
	_, authorityCert, authorityKey, authorityFP := leaf("p07-reader", 5, x509.ExtKeyUsageClientAuth)
	parsedAuthority, err := x509.ParseCertificate(authorityServer.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(parsedAuthority.RawSubjectPublicKeyInfo)
	binding, _, err := chainview.ParseGenerationProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	expected := rentalv1.AuthorityRequest{Stream: stream, StreamGeneration: active, Branch: branch, Board: board,
		Generation: strconv.FormatUint(binding.Generation, 10), Challenge: challenge.Hex(), CallerFingerprint: writerFP}
	var mode, reads atomic.Int32
	authority := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fingerprint := sha256.Sum256(r.TLS.PeerCertificates[0].Raw)
		var lookup rentalv1.AuthorityRequest
		if r.Method != http.MethodPost || r.URL.Path != rentalv1.AuthorityPath || hex.EncodeToString(fingerprint[:]) != authorityFP ||
			json.NewDecoder(r.Body).Decode(&lookup) != nil || lookup != expected {
			http.NotFound(w, r)
			return
		}
		current := mode.Load()
		read := reads.Add(1)
		if read == 2 && current == 1 {
			http.NotFound(w, r)
			return
		}
		if read == 2 && current == 4 {
			if _, err := db.Exec(ctx, `UPDATE chainview.block SET canonical=false
				WHERE generation_id=$1 AND height=$2`, active, profile.StartHeight+1); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		until := int64(t0) + 80
		if current == 2 {
			until = time.Now().Unix() - 1
		}
		reply := rentalv1.AuthorityResponse{AuthorityRequest: lookup, Registration: binding.Registration.Hex(), Measurement: binding.Measurement.Hex(),
			Deployment: binding.Deployment.Hex(), ChainID: 8283, Settlement: strings.ToLower(binding.Settlement.Hex()),
			Account: strings.ToLower(binding.Account.Hex()), BoardSigner: strings.ToLower(crypto.PubkeyToAddress(boardKey.PublicKey).Hex()),
			Operator: common.Address{19: 71}.Hex(), RegisteredAt: int64(t0), ChallengeUntil: int64(t0) + 110, ValidUntil: until}
		if current == 3 {
			reply.StreamGeneration = branch
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	approvalPub, approvalKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profileBytes := jsonBytes(profile)
	signature := ed25519.Sign(approvalKey, append([]byte("NU54-chainview-profile-v1\x00"), profileBytes...))
	grantPath := save("grant.json", jsonBytes(map[string]evidencev2.GenerationGrant{writerFP: {Stream: stream, Branch: branch, Board: board, ExpiresAt: time.Now().Add(time.Hour)}}))
	socket, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := socket.Addr().String()
	socket.Close()
	var combined *combinedGenerationOpsd
	var authorityOrigin string
	if os.Getenv("P05_O09_COMBINED_OPSD") != "" {
		collector, _, _, collectorFP := leaf("p05-collector", 6, x509.ExtKeyUsageClientAuth)
		foreignRenterCert, _, _, foreignRenterFP := leaf("p05-foreign-renter", 11, x509.ExtKeyUsageClientAuth)
		otherRenterCert, _, _, otherRenterFP := leaf("p05-another-renter", 10, x509.ExtKeyUsageClientAuth)
		renterCert, _, _, renterFP := leaf("p05-renter", 9, x509.ExtKeyUsageClientAuth)
		terminalCert, _, _, terminalFP := leaf("p05-v1-terminal", 12, x509.ExtKeyUsageClientAuth)
		operatorCert, operatorCertFile, operatorKeyFile, operatorFP := leaf("p05-operator", 7, x509.ExtKeyUsageClientAuth)
		p07Server, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			t.Fatal(err)
		}
		p07Leaf, err := x509.ParseCertificate(p07Server.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		p07Pin := sha256.Sum256(p07Leaf.RawSubjectPublicKeyInfo)
		if os.Getenv("P05_O09_RENTER_V1_UPGRADE") == "1" {
			startRenterV1UpgradeDB(t, ctx)
		}
		var credential *w12CredentialFixture
		if os.Getenv("P05_O09_W12_CREDENTIAL") == "1" {
			credential = startW12CredentialFixture(t, projection, t0, fixtureOperator, boardKey, authorityCert, authorityKey)
		}
		combined = startCombinedGenerationOpsd(t, ctx, combinedGenerationInputs{
			rpc: rpc, p07Origin: "https://" + addr, p07Pin: hex.EncodeToString(p07Pin[:]), caFile: caFile, roots: pool,
			p05Cert: authorityServerCert, p05Key: authorityServerKey, p07WriterCert: writerCert, p07WriterKey: writerKey,
			audienceFP: authorityFP, collectorFP: collectorFP, operatorFP: operatorFP, renterFP: renterFP, renterCert: renterCert, otherRenterFP: otherRenterFP, otherRenterCert: otherRenterCert, foreignRenterFP: foreignRenterFP, foreignRenterCert: foreignRenterCert, collector: collector, operatorCert: operatorCert, operatorCertFile: operatorCertFile, operatorKeyFile: operatorKeyFile,
			expected: expected, profile: profile, projection: projection, manifest: manifest, boardSigner: crypto.PubkeyToAddress(boardKey.PublicKey), cleanupBoardKey: boardKey,
			operator: fixtureOperator, credential: credential, terminal: terminalCert, terminalFP: terminalFP, challenge: challenge, t0: t0,
		})
		authorityOrigin = combined.origin
	} else {
		authority.TLS = evidencev2.TLSConfig(pool)
		authority.TLS.Certificates = []tls.Certificate{authorityServer}
		authority.StartTLS()
		defer authority.Close()
		authorityOrigin = authority.URL
	}
	env := []string{"P07_RENTAL_DATABASE_URL=" + readerDSN, "P07_RENTAL_RPC=" + rpc, "P07_RENTAL_PROFILE=" + save("profile.json", profileBytes),
		"P07_RENTAL_PROFILE_KEY=" + hex.EncodeToString(approvalPub), "P07_RENTAL_PROFILE_SIGNATURE=" + hex.EncodeToString(signature),
		"P07_RENTAL_HTTP_ADDR=" + addr, "P07_RENTAL_CA=" + caFile, "P07_RENTAL_CERT=" + serverCert, "P07_RENTAL_KEY=" + serverKey,
		"P07_RENTAL_GRANTS=" + grantPath, "P07_RENTAL_AUTHORITY_ORIGIN=" + authorityOrigin, "P07_RENTAL_AUTHORITY_CA=" + caFile,
		"P07_RENTAL_AUTHORITY_CERT=" + authorityCert, "P07_RENTAL_AUTHORITY_KEY=" + authorityKey, "P07_RENTAL_AUTHORITY_SPKI=" + hex.EncodeToString(pin[:])}
	{
		wrong := exec.CommandContext(ctx, binary, "rental-generation")
		wrong.Env = append(append([]string(nil), env...), "P07_RENTAL_PROFILE_SIGNATURE="+strings.Repeat("0", 128))
		if out, err := wrong.CombinedOutput(); err == nil || !bytes.Contains(out, []byte("requires an approved v1 profile")) {
			t.Fatalf("unapproved profile did not fail closed: %s %v", out, err)
		}
	}
	logFile, err := os.Create(filepath.Join(dir, "daemon.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd := exec.CommandContext(ctx, binary, "rental-generation")
	cmd.Env, cmd.Stdout, cmd.Stderr = env, logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_ = cmd.Wait()
		if t.Failed() {
			out, _ := os.ReadFile(logFile.Name())
			t.Log(string(out))
		}
	}()
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{writer}}}
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second}
	defer client.CloseIdleConnections()
	endpoint := "https://" + addr
	for attempts := 0; ; attempts++ {
		res, err := client.Get(endpoint + "/v2/evidence/read")
		if err == nil {
			res.Body.Close()
			if res.StatusCode != http.StatusNotFound {
				t.Fatal("dedicated listener unexpectedly exposed C7")
			}
			break
		}
		if attempts == 20 {
			t.Fatalf("dedicated listener did not start: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if until := time.Until(time.Unix(int64(t0)+24, 0)); until > 0 {
		select {
		case <-time.After(until):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	receipt := signedFreeze(t, boardKey, projection, challenge, t0+15, uint64(time.Now().Unix()+55))
	details := os.Getenv("P05_O09_RENTER_PENDING_ENTRIES") == "1"
	request := jsonBytes(rentalv1.Request{Projection: projection, Receipt: receipt, IncludePendingPaymentEntries: details})
	post := func(status int) (chainview.GenerationAssessment, int32) {
		t.Helper()
		reads.Store(0)
		res, err := client.Post(endpoint+rentalv1.ReconcilePath, "application/json", bytes.NewReader(request))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != status {
			t.Fatalf("expected status %d, got %d", status, res.StatusCode)
		}
		if status != http.StatusOK {
			return chainview.GenerationAssessment{}, reads.Load()
		}
		var reply chainview.GenerationAssessment
		if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
			t.Fatal(err)
		}
		return reply, reads.Load()
	}
	scopeDigest, err := chainview.GenerationBindingDigest(binding)
	if err != nil {
		t.Fatal(err)
	}
	closeRequest := func(result chainview.GenerationAssessment) rentalv1.CloseAnchorRequest {
		t.Helper()
		if result.AnchorHeight == nil {
			t.Fatal("assessment has no established checkpoint")
		}
		return rentalv1.CloseAnchorRequest{Stream: stream, StreamGeneration: active, Branch: branch, Board: board,
			Generation: expected.Generation, Challenge: challenge.Hex(), RegistrationBinding: scopeDigest.Hex(),
			Previous: rentalv1.Anchor{Height: *result.AnchorHeight, Hash: result.AnchorHash}, ReviewDigest: result.Digest, ValidUntil: result.ValidUntil}
	}
	confirm := func(request rentalv1.CloseAnchorRequest, status int) rentalv1.CloseAnchorConfirmation {
		t.Helper()
		reads.Store(0)
		response, err := client.Post(endpoint+rentalv1.CloseAnchorPath, "application/json", bytes.NewReader(jsonBytes(request)))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != status {
			t.Fatalf("close-anchor expected HTTP %d, got %d", status, response.StatusCode)
		}
		var confirmation rentalv1.CloseAnchorConfirmation
		if status == http.StatusOK && json.NewDecoder(response.Body).Decode(&confirmation) != nil {
			t.Fatal("invalid close-anchor confirmation")
		}
		return confirmation
	}
	if runner := os.Getenv("P05_O09_W12_INSTALLED_RUNNER"); combined != nil &&
		(runner != "" || combined.input.credential != nil) {
		linked := os.Getenv("P05_O09_RENTER_V1_ORIGIN") == "1"
		name := "InstalledRenterReturnHandover"
		var record func(*testing.T)
		if linked {
			if os.Getenv("P05_O09_RENTER_ASSESSMENT") != "mixed" {
				t.Fatal("installed origin fixture requires mixed v1 obligations")
			}
			name = "InstalledRenterOriginalPayment"
			record = func(t *testing.T) {
				expiry := time.Now().Add(48 * time.Second).Unix()
				latest := int64(t0) + 105
				if installedW12Credential() {
					latest = int64(t0) + 147
				}
				if expiry > latest {
					expiry = latest
				}
				receipt := signedFreeze(t, boardKey, projection, challenge, t0+15, uint64(expiry))
				combined.recordWithExpectedPending(t, projection, receipt, 1)
			}
		}
		if runner == "" {
			t.Run("W12CredentialLiveFreezeHost", func(t *testing.T) {
				combined.verifyW12CredentialHost(t, fixtureSignature, record)
			})
			return
		}
		if combined.input.credential != nil {
			name = "InstalledRenterCredentialAndOriginalPayment"
			if w12SlotOpenFixture() {
				name = "InstalledRenterSlotOpenAndOriginalPayment"
			}
		}
		t.Run(name, func(t *testing.T) {
			combined.verifyInstalledRenter(t, runner, authorityServer, caParsed, caKey, fixtureSignature, record)
		})
		return
	}
	if combined != nil && os.Getenv("P05_O09_RENTER_V1_UPGRADE") == "1" {
		t.Run("Upgrade087To088RenterV1Origin", func(t *testing.T) { combined.verifyRenterV1Upgrade(t, fixtureSignature, receipt) })
		return
	}
	if combined != nil && os.Getenv("P05_O09_RENTER_V1_ORIGIN") == "1" {
		t.Run("RenterV1OrderOriginHTTP", func(t *testing.T) { combined.verifyRenterV1Origin(t, fixtureSignature, receipt) })
		return
	}
	if mode := os.Getenv("P05_O09_RENTER_ASSESSMENT"); combined != nil && mode != "" {
		var payments, limits int
		var name string
		switch mode {
		case "positive":
			payments, name = 1, "RenterReturnPendingAssessmentHTTP"
		case "zero":
			name = "RenterReturnVerifiedZeroHTTP"
		case "limit":
			limits, name = 1, "RenterReturnLimitOnlyHTTP"
		case "mixed":
			payments, limits, name = 1, 1, "RenterReturnMixedPendingHTTP"
		default:
			t.Fatal("unknown renter assessment fixture mode")
		}
		t.Run(name, func(t *testing.T) {
			reply, _ := post(http.StatusOK)
			if reply.PendingCount == nil || *reply.PendingCount != payments+limits {
				t.Fatalf("genuine assessed count missing: %+v", reply)
			}
			if details {
				var want uint8
				if payments == 1 {
					want = 1 << 3
				}
				if reply.PendingPaymentEntries == nil || *reply.PendingPaymentEntries != want {
					t.Fatalf("unexpected pending payment entry indexes: %v", reply.PendingPaymentEntries)
				}
			} else if reply.PendingPaymentEntries != nil {
				t.Fatal("default response disclosed pending payment entry indexes")
			}
			combined.verifyRenterAssessment(t, dsn, projection, receipt, payments, limits)
		})
		return
	}
	if combined != nil && os.Getenv("P05_O09_RENTER_RETURN") == "1" {
		t.Run("RenterReturnHandoverHTTP", func(t *testing.T) { combined.verifyRenterReturn(t) })
		return
	}
	if combined != nil && os.Getenv("P05_O09_PROTECTED_WITHDRAWAL") != "" {
		caseName := "ProtectedWithdrawalHTTP"
		switch os.Getenv("P05_O09_PROTECTED_WITHDRAWAL") {
		case "zero":
			caseName = "ProtectedZeroWithdrawalHTTP"
		case "recover":
			caseName = "ProtectedWithdrawalRestartHTTP"
			switch os.Getenv("P05_O09_RETRY_WITHDRAWAL") {
			case "1":
				caseName = "ProtectedWithdrawalTerminalRetryHTTP"
			case "third-party":
				caseName = "ProtectedWithdrawalRevertedAlreadyPaidHTTP"
			}
		}
		t.Run(caseName, func(t *testing.T) {
			reply, _ := post(http.StatusOK)
			if reply.Status != "verified" || reply.PendingCount == nil || *reply.PendingCount != 0 {
				t.Fatal("genuine 073 zero proof missing")
			}
			combined.supervisedWithdrawal(t, dsn, projection, receipt)
		})
		return
	}
	if combined != nil && os.Getenv("P05_O09_ROLLOVER_CLOSE") != "" {
		name := "ProtectedCloseLegacyRollover"
		if os.Getenv("P05_O09_ROLLOVER_CLOSE") == "renewed" {
			name = "ProtectedCloseLegacyReauthorization"
		}
		t.Run(name, func(t *testing.T) {
			reply, _ := post(http.StatusOK)
			if reply.Status != "verified" || reply.PendingCount == nil || *reply.PendingCount != 0 {
				t.Fatal("genuine 073 zero proof missing")
			}
			combined.rolloverClose(t, projection, receipt)
		})
		return
	}
	if combined != nil && os.Getenv("P05_O09_SUPERVISED_CLOSE") != "" {
		t.Run("SupervisedOperatorHTTP", func(t *testing.T) {
			reply, _ := post(http.StatusOK)
			if reply.Status != "verified" || reply.PendingCount == nil || *reply.PendingCount != 0 {
				t.Fatal("genuine 073 zero proof missing")
			}
			combined.supervisedClose(t, projection, receipt)
		})
		return
	}
	if combined != nil && os.Getenv("P05_O09_PROTECTED_CLOSE_EXECUTOR") != "" {
		t.Run("ActualProtectedCloseWithLostResponse", func(t *testing.T) {
			result, _ := post(http.StatusOK)
			if result.Status != "verified" || result.PendingCount == nil || *result.PendingCount != 0 {
				t.Fatalf("actual v1 proof must complete before staged close: %+v", result)
			}
			combined.recordWithExpectedPending(t, projection, receipt, 0)
			combined.executeProtectedClose(t)
			combined.assertHeldReview(t)
		})
		return
	}
	if combined != nil {
		t.Run("ActualP05AuthorityAndWriter", func(t *testing.T) {
			result, _ := post(http.StatusOK)
			if result.Status != "pending" || result.PendingCount == nil || *result.PendingCount != 1 {
				t.Fatal("actual 074 lookup did not authorize one genuine v1 pending item")
			}
			combined.recordAndReview(t, projection, receipt)
			request := closeRequest(result)
			if checked := confirm(request, http.StatusOK); !checked.ValidFor(request, time.Now()) {
				t.Fatal("actual P05 authority did not confirm the original P07 checkpoint")
			}
			combined.revoke(t)
			confirm(request, http.StatusConflict)
			result, _ = post(http.StatusOK)
			if result != (chainview.GenerationAssessment{Status: "unknown"}) {
				t.Fatal("actual P05 revocation retained a P07 count")
			}
			combined.verifyRevocation(t, projection, receipt)
		})
		return
	}
	reply, lookups := post(200)
	if reply.Status != "pending" || reply.PendingCount == nil || *reply.PendingCount != 1 || lookups != 2 {
		t.Fatalf("dedicated process did not return one current pending payment with live recheck: %+v, lookups=%d", reply, lookups)
	}
	anchor := closeRequest(reply)
	confirmed := confirm(anchor, http.StatusOK)
	if confirmed.CloseAnchorRequest != anchor || confirmed.Finalized != anchor.Previous || confirmed.ExpiresAt > anchor.ValidUntil || reads.Load() != 2 {
		t.Fatalf("close-anchor failed to confirm exact scoped checkpoint: %+v", confirmed)
	}
	bad := anchor
	bad.Previous.Hash = challenge.Hex()
	confirm(bad, http.StatusConflict)
	bad = anchor
	bad.RegistrationBinding = challenge.Hex()
	confirm(bad, http.StatusConflict)
	bad = anchor
	bad.Branch = board
	confirm(bad, http.StatusNotFound)
	bad = anchor
	bad.ValidUntil = time.Now().Unix()
	confirm(bad, http.StatusBadRequest)
	mode.Store(1)
	confirm(anchor, http.StatusConflict)
	mode.Store(4)
	confirm(anchor, http.StatusConflict)
	if _, err := db.Exec(ctx, `UPDATE chainview.block SET canonical=true WHERE generation_id=$1 AND height=$2`, active, profile.StartHeight+1); err != nil {
		t.Fatal(err)
	}
	mode.Store(0)
	for _, scenario := range []int32{1, 2, 3} {
		mode.Store(scenario)
		reply, lookups = post(200)
		if reply != (chainview.GenerationAssessment{Status: "unknown"}) || (scenario == 1 && lookups != 2) {
			t.Fatalf("authority denial %d exposed a count: %+v, lookups=%d", scenario, reply, lookups)
		}
	}
	mode.Store(4)
	reply, lookups = post(http.StatusOK)
	if _, err := db.Exec(ctx, `UPDATE chainview.block SET canonical=true
		WHERE generation_id=$1 AND height=$2`, active, profile.StartHeight+1); err != nil {
		t.Fatal(err)
	}
	if reply != (chainview.GenerationAssessment{Status: "incomplete"}) || lookups != 2 {
		t.Fatalf("coverage change during authority recheck escaped final anchor check: %+v, lookups=%d", reply, lookups)
	}
	mode.Store(0)
	chain, err := ethclient.DialContext(ctx, rpc)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Close()
	if err := chain.Client().CallContext(ctx, nil, "anvil_mine", 3); err != nil {
		t.Fatal(err)
	}
	confirm(anchor, http.StatusConflict)
	ingest, err := chainview.Open(ctx, dsn+" user=rental_o09_test_ingest")
	if err != nil {
		t.Fatal(err)
	}
	defer ingest.Close()
	source, err := chainview.Dial(ctx, rpc, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	ingester := chainview.Reader{Profile: profile, DB: ingest, Source: source}
	if err := ingester.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	newer := confirm(anchor, http.StatusOK)
	if newer.Finalized.Height <= anchor.Previous.Height || !newer.ValidFor(anchor, time.Now()) {
		t.Fatalf("stored canonical ancestor did not survive a newer final head: %+v", newer)
	}
	replaced := save("withdrawn-grants", []byte("{}"))
	if err := os.Rename(replaced, grantPath); err != nil {
		t.Fatal(err)
	}
	_, lookups = post(http.StatusNotFound)
	if lookups != 0 {
		t.Fatal("withdrawn TLS grant accessed authority")
	}
	unauthenticated := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool}}, Timeout: 3 * time.Second}
	defer unauthenticated.CloseIdleConnections()
	if res, err := unauthenticated.Post(endpoint+rentalv1.ReconcilePath, "application/json", bytes.NewReader(request)); err == nil {
		_ = res.Body.Close()
		t.Fatal("dedicated listener accepted a TLS connection without client authentication")
	}
}
