package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
)

type afterProtectedCloseSigning struct {
	protectedCloseChain
	action func()
}

func (c afterProtectedCloseSigning) PrepareRentalClose(ctx context.Context, signer *ecdsa.PrivateKey, account common.Address) (core.RentalCloseTransaction, error) {
	tx, err := c.protectedCloseChain.PrepareRentalClose(ctx, signer, account)
	if err == nil {
		c.action()
	}
	return tx, err
}

func TestProtectedCloseActualDaemons(t *testing.T) {
	get := func(key string) string { return os.Getenv("P05_TEST_CLOSE_" + key) }
	adminURL := get("ADMIN")
	if adminURL == "" {
		t.Skip("requires the two-daemon staged protected close fixture")
	}
	dbConfig, err := pgx.ParseConfig(adminURL)
	if err != nil || !strings.HasPrefix(dbConfig.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(dbConfig.Host, "/private/tmp/") || strings.HasPrefix(dbConfig.Host, "/tmp/")) ||
		!strings.HasPrefix(get("RPC"), "http://127.0.0.1:") || !strings.HasPrefix(get("P07"), "https://127.0.0.1:") {
		t.Fatal("staged close requires disposable local databases and loopback services")
	}
	ctx, stop := context.WithTimeout(t.Context(), 22*time.Second)
	defer stop()
	db, err := pgx.ConnectConfig(ctx, dbConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	read := func(key string) []byte {
		t.Helper()
		b, err := os.ReadFile(get(key))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	dep, err := core.ParseDeployment(read("MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := core.LoadKey(get("KEYSTORE"), "file:"+filepath.Clean(get("PASSWORD_FILE")))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(read("CA")) {
		t.Fatal("fixture P07 CA missing")
	}
	cert, err := tls.LoadX509KeyPair(get("CERT"), get("KEY"))
	if err != nil {
		t.Fatal(err)
	}
	pin, err := hex.DecodeString(get("P07_PIN"))
	if err != nil || len(pin) != 32 {
		t.Fatal("fixture P07 pin missing")
	}
	manifest := sha256.Sum256(read("MANIFEST"))

	var sends atomic.Int32
	var suppressHashIndexes, suppressBlockBody, corruptBlockBody, corruptReceipts atomic.Bool
	var originalEncoded atomic.Value
	upstream := &http.Client{Timeout: 5 * time.Second}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
		if err != nil {
			w.WriteHeader(500)
			return
		}
		var call struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.Unmarshal(body, &call) != nil {
			w.WriteHeader(400)
			return
		}
		if (suppressHashIndexes.Load() && (call.Method == "eth_getTransactionByHash" || call.Method == "eth_getTransactionReceipt")) ||
			(suppressBlockBody.Load() && call.Method == "eth_getBlockByHash") {
			var request struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal(body, &request) != nil {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": nil})
			return
		}
		out, err := http.NewRequestWithContext(r.Context(), http.MethodPost, get("RPC"), bytes.NewReader(body))
		if err != nil {
			w.WriteHeader(500)
			return
		}
		out.Header.Set("Content-Type", "application/json")
		response, err := upstream.Do(out)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		defer response.Body.Close()
		reply, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		if err != nil {
			w.WriteHeader(502)
			return
		}
		if call.Method == "eth_sendRawTransaction" {
			attempt := sends.Add(1)
			if len(call.Params) != 1 {
				t.Error("unexpected tx publisher input")
				w.WriteHeader(400)
				return
			}
			if attempt == 1 {
				originalEncoded.Store(string(call.Params[0]))
			} else if originalEncoded.Load() != string(call.Params[0]) {
				t.Error("recovery tried publishing different signed transaction bytes")
			}
			var accepted struct {
				Result common.Hash `json:"result"`
			}
			if json.Unmarshal(reply, &accepted) != nil || accepted.Result == (common.Hash{}) {
				t.Error("Anvil did not accept the original close transaction")
			}
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if call.Method == "eth_getBlockReceipts" && corruptReceipts.Load() {
			var envelope struct {
				JSONRPC string                       `json:"jsonrpc"`
				ID      json.RawMessage              `json:"id"`
				Result  []map[string]json.RawMessage `json:"result"`
			}
			if json.Unmarshal(reply, &envelope) != nil || len(envelope.Result) == 0 {
				w.WriteHeader(500)
				return
			}
			envelope.Result[0]["status"] = json.RawMessage(`"0x0"`)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(envelope)
			return
		}
		if call.Method == "eth_getBlockByHash" && corruptBlockBody.Load() {
			var envelope struct {
				JSONRPC string                     `json:"jsonrpc"`
				ID      json.RawMessage            `json:"id"`
				Result  map[string]json.RawMessage `json:"result"`
			}
			if json.Unmarshal(reply, &envelope) != nil || envelope.Result == nil {
				w.WriteHeader(500)
				return
			}
			var entries []map[string]json.RawMessage
			if json.Unmarshal(envelope.Result["transactions"], &entries) != nil || len(entries) == 0 {
				w.WriteHeader(500)
				return
			}
			entries[0]["input"] = json.RawMessage(`"0x"`)
			envelope.Result["transactions"], _ = json.Marshal(entries)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(envelope)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(reply)
	}))
	defer proxy.Close()
	defer upstream.CloseIdleConnections()
	seal, err := NewCipher(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	open := func() *protectedCloseExecutor {
		t.Helper()
		anchor, err := core.NewRentalEvidenceReader(get("P07"), roots, cert, [32]byte(pin))
		if err != nil {
			t.Fatal(err)
		}
		chain, err := core.Dial(ctx, proxy.URL, dep)
		if err != nil {
			anchor.Close()
			t.Fatal(err)
		}
		executor, err := openProtectedCloseExecutor(ctx, get("DATABASE"), chain, anchor, seal, key, dep, manifest)
		if err != nil {
			anchor.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() { executor.store.(*postgresProtectedCloseStore).db.Close(); anchor.Close() })
		return executor
	}
	generation, err := strconv.ParseInt(get("GENERATION"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	req := protectedCloseRequest{Tenant: get("TENANT"), Branch: get("BRANCH"), Actor: get("ACTOR"), Device: get("DEVICE"),
		Generation: generation, Key: "eacc72ab-1c12-4bfe-b07a-e4c385cb7d6a"}
	executor := open()
	plan, err := executor.store.plan(ctx, req, executor.anchor.CallerFingerprint())
	if err != nil {
		t.Fatal(err)
	}
	req.Review = plan.ReviewRef
	var originalExpiry time.Time
	if err := db.QueryRow(ctx, "SELECT valid_until FROM ledger.rental_authority_audience WHERE branch_id=$1", req.Branch).Scan(&originalExpiry); err != nil {
		t.Fatal(err)
	}
	setExpiry := func(value time.Time) {
		t.Helper()
		if _, err := db.Exec(ctx, "UPDATE ledger.rental_authority_audience SET valid_until=$2 WHERE branch_id=$1", req.Branch, value); err != nil {
			t.Fatal(err)
		}
	}
	baseChain := executor.chain
	executor.chain = afterProtectedCloseSigning{protectedCloseChain: baseChain, action: func() { setExpiry(time.Now().Add(-time.Second)) }}
	candidate, err := executor.execute(ctx, req)
	if !errors.Is(err, service.Conflict) || candidate.State != "" || sends.Load() != 0 {
		t.Fatalf("revocation during actual v1 signing escaped: %s %v", candidate.State, err)
	}
	if prior, err := executor.store.recover(ctx, req); err != nil || prior.State != "" {
		t.Fatal("revoked signing persisted a candidate", err)
	}
	setExpiry(originalExpiry)
	executor.chain = baseChain
	chain, err := ethclient.DialContext(ctx, get("RPC"))
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Close()
	var snapshot string
	if err := chain.Client().CallContext(ctx, &snapshot, "evm_snapshot"); err != nil {
		t.Fatal(err)
	}
	candidate, err = executor.execute(ctx, req)
	if err != nil || candidate.State != "attempting" || candidate.Hash == (common.Hash{}) || sends.Load() != 1 || len(candidate.sealed) == 0 {
		t.Fatalf("lost send receipt lost the original prepared close: %s %v", candidate.State, err)
	}
	setExpiry(time.Now().Add(-time.Second))
	restart := open()
	recovered, err := restart.execute(ctx, req)
	if err != nil || recovered.State != "submitted" || recovered.Hash != candidate.Hash || recovered.Nonce != candidate.Nonce ||
		len(recovered.sealed) == 0 || sends.Load() != 1 {
		t.Fatalf("hash-first recovery failed after live revocation: %s %v", recovered.State, err)
	}
	var revert bool
	if err := chain.Client().CallContext(ctx, &revert, "evm_revert", snapshot); err != nil || !revert {
		t.Fatalf("could not orphan the unfinalized protected close: %v", err)
	}
	var dropped json.RawMessage
	if err := chain.Client().CallContext(ctx, &dropped, "anvil_dropTransaction", candidate.Hash); err != nil {
		t.Fatal(err)
	}
	recovered, err = restart.execute(ctx, req)
	if err == nil || recovered.State != "submitted" || recovered.Hash != candidate.Hash || len(recovered.sealed) == 0 || sends.Load() != 1 {
		t.Fatalf("eviction with revoked authority lost bytes or triggered a send: %s %v", recovered.State, err)
	}
	if _, err := db.Exec(ctx, "UPDATE ledger.rental_close_candidate SET attempt_until=clock_timestamp()-interval '1 second' WHERE branch_id=$1", req.Branch); err != nil {
		t.Fatal(err)
	}
	setExpiry(originalExpiry)
	recovered, err = restart.execute(ctx, req)
	if err != nil || recovered.State != "attempting" || recovered.Hash != candidate.Hash || sends.Load() != 2 {
		t.Fatalf("fresh authority did not re-send the identical original: %s %v", recovered.State, err)
	}
	setExpiry(time.Now().Add(-time.Second))
	if err := chain.Client().CallContext(ctx, nil, "anvil_mine", 4); err != nil {
		t.Fatal(err)
	}
	suppressHashIndexes.Store(true)
	parallel := open()
	type recovery struct {
		candidate protectedCloseCandidate
		err       error
	}
	done := make(chan recovery, 2)
	for _, instance := range []*protectedCloseExecutor{restart, parallel} {
		go func(instance *protectedCloseExecutor) {
			wanted, err := instance.execute(ctx, req)
			done <- recovery{wanted, err}
		}(instance)
	}
	for range 2 {
		answer := <-done
		if (answer.candidate.State != "attempting" && answer.candidate.State != "submitted") ||
			answer.candidate.Hash != candidate.Hash || len(answer.candidate.sealed) == 0 || sends.Load() != 2 {
			t.Fatalf("missing finalized hash indexes erased or republished the canonical original: %s %v", answer.candidate.State, answer.err)
		}
	}
	for _, fault := range []*atomic.Bool{&suppressBlockBody, &corruptBlockBody} {
		fault.Store(true)
		held, err := restart.execute(ctx, req)
		if err == nil || len(held.sealed) == 0 || held.Hash != candidate.Hash || sends.Load() != 2 {
			t.Fatalf("unverified finalized body released the signed candidate: %s %v", held.State, err)
		}
		fault.Store(false)
	}
	suppressHashIndexes.Store(false)
	corruptReceipts.Store(true)
	held, err := restart.execute(ctx, req)
	if err == nil || held.Hash != candidate.Hash || len(held.sealed) == 0 || sends.Load() != 2 {
		t.Fatalf("different receipt root changed the durable original: %s %v", held.State, err)
	}
	corruptReceipts.Store(false)
	recovered, err = restart.execute(ctx, req)
	if err != nil || recovered.State != "confirmed" || recovered.Hash != candidate.Hash || len(recovered.sealed) != 0 || sends.Load() != 2 {
		t.Fatalf("final receipt did not close the exact stored candidate: %s %v", recovered.State, err)
	}
	_, finalizedHash, found, err := baseChain.(*core.Chain).AccountClosedBlock(ctx, common.HexToAddress(plan.Account))
	if err != nil || !found || finalizedHash != candidate.Hash || sends.Load() != 2 {
		t.Fatalf("same v1 close did not become canonical: %v", err)
	}
	var encoded string
	if json.Unmarshal([]byte(originalEncoded.Load().(string)), &encoded) != nil {
		t.Fatal("original transaction fixture missing")
	}
	raw, err := hexutil.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var original types.Transaction
	if err := original.UnmarshalBinary(raw); err != nil {
		t.Fatal(err)
	}
	target := dep.Settlement
	different := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(8283), Nonce: original.Nonce(),
		GasTipCap: original.GasTipCap(), GasFeeCap: original.GasFeeCap(), Gas: original.Gas() + 1, To: &target, Data: original.Data(), Value: big.NewInt(0)})
	competing, err := types.SignTx(different, types.LatestSignerForChainID(different.ChainId()), key)
	if err != nil || competing.Hash() == original.Hash() {
		t.Fatal("could not construct distinct valid same-nonce candidate")
	}
	unknown := core.RentalCloseTransaction{Hash: competing.Hash(), Nonce: candidate.Nonce}
	state, err := baseChain.(*core.Chain).InspectRentalClose(ctx, crypto.PubkeyToAddress(key.PublicKey), common.HexToAddress(plan.Account), unknown)
	if err != nil || state != core.RentalCloseSuperseded {
		t.Fatalf("unrelated hash did not distinguish canonical nonce consumption: %s %v", state, err)
	}
	signerAddress := crypto.PubkeyToAddress(key.PublicKey)
	next, err := chain.PendingNonceAt(ctx, signerAddress)
	if err != nil {
		t.Fatal(err)
	}
	mustRevert := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(8283), Nonce: next, GasTipCap: original.GasTipCap(),
		GasFeeCap: original.GasFeeCap(), Gas: 150000, To: &target, Data: original.Data(), Value: big.NewInt(0)})
	reverted, err := types.SignTx(mustRevert, types.LatestSignerForChainID(mustRevert.ChainId()), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := chain.SendTransaction(ctx, reverted); err != nil {
		t.Fatal(err)
	}
	if err := chain.Client().CallContext(ctx, nil, "anvil_mine", 4); err != nil {
		t.Fatal(err)
	}
	failed := core.RentalCloseTransaction{Hash: reverted.Hash(), Nonce: next}
	state, err = baseChain.(*core.Chain).InspectRentalClose(ctx, signerAddress, common.HexToAddress(plan.Account), failed)
	if err != nil || state != core.RentalCloseReverted {
		t.Fatalf("actual finalized failed receipt was confused with a nonce collision: %s %v", state, err)
	}
	t.Log("one finalized v1 close after authority race, lost RPC response, orphan/eviction, unauthorized retry held and exact-byte resubmission")
}
