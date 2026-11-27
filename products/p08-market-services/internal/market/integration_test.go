package market

import (
	"bytes"
	"cmp"
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/big"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMarketRejectsNonCanonicalUnits(t *testing.T) {
	for _, input := range []string{"0.1", "1e6", "-1", "+1", "01", " 1", new(big.Int).Lsh(big.NewInt(1), 256).String()} {
		if _, err := units(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	if x, err := units(maxWord.String()); err != nil || x.Cmp(maxWord) != 0 {
		t.Fatal("rejected maximum word")
	}
}

func fixtureKey(t *testing.T, id int64) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.ToECDSA(common.LeftPadBytes(big.NewInt(id).Bytes(), 32))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestIsolatedMarketExecutionAndKeeperRestart(t *testing.T) {
	endpoint, dsn := os.Getenv("P08_TEST_RPC"), os.Getenv("P08_TEST_DATABASE_URL")
	if endpoint == "" || dsn == "" {
		t.Skip("requires isolated loopback Anvil and disposable migrated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	keys := make([]*ecdsa.PrivateKey, 6)
	addrs := make([]common.Address, 6)
	for i := range keys {
		keys[i] = fixtureKey(t, int64(i+101))
		addrs[i] = crypto.PubkeyToAddress(keys[i].PublicKey)
	}
	var roles [4]common.Address
	copy(roles[:], addrs[1:5])
	// Anvil publishes only the fixture RPC here. No externally hosted endpoints participate.
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, addr := range addrs {
		if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", addr, "0x3635c9adc5dea00000"); err != nil {
			t.Fatal(err)
		}
	}
	artifact := filepath.Join(cmp.Or(os.Getenv("FOUNDRY_OUT"), "../../out"), "Market.sol", "Market.json")
	profile, err := Deploy(ctx, endpoint, artifact, keys[0], roles)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Dial(ctx, "https://example.invalid", profile); err == nil {
		t.Fatal("external RPC accepted")
	}
	wrong := profile
	wrong.Venue.CodeHash[0] ^= 1
	if _, err := Dial(ctx, endpoint, wrong); err == nil {
		t.Fatal("deployment code substitution accepted")
	}
	wrong = profile
	wrong.Genesis[0] ^= 1
	if _, err := Dial(ctx, endpoint, wrong); err == nil {
		t.Fatal("genesis substitution accepted")
	}
	c, err := Dial(ctx, endpoint, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer c.RPC.Close()
	// The fixture advances and reverts time deliberately. Assert the real clock rejects
	// such an unsynchronized chain before using its simulated clock for domain tests.
	headForClock, err := c.RPC.HeaderByNumber(ctx, nil)
	if err != nil || c.headClock(headForClock.Time) == "current" {
		t.Fatalf("fixture did not expose the wall clock fault: %v", err)
	}
	if _, err := c.Quote(ctx, keys[1], QuoteRequest{addrs[5], "fx-base", "100000000", "0", "1", "0"}); err == nil ||
		!strings.Contains(err.Error(), "head is not current") {
		t.Fatalf("stale head issued an order: %v", err)
	}
	c.clock = simulatedClock(ctx, c)

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Commercial integration provisions only the restricted P08 role for this process.
	var canRead, canRewrite bool
	if err := db.QueryRow(ctx, `SELECT has_table_privilege(current_user,'market.keeper_attempt','SELECT'),
		has_column_privilege(current_user,'market.keeper_attempt','raw_tx','UPDATE')`).Scan(&canRead, &canRewrite); err != nil ||
		!canRead || canRewrite {
		t.Fatalf("expected bounded keeper role: %v %v %v", canRead, canRewrite, err)
	}

	send := func(to common.Address, contract abi.ABI, key int, method string, args ...any) *types.Receipt {
		t.Helper()
		input, err := contract.Pack(method, args...)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := submit(ctx, c.RPC, keys[key], &to, input)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		return receipt
	}
	call := func(out any, method string, args ...any) {
		t.Helper()
		if err := c.RPC.Client().CallContext(ctx, out, method, args...); err != nil {
			t.Fatal(err)
		}
	}
	price := func(value int64) {
		head, err := c.RPC.HeaderByNumber(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		ts := new(big.Int).SetUint64(head.Time)
		send(profile.Venue.Address, venueABI, 2, "observe", big.NewInt(value), ts)
		send(profile.Venue.Address, venueABI, 3, "observe", big.NewInt(value), ts)
	}
	mine := func() { call(nil, "anvil_mine", "0x45") }
	advance := func(ts uint64) {
		call(nil, "evm_setNextBlockTimestamp", ts)
		call(nil, "evm_mine")
	}
	mint := func(asset common.Address, to common.Address, qty int64) {
		send(asset, tokenABI, 0, "mint", to, big.NewInt(qty))
	}
	authorize := func(asset common.Address, key int, amount int64) {
		send(asset, tokenABI, key, "approve", profile.Venue.Address, big.NewInt(amount))
	}
	user := addrs[5]
	mint(profile.Base.Address, user, 150_000_000_000)
	mint(profile.Quote.Address, user, 1_500_000_000)
	mint(profile.Quote.Address, addrs[0], 100_000_000)
	authorize(profile.Base.Address, 5, 150_000_000_000)
	authorize(profile.Quote.Address, 5, 1_500_000_000)
	authorize(profile.Quote.Address, 0, 100_000_000)
	send(profile.Venue.Address, venueABI, 0, "seedRisk", big.NewInt(100_000_000))
	price(1_200_000)

	handler := Handler{Chain: c, Signer: keys[1]}.Routes()
	request := func(method, path string, payload any, out any) int {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(body)))
		if out != nil {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatalf("%s: %v", w.Body, err)
			}
		}
		return w.Code
	}
	quote := func(action, a, b, minA, minB string) Approval {
		t.Helper()
		var q Approval
		status := request("POST", "/v1/quotes", QuoteRequest{user, action, a, b, minA, minB}, &q)
		if status != 200 {
			t.Fatalf("quote %s: status %d", action, status)
		}
		return q
	}
	nativeSource := filepath.Join("..", "..", "..", "p02-user-app", "native", "market")
	signTool := filepath.Join(t.TempDir(), "market-sign")
	compile := exec.CommandContext(ctx, "cc", "-O2", "-std=c11", "-o", signTool,
		filepath.Join(nativeSource, "wallet.c"), filepath.Join(nativeSource, "crypto.c"), filepath.Join(nativeSource, "test_sign.c"))
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile mobile native guard: %s %v", out, err)
	}
	hex := func(data []byte) string { return common.Bytes2Hex(data) }
	domain := hex(profile.Venue.Address.Bytes()) + hex(profile.Base.Address.Bytes()) + hex(profile.Quote.Address.Bytes()) +
		hex(profile.QuoteSigner.Bytes()) + hex(profile.Venue.CodeHash.Bytes())
	nativeSign := func(scope string, target common.Address, nonce uint64, gasPrice *big.Int, input, offer []byte, now uint64) ([]byte, error) {
		t.Helper()
		command := exec.CommandContext(ctx, signTool, scope, hex(target.Bytes()), strconv.FormatUint(nonce, 10),
			gasPrice.String(), hex(input), hex(offer), strconv.FormatUint(now, 10))
		out, err := command.Output()
		if err != nil {
			return nil, err
		}
		return hexutil.Decode(strings.TrimSpace(string(out)))
	}
	boardBinary := os.Getenv("P08_TEST_BOARD_SIGNER")
	if boardBinary == "" {
		t.Fatal("P08 board host approval executable is required")
	}
	boardSign := func(q Approval, to common.Address, nonce uint64, gasPrice *big.Int, input []byte, now uint64) ([]byte, error) {
		t.Helper()
		wire := make([]byte, 682)
		wire[0] = 21
		binary.BigEndian.PutUint64(wire[1:9], q.Deadline)
		binary.BigEndian.PutUint64(wire[9:17], 1)
		copy(wire[17:49], profile.Genesis[:])
		copy(wire[49:81], profile.BlockHash[:])
		pinned := common.FromHex("0x" + domain)
		copy(wire[81:193], pinned)
		copy(wire[193:213], to[:])
		binary.BigEndian.PutUint64(wire[213:221], nonce)
		binary.BigEndian.PutUint64(wire[221:229], gasPrice.Uint64())
		copy(wire[230:682], q.Data)
		if len(input) == marketlab.BoundAllowanceLength {
			wire[229] = 2
			wire = append(wire, input...)
		}
		board := common.Hash{0: 8, 31: 9}
		statement := append([]byte("N54J"), board[:]...)
		statement = binary.BigEndian.AppendUint64(statement, 1)
		statement = append(statement, user[:]...)
		statement = append(statement, wire...)
		// Host substitutes compare the exact authority and display transcripts.
		// The provisioned board uses its installed P-256 keys for both.
		proof := bytes.Repeat([]byte{8}, 64)
		wire = append(wire, proof...)
		command := exec.CommandContext(ctx, boardBinary, hex(wire), hex(statement), hex(proof),
			hex(board[:]), hex(user[:]), hex(crypto.FromECDSA(keys[5])), strconv.FormatUint(now, 10), "1")
		raw, err := command.Output()
		if err != nil {
			return nil, err
		}
		return hexutil.Decode("0x" + strings.TrimSpace(string(raw)))
	}
	checkedDispatchClock := false
	broadcastRaw := func(q Approval, to common.Address, input []byte) common.Hash {
		t.Helper()
		nonce, err := c.RPC.PendingNonceAt(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		gasPrice, err := c.RPC.SuggestGasPrice(ctx)
		if err != nil {
			t.Fatal(err)
		}
		head, err := c.RPC.HeaderByNumber(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := nativeSign(domain, to, nonce, gasPrice, input, q.Data, head.Time)
		if err != nil {
			t.Fatalf("native app signer refused a valid order: %v", err)
		}
		hostSigned, err := boardSign(q, to, nonce, gasPrice, input, head.Time)
		if err != nil || !bytes.Equal(hostSigned, raw) {
			t.Fatalf("board host approval did not sign the exact venue transaction: %v", err)
		}
		var tx types.Transaction
		if err := tx.UnmarshalBinary(raw); err != nil {
			t.Fatal(err)
		}
		review, reviewErr := marketlab.Review(marketlab.Policy{
			Venue: profile.Venue.Address, Base: profile.Base.Address, Quote: profile.Quote.Address, Signer: profile.QuoteSigner,
			CodeHash: profile.Venue.CodeHash, Genesis: profile.Genesis, Deployment: profile.BlockHash,
		}, marketlab.Request{Deployment: profile.BlockHash, User: user, To: to, Nonce: nonce, GasPrice: gasPrice.Uint64(),
			Calldata: input, Offer: q.Data}, time.Unix(int64(head.Time), 0))
		signer := types.LatestSignerForChainID(big.NewInt(ChainID))
		if reviewErr != nil || review.Digest != signer.Hash(&tx) || review.OfferDigest != q.Digest {
			t.Fatalf("independent review diverged from contract offer and native signed transaction: %+v %v", review, reviewErr)
		}
		sender, err := types.Sender(signer, &tx)
		if err != nil || sender != user || tx.To() == nil || *tx.To() != to || !bytes.Equal(tx.Data(), input) {
			t.Fatalf("native app signature differs from purchase: %s %v", sender, err)
		}
		if !checkedDispatchClock {
			checkedDispatchClock = true
			if path := os.Getenv("P08_TEST_SIGNER_VECTOR"); path != "" {
				fingerprint := map[string]any{
					"raw": hexutil.Encode(raw),
					"transfer": map[string]any{"profile": "isolated-market-8283-v1", "chain": "8283", "to": strings.ToLower(to.Hex()),
						"valueWei": "0", "nonce": strconv.FormatUint(nonce, 10), "maxFeeWei": gasPrice.String(), "priorityFeeWei": "0",
						"market": map[string]string{"deploymentHash": profile.BlockHash.Hex(), "user": strings.ToLower(user.Hex()), "calldata": hexutil.Encode(input), "offer": q.Data.String()}},
				}
				encoded, err := json.Marshal(fingerprint)
				if err != nil || os.WriteFile(path, encoded, 0600) != nil {
					t.Fatalf("could not write public signature fixture: %v", err)
				}
			}
			healthy := c.clock
			c.clock = func() time.Time { return time.Unix(int64(head.Time)+31, 0) }
			if got := request("POST", "/v1/transactions", map[string]any{"raw": hexutil.Bytes(raw), "offer": q.Data}, nil); got != 400 {
				t.Fatalf("stalled chain accepted a new submission: HTTP %d", got)
			}
			c.clock = healthy
		}
		var reply struct {
			TxHash common.Hash
			State  string
		}
		if status := request("POST", "/v1/transactions", map[string]any{"raw": hexutil.Bytes(raw), "offer": q.Data}, &reply); status != 202 || reply.TxHash != tx.Hash() || reply.State != "submitted" {
			t.Fatalf("relay failed: %d %+v", status, reply)
		}
		return reply.TxHash
	}
	broadcast := func(q Approval) common.Hash { return broadcastRaw(q, q.To, q.Data) }
	reconcile := func(hash, digest common.Hash, state string) Result {
		t.Helper()
		var result Result
		if status := request("GET", "/v1/executions/"+hash.Hex()+"/"+digest.Hex(), nil, &result); status != 200 || result.State != state || result.TxHash != hash {
			t.Fatalf("execution expected %s: %d %+v", state, status, result)
		}
		return result
	}

	add := quote("lp-add", "100000000000", "1200000000", "100000000000", "0")
	altered := bytes.Clone(add.Data)
	altered[99] ^= 1
	different := profile.Venue.CodeHash
	different[31] ^= 1
	badScope := domain[:len(domain)-64] + hex(different.Bytes())
	for _, invalid := range []struct {
		scope           string
		target          common.Address
		calldata, offer []byte
	}{
		{domain, profile.Venue.Address, altered, altered},
		{domain, profile.Base.Address, add.Data, add.Data},
		{badScope, profile.Venue.Address, add.Data, add.Data},
		{domain, user, add.Data, add.Data},
	} {
		if _, err := nativeSign(invalid.scope, invalid.target, 0, big.NewInt(1), invalid.calldata, invalid.offer, add.Deadline-30); err == nil {
			t.Fatal("native app signer accepted different authority, target or terms")
		}
	}
	for _, now := range []uint64{add.Deadline, add.Deadline - 61} {
		var inputs = []struct {
			target common.Address
			data   []byte
		}{{profile.Venue.Address, add.Data}}
		allowed, err := tokenABI.Pack("approve", profile.Venue.Address, big.NewInt(100_000_000_000))
		if err != nil {
			t.Fatal(err)
		}
		allowed = append(allowed, add.Digest[:]...)
		inputs = append(inputs, struct {
			target common.Address
			data   []byte
		}{profile.Base.Address, allowed})
		for _, invalid := range inputs {
			if _, err := nativeSign(domain, invalid.target, 0, big.NewInt(1), invalid.data, add.Data, now); err == nil {
				t.Fatal("native wallet signed stale or too-far-future execution or allowance")
			}
		}
	}
	approve := func(q Approval, asset common.Address, amount int64) {
		t.Helper()
		allowanceData, err := tokenABI.Pack("approve", profile.Venue.Address, big.NewInt(amount))
		if err != nil {
			t.Fatal(err)
		}
		allowanceData = append(allowanceData, q.Digest[:]...)
		incorrect := bytes.Clone(allowanceData)
		incorrect[67] ^= 1
		if _, err := nativeSign(domain, asset, 0, big.NewInt(1), incorrect, q.Data, q.Deadline-30); err == nil {
			t.Fatal("native wallet accepted an allowance amount outside the actual quote")
		}
		incorrect = bytes.Clone(allowanceData)
		incorrect[len(incorrect)-1] ^= 1
		if _, err := nativeSign(domain, asset, 0, big.NewInt(1), incorrect, q.Data, q.Deadline-30); err == nil {
			t.Fatal("native wallet accepted an allowance referencing a different signed quote")
		}
		allowHash := broadcastRaw(q, asset, allowanceData)
		mine()
		var allowance Result
		if request("GET", "/v1/approvals/"+allowHash.Hex()+"/"+user.Hex(), nil, &allowance) != 200 ||
			allowance.State != "confirmed" || allowance.GasFee == "" {
			t.Fatalf("native allowance did not reconcile %+v", allowance)
		}
	}
	approve(add, profile.Base.Address, 100_000_000_000)
	addHash := broadcast(add)
	reconcile(addHash, add.Digest, "pending")
	mine()
	added := reconcile(addHash, add.Digest, "confirmed")
	if added.AddedBase != "100000000000" || added.AddedQuote != "1200000000" {
		t.Fatalf("LP evidence %+v", added)
	}
	swap := quote("swap-base", "1000000000", "0", "11000000", "0")
	approve(swap, profile.Base.Address, 1_000_000_000)
	swapHash := broadcast(swap)
	mine()
	swapped := reconcile(swapHash, swap.Digest, "confirmed")
	if swapped.First != "11845896" {
		t.Fatalf("swap evidence %+v", swapped)
	}
	remove := quote("lp-remove", "100000000000", "0", "100000000000", "1100000000")
	removeHash := broadcast(remove)
	mine()
	removed := reconcile(removeHash, remove.Digest, "confirmed")
	if removed.First != "101000000000" || removed.Second != "1188154104" {
		t.Fatalf("redemption evidence %+v", removed)
	}
	if request("GET", "/v1/executions/"+swapHash.Hex()+"/"+remove.Digest.Hex(), nil, nil) != 409 {
		t.Fatal("confused quote/execution accepted")
	}

	fx := quote("fx-base", "100000055", "0", "1200000", "0")
	fxHash := broadcast(fx)
	mine()
	if result := reconcile(fxHash, fx.Digest, "confirmed"); result.First != "1200000" {
		t.Fatalf("precision wrong %+v", result)
	}
	repeated := broadcast(fx)
	mine()
	reconcile(repeated, fx.Digest, "rejected")

	open := quote("open-long", "9600000", "800000000", "0", "0")
	openHash := broadcast(open)
	mine()
	positionID := reconcile(openHash, open.Digest, "confirmed").First
	if positionID != "1" {
		t.Fatalf("position id %s", positionID)
	}
	head, err := c.RPC.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	position, err := c.Position(ctx, head.Hash(), big.NewInt(1))
	if err != nil {
		t.Fatal(err)
	}
	next, _ := units(position.NextPeriod)
	advance(next.Uint64()*FundingInterval + 1)
	price(1_200_000)
	var snapshot string
	call(&snapshot, "evm_snapshot")

	config := filepath.Join(t.TempDir(), "profile.json")
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	child := func(crash bool) {
		t.Helper()
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, binary, "-test.run", "^TestKeeperWorkerFixtureChild$")
		cmd.Env = append(os.Environ(), "P08_TEST_CHILD=1", "P08_TEST_CHILD_PROFILE="+config)
		if crash {
			cmd.Env = append(cmd.Env, "P08_TEST_CHILD_CRASH=1")
		}
		out, err := cmd.CombinedOutput()
		var status *exec.ExitError
		if crash {
			if !errors.As(err, &status) || status.ExitCode() != 42 {
				t.Fatalf("expected crash after commit: %s %v", out, err)
			}
		} else if err != nil {
			t.Fatalf("keeper subprocess failed: %s %v", out, err)
		}
	}
	funding := func(want string) (common.Hash, common.Hash) {
		t.Helper()
		var hash, blockHash []byte
		var state string
		var count int
		if err := db.QueryRow(ctx, `SELECT tx_hash,block_hash,state,count(*) over () FROM market.keeper_attempt WHERE deployment_hash=$1 AND kind='funding'`,
			profile.BlockHash.Bytes()).Scan(&hash, &blockHash, &state, &count); err != nil || count != 1 || state != want {
			t.Fatalf("funding not reconciled: count %d state %s wanted %s, %v", count, state, want, err)
		}
		return common.BytesToHash(hash), common.BytesToHash(blockHash)
	}
	child(true)
	hash, _ := funding("prepared")
	healthy := c.clock
	c.clock = func() time.Time { return healthy().Add(31 * time.Second) }
	keeper := Keeper{Chain: c, DB: db, Key: keys[4]}
	if err := keeper.Tick(ctx, nil); err != nil {
		t.Fatal(err)
	}
	funding("prepared")
	c.clock = healthy
	child(false)
	funding("submitted")
	mine()
	child(false)
	observed, firstBlock := funding("confirmed")
	if hash != observed {
		t.Fatal("restart substituted the prepared hash")
	}

	var reverted bool
	call(&reverted, "evm_revert", snapshot)
	if !reverted {
		t.Fatal("could not simulate a divergent branch")
	}
	call(nil, "anvil_mine", "0x03")
	child(false)
	funding("reorged")
	child(false)
	funding("submitted")
	mine()
	child(false)
	observed, secondBlock := funding("confirmed")
	if hash != observed || firstBlock == secondBlock {
		t.Fatal("reorg did not reconcile original hash on alternate block")
	}

	// An explicit new observation moves the price through the liquidation condition.
	head, err = c.RPC.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	advance(head.Time + 1)
	price(1_000)
	close := quote("close", "1", "2", "0", "0")
	child(true)
	closeHash := broadcast(close)
	child(false)
	mine()
	reconcile(closeHash, close.Digest, "confirmed")
	var history []PositionEvent
	if status := request("GET", "/v1/positions/1/history", nil, &history); status != 200 ||
		len(history) != 3 || history[0].Kind != "closed" || history[1].Kind != "funded" ||
		history[1].TxHash != hash || history[2].Kind != "opened" {
		t.Fatalf("position history lacks canonical original effects: %d %+v", status, history)
	}
	child(false)
	var liquidation string
	if err := db.QueryRow(ctx, `SELECT state FROM market.keeper_attempt WHERE deployment_hash=$1 AND kind='liquidation'`,
		profile.BlockHash.Bytes()).Scan(&liquidation); err != nil || liquidation != "rejected" {
		t.Fatalf("manual close/liquidation race double applied: %s %v", liquidation, err)
	}

	send(profile.Venue.Address, venueABI, 0, "setPaused", true)
	if status := request("POST", "/v1/quotes", QuoteRequest{user, "fx-base", "100000000", "0", "1", "0"}, nil); status != 409 {
		t.Fatalf("market pause did not stop trading: %d", status)
	}
	reconcile(closeHash, close.Digest, "confirmed")
	dsnInspect := os.Getenv("P08_TEST_INSPECT_DATABASE_URL")
	if dsnInspect == "" {
		t.Fatal("P08 operator integration requires the dedicated PostgreSQL role")
	}
	if unauthorized, err := OpenInspector(ctx, dsn); err == nil {
		unauthorized.Close()
		t.Fatal("keeper database login was accepted as operator")
	}
	inspector, err := OpenInspector(ctx, dsnInspect)
	if err != nil {
		t.Fatal(err)
	}
	defer inspector.Close()
	var inspectRaw, updateWork bool
	err = inspector.QueryRow(ctx, `SELECT has_column_privilege(current_user,'market.keeper_attempt','raw_tx','SELECT'),
		has_column_privilege(current_user,'market.keeper_attempt','state','UPDATE')`).Scan(&inspectRaw, &updateWork)
	if err != nil || inspectRaw || updateWork {
		t.Fatalf("inspector privileges: %v %v %v", inspectRaw, updateWork, err)
	}
	token := strings.Repeat("a", 64)
	operator, err := OperatorPage("operator", token, c, inspector)
	if err != nil {
		t.Fatal(err)
	}
	checkOperator := func(expected ...string) {
		t.Helper()
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("operator", token)
		rec := httptest.NewRecorder()
		operator.ServeHTTP(rec, req)
		for _, want := range expected {
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("operator response missing %q: %s", want, rec.Body.String())
			}
		}
		if strings.Contains(strings.ToLower(rec.Body.String()), strings.ToLower(user.Hex())) {
			t.Fatal("operator response exposed a user account address")
		}
	}
	checkOperator("paused", "confirmed", "rejected", hash.Hex(), "3 · closed", `id="duplicate-count">0`)
	checkMTLSOperatorFixture(t, ctx, c, inspector, hash, "paused")
	if browser := os.Getenv("P08_TEST_OPERATOR_BROWSER"); browser != "" {
		server := httptest.NewServer(operator)
		out, runErr := exec.CommandContext(ctx, browser, server.URL, hash.Hex()).CombinedOutput()
		server.Close()
		if runErr != nil {
			t.Fatalf("operator browser: %s %v", out, runErr)
		}
		t.Logf("operator browser: %s", out)
	}
	send(profile.Venue.Address, venueABI, 0, "setPaused", false)
	head, err = c.RPC.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	advance(head.Time + 121)
	checkOperator("price-unavailable", "stale", hash.Hex())
	checkMTLSOperatorFixture(t, ctx, c, inspector, hash, "price-unavailable")
	// Advance the disposable chain monotonically after the funding/reorg time travel.
	head, err = c.RPC.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	advance(max(head.Time+1, uint64(time.Now().Unix())))
	price(1012000)
	call(nil, "evm_setIntervalMining", 1)
	child(false)
	observed, _ = funding("confirmed")
	if observed != hash {
		t.Fatal("resumed keeper substituted funding hash")
	}
	checkOperator("ready", "fresh", hash.Hex(), "3 · closed", `id="duplicate-count">0`)
	checkMTLSOperatorFixture(t, ctx, c, inspector, hash, "ready")
	dead := httptest.NewServer(operator)
	dead.Close()
	unavailable, err := isolatedRPC(ctx, dead.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer unavailable.Close()
	operator, err = OperatorPage("operator", token, &Chain{Profile: profile, RPC: unavailable}, inspector)
	if err != nil {
		t.Fatal(err)
	}
	checkOperator("chain-unavailable", hash.Hex(), "confirmed")
	// The same commercial gate separately runs P05 registration/payment with neither market service nor keeper listening.
}

func TestKeeperWorkerFixtureChild(t *testing.T) {
	if os.Getenv("P08_TEST_CHILD") != "1" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	var profile Profile
	data, err := os.ReadFile(os.Getenv("P08_TEST_CHILD_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatal(err)
	}
	chain, err := Dial(ctx, os.Getenv("P08_TEST_RPC"), profile)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.RPC.Close()
	chain.clock = simulatedClock(ctx, chain)
	db, err := pgxpool.New(ctx, os.Getenv("P08_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	k := Keeper{Chain: chain, DB: db, Key: fixtureKey(t, 105)}
	if err := k.Tick(ctx, func() error {
		if os.Getenv("P08_TEST_CHILD_CRASH") == "1" {
			os.Exit(42)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func simulatedClock(ctx context.Context, c *Chain) func() time.Time {
	return func() time.Time {
		head, err := c.RPC.HeaderByNumber(ctx, nil)
		if err != nil {
			return time.Time{}
		}
		return time.Unix(int64(head.Time), 0)
	}
}
