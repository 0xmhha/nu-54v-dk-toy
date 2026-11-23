package settlement

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPaymentContractABIsMatchManifest(t *testing.T) {
	raw, err := os.ReadFile("../../../../packages/contracts-abi/abi/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Contracts map[string]struct {
			SHA256 string `json:"sha256"`
		} `json:"abis"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.Contracts["OrderEscrow"].SHA256 != EscrowABIHash ||
		manifest.Contracts["SettlementV2"].SHA256 != ABIHash {
		t.Fatalf("enrolled ABI hashes differ from the manifest: %v", err)
	}
	if (Target{}).Escrow() || (Target{}).ABIHash() != ABIHash || (Target{}).profileVersion() != 2 ||
		!(Target{ABI: EscrowABIHash}).Escrow() || (Target{ABI: EscrowABIHash}).profileVersion() != 3 {
		t.Fatal("ABI does not select the payment contract")
	}
}

func escrowQuote(t *testing.T, target Target, role byte) (Quote, error) {
	t.Helper()
	profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
		Signer: fixtureSigner(101).Address(), MerchantID: [32]byte{31: 50}, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}}
	issued := time.Unix(1_800_000_000, 0)
	const id = "00000000-0000-4000-8000-000000000001"
	signed, digest, err := commerce.SignQuote(context.Background(), commerce.QuoteOrder{Tenant: id, Branch: id, Sale: id,
		QuoteID: [32]byte{1}, OrderID: [32]byte{2}, Claim: [32]byte{3}, Intent: [32]byte{4}, Nonce: [32]byte{5}, Payer: fixtureSigner(102).Address(),
		PayerGeneration: 1, PayerRole: role, Amount: "8000000", Issued: issued, Expires: issued.Add(2 * time.Minute)}, profile, fixtureSigner(101), issued)
	if err != nil {
		t.Fatal(err)
	}
	return unpackQuote(signed, digest[:], profile.Signer.Bytes(), target)
}

func TestLockBindsQuoteAndPhoneKey(t *testing.T) {
	target := Target{Contract: common.HexToAddress("0x1002"), Asset: common.HexToAddress("0x1003"), ABI: EscrowABIHash}
	if _, err := escrowQuote(t, target, 2); err == nil {
		t.Fatal("a rented payer's quote opened for the escrow")
	}
	quote, err := escrowQuote(t, target, 1)
	if err != nil {
		t.Fatal(err)
	}
	phone := fixtureSigner(120).Address()
	lock, err := orderescrow.NewLock(quote.Unsigned, phone)
	if err != nil {
		t.Fatal(err)
	}
	board, _ := fixtureSigner(102).SignDigest(context.Background(), lock.Digest)
	if got, err := verifyLock(quote, target, phone, board); err != nil || got.Digest != lock.Digest {
		t.Fatalf("valid lock refused: %v", err)
	}
	if _, err := verifyLock(quote, target, fixtureSigner(121).Address(), board); err == nil {
		t.Fatal("another phone key accepted")
	}
	// The other half-order signature recovers the same signer; only the low one is canonical.
	high := append([]byte(nil), board...)
	order := crypto.S256().Params().N
	s := new(big.Int).Sub(order, new(big.Int).SetBytes(board[32:64]))
	s.FillBytes(high[32:64])
	high[64] = 27 + 28 - high[64]
	if _, err := verifyLock(quote, target, phone, high); err == nil {
		t.Fatal("malleable signature accepted")
	}
	for _, confirmer := range []common.Address{{}, quote.Signer, quote.Args.Beneficiary, quote.Args.Payer, target.Contract} {
		other, err := orderescrow.NewLock(quote.Unsigned, confirmer)
		if err == nil {
			board, _ = fixtureSigner(102).SignDigest(context.Background(), other.Digest)
		}
		if _, err := verifyLock(quote, target, confirmer, board); err == nil {
			t.Fatalf("confirmer %s accepted", confirmer)
		}
	}
}

func TestPostgresOrderEscrowLedger(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := context.Background()
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x2222"), ABI: EscrowABIHash}
	issued := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	held := seedPayerOrder(t, db, target, issued, "70", 91, fixtureSigner(102), 1)
	sold := seedPayerOrder(t, db, Target{Stream: target.Stream, Asset: target.Asset, Contract: common.HexToAddress("0x3333")}, issued, "30", 92, fixtureSigner(102), 1, held)
	included := issued.Add(10 * time.Second)
	_, workerDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	worker, err := Open(ctx, workerDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	_, commerceDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	seller, err := Open(ctx, commerceDSN, "nu54_p05_commerce")
	if err != nil {
		t.Fatal(err)
	}
	defer seller.Close()
	merchant, evidence := common.Hash(held.Quote.Args.MerchantId).Hex(), fixtureUUID(t, db)
	hash := func(label string) string { return crypto.Keccak256Hash([]byte(label + held.Sale)).Hex() }
	const confirm = `SELECT ledger.confirm_order_lock($1,$2,$3,$4,$5,0,$6,$7,$8,$9::text::numeric,$10,1,$11,$12,$13,$14,172800)`
	args := func(amount, confirmer string, deliver int64) []any {
		return []any{held.Sale, held.Quote.Digest[:], target.Stream, evidence, hash("lock"), hash("block"), included, lower(target.Asset),
			amount, lower(held.Order.Payer), merchant, lower(target.Contract), confirmer, deliver}
	}
	phone, deliver := lower(fixtureSigner(120).Address()), included.Unix()+604800
	denied := func(store *Store, query string, input ...any) {
		t.Helper()
		var pg *pgconn.PgError
		if _, err := store.db.Exec(ctx, query, input...); !errors.As(err, &pg) || pg.Code != "42501" {
			t.Fatalf("expected no permission, got %v", err)
		}
	}
	denied(seller, confirm, args("70", phone, deliver)...)
	denied(worker, "SELECT * FROM ledger.order_escrow")
	denied(seller, "SELECT * FROM ledger.order_escrow")
	denied(worker, "SELECT * FROM ledger.order_escrow_status($1,$2,$3,$4)", held.Tenant, held.Branch, held.Actor, held.Sale)
	if _, err := worker.db.Exec(ctx, confirm, args("71", phone, deliver)...); service.CodeOf(sqlError(err)) != service.Conflict {
		t.Fatalf("another amount was booked: %v", err)
	}
	for index, want := range []bool{true, false} {
		var changed bool
		if err := worker.db.QueryRow(ctx, confirm, args("70", phone, deliver)...).Scan(&changed); err != nil || changed != want {
			t.Fatalf("confirmation %d: %t %v", index, changed, err)
		}
	}
	for _, other := range [][]any{args("70", lower(fixtureSigner(121).Address()), deliver), args("70", phone, deliver+1)} {
		if _, err := worker.db.Exec(ctx, confirm, other...); service.CodeOf(sqlError(err)) != service.Conflict {
			t.Fatalf("a second, different lock was accepted: %v", err)
		}
	}
	fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),$4,1,$5,$6,$7,30,$8,1,1,$9)`, sold.Sale, sold.Quote.Digest[:],
		target.Stream, hash("sale"), hash("block"), included, lower(target.Asset), lower(sold.Order.Payer), merchant)

	finance := fixtureUUID(t, db)
	for _, capability := range []string{"report_read", "day_close", "refund_request"} {
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,$3,now()+interval '1 hour')", held.Branch, finance, capability)
	}
	_, financeDSN := testLogin(t, db, dsn, "nu54_p05_finance")
	books, err := Open(ctx, financeDSN, "nu54_p05_finance")
	if err != nil {
		t.Fatal(err)
	}
	defer books.Close()
	denied(books, "SELECT ledger.console_refunds_settled($1,$2,$3,$4)", held.Tenant, held.Branch, finance, held.Sale)
	denied(books, "SELECT * FROM ledger.finance_order_snapshot_settled($1,$2,$3,$4)", held.Tenant, held.Branch, finance, held.Sale)
	var gross, balance string
	var postings int
	if err := books.db.QueryRow(ctx, `SELECT r->>'gross',(r->>'postings')::int FROM ledger.finance_day_report($1,$2,$3,
		(($5::timestamptz AT TIME ZONE 'Asia/Seoul') - interval '4 hours')::date,$4,false) r`,
		held.Tenant, held.Branch, finance, lower(target.Asset), included).Scan(&gross, &postings); err != nil || gross != "30" || postings != 2 {
		t.Fatalf("report counts %s in %d postings: %v", gross, postings, err)
	}
	if err := db.QueryRow(ctx, "SELECT sum(CASE side WHEN 'debit' THEN amount ELSE -amount END)::text FROM ledger.posting WHERE sale_id=$1", held.Sale).
		Scan(&balance); err != nil || balance != "0" {
		t.Fatalf("unbalanced escrow posting %s: %v", balance, err)
	}
	if _, err := books.db.Exec(ctx, "SELECT ledger.reserve_console_refund($1,$2,$3,$4,gen_random_uuid(),1,'merchant_cancel')",
		held.Tenant, held.Branch, finance, held.Sale); service.CodeOf(sqlError(err)) != service.Conflict {
		t.Fatalf("refund of held money: %v", err)
	}
	if _, err := books.db.Exec(ctx, "SELECT ledger.reserve_console_refund($1,$2,$3,$4,gen_random_uuid(),1,'merchant_cancel')",
		sold.Tenant, sold.Branch, finance, sold.Sale); err != nil {
		t.Fatalf("settled order lost its refund path: %v", err)
	}
	// The held 70 stays out of the day's total; the released sale counts in it.
	closeDay := func(gross string) error {
		_, err := books.db.Exec(ctx, `SELECT ledger.close_business_day($1,$2,$3,(($5::timestamptz AT TIME ZONE 'Asia/Seoul') - interval '4 hours')::date,$4,$6::text::numeric,0,gen_random_uuid())`,
			held.Tenant, held.Branch, finance, lower(target.Asset), included, gross)
		return err
	}
	if err := closeDay("30"); err != nil {
		t.Fatal(err)
	}
	const event = `SELECT ledger.record_escrow_event($1,$2,$3,$4,$5,$6,$7::text::numeric,$8,$9,$10,$11)`
	record := func(id, kind string, due, reason, amount any, index int) error {
		_, err := worker.db.Exec(ctx, event, held.Sale, lower(target.Contract), id, kind, due, reason, amount, hash(kind), index, hash("block"), included.Add(time.Second))
		return err
	}
	released := fixtureUUID(t, db)
	if err := record(fixtureUUID(t, db), "delivered", deliver, nil, nil, 1); err != nil {
		t.Fatal(err)
	}
	if err := record(released, "released", nil, 1, "69", 2); service.CodeOf(sqlError(err)) != service.Conflict {
		t.Fatalf("a short payout was booked: %v", err)
	}
	for range 2 {
		if err := record(released, "released", nil, 1, "70", 2); err != nil {
			t.Fatal(err)
		}
	}
	if err := record(fixtureUUID(t, db), "cancelled", nil, 0, "70", 3); err == nil {
		t.Fatal("an order was paid and refunded")
	}
	if err := closeDay("30"); service.CodeOf(sqlError(err)) != service.Conflict {
		t.Fatalf("income released later that day was left out: %v", err)
	}
	if err := closeDay("100"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT sum(CASE side WHEN 'debit' THEN amount ELSE -amount END)::text FROM ledger.posting WHERE sale_id=$1", held.Sale).
		Scan(&balance); err != nil || balance != "0" {
		t.Fatalf("unbalanced release %s: %v", balance, err)
	}
	denied(seller, "SELECT * FROM ledger.claim_escrow_action(ARRAY[]::uuid[])")
	denied(books, event, held.Sale, "", released, "released", nil, 1, "70", "", 0, "", included)
	denied(worker, "SELECT ledger.finance_escrow_decide($1,$2,$3,gen_random_uuid(),true)", held.Tenant, held.Branch, finance)
	denied(books, "SELECT ledger.store_order_answer($1,$2,$3,$4,gen_random_uuid(),''::bytea,'receipt',1,''::bytea)", held.Tenant, held.Branch, held.Actor, held.Sale)
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=true WHERE sale_id IN ($1,$2)", held.Sale, sold.Sale)
}

// lockedOrder seeds an order whose lock is final.
func lockedOrder(t *testing.T, db *pgxpool.Pool, target Target, nonce byte, deliverBy time.Time, owner ...seededOrder) seededOrder {
	t.Helper()
	issued := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	order := seedPayerOrder(t, db, target, issued, "70", nonce, fixtureSigner(102), 1, owner...)
	hash := crypto.Keccak256Hash([]byte(order.Sale)).Hex()
	fixtureExec(t, db, `SELECT ledger.confirm_order_lock($1,$2,$3,gen_random_uuid(),$4,0,$4,$5,$6,70,$7,1,$8,$9,$10,$11,172800)`, order.Sale, order.Quote.Digest[:],
		target.Stream, hash, issued.Add(5*time.Second), lower(target.Asset), lower(order.Order.Payer), common.Hash(order.Quote.Args.MerchantId).Hex(),
		lower(target.Contract), lower(fixtureSigner(120).Address()), deliverBy.Unix())
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=true WHERE id=$1", order.Sale)
	t.Cleanup(func() {
		fixtureExec(t, db, "UPDATE ledger.order_escrow SET settled=true WHERE sale_id=$1", order.Sale)
		fixtureExec(t, db, "UPDATE ledger.settlement_work w SET finished=true FROM ledger.escrow_action a WHERE a.id=w.action_id AND a.sale_id=$1", order.Sale)
	})
	return order
}

func TestPostgresEscrowDeadlines(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := context.Background()
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x2222"), ABI: EscrowABIHash}
	future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Second)
	waiting := lockedOrder(t, db, target, 61, future)
	undelivered := lockedOrder(t, db, target, 62, past, waiting)
	delivered := lockedOrder(t, db, target, 63, past, waiting)
	openDispute := lockedOrder(t, db, target, 64, past, waiting)
	// A long delivery deadline must not delay the return of an unsettled dispute.
	oldDispute := lockedOrder(t, db, target, 65, future, waiting)
	_, workerDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	worker, err := Open(ctx, workerDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	record := func(order seededOrder, kind string, due time.Time, index int) {
		t.Helper()
		if _, err := worker.db.Exec(ctx, "SELECT ledger.record_escrow_event($1,$2,gen_random_uuid(),$3,$4,NULL,NULL,$5,$6,$5,now())",
			order.Sale, lower(target.Contract), kind, due.Unix(), crypto.Keccak256Hash([]byte(order.Sale)).Hex(), index); err != nil {
			t.Fatal(err)
		}
	}
	for _, order := range []seededOrder{delivered, openDispute, oldDispute} {
		record(order, "delivered", future, 1)
	}
	record(openDispute, "disputed", future, 2)
	record(oldDispute, "disputed", past, 2)
	for range 8 {
		if _, err := worker.db.Exec(ctx, "SELECT * FROM ledger.claim_escrow_follow(ARRAY[$1]::uuid[])", waiting.Branch); err != nil {
			t.Fatal(err)
		}
	}
	var queued string
	if err := db.QueryRow(ctx, `SELECT string_agg(a.kind||'/'||a.sale_id,' ' ORDER BY a.kind) FROM ledger.escrow_action a WHERE a.branch_id=$1`, waiting.Branch).Scan(&queued); err != nil ||
		queued != "refund-dispute/"+oldDispute.Sale+" refund-undelivered/"+undelivered.Sale {
		t.Fatalf("queued %q: %v", queued, err)
	}
	// A delivery that becomes final after all ends the automatic return.
	record(undelivered, "delivered", future, 1)
	var state string
	if err := db.QueryRow(ctx, "SELECT state FROM ledger.escrow_action WHERE sale_id=$1", undelivered.Sale).Scan(&state); err != nil || state != "superseded" {
		t.Fatalf("automatic return is %s: %v", state, err)
	}
}

func TestPostgresOpenOrdersKeepTheirContract(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := context.Background()
	old := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x3333")}
	escrow := Target{Stream: old.Stream, Asset: old.Asset, Contract: common.HexToAddress("0x2222"), ABI: EscrowABIHash}
	_, workerDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	worker, err := Open(ctx, workerDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	order := seedPayerOrder(t, db, old, time.Now().UTC().Truncate(time.Second), "70", 66, fixtureSigner(102), 1)
	cipher, _ := commerce.NewCipher(order.CipherKey[:])
	if worker.CheckOpenContracts(ctx, order.Branch, old, cipher) != nil || worker.CheckOpenContracts(ctx, order.Branch, escrow, cipher) == nil {
		t.Fatal("an open SettlementV2 order was left to an escrow branch")
	}
	fixtureExec(t, db, "UPDATE ledger.sale SET state='closed' WHERE id=$1", order.Sale)
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=true WHERE id=$1", order.Sale)
	if err := worker.CheckOpenContracts(ctx, order.Branch, escrow, cipher); err != nil {
		t.Fatal(err)
	}
	held := lockedOrder(t, db, escrow, 67, time.Now().Add(time.Hour))
	// A refund that still needs the old contract keeps the branch there.
	paid := seedPayerOrder(t, db, old, time.Now().UTC().Truncate(time.Second), "70", 68, fixtureSigner(102), 1)
	fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),$4,0,$4,$5,$6,70,$7,1,1,$8)`, paid.Sale, paid.Quote.Digest[:], old.Stream,
		crypto.Keccak256Hash([]byte(paid.Sale)).Hex(), paid.Order.Issued.Add(time.Second), lower(old.Asset), lower(paid.Order.Payer), common.Hash(paid.Quote.Args.MerchantId).Hex())
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=true WHERE id=$1", paid.Sale)
	if err := worker.CheckOpenContracts(ctx, paid.Branch, escrow, cipher); err != nil {
		t.Fatal(err)
	}
	fixtureExec(t, db, `INSERT INTO ledger.refund(sale_id,request_key,chain_refund_id,amount,requester,requested_at,state,reason_code)
		VALUES($1,gen_random_uuid(),sha256(gen_random_uuid()::text::bytea),1,gen_random_uuid(),now(),'requested','merchant_cancel')`, paid.Sale)
	if worker.CheckOpenContracts(ctx, paid.Branch, escrow, cipher) == nil {
		t.Fatal("an open SettlementV2 refund was left to an escrow branch")
	}
	if worker.CheckOpenContracts(ctx, held.Branch, escrow, cipher) != nil || worker.CheckOpenContracts(ctx, held.Branch, old, cipher) == nil {
		t.Fatal("a held order was left to a SettlementV2 branch")
	}
}

// A nonce whose stored transaction can no longer act must not stop the gas account.
func TestPostgresSpentNonceIsFree(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := context.Background()
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x3333")}
	_, workerDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	worker, err := Open(ctx, workerDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	owner := seedPayerOrder(t, db, target, time.Now().UTC().Truncate(time.Second), "70", 70, fixtureSigner(102), 1)
	for i, c := range []struct {
		name                     string
		expired                  time.Duration
		finished, mined, refund  bool
		pending                  uint64
		free                     bool
		unsigned, action, leased bool
		above, final             bool
	}{
		{name: "closed order, expired", expired: time.Second, finished: true, pending: 5, free: true},
		{name: "closed order, not expired", expired: -time.Minute, finished: true, pending: 5},
		{name: "open order, just expired", expired: time.Second, pending: 5},
		{name: "reconciliation fails for good", expired: 6 * time.Minute, pending: 5, free: true},
		{name: "refund never sent", expired: 6 * time.Minute, refund: true, pending: 5, free: true},
		{name: "refund just expired", expired: time.Second, refund: true, finished: true, pending: 5},
		{name: "the payment itself, behind a lagging node", expired: 6 * time.Minute, finished: true, mined: true, pending: 5},
		{name: "already on chain", expired: -time.Minute, pending: 6, free: true},
		{name: "reserved action without a worker", expired: -time.Hour, unsigned: true, action: true, pending: 5, free: true},
		{name: "reserved action being signed", expired: -time.Hour, unsigned: true, action: true, leased: true, pending: 5},
		{name: "signed action that cannot expire", expired: -time.Hour, action: true, finished: true, pending: 5},
		{name: "finished action on a chain that started over", expired: -time.Hour, action: true, finished: true, final: true, pending: 5, free: true},
		{name: "dead lock under a waiting action", expired: time.Second, finished: true, above: true, pending: 5, free: true},
		{name: "live lock under a waiting action", expired: -time.Minute, above: true, pending: 5},
	} {
		t.Run(c.name, func(t *testing.T) {
			stuck := seedPayerOrder(t, db, target, time.Now().UTC().Truncate(time.Second), "70", byte(71+i), fixtureSigner(102), 1, owner)
			next := seedPayerOrder(t, db, target, time.Now().UTC().Truncate(time.Second), "70", byte(101+i), fixtureSigner(102), 1, owner)
			sender := lower(common.BytesToAddress(crypto.Keccak256([]byte(stuck.Sale))))
			t.Cleanup(func() {
				fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=true,lease_until=NULL WHERE sale_id IN ($1,$2)", stuck.Sale, next.Sale)
			})
			job := stuck.Sale
			hash := crypto.Keccak256Hash([]byte(stuck.Sale)).Hex()
			if c.mined || c.refund {
				tx := hash
				if c.refund {
					tx = crypto.Keccak256Hash([]byte("paid" + stuck.Sale)).Hex()
				}
				fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),$4,0,$4,$5,$6,70,$7,1,1,$8)`, stuck.Sale, stuck.Quote.Digest[:],
					target.Stream, tx, stuck.Order.Issued.Add(time.Second), lower(target.Asset), lower(stuck.Order.Payer), common.Hash(stuck.Quote.Args.MerchantId).Hex())
			}
			if c.refund {
				fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=true WHERE id=$1", stuck.Sale)
				fixtureExec(t, db, `INSERT INTO ledger.refund(id,sale_id,request_key,chain_refund_id,amount,requester,approver,requested_at,state,reason_code)
					VALUES($1::uuid,$1::uuid,gen_random_uuid(),sha256($1::uuid::text::bytea),1,gen_random_uuid(),gen_random_uuid(),now(),'inflight','merchant_cancel')`, stuck.Sale)
				job = fixtureUUID(t, db)
				fixtureExec(t, db, "INSERT INTO ledger.settlement_work(id,sale_id,refund_id) VALUES($1,$2,$2)", job, stuck.Sale)
			}
			if c.action {
				job = fixtureUUID(t, db)
				fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=true WHERE id=$1", stuck.Sale)
				fixtureExec(t, db, "INSERT INTO ledger.escrow_action(id,branch_id,kind) VALUES($1,$2,'claim')", job, stuck.Branch)
				fixtureExec(t, db, "INSERT INTO ledger.settlement_work(id,action_id) VALUES($1,$1)", job)
			}
			fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=$2,lease_until=CASE WHEN $3 THEN now()+interval '20 seconds' END WHERE id=$1", job, c.finished, c.leased)
			fixtureExec(t, db, "INSERT INTO ledger.relay_attempt(work_id,chain_id,sender,nonce,intent_digest,expires_at) VALUES($1,8283,$2,5,sha256('x'),$3)",
				job, sender, time.Now().Add(-c.expired))
			if !c.unsigned {
				fixtureExec(t, db, "UPDATE ledger.relay_attempt SET tx_hash=$2,tx_cipher=decode(repeat('00',40),'hex'),state='signed' WHERE work_id=$1", job, hash)
			}
			if c.final {
				fixtureExec(t, db, "UPDATE ledger.relay_attempt SET state='final' WHERE work_id=$1", job)
			}
			if c.above {
				higher := fixtureUUID(t, db)
				fixtureExec(t, db, "INSERT INTO ledger.escrow_action(id,branch_id,kind) VALUES($1,$2,'claim')", higher, stuck.Branch)
				fixtureExec(t, db, "INSERT INTO ledger.settlement_work(id,action_id,finished) VALUES($1,$1,true)", higher)
				fixtureExec(t, db, `INSERT INTO ledger.relay_attempt(work_id,chain_id,sender,nonce,intent_digest,expires_at,tx_hash,tx_cipher,state,broadcast_epoch,broadcast_fence)
					VALUES($1,8283,$2,6,sha256('y'),now()+interval '10 years',$3,decode(repeat('00',40),'hex'),'broadcast',repeat('a',64),1)`,
					higher, sender, crypto.Keccak256Hash([]byte(higher)).Hex())
			}
			fixtureExec(t, db, "UPDATE ledger.settlement_work SET fence=7,lease_until=now()+interval '20 seconds' WHERE id=$1", next.Sale)
			nonce, err := worker.reserve(ctx, work{id: next.Sale, fence: 7}, sender, c.pending, sha256.Sum256([]byte("next")), time.Now().Add(time.Minute))
			if c.free && (err != nil || nonce != c.pending) {
				t.Fatalf("the gas account stays blocked: %v", err)
			}
			if !c.free && service.CodeOf(sqlError(err)) != service.Busy {
				t.Fatalf("a nonce that may still act was given away: %d %v", nonce, err)
			}
		})
	}
}

func TestPostgresPayerAccount(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := context.Background()
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x2222"), ABI: EscrowABIHash}
	_, commerceDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	seller, err := Open(ctx, commerceDSN, "nu54_p05_commerce")
	if err != nil {
		t.Fatal(err)
	}
	defer seller.Close()
	board := fixtureSigner(140)
	order := seedPayerOrder(t, db, target, time.Now().UTC().Truncate(time.Second), "70", 81, board, 1)
	intent := func(label string) []byte { hash := sha256.Sum256([]byte(label)); return hash[:] }
	enroll := func(key, label string, payer common.Address, generation int64) error {
		_, err := seller.db.Exec(ctx, "SELECT ledger.enroll_payer($1,$2,$3,$4,$5,$6,$7)", order.Tenant, order.Branch, order.Actor, key, intent(label), payer[:], generation)
		return sqlError(err)
	}
	first := fixtureUUID(t, db)
	if err := enroll(first, "a", board.Address(), 1); err != nil {
		t.Fatal(err)
	}
	var expires, until time.Time
	if err := db.QueryRow(ctx, `SELECT e.expires_at,g.valid_until FROM ledger.payer_enrollment e JOIN ledger.commerce_grant g USING (branch_id,actor_id)
		WHERE e.branch_id=$1 AND e.actor_id=$2`, order.Branch, order.Actor).Scan(&expires, &until); err != nil || !expires.Equal(until) {
		t.Fatalf("enrollment outlives the customer's access: %s %s %v", expires, until, err)
	}
	fixtureExec(t, db, "UPDATE ledger.commerce_grant SET valid_until=now()+interval '1 year' WHERE branch_id=$1", order.Branch)
	if err := enroll(fixtureUUID(t, db), "renew", board.Address(), 1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT expires_at FROM ledger.payer_enrollment WHERE actor_id=$1", order.Actor).Scan(&expires); err != nil ||
		expires.After(time.Now().Add(30*24*time.Hour)) || expires.Before(time.Now().Add(29*24*time.Hour)) {
		t.Fatalf("enrollment ends %s: %v", expires, err)
	}
	next := fixtureSigner(141).Address()
	if err := enroll(fixtureUUID(t, db), "b", next, 1); service.CodeOf(err) != service.CheckoutConflict {
		t.Fatalf("the account changed during an order: %v", err)
	}
	if err := enroll(first, "other", board.Address(), 1); service.CodeOf(err) != service.Conflict {
		t.Fatalf("a key was used twice: %v", err)
	}
	nonces := 0
	store := func(actor, key, label string, payer common.Address, valid time.Duration, sponsored any) error {
		nonces++
		nonce := crypto.Keccak256([]byte(fmt.Sprint(order.Sale, nonces)))
		_, err := seller.db.Exec(ctx, "SELECT ledger.store_wallet_return($1,$2,$3,$4,$5,$6,$7,$8,10,$9,$10,$11,$12)", order.Tenant, order.Branch, actor, key,
			intent(label), payer[:], next[:], lower(target.Asset), time.Now().Add(valid).Unix(), nonce, make([]byte, 93), sponsored)
		return sqlError(err)
	}
	paidGas := time.Now().Add(time.Hour)
	refuse := func(name string, want service.Code, err error) {
		t.Helper()
		if service.CodeOf(err) != want {
			t.Fatalf("%s: %v", name, err)
		}
	}
	refuse("open order", service.CheckoutConflict, store(order.Actor, fixtureUUID(t, db), "r", board.Address(), time.Minute, paidGas))
	fixtureExec(t, db, "UPDATE ledger.sale SET state='closed' WHERE id=$1", order.Sale)
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=true WHERE id=$1", order.Sale)
	refuse("another account", service.Mismatch, store(order.Actor, fixtureUUID(t, db), "r", next, time.Minute, paidGas))
	refuse("expired", service.Expired, store(order.Actor, fixtureUUID(t, db), "r", board.Address(), -time.Minute, paidGas))
	if err := store(order.Actor, fixtureUUID(t, db), "r", board.Address(), 11*time.Minute, paidGas); err == nil {
		t.Fatal("a return valid for more than ten minutes was stored")
	}
	refuse("no sponsorship", service.Capability, store(order.Actor, fixtureUUID(t, db), "r", board.Address(), time.Minute, nil))
	other := fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.commerce_grant VALUES($1,$2,'customer',NULL,now()+interval '1 hour')", order.Branch, other)
	refuse("another customer", service.Mismatch, store(other, fixtureUUID(t, db), "r", board.Address(), time.Minute, paidGas))
	key := fixtureUUID(t, db)
	if err := store(order.Actor, key, "one", board.Address(), time.Minute, paidGas); err != nil {
		t.Fatal(err)
	}
	if err := store(order.Actor, key, "one", board.Address(), time.Minute, paidGas); err != nil {
		t.Fatalf("the same request again: %v", err)
	}
	refuse("same key, another request", service.Conflict, store(order.Actor, key, "two", board.Address(), time.Minute, paidGas))
	refuse("second return", service.Conflict, store(order.Actor, fixtureUUID(t, db), "two", board.Address(), time.Minute, paidGas))
	refuse("account change during a return", service.CheckoutConflict, enroll(fixtureUUID(t, db), "c", next, 1))
	if _, err := db.Exec(ctx, `INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,profile_version,merchant_signer,payer,
		payer_generation,payer_role,issued_at,expires_at,fence,lease_until)
		SELECT gen_random_uuid(),actor_id,claim_digest,sha256('q'),sha256('o'),sha256('n'),0,1,merchant_signer,payer,1,1,issued_at,expires_at,1,expires_at
		FROM ledger.quote_work WHERE sale_id=$1`, order.Sale); service.CodeOf(sqlError(err)) != service.CheckoutConflict {
		t.Fatalf("a quote was issued while the account is being emptied: %v", err)
	}
	for i := range 3 {
		fixtureExec(t, db, "UPDATE ledger.escrow_action SET state='failed',closed_at=now() WHERE requester=$1 AND state='queued'", order.Actor)
		err := store(order.Actor, fixtureUUID(t, db), fmt.Sprint(i), board.Address(), time.Minute, paidGas)
		if i < 2 && err != nil || i == 2 && service.CodeOf(err) != service.Busy {
			t.Fatalf("return %d: %v", i+2, err)
		}
	}
	fixtureExec(t, db, "UPDATE ledger.escrow_action SET state='failed',closed_at=now() WHERE requester=$1 AND state='queued'", order.Actor)
	fixtureExec(t, db, "UPDATE ledger.settlement_work w SET finished=true FROM ledger.escrow_action a WHERE a.id=w.action_id AND a.requester=$1", order.Actor)
	if err := enroll(fixtureUUID(t, db), "d", next, 2); err != nil {
		t.Fatalf("a free customer cannot change the account: %v", err)
	}
	// Repeating the first accepted request does not bring the old account back.
	if err := enroll(first, "a", board.Address(), 1); err != nil {
		t.Fatal(err)
	}
	var payer []byte
	if err := db.QueryRow(ctx, "SELECT payer FROM ledger.payer_enrollment WHERE branch_id=$1 AND actor_id=$2", order.Branch, order.Actor).Scan(&payer); err != nil ||
		common.BytesToAddress(payer) != next {
		t.Fatalf("a replay restored the old account: %v", err)
	}
	fixtureExec(t, db, "UPDATE ledger.payer_enrollment SET payer_role=2 WHERE branch_id=$1 AND actor_id=$2", order.Branch, order.Actor)
	refuse("rented account", service.CheckoutConflict, enroll(fixtureUUID(t, db), "e", next, 2))
	// Another customer who proves the same board takes it over, unless something of the first is open on it.
	fixtureExec(t, db, "UPDATE ledger.payer_enrollment SET payer_role=1 WHERE branch_id=$1 AND actor_id=$2", order.Branch, order.Actor)
	shared := func(actor, label string) error {
		_, err := seller.db.Exec(ctx, "SELECT ledger.enroll_payer($1,$2,$3,$4,$5,$6,3)", order.Tenant, order.Branch, actor, fixtureUUID(t, db), intent(label), next[:])
		return sqlError(err)
	}
	fixtureExec(t, db, "UPDATE ledger.quote_work SET payer=$2 WHERE sale_id=$1", order.Sale, next[:])
	fixtureExec(t, db, "UPDATE ledger.sale SET state='checking' WHERE id=$1", order.Sale)
	refuse("another customer's open order", service.CheckoutConflict, shared(other, "s1"))
	fixtureExec(t, db, "UPDATE ledger.sale SET state='closed' WHERE id=$1", order.Sale)
	if err := shared(other, "s2"); err != nil {
		t.Fatal(err)
	}
	var holder string
	if err := db.QueryRow(ctx, "SELECT string_agg(actor_id::text,',') FROM ledger.payer_enrollment WHERE payer=$1 AND expires_at>now()", next[:]).Scan(&holder); err != nil || holder != other {
		t.Fatalf("account held by %s: %v", holder, err)
	}
	// A POS order has no kitchen handover, so it cannot be paid into escrow.
	for origin, want := range map[string]bool{"pos": false, "menu": true} {
		fixtureExec(t, db, "UPDATE ledger.sale SET origin=$2 WHERE id=$1", order.Sale, origin)
		var allowed bool
		if err := seller.db.QueryRow(ctx, "SELECT ledger.order_escrow_allowed($1,$2,$3,$4,$5)", order.Tenant, order.Branch, order.Actor, order.Sale, order.Order.Claim[:]).
			Scan(&allowed); err != nil || allowed != want {
			t.Fatalf("%s order allowed=%t: %v", origin, allowed, err)
		}
	}
}

// Whichever price of a relay transaction is included becomes the attempt.
func TestPostgresRelayVersions(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := context.Background()
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x3333")}
	order := seedPayerOrder(t, db, target, time.Now().UTC().Truncate(time.Second), "70", 82, fixtureSigner(102), 1)
	_, workerDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	worker, err := Open(ctx, workerDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	hash := func(i int) string { return crypto.Keccak256Hash([]byte(fmt.Sprint(order.Sale, i))).Hex() }
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET fence=3,lease_until=now()+interval '20 seconds' WHERE id=$1", order.Sale)
	t.Cleanup(func() {
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET finished=true,lease_until=NULL WHERE id=$1", order.Sale)
	})
	fixtureExec(t, db, `INSERT INTO ledger.relay_attempt(work_id,chain_id,sender,nonce,intent_digest,expires_at,tx_hash,tx_cipher,state,broadcast_epoch,broadcast_fence)
		VALUES($1,8283,$2,0,sha256('x'),now()+interval '1 minute',$3,decode(repeat('01',40),'hex'),'broadcast',repeat('a',64),2)`,
		order.Sale, lower(common.BytesToAddress(crypto.Keccak256([]byte(order.Sale)))), hash(1))
	replace := func(old, next int, fence int64) error {
		_, err := worker.db.Exec(ctx, "SELECT ledger.replace_signed_relay($1,$2,$3,$4,decode(repeat('02',40),'hex'))", order.Sale, fence, hash(old), hash(next))
		return err
	}
	if replace(1, 2, 2) == nil || replace(9, 2, 3) == nil {
		t.Fatal("a stale worker replaced the transaction")
	}
	if err := replace(1, 2, 3); err != nil {
		t.Fatal(err)
	}
	var sent string
	if err := db.QueryRow(ctx, "SELECT ledger.relay_sent_hash($1)", order.Sale).Scan(&sent); err != nil || sent != hash(1) {
		t.Fatalf("a stored higher price undid the submission: %s %v", sent, err)
	}
	// Seen in a block that vanished: it can be sent and replaced again.
	fixtureExec(t, db, "UPDATE ledger.relay_attempt SET state='observed' WHERE work_id=$1", order.Sale)
	if _, err := worker.db.Exec(ctx, "SELECT ledger.authorize_relay_send($1,3,$2,repeat('b',64))", order.Sale, hash(2)); err != nil {
		t.Fatal(err)
	}
	fixtureExec(t, db, "UPDATE ledger.relay_attempt SET state='reverted' WHERE work_id=$1", order.Sale)
	if err := replace(2, 3, 3); err != nil {
		t.Fatal(err)
	}
	var versions []string
	if err := worker.db.QueryRow(ctx, "SELECT ledger.relay_versions($1,3)", order.Sale).Scan(&versions); err != nil || len(versions) != 2 {
		t.Fatalf("versions %v: %v", versions, err)
	}
	// The first price is the one that was paid.
	fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),$4,0,$4,$5,$6,70,$7,1,1,$8)`, order.Sale, order.Quote.Digest[:], target.Stream,
		hash(1), order.Order.Issued.Add(time.Second), lower(target.Asset), lower(order.Order.Payer), common.Hash(order.Quote.Args.MerchantId).Hex())
	var current string
	var kept int
	if err := db.QueryRow(ctx, "SELECT tx_hash,(SELECT count(*) FROM ledger.relay_version WHERE work_id=$1) FROM ledger.relay_attempt WHERE work_id=$1", order.Sale).
		Scan(&current, &kept); err != nil || current != hash(1) || kept != 2 {
		t.Fatalf("the paid transaction is %s with %d others: %v", current, kept, err)
	}
	fixtureExec(t, db, "UPDATE ledger.relay_attempt SET expires_at=now()-interval '1 second' WHERE work_id=$1", order.Sale)
	if replace(1, 4, 3) == nil {
		t.Fatal("an expired authorization was signed again")
	}
}
