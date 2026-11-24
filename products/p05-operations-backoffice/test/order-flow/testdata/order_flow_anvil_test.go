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
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// deploySponsor puts a GasSponsor before the escrow and the token, for the store's gas account.
func deploySponsor(t *testing.T, ctx context.Context, client *ethclient.Client, escrow escrowDeployment) common.Address {
	t.Helper()
	address, tx, err := deployArtifact(t, escrow.operator, client, "GasSponsor", escrow.operator.From)
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
	ready := escrow.target
	ready.Sponsor = address
	if SponsorReady(ctx, client, ready, escrow.gas) {
		t.Fatal("a sponsor that knows no relay is ready")
	}
	mined(sponsor.SetRelay(escrow.operator, escrow.gas, true))
	lists, err := sponsoredSelectors()
	if err != nil {
		t.Fatal(err)
	}
	mined(sponsor.SetActions(escrow.operator, escrow.target.Contract, lists.escrow, signedAction))
	if SponsorReady(ctx, client, ready, escrow.gas) {
		t.Fatal("a sponsor without the token is ready")
	}
	mined(sponsor.SetActions(escrow.operator, escrow.target.Asset, lists.token, signedAction))
	if !SponsorReady(ctx, client, ready, escrow.gas) || SponsorReady(ctx, client, ready, escrow.operator.From) {
		t.Fatal("sponsor readiness")
	}
	return address
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
	escrow := deployEscrow(t, ctx, client, db)
	target := escrow.target
	target.Sponsor = deploySponsor(t, ctx, client, escrow)
	target.SendsPerDay = 5

	// Identities. The customer starts without a board account: it enrolls its own.
	const customer, finance1, kitchen, dispatcher, terminal, finance2 = 0, 1, 3, 2, 7, 8
	tenant, branch := fixtureUUID(t, db), fixtureUUID(t, db)
	actors := make([]string, 10)
	for i := range actors {
		actors[i] = fixtureUUID(t, db)
	}
	fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	fixtureExec(t, db, "INSERT INTO ledger.operator_branch_label VALUES($1,'Order flow cafe')", branch)
	fixtureExec(t, db, "INSERT INTO ledger.command_rule VALUES($1,'escrow','escrow','escrow',interval '15 minutes')", branch)
	fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'customer',NULL,now()+interval '1 hour')", branch, actors[customer])
	fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'terminal','menu',now()+interval '1 hour')", branch, actors[terminal])
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'pickup_manage',now()+interval '1 hour')", branch, actors[kitchen])
	for actor, capabilities := range map[int][]string{
		finance1: {"report_read", "escrow_claim", "escrow_dispute_request", "escrow_dispute_approve"},
		finance2: {"escrow_dispute_request", "escrow_dispute_approve"}} {
		for _, capability := range capabilities {
			fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,$3,now()+interval '1 hour')", branch, actors[actor], capability)
		}
	}
	fixtureExec(t, db, "INSERT INTO ledger.menu_release VALUES($1,1,now()-interval '1 minute',now()+$2*interval '1 second')", branch, max(15*time.Minute, life).Seconds())
	fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,'escrow-cafe','plain','Escrow cafe',$2,$3::text::numeric)", branch, lower(target.Asset), escrowAmount)
	fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,'escrow-cafe',60)", branch)
	profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
		Signer: fixtureSigner(101).Address(), MerchantID: [32]byte{31: 50}, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}, Escrow: true}
	digest := profile.Digest()
	fixtureExec(t, db, "INSERT INTO ledger.issuance_profile VALUES($1,1,$2,120,true)", branch, digest[:])

	// The faucet has test tokens of its own: the store's, and one the board cannot send yet.
	faucetKey := fixtureSigner(160)
	if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", faucetKey.Address(), "0x56BC75E2D63100000"); err != nil {
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
	mined(escrow.token.Mint(escrow.operator, faucetKey.Address(), big.NewInt(1_000_000_000)))
	won, tx, err := deployArtifact(t, escrow.operator, client, "TestToken3009", escrow.operator.From, "NU54 Test KRW", "tKRWC", uint8(18))
	mined(tx, err)
	symbol, err := escrow.token.Symbol(&bind.CallOpts{Context: ctx})
	if err != nil {
		t.Fatal(err)
	}
	faucet := &Faucet{Account: faucetKey.Address(), Signer: faucetKey, Funds: FaucetFunds(client, faucetKey.Address(), big.NewInt(100_000_000_000)),
		Tokens: map[string][]FaucetToken{branch: {
			{Address: target.Asset, Symbol: symbol, Decimals: 6, Amount: "5000000", PerDay: 1, DailyTotal: "50000000"},
			{Address: won, Symbol: "tKRWC", Decimals: 18, Amount: "1000000000000000000000", PerDay: 1, DailyTotal: "10000000000000000000000"}}}}
	if err := CheckFaucet(*faucet, map[string]Target{branch: target}, escrow.gas); err != nil {
		t.Fatal(err)
	}

	// The services, as cmd/opsd composes them.
	qCipher, _ := commerce.NewCipher(bytes.Repeat([]byte{0x35}, 32))
	relayCipher, _ := commerce.NewCipher(bytes.Repeat([]byte{0x5f}, 32))
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
	_, httpDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	commerceStore, err := commerce.Open(ctx, httpDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(commerceStore.Close)
	_, proof := startEscrowChainview(t, ctx, db, dsn, endpoint, os.Getenv("P05_SETTLEMENT_TEST_INDEXER"), client, target, escrow.deployed)
	trust, certs, _ := mtlsClients(t, 10)
	expires := time.Now().Add(50 * time.Minute).Unix()
	credentials := map[string]service.Principal{}
	for index, role := range map[int]string{customer: "customer", terminal: "terminal", kitchen: "operator"} {
		credentials[fingerprint(certs[index])] = service.Principal{Role: role, Tenant: tenant, Subject: actors[index], ExpiresAt: expires}
	}
	financeCredentials := map[string]service.Principal{}
	for _, index := range []int{finance1, finance2} {
		financeCredentials[fingerprint(certs[index])] = service.Principal{Role: "finance", Tenant: tenant, Subject: actors[index], ExpiresAt: expires}
	}
	sponsors := func(_ context.Context, named string, p commerce.Profile, payer common.Address, now time.Time) (commerce.GasSponsorship, bool) {
		until := now.Add(time.Minute)
		return commerce.GasSponsorship{Account: escrow.gas, Grant: [32]byte{31: 91}, ExpiresAt: until.Unix(),
			CurrentPublication: func(at time.Time) bool { return at.Before(until) }}, named == branch && p.Contract == target.Contract && payer != escrow.gas
	}
	targets := map[string]Target{branch: target}
	merchantIDs := map[string]common.Hash{branch: profile.MerchantID}
	mux := http.NewServeMux()
	commerce.HTTP{Store: commerceStore, GasSponsors: sponsors, Credentials: credentials, Signer: fixtureSigner(101), Cipher: qCipher,
		Payment: Forwarder{Store: open("nu54_p05_commerce"), QuoteCipher: qCipher, RelayCipher: relayCipher, Targets: targets,
			GasSponsors: sponsors, MerchantIDs: merchantIDs, Faucet: faucet, GasAccount: escrow.gas},
		Profiles: map[string]commerce.Profile{branch: profile}, PresentationKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, 32))}.Register(mux)
	(FinanceHTTP{Store: open("nu54_p05_finance"), Funds: LiveChain{client}, Targets: targets, Merchants: map[string]string{branch: common.Hash{31: 50}.Hex()},
		Evidence: proof, CloseStore: open("nu54_p05_settlement"), QuoteCipher: qCipher, OperatorOrigin: "https://ops.test",
		Resolver: fixtureSigner(106).Address(), Credentials: financeCredentials}).Register(mux)
	mtls := func(handler http.Handler) *httptest.Server {
		server := httptest.NewUnstartedServer(handler)
		server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
		server.StartTLS()
		t.Cleanup(server.Close)
		return server
	}
	server := mtls(mux)
	receiverMux := http.NewServeMux()
	Receiver{Store: open("nu54_p05_receiver"), Evidence: proof, Grants: map[string]ReceiverGrant{
		fingerprint(certs[dispatcher]): {Branches: []string{branch}, ExpiresAt: expires}}}.Register(receiverMux)
	receiver := mtls(receiverMux)
	transport := receiver.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{certs[dispatcher]}
	t.Cleanup(transport.CloseIdleConnections)
	dispatch := Dispatcher{Evidence: proof, Store: open("nu54_p05_dispatch"), Client: &http.Client{Transport: transport, Timeout: 4 * time.Second},
		Targets: map[string]string{branch: receiver.URL}}
	if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", fixtureSigner(106).Address(), "0x56BC75E2D63100000"); err != nil {
		t.Fatal(err)
	}
	worker := &Worker{Store: open("nu54_p05_settlement"), Evidence: proof, Chain: LiveChain{client}, Targets: targets, QuoteCipher: qCipher,
		RelayCipher: relayCipher, GasAccount: escrow.gas, GasSigner: escrow.gasKey, MaxGasPrice: big.NewInt(100_000_000_000), Now: time.Now,
		MerchantSigner: fixtureSigner(101), MerchantSigners: map[string]common.Address{branch: profile.Signer},
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
	identity := func(index int, subject string) map[string]string {
		der, err := x509.MarshalPKCS8PrivateKey(certs[index].PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		name := hex.EncodeToString([]byte{byte(index)})
		return map[string]string{"subject": subject, "cert": write(name+".crt", encode("CERTIFICATE", certs[index].Certificate[0])),
			"key": write(name+".key", encode("PRIVATE KEY", der))}
	}
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	home := escrow.operator // the buyer's ordinary wallet: it holds the minted token and its own gas
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
			switch in.Op {
			case "deposit": // mint to the ordinary wallet, then an ordinary transfer to the board account
				amount, ok := new(big.Int).SetString(in.Amount, 10)
				if !ok {
					return nil, errors.New("amount")
				}
				chain.Lock()
				defer chain.Unlock()
				minted, err := escrow.token.Mint(home, home.From, amount)
				if err != nil {
					return nil, err
				}
				moved, err := escrow.token.Transfer(home, address, amount)
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
				units, err := escrow.token.BalanceOf(&bind.CallOpts{Context: ctx}, address)
				if err != nil {
					return nil, err
				}
				return map[string]string{"amount": units.String()}, nil
			case "held":
				units, err := escrow.contract.TotalHeld(&bind.CallOpts{Context: ctx})
				if err != nil {
					return nil, err
				}
				return map[string]string{"amount": units.String()}, nil
			case "locks": // OrderLocked events of one order, straight from the chain
				iterator, err := escrow.contract.FilterOrderLocked(&bind.FilterOpts{Context: ctx}, nil, [][32]byte{common.HexToHash(in.OrderKey)}, nil)
				if err != nil {
					return nil, err
				}
				defer iterator.Close()
				count := 0
				for iterator.Next() {
					count++
				}
				stored, err := escrow.contract.Orders(&bind.CallOpts{Context: ctx}, profile.MerchantID, common.HexToHash(in.OrderKey))
				if err != nil {
					return nil, err
				}
				return map[string]any{"count": count, "state": stored.State, "payer": lower(stored.Payer), "confirmer": lower(stored.Confirmer)}, nil
			case "submissions":
				var rows int
				err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.settlement_submission").Scan(&rows)
				return map[string]int{"count": rows}, err
			case "quote-lifetime":
				_, err := db.Exec(ctx, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=$2 WHERE branch_id=$1", branch, in.Seconds)
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
	fixture, _ := json.Marshal(map[string]any{
		"endpoint": server.URL, "ca": write("ca.crt", encode("CERTIFICATE", server.Certificate().Raw)), "expiresAt": expires,
		"tenant": tenant, "branch": branch, "rpc": endpoint, "control": control.URL, "secret": rawHex(secret), "home": lower(home.From),
		"amount": escrowAmount, "directory": dir,
		"payment": map[string]any{"chain": 8283, "contract": lower(target.Contract), "asset": lower(target.Asset), "beneficiary": lower(profile.Beneficiary),
			"merchantId": rawHex(profile.MerchantID[:]), "signer": lower(profile.Signer), "keyVersion": 1, "beneficiaryVersion": 1, "maxBaseUnits": "30000000",
			"abiSha256":       strings.TrimPrefix(target.ABIHash(), "0x"),
			"rpc":             "https://rpc.test", // the phone's stand-in answers for it
			"presentationKey": rawHex(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, 32)).Public().(ed25519.PublicKey))},
		"customer": identity(customer, actors[customer]), "terminal": identity(terminal, actors[terminal]), "kitchen": identity(kitchen, actors[kitchen]),
		"finance1": identity(finance1, actors[finance1]), "finance2": identity(finance2, actors[finance2]),
	})
	config := os.Getenv("NU54_ORDER_FLOW_JEST")
	if config == "" || os.Getenv("NU54_ORDER_FLOW_BOARD") == "" {
		t.Fatal("run scripts/test_order_flow.py")
	}
	child := exec.CommandContext(ctx, "node", "../../scripts/test-jest.cjs", "--config", config, "--runInBand", "--forceExit", "--ci", "--watchman=false")
	child.Dir = "../../../p02-user-app"
	child.Env = append(os.Environ(), "NU54_ORDER_FLOW_FIXTURE="+write("fixture.json", fixture))
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	child.Cancel = func() error { return child.Process.Signal(syscall.SIGTERM) }
	child.WaitDelay = 10 * time.Second
	if err := child.Run(); err != nil {
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
	// Every payment went through the sponsor, sent by the store's gas account.
	locked, err := escrow.contract.FilterOrderLocked(&bind.FilterOpts{Context: ctx}, nil, nil, nil)
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
		if err != nil || sender != escrow.gas || tx.To() == nil || *tx.To() != target.Sponsor {
			t.Fatalf("payment %s was not sponsored: %v %v", tx.Hash(), sender, tx.To())
		}
		payments++
	}
	if payments == 0 && os.Getenv("NU54_ORDER_FLOW_ONLY") == "" && os.Getenv("NU54_ORDER_FLOW_MINUTES") == "" {
		t.Fatal("no payment was sponsored")
	}
	t.Logf("%d sponsored payments", payments)
}
