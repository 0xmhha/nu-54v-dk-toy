package chainview

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	escrow "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/orderescrow"
	token "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc3009"
	signing "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAnvilOrderEscrowFacts(t *testing.T) {
	endpoint, database := os.Getenv("P07_CHAINVIEW_TEST_RPC"), os.Getenv("P07_CHAINVIEW_TEST_URL")
	if endpoint == "" || database == "" {
		t.Skip("isolated Anvil and PostgreSQL not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	call := func(result any, method string, args ...any) {
		t.Helper()
		if err := client.Client().CallContext(ctx, result, method, args...); err != nil {
			t.Fatal(err)
		}
	}
	account := func(value byte) (*bind.TransactOpts, func(common.Hash) []byte) {
		key, err := crypto.ToECDSA(append(make([]byte, 31), value))
		if err != nil {
			t.Fatal(err)
		}
		auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
		if err != nil {
			t.Fatal(err)
		}
		auth.Context, auth.GasLimit = ctx, 10_000_000
		return auth, func(hash common.Hash) []byte {
			sig, err := crypto.Sign(hash[:], key)
			if err != nil {
				t.Fatal(err)
			}
			sig[64] += 27
			return sig
		}
	}
	operator, _ := account(120) // governor, arbiter and gas payer
	store, signStore := account(121)
	board, signBoard := account(122)
	phone, signPhone := account(123)
	beneficiary, _ := account(124)
	call(nil, "anvil_setBalance", operator.From, "0x56bc75e2d63100000")
	mined := func(tx *types.Transaction, err error) *types.Receipt {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := bind.WaitMined(ctx, client, tx)
		if err != nil || receipt.Status != 1 {
			t.Fatalf("local transaction reverted: %v", err)
		}
		return receipt
	}
	asset, tx, err := deployArtifact(t, operator, client, "TestUSDC3009", operator.From)
	mined(tx, err)
	address, tx, err := deployArtifact(t, operator, client, "OrderEscrow", asset, operator.From, operator.From)
	deployment := mined(tx, err)
	contract, err := escrow.NewOrderescrow(address, client)
	if err != nil {
		t.Fatal(err)
	}
	usd, err := token.NewTestusdc3009(asset, client)
	if err != nil {
		t.Fatal(err)
	}
	merchant := common.Hash{31: 7}
	mined(contract.ConfigureMerchant(operator, merchant, store.From, beneficiary.From, 3600, 7200))
	mined(usd.Mint(operator, board.From, big.NewInt(1_000_000)))

	parsed, err := escrow.OrderescrowMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	offer := func(order byte) (escrow.PaymentTypesV2MerchantQuote, []byte, []byte) {
		t.Helper()
		head, err := client.HeaderByNumber(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		quote := escrow.PaymentTypesV2MerchantQuote{
			QuoteId: common.Hash{31: order}, TenantCommitment: common.Hash{31: 3}, BranchCommitment: common.Hash{31: 4},
			OrderId: common.Hash{31: order}, MerchantId: merchant, ClaimCommitment: common.Hash{31: 8}, IntentCommitment: common.Hash{31: 9},
			Payer: board.From, Asset: asset, Beneficiary: beneficiary.From, Amount: big.NewInt(int64(order) * 100), Nonce: big.NewInt(int64(order)),
			MerchantKeyVersion: 1, BeneficiaryVersion: 1, PayerGeneration: 1, IssuedAt: head.Time, ExpiresAt: head.Time + 120, Mode: 1, PayerRole: 1,
		}
		fields, err := (abi.Arguments{parsed.Methods["lock"].Inputs[0]}).Pack(quote)
		if err != nil {
			t.Fatal(err)
		}
		unsigned := append(common.LeftPadBytes(big.NewInt(8283).Bytes(), 32), common.LeftPadBytes(address[:], 32)...)
		authorization, err := signing.NewLock(append(unsigned, fields...), phone.From)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := contract.QuoteDigest(nil, quote)
		if err != nil {
			t.Fatal(err)
		}
		return quote, signStore(digest), signBoard(authorization.Digest)
	}
	lock := func(order byte) *types.Receipt {
		t.Helper()
		quote, merchantSig, boardSig := offer(order)
		return mined(contract.Lock(operator, quote, phone.From, merchantSig, boardSig))
	}
	act := func(action signing.Action, order byte, sign func(common.Hash) []byte,
		send func(*bind.TransactOpts, [32]byte, [32]byte, []byte) (*types.Transaction, error)) {
		t.Helper()
		id := common.Hash{31: order}
		state, err := contract.Orders(nil, merchant, id)
		if err != nil {
			t.Fatal(err)
		}
		var claimable uint64
		if action == signing.Receipt || action == signing.Dispute || action == signing.Withdraw {
			claimable = state.ClaimableAt
		}
		digest, err := signing.ActionDigest(action, address, merchant, id, claimable)
		if err != nil {
			t.Fatal(err)
		}
		mined(send(operator, merchant, id, sign(digest)))
	}
	const received, resolved, claimed, indirect, undelivered byte = 41, 42, 43, 44, 45
	const neverLocked, reorged, unknown, moved byte = 46, 47, 48, 49
	call(nil, "evm_increaseTime", 3600) // the stream covers the first business day from before its start
	call(nil, "anvil_mine", 1)
	firstLock := lock(received)
	lock(resolved)
	lock(claimed)
	lock(undelivered)
	// A forwarding contract submits this lock, so the transaction does not call the escrow directly.
	forward := append(common.FromHex("6024806009"+"5f395ff3"+"365f5f375f5f365f5f73"), append(address[:], common.FromHex("5af100")...)...)
	forward[1] = byte(len(forward) - 9)
	forwarder, tx, _, err := bind.DeployContract(operator, abi.ABI{}, forward, client)
	mined(tx, err)
	quote, merchantSig, boardSig := offer(indirect)
	data, err := parsed.Pack("lock", quote, phone.From, merchantSig, boardSig)
	if err != nil {
		t.Fatal(err)
	}
	if receipt := mined(bind.NewBoundContract(forwarder, abi.ABI{}, client, client, client).RawTransact(operator, data)); !slices.ContainsFunc(
		receipt.Logs, func(log *types.Log) bool {
			return log.Address == address && log.Topics[0] == parsed.Events["OrderLocked"].ID
		}) {
		t.Fatal("forwarded lock emitted no order event")
	}
	for _, order := range []byte{received, resolved, claimed, indirect} {
		act(signing.Delivered, order, signStore, contract.MarkDelivered)
	}
	act(signing.Receipt, received, signPhone, contract.Release)
	act(signing.Dispute, resolved, signPhone, contract.Dispute)
	mined(contract.Resolve(operator, merchant, common.Hash{31: resolved}, false))
	call(nil, "evm_increaseTime", 90_000) // the claim and the store's cancellation belong to the next business day
	batch := mined(contract.ClaimBatch(operator, merchant, beneficiary.From,
		[][32]byte{{31: claimed}, {31: undelivered}, {31: indirect}, {31: claimed}, {31: undelivered}, {31: unknown}, {}}))
	// Nobody needs a registered merchant to make the contract report a skipped claim.
	mined(contract.ClaimBatch(operator, [32]byte{}, beneficiary.From, [][32]byte{{}}))
	act(signing.Cancel, undelivered, signStore, contract.Cancel)

	header, err := client.HeaderByNumber(ctx, deployment.BlockNumber)
	if err != nil {
		t.Fatal(err)
	}
	code, err := client.CodeAt(ctx, address, deployment.BlockNumber)
	if err != nil {
		t.Fatal(err)
	}
	profile := Profile{StreamID: "b0cabaf8-23e5-43d8-b73e-362fa0fb1511", GenerationID: "b0cabaf8-23e5-43d8-b73e-362fa0fb1512",
		Version: 3, ChainID: 8283, Contract: address, Asset: asset, ABIHash: escrowABIHash, CodeHash: crypto.Keccak256Hash(code),
		Manifest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab1511",
		StartHeight: deployment.BlockNumber.Int64(), StartHash: header.Hash()}
	db, err := pgxpool.New(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	_, ingest := fixtureLogin(t, database, db, "nu54_p07_ingest")
	repository, err := Open(ctx, ingest)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	source, err := Dial(ctx, endpoint, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err := repository.Register(ctx, profile); err != nil {
		t.Fatal(err)
	}
	// Finality is first held at the deployment, then moved over the whole story.
	controlled := &controlledFinality{RPCSource: source, height: profile.StartHeight}
	reader := &Reader{Profile: profile, Source: controlled, DB: repository}
	poll := func() {
		t.Helper()
		if err := reader.Poll(ctx); err != nil {
			t.Fatal(err)
		}
	}
	poll()
	activate(t, db, profile.StreamID, profile.GenerationID)
	server, _ := orderReader(t, database, db, profile.StreamID, merchant.Hex())
	facts := func(order byte) []orderFact {
		t.Helper()
		return orderFacts(t, server, profile.StreamID, merchant.Hex(), (common.Hash{31: order}).Hex())
	}
	var observed int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.observed_event WHERE generation_id=$1", profile.GenerationID).
		Scan(&observed); err != nil || observed != 17 {
		t.Fatalf("observed %d order events: %v", observed, err)
	}
	if got := facts(received); len(got) != 0 {
		t.Fatalf("unconfirmed order exposed: %+v", got)
	}
	controlled.height = -2
	poll()

	expect := func(order byte, want ...string) []orderFact {
		t.Helper()
		got := facts(order)
		var kinds []string
		for _, fact := range got {
			kinds = append(kinds, fact.kind)
		}
		if !slices.Equal(kinds, want) {
			t.Fatalf("order %d: %v, want %v", order, kinds, want)
		}
		return got
	}
	first := expect(received, "locked", "delivered", "released")
	if f := first[0]; f.payer != lower(board.From) || f.beneficiary != lower(beneficiary.From) || f.confirmer != lower(phone.From) ||
		f.amount != "4100" || f.window != 3600 || f.deliverBy != f.included.Unix()+7200 || f.tx != firstLock.TxHash.Hex() ||
		f.block != firstLock.BlockHash.Hex() || f.height != firstLock.BlockNumber.Int64() || f.contract != lower(address) ||
		f.abi != escrowABIHash || f.policy != profile.Manifest {
		t.Fatalf("wrong lock fact: %+v", f)
	}
	if first[1].claimableAt != first[1].included.Unix()+3600 || first[2].reason != 0 || first[2].beneficiary != lower(beneficiary.From) {
		t.Fatalf("wrong delivery or receipt: %+v", first)
	}
	if got := expect(resolved, "locked", "delivered", "disputed", "cancelled"); got[2].resolveBy != got[2].included.Unix()+14*86400 ||
		got[3].reason != 2 || got[3].payer != lower(board.From) || got[3].amount != "4200" {
		t.Fatalf("wrong dispute outcome: %+v", got)
	}
	repeated := expect(claimed, "locked", "delivered", "released", "claim-skipped")
	if repeated[2].reason != 1 || repeated[3].reason != 5 || repeated[2].tx != batch.TxHash.Hex() || repeated[3].tx != batch.TxHash.Hex() {
		t.Fatalf("wrong batch facts: %+v", repeated)
	}
	if got := expect(indirect, "locked", "delivered", "released"); got[2].tx != batch.TxHash.Hex() {
		t.Fatalf("second claim left the batch: %+v", got)
	}
	if got := expect(undelivered, "locked", "claim-skipped", "cancelled"); got[1].reason != 2 || got[2].reason != 0 {
		t.Fatalf("wrong undelivered order: %+v", got)
	}
	expect(unknown) // an order nobody locked leaves no row
	for order, window := range map[byte]bool{received: true, indirect: false} {
		var bound bool
		if err := db.QueryRow(ctx, `SELECT quote_expires_at IS NOT NULL FROM chainview.observed_event
			WHERE generation_id=$1 AND kind='locked' AND order_key=$2`, profile.GenerationID, (common.Hash{31: order}).Hex()).
			Scan(&bound); err != nil || bound != window {
			t.Fatalf("quote window of order %d: %v %v", order, bound, err)
		}
	}

	// A lock that a reorganization removes before finality never becomes a fact.
	base, err := source.Header(ctx, -2)
	if err != nil {
		t.Fatal(err)
	}
	controlled.height = base.Height
	quote, _, _ = offer(reorged)
	var snapshot string
	var reverted bool
	call(&snapshot, "evm_snapshot")
	lockedFork := lock(reorged)
	lock(moved)
	poll()
	seen := func() (out bool) {
		t.Helper()
		if err := db.QueryRow(ctx, `SELECT COALESCE(bool_or(b.canonical),false) FROM chainview.observed_event e JOIN chainview.block b USING(generation_id,block_hash)
			WHERE e.generation_id=$1 AND e.order_key=$2`, profile.GenerationID, (common.Hash{31: reorged}).Hex()).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if !seen() || len(facts(reorged)) != 0 {
		t.Fatal("unconfirmed lock was not only observed")
	}
	call(&reverted, "evm_revert", snapshot)
	if !reverted {
		t.Fatal("could not restore local fork")
	}
	// On the surviving branch the second lock lands on the third business day, after the first order's quote expired.
	day1 := first[0].included.Add(-time.Second)
	day3 := day1.Add(48 * time.Hour)
	call(nil, "evm_setNextBlockTimestamp", day3.Unix())
	call(nil, "anvil_mine", 1)
	lock(moved)
	call(nil, "evm_setNextBlockTimestamp", day3.Add(24*time.Hour).Unix())
	call(nil, "anvil_mine", 1)
	replaced, err := source.Header(ctx, lockedFork.BlockNumber.Int64())
	if err != nil || replaced.Hash == lockedFork.BlockHash {
		t.Fatalf("fork did not diverge: %v", err)
	}
	controlled.height = -2
	poll()
	if seen() || len(facts(reorged)) != 0 {
		t.Fatal("removed lock kept its effect")
	}
	expect(moved, "locked")
	for i, want := range []escrowDay{
		{Locked: "21500", LockedCount: 5, Released: "4100", ReleasedCount: 1, Cancelled: "4200", CancelledCount: 1, Count: 7},
		{Locked: "0", Released: "8700", ReleasedCount: 2, Cancelled: "4500", CancelledCount: 1, Count: 3},
		{Locked: "4900", LockedCount: 1, Released: "0", Cancelled: "0", Count: 1},
	} {
		got := dayEvidence(t, server, profile, merchant.Hex(), day1.Add(time.Duration(i)*24*time.Hour))
		if got == nil {
			t.Fatalf("day %d not proven", i+1)
		}
		want.Anchor, want.Height, want.Digest = got.Anchor, got.Height, got.Digest
		if *got != want {
			t.Fatalf("day %d: %+v", i+1, got)
		}
	}
	var outcomes []string
	if err := server.QueryRow(ctx, `SELECT array_agg(kind||COALESCE(reason::text,'') ORDER BY log_index)
		FROM chainview.escrow_transaction($1,$2,$3)`, profile.StreamID, merchant.Hex(), batch.TxHash.Hex()).Scan(&outcomes); err != nil ||
		!slices.Equal(outcomes, []string{"released1", "claim-skipped2", "released1", "claim-skipped5"}) {
		t.Fatalf("claim transaction outcomes: %v %v", outcomes, err)
	}

	// Both the removed lock and an order nobody locked are proven absent after the quote expires.
	end := time.Unix(int64(quote.ExpiresAt), 0)
	for _, order := range []byte{reorged, neverLocked, unknown} {
		key := (common.Hash{31: order}).Hex()
		id, err := reader.ProveAbsence(ctx, merchant.Hex(), key, end)
		if err != nil {
			t.Fatalf("absence of order %d: %v", order, err)
		}
		var shown string
		var anchored time.Time
		if err := server.QueryRow(ctx, "SELECT evidence_id::text,anchored_at FROM chainview.reconciliation_absence($1,$2,$3)",
			profile.StreamID, merchant.Hex(), key).Scan(&shown, &anchored); err != nil || shown != id || anchored.Before(end) {
			t.Fatalf("absence not readable: %s %s %v", shown, anchored, err)
		}
	}
	final, err := source.Header(ctx, -2)
	if err != nil {
		t.Fatal(err)
	}
	for _, order := range []byte{received, resolved, claimed, undelivered} {
		key := (common.Hash{31: order}).Hex()
		if paid, err := source.Paid(ctx, merchant.Hex(), key, final); err != nil || !paid {
			t.Fatalf("settled order %d reads as never locked: %v %v", order, paid, err)
		}
		if _, err := reader.ProveAbsence(ctx, merchant.Hex(), key, end); err == nil {
			t.Fatalf("proved absence of locked order %d", order)
		}
	}
	if _, err := reader.ProveAbsence(ctx, merchant.Hex(), (common.Hash{31: neverLocked}).Hex(), final.Time.Add(time.Second)); err == nil {
		t.Fatal("proved absence before the quote expired")
	}

	// The running command answers a request by itself. Anvil reports finality 64 blocks behind its head.
	call(nil, "anvil_mine", 80)
	requested := (common.Hash{31: 50}).Hex()
	if !requestAbsence(t, server, profile.StreamID, merchant.Hex(), requested, end.Unix()) {
		t.Fatal("absence request not kept")
	}
	dir := t.TempDir()
	profiles, logs := filepath.Join(dir, "profiles.json"), filepath.Join(dir, "log")
	raw, err := json.Marshal([]Profile{profile})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profiles, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(logs)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	command := exec.CommandContext(ctx, os.Getenv("P07_CHAINVIEW_TEST_INDEXER"), "chainview")
	command.Env = append(os.Environ(), "P07_CHAINVIEW_DATABASE_URL="+ingest, "P07_CHAINVIEW_PROFILES="+profiles,
		"P07_RPC="+endpoint, "P07_POLL=50ms")
	command.Stderr = log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	for deadline := time.Now().Add(15 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		var found bool
		if err := server.QueryRow(ctx, "SELECT EXISTS(SELECT FROM chainview.reconciliation_absence($1,$2,$3))",
			profile.StreamID, merchant.Hex(), requested).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			out, _ := os.ReadFile(logs)
			t.Fatalf("running indexer left the request: %s", out)
		}
	}
	var tip int64
	for deadline := time.Now().Add(15 * time.Second); tip != final.Height+80; time.Sleep(50 * time.Millisecond) {
		if err := db.QueryRow(ctx, "SELECT tip_height FROM chainview.generation WHERE id=$1", profile.GenerationID).Scan(&tip); err != nil || time.Now().After(deadline) {
			t.Fatalf("ingest stopped at %d: %v", tip, err)
		}
	}
}
