package chainview

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeChain struct {
	blocks map[int64]Block
	final  int64
	fault  int64
	paid   bool
}

func (c *fakeChain) Heads(context.Context) (Header, Header, error) {
	var tip int64
	for height := range c.blocks {
		if height > tip {
			tip = height
		}
	}
	return c.blocks[tip].Header, c.blocks[c.final].Header, nil
}

func (c *fakeChain) Header(_ context.Context, height int64) (Header, error) {
	if b, ok := c.blocks[height]; ok {
		return b.Header, nil
	}
	return Header{}, errors.New("missing header")
}

func (c *fakeChain) Block(_ context.Context, header Header) (Block, error) {
	if header.Height == c.fault {
		return Block{}, errors.New("unavailable receipts")
	}
	return c.blocks[header.Height], nil
}

func (c *fakeChain) Paid(_ context.Context, merchant, order string, anchor Header) (bool, error) {
	return c.paid, nil
}

func sampleBlock(height int64, hash, parent byte) Block {
	return Block{Header: Header{Height: height, Hash: common.Hash{31: hash}, Parent: common.Hash{31: parent},
		Time: time.Unix(1800000000+height, 0).UTC()}}
}

func TestPostgresAtomicReplayAndFinality(t *testing.T) {
	url := os.Getenv("P07_CHAINVIEW_TEST_URL")
	if url == "" {
		t.Skip("P07_CHAINVIEW_TEST_URL not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	repository := &Repository{db: db}
	streamId := "b0cabaf8-23e5-43d8-b73e-362fa0fb1497"
	generationId := "b0cabaf8-23e5-43d8-b73e-362fa0fb1498"
	profile := Profile{StreamID: streamId, GenerationID: generationId, Version: 2, ChainID: 8283,
		Contract: common.HexToAddress("0xcafe"), Asset: common.HexToAddress("0xbeef"), ABIHash: v2ABIHash,
		Manifest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		StartHeight: 1, StartHash: common.Hash{31: 1}, CodeHash: common.Hash{31: 99}}
	if err := repository.Register(ctx, profile); err != nil {
		t.Fatal(err)
	}

	block1, block2, block3 := sampleBlock(1, 1, 0), sampleBlock(2, 2, 1), sampleBlock(3, 3, 2)
	event := Event{Kind: "paid", Merchant: common.Hash{31: 20}.Hex(), Order: common.Hash{31: 21}.Hex(), Asset: lower(profile.Asset),
		Amount: "30", Nonce: "1", Payer: lower(common.HexToAddress("0xaabb")), Recipient: lower(common.HexToAddress("0xbbcc")),
		QuoteDigest: (common.Hash{31: 23}).Hex(), Mode: 1, PayerGeneration: 1, TxHash: (common.Hash{31: 5}).Hex()}
	block2.Events = []Event{event}
	source := &fakeChain{blocks: map[int64]Block{1: block1, 2: block2, 3: block3}, final: 1, fault: 2}
	reader := Reader{Profile: profile, Source: source, DB: repository}
	if err := reader.Poll(ctx); err == nil {
		t.Fatal("accepted missing receipts")
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.block WHERE generation_id=$1", generationId).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial blocks after unavailable receipt: %d, %v", count, err)
	}
	source.fault = 0
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.evidence WHERE generation_id=$1", generationId).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unfinalized payment emitted evidence: %d, %v", count, err)
	}
	fork2, fork3 := sampleBlock(2, 12, 1), sampleBlock(3, 13, 12)
	fork2.Events = []Event{event}
	source.blocks[2], source.blocks[3], source.final = fork2, fork3, 3
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := db.QueryRow(ctx, "SELECT log_block FROM chainview.evidence WHERE generation_id=$1", generationId).Scan(&hash); err != nil || hash != fork2.Hash.Hex() {
		t.Fatalf("wrong finalized evidence fork: %s, %v", hash, err)
	}
	activate(t, db, streamId, generationId)
	verifyReceiptRoles(t, url, db, streamId, event)
	order := (common.Hash{31: 22}).Hex()
	expiredAt := fork2.Time.Add(time.Second)
	if _, err := reader.ProveAbsence(ctx, event.Merchant, event.Order, expiredAt); err == nil {
		t.Fatal("proved absent for an observed payment")
	}
	if _, err := reader.ProveAbsence(ctx, event.Merchant, order, expiredAt.Add(time.Second)); err == nil {
		t.Fatal("proved absent before a finalized post-expiry block")
	}
	source.paid = true
	if _, err := reader.ProveAbsence(ctx, event.Merchant, order, expiredAt); err == nil {
		t.Fatal("ignored consumed state returned by the deployment")
	}
	source.paid = false
	_, err = db.Exec(ctx, "UPDATE chainview.block SET canonical=false WHERE generation_id=$1 AND block_hash=$2",
		generationId, fork2.Hash.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ProveAbsence(ctx, event.Merchant, order, expiredAt); err == nil {
		t.Fatal("accepted an absence proof across a coverage gap")
	}
	_, err = db.Exec(ctx, "UPDATE chainview.block SET canonical=true WHERE generation_id=$1 AND block_hash=$2",
		generationId, fork2.Hash.Hex())
	if err != nil {
		t.Fatal(err)
	}
	id, err := reader.ProveAbsence(ctx, event.Merchant, order, expiredAt)
	if err != nil || id == "" {
		t.Fatalf("absence proof: %s %v", id, err)
	}
	proofRole, proofURL := fixtureLogin(t, url, db, "nu54_p07_reconcile")
	_, err = db.Exec(ctx, "INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '1 minute')", proofRole, streamId, event.Merchant)
	if err != nil {
		t.Fatal(err)
	}
	proofReader, err := pgx.Connect(ctx, proofURL)
	if err != nil {
		t.Fatal(err)
	}
	defer proofReader.Close(ctx)
	var exposed string
	if err := proofReader.QueryRow(ctx, "SELECT evidence_id::text FROM chainview.reconciliation_absence($1,$2,$3)",
		streamId, event.Merchant, order).Scan(&exposed); err != nil || exposed != id {
		t.Fatalf("scoped absence evidence not usable by owner: %s %v", exposed, err)
	}
	var included time.Time
	var anchored int64
	err = db.QueryRow(ctx, "SELECT included_at,anchor_height FROM chainview.finalized_receipt WHERE generation_id=$1",
		generationId).Scan(&included, &anchored)
	if err != nil || !included.Before(expiredAt) || anchored != 3 {
		t.Fatalf("inclusion and finality not separated: %s %d %v", included, anchored, err)
	}
	shadow := profile
	shadow.GenerationID = "b0cabaf8-23e5-43d8-b73e-362fa0fb149a"
	if err := repository.Register(ctx, shadow); err != nil {
		t.Fatal(err)
	}
	replay := &Reader{Profile: shadow, Source: source, DB: repository}
	if err := replay.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.finalized_receipt WHERE generation_id=$1",
		shadow.GenerationID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unapproved replay exposed: %d %v", count, err)
	}
	if _, err := db.Exec(ctx, "UPDATE chainview.observed_event SET amount=31 WHERE generation_id=$1",
		shadow.GenerationID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "SELECT chainview.record_comparison($1,$2,1)", streamId, shadow.GenerationID); err == nil {
		t.Fatal("compared different finalized settlement values as equal")
	}
	if _, err := db.Exec(ctx, "UPDATE chainview.observed_event SET amount=30 WHERE generation_id=$1",
		shadow.GenerationID); err != nil {
		t.Fatal(err)
	}
	switchTo(t, db, streamId, shadow.GenerationID, 1)
	if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.finalized_receipt WHERE generation_id=$1",
		shadow.GenerationID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("approved replay did not activate: %d %v", count, err)
	}
	source.blocks[3] = sampleBlock(3, 33, 12)
	if err := reader.Poll(ctx); !errors.Is(err, ErrFinalizedConflict) {
		t.Fatalf("want finalized conflict, got %v", err)
	}
	var state string
	if err := db.QueryRow(ctx, "SELECT state FROM chainview.generation WHERE id=$1", generationId).Scan(&state); err != nil || state != "frozen" {
		t.Fatalf("generation did not freeze: %s, %v", state, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.pending_incident WHERE generation_id=$1", generationId).Scan(&count); err != nil || count != 1 {
		t.Fatalf("durable pending notification missing: %d, %v", count, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.finalized_receipt WHERE stream_id=$1",
		streamId).Scan(&count); err != nil || count != 0 {
		t.Fatalf("frozen stream still published through another generation: %d, %v", count, err)
	}
	if err := reader.Poll(ctx); !errors.Is(err, ErrLeaseUnavailable) {
		t.Fatalf("frozen generation restarted: %v", err)
	}
	var incidentID string
	if err := db.QueryRow(ctx, "SELECT id::text FROM chainview.pending_incident WHERE generation_id=$1",
		generationId).Scan(&incidentID); err != nil {
		t.Fatal("pending incident vanished before acknowledgement", err)
	}
	_, operatorURL := fixtureLogin(t, url, db, "nu54_p07_incident")
	operator, err := pgx.Connect(ctx, operatorURL)
	if err != nil {
		t.Fatal(err)
	}
	defer operator.Close(ctx)
	for _, want := range []bool{true, false} {
		var applied bool
		if err := operator.QueryRow(ctx, "SELECT chainview.acknowledge_incident($1)", incidentID).Scan(&applied); err != nil || applied != want {
			t.Fatalf("incident acknowledgement: %v %v", applied, err)
		}
	}
	if _, err := operator.Exec(ctx, "SELECT chainview.record_comparison($1,$2,2)", streamId, shadow.GenerationID); err == nil {
		t.Fatal("incident delivery login gained switch privileges")
	}
	if _, err := db.Exec(ctx, "SELECT chainview.record_comparison($1,$2,2)", streamId, shadow.GenerationID); err == nil {
		t.Fatal("acknowledging notice cleared finalized-conflict hold")
	}
}

func activate(t *testing.T, db *pgxpool.Pool, stream, generation string) {
	t.Helper()
	switchTo(t, db, stream, generation, 0)
}

func switchTo(t *testing.T, db *pgxpool.Pool, stream, generation string, revision int64) {
	t.Helper()
	ctx := context.Background()
	var id string
	err := db.QueryRow(ctx, `INSERT INTO chainview.switch_request
		(id,stream_id,target_id,expected_revision,operator_ref,compare_ref,requested_at)
		VALUES(gen_random_uuid(),$1,$2,$3,gen_random_uuid(),chainview.record_comparison($1,$2,$3),now()) RETURNING id::text`,
		stream, generation, revision).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "SELECT chainview.apply_switch($1)", id); err != nil {
		t.Fatal(err)
	}
}
