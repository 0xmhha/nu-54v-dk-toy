package settlement

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	escrowabi "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc3009"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

const escrowAmount = "8000000"

// escrowChainview drives the real indexer.
type escrowChainview struct {
	sync  func(common.Hash)
	prove func(*testing.T, seededOrder)
}

func startEscrowChainview(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn, endpoint, binary string, client *ethclient.Client,
	target Target, deployed *types.Receipt) (escrowChainview, *EvidenceStore) {
	t.Helper()
	merchant := common.Hash{31: 50}.Hex()
	var installed bool
	if err := db.QueryRow(ctx, "SELECT to_regprocedure('chainview.escrow_transaction(uuid,text,text)') IS NOT NULL").Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Skip("requires the P07 order escrow migrations")
	}
	var view escrowChainview
	_, _, _, prove := startFixtureIndexer(t, ctx, db, dsn, endpoint, binary, target, deployed)
	view.prove = func(t *testing.T, order seededOrder) {
		t.Helper()
		waitUntil(t, func() bool { // the proof needs an indexed final block after the expiry
			var covered bool
			_ = db.QueryRow(ctx, `SELECT EXISTS (SELECT FROM chainview.generation g JOIN chainview.block b
				ON b.generation_id=g.id AND b.height=g.finalized_height AND b.canonical
				WHERE g.stream_id=$1 AND b.block_time>=$2)`, target.Stream, order.Order.Expires).Scan(&covered)
			return covered
		})
		prove(t, order)
	}
	view.sync = func(hash common.Hash) {
		receipt, err := client.TransactionReceipt(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		mine(t, ctx, client, 5)
		waitUntil(t, func() bool {
			var height *int64
			_ = db.QueryRow(ctx, "SELECT finalized_height FROM chainview.generation WHERE stream_id=$1", target.Stream).Scan(&height)
			return height != nil && *height >= receipt.BlockNumber.Int64()
		})
	}
	name, evidenceDSN := testLogin(t, db, dsn, "nu54_p07_reconcile")
	fixtureExec(t, db, `INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '1 hour')`, name, target.Stream, merchant)
	proof, err := OpenEvidence(ctx, evidenceDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(proof.Close)
	return view, proof
}

type escrowDeployment struct {
	target   Target
	deployed *types.Receipt
	operator *bind.TransactOpts
	gasKey   receiptGasSigner
	gas      common.Address
	token    *testusdc3009.Testusdc3009
	contract *escrowabi.Orderescrow
}

func deployEscrow(t *testing.T, ctx context.Context, client *ethclient.Client, db *pgxpool.Pool) escrowDeployment {
	t.Helper()
	return deployEscrowFor(t, ctx, client, db, "TestUSDC3009")
}

// deployEscrowFor deploys the order contract with its own copy of a test token.
func deployEscrowFor(t *testing.T, ctx context.Context, client *ethclient.Client, db *pgxpool.Pool, artifact string) escrowDeployment {
	t.Helper()
	// Fresh keys give this chain a P07 stream identity no other test uses.
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	gasKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []*ecdsa.PrivateKey{key, gasKey} {
		if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", crypto.PubkeyToAddress(account.PublicKey), "0x56BC75E2D63100000"); err != nil {
			t.Fatal(err)
		}
	}
	operator, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	operator.Context, operator.GasLimit = ctx, 10_000_000
	mined := func(tx *types.Transaction, err error) *types.Receipt {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := bind.WaitMined(ctx, client, tx)
		if err != nil || receipt.Status != 1 {
			t.Fatalf("escrow fixture reverted: %v", err)
		}
		return receipt
	}
	var asset common.Address
	if artifact == "NativeCoinStandIn" { // the chain's own coin has an address of its own
		deployed, tx, err := deployArtifact(t, operator, client, artifact)
		mined(tx, err)
		code, err := client.CodeAt(ctx, deployed, nil)
		if err != nil {
			t.Fatal(err)
		}
		asset = commerce.StableNetCoin
		if err := client.Client().CallContext(ctx, nil, "anvil_setCode", asset, hexutil.Bytes(code)); err != nil {
			t.Fatal(err)
		}
	} else {
		deployed, tx, err := deployArtifact(t, operator, client, artifact, operator.From)
		mined(tx, err)
		asset = deployed
	}
	address, tx, err := deployArtifact(t, operator, client, "OrderEscrow", asset, operator.From, fixtureSigner(106).Address())
	deployed := mined(tx, err)
	token, err := testusdc3009.NewTestusdc3009(asset, client)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := escrowabi.NewOrderescrow(address, client)
	if err != nil {
		t.Fatal(err)
	}
	if artifact == "TestUSDC3009" { // the other test tokens hold nothing back
		mined(token.SetTrustedSender(operator, address, true))
	}
	if artifact == "TestUSDC3009" {
		mined(token.Mint(operator, fixtureSigner(102).Address(), big.NewInt(300_000_000)))
	}
	mined(contract.ConfigureMerchant(operator, [32]byte{31: 50}, fixtureSigner(101).Address(), fixtureSigner(104).Address(), 2*86400, 7*86400))
	code, err := client.CodeAt(ctx, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	return escrowDeployment{deployed: deployed, operator: operator, gasKey: receiptGasSigner{gasKey}, gas: crypto.PubkeyToAddress(gasKey.PublicKey), token: token, contract: contract,
		target: Target{Stream: fixtureUUID(t, db), Manifest: strings.Repeat("24", 32), Contract: address,
			CodeHash: crypto.Keccak256Hash(code), Asset: asset, ABI: EscrowABIHash}}
}

func TestLoopbackOrderEscrow(t *testing.T) {
	settlementEndpoint(t)
	endpoint := settlementChainSince(t, 25*time.Hour) // a whole business day, for its close
	db, dsn := scratchDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	escrow := deployEscrow(t, ctx, client, db)
	target := escrow.target

	// The certificate positions are those of the cafe fixture, so its kitchen checks run unchanged.
	const customer, operator, stranger, foreignTerminal, terminal = 0, 3, 4, 5, 7
	const finance1, finance2, newcomer = 1, 8, 9
	base := seededOrder{Tenant: fixtureUUID(t, db), Branch: fixtureUUID(t, db), CipherKey: [32]byte{31: 53}, Target: target}
	actors := make([]string, 10)
	for i := range actors {
		actors[i] = fixtureUUID(t, db)
	}
	base.Actor = actors[customer]
	fixtureExec(t, db, "INSERT INTO ledger.branch VALUES($1,$2,'Asia/Seoul','04:00')", base.Branch, base.Tenant)
	fixtureExec(t, db, "INSERT INTO ledger.command_rule VALUES($1,'escrow','escrow','escrow',interval '15 minutes')", base.Branch)
	payers := map[int]fixtureSigner{customer: 102, stranger: 105}
	for actor, payer := range payers {
		fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'customer',NULL,now()+interval '1 hour')", base.Branch, actors[actor])
		fixtureExec(t, db, "INSERT INTO ledger.payer_enrollment VALUES($1,$2,$3,1,1,now()+interval '1 hour')", base.Branch, actors[actor], payer.Address().Bytes())
	}
	payers[newcomer] = 150 // enrolls its own board below
	fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'customer',NULL,now()+interval '1 hour')", base.Branch, actors[newcomer])
	fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'terminal','menu',now()+interval '1 hour')", base.Branch, actors[terminal])
	fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'terminal','menu',now()+interval '1 hour')", base.Branch, actors[foreignTerminal])
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'pickup_manage',now()+interval '1 hour')", base.Branch, actors[operator])
	for _, actor := range []int{finance1, finance2} {
		for _, capability := range []string{"refund_request", "report_read", "report_export", "reconcile_read", "day_close", "escrow_claim", "escrow_dispute_request", "escrow_dispute_approve"} {
			if actor == finance1 || strings.HasPrefix(capability, "escrow_dispute") {
				fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,$3,now()+interval '1 hour')", base.Branch, actors[actor], capability)
			}
		}
	}
	fixtureExec(t, db, "INSERT INTO ledger.menu_release VALUES($1,1,now()-interval '1 minute',now()+interval '15 minutes')", base.Branch)
	fixtureExec(t, db, "INSERT INTO ledger.price VALUES($1,1,'escrow-cafe','plain','Escrow cafe',$2,$3::text::numeric)", base.Branch, lower(target.Asset), escrowAmount)
	fixtureExec(t, db, "INSERT INTO ledger.stock VALUES($1,'escrow-cafe',60)", base.Branch)
	profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
		Signer: fixtureSigner(101).Address(), MerchantID: [32]byte{31: 50}, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}, Escrow: true}
	digest := profile.Digest()
	fixtureExec(t, db, "INSERT INTO ledger.issuance_profile VALUES($1,1,$2,120,true)", base.Branch, digest[:])

	qCipher, _ := commerce.NewCipher(base.CipherKey[:])
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
	_, httpDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	commerceStore, err := commerce.Open(ctx, httpDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer commerceStore.Close()
	view, proof := startEscrowChainview(t, ctx, db, dsn, endpoint, os.Getenv("P05_SETTLEMENT_TEST_INDEXER"), client, target, escrow.deployed)

	trust, certs, _ := mtlsClients(t, 10)
	expires := time.Now().Add(time.Hour).Unix()
	roles := map[int]string{customer: "customer", stranger: "customer", newcomer: "customer", terminal: "terminal", foreignTerminal: "terminal", operator: "operator"}
	credentials := map[string]service.Principal{}
	for index, role := range roles {
		credentials[fingerprint(certs[index])] = service.Principal{Role: role, Tenant: base.Tenant, Subject: actors[index], ExpiresAt: expires}
	}
	// Another tenant with an otherwise valid terminal.
	credentials[fingerprint(certs[6])] = service.Principal{Role: "terminal", Tenant: fixtureUUID(t, db), Subject: actors[terminal], ExpiresAt: expires}
	grant := [32]byte{31: 91}
	sponsors := func(_ context.Context, branch string, p commerce.Profile, payer common.Address, now time.Time) (commerce.GasSponsorship, bool) {
		until := now.Add(time.Minute)
		return commerce.GasSponsorship{Account: escrow.gas, Grant: grant, ExpiresAt: until.Unix(),
			CurrentPublication: func(at time.Time) bool { return at.Before(until) }}, branch == base.Branch && p.Contract == target.Contract && payer != escrow.gas
	}
	target.SendsPerDay = 3
	targets := map[string]Target{base.Branch: target}
	merchants := map[string]string{base.Branch: common.Hash{31: 50}.Hex()}
	faucet := &Faucet{Account: fixtureSigner(160).Address(), Signer: fixtureSigner(160), Tokens: map[string][]FaucetToken{base.Branch: {
		{Address: target.Asset, Symbol: "tUSDC", Decimals: 6, Amount: "5000000", PerDay: 2, DailyTotal: "12000000"}}},
		Funds: FaucetFunds(client, fixtureSigner(160).Address(), big.NewInt(100_000_000_000))}
	forwarder := Forwarder{Store: open("nu54_p05_commerce"), QuoteCipher: qCipher, RelayCipher: relayCipher, Targets: targets, GasSponsors: sponsors,
		MerchantIDs: map[string]common.Hash{base.Branch: profile.MerchantID}, Faucet: faucet, GasAccount: escrow.gas}
	mux := http.NewServeMux()
	commerce.HTTP{Store: commerceStore, Payment: forwarder, GasSponsors: sponsors, Credentials: credentials, Signer: fixtureSigner(101), Cipher: qCipher,
		Profiles: map[string]commerce.Profile{base.Branch: profile}, PresentationKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, 32))}.Register(mux)
	finance := FinanceHTTP{Store: open("nu54_p05_finance"), Funds: LiveChain{client}, Targets: targets, Merchants: merchants, Evidence: proof,
		CloseStore: open("nu54_p05_settlement"), QuoteCipher: qCipher, OperatorOrigin: "https://ops.test", Resolver: fixtureSigner(106).Address(),
		Credentials: map[string]service.Principal{
			fingerprint(certs[finance1]): {Role: "finance", Tenant: base.Tenant, Subject: actors[finance1], ExpiresAt: expires},
			fingerprint(certs[finance2]): {Role: "finance", Tenant: base.Tenant, Subject: actors[finance2], ExpiresAt: expires}}}
	finance.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	clients := make([]*http.Client, len(certs))
	for i := range certs {
		transport := server.Client().Transport.(*http.Transport).Clone()
		transport.TLSClientConfig.Certificates = []tls.Certificate{certs[i]}
		defer transport.CloseIdleConnections()
		clients[i] = &http.Client{Transport: transport, Timeout: 7 * time.Second}
	}
	post := func(identity int, path string, body any, want int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		request, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "https://ops.test")
		response, err := clients[identity].Do(request)
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
	call := func(identity int, command service.Request, extras map[string]any) pilotReply {
		t.Helper()
		reply, _, err := pilotCall(ctx, clients[identity], server.URL, command, extras)
		if err != nil {
			t.Fatal(err)
		}
		return reply
	}
	status := func(identity int, order seededOrder, want int) (commerce.Order, []byte) {
		t.Helper()
		raw := post(identity, "/v2/commerce", map[string]any{"command": newPilotCommand(t, db, order.Branch, order.Sale, "order.status", map[string]string{}, false)}, want)
		var reply struct{ Order commerce.Order }
		if want == 200 {
			if err := json.Unmarshal(raw, &reply); err != nil {
				t.Fatal(err)
			}
			validateEscrowWire(t, ctx, raw)
		}
		return reply.Order, raw
	}
	// The kiosk reserves for a nearby customer, binds the payer and asks for the quote.
	reserve := func(buyer int) (seededOrder, string) {
		t.Helper()
		order := base
		order.Actor = actors[buyer]
		cart := commerce.Cart{Origin: "menu", OriginRef: fixtureUUID(t, db), MenuVersion: 1,
			Items:  []commerce.Item{{SKU: "escrow-cafe", Option: "plain", Qty: 1}},
			Nearby: &commerce.NearbyReservation{Customer: actors[buyer], ExpiresAt: time.Now().Add(time.Minute).Unix()}}
		raw, _ := json.Marshal(cart)
		hash := sha256.Sum256(raw)
		order.Sale = call(terminal, newPilotCommand(t, db, order.Branch, cart.OriginRef, "order.create", map[string]string{
			"asset": lower(target.Asset), "quantity": "1", "intentDigest": rawHex(hash[:])}, true), map[string]any{"cart": cart}).ResultRef
		claim := call(terminal, newPilotCommand(t, db, order.Branch, order.Sale, "checkout.authorize", map[string]string{"customerRef": actors[buyer]}, true), nil).Claim
		copy(order.Order.Claim[:], common.FromHex(claim.Digest))
		return order, claim.Digest
	}
	place := func(buyer int) seededOrder {
		t.Helper()
		order, claim := reserve(buyer)
		opened := call(terminal, newPilotCommand(t, db, order.Branch, order.Sale, "order.status", map[string]string{}, false), nil).Order
		quote := call(terminal, newPilotCommand(t, db, order.Branch, order.Sale, "quote.issue", map[string]string{
			"claimDigest": claim, "orderRevision": strconv.FormatInt(opened.Revision, 10)}, true), nil).Quote
		order.Signed, _ = hex.DecodeString(quote.Signed)
		signedDigest, _ := hex.DecodeString(quote.Digest)
		order.Quote, err = unpackQuote(order.Signed, signedDigest, profile.Signer.Bytes(), target)
		if err != nil {
			t.Fatal(err)
		}
		args := order.Quote.Args
		lifetime := quote.Expires - int64(args.IssuedAt)
		if order.Quote.Args.Payer != payers[buyer].Address() || args.Mode != 1 || args.PayerRole != 1 ||
			common.BytesToAddress(order.Signed[32:64]) != target.Contract || lifetime > 122 || quote.Presentation.GasSponsor == nil {
			t.Fatalf("escrow quote fields: lifetime %d, %+v", lifetime, args)
		}
		order.Order.Payer, order.Order.Amount, order.Order.OrderID = args.Payer, escrowAmount, args.OrderId
		order.Order.Issued, order.Order.Expires = time.Unix(int64(args.IssuedAt), 0), time.Unix(quote.Expires, 0)
		return order
	}
	phone, otherPhone := fixtureSigner(120).Address(), fixtureSigner(121).Address()
	command := func(order seededOrder, confirmer common.Address) (service.Request, map[string]any) {
		t.Helper()
		lock, err := orderescrow.NewLock(order.Quote.Unsigned, confirmer)
		if err != nil {
			t.Fatal(err)
		}
		signature, err := boardFor(order, payers).SignDigest(ctx, lock.Digest)
		if err != nil {
			t.Fatal(err)
		}
		return newPilotCommand(t, db, order.Branch, order.Sale, "order.lock", map[string]string{"quoteDigest": rawHex(order.Quote.Digest[:]),
			"confirmer": lower(confirmer), "lockDigest": rawHex(lock.Digest[:])}, true), map[string]any{"approvalSignature": rawHex(signature)}
	}
	refused := func(identity int, cmd service.Request, extras map[string]any, httpStatus int, code service.Code) {
		t.Helper()
		reply, got, err := pilotCall(ctx, clients[identity], server.URL, cmd, extras)
		if err == nil || got != httpStatus || reply.Error != code {
			t.Fatalf("%s: expected %d %s, got %d %s (%v)", cmd.Operation, httpStatus, code, got, reply.Error, err)
		}
	}
	chain := &lostResponseChain{LiveChain: LiveChain{client}}
	worker := func() *Worker {
		return &Worker{Store: open("nu54_p05_settlement"), Evidence: proof, Chain: chain, Targets: targets, QuoteCipher: qCipher, RelayCipher: relayCipher,
			GasAccount: escrow.gas, GasSigner: escrow.gasKey, MaxGasPrice: big.NewInt(100_000_000_000), Now: time.Now,
			MerchantSigner: fixtureSigner(101), MerchantSigners: map[string]common.Address{base.Branch: profile.Signer},
			Resolver: fixtureSigner(106).Address(), ResolverSigner: fixtureSigner(106),
			MerchantIDs: map[string]common.Hash{base.Branch: profile.MerchantID}, Faucet: faucet}
	}
	w := worker()
	step := func(order seededOrder) error {
		t.Helper()
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now()+interval '1 hour' WHERE id<>$1 AND NOT finished", order.Sale)
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now() WHERE id=$1", order.Sale)
		moved, err := w.stepPayment(ctx)
		if !moved {
			t.Fatal("settlement case missing", err)
		}
		return err
	}
	// Keep the chain just behind the wall clock, inside the quote's lifetime.
	inTime := func(order seededOrder) {
		t.Helper()
		head, err := client.HeaderByNumber(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if issued := uint64(order.Order.Issued.Unix()); head.Time <= issued {
			if err := client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", issued+1); err != nil {
				t.Fatal(err)
			}
		}
		mine(t, ctx, client, 1)
	}
	// reach moves the chain to a time it has not passed yet.
	reach := func(at int64) {
		t.Helper()
		head, err := client.HeaderByNumber(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		forceTime(t, ctx, client, max(at, int64(head.Time)+1))
	}
	relay := func(order seededOrder) (hash *string, state *string) {
		t.Helper()
		if err := db.QueryRow(ctx, "SELECT (SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1),(SELECT state FROM ledger.relay_attempt WHERE work_id=$1)",
			order.Sale).Scan(&hash, &state); err != nil {
			t.Fatal(err)
		}
		return
	}
	stage := func(order seededOrder, buyer int, state, want string, pays ...string) commerce.Order {
		t.Helper()
		var seen commerce.Order
		for _, identity := range []int{terminal, buyer} {
			got, _ := status(identity, order, 200)
			if got.State != state || got.Escrow == nil || got.Escrow.Stage != want || got.Escrow.Confirmer != lower(phone) ||
				got.Escrow.Payer != lower(order.Quote.Args.Payer) || got.Escrow.Escrow != lower(target.Contract) ||
				got.Escrow.OrderKey != rawHex(order.Quote.Args.OrderId[:]) || got.Escrow.MerchantID != rawHex(order.Quote.Args.MerchantId[:]) {
				t.Fatalf("expected %s/%s, got %s %+v", state, want, got.State, got.Escrow)
			}
			if identity == buyer && (got.Payment == nil || !bytes.Contains([]byte(strings.Join(pays, ",")), []byte(got.Payment.Stage))) {
				t.Fatalf("payment stage %+v, expected one of %v", got.Payment, pays)
			}
			seen = got
		}
		return seen
	}

	t.Run("DeploymentReview", func(t *testing.T) {
		public, private, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		review := func(target Target, version uint8, token common.Address) string {
			signed := deployment.Profile{StreamID: target.Stream, GenerationID: base.Branch, Version: version, ChainID: 8283, Contract: target.Contract,
				Asset: token, ABIHash: target.ABIHash(), Manifest: target.Manifest, CodeHash: target.CodeHash,
				StartHash: escrow.deployed.BlockHash, StartHeight: escrow.deployed.BlockNumber.Int64()}
			body, _ := json.Marshal(signed)
			signature := ed25519.Sign(private, append([]byte("NU54-chainview-profile-v1\x00"), body...))
			return VerifyOperatorDeployment(ctx, client, target, escrow.gas, []deployment.Profile{signed}, public,
				map[string]string{base.Branch: rawHex(signature)}, true).Status
		}
		old := target
		old.ABI = ""
		if review(target, 3, target.Asset) != "verified" || review(target, 2, target.Asset) != "invalid" ||
			review(old, 3, target.Asset) != "invalid" || review(target, 3, common.HexToAddress("0xbad")) != "invalid" {
			t.Fatal("deployment review accepted the wrong payment contract")
		}
		shown := finance
		shown.Deployments = map[string]DeploymentReview{base.Branch: {Status: "verified"}}
		mine(t, ctx, client, 5)
		if got := shown.deploymentReview(ctx, base.Branch, target); got.BeneficiaryVersion == nil || *got.BeneficiaryVersion != 1 {
			t.Fatalf("escrow merchant registration not read: %+v", got)
		}
	})

	t.Run("TerminalLock", func(t *testing.T) {
		order := place(customer)
		inTime(order)
		cmd, body := command(order, phone)

		for name, change := range map[string]func(service.Request, map[string]any){
			"another phone key": func(c service.Request, _ map[string]any) { c.Body["confirmer"] = lower(otherPhone) },
			"another quote":     func(c service.Request, _ map[string]any) { c.Body["quoteDigest"] = strings.Repeat("a", 64) },
			"another lock":      func(c service.Request, _ map[string]any) { c.Body["lockDigest"] = strings.Repeat("b", 64) },
			"another signer": func(c service.Request, b map[string]any) {
				signature, _ := fixtureSigner(105).SignDigest(ctx, common.HexToHash(c.Body["lockDigest"]))
				b["approvalSignature"] = rawHex(signature)
			},
		} {
			changed, altered := command(order, phone)
			change(changed, altered)
			t.Log(name)
			refused(terminal, changed, altered, 400, service.Mismatch)
		}
		for _, confirmer := range []common.Address{profile.Signer, profile.Beneficiary, order.Quote.Args.Payer, target.Contract} {
			changed, altered := command(order, confirmer)
			refused(terminal, changed, altered, 400, service.Mismatch)
		}
		for _, identity := range []int{stranger, foreignTerminal, 6} {
			refused(identity, cmd, body, 404, "")
		}
		approval, _ := payers[customer].SignDigest(ctx, order.Quote.ApprovalHash)
		refused(customer, newPilotCommand(t, db, order.Branch, order.Sale, "payment.forward", map[string]string{
			"quoteDigest": rawHex(order.Quote.Digest[:]), "approvalDigest": rawHex(order.Quote.ApprovalHash[:])}, true),
			map[string]any{"approvalSignature": rawHex(approval)}, 400, service.Capability)
		refused(customer, newPilotCommand(t, db, order.Branch, order.Sale, "order.answer", map[string]string{"action": "receipt", "claimableAt": "1800000000"}, true),
			map[string]any{"approvalSignature": rawHex(approval)}, 404, "")
		if view, _ := status(terminal, order, 200); view.State != "issued" || view.Escrow != nil {
			t.Fatalf("a refused lock changed the order: %+v", view)
		}

		for range 2 { // the same request again gives the same result
			cmd.RequestID = fixtureUUID(t, db)
			if reply := call(terminal, cmd, body); reply.ResultRef != order.Sale {
				t.Fatal("lock returned another order")
			}
		}
		// The phone sends the same lock over the internet.
		again, againBody := command(order, phone)
		for range 2 {
			again.RequestID = fixtureUUID(t, db)
			if reply := call(customer, again, againBody); reply.ResultRef != order.Sale {
				t.Fatal("the same lock over the other route returned another order")
			}
		}
		different, differentBody := command(order, otherPhone)
		refused(customer, different, differentBody, 409, service.Conflict)
		other, signed := command(order, otherPhone)
		refused(terminal, other, signed, 409, service.Conflict)
		other.IdempotencyKey = cmd.IdempotencyKey
		refused(terminal, other, signed, 409, service.Conflict)
		var sealed, submissions int
		if err := db.QueryRow(ctx, "SELECT octet_length(approval_cipher),(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1) FROM ledger.settlement_submission WHERE sale_id=$1",
			order.Sale).Scan(&sealed, &submissions); err != nil || sealed != 113 || submissions != 1 {
			t.Fatalf("sealed lock %d bytes, %d rows: %v", sealed, submissions, err)
		}
		stage(order, customer, "checking", "received", "received")
		if _, raw := status(stranger, order, 404); bytes.Contains(raw, []byte("escrow")) {
			t.Fatal("another customer saw the escrow order")
		}
		verifyKioskCafeHold(t, db, order, post)

		if err := step(order); err != nil {
			t.Fatal(err)
		}
		prepared, _ := relay(order)
		if prepared == nil || chain.sends != 0 {
			t.Fatal("preparation must store the signed bytes before anything is sent")
		}
		if view := stage(order, customer, "checking", "submitted", "relaySigned"); *view.Escrow.LockTxHash != *prepared {
			t.Fatal("status shows another lock transaction")
		}
		w = worker() // the process restarts between signing and sending
		inTime(order)
		if err := step(order); !errors.Is(err, errLostResponse) {
			t.Fatalf("expected the reply to be lost after delivery: %v", err)
		}
		w = worker() // and again after the lost reply
		receipt, err := bind.WaitMinedHash(ctx, client, common.HexToHash(*prepared))
		if err != nil || receipt.Status != 1 {
			t.Fatalf("lock reverted: %v", err)
		}
		t.Logf("OrderEscrow.lock used %d gas, limit %d", receipt.GasUsed, relayGasLimit)
		if receipt.GasUsed > relayGasLimit*3/5 {
			t.Fatalf("lock used %d gas; raise the limit", receipt.GasUsed)
		}
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		if again, _ := relay(order); *again != *prepared || chain.sends != 1 {
			t.Fatalf("a retry changed or repeated the lock: sends %d", chain.sends)
		}
		stage(order, customer, "checking", "submitted", "checking")
		if held, err := escrow.contract.TotalHeld(nil); err != nil || held.String() != escrowAmount {
			t.Fatalf("escrow holds %v: %v", held, err)
		}

		view.sync(receipt.TxHash)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		locked := stage(order, customer, "paid", "locked", "finalized")
		onchain, err := escrow.contract.Orders(nil, order.Quote.Args.MerchantId, order.Quote.Args.OrderId)
		if err != nil || locked.PickupNo == nil || *locked.Escrow.LockTxHash != *prepared || locked.Escrow.DeliverBy != int64(onchain.DeliverBy) ||
			onchain.Confirmer != phone || onchain.State != 1 {
			t.Fatalf("locked order differs from the chain: %+v %+v %v", locked.Escrow, onchain, err)
		}
		// Reading the same fact again changes nothing.
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=false WHERE id=$1", order.Sale)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		verifyEscrowAccounts(t, ctx, db, finance, order, actors[1], post)
		verifyKioskCafeProgress(t, ctx, db, dsn, proof, order, trust, certs[2], locked, post, status, customer, "collected", nil)
	})

	t.Run("AlreadyPaidByAnotherSender", func(t *testing.T) {
		order := place(customer)
		inTime(order)
		cmd, body := command(order, phone)
		if reply := call(customer, cmd, body); reply.ResultRef != order.Sale { // the customer role may send it too
			t.Fatal("customer lock refused")
		}
		sends := chain.sends
		// The relay signs first; anybody may then land the same lock with their own gas.
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		signature := common.FromHex(body["approvalSignature"].(string))
		outside, err := bind.NewKeyedTransactorWithChainID(mustKey(t, 130), big.NewInt(8283))
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", outside.From, "0x56BC75E2D63100000"); err != nil {
			t.Fatal(err)
		}
		tx, err := escrow.contract.Lock(outside, escrowabi.PaymentTypesV2MerchantQuote(order.Quote.Args), phone, order.Quote.MerchantSig, signature)
		if err != nil {
			t.Fatal(err)
		}
		landed, err := bind.WaitMined(ctx, client, tx)
		if err != nil || landed.Status != 1 {
			t.Fatalf("outside lock: %v", err)
		}
		if err := step(order); err != nil { // our transaction now reverts
			t.Fatal(err)
		}
		hash, _ := relay(order)
		ours, err := bind.WaitMinedHash(ctx, client, common.HexToHash(*hash))
		if err != nil || ours.Status != 0 || chain.sends != sends+1 {
			t.Fatalf("expected one reverted duplicate: %v", err)
		}
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		stage(order, customer, "checking", "submitted", "checking") // a revert alone never closes or fails the order
		view.sync(landed.TxHash)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		if paid := stage(order, customer, "paid", "locked", "finalized"); *paid.Escrow.LockTxHash != landed.TxHash.Hex() {
			t.Fatal("the confirmed lock is not the one on chain")
		}
	})

	t.Run("InsufficientBalanceThenExpiry", func(t *testing.T) {
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=8 WHERE branch_id=$1", base.Branch)
		order := place(stranger) // a wallet without money
		late := place(customer)
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=120 WHERE branch_id=$1", base.Branch)
		inTime(order)
		cmd, body := command(order, phone)
		call(terminal, cmd, body)
		sends := chain.sends
		if err := step(order); err == nil || !strings.Contains(err.Error(), "would not execute") {
			t.Fatalf("a lock that must revert was prepared: %v", err)
		}
		if hash, _ := relay(order); hash != nil || chain.sends != sends {
			t.Fatal("the store paid gas for a lock without money")
		}
		stage(order, stranger, "checking", "received", "received")
		time.Sleep(time.Until(order.Order.Expires) + time.Second)
		lateCommand, lateBody := command(late, phone)
		refused(terminal, lateCommand, lateBody, 400, service.Expired)
		if err := step(order); err != nil { // expired, but nothing proves the lock is absent
			t.Fatal(err)
		}
		stage(order, stranger, "checking", "received", "received")
		reach(order.Order.Expires.Unix() + 1)
		before := stockOf(t, ctx, db, base.Branch)
		closeExpired(t, db, order, func() error { return step(order) })
		stage(order, stranger, "closed", "failed", "unpaid")
		if stockOf(t, ctx, db, base.Branch) != before+1 {
			t.Fatal("a failed lock did not release its stock")
		}
	})

	t.Run("UnsentOrderExpires", func(t *testing.T) {
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=10 WHERE branch_id=$1", base.Branch)
		order := place(customer)
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=120 WHERE branch_id=$1", base.Branch)
		time.Sleep(time.Until(order.Order.Expires) + time.Second)
		reach(order.Order.Expires.Unix() + 1)
		closeExpired(t, db, order, func() error { return step(order) })
		if view, _ := status(customer, order, 200); view.State != "closed" {
			t.Fatalf("an order nobody paid stays %s", view.State)
		}
	})

	t.Run("RevertedLockThenExpiry", func(t *testing.T) {
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=10 WHERE branch_id=$1", base.Branch)
		order := place(customer)
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=120 WHERE branch_id=$1", base.Branch)
		inTime(order)
		cmd, body := command(order, phone)
		call(terminal, cmd, body)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		// The merchant is switched off after signing; the sent lock reverts on chain.
		setActive := func(active bool) {
			t.Helper()
			tx, err := escrow.contract.SetMerchantActive(escrow.operator, [32]byte{31: 50}, active)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := bind.WaitMined(ctx, client, tx); err != nil {
				t.Fatal(err)
			}
		}
		setActive(false)
		sends := chain.sends
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		hash, _ := relay(order)
		receipt, err := bind.WaitMinedHash(ctx, client, common.HexToHash(*hash))
		if err != nil || receipt.Status != 0 {
			t.Fatalf("expected a reverted lock: %v", err)
		}
		for range 2 { // the recorded revert is not sent again
			if err := step(order); err != nil {
				t.Fatal(err)
			}
		}
		if _, state := relay(order); *state != "reverted" || chain.sends != sends+1 {
			t.Fatalf("reverted lock: state %s, %d sends", *state, chain.sends-sends)
		}
		stage(order, customer, "checking", "submitted", "checking")
		setActive(true)
		time.Sleep(time.Until(order.Order.Expires) + time.Second)
		reach(order.Order.Expires.Unix() + 1)
		view.prove(t, order)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		stage(order, customer, "closed", "failed", "unpaid")
	})

	t.Run("SignedNeverSentKeepsNoNonce", func(t *testing.T) {
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=10 WHERE branch_id=$1", base.Branch)
		order := place(customer)
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=120 WHERE branch_id=$1", base.Branch)
		inTime(order)
		cmd, body := command(order, phone)
		call(terminal, cmd, body)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		hash, _ := relay(order)
		sends := chain.sends
		time.Sleep(time.Until(order.Order.Expires) + time.Second) // the relay is down until the quote expires
		reach(order.Order.Expires.Unix() + 1)
		view.prove(t, order)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		if failed := stage(order, customer, "closed", "failed", "unpaid"); hash == nil || chain.sends != sends || failed.Escrow.LockTxHash != nil {
			t.Fatalf("a lock that was never sent is shown: %+v", failed.Escrow)
		}
		next := place(customer)
		inTime(next)
		cmd, body = command(next, phone)
		call(terminal, cmd, body)
		for range 2 {
			if err := step(next); err != nil && !errors.Is(err, errLostResponse) {
				t.Fatalf("the abandoned lock blocks the gas account: %v", err)
			}
		}
		if _, state := relay(next); state == nil || *state != "broadcast" {
			t.Fatal("the next order was not sent")
		}
		sent, _ := relay(next)
		if _, err := bind.WaitMinedHash(ctx, client, common.HexToHash(*sent)); err != nil {
			t.Fatal(err)
		}
		view.sync(common.HexToHash(*sent))
		if err := step(next); err != nil {
			t.Fatal(err)
		}
		stage(next, customer, "paid", "locked", "finalized")
	})

	t.Run("ReconciliationFailsAfterSigning", func(t *testing.T) {
		order := place(customer)
		inTime(order)
		cmd, body := command(order, phone)
		call(terminal, cmd, body)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		sends := chain.sends
		w.Evidence = brokenEvidence{proof}
		if err := step(order); !errors.Is(err, errBrokenEvidence) || chain.sends != sends+1 {
			t.Fatalf("a fact that cannot be read kept signed bytes unsent: %v", err)
		}
		w = worker()
		hash, _ := relay(order)
		receipt, err := bind.WaitMinedHash(ctx, client, common.HexToHash(*hash))
		if err != nil || receipt.Status != 1 {
			t.Fatal(err)
		}
		view.sync(receipt.TxHash)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		stage(order, customer, "paid", "locked", "finalized")
	})

	// ---- after the lock ----

	due := func() {
		fixtureExec(t, db, `UPDATE ledger.settlement_work w SET retry_at=now() FROM ledger.escrow_action a WHERE a.id=w.action_id AND a.branch_id=$1 AND NOT w.finished`, base.Branch)
	}
	act := func() error { // one pass over one action
		t.Helper()
		due()
		moved, err := w.stepAction(ctx, []string{base.Branch})
		if !moved && err == nil {
			t.Fatal("no escrow action is due")
		}
		return err
	}
	settleActions := func(until func() bool) {
		t.Helper()
		var last error
		for deadline := time.Now().Add(25 * time.Second); time.Now().Before(deadline); {
			if until() {
				return
			}
			due()
			if _, err := w.stepAction(ctx, []string{base.Branch}); err != nil && !errors.Is(err, errLostResponse) {
				last = err
			}
			mine(t, ctx, client, 2)
			time.Sleep(150 * time.Millisecond) // the indexer reads every block it has to pass
		}
		var queue string
		_ = db.QueryRow(ctx, `SELECT coalesce(string_agg(a.kind||':'||a.state||':'||coalesce(r.state,'-')||':'||w.finished,' '),'') FROM ledger.escrow_action a
			JOIN ledger.settlement_work w ON w.action_id=a.id LEFT JOIN ledger.relay_attempt r ON r.work_id=w.id WHERE a.branch_id=$1`, base.Branch).Scan(&queue)
		t.Fatalf("escrow action did not finish: %v [%s]", last, queue)
	}
	at := func(order seededOrder, buyer int, want string) func() bool {
		return func() bool {
			seen, _ := status(buyer, order, 200)
			return seen.Escrow != nil && seen.Escrow.Stage == want
		}
	}
	lockOrder := func(buyer int) seededOrder {
		t.Helper()
		order := place(buyer)
		inTime(order)
		cmd, body := command(order, phone)
		call(terminal, cmd, body)
		for range 2 {
			if err := step(order); err != nil && !errors.Is(err, errLostResponse) {
				t.Fatal(err)
			}
		}
		hash, _ := relay(order)
		receipt, err := bind.WaitMinedHash(ctx, client, common.HexToHash(*hash))
		if err != nil || receipt.Status != 1 {
			t.Fatalf("lock: %v", err)
		}
		view.sync(receipt.TxHash)
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		stage(order, buyer, "paid", "locked", "finalized")
		return order
	}
	receiverMux := http.NewServeMux()
	Receiver{Store: open("nu54_p05_receiver"), Evidence: proof, Grants: map[string]ReceiverGrant{
		fingerprint(certs[2]): {Branches: []string{base.Branch}, ExpiresAt: expires}}}.Register(receiverMux)
	receiver := httptest.NewUnstartedServer(receiverMux)
	receiver.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	receiver.StartTLS()
	defer receiver.Close()
	receiverTransport := receiver.Client().Transport.(*http.Transport).Clone()
	receiverTransport.TLSClientConfig.Certificates = []tls.Certificate{certs[2]}
	defer receiverTransport.CloseIdleConnections()
	dispatch := Dispatcher{Evidence: proof, Store: open("nu54_p05_dispatch"), Client: &http.Client{Transport: receiverTransport, Timeout: 4 * time.Second},
		Targets: map[string]string{base.Branch: receiver.URL}}
	pickup := func(order seededOrder, targets ...string) {
		t.Helper()
		waitUntil(t, func() bool {
			_, _ = dispatch.Step(ctx) // the stream may be held for a moment while the indexer follows the chain
			var accepted bool
			if err := db.QueryRow(ctx, "SELECT ledger.travel_pickup_accepted($1)", order.Sale).Scan(&accepted); err != nil {
				t.Fatal(err)
			}
			return accepted
		})
		var revision int64
		if err := db.QueryRow(ctx, "SELECT coalesce(max(revision),0) FROM ledger.travel_pickup_progress WHERE sale_id=$1", order.Sale).Scan(&revision); err != nil {
			t.Fatal(err)
		}
		for _, target := range targets {
			kitchenCommand(post, operator, order.Branch, "pickup-change", map[string]any{"sale": order.Sale, "requestKey": fixtureUUID(t, db),
				"expectedRevision": revision, "target": target}, 200)
			revision++
		}
	}
	deliver := func(order seededOrder, buyer int) int64 {
		t.Helper()
		pickup(order, "preparing", "ready", "collected")
		settleActions(at(order, buyer, "delivered"))
		seen := stage(order, buyer, "paid", "delivered", "finalized")
		return *seen.Escrow.ClaimableAt
	}
	phoneKey := fixtureSigner(120)
	answer := func(order seededOrder, action string, claimableAt int64, key fixtureSigner) (service.Request, map[string]any) {
		t.Helper()
		digest, err := orderescrow.ActionDigest(orderescrow.BuyerActions[action], target.Contract, order.Quote.Args.MerchantId, order.Quote.Args.OrderId, uint64(claimableAt))
		if err != nil {
			t.Fatal(err)
		}
		signature, _ := key.SignDigest(ctx, digest)
		return newPilotCommand(t, db, order.Branch, order.Sale, "order.answer", map[string]string{"action": action, "claimableAt": strconv.FormatInt(claimableAt, 10)}, true),
			map[string]any{"approvalSignature": rawHex(signature)}
	}
	financeCall := func(identity int, operation string, body map[string]any, want int) map[string]any {
		t.Helper()
		body["branchId"] = base.Branch
		raw := post(identity, "/v2/finance/"+operation, body, want)
		var out map[string]any
		if want == 200 && json.Unmarshal(raw, &out) != nil {
			t.Fatalf("%s: %s", operation, raw)
		}
		return out
	}
	balance := func(account common.Address) *big.Int {
		t.Helper()
		units, err := escrow.token.BalanceOf(nil, account)
		if err != nil {
			t.Fatal(err)
		}
		return units
	}
	amount, _ := new(big.Int).SetString(escrowAmount, 10)
	if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", fixtureSigner(106).Address(), "0x56BC75E2D63100000"); err != nil {
		t.Fatal(err)
	}
	// Everything that needs a live quote is locked now; later steps move the chain by days.
	var claimable, contested, late, undelivered, missed seededOrder

	settleActions(func() bool { // the first order's delivery
		var open int
		_ = db.QueryRow(ctx, "SELECT count(*) FROM ledger.settlement_work w JOIN ledger.escrow_action a ON a.id=w.action_id WHERE a.branch_id=$1 AND NOT w.finished", base.Branch).Scan(&open)
		return open == 0
	})
	t.Run("DeliveryThenReceipt", func(t *testing.T) {
		order := lockOrder(customer)
		early, earlyBody := answer(order, "receipt", time.Now().Unix()+172800, phoneKey)
		refused(customer, early, earlyBody, 400, service.Mismatch)
		pickup(order, "preparing", "ready")
		var list struct {
			Pickups []struct {
				SaleRef string
				Items   []struct{ Label string }
				Escrow  *struct{ Stage string }
			}
		}
		if err := json.Unmarshal(kitchenCommand(post, operator, order.Branch, "pickup-queue", nil, 200), &list); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range list.Pickups {
			if row.SaleRef == order.Sale {
				found = len(row.Items) == 1 && row.Escrow != nil && row.Escrow.Stage == "locked"
			}
		}
		if !found {
			t.Fatal("the kitchen list lacks the items or the escrow stage")
		}
		pickup(order, "collected")
		stage(order, customer, "paid", "locked", "finalized") // the handover is not final yet
		// Finish the older queued delivery, if any, so the steps below are about this order.
		only := func() {
			fixtureExec(t, db, "UPDATE ledger.settlement_work w SET retry_at=now()+interval '1 hour' FROM ledger.escrow_action a WHERE a.id=w.action_id AND a.sale_id<>$1", order.Sale)
			fixtureExec(t, db, "UPDATE ledger.settlement_work w SET retry_at=now() FROM ledger.escrow_action a WHERE a.id=w.action_id AND a.sale_id=$1 AND NOT w.finished", order.Sale)
		}
		pass := func() error {
			t.Helper()
			only()
			moved, err := w.stepAction(ctx, []string{base.Branch})
			if !moved {
				t.Fatal("delivery is not queued", err)
			}
			return err
		}
		attempt := func() (hash, state string) {
			t.Helper()
			if err := db.QueryRow(ctx, "SELECT r.tx_hash,r.state FROM ledger.relay_attempt r JOIN ledger.escrow_action a ON a.id=r.work_id WHERE a.sale_id=$1 AND a.kind='deliver'", order.Sale).Scan(&hash, &state); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := pass(); err != nil {
			t.Fatal(err)
		}
		signed, state := attempt()
		w = worker() // crash between signing and sending
		chain.sends = 0
		if err := pass(); state != "signed" || !errors.Is(err, errLostResponse) {
			t.Fatalf("expected a lost reply: %s %v", state, err)
		}
		w = worker() // crash after the lost reply
		settleActions(at(order, customer, "delivered"))
		if sent, _ := attempt(); sent != signed || chain.sends != 1 {
			t.Fatalf("delivery was signed or sent twice: %d sends", chain.sends)
		}
		at1 := *stage(order, customer, "paid", "delivered", "finalized").Escrow.ClaimableAt

		wrongKey, wrongKeyBody := answer(order, "receipt", at1, fixtureSigner(121))
		refused(customer, wrongKey, wrongKeyBody, 400, service.Mismatch)
		wrongTime, wrongTimeBody := answer(order, "receipt", at1+1, phoneKey)
		refused(customer, wrongTime, wrongTimeBody, 400, service.Mismatch)
		cmd, body := answer(order, "receipt", at1, phoneKey)
		refused(stranger, cmd, body, 404, "")
		refused(terminal, cmd, body, 404, "")
		withdraw, withdrawBody := answer(order, "withdraw", at1, phoneKey)
		refused(customer, withdraw, withdrawBody, 409, service.CheckoutConflict)
		for range 2 {
			cmd.RequestID = fixtureUUID(t, db)
			if reply := call(customer, cmd, body); reply.ResultRef != order.Sale {
				t.Fatal("answer returned another order")
			}
		}
		if seen := stage(order, customer, "paid", "delivered", "finalized"); seen.Escrow.PendingAction != "receipt" {
			t.Fatalf("pending answer not shown: %+v", seen.Escrow)
		}
		second, secondBody := answer(order, "dispute", at1, phoneKey)
		refused(customer, second, secondBody, 409, service.Conflict)
		second.IdempotencyKey = cmd.IdempotencyKey
		refused(customer, second, secondBody, 409, service.Conflict)
		before := balance(profile.Beneficiary)
		settleActions(at(order, customer, "paid"))
		done := stage(order, customer, "paid", "paid", "finalized")
		if done.Escrow.Payout != "receipt" || done.Escrow.PendingAction != "" || done.PickupNo == nil ||
			new(big.Int).Sub(balance(profile.Beneficiary), before).Cmp(amount) != 0 {
			t.Fatalf("receipt did not pay the merchant: %+v", done.Escrow)
		}
		again, againBody := answer(order, "receipt", at1, phoneKey)
		refused(customer, again, againBody, 409, service.CheckoutConflict)
		call(customer, cmd, body) // the accepted request still returns its result
	})

	t.Run("StoreCancel", func(t *testing.T) {
		order := lockOrder(customer)
		before := balance(order.Quote.Args.Payer)
		pickup(order, "preparing", "unfulfillable")
		settleActions(at(order, customer, "refunded"))
		if done := stage(order, customer, "paid", "refunded", "finalized"); done.Escrow.Refund != "store" ||
			new(big.Int).Sub(balance(order.Quote.Args.Payer), before).Cmp(amount) != 0 {
			t.Fatalf("store cancel: %+v", done.Escrow)
		}
	})

	t.Run("NonceTakenByAnotherTransaction", func(t *testing.T) {
		order := lockOrder(customer)
		pickup(order, "preparing", "ready", "collected")
		if err := act(); err != nil {
			t.Fatal(err)
		}
		nonce, err := client.PendingNonceAt(ctx, escrow.gas)
		if err != nil {
			t.Fatal(err)
		}
		price, _ := client.SuggestGasPrice(ctx)
		foreign, _ := types.SignTx(types.NewTx(&types.LegacyTx{Nonce: nonce, To: &escrow.gas, Gas: 21000, GasPrice: price, Value: new(big.Int)}),
			types.LatestSignerForChainID(big.NewInt(8283)), escrow.gasKey.key)
		if err := client.SendTransaction(ctx, foreign); err != nil {
			t.Fatal(err)
		}
		settleActions(at(order, customer, "delivered")) // signed again with a fresh nonce
	})

	t.Run("DisputeRefundedByTwoPeople", func(t *testing.T) {
		order := lockOrder(customer)
		at1 := deliver(order, customer)
		cmd, body := answer(order, "dispute", at1, phoneKey)
		call(customer, cmd, body)
		settleActions(at(order, customer, "disputed"))
		receipt, receiptBody := answer(order, "receipt", at1, phoneKey)
		refused(customer, receipt, receiptBody, 409, service.CheckoutConflict)
		propose := map[string]any{"saleId": order.Sale, "outcome": "refund-buyer", "reason": "not_delivered", "idempotencyKey": fixtureUUID(t, db)}
		made := financeCall(finance1, "escrow-resolve", propose, 200)
		if again := financeCall(finance1, "escrow-resolve", propose, 200); made["decisionId"] != again["decisionId"] || made["state"] != "proposed" || made["canApprove"] != false {
			t.Fatalf("proposal: %v", made)
		}
		decide := func() map[string]any {
			return map[string]any{"decisionId": made["decisionId"], "idempotencyKey": fixtureUUID(t, db)}
		}
		financeCall(finance1, "escrow-resolve-approve", decide(), 404)
		financeCall(finance1, "escrow-resolve-reject", decide(), 404)
		financeCall(finance1, "escrow-resolve", map[string]any{"saleId": order.Sale, "outcome": "pay-merchant", "reason": "goodwill", "idempotencyKey": fixtureUUID(t, db)}, 409)
		financeCall(finance2, "escrow-resolve-reject", decide(), 200)
		made = financeCall(finance1, "escrow-resolve", map[string]any{"saleId": order.Sale, "outcome": "refund-buyer", "reason": "wrong_item", "idempotencyKey": fixtureUUID(t, db)}, 200)
		if approved := financeCall(finance2, "escrow-resolve-approve", decide(), 200); approved["state"] != "approved" {
			t.Fatalf("approval: %v", approved)
		}
		financeCall(finance2, "escrow-resolve-approve", decide(), 200)
		financeCall(finance2, "escrow-resolve-reject", decide(), 409)
		settleActions(at(order, customer, "refunded"))
		done := stage(order, customer, "paid", "refunded", "finalized")
		tx, _, err := client.TransactionByHash(ctx, common.HexToHash(*done.Escrow.SettledTxHash))
		if err != nil {
			t.Fatal(err)
		}
		if sender, _ := types.Sender(types.LatestSignerForChainID(big.NewInt(8283)), tx); done.Escrow.Refund != "arbiter" || sender != fixtureSigner(106).Address() {
			t.Fatalf("dispute: %+v sent by %s", done.Escrow, sender)
		}
		item := financeCall(finance1, "escrow-orders", map[string]any{"status": "refunded"}, 200)["items"].([]any)[0].(map[string]any)
		if decision := item["decision"].(map[string]any); item["saleId"] != order.Sale || decision["state"] != "final" || decision["transaction"] != *done.Escrow.SettledTxHash {
			t.Fatalf("decision record: %v", item)
		}
	})

	t.Run("DisputeWithdrawn", func(t *testing.T) {
		order := lockOrder(customer)
		at1 := deliver(order, customer)
		cmd, body := answer(order, "dispute", at1, phoneKey)
		call(customer, cmd, body)
		settleActions(at(order, customer, "disputed"))
		cmd, body = answer(order, "withdraw", at1, phoneKey)
		call(customer, cmd, body)
		settleActions(at(order, customer, "paid"))
		if done := stage(order, customer, "paid", "paid", "finalized"); done.Escrow.Payout != "dispute-withdrawn" || done.Escrow.ResolveBy == nil {
			t.Fatalf("withdrawn dispute: %+v", done.Escrow)
		}
	})

	t.Run("BuyerLocksWithAnotherPhoneKey", func(t *testing.T) {
		order := place(customer)
		inTime(order)
		cmd, body := command(order, otherPhone)
		call(customer, cmd, body) // the store holds one phone key
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		_, signed := command(order, phone) // the buyer lands the other by itself
		outside, _ := bind.NewKeyedTransactorWithChainID(mustKey(t, 130), big.NewInt(8283))
		tx, err := escrow.contract.Lock(outside, escrowabi.PaymentTypesV2MerchantQuote(order.Quote.Args), phone, order.Quote.MerchantSig, common.FromHex(signed["approvalSignature"].(string)))
		if err != nil {
			t.Fatal(err)
		}
		if landed, err := bind.WaitMined(ctx, client, tx); err != nil || landed.Status != 1 {
			t.Fatal(err)
		}
		if err := step(order); err != nil { // the store's own lock reverts
			t.Fatal(err)
		}
		view.sync(tx.Hash())
		if err := step(order); err != nil {
			t.Fatal(err)
		}
		stage(order, customer, "paid", "locked", "finalized") // with the phone key the contract checked
	})

	fee := func(wei uint64) {
		t.Helper()
		if err := client.Client().CallContext(ctx, nil, "anvil_setNextBlockBaseFeePerGas", fmt.Sprintf("0x%x", wei)); err != nil {
			t.Fatal(err)
		}
	}
	automine := func(on bool) {
		t.Helper()
		if err := client.Client().CallContext(ctx, nil, "evm_setAutomine", on); err != nil {
			t.Fatal(err)
		}
	}
	relayPrices := func(kind string, order seededOrder) (current string, earlier int) {
		t.Helper()
		if err := db.QueryRow(ctx, `SELECT r.tx_hash,(SELECT count(*) FROM ledger.relay_version v WHERE v.work_id=r.work_id) FROM ledger.relay_attempt r
			JOIN ledger.escrow_action a ON a.id=r.work_id WHERE a.sale_id=$1 AND a.kind=$2`, order.Sale, kind).Scan(&current, &earlier); err != nil {
			t.Fatal(err)
		}
		return
	}
	const gwei = 1_000_000_000

	t.Run("BaseFeeRisesAboveSignedPrice", func(t *testing.T) {
		first, second := lockOrder(customer), lockOrder(customer)
		pickup(first, "preparing", "ready", "collected")
		if err := act(); err != nil { // signed at the price of a quiet chain
			t.Fatal(err)
		}
		signed, _ := relayPrices("deliver", first)
		pickup(second, "preparing", "unfulfillable")
		automine(false)
		defer automine(true)
		for range 12 {
			fee(50 * gwei)
			mine(t, ctx, client, 1)
			due()
			if _, err := w.stepAction(ctx, []string{base.Branch}); err != nil && !errors.Is(err, errLostResponse) {
				t.Log(err)
			}
		}
		replaced, earlier := relayPrices("deliver", first)
		if replaced == signed || earlier == 0 {
			t.Fatal("an underpriced delivery was not replaced")
		}
		for range 6 {
			fee(50 * gwei)
			mine(t, ctx, client, 1)
			due()
			_, _ = w.stepAction(ctx, []string{base.Branch})
		}
		receipt, err := client.TransactionReceipt(ctx, common.HexToHash(replaced))
		if err != nil || receipt.Status != 1 {
			t.Fatalf("the replacement was not included: %v", err)
		}
		automine(true)
		fee(gwei)
		settleActions(at(first, customer, "delivered"))
		settleActions(at(second, customer, "refunded")) // the next job was never blocked for good
	})

	t.Run("EarlierPriceIsIncluded", func(t *testing.T) {
		order := lockOrder(customer)
		pickup(order, "preparing", "ready", "collected")
		automine(false)
		defer automine(true)
		if err := act(); err != nil { // signs
			t.Fatal(err)
		}
		if err := act(); err != nil && !errors.Is(err, errLostResponse) { // sends
			t.Fatal(err)
		}
		signed, _ := relayPrices("deliver", order)
		fee(50 * gwei)
		mine(t, ctx, client, 1)
		w = worker()
		if err := act(); err != nil && !errors.Is(err, errLostResponse) { // a higher price is stored and sent
			t.Fatal(err)
		}
		newer, earlier := relayPrices("deliver", order)
		if newer == signed || earlier != 1 {
			t.Fatalf("no replacement: %d", earlier)
		}
		// The network kept the first one.
		var action string
		var sealed []byte
		if err := db.QueryRow(ctx, "SELECT work_id::text,tx_cipher FROM ledger.relay_version WHERE tx_hash=$1", signed).Scan(&action, &sealed); err != nil {
			t.Fatal(err)
		}
		raw, err := relayCipher.Open(nil, nil, sealed, relayAAD(action, signed))
		if err != nil {
			t.Fatal(err)
		}
		var original types.Transaction
		if err := original.UnmarshalBinary(raw); err != nil {
			t.Fatal(err)
		}
		if err := client.Client().CallContext(ctx, nil, "anvil_dropTransaction", common.HexToHash(newer)); err != nil {
			t.Fatal(err)
		}
		fee(gwei)
		if err := client.SendTransaction(ctx, &original); err != nil {
			t.Fatal(err)
		}
		mine(t, ctx, client, 1)
		automine(true)
		settleActions(at(order, customer, "delivered"))
		if adopted, _ := relayPrices("deliver", order); adopted != signed {
			t.Fatalf("the record names %s instead of the included %s", adopted, signed)
		}
	})

	t.Run("ObservedThenGone", func(t *testing.T) {
		order := lockOrder(customer)
		pickup(order, "preparing", "ready", "collected")
		automine(false)
		defer automine(true)
		for range 2 { // signs, then sends
			if err := act(); err != nil && !errors.Is(err, errLostResponse) {
				t.Fatal(err)
			}
		}
		signed, _ := relayPrices("deliver", order)
		var snapshot string
		if err := client.Client().CallContext(ctx, &snapshot, "evm_snapshot"); err != nil {
			t.Fatal(err)
		}
		mine(t, ctx, client, 1)
		if err := act(); err != nil { // sees the receipt in a block that is not final
			t.Fatal(err)
		}
		var state string
		if err := db.QueryRow(ctx, "SELECT state FROM ledger.relay_attempt WHERE tx_hash=$1", signed).Scan(&state); err != nil || state != "observed" {
			t.Fatalf("relay is %s: %v", state, err)
		}
		var undone bool
		if err := client.Client().CallContext(ctx, &undone, "evm_revert", snapshot); err != nil || !undone {
			t.Fatal(err)
		}
		_ = client.Client().CallContext(ctx, nil, "anvil_dropTransaction", common.HexToHash(signed))
		if err := act(); err != nil && !errors.Is(err, errLostResponse) {
			t.Fatal(err)
		}
		again, _ := relayPrices("deliver", order)
		if _, _, err := client.TransactionByHash(ctx, common.HexToHash(again)); err != nil {
			t.Fatalf("the transaction of a vanished block was not sent again: %v", err)
		}
		automine(true)
		settleActions(at(order, customer, "delivered"))
	})

	t.Run("DroppedLockUnderAWaitingAction", func(t *testing.T) {
		waiting := lockOrder(customer)
		pickup(waiting, "preparing", "ready", "collected")
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=25 WHERE branch_id=$1", base.Branch)
		dropped := place(customer)
		fixtureExec(t, db, "UPDATE ledger.issuance_profile SET quote_lifetime_seconds=120 WHERE branch_id=$1", base.Branch)
		inTime(dropped)
		cmd, body := command(dropped, phone)
		call(terminal, cmd, body)
		automine(false)
		defer automine(true)
		for range 2 {
			if err := step(dropped); err != nil && !errors.Is(err, errLostResponse) {
				t.Fatal(err)
			}
		}
		for range 2 { // the delivery is signed and sent one nonce higher
			if err := act(); err != nil && !errors.Is(err, errLostResponse) {
				t.Fatal(err)
			}
		}
		lockHash, _ := relay(dropped)
		if err := client.Client().CallContext(ctx, nil, "anvil_dropTransaction", common.HexToHash(*lockHash)); err != nil {
			t.Fatal(err)
		}
		automine(true)
		time.Sleep(time.Until(dropped.Order.Expires) + time.Second)
		reach(dropped.Order.Expires.Unix() + 1)
		view.prove(t, dropped)
		if err := step(dropped); err != nil {
			t.Fatal(err)
		}
		stage(dropped, customer, "closed", "failed", "unpaid")
		next := lockOrder(customer) // takes the free nonce below the waiting delivery
		settleActions(at(waiting, customer, "delivered"))
		stage(next, customer, "paid", "locked", "finalized")
	})

	var oldReturn func(*testing.T)
	t.Run("OwnBoardPaysAndReturns", func(t *testing.T) {
		board, home := payers[newcomer], fixtureSigner(151).Address()
		if tx, err := escrow.token.Mint(escrow.operator, board.Address(), big.NewInt(20_000_000)); err != nil {
			t.Fatal(err)
		} else if _, err := bind.WaitMined(ctx, client, tx); err != nil {
			t.Fatal(err)
		}
		account := func(identity int, operation string, body map[string]string, write bool) service.Request {
			return newPilotCommand(t, db, base.Branch, actors[identity], operation, body, write)
		}
		enroll := func(identity int, payer common.Address, signer fixtureSigner, subject string) (service.Request, map[string]any) {
			cmd := account(identity, "payer.enroll", map[string]string{"payer": lower(payer), "generation": "1"}, true)
			challenge := orderescrow.EnrollChallenge(base.Tenant, base.Branch, subject, payer, 1, cmd.IdempotencyKey)
			signature, _ := signer.SignDigest(ctx, orderescrow.AccountProofDigest(payer, challenge))
			return cmd, map[string]any{"approvalSignature": rawHex(signature)}
		}
		read := func(identity int, operation string) map[string]any {
			raw := post(identity, "/v2/commerce", map[string]any{"command": account(identity, operation, map[string]string{}, false)}, 200)
			validateEscrowWire(t, ctx, raw)
			var out map[string]any
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			return out
		}
		if read(newcomer, "payer.status")["payer"] != nil || read(newcomer, "wallet.return-status")["walletReturn"] != nil {
			t.Fatal("an empty account shows something")
		}
		cmd, body := enroll(newcomer, board.Address(), fixtureSigner(152), actors[newcomer])
		refused(newcomer, cmd, body, 400, service.Mismatch) // another key
		cmd, body = enroll(newcomer, board.Address(), board, actors[customer])
		refused(newcomer, cmd, body, 400, service.Mismatch) // a proof made for another customer
		cmd, body = enroll(newcomer, board.Address(), board, actors[newcomer])
		foreign := cmd
		foreign.Object = actors[customer]
		refused(newcomer, foreign, body, 404, "")
		refused(terminal, cmd, body, 404, "")
		replayed := cmd
		replayed.IdempotencyKey = fixtureUUID(t, db)
		refused(newcomer, replayed, body, 400, service.Mismatch) // the proof belongs to one request
		for range 2 {
			cmd.RequestID = fixtureUUID(t, db)
			call(newcomer, cmd, body)
		}
		if payer := read(newcomer, "payer.status")["payer"].(map[string]any); payer["payer"] != lower(board.Address()) || payer["generation"] != "1" {
			t.Fatalf("enrolled account: %v", payer)
		}

		transfer := func(amount int64, valid time.Duration, signer fixtureSigner) (service.Request, map[string]any) {
			nonce := crypto.Keccak256Hash([]byte(fixtureUUID(t, db)))
			before := time.Now().Add(valid).Unix()
			digest, err := orderescrow.ReturnDigest(target.Asset, board.Address(), home, big.NewInt(amount), uint64(before), nonce)
			if err != nil {
				t.Fatal(err)
			}
			signature, _ := signer.SignDigest(ctx, digest)
			return account(newcomer, "wallet.return", map[string]string{"payer": lower(board.Address()), "to": lower(home), "amount": fmt.Sprint(amount),
				"validBefore": fmt.Sprint(before), "nonce": rawHex(nonce[:])}, true), map[string]any{"approvalSignature": rawHex(signature)}
		}
		order := lockOrder(newcomer)
		cmd, body = transfer(12_000_000, 5*time.Minute, board)
		refused(newcomer, cmd, body, 409, service.CheckoutConflict) // an order is open
		other, otherBody := enroll(newcomer, fixtureSigner(153).Address(), 153, actors[newcomer])
		refused(newcomer, other, otherBody, 409, service.CheckoutConflict)
		answerCmd, answerBody := answer(order, "receipt", deliver(order, newcomer), phoneKey)
		call(newcomer, answerCmd, answerBody)
		settleActions(at(order, newcomer, "paid"))

		cmd, body = transfer(12_000_000, 5*time.Minute, fixtureSigner(152))
		refused(newcomer, cmd, body, 400, service.Mismatch)
		cmd, body = transfer(12_000_000, 20*time.Minute, board)
		refused(newcomer, cmd, body, 400, service.BadRequest)
		cmd, body = transfer(12_000_000, 5*time.Minute, board)
		refused(customer, cmd, body, 404, "")
		for range 2 {
			cmd.RequestID = fixtureUUID(t, db)
			call(newcomer, cmd, body)
		}
		if state := read(newcomer, "wallet.return-status")["walletReturn"].(map[string]any)["state"]; state != "queued" {
			t.Fatalf("return is %v", state)
		}
		second, secondBody := transfer(1, 5*time.Minute, board)
		refused(newcomer, second, secondBody, 409, service.Conflict)
		blocked, claim := reserve(newcomer)
		opened := call(terminal, newPilotCommand(t, db, blocked.Branch, blocked.Sale, "order.status", map[string]string{}, false), nil).Order
		refused(terminal, newPilotCommand(t, db, blocked.Branch, blocked.Sale, "quote.issue", map[string]string{
			"claimDigest": claim, "orderRevision": strconv.FormatInt(opened.Revision, 10)}, true), nil, 409, service.CheckoutConflict)
		returned := func(state string) func() bool {
			return func() bool {
				return read(newcomer, "wallet.return-status")["walletReturn"].(map[string]any)["state"] == state
			}
		}
		settleActions(returned("final"))
		final := read(newcomer, "wallet.return-status")["walletReturn"].(map[string]any)
		receipt, err := client.TransactionReceipt(ctx, common.HexToHash(final["txHash"].(string)))
		if err != nil || receipt.Status != 1 || balance(home).Int64() != 12_000_000 || balance(board.Address()).Sign() != 0 {
			t.Fatalf("return: %v home %s board %s", err, balance(home), balance(board.Address()))
		}
		call(newcomer, cmd, body) // the accepted request still answers

		// A return that never ran fails only after its deadline is final, and gives its nonce back.
		cmd, body = transfer(5, 4*time.Second, board) // no balance left, so it is never sent
		call(newcomer, cmd, body)
		time.Sleep(5 * time.Second)
		if !returned("queued")() {
			t.Fatal("an unproven return changed its state")
		}
		reach(time.Now().Unix() + 1)
		mine(t, ctx, client, 4)
		settleActions(returned("failed"))
		// The same after signing: the transaction expires unsent.
		if tx, err := escrow.token.Mint(escrow.operator, board.Address(), big.NewInt(20_000_000)); err != nil {
			t.Fatal(err)
		} else if _, err := bind.WaitMined(ctx, client, tx); err != nil {
			t.Fatal(err)
		}
		cmd, body = transfer(20_000_000, 8*time.Second, board)
		call(newcomer, cmd, body)
		signed := 0
		for pass := 0; pass < 4 && signed == 0; pass++ { // an earlier return may still be closing its job
			if err := act(); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(ctx, `SELECT count(*) FROM ledger.relay_attempt r JOIN ledger.escrow_action a ON a.id=r.work_id
				WHERE a.kind='return' AND a.state='queued' AND a.branch_id=$1`, base.Branch).Scan(&signed); err != nil {
				t.Fatal(err)
			}
		}
		if signed != 1 {
			t.Fatal("the return was not signed")
		}
		time.Sleep(9 * time.Second)
		reach(time.Now().Unix() + 1)
		mine(t, ctx, client, 4)
		settleActions(returned("failed"))
		// Somebody else lands the same authorization; the last subtest lets the worker see it much later.
		cmd, body = transfer(1_000_000, 9*time.Minute, board)
		signature := common.FromHex(body["approvalSignature"].(string))
		before, _ := new(big.Int).SetString(cmd.Body["validBefore"], 10)
		tx, err := escrow.token.TransferWithAuthorization(escrow.operator, board.Address(), home, big.NewInt(1_000_000), new(big.Int), before,
			common.HexToHash(cmd.Body["nonce"]), signature[64], [32]byte(signature[:32]), [32]byte(signature[32:64]))
		if err != nil {
			t.Fatal(err)
		}
		if landed, err := bind.WaitMined(ctx, client, tx); err != nil || landed.Status != 1 {
			t.Fatal(err)
		}
		oldReturn = func(t *testing.T) {
			fixtureExec(t, db, "UPDATE ledger.escrow_action SET created_at=now()-interval '2 days' WHERE kind='return' AND branch_id=$1", base.Branch)
			call(newcomer, account(newcomer, "wallet.return", cmd.Body, true), body)
			mine(t, ctx, client, 20_010)
			for pass := 0; pass < 5 && !returned("final")(); pass++ {
				if err := act(); err != nil {
					t.Fatal(err)
				}
			}
			if got := read(newcomer, "wallet.return-status")["walletReturn"].(map[string]any); got["state"] != "final" || got["txHash"] != tx.Hash().Hex() {
				t.Fatalf("an old return stays %v", got)
			}
			var open bool
			if err := db.QueryRow(ctx, "SELECT ledger.payer_has_open_return($1)", board.Address().Bytes()).Scan(&open); err != nil || open {
				t.Fatalf("the account is still being emptied: %v %v", open, err)
			}
		}
	})

	t.Run("BoardSendsToAnyAddress", func(t *testing.T) {
		board, friend := payers[newcomer], common.HexToAddress("0x00000000000000000000000000000000000f0001")
		held := func() commerce.WalletSends {
			t.Helper()
			raw := post(newcomer, "/v2/commerce", map[string]any{"command": newPilotCommand(t, db, base.Branch, actors[newcomer], "wallet.send-status", map[string]string{}, false)}, 200)
			validateEscrowWire(t, ctx, raw)
			var out struct{ WalletSends commerce.WalletSends }
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			return out.WalletSends
		}
		signed := func(operation string, to common.Address, amount int64, valid time.Duration, signer fixtureSigner, nonce common.Hash) (service.Request, map[string]any) {
			t.Helper()
			before := time.Now().Add(valid).Unix()
			digest, err := orderescrow.ReturnDigest(target.Asset, board.Address(), to, big.NewInt(amount), uint64(before), nonce)
			if err != nil {
				t.Fatal(err)
			}
			signature, _ := signer.SignDigest(ctx, digest)
			body := map[string]string{"payer": lower(board.Address()), "to": lower(to), "amount": fmt.Sprint(amount), "validBefore": fmt.Sprint(before), "nonce": rawHex(nonce[:])}
			if operation == "wallet.send" {
				body["token"] = lower(target.Asset)
			}
			return newPilotCommand(t, db, base.Branch, actors[newcomer], operation, body, true), map[string]any{"approvalSignature": rawHex(signature)}
		}
		fresh := func() common.Hash { return crypto.Keccak256Hash([]byte(fixtureUUID(t, db))) }
		if tx, err := escrow.token.Mint(escrow.operator, board.Address(), big.NewInt(20_000_000)); err != nil {
			t.Fatal(err)
		} else if _, err := bind.WaitMined(ctx, client, tx); err != nil {
			t.Fatal(err)
		}
		if got := held(); got.PerDay != 3 || got.Remaining != 3 || len(got.Sends) != 0 {
			t.Fatalf("no transfer yet: %+v", got)
		}
		// The returns of the subtest before do not count against the ones refused here.
		fixtureExec(t, db, "UPDATE ledger.escrow_action SET created_at=now()-interval '2 days' WHERE kind='return' AND branch_id=$1", base.Branch)
		// What the board did not sign, or the store does not relay.
		cmd, body := signed("wallet.send", friend, 4_000_000, 5*time.Minute, fixtureSigner(152), fresh())
		refused(newcomer, cmd, body, 400, service.Mismatch)
		cmd, body = signed("wallet.send", friend, 4_000_000, 20*time.Minute, board, fresh())
		refused(newcomer, cmd, body, 400, service.BadRequest)
		cmd, body = signed("wallet.send", target.Asset, 4_000_000, 5*time.Minute, board, fresh())
		refused(newcomer, cmd, body, 400, service.BadRequest)
		cmd, body = signed("wallet.send", friend, 4_000_000, 5*time.Minute, board, fresh())
		other := cmd
		other.Body = maps.Clone(cmd.Body)
		other.Body["token"] = lower(friend)
		refused(newcomer, other, body, 400, service.Capability)
		changed := cmd
		changed.Body = maps.Clone(cmd.Body)
		changed.Body["to"] = lower(fixtureSigner(152).Address())
		refused(newcomer, changed, body, 400, service.Mismatch)
		refused(customer, cmd, body, 404, "")
		offered := targets[base.Branch]
		off := offered
		off.SendsPerDay = 0
		targets[base.Branch] = off
		refused(newcomer, cmd, body, 400, service.Capability)
		refused(newcomer, newPilotCommand(t, db, base.Branch, actors[newcomer], "wallet.send-status", map[string]string{}, false), nil, 400, service.Capability)
		targets[base.Branch] = offered

		spent := fresh()
		cmd, body = signed("wallet.send", friend, 4_000_000, 5*time.Minute, board, spent)
		for range 2 { // a repeated request is the same transfer
			cmd.RequestID = fixtureUUID(t, db)
			call(newcomer, cmd, body)
		}
		second, secondBody := signed("wallet.send", friend, 1, 5*time.Minute, board, fresh())
		refused(newcomer, second, secondBody, 409, service.Conflict)
		home, homeBody := signed("wallet.return", fixtureSigner(151).Address(), 1, 5*time.Minute, board, fresh())
		refused(newcomer, home, homeBody, 409, service.CheckoutConflict)
		settleActions(func() bool { got := held(); return len(got.Sends) == 1 && got.Sends[0].State == "final" })
		got := held()
		receipt, err := client.TransactionReceipt(ctx, common.HexToHash(got.Sends[0].TxHash))
		if err != nil || receipt.Status != 1 || balance(friend).Int64() != 4_000_000 || got.Remaining != 2 || got.Sends[0].To != lower(friend) ||
			got.Sends[0].Nonce != rawHex(spent[:]) {
			t.Fatalf("transfer: %v %s %+v", err, balance(friend), got)
		}
		// One authorization is one transfer, whatever it is called.
		home, homeBody = signed("wallet.return", fixtureSigner(151).Address(), 1, 5*time.Minute, board, spent)
		refused(newcomer, home, homeBody, 409, service.Conflict)
		again, againBody := signed("wallet.send", friend, 1, 5*time.Minute, board, spent)
		refused(newcomer, again, againBody, 409, service.Conflict)
		// More than the account holds: never sent, and failed once its deadline is final.
		cmd, body = signed("wallet.send", friend, 900_000_000, 4*time.Second, board, fresh())
		call(newcomer, cmd, body)
		time.Sleep(5 * time.Second)
		reach(time.Now().Unix() + 1)
		mine(t, ctx, client, 4)
		settleActions(func() bool { return held().Sends[0].State == "failed" })
		// The daily number holds for failed transfers too, and one day later it is free again.
		cmd, body = signed("wallet.send", friend, 1_000_000, 5*time.Minute, board, fresh())
		call(newcomer, cmd, body)
		settleActions(func() bool { return held().Sends[0].State == "final" })
		cmd, body = signed("wallet.send", friend, 1_000_000, 5*time.Minute, board, fresh())
		refused(newcomer, cmd, body, 503, service.Busy)
		if held().Remaining != 0 {
			t.Fatal("a transfer over the limit is offered")
		}
		fixtureExec(t, db, "UPDATE ledger.escrow_action SET created_at=now()-interval '2 days' WHERE kind='send'")
		call(newcomer, cmd, body)
		settleActions(func() bool { return held().Sends[0].State == "final" })
		if balance(friend).Int64() != 6_000_000 {
			t.Fatalf("friend holds %s", balance(friend))
		}
	})

	t.Run("TestTokenFaucet", func(t *testing.T) {
		account := faucet.Account
		claim := func() service.Request {
			return newPilotCommand(t, db, base.Branch, actors[newcomer], "faucet.claim", map[string]string{"token": lower(target.Asset)}, true)
		}
		state := func(identity int) commerce.FaucetToken {
			t.Helper()
			raw := post(identity, "/v2/commerce", map[string]any{"command": newPilotCommand(t, db, base.Branch, actors[identity], "faucet.status", map[string]string{}, false)}, 200)
			validateEscrowWire(t, ctx, raw)
			var out struct{ Faucet commerce.FaucetStatus }
			if err := json.Unmarshal(raw, &out); err != nil || len(out.Faucet.Tokens) != 1 || out.Faucet.Payer != lower(payers[identity].Address()) {
				t.Fatalf("faucet: %s %v", raw, err)
			}
			return out.Faucet.Tokens[0]
		}
		want := func(identity int, stage string, remaining int32, last string) {
			t.Helper()
			got := state(identity)
			if got.State != stage || got.Remaining != remaining || (got.Claim == nil) != (last == "") || (got.Claim != nil && got.Claim.State != last) {
				t.Fatalf("faucet shows %+v %+v", got, got.Claim)
			}
		}
		drain := func() { // earlier jobs of this account and everyone else's
			settleActions(func() bool {
				var open int
				_ = db.QueryRow(ctx, `SELECT count(*) FROM ledger.settlement_work w JOIN ledger.escrow_action a ON a.id=w.action_id
					WHERE a.branch_id=$1 AND NOT w.finished AND a.kind='faucet'`, base.Branch).Scan(&open)
				return open == 0
			})
		}
		// Finished returns still close their jobs; let them go first.
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET lease_until=NULL WHERE NOT finished")
		want(newcomer, "empty", 2, "")
		refused(newcomer, claim(), nil, 409, service.OutOfStock) // neither tokens nor gas money
		if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", account, "0x56BC75E2D63100000"); err != nil {
			t.Fatal(err)
		}
		mint := func(amount int64) {
			t.Helper()
			tx, err := escrow.token.Mint(escrow.operator, account, big.NewInt(amount))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := bind.WaitMined(ctx, client, tx); err != nil {
				t.Fatal(err)
			}
		}
		mint(4_999_999)
		want(newcomer, "empty", 2, "")
		mint(15_000_001)
		want(newcomer, "ready", 2, "")
		// Only the customer itself, only a listed token, and never an account that the request names.
		refused(terminal, claim(), nil, 404, "")
		foreign := claim()
		foreign.Object = actors[customer]
		refused(newcomer, foreign, nil, 404, "")
		unlisted := claim()
		unlisted.Body = map[string]string{"token": lower(common.HexToAddress("0xbad"))}
		refused(newcomer, unlisted, nil, 400, service.Capability)
		named := claim()
		named.Body = map[string]string{"token": lower(target.Asset), "to": lower(account)}
		refused(newcomer, named, nil, 400, service.BadRequest)
		board := payers[newcomer].Address()
		before := balance(board)
		first := claim()
		for range 2 {
			first.RequestID = fixtureUUID(t, db)
			call(newcomer, first, nil)
		}
		want(newcomer, "open", 1, "queued")
		refused(newcomer, claim(), nil, 409, service.Conflict)
		// The answer to the first send is lost and the process starts again: the same bytes are found or sent again.
		chain.sends = 0
		signed := func() (hash *string) {
			t.Helper()
			if err := db.QueryRow(ctx, `SELECT (SELECT r.tx_hash FROM ledger.relay_attempt r JOIN ledger.escrow_action a ON a.id=r.work_id
				WHERE a.kind='faucet' AND a.branch_id=$1 AND a.state='queued')`, base.Branch).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			return
		}
		for pass := 0; pass < 8 && signed() == nil; pass++ {
			_ = act()
		}
		kept := signed()
		if kept == nil || balance(board).Cmp(before) != 0 {
			t.Fatal("the claim was not stored before it was sent")
		}
		w = worker()
		drain()
		got := state(newcomer)
		if got.Claim == nil || got.Claim.State != "final" || got.Claim.TxHash != *kept || got.Claim.To != lower(board) ||
			new(big.Int).Sub(balance(board), before).Int64() != 5_000_000 {
			t.Fatalf("claim: %+v paid %s", got.Claim, new(big.Int).Sub(balance(board), before))
		}
		sent, err := client.NonceAt(ctx, account, nil)
		if err != nil || sent != 1 {
			t.Fatalf("the faucet sent %d transactions: %v", sent, err)
		}
		call(newcomer, first, nil) // the accepted request still answers, and pays nothing
		call(newcomer, claim(), nil)
		drain()
		want(newcomer, "limit", 0, "final")
		if at := state(newcomer).AvailableAt; at < time.Now().Add(23*time.Hour).Unix() || at > time.Now().Add(25*time.Hour).Unix() {
			t.Fatalf("next claim at %d", at)
		}
		refused(newcomer, claim(), nil, 503, service.Busy)
		// 10 of 12 are gone today; another customer's claim does not fit either.
		want(customer, "limit", 2, "")
		total := newPilotCommand(t, db, base.Branch, actors[customer], "faucet.claim", map[string]string{"token": lower(target.Asset)}, true)
		refused(customer, total, nil, 503, service.Busy)
		// The nonce of a signed claim is taken by another transaction of the same key. Nothing proves that
		// the claim did not run, so it is never signed again and keeps its place in the limit.
		fixtureExec(t, db, "UPDATE ledger.faucet_claim SET claimed_at=now()-interval '2 days'")
		before = balance(board)
		call(newcomer, claim(), nil)
		for pass := 0; pass < 8 && signed() == nil; pass++ {
			_ = act()
		}
		key, _ := crypto.ToECDSA(append(make([]byte, 31), byte(160)))
		price, err := client.SuggestGasPrice(ctx)
		if err != nil {
			t.Fatal(err)
		}
		rival, err := types.SignNewTx(key, types.LatestSignerForChainID(big.NewInt(8283)), &types.LegacyTx{Nonce: 2, To: &account, Gas: 21_000, GasPrice: price.Lsh(price, 1)})
		if err != nil {
			t.Fatal(err)
		}
		if err := client.SendTransaction(ctx, rival); err != nil {
			t.Fatal(err)
		}
		mine(t, ctx, client, 4)
		for range 4 {
			if err := act(); err != nil && !errors.Is(err, errFaucetSpent) && !errors.Is(err, errLostResponse) {
				t.Fatal(err)
			}
		}
		if got := state(newcomer); got.State != "open" {
			t.Fatalf("a claim with a spent nonce ended early: %+v", got.Claim)
		}
		fixtureExec(t, db, "UPDATE ledger.escrow_action SET created_at=now()-interval '2 hours' WHERE kind='faucet' AND state='queued'")
		drain()
		want(newcomer, "ready", 1, "unknown")
		if balance(board).Cmp(before) != 0 {
			t.Fatal("a claim was signed for a second nonce")
		}
		// A claim that is never signed ends and gives its place back.
		call(newcomer, claim(), nil)
		fixtureExec(t, db, "UPDATE ledger.escrow_action SET created_at=now()-interval '2 hours' WHERE kind='faucet' AND state='queued'")
		drain()
		want(newcomer, "ready", 1, "failed")
		// A withdrawn list turns the faucet off.
		tokens := faucet.Tokens
		faucet.Tokens = map[string][]FaucetToken{}
		refused(newcomer, claim(), nil, 400, service.Capability)
		faucet.Tokens = tokens
	})

	t.Run("OrdersForLater", func(t *testing.T) {
		claimable, contested, late, undelivered, missed = lockOrder(customer), lockOrder(customer), lockOrder(customer), lockOrder(customer), lockOrder(customer)
		deliver(claimable, customer)
		cmd, body := answer(contested, "dispute", deliver(contested, customer), phoneKey)
		call(customer, cmd, body)
		settleActions(at(contested, customer, "disputed"))
		fixtureExec(t, db, "UPDATE ledger.payer_enrollment SET payer_role=1 WHERE branch_id=$1", base.Branch)
	})

	warp := func(d time.Duration) {
		t.Helper()
		head, err := client.HeaderByNumber(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		forceTime(t, ctx, client, int64(head.Time)+int64(d/time.Second))
	}

	t.Run("ClaimBatch", func(t *testing.T) {
		warp(49 * time.Hour)
		deliver(late, customer) // its window has just opened
		key := fixtureUUID(t, db)
		sales := []string{contested.Sale, claimable.Sale, late.Sale}
		request := map[string]any{"saleIds": sales, "idempotencyKey": key}
		batch := financeCall(finance1, "escrow-claim", request, 200)
		if again := financeCall(finance1, "escrow-claim", request, 200); again["batchId"] != batch["batchId"] || batch["state"] != "queued" || batch["requested"] != "24000000" {
			t.Fatalf("claim: %v", batch)
		}
		financeCall(finance1, "escrow-claim", map[string]any{"saleIds": sales[:1], "idempotencyKey": key}, 409)
		financeCall(finance1, "escrow-claim", map[string]any{"saleIds": sales[:1], "idempotencyKey": fixtureUUID(t, db)}, 409) // all in the first batch
		mixed := financeCall(finance1, "escrow-claim", map[string]any{"saleIds": []string{sales[0], undelivered.Sale}, "idempotencyKey": fixtureUUID(t, db)}, 200)
		if row := mixed["skipped"].([]any)[0].(map[string]any); row["reason"] != "in-another-batch" || row["saleId"] != sales[0] || row["pickupNo"] == nil ||
			len(mixed["receiptScope"].(string)) != 64 {
			t.Fatalf("second batch: %v", mixed)
		}
		financeCall(finance1, "escrow-claim", map[string]any{"saleIds": []string{sales[0], sales[0]}, "idempotencyKey": fixtureUUID(t, db)}, 400)
		financeCall(finance1, "escrow-claim", map[string]any{"saleIds": []string{fixtureUUID(t, db)}, "idempotencyKey": fixtureUUID(t, db)}, 404)
		financeCall(finance2, "escrow-claim", map[string]any{"saleIds": sales, "idempotencyKey": fixtureUUID(t, db)}, 404)
		settleActions(at(claimable, customer, "paid"))
		settleActions(func() bool {
			items := financeCall(finance1, "escrow-claims", map[string]any{}, 200)["items"].([]any)
			mixed, batch = items[0].(map[string]any), items[1].(map[string]any)
			return batch["state"] == "final" && mixed["state"] == "final"
		})
		skipped, _ := json.Marshal(batch["skipped"])
		if notDelivered, _ := json.Marshal(mixed["skipped"]); mixed["paid"] != "0" || !bytes.Contains(notDelivered, []byte(`"not-delivered"`)) {
			t.Fatalf("second batch result: %v", mixed)
		}
		if batch["paid"] != escrowAmount || !bytes.Contains(skipped, []byte(`"saleId":"`+contested.Sale)) ||
			!bytes.Contains(skipped, []byte(`"saleId":"`+late.Sale)) || len(batch["skipped"].([]any)) != 2 ||
			!bytes.Contains(skipped, []byte(`"disputed"`)) || !bytes.Contains(skipped, []byte(`"window-open"`)) {
			t.Fatalf("claim result: %v %s", batch, skipped)
		}
		listed := financeCall(finance1, "escrow-orders", map[string]any{"status": "disputed"}, 200)["items"].([]any)
		if row := listed[0].(map[string]any); len(listed) != 1 || row["saleId"] != contested.Sale || len(row["items"].([]any)) != 1 {
			t.Fatalf("disputed orders: %v", listed)
		}
		byID := financeCall(finance1, "order", map[string]any{"saleId": claimable.Sale}, 200)
		if one := byID["escrow"].(map[string]any); one["status"] != "paid" || one["stage"] != nil || byID["fundsStatus"] != "confirmed" {
			t.Fatalf("order by id: %v", byID)
		}
		receipt, err := client.TransactionReceipt(ctx, common.HexToHash(batch["transaction"].(string)))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("claimBatch of 3 used %d gas", receipt.GasUsed)
		if paid := stage(claimable, customer, "paid", "paid", "finalized"); paid.Escrow.Payout != "claim" {
			t.Fatalf("claimed order: %+v", paid.Escrow)
		}
		stage(late, customer, "paid", "delivered", "finalized")
	})

	t.Run("UndeliveredAndMissedDelivery", func(t *testing.T) {
		warp(7 * 24 * time.Hour)
		// The server clock cannot be moved, so the stored deadline is.
		fixtureExec(t, db, "UPDATE ledger.order_escrow SET deliver_by=extract(epoch FROM now())::bigint-1 WHERE sale_id IN ($1,$2)", undelivered.Sale, missed.Sale)
		pickup(missed, "preparing", "ready", "collected")
		settleActions(func() bool {
			var failed bool
			_ = db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM ledger.escrow_action WHERE sale_id=$1 AND kind='deliver' AND state='failed')", missed.Sale).Scan(&failed)
			return failed
		})
		shown := false
		for _, item := range financeCall(finance1, "escrow-orders", map[string]any{"status": "held"}, 200)["items"].([]any) {
			row := item.(map[string]any)
			shown = shown || (row["saleId"] == missed.Sale && row["deliveryFailed"] == true)
		}
		if !shown {
			t.Fatal("a delivery that can no longer be recorded is not shown")
		}
		fixtureExec(t, db, "UPDATE ledger.order_escrow SET check_at=now() WHERE sale_id=$1", undelivered.Sale)
		if moved, err := w.stepFollow(ctx, []string{base.Branch}); !moved || err != nil {
			t.Fatal(moved, err)
		}
		for _, order := range []seededOrder{undelivered, missed} {
			settleActions(at(order, customer, "refunded"))
			if done := stage(order, customer, "paid", "refunded", "finalized"); done.Escrow.Refund != "undelivered" || done.Escrow.ClaimableAt != nil {
				t.Fatalf("undelivered order: %+v", done.Escrow)
			}
		}
	})

	t.Run("ExpiredDispute", func(t *testing.T) {
		warp(15 * 24 * time.Hour)
		// TestPostgresEscrowDeadlines covers when the server queues this by itself.
		fixtureExec(t, db, "SELECT ledger.queue_escrow_action($1,'refund-dispute',$2,NULL,NULL,NULL,NULL)", base.Branch, contested.Sale)
		settleActions(at(contested, customer, "refunded"))
		if done := stage(contested, customer, "paid", "refunded", "finalized"); done.Escrow.Refund != "dispute-expired" {
			t.Fatalf("expired dispute: %+v", done.Escrow)
		}
	})

	t.Run("ClaimSentByAnotherAccount", func(t *testing.T) {
		outside, _ := bind.NewKeyedTransactorWithChainID(mustKey(t, 130), big.NewInt(8283))
		tx, err := escrow.contract.Claim(outside, late.Quote.Args.MerchantId, late.Quote.Args.OrderId)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bind.WaitMined(ctx, client, tx); err != nil {
			t.Fatal(err)
		}
		view.sync(tx.Hash())
		fixtureExec(t, db, "UPDATE ledger.order_escrow SET check_at=now()+interval '1 hour' WHERE sale_id<>$1", late.Sale)
		fixtureExec(t, db, "UPDATE ledger.order_escrow SET check_at=now() WHERE sale_id=$1", late.Sale)
		if moved, err := w.stepFollow(ctx, []string{base.Branch}); !moved || err != nil {
			t.Fatal(moved, err)
		}
		if done := stage(late, customer, "paid", "paid", "finalized"); done.Escrow.Payout != "claim" {
			t.Fatalf("outside claim: %+v", done.Escrow)
		}
	})

	t.Run("DayClose", func(t *testing.T) {
		settleActions(func() bool {
			var open int
			_ = db.QueryRow(ctx, "SELECT count(*) FROM ledger.settlement_work w JOIN ledger.escrow_action a ON a.id=w.action_id WHERE a.branch_id=$1 AND NOT w.finished", base.Branch).Scan(&open)
			return open == 0
		})
		for range 30 { // every paid order has reached its kitchen before a day closes
			if moved, _ := dispatch.Step(ctx); !moved {
				break
			}
		}
		var day, gross, locked, returned string
		if err := db.QueryRow(ctx, `SELECT j.business_day::text FROM ledger.journal j WHERE j.branch_id=$1 GROUP BY 1 ORDER BY count(*) DESC LIMIT 1`, base.Branch).Scan(&day); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `SELECT coalesce(sum(amount) FILTER (WHERE credit_account='sales'),0)::text,
			coalesce(sum(amount) FILTER (WHERE credit_account='order_escrow' AND NOT EXISTS (SELECT FROM ledger.escrow_journal s WHERE s.evidence_id=x.evidence_id AND s.credit_account='sales')),0)::text,
			(SELECT coalesce(sum(amount),0)::text FROM ledger.journal WHERE branch_id=$1 AND business_day=$2::text::date)
			FROM ledger.escrow_journal x WHERE branch_id=$1 AND business_day=$2::text::date`, base.Branch, day).Scan(&gross, &returned, &locked); err != nil {
			t.Fatal(err)
		}
		asset := lower(target.Asset)
		var report dayReport
		if err := json.Unmarshal(post(finance1, "/v2/finance/report", map[string]any{"branchId": base.Branch, "businessDay": day, "asset": asset}, 200), &report); err != nil ||
			report.Gross != gross || report.Net != gross || report.Refunds != "0" || report.EscrowLocked != locked || report.EscrowReturned != returned || gross == "0" || returned == "0" {
			t.Fatalf("day report %+v, booked %s/%s/%s: %v", report, gross, locked, returned, err)
		}
		csv := string(post(finance1, "/v2/finance/report-export", map[string]any{"branchId": base.Branch, "businessDay": day, "asset": asset, "idempotencyKey": fixtureUUID(t, db)}, 200))
		if !strings.Contains(csv, "escrow_locked_units,escrow_returned_units") || !strings.Contains(csv, ","+locked+","+returned) {
			t.Fatalf("export: %s", csv)
		}
		waitUntil(t, func() bool {
			raw := post(finance1, "/v2/finance/reconciliation", map[string]any{"branchId": base.Branch, "businessDay": day, "asset": asset}, 200)
			return bytes.Contains(raw, []byte(`"status":"matched"`))
		})
		closed := financeCall(finance1, "report-close", map[string]any{"businessDay": day, "asset": asset, "idempotencyKey": fixtureUUID(t, db)}, 200)
		if closed["revision"] != float64(1) {
			t.Fatalf("close: %v", closed)
		}
		var unbalanced int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM (SELECT sale_id FROM ledger.posting WHERE branch_id=$1 GROUP BY sale_id
			HAVING sum(CASE side WHEN 'debit' THEN amount ELSE -amount END)<>0) x`, base.Branch).Scan(&unbalanced); err != nil || unbalanced != 0 {
			t.Fatalf("unbalanced orders: %d %v", unbalanced, err)
		}
	})

	t.Run("ReturnUsedLongAgo", func(t *testing.T) {
		if oldReturn == nil {
			t.Skip("needs OwnBoardPaysAndReturns")
		}
		oldReturn(t)
	})

	t.Run("RentedPayerQuoteRefused", func(t *testing.T) {
		fixtureExec(t, db, "UPDATE ledger.payer_enrollment SET payer_role=2 WHERE branch_id=$1 AND actor_id=$2", base.Branch, actors[stranger])
		order, claim := reserve(stranger)
		opened := call(terminal, newPilotCommand(t, db, order.Branch, order.Sale, "order.status", map[string]string{}, false), nil).Order
		refused(terminal, newPilotCommand(t, db, order.Branch, order.Sale, "quote.issue", map[string]string{
			"claimDigest": claim, "orderRevision": strconv.FormatInt(opened.Revision, 10)}, true), nil, 400, service.Capability)
	})
}

func mustKey(t *testing.T, value byte) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.ToECDSA(append(make([]byte, 31), value))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// boardFor returns the wallet key of the order's enrolled payer.
func boardFor(order seededOrder, payers map[int]fixtureSigner) fixtureSigner {
	for _, payer := range payers {
		if payer.Address() == order.Quote.Args.Payer {
			return payer
		}
	}
	panic("order has no enrolled payer")
}

func stockOf(t *testing.T, ctx context.Context, db *pgxpool.Pool, branch string) (units int64) {
	t.Helper()
	if err := db.QueryRow(ctx, "SELECT available FROM ledger.stock WHERE branch_id=$1 AND sku='escrow-cafe'", branch).Scan(&units); err != nil {
		t.Fatal(err)
	}
	return units
}

// validateEscrowWire runs the check that the phone and the kiosk apply to the same JSON.
func validateEscrowWire(t *testing.T, ctx context.Context, raw []byte) {
	t.Helper()
	command := exec.CommandContext(ctx, "node", "--experimental-strip-types", "--input-type=module", "-e", `
import {readFileSync} from "node:fs";
import {validOrderEscrowStatus, validPayerEnrollment, validWalletReturn, validWalletSends} from "../../../../packages/protocol/ts/src/order-escrow.ts";
import {validFaucetStatus} from "../../../../packages/protocol/ts/src/faucet.ts";
const {order = {}, payer, walletReturn, walletSends, faucet} = JSON.parse(readFileSync(0, "utf8"));
if (faucet !== undefined && !validFaucetStatus(faucet)) { console.error(JSON.stringify(faucet)); process.exit(1); }
if (payer !== undefined && !validPayerEnrollment(payer)) { console.error(JSON.stringify(payer)); process.exit(1); }
if (walletReturn !== undefined && !validWalletReturn(walletReturn)) { console.error(JSON.stringify(walletReturn)); process.exit(1); }
if (walletSends !== undefined && !validWalletSends(walletSends)) { console.error(JSON.stringify(walletSends)); process.exit(1); }
if (order.escrow !== undefined && !validOrderEscrowStatus(order.escrow)) { console.error(JSON.stringify(order.escrow)); process.exit(1); }`)
	command.Stdin = bytes.NewReader(raw)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("clients refuse this escrow status: %v %s", err, output)
	}
}

// verifyEscrowAccounts checks that held money is neither income nor refundable by the store.
func verifyEscrowAccounts(t *testing.T, ctx context.Context, db *pgxpool.Pool, h FinanceHTTP, order seededOrder, actor string, post cafePost) {
	t.Helper()
	const finance = 1
	var debit, credit, day string
	var payments, pickups, events int
	if err := db.QueryRow(ctx, `SELECT debit_account,credit_account,business_day::text,
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),(SELECT count(*) FROM ledger.pickup WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$1) FROM ledger.journal WHERE sale_id=$1`, order.Sale).
		Scan(&debit, &credit, &day, &payments, &pickups, &events); err != nil ||
		debit != "order_escrow" || credit != "escrow_deposits" || payments != 1 || pickups != 1 || events != 1 {
		t.Fatalf("escrow posting %s/%s payments %d pickups %d events %d: %v", debit, credit, payments, pickups, events, err)
	}
	asset := lower(order.Target.Asset)
	var report dayReport
	if err := json.Unmarshal(post(finance, "/v2/finance/report", map[string]any{"branchId": order.Branch, "businessDay": day, "asset": asset}, 200), &report); err != nil ||
		report.Gross != "0" || report.Net != "0" || report.EscrowLocked == "" || report.EscrowReturned != "0" {
		t.Fatalf("held money reported as income: %+v %v", report, err)
	}
	post(finance, "/v2/finance/report-close", map[string]any{"branchId": order.Branch, "businessDay": day, "asset": asset, "idempotencyKey": fixtureUUID(t, db)}, 409)
	var detail struct {
		FundsStatus string `json:"fundsStatus"`
	}
	if err := json.Unmarshal(post(finance, "/v2/finance/order", map[string]any{"branchId": order.Branch, "saleId": order.Sale}, 200), &detail); err != nil || detail.FundsStatus != "held" {
		t.Fatalf("operator order shows %q: %v", detail.FundsStatus, err)
	}
	var refund struct {
		Available  string `json:"available"`
		CanRequest bool   `json:"canRequest"`
		Funds      string `json:"funds"`
	}
	if err := json.Unmarshal(post(finance, "/v2/finance/refund-status", map[string]any{"branchId": order.Branch, "saleId": order.Sale}, 200), &refund); err != nil ||
		refund.Available != "0" || refund.CanRequest || refund.Funds != "unknown" {
		t.Fatalf("refund console offers held money: %+v %v", refund, err)
	}
	post(finance, "/v2/finance/refund-reserve", map[string]any{"branchId": order.Branch, "saleId": order.Sale, "idempotencyKey": fixtureUUID(t, db),
		"amount": "1", "reason": "merchant_cancel"}, 409)
	var evidence struct {
		Status string
		Escrow struct{ Status string }
	}
	if err := json.Unmarshal(post(finance, "/v2/finance/payment-evidence", map[string]any{"branchId": order.Branch, "saleId": order.Sale}, 200), &evidence); err != nil ||
		evidence.Status != "verified" || evidence.Escrow.Status != "held" {
		t.Fatalf("payment evidence: %+v %v", evidence, err)
	}
	var refunds int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.refund WHERE sale_id=$1", order.Sale).Scan(&refunds); err != nil || refunds != 0 {
		t.Fatalf("refunds recorded against the escrow: %d %v", refunds, err)
	}
}

var errBrokenEvidence = errors.New("finalized fact outside the enrolled quote")

type brokenEvidence struct{ *EvidenceStore }

func (brokenEvidence) reconciliation(context.Context, *Store, work, Target, Quote) (bool, error) {
	return false, errBrokenEvidence
}
