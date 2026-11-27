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
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
)

func TestPostgresPrivateRentalCollectorAuthority(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires migrated disposable PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(config.Host, "/private/tmp/") || strings.HasPrefix(config.Host, "/tmp/")) {
		t.Fatal("use disposable commerce database")
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	db, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	const tenant = "17ee49c5-c96e-48cc-a2e7-261efd3ec983"
	const branch = "966e84c6-dc18-4d57-bf86-da8e35b11aa8"
	const actor = "3375093b-f96d-4838-9cc0-ce7bbf9577f9"
	const device = "a207d53a-1b90-45c1-ac6d-ecedadf8d142"
	const stream = "a6fe381f-ffe3-411e-b2bb-ef0158fc7bc8"
	const index = "b6fe381f-ffe3-411e-b2bb-ef0158fc7bc8"
	const account = "0x5151515151515151515151515151515151515151"
	const payout = "0x5252525252525252525252525252525252525252"
	boardKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	boardSigner := strings.ToLower(crypto.PubkeyToAddress(boardKey.PublicKey).Hex())
	const settlement = "0x5454545454545454545454545454545454545454"
	const role = "rental_072_fixture_authority"
	const readerLogin = "rental_091_fixture_reader"
	const writerLogin = "rental_091_fixture_writer"
	const customerLogin = "rental_091_fixture_customer"
	exec("CREATE ROLE " + readerLogin + " LOGIN IN ROLE nu54_p05_rental_authority_read")
	exec("CREATE ROLE " + writerLogin + " LOGIN IN ROLE nu54_p05_rental_evidence")
	exec("CREATE ROLE " + customerLogin + " LOGIN IN ROLE nu54_p05_commerce")
	exec("CREATE ROLE " + role + " LOGIN IN ROLE nu54_p05_rental_authority")
	defer db.Exec(context.Background(), "DROP ROLE "+role)
	exec("INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	exec("INSERT INTO ledger.login_subject VALUES('rental_072_fixture',$1)", actor)
	exec("INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',now()+interval '3 minutes')", branch, actor)
	fill := func(v byte) []byte { return bytes.Repeat([]byte{v}, 32) }
	registration, measurement, challenge, manifest := fill(1), fill(2), fill(3), fill(4)
	board, first, peer := fill(5), make([]byte, 32), []byte{1, 2, 3, 4, 5, 6, 7}
	deadline := time.Now().Truncate(time.Second).Add(time.Minute)
	exec(`INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
		VALUES($1,$2,1,$3,$4,'qualified',$5,now()+interval '2 minutes',now()-interval '1 minute')`,
		branch, device, account, payout, registration)
	exec(`INSERT INTO ledger.rental_generation_source VALUES($1,$2,1,$3,$4,$5,'qualified',$6,$7,$8,$9,$10,$11,$12,
		$13,$14,now()+interval '2 minutes')`, branch, device, registration, account, payout, measurement, boardSigner,
		settlement, manifest, stream, index, challenge, deadline, writerLogin)
	installKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO ledger.firmware_member VALUES($1,$2,'qualified',$3,$4,2,'0.0.2',false,NULL)",
		branch, device, board, elliptic.Marshal(installKey.Curve, installKey.X, installKey.Y))
	exec(`INSERT INTO ledger.firmware_campaign(branch_id,device_ref,campaign_ref,actor_id,release_digest,version,counter,measurement,
		challenge,challenge_until,state,proof_digest) VALUES($1,$2,$3,$4,$5,'0.0.2',2,$6,$7,$8,'verified',$9)`,
		branch, device, index, actor, hex.EncodeToString(fill(6)), measurement, fill(7), deadline, fill(8))
	operator, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	deployment := core.Deployment{ChainID: 8283, Settlement: common.HexToAddress(settlement)}
	issuer, err := OpenRentalCollectorIssuer(ctx, config.ConnString()+" user="+role, operator, deployment, [32]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	defer issuer.Close()
	roots := x509.NewCertPool()
	credentials := make(map[string]service.Principal)
	identity := func(serial int64, role string) (tls.Certificate, [32]byte) {
		t.Helper()
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(serial), IsCA: true, BasicConstraintsValid: true,
			NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(3 * time.Minute),
			KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		wire, err := x509.CreateCertificate(rand.Reader, cert, cert, public, private)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := x509.ParseCertificate(wire)
		if err != nil {
			t.Fatal(err)
		}
		roots.AddCert(parsed)
		fingerprint := sha256.Sum256(wire)
		credentials[hex.EncodeToString(fingerprint[:])] = service.Principal{
			Tenant: tenant, Subject: actor, Role: role, ExpiresAt: cert.NotAfter.Unix(),
		}
		return tls.Certificate{Certificate: [][]byte{wire}, PrivateKey: private}, fingerprint
	}
	operatorCert, _ := identity(1, "operator")
	collectorCert, collector := identity(2, "rental-collector")
	unregisteredCert, replacementCollector := identity(3, "rental-collector")
	const customer = "3375093b-f96d-4838-9cc0-ce7bbf957810"
	const otherCustomer = "3375093b-f96d-4838-9cc0-ce7bbf957811"
	customerCert, customerPin := identity(4, "customer")
	otherCert, otherPin := identity(5, "customer")
	audienceCert, audience := identity(6, "w12-private-audience")
	_, writer := identity(7, "w12-private-writer")
	foreignCert, foreignPin := identity(8, "customer")
	foreignKey := hex.EncodeToString(foreignPin[:])
	foreign := credentials[foreignKey]
	foreign.Tenant = "17ee49c5-c96e-48cc-a2e7-261efd3ec984"
	foreign.Subject = customer
	credentials[foreignKey] = foreign
	for pin, subject := range map[[32]byte]string{customerPin: customer, otherPin: otherCustomer} {
		key := hex.EncodeToString(pin[:])
		credential := credentials[key]
		credential.Subject = subject
		credentials[key] = credential
	}
	exec(`INSERT INTO ledger.rental_collector_registration(branch_id,device_ref,generation,operator_ref,collector_certificate,
		collector_peer,first_nonce,authority_source,valid_until)
		SELECT $1,$2,1,$3,$4,$5,$6,ledger.rental_generation_source_digest(s),$7 FROM ledger.rental_generation_source s
		WHERE (s.branch_id,s.device_ref,s.generation)=($1,$2,1)`, branch, device, actor, collector[:], peer, first, deadline)

	mux := http.NewServeMux()
	issuer.Register(mux, credentials)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	call := func(cert tls.Certificate, path, kind string, body []byte, want int) []byte {
		t.Helper()
		transport := server.Client().Transport.(*http.Transport).Clone()
		defer transport.CloseIdleConnections()
		transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
		client := &http.Client{Transport: transport}
		method := http.MethodGet
		if body != nil {
			method = http.MethodPost
		}
		request, err := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", kind)
		reply, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		raw, err := io.ReadAll(reply.Body)
		if err != nil || reply.StatusCode != want {
			t.Fatalf("wanted HTTP %d, got %d (%v)", want, reply.StatusCode, err)
		}
		if reply.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("private response must not be cached")
		}
		return raw
	}
	registrationPath := fmt.Sprintf("/v2/private/rental-registration/%s/%s/1", branch, device)
	freezePath := fmt.Sprintf("/v2/private/rental-collector/%s/%s/1/freeze-challenge", branch, device)
	link := fill(9)
	freezeBody := []byte(fmt.Sprintf(`{"channelNonce":"%x","expiresAt":%d}`, link, deadline.Unix()))
	call(collectorCert, freezePath, "application/json", freezeBody, http.StatusNotFound)
	call(collectorCert, registrationPath, "", nil, http.StatusNotFound)
	registrationResponse := call(operatorCert, registrationPath, "", nil, http.StatusOK)
	binding := rentalCollectorBinding(branch, device, 1, deployment, account, registration, measurement, manifest)
	wire := make([]byte, 439)
	copy(wire, "N54G")
	copy(wire[4:], challenge)
	copy(wire[36:], board)
	copy(wire[68:], binding[:])
	copy(wire[276:], common.HexToAddress(boardSigner).Bytes())
	copy(wire[296:], collector[:])
	copy(wire[328:], peer)
	copy(wire[335:], first)
	binary.BigEndian.PutUint64(wire[367:], uint64(deadline.Unix()))
	var staged struct {
		InstallationRequest string
	}
	if json.Unmarshal(registrationResponse, &staged) != nil || staged.InstallationRequest != hex.EncodeToString(wire[:375]) {
		t.Fatal("installation request did not equal the exact board and collector scope")
	}
	digest := sha256.Sum256(wire[:375])
	r, s, err := ecdsa.Sign(rand.Reader, installKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	r.FillBytes(wire[375:407])
	s.FillBytes(wire[407:439])
	wire[296] ^= 1
	call(operatorCert, registrationPath, "application/octet-stream", wire, http.StatusNotFound)
	wire[296] ^= 1
	call(operatorCert, registrationPath, "application/octet-stream", wire, http.StatusOK)
	call(operatorCert, registrationPath, "application/octet-stream", wire, http.StatusOK)
	retryR, retryS, err := ecdsa.Sign(rand.Reader, installKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	retryR.FillBytes(wire[375:407])
	retryS.FillBytes(wire[407:439])
	call(operatorCert, registrationPath, "application/octet-stream", wire, http.StatusOK)
	call(unregisteredCert, freezePath, "application/json", freezeBody, http.StatusNotFound)
	call(operatorCert, freezePath, "application/json", freezeBody, http.StatusNotFound)
	response := call(collectorCert, freezePath, "application/json", freezeBody, http.StatusOK)
	var reply struct {
		Binding, Challenge, Signature string
		ExpiresAt                     uint64
	}
	wantBinding := crypto.Keccak256Hash(binding[:])
	if json.Unmarshal(response, &reply) != nil || reply.Binding != hex.EncodeToString(wantBinding[:]) ||
		reply.Challenge != hex.EncodeToString(challenge) || reply.ExpiresAt != uint64(deadline.Unix()) {
		t.Fatal("freeze response did not equal the current enrollment")
	}
	request := ops.RentalFreezeRequestV2{Binding: wantBinding, Challenge: common.Hash(challenge),
		ChannelNonce: common.Hash(link), ExpiresAt: reply.ExpiresAt}
	rfq, err := request.Digest(deployment.Domain())
	if err != nil {
		t.Fatal(err)
	}
	sig, err := hex.DecodeString(reply.Signature)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := core.RecoverSigner(rfq, sig)
	if err != nil || recovered != crypto.PubkeyToAddress(operator.PublicKey) {
		t.Fatal("wrong freeze grant signature")
	}
	exec("UPDATE ledger.rental_collector_registration SET collector_certificate=$3 WHERE (branch_id,device_ref)=($1,$2)", branch, device, replacementCollector[:])
	call(unregisteredCert, freezePath, "application/json", freezeBody, http.StatusNotFound)
	exec("UPDATE ledger.rental_collector_registration SET collector_certificate=$3 WHERE (branch_id,device_ref)=($1,$2)", branch, device, collector[:])
	replacementBoard, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE ledger.firmware_member SET identity_p256=$3 WHERE (branch_id,device_ref)=($1,$2)", branch, device,
		elliptic.Marshal(replacementBoard.Curve, replacementBoard.X, replacementBoard.Y))
	call(collectorCert, freezePath, "application/json", freezeBody, http.StatusNotFound)
	exec("UPDATE ledger.firmware_member SET identity_p256=$3 WHERE (branch_id,device_ref)=($1,$2)", branch, device,
		elliptic.Marshal(installKey.Curve, installKey.X, installKey.Y))
	exec("UPDATE ledger.rental_collector_registration SET collector_peer=$3 WHERE (branch_id,device_ref)=($1,$2)", branch, device, []byte{1, 8, 9, 4, 5, 6, 7})
	call(collectorCert, freezePath, "application/json", freezeBody, http.StatusNotFound)
	exec("UPDATE ledger.rental_collector_registration SET collector_peer=$3 WHERE (branch_id,device_ref)=($1,$2)", branch, device, peer)
	call(collectorCert, freezePath, "application/json", freezeBody, http.StatusOK)
	exec("UPDATE ledger.firmware_member SET withdrawn=true WHERE (branch_id,device_ref)=($1,$2)", branch, device)
	call(collectorCert, freezePath, "application/json", freezeBody, http.StatusNotFound)
	exec("UPDATE ledger.firmware_member SET withdrawn=false WHERE (branch_id,device_ref)=($1,$2)", branch, device)
	t.Run("current_renter_credential_projection", func(t *testing.T) {
		reader, err := OpenRentalAuthorityReader(ctx, config.ConnString()+" user="+readerLogin, deployment,
			[32]byte(manifest), crypto.PubkeyToAddress(operator.PublicKey))
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		store, err := Open(ctx, config.ConnString()+" user="+customerLogin)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if raw := store.db.QueryRow(ctx, "SELECT count(*) FROM ledger.rental_authority_audience"); raw.Scan(new(int)) == nil {
			t.Fatal("ordinary commerce database credentials could read private pins")
		}
		pinAudience := func() {
			exec(`INSERT INTO ledger.rental_authority_audience
				SELECT $1,$2,1,$3,$4,$5,ledger.rental_generation_source_digest(s),$6
				FROM ledger.rental_generation_source s WHERE (s.branch_id,s.device_ref,s.generation)=($1,$2,1)`,
				branch, device, readerLogin, audience[:], writer[:], deadline)
		}
		pinAudience()
		states := make(chan *tls.ConnectionState, 1)
		mux.HandleFunc("GET /fixture/current-reader-tls", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			states <- r.TLS
			w.WriteHeader(http.StatusNoContent)
		})
		call(audienceCert, "/fixture/current-reader-tls", "", nil, http.StatusNoContent)
		readerTLS := <-states
		successorKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		successorAccount := strings.ToLower(crypto.PubkeyToAddress(successorKey.PublicKey).Hex())
		type fixtureChange int
		const (
			noChange fixtureChange = iota
			revokeAudience
			expireGrant
			revokeHandover
			replaceGeneration
		)
		var change atomic.Int32
		var assignment string
		var assignmentForRace atomic.Value
		var sourceCalls, slotCalls atomic.Int32
		sourceFor := func(ctx context.Context, exact [208]byte, nonce [32]byte, receiptType string) (rentalCredentialEvidence, error) {
			if receiptType == slotOpenReceiptType {
				slotCalls.Add(1)
			} else {
				sourceCalls.Add(1)
			}
			if exact != binding || nonce == [32]byte(challenge) {
				t.Error("wrong credential collection scope")
			}
			now := uint64(time.Now().Unix())
			e := rentalCredentialEvidence{revision: 1, policy: 1, issuedAt: now, checkedAt: now, endsAt: now + 120, challenge: nonce}
			issued, checked := e.receiptDigests(exact, receiptType)
			for i, signed := range [2]common.Hash{issued, checked} {
				key := operator
				if i == 1 {
					key = boardKey
				}
				signature, err := crypto.Sign(signed[:], key)
				if err != nil {
					return rentalCredentialEvidence{}, err
				}
				signature[64] += 27
				if i == 0 {
					e.issuerSignature = [65]byte(signature)
				} else {
					e.boardSignature = [65]byte(signature)
				}
			}
			switch fixtureChange(change.Load()) {
			case revokeAudience:
				_, err = db.Exec(ctx, "DELETE FROM ledger.rental_authority_audience WHERE branch_id=$1 AND device_ref=$2", branch, device)
			case expireGrant:
				_, err = db.Exec(ctx, "UPDATE ledger.rental_authority_audience SET valid_until=clock_timestamp() WHERE branch_id=$1 AND device_ref=$2", branch, device)
			case revokeHandover:
				var changed bool
				if err := store.db.QueryRow(ctx, `SELECT ledger.change_renter_return($1,$2,$3,'revoke','')`, tenant, actor, assignmentForRace.Load().(string)).Scan(&changed); err != nil || !changed {
					t.Errorf("revoke during board reply: %v", err)
				}
			case replaceGeneration:
				_, err = db.Exec(ctx, `INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
					VALUES($1,$2,2,$3,$4,'qualified',$5,now()+interval '1 minute',now())`,
					branch, device, successorAccount, payout, fill(31))
			}
			return e, err
		}
		source := func(ctx context.Context, exact [208]byte, nonce [32]byte) (rentalCredentialEvidence, error) {
			return sourceFor(ctx, exact, nonce, credentialReceiptType)
		}
		provider := &renterCredentialCollector{authority: reader, tls: readerTLS, profile: [32]byte(measurement), writer: writer, source: source}
		slot := &renterSlotOpenCollector{authority: reader, tls: readerTLS, profile: [32]byte(measurement), writer: writer,
			source: func(ctx context.Context, exact [208]byte, nonce [32]byte) (rentalCredentialEvidence, error) {
				return sourceFor(ctx, exact, nonce, slotOpenReceiptType)
			}}
		h := HTTP{Store: store, Credentials: credentials, renterCredentialCollector: provider, renterSlotOpenCollector: slot}
		h.registerRenterStatus(mux, http.NewServeMux())
		legacy := HTTP{Store: store, Credentials: credentials}
		mux.HandleFunc("GET /fixture/slot-open/{assignment}", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if _, known := h.readRenterSlotOpen(r.Context(), r.TLS, r.PathValue("assignment"), slot); !known {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc("GET /fixture/unconfigured/{assignment}", legacy.renterReturn)
		offer := func() {
			if err := store.db.QueryRow(ctx, `SELECT ledger.offer_renter_return($1,$2,$3,$4,1,$5,$6,$7)::text`,
				tenant, branch, actor, device, customer, account, deadline).Scan(&assignment); err != nil {
				t.Fatal(err)
			}
			assignmentForRace.Store(assignment)
		}
		statusPath := func() string { return "/v2/renter/returns/" + assignment }
		expect := func(raw []byte, available bool) {
			t.Helper()
			var received map[string]json.RawMessage
			if err := json.Unmarshal(raw, &received); err != nil {
				t.Fatal(err)
			}
			if string(received["settlement"]) != `"unknown"` || string(received["account"]) != `"`+account+`"` {
				t.Fatal("credential projection replaced prior settlement or account")
			}
			wantFields := 6
			if available {
				wantFields = 8
			}
			if len(received) != wantFields || (len(received["credentialExpiresAt"]) > 0) != available ||
				(len(received["credentialEvidenceUntil"]) > 0) != available {
				t.Fatalf("unexpected projection field count=%d wanted=%d", len(received), wantFields)
			}
			if available {
				var ends, evidenceUntil time.Time
				if json.Unmarshal(received["credentialExpiresAt"], &ends) != nil ||
					json.Unmarshal(received["credentialEvidenceUntil"], &evidenceUntil) != nil ||
					!ends.After(evidenceUntil) || !evidenceUntil.After(time.Now()) {
					t.Fatal("invalid credential or observation deadline")
				}
			}
		}
		offer()
		accept := func() {
			expect(call(customerCert, statusPath()+"/accept", "application/json", []byte(`{"account":"`+account+`"}`), http.StatusOK), false)
		}
		call(customerCert, statusPath(), "", nil, http.StatusNotFound)
		accept()
		call(otherCert, statusPath()+"?credential=1", "", nil, http.StatusNotFound)
		call(foreignCert, statusPath()+"?credential=1", "", nil, http.StatusNotFound)
		expect(call(customerCert, statusPath(), "", nil, http.StatusOK), false)
		expect(call(customerCert, statusPath()+"?credential=1&credential=1", "", nil, http.StatusOK), false)
		expect(call(customerCert, "/fixture/unconfigured/"+assignment+"?credential=1", "", nil, http.StatusOK), false)
		if sourceCalls.Load() != 0 {
			t.Fatal("credential collection ran without an authorized configured GET")
		}
		expect(call(customerCert, statusPath()+"?credential=1", "", nil, http.StatusOK), true)
		slotPath := func() string { return "/fixture/slot-open/" + assignment }
		call(customerCert, slotPath(), "", nil, http.StatusNoContent)
		assertSlotProjection := func(path string, wantFields int) {
			t.Helper()
			var body map[string]json.RawMessage
			if err := json.Unmarshal(call(customerCert, path, "", nil, http.StatusOK), &body); err != nil {
				t.Fatal(err)
			}
			var checked, until time.Time
			if len(body) != wantFields || string(body["slot"]) != `"open"` ||
				json.Unmarshal(body["slotCheckedAt"], &checked) != nil || json.Unmarshal(body["slotEvidenceUntil"], &until) != nil ||
				checked.After(time.Now()) || !until.After(time.Now()) || until.Sub(checked) > 30*time.Second {
				t.Fatal("invalid negotiated slot observation")
			}
		}
		assertSlotProjection(statusPath()+"?slot=1", 8)
		assertSlotProjection(statusPath()+"?credential=1&slot=1", 10)
		original := call(customerCert, statusPath(), "", nil, http.StatusOK)
		for _, result := range [][]byte{
			call(customerCert, statusPath()+"?slot=1&slot=1", "", nil, http.StatusOK),
			call(customerCert, "/fixture/unconfigured/"+assignment+"?slot=1", "", nil, http.StatusOK),
			call(customerCert, statusPath()+"/accept?slot=1", "application/json", []byte(`{"account":"`+account+`"}`), http.StatusOK),
		} {
			if !bytes.Equal(original, result) {
				t.Fatal("legacy negotiated or acceptance response changed")
			}
		}
		call(otherCert, slotPath(), "", nil, http.StatusNotFound)
		call(foreignCert, slotPath(), "", nil, http.StatusNotFound)
		change.Store(int32(revokeAudience))
		expect(call(customerCert, statusPath()+"?credential=1", "", nil, http.StatusOK), false)
		change.Store(int32(noChange))
		expect(call(customerCert, statusPath()+"?credential=1", "", nil, http.StatusOK), false)
		pinAudience()
		change.Store(int32(expireGrant))
		expect(call(customerCert, statusPath()+"?credential=1", "", nil, http.StatusOK), false)
		call(customerCert, slotPath(), "", nil, http.StatusNotFound)
		expect(call(customerCert, statusPath()+"?slot=1", "", nil, http.StatusOK), false)
		exec("UPDATE ledger.rental_authority_audience SET valid_until=$2 WHERE branch_id=$1", branch, deadline)
		change.Store(int32(revokeHandover))
		call(customerCert, statusPath()+"?slot=1", "", nil, http.StatusNotFound)
		offer()
		accept()
		call(customerCert, statusPath()+"?credential=1", "", nil, http.StatusNotFound)
		offer()
		accept()
		// Hold 091's device fence before its first read, then revoke on the
		// lock holder. The initial public tuple has already been read.
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err := tx.Exec(ctx, "SELECT ledger.lock_rental_evidence_device($1,$2)", branch, device); err != nil {
			t.Fatal(err)
		}
		transport := server.Client().Transport.(*http.Transport).Clone()
		defer transport.CloseIdleConnections()
		transport.TLSClientConfig.Certificates = []tls.Certificate{customerCert}
		status := make(chan int, 1)
		path := statusPath() + "?credential=1"
		go func() {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
			if err != nil {
				status <- 0
				return
			}
			reply, err := (&http.Client{Transport: transport}).Do(request)
			if err != nil {
				status <- 0
				return
			}
			defer reply.Body.Close()
			body, err := io.ReadAll(reply.Body)
			if err != nil || bytes.Contains(body, []byte(account)) {
				status <- 0
				return
			}
			status <- reply.StatusCode
		}()
		waitUntil := time.Now().Add(time.Second)
		for {
			if _, err := tx.Exec(ctx, "SELECT pg_stat_clear_snapshot()"); err != nil {
				t.Fatal(err)
			}
			var blocked bool
			err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_stat_activity
				WHERE usename=$1 AND wait_event='advisory' AND query LIKE '%ledger.renter_credential_scope%')`,
				customerLogin).Scan(&blocked)
			if err != nil || time.Now().After(waitUntil) {
				t.Fatalf("initial credential scope did not block at device fence: %v", err)
			}
			if blocked {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		var revoked bool
		if err := tx.QueryRow(ctx, `SELECT ledger.change_renter_return($1,$2,$3,'revoke','')`,
			tenant, actor, assignment).Scan(&revoked); err != nil || !revoked {
			t.Fatalf("revoke under initial lookup fence: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if got := <-status; got != http.StatusNotFound {
			t.Fatalf("initial credential scope race returned status %d or leaked prior account", got)
		}
		offer()
		accept()
		change.Store(int32(replaceGeneration))
		call(customerCert, statusPath()+"?credential=1", "", nil, http.StatusNotFound)
		if sourceCalls.Load() != 6 || slotCalls.Load() != 4 {
			t.Fatalf("expected six credential and four slot evidence attempts, got %d and %d", sourceCalls.Load(), slotCalls.Load())
		}
	})
	exec("DELETE FROM ledger.finance_grant WHERE branch_id=$1 AND actor_id=$2 AND capability='rental_manage'", branch, actor)
	call(collectorCert, freezePath, "application/json", freezeBody, http.StatusNotFound)
}
