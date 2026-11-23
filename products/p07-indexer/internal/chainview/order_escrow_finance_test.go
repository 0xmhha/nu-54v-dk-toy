package chainview

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type escrowDay struct {
	Anchor                                     string
	Height                                     int64
	Gross, Refunds                             *string
	Locked, Released, Cancelled                string
	LockedCount, ReleasedCount, CancelledCount int64
	Count                                      int64
	Digest                                     string
}

func dayEvidence(t *testing.T, db *pgx.Conn, profile Profile, merchant string, from time.Time) *escrowDay {
	t.Helper()
	var raw []byte
	if err := db.QueryRow(context.Background(), "SELECT chainview.finance_day_evidence($1,$2,$3,$4,$5,'8283',$6,$7,$8)",
		profile.StreamID, merchant, lower(profile.Asset), from, from.Add(24*time.Hour), lower(profile.Contract),
		profile.ABIHash, profile.Manifest).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw == nil {
		return nil
	}
	var day escrowDay
	if err := json.Unmarshal(raw, &day); err != nil {
		t.Fatal(err)
	}
	if day.Gross != nil || day.Refunds != nil {
		t.Fatalf("escrow day looks like a settlement day: %s", raw)
	}
	return &day
}

func escrowFixture(t *testing.T, last byte) (string, *pgxpool.Pool, *Repository, Profile) {
	t.Helper()
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
	profile := orderProfile()
	digits := string([]byte{'0' + last/10, '0' + last%10})
	profile.StreamID = "b0cabaf8-23e5-43d8-b73e-362fa0fb1" + digits + "1"
	profile.GenerationID = "b0cabaf8-23e5-43d8-b73e-362fa0fb1" + digits + "2"
	profile.Manifest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab1" + digits + "1"
	profile.StartHeight, profile.StartHash, profile.CodeHash = 1, common.Hash{31: 1}, common.Hash{31: 99}
	_, login := fixtureLogin(t, url, db, "nu54_p07_ingest")
	repository, err := Open(ctx, login)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repository.Close)
	if err := repository.Register(ctx, profile); err != nil {
		t.Fatal(err)
	}
	return url, db, repository, profile
}

func moneyEvent(profile Profile, kind string, order, tx byte, index int32, amount string) Event {
	e := Event{Kind: kind, Merchant: orderMerchant.Hex(), Order: (common.Hash{31: order}).Hex(), TxHash: (common.Hash{30: 1, 31: tx}).Hex(),
		Index: index, Asset: lower(profile.Asset), Amount: amount}
	switch kind {
	case "locked":
		e.Payer, e.Recipient, e.Confirmer = lower(orderPayer), lower(orderBeneficiary), lower(orderConfirmer)
		e.QuoteDigest, e.DeliverBy, e.ConfirmWindow = (common.Hash{31: order}).Hex(), 1900000000, 3600
	case "released":
		e.Recipient, e.Reason = lower(orderBeneficiary), reason(1)
	case "cancelled":
		e.Payer, e.Reason = lower(orderPayer), reason(0)
	}
	return e
}

func TestPostgresEscrowDayEvidence(t *testing.T) {
	url, db, repository, profile := escrowFixture(t, 52)
	ctx := context.Background()
	day1 := time.Unix(1800000000, 0).UTC().Truncate(24 * time.Hour)
	day2, day3 := day1.Add(24*time.Hour), day1.Add(48*time.Hour)
	at := func(height int64, hash, parent byte, when time.Time, events ...Event) Block {
		return Block{Header: Header{Height: height, Hash: common.Hash{31: hash}, Parent: common.Hash{31: parent}, Time: when}, Events: events}
	}
	source := &fakeChain{final: 2, blocks: map[int64]Block{
		1: at(1, 1, 0, day1.Add(-time.Hour)),
		2: at(2, 2, 1, day1, moneyEvent(profile, "locked", 1, 1, 0, "100"), moneyEvent(profile, "locked", 2, 2, 1, "200"),
			moneyEvent(profile, "locked", 3, 3, 2, "400")),
		// First fork: order 4 is locked in the last second of day 1.
		3: at(3, 3, 2, day2.Add(-time.Second), moneyEvent(profile, "locked", 4, 4, 0, "800")),
	}}
	reader := &Reader{Profile: profile, Source: source, DB: repository}
	poll := func() {
		t.Helper()
		if err := reader.Poll(ctx); err != nil {
			t.Fatal(err)
		}
	}
	poll()
	activate(t, db, profile.StreamID, profile.GenerationID)
	merchant := orderMerchant.Hex()
	server, role := orderReader(t, url, db, profile.StreamID, merchant)
	if day := dayEvidence(t, server, profile, merchant, day1); day != nil {
		t.Fatalf("day not yet covered by a final block was proven: %+v", day)
	}
	// Second fork: the same lock lands on the first second of day 2 instead. The other orders settle that day.
	skipped := Event{Kind: "claim-skipped", Merchant: merchant, Order: (common.Hash{31: 3}).Hex(), TxHash: (common.Hash{31: 70}).Hex(), Index: 4, Reason: reason(2)}
	source.blocks[3] = at(3, 13, 2, day2, moneyEvent(profile, "locked", 4, 4, 0, "800"),
		moneyEvent(profile, "released", 1, 5, 1, "100"), moneyEvent(profile, "released", 2, 5, 2, "200"),
		moneyEvent(profile, "cancelled", 3, 6, 3, "400"), skipped)
	source.blocks[4] = at(4, 14, 13, day3)
	source.blocks[5] = at(5, 15, 14, day3.Add(25*time.Hour))
	source.final = 5
	poll()
	first, second, third := dayEvidence(t, server, profile, merchant, day1), dayEvidence(t, server, profile, merchant, day2),
		dayEvidence(t, server, profile, merchant, day3)
	if first == nil || first.Locked != "700" || first.LockedCount != 3 || first.Released != "0" || first.Cancelled != "0" || first.Count != 3 {
		t.Fatalf("day 1: %+v", first)
	}
	if second == nil || second.Locked != "800" || second.LockedCount != 1 || second.Released != "300" || second.ReleasedCount != 2 ||
		second.Cancelled != "400" || second.CancelledCount != 1 || second.Count != 4 || second.Anchor != source.blocks[5].Hash.Hex() ||
		second.Height != 5 {
		t.Fatalf("day 2: %+v", second)
	}
	if third == nil || third.Count != 0 || third.Digest == first.Digest || first.Digest == second.Digest {
		t.Fatalf("day 3: %+v", third)
	}
	// The digest covers the exact facts in the documented form, so the server can rebuild it from its own rows.
	var rebuilt string
	if err := db.QueryRow(ctx, `SELECT encode(sha256(convert_to(jsonb_agg(jsonb_build_array(kind,tx,idx,block,amount)
		ORDER BY tx COLLATE "C",idx,block COLLATE "C")::text,'UTF8')),'hex') FROM (VALUES
		('locked',$1::text,0,$5::text,'800'),('released',$2,1,$5,'100'),('released',$2,2,$5,'200'),('cancelled',$3,3,$5,$4::text)) v(kind,tx,idx,block,amount)`,
		(common.Hash{30: 1, 31: 4}).Hex(), (common.Hash{30: 1, 31: 5}).Hex(), (common.Hash{30: 1, 31: 6}).Hex(), "400",
		source.blocks[3].Hash.Hex()).Scan(&rebuilt); err != nil || rebuilt != second.Digest {
		t.Fatalf("digest cannot be rebuilt: %s %s %v", rebuilt, second.Digest, err)
	}

	// One call settles a whole claim from its transaction.
	var orders, released int
	if err := server.QueryRow(ctx, `SELECT count(DISTINCT order_ref),count(*) FILTER (WHERE kind='released' AND reason=1)
		FROM chainview.escrow_transaction($1,$2,$3)`, profile.StreamID, merchant, (common.Hash{30: 1, 31: 5}).Hex()).
		Scan(&orders, &released); err != nil || orders != 2 || released != 2 {
		t.Fatalf("claim transaction: %d %d %v", orders, released, err)
	}
	if day := dayEvidence(t, server, profile, (common.Hash{31: 8}).Hex(), day2); day != nil {
		t.Fatal("ungranted merchant read a day")
	}
	settlement := profile
	settlement.ABIHash = v2ABIHash
	if day := dayEvidence(t, server, settlement, merchant, day2); day != nil {
		t.Fatal("escrow stream answered under the settlement ABI hash")
	}

	// A replay reproduces the same days.
	const job = "b0cabaf8-23e5-43d8-b73e-362fa0fb1529"
	if _, err := db.Exec(ctx, "SELECT chainview.start_replay($1,$2,1,now()+interval '1 minute')", profile.StreamID, job); err != nil {
		t.Fatal(err)
	}
	if err := reader.PollReplay(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(ctx, "SELECT state FROM chainview.replay_job WHERE id=$1", job).Scan(&state); err != nil || state != "matched" {
		t.Fatalf("replay: %s %v", state, err)
	}
	switchTo(t, db, profile.StreamID, job, 1)
	if again := dayEvidence(t, server, profile, merchant, day2); again == nil || *again != *second {
		t.Fatalf("replayed day differs: %+v %+v", again, second)
	}
	if _, err := db.Exec(ctx, "UPDATE chainview.receipt_scope SET valid_until=now() WHERE login_role=$1", role); err != nil {
		t.Fatal(err)
	}
	if day := dayEvidence(t, server, profile, merchant, day2); day != nil {
		t.Fatal("expired scope read a day")
	}
	if _, err := db.Exec(ctx, "UPDATE chainview.receipt_scope SET valid_until=now()+interval '1 hour' WHERE login_role=$1", role); err != nil {
		t.Fatal(err)
	}
	source.blocks[5] = at(5, 25, 14, day3.Add(25*time.Hour))
	if err := reader.Poll(ctx); err == nil {
		t.Fatal("contradicted final block accepted")
	}
	if day := dayEvidence(t, server, profile, merchant, day2); day != nil {
		t.Fatal("held stream proved a day")
	}
}

// Anyone can pay gas to make the contract report skipped claims of a real order.
func TestPostgresEscrowSkipSpam(t *testing.T) {
	url, db, repository, profile := escrowFixture(t, 53)
	ctx := context.Background()
	const spam = 10_000
	hash := func(n int64) common.Hash { return common.BigToHash(big.NewInt(n)) }
	when := time.Unix(1800000000, 0).UTC()
	block := func(height int64, events ...Event) Block {
		return Block{Header: Header{Height: height, Hash: hash(height), Parent: hash(height - 1), Time: when.Add(time.Duration(height) * time.Second)}, Events: events}
	}
	skip := func(order common.Hash, n int64) Event {
		return Event{Kind: "claim-skipped", Merchant: orderMerchant.Hex(), Order: order.Hex(), TxHash: hash(1_000_000 + n).Hex(), Index: int32(n), Reason: reason(2)}
	}
	var sameOrder, manyOrders []Event
	for n := int64(0); n < spam; n++ {
		sameOrder = append(sameOrder, skip(common.Hash{31: 1}, n))
		manyOrders = append(manyOrders, skip(hash(10_000+n), spam+n))
	}
	source := &fakeChain{final: 3, blocks: map[int64]Block{1: block(1), 2: block(2, moneyEvent(profile, "locked", 1, 1, 0, "100")), 3: block(3)}}
	reader := &Reader{Profile: profile, Source: source, DB: repository}
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	activate(t, db, profile.StreamID, profile.GenerationID)
	merchant, order := orderMerchant.Hex(), (common.Hash{31: 1}).Hex()
	server, _ := orderReader(t, url, db, profile.StreamID, merchant)
	read := func() time.Duration {
		t.Helper()
		orderFacts(t, server, profile.StreamID, merchant, order) // first use plans the function
		start := time.Now()
		for range 10 {
			orderFacts(t, server, profile.StreamID, merchant, order)
		}
		return time.Since(start) / 10
	}
	before := read()
	// A refused row still fails the whole poll when the events travel in one batch.
	broken := moneyEvent(profile, "locked", 9, 9, 0, "100")
	broken.Confirmer = ""
	source.blocks[4] = block(4, broken, moneyEvent(profile, "locked", 8, 8, 1, "100"))
	var tip int64
	if err := reader.Poll(ctx); err == nil {
		t.Fatal("incomplete event stored")
	}
	if err := db.QueryRow(ctx, "SELECT tip_height FROM chainview.generation WHERE id=$1", profile.GenerationID).Scan(&tip); err != nil || tip != 3 {
		t.Fatalf("partial block stored: %d %v", tip, err)
	}
	source.blocks[4], source.blocks[5] = block(4, sameOrder...), block(5, manyOrders...)
	source.blocks[6] = block(6, moneyEvent(profile, "released", 1, 2, 0, "100"))
	source.blocks[7] = Block{Header: Header{Height: 7, Hash: hash(7), Parent: hash(6), Time: when.Add(48 * time.Hour)}}
	source.final = 7
	start, polls := time.Now(), 0
	for tip := int64(0); tip != 7; polls++ {
		if err := reader.Poll(ctx); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, "SELECT finalized_height FROM chainview.generation WHERE id=$1", profile.GenerationID).Scan(&tip); err != nil {
			t.Fatal(err)
		}
	}
	ingest := time.Since(start)
	after := read()
	facts := orderFacts(t, server, profile.StreamID, merchant, order)
	if len(facts) != 22 || facts[0].kind != "locked" || facts[21].kind != "released" {
		t.Fatalf("real facts crowded out: %d", len(facts))
	}
	var skipsInOneRead int
	if err := server.QueryRow(ctx, "SELECT count(*) FROM chainview.escrow_transaction($1,$2,$3) WHERE order_ref=$4 AND kind='claim-skipped'",
		profile.StreamID, merchant, sameOrder[0].TxHash, order).Scan(&skipsInOneRead); err != nil || skipsInOneRead != 1 {
		t.Fatalf("an early claim result is hidden: %d %v", skipsInOneRead, err)
	}
	start = time.Now()
	day := dayEvidence(t, server, profile, merchant, when.Add(time.Second))
	if day == nil || day.Locked != "100" || day.Released != "100" || day.Count != 2 {
		t.Fatalf("spammed day: %+v", day)
	}
	t.Logf("day evidence %s", time.Since(start))
	const job = "b0cabaf8-23e5-43d8-b73e-362fa0fb1539"
	if _, err := db.Exec(ctx, "SELECT chainview.start_replay($1,$2,1,now()+interval '1 minute')", profile.StreamID, job); err != nil {
		t.Fatal(err)
	}
	state := "running"
	for range 10 {
		if err := reader.PollReplay(ctx); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, "SELECT state FROM chainview.replay_job WHERE id=$1", job).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != "running" {
			break
		}
	}
	if state != "matched" {
		t.Fatalf("replay of a spammed stream: %s", state)
	}
	t.Logf("%d skips ingested in %s over %d polls; escrow_receipt %s before, %s after", 2*spam, ingest, polls, before, after)
	if after > 100*time.Millisecond {
		t.Fatalf("skipped claims slowed an order read to %s", after)
	}
}
