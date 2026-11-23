package settlement

import (
	"bytes"
	"cmp"
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
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	v1 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

const orphanIndexerRecoveryLimit = 60 * time.Second

func TestLoopbackSettlementEvidenceAndRefund(t *testing.T) {
	loopbackSettlement(t, settlementEndpoint(t), false, false)
}

func TestLoopbackCustomerOrderReceipt(t *testing.T) {
	settlementEndpoint(t)
	// Keep the same disposable PostgreSQL while isolating block time from
	// preceding suites that deliberately advance subscription and refund time.
	loopbackSettlement(t, freshSettlementChain(t), true, false)
}

func TestLoopbackK05NearbyCafeC2(t *testing.T) {
	settlementEndpoint(t)
	t.Setenv("P05_PILOT_HUMAN_UNITS", "1")
	loopbackSettlement(t, freshSettlementChain(t), true, true)
}

func settlementEndpoint(t *testing.T) string {
	t.Helper()
	endpoint := os.Getenv("P05_SETTLEMENT_TEST_RPC")
	if endpoint == "" || os.Getenv("P05_SETTLEMENT_TEST_INDEXER") == "" {
		t.Skip("requires test/settlement/anvil.sh")
	}
	if !strings.HasPrefix(endpoint, "http://127.0.0.1:") {
		t.Fatal("only loopback Anvil fixtures are supported")
	}
	return endpoint
}

func freshSettlementChain(t *testing.T) string {
	t.Helper()
	return settlementChainSince(t, 65*time.Second)
}

// settlementChainSince starts a chain whose first block is age old.
func settlementChainSince(t *testing.T, age time.Duration) string {
	t.Helper()
	binary := os.Getenv("ANVIL")
	if binary == "" {
		binary = "anvil"
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "http://" + listener.Addr().String()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(t.TempDir(), "settlement-anvil.log"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-age).Unix()
	node := exec.Command(binary, "--host", "127.0.0.1", "--port", port, "--chain-id", "8283",
		"--timestamp", strconv.FormatInt(start, 10), "--slots-in-an-epoch", "1", "--silent")
	node.Stdout, node.Stderr = log, log
	if err := node.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = node.Process.Kill()
		_ = node.Wait()
		_ = log.Close()
		if t.Failed() {
			if output, err := os.ReadFile(log.Name()); err == nil {
				t.Log(string(output))
			}
		}
	})
	waitUntil(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		rpc, err := ethclient.DialContext(ctx, endpoint)
		if err != nil {
			return false
		}
		defer rpc.Close()
		chain, err := rpc.ChainID(ctx)
		return err == nil && chain.Int64() == 8283
	})
	return endpoint
}

type receiptGasSigner struct{ key *ecdsa.PrivateKey }

func (s receiptGasSigner) SignDigest(_ context.Context, digest [32]byte) ([]byte, error) {
	signature, err := crypto.Sign(digest[:], s.key)
	if err == nil {
		signature[64] += 27
	}
	return signature, err
}

func loopbackSettlement(t *testing.T, endpoint string, onlyCustomerReceipt, nearbyCafe bool) {
	binary := os.Getenv("P05_SETTLEMENT_TEST_INDEXER")
	if nearbyCafe && (pilotScale() != 1000000 || os.Getenv("NU54_P01_INSTALL_TEST") == "") {
		t.Fatal("nearby cafe requires human TestUSDC and the P01 host")
	}
	hold := pilotHoldDuration(t)
	if os.Getenv("P05_PILOT_SAME_SKU") == "1" &&
		(os.Getenv("P05_PILOT_INDEPENDENT") == "" || os.Getenv("P05_PILOT_BULK") == "" || hold != 5*time.Minute) {
		t.Fatal("same-SKU pilot requires independent bulk issuance and a five-minute public hold")
	}
	db, dsn := scratchDB(t)
	budget := 50*time.Second + orphanIndexerRecoveryLimit + hold
	if os.Getenv("NU54_REFUND_REVIEW_BROWSER") != "" {
		if os.Getenv("P05_PILOT_OUTPUT") == "" || os.Getenv("P05_PILOT_HUMAN_UNITS") != "1" {
			t.Fatal("refund review browser requires isolated pilot output and human fixture amounts")
		}
		budget += 2 * time.Minute
	}
	if nativeCommerceLabDirectory() != "" || os.Getenv("NU54_POS_NATIVE_LAB") != "" || os.Getenv("NU54_REPRICED_NATIVE_LAB") != "" {
		budget += 3 * time.Minute
	}
	if os.Getenv("NU54_KIOSK_OUTCOME_LAB") != "" || os.Getenv("NU54_TRAVEL_NATIVE_T05") == "1" || os.Getenv("NU54_TRAVEL_NATIVE_UC1") == "1" {
		budget += 2 * time.Minute
	}
	if os.Getenv("NU54_SUBSCRIPTION_NATIVE_FLOW") != "" || os.Getenv("NU54_PRINT_NATIVE_LAB") != "" || os.Getenv("NU54_CAFE_NATIVE_LAB") != "" {
		budget += 5 * time.Minute
	}
	if os.Getenv("P05_PILOT_OUTPUT") == "" {
		budget += time.Minute
	}
	if os.Getenv("P05_PILOT_BULK") != "" {
		budget += 3 * time.Minute
	}
	if os.Getenv("NU54_REPRICED_ORDER_TEST") == "1" || os.Getenv("NU54_REPRICED_NATIVE_LAB") != "" {
		// Absence cannot close an issued quote until its real wall-clock validity ends.
		budget += 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var deploymentKey *ecdsa.PrivateKey
	var gasSigner Signer = fixtureSigner(100)
	gasAccount := fixtureSigner(100).Address()
	if onlyCustomerReceipt {
		// A new Anvil with the same private deployer and nonce would still
		// produce the existing P07 (chain, contract) stream identity.
		deploymentKey, err = crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		gasSigner = receiptGasSigner{deploymentKey}
		gasAccount = crypto.PubkeyToAddress(deploymentKey.PublicKey)
	}
	target, deployment, token, contract, checkLegacy := deployFixtureWithOperator(t, ctx, client, db, deploymentKey, !onlyCustomerReceipt && os.Getenv("P05_PILOT_OUTPUT") != "")
	head, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	issued := time.Unix(int64(head.Time), 0).UTC()
	if onlyCustomerReceipt && os.Getenv("NU54_CAFE_NATIVE_LAB") != "" {
		// The native relaunch uses wall time. Issue a fresh quote without
		// extending its normal lifetime when Anvil starts behind that clock.
		issued = time.Now().UTC().Truncate(time.Second)
	}
	buyers := [2]fixtureSigner{102, 105}
	genuine := !onlyCustomerReceipt && os.Getenv("P05_PILOT_OUTPUT") != ""
	var order, second seededOrder
	if genuine {
		order = seededOrder{Tenant: fixtureUUID(t, db), Branch: fixtureUUID(t, db), Actor: fixtureUUID(t, db), CipherKey: [32]byte{31: 53}, Target: target}
		second = order
		second.Actor = fixtureUUID(t, db)
		order.Order.Payer, second.Order.Payer = buyers[0].Address(), buyers[1].Address()
		fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", order.Branch, order.Tenant)
		fixtureExec(t, db, "INSERT INTO ledger.command_rule VALUES($1,'pilot','pilot','pilot',interval '15 minutes')", order.Branch)
		for _, subject := range []seededOrder{order, second} {
			fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'customer',NULL,now()+interval '1 hour')", order.Branch, subject.Actor)
		}
	} else {
		order = seedPayerOrder(t, db, target, issued, "10", 97, buyers[0], 1)
		if nearbyCafe {
			second = seededOrder{Tenant: order.Tenant, Branch: order.Branch, Actor: fixtureUUID(t, db), CipherKey: order.CipherKey, Target: target}
			second.Order.Payer = buyers[1].Address()
			fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'customer',NULL,now()+interval '1 hour')", second.Branch, second.Actor)
		} else {
			second = seedPayerPreview(t, db, target, "20", buyers[1], order)
		}
	}
	var approvalB []byte
	if !genuine {
		approvalB = pilotBoardApproval(t, ctx, order, buyers[0], "management")
	}

	if order.Sale != "" {
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now()+interval '1 day' WHERE id<>$1", order.Sale)
	}
	qCipher, _ := commerce.NewCipher(order.CipherKey[:])
	relayKey := [32]byte{31: 95}
	relayCipher, _ := commerce.NewCipher(relayKey[:])
	open := func(role string) *Store {
		t.Helper()
		_, roleDSN := testLogin(t, db, dsn, role)
		store, err := Open(ctx, roleDSN, role)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(store.Close)
		return store
	}
	forwardStore := open("nu54_p05_commerce")
	workerStore := open("nu54_p05_settlement")
	financeStore := open("nu54_p05_finance")
	workerProfile := map[string]Target{order.Branch: target}
	var approved []*fixtureDeploymentApproval
	if os.Getenv("NU54_NOTIFICATION_BROWSER") != "" {
		approved = append(approved, new(fixtureDeploymentApproval))
	}
	indexer, restartIndexer, startTip, proveUnpaid := startFixtureIndexer(t, ctx, db, dsn, endpoint, binary, target, deployment, approved...)
	indexerTip := func() int64 {
		var h *int64
		_ = db.QueryRow(ctx, "SELECT finalized_height FROM chainview.generation WHERE stream_id=$1", target.Stream).Scan(&h)
		if h == nil {
			return 0
		}
		return *h
	}
	if startTip == 0 {
		t.Fatal("producer did not reach finality")
	}
	name, evidenceDSN := testLogin(t, db, dsn, "nu54_p07_reconcile")
	merchant := (common.Hash{31: 50}).Hex()
	fixtureExec(t, db, `INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '1 hour')`, name, target.Stream, merchant)
	proof, err := OpenEvidence(ctx, evidenceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer proof.Close()

	requester, approver, manualTreasury := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	for _, grant := range [][2]string{{requester, "refund_request"}, {requester, "refund_approve"}, {approver, "refund_approve"}, {approver, "day_close"}, {manualTreasury, "refund_submit"}} {
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,$3,now()+interval '1 hour')", order.Branch, grant[0], grant[1])
	}
	terminalActor := fixtureUUID(t, db)
	var menuTerminal string
	pickupOperator := fixtureUUID(t, db)
	var otherPickupBranch, otherPickupOperator string
	if onlyCustomerReceipt {
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'pickup_manage',now()+interval '1 hour')", order.Branch, pickupOperator)
		otherPickupBranch, otherPickupOperator = fixtureUUID(t, db), fixtureUUID(t, db)
		fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", otherPickupBranch, order.Tenant)
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'pickup_manage',now()+interval '1 hour')", otherPickupBranch, otherPickupOperator)
	}
	fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'terminal','pos',now()+interval '1 hour')", order.Branch, terminalActor)
	fixtureExec(t, db, "INSERT INTO ledger.menu_release VALUES($1,1,now()-interval '1 minute',now()+interval '15 minutes')", order.Branch)
	fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,'sample','plain','Sample',$2,10)", order.Branch, lower(target.Asset))
	fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,'sample',105)", order.Branch)
	if nearbyCafe {
		menuTerminal = fixtureUUID(t, db)
		fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'terminal','menu',now()+interval '1 hour')", order.Branch, menuTerminal)
		fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,'near-cafe','plain','Nearby cafe',$2,$3::text::numeric)", order.Branch, lower(target.Asset), nearbyCafeAmount)
		fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,'near-cafe',1)", order.Branch)
	}
	if onlyCustomerReceipt {
		for _, item := range []seededOrder{order, second} {
			if item.Sale == "" {
				continue
			}
			fixtureExec(t, db, "UPDATE ledger.sale SET origin='menu' WHERE id=$1", item.Sale)
			fixtureExec(t, db, "INSERT INTO ledger.order_line VALUES($1,$2,1,'sample','plain',1,10)", item.Sale, item.Branch)
		}
	}
	for _, enrolled := range []seededOrder{order, second} {
		fixtureExec(t, db, `INSERT INTO ledger.payer_enrollment VALUES($1,$2,$3,1,1,now()+interval '1 hour')`,
			enrolled.Branch, enrolled.Actor, enrolled.Order.Payer.Bytes())
		if !genuine && enrolled.Sale != "" {
			checkoutExpiry := time.Now().Add(15 * time.Minute)
			fixtureExec(t, db, `INSERT INTO ledger.order_checkout VALUES($1,$2,$3,1,$4,sha256('pilot fixture'::bytea))`, enrolled.Sale, enrolled.Branch, terminalActor, checkoutExpiry)
			fixtureExec(t, db, `INSERT INTO ledger.checkout_claim VALUES($1,$2,$3,$4,$5,1,1,$6)`, enrolled.Sale, enrolled.Branch, enrolled.Actor, enrolled.Order.Claim[:], enrolled.Order.Payer.Bytes(), checkoutExpiry)
		}
	}

	_, httpDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	commerceStore, err := commerce.Open(ctx, httpDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer commerceStore.Close()
	certCount := 6
	if onlyCustomerReceipt {
		certCount++
	}
	if nearbyCafe {
		certCount++
	}
	if os.Getenv("P05_PILOT_INDEPENDENT") != "" {
		certCount += pilotIndependentCustomers
	}
	var simulatorKey []any
	if dir := os.Getenv("NU54_POS_NATIVE_LAB"); dir != "" {
		raw, err := os.ReadFile(filepath.Join(dir, "terminal-spki.der"))
		if err != nil {
			t.Fatal(err)
		}
		key, err := x509.ParsePKIXPublicKey(raw)
		if err != nil {
			t.Fatal(err)
		}
		simulatorKey = append(simulatorKey, key)
	}
	trust, certs, clientCA := mtlsClients(t, certCount, simulatorKey...)
	var simulatorCert *tls.Certificate
	if len(simulatorKey) != 0 {
		simulatorCert = &certs[certCount]
		certs = certs[:certCount]
	}
	var issuerURL string
	var actors []pilotIdentity
	if genuine {
		profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
			Signer: fixtureSigner(101).Address(), MerchantID: [32]byte{31: 50}, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}}
		digest := profile.Digest()
		fixtureExec(t, db, "INSERT INTO ledger.issuance_profile VALUES($1,1,$2,45,true)", order.Branch, digest[:])
		for i, label := range []string{"pilot-one", "pilot-two"} {
			fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,$2,'plain',$2,$3,$4)", order.Branch, label, lower(target.Asset), int64(10*(i+1))*pilotScale())
			fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,$2,1)", order.Branch, label)
		}
		issuerURL, actors = startPilotIssuer(t, ctx, db, dsn, order, second, terminalActor, certs, clientCA, simulatorCert)
		order = reserveTimedPilot(t, ctx, db, issuerURL, order, actors[2], "pilot-one", &actors[3])
		second = reserveTimedPilot(t, ctx, db, issuerURL, second, actors[1], "pilot-two", nil)
		forceTime(t, ctx, client, time.Now().Unix()+1)
		order = issueTimedPilot(t, ctx, db, issuerURL, order, actors[0], actors[2], pilotUnits(10), actors[1])
		approvalB = pilotBoardApproval(t, ctx, order, buyers[0], "management")
	}

	customerFP, requesterFP, approverFP := fingerprint(certs[0]), fingerprint(certs[1]), fingerprint(certs[2])
	expires := time.Now().Add(time.Hour).Unix()
	mux := http.NewServeMux()
	commerceHTTP := commerce.HTTP{Store: commerceStore, Payment: Forwarder{Store: forwardStore, QuoteCipher: qCipher, RelayCipher: relayCipher, Targets: workerProfile},
		Credentials: map[string]service.Principal{
			customerFP:            {Role: "customer", Tenant: order.Tenant, Subject: order.Actor, ExpiresAt: expires},
			fingerprint(certs[4]): {Role: "customer", Tenant: second.Tenant, Subject: second.Actor, ExpiresAt: expires},
			fingerprint(certs[5]): {Role: "terminal", Tenant: order.Tenant, Subject: terminalActor, ExpiresAt: expires},
			fingerprint(certs[3]): {Role: "operator", Tenant: order.Tenant, Subject: pickupOperator, ExpiresAt: expires},
		}}
	if onlyCustomerReceipt {
		commerceHTTP.Credentials[fingerprint(certs[6])] = service.Principal{
			Role: "operator", Tenant: order.Tenant, Subject: otherPickupOperator, ExpiresAt: expires,
		}
	}
	if nearbyCafe {
		commerceHTTP.Credentials[fingerprint(certs[7])] = service.Principal{
			Role: "terminal", Tenant: order.Tenant, Subject: menuTerminal, ExpiresAt: expires,
		}
	}
	if onlyCustomerReceipt && (repricedTestEnabled() || nearbyCafe) {
		profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
			Signer: fixtureSigner(101).Address(), MerchantID: [32]byte{31: 50}, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}}
		digest := profile.Digest()
		fixtureExec(t, db, "INSERT INTO ledger.issuance_profile VALUES($1,1,$2,120,true)", order.Branch, digest[:])
		commerceHTTP.Profiles = map[string]commerce.Profile{order.Branch: profile}
		commerceHTTP.Signer, commerceHTTP.Cipher = fixtureSigner(101), qCipher
		commerceHTTP.PresentationKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, 32))
	}
	var repricedLab *nativeSubscriptionLab
	var repricedTLS tls.Certificate
	if dir := os.Getenv("NU54_REPRICED_NATIVE_LAB"); onlyCustomerReceipt && dir != "" {
		repricedTLS = newNativeLoopbackTLS(t)
		clientPEM, lab := enrollNativeDevices(t, dir, repricedTLS, nil, commerceHTTP.Credentials,
			service.Principal{Role: "customer", Tenant: order.Tenant, Subject: order.Actor, ExpiresAt: time.Now().Add(time.Hour).Unix()})
		if !trust.AppendCertsFromPEM(clientPEM) {
			t.Fatal("native test client CA")
		}
		repricedLab = lab
	}
	commerceHTTP.Register(mux)
	FinanceHTTP{Store: financeStore, Funds: LiveChain{client}, Targets: workerProfile, Merchants: map[string]string{order.Branch: (common.Hash{31: 50}).Hex()}, Credentials: map[string]service.Principal{
		requesterFP: {Role: "finance", Tenant: order.Tenant, Subject: requester, ExpiresAt: expires},
		approverFP:  {Role: "finance", Tenant: order.Tenant, Subject: approver, ExpiresAt: expires}, fingerprint(certs[3]): {Role: "finance", Tenant: order.Tenant, Subject: manualTreasury, ExpiresAt: expires}}}.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	if repricedLab != nil {
		server.TLS.Certificates = []tls.Certificate{repricedTLS}
	}
	server.StartTLS()
	defer server.Close()
	post := func(identity int, path string, body any, want int) []byte {
		t.Helper()
		transport := server.Client().Transport.(*http.Transport).Clone()
		defer transport.CloseIdleConnections()
		transport.TLSClientConfig.Certificates = []tls.Certificate{certs[identity]}
		user := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		raw, _ := json.Marshal(body)
		response, err := user.Post(server.URL+path, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("POST %s: %d for %s", path, response.StatusCode, data)
		}
		return data
	}
	forward := func(order seededOrder, identity int, approval []byte) {
		cmd := map[string]any{"version": 2, "contract": "C3", "operation": "payment.forward", "domainRef": order.Branch, "objectRef": order.Sale,
			"requestId": fixtureUUID(t, db), "idempotencyKey": fixtureUUID(t, db), "expiresAt": time.Now().Unix() + 30,
			"body": map[string]string{"quoteDigest": rawHex(order.Quote.Digest[:]), "approvalDigest": rawHex(order.Quote.ApprovalHash[:])}}
		for range 2 {
			cmd["requestId"] = fixtureUUID(t, db)
			post(identity, "/v2/commerce", map[string]any{"command": cmd, "approvalSignature": rawHex(approval)}, 200)
		}
	}
	paymentStatus := func(identity int, placed seededOrder, expected int) (commerce.Order, []byte) {
		t.Helper()
		command := service.Request{Version: 2, Contract: "C3", Operation: "order.status",
			RequestID: fixtureUUID(t, db), Domain: placed.Branch, Object: placed.Sale,
			ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{}}
		wire := post(identity, "/v2/commerce", map[string]any{"command": command}, expected)
		var response struct {
			Order commerce.Order `json:"order"`
		}
		if expected == 200 {
			if err := json.Unmarshal(wire, &response); err != nil || response.Order.ID != placed.Sale {
				t.Fatalf("wrong original payment status: %+v %v", response, err)
			}
		}
		return response.Order, wire
	}
	requireChecking := func(stage, expectedHash string) {
		t.Helper()
		view, _ := paymentStatus(0, order, 200)
		if view.State != "checking" || view.Payment == nil || view.Payment.Stage != stage ||
			view.Payment.TxHash != nil || view.Payment.IncludedAt != nil || view.ReceiptRef != nil || view.PickupNo != nil {
			t.Fatalf("expected checking %s without receipt/pickup: %+v payment: %+v", stage, view, view.Payment)
		}
		if expectedHash == "" && view.Payment.RelayTxHash != nil ||
			expectedHash != "" && (view.Payment.RelayTxHash == nil || *view.Payment.RelayTxHash != expectedHash) {
			t.Fatalf("wrong original relay hash in %s: %+v", stage, view.Payment)
		}
	}
	var holdResult func() pilotPublicSlot
	if genuine {
		verifyPilotClientApproval(t, ctx, order, approvalB, 0)
		holdResult = forwardThroughPilotClients(t, ctx, server, order, second, terminalActor, certs[0], certs[5], approvalB, fixtureUUID(t, db), hold)
	} else {
		forward(order, 0, approvalB)
	}
	requireChecking("received", "")
	var cafeLab *cafeNativeLab
	if onlyCustomerReceipt {
		if path := os.Getenv("NU54_CAFE_NATIVE_LAB"); path != "" && !nearbyCafe {
			fixtureExec(t, db, "INSERT INTO ledger.print_staff_grant VALUES($1,$2,now()+interval '1 hour')", order.Branch, pickupOperator)
			cafeLab = newCafeNativeLab(t, path, order, pickupOperator, commerceStore)
			cafeLab.mark("checking")
			cafeLab.wait(t, ctx, "held-seen")
			cafeLab.wait(t, ctx, "buyer-held-seen")
			remaining := time.Until(order.Order.Expires)
			if order.Order.Expires.Sub(order.Order.Issued) != 120*time.Second || remaining <= 0 {
				t.Fatalf("installed K05 original quote was not live after reconciliation: TTL remaining %s", remaining.Round(time.Second))
			}
			t.Logf("installed K05 resumed before relay with %s of the unchanged 120s quote lifetime remaining", remaining.Round(time.Second))
		}
		t.Run("KioskCafeBeforeFinality", func(t *testing.T) {
			verifyKioskCafeHold(t, db, order, post)
		})
	}
	if onlyCustomerReceipt {
		// Keep historical work intact. The global dispatcher skips the other
		// scenario's temporarily locked rows while this test uses its own chain.
		otherWork, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer otherWork.Rollback(context.Background())
		if _, err := otherWork.Exec(ctx, `SELECT w.id FROM ledger.settlement_work w
			JOIN ledger.sale s ON s.id=w.sale_id WHERE s.branch_id<>$1 AND NOT w.finished FOR UPDATE OF w`, order.Branch); err != nil {
			t.Fatal(err)
		}
	}
	chain := &lostResponseChain{LiveChain: LiveChain{Client: client}}
	w := Worker{Store: workerStore, Evidence: proof, Chain: chain, Targets: workerProfile, QuoteCipher: qCipher, RelayCipher: relayCipher,
		GasAccount: gasAccount, GasSigner: gasSigner, RefundSigner: fixtureSigner(103),
		MaxGasPrice: big.NewInt(100_000_000_000), Now: time.Now}
	step := func(workID string) error {
		t.Helper()
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now() WHERE id=$1", workID)
		moved, err := w.Step(ctx)
		if !moved {
			t.Fatal("settlement case missing", err)
		}
		return err
	}
	if err := step(order.Sale); err != nil {
		t.Fatal(err)
	}
	var originalHash string
	if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", order.Sale).Scan(&originalHash); err != nil {
		t.Fatalf("original relay missing with quote TTL %s: %v", time.Until(order.Order.Expires).Round(time.Second), err)
	}
	requireChecking("relaySigned", originalHash)
	if onlyCustomerReceipt {
		head, err := client.HeaderByNumber(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		// Exercise a chain clock behind the journal independently of the
		// nearby order's future block timestamp later in this fixture.
		included := max(head.Time+1, uint64(order.Order.Issued.Unix()+1))
		if err := client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", included); err != nil {
			t.Fatal(err)
		}
	}
	if err := step(order.Sale); !errors.Is(err, errLostResponse) {
		t.Fatalf("expected simulated RPC loss after delivery: %v", err)
	}
	var txHash string
	if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", order.Sale).Scan(&txHash); err != nil {
		t.Fatal(err)
	}
	requireChecking("checking", txHash)
	_, foreignEvidence := paymentStatus(4, order, 404)
	if bytes.Contains(foreignEvidence, []byte(txHash)) || bytes.Contains(foreignEvidence, []byte(`"payment"`)) {
		t.Fatal("another customer obtained pending payment evidence")
	}
	if onlyCustomerReceipt {
		// A comparison claim allows the original order preview before payment.
		// The selected payer is the only customer who can see the relay hash.
		fixtureExec(t, db, `INSERT INTO ledger.checkout_claim(sale_id,branch_id,actor_id,claim_digest,payer,payer_generation,payer_role,expires_at)
          VALUES($1,$2,$3,sha256('other comparison'),$4,1,1,now()+interval '2 minutes')`,
			order.Sale, order.Branch, second.Actor, buyers[1].Address().Bytes())
		comparison, wire := paymentStatus(4, order, 200)
		if comparison.Payment != nil || comparison.ReceiptRef != nil || comparison.PickupNo != nil ||
			bytes.Contains(wire, []byte(txHash)) {
			t.Fatal("another customer's comparison claim disclosed payment details")
		}
		// Wrap the actual 051 projection only in the disposable database.
		// Materialize the authorized candidate before the comparison expires.
		fixtureExec(t, db, "CREATE SEQUENCE ledger.test_receipt_projection_calls")
		fixtureExec(t, db, "GRANT USAGE ON SEQUENCE ledger.test_receipt_projection_calls TO nu54_p05_commerce")
		fixtureExec(t, db, "ALTER FUNCTION ledger.commerce_order_receipt(uuid,uuid,uuid,uuid) RENAME TO delayed_receipt_original")
		fixtureExec(t, db, `CREATE FUNCTION ledger.commerce_order_receipt(uuid,uuid,uuid,uuid)
		  RETURNS TABLE(id uuid,revision bigint,state text,asset_ref text,amount numeric,expires_at timestamptz,
		      pickup_no bigint,receipt_ref uuid,quote_digest text,order_key text,receipt_kind text,subscription jsonb,
		      payment jsonb,access_ends_at timestamptz)
		  LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,pg_temp AS $body$
		  DECLARE observed int;
		  BEGIN
		    PERFORM nextval('ledger.test_receipt_projection_calls');
		    RETURN QUERY SELECT * FROM ledger.delayed_receipt_original($1,$2,$3,$4);
		    GET DIAGNOSTICS observed = ROW_COUNT;
		    IF observed<>1 THEN RAISE EXCEPTION 'expected one originally authorized detail'; END IF;
		    PERFORM pg_sleep(1.8);
		  END
		  $body$`)
		fixtureExec(t, db, "REVOKE ALL ON FUNCTION ledger.commerce_order_receipt(uuid,uuid,uuid,uuid) FROM PUBLIC")
		fixtureExec(t, db, "GRANT EXECUTE ON FUNCTION ledger.commerce_order_receipt(uuid,uuid,uuid,uuid) TO nu54_p05_commerce")
		fixtureExec(t, db, "UPDATE ledger.checkout_claim SET expires_at=clock_timestamp()+interval '1.4 seconds' WHERE sale_id=$1 AND actor_id=$2",
			order.Sale, second.Actor)
		began := time.Now()
		_, withheld := paymentStatus(4, order, 404)
		var projections int
		if err := db.QueryRow(ctx, "SELECT last_value FROM ledger.test_receipt_projection_calls").Scan(&projections); err != nil || projections != 1 {
			t.Fatalf("expected one authorized receipt projection, observed %d: %v", projections, err)
		}
		if elapsed := time.Since(began); elapsed < 1750*time.Millisecond {
			t.Fatalf("expected authorized detail to wait for claim expiry, observed %v", elapsed)
		} else {
			t.Logf("051 detail claim expired after one DB receipt projection, withheld TLS body after %v", elapsed)
		}
		for _, marker := range []string{order.Sale, txHash, `"amount"`, `"payment"`, `"pickupNo"`, `"receiptRef"`} {
			if bytes.Contains(withheld, []byte(marker)) {
				t.Fatalf("expired comparison disclosed %s in the response", marker)
			}
		}
		fixtureExec(t, db, "DROP FUNCTION ledger.commerce_order_receipt(uuid,uuid,uuid,uuid)")
		fixtureExec(t, db, "DROP SEQUENCE ledger.test_receipt_projection_calls")
		fixtureExec(t, db, "ALTER FUNCTION ledger.delayed_receipt_original(uuid,uuid,uuid,uuid) RENAME TO commerce_order_receipt")
		requireChecking("checking", txHash)
		if !genuine {
			creator, _ := paymentStatus(5, order, 200)
			if creator.State != "checking" || creator.Payment != nil {
				t.Fatal("original terminal creator lost its compatible read or saw private payment evidence")
			}
		}
		fixtureExec(t, db, "UPDATE ledger.checkout_claim SET expires_at=clock_timestamp()+interval '2 minutes' WHERE sale_id=$1 AND actor_id=$2",
			order.Sale, second.Actor)
	}
	paid, err := bind.WaitMinedHash(ctx, client, common.HexToHash(txHash))
	if err != nil || paid.Status != 1 {
		t.Fatalf("real payment failed %v", err)
	}
	// Reopen the restricted login and reconstruct the worker after the accepted
	// first transaction's reply was lost.
	restarted := open("nu54_p05_settlement")
	w.Store = restarted
	orphanRestart := restartIndexer(func() { mine(t, ctx, client, 5) })
	if indexerTip() < paid.BlockNumber.Int64() {
		t.Fatal("replacement indexer did not catch up to the accepted payment")
	}
	if err := step(order.Sale); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT p.evidence_id::text,
			(SELECT count(*) FROM ledger.journal j WHERE j.sale_id=p.sale_id AND j.refund_id IS NULL),
			(SELECT count(*) FROM ledger.fulfillment_outbox e WHERE e.sale_id=p.sale_id AND e.kind='paid')
			FROM ledger.payment p WHERE p.sale_id=$1 AND p.tx_hash=$2`, order.Sale, txHash).
		Scan(&orphanRestart.PaymentEvidence, &orphanRestart.SaleJournals, &orphanRestart.PaidOutbox); err != nil ||
		orphanRestart.SaleJournals != 1 || orphanRestart.PaidOutbox != 1 {
		t.Fatalf("replacement changed the accepted payment's evidence or effect counts: %v", err)
	}
	finalized, _ := paymentStatus(0, order, 200)
	if finalized.State != "paid" || finalized.ReceiptRef == nil || finalized.PickupNo == nil ||
		finalized.Payment == nil || finalized.Payment.Stage != "finalized" || finalized.Payment.TxHash == nil ||
		*finalized.Payment.TxHash != txHash || finalized.Payment.IncludedAt == nil || finalized.Payment.PickupStage != nil {
		t.Fatalf("original payment/receipt were not recovered: %+v payment=%+v", finalized, finalized.Payment)
	}
	_, foreignReceipt := paymentStatus(4, order, 404)
	if bytes.Contains(foreignReceipt, []byte(*finalized.ReceiptRef)) ||
		bytes.Contains(foreignReceipt, []byte(txHash)) {
		t.Fatal("another customer obtained the finalized receipt")
	}
	if onlyCustomerReceipt {
		paymentStatus(4, order, 404)
		list := service.Request{Version: 2, Contract: "C3", Operation: "order.list", RequestID: fixtureUUID(t, db),
			Domain: order.Branch, Object: second.Actor, ExpiresAt: time.Now().Unix() + 20,
			Body: map[string]string{"afterSale": second.Actor}}
		if body := post(4, "/v2/commerce", map[string]any{"command": list}, 200); bytes.Contains(body, []byte(order.Sale)) {
			t.Fatal("completed order remained visible to a different customer's comparison claim")
		}
		if originalHash != txHash || chain.sends != 1 {
			t.Fatal("receipt lookup replaced the original transaction")
		}
		t.Log("W11 returned the original checking hash and one finalized receipt across worker and producer restart; matching foreign claim denied")
		var nearby commerce.Order
		if !t.Run("KioskNearbyPaidSeparateOriginal", func(t *testing.T) {
			// A phone-created cart retains its own checkout rather than the
			// public terminal's presentation and payment identity.
			head, err := client.HeaderByNumber(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			secondTime := time.Unix(max(time.Now().Unix(), int64(head.Time)+1), 0).UTC()
			priorSends := chain.sends
			if nearbyCafe {
				second = admitNearbyCafe(t, ctx, db, server, certs[7], menuTerminal, order.Actor, second,
					commerceHTTP.Profiles[order.Branch], pilotPresentationKey(), buyers[1])
				paymentStatus(0, second, 404)
			} else {
				fixtureExec(t, db, "UPDATE ledger.order_checkout SET creator=$2 WHERE sale_id=$1", second.Sale, second.Actor)
				second = seedPayerIssue(t, db, second, secondTime, "20", 98, 1, 45*time.Second)
				approval := pilotBoardApproval(t, ctx, second, buyers[1], "payment")
				forward(second, 4, approval)
			}
			if os.Getenv("NU54_W11_PENDING_HOST") == "1" {
				t.Run("W11NearbyPendingOriginal", func(t *testing.T) {
					first, _ := paymentStatus(4, second, 200)
					again, _ := paymentStatus(4, second, 200)
					for _, pending := range []commerce.Order{first, again} {
						if pending.ID != second.Sale || pending.Amount != second.Quote.Args.Amount.String() || pending.State != "checking" ||
							pending.Payment == nil || pending.Payment.Stage != "received" || pending.Payment.TxHash != nil ||
							pending.Payment.IncludedAt != nil || pending.ReceiptRef != nil || pending.PickupNo != nil {
							t.Fatal("original nearby admission must remain checking before a mined transaction")
						}
					}
					paymentStatus(0, second, 404)
					const branchLabel = "Nearby settlement shop"
					fixtureExec(t, db, "INSERT INTO ledger.operator_branch_label(branch_id,label) VALUES($1,$2)", second.Branch, branchLabel)
					request := service.Request{Version: 2, Contract: "C3", Operation: "catalog.read",
						RequestID: fixtureUUID(t, db), Domain: second.Branch, Object: second.Branch,
						ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{"page": "0"}}
					var menu struct {
						Shop struct{ Branch, BranchLabel string }
					}
					if response := post(4, "/v2/commerce", map[string]any{"command": request}, 200); json.Unmarshal(response, &menu) != nil || menu.Shop.Branch != second.Branch || menu.Shop.BranchLabel != branchLabel {
						t.Fatal("registered customer could not read the original branch identity")
					}
					var admitted, claims, paid, pickup int
					if err := db.QueryRow(ctx, `SELECT
						(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
						(SELECT count(*) FROM ledger.relay_attempt WHERE work_id=$1),
						(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
						(SELECT count(*) FROM ledger.pickup WHERE sale_id=$1)`, second.Sale).
						Scan(&admitted, &claims, &paid, &pickup); err != nil ||
						admitted != 1 || claims != 0 || paid != 0 || pickup != 0 || chain.sends != priorSends {
						t.Fatal("reopening an admitted original must not submit, pay, or allocate a pickup")
					}
				})
			}
			verifyKioskCafeHold(t, db, second, post)
			if err := step(second.Sale); err != nil {
				t.Fatal(err)
			}
			// Deliberately put the canonical inclusion ahead of the SQL clock.
			if err := client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", secondTime.Unix()+15); err != nil {
				t.Fatal(err)
			}
			if err := step(second.Sale); err != nil {
				t.Fatal(err)
			}
			var hash string
			if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", second.Sale).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			evidence, err := bind.WaitMinedHash(ctx, client, common.HexToHash(hash))
			if err != nil || evidence.Status != types.ReceiptStatusSuccessful {
				t.Fatalf("nearby payment not mined: %v", err)
			}
			mine(t, ctx, client, 5)
			waitUntil(t, func() bool { return indexerTip() >= evidence.BlockNumber.Int64() })
			if err := step(second.Sale); err != nil {
				t.Fatal(err)
			}
			nearby, _ = paymentStatus(4, second, 200)
			if nearby.ReceiptRef == nil || nearby.PickupNo == nil || *nearby.ReceiptRef == *finalized.ReceiptRef ||
				*nearby.PickupNo == *finalized.PickupNo || nearby.Payment == nil || nearby.Payment.TxHash == nil ||
				*nearby.Payment.TxHash != hash || hash == txHash || chain.sends != priorSends+1 {
				t.Fatal("nearby order did not retain its separate payment, original receipt and pickup number")
			}
		}) {
			return
		}
		pendingNearby := func() string {
			t.Helper()
			var pending string
			if err := db.QueryRow(ctx, `SELECT jsonb_build_array(event_id,lease_until)::text
				FROM ledger.fulfillment_outbox WHERE sale_id=$1 AND kind='paid' AND state='pending'`, second.Sale).Scan(&pending); err != nil {
				t.Fatalf("the independent original must still have an eligible pending dispatch: %v", err)
			}
			return pending
		}
		originalPending := pendingNearby()
		publicStage := "collected"
		if nearbyCafe {
			publicStage = "preparing"
		}
		t.Run("KioskCafeAfterFulfillment", func(t *testing.T) {
			verifyKioskCafeProgress(t, ctx, db, dsn, proof, order, trust, certs[3], finalized, post, paymentStatus, 0, publicStage, cafeLab)
		})
		if pendingNearby() != originalPending {
			t.Fatal("direct cafe dispatch changed the unrelated pending event or its lease")
		}
		publicDetail := kitchenCommand(post, 3, order.Branch, "pickup-detail", map[string]any{"sale": order.Sale}, 200)
		publicPayment, _ := paymentStatus(0, order, 200)
		t.Run("KioskNearbyPickupSeparateOrder", func(t *testing.T) {
			var lab *cafeNativeLab
			if path := os.Getenv("NU54_CAFE_NATIVE_LAB"); path != "" && nearbyCafe {
				fixtureExec(t, db, "INSERT INTO ledger.print_staff_grant VALUES($1,$2,now()+interval '1 hour')", order.Branch, pickupOperator)
				publicEvent, _ := kioskCafeDetail(t, order, post)
				lab = newCafeNativeLab(t, path, second, pickupOperator, commerceStore, &cafeNativeSelection{
					publicSale: order.Sale, publicEvent: publicEvent, publicNumber: *finalized.PickupNo,
					payment: cafePaymentProfile(commerceHTTP.Profiles[order.Branch], pilotPresentationKey()),
				})
			}
			verifyKioskCafeProgress(t, ctx, db, dsn, proof, second, trust, certs[3], nearby, post, paymentStatus, 4, "ready", lab)
			t.Log("the previously locked unrelated pending event was subsequently dispatched and acknowledged unchanged")
		})
		if observed := kitchenCommand(post, 3, order.Branch, "pickup-detail", map[string]any{"sale": order.Sale}, 200); !bytes.Equal(observed, publicDetail) {
			t.Fatal("nearby pickup changed the public original's payment, preparation or fulfillment details")
		}
		publicAfter, _ := paymentStatus(0, order, 200)
		if !reflect.DeepEqual(publicAfter, publicPayment) {
			t.Fatal("the public customer's original payment or pickup changed during the separate nearby order")
		}
		// This second operator can open their own branch using the same
		// feature flag, but can neither cross branches nor repoint the sale.
		kitchenCommand(post, 6, otherPickupBranch, "pickup-queue", map[string]any{"includeCafe": true}, 200)
		for _, scope := range []string{otherPickupBranch, order.Branch} {
			closed := kitchenCommand(post, 6, scope, "pickup-detail", map[string]any{"sale": second.Sale, "includeCafe": true}, 404)
			for _, hidden := range []string{second.Sale, "eventRef", "amount", "asset", "finalizedAt", "pickupReadyAt"} {
				if bytes.Contains(closed, []byte(hidden)) {
					t.Fatal("another branch's authorized operator saw original pickup evidence")
				}
			}
		}

		if os.Getenv("NU54_W11_UNPAID_HOST") == "1" {
			t.Run("W11NearbyClosedOriginal", func(t *testing.T) {
				original := seedPayerPreview(t, db, target, "20", buyers[1], second)
				fixtureExec(t, db, "UPDATE ledger.sale SET origin='menu' WHERE id=$1", original.Sale)
				head, err := client.HeaderByNumber(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				if int64(head.Time) > time.Now().Unix()+30 {
					t.Fatal("fixture chain advanced beyond normal signed quote window")
				}
				original = seedPayerIssue(t, db, original, time.Now().UTC().Truncate(time.Second), "20", 99, 1, 12*time.Second)
				fixtureExec(t, db, `INSERT INTO ledger.order_checkout VALUES($1,$2,$3,1,$4,sha256('nearby-close fixture'::bytea))`,
					original.Sale, original.Branch, terminalActor, original.Order.Expires)
				fixtureExec(t, db, `INSERT INTO ledger.menu_reservation_outcome(branch_id,actor_id,request_key,origin_ref,
					nearby_customer,intent_digest,sale_id)
					SELECT branch_id,$2,gen_random_uuid(),origin_ref,$3,sha256('nearby-close fixture'::bytea),id
					FROM ledger.sale WHERE id=$1`, original.Sale, terminalActor, original.Actor)
				fixtureExec(t, db, `INSERT INTO ledger.checkout_claim VALUES($1,$2,$3,$4,$5,1,1,$6)`,
					original.Sale, original.Branch, original.Actor, original.Order.Claim[:], original.Order.Payer.Bytes(), original.Order.Expires)
				before := chain.sends
				forward(original, 4, pilotBoardApproval(t, ctx, original, buyers[1], "payment"))
				pending, _ := paymentStatus(4, original, 200)
				if pending.State != "checking" || pending.Payment == nil || pending.Payment.NearbyUnpaid {
					t.Fatal("a durable approval is still pending before final absence")
				}
				forceTime(t, ctx, client, max(original.Order.Expires.Unix()+1, int64(head.Time)+1))
				mine(t, ctx, client, 6)
				checkpoint, err := client.HeaderByNumber(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				waitUntil(t, func() bool { return indexerTip() >= checkpoint.Number.Int64()-2 })
				if remaining := time.Until(original.Order.Expires); remaining > 0 {
					select {
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-time.After(remaining):
					}
				}
				closeExpired(t, db, original, func() error { return step(original.Sale) })
				list := func(identity int, subject string, expected int) []byte {
					request := service.Request{Version: 2, Contract: "C3", Operation: "order.list", Domain: original.Branch,
						Object: subject, RequestID: fixtureUUID(t, db), ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{"afterSale": subject}}
					return post(identity, "/v2/commerce", map[string]any{"command": request}, expected)
				}
				upgradeW11UnpaidArchive(t, ctx, db, dsn, original.Sale, func() {
					paymentStatus(4, original, 404)
					if bytes.Contains(list(4, original.Actor, 200), []byte(original.Sale)) {
						t.Fatal("predecessor unexpectedly retained the expired comparison scope")
					}
				})
				closed, wire := paymentStatus(4, original, 200)
				if closed.State != "closed" || closed.Amount != original.Order.Amount || closed.Payment == nil || !closed.Payment.NearbyUnpaid ||
					closed.Payment.Stage != "unpaid" || closed.PickupNo != nil || closed.ReceiptRef != nil || closed.Payment.TxHash != nil ||
					bytes.Contains(wire, []byte("accessEndsAt")) || !bytes.Contains(list(4, original.Actor, 200), []byte(original.Sale)) ||
					bytes.Contains(list(0, order.Actor, 200), []byte(original.Sale)) {
					t.Fatal("only the signing owner can recover the original unpaid archive after comparison expiry")
				}
				paymentStatus(0, original, 404)
				fixtureExec(t, db, "UPDATE ledger.commerce_grant SET valid_until=now() WHERE branch_id=$1 AND actor_id=$2", original.Branch, original.Actor)
				paymentStatus(4, original, 404)
				list(4, original.Actor, 404)
				fixtureExec(t, db, "UPDATE ledger.commerce_grant SET valid_until=now()+interval '1 hour' WHERE branch_id=$1 AND actor_id=$2", original.Branch, original.Actor)
				paymentStatus(4, original, 200)
				var approvals, payments, pickups, absence int
				if err := db.QueryRow(ctx, `SELECT
					(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
					(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
					(SELECT count(*) FROM ledger.pickup WHERE sale_id=$1),
					(SELECT count(*) FROM ledger.sale_resolution r JOIN chainview.active_evidence e ON e.id=r.evidence_id AND e.kind='absent' WHERE r.sale_id=$1)`, original.Sale).Scan(&approvals, &payments, &pickups, &absence); err != nil || approvals != 1 || payments != 0 || pickups != 0 || absence != 1 || chain.sends != before {
					t.Fatal("one original admission and canonical absence must yield no relay, payment or pickup")
				}
			})
		}

		var oldRepriced seededOrder
		if repricedTestEnabled() {
			t.Run("RepricedOriginalOrder", func(t *testing.T) {
				transport := server.Client().Transport.(*http.Transport).Clone()
				transport.TLSClientConfig.Certificates = []tls.Certificate{certs[0]}
				defer transport.CloseIdleConnections()
				owner := pilotIdentity{actor: order.Actor, board: buyers[0], client: &http.Client{Transport: transport, Timeout: 4 * time.Second}}
				operator, err := bind.NewKeyedTransactorWithChainID(deploymentKey, big.NewInt(8283))
				if err != nil {
					t.Fatal(err)
				}
				operator.Context = ctx
				fund := func(tx *types.Transaction, err error) {
					if err != nil {
						t.Fatal(err)
					}
					receipt, err := bind.WaitMined(ctx, client, tx)
					if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
						t.Fatalf("test funding: %v", err)
					}
				}
				fund(token.Mint(operator, operator.From, big.NewInt(13_500_000)))
				fund(token.Approve(operator, target.Contract, big.NewInt(13_500_000)))
				fund(contract.DepositEscrow(operator, buyers[0].Address(), 1, big.NewInt(13_500_000)))
				oldRepriced = verifyRepricedOriginal(t, ctx, db, order, owner, server.URL, buyers[0], post, paymentStatus, repricedLab, qCipher, func(repriced seededOrder) {
					beneficiary := fixtureSigner(104).Address()
					balanceBefore, err := token.BalanceOf(&bind.CallOpts{Context: ctx}, beneficiary)
					if err != nil {
						t.Fatal(err)
					}
					if err := step(repriced.Sale); err != nil {
						t.Fatal(err)
					}
					head, err := client.HeaderByNumber(ctx, nil)
					if err != nil {
						t.Fatal(err)
					}
					inclusion := max(int64(head.Time)+1, int64(repriced.Quote.Args.IssuedAt)+1)
					if inclusion >= int64(repriced.Quote.Args.ExpiresAt) {
						t.Fatal("fixture clock advanced beyond the original new quote")
					}
					if err := client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", inclusion); err != nil {
						t.Fatal(err)
					}
					if err := step(repriced.Sale); err != nil {
						t.Fatal(err)
					}
					var hash string
					if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", repriced.Sale).Scan(&hash); err != nil {
						t.Fatal(err)
					}
					receipt, err := bind.WaitMinedHash(ctx, client, common.HexToHash(hash))
					if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
						t.Fatalf("repriced transfer at %d in quote [%d,%d): receipt=%+v error=%v", inclusion, repriced.Quote.Args.IssuedAt, repriced.Quote.Args.ExpiresAt, receipt, err)
					}
					raw, pending, err := client.TransactionByHash(ctx, receipt.TxHash)
					if err != nil || pending {
						t.Fatalf("repriced relay transaction unavailable: %v", err)
					}
					sender, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8283)), raw)
					if err != nil || sender != gasAccount || sender == buyers[0].Address() || receipt.GasUsed == 0 {
						t.Fatalf("the relay must pay the fee for the original transaction: %v", err)
					}
					mine(t, ctx, client, 5)
					waitUntil(t, func() bool { return indexerTip() >= receipt.BlockNumber.Int64() })
					if err := step(repriced.Sale); err != nil {
						t.Fatal(err)
					}
					balanceAfter, err := token.BalanceOf(&bind.CallOpts{Context: ctx}, beneficiary)
					if err != nil || new(big.Int).Sub(balanceAfter, balanceBefore).Cmp(big.NewInt(7_000_000)) != 0 {
						t.Fatalf("beneficiary did not receive exactly 7.00: %v", err)
					}
					final, _ := paymentStatus(0, repriced, 200)
					verifyKioskCafeProgress(t, ctx, db, dsn, proof, repriced, trust, certs[3], final, post, paymentStatus, 0, "collected", nil)
				})
			})
		}

		if oldRepriced.Sale != "" {
			t.Run("OriginalOldQuoteExpiresUnpaid", func(t *testing.T) {
				phase := "finalized-absence"
				defer func() {
					if t.Failed() {
						logRepricedExpiryState(t, db, oldRepriced.Sale, phase)
					}
				}()
				previous, err := client.HeaderByNumber(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				forceTime(t, ctx, client, max(oldRepriced.Order.Expires.Unix()+1, int64(previous.Time)+1))
				mine(t, ctx, client, 6)
				head, err := client.HeaderByNumber(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				waitUntil(t, func() bool { return indexerTip() >= head.Number.Int64()-2 })
				proveUnpaid(t, oldRepriced)
				expiry := time.Unix(int64(oldRepriced.Quote.Args.ExpiresAt), 0)
				if remaining := time.Until(expiry); remaining > 0 {
					phase = "valid-before-wall-expiry"
					if err := step(oldRepriced.Sale); err != nil {
						t.Fatal(err)
					}
					stillValid, _ := paymentStatus(0, oldRepriced, 200)
					if stillValid.State != "issued" || stillValid.Payment != nil {
						t.Fatal("chain time alone closed a valid quote")
					}
					timer := time.NewTimer(time.Until(expiry))
					defer timer.Stop()
					select {
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-timer.C:
					}
				}
				phase = "reconcile-after-wall-expiry"
				if err := step(oldRepriced.Sale); err != nil {
					t.Fatal(err)
				}
				phase = "read-closed-order"
				closed, _ := paymentStatus(0, oldRepriced, 200)
				if closed.Amount != "6500000" || closed.State != "closed" || closed.Payment == nil ||
					closed.Payment.Stage != "unpaid" || closed.ReceiptRef != nil || closed.PickupNo != nil || closed.Payment.TxHash != nil {
					t.Fatal("old quote did not close without a payment")
				}
				phase = "read-ledger-effects"
				var paid, forwarded int
				if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
					(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1)`,
					oldRepriced.Sale).Scan(&paid, &forwarded); err != nil || paid != 0 || forwarded != 0 {
					t.Fatalf("old quote effects: %d/%d %v", paid, forwarded, err)
				}
				t.Log("separate archived 6.50 quote retained its price and closed on finalized non-consumption, zero submission or payment")
			})
		}

		return
	}
	var publicWindow pilotPublicSlot
	if holdResult != nil {
		publicWindow = holdResult()
	}
	publicPaymentEntered := time.Now().UTC()
	publicQuoteTime := publicPaymentEntered.Truncate(time.Second)
	forceTime(t, ctx, client, publicQuoteTime.Unix()+1)
	if genuine {
		second = issueTimedPilot(t, ctx, db, issuerURL, second, actors[1], actors[1], pilotUnits(20))
	} else {
		second = seedPayerIssue(t, db, second, publicQuoteTime, "20", 98, 1, 45*time.Second)
	}
	approvalA := pilotBoardApproval(t, ctx, second, buyers[1], "payment")
	if os.Getenv("P05_PILOT_OUTPUT") != "" {
		verifyPilotClientApproval(t, ctx, second, approvalA, 1)
	}
	forward(second, 4, approvalA)
	if err := step(second.Sale); err != nil {
		t.Fatal(err)
	}
	reached, release := make(chan struct{}), make(chan struct{})
	var allowResponse sync.Once
	releaseRPC := func() { allowResponse.Do(func() { close(release) }) }
	defer releaseRPC()
	chain.afterSend = func() {
		close(reached)
		<-release
	}
	sendResult := make(chan error, 1)
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now() WHERE id=$1", second.Sale)
	go func() {
		_, err := w.Step(ctx)
		sendResult <- err
	}()
	select {
	case <-reached:
	case <-time.After(3 * time.Second):
		releaseRPC()
		t.Fatal("second transaction did not reach loopback RPC")
	}
	gasIncident := recordFixtureIncident(t, ctx, db, target.Stream)
	releaseRPC()
	if err := <-sendResult; !errors.Is(err, ErrEvidenceHeld) {
		t.Fatalf("stale gas acknowledgement after incident: %v", err)
	}
	chain.afterSend = nil
	var attemptState string
	if err := db.QueryRow(ctx, "SELECT state FROM ledger.relay_attempt WHERE work_id=$1", second.Sale).Scan(&attemptState); err != nil ||
		attemptState != "unknown" {
		t.Fatalf("late gas acknowledgement changed the durable state: %s %v", attemptState, err)
	}
	if err := step(second.Sale); !errors.Is(err, ErrEvidenceHeld) || chain.sends != 2 {
		t.Fatalf("incident did not restrict sent transaction to lookup: %v", err)
	}
	fixtureExec(t, db, "UPDATE chainview.incident SET resolved_at=clock_timestamp() WHERE id=$1", gasIncident)

	if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", second.Sale).Scan(&txHash); err != nil {
		t.Fatal(err)
	}
	paidSecond, err := bind.WaitMinedHash(ctx, client, common.HexToHash(txHash))
	if err != nil || paidSecond.Status != 1 {
		t.Fatalf("second real payment failed %v", err)
	}
	// Advance the finalized chain beyond the quote lifetime. The inclusion
	// block retains the original time and must settle once.
	forceTime(t, ctx, client, second.Order.Expires.Unix()+1)
	waitUntil(t, func() bool { return indexerTip() >= paidSecond.BlockNumber.Int64() })
	if err := step(second.Sale); err != nil {
		t.Fatal(err)
	}
	if chain.sends != 2 {
		t.Fatalf("restarted worker retransmitted a known receipt (%d)", chain.sends)
	}
	var payments int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.payment WHERE sale_id IN ($1,$2)", order.Sale, second.Sale).Scan(&payments); err != nil || payments != 2 {
		t.Fatalf("late finality did not commit exact payment: %d %v", payments, err)
	}

	if os.Getenv("P05_PILOT_OUTPUT") != "" {
		for i, placed := range []seededOrder{order, second} {
			own, other := [2]int{0, 4}[i], [2]int{4, 0}[i]
			cmd := map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: "order.status",
				RequestID: fixtureUUID(t, db), Domain: placed.Branch, Object: placed.Sale, ExpiresAt: time.Now().Unix() + 20,
				Body: map[string]string{}}}
			var ownResponse struct {
				Order commerce.Order `json:"order"`
			}
			if err := json.Unmarshal(post(own, "/v2/commerce", cmd, 200), &ownResponse); err != nil ||
				ownResponse.Order.State != "paid" || ownResponse.Order.ReceiptRef == nil || ownResponse.Order.PickupNo == nil {
				t.Fatalf("customer could not read their finalized receipt: %v", err)
			}
			refused := post(other, "/v2/commerce", cmd, 404)
			if bytes.Contains(refused, []byte(*ownResponse.Order.ReceiptRef)) {
				t.Fatal("another customer obtained a private receipt")
			}
		}
	}

	refundInput := map[string]string{"branchId": order.Branch, "saleId": order.Sale, "idempotencyKey": fixtureUUID(t, db), "amount": pilotUnits(3)}
	post(1, "/v2/finance/refund-reserve", refundInput, 400)
	refundInput["reason"] = "partial_service"
	reserved := post(1, "/v2/finance/refund-reserve", refundInput, 200)
	var request map[string]string
	if err := json.Unmarshal(reserved, &request); err != nil {
		t.Fatal(err)
	}
	refund := request["refundId"]
	decision := map[string]string{"branchId": order.Branch, "saleId": order.Sale, "refundId": refund}
	post(1, "/v2/finance/refund-approve", decision, 404)
	post(2, "/v2/finance/refund-approve", decision, 200)
	post(2, "/v2/finance/refund-submit", decision, 404)
	post(3, "/v2/finance/refund-submit", decision, 200)
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", refund).Scan(&txHash); err != nil {
		t.Fatal(err)
	}
	refunded, err := bind.WaitMinedHash(ctx, client, common.HexToHash(txHash))
	if err != nil || refunded.Status != 1 {
		t.Fatalf("real refund failed: %v", err)
	}
	mine(t, ctx, client, 5)
	waitUntil(t, func() bool { return indexerTip() >= refunded.BlockNumber.Int64() })
	if err := step(refund); err != nil {
		t.Fatal(err)
	}
	expectedReserve := refundReserveFixture(t) - 3*pilotScale()
	deposit, err := contract.EscrowBalance(nil, order.Order.Payer, 1)
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := contract.RefundReserve(nil, order.Quote.Args.MerchantId)
	if err != nil {
		t.Fatal(err)
	}
	beneficiary, err := token.BalanceOf(nil, order.Quote.Args.Beneficiary)
	if err != nil || deposit.Int64() != 3*pilotScale() || reserve.Int64() != expectedReserve || beneficiary.Int64() != 30*pilotScale() {
		t.Fatalf("actual beneficiary/escrow/reserve differ: %v %v %v %v", beneficiary, deposit, reserve, err)
	}
	var journal int
	var day string
	if err := db.QueryRow(ctx, "SELECT count(*),min(business_day)::text FROM ledger.journal WHERE sale_id IN ($1,$2)", order.Sale, second.Sale).
		Scan(&journal, &day); err != nil || journal != 3 {
		t.Fatalf("expected two sales and one refund journal, got %d %v", journal, err)
	}
	if genuine {
		var handoff, reference string
		if err := db.QueryRow(ctx, "SELECT handoff_ref::text,external_ref FROM ledger.pos_order WHERE sale_id=$1", order.Sale).Scan(&handoff, &reference); err != nil {
			t.Fatal(err)
		}
		lookup := map[string]any{"operation": "lookup", "branch": order.Branch, "handoff": handoff}
		actors[2].client.CloseIdleConnections()
		var replay struct {
			Order commerce.Order `json:"order"`
		}
		if err := json.Unmarshal(postPOSFixture(t, ctx, actors[2].client, issuerURL+"/v2/commerce", lookup, 200), &replay); err != nil || replay.Order.ID != order.Sale || replay.Order.State != "paid" || replay.Order.ReceiptRef == nil {
			t.Fatalf("settled POS handoff must return original receipt: %+v %v", replay, err)
		}
		postPOSFixture(t, ctx, actors[0].client, issuerURL+"/v2/commerce", lookup, 404)
		postPOSFixture(t, ctx, actors[3].client, issuerURL+"/v2/commerce",
			map[string]any{"operation": "submit", "branch": order.Branch, "reference": reference, "chainId": 8283, "asset": lower(target.Asset), "amount": pilotUnits(10)}, 200)
		var status struct {
			Order commerce.Order `json:"order"`
		}
		if err := json.Unmarshal(postPOSFixture(t, ctx, actors[3].client, issuerURL+"/v2/commerce",
			map[string]any{"operation": "status", "branch": order.Branch, "reference": reference}, 200), &status); err != nil || status.Order.ID != replay.Order.ID || status.Order.ReceiptRef == nil || *status.Order.ReceiptRef != *replay.Order.ReceiptRef {
			t.Fatalf("source POS must reconcile the same settled receipt: %+v %v", status, err)
		}

		var sales, claims, quotes, payments, effects int
		err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.pos_order WHERE branch_id=$1 AND external_ref=$2),
		 (SELECT count(*) FROM ledger.checkout_claim WHERE sale_id=$3),
		 (SELECT count(*) FROM ledger.quote_work WHERE sale_id=$3),
		 (SELECT count(*) FROM ledger.payment WHERE sale_id=$3),
		 (SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$3 AND kind='paid')`,
			order.Branch, reference, order.Sale).Scan(&sales, &claims, &quotes, &payments, &effects)
		if err != nil || [5]int{sales, claims, quotes, payments, effects} != [5]int{1, 1, 1, 1, 1} {
			t.Fatalf("POS dedup expected one sale, claim, quote, payment and paid effect: %v %v", [5]int{sales, claims, quotes, payments, effects}, err)
		}
	}
	closed := post(2, "/v2/finance/day-close", map[string]string{
		"branchId": order.Branch, "businessDay": day, "asset": lower(target.Asset), "gross": pilotUnits(30), "refunds": pilotUnits(3),
		"evidenceRef": fixtureUUID(t, db)}, 200)
	if !bytes.Contains(closed, []byte(`"net":"`+pilotUnits(27)+`"`)) {
		t.Fatal("net total missing from day close")
	}
	testOpsdProcess(t, ctx, db, dsn, endpoint, evidenceDSN, order, target, requester, approver,
		server.TLS.Certificates[0], clientCA, certs, day, client, token, contract, proveUnpaid, approved)
	if os.Getenv("NU54_NOTIFICATION_BROWSER") != "" {
		var closes, retries, events, receipts int
		if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.verified_finance_close),
			(SELECT count(*) FROM ledger.finance_fulfillment_retry),
			(SELECT count(DISTINCT event_id) FROM ledger.finance_fulfillment_retry),
			(SELECT count(*) FROM ledger.consumer_receipt)`).Scan(&closes, &retries, &events, &receipts); err != nil || closes != 1 || retries != 2 || events != 2 || receipts != 3 {
			t.Fatalf("verified close/retries/distinct events/consumer receipts: %d/%d/%d/%d: %v", closes, retries, events, receipts, err)
		}
		t.Log("notification and close verification: one close, two distinct retry records, three original consumer receipts")
		// This optional journey has already delivered both pending fixtures.
		// The ordinary incident test requires an undelivered event.
		return
	}
	testFulfillmentIncidentRace(t, ctx, db, dsn, proof, target, order, second, trust, certs[0])
	if out := os.Getenv("P05_PILOT_OUTPUT"); out != "" {
		writePilotTrace(t, ctx, db, out, [2]seededOrder{order, second}, [3]*types.Receipt{paid, paidSecond, refunded}, refund, checkLegacy())
		writePilotPublicSlot(t, ctx, db, out, hold, publicWindow, publicPaymentEntered)
	}
	if dir := os.Getenv("NU54_POS_NATIVE_LAB"); dir != "" {
		fundingKey, err := crypto.ToECDSA(common.LeftPadBytes([]byte{100}, 32))
		if err != nil {
			t.Fatal(err)
		}
		funder, err := bind.NewKeyedTransactorWithChainID(fundingKey, big.NewInt(8283))
		if err != nil {
			t.Fatal(err)
		}
		funder.Context = ctx
		mineFunding := func(tx *types.Transaction, err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
			mined, err := bind.WaitMined(ctx, client, tx)
			if err != nil || mined.Status != types.ReceiptStatusSuccessful {
				t.Fatalf("native POS fixture funding: %v", err)
			}
		}
		funding := big.NewInt(6_500_000)
		mineFunding(token.Mint(funder, funder.From, funding))
		mineFunding(token.Approve(funder, target.Contract, funding))
		mineFunding(contract.DepositEscrow(funder, buyers[0].Address(), 1, funding))
		verifyPOSNativeLab(t, ctx, db, dir, issuerURL, order, actors, func(posOrder seededOrder) {
			approval := pilotBoardApproval(t, ctx, posOrder, buyers[0], "payment")
			verifyPilotClientApproval(t, ctx, posOrder, approval, 2)
			forward(posOrder, 0, approval)
			if err := step(posOrder.Sale); err != nil {
				t.Fatal(err)
			}
			// Earlier regressions advance Anvil beyond an old quote's expiry.
			// Set the next block explicitly for the new, wall-clock signed quote.
			h, err := client.HeaderByNumber(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			next := max(h.Time+1, posOrder.Quote.Args.IssuedAt)
			if next >= posOrder.Quote.Args.ExpiresAt {
				t.Fatalf("native POS fixture would mine beyond the live quote: head=%d, issued=%d, expires=%d",
					h.Time, posOrder.Quote.Args.IssuedAt, posOrder.Quote.Args.ExpiresAt)
			}
			if err := client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", next); err != nil {
				t.Fatal(err)
			}
			if err := step(posOrder.Sale); err != nil {
				t.Fatal(err)
			}
			var hash string
			if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", posOrder.Sale).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			mined, err := bind.WaitMinedHash(ctx, client, common.HexToHash(hash))
			if err != nil {
				t.Fatal(err)
			}
			if mined.Status != types.ReceiptStatusSuccessful {
				tx, _, txErr := client.TransactionByHash(ctx, mined.TxHash)
				if txErr != nil {
					t.Fatal(txErr)
				}
				_, revert := client.CallContract(ctx, ethereum.CallMsg{From: funder.From, To: tx.To(), Data: tx.Data()}, mined.BlockNumber)
				t.Fatalf("native POS payment failed on Anvil at timestamp %d with quote %d..%d: %v",
					next, posOrder.Quote.Args.IssuedAt, posOrder.Quote.Args.ExpiresAt, revert)
			}
			mine(t, ctx, client, 5)
			waitUntil(t, func() bool { return indexerTip() >= mined.BlockNumber.Int64() })
			if err := step(posOrder.Sale); err != nil {
				t.Fatal(err)
			}
		})
	}

	if os.Getenv("NU54_REFUND_RETRY_TEST") != "" || os.Getenv("NU54_REFUND_REVIEW_BROWSER") != "" {
		t.Run("FinalizedRefundRetry", func(t *testing.T) {
			var browser func(string, string, string, int)
			if script := os.Getenv("NU54_REFUND_REVIEW_BROWSER"); script != "" {
				browser = refundReviewBrowser(t, ctx, script, order, certs, trust, FinanceHTTP{
					Store: financeStore, Funds: LiveChain{client}, Targets: workerProfile,
					Merchants: map[string]string{order.Branch: (common.Hash{31: 50}).Hex()},
					Credentials: map[string]service.Principal{
						requesterFP:           {Role: "finance", Tenant: order.Tenant, Subject: requester, ExpiresAt: expires},
						approverFP:            {Role: "finance", Tenant: order.Tenant, Subject: approver, ExpiresAt: expires},
						fingerprint(certs[3]): {Role: "finance", Tenant: order.Tenant, Subject: manualTreasury, ExpiresAt: expires},
					},
				})
				browser("pending", "", "", 1)
			}
			verifyFinalizedRefundRetry(t, ctx, db, client, &w, order, step, indexerTip, post, browser)
		})
	}
	// The recovery snapshot requires a stopped writer. All live chain tests
	// must have received finality before the writer is stopped.
	indexer()
	if root := os.Getenv("NU54_RECOVERY_REHEARSAL_OUTPUT"); root != "" {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		raw, err := json.MarshalIndent(orphanRestart, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "p07-orphan-restart.json"), append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		probe := exec.CommandContext(ctx, "python3", "../../../p10-platform/recovery/rehearse.py",
			"--dsn", dsn, "--rpc", endpoint, "--output", root)
		if report, err := probe.CombinedOutput(); err != nil {
			t.Fatalf("isolated backup/restore rehearsal failed: %v\n%s", err, report)
		} else {
			t.Logf("isolated backup/restore rehearsal: %s", report)
		}
	}
}

var errLostResponse = errors.New("fixture network response lost after actual RPC")

type lostResponseChain struct {
	LiveChain
	sends     int
	afterSend func()
}

func (c *lostResponseChain) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	c.sends++
	err := c.LiveChain.SendTransaction(ctx, tx)
	if c.afterSend != nil {
		c.afterSend()
	}
	if err == nil && c.sends == 1 {
		return errLostResponse
	}
	return err
}

func pilotBoardApproval(t *testing.T, ctx context.Context, order seededOrder, buyer fixtureSigner, route string) []byte {
	t.Helper()
	return pilotBoardDecision(t, ctx, order, buyer, route, nil, false)
}

func pilotBoardDecision(t *testing.T, ctx context.Context, order seededOrder, buyer fixtureSigner, route string, displayEnv []string, refused bool) []byte {
	t.Helper()
	runner := os.Getenv("NU54_P01_INSTALL_TEST")
	if len(displayEnv) != 0 && runner == "" {
		t.Fatal("independent display approval requires the P01 host executable")
	}
	if runner == "" && os.Getenv("P05_PILOT_OUTPUT") != "" {
		t.Fatal("pilot evidence requires the P01 host executable")
	}
	if runner == "" {
		signature, err := buyer.SignDigest(ctx, order.Quote.ApprovalHash)
		if err != nil {
			t.Fatal(err)
		}
		return signature
	}
	name := filepath.Join(t.TempDir(), "order")
	frame := bytes.Clone(order.Signed)
	frame = append(frame, fixtureSigner(101).Address().Bytes()...)
	frame = append(frame, common.LeftPadBytes([]byte{byte(buyer)}, 32)...)
	if err := os.WriteFile(name, frame, 0o600); err != nil {
		t.Fatal(err)
	}
	mode := "--approve-one"
	if order.Quote.Args.Amount.Cmp(big.NewInt(1000000)) > 0 {
		mode = "--approve-commercial-testnet"
	}
	command := exec.CommandContext(ctx, runner, mode, route, name)
	if len(displayEnv) != 0 {
		command.Env = append(os.Environ(), displayEnv...)
	}
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	signature, err := command.Output()
	if refused {
		if err != nil || len(signature) != 0 {
			t.Fatalf("P01 host recorded a signature after local refusal: %s, %v", diagnostic.String(), err)
		}
		return nil
	}
	if err != nil || len(signature) != 65 || !matchesSigner(order.Quote.ApprovalHash, signature, buyer.Address()) {
		t.Fatalf("P01 host approval did not authorize the exact order: %s, %v", diagnostic.String(), err)
	}
	return signature
}

func verifyPilotClientApproval(t *testing.T, ctx context.Context, order seededOrder, signature []byte, index int) {
	t.Helper()
	input := map[string]any{"approvalIndex": index, "branch": order.Branch, "profile": map[string]any{
		"chain": 8283, "contract": lower(order.Target.Contract), "asset": lower(order.Target.Asset),
		"beneficiary": lower(order.Quote.Args.Beneficiary), "merchantId": rawHex(order.Quote.Args.MerchantId[:]),
		"signer": lower(fixtureSigner(101).Address()), "keyVersion": 1, "beneficiaryVersion": 1, "maxBaseUnits": pilotUnits(30), "presentationKey": rawHex(pilotPresentationKey()),
	}}
	digest, orderKey := rawHex(order.Quote.Digest[:]), rawHex(order.Order.OrderID[:])
	input["humanUnits"] = pilotScale() == 1_000_000
	input["current"] = map[string]any{
		"actor": order.Actor, "signature": rawHex(signature), "request": rawHex(order.Order.OrderID[:16]),
		"quote": map[string]any{"saleRef": order.Sale, "branchRef": order.Branch, "orderKey": orderKey,
			"signedQuote": rawHex(order.Signed), "quoteDigest": digest, "expiresAt": order.Order.Expires.Unix(), "presentation": order.Presentation},
		"order": map[string]any{"id": order.Sale, "revision": 1, "state": "issued", "asset": lower(order.Target.Asset),
			"amount": order.Order.Amount, "quoteDigest": digest, "orderKey": orderKey, "expiresAt": order.Order.Expires},
	}

	run := func(index int, expected string) {
		t.Helper()
		input["approvalIndex"] = index
		payload, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		clients := exec.CommandContext(ctx, "node", "../../test/settlement/pilot-client-wire.mjs")
		clients.Stdin = bytes.NewReader(payload)
		var diagnostic bytes.Buffer
		clients.Stderr = &diagnostic
		output, err := clients.Output()
		var verified string
		if err != nil || json.Unmarshal(output, &verified) != nil || verified != expected {
			t.Fatalf("P02/P04 synthetic transport approval %d: %s (%v)", index, diagnostic.String(), err)
		}
	}
	if index == 0 {
		run(-1, "synthetic-owner-session-ready")
	}
	run(index, rawHex(signature))
}

func fingerprint(cert tls.Certificate) string {
	hash := sha256.Sum256(cert.Certificate[0])
	return hex.EncodeToString(hash[:])
}
func mtlsClients(t *testing.T, n int, additional ...any) (*x509.CertPool, []tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour)}
	bytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := x509.ParseCertificate(bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parent)
	var certs []tls.Certificate
	for i := 0; i < n; i++ {
		leaf, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			KeyUsage: x509.KeyUsageDigitalSignature, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter}
		der, err := x509.CreateCertificate(rand.Reader, cert, parent, &leaf.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		certs = append(certs, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: leaf})
	}
	for i, public := range additional {
		cert := &x509.Certificate{SerialNumber: big.NewInt(int64(n + i + 2)), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			KeyUsage: x509.KeyUsageDigitalSignature, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter}
		der, err := x509.CreateCertificate(rand.Reader, cert, parent, public, key)
		if err != nil {
			t.Fatal(err)
		}
		certs = append(certs, tls.Certificate{Certificate: [][]byte{der}})
	}

	return pool, certs, bytes
}

func deployFixture(t *testing.T, ctx context.Context, client *ethclient.Client, db *pgxpool.Pool) (Target, *types.Receipt, *testusdc.Testusdc, *v2.Settlementv2, func() map[string]any) {
	t.Helper()
	return deployFixtureWithOperator(t, ctx, client, db, nil, os.Getenv("P05_PILOT_OUTPUT") != "")
}

func deployFixtureWithOperator(t *testing.T, ctx context.Context, client *ethclient.Client, db *pgxpool.Pool, key *ecdsa.PrivateKey, includePilot bool) (Target, *types.Receipt, *testusdc.Testusdc, *v2.Settlementv2, func() map[string]any) {
	t.Helper()
	if key == nil {
		key, _ = crypto.ToECDSA(append(make([]byte, 31), 100))
	}
	operator, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	operator.Context = ctx
	operator.GasLimit = 10_000_000
	if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", operator.From, "0x56BC75E2D63100000"); err != nil {
		t.Fatal(err)
	}
	mined := func(tx *types.Transaction, err error) *types.Receipt {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		r, err := bind.WaitMined(ctx, client, tx)
		if err != nil || r.Status != 1 {
			t.Fatalf("fixture deployment reverted %v", err)
		}
		return r
	}
	deploy := func(name string, args ...any) (common.Address, *types.Receipt) {
		t.Helper()
		address, tx, err := deployArtifact(t, operator, client, name, args...)
		return address, mined(tx, err)
	}
	asset, _ := deploy("TestUSDC", operator.From)
	address, deployment := deploy("SettlementV2", asset, operator.From, false)
	token, err := testusdc.NewTestusdc(asset, client)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := v2.NewSettlementv2(address, client)
	if err != nil {
		t.Fatal(err)
	}
	checkLegacy := func() map[string]any { return nil }
	if includePilot {
		registry, _ := deploy("MerchantRegistry", operator.From, big.NewInt(300))
		legacyAddr, _ := deploy("PaymentSettlement", asset, registry, operator.From, big.NewInt(20), big.NewInt(40), big.NewInt(300), big.NewInt(60))
		legacy, err := v1.NewSettlement(legacyAddr, client)
		if err != nil {
			t.Fatal(err)
		}
		// The same integration database can exercise multiple deployments.
		// This pending legacy account belongs to the exact legacy contract.
		pendingDevice := common.BytesToAddress(crypto.Keccak256Hash([]byte("nu54-legacy-rental-fixture"), legacyAddr.Bytes()).Bytes())
		mined(token.Mint(operator, operator.From, big.NewInt(7)))
		mined(token.Approve(operator, legacyAddr, big.NewInt(7)))
		deposit := mined(legacy.DepositFor(operator, pendingDevice, big.NewInt(7), pendingDevice))
		fixtureExec(t, db, "INSERT INTO public.rentals(device,withdraw_addr,state,deposit_tx) VALUES($1,$1,'active',$2)",
			lower(pendingDevice), deposit.TxHash.Hex())
		checkLegacy = func() map[string]any {
			t.Helper()
			balance, err := legacy.BalanceOf(nil, pendingDevice)
			if err != nil || balance.Int64() != 7 {
				t.Fatalf("v1 pending escrow lost during v2 settlement: %v %v", balance, err)
			}
			var state, hash string
			if err := db.QueryRow(ctx, "SELECT state,deposit_tx FROM public.rentals WHERE device=$1", lower(pendingDevice)).
				Scan(&state, &hash); err != nil || state != "active" || hash != deposit.TxHash.Hex() {
				t.Fatalf("v1 active rental changed: %s %s %v", state, hash, err)
			}
			return map[string]any{"depositHash": hash, "escrowBaseUnits": balance.String(), "state": state}
		}
		checkLegacy()
	}
	merchant := [32]byte{31: 50}
	payer, beneficiary := fixtureSigner(102).Address(), fixtureSigner(104).Address()
	mined(contract.RegisterMerchant(operator, merchant, fixtureSigner(101).Address(), beneficiary, fixtureSigner(103).Address()))
	mined(contract.RegisterPayer(operator, payer, 1, 1, payer))
	secondPayer := fixtureSigner(105).Address()
	mined(contract.RegisterPayer(operator, secondPayer, 1, 1, secondPayer))
	mined(token.Mint(operator, operator.From, big.NewInt(60*pilotScale())))
	mined(token.Approve(operator, address, big.NewInt(60*pilotScale())))
	mined(contract.DepositEscrow(operator, payer, 1, big.NewInt(10*pilotScale())))
	mined(contract.DepositEscrow(operator, secondPayer, 1, big.NewInt(20*pilotScale())))
	mined(contract.FundRefundReserve(operator, merchant, big.NewInt(refundReserveFixture(t))))
	code, err := client.CodeAt(ctx, address, deployment.BlockNumber)
	if err != nil {
		t.Fatal(err)
	}
	return Target{Stream: fixtureUUID(t, db), Manifest: strings.Repeat("23", 32), Contract: address, CodeHash: crypto.Keccak256Hash(code),
		Asset: asset, RefundSigner: fixtureSigner(103).Address(), RefundVersion: 1}, deployment, token, contract, checkLegacy
}

func deployArtifact(t *testing.T, operator *bind.TransactOpts, client *ethclient.Client, name string, args ...any) (common.Address, *types.Transaction, error) {
	t.Helper()
	path := filepath.Join(cmp.Or(os.Getenv("FOUNDRY_OUT"), "../../../../products/p06-stablenet-contracts/out"), name+".sol", name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct {
		ABI      json.RawMessage
		Bytecode struct{ Object hexutil.Bytes }
	}
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatal(err)
	}
	abi, err := abi.JSON(bytes.NewReader(artifact.ABI))
	if err != nil {
		t.Fatal(err)
	}
	address, tx, _, err := bind.DeployContract(operator, abi, artifact.Bytecode.Object, client, args...)
	return address, tx, err
}

func refundReserveFixture(t *testing.T) int64 {
	t.Helper()
	if os.Getenv("NU54_REFUND_LOW_TEST") != "" {
		if pilotScale() != 1000000 {
			t.Fatal("low-funds scenario requires human TestUSDC amounts")
		}
		return 3500000
	}
	return 30 * pilotScale()
}

type fixtureDeploymentApproval struct {
	Signer                    ed25519.PrivateKey
	Profiles, Signatures, Key string
	Profile                   deployment.Profile
}

func startFixtureIndexer(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn, endpoint, binary string, target Target, deployed *types.Receipt, approved ...*fixtureDeploymentApproval) (func(), func(func()) orphanIndexerRestart, int64, func(*testing.T, seededOrder)) {
	t.Helper()
	_, ingestDSN := testLogin(t, db, dsn, "nu54_p07_ingest")
	generation := fixtureUUID(t, db)
	file := filepath.Join(t.TempDir(), "profiles.json")
	profile := deployment.Profile{StreamID: target.Stream, GenerationID: generation, Version: target.profileVersion(), ChainID: 8283, Contract: target.Contract,
		Asset: target.Asset, ABIHash: target.ABIHash(), Manifest: target.Manifest, CodeHash: target.CodeHash,
		StartHash: deployed.BlockHash, StartHeight: deployed.BlockNumber.Int64()}
	raw, _ := json.Marshal([]deployment.Profile{profile})
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var approvalEnv []string
	if len(approved) == 1 {
		key, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(profile)
		signature := ed25519.Sign(private, append([]byte("NU54-chainview-profile-v1\x00"), body...))
		signatures, _ := json.Marshal(map[string]string{generation: rawHex(signature)})
		path := filepath.Join(filepath.Dir(file), "profile-signatures.json")
		if err := os.WriteFile(path, signatures, 0600); err != nil {
			t.Fatal(err)
		}
		*approved[0] = fixtureDeploymentApproval{Profiles: file, Signatures: path, Key: rawHex(key), Profile: profile, Signer: private}
		approvalEnv = []string{"P07_CHAINVIEW_APPROVAL_KEY=" + rawHex(key), "P07_CHAINVIEW_SIGNATURES=" + path}
	}
	rpc := restartFixtureProxy(t, endpoint)
	var process *exec.Cmd
	var logFile *os.File
	stop := func() {
		if process == nil {
			return
		}
		_ = process.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = process.Process.Kill()
			<-done
		}
		_ = logFile.Close()
		process = nil
	}
	start := func() {
		t.Helper()
		log, err := os.Create(filepath.Join(t.TempDir(), "p07.log"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, binary, "chainview")
		cmd.Env = append(os.Environ(), "P07_CHAINVIEW_DATABASE_URL="+ingestDSN, "P07_CHAINVIEW_PROFILES="+file, "P07_RPC="+rpc.URL, "P07_POLL=50ms")
		cmd.Env = append(cmd.Env, approvalEnv...)
		cmd.Stderr = log
		if err := cmd.Start(); err != nil {
			log.Close()
			t.Fatal(err)
		}
		process, logFile = cmd, log
	}
	t.Cleanup(func() {
		var name string
		if logFile != nil {
			name = logFile.Name()
		}
		stop()
		if t.Failed() && name != "" {
			if output, err := os.ReadFile(name); err == nil {
				t.Logf("P07 indexer: %s", output)
			}
		}
	})
	if len(approved) == 1 {
		badFile := filepath.Join(filepath.Dir(file), "bad-signatures.json")
		if err := os.WriteFile(badFile, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		approvalEnv[1] = "P07_CHAINVIEW_SIGNATURES=" + badFile
		start()
		waitUntil(t, func() bool {
			var held bool
			err := db.QueryRow(ctx, `SELECT EXISTS (SELECT FROM chainview.profile_hold
				WHERE stream_id=$1 AND generation_id=$2 AND reason='unapproved_profile')
				AND NOT EXISTS(SELECT FROM chainview.generation WHERE id=$2)`, target.Stream, generation).Scan(&held)
			return err == nil && held
		})
		stop()
		approvalEnv[1] = "P07_CHAINVIEW_SIGNATURES=" + approved[0].Signatures
		t.Log("unsigned P07 candidate held without generation; signed retry accepted")
	}
	start()
	var height *int64
	waitUntil(t, func() bool {
		_ = db.QueryRow(ctx, "SELECT finalized_height FROM chainview.generation WHERE id=$1", generation).Scan(&height)
		return height != nil
	})
	var switchID string
	err := db.QueryRow(ctx, `INSERT INTO chainview.switch_request(id,stream_id,target_id,expected_revision,operator_ref,compare_ref,requested_at)
		VALUES(gen_random_uuid(),$1,$2,0,gen_random_uuid(),chainview.record_comparison($1,$2,0),now()) RETURNING id::text`,
		target.Stream, generation).Scan(&switchID)
	if err != nil {
		t.Fatal(err)
	}
	fixtureExec(t, db, "SELECT chainview.apply_switch($1)", switchID)
	restart := func(advance func()) orphanIndexerRestart {
		t.Helper()
		stall := &fixtureRPCStall{entered: make(chan struct{}), release: make(chan struct{})}
		if !rpc.stall.CompareAndSwap(nil, stall) {
			t.Fatal("indexer poll already paused")
		}
		defer close(stall.release)
		select {
		case <-stall.entered:
		case <-time.After(6 * time.Second):
			t.Fatal("indexer did not reach the RPC after obtaining its durable lease")
		}
		probe := func(ctx context.Context) (indexerClock, error) {
			var state indexerClock
			err := db.QueryRow(ctx, `SELECT clock_timestamp(),fence,lease_until,tip_height,finalized_height
				FROM chainview.generation WHERE id=$1`, generation).Scan(&state.At, &state.Fence, &state.ExpiresAt, &state.Tip, &state.Finalized)
			return state, err
		}
		crashed, err := probe(ctx)
		if err != nil || crashed.ExpiresAt == nil || !crashed.At.Before(*crashed.ExpiresAt) {
			t.Fatalf("cannot establish that an in-flight poll has an active lease: %v", err)
		}
		if err := process.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		_ = process.Wait()
		_ = logFile.Close()
		process = nil
		start()
		advance()
		recovery, cancel := context.WithTimeout(ctx, orphanIndexerRecoveryLimit)
		defer cancel()
		var checkedFence int
		ticker := time.NewTicker(75 * time.Millisecond)
		defer ticker.Stop()
		for {
			state, err := probe(recovery)
			if err != nil {
				t.Fatalf("inspect restarted indexer: %v", err)
			}
			if state.At.Before(*crashed.ExpiresAt) {
				if state.Fence != crashed.Fence || state.Tip != crashed.Tip || state.Finalized != crashed.Finalized {
					t.Fatal("replacement indexer wrote before old lease expired")
				}
				checkedFence++
			} else if state.Fence > crashed.Fence && state.Tip > crashed.Tip && state.Finalized >= crashed.Finalized {
				if checkedFence < 3 {
					t.Fatal("replacement was not observed while the previous lease was active")
				}
				result := orphanIndexerRestart{Crashed: crashed, Recovered: state, ChecksBeforeExpiry: checkedFence,
					RecoveryMillis: state.At.Sub(crashed.At).Milliseconds()}
				t.Logf("P07 orphan lease recovered after %dms, fence %d→%d, tip %d→%d, %d checks before expiry",
					result.RecoveryMillis, crashed.Fence, state.Fence, crashed.Tip, state.Tip, checkedFence)
				return result
			}
			select {
			case <-recovery.Done():
				t.Fatalf("replacement indexer did not recover within 60 seconds: %v", recovery.Err())
			case <-ticker.C:
			}
		}
	}
	return stop, restart, *height, func(t *testing.T, item seededOrder) {
		t.Helper()
		for attempt := 0; ; attempt++ {
			cmd := exec.CommandContext(ctx, binary, "prove-absence", generation, common.Hash(item.Quote.Args.MerchantId).Hex(),
				common.Hash(item.Quote.Args.OrderId).Hex(), strconv.FormatInt(item.Order.Expires.Unix(), 10))
			cmd.Env = append(os.Environ(), "P07_CHAINVIEW_DATABASE_URL="+ingestDSN, "P07_CHAINVIEW_PROFILES="+file, "P07_RPC="+endpoint)
			out, err := cmd.CombinedOutput()
			if err == nil {
				return
			}
			if attempt == 9 || (!bytes.Contains(out, []byte("active checkpoint changed before proof commit")) &&
				!bytes.Contains(out, []byte("active checkpoint advanced before proof commit"))) {
				t.Fatalf("scoped P07 absence proof: %v %s", err, out)
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

type indexerClock struct {
	At        time.Time  `json:"observedAt"`
	Fence     int64      `json:"fence"`
	ExpiresAt *time.Time `json:"leaseExpiresAt,omitempty"`
	Tip       int64      `json:"tipHeight"`
	Finalized int64      `json:"finalizedHeight"`
}

type orphanIndexerRestart struct {
	Crashed            indexerClock `json:"crashed"`
	Recovered          indexerClock `json:"recovered"`
	ChecksBeforeExpiry int          `json:"checksBeforeExpiry"`
	RecoveryMillis     int64        `json:"recoveryMillis"`
	PaymentEvidence    string       `json:"paymentEvidence"`
	SaleJournals       int          `json:"saleJournals"`
	PaidOutbox         int          `json:"paidOutbox"`
}

type fixtureRPCStall struct {
	entered chan struct{}
	release chan struct{}
}

type fixtureIndexerProxy struct {
	*httptest.Server
	stall atomic.Pointer[fixtureRPCStall]
}

func restartFixtureProxy(t *testing.T, endpoint string) *fixtureIndexerProxy {
	t.Helper()
	proxy := &fixtureIndexerProxy{}
	// Reuse connections: a fast poll over many blocks otherwise uses up the local ports.
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 64}}
	proxy.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(req.Body, 2<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var rpc struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.Unmarshal(raw, &rpc) == nil && rpc.Method == "eth_getBlockByNumber" &&
			len(rpc.Params) != 0 && string(rpc.Params[0]) == `"latest"` {
			if stall := proxy.stall.Swap(nil); stall != nil {
				close(stall.entered)
				select {
				case <-req.Context().Done():
					return
				case <-stall.release:
				}
			}
		}
		downstream, err := http.NewRequestWithContext(req.Context(), http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		downstream.Header.Set("Content-Type", "application/json")
		res, err := client.Do(downstream)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer res.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
	}))
	t.Cleanup(proxy.Close)
	return proxy
}

// closeExpired lets the worker ask for the proof that an expired order was never paid, and close it.
func closeExpired(t *testing.T, db *pgxpool.Pool, order seededOrder, step func() error) {
	t.Helper()
	waitUntil(t, func() bool {
		var state string
		if err := db.QueryRow(context.Background(), "SELECT state FROM ledger.sale WHERE id=$1", order.Sale).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "closed" {
			return true
		}
		if err := step(); err != nil {
			t.Log(err)
		}
		return false
	})
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	until := time.Now().Add(9 * time.Second)
	for time.Now().Before(until) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("fixture did not reach expected state on time")
}

func mine(t *testing.T, ctx context.Context, client *ethclient.Client, n int) {
	t.Helper()
	if err := client.Client().CallContext(ctx, nil, "anvil_mine", n); err != nil {
		t.Fatal(err)
	}
}

func forceTime(t *testing.T, ctx context.Context, client *ethclient.Client, ts int64) {
	t.Helper()
	if err := client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", ts); err != nil {
		t.Fatal(err)
	}
	mine(t, ctx, client, 5)
}

func testOpsdProcess(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn, endpoint, evidenceDSN string,
	order seededOrder, target Target, requester, approver string, serverCert tls.Certificate, clientCA []byte, clients []tls.Certificate, day string, chain *ethclient.Client, token *testusdc.Testusdc, deployed *v2.Settlementv2, proveUnpaid func(*testing.T, seededOrder), approved []*fixtureDeploymentApproval) {
	t.Helper()
	treasury := fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'refund_submit',now()+interval '1 hour')", order.Branch, treasury)
	binary := os.Getenv("P05_SETTLEMENT_TEST_OPSD")
	if binary == "" {
		t.Fatal("settlement smoke requires the actual opsd binary")
	}
	// A killed opsd cannot remove its pinned signer copies. The owning test can.
	tempDir := t.TempDir()
	tempEnv := "TMPDIR=" + tempDir
	for _, inputs := range []struct {
		env   []string
		error string
	}{
		{[]string{"P05_SETTLEMENT_TARGETS=missing.json"}, "P05_COMMERCE_DATABASE_URL"},
		{[]string{"P05_SUBSCRIPTION_WORKER_DATABASE_URL=unused"}, "P05_COMMERCE_DATABASE_URL"},
		{[]string{"P05_FINANCE_DATABASE_URL=unused", "P05_COMMERCE_DATABASE_URL=unused"}, "P05_SETTLEMENT_TARGETS"},
		{[]string{"P05_REFUND_SIGNER_SHA256="}, "P05_COMMERCE_DATABASE_URL"},
		{[]string{"P05_DISPATCH_CERT=unused", "P05_SETTLEMENT_TARGETS=unused", "P05_COMMERCE_DATABASE_URL=unused"}, "P05_FULFILLMENT_TARGETS"},
	} {
		failureCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		cmd := exec.CommandContext(failureCtx, binary)
		cmd.Env = append([]string{"P05_HTTP_ADDR=127.0.0.1:0", tempEnv}, inputs.env...)
		out, err := cmd.CombinedOutput()
		timeout := failureCtx.Err()
		cancel()
		if err == nil || timeout != nil || !strings.Contains(string(out), inputs.error) {
			t.Fatalf("incomplete enrollment did not fail immediately: %v %s", err, out)
		}
	}
	start := func(config []string, enroll func(string) []string) (string, func()) {
		t.Helper()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := listener.Addr().String()
		listener.Close()
		cmd := exec.CommandContext(ctx, binary)
		cmd.Env = append([]string{"P05_HTTP_ADDR=" + addr, tempEnv}, config...)
		if enroll != nil {
			cmd.Env = append(cmd.Env, enroll(addr)...)
		}
		var output bytes.Buffer
		cmd.Stderr = &output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		var once sync.Once
		stop := func() {
			once.Do(func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				if t.Failed() {
					t.Log(output.String())
				}
			})
		}
		t.Cleanup(stop)
		return addr, stop
	}
	legacy := &http.Client{Timeout: 300 * time.Millisecond}
	probe := func(client *http.Client, location string) bool {
		response, err := client.Get(location + "/healthz")
		if err != nil {
			return false
		}
		response.Body.Close()
		return response.StatusCode == http.StatusOK
	}
	legacyAddr, stopLegacy := start(nil, nil)
	defer stopLegacy()
	legacyURL := "http://" + legacyAddr
	waitUntil(t, func() bool { return probe(legacy, legacyURL) })

	dir := t.TempDir()
	write := func(name string, raw []byte, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	settings := func(name string, value any) string {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return name + "=" + write(name, raw, 0o600)
	}
	pemBlock := func(kind string, der []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}) }
	serverCert = nativeSubscriptionServer(t, serverCert)
	serverPEM := pemBlock("CERTIFICATE", serverCert.Certificate[0])
	serverTrust := serverCert.Certificate[0]
	for _, chainCert := range serverCert.Certificate[1:] {
		serverPEM = append(serverPEM, pemBlock("CERTIFICATE", chainCert)...)
		serverTrust = chainCert
	}
	serverKey, err := x509.MarshalPKCS8PrivateKey(serverCert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).Unix()
	commerceGrants := map[string]service.Principal{
		fingerprint(clients[0]): {Role: "customer", Tenant: order.Tenant, Subject: order.Actor, ExpiresAt: expires},
	}
	nativeT06 := os.Getenv("NU54_TRAVEL_NATIVE_T06") == "1"
	t06 := os.Getenv("NU54_TRAVEL_T06_HOST") == "1" || nativeT06
	if t06 {
		pickupOther, terminal := fixtureUUID(t, db), fixtureUUID(t, db)
		commerceGrants[fingerprint(clients[4])] = service.Principal{Role: "terminal", Tenant: order.Tenant, Subject: terminal, ExpiresAt: expires}
		commerceGrants[fingerprint(clients[1])] = service.Principal{Role: "customer", Tenant: order.Tenant, Subject: pickupOther, ExpiresAt: expires}
		fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'terminal','menu',now()+interval '1 hour'),($1,$3,'customer',NULL,now()+interval '1 hour')", order.Branch, terminal, pickupOther)
		fixtureExec(t, db, "INSERT INTO ledger.travel_pickup_terminal VALUES($1,$2)", order.Branch, terminal)
	}
	subscriptionTest := os.Getenv("P05_PILOT_OUTPUT") == "" && os.Getenv("NU54_PRINT_NATIVE_LAB") == ""
	experienceScript := os.Getenv("NU54_EXPERIENCE_BROWSER_SCRIPT")
	if experienceScript != "" {
		if !subscriptionTest {
			t.Fatal("experience browser requires the travel and subscription fixtures")
		}
		commerceGrants[fingerprint(clients[2])] = service.Principal{Role: "operator", Tenant: order.Tenant, Subject: approver, ExpiresAt: expires}
		terminal := fixtureUUID(t, db)
		fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'terminal','pos',now()+interval '1 hour')", order.Branch, terminal)
		commerceGrants[fingerprint(clients[3])] = service.Principal{Role: "terminal", Tenant: order.Tenant, Subject: terminal, ExpiresAt: expires}
	}
	if subscriptionTest {
		commerceGrants[fingerprint(clients[5])] = commerceGrants[fingerprint(clients[0])]
		profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
			Signer: fixtureSigner(101).Address(), MerchantID: order.Quote.Args.MerchantId, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}}
		digest := profile.Digest()
		fixtureExec(t, db, "INSERT INTO ledger.issuance_profile VALUES($1,1,$2,120,true) ON CONFLICT(branch_id) DO UPDATE SET quote_lifetime_seconds=120", order.Branch, digest[:])
		fixtureExec(t, db, "INSERT INTO ledger.payer_enrollment VALUES($1,$2,$3,1,1,now()+interval '1 hour') ON CONFLICT DO NOTHING", order.Branch, order.Actor, fixtureSigner(102).Address().Bytes())
	}
	sameSKU := os.Getenv("P05_PILOT_SAME_SKU") == "1"
	var independent []pilotIdentity
	if len(clients) > 6 {
		for i, cert := range clients[6:] {
			owner := pilotIdentity{actor: fixtureUUID(t, db), board: fixtureSigner(151 + i), cert: cert}
			commerceGrants[fingerprint(cert)] = service.Principal{Role: "customer", Tenant: order.Tenant, Subject: owner.actor, ExpiresAt: expires}
			fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'customer',NULL,now()+interval '1 hour')", order.Branch, owner.actor)
			fixtureExec(t, db, "INSERT INTO ledger.payer_enrollment VALUES($1,$2,$3,1,1,now()+interval '1 hour')", order.Branch, owner.actor, owner.board.Address().Bytes())
			independent = append(independent, owner)
		}
		profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
			Signer: fixtureSigner(101).Address(), MerchantID: order.Quote.Args.MerchantId, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}}
		digest := profile.Digest()
		fixtureExec(t, db, "INSERT INTO ledger.issuance_profile VALUES($1,1,$2,120,true) ON CONFLICT(branch_id) DO UPDATE SET quote_lifetime_seconds=120", order.Branch, digest[:])
		seedIndependentInventory(t, db, order, sameSKU)
	}
	nativePrincipal := commerceGrants[fingerprint(clients[0])]
	kioskOutcome := os.Getenv("NU54_KIOSK_OUTCOME_LAB") != ""
	var kioskOtherCustomer pilotIdentity
	if kioskOutcome && os.Getenv("NU54_KIOSK_BINDING_RECOVERY") == "1" {
		kioskOtherCustomer = pilotIdentity{actor: fixtureUUID(t, db), board: fixtureSigner(150), cert: clients[4]}
		commerceGrants[fingerprint(kioskOtherCustomer.cert)] = service.Principal{Role: "customer", Tenant: order.Tenant, Subject: kioskOtherCustomer.actor, ExpiresAt: expires}
		fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'customer',NULL,now()+interval '1 hour')", order.Branch, kioskOtherCustomer.actor)
		fixtureExec(t, db, "INSERT INTO ledger.payer_enrollment VALUES($1,$2,$3,1,1,now()+interval '1 hour')", order.Branch, kioskOtherCustomer.actor, kioskOtherCustomer.board.Address().Bytes())
	}
	if kioskOutcome {
		terminal := fixtureUUID(t, db)
		nativePrincipal = service.Principal{Role: "terminal", Tenant: order.Tenant, Subject: terminal, ExpiresAt: expires}
		fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'terminal','menu',now()+interval '1 hour')", order.Branch, terminal)
		commerceGrants[fingerprint(clients[3])] = nativePrincipal
	}
	clientRoots, nativeSubscription := enrollNativeSubscriptionLab(t, serverCert, pemBlock("CERTIFICATE", clientCA), commerceGrants, nativePrincipal)
	config := []string{
		"P05_COMMERCE_CA=" + write("ca.pem", clientRoots, 0o600),
		"P05_COMMERCE_CERT=" + write("server.pem", serverPEM, 0o600),
		"P05_COMMERCE_KEY=" + write("server-key.pem", pemBlock("PRIVATE KEY", serverKey), 0o600),
		"P05_P07_RECONCILIATION_DATABASE_URL=" + evidenceDSN,
		"P05_SETTLEMENT_RPC=" + endpoint,
		"P05_RELAY_ADDRESS=" + fixtureSigner(100).Address().Hex(),
		"P05_RELAY_MAX_GAS_PRICE=100000000000",
		settings("P05_SETTLEMENT_TARGETS", map[string]Target{order.Branch: target}),
		settings("P05_RECEIVER_GRANTS", map[string]ReceiverGrant{
			fingerprint(clients[3]): {Branches: []string{order.Branch}, ExpiresAt: expires},
		}),
		settings("P05_PRINT_STAFF_GRANTS", map[string]service.Principal{
			fingerprint(clients[1]): {Role: "printer_staff", Tenant: order.Tenant, Subject: requester, ExpiresAt: expires},
		}),
		settings("P05_COMMERCE_GRANTS", commerceGrants),
		"P05_PRESENTATION_KEY_FILE=" + write("synthetic-presentation", []byte(strings.Repeat("31", 32)), 0o600),
		settings("P05_FINANCE_GRANTS", map[string]service.Principal{
			fingerprint(clients[1]): {Role: "finance", Tenant: order.Tenant, Subject: requester, ExpiresAt: expires},
			fingerprint(clients[2]): {Role: "finance", Tenant: order.Tenant, Subject: approver, ExpiresAt: expires},
			fingerprint(clients[3]): {Role: "finance", Tenant: order.Tenant, Subject: treasury, ExpiresAt: expires},
		}),
		settings("P05_COMMERCE_PROFILES", map[string]any{order.Branch: map[string]any{
			"chainId": 8283, "contract": target.Contract, "asset": target.Asset, "beneficiary": fixtureSigner(104).Address(),
			"signer": fixtureSigner(101).Address(), "merchantId": rawHex(order.Quote.Args.MerchantId[:]), "keyVersion": 1, "beneficiaryVersion": 1,
		}}),
	}
	for key, role := range map[string]string{
		"P05_COMMERCE_DATABASE_URL": "nu54_p05_commerce", "P05_SETTLEMENT_DATABASE_URL": "nu54_p05_settlement",
		"P05_FINANCE_DATABASE_URL": "nu54_p05_finance", "P05_RECEIVER_DATABASE_URL": "nu54_p05_receiver",
		"P05_PRINT_DATABASE_URL": "nu54_p05_print", "P05_PRINT_STAFF_DATABASE_URL": "nu54_p05_print_staff",
	} {
		_, enrolled := testLogin(t, db, dsn, role)
		config = append(config, key+"="+enrolled)
	}
	if subscriptionTest {
		_, enrolled := testLogin(t, db, dsn, "nu54_p05_subscription_worker")
		config = append(config, "P05_SUBSCRIPTION_WORKER_DATABASE_URL="+enrolled)
	}
	processRelayKey := [32]byte{31: 95}
	processRelayCipher, err := commerce.NewCipher(processRelayKey[:])
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string][32]byte{
		"P05_QUOTE_COMMITMENT_KEY_FILE": {31: 9}, "P05_QUOTE_STORAGE_KEY_FILE": order.CipherKey,
		"P05_SETTLEMENT_STORAGE_KEY_FILE": processRelayKey,
	} {
		config = append(config, name+"="+write(name, []byte(rawHex(key[:])), 0o600))
	}
	cast, err := exec.LookPath(os.Getenv("P05_SETTLEMENT_TEST_SIGNER"))
	if err != nil {
		t.Fatal(err)
	}
	for name, fixture := range map[string]int{"P05_QUOTE_SIGNER": 101, "P05_RELAY_SIGNER": 100, "P05_REFUND_SIGNER": 103} {
		// These are the pre-enrolled synthetic Anvil keys.
		script := fixtureSignerProgram(cast, fixture)
		sum := sha256.Sum256(script)
		config = append(config, name+"="+write(name, script, 0o700), name+"_SHA256="+rawHex(sum[:]))
	}
	device := []byte("#!/bin/sh\nexit 1\n")
	deviceHash := sha256.Sum256(device)
	config = append(config, "P05_PRINT_DEVICE="+write("device", device, 0o700), "P05_PRINT_DEVICE_SHA256="+rawHex(deviceHash[:]))
	dispatchKey, err := x509.MarshalPKCS8PrivateKey(clients[3].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	_, dispatchDSN := testLogin(t, db, dsn, "nu54_p05_dispatch")
	dispatchSettings := []string{"P05_DISPATCH_DATABASE_URL=" + dispatchDSN,
		"P05_DISPATCH_CA=" + write("dispatch-ca.pem", pemBlock("CERTIFICATE", serverTrust), 0o600),
		"P05_DISPATCH_CERT=" + write("dispatch-identity.pem", pemBlock("CERTIFICATE", clients[3].Certificate[0]), 0o600),
		"P05_DISPATCH_KEY=" + write("dispatch-key.pem", pemBlock("PRIVATE KEY", dispatchKey), 0o600),
	}
	var eventID string
	if err := db.QueryRow(ctx, "SELECT event_id::text FROM ledger.fulfillment_outbox WHERE sale_id=$1 AND kind='refunded'",
		order.Sale).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	fixtureExec(t, db, "UPDATE ledger.fulfillment_outbox SET lease_until=now()+interval '1 day'")
	enroll := func(addr string) []string {
		return append(dispatchSettings, settings("P05_FULFILLMENT_TARGETS", map[string]string{order.Branch: "https://" + addr}))
	}
	firstDispatch := enroll
	if os.Getenv("P05_PILOT_BULK") != "" {
		firstDispatch = nil // accumulate 100 confirmed paid rows before releasing the sender
	}
	addr, stop := start(config, firstDispatch)
	defer stop()
	url := "https://" + addr
	serverLeaf, err := x509.ParseCertificate(serverTrust)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(serverLeaf)
	certClient := func(certs ...tls.Certificate) *http.Client {
		return settlementFixtureTLSClient(t, roots, settlementProbeTimeout, certs...)
	}
	customerClient := func(cert tls.Certificate) *http.Client {
		return settlementFixtureTLSClient(t, roots, settlementCustomerTimeout, cert)
	}
	finance := certClient(clients[2])
	waitUntil(t, func() bool { return probe(finance, url) })
	if probe(certClient(), url) {
		t.Fatal("finance listener accepted a connection without a client identity")
	}
	payload, err := json.Marshal(map[string]string{"branchId": order.Branch, "businessDay": day, "asset": lower(target.Asset),
		"gross": pilotUnits(30), "refunds": pilotUnits(3), "evidenceRef": fixtureUUID(t, db)})
	if err != nil {
		t.Fatal(err)
	}
	for _, caller := range []struct {
		client *http.Client
		status int
	}{{certClient(clients[0]), 404}, {finance, 200}} {
		response, err := caller.client.Post(url+"/v2/finance/day-close", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != caller.status || (caller.status == 200 && !bytes.Contains(body, []byte(`"net":"`+pilotUnits(27)+`"`))) {
			t.Fatalf("unexpected finance reply: %d %s %v", response.StatusCode, body, err)
		}
	}

	var batch *pilotBatch
	var baselineDeadlocks int
	out := os.Getenv("P05_PILOT_OUTPUT")
	if out != "" && os.Getenv("NU54_NOTIFICATION_BROWSER") == "" {
		customer := customerClient(clients[0])
		var ids []string
		if len(independent) == 0 {
			ids = measurePilotOrders(t, ctx, db, customer, url, order, target, out)
		} else {
			for i := range independent {
				independent[i].client = certClient(independent[i].cert)
				independent[i].client.Timeout = 12 * time.Second
			}
			baselineDeadlocks = pilotDeadlocks(t, ctx, db)
			var timings []orderTiming
			ids, timings = independentReservations(t, ctx, db, url, order, independent, sameSKU)
			saveIndependentReservations(t, ctx, db, order, out, timings, baselineDeadlocks, sameSKU)
		}
		if os.Getenv("P05_PILOT_BULK") != "" {
			batch = preparePilotBatch(t, ctx, db, chain, token, deployed, customer, url, order, target, ids, out, independent)
		}
	}

	// Restart the actual command before releasing the pending notification.
	stop()
	// A hard stop leaves the pinned fixture signer for the test directory to collect.
	pinned, err := filepath.Glob(filepath.Join(tempDir, "nu54-pinned-program-*", "program"))
	if err != nil || len(pinned) == 0 {
		t.Fatalf("stopped opsd left no pinned fixture signer in its test directory: %v", err)
	}
	readyAt := time.Now()
	addr, stopAfterRecovery := start(config, enroll)
	defer stopAfterRecovery()
	waitUntil(t, func() bool { return probe(finance, "https://"+addr) })
	if batch != nil {
		batch.finish(t, ctx, db, chain, readyAt, out)
		if len(independent) > 0 {
			pilotPrivateReceipts(t, ctx, db, "https://"+addr, order.Branch, batch.ids, independent, baselineDeadlocks, out)
		}
	}
	// The production worker delivers to the production receiver over mTLS.
	eligible := time.Now()
	fixtureExec(t, db, "UPDATE ledger.fulfillment_outbox SET lease_until=NULL WHERE event_id=$1", eventID)
	var recorded bool
	waitUntil(t, func() bool {
		err := db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM ledger.consumer_receipt WHERE event_id=$1) AND "+
			"EXISTS(SELECT FROM ledger.fulfillment_inbox WHERE event_id=$1) AND NOT EXISTS(SELECT FROM ledger.print_attempt WHERE event_id=$1)",
			eventID).Scan(&recorded)
		return err == nil && recorded
	})
	if out := os.Getenv("P05_PILOT_OUTPUT"); out != "" {
		writePilotConvergence(t, out, eligible, time.Now(), eventID)
	}

	var acknowledgedFence int64
	if err := db.QueryRow(ctx, "SELECT fence FROM ledger.fulfillment_outbox WHERE event_id=$1 AND state='delivered'", eventID).
		Scan(&acknowledgedFence); err != nil {
		t.Fatal(err)
	}
	stopAfterRecovery()
	restartedAddr, stopResumed := start(config, enroll)
	defer stopResumed()
	waitUntil(t, func() bool { return probe(finance, "https://"+restartedAddr) })
	restartedAt := time.NewTimer(1100 * time.Millisecond)
	defer restartedAt.Stop()
	select {
	case <-restartedAt.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var actualFence int64
	if err := db.QueryRow(ctx, "SELECT fence FROM ledger.fulfillment_outbox WHERE event_id=$1 AND state='delivered'", eventID).
		Scan(&actualFence); err != nil || actualFence != acknowledgedFence {
		t.Fatalf("process restart attempted a second dispatch: %d vs %d, %v", actualFence, acknowledgedFence, err)
	}
	if kioskOutcome {
		t.Run("KioskMenuFinalOutcomes", func(t *testing.T) {
			if kioskOtherCustomer.actor != "" {
				kioskOtherCustomer.client = certClient(kioskOtherCustomer.cert)
			}
			testKioskMenuFinalOutcomes(t, ctx, db, "https://"+restartedAddr, order, nativePrincipal.Subject,
				nativeSubscription, certClient(clients[0]), certClient(clients[3]), kioskOtherCustomer, chain, token, deployed, proveUnpaid)
		})
		return
	}
	var nativeTravel *nativeSubscriptionLab
	if os.Getenv("NU54_TRAVEL_NATIVE_LAB") != "" {
		nativeTravel, nativeSubscription = nativeSubscription, nil
	}
	if t06 {
		if nativeT06 && (nativeTravel == nil || os.Getenv("NU54_TRAVEL_NATIVE_BOARD") != "1" || os.Getenv("NU54_TRAVEL_DISPLAY_PYTHON") == "") {
			t.Fatal("installed T06 requires the enrollment lab, C board and display verifier")
		}
		t.Run("TravelPickupOriginalPendingToReady", func(t *testing.T) {
			stopResumed()
			addr, stop := start(config, nil)
			defer stop()
			url := "https://" + addr
			waitUntil(t, func() bool { return probe(finance, url) })
			dispatchStore, err := Open(ctx, dispatchDSN, "nu54_p05_dispatch")
			if err != nil {
				t.Fatal(err)
			}
			defer dispatchStore.Close()
			evidence, err := OpenEvidence(ctx, evidenceDSN)
			if err != nil {
				t.Fatal(err)
			}
			defer evidence.Close()
			dispatcher := Dispatcher{Evidence: evidence, Store: dispatchStore, Client: certClient(clients[3]),
				Targets: map[string]string{order.Branch: url}}
			testTravelPickupOriginal(t, ctx, db, url, order, chain, token, deployed,
				[3]*http.Client{customerClient(clients[0]), customerClient(clients[1]), certClient(clients[4])}, dispatcher, nativeTravel)
		})
		var stopAfterT06 func()
		restartedAddr, stopAfterT06 = start(config, enroll)
		defer stopAfterT06()
		waitUntil(t, func() bool { return probe(finance, "https://"+restartedAddr) })
	}
	var inspectExperience func(*testing.T, string, string)
	if experienceScript != "" {
		console := os.Getenv("NU54_FINANCE_BROWSER_DIST")
		if !filepath.IsAbs(experienceScript) || !filepath.IsAbs(console) {
			t.Fatal("experience browser requires absolute script and console paths")
		}
		subscription := seedExperienceBrowser(t, db, order, approver)
		addr, stop := start(append(append([]string(nil), config...), "P05_CONSOLE_DIR="+console), func(addr string) []string {
			return []string{"P05_OPERATOR_ORIGIN=https://" + addr}
		})
		defer stop()
		url := "https://" + addr
		waitUntil(t, func() bool { return probe(finance, url) })
		key, err := x509.MarshalPKCS8PrivateKey(clients[2].PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		inspectExperience = func(t *testing.T, stage, booking string) {
			t.Helper()
			browser := exec.CommandContext(ctx, experienceScript, url, order.Branch, subscription,
				write("experience-operator.pem", pemBlock("CERTIFICATE", clients[2].Certificate[0]), 0o600),
				write("experience-operator-key.pem", pemBlock("PRIVATE KEY", key), 0o600),
				write("experience-terminal.pem", pemBlock("CERTIFICATE", clients[3].Certificate[0]), 0o600),
				write("experience-terminal-key.pem", pemBlock("PRIVATE KEY", dispatchKey), 0o600))
			browser.Env = append(os.Environ(), "NU54_EXPERIENCE_STAGE="+stage, "NU54_EXPERIENCE_BOOKING="+booking)
			result, err := browser.CombinedOutput()
			if err != nil {
				t.Fatalf("production experience browser: %v\n%s", err, result)
			}
			t.Logf("production experience browser: %s", result)
		}
	}
	if subscriptionTest {
		t.Run("TravelBookingPaymentsAndPickup", func(t *testing.T) {
			testTravelProcess(t, ctx, db, dsn, evidenceDSN, "https://"+restartedAddr, order,
				[2]*http.Client{customerClient(clients[0]), customerClient(clients[5])},
				[3]*http.Client{certClient(clients[1]), certClient(clients[2]), certClient(clients[3])}, chain, token, deployed, processRelayCipher, nativeTravel, inspectExperience, proveUnpaid)
		})
	}
	if subscriptionTest {
		t.Run("SubscriptionCycleLateFinality", func(t *testing.T) {
			testSubscriptionProcess(t, ctx, db, "https://"+restartedAddr, order,
				[2]*http.Client{customerClient(clients[0]), customerClient(clients[5])}, chain, token, deployed, nativeSubscription, proveUnpaid)
		})
	}
	if inspectExperience != nil {
		inspectExperience(t, "portfolio", "")
	}
	script := os.Getenv("NU54_FINANCE_BROWSER_SCRIPT")
	notification := os.Getenv("NU54_NOTIFICATION_BROWSER")
	if notification != "" {
		if script != "" {
			t.Fatal("select one finance browser fixture")
		}
		script = notification
		zone, err := time.LoadLocation("Asia/Seoul")
		if err != nil {
			t.Fatal(err)
		}
		branchDay, err := time.ParseInLocation("2006-01-02", day, zone)
		if err != nil {
			t.Fatal(err)
		}
		from, until := branchDay.Add(4*time.Hour), branchDay.AddDate(0, 0, 1).Add(4*time.Hour)
		forceTime(t, ctx, chain, until.Unix()+2)
		waitUntil(t, func() bool {
			var covered bool
			err := db.QueryRow(ctx, `SELECT base.block_time<=$2 AND final.block_time>=$3
				FROM chainview.generation g
				JOIN chainview.block base ON base.generation_id=g.id AND base.height=g.base_height AND base.canonical
				JOIN chainview.block final ON final.generation_id=g.id AND final.block_hash=g.finalized_hash
				WHERE g.stream_id=$1 AND g.id=(SELECT generation_id FROM chainview.read_target WHERE stream_id=$1)`, target.Stream, from, until).Scan(&covered)
			return err == nil && covered
		})
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'reconcile_retry',now()+interval '1 hour')",
			order.Branch, approver)
	}
	if script != "" {
		console := os.Getenv("NU54_FINANCE_BROWSER_DIST")
		if !filepath.IsAbs(script) || !filepath.IsAbs(console) || os.Getenv("P05_PILOT_HUMAN_UNITS") == "" {
			t.Fatal("finance browser proof requires absolute browser script and dist plus human test amounts")
		}
		payer := order.Order.Payer
		withdrawal, err := (LiveChain{chain}).RefundWithdrawal(ctx, target, payer, order.Quote.Args.PayerGeneration)
		if err != nil || withdrawal != payer {
			t.Fatalf("fixture's original withdrawal route did not match pinned finality: %v", err)
		}
		for _, capability := range []string{"report_read", "report_export", "reconcile_read"} {
			fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,$3,now()+interval '1 hour')",
				order.Branch, approver, capability)
		}
		webConfig := append(append([]string(nil), config...), "P05_CONSOLE_DIR="+console)
		var explorerFixture []string
		if notification != "" {
			if len(approved) != 1 {
				t.Fatal("notification browser requires the signed P07 deployment profile")
			}
			profile := approved[0]
			public := startFixturePublicExplorer(t, ctx, db, dsn)
			webConfig = append(webConfig, "P05_PUBLIC_EXPLORER_ORIGIN="+public,
				"P05_SETTLEMENT_DEPLOYMENT_PROFILES="+profile.Profiles,
				"P05_SETTLEMENT_DEPLOYMENT_SIGNATURES="+profile.Signatures,
				"P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY="+profile.Key)
			explorerFixture = notificationExplorerEnvironment(t, ctx, db, evidenceDSN, order.Branch, target.Stream, public)
		}
		webAddr, stopWeb := start(webConfig, func(addr string) []string {
			return []string{"P05_OPERATOR_ORIGIN=https://" + addr}
		})
		defer stopWeb()
		webURL := "https://" + webAddr
		waitUntil(t, func() bool { return probe(finance, webURL) })
		makerKey, err := x509.MarshalPKCS8PrivateKey(clients[1].PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		browserKey, err := x509.MarshalPKCS8PrivateKey(clients[2].PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		browser := exec.CommandContext(ctx, script, webURL, order.Branch, day, order.Sale,
			write("browser-finance.pem", pemBlock("CERTIFICATE", clients[2].Certificate[0]), 0o600),
			write("browser-finance-key.pem", pemBlock("PRIVATE KEY", browserKey), 0o600),
			write("browser-maker.pem", pemBlock("CERTIFICATE", clients[1].Certificate[0]), 0o600),
			write("browser-maker-key.pem", pemBlock("PRIVATE KEY", makerKey), 0o600),
			write("browser-treasury.pem", pemBlock("CERTIFICATE", clients[3].Certificate[0]), 0o600),
			write("browser-treasury-key.pem", pemBlock("PRIVATE KEY", dispatchKey), 0o600))
		browser.Env = append(os.Environ(), explorerFixture...)
		if os.Getenv("NU54_REFUND_RENTAL_TEST") != "" {
			verifyRentalRefundBrowser(t, ctx, db, order, requester, browser)
			return
		}

		type browserOutcome struct {
			report []byte
			err    error
		}
		finished := make(chan browserOutcome, 1)
		go func() { report, err := browser.CombinedOutput(); finished <- browserOutcome{report, err} }()
		timer := time.NewTicker(250 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case outcome := <-finished:
				if outcome.err != nil {
					t.Fatalf("production browser smoke: %v\n%s", outcome.err, outcome.report)
				}
				if os.Getenv("NU54_REFUND_LOW_TEST") != "" {
					var reservedWork int
					if err := db.QueryRow(ctx, `SELECT count(*) FROM ledger.refund r
                      JOIN ledger.settlement_work w ON w.refund_id=r.id
                      WHERE r.treasury_actor=$1`, treasury).Scan(&reservedWork); err != nil || reservedWork != 0 {
						t.Fatalf("underfunded treasury created a relay work item: %d %v", reservedWork, err)
					}
					t.Log("O06: real 0.50 TestUSDC observed for 1.10 request; independent treasury submit disabled; no worker item")
				}
				t.Logf("production browser finance: %s", outcome.report)
				stopWeb()
				return
			case <-timer.C:
				var broadcast bool
				if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT FROM ledger.refund r JOIN ledger.relay_attempt a ON a.work_id=r.id
					WHERE r.treasury_actor=$1 AND r.state='inflight' AND a.state IN ('broadcast','observed','unknown'))`, treasury).Scan(&broadcast); err != nil {
					t.Fatal(err)
				}
				if broadcast {
					mine(t, ctx, chain, 5)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}

}

func testFulfillmentIncidentRace(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn string, proof *EvidenceStore,
	target Target, first, second seededOrder, trust *x509.CertPool, cert tls.Certificate) {
	t.Helper()
	open := func(role string) *Store {
		t.Helper()
		_, restricted := testLogin(t, db, dsn, role)
		store, err := Open(ctx, restricted, role)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(store.Close)
		return store
	}
	receiver := open("nu54_p05_receiver")
	dispatchStore := open("nu54_p05_dispatch")
	var canSeeRaw bool
	if err := receiver.db.QueryRow(ctx, `SELECT has_table_privilege(current_user,'ledger.quote_work','SELECT')
		OR has_table_privilege(current_user,'ledger.consumer_receipt','SELECT')`).Scan(&canSeeRaw); err != nil || canSeeRaw {
		t.Fatalf("receiver privilege projection: %t %v", canSeeRaw, err)
	}
	mux := http.NewServeMux()
	Receiver{Store: receiver, Evidence: proof, Grants: map[string]ReceiverGrant{
		fingerprint(cert): {Branches: []string{first.Branch}, ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}}.Register(mux)
	type pause struct {
		started chan struct{}
		release chan struct{}
		after   bool
	}
	var stalled atomic.Pointer[pause]
	var posts atomic.Int64
	downstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
		p := stalled.Swap(nil)
		if p == nil {
			mux.ServeHTTP(w, r)
			return
		}
		if p.after {
			mux.ServeHTTP(w, r)
		}
		close(p.started)
		select {
		case <-p.release:
		case <-r.Context().Done():
			return
		}
		if !p.after {
			mux.ServeHTTP(w, r)
		}
	}))
	downstream.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	downstream.StartTLS()
	defer downstream.Close()
	transport := downstream.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
	defer transport.CloseIdleConnections()
	dispatch := Dispatcher{Evidence: proof, Store: dispatchStore, Client: &http.Client{Transport: transport, Timeout: 4 * time.Second},
		Targets: map[string]string{first.Branch: downstream.URL}}
	selectEvent := func(sale string) string {
		t.Helper()
		var id string
		if err := db.QueryRow(ctx, "SELECT event_id::text FROM ledger.fulfillment_outbox WHERE sale_id=$1 AND kind='paid'", sale).Scan(&id); err != nil {
			t.Fatal(err)
		}
		fixtureExec(t, db, "UPDATE ledger.fulfillment_outbox SET lease_until=now()+interval '1 day' WHERE event_id<>$1", id)
		fixtureExec(t, db, "UPDATE ledger.fulfillment_outbox SET lease_until=NULL WHERE event_id=$1", id)
		return id
	}
	sessionStep := func() error {
		ok, err := dispatch.Step(ctx)
		if !ok && err == nil {
			return errors.New("outbox had no eligible work")
		}
		return err
	}
	inboxes := func(id string, expectedConsumer, expectedSender int) {
		t.Helper()
		var c, s int
		if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.consumer_receipt WHERE event_id=$1),
		  (SELECT count(*) FROM ledger.fulfillment_inbox WHERE event_id=$1)`, id).Scan(&c, &s); err != nil ||
			c != expectedConsumer || s != expectedSender {
			t.Fatalf("durable inbox states got %d/%d want %d/%d: %v", c, s, expectedConsumer, expectedSender, err)
		}
	}
	record := func() string {
		t.Helper()
		incidentID := recordFixtureIncident(t, ctx, db, target.Stream)
		fast, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
		defer cancel()
		var independent string
		if err := db.QueryRow(fast, "SELECT id::text FROM ledger.branch WHERE id=$1", first.Branch).Scan(&independent); err != nil {
			t.Fatalf("independent branch command blocked: %v", err)
		}
		return incidentID
	}
	for _, scenario := range []struct {
		sale           string
		beforeIncident bool
	}{{first.Sale, false}, {second.Sale, true}} {
		id := selectEvent(scenario.sale)
		p := &pause{started: make(chan struct{}), release: make(chan struct{}), after: scenario.beforeIncident}
		var allowResponse sync.Once
		releaseHTTP := func() { allowResponse.Do(func() { close(p.release) }) }
		defer releaseHTTP()
		stalled.Store(p)
		done := make(chan error, 1)
		go func() { done <- sessionStep() }()
		select {
		case <-p.started:
		case <-time.After(3 * time.Second):
			releaseHTTP()
			t.Fatal("outbound request did not reach the receiver")
		}
		incident := record()
		releaseHTTP()
		if err := <-done; !errors.Is(err, ErrEvidenceHeld) {
			t.Fatalf("old acknowledgement completed after incident: %v", err)
		}
		consumer := 0
		if scenario.beforeIncident {
			consumer = 1
		}
		inboxes(id, consumer, 0)
		selectEvent(scenario.sale)
		before := posts.Load()
		if err := sessionStep(); !errors.Is(err, ErrEvidenceHeld) || posts.Load() != before {
			t.Fatalf("active incident allowed outbound dispatch: %v", err)
		}
		fixtureExec(t, db, "UPDATE chainview.incident SET resolved_at=clock_timestamp() WHERE id=$1", incident)
		selectEvent(scenario.sale)
		if err := sessionStep(); err != nil || posts.Load() != before {
			t.Fatalf("recovery did not first use exact GET: %v", err)
		}
		if !scenario.beforeIncident {
			selectEvent(scenario.sale)
			if err := sessionStep(); err != nil || posts.Load() != before+1 {
				t.Fatalf("authorized absence did not allow one fresh POST: %v", err)
			}
		}
		inboxes(id, 1, 1)
		if !scenario.beforeIncident {
			testPrintResolution(t, ctx, db, dsn, proof, first, trust, cert)
		}
	}
}

func recordFixtureIncident(t *testing.T, ctx context.Context, db *pgxpool.Pool, stream string) string {
	t.Helper()
	incidentID := fixtureUUID(t, db)
	fast, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	tx, err := db.Begin(fast)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(fast, "SELECT pg_advisory_xact_lock(hashtextextended($1::text,9))", stream); err != nil {
		t.Fatalf("incident blocked by an outbound call: %v", err)
	}
	if _, err := tx.Exec(fast, `INSERT INTO chainview.incident(id,generation_id,disputed_height,old_hash,new_hash,opened_at)
			SELECT $3,id,finalized_height,finalized_hash,$2,now() FROM chainview.generation WHERE stream_id=$1`, stream,
		(common.Hash{31: 3}).Hex(), incidentID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fast); err != nil {
		t.Fatal(err)
	}
	return incidentID
}

type recordingPrinter struct{ attempts []PrintFrame }

func (p *recordingPrinter) Print(_ context.Context, frame PrintFrame) (PrintAck, error) {
	p.attempts = append(p.attempts, frame)
	return PrintAck{}, errors.New("device may have printed but response was lost")
}

func testPrintResolution(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn string, proof *EvidenceStore,
	order seededOrder, trust *x509.CertPool, cert tls.Certificate) {
	t.Helper()
	open := func(role string) *Store {
		t.Helper()
		_, restricted := testLogin(t, db, dsn, role)
		store, err := Open(ctx, restricted, role)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(store.Close)
		return store
	}
	fixtureExec(t, db, `UPDATE ledger.print_attempt p SET next_check=now()+interval '1 day'
		WHERE p.state='pending' AND p.event_id NOT IN (SELECT event_id FROM ledger.fulfillment_outbox WHERE sale_id=$1)`, order.Sale)
	printer := &recordingPrinter{}
	worker := Printer{Store: open("nu54_p05_print"), Evidence: proof, Device: printer}
	done := func(expectAttempt bool) {
		t.Helper()
		did, err := worker.Step(ctx)
		if did != expectAttempt || (expectAttempt != (err != nil)) {
			t.Fatalf("print attempt processing: %t %v", did, err)
		}
	}
	done(true)
	done(false)
	if len(printer.attempts) != 1 {
		t.Fatal("unknown printer reply was resent")
	}
	initial := printer.attempts[0]
	employee := fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.print_staff_grant VALUES($1,$2,now()+interval '1 hour')", order.Branch, employee)
	mux := http.NewServeMux()
	PrintStaffHTTP{Store: open("nu54_p05_print_staff"), Evidence: proof, Credentials: map[string]service.Principal{
		fingerprint(cert): {Role: "printer_staff", Tenant: order.Tenant, Subject: employee, ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}}.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
	defer transport.CloseIdleConnections()
	employeeClient := &http.Client{Transport: transport, Timeout: 7 * time.Second}
	query := func(branch string, want int, contains string) {
		t.Helper()
		response, err := employeeClient.Get(server.URL + "/v2/fulfillment/print-attempts?branchId=" + branch + "&receiptRef=" + initial.ReceiptRef)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != want || !bytes.Contains(body, []byte(contains)) {
			t.Fatalf("print status projection: %d %s", response.StatusCode, body)
		}
	}
	decide := func(branch, attempt, key, action string, want int) string {
		t.Helper()
		input, _ := json.Marshal(map[string]string{"branchId": branch, "attemptId": attempt, "idempotencyKey": key, "action": action})
		response, err := employeeClient.Post(server.URL+"/v2/fulfillment/print-decisions", "application/json", bytes.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("print decision %s: %d %s", action, response.StatusCode, body)
		}
		if want != 200 {
			return ""
		}
		var result map[string]string
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		return result["attemptId"]
	}
	wrongBranch := fixtureUUID(t, db)
	queue := func(client *http.Client, branch, sale string, status int, expected ...string) {
		t.Helper()
		response, err := client.Get(server.URL + "/v2/fulfillment/print-attempts?branchId=" + branch + "&saleRef=" + sale)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != status {
			t.Fatalf("queue status: %d %v", response.StatusCode, err)
		}
		for _, item := range expected {
			if !bytes.Contains(body, []byte(item)) {
				t.Fatal("original sale and receipt were not paired")
			}
		}
	}
	queue(employeeClient, order.Branch, order.Sale, 200, order.Sale, initial.ReceiptRef)
	queue(employeeClient, wrongBranch, order.Sale, 404)
	query(wrongBranch, 404, "")
	query(order.Branch, 200, `"unknown"`)
	key := fixtureUUID(t, db)
	decide(wrongBranch, initial.AttemptID, key, "reprint", 404)
	copyID := decide(order.Branch, initial.AttemptID, key, "reprint", 200)
	if copyID == initial.AttemptID || copyID != decide(order.Branch, initial.AttemptID, key, "reprint", 200) {
		t.Fatal("staff replay duplicated the explicit reprint")
	}
	decide(order.Branch, initial.AttemptID, fixtureUUID(t, db), "reprint", 409)
	done(true)
	done(false)
	if len(printer.attempts) != 2 || printer.attempts[1].AttemptID != copyID ||
		printer.attempts[1].ReceiptRef != initial.ReceiptRef || printer.attempts[1].PickupNo != initial.PickupNo {
		t.Fatal("explicit reprint changed receipt or pickup identity")
	}
	decide(order.Branch, copyID, fixtureUUID(t, db), "confirm_printed", 200)
	query(order.Branch, 200, `"printed"`)
	done(false)
	extraKey := fixtureUUID(t, db)
	extra := decide(order.Branch, copyID, extraKey, "reprint", 200)
	if extra == copyID || decide(order.Branch, copyID, extraKey, "reprint", 200) != extra {
		t.Fatal("same explicit copy decision was not replayed")
	}
	decide(order.Branch, copyID, fixtureUUID(t, db), "reprint", 409)
	fixtureExec(t, db, "UPDATE ledger.print_staff_grant SET valid_until=clock_timestamp()-interval '1 second' WHERE actor_id=$1", employee)
	queue(employeeClient, order.Branch, order.Sale, 404)
	query(order.Branch, 404, "")
	decide(order.Branch, copyID, extraKey, "reprint", 404)
	done(true)
	done(false)
	if len(printer.attempts) != 3 || printer.attempts[2].ReceiptRef != initial.ReceiptRef {
		t.Fatal("original receipt was not retained after deliberate extra copy")
	}
	t.Log("K05: exact original receipt and sale, no automatic duplicates, scoped replay and expired staff denied")
	if lab := os.Getenv("NU54_PRINT_NATIVE_LAB"); lab != "" {
		verifyNativePrint(t, ctx, db, dsn, proof, order, initial, extra, lab, worker, printer)
	} else {
		verifyPrintGrantWait(t, ctx, db, employeeClient, server.URL, order.Branch, employee, initial.ReceiptRef, extra)
	}
}

func fixtureSignerProgram(executable string, fixture int) []byte {
	return []byte(fmt.Sprintf("#!/bin/sh\nexport GOMAXPROCS=1\nexec %q %064x\n", executable, fixture))
}

func pilotScale() int64 {
	if os.Getenv("P05_PILOT_HUMAN_UNITS") != "" {
		return 1_000_000
	}
	return 1
}

func pilotUnits(amount int64) string { return fmt.Sprint(amount * pilotScale()) }
