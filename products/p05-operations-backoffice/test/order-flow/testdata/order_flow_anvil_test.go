// The server side of scripts/test_order_flow.py. The runner compiles this file into the settlement
// package with `go test -overlay`, so it uses the same fixtures as order_escrow_anvil_test.go.
package settlement

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/gassponsor"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

// shop is one store of the run: a token, an order contract, a branch and the people of both sides.
type shop struct {
	name, branch string
	escrow       escrowDeployment
	target       Target
	profile      commerce.Profile
	actors       []string
	first        int // its first client certificate
	board, only  string
	holders      sync.Map // accounts that may hold its token, besides the known ones
}

// The positions of a shop's identities among its certificates. The customer starts without a board account: it enrolls its own.
const customer, finance1, dispatcher, kitchen, terminal, finance2 = 0, 1, 2, 3, 7, 8
const perShop = 10

// namedTokenABI is namedTokenAbiSha256 of packages/protocol/ts/src/commerce-payment.ts.
const namedTokenABI = "6627cf1b4142565d31cb1fc786a43c3aca301a107eb19a97eb3093e5a93bcc92"

// wonToken is what opsd reads from the settings of a won branch.
var wonToken = commerce.Token{Name: "NU54 Test KRW", Symbol: "tKRW", Label: "tKRW", Decimals: 2}

// coinToken is the same for a branch that takes StableNet's own coin, which its board counts in hundredths of a won.
var coinToken = commerce.Token{Name: "WKRC", Symbol: "WKRC", Label: "KRWC", Decimals: 18, BoardDecimals: 2}

// units is an amount of the shop's token from what its board counts.
func (s *shop) units(board int64) *big.Int {
	token := s.target.Token.OrDollar()
	unit := big.NewInt(1)
	if token.BoardDecimals != 0 {
		unit.Exp(big.NewInt(10), big.NewInt(int64(token.Decimals-token.BoardDecimals)), nil)
	}
	return unit.Mul(unit, big.NewInt(board))
}

// shopsOf names the stores of a run. "all" adds one more whose settings do not match its chain.
func shopsOf(t *testing.T, ctx context.Context, client *ethclient.Client, db *pgxpool.Pool) []*shop {
	t.Helper()
	usdBoard, wonBoard, coinBoard := os.Getenv("NU54_ORDER_FLOW_BOARD"), os.Getenv("NU54_ORDER_FLOW_BOARD_WON"), os.Getenv("NU54_ORDER_FLOW_BOARD_KRWC")
	open := func(name, artifact, board string, token commerce.Token) *shop {
		escrow := deployEscrowFor(t, ctx, client, db, artifact)
		escrow.target.Token, escrow.target.SendsPerDay = token, 5
		return &shop{name: name, escrow: escrow, target: escrow.target, board: board, only: os.Getenv("NU54_ORDER_FLOW_ONLY")}
	}
	var shops []*shop
	switch os.Getenv("NU54_ORDER_FLOW_TOKEN") {
	case "", "usd":
		shops = []*shop{open("usd", "TestUSDC3009", usdBoard, commerce.Token{})}
	case "won":
		shops = []*shop{open("won", "TestKRW3009", wonBoard, wonToken)}
	case "krwc":
		shops = []*shop{open("krwc", "NativeCoinStandIn", coinBoard, coinToken)}
	case "all":
		refused := open("refused", "TestUSDC3009", usdBoard, wonToken)
		refused.only = "a branch refused at the start"
		shops = []*shop{open("usd", "TestUSDC3009", usdBoard, commerce.Token{}), open("won", "TestKRW3009", wonBoard, wonToken), open("krwc", "NativeCoinStandIn", coinBoard, coinToken), refused}
	default:
		t.Fatal("NU54_ORDER_FLOW_TOKEN")
	}
	for n, s := range shops {
		s.branch, s.first = fixtureUUID(t, db), n*perShop
	}
	return shops
}

// seed writes what an operator enrolled before the store opens.
func (s *shop) seed(t *testing.T, db *pgxpool.Pool, tenant string, life time.Duration) {
	t.Helper()
	s.actors = make([]string, perShop)
	for i := range s.actors {
		s.actors[i] = fixtureUUID(t, db)
	}
	fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", s.branch, tenant)
	fixtureExec(t, db, "INSERT INTO ledger.operator_branch_label VALUES($1,'Order flow cafe')", s.branch)
	fixtureExec(t, db, "INSERT INTO ledger.command_rule VALUES($1,'escrow','escrow','escrow',interval '15 minutes')", s.branch)
	fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'customer',NULL,now()+interval '1 hour')", s.branch, s.actors[customer])
	fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'terminal','menu',now()+interval '1 hour')", s.branch, s.actors[terminal])
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'pickup_manage',now()+interval '1 hour')", s.branch, s.actors[kitchen])
	if os.Getenv("NU54_ORDER_FLOW_OPS_WEB") != "" {
		// The kitchen's certificate also edits the menu on the operations page of a local run.
		fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES($2,$1) ON CONFLICT DO NOTHING", s.actors[kitchen], "cafe_"+s.actors[kitchen])
		fixtureExec(t, db, "INSERT INTO ledger.staff_grant VALUES($1,$2,'operator',now()-interval '1 minute',null)", s.branch, s.actors[kitchen])
	}
	for actor, capabilities := range map[int][]string{
		finance1: {"report_read", "escrow_claim", "escrow_dispute_request", "escrow_dispute_approve"},
		finance2: {"escrow_dispute_request", "escrow_dispute_approve"}} {
		for _, capability := range capabilities {
			fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,$3,now()+interval '1 hour')", s.branch, s.actors[actor], capability)
		}
	}
	fixtureExec(t, db, "INSERT INTO ledger.menu_release VALUES($1,1,now()-interval '1 minute',now()+$2*interval '1 second')", s.branch, max(15*time.Minute, life).Seconds())
	fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,'escrow-cafe','plain','Escrow cafe',$2,$3::text::numeric)", s.branch, lower(s.target.Asset), s.units(8_000_000).String())
	fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,'escrow-cafe',60)", s.branch)
	// A product with two groups of choices, as the operations page publishes it: a row for every combination.
	fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,'option-latte',60)", s.branch)
	var breakdown []map[string]any
	for _, temp := range [][2]string{{"hot", "뜨겁게"}, {"ice", "차갑게"}} {
		for _, size := range [][3]any{{"s", "보통", int64(0)}, {"l", "크게", int64(1_000_000)}} {
			option := temp[0] + "-" + size[0].(string)
			fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,'option-latte',$2,'Option latte',$3,$4::text::numeric)",
				s.branch, option, lower(s.target.Asset), s.units(8_000_000+size[2].(int64)).String())
			breakdown = append(breakdown, map[string]any{"sku": "option-latte", "option": option, "required": true, "metadata": map[string]any{
				"labels": map[string]string{"ko": "옵션 라떼"}, "optionLabels": map[string]string{"ko": temp[1] + " · " + size[1].(string)},
				"optionGroupLabels": map[string]string{"ko": "온도 · 크기"}}})
		}
	}
	fixtureExec(t, db, "INSERT INTO ledger.catalog_pricing VALUES($1,1,NULL,$2)", s.branch, breakdown)
	fixtureExec(t, db, `INSERT INTO ledger.menu_sku_options VALUES($1,1,'option-latte','{"label":"Option latte"}',
		'[{"key":"temp","labels":{"ko":"온도"},"values":[{"key":"hot","labels":{"ko":"뜨겁게"}},{"key":"ice","labels":{"ko":"차갑게"}}]},
		  {"key":"size","labels":{"ko":"크기"},"values":[{"key":"s","labels":{"ko":"보통"}},{"key":"l","labels":{"ko":"크게"}}]}]')`, s.branch)
	s.profile = commerce.Profile{Chain: 8283, Contract: s.target.Contract, Asset: s.target.Asset, Beneficiary: fixtureSigner(104).Address(),
		Signer: fixtureSigner(101).Address(), MerchantID: [32]byte{31: 50}, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}, Escrow: true, Token: s.target.Token}
	digest := s.profile.Digest()
	fixtureExec(t, db, "INSERT INTO ledger.issuance_profile VALUES($1,1,$2,120,true)", s.branch, digest[:])
}

// deploySponsor puts one GasSponsor before the escrows and the tokens, for the stores' gas account.
func deploySponsor(t *testing.T, ctx context.Context, client *ethclient.Client, shops []*shop) common.Address {
	t.Helper()
	owner, gas := shops[0].escrow.operator, shops[0].escrow.gas
	address, tx, err := deployArtifact(t, owner, client, "GasSponsor", owner.From)
	mined := func(tx *types.Transaction, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if receipt, err := bind.WaitMined(ctx, client, tx); err != nil || receipt.Status != 1 {
			t.Fatalf("sponsor fixture reverted: %v", err)
		}
	}
	mined(tx, err)
	sponsor, err := gassponsor.NewGassponsor(address, client)
	if err != nil {
		t.Fatal(err)
	}
	lists, err := sponsoredSelectors()
	if err != nil {
		t.Fatal(err)
	}
	for n, s := range shops {
		s.target.Sponsor = address
		if n == 0 {
			if SponsorReady(ctx, client, s.target, gas) {
				t.Fatal("a sponsor that knows no relay is ready")
			}
			mined(sponsor.SetRelay(owner, gas, true))
		}
		mined(sponsor.SetActions(owner, s.target.Contract, lists.escrow, signedAction))
		if SponsorReady(ctx, client, s.target, gas) {
			t.Fatal("a sponsor without the token is ready")
		}
		mined(sponsor.SetActions(owner, s.target.Asset, lists.token, signedAction))
		if !SponsorReady(ctx, client, s.target, gas) || SponsorReady(ctx, client, s.target, owner.From) {
			t.Fatal("sponsor readiness")
		}
	}
	return gas
}

// startFlowIndexer runs one indexer process for every store and lets one login read their receipts.
func startFlowIndexer(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn, endpoint string, shops []*shop) *EvidenceStore {
	t.Helper()
	if len(shops) == 1 {
		client, err := ethclient.DialContext(ctx, endpoint)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.Close)
		_, proof := startEscrowChainview(t, ctx, db, dsn, endpoint, os.Getenv("P05_SETTLEMENT_TEST_INDEXER"), client, shops[0].target, shops[0].escrow.deployed)
		return proof
	}
	_, ingest := testLogin(t, db, dsn, "nu54_p07_ingest")
	reader, evidence := testLogin(t, db, dsn, "nu54_p07_reconcile")
	profiles, generations := []deployment.Profile{}, []string{}
	for _, s := range shops {
		generation := fixtureUUID(t, db)
		generations = append(generations, generation)
		profiles = append(profiles, deployment.Profile{StreamID: s.target.Stream, GenerationID: generation, Version: s.target.profileVersion(), ChainID: 8283,
			Contract: s.target.Contract, Asset: s.target.Asset, ABIHash: s.target.ABIHash(), Manifest: s.target.Manifest, CodeHash: s.target.CodeHash,
			StartHash: s.escrow.deployed.BlockHash, StartHeight: s.escrow.deployed.BlockNumber.Int64()})
	}
	raw, _ := json.Marshal(profiles)
	file := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(filepath.Join(t.TempDir(), "p07.log"))
	if err != nil {
		t.Fatal(err)
	}
	process := exec.CommandContext(ctx, os.Getenv("P05_SETTLEMENT_TEST_INDEXER"), "chainview")
	process.Env = append(os.Environ(), "P07_CHAINVIEW_DATABASE_URL="+ingest, "P07_CHAINVIEW_PROFILES="+file, "P07_RPC="+endpoint, "P07_POLL=50ms")
	process.Stderr = output
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = process.Process.Kill()
		_ = process.Wait()
		_ = output.Close()
		if t.Failed() {
			if text, err := os.ReadFile(output.Name()); err == nil {
				t.Logf("P07 indexer: %s", text)
			}
		}
	})
	for n, s := range shops {
		waitUntil(t, func() bool {
			var height *int64
			_ = db.QueryRow(ctx, "SELECT finalized_height FROM chainview.generation WHERE id=$1", generations[n]).Scan(&height)
			return height != nil
		})
		var switched string
		if err := db.QueryRow(ctx, `INSERT INTO chainview.switch_request(id,stream_id,target_id,expected_revision,operator_ref,compare_ref,requested_at)
			VALUES(gen_random_uuid(),$1,$2,0,gen_random_uuid(),chainview.record_comparison($1,$2,0),now()) RETURNING id::text`,
			s.target.Stream, generations[n]).Scan(&switched); err != nil {
			t.Fatal(err)
		}
		fixtureExec(t, db, "SELECT chainview.apply_switch($1)", switched)
		fixtureExec(t, db, `INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '1 hour')`, reader, s.target.Stream, common.Hash{31: 50}.Hex())
	}
	proof, err := OpenEvidence(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(proof.Close)
	return proof
}

// balances checks one store after its run: what was locked is what was paid out, given back or is still held,
// and every token that exists is on a known account.
func (s *shop) balances(t *testing.T, ctx context.Context, client *ethclient.Client, faucet common.Address, others []*shop) {
	t.Helper()
	opts, all := &bind.CallOpts{Context: ctx}, &bind.FilterOpts{Context: ctx}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	balance := func(holder common.Address) *big.Int {
		t.Helper()
		units, err := s.escrow.token.BalanceOf(opts, holder)
		must(err)
		return units
	}
	locked, paid, back := new(big.Int), new(big.Int), new(big.Int)
	locks, err := s.escrow.contract.FilterOrderLocked(all, nil, nil, nil)
	must(err)
	orders := 0
	for ; locks.Next(); orders++ {
		locked.Add(locked, locks.Event.Amount)
	}
	releases, err := s.escrow.contract.FilterOrderReleased(all, nil, nil)
	must(err)
	for releases.Next() {
		paid.Add(paid, releases.Event.Amount)
	}
	cancels, err := s.escrow.contract.FilterOrderCancelled(all, nil, nil)
	must(err)
	for cancels.Next() {
		back.Add(back, cancels.Event.Amount)
	}
	must(errors.Join(locks.Error(), releases.Error(), cancels.Error()))
	held, err := s.escrow.contract.TotalHeld(opts)
	must(err)
	sum := new(big.Int).Add(new(big.Int).Add(paid, back), held)
	if locked.Cmp(sum) != 0 || balance(s.target.Contract).Cmp(held) != 0 || balance(s.profile.Beneficiary).Cmp(paid) != 0 {
		t.Fatalf("%s: locked %s, paid %s, given back %s, held %s, contract %s, store %s", s.name, locked, paid, back, held, balance(s.target.Contract), balance(s.profile.Beneficiary))
	}
	holders := map[common.Address]bool{s.escrow.operator.From: true, fixtureSigner(102).Address(): true, faucet: true, s.profile.Beneficiary: true, s.target.Contract: true}
	s.holders.Range(func(key, _ any) bool { holders[key.(common.Address)] = true; return true })
	total := new(big.Int)
	for holder := range holders {
		total.Add(total, balance(holder))
	}
	supply, err := s.escrow.token.TotalSupply(opts)
	must(err)
	if total.Cmp(supply) != 0 {
		t.Fatalf("%s: known accounts hold %s of %s", s.name, total, supply)
	}
	for _, other := range others {
		if other != s && balance(other.target.Contract).Sign() != 0 {
			t.Fatalf("%s: its token is held by the contract of %s", s.name, other.name)
		}
	}
	t.Logf("%s: %d orders, locked %s = paid %s + given back %s + held %s; supply %s", s.name, orders, locked, paid, back, held, supply)
	if len(others) > 1 && strings.Contains(s.only, "return, blocked") && (orders < 4 || paid.Sign() == 0 || back.Sign() == 0 || held.Sign() != 0) && !t.Failed() {
		t.Fatalf("%s: the run settled too little", s.name)
	}
}

func TestOrderFlow(t *testing.T) {
	settlementEndpoint(t)
	endpoint := settlementChainSince(t, 65*time.Second)
	db, dsn := scratchDB(t)
	life := 8 * time.Minute
	if minutes, err := strconv.Atoi(os.Getenv("NU54_ORDER_FLOW_MINUTES")); err == nil { // scripts/simulate_user_app.py
		life = time.Duration(minutes) * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), life)
	defer cancel()
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	shops := shopsOf(t, ctx, client, db)
	gas := deploySponsor(t, ctx, client, shops)
	gasKey := shops[0].escrow.gasKey

	tenant := fixtureUUID(t, db)
	for _, s := range shops {
		s.seed(t, db, tenant, life)
	}
	// What opsd finds at its start: a branch whose token is not on chain is switched off, and only that one.
	targets, active, reviews := map[string]Target{}, map[string]Target{}, map[string]DeploymentReview{}
	profiles, merchantIDs, signers, merchants := map[string]commerce.Profile{}, map[string]common.Hash{}, map[string]common.Address{}, map[string]string{}
	var open []*shop
	for _, s := range shops {
		targets[s.branch], profiles[s.branch] = s.target, s.profile
		review := VerifyOperatorDeployment(ctx, client, s.target, gas, nil, nil, nil, false)
		reviews[s.branch] = review
		if (review.Status == "invalid") != (s.name == "refused") {
			t.Fatalf("%s: review %s", s.name, review.Status)
		}
		if review.Status == "invalid" {
			continue
		}
		open = append(open, s)
		active[s.branch], merchantIDs[s.branch] = s.target, s.profile.MerchantID
		signers[s.branch], merchants[s.branch] = s.profile.Signer, common.Hash(s.profile.MerchantID).Hex()
	}

	// The faucet has test tokens of its own: each store's, and one the board cannot send yet.
	faucetKey := fixtureSigner(160)
	// Ten million: of the chain's own coin it gives what it pays gas with.
	if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", faucetKey.Address(), "0x84595161401484A000000"); err != nil {
		t.Fatal(err)
	}
	mined := func(tx *types.Transaction, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if receipt, err := bind.WaitMined(ctx, client, tx); err != nil || receipt.Status != 1 {
			t.Fatalf("faucet fixture reverted: %v", err)
		}
	}
	unsendable, tx, err := deployArtifact(t, shops[0].escrow.operator, client, "TestToken3009", shops[0].escrow.operator.From, "NU54 Test KRW", "tKRWC", uint8(18))
	mined(tx, err)
	faucet := &Faucet{Account: faucetKey.Address(), Signer: faucetKey, Funds: FaucetFunds(client, faucetKey.Address(), big.NewInt(100_000_000_000)),
		Tokens: map[string][]FaucetToken{}}
	for _, s := range open {
		mined(s.escrow.token.Mint(s.escrow.operator, faucetKey.Address(), s.units(1_000_000_000)))
		symbol, err := s.escrow.token.Symbol(&bind.CallOpts{Context: ctx})
		if err != nil {
			t.Fatal(err)
		}
		faucet.Tokens[s.branch] = []FaucetToken{
			{Address: s.target.Asset, Symbol: symbol, Decimals: s.target.Token.OrDollar().Decimals, Amount: s.units(5_000_000).String(), PerDay: 1, DailyTotal: s.units(50_000_000).String()},
			{Address: unsendable, Symbol: "tKRWC", Decimals: 18, Amount: "1000000000000000000000", PerDay: 1, DailyTotal: "10000000000000000000000"}}
	}
	if err := CheckFaucet(*faucet, active, gas); err != nil {
		t.Fatal(err)
	}
	if err := CheckFaucetTokens(ctx, client, *faucet); err != nil {
		t.Fatal(err)
	}

	// The services, as cmd/opsd composes them.
	qCipher, _ := commerce.NewCipher(bytes.Repeat([]byte{0x35}, 32))
	relayCipher, _ := commerce.NewCipher(bytes.Repeat([]byte{0x5f}, 32))
	login := func(role string) *Store {
		t.Helper()
		_, roleDSN := testLogin(t, db, dsn, role)
		store, err := Open(ctx, roleDSN, role)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(store.Close)
		return store
	}
	_, httpDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	commerceStore, err := commerce.Open(ctx, httpDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(commerceStore.Close)
	proof := startFlowIndexer(t, ctx, db, dsn, endpoint, open)
	trust, certs, _ := mtlsClients(t, perShop*len(shops))
	expires := time.Now().Add(50 * time.Minute).Unix()
	credentials := map[string]service.Principal{}
	financeCredentials, receivers, endpoints := map[string]service.Principal{}, map[string]ReceiverGrant{}, map[string]string{}
	var branches []string
	for _, s := range shops {
		for index, role := range map[int]string{customer: "customer", terminal: "terminal", kitchen: "operator"} {
			credentials[fingerprint(certs[s.first+index])] = service.Principal{Role: role, Tenant: tenant, Subject: s.actors[index], ExpiresAt: expires}
		}
		for _, index := range []int{finance1, finance2} {
			financeCredentials[fingerprint(certs[s.first+index])] = service.Principal{Role: "finance", Tenant: tenant, Subject: s.actors[index], ExpiresAt: expires}
		}
	}
	for _, s := range open {
		branches = append(branches, s.branch)
	}
	receivers[fingerprint(certs[dispatcher])] = ReceiverGrant{Branches: branches, ExpiresAt: expires}
	sponsors := func(_ context.Context, named string, p commerce.Profile, payer common.Address, now time.Time) (commerce.GasSponsorship, bool) {
		until := now.Add(time.Minute)
		target, known := active[named]
		return commerce.GasSponsorship{Account: gas, Grant: [32]byte{31: 91}, ExpiresAt: until.Unix(),
			CurrentPublication: func(at time.Time) bool { return at.Before(until) }}, known && p.Contract == target.Contract && payer != gas
	}
	mux := http.NewServeMux()
	commerceHTTP := commerce.HTTP{Store: commerceStore, GasSponsors: sponsors, Credentials: credentials, Signer: fixtureSigner(101), Cipher: qCipher,
		Payment: Forwarder{Store: login("nu54_p05_commerce"), QuoteCipher: qCipher, RelayCipher: relayCipher, Targets: active,
			GasSponsors: sponsors, MerchantIDs: merchantIDs, Faucet: faucet, GasAccount: gas,
			RentalEscrow: RentalEscrow{Client: client, Targets: active, Reviews: reviews}},
		Profiles: profiles, PresentationKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, 32))}
	(FinanceHTTP{Store: login("nu54_p05_finance"), Funds: LiveChain{client}, Targets: targets, Merchants: merchants, Deployments: reviews, GasAccount: gas,
		Evidence: proof, CloseStore: login("nu54_p05_settlement"), QuoteCipher: qCipher, OperatorOrigin: "https://ops.test",
		Resolver: fixtureSigner(106).Address(), Credentials: financeCredentials}).Register(mux)
	mtls := func(handler http.Handler) *httptest.Server {
		server := httptest.NewUnstartedServer(handler)
		server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
		server.StartTLS()
		t.Cleanup(server.Close)
		return server
	}
	server := mtls(mux)
	// A local run serves the operations page from the same origin, which the menu requests must come from.
	if web := os.Getenv("NU54_ORDER_FLOW_OPS_WEB"); web != "" {
		commerceHTTP.OperatorOrigin = server.URL
		mux.Handle("/ops/", http.StripPrefix("/ops/", http.FileServer(http.Dir(web))))
	}
	commerceHTTP.Register(mux)
	receiverMux := http.NewServeMux()
	Receiver{Store: login("nu54_p05_receiver"), Evidence: proof, Grants: receivers}.Register(receiverMux)
	receiver := mtls(receiverMux)
	transport := receiver.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{certs[dispatcher]}
	t.Cleanup(transport.CloseIdleConnections)
	dispatch := Dispatcher{Evidence: proof, Store: login("nu54_p05_dispatch"), Client: &http.Client{Transport: transport, Timeout: 4 * time.Second},
		Targets: endpoints}
	for _, branch := range branches {
		endpoints[branch] = receiver.URL
	}
	if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", fixtureSigner(106).Address(), "0x56BC75E2D63100000"); err != nil {
		t.Fatal(err)
	}
	ledger := login("nu54_p05_settlement")
	worker := &Worker{Store: ledger, Evidence: proof, Chain: LiveChain{client}, Targets: active, QuoteCipher: qCipher,
		RelayCipher: relayCipher, GasAccount: gas, GasSigner: gasKey, MaxGasPrice: big.NewInt(100_000_000_000), Now: time.Now,
		MerchantSigner: fixtureSigner(101), MerchantSigners: signers,
		Resolver: fixtureSigner(106).Address(), ResolverSigner: fixtureSigner(106), MerchantIDs: merchantIDs, Faucet: faucet}

	// Background work: opsd's loops, a block every second, and the retry delays cut short.
	var background sync.WaitGroup
	stop, halt := context.WithCancel(ctx)
	stopped := func() { halt(); background.Wait() }
	defer stopped()
	every := func(period time.Duration, step func()) {
		background.Add(1)
		go func() {
			defer background.Done()
			ticker := time.NewTicker(period)
			defer ticker.Stop()
			for {
				select {
				case <-stop.Done():
					return
				case <-ticker.C:
					step()
				}
			}
		}()
	}
	var noted sync.Map
	tick := func(name string, step func(context.Context) (bool, error)) func() {
		return func() {
			for range 8 {
				call, end := context.WithTimeout(stop, 7*time.Second)
				moved, err := step(call)
				end()
				if err != nil && stop.Err() == nil {
					if _, seen := noted.LoadOrStore(name+err.Error(), true); !seen {
						t.Logf("%s stays pending: %v", name, err)
					}
				}
				if !moved {
					return
				}
			}
		}
	}
	if err := client.Client().CallContext(ctx, nil, "evm_increaseTime", 65); err != nil {
		t.Fatal(err)
	}
	var chain sync.Mutex
	block := func() {
		chain.Lock()
		defer chain.Unlock()
		_ = client.Client().CallContext(stop, nil, "anvil_mine", 1)
	}
	every(time.Second, block)
	every(300*time.Millisecond, func() {
		_, _ = db.Exec(stop, "UPDATE ledger.settlement_work SET retry_at=clock_timestamp() WHERE NOT finished AND retry_at>clock_timestamp()+interval '600 milliseconds'")
	})
	every(300*time.Millisecond, tick("worker", worker.Step))
	every(300*time.Millisecond, tick("dispatch", dispatch.Step))
	every(time.Second, tick("expiry", commerceStore.ReclaimExpired))

	// The phones, the kiosk and the test itself run in a child process.
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	encode := func(kind string, der []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}) }
	identity := func(s *shop, position int) map[string]string {
		index, subject := s.first+position, s.actors[position]
		der, err := x509.MarshalPKCS8PrivateKey(certs[index].PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		name := hex.EncodeToString([]byte{byte(index)})
		return map[string]string{"subject": subject, "cert": write(name+".crt", encode("CERTIFICATE", certs[index].Certificate[0])),
			"key": write(name+".key", encode("PRIVATE KEY", der))}
	}
	config := os.Getenv("NU54_ORDER_FLOW_JEST")
	if config == "" || os.Getenv("NU54_ORDER_FLOW_BOARD") == "" {
		t.Fatal("run scripts/test_order_flow.py")
	}
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	run := func(s *shop) error {
		home := s.escrow.operator // the buyer's ordinary wallet: it holds the minted token and its own gas
		control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Op, Address, OrderKey, Amount string
				Seconds                       int64
			}
			if r.Header.Get("X-Order-Flow") != rawHex(secret) || json.NewDecoder(r.Body).Decode(&in) != nil {
				http.Error(w, "denied", http.StatusForbidden)
				return
			}
			out, err := func() (any, error) {
				address := common.HexToAddress(in.Address)
				if in.Address != "" {
					s.holders.Store(address, true)
				}
				switch in.Op {
				case "deposit": // mint to the ordinary wallet, then an ordinary transfer to the board account
					amount, ok := new(big.Int).SetString(in.Amount, 10)
					if !ok {
						return nil, errors.New("amount")
					}
					chain.Lock()
					defer chain.Unlock()
					minted, err := s.escrow.token.Mint(home, home.From, amount)
					if err != nil {
						return nil, err
					}
					moved, err := s.escrow.token.Transfer(home, address, amount)
					if err != nil {
						return nil, err
					}
					for _, tx := range []*types.Transaction{minted, moved} {
						if receipt, err := bind.WaitMined(ctx, client, tx); err != nil || receipt.Status != 1 {
							return nil, errors.New("deposit reverted")
						}
					}
					return map[string]string{"hash": moved.Hash().Hex()}, nil
				case "balance":
					units, err := s.escrow.token.BalanceOf(&bind.CallOpts{Context: ctx}, address)
					if err != nil {
						return nil, err
					}
					return map[string]string{"amount": units.String()}, nil
				case "held":
					units, err := s.escrow.contract.TotalHeld(&bind.CallOpts{Context: ctx})
					if err != nil {
						return nil, err
					}
					return map[string]string{"amount": units.String()}, nil
				case "locks": // OrderLocked events of one order, straight from the chain
					iterator, err := s.escrow.contract.FilterOrderLocked(&bind.FilterOpts{Context: ctx}, nil, [][32]byte{common.HexToHash(in.OrderKey)}, nil)
					if err != nil {
						return nil, err
					}
					defer iterator.Close()
					count := 0
					for iterator.Next() {
						count++
					}
					stored, err := s.escrow.contract.Orders(&bind.CallOpts{Context: ctx}, s.profile.MerchantID, common.HexToHash(in.OrderKey))
					if err != nil {
						return nil, err
					}
					return map[string]any{"count": count, "state": stored.State, "payer": lower(stored.Payer), "confirmer": lower(stored.Confirmer)}, nil
				case "submissions":
					var rows int
					err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.settlement_submission WHERE stream_id=$1", s.target.Stream).Scan(&rows)
					return map[string]int{"count": rows}, err
				case "take-back": // what a start with other settings for this branch would say
					other, dollar := s.target, s.target
					other.Contract[0] ^= 1
					dollar.Token = commerce.Token{}
					said := ""
					if err := ledger.CheckOpenContracts(ctx, s.branch, other, qCipher); err != nil {
						said = err.Error()
					}
					return map[string]string{"anotherContract": said,
						"withoutToken": VerifyOperatorDeployment(ctx, client, dollar, gas, nil, nil, nil, false).Status}, nil
				case "quote-lifetime":
					_, err := db.Exec(ctx, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=$2 WHERE branch_id=$1", s.branch, in.Seconds)
					return map[string]bool{"ok": true}, err
				case "warp": // only the chain clock moves
					chain.Lock()
					defer chain.Unlock()
					if err := client.Client().CallContext(ctx, nil, "evm_increaseTime", in.Seconds); err != nil {
						return nil, err
					}
					err := client.Client().CallContext(ctx, nil, "anvil_mine", 5)
					return map[string]bool{"ok": err == nil}, err
				}
				return nil, errors.New("unknown operation")
			}()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(out)
		}))
		t.Cleanup(control.Close)
		payment := map[string]any{"chain": 8283, "contract": lower(s.target.Contract), "asset": lower(s.target.Asset), "beneficiary": lower(s.profile.Beneficiary),
			"merchantId": rawHex(s.profile.MerchantID[:]), "signer": lower(s.profile.Signer), "keyVersion": 1, "beneficiaryVersion": 1, "maxBaseUnits": s.units(30_000_000).String(),
			"abiSha256":       strings.TrimPrefix(s.target.ABIHash(), "0x"),
			"rpc":             "https://rpc.test", // the phone's stand-in answers for it
			"presentationKey": rawHex(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, 32)).Public().(ed25519.PublicKey))}
		if s.target.Token != (commerce.Token{}) { // the mark that an app which knows only dollars refuses
			payment["token"], payment["abiSha256"] = s.target.Token, namedTokenABI
		}
		fixture, _ := json.Marshal(map[string]any{
			"endpoint": server.URL, "ca": write("ca.crt", encode("CERTIFICATE", server.Certificate().Raw)), "expiresAt": expires,
			"shop": s.name, "shops": len(shops), "tenant": tenant, "branch": s.branch, "rpc": endpoint, "control": control.URL, "secret": rawHex(secret), "home": lower(home.From),
			"amount": s.units(8_000_000).String(), "unit": s.units(1).String(), "directory": dir,
			"payment":  payment,
			"customer": identity(s, customer), "terminal": identity(s, terminal), "kitchen": identity(s, kitchen),
			"finance1": identity(s, finance1), "finance2": identity(s, finance2),
		})
		child := exec.CommandContext(ctx, "node", "../../scripts/test-jest.cjs", "--config", config, "--runInBand", "--forceExit", "--ci", "--watchman=false")
		child.Dir = "../../../p02-user-app"
		child.Env = append(os.Environ(), "NU54_ORDER_FLOW_FIXTURE="+write(s.name+".json", fixture), "NU54_ORDER_FLOW_BOARD="+s.board, "NU54_ORDER_FLOW_ONLY="+s.only)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		child.Cancel = func() error { return child.Process.Signal(syscall.SIGTERM) }
		child.WaitDelay = 10 * time.Second
		return child.Run()
	}
	var runs sync.WaitGroup
	results := make([]error, len(shops))
	for n, s := range shops {
		runs.Add(1)
		go func() { defer runs.Done(); results[n] = run(s) }()
	}
	runs.Wait()
	if err := errors.Join(results...); err != nil {
		t.Errorf("order flows: %v", err)
		rows, err := db.Query(ctx, `SELECT coalesce(a.kind,'payment')||' '||coalesce(a.state,'')||' relay='||coalesce(r.state,'-')||' finished='||w.finished
			FROM ledger.settlement_work w LEFT JOIN ledger.escrow_action a ON a.id=w.action_id LEFT JOIN ledger.relay_attempt r ON r.work_id=w.id ORDER BY w.retry_at`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var line string
				_ = rows.Scan(&line)
				t.Log("queue:", line)
			}
		}
	}
	stopped()
	// Every payment went through the sponsor, sent by the stores' gas account.
	for _, s := range open {
		locked, err := s.escrow.contract.FilterOrderLocked(&bind.FilterOpts{Context: ctx}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer locked.Close()
		payments := 0
		for locked.Next() {
			tx, _, err := client.TransactionByHash(ctx, locked.Event.Raw.TxHash)
			if err != nil {
				t.Fatal(err)
			}
			sender, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8283)), tx)
			if err != nil || sender != gas || tx.To() == nil || *tx.To() != s.target.Sponsor {
				t.Fatalf("payment %s was not sponsored: %v %v", tx.Hash(), sender, tx.To())
			}
			payments++
		}
		if payments == 0 && os.Getenv("NU54_ORDER_FLOW_ONLY") == "" && os.Getenv("NU54_ORDER_FLOW_MINUTES") == "" {
			t.Fatal("no payment was sponsored")
		}
		t.Logf("%s: %d sponsored payments", s.name, payments)
		s.balances(t, ctx, client, faucet.Account, shops)
	}
}
