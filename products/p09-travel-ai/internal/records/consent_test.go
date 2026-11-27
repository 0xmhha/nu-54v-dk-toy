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
	"encoding/pem"
	"fmt"
	"io"
	"maps"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	osExec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/places"
	"github.com/jackc/pgx/v5"
)

func TestPrivateConsentMutualTLS(t *testing.T) {
	dsn := os.Getenv("P09_PRIVATE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires a freshly migrated disposable P09 database")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(cfg.Database, "nu54_p09_private") ||
		!(strings.HasPrefix(cfg.Host, "/tmp/") || strings.HasPrefix(cfg.Host, "/private/tmp/")) {
		t.Fatal("use private temporary PG socket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	exec := func(statement string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	login := "p09_consent_test_" + hex.EncodeToString(nonce[:])
	role := pgx.Identifier{login}.Sanitize()
	exec("CREATE ROLE " + role + " LOGIN IN ROLE nu54_p09_private_user")
	defer admin.Exec(context.Background(), "DROP ROLE "+role)
	exec(`INSERT INTO travel_private.policy VALUES
		('voice',1,'p09-sync',gen_random_uuid(),604800,false,true),
		('location',1,'p09-sync',gen_random_uuid(),2592000,false,true),
		('transcription',1,'fixture-transcriber',gen_random_uuid(),300,true,true),
		('review',1,'p09',gen_random_uuid(),300,true,true),
		('challenge',1,'p09',gen_random_uuid(),300,true,true),
		('recommendation',1,'p09',gen_random_uuid(),3600,false,true) ON CONFLICT(purpose,version,recipient) DO NOTHING`)
	issuerLogin := login + "_issuer"
	issuerRole := pgx.Identifier{issuerLogin}.Sanitize()
	exec("CREATE ROLE " + issuerRole + " LOGIN IN ROLE nu54_p09_visit_issuer")
	defer admin.Exec(context.Background(), "DROP ROLE "+issuerRole)
	var allowed bool
	err = admin.QueryRow(ctx, `SELECT NOT has_schema_privilege('nu54_p09_public','travel_private','USAGE') AND
		NOT has_table_privilege($1,'travel_private.consent_decision','UPDATE,INSERT,DELETE')`, login).Scan(&allowed)
	if err != nil || !allowed {
		t.Fatal("private purpose roles overlap")
	}
	roots := x509.NewCertPool()
	grants := make(map[string]Principal)
	var clients []tls.Certificate
	subjects := []string{"ae08fd79-5adc-4850-885f-0812b12ee892", "4d4d354d-c353-4daf-a2b2-ce046ce3700c"}
	now := time.Now().UTC().Truncate(time.Second)
	benefitDeadline := now.Add(4 * time.Second)
	peerDeadline := now.Add(12 * time.Second)
	for idx := range 9 {
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(int64(idx + 1)), IsCA: true, BasicConstraintsValid: true,
			NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Minute),
			KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		if idx == 5 {
			template.NotAfter = peerDeadline
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		roots.AddCert(cert)
		clients = append(clients, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: private})
		digest := sha256.Sum256(der)
		if idx < len(subjects) {
			grants[hex.EncodeToString(digest[:])] = Principal{Subject: subjects[idx], Role: "private-user", ExpiresAt: now.Add(time.Minute).Unix()}
		} else if idx == 3 {
			grants[hex.EncodeToString(digest[:])] = Principal{Subject: subjects[0], Role: "visit-issuer", Issuer: "fixture-p05", ExpiresAt: now.Add(time.Minute).Unix()}
		} else if idx == 4 {
			grants[hex.EncodeToString(digest[:])] = Principal{Subject: subjects[0], Role: "visit-issuer", Issuer: "fixture-live-p05", ExpiresAt: now.Add(time.Minute).Unix()}
		} else if idx == 8 {
			grants[hex.EncodeToString(digest[:])] = Principal{Subject: subjects[0], Role: "private-user", ExpiresAt: now.Add(time.Minute).Unix()}
		} else if idx >= 5 {
			name := "fixture-live-p05"
			if idx == 6 {
				name = "fixture-p05"
			}
			grantDeadline := now.Add(time.Minute)
			if idx == 7 {
				grantDeadline = benefitDeadline
			}
			grants[hex.EncodeToString(digest[:])] = Principal{Subject: subjects[0], Role: "benefit-worker", Issuer: name, ExpiresAt: grantDeadline.Unix()}
		}
	}
	service, err := Open(ctx, dsn+" user="+login, grants)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	vaultKey := make([]byte, 32)
	if _, err := rand.Read(vaultKey); err != nil {
		t.Fatal(err)
	}
	defer clear(vaultKey)
	if err := service.EnableVault(vaultKey); err != nil {
		t.Fatal(err)
	}
	signingPublic, signingPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnableVisitIngest(ctx, dsn+" user="+issuerLogin, map[string]VisitIssuer{"fixture-p05": {
		Key: base64.StdEncoding.EncodeToString(signingPublic), Environment: "synthetic",
	}, "fixture-live-p05": {Key: base64.StdEncoding.EncodeToString(signingPublic), Environment: "live"}}); err != nil {
		t.Fatal(err)
	}
	benefitKey := make([]byte, 32)
	if _, err := rand.Read(benefitKey); err != nil {
		t.Fatal(err)
	}
	defer clear(benefitKey)
	benefitLogin := login + "_benefit"
	benefitRole := pgx.Identifier{benefitLogin}.Sanitize()
	exec("CREATE ROLE " + benefitRole + " LOGIN IN ROLE nu54_p09_benefit_assert")
	defer admin.Exec(context.Background(), "DROP ROLE "+benefitRole)
	if err := service.EnableBenefitIssuer(ctx, dsn+" user="+benefitLogin, benefitKey); err != nil {
		t.Fatal(err)
	}
	reviewKey := make([]byte, 32)
	if _, err := rand.Read(reviewKey); err != nil {
		t.Fatal(err)
	}
	defer clear(reviewKey)
	if err := service.EnableReviewKey(reviewKey); err != nil {
		t.Fatal(err)
	}
	readerLogin := login + "_review"
	readerRole := pgx.Identifier{readerLogin}.Sanitize()
	exec("CREATE ROLE " + readerRole + " LOGIN IN ROLE nu54_p09_review_read")
	defer admin.Exec(context.Background(), "DROP ROLE "+readerRole)
	reader, err := OpenReviewReader(ctx, dsn+" user="+readerLogin, reviewKey)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	publicMux := http.NewServeMux()
	reader.Register(publicMux)
	public := httptest.NewServer(publicMux)
	defer public.Close()
	mux := http.NewServeMux()
	grantPath := filepath.Join(t.TempDir(), "grants.json")
	publishGrants := func(snapshot map[string]Principal) {
		t.Helper()
		data, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		publishGrantFixture(t, grantPath, data)
	}
	publishGrants(grants)
	service.UseGrantFile(grantPath, false)
	service.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	serverURL := server.URL
	if binary := os.Getenv("P09_PRIVATE_TEST_BIN"); binary != "" {
		files := t.TempDir()
		file := func(name string, body []byte) string {
			path := filepath.Join(files, name)
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			return path
		}
		encoded := func(value any) []byte {
			b, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
		pemData := func(name string, body []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: name, Bytes: body}) }
		tlsServer := server.TLS.Certificates[0]
		key, err := x509.MarshalPKCS8PrivateKey(tlsServer.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		serverKey := file("server.key", pemData("PRIVATE KEY", key))
		serverCert := file("server.pem", pemData("CERTIFICATE", tlsServer.Certificate[0]))
		var clientCAs []byte
		for _, client := range clients {
			clientCAs = append(clientCAs, pemData("CERTIFICATE", client.Certificate[0])...)
		}
		clientRoot := file("clients.pem", clientCAs)
		approvedIssuers := file("issuers.json", encoded(service.visitTrust))
		keyFile := file("review-key", []byte(base64.StdEncoding.EncodeToString(reviewKey)))
		benefitFile := file("benefit-key", []byte(base64.StdEncoding.EncodeToString(benefitKey)))
		vaultFile := file("content-key", []byte(base64.StdEncoding.EncodeToString(vaultKey)))
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		cmd := osExec.CommandContext(ctx, binary, "-listen", addr, "-cert", serverCert, "-key", serverKey, "-client-ca", clientRoot, "-grants", grantPath)
		cmd.Env = []string{"P09_PRIVATE_DATABASE_URL=" + dsn + " user=" + login, "P09_VISIT_DATABASE_URL=" + dsn + " user=" + issuerLogin,
			"P09_VISIT_TRUST=" + approvedIssuers, "P09_REVIEW_KEY=" + keyFile, "P09_CONTENT_KEY=" + vaultFile,
			"P09_BENEFIT_SIGNING_KEY=" + benefitFile, "P09_BENEFIT_DATABASE_URL=" + dsn + " user=" + benefitLogin}
		var output bytes.Buffer
		cmd.Stderr = &output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() {
			cmd.Process.Kill()
			cmd.Wait()
			if t.Failed() {
				t.Log(output.String())
			}
		}()
		for retries := 0; retries < 50; retries++ {
			c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				c.Close()
				break
			}
			if retries == 49 {
				t.Fatal("normal recordsd did not open listener")
			}
			time.Sleep(20 * time.Millisecond)
		}
		serverURL = "https://" + addr
	}
	route := func(identity int, method, path string, command any, want int) []byte {
		t.Helper()
		raw, err := json.Marshal(command)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(ctx, method, serverURL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		config := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		config.Certificates = []tls.Certificate{clients[identity]}
		httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: config}}
		defer httpClient.CloseIdleConnections()
		res, err := httpClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err = io.ReadAll(res.Body)
		if err != nil || res.StatusCode != want {
			t.Fatalf("wanted %d, got %d: %s %v", want, res.StatusCode, raw, err)
		}
		return raw
	}
	call := func(identity int, method string, command any, want int) []byte {
		t.Helper()
		return route(identity, method, "/v1/private/consents", command, want)
	}
	accepted, rejected := true, false
	expiry := now.Add(time.Minute)
	first := Decision{Purpose: "voice", Policy: 1, Recipient: "p09-sync", Accept: &accepted, Expires: &expiry, Request: "16d957a8-7bf8-4680-a2cb-d56b5943d678"}
	call(2, "GET", nil, 404)
	if got := call(0, "GET", nil, 200); !bytes.Contains(got, []byte(`"consents":[]`)) {
		t.Fatal("default authority granted")
	}
	wrong := first
	wrong.Recipient = "other"
	call(0, "POST", wrong, 404)
	unconsented := first
	unconsented.Purpose, unconsented.Recipient = "transcription", "fixture-transcriber"
	call(0, "POST", unconsented, 404)
	success := call(0, "POST", first, 200)
	if !bytes.Equal(call(0, "POST", first, 200), success) {
		t.Fatal("retry returned a different decision receipt")
	}
	stale := first
	stale.Request = "a10cb73a-2719-47d8-ab2c-6e35adb7ddd6"
	call(0, "POST", stale, 409)
	if got := call(1, "GET", nil, 200); !bytes.Contains(got, []byte(`"consents":[]`)) {
		t.Fatal("other subject read private scope")
	}
	var guard time.Time
	if err := admin.QueryRow(ctx, `SELECT travel_private.require_consent($1,'voice',1,'p09-sync',null)`, subjects[0]).Scan(&guard); err != nil {
		t.Fatal(err)
	}
	withdrawal := first
	withdrawal.Expected, withdrawal.Accept, withdrawal.Expires, withdrawal.Request = 1, &rejected, nil, stale.Request
	call(0, "POST", withdrawal, 200)
	if err := admin.QueryRow(ctx, `SELECT travel_private.require_consent($1,'voice',1,'p09-sync',null)`, subjects[0]).Scan(&guard); err == nil {
		t.Fatal("withdrawal left prior generation authorized")
	}
	if got := call(0, "GET", nil, 200); !bytes.Contains(got, []byte(`"accepted":false`)) {
		t.Fatal("withdrawal status not persisted")
	}
	t.Run("VisitEvidenceNoRealBenefitAndReorg", func(t *testing.T) {
		proof := "e7660f94-9756-4543-b55a-c0e1a22e6fd8"
		item := visit{Issuer: "fixture-p05", Proof: proof, Subject: subjects[0], Place: "merchant", Version: 1, State: "settled", Observed: now}
		signed := func(v visit) signedVisit {
			body, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			sig := ed25519.Sign(signingPrivate, append([]byte("NU54-visit-assertion-v1\x00"), body...))
			return signedVisit{Document: base64.StdEncoding.EncodeToString(body), Signature: base64.StdEncoding.EncodeToString(sig)}
		}
		value := signed(item)
		route(0, "POST", "/v1/issuer/visit-evidence", value, 404)
		route(3, "GET", "/v1/private/consents", nil, 404)
		changed := value
		changed.Document = base64.StdEncoding.EncodeToString([]byte(`{"subject":"changed"}`))
		route(3, "POST", "/v1/issuer/visit-evidence", changed, 404)
		route(3, "POST", "/v1/issuer/visit-evidence", value, 200)
		ownOptions := route(0, "POST", "/v1/private/visits", map[string]string{"projection": "options"}, 200)
		if !bytes.Contains(ownOptions, []byte(`"options":[{`)) || !bytes.Contains(ownOptions, []byte(proof)) ||
			bytes.Contains(ownOptions, []byte(subjects[0])) {
			t.Fatalf("owned evidence missing or subject disclosed: %s", ownOptions)
		}
		otherOptions := route(1, "POST", "/v1/private/visits", map[string]string{"projection": "options"}, 200)
		if !bytes.Contains(otherOptions, []byte(`"options":[]`)) || bytes.Contains(otherOptions, []byte(proof)) {
			t.Fatalf("foreign evidence disclosed: %s", otherOptions)
		}
		claim := VisitClaim{Issuer: item.Issuer, Proof: proof, Place: item.Place, Purpose: "challenge", Revision: 1}
		route(0, "POST", "/v1/private/visits", claim, 404)
		want := first
		want.Purpose, want.Recipient, want.Object = "challenge", "p09", &proof
		call(0, "POST", want, 200)
		one := route(0, "POST", "/v1/private/visits", claim, 200)
		if !bytes.Equal(one, route(0, "POST", "/v1/private/visits", claim, 200)) {
			t.Fatal("same proof granted more than once")
		}
		if got := route(1, "GET", "/v1/private/visits", nil, 200); !bytes.Contains(got, []byte(`"visits":[]`)) {
			t.Fatal("foreign visit disclosure")
		}
		if got := route(0, "GET", "/v1/private/visits", nil, 200); !bytes.Contains(got, []byte(`"classification":"test-only"`)) {
			t.Fatal("test proof exposed real benefit")
		}
		item.Version, item.State = 2, "refunded"
		route(3, "POST", "/v1/issuer/visit-evidence", signed(item), 200)
		if got := route(0, "GET", "/v1/private/visits", nil, 200); !bytes.Contains(got, []byte(`"classification":"adjusted"`)) {
			t.Fatal("refund left original eligibility active")
		}
		item.Version, item.State = 3, "reorg"
		route(3, "POST", "/v1/issuer/visit-evidence", signed(item), 200)
		route(3, "POST", "/v1/issuer/visit-evidence", value, 409)
		route(0, "POST", "/v1/private/visits", claim, 404)
		reviewSubject := want
		reviewSubject.Purpose = "review"
		call(0, "POST", reviewSubject, 200)
		review := ReviewInput{ID: "df2b7615-a4bf-470f-aed6-e4f13797cf9f", Issuer: claim.Issuer, Proof: proof, Place: claim.Place,
			Revision: 1, Publish: &accepted, Body: "Fixture Korean review"}
		route(0, "POST", "/v1/private/reviews", review, 404)
		liveProof := "e7660f94-9756-4543-b55a-c0e1a22e6fd9"
		item.Issuer, item.Proof, item.State, item.Version = "fixture-live-p05", liveProof, "settled", 1
		route(4, "POST", "/v1/issuer/visit-evidence", signed(item), 200)
		review.Issuer, review.Proof, review.Revision = item.Issuer, liveProof, 2
		reviewSubject.Expected, reviewSubject.Object, reviewSubject.Request = 1, &liveProof, stale.Request
		call(0, "POST", reviewSubject, 200)
		route(1, "POST", "/v1/private/reviews", review, 404)
		route(0, "POST", "/v1/private/reviews", review, 200)
		publicCheck := func(published bool) {
			t.Helper()
			req, err := http.NewRequestWithContext(ctx, "GET", public.URL+"/v1/reviews/merchant", nil)
			if err != nil {
				t.Fatal(err)
			}
			res, err := public.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			content, err := io.ReadAll(res.Body)
			if err != nil || res.StatusCode != 200 || bytes.Contains(content, []byte(review.Body)) != published ||
				bytes.Contains(content, []byte(subjects[0])) || bytes.Contains(content, []byte(liveProof)) || bytes.Contains(content, []byte(item.Issuer)) {
				t.Fatalf("unexpected public review status: %d %s %v", res.StatusCode, content, err)
			}
		}
		publicCheck(true)
		retry := review
		retry.ID = ""
		response := route(0, "POST", "/v1/private/reviews", retry, 200)
		if !bytes.Contains(response, []byte(review.ID)) {
			t.Fatalf("replay lost the existing review identity: %s", response)
		}
		retry.Body = "Changed draft"
		route(0, "POST", "/v1/private/reviews", retry, 409)

		assertBenefit := func(index int, token, stage string, rev int64) {
			started := time.Now()
			t.Helper()
			response := route(index, "GET", "/v1/issuer/benefit/"+token, nil, 200)
			var signed signedVisit
			if json.Unmarshal(response, &signed) != nil {
				t.Fatal("benefit envelope invalid")
			}
			raw, err := base64.StdEncoding.Strict().DecodeString(signed.Document)
			if err != nil {
				t.Fatal(err)
			}
			sig, err := base64.StdEncoding.Strict().DecodeString(signed.Signature)
			if err != nil || !ed25519.Verify(service.benefit.key.Public().(ed25519.PublicKey), append([]byte("NU54-visit-benefit-v1\x00"), raw...), sig) {
				t.Fatal("benefit source signature invalid")
			}
			var assertion BenefitAssertion
			if json.Unmarshal(raw, &assertion) != nil || assertion.Proof != token || assertion.Classification != stage || assertion.Revision != rev ||
				bytes.Contains(raw, []byte(subjects[0])) || !assertion.Expires.After(started) ||
				assertion.Expires.After(time.Now().Add(20*time.Second)) ||
				(index == 7 && !assertion.Expires.Equal(benefitDeadline)) ||
				(index == 5 && !assertion.Expires.Equal(peerDeadline)) {
				t.Fatalf("unexpected benefit response %s", raw)
			}
		}
		route(1, "GET", "/v1/issuer/benefit/"+liveProof, nil, 404)
		assertBenefit(6, proof, "test-only", 1)
		assertBenefit(5, liveProof, "permission-held", 1)
		newRequest := "5af181a2-0568-43fd-ab8f-2a11fe7d90e6"
		authorizes := want
		authorizes.Object, authorizes.Expected, authorizes.Request = &liveProof, 1, newRequest
		call(0, "POST", authorizes, 200)
		claim.Issuer, claim.Proof, claim.Revision = item.Issuer, liveProof, 2
		route(0, "POST", "/v1/private/visits", claim, 200)
		assertBenefit(5, liveProof, "eligible", 2)
		assertBenefit(5, liveProof, "eligible", 2)
		item.Version, item.State = 2, "reorg"
		route(4, "POST", "/v1/issuer/visit-evidence", signed(item), 200)
		publicCheck(true)
		assertBenefit(5, liveProof, "adjusted", 3)
		assertBenefit(7, liveProof, "adjusted", 3)
		t.Run("BenefitAuthorityExpiresWhileWaitingForPostgres", func(t *testing.T) {
			tx, err := admin.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1 || ':challenge',900018))`, subjects[0])
			if err != nil {
				t.Fatal(err)
			}
			observer, err := pgx.ConnectConfig(ctx, cfg.Copy())
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Close(context.Background())
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/v1/issuer/benefit/"+liveProof, nil)
			if err != nil {
				t.Fatal(err)
			}
			config := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			config.Certificates = []tls.Certificate{clients[7]}
			client := &http.Client{Transport: &http.Transport{TLSClientConfig: config}}
			defer client.CloseIdleConnections()
			result := make(chan error, 1)
			go func() {
				res, err := client.Do(request)
				if err == nil {
					defer res.Body.Close()
					var raw []byte
					raw, err = io.ReadAll(res.Body)
					if err == nil && (res.StatusCode != http.StatusNotFound || bytes.Contains(raw, []byte(`"signature"`))) {
						err = fmt.Errorf("expired in-flight request returned status %d with body %s", res.StatusCode, raw)
					}
				}
				result <- err
			}()
			for {
				var waiting bool
				err = observer.QueryRow(ctx, `SELECT EXISTS (SELECT FROM pg_stat_activity WHERE usename=$1
					AND wait_event_type='Lock' AND query LIKE '%assert_visit_benefit%')`, benefitLogin).Scan(&waiting)
				if err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				if !time.Now().Before(benefitDeadline) {
					t.Fatal("request did not reach the DB lock before its grant expired")
				}
				time.Sleep(10 * time.Millisecond)
			}
			// Release the query immediately after the exact grant deadline.
			time.Sleep(time.Until(benefitDeadline))
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-result; err != nil {
				t.Fatal(err)
			}
		})

		deletion := map[string]string{"object": review.ID, "kind": "review", "requestKey": "2ead5654-a4c7-4079-96bb-14e07637e59c"}
		t.Cleanup(func() {
			_, err := admin.Exec(context.Background(), `DELETE FROM travel_private.erasure_target WHERE subject_ref=$1 AND request_id=$2;
				DELETE FROM travel_private.erasure_request WHERE subject_ref=$1 AND id=$2`, pgx.QueryExecModeSimpleProtocol, subjects[0], "2ead5654-a4c7-4079-96bb-14e07637e59c")
			if err != nil {
				t.Error(err)
			}
		})

		route(1, "POST", "/v1/private/erasure", deletion, 404)
		route(0, "POST", "/v1/private/erasure", deletion, 200)
		publicCheck(false)
		route(0, "POST", "/v1/private/reviews", map[string]any{"inspect": true, "issuer": item.Issuer, "proof": liveProof, "place": item.Place}, 404)
		route(0, "POST", "/v1/private/reviews", review, 404)

	})

	t.Run("DistinctChallengeProgressAndErasure", func(t *testing.T) {
		const endpoint = "/v1/private/visits"
		exec(`UPDATE travel_private.policy SET active=false WHERE purpose='challenge' AND recipient='p09';
			INSERT INTO travel_private.policy VALUES ('challenge',2,'p09',gen_random_uuid(),300,false,true)`)
		decision := Decision{Purpose: "challenge", Expected: 2, Policy: 2, Recipient: "p09", Accept: &accepted, Expires: &expiry,
			Request: "e66423a3-1abe-481e-9d30-876f431d5cc0"}
		call(0, "POST", decision, 200)
		references := []string{"6995dcdb-075f-47fe-b207-0961bc7ac216", "89c2cef6-83e7-4bf7-8ff8-b180ca86f3d8",
			"26fc01d1-d88d-40dc-b430-f1b19b96ed1b", "73a4aba1-578d-41f6-b59d-76bc802f5b67"}
		places := []string{"garden", "garden", "museum", "bakery"}
		var claims []string
		for i, proof := range references {
			exec(`SELECT travel_private.put_visit('fixture-live-p05',$1,$2,$3,'live',1,'settled',clock_timestamp())`, proof, subjects[0], places[i])
			data := route(0, "POST", endpoint, VisitClaim{Issuer: "fixture-live-p05", Proof: proof, Place: places[i], Purpose: "challenge", Revision: 3}, 200)
			var result map[string]string
			if err := json.Unmarshal(data, &result); err != nil || !uuid.MatchString(result["claim"]) {
				t.Fatalf("invalid claim: %s", data)
			}
			claims = append(claims, result["claim"])
		}
		count := func(want int) {
			t.Helper()
			data := route(0, "POST", endpoint, map[string]string{"projection": "options"}, 200)
			var value VisitExperience
			if err := json.Unmarshal(data, &value); err != nil || value.ChallengeCount != want {
				t.Fatalf("wanted %d distinct visits, response: %s", want, data)
			}
		}
		count(3)
		exec(`SELECT travel_private.put_visit('fixture-live-p05',$1,$2,'bakery','live',2,'refunded',clock_timestamp())`,
			references[3], subjects[0])
		count(2)
		erase := map[string]string{"object": claims[2], "kind": "visit-claim", "requestKey": "27457105-be43-4ae6-9bc9-031b48ab966b"}
		t.Cleanup(func() {
			_, err := admin.Exec(context.Background(), `DELETE FROM travel_private.erasure_target WHERE subject_ref=$1 AND request_id=$2;
				DELETE FROM travel_private.erasure_request WHERE subject_ref=$1 AND id=$2`, pgx.QueryExecModeSimpleProtocol, subjects[0], "27457105-be43-4ae6-9bc9-031b48ab966b")
			if err != nil {
				t.Error(err)
			}
		})

		route(1, "POST", "/v1/private/erasure", erase, 404)
		response := route(0, "POST", "/v1/private/erasure", erase, 200)
		if !bytes.Contains(response, []byte(`"localDeviceEvidence":"not-applicable"`)) {
			t.Fatal("server-only erasure reported device evidence")
		}
		count(1)
		route(0, "POST", endpoint, VisitClaim{Issuer: "fixture-live-p05", Proof: references[2], Place: "museum", Purpose: "challenge", Revision: 3}, 404)
		decision.Expected, decision.Accept, decision.Expires, decision.Request = 3, &rejected, nil, "8562f9b8-9528-4d0b-8512-3a4f63b92a38"
		call(0, "POST", decision, 200)
		count(0)
	})

	t.Run("VisitHistoryContinuationHasBoundedSubjectScopedResponses", func(t *testing.T) {
		previousServer := serverURL
		serverURL = server.URL
		defer func() { serverURL = previousServer }()
		previousReader := service.benefitRead
		benefits := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var scope []map[string]string
			if err := json.NewDecoder(r.Body).Decode(&scope); err != nil {
				w.WriteHeader(400)
				return
			}
			states := make([]BenefitStatus, len(scope))
			for i := range states {
				states[i] = BenefitStatus{Credited: 1<<63 - 1, Adjusted: 1<<63 - 1, Uncredited: 1<<63 - 1}
			}
			reply(w, http.StatusOK, states)
		}))
		defer benefits.Close()
		service.benefitRead = &benefitReader{endpoint: benefits.URL, client: benefits.Client()}
		defer func() { service.benefitRead = previousReader }()
		const endpoint = "/v1/private/visits"
		issuer, place := strings.Repeat("i", 64), strings.Repeat("p", 48)
		exec(`INSERT INTO travel_private.visit_evidence SELECT $1,gen_random_uuid(),$2,$3,'live',1,'settled',now()-make_interval(hours=>n)
		    FROM generate_series(1,111) n`, issuer, subjects[0], place)
		exec(`INSERT INTO travel_private.visit_claim(issuer,proof,purpose,consent_revision)
			SELECT issuer,proof,'challenge',4 FROM travel_private.visit_evidence WHERE issuer=$1 AND subject_ref=$2`, issuer, subjects[0])
		exec(`INSERT INTO travel_private.review
			SELECT issuer,proof,gen_random_uuid(),subject_ref,2,false,decode(repeat('ab',32),'hex'),decode(repeat('ab',29),'hex'),now()+interval '3 minutes'
			  FROM travel_private.visit_evidence WHERE issuer=$1 AND subject_ref=$2`, issuer, subjects[0])
		service.EnableCatalog(func(context.Context) (places.Catalog, error) {
			return places.Catalog{Places: []places.Place{{ID: place, Name: strings.Repeat("<", 180)}}}, nil
		})
		defer service.EnableCatalog(nil)
		var cursor string
		seen := make(map[string]bool)
		for page := 0; ; page++ {
			if page > 15 {
				t.Fatal("repeated cursor")
			}
			args := map[string]any{"projection": "options", "place": place, "includeBenefits": true}
			if cursor != "" {
				args["cursor"] = cursor
			}
			data := route(0, "POST", endpoint, args, 200)
			var value VisitExperience
			if err := json.Unmarshal(data, &value); err != nil || len(data) > 32768 || len(value.Options) == 0 || !value.BenefitAvailable {
				t.Fatalf("invalid mobile visit page (%d bytes): %s", len(data), data)
			}
			for _, item := range value.Options {
				if item.Place != place || item.Issuer != issuer || seen[item.Proof] {
					t.Fatal("foreign, filtered, or repeated visit")
				}
				seen[item.Proof] = true
			}
			if page == 0 {
				t.Logf("worst-case enriched first page: %d serialized bytes", len(data))
				if value.Next == "" {
					t.Fatal("first page lacked continuation")
				}
				route(1, "POST", endpoint, map[string]string{"projection": "options", "cursor": value.Next, "place": place}, 404)
				route(0, "POST", endpoint, map[string]string{"projection": "options", "cursor": value.Next, "place": "different"}, 404)
				// A correction changes observed_at and eligibility but must never move the item across the cursor.
				exec(`UPDATE travel_private.visit_evidence SET state='refunded', observed_at=now()
				   WHERE issuer=$1 AND proof=$2`, issuer, value.Options[len(value.Options)-1].Proof)
			}
			if value.Next == "" {
				break
			}
			cursor = value.Next
		}
		if len(seen) != 111 {
			t.Fatalf("only reached %d issuer events", len(seen))
		}
	})

	t.Run("PrivateItineraryProjectionRequiresConsentAndSurvivesNoRestoreFence", func(t *testing.T) {
		recordsURL := "/v1/private/records"
		optIn := map[string]string{"projection": "saved-itineraries"}
		if got := route(0, "GET", recordsURL, nil, 200); string(bytes.TrimSpace(got)) != `{"records":[]}` {
			t.Fatalf("legacy inventory changed: %s", got)
		}
		route(0, "POST", recordsURL, map[string]string{"projection": "saved-itineraries", "data": "mixed"}, 400)
		recommend := Decision{Purpose: "recommendation", Policy: 1, Recipient: "p09", Accept: &accepted,
			Expires: &expiry, Request: "36d6dac5-5ff1-45c6-98b3-dc054addd976"}
		call(0, "POST", recommend, 200)
		// Use the in-process records service for the injected travel catalog.
		remoteURL := serverURL
		serverURL = server.URL
		defer func() { serverURL = remoteURL }()
		visitStart := time.Now().UTC().Add(20 * time.Second)
		visitEnd := visitStart.Add(time.Second)
		zone := time.UTC
		if visitStart.Hour() >= 22 {
			zone, err = time.LoadLocation("Asia/Seoul")
			if err != nil {
				t.Fatal(err)
			}
		}
		budgetUnit, planningCost := "KRW", "3000"
		service.EnableCatalog(func(context.Context) (places.Catalog, error) {
			return places.Catalog{Version: 1, Places: []places.Place{{ID: "park", Name: "Park", Region: "seoul",
				Source: "approved fixture", UpdatedAt: now, PlanningUnit: &budgetUnit, PlanningCost: &planningCost,
				Opening: []places.OpeningInterval{{
					Weekday: int(visitStart.In(zone).Weekday()), Timezone: zone.String(), Opens: "00:00", Closes: "23:59",
				}},
			}}}, nil
		})
		previewPath := "/v1/private/itineraries/preview"
		applyPath := "/v1/private/itineraries/apply"
		preview := ApplyPlan{Plan: Plan{ID: "46d75cbd-ef9f-41bd-b5bc-ece3625e30b9", Consent: 1, Catalog: 1,
			Expires: expiry, Budget: "15000", Stops: []StopInput{{Place: "park", Start: visitStart, End: visitEnd}}}}
		var approved struct{ Confirmation string }
		if err := json.Unmarshal(route(0, "POST", previewPath, preview, 200), &approved); err != nil || approved.Confirmation == "" {
			t.Fatalf("current consent could not review preview: %v", err)
		}
		apply := preview
		apply.Confirm, apply.Request = approved.Confirmation, "0070036e-f4b1-4b90-be9d-0733454ff922"
		route(1, "POST", previewPath, preview, 404)
		serverURL = remoteURL

		const planID = "43a668b5-142a-4e04-b178-bd736c30779d"
		scope := subjects[0] + ":itinerary:" + planID
		plan := proposal{Plan: Plan{ID: planID, Stops: []StopInput{{Place: "park", Start: expiry}}}}
		plan.Details = []stopDetail{{Name: "중앙 공원"}}
		doc, err := json.Marshal(plan)
		if err != nil {
			t.Fatal(err)
		}
		sealed, err := service.content.seal(scope, doc)
		if err != nil {
			t.Fatal(err)
		}
		exec(`SELECT travel_private.apply_itinerary($1,$2,0,1,1,gen_random_uuid(),decode(repeat('bb',32),'hex'),$3,$4)`,
			subjects[0], planID, sealed, expiry)
		if got := route(0, "POST", recordsURL, optIn, 200); !bytes.Contains(got, []byte("중앙 공원")) ||
			bytes.Contains(got, []byte("stops")) {
			t.Fatalf("explicit caller missing minimized itinerary: %s", got)
		}
		route(0, "GET", "/v1/private/itineraries/"+planID, nil, 200)
		empty := func(identity int) {
			t.Helper()
			if got := route(identity, "POST", recordsURL, optIn, 200); string(bytes.TrimSpace(got)) != `{"itineraries":[]}` {
				t.Fatalf("unavailable saved plan projected: %s", got)
			}
		}
		empty(1)
		// This isolated test fence does not occupy the restore stream's next cursor.
		const projectionFence = int64(1 << 62)
		exec(`INSERT INTO travel_private.restore_fence(sequence,subject_ref,kind,object_kind,object_token,cutoff,expire_after)
			VALUES ($1,$2,'object','itinerary',$3,clock_timestamp(),clock_timestamp()+interval '91 days')`,
			projectionFence, subjects[0], planID)
		t.Cleanup(func() {
			_, err := admin.Exec(context.Background(), `DELETE FROM travel_private.restore_fence
				WHERE sequence=$1 AND subject_ref=$2 AND object_kind='itinerary' AND object_token=$3`,
				projectionFence, subjects[0], planID)
			if err != nil {
				t.Error(err)
			}
		})
		empty(0)
		route(0, "GET", "/v1/private/itineraries/"+planID, nil, 404)
		recommend.Accept, recommend.Expires, recommend.Expected, recommend.Request = &rejected, nil, 1, "6b92400c-85a1-429a-8550-e54a87cafd51"
		call(0, "POST", recommend, 200)
		empty(0)
		serverURL = server.URL
		route(0, "POST", previewPath, preview, 404)
		route(0, "POST", applyPath, apply, 404)
		rankingExpiry := time.Now().UTC().Add(55 * time.Minute).Truncate(time.Microsecond)
		recommend.Accept, recommend.Expires, recommend.Expected, recommend.Request = &accepted, &rankingExpiry, 2, "384a6d8a-908f-453f-a9f2-48955ea2b57a"
		call(0, "POST", recommend, 200)
		route(0, "POST", previewPath, preview, 404)
		route(0, "POST", applyPath, apply, 404)
		preview.Consent = 3
		route(0, "POST", previewPath, preview, 200)
		empty(0)
		// The provider sends a valid reply only after a second connection has withdrawn its consent.
		rankingResult := make(chan error, 1)
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			second, err := pgx.ConnectConfig(r.Context(), cfg)
			if err == nil {
				defer second.Close(context.Background())
				_, err = second.Exec(r.Context(), `SELECT travel_private.decide($1,'recommendation',3,1,'p09',NULL,false,NULL,gen_random_uuid())`, subjects[0])
			}
			rankingResult <- err
			reply(w, 200, map[string]any{"response": `{"order":["park"]}`, "done": true})
		}))
		defer provider.Close()
		ranker, err := OpenLocalRanker(provider.URL+"/api/generate", "test-model")
		if err != nil {
			t.Fatal(err)
		}
		service.EnableLocalRanker(ranker)
		generate := GeneratePlan{ID: preview.ID, Region: "seoul", Preference: "all", Consent: 3, Catalog: 1,
			Expires: rankingExpiry, Start: visitStart, End: visitStart.Add(45 * time.Minute), Budget: "15000", BudgetUnit: budgetUnit}
		route(0, "POST", "/v1/private/itineraries/generate", generate, 404)
		select {
		case err := <-rankingResult:
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("provider was not entered while consent was valid")
		}
		empty(0)
	})

	t.Run("ErasureReceiptHistorySubjectAndExpiry", func(t *testing.T) {
		const receipt = "e1b79702-1ff3-4431-9662-36d7d350b889"
		const expired = "b080a332-602f-4fd4-b0d4-862d20969347"
		const source = "f1417add-58bb-471d-b6f2-cb43c3e2c9c4"
		const older = "29a82980-632b-48e5-ad81-a745196ef87b"
		exec(`INSERT INTO travel_private.erasure_request(subject_ref,id,source_id,requested_at,journaled_at,this_device_erased_at) VALUES
			($1,$2,$3,statement_timestamp()-interval '89 days 23 hours 59 minutes',statement_timestamp(),statement_timestamp()),
			($1,$4,$5,statement_timestamp()-interval '90 days',statement_timestamp(),NULL)`,
			subjects[0], receipt, source, expired, older)
		t.Cleanup(func() {
			_, err := admin.Exec(context.Background(), `DELETE FROM travel_private.erasure_request WHERE subject_ref=$1 AND id IN ($2,$3)`, subjects[0], receipt, expired)
			if err != nil {
				t.Error(err)
			}
		})
		history := route(0, "GET", "/v1/private/erasure", nil, 200)
		if !bytes.Contains(history, []byte(receipt)) || bytes.Contains(history, []byte(expired)) ||
			bytes.Contains(history, []byte(source)) || bytes.Contains(history, []byte(subjects[0])) {
			t.Fatalf("receipt history exposed incorrect scope: %s", history)
		}
		if other := route(1, "GET", "/v1/private/erasure", nil, 200); string(bytes.TrimSpace(other)) != `{"receipts":[]}` {
			t.Fatalf("foreign receipt listed: %s", other)
		}
		route(1, "GET", "/v1/private/erasure/"+receipt, nil, 404)
		route(1, "POST", "/v1/private/erasure/"+receipt, map[string]bool{"localDeviceErased": true}, 404)
		route(0, "GET", "/v1/private/erasure/"+expired, nil, 404)
		route(0, "POST", "/v1/private/erasure/"+expired, map[string]bool{"localDeviceErased": true}, 200)
		route(0, "GET", "/v1/private/erasure/"+receipt, nil, 200)
		insertedAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
		exec(`INSERT INTO travel_private.erasure_request(subject_ref,id,source_id,requested_at)
			SELECT $1,gen_random_uuid(),gen_random_uuid(),$2 FROM generate_series(1,51)`, subjects[0], insertedAt)
		t.Cleanup(func() {
			_, err := admin.Exec(context.Background(), `DELETE FROM travel_private.erasure_request WHERE subject_ref=$1 AND requested_at=$2`, subjects[0], insertedAt)
			if err != nil {
				t.Error(err)
			}
		})
		var first struct {
			Receipts []struct {
				Request string `json:"request"`
			} `json:"receipts"`
			Next string `json:"next"`
		}
		if err := json.Unmarshal(route(0, "GET", "/v1/private/erasure", nil, 200), &first); err != nil ||
			len(first.Receipts) != 50 || first.Next != first.Receipts[49].Request {
			t.Fatalf("invalid erasure receipt cursor: %v %v", first, err)
		}
		cursor := map[string]string{"before": first.Next}
		second := route(0, "POST", "/v1/private/erasure", cursor, 200)
		if !bytes.Contains(second, []byte(receipt)) || bytes.Contains(second, []byte(first.Next)) {
			t.Fatalf("older receipt unavailable: %s", second)
		}
		if foreign := route(1, "POST", "/v1/private/erasure", cursor, 200); string(bytes.TrimSpace(foreign)) != `{"receipts":[]}` {
			t.Fatalf("foreign receipt cursor exposed history: %s", foreign)
		}
		route(0, "POST", "/v1/private/erasure", map[string]string{"before": first.Next, "object": source}, 400)
	})

	t.Run("SelectedErasureRequiresOriginalInstallationProof", func(t *testing.T) {
		const object = "45a0b087-78f2-4233-a0b3-204c817e0ecb"
		const request = "24de25d8-af21-43c2-9e8e-e3937fcc1d61"
		const otherRequest = "aafe6537-4b3f-467a-85f1-6cd759798d9f"
		defer func() {
			exec(`DELETE FROM travel_private.erasure_target WHERE subject_ref=$1 AND request_id=$2`, subjects[0], request)
			exec(`DELETE FROM travel_private.erasure_request WHERE subject_ref=$1 AND id=$2`, subjects[0], request)
			exec(`DELETE FROM travel_private.record WHERE subject_ref=$1 AND id=$2`, subjects[0], object)
		}()
		secret := strings.Repeat("42", 32)
		otherSecret := strings.Repeat("24", 32)
		exec(`INSERT INTO travel_private.erasure_request(subject_ref,id,source_id,requested_at,this_device_erased_at) VALUES
			($1,$2,$3,clock_timestamp(),clock_timestamp())`, subjects[0], otherRequest, object)
		defer exec(`DELETE FROM travel_private.erasure_request WHERE subject_ref=$1 AND id=$2`, subjects[0], otherRequest)
		legacy := route(8, "GET", "/v1/private/erasure/"+otherRequest, nil, 200)
		if !bytes.Contains(legacy, []byte(`"localDeviceEvidence":"subject-reported"`)) {
			t.Fatal("historical receipt misclassified")
		}
		exec(`UPDATE travel_private.erasure_request SET requested_at=clock_timestamp()-interval '91 days' WHERE id=$1`, otherRequest)
		route(8, "GET", "/v1/private/erasure/"+otherRequest, nil, 404)
		exec(`DELETE FROM travel_private.erasure_request WHERE id=$1`, otherRequest)
		capture := Decision{Purpose: "voice", Policy: 1, Recipient: "p09-sync", Accept: &accepted,
			Expires: &expiry, Request: "f773c244-d5fa-4fd2-8bb3-c0972d168621"}
		var current int64
		if err := admin.QueryRow(ctx, `SELECT coalesce((SELECT revision FROM travel_private.consent
			WHERE subject_ref=$1 AND purpose='voice'),0)`, subjects[0]).Scan(&current); err != nil {
			t.Fatal(err)
		}
		capture.Expected = current
		route(0, "POST", "/v1/private/consents", capture, 200)
		route(0, "POST", "/v1/private/records", map[string]any{"id": object, "purpose": "voice",
			"consentRevision": current + 1, "recipient": "p09-sync", "deleteAfter": expiry, "data": base64.StdEncoding.EncodeToString([]byte("\x00\x00\x00\x10ftypM4A \x00\x00\x00\x00"))}, 200)
		create := map[string]any{"object": object, "requestKey": request, "installationProof": secret}
		path := "/v1/private/erasure/" + request
		for range 2 {
			state := route(0, "POST", "/v1/private/erasure", create, 200)
			if !bytes.Contains(state, []byte(`"localDeviceEvidence":"none"`)) || bytes.Contains(state, []byte(secret)) {
				t.Fatal("confirmation is not pending or request capability was disclosed")
			}
		}
		route(8, "POST", "/v1/private/erasure", map[string]any{"object": object,
			"requestKey": otherRequest, "installationProof": otherSecret}, 200)
		route(8, "POST", path, map[string]any{"localDeviceErased": true, "installationProof": otherSecret}, 404)
		route(8, "POST", path, map[string]any{"localDeviceErased": true}, 404)
		route(1, "POST", path, map[string]any{"localDeviceErased": true, "installationProof": secret}, 404)
		exec(`UPDATE travel_private.erasure_request SET requested_at=clock_timestamp()-interval '91 days' WHERE id=$1`, request)
		route(8, "GET", path, nil, 404)
		for range 2 {
			state := route(0, "POST", path, map[string]any{"localDeviceErased": true, "installationProof": secret}, 200)
			if !bytes.Contains(state, []byte(`"localDeviceEvidence":"installation-reported"`)) || bytes.Contains(state, []byte(secret)) {
				t.Fatal("delayed local receipt missing or request capability was disclosed")
			}
		}
		route(8, "GET", path, nil, 404)
	})

	t.Run("PublishedGrantChangeRollsBackWaitingMutationOnSameTLSConnection", func(t *testing.T) {
		const requestID = "d6b61d57-2c22-4346-a3b2-11a10f89ce77"
		subject := subjects[1]
		digest := sha256.Sum256(clients[1].Certificate[0])
		key := hex.EncodeToString(digest[:])
		config := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		config.Certificates = []tls.Certificate{clients[1]}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: config}}
		defer client.CloseIdleConnections()
		send := func(method string, command []byte) (int, bool, error) {
			req, err := http.NewRequestWithContext(ctx, method, serverURL+"/v1/private/consents", bytes.NewReader(command))
			if err != nil {
				return 0, false, err
			}
			var reused bool
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
				GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
			}))
			res, err := client.Do(req)
			if err != nil {
				return 0, reused, err
			}
			defer res.Body.Close()
			_, err = io.Copy(io.Discard, res.Body)
			return res.StatusCode, reused, err
		}
		if status, reused, err := send(http.MethodGet, nil); status != 200 || reused || err != nil {
			t.Fatalf("original enrollment: status=%d reused=%v err=%v", status, reused, err)
		}
		lock, err := admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Rollback(context.Background())
		if _, err := lock.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1 || ':voice',900018))`, subject); err != nil {
			t.Fatal(err)
		}
		observer, err := pgx.ConnectConfig(ctx, cfg.Copy())
		if err != nil {
			t.Fatal(err)
		}
		defer observer.Close(context.Background())
		cmd, err := json.Marshal(Decision{Purpose: "voice", Policy: 1, Recipient: "p09-sync", Accept: &accepted,
			Expires: &expiry, Request: requestID})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			status, reused, err := send(http.MethodPost, cmd)
			if err == nil && (status != http.StatusNotFound || !reused) {
				err = fmt.Errorf("reassigned enrollment status=%d reused=%v", status, reused)
			}
			done <- err
		}()
		for {
			var waiting bool
			err := observer.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE usename=$1
				AND wait_event_type='Lock' AND query LIKE '%travel_private.decide%')`, login).Scan(&waiting)
			if err != nil {
				t.Fatal(err)
			}
			if waiting {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		changed := maps.Clone(grants)
		replacement := changed[key]
		replacement.Subject = subjects[0]
		changed[key] = replacement
		publishGrants(changed)
		defer publishGrants(grants)
		if err := lock.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		var persisted int
		err = observer.QueryRow(ctx, `SELECT count(*) FROM travel_private.consent_decision WHERE request_key=$1`, requestID).Scan(&persisted)
		if err != nil || persisted != 0 {
			t.Fatalf("withdrawn consent transaction had %d effects: %v", persisted, err)
		}
		delete(changed, key)
		publishGrants(changed)
		if status, reused, err := send(http.MethodGet, nil); status != 404 || !reused || err != nil {
			t.Fatalf("removed enrollment: status=%d reused=%v err=%v", status, reused, err)
		}
		publishGrantFixture(t, grantPath, []byte("{"))
		if status, reused, err := send(http.MethodGet, nil); status != 404 || !reused || err != nil {
			t.Fatalf("malformed enrollment: status=%d reused=%v err=%v", status, reused, err)
		}
	})

}
