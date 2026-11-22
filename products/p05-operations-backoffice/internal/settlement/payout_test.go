package settlement

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func TestLoopbackPayoutApprovalAndFinalizedRotation(t *testing.T) {
	settlementEndpoint(t)
	endpoint := freshSettlementChain(t)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	db, dsn := scratchDB(t)
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	gasAccount := crypto.PubkeyToAddress(privateKey.PublicKey)
	target, deployment, token, contract, _ := deployFixtureWithOperator(t, ctx, client, db, privateKey, false)
	stopIndexer, _, _, proveAbsence := startFixtureIndexer(t, ctx, db, dsn, endpoint, os.Getenv("P05_SETTLEMENT_TEST_INDEXER"), target, deployment)
	defer stopIndexer()
	head, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	issuedAt := time.Unix(int64(head.Time), 0).UTC()
	old := seedPayerOrder(t, db, target, issuedAt, "4", 174, fixtureSigner(102), 1)
	expired := seedPayerIssue(t, db, seedPayerPreview(t, db, target, "4", fixtureSigner(102), old),
		issuedAt, "4", 175, 1, 2*time.Second)
	nextWallet := fixtureSigner(171)
	profile := commerce.Profile{
		Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
		Signer: fixtureSigner(101).Address(), MerchantID: old.Quote.Args.MerchantId,
		KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9},
	}
	digest := profile.Digest()
	fixtureExec(t, db, "INSERT INTO ledger.issuance_profile VALUES($1,1,$2,120,true)", old.Branch, digest[:])
	_, commerceDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	store, err := commerce.Open(ctx, commerceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, workerDSN := testLogin(t, db, dsn, "nu54_p05_payout_worker")
	workerStore, err := Open(ctx, workerDSN, "nu54_p05_payout_worker")
	if err != nil {
		t.Fatal(err)
	}
	defer workerStore.Close()
	worker := PayoutWorker{Store: workerStore, Client: client, Signer: receiptGasSigner{privateKey}, Governor: gasAccount,
		Profiles: map[string]commerce.Profile{old.Branch: profile}, MaxGasPrice: big.NewInt(100_000_000_000)}
	trust, certs, _ := mtlsClients(t, 5)
	identity := map[string]service.Principal{}
	for i, grants := range [][]string{{"payout_propose", "payout_approve"}, {"payout_approve"}, {"payout_approve"}, {"payout_notice"}, {}} {
		actor := fixtureUUID(t, db)
		fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES($1,$2)", "payout_fixture_"+actor, actor)
		for _, grant := range grants {
			fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,$3,now()+interval '2 days')", old.Branch, actor, grant)
		}
		identity[fingerprint(certs[i])] = service.Principal{Tenant: old.Tenant, Subject: actor, Role: "operator", ExpiresAt: time.Now().Add(5 * time.Minute).Unix()}
	}
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	origin := "https://" + server.Listener.Addr().String()
	if dir := os.Getenv("NU54_PAYOUT_BROWSER_DIST"); dir != "" {
		if !filepath.IsAbs(dir) {
			t.Fatal("payout browser build must be an absolute path")
		}
		mux.Handle("/ops/", http.StripPrefix("/ops/", http.FileServer(http.Dir(dir))))
	}
	commerce.HTTP{Store: store, Credentials: identity, OperatorOrigin: origin, Profiles: worker.Profiles}.Register(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	operators := make([]*http.Client, len(certs))
	for i, certificate := range certs {
		cfg := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		cfg.Certificates = []tls.Certificate{certificate}
		operators[i] = &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 5 * time.Second}
		defer operators[i].CloseIdleConnections()
	}
	call := func(i int, suffix string, body map[string]any, want int) map[string]any {
		t.Helper()
		method := "POST"
		if body == nil {
			method = "GET"
		}
		wire, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, method, server.URL+"/v2/operations/payout/"+old.Branch+suffix, bytes.NewReader(wire))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		response, err := operators[i].Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != want {
			t.Fatalf("payout %s: %d %s %v", suffix, response.StatusCode, raw, err)
		}
		var out map[string]any
		if want == 200 && json.Unmarshal(raw, &out) != nil {
			t.Fatal("unreadable payout JSON")
		}
		return out
	}
	key := fixtureUUID(t, db)
	browser := func(index int, phase string) {
		t.Helper()
		if os.Getenv("NU54_PAYOUT_REGISTRY_BROWSER") != "1" {
			return
		}
		if os.Getenv("NU54_PAYOUT_BROWSER_DIST") == "" {
			t.Fatal("registry browser requires the payout console build")
		}
		cert := certs[index]
		dir := t.TempDir()
		keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		certPath, keyPath := filepath.Join(dir, "operator.pem"), filepath.Join(dir, "operator-key.pem")
		for path, block := range map[string]*pem.Block{
			certPath: {Type: "CERTIFICATE", Bytes: cert.Certificate[0]},
			keyPath:  {Type: "PRIVATE KEY", Bytes: keyDER},
		} {
			if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.CommandContext(ctx, "node", "../../test/finance/payout-registry-browser.cjs", origin, old.Branch,
			certPath, keyPath, key, phase, common.Hash(old.Order.QuoteID).Hex(), common.Hash(expired.Order.QuoteID).Hex())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("desktop Chrome %s: %v\n%s", phase, err, out)
		} else {
			t.Logf("desktop Chrome %s: %s", phase, out)
		}
	}
	quoteState := func(view map[string]any, item seededOrder) map[string]any {
		t.Helper()
		quotes, _ := view["oldQuotes"].([]any)
		for _, candidate := range quotes {
			q, _ := candidate.(map[string]any)
			if q["saleId"] == item.Sale && q["quoteId"] == common.Hash(item.Order.QuoteID).Hex() {
				return q
			}
		}
		t.Fatalf("signed predecessor quote absent: %v", view)
		return nil
	}
	propose := map[string]any{"requestKey": key, "address": lower(nextWallet.Address()), "reason": "custody_rotation",
		"expiresAt": time.Now().Add(10 * time.Minute).Unix()}
	ref := map[string]any{"requestKey": key}
	call(4, "", nil, 404)
	if initial := call(0, "", nil, 200); initial["currentVersion"] != float64(1) ||
		initial["currentAddress"] != lower(profile.Beneficiary) || initial["requestKey"] != nil {
		t.Fatalf("missing enrolled registry predecessor: %v", initial)
	}
	browser(0, "start")
	challenge := call(0, "/challenge", propose, 200)["message"].(string)
	proof, err := nextWallet.SignDigest(ctx, common.BytesToHash(accounts.TextHash([]byte(challenge))))
	if err != nil {
		t.Fatal(err)
	}
	propose["proof"] = hex.EncodeToString(proof)
	call(0, "/propose", propose, 200)
	call(0, "/approve", ref, 404)
	call(3, "/acknowledge", ref, 409)
	if script := os.Getenv("NU54_PAYOUT_BROWSER_SCRIPT"); script != "" {
		if !filepath.IsAbs(script) || os.Getenv("NU54_PAYOUT_BROWSER_DIST") == "" {
			t.Fatal("payout browser requires the absolute script and build paths")
		}
		args := []string{server.URL, old.Branch}
		directory := t.TempDir()
		for i, cert := range certs[:2] {
			key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, []string{"maker", "checker"}[i])
			if err := os.WriteFile(path+".crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+".key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0o600); err != nil {
				t.Fatal(err)
			}
			args = append(args, path+".crt", path+".key")
		}
		if output, err := exec.CommandContext(ctx, "sh", append([]string{script}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("payout browser path: %v\n%s", err, output)
		}
		if browserState := call(1, "?requestKey="+key, nil, 200); browserState["approvals"] != float64(1) {
			t.Fatalf("browser did not record exactly one checker: %v", browserState)
		}
		t.Log("independent maker and checker browser profiles verified")
	} else {
		call(1, "/approve", ref, 200)
	}
	if retry := call(1, "/approve", ref, 200); retry["approvals"] != float64(1) {
		t.Fatal(retry)
	}
	if approvals := call(2, "/approve", ref, 200); approvals["approvals"] != float64(2) {
		t.Fatal(approvals)
	}
	scheduled := call(3, "/acknowledge", ref, 200)
	if scheduled["currentVersion"] != float64(1) || scheduled["proposedVersion"] != float64(2) ||
		scheduled["notifiedAt"] == nil || scheduled["effectiveAt"] == nil || scheduled["activatedAt"] != nil {
		t.Fatalf("incorrect scheduled registry view: %v", scheduled)
	}
	if scheduled["oldQuoteCount"] != float64(2) || quoteState(scheduled, old)["status"] != "pending" ||
		quoteState(scheduled, expired)["status"] != "pending" {
		t.Fatalf("scheduled address did not retain the signed original quotes: %v", scheduled)
	}
	browser(3, "scheduled")
	if moved, err := worker.Step(ctx); err != nil || moved {
		t.Fatalf("effective delay ignored: %v %v", moved, err)
	}
	// The expired quote is excluded by the issuer clock. It can still be
	// unexpired at the cutover block when the chain clock lags the issuer.
	waitUntil(t, func() bool { return time.Now().After(expired.Order.Expires) })
	// Advance only this synthetic approval record, preserving the production delay.
	fixtureExec(t, db, "UPDATE ledger.payout_change SET ready_at=now()-interval '1 second' WHERE branch_id=$1", old.Branch)
	if moved, err := worker.Step(ctx); err != nil || !moved {
		t.Fatalf("freeze and archive: %v %v", moved, err)
	}
	signed := call(3, "?requestKey="+key, nil, 200)
	if signed["phase"] != "signed" || signed["txHash"] == nil {
		t.Fatal(signed)
	}
	var enabled bool
	if err := db.QueryRow(ctx, "SELECT enabled FROM ledger.issuance_profile WHERE branch_id=$1", old.Branch).Scan(&enabled); err != nil || enabled {
		t.Fatalf("new previews not fenced: %v %v", enabled, err)
	}
	if err := client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", issuedAt.Unix()+1); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.Step(ctx); err != nil {
		t.Fatal(err)
	}
	mine(t, ctx, client, 5)
	// Fresh worker process state, same private SQL credential and transaction.
	recovered := worker
	if moved, err := recovered.Step(ctx); err != nil || !moved {
		t.Fatalf("finalized registry did not activate: %v %v", moved, err)
	}
	active := call(3, "?requestKey="+key, nil, 200)
	if active["phase"] != "active" || active["txHash"] != signed["txHash"] || active["currentVersion"] != float64(2) ||
		active["currentAddress"] != lower(nextWallet.Address()) || active["activatedAt"] == nil || active["effectiveBlock"] == nil {
		t.Fatal(active)
	}
	var nextAddress []byte
	var version uint64
	if err := workerStore.db.QueryRow(ctx, "SELECT address,version FROM ledger.active_payout($1)", old.Branch).Scan(&nextAddress, &version); err != nil ||
		version != 2 || common.BytesToAddress(nextAddress) != nextWallet.Address() {
		t.Fatalf("P05 payout unavailable: %d %v", version, err)
	}
	open, err := contract.Grandfathered(nil, profile.MerchantID, old.Quote.Digest)
	if err != nil || !open {
		t.Fatalf("previous quote excluded from the exact snapshot: %v %v", open, err)
	}
	if quote := quoteState(active, old); quote["admission"] != "grandfathered" || quote["status"] != "pending" {
		t.Fatalf("active registry lost the old address admission: %v", quote)
	}
	if quote := quoteState(active, expired); quote["admission"] != "cutover-rejected" || quote["status"] != "pending" {
		t.Fatalf("cutover rejected a historical quote without resolving its payment: %v", quote)
	}
	rotation, err := client.TransactionReceipt(ctx, common.HexToHash(active["txHash"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	expiredApproval, err := fixtureSigner(102).SignDigest(ctx, expired.Quote.ApprovalHash)
	if err != nil {
		t.Fatal(err)
	}
	codec, err := v2.Settlementv2MetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	calldata, err := codec.Pack("settle", expired.Quote.Args, expired.Quote.MerchantSig, expiredApproval)
	if err != nil {
		t.Fatal(err)
	}
	_, rejection := client.CallContract(ctx, ethereum.CallMsg{From: gasAccount, To: &target.Contract, Data: calldata}, rotation.BlockNumber)
	data, ok := ethclient.RevertErrorData(rejection)
	if !ok || !bytes.Equal(data, codec.Errors["InvalidMerchant"].ID.Bytes()[:4]) {
		t.Fatalf("excluded historical quote accepted or rejected for another reason at the cutover block: %x %v", data, rejection)
	}
	browser(3, "active")
	operator, err := bind.NewKeyedTransactorWithChainID(privateKey, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	operator.Context = ctx
	payer := fixtureSigner(102)
	approval, err := payer.SignDigest(ctx, old.Quote.ApprovalHash)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := contract.Settle(operator, old.Quote.Args, old.Quote.MerchantSig, approval)
	if err != nil {
		latest, _ := client.HeaderByNumber(ctx, nil)
		t.Fatalf("settle old at %d with bounds %d–%d: %v", latest.Time, old.Quote.Args.IssuedAt, old.Quote.Args.ExpiresAt, err)
	}
	receipt, err := bind.WaitMined(ctx, client, transaction)
	if err != nil || receipt.Status != 1 {
		t.Fatalf("old exact quote did not settle: %v", err)
	}
	previousBalance, err := token.BalanceOf(nil, profile.Beneficiary)
	if err != nil || previousBalance.String() != "4" {
		t.Fatalf("old quote beneficiary changed: %v %v", previousBalance, err)
	}
	head, err = client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	newOrder := old.Order
	newOrder.QuoteID[31]++
	newOrder.OrderID[31]++
	newOrder.Nonce[31]++
	newOrder.Issued = time.Unix(int64(head.Time), 0).UTC()
	newOrder.Expires = newOrder.Issued.Add(time.Minute)
	newProfile := profile
	newProfile.BeneficiaryVersion = version
	newProfile.Beneficiary = common.BytesToAddress(nextAddress)
	newWire, newDigest, err := commerce.SignQuote(ctx, newOrder, newProfile, fixtureSigner(101), newOrder.Issued)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := unpackQuote(newWire, newDigest[:], profile.Signer.Bytes(), target)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = payer.SignDigest(ctx, issued.ApprovalHash)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := contract.Settle(operator, issued.Args, issued.MerchantSig, approval)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err = bind.WaitMined(ctx, client, tx)
	if err != nil || receipt.Status != 1 {
		t.Fatalf("new quote did not settle: %v", err)
	}
	newBalance, err := token.BalanceOf(nil, nextWallet.Address())
	if err != nil || newBalance.String() != "4" {
		t.Fatalf("new payout received %v %v", newBalance, err)
	}

	oldHash := transaction.Hash().Hex()
	head, err = client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The quick headless path may mine all blocks in the same second. Advance
	// only after both exact deposits so P07 has a genuinely post-expiry anchor.
	forceTime(t, ctx, client, max(int64(head.Time)+1, expired.Order.Expires.Unix()+1))
	waitUntil(t, func() bool {
		var seen bool
		err := db.QueryRow(ctx, `SELECT EXISTS(
			SELECT FROM chainview.read_target t JOIN chainview.generation g ON g.id=t.generation_id
			JOIN chainview.block anchor ON anchor.generation_id=g.id AND anchor.block_hash=g.finalized_hash
			WHERE t.stream_id=$1 AND anchor.canonical AND anchor.block_time >= $2
				AND chainview.complete_prefix(g.id,anchor.height)
		) AND EXISTS (SELECT FROM chainview.finalized_receipt WHERE tx_hash=$3 AND kind='paid')`,
			target.Stream, expired.Order.Expires, oldHash).Scan(&seen)
		return err == nil && seen
	})
	if quoteState(call(3, "?requestKey="+key, nil, 200), old)["status"] != "pending" {
		t.Fatal("a chain-only result appeared as a booked payment")
	}
	waitUntil(t, func() bool { return time.Now().After(expired.Order.Expires) })
	proveAbsence(t, expired)
	proofLogin, proofDSN := testLogin(t, db, dsn, "nu54_p07_reconcile")
	merchant := common.Hash(profile.MerchantID).Hex()
	fixtureExec(t, db, "INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '1 hour')", proofLogin, target.Stream, merchant)
	proofStore, err := OpenEvidence(ctx, proofDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer proofStore.Close()
	_, ledgerDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	ledger, err := Open(ctx, ledgerDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	for _, item := range []seededOrder{old, expired} {
		if committed, err := proofStore.reconciliation(ctx, ledger, work{sale: item.Sale}, target, item.Quote); err != nil || !committed {
			t.Fatalf("P07 disposition did not commit: %v %v", committed, err)
		}
	}
	processed := call(3, "?requestKey="+key, nil, 200)
	paid, unpaid := quoteState(processed, old), quoteState(processed, expired)
	if paid["status"] != "paid" || paid["transaction"] != oldHash || unpaid["status"] != "expired" || unpaid["expiryEvidence"] == nil {
		t.Fatalf("wrong P07-backed original quote dispositions: %v %v", paid, unpaid)
	}
	browser(3, "outcomes")

	// The predecessor profile and effective timestamp were not retained by
	// older workers. Their finalized block and exact quote archive still work.
	fixtureExec(t, db, `UPDATE ledger.payout_change SET predecessor_profile_version=NULL,activated_at=NULL
		WHERE branch_id=$1 AND request_key=$2`, old.Branch, key)
	legacy := call(3, "?requestKey="+key, nil, 200)
	if legacy["currentVersion"] != float64(2) || legacy["effectiveBlock"] != active["effectiveBlock"] ||
		legacy["quoteTracking"] != "rotation-archive" || legacy["activatedAt"] != nil ||
		legacy["oldQuoteCount"] != float64(1) || quoteState(legacy, old)["transaction"] != oldHash {
		t.Fatalf("older registered rotation did not preserve available evidence: %v", legacy)
	}
	t.Logf("original quote paid %s, exact expired evidence %s, rotation %s block %s",
		oldHash, unpaid["expiryEvidence"], active["txHash"], active["effectiveBlock"])
}
