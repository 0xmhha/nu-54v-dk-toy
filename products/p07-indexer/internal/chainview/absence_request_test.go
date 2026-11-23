package chainview

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

type failingPaid struct {
	*fakeChain
	err error
}

func (s *failingPaid) Paid(ctx context.Context, merchant, order string, anchor Header) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.fakeChain.Paid(ctx, merchant, order, anchor)
}

func requestAbsence(t *testing.T, db *pgx.Conn, stream, merchant, order string, expires int64) (kept bool) {
	t.Helper()
	if err := db.QueryRow(context.Background(), "SELECT chainview.request_absence($1,$2,$3,$4)", stream, merchant, order, expires).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	return kept
}

func TestPostgresAbsenceRequests(t *testing.T) {
	url, db, repository, profile := escrowFixture(t, 54)
	ctx := context.Background()
	merchant := orderMerchant.Hex()
	key := func(n byte) string { return (common.Hash{31: n}).Hex() }
	base := time.Now().Add(-time.Hour).Truncate(time.Second).UTC()
	block := func(height int64, hash byte, events ...Event) Block {
		return Block{Header: Header{Height: height, Hash: common.Hash{31: hash}, Parent: common.Hash{31: hash - 1},
			Time: base.Add(time.Duration(height) * time.Minute)}, Events: events}
	}
	chain := &fakeChain{final: 2, blocks: map[int64]Block{1: block(1, 1), 2: block(2, 2, moneyEvent(profile, "locked", 1, 1, 0, "100"))}}
	source := &failingPaid{fakeChain: chain}
	reader := &Reader{Profile: profile, Source: source, DB: repository}
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	activate(t, db, profile.StreamID, profile.GenerationID)
	server, role := orderReader(t, url, db, profile.StreamID, merchant)
	ask := func(order string, expires int64) bool {
		t.Helper()
		return requestAbsence(t, server, profile.StreamID, merchant, order, expires)
	}
	waiting := func() (n int) {
		t.Helper()
		if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.absence_request WHERE stream_id=$1", profile.StreamID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	proofs := func(order string) (n int) {
		t.Helper()
		if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview.evidence WHERE generation_id=$1 AND order_key=$2 AND kind='absent'",
			profile.GenerationID, order).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	answer := func() {
		t.Helper()
		if err := reader.AnswerAbsence(ctx, time.Second); err != nil {
			t.Fatal(err)
		}
	}
	soon := base.Add(150 * time.Second).Unix() // after block 2, before block 3

	// Only a granted server may ask, and only for something that can be proven.
	if requestAbsence(t, server, profile.StreamID, key(8), key(2), soon) || ask(key(1), soon) ||
		ask(key(2), 0) || ask(key(2), time.Now().Add(2*time.Hour).Unix()) || ask(key(0), soon) || ask("0xAB", soon) {
		t.Fatal("invalid request kept")
	}
	if waiting() != 0 {
		t.Fatal("refused request left a row")
	}
	for _, group := range []string{"nu54_p07_public", "nu54_p07_explorer", "nu54_p07_read", "nu54_p07_ingest"} {
		_, login := fixtureLogin(t, url, db, group)
		conn, err := pgx.Connect(ctx, login)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, "SELECT chainview.request_absence($1,$2,$3,$4)", profile.StreamID, merchant, key(2), soon); err == nil {
			t.Fatalf("%s requested a proof", group)
		}
		conn.Close(ctx)
	}
	if _, err := server.Exec(ctx, "SELECT * FROM chainview.absence_request"); err == nil {
		t.Fatal("server reads the request table")
	}

	// The same request stays once and waits for a finalized block at or after the expiry.
	if !ask(key(2), soon) || !ask(key(2), soon) || waiting() != 1 {
		t.Fatal("request not kept exactly once")
	}
	answer()
	if waiting() != 1 || proofs(key(2)) != 0 {
		t.Fatal("proved before expiry was finalized")
	}
	chain.blocks[3] = block(3, 3)
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	answer()
	if proofs(key(2)) != 0 {
		t.Fatal("proved at an unfinalized block")
	}
	chain.final = 3
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	// A failing RPC keeps the request with a delay. Ingest goes on.
	source.err = errors.New("RPC unavailable")
	if err := reader.AnswerAbsence(ctx, time.Second); !errors.Is(err, source.err) {
		t.Fatalf("RPC failure hidden: %v", err)
	}
	var attempts int
	var delayed bool
	if err := db.QueryRow(ctx, "SELECT attempts,next_attempt_at>clock_timestamp() FROM chainview.absence_request WHERE order_key=$1 AND stream_id=$2",
		key(2), profile.StreamID).Scan(&attempts, &delayed); err != nil || attempts != 1 || !delayed {
		t.Fatalf("failed request not delayed: %d %v %v", attempts, delayed, err)
	}
	answer() // not due, so the failing source is not asked again
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	source.err = nil
	due := func() {
		t.Helper()
		if _, err := db.Exec(ctx, "UPDATE chainview.absence_request SET next_attempt_at=clock_timestamp() WHERE stream_id=$1", profile.StreamID); err != nil {
			t.Fatal(err)
		}
	}
	due()
	// Two workers share the request.
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- reader.AnswerAbsence(ctx, time.Second) }()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if waiting() != 0 || proofs(key(2)) != 1 {
		t.Fatalf("request answered %d times, %d waiting", proofs(key(2)), waiting())
	}
	var requested, direct string
	if err := server.QueryRow(ctx, "SELECT state_proof_ref FROM chainview.reconciliation_absence($1,$2,$3)",
		profile.StreamID, merchant, key(2)).Scan(&requested); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ProveAbsence(ctx, merchant, key(2), time.Unix(soon, 0)); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT state_proof_ref FROM chainview.evidence WHERE generation_id=$1 AND order_key=$2 ORDER BY created_at DESC LIMIT 1",
		profile.GenerationID, key(2)).Scan(&direct); err != nil || direct != requested {
		t.Fatalf("requested proof differs from the command: %s %s %v", requested, direct, err)
	}
	if !ask(key(2), soon) || waiting() != 0 {
		t.Fatal("answered request was kept again")
	}
	if !ask(key(2), soon-1) || waiting() != 1 {
		t.Fatal("a proof for another expiry answered this one")
	}
	answer()
	// A proof about to expire is renewed.
	if _, err := db.Exec(ctx, "UPDATE chainview.evidence SET expires_at=clock_timestamp()+interval '1 minute' WHERE generation_id=$1 AND kind='absent'",
		profile.GenerationID); err != nil {
		t.Fatal(err)
	}
	if !ask(key(2), soon) || waiting() != 1 {
		t.Fatal("expiring proof not renewed")
	}
	answer()

	// A lock confirmed while the request waited drops it without a proof.
	later := base.Add(200 * time.Second).Unix()
	if !ask(key(5), later) {
		t.Fatal("request not kept")
	}
	chain.blocks[4], chain.final = block(4, 4, moneyEvent(profile, "locked", 5, 5, 0, "100")), 4
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	answer()
	if waiting() != 0 || proofs(key(5)) != 0 || ask(key(5), later) {
		t.Fatal("locked order still requested")
	}

	// The backlog of one merchant is bounded, and old requests are removed.
	if _, err := db.Exec(ctx, `INSERT INTO chainview.absence_request(stream_id,merchant_key,order_key,quote_expires,next_attempt_at)
		SELECT $1,$2,'0x'||lpad(to_hex(n+4096),64,'0'),$3,clock_timestamp()+interval '1 hour' FROM generate_series(1,1000) n`,
		profile.StreamID, merchant, later); err != nil {
		t.Fatal(err)
	}
	if ask(key(6), later) || !ask(fmt.Sprintf("0x%064x", 4097), later) {
		t.Fatal("full backlog took a new request or refused a kept one")
	}
	if _, err := db.Exec(ctx, "UPDATE chainview.absence_request SET requested_at=now()-interval '8 days' WHERE stream_id=$1", profile.StreamID); err != nil {
		t.Fatal(err)
	}
	answer()
	if waiting() != 0 || !ask(key(6), later) {
		t.Fatal("old requests kept")
	}
	if _, err := db.Exec(ctx, "UPDATE chainview.receipt_scope SET valid_until=now() WHERE login_role=$1", role); err != nil {
		t.Fatal(err)
	}
	if ask(key(7), later) {
		t.Fatal("expired grant requested a proof")
	}
	// A held stream answers nothing and keeps the request.
	chain.blocks[4] = block(4, 44)
	if err := reader.Poll(ctx); err == nil {
		t.Fatal("contradicted final block accepted")
	}
	answer()
	if waiting() != 1 || proofs(key(6)) != 0 {
		t.Fatal("held stream worked a request")
	}
}
