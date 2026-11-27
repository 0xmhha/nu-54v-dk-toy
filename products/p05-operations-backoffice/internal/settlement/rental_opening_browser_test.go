package settlement

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func verifyRentalOpeningBrowser(t *testing.T, endpoint string, chain *core.Chain, client *ethclient.Client, operator *ecdsa.PrivateKey, deployment core.Deployment, dist string, protected bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	db, dsn := scratchDB(t)
	_, commerceDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	branch, tenant, staff, device := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	account, destination := fixtureSigner(113).Address(), fixtureSigner(114).Address()
	if protected {
		account, destination = fixtureSigner(115).Address(), fixtureSigner(116).Address()
	}
	fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES($1,$2)", "rental_"+staff, staff)
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',now()+interval '1 hour'),($1,$2,'legacy_ledger',now()+interval '1 hour')", branch, staff)
	attested := time.Now().Add(-5 * time.Minute).Truncate(time.Second).UTC()
	fixtureExec(t, db, `INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,
		revision,enrollment_digest,attested_until,no_pending_until,attested_at)
		VALUES($1,$2,17,$3,$4,'nu54_rev1',sha256('fixture generation 17'),now()+interval '1 hour',now()+interval '1 hour',$5)`,
		branch, device, lower(account), lower(destination), attested)
	if _, err := chain.MintTestToken(ctx, operator, crypto.PubkeyToAddress(operator.PublicKey), big.NewInt(25)); err != nil {
		t.Fatal(err)
	}
	mine(t, ctx, client, 5)

	dir := t.TempDir()
	write := func(name string, raw []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	pemFile := func(name, kind string, raw []byte) string {
		return write(name, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: raw}))
	}
	settings := func(name string, value any) string {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return name + "=" + write(name, raw)
	}
	certFiles := func(name string, cert tls.Certificate) (string, string) {
		t.Helper()
		key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		return pemFile(name+".crt", "CERTIFICATE", cert.Certificate[0]), pemFile(name+".key", "PRIVATE KEY", key)
	}
	_, certificates, ca := mtlsClients(t, 1)
	serverCert, serverKey := certFiles("server", newNativeLoopbackTLS(t))
	clientCert, clientKey := certFiles("operator", certificates[0])
	manifest, err := json.Marshal(map[string]any{"chainId": 8283, "contracts": map[string]any{
		"PaymentSettlement": map[string]any{"address": deployment.Settlement, "block": deployment.SettlementBlock},
		"MerchantRegistry":  map[string]any{"address": deployment.Registry},
		"TestUSDC":          map[string]any{"address": deployment.Token},
	}})
	if err != nil {
		t.Fatal(err)
	}
	password := fixtureUUID(t, db)
	keys := keystore.NewKeyStore(filepath.Join(dir, "fixture-key"), keystore.LightScryptN, keystore.LightScryptP)
	operatorAccount, err := keys.ImportECDSA(operator, password)
	if err != nil {
		t.Fatal(err)
	}
	cashoutKey, err := crypto.ToECDSA(append(make([]byte, 31), 118))
	if err != nil {
		t.Fatal(err)
	}
	cashoutAccount, err := keys.ImportECDSA(cashoutKey, password)
	if err != nil {
		t.Fatal(err)
	}
	signer := os.Getenv("P05_SETTLEMENT_TEST_SIGNER")
	signerProgram, err := os.ReadFile(signer)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	origin := "https://" + addr
	serviceEnv := []string{
		"P05_HTTP_ADDR=" + addr, "TMPDIR=" + dir, "P05_COMMERCE_DATABASE_URL=" + commerceDSN,
		"P05_COMMERCE_CA=" + pemFile("ca", "CERTIFICATE", ca), "P05_COMMERCE_CERT=" + serverCert, "P05_COMMERCE_KEY=" + serverKey,
		settings("P05_COMMERCE_GRANTS", map[string]service.Principal{fingerprint(certificates[0]): {
			Tenant: tenant, Subject: staff, Role: "operator", ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}}),
		settings("P05_COMMERCE_PROFILES", map[string]any{branch: map[string]any{
			"chainId": 8283, "contract": deployment.Settlement, "asset": deployment.Token, "beneficiary": destination,
			"signer": fixtureSigner(101).Address(), "merchantId": strings.Repeat("ab", 32), "keyVersion": 1, "beneficiaryVersion": 1,
		}}),
		"P05_QUOTE_STORAGE_KEY_FILE=" + write("storage", []byte(strings.Repeat("42", 32))),
		"P05_QUOTE_COMMITMENT_KEY_FILE=" + write("commit", []byte(strings.Repeat("31", 32))),
		"P05_QUOTE_SIGNER=" + signer, fmt.Sprintf("P05_QUOTE_SIGNER_SHA256=%x", sha256.Sum256(signerProgram)),
		"P05_OPERATOR_ORIGIN=" + origin, "P05_CONSOLE_DIR=" + dist,
		"P05_V1_RENTAL_RPC=" + endpoint, "P05_V1_RENTAL_MANIFEST=" + write("manifest", manifest),
		fmt.Sprintf("P05_V1_RENTAL_MANIFEST_SHA256=%x", sha256.Sum256(manifest)), "P05_V1_RENTAL_KEYSTORE=" + operatorAccount.URL.Path,
		"P05_V1_RENTAL_SECRET_REF=file:" + write("password", []byte(password)),
		settings("P05_V1_LEGACY_MERCHANTS", map[string]common.Address{branch: destination}),
		"P05_V1_CASHOUT_KEYSTORE=" + cashoutAccount.URL.Path,
		"P05_V1_CASHOUT_SECRET_REF=file:" + filepath.Join(dir, "password"),
	}
	var cmd *exec.Cmd
	var logs bytes.Buffer
	start := func() {
		t.Helper()
		cmd = exec.CommandContext(ctx, os.Getenv("P05_SETTLEMENT_TEST_OPSD"))
		cmd.Env = serviceEnv
		cmd.Stderr = &logs
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, func() bool {
			conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
			if err != nil {
				return false
			}
			conn.Close()
			return true
		})
	}
	stop := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	start()
	defer func() {
		stop()
		if t.Failed() {
			t.Log(logs.String())
		}
	}()
	browser := func(phase, request, hash, block string) {
		t.Helper()
		command := exec.CommandContext(ctx, "node", "../../test/finance/rental-opening-browser.cjs",
			origin, branch, device, lower(account), lower(destination), phase, clientCert, clientKey,
			attested.Format(time.RFC3339), request, hash, block)
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("opening Chrome %s: %v\n%s", phase, err, out)
		}
		t.Logf("opening Chrome %s: %s", phase, out)
	}
	checkActions := func(opens int) {
		t.Helper()
		var count, openingCount int
		err := db.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE action='open')
			FROM ledger.rental_operation WHERE branch_id=$1 AND device_ref=$2 AND generation=17`,
			branch, device).Scan(&count, &openingCount)
		if err != nil || count != opens || openingCount != opens {
			t.Fatalf("device review moved commands: total=%d opens=%d want=%d err=%v", count, openingCount, opens, err)
		}
	}
	if !protected {
		browser("start-unopened", "", "", "")
		checkActions(0)
		actual, err := chain.AccountOf(ctx, account)
		if err != nil || actual.Exists || actual.Balance == nil || actual.Balance.Sign() != 0 {
			t.Fatalf("opening review cancellation changed the account: %+v %v", actual, err)
		}
		fixtureExec(t, db, `UPDATE ledger.rental_enrollment SET revision='fixture-unlisted',attested_at=NULL
			WHERE branch_id=$1 AND device_ref=$2 AND generation=17`, branch, device)
		browser("start-legacy", "", "", "")
		fixtureExec(t, db, `UPDATE ledger.rental_enrollment SET revision='nu54_rev1',attested_at=$3
			WHERE branch_id=$1 AND device_ref=$2 AND generation=17`, branch, device, attested)
		checkActions(0)
	}
	browser("review", "", "", "")
	var request, hash, amount string
	err = db.QueryRow(ctx, `SELECT request_key::text,evidence->>'submitted',amount::text FROM ledger.rental_operation
		WHERE branch_id=$1 AND device_ref=$2 AND generation=17 AND action='open' AND status='submitted'`, branch, device).
		Scan(&request, &hash, &amount)
	if err != nil || amount != "25" {
		t.Fatalf("wrong observed opening: %s %v", amount, err)
	}
	mine(t, ctx, client, 5)
	block, err := chain.RentalOpeningBlock(ctx, common.HexToHash(hash), account, destination, big.NewInt(25))
	if err != nil || block == nil {
		t.Fatalf("opening did not reach actual finality: %+v %v", block, err)
	}
	browser("confirmed", request, hash, block.Hash.Hex())
	if !protected {
		browser("start-active", request, hash, block.Hash.Hex())
		checkActions(1)
	}
	actual, err := chain.AccountOf(ctx, account)
	if err != nil || !actual.Exists || actual.WithdrawAddress != destination || actual.Balance.String() != "25" {
		t.Fatalf("same generation did not receive one exact deposit: %+v %v", actual, err)
	}
	t.Logf("same request %s and transaction %s finalized in block %s at %d", request, hash, block.Hash.Hex(), block.Height)

	if protected {
		verifyProtectedReturnBrowser(t, ctx, db, dsn, client, deployment, attested, branch, device, account, destination, browser)
	} else {
		fixtureExec(t, db, "UPDATE ledger.rental_enrollment SET no_pending_until=now()-interval '1 second' WHERE branch_id=$1 AND device_ref=$2", branch, device)
		browser("return-blocked", "", "", "")
		var closeOps int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM ledger.rental_operation
		WHERE branch_id=$1 AND device_ref=$2 AND action='close'`, branch, device).Scan(&closeOps); err != nil || closeOps != 0 {
			t.Fatalf("held return created a command: %d %v", closeOps, err)
		}
		fixtureExec(t, db, "UPDATE ledger.rental_enrollment SET no_pending_until=now()+interval '1 hour' WHERE branch_id=$1 AND device_ref=$2", branch, device)
		clearanceSync := filepath.Join(dir, "clearance-review")
		changed := make(chan error, 1)
		go func() {
			for ctx.Err() == nil {
				if _, err := os.Stat(clearanceSync + ".reviewed"); err == nil {
					_, err := db.Exec(ctx, `UPDATE ledger.rental_enrollment
					SET no_pending_until=no_pending_until+interval '1 hour'
					WHERE branch_id=$1 AND device_ref=$2 AND generation=17`, branch, device)
					if err == nil {
						err = os.WriteFile(clearanceSync+".changed", nil, 0600)
					}
					changed <- err
					return
				}
				time.Sleep(25 * time.Millisecond)
			}
			changed <- ctx.Err()
		}()
		browser("return-stale", "", "", clearanceSync)
		if err := <-changed; err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `SELECT count(*) FROM ledger.rental_operation
		WHERE branch_id=$1 AND device_ref=$2 AND action='close'`, branch, device).Scan(&closeOps); err != nil || closeOps != 0 {
			t.Fatalf("stale reviewed return created a command: %d %v", closeOps, err)
		}

		browser("return-close", "", "", "")
	}

	var closeKey, closeHash string
	if err := db.QueryRow(ctx, `SELECT request_key::text,evidence->>'submitted'
		FROM ledger.rental_operation WHERE branch_id=$1 AND device_ref=$2 AND generation=17
		AND action='close' AND status='submitted'`, branch, device).Scan(&closeKey, &closeHash); err != nil {
		t.Fatalf("original close was not retained: %v", err)
	}
	mine(t, ctx, client, 5)
	closed, err := chain.AccountOf(ctx, account)
	if err != nil || closed.ClosedAt == 0 || closed.WithdrawAddress != destination || closed.WithdrawAfter == 0 {
		t.Fatalf("same v1 account did not close: %+v %v", closed, err)
	}
	forceTime(t, ctx, client, int64(closed.WithdrawAfter)+1)
	browser("return-withdrawal", closeKey, "", "")
	var withdrawalKey, withdrawalHash string
	if err := db.QueryRow(ctx, `SELECT request_key::text,evidence->>'submitted'
		FROM ledger.rental_operation WHERE branch_id=$1 AND device_ref=$2 AND generation=17
		AND action='withdraw' AND status='submitted'`, branch, device).Scan(&withdrawalKey, &withdrawalHash); err != nil {
		t.Fatalf("original withdrawal was not retained: %v", err)
	}
	mine(t, ctx, client, 5)
	withdrawal, proven, err := chain.RentalWithdrawalProof(ctx, account, destination)
	if err != nil || !proven || withdrawal.Hex() != withdrawalHash {
		t.Fatalf("confirmed withdrawal does not match the stored operation: %s %t %v", withdrawal, proven, err)
	}
	stop()
	start()
	browser("return-resumed", closeKey, withdrawalHash, closeHash)
	var sent, cleanups int
	if err := db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='submitted'),count(*) FILTER(WHERE action='cleanup')
		FROM ledger.rental_operation WHERE branch_id=$1 AND device_ref=$2 AND generation=17`, branch, device).Scan(&sent, &cleanups); err != nil || sent != 3 || cleanups != 0 {
		t.Fatalf("expected one open, close and withdrawal and no implicit device cleanup: %d %d %v", sent, cleanups, err)
	}
	t.Logf("same v1 generation close %s/%s and withdrawal %s/%s confirmed; device cleanup was not dispatched", closeKey, closeHash, withdrawalKey, withdrawalHash)
}

func verifyProtectedReturnBrowser(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn string,
	client *ethclient.Client, deployment core.Deployment, attested time.Time, branch, device string,
	account, destination common.Address, browser func(string, string, string, string)) {
	t.Helper()
	// Test-only authenticated input to the 070 writer. The browser talks to the normal opsd.
	writerName, writerDSN := testLogin(t, db, dsn, "nu54_p05_rental_evidence")
	writer, err := pgx.Connect(ctx, writerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close(context.Background())
	sum := func(n byte) []byte { return bytes.Repeat([]byte{n}, 32) }
	nonce := func(n byte) []byte { return append(make([]byte, 31), n) }
	stream, streamGeneration := fixtureUUID(t, db), fixtureUUID(t, db)
	registration := sha256.Sum256([]byte("fixture generation 17"))
	boardSigner := lower(fixtureSigner(117).Address())
	fixtureExec(t, db, `INSERT INTO ledger.rental_generation_source
	    (branch_id,device_ref,generation,enrollment_digest,account_ref,withdraw_ref,enrollment_revision,board_measurement,
	     board_signer_ref,settlement_ref,deployment_digest,stream_ref,stream_generation,challenge,challenge_until,writer_login,valid_until)
	    VALUES($1,$2,17,$3,$4,$5,'nu54_rev1',$6,$7,$8,$9,$10,$11,$12,clock_timestamp()+interval '120 seconds',$13,
		clock_timestamp()+interval '180 seconds')`,
		branch, device, registration[:], lower(account), lower(destination), sum(2), boardSigner, lower(deployment.Settlement), sum(3),
		stream, streamGeneration, sum(10), writerName)
	initial, err := client.HeaderByNumber(ctx, big.NewInt(-3))
	if err != nil {
		t.Fatal(err)
	}
	frozenAt := time.Unix(int64(initial.Time), 0).Add(-time.Second)
	if frozenAt.Before(attested) {
		t.Fatalf("test freeze predates enrollment %s/%s", frozenAt, attested)
	}
	var priorHeight any
	var priorHash any
	writeProof := func(challenge byte, payments, limits int, fresh time.Duration) {
		t.Helper()
		if challenge != 10 {
			fixtureExec(t, db, `UPDATE ledger.rental_generation_source SET challenge=$3,challenge_until=clock_timestamp()+interval '110 seconds'
	          WHERE branch_id=$1 AND device_ref=$2 AND generation=17`, branch, device, sum(challenge))
		}
		mine(t, ctx, client, 5)
		h, err := client.HeaderByNumber(ctx, big.NewInt(-3))
		if err != nil {
			t.Fatal(err)
		}
		finalTime := time.Unix(int64(h.Time), 0)
		if wait := time.Until(finalTime); wait > 0 {
			if wait > 15*time.Second {
				t.Fatal("unexpected future local chain time")
			}
			time.Sleep(wait + 20*time.Millisecond)
		}
		now := time.Now().UTC()
		var saved bool
		err = writer.QueryRow(ctx, `SELECT ledger.record_rental_generation_proof($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
	        $11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)`,
			branch, device, int64(17), fixtureUUID(t, db), registration[:], sum(2), boardSigner, lower(deployment.Settlement), sum(3),
			stream, streamGeneration, sum(challenge), sum(challenge+1), sum(6), sum(challenge+2), nonce(0), nonce(2),
			1, 1, 0, payments, limits, h.Number.Int64(), h.Hash().Bytes(), frozenAt, now.Add(85*time.Second),
			finalTime, now.Add(fresh), priorHeight, priorHash).Scan(&saved)
		if err != nil || !saved {
			t.Fatalf("scoped fixture %d/%d not recorded: %v", payments, limits, err)
		}
		priorHeight = h.Number.Int64()
		priorHash = h.Hash().Bytes()
	}
	zeroCloses := func() {
		t.Helper()
		var count int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM ledger.rental_operation WHERE branch_id=$1 AND device_ref=$2
	        AND generation=17 AND action='close'`, branch, device).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unexpected close %d %v", count, err)
		}
	}
	writeProof(10, 1, 1, 60*time.Second)
	browser("qualified-two", "", "", "")
	zeroCloses()
	writeProof(20, 1, 0, 18*time.Second)
	browser("qualified-one", "", "", "")
	zeroCloses()
	writeProof(30, 0, 0, 60*time.Second)
	browser("qualified-zero", "", "", "")
}
