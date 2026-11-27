package commerce

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresProtectedCloseCandidate(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a migrated isolated commerce database")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(config.Host, "/tmp/") || strings.HasPrefix(config.Host, "/private/tmp/")) {
		t.Fatal("requires disposable named socket database")
	}
	ctx, done := context.WithTimeout(t.Context(), 20*time.Second)
	defer done()
	db, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	run := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	id := func() string {
		t.Helper()
		var value string
		if err := db.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	branch, tenant, actor, device, stream, epoch := id(), id(), id(), id(), id(), id()
	proof, key, wrongKey := id(), id(), id()
	const login = "protected_close_075_fixture"
	const writerLogin = "protected_close_075_writer"
	const readerLogin = "protected_close_075_reader"
	const account = "0x1010101010101010101010101010101010101010"
	const destination = "0x2020202020202020202020202020202020202020"
	const boardSigner = "0x3030303030303030303030303030303030303030"
	const settlement = "0x4040404040404040404040404040404040404040"
	const signer = "0x5050505050505050505050505050505050505050"
	now := time.Now().UTC().Truncate(time.Second)
	until := now.Add(2 * time.Minute)
	fill := func(v byte) []byte { return bytes.Repeat([]byte{v}, 32) }
	writer, audience, measurement, challenge := fill(1), fill(2), fill(4), fill(5)
	cert := &x509.Certificate{Raw: []byte("synthetic verified TLS leaf"), NotBefore: now.Add(-time.Minute), NotAfter: until}
	collectorPin := sha256.Sum256(cert.Raw)
	collector := collectorPin[:]
	registeredAt := now.Add(-time.Minute)
	run("CREATE ROLE " + login + " LOGIN IN ROLE nu54_p05_rental_close")
	run("CREATE ROLE " + writerLogin + " LOGIN IN ROLE nu54_p05_rental_evidence")
	run("CREATE ROLE " + readerLogin + " LOGIN IN ROLE nu54_p05_rental_authority_read")
	run("INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	run("INSERT INTO ledger.login_subject VALUES('rental_075_fixture',$1)", actor)
	run("INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',$3)", branch, actor, until)
	run(`INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
		VALUES($1,$2,1,$3,$4,'qualified',$5,$6,$7)`, branch, device, account, destination, fill(6), until, registeredAt)
	run(`INSERT INTO ledger.rental_generation_source VALUES($1,$2,1,$3,$4,$5,'qualified',$6,$7,$8,$9,$10,$11,$12,$13,$14,$13)`,
		branch, device, fill(6), account, destination, measurement, boardSigner, settlement, fill(7), stream, epoch, challenge, until, writerLogin)
	run("INSERT INTO ledger.firmware_member VALUES($1,$2,'qualified',$3,$4,2,'0.0.2',false,NULL)", branch, device, fill(8), bytes.Repeat([]byte{4}, 65))
	run(`INSERT INTO ledger.firmware_campaign(branch_id,device_ref,campaign_ref,actor_id,release_digest,version,counter,measurement,
		challenge,challenge_until,state,proof_digest) VALUES($1,$2,$3,$4,$5,'0.0.2',2,$6,$7,$8,'verified',$9)`,
		branch, device, id(), actor, hex.EncodeToString(fill(10)), measurement, fill(11), until, fill(12))
	run(`INSERT INTO ledger.rental_collector_registration(branch_id,device_ref,generation,operator_ref,collector_certificate,
		collector_peer,first_nonce,registration_proof,authority_source,valid_until)
		SELECT $1,$2,1,$3,$4,$5,$6,$7,ledger.rental_generation_source_digest(s),$8 FROM ledger.rental_generation_source s WHERE s.branch_id=$1 AND s.device_ref=$2`,
		branch, device, actor, collector, []byte{1, 2, 3, 4, 5, 6, 7}, make([]byte, 32), fill(13), until)
	run(`INSERT INTO ledger.rental_authority_audience SELECT $1,$2,1,$3,$4,$5,ledger.rental_generation_source_digest(s),$6
		FROM ledger.rental_generation_source s WHERE s.branch_id=$1 AND s.device_ref=$2`, branch, device, readerLogin, audience, writer, until)
	run(`INSERT INTO ledger.rental_generation_authorized_proof(branch_id,device_ref,generation,request_key,request_digest,source_digest,enrollment_digest,
		challenge,freeze_digest,journal_root,first_nonce,next_nonce,payment_entries,limit_entries,burned_entries,stream_ref,stream_generation,
		finalized_height,finalized_hash,assessment_digest,frozen_at,finalized_at,recorded_at,fresh_until,pending_payments,pending_limits)
		SELECT $1,$2,1,$3,$4,ledger.rental_generation_source_digest(s),s.enrollment_digest,s.challenge,$5,$6,$7,$7,
		0,0,0,s.stream_ref,s.stream_generation,20,$8,$9,$10,$11,$11,$12,0,0
		FROM ledger.rental_generation_source s WHERE s.branch_id=$1 AND s.device_ref=$2`,
		branch, device, proof, fill(14), fill(15), fill(16), make([]byte, 32), fill(17), fill(18), now.Add(-20*time.Second), now.Add(-10*time.Second), until)
	loginWith := func(role string) *pgx.Conn {
		t.Helper()
		custom := config.Copy()
		custom.User = role
		conn, err := pgx.ConnectConfig(ctx, custom)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close(context.Background()) })
		return conn
	}
	store, parallel := loginWith(login), loginWith(login)
	if _, err := store.Exec(ctx, "SELECT * FROM ledger.rental_close_candidate"); err == nil {
		t.Fatal("executor can bypass bounded SQL functions")
	}
	args := []any{tenant, branch, actor, device, int64(1)}
	var plan struct{ ScopeDigest, ReviewRef, ProofKey string }
	scope := func() error {
		var raw []byte
		err := store.QueryRow(ctx, "SELECT ledger.rental_close_live_scope($1,$2,$3,$4,$5,$6)", append(args, writer)...).Scan(&raw)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &plan)
	}
	code := func(err error, want string) {
		t.Helper()
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != want {
			t.Fatalf("expected PostgreSQL %s, got %v", want, err)
		}
	}
	code(scope(), "P0501")
	run(`INSERT INTO ledger.rental_proof_submission VALUES($1,$2,1,$3,$4)`, branch, device, proof, fill(19))
	if err := scope(); err != nil || plan.ProofKey != proof {
		t.Fatalf("qualified snapshot missing: %+v %v", plan, err)
	}
	digest, err := hex.DecodeString(plan.ScopeDigest)
	if err != nil {
		t.Fatal(err)
	}
	hash, sealed := fill(20), bytes.Repeat([]byte{21}, 140)
	admit := func(conn *pgx.Conn, request string) (bool, error) {
		var inserted bool
		params := append(append([]any(nil), args...), request, plan.ReviewRef, writer, digest, time.Now().Unix()+4, signer, "5", hash, sealed)
		err := conn.QueryRow(ctx, `SELECT ledger.rental_close_admit($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::text::numeric,$13,$14)`, params...).Scan(&inserted)
		return inserted, err
	}
	run("UPDATE ledger.rental_authority_audience SET valid_until=$3 WHERE branch_id=$1 AND device_ref=$2", branch, device, now.Add(-time.Second))
	_, err = admit(store, key)
	code(err, "P0501")
	run("UPDATE ledger.rental_authority_audience SET valid_until=$3 WHERE branch_id=$1 AND device_ref=$2", branch, device, until)
	type outcome struct {
		inserted bool
		err      error
	}
	responses := make(chan outcome, 2)
	for _, conn := range []*pgx.Conn{store, parallel} {
		go func(c *pgx.Conn) { ok, err := admit(c, key); responses <- outcome{ok, err} }(conn)
	}
	a, b := <-responses, <-responses
	if a.err != nil || b.err != nil || a.inserted == b.inserted {
		t.Fatalf("one original candidate required: %+v %+v", a, b)
	}
	_, err = admit(store, wrongKey)
	code(err, "P0501")
	recoverOriginal := func(status string, expectSealed bool) {
		t.Helper()
		var actual string
		var h, data []byte
		err := parallel.QueryRow(ctx, `SELECT state,tx_hash,sealed_transaction FROM ledger.rental_close_recover($1,$2,$3,$4,$5,$6,$7)`,
			append(args, key, plan.ReviewRef)...).Scan(&actual, &h, &data)
		if err != nil || actual != status || !bytes.Equal(hash, h) || (expectSealed && !bytes.Equal(sealed, data)) || (!expectSealed && data != nil) {
			t.Fatalf("durable original differs for %s: %s %v", status, actual, err)
		}
	}
	recoverOriginal("prepared", true)
	mark := func(status string) error {
		var changed bool
		return store.QueryRow(ctx, `SELECT ledger.rental_close_mark($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			append(args, key, plan.ReviewRef, hash, status, writer, digest, time.Now().Unix()+4)...).Scan(&changed)
	}
	code(mark("confirmed"), "P0501")
	if err := mark("attempting"); err != nil {
		t.Fatal(err)
	}
	recoverOriginal("attempting", true)
	var locked bool
	if err := store.QueryRow(ctx, `SELECT ledger.rental_close_mark($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		append(args, key, plan.ReviewRef, hash, "attempting", writer, digest, time.Now().Unix()+4)...).Scan(&locked); err != nil || locked {
		t.Fatalf("parallel retry acquired the active dispatch lease: %v", err)
	}
	run("UPDATE ledger.rental_close_candidate SET attempt_until=now()-interval '1 second' WHERE branch_id=$1", branch)
	run("UPDATE ledger.rental_authority_audience SET valid_until=$3 WHERE branch_id=$1 AND device_ref=$2", branch, device, now.Add(-time.Second))
	code(mark("attempting"), "P0501")
	recoverOriginal("attempting", true)
	if err := mark("submitted"); err != nil {
		t.Fatal(err)
	}
	recoverOriginal("submitted", true)
	if err := mark("confirmed"); err != nil {
		t.Fatal(err)
	}
	recoverOriginal("confirmed", false)
	// Cleanup has a separate capability and cannot reinterpret a public legacy ACK.
	run("UPDATE ledger.rental_authority_audience SET valid_until=$3 WHERE branch_id=$1 AND device_ref=$2", branch, device, until)
	if err := scope(); err != nil {
		t.Fatalf("renew protected source: %v", err)
	}
	renewed := id()
	run(`UPDATE ledger.rental_generation_source SET challenge=$3 WHERE (branch_id,device_ref)=($1,$2)`, branch, device, fill(53))
	for _, table := range []string{"rental_collector_registration", "rental_authority_audience"} {
		run(`UPDATE ledger.`+table+` m SET authority_source=ledger.rental_generation_source_digest(s)
			FROM ledger.rental_generation_source s WHERE (m.branch_id,m.device_ref,m.generation)=(s.branch_id,s.device_ref,s.generation)
			AND (s.branch_id,s.device_ref)=($1,$2)`, branch, device)
	}
	run(`INSERT INTO ledger.rental_generation_authorized_proof
		(branch_id,device_ref,generation,request_key,request_digest,source_digest,enrollment_digest,challenge,freeze_digest,
		 journal_root,first_nonce,next_nonce,payment_entries,limit_entries,burned_entries,stream_ref,stream_generation,
		 finalized_height,finalized_hash,assessment_digest,frozen_at,finalized_at,recorded_at,fresh_until,pending_payments,pending_limits)
		SELECT p.branch_id,p.device_ref,p.generation,$3,$4,ledger.rental_generation_source_digest(s),p.enrollment_digest,s.challenge,$5,
		       p.journal_root,p.first_nonce,p.next_nonce,p.payment_entries,p.limit_entries,p.burned_entries,p.stream_ref,p.stream_generation,
		       p.finalized_height+1,$6,$7,p.frozen_at,p.finalized_at,now(),p.fresh_until,0,0
		FROM ledger.rental_generation_authorized_proof p JOIN ledger.rental_generation_source s USING(branch_id,device_ref,generation)
		WHERE (p.branch_id,p.device_ref)=($1,$2)`, branch, device, renewed, fill(54), fill(55), fill(56), fill(57))
	run(`INSERT INTO ledger.rental_proof_submission VALUES($1,$2,1,$3,$4)`, branch, device, renewed, fill(58))
	if err := scope(); err != nil || plan.ProofKey != renewed {
		t.Fatalf("renewed attestation for the same freeze unavailable: %v", err)
	}
	var frozen []byte
	cleanupFreeze := func() error {
		return store.QueryRow(ctx, "SELECT ledger.rental_cleanup_freeze($1,$2,$3,$4,$5,$6,$7,$8)",
			append(append([]any(nil), args...), writer, plan.ScopeDigest, hash)...).Scan(&frozen)
	}
	if err := cleanupFreeze(); err != nil || !bytes.Equal(frozen, fill(15)) {
		t.Fatalf("original freeze unavailable: %v", err)
	}
	for _, case_ := range []struct{ field, changed, original string }{
		{"journal_root", "sha256('another journal')", "decode(repeat('10',32),'hex')"},
		{"frozen_at", "frozen_at-interval '1 second'", "frozen_at+interval '1 second'"},
		{"first_nonce", "sha256('other nonce')", "decode(repeat('00',32),'hex')"},
		{"next_nonce", "sha256('other nonce')", "decode(repeat('00',32),'hex')"},
		{"pending_payments", "1", "0"},
		{"pending_limits", "1", "0"},
	} {
		t.Run("CleanupRejectsChanged_"+case_.field, func(t *testing.T) {
			if case_.field == "first_nonce" {
				// Match the storage CHECK while breaking both bounds of the original range.
				run("UPDATE ledger.rental_generation_authorized_proof SET first_nonce=sha256('other nonce'),next_nonce=sha256('other nonce') WHERE request_key=$1", renewed)
				code(cleanupFreeze(), "P0501")
				run("UPDATE ledger.rental_generation_authorized_proof SET first_nonce=decode(repeat('00',32),'hex'),next_nonce=decode(repeat('00',32),'hex') WHERE request_key=$1", renewed)
				return
			}
			run("UPDATE ledger.rental_generation_authorized_proof SET "+case_.field+"="+case_.changed+" WHERE request_key=$1", renewed)
			code(cleanupFreeze(), "P0501")
			run("UPDATE ledger.rental_generation_authorized_proof SET "+case_.field+"="+case_.original+" WHERE request_key=$1", renewed)
		})
	}
	if err := cleanupFreeze(); err != nil || !bytes.Equal(frozen, fill(15)) {
		t.Fatalf("original freeze did not survive negative checks: %v", err)
	}
	wrongGeneration := []any{tenant, branch, actor, device, int64(2)}
	code(store.QueryRow(ctx, "SELECT ledger.rental_cleanup_freeze($1,$2,$3,$4,$5,$6,$7,$8)",
		append(wrongGeneration, writer, plan.ScopeDigest, hash)...).Scan(&frozen), "42501")
	cleanup := func(conn *pgx.Conn, req string, freeze, disposition []byte) error {
		_, err := conn.Exec(ctx, "SELECT ledger.admit_rental_cleanup($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)",
			append(append([]any(nil), args...), req, writer, plan.ScopeDigest, hash, freeze, disposition, time.Now().Unix()+4)...)
		return err
	}
	cleanupKey, competingKey := id(), id()
	code(cleanup(store, cleanupKey, fill(99), fill(31)), "P0501")
	_, err = store.Exec(ctx, "SELECT ledger.admit_rental_cleanup($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)",
		append(append([]any(nil), args...), cleanupKey, writer, plan.ScopeDigest, hash, frozen, fill(31), time.Now().Unix()-1)...)
	code(err, "P0501")
	type cleanupOutcome struct {
		request string
		err     error
	}
	cleanupResults := make(chan cleanupOutcome, 2)
	go func() { cleanupResults <- cleanupOutcome{cleanupKey, cleanup(store, cleanupKey, frozen, fill(31))} }()
	go func() {
		cleanupResults <- cleanupOutcome{competingKey, cleanup(parallel, competingKey, frozen, fill(31))}
	}()
	x, y := <-cleanupResults, <-cleanupResults
	if x.err != nil {
		x, y = y, x
	}
	if x.err != nil {
		t.Fatal(x.err)
	}
	code(y.err, "P0501")
	if err := cleanup(store, x.request, fill(99), fill(32)); err != nil {
		t.Fatalf("same request should recover original disposition: %v", err)
	}
	var cleanupIdentity string
	var originalDisposition []byte
	if err := parallel.QueryRow(ctx, "SELECT ledger.rental_cleanup_pending($1,$2,$3,$4,$5)->>'requestKey'", args...).Scan(&cleanupIdentity); err != nil || cleanupIdentity != x.request {
		t.Fatalf("cleanup recovery differs: %v", err)
	}
	if err := db.QueryRow(ctx, "SELECT disposition FROM ledger.rental_cleanup_intent WHERE branch_id=$1", branch).Scan(&originalDisposition); err != nil || !bytes.Equal(originalDisposition, fill(31)) {
		t.Fatalf("cleanup changed original disposition: %v", err)
	}
	otherActor := id()
	run("INSERT INTO ledger.login_subject VALUES('rental_093_other_actor',$1)", otherActor)
	run("INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',$3)", branch, otherActor, until)
	otherArgs := []any{tenant, branch, otherActor, device, int64(1)}
	var status string
	var leaked *string
	if err := parallel.QueryRow(ctx, "SELECT s->>'state',s->>'requestKey' FROM ledger.rental_cleanup_pending($1,$2,$3,$4,$5) s", otherArgs...).Scan(&status, &leaked); err != nil || status != "board-receipt-required" || leaked != nil {
		t.Fatalf("another actor read the original cleanup: %v", err)
	}
	originalRetry, otherRetry := make(chan error, 1), make(chan error, 1)
	go func() { originalRetry <- cleanup(store, x.request, frozen, fill(31)) }()
	go func() {
		_, err := parallel.Exec(ctx, "SELECT ledger.admit_rental_cleanup($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)",
			append(otherArgs, x.request, writer, plan.ScopeDigest, hash, frozen, fill(31), time.Now().Unix()+4)...)
		otherRetry <- err
	}()
	if err := <-originalRetry; err != nil {
		t.Fatalf("original operator could not recover during handover: %v", err)
	}
	code(<-otherRetry, "P0501")
	run("DELETE FROM ledger.finance_grant WHERE branch_id=$1 AND actor_id=$2 AND capability='rental_manage'", branch, otherActor)
	code(parallel.QueryRow(ctx, "SELECT ledger.rental_cleanup_pending($1,$2,$3,$4,$5)", otherArgs...).Scan(&status), "42501")
	t.Run("CleanupIssuance", func(t *testing.T) {
		access := []any{tenant, branch, device, int64(1), x.request, collector}
		read := func(conn *pgx.Conn, inputs []any) (rentalCleanupIssueAuthority, error) {
			return readCleanupIssueAuthority(conn.QueryRow(ctx, cleanupIssueScopeSQL, inputs...))
		}
		_, err := read(store, access)
		code(err, "42501") // ordinary freeze collector
		run("INSERT INTO ledger.rental_cleanup_station VALUES($1,$2,1,$3,$4)", branch, device, collector, until)
		_, err = read(store, access)
		code(err, "P0501") // accepted before 095, original anchor unknown
		run("DELETE FROM ledger.rental_cleanup_intent WHERE branch_id=$1 AND device_ref=$2", branch, device)
		anchor := fill(53)
		const height = 20
		_, err = store.Exec(ctx, "SELECT ledger.admit_anchored_rental_cleanup($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)",
			append(append([]any(nil), args...), x.request, writer, plan.ScopeDigest, hash, frozen, fill(31), time.Now().Unix()+20, height, anchor)...)
		if err != nil {
			t.Fatal(err)
		}
		pinned, err := read(store, access)
		if err != nil || pinned.actor != actor || pinned.height != height || pinned.anchor != common.Hash(anchor) || pinned.board != common.HexToAddress(boardSigner) || pinned.envelope != nil {
			t.Fatalf("unexpected anchored cleanup scope: %v", err)
		}
		for _, change := range []struct {
			at    int
			value any
		}{{0, id()}, {1, id()}, {2, id()}, {3, int64(2)}, {4, id()}, {5, fill(9)}} {
			wrong := append([]any(nil), access...)
			wrong[change.at] = change.value
			_, err := read(store, wrong)
			code(err, "42501")
		}
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		proofScope := rentalv1.CleanupScope{ChainID: 8283, Settlement: common.HexToAddress(settlement), Operator: crypto.PubkeyToAddress(key.PublicKey),
			Board: pinned.board, Binding: [32]byte(fill(80)), Disposition: pinned.disposition, Request: [32]byte(fill(81))}
		issued := time.Now().Unix()
		envelope, err := signRentalCleanup(proofScope, key, uint64(issued), uint64(issued+20))
		if err != nil {
			t.Fatal(err)
		}
		pin := func(conn *pgx.Conn, proposal []byte, expiry int64) ([]byte, error) {
			var raw []byte
			err := conn.QueryRow(ctx, "SELECT ledger.pin_rental_cleanup_issuance($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)",
				append(append([]any(nil), access...), writer, plan.ScopeDigest, hash, frozen, pinned.disposition[:], proposal, expiry)...).Scan(&raw)
			return raw, err
		}
		_, err = pin(store, envelope, issued-1)
		code(err, "P0501")
		run("UPDATE ledger.rental_generation_authorized_proof SET pending_limits=1 WHERE request_key=$1", renewed)
		_, err = pin(store, envelope, issued+20)
		code(err, "P0501")
		run("UPDATE ledger.rental_generation_authorized_proof SET pending_limits=0 WHERE request_key=$1", renewed)
		type issuedResult struct {
			raw []byte
			err error
		}
		results := make(chan issuedResult, 2)
		for _, conn := range []*pgx.Conn{store, parallel} {
			go func(conn *pgx.Conn) {
				raw, err := pin(conn, envelope, issued+20)
				results <- issuedResult{raw, err}
			}(conn)
		}
		for range 2 {
			result := <-results
			if result.err != nil || !bytes.Equal(result.raw, envelope) {
				t.Fatalf("concurrent exact retry disagreed: %v", result.err)
			}
		}
		pinned, err = read(store, access)
		if err != nil || !bytes.Equal(pinned.envelope, envelope) {
			t.Fatalf("lost-ack replay changed durable envelope: %v", err)
		}
		run("UPDATE ledger.rental_cleanup_station SET valid_until=clock_timestamp() WHERE branch_id=$1 AND device_ref=$2", branch, device)
		_, err = read(store, access)
		code(err, "42501")
		run("UPDATE ledger.rental_cleanup_station SET valid_until=$3 WHERE branch_id=$1 AND device_ref=$2", branch, device, until)
		run("DELETE FROM ledger.finance_grant WHERE branch_id=$1 AND actor_id=$2 AND capability='rental_manage'", branch, actor)
		_, err = read(store, access)
		code(err, "42501")
		run("INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',$3)", branch, actor, until)
		t.Run("HTTPFinalWaitRevalidatesStation", func(t *testing.T) {
			roleConfig, err := pgxpool.ParseConfig(dsn)
			if err != nil {
				t.Fatal(err)
			}
			roleConfig.ConnConfig.User = login
			pool, err := pgxpool.NewWithConfig(ctx, roleConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			store := &postgresProtectedCloseStore{pool}
			req := protectedCloseRequest{Tenant: tenant, Branch: branch, Actor: actor, Device: device, Generation: 1, Key: x.request}
			plan, err := store.plan(ctx, req, [32]byte(writer))
			if err != nil {
				t.Fatal(err)
			}
			dep := core.Deployment{ChainID: 8283, Settlement: common.HexToAddress(settlement)}
			observed := core.ProtectedWithdrawalEvidence{State: "withdrawal-no-payout", CloseHash: common.Hash(hash),
				Amount: big.NewInt(0), FinalizedHeight: plan.Height, FinalizedHash: common.HexToHash(plan.Hash)}
			started, release := make(chan struct{}), make(chan struct{})
			chain := &cleanupRaceChain{proof: observed, started: started, release: release, operator: crypto.PubkeyToAddress(key.PublicKey)}
			controller := &ProtectedRentalClose{store: store, protectedCloseExecutor: &protectedCloseExecutor{
				store: store, chain: chain, anchor: cleanupFixtureAnchor{[32]byte(writer)}, operator: key,
				deployment: dep, manifest: [32]byte(fill(7)), now: time.Now,
			}}
			_, _, authentic, err := controller.cleanupDisposition(ctx, req, protectedWithdrawalApproval{plan: plan, evidence: observed})
			if err != nil {
				t.Fatal(err)
			}
			// Replace this disposable setup with an anchored request matching the observable terminal evidence.
			run("UPDATE ledger.rental_cleanup_intent SET disposition=$3,anchor_height=$4,anchor_hash=$5,signed_intent=NULL WHERE branch_id=$1 AND device_ref=$2",
				branch, device, authentic[:], observed.FinalizedHeight, observed.FinalizedHash[:])
			handler := HTTP{ProtectedClose: controller, ProtectedWithdrawal: true, Credentials: map[string]service.Principal{
				hex.EncodeToString(collector): {Tenant: tenant, Subject: actor, Role: "rental-collector", ExpiresAt: until.Unix()},
			}}
			mux := http.NewServeMux()
			handler.Register(mux)
			route := "/v2/private/rental-cleanup/" + branch + "/" + device + "/1/" + x.request + "/issue"
			call := func() *httptest.ResponseRecorder {
				req := httptest.NewRequestWithContext(ctx, http.MethodPost, route, nil)
				req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				return rec
			}
			response := make(chan *httptest.ResponseRecorder, 1)
			go func() { response <- call() }()
			select {
			case <-started:
			case out := <-response:
				t.Fatalf("station HTTP did not reach final external wait: %d %s", out.Code, out.Body)
			case <-ctx.Done():
				t.Fatal("station did not reach final external wait")
			}
			// The lock is available while external confirmation is stalled.
			short, cancel := context.WithTimeout(ctx, time.Second)
			_, lockErr := db.Exec(short, "SELECT ledger.lock_rental_evidence_device($1,$2)", branch, device)
			if lockErr == nil {
				_, lockErr = db.Exec(short, "DELETE FROM ledger.rental_cleanup_station WHERE branch_id=$1 AND device_ref=$2", branch, device)
			}
			cancel()
			close(release)
			out := <-response
			if lockErr != nil || out.Code != http.StatusNotFound || bytes.Contains(out.Body.Bytes(), []byte(`"intent"`)) {
				t.Fatalf("revoke during external wait: lock=%v HTTP=%d %s", lockErr, out.Code, out.Body)
			}
			run("INSERT INTO ledger.rental_cleanup_station VALUES($1,$2,1,$3,$4)", branch, device, collector, until)
			before, err := readCleanupIssueAuthority(pool.QueryRow(ctx, cleanupIssueScopeSQL, access...))
			if err != nil || len(before.envelope) != rentalv1.CleanupIntentSize {
				t.Fatalf("original committed envelope unavailable: %v", err)
			}
			original := call()
			var reply map[string]string
			if original.Code != http.StatusOK || json.Unmarshal(original.Body.Bytes(), &reply) != nil || reply["intent"] != hex.EncodeToString(before.envelope) {
				t.Fatalf("authorized retry lost original signed bytes: HTTP=%d %s", original.Code, original.Body)
			}
			controller.now = func() time.Time { return time.Now().Add(6 * time.Second) }
			out = call()
			if out.Code != http.StatusConflict || bytes.Contains(out.Body.Bytes(), []byte(`"intent"`)) {
				t.Fatalf("expired envelope was returned: HTTP=%d %s", out.Code, out.Body)
			}
		})

	})
	_, err = db.Exec(ctx, `INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until)
		VALUES($1,$2,2,$3,$4,'nu54_rev1',$5,$6)`, branch, device, account, destination, fill(45), until)
	code(err, "P0501")
	var publicCount int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.rental_operation WHERE branch_id=$1", branch).Scan(&publicCount); err != nil || publicCount != 0 {
		t.Fatal("staging enabled a public close")
	}
}

// The actual HTTP/DB boundary uses this controlled external adapter only to
// introduce an exact final-RPC pause and exercise authority races.
type cleanupRaceChain struct {
	protectedCloseChain
	protectedWithdrawalChain
	proof    core.ProtectedWithdrawalEvidence
	operator common.Address
	started  chan struct{}
	release  chan struct{}
	checks   int
}

func (c *cleanupRaceChain) ObserveProtectedWithdrawal(context.Context, common.Address, common.Address, common.Hash) (core.ProtectedWithdrawalEvidence, error) {
	return c.proof, nil
}
func (c *cleanupRaceChain) ProtectedRentalOperatorAt(ctx context.Context, height int64, hash common.Hash) (common.Address, error) {
	if !c.ProtectedWithdrawalAnchorCurrent(ctx, height, hash) {
		return common.Address{}, service.Conflict
	}
	return c.operator, nil
}

func (c *cleanupRaceChain) ProtectedWithdrawalAnchorCurrent(ctx context.Context, height int64, hash common.Hash) bool {
	c.checks++
	if c.checks == 2 {
		close(c.started)
		select {
		case <-c.release:
		case <-ctx.Done():
			return false
		}
	}
	return c.proof.FinalizedHeight == height && c.proof.FinalizedHash == hash
}

type cleanupFixtureAnchor struct{ fingerprint [32]byte }

func (a cleanupFixtureAnchor) CallerFingerprint() [32]byte { return a.fingerprint }
func (a cleanupFixtureAnchor) ConfirmCloseAnchor(_ context.Context, req rentalv1.CloseAnchorRequest) (rentalv1.CloseAnchorConfirmation, error) {
	return rentalv1.CloseAnchorConfirmation{CloseAnchorRequest: req, Finalized: req.Previous, ExpiresAt: time.Now().Unix() + 5}, nil
}
