package settlement

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/test/settlement/w10fixture"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoopbackFinanceOriginalOrder(t *testing.T) {
	loopbackFinanceAttempt(t, false, false, sponsorAdmissionOnly)
}

func TestLoopbackFinanceRevertedEvidence(t *testing.T) {
	t.Setenv("P05_PILOT_HUMAN_UNITS", "1")
	loopbackFinanceAttempt(t, true, false, sponsorAdmissionOnly)
}

func TestLoopbackMerchantGasSponsorship(t *testing.T) {
	loopbackFinanceAttempt(t, false, true, sponsorAdmissionOnly)
}

type sponsoredScenario uint8

const (
	sponsorAdmissionOnly sponsoredScenario = iota
	sponsorBoardSettlement
	sponsorInstalledFinalizer
)

func TestLoopbackSponsoredBoardPayment(t *testing.T) {
	t.Setenv("P05_PILOT_HUMAN_UNITS", "1")
	loopbackFinanceAttempt(t, false, true, sponsorBoardSettlement)
}

func TestLoopbackW10InstalledFinalizer(t *testing.T) {
	t.Setenv("P05_PILOT_HUMAN_UNITS", "1")
	loopbackFinanceAttempt(t, false, true, sponsorInstalledFinalizer)
}

func loopbackFinanceAttempt(t *testing.T, reject, sponsorsOnly bool, sponsoredMode sponsoredScenario) {
	t.Helper()
	sponsoredBoard := sponsoredMode != sponsorAdmissionOnly
	settlementEndpoint(t)
	indexer, opsd := os.Getenv("P05_SETTLEMENT_TEST_INDEXER"), os.Getenv("P05_SETTLEMENT_TEST_OPSD")
	if opsd == "" {
		t.Skip("requires test/settlement/anvil.sh")
	}
	// The shared chain may have advanced past the real PostgreSQL clock in earlier cases.
	endpoint := freshSettlementChain(t)
	ctx, cancel := context.WithTimeout(t.Context(), 85*time.Second)
	defer cancel()
	db, dsn := scratchDB(t)
	chain, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Close()
	deploymentKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	gasAccount := crypto.PubkeyToAddress(deploymentKey.PublicKey)
	target, deployed, token, contract, _ := deployFixtureWithOperator(t, ctx, chain, db, deploymentKey, false)
	var approved []*fixtureDeploymentApproval
	if reject || sponsorsOnly {
		approved = append(approved, &fixtureDeploymentApproval{})
	}
	stopIndexer, _, _, _ := startFixtureIndexer(t, ctx, db, dsn, endpoint, indexer, target, deployed, approved...)
	defer stopIndexer()
	beneficiary := fixtureSigner(104).Address()
	refusal := func(refuse bool) {
		t.Helper()
		key, err := crypto.ToECDSA(append(make([]byte, 31), 104))
		if err != nil {
			t.Fatal(err)
		}
		if err := chain.Client().CallContext(ctx, nil, "anvil_setBalance", beneficiary, "0x56BC75E2D63100000"); err != nil {
			t.Fatal(err)
		}
		operator, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
		if err != nil {
			t.Fatal(err)
		}
		operator.Context = ctx
		transaction, err := token.SetRefusalMode(operator, refuse)
		if err != nil {
			t.Fatal(err)
		}
		result, err := bind.WaitMined(ctx, chain, transaction)
		if err != nil || result.Status != 1 {
			t.Fatalf("token refusal fixture failed: %v", err)
		}
	}
	amount := pilotUnits(2)
	if reject {
		refusal(true)
		amount = "6500000"
	}
	var issued time.Time
	if err := db.QueryRow(ctx, "SELECT date_trunc('second',clock_timestamp())").Scan(&issued); err != nil {
		t.Fatal(err)
	}
	buyer := fixtureSigner(102)
	order := seedPayerOrder(t, db, target, issued, amount, 98, buyer, 1)
	// Preserve historical rows. The dispatcher skips unrelated jobs in the shared test database.
	otherWork, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer otherWork.Rollback(context.Background())
	if _, err := otherWork.Exec(ctx, `SELECT w.id FROM ledger.settlement_work w
		JOIN ledger.sale s ON s.id=w.sale_id WHERE s.branch_id<>$1 AND NOT w.finished FOR UPDATE OF w`, order.Branch); err != nil {
		t.Fatal(err)
	}
	quoteID := common.Hash(order.Order.QuoteID).Hex()
	merchant := common.Hash(order.Quote.Args.MerchantId).Hex()
	reader, foreign := fixtureUUID(t, db), fixtureUUID(t, db)
	otherBranch := fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", otherBranch, order.Tenant)
	for _, grant := range [][2]string{{order.Branch, reader}, {otherBranch, foreign}} {
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'report_read',now()+interval '1 hour')", grant[0], grant[1])
	}
	if reject {
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'reconcile_read',now()+interval '1 hour')", order.Branch, reader)
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'reconcile_read',now()+interval '1 hour')", otherBranch, foreign)
	}
	evidenceLogin, evidenceDSN := testLogin(t, db, dsn, "nu54_p07_reconcile")
	fixtureExec(t, db, "INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '1 hour')", evidenceLogin, target.Stream, merchant)
	_, commerceDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	_, financeDSN := testLogin(t, db, dsn, "nu54_p05_finance")
	_, workerDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	workerStore, err := Open(ctx, workerDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer workerStore.Close()
	proof, err := OpenEvidence(ctx, evidenceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer proof.Close()
	qCipher, _ := commerce.NewCipher(order.CipherKey[:])
	relayKey := [32]byte{31: 95}
	relayCipher, _ := commerce.NewCipher(relayKey[:])
	relay := &lostResponseChain{LiveChain: LiveChain{Client: chain}}
	worker := Worker{Store: workerStore, Evidence: proof, Chain: relay, Targets: map[string]Target{order.Branch: target},
		QuoteCipher: qCipher, RelayCipher: relayCipher, GasAccount: gasAccount, GasSigner: receiptGasSigner{deploymentKey},
		RefundSigner: fixtureSigner(103), MaxGasPrice: big.NewInt(100_000_000_000), Now: time.Now}
	holdWorker := func() {
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now()+interval '1 day' WHERE id=$1", order.Sale)
	}
	step := func() error {
		t.Helper()
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now() WHERE id=$1", order.Sale)
		worked, err := worker.Step(ctx)
		if !worked {
			t.Fatal("settlement worker missed the original order")
		}
		holdWorker()
		return err
	}
	holdWorker()

	_, certificates, ca := mtlsClients(t, 3)
	entity := []service.Principal{
		{Role: "customer", Tenant: order.Tenant, Subject: order.Actor},
		{Role: "finance", Tenant: order.Tenant, Subject: reader},
		{Role: "finance", Tenant: order.Tenant, Subject: foreign},
	}
	var unsponsored seededOrder
	if sponsorsOnly {
		entity[0].Subject, entity[0].Role = fixtureUUID(t, db), "terminal"
		unsponsored = seedPayerOrder(t, db, target, issued, amount, 99, buyer, 1,
			seededOrder{Tenant: order.Tenant, Branch: otherBranch})
		for _, original := range []seededOrder{order, unsponsored} {
			fixtureExec(t, db, "UPDATE ledger.sale SET revision=1 WHERE id=$1 AND state='issued'", original.Sale)
			fixtureExec(t, db, `INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,terminal_mode,valid_until)
				VALUES($1,$2,'terminal','pos',clock_timestamp()+interval '2 minutes')`, original.Branch, entity[0].Subject)
			fixtureExec(t, db, `INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,expires_at,intent_digest)
				VALUES($1,$2,$3,$4,$5)`, original.Sale, original.Branch, entity[0].Subject, original.Order.Expires, original.Order.Intent[:])
			fixtureExec(t, db, `INSERT INTO ledger.payer_enrollment VALUES($1,$2,$3,1,1,$4)`,
				original.Branch, original.Actor, original.Order.Payer.Bytes(), original.Order.Expires)
			fixtureExec(t, db, `INSERT INTO ledger.checkout_claim VALUES($1,$2,$3,$4,$5,1,1,$6)`,
				original.Sale, original.Branch, original.Actor, original.Order.Claim[:], original.Order.Payer.Bytes(), original.Order.Expires)
			fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now()+interval '1 day' WHERE id=$1", original.Sale)
		}
	}

	for i := range entity {
		entity[i].ExpiresAt = time.Now().Add(time.Hour).Unix()
	}
	serverTLS := newNativeLoopbackTLS(t)
	dir := t.TempDir()
	write := func(name string, raw []byte, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	pemBlock := func(kind string, data []byte) []byte {
		return pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: data})
	}
	writeKey := func(name string, cert tls.Certificate) string {
		t.Helper()
		key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		return write(name, pemBlock("PRIVATE KEY", key), 0600)
	}
	setting := func(name string, payload any) string {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return name + "=" + write(name, raw, 0600)
	}
	browserCert := write("finance.pem", pemBlock("CERTIFICATE", certificates[1].Certificate[0]), 0600)
	browserKey := writeKey("finance-key.pem", certificates[1])
	issuerProfile := map[string]any{
		"chainId": 8283, "contract": target.Contract, "asset": target.Asset, "beneficiary": fixtureSigner(104).Address(),
		"signer": fixtureSigner(101).Address(), "merchantId": rawHex(order.Quote.Args.MerchantId[:]), "keyVersion": 1, "beneficiaryVersion": 1,
	}
	profiles := map[string]any{order.Branch: issuerProfile}
	if sponsorsOnly {
		profiles[otherBranch] = issuerProfile
	}

	config := []string{
		"P05_COMMERCE_CA=" + write("ca.pem", pemBlock("CERTIFICATE", ca), 0600),
		"P05_COMMERCE_CERT=" + write("server.pem", pemBlock("CERTIFICATE", serverTLS.Certificate[0]), 0600),
		"P05_COMMERCE_KEY=" + writeKey("server-key.pem", serverTLS),
		"P05_COMMERCE_DATABASE_URL=" + commerceDSN, "P05_FINANCE_DATABASE_URL=" + financeDSN,
		"P05_SETTLEMENT_DATABASE_URL=" + workerDSN, "P05_P07_RECONCILIATION_DATABASE_URL=" + evidenceDSN,
		"P05_SETTLEMENT_RPC=" + endpoint, "P05_RELAY_ADDRESS=" + gasAccount.Hex(),
		"P05_RELAY_MAX_GAS_PRICE=100000000000",
		setting("P05_SETTLEMENT_TARGETS", map[string]Target{order.Branch: target, otherBranch: target}),
		setting("P05_COMMERCE_GRANTS", map[string]service.Principal{fingerprint(certificates[0]): entity[0]}),
		setting("P05_FINANCE_GRANTS", map[string]service.Principal{
			fingerprint(certificates[1]): entity[1], fingerprint(certificates[2]): entity[2],
		}),
		setting("P05_COMMERCE_PROFILES", profiles),
	}
	for name, key := range map[string][32]byte{
		"P05_QUOTE_COMMITMENT_KEY_FILE": {31: 9}, "P05_QUOTE_STORAGE_KEY_FILE": order.CipherKey,
		"P05_SETTLEMENT_STORAGE_KEY_FILE": relayKey, "P05_PRESENTATION_KEY_FILE": {31: 11},
	} {
		config = append(config, name+"="+write(name, []byte(rawHex(key[:])), 0600))
	}
	signer := os.Getenv("P05_SETTLEMENT_TEST_SIGNER")
	for name, program := range map[string][]byte{
		"P05_QUOTE_SIGNER":  fixtureSignerProgram(signer, 101),
		"P05_RELAY_SIGNER":  []byte(fmt.Sprintf("#!/bin/sh\nexport GOMAXPROCS=1\nexec %q %x\n", signer, crypto.FromECDSA(deploymentKey))),
		"P05_REFUND_SIGNER": fixtureSignerProgram(signer, 103),
	} {
		digest := sha256.Sum256(program)
		config = append(config, name+"="+write(name, program, 0700), name+"_SHA256="+rawHex(digest[:]))
	}
	if reject || sponsorsOnly {
		config = append(config, "P05_SETTLEMENT_DEPLOYMENT_PROFILES="+approved[0].Profiles,
			"P05_SETTLEMENT_DEPLOYMENT_SIGNATURES="+approved[0].Signatures,
			"P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY="+approved[0].Key)
	}
	dist := os.Getenv("NU54_ORDER_DETAIL_DIST")
	if reject {
		dist = os.Getenv("NU54_DEPLOYMENT_EVIDENCE_DIST")
	}
	if dist != "" {
		config = append(config, "P05_CONSOLE_DIR="+dist)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	origin := "https://" + addr
	roots := x509.NewCertPool()
	for _, der := range serverTLS.Certificate {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		roots.AddCert(cert)
	}
	clients := make([]*http.Client, len(certificates))
	for i, cert := range certificates {
		transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}}
		t.Cleanup(transport.CloseIdleConnections)
		clients[i] = &http.Client{Transport: transport, Timeout: 2 * time.Second}
	}
	start := func(overrides ...string) func() {
		t.Helper()
		var output bytes.Buffer
		cmd := exec.CommandContext(ctx, opsd)
		cmd.Env = append([]string{"P05_HTTP_ADDR=" + addr, "P05_OPERATOR_ORIGIN=" + origin}, config...)
		cmd.Env = append(cmd.Env, overrides...)
		cmd.Env = append(cmd.Env, "TMPDIR="+dir)
		cmd.Stderr = &output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			stopped = true
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			for _, client := range clients {
				client.CloseIdleConnections()
			}
			if t.Failed() {
				t.Logf("opsd: %s", &output)
			}
		}
		t.Cleanup(stop)
		waitUntil(t, func() bool {
			reply, err := clients[1].Get(origin + "/healthz")
			if err != nil {
				return false
			}
			reply.Body.Close()
			return reply.StatusCode == http.StatusOK
		})
		return stop
	}
	post := func(who int, path string, payload any, want int) []byte {
		t.Helper()
		raw, _ := json.Marshal(payload)
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		reply, err := clients[who].Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		body, err := io.ReadAll(reply.Body)
		if err != nil || reply.StatusCode != want {
			t.Fatalf("%s unexpectedly returned %d: %s (%v)", path, reply.StatusCode, body, err)
		}
		return body
	}
	lookup := map[string]string{"branchId": order.Branch, "saleId": order.Sale}
	orderURL := "/v2/finance/order"
	read := func(hash string, observed, confirmed bool) {
		t.Helper()
		raw := post(1, orderURL, lookup, 200)
		var row struct {
			SaleID, QuoteID, FundsStatus, FulfillmentStatus, Amount string
			PaidAt, Transaction                                     *string
			ObservedHeight, ConfirmedHeight                         *int64
			Events                                                  []json.RawMessage
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			t.Fatal(err)
		}
		if row.SaleID != order.Sale || row.QuoteID != quoteID || row.Amount != amount ||
			(hash == "") != (row.Transaction == nil) || (row.Transaction != nil && *row.Transaction != hash) ||
			observed != (row.ObservedHeight != nil) || confirmed != (row.ConfirmedHeight != nil) {
			t.Fatalf("wrong order correlation: %s", raw)
		}
		if row.ObservedHeight != nil {
			t.Logf("original order %s quote %s hash %s: inclusion height %d, funds %s, fulfillment %s",
				row.SaleID, row.QuoteID, hash, *row.ObservedHeight, row.FundsStatus, row.FulfillmentStatus)
		}
		if confirmed {
			if row.FundsStatus != "confirmed" || row.FulfillmentStatus != "recorded" || row.PaidAt == nil ||
				len(row.Events) != 1 || *row.ConfirmedHeight < *row.ObservedHeight {
				t.Fatalf("wrong paid order: %s", raw)
			}
			t.Logf("P07 confirmation anchor height %d", *row.ConfirmedHeight)
		} else if row.FundsStatus != "held" || row.FulfillmentStatus != "held" || row.PaidAt != nil || len(row.Events) != 0 {
			t.Fatalf("unconfirmed order released funds or fulfillment: %s", raw)
		}
		for _, forbidden := range []string{"_chain", "orderKey", "quoteDigest", "cipher", "payer", "approval", lower(buyer.Address())} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Fatalf("operator order response disclosed %q", forbidden)
			}
		}
	}
	browser := func(phase, hash string) {
		t.Helper()
		if dist == "" {
			return
		}
		script := "../../test/finance/order-detail-browser.cjs"
		if reject {
			script = "../../test/finance/deployment-evidence-browser.cjs"
		}
		cmd := exec.CommandContext(ctx, "node", script, origin, order.Branch, order.Sale,
			browserCert, browserKey, quoteID, phase, hash)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("desktop Chrome %s: %v\n%s", phase, err, out)
		}
		t.Logf("desktop Chrome %s: %s", phase, out)
	}
	if sponsorsOnly {
		grant := GasSponsorGrant{Branch: order.Branch, Chain: 8283, Contract: target.Contract, Asset: target.Asset,
			Manifest: target.Manifest, Merchant: common.Hash(order.Quote.Args.MerchantId), Relay: gasAccount,
			StartsAt: issued.Add(-time.Minute).Unix(), ExpiresAt: issued.Add(time.Minute).Unix()}
		grantBody, valid := GasSponsorGrantMessage(grant)
		if !valid {
			t.Fatal("could not encode sponsor enrollment")
		}
		grant.Signature = rawHex(ed25519.Sign(approved[0].Signer, grantBody))
		published := setting("P05_MERCHANT_GAS_SPONSORS", []GasSponsorGrant{grant})
		stop := start(published)
		defer stop()
		issue := func(item seededOrder) commerce.Presentation {
			t.Helper()
			command := map[string]any{"version": 2, "contract": "C3", "operation": "quote.issue",
				"domainRef": item.Branch, "objectRef": item.Sale, "requestId": fixtureUUID(t, db), "idempotencyKey": item.Sale,
				"expiresAt": time.Now().Add(25 * time.Second).Unix(),
				"body":      map[string]string{"claimDigest": rawHex(item.Order.Claim[:]), "orderRevision": "0"}}
			data := post(0, "/v2/commerce", map[string]any{"command": command}, 200)
			var response struct {
				Quote struct {
					Signed       string                `json:"signedQuote"`
					Digest       string                `json:"quoteDigest"`
					Presentation commerce.Presentation `json:"presentation"`
				} `json:"quote"`
			}
			if err := json.Unmarshal(data, &response); err != nil ||
				response.Quote.Signed != rawHex(item.Signed) || response.Quote.Digest != rawHex(item.Quote.Digest[:]) ||
				response.Quote.Presentation.Customer != item.Actor {
				t.Fatalf("the original quote could not be recovered through the enrolled terminal: %v", err)
			}
			return response.Quote.Presentation
		}
		terminalCert := write("gas-terminal.pem", pemBlock("CERTIFICATE", certificates[0].Certificate[0]), 0600)
		terminalKey := writeKey("gas-terminal-key.pem", certificates[0])
		root := write("gas-loopback-roots.pem", pemBlock("CERTIFICATE", serverTLS.Certificate[1]), 0600)
		nearby := func(sponsored bool, forward map[string]string) {
			t.Helper()
			presentationKey := ed25519.NewKeyFromSeed(append(make([]byte, 31), 11)).Public().(ed25519.PublicKey)
			var originalOrigin string
			if err := db.QueryRow(ctx, "SELECT origin_ref::text FROM ledger.sale WHERE id=$1", order.Sale).Scan(&originalOrigin); err != nil {
				t.Fatal(err)
			}
			input, err := json.Marshal(map[string]any{
				"endpoint": origin + "/v2/commerce", "ca": root, "cert": terminalCert, "key": terminalKey,
				"tenant": order.Tenant, "terminal": entity[0].Subject, "branch": order.Branch, "customer": order.Actor,
				"sale": order.Sale, "origin": originalOrigin, "signed": rawHex(order.Signed), "sponsored": sponsored,
				"forward": forward,
				"profile": map[string]any{"chain": 8283, "contract": lower(target.Contract), "asset": lower(target.Asset),
					"beneficiary": lower(fixtureSigner(104).Address()), "merchantId": rawHex(order.Quote.Args.MerchantId[:]),
					"signer": lower(fixtureSigner(101).Address()), "keyVersion": 1, "beneficiaryVersion": 1,
					"maxBaseUnits": "30000000", "presentationKey": rawHex(presentationKey)},
			})
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, "node", "--experimental-transform-types", "../../test/settlement/w10-gas-nearby.mts")
			cmd.Stdin = bytes.NewReader(input)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("nearby terminal could not project managed sponsorship: %v\n%s", err, out)
			}
			t.Log(strings.TrimSpace(string(out)))
		}

		proof := issue(order).GasSponsor
		digest := sha256.Sum256(grantBody)
		if proof == nil || proof.Account != lower(gasAccount) || proof.Grant != rawHex(digest[:]) ||
			proof.ExpiresAt <= time.Now().Unix() || proof.ExpiresAt > time.Now().Add(30*time.Second).Unix() {
			t.Fatal("the live enrolled deployment did not publish its bounded gas sponsorship")
		}
		nearby(true, nil)
		if issue(unsponsored).GasSponsor != nil {
			t.Fatal("a different issued branch retained the gas sponsorship")
		}
		raceGasSponsorPin(t, ctx, commerceDSN, entity[0].Subject, unsponsored, grant)
		path := strings.TrimPrefix(published, "P05_MERCHANT_GAS_SPONSORS=")
		publish := func(grants []GasSponsorGrant) {
			t.Helper()
			raw, err := json.Marshal(grants)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+".new", raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path+".new", path); err != nil {
				t.Fatal(err)
			}
		}
		publish([]GasSponsorGrant{})
		if issue(order).GasSponsor != nil {
			t.Fatal("a removed grant was accepted without restarting opsd")
		}
		nearby(false, nil)
		var approval []byte
		if sponsoredBoard {
			w10BoardDecision(t, ctx, unsponsored, buyer, true)
			approval = w10BoardDecision(t, ctx, order, buyer, false)
		} else {
			approval, err = buyer.SignDigest(ctx, order.Quote.ApprovalHash)
			if err != nil {
				t.Fatal(err)
			}
		}
		key := fixtureUUID(t, db)
		forward := func(key string, status int) {
			t.Helper()
			command := map[string]any{"version": 2, "contract": "C3", "operation": "payment.forward",
				"domainRef": order.Branch, "objectRef": order.Sale, "requestId": fixtureUUID(t, db), "idempotencyKey": key,
				"expiresAt": time.Now().Add(25 * time.Second).Unix(),
				"body":      map[string]string{"quoteDigest": rawHex(order.Quote.Digest[:]), "approvalDigest": rawHex(order.Quote.ApprovalHash[:])},
			}
			body := post(0, "/v2/commerce", map[string]any{"command": command, "approvalSignature": rawHex(approval)}, status)
			if status == 400 && !bytes.Contains(body, []byte("UNSUPPORTED_CAPABILITY")) {
				t.Fatal("the previously unsubmitted approval did not reach the gas admission check")
			}
		}
		counts := func(submissions int) {
			t.Helper()
			var accepted, paid, relayed int
			err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
				(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
				(SELECT count(*) FROM ledger.relay_attempt WHERE work_id=$1)`, order.Sale).Scan(&accepted, &paid, &relayed)
			if err != nil || accepted != submissions || paid != 0 || relayed != 0 {
				t.Fatalf("unexpected original settlement effects: %d/%d/%d %v", accepted, paid, relayed, err)
			}
		}
		forward(key, 400)
		counts(0)
		replacement := grant
		replacement.ExpiresAt += 10
		replacementBody, valid := GasSponsorGrantMessage(replacement)
		if !valid {
			t.Fatal("invalid same-merchant replacement control")
		}
		replacement.Signature = rawHex(ed25519.Sign(approved[0].Signer, replacementBody))
		publish([]GasSponsorGrant{replacement})
		if issue(order).GasSponsor != nil {
			t.Fatal("a new grant replaced the same quote's pinned commitment")
		}
		forward(key, 400)
		counts(0)
		t.Log("actual opsd mTLS: verified 8283 deployment and enrolled merchant fee authority, original quote bytes unchanged, other branch, revoked and replacement grants denied, 0 approvals/payments/relays")
		publish([]GasSponsorGrant{grant})
		if sponsoredBoard {
			nearby(true, map[string]string{"key": key, "signature": rawHex(approval)})
		} else {
			forward(key, 200)
		}
		publish([]GasSponsorGrant{})
		forward(key, 200)
		forward(fixtureUUID(t, db), 409)
		counts(1)
		t.Log("the one pre-withdrawal submission recovered by its original key after withdrawal; 0 payments and relay attempts")
		if sponsoredBoard {
			publish([]GasSponsorGrant{grant})
			// Keep the original work held until the mTLS daemon and its
			// background ticker have exited. The fault-injected worker is
			// the sole claimant when the fixture makes this work eligible.
			stop()
			var held bool
			if err := db.QueryRow(ctx, `SELECT fence=0 AND lease_until IS NULL AND retry_at>clock_timestamp()
				FROM ledger.settlement_work WHERE id=$1 AND sale_id=$1 AND refund_id IS NULL`, order.Sale).
				Scan(&held); err != nil || !held {
				t.Fatalf("original sponsor work changed before worker handoff: %v", err)
			}
			counts(1)
			var hash string
			if sponsoredMode == sponsorInstalledFinalizer {
				worker.Chain = LiveChain{Client: chain}
				driver := w10fixture.Settlement{DB: db, Chain: chain, Step: worker.Step,
					Profile: commerce.Profile{Contract: target.Contract, Asset: target.Asset, Beneficiary: beneficiary},
					Payer:   order.Order.Payer, Sponsor: gasAccount}
				hash, err = driver.Finalize(t, ctx, order.Sale, order.Quote.Args.Amount)
				if err != nil {
					t.Fatal("installed finalizer did not settle the exact host control")
				}
			} else {
				hash = verifyW10SponsoredSettlement(t, ctx, chain, db, token, contract, order, gasAccount, relay, step)
			}
			// The earlier cross-branch control intentionally shared one
			// deployment merchant. Finance withholds heights for that
			// ambiguous scope. Admit only the original profile for the read.
			stop = start(published, setting("P05_COMMERCE_PROFILES", map[string]any{order.Branch: issuerProfile}))
			defer stop()
			read(hash, true, true)
			post(2, orderURL, lookup, 404)
			t.Log("W10 SW1 finance: original paid transaction read by the enrolled finance identity; another branch denied")
		}
		return
	}

	stop := start()
	read("", false, false)
	post(2, orderURL, lookup, 404)
	post(2, orderURL, map[string]string{"branchId": otherBranch, "saleId": order.Sale}, 404)
	approval, err := buyer.SignDigest(ctx, order.Quote.ApprovalHash)
	if err != nil {
		t.Fatal(err)
	}
	command := map[string]any{"version": 2, "contract": "C3", "operation": "payment.forward", "domainRef": order.Branch, "objectRef": order.Sale,
		"requestId": fixtureUUID(t, db), "idempotencyKey": fixtureUUID(t, db),
		"body": map[string]string{"quoteDigest": rawHex(order.Quote.Digest[:]), "approvalDigest": rawHex(order.Quote.ApprovalHash[:])},
	}
	forward := func(status int) {
		t.Helper()
		command["expiresAt"] = time.Now().Add(30 * time.Second).Unix()
		raw := post(0, "/v2/commerce", map[string]any{"command": command, "approvalSignature": rawHex(approval)}, status)
		if status != 200 && bytes.Contains(raw, []byte(`"EXPIRED"`)) {
			t.Fatal("deployment validation was masked by an expired commerce envelope")
		}
	}
	if reject {
		evidence := func(want string) map[string]any {
			t.Helper()
			raw := post(1, "/v2/finance/payment-evidence", lookup, 200)
			var response map[string]any
			if err := json.Unmarshal(raw, &response); err != nil || response["status"] != want {
				t.Fatalf("wrong operator payment evidence: %s %v", raw, err)
			}
			return response
		}
		deploymentResponse := post(1, "/v2/finance/deployment-profile", map[string]string{"branchId": order.Branch}, 200)
		var enrolled DeploymentReview
		if err := json.Unmarshal(deploymentResponse, &enrolled); err != nil || enrolled.Status != "verified" ||
			enrolled.StartHash != deployed.BlockHash.Hex() || enrolled.BeneficiaryVersion == nil || *enrolled.BeneficiaryVersion != 1 {
			t.Fatalf("wrong signed deployment profile: %s %v", deploymentResponse, err)
		}
		post(2, "/v2/finance/deployment-profile", map[string]string{"branchId": order.Branch}, 404)
		browser("profile", "")
		stop()
		bad := setting("P05_SETTLEMENT_DEPLOYMENT_SIGNATURES", map[string]string{approved[0].Profile.GenerationID: rawHex(make([]byte, 64))})
		stop = start(bad)
		forward(400)
		if rejected := evidence("invalid"); rejected["revertedAttempt"] != nil || rejected["amount"] != nil || rejected["attempt"] != nil {
			t.Fatalf("invalid profile disclosed unverified effects: %v", rejected)
		}
		browser("invalid", "")
		stop()
		key, signingKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		for name, change := range map[string]func(*deployment.Profile){
			"chain":    func(profile *deployment.Profile) { profile.ChainID++ },
			"contract": func(profile *deployment.Profile) { profile.Contract = common.HexToAddress("0x1234") },
			"ABI":      func(profile *deployment.Profile) { profile.ABIHash = rawHex(make([]byte, 32)) },
		} {
			invalid := approved[0].Profile
			change(&invalid)
			encoded, _ := json.Marshal(invalid)
			signature := ed25519.Sign(signingKey, append([]byte("NU54-chainview-profile-v1\x00"), encoded...))
			stop = start(setting("P05_SETTLEMENT_DEPLOYMENT_PROFILES", []deployment.Profile{invalid}),
				setting("P05_SETTLEMENT_DEPLOYMENT_SIGNATURES", map[string]string{invalid.GenerationID: rawHex(signature)}),
				"P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY="+rawHex(key))
			forward(400)
			evidence("invalid")
			stop()
			t.Logf("signed mismatched %s blocked without financial evidence", name)
		}
		var submissions int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1", order.Sale).Scan(&submissions); err != nil || submissions != 0 {
			t.Fatalf("invalid deployment submitted: %d %v", submissions, err)
		}
		stop()
		stop = start()
	}
	forward(200)
	stop()
	if err := step(); err != nil {
		t.Fatal(err)
	}
	stop = start()
	read("", false, false)
	if !reject {
		browser("signed", "")
	}
	stop()
	var beforeGas *big.Int
	if reject {
		beforeGas, err = chain.BalanceAt(ctx, gasAccount, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := chain.Client().CallContext(ctx, nil, "evm_setAutomine", false); err != nil {
		t.Fatal(err)
	}
	if err := step(); !errors.Is(err, errLostResponse) {
		t.Fatalf("expected lost RPC response after one real send: %v", err)
	}
	var hash string
	if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1 AND state='unknown'", order.Sale).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	stop = start()
	read("", false, false)
	withoutRevertEvidence := func() {
		t.Helper()
		raw := post(1, "/v2/finance/payment-evidence", lookup, 200)
		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil || result["status"] != "pending" || result["amount"] != "6500000" {
			t.Fatalf("unconfirmed order evidence: %s %v", raw, err)
		}
		for _, field := range []string{"revertedAttempt", "feeWei", "principalDelta", "orderConsumed", "transaction"} {
			if _, exists := result[field]; exists {
				t.Fatalf("unconfirmed attempt exposed %s: %s", field, raw)
			}
		}
	}
	if reject {
		if _, err := chain.TransactionReceipt(ctx, common.HexToHash(hash)); !errors.Is(err, ethereum.NotFound) {
			t.Fatalf("expected no receipt for the unmined relay: %v", err)
		}
		withoutRevertEvidence()
		t.Log("unmined relay receipt unavailable; operator has no fee, principal, or consumption evidence")
	}
	head, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Give the mined payment a timestamp inside the quote's real validity window.
	inclusion := max(int64(head.Time)+1, issued.Unix()+1)
	if err := chain.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", inclusion); err != nil {
		t.Fatal(err)
	}
	mine(t, ctx, chain, 1)
	if reject {
		withoutRevertEvidence()
		t.Log("mined but unfinalized relay still has no financial effect evidence")
		receipt, err := chain.TransactionReceipt(ctx, common.HexToHash(hash))
		if err != nil || receipt.Status != types.ReceiptStatusFailed {
			t.Fatalf("actual relay did not revert: %v", err)
		}
		afterGas, err := chain.BalanceAt(ctx, gasAccount, nil)
		if err != nil {
			t.Fatal(err)
		}
		cost := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
		if new(big.Int).Sub(beforeGas, afterGas).Cmp(cost) != 0 {
			t.Fatal("reverted gas debit differs from receipt")
		}
		balance, err := token.BalanceOf(nil, beneficiary)
		booked, bookErr := contract.Payments(nil, order.Quote.Args.MerchantId, order.Quote.Args.OrderId)
		if err != nil || bookErr != nil || balance.Sign() != 0 || booked.Amount.Sign() != 0 {
			t.Fatalf("reverted call moved principal or consumed the order: %v %v %v %v", balance, booked, err, bookErr)
		}
		mine(t, ctx, chain, 5)
		waitUntil(t, func() bool {
			var final bool
			err := proof.db.QueryRow(ctx, "SELECT chainview.reconciliation_finalized_block($1,$2,$3,$4)",
				target.Stream, merchant, receipt.BlockNumber.Int64(), receipt.BlockHash.Hex()).Scan(&final)
			return err == nil && final
		})
		verify := func(status, paid string) {
			t.Helper()
			raw := post(1, "/v2/finance/payment-evidence", lookup, 200)
			var result struct {
				Status, Transaction, Amount string
				RevertedAttempt             *RevertedAttempt
			}
			if err := json.Unmarshal(raw, &result); err != nil || result.Status != status || result.Amount != "6500000" || result.Transaction != paid {
				t.Fatalf("unexpected O14 view: %s %v", raw, err)
			}
			failure := result.RevertedAttempt
			if failure == nil || failure.Transaction != hash || failure.GasUsed != receipt.GasUsed ||
				failure.EffectivePriceWei != receipt.EffectiveGasPrice.String() || failure.FeeWei != cost.String() ||
				failure.GasPayer != lower(gasAccount) || failure.PrincipalDelta != "0" || failure.OrderConsumed {
				t.Fatalf("original revert proof differs: %s", raw)
			}
		}
		verify("reverted-attempt", "")
		post(2, "/v2/finance/payment-evidence", lookup, 404)
		post(2, "/v2/finance/payment-evidence", map[string]string{"branchId": otherBranch, "saleId": order.Sale}, 404)
		t.Log("finance reader enrolled at the other branch cannot read the original order under either branch")
		browser("reverted", hash)
		if err := chain.Client().CallContext(ctx, nil, "evm_setAutomine", true); err != nil {
			t.Fatal(err)
		}
		refusal(false)
		operator, err := bind.NewKeyedTransactorWithChainID(deploymentKey, big.NewInt(8283))
		if err != nil {
			t.Fatal(err)
		}
		operator.Context = ctx
		paidTx, err := contract.Settle(operator, order.Quote.Args, order.Quote.MerchantSig, approval)
		if err != nil {
			t.Fatal(err)
		}
		paidReceipt, err := bind.WaitMined(ctx, chain, paidTx)
		if err != nil || paidReceipt.Status != 1 {
			t.Fatalf("independent same-order settlement failed: %v", err)
		}
		mine(t, ctx, chain, 5)
		waitUntil(t, func() bool {
			var final bool
			err := db.QueryRow(ctx, "SELECT EXISTS (SELECT FROM chainview.finalized_receipt WHERE tx_hash=$1 AND kind='paid')", paidTx.Hash().Hex()).Scan(&final)
			return err == nil && final
		})
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now() WHERE id=$1", order.Sale)
		waitUntil(t, func() bool {
			var paid bool
			err := db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM ledger.payment WHERE sale_id=$1 AND tx_hash=$2)", order.Sale, paidTx.Hash().Hex()).Scan(&paid)
			return err == nil && paid
		})
		verify("verified", paidTx.Hash().Hex())
		balance, err = token.BalanceOf(nil, beneficiary)
		booked, bookErr = contract.Payments(nil, order.Quote.Args.MerchantId, order.Quote.Args.OrderId)
		if err != nil || bookErr != nil || balance.Cmp(order.Quote.Args.Amount) != 0 || booked.Amount.Cmp(order.Quote.Args.Amount) != 0 {
			t.Fatalf("later payment did not settle the exact original amount: %v %v %v %v", balance, booked, err, bookErr)
		}
		var payments int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.payment WHERE sale_id=$1", order.Sale).Scan(&payments); err != nil || payments != 1 {
			t.Fatalf("expected exactly one payment for the original order: %d %v", payments, err)
		}
		t.Logf("later payment credited %s base units to the beneficiary, consumed the order and produced %d ledger payment", balance, payments)
		browser("paid", hash)
		t.Logf("one original %s reverted %s, gas=%d x %s=%s wei; independent success %s", order.Sale, hash, receipt.GasUsed, receipt.EffectiveGasPrice, cost, paidTx.Hash())
		stop()
		return
	}
	waitUntil(t, func() bool {
		var seen bool
		err := db.QueryRow(ctx, "SELECT EXISTS (SELECT FROM chainview.observed_event WHERE tx_hash=$1 AND kind='paid')", hash).Scan(&seen)
		return err == nil && seen
	})
	read(hash, true, false)
	browser("observed", hash)
	stop()
	stop = start()
	read(hash, true, false)
	mine(t, ctx, chain, 4)
	waitUntil(t, func() bool {
		var ready bool
		err := db.QueryRow(ctx, "SELECT EXISTS (SELECT FROM chainview.finalized_receipt WHERE tx_hash=$1 AND kind='paid')", hash).Scan(&ready)
		return err == nil && ready
	})
	read(hash, true, false) // chain finality cannot release an unbooked order
	var before int
	err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.payment WHERE sale_id=$1)
		+ (SELECT count(*) FROM ledger.journal WHERE sale_id=$1)
		+ (SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$1)`, order.Sale).Scan(&before)
	if err != nil || before != 0 {
		t.Fatalf("unbooked order created payment, journal, or fulfillment rows: %d (%v)", before, err)
	}
	t.Log("P07 finalized before P05 confirmation: 0 payment, journal, or fulfillment rows")
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now() WHERE id=$1", order.Sale)
	waitUntil(t, func() bool {
		var paid bool
		err := db.QueryRow(ctx, "SELECT EXISTS (SELECT FROM ledger.payment WHERE sale_id=$1 AND tx_hash=$2)", order.Sale, hash).Scan(&paid)
		return err == nil && paid
	})
	read(hash, true, true)
	browser("confirmed", hash)
	stop()
	stop = start()
	defer stop()
	read(hash, true, true)
	var attempts, payments, journal, events int
	err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.relay_attempt WHERE work_id=$1),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$1), (SELECT count(*) FROM ledger.journal WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$1)`, order.Sale).Scan(&attempts, &payments, &journal, &events)
	if err != nil || relay.sends != 1 || attempts != 1 || payments != 1 || journal != 1 || events != 1 {
		t.Fatalf("original order did not converge exactly once: send, attempt, payment, journal, outbox = %v (%v)",
			[]int{relay.sends, attempts, payments, journal, events}, err)
	}
	t.Log("restarted read: 1 send, relay attempt, payment, journal, and fulfillment row")
	nonce, err := chain.NonceAt(ctx, gasAccount, nil)
	tx, _, lookupErr := chain.TransactionByHash(ctx, common.HexToHash(hash))
	if err != nil || lookupErr != nil || tx == nil || nonce != tx.Nonce()+1 {
		t.Fatalf("unexpected relay nonce after retries: %s %v %v", strconv.FormatUint(nonce, 10), err, lookupErr)
	}
}

func raceGasSponsorPin(t *testing.T, ctx context.Context, dsn, terminal string, order seededOrder, grant GasSponsorGrant) {
	t.Helper()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	type result struct {
		matched bool
		err     error
	}
	results := make(chan result, 2)
	ready := make(chan struct{})
	for _, extra := range []int64{0, 1} {
		candidate := grant
		candidate.Branch, candidate.ExpiresAt = order.Branch, grant.ExpiresAt+extra
		body, ok := GasSponsorGrantMessage(candidate)
		if !ok {
			t.Fatal("bad concurrent grant fixture")
		}
		digest := sha256.Sum256(body)
		go func() {
			<-ready
			var item result
			item.err = db.QueryRow(ctx, "SELECT ledger.pin_merchant_gas_sponsor($1,$2,$3,$4,$5,$6,$7)",
				order.Tenant, order.Branch, terminal, order.Sale, order.Order.Claim[:], order.Quote.Digest[:], digest[:]).Scan(&item.matched)
			results <- item
		}()
	}
	close(ready)
	winners := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.matched {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("different concurrent sponsors recorded %d winners", winners)
	}
	t.Log("two authorized calls racing different sponsorship digests pinned one")
}
