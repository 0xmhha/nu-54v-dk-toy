package chainview

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

type controlledFinality struct {
	*RPCSource
	height int64
}

func (s *controlledFinality) Heads(ctx context.Context) (Header, Header, error) {
	latest, err := s.Header(ctx, -2)
	if err != nil {
		return Header{}, Header{}, err
	}
	final, err := s.Header(ctx, s.height)
	return latest, final, err
}

func TestAnvilActualReceiptsAndDivergentFork(t *testing.T) {
	endpoint, database := os.Getenv("P07_CHAINVIEW_TEST_RPC"), os.Getenv("P07_CHAINVIEW_TEST_URL")
	fixturePath := os.Getenv("P07_CHAINVIEW_ANVIL_RECEIPTS")
	if endpoint == "" || database == "" || fixturePath == "" {
		t.Skip("isolated Anvil fixture and PostgreSQL not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Asset, Settlement common.Address
		ChainID           uint64
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	legacyProfile, sharedHeight := postBothVersions(t, ctx, client, fixture.Asset, fixture.Settlement)
	start := big.NewInt(2) // The fixture deploys token and settlement as its first two transactions.
	header, err := client.HeaderByNumber(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	code, err := client.CodeAt(ctx, fixture.Settlement, start)
	if err != nil || len(code) == 0 {
		t.Fatalf("deployment code unavailable: %v", err)
	}
	const stream = "b0cabaf8-23e5-43d8-b73e-362fa0fb14a0"
	const generation = "b0cabaf8-23e5-43d8-b73e-362fa0fb14a1"
	profile := Profile{StreamID: stream, GenerationID: generation, Version: 2, ChainID: fixture.ChainID,
		Contract: fixture.Settlement, Asset: fixture.Asset, ABIHash: v2ABIHash,
		Manifest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdff",
		CodeHash: crypto.Keccak256Hash(code), StartHeight: start.Int64(), StartHash: header.Hash()}
	source, err := Dial(ctx, endpoint, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	db, err := pgxpool.New(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &Repository{db: db}
	if err := repo.Register(ctx, profile); err != nil {
		t.Fatal(err)
	}
	base, err := source.Header(ctx, -2)
	if err != nil {
		t.Fatal(err)
	}
	verifyIngestCommand(t, ctx, database, endpoint, db, profile, base.Height)
	controlled := &controlledFinality{RPCSource: source, height: base.Height}
	reader := Reader{Profile: profile, Source: controlled, DB: repo}
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	activate(t, db, stream, generation)
	merchant := (common.Hash{31: 50}).Hex()
	onchainPaid, err := source.Paid(ctx, merchant, (common.Hash{31: 4}).Hex(), base)
	if err != nil || !onchainPaid {
		t.Fatalf("hash-pinned state did not confirm paid order: %v, %v", onchainPaid, err)
	}
	absence, err := reader.ProveAbsence(ctx, merchant, (common.Hash{31: 99}).Hex(), base.Time)
	if err != nil || absence == "" {
		t.Fatalf("could not prove actual unconsumed order at latest finalized hash: %s %v", absence, err)
	}
	rows, err := db.Query(ctx, `SELECT kind,count(*),sum(amount)::text FROM chainview.finalized_receipt
		WHERE stream_id=$1 GROUP BY kind ORDER BY kind`, stream)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		kind, amount string
		count        int
	}{{"paid", "60", 2}, {"refunded", "30", 2}} {
		if !rows.Next() {
			t.Fatal("missing canonical receipt group")
		}
		var got, amount string
		var count int
		if err := rows.Scan(&got, &count, &amount); err != nil ||
			got != expected.kind || count != expected.count || amount != expected.amount {
			t.Fatalf("receipt group mismatch: %s %d %s: %v", got, count, amount, err)
		}
	}
	rows.Close()
	legacySource, err := Dial(ctx, endpoint, legacyProfile)
	if err != nil {
		t.Fatal(err)
	}
	defer legacySource.Close()
	if err := repo.Register(ctx, legacyProfile); err != nil {
		t.Fatal(err)
	}
	legacyReader := Reader{Profile: legacyProfile, Source: &controlledFinality{RPCSource: legacySource, height: base.Height}, DB: repo}
	if err := legacyReader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	activate(t, db, legacyProfile.StreamID, legacyProfile.GenerationID)
	var count int
	var total, firstCheckpoint, lastCheckpoint string
	err = db.QueryRow(ctx, `SELECT count(*),sum(amount)::text,min(anchor_hash),max(anchor_hash)
		FROM chainview.finalized_receipt WHERE inclusion_height=$1 AND kind='paid'`, sharedHeight).
		Scan(&count, &total, &firstCheckpoint, &lastCheckpoint)
	if err != nil || count != 2 || total != "65" || firstCheckpoint != base.Hash.Hex() || lastCheckpoint != base.Hash.Hex() {
		t.Fatalf("shared-block v1/v2 sum or hash mismatch: %d %s %s %s, %v", count, total, firstCheckpoint, lastCheckpoint, err)
	}

	verifyLiveGrantCommands(t, ctx, database, db, stream, generation, merchant)
	t.Run("read-revision", func(t *testing.T) {
		verifyReadRevision(t, ctx, database, db, stream, merchant, (common.Hash{31: 4}).Hex())
	})
	checkExplorerBrowser(t, ctx, database, "current", stream, legacyProfile.StreamID, merchant)
	verifyHeldProfileResume(t, ctx, database, endpoint, db, profile, base.Height)
	verifyReplayBrowser(t, ctx, database, endpoint, db, profile, legacyProfile.StreamID, merchant)
	if os.Getenv("P07_EXPLORER_BROWSER_SCRIPT") != "" {
		if _, err := db.Exec(ctx, "UPDATE chainview.generation SET observed_at=clock_timestamp()-interval '2 minutes' WHERE id=$1", generation); err != nil {
			t.Fatal(err)
		}
		checkExplorerBrowser(t, ctx, database, "stale", stream, legacyProfile.StreamID, merchant)
		if err := reader.Poll(ctx); err != nil {
			t.Fatal(err)
		}
	}

	mine := func(seconds int64) Header {
		t.Helper()
		if err := client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", base.Time.Unix()+seconds); err != nil {
			t.Fatal(err)
		}
		if err := client.Client().CallContext(ctx, nil, "anvil_mine", 1); err != nil {
			t.Fatal(err)
		}
		got, err := source.Header(ctx, base.Height+1)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	snapshot := func() string {
		t.Helper()
		var id string
		if err := client.Client().CallContext(ctx, &id, "evm_snapshot"); err != nil {
			t.Fatal(err)
		}
		return id
	}
	restore := func(id string) {
		t.Helper()
		var ok bool
		if err := client.Client().CallContext(ctx, &ok, "evm_revert", id); err != nil || !ok {
			t.Fatalf("could not restore local fork: %v", err)
		}
	}
	branch := snapshot()
	left := mine(5)
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	restore(branch)
	branch = snapshot()
	right := mine(9)
	if right.Hash == left.Hash {
		t.Fatal("fork did not diverge")
	}
	controlled.height = right.Height
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := repo.canonical(ctx, generation, right.Height)
	if err != nil || got != right.Hash {
		t.Fatalf("wrong fork projection at %d: %s %v", right.Height, got, err)
	}
	checkExplorerBrowser(t, ctx, database, "reorg", stream, legacyProfile.StreamID, merchant)
	restore(branch)
	mine(12)
	if err := reader.Poll(ctx); !errors.Is(err, ErrFinalizedConflict) {
		t.Fatalf("contradictory finalized block failed to hold publication: %v", err)
	}
	var visible int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.finalized_receipt WHERE stream_id=$1", stream).Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("frozen receipts remain visible: %d %v", visible, err)
	}
	checkExplorerBrowser(t, ctx, database, "frozen", stream, legacyProfile.StreamID, merchant)
}
