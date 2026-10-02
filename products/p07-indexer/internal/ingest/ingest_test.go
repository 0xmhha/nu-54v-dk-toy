package ingest

// The ingest loop (P07 design 2 and 6) against a fake chain. With P07_TEST_DATABASE_URL set it
// also runs against PostgreSQL, including a restart from the stored cursor.

import (
	"context"
	"errors"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/receipt"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/store"
)

var (
	settlement = common.HexToAddress("0xDe7596556D35Fa62F238F074A0f0E59cF730caA4")
	other      = common.HexToAddress("0x00000000000000000000000000000000000000aa")
	merchant   = common.HexToAddress("0xF92a32CEB9d940057be7b76BF4e2Eb76409F61A4")
	device     = common.HexToAddress("0xbc3152c1fd512552b3962e86c7b7c248ea17f4d8")
)

func settled(contract common.Address, block uint64, index uint, order byte, amount int64) types.Log {
	word := func(v int64) []byte { return common.LeftPadBytes(big.NewInt(v).Bytes(), 32) }
	return types.Log{
		Address:     contract,
		Topics:      []common.Hash{Topic0, common.BytesToHash(merchant.Bytes()), common.BytesToHash([]byte{order}), common.BytesToHash(device.Bytes())},
		Data:        append(word(amount), word(256)...),
		BlockNumber: block, TxHash: common.BytesToHash([]byte{byte(block), byte(index)}), Index: index,
	}
}

// fakeChain answers with logs in [from, to] of the asked contract and topic, like eth_getLogs.
type fakeChain struct {
	finalized uint64
	logs      []types.Log
	fail      error
	calls     [][2]uint64
}

func (c *fakeChain) Finalized(context.Context) (uint64, error) { return c.finalized, c.fail }
func (c *fakeChain) BlockTime(_ context.Context, n uint64) (uint64, error) {
	return 1_790_000_000 + n, c.fail
}
func (c *fakeChain) Logs(_ context.Context, contract common.Address, t0 common.Hash, from, to uint64) ([]types.Log, error) {
	if c.fail != nil {
		return nil, c.fail
	}
	c.calls = append(c.calls, [2]uint64{from, to})
	var out []types.Log
	for _, l := range c.logs {
		if l.Address == contract && l.Topics[0] == t0 && l.BlockNumber >= from && l.BlockNumber <= to {
			out = append(out, l)
		}
	}
	return out, nil
}

// memSink is an in-memory Sink with the same rules as the PostgreSQL one.
type memSink struct {
	rows   map[[2]string]store.Row
	cursor *uint64
	fail   bool
}

func newMem() *memSink { return &memSink{rows: map[[2]string]store.Row{}} }
func (m *memSink) Cursor(_ context.Context, start uint64) (uint64, error) {
	if m.cursor == nil {
		return start, nil
	}
	return *m.cursor, nil
}
func (m *memSink) Commit(_ context.Context, rows []store.Row, to uint64) error {
	if m.fail {
		return errors.New("database down")
	}
	for _, r := range rows {
		m.rows[[2]string{r.TxHash, string(rune(r.LogIndex))}] = r
	}
	m.cursor = &to
	return nil
}

func drain(t *testing.T, in *Ingester) {
	t.Helper()
	for i := 0; i < 100; i++ {
		done, err := in.Step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if done {
			return
		}
	}
	t.Fatal("did not catch up")
}

func TestIngestRangesAndOnlyTheSettlementEvent(t *testing.T) {
	c := &fakeChain{finalized: 2600, logs: []types.Log{
		settled(settlement, 150, 0, 1, 4_500_000),
		settled(other, 160, 0, 2, 1), // same event, other contract (P07-FR-01)
		settled(settlement, 1500, 3, 3, 1_000_000),
		settled(settlement, 2600, 0, 4, 7),
	}}
	sink := newMem()
	in := &Ingester{Chain: c, Sink: sink, Contract: settlement, Start: 99}
	drain(t, in)
	if len(sink.rows) != 3 {
		t.Fatalf("%d rows, want 3", len(sink.rows))
	}
	want := [][2]uint64{{100, 1099}, {1100, 2099}, {2100, 2600}}
	if len(c.calls) != 3 || c.calls[0] != want[0] || c.calls[1] != want[1] || c.calls[2] != want[2] {
		t.Fatalf("ranges %v, want %v", c.calls, want)
	}
	if lag, cur, ok := in.Lag(); !ok || lag != 0 || cur != 2600 {
		t.Fatalf("lag %d cursor %d", lag, cur)
	}
}

func TestErrorsKeepTheCursor(t *testing.T) {
	c := &fakeChain{finalized: 500, logs: []types.Log{settled(settlement, 200, 0, 1, 5)}}
	sink := newMem()
	in := &Ingester{Chain: c, Sink: sink, Contract: settlement, Start: 99}
	c.fail = errors.New("rpc down")
	if _, err := in.Step(context.Background()); err == nil || sink.cursor != nil {
		t.Fatal("an RPC error must not move the cursor")
	}
	c.fail = nil
	sink.fail = true
	if _, err := in.Step(context.Background()); err == nil || sink.cursor != nil {
		t.Fatal("a store error must not move the cursor")
	}
	sink.fail = false
	drain(t, in)
	if len(sink.rows) != 1 || *sink.cursor != 500 {
		t.Fatalf("after recovery: %d rows, cursor %d", len(sink.rows), *sink.cursor)
	}
}

func TestDecodeRefusesOtherLogs(t *testing.T) {
	l := settled(settlement, 1, 0, 1, 1)
	if _, err := Decode(other, l, 0); err == nil {
		t.Error("other contract accepted")
	}
	l.Topics[0] = common.Hash{1}
	if _, err := Decode(settlement, l, 0); err == nil {
		t.Error("other event accepted")
	}
	r, err := Decode(settlement, settled(settlement, 9, 2, 5, 4_500_000), 77)
	if err != nil || r.Amount != "4500000" || r.Nonce != "256" || r.Merchant != merchant.Hex() || r.Device != device.Hex() || r.BlockTime != 77 {
		t.Errorf("%+v %v", r, err)
	}
}

func TestTopic0IsTheContractEvent(t *testing.T) {
	// The value the kiosk and the week-6 gate use (P04 submit tests, PAYMENT_SETTLED_TOPIC).
	if Topic0.Hex() != "0xeef4300dbd9217414481ce2ade0c4791c3b47c75b0c0d4b6bbe5fbb05260195f" {
		t.Fatal(Topic0.Hex())
	}
}

// ---------------------------------------------------------------- PostgreSQL

func pg(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("P07_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("P07_TEST_DATABASE_URL not set")
	}
	s, err := store.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestPostgresIdempotentRestartAndDuplicate(t *testing.T) {
	ctx := context.Background()
	s := pg(t)
	reset(t)
	logs := []types.Log{
		settled(settlement, 120, 0, 1, 4_500_000),
		settled(settlement, 700, 1, 2, 1_000_000),
		settled(settlement, 1800, 0, 1, 9), // a second log for order 1 (P07-FR-05)
	}
	c := &fakeChain{finalized: 1000, logs: logs}
	in := &Ingester{Chain: c, Sink: s, Contract: settlement, Start: 99}
	drain(t, in)
	// Restart: a new store and ingester continue from the stored cursor (P07-FR-02).
	s2 := pg(t)
	c.finalized = 2000
	in2 := &Ingester{Chain: c, Sink: s2, Contract: settlement, Start: 99}
	drain(t, in2)
	if c.calls[len(c.calls)-1][0] != 1001 {
		t.Fatalf("restart read from %d, want 1001", c.calls[len(c.calls)-1][0])
	}
	// The same range again stores nothing new (P07-FR-03).
	rows := []store.Row{}
	for _, l := range logs {
		r, _ := Decode(settlement, l, 1)
		rows = append(rows, r)
	}
	if err := s2.Commit(ctx, rows, 2000); err != nil {
		t.Fatal(err)
	}
	if n := count(t); n != 3 {
		t.Fatalf("%d rows, want 3", n)
	}
	// Lookup by the indexed topics; the earliest log wins and is marked duplicate (FR-04, FR-05).
	order1 := common.BytesToHash([]byte{1}).Hex()
	r, err := s2.Get(ctx, merchant.Hex(), order1)
	if err != nil || r.BlockNumber != 120 || r.Amount != "4500000" || !r.Duplicate || r.BlockTime != 1_790_000_120 {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = s2.Get(ctx, merchant.Hex(), common.BytesToHash([]byte{2}).Hex())
	if err != nil || r.Duplicate {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := s2.Get(ctx, merchant.Hex(), common.BytesToHash([]byte{9}).Hex()); !errors.Is(err, receipt.ErrNotFound) {
		t.Fatalf("missing order: %v", err)
	}
}

func reset(t *testing.T) {
	t.Helper()
	s := pg(t)
	if err := s.Exec(context.Background(), "TRUNCATE receipts, cursor"); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T) int {
	t.Helper()
	n, err := pg(t).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}
