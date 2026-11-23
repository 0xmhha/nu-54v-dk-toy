package chainview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"testing"
	"time"

	escrow "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/orderescrow"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	orderMerchant    = common.Hash{31: 7}
	orderPayer       = common.HexToAddress("0xaa01")
	orderBeneficiary = common.HexToAddress("0xaa02")
	orderConfirmer   = common.HexToAddress("0xaa03")
)

func orderProfile() Profile {
	return Profile{Version: 3, ChainID: 8283, Contract: common.HexToAddress("0xe5c0"), Asset: common.HexToAddress("0xbeef"),
		ABIHash: escrowABIHash}
}

func orderLog(t *testing.T, profile Profile, name string, topics []common.Hash, values ...any) types.Log {
	t.Helper()
	contract, err := escrow.OrderescrowMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	event := contract.Events[name]
	data, err := event.Inputs.NonIndexed().Pack(values...)
	if err != nil {
		t.Fatal(err)
	}
	return types.Log{Address: profile.Contract, Topics: append([]common.Hash{event.ID}, topics...), Data: data,
		TxHash: common.Hash{31: 1}, BlockHash: common.Hash{31: 2}, Index: 4}
}

func TestDecodeOrderEscrowEvents(t *testing.T) {
	raw, err := os.ReadFile("../../../../packages/contracts-abi/abi/OrderEscrow.json")
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != escrowABIHash {
		t.Fatal("pinned ABI differs from the published OrderEscrow ABI")
	}
	profile := orderProfile()
	decoder, err := NewDecoder(profile)
	if err != nil {
		t.Fatal(err)
	}
	order := common.Hash{31: 21}
	ids := []common.Hash{orderMerchant, order}
	amount := big.NewInt(6_500_000)
	locked := orderLog(t, profile, "OrderLocked", append(ids[:2:2], common.BytesToHash(orderPayer[:])),
		orderBeneficiary, orderConfirmer, amount, uint64(1900000000), uint32(3600), [32]byte{31: 9})
	released := orderLog(t, profile, "OrderReleased", ids, orderBeneficiary, amount, uint8(1))
	cancelled := orderLog(t, profile, "OrderCancelled", ids, orderPayer, amount, uint8(0))
	skipped := orderLog(t, profile, "ClaimSkipped", ids, uint8(6))
	decode := func(log types.Log) Event {
		t.Helper()
		event, err := decoder.Decode(log, types.ReceiptStatusSuccessful)
		if err != nil {
			t.Fatal(err)
		}
		if event.Merchant != orderMerchant.Hex() || event.Order != order.Hex() || event.Index != 4 {
			t.Fatalf("wrong order identity: %+v", event)
		}
		return event
	}
	if e := decode(locked); e.Kind != "locked" || e.Payer != lower(orderPayer) || e.Recipient != lower(orderBeneficiary) ||
		e.Confirmer != lower(orderConfirmer) || e.Amount != "6500000" || e.Asset != lower(profile.Asset) ||
		e.DeliverBy != 1900000000 || e.ConfirmWindow != 3600 || e.QuoteDigest != (common.Hash{31: 9}).Hex() {
		t.Fatalf("wrong lock: %+v", e)
	}
	if e := decode(orderLog(t, profile, "OrderDelivered", ids, uint64(1800))); e.Kind != "delivered" || e.ClaimableAt != 1800 {
		t.Fatalf("wrong delivery: %+v", e)
	}
	if e := decode(orderLog(t, profile, "OrderDisputed", ids, uint64(1900))); e.Kind != "disputed" || e.ResolveBy != 1900 {
		t.Fatalf("wrong dispute: %+v", e)
	}
	if e := decode(released); e.Kind != "released" || e.Recipient != lower(orderBeneficiary) || e.Amount != "6500000" || *e.Reason != 1 {
		t.Fatalf("wrong release: %+v", e)
	}
	if e := decode(cancelled); e.Kind != "cancelled" || e.Payer != lower(orderPayer) || *e.Reason != 0 {
		t.Fatalf("wrong cancellation: %+v", e)
	}
	if e := decode(skipped); e.Kind != "claim-skipped" || *e.Reason != 6 || e.Amount != "" {
		t.Fatalf("wrong skipped claim: %+v", e)
	}

	with := func(log types.Log, change func(*types.Log)) types.Log {
		log.Topics, log.Data = append([]common.Hash(nil), log.Topics...), append([]byte(nil), log.Data...)
		change(&log)
		return log
	}
	for name, log := range map[string]types.Log{
		"wrong address":        with(locked, func(l *types.Log) { l.Address = common.HexToAddress("0x1234") }),
		"removed":              with(locked, func(l *types.Log) { l.Removed = true }),
		"missing transaction":  with(locked, func(l *types.Log) { l.TxHash = common.Hash{} }),
		"missing block":        with(locked, func(l *types.Log) { l.BlockHash = common.Hash{} }),
		"missing topic":        with(locked, func(l *types.Log) { l.Topics = l.Topics[:3] }),
		"extra topic":          with(released, func(l *types.Log) { l.Topics = append(l.Topics, common.Hash{31: 1}) }),
		"zero merchant":        with(locked, func(l *types.Log) { l.Topics[1] = common.Hash{} }),
		"zero order":           with(released, func(l *types.Log) { l.Topics[2] = common.Hash{} }),
		"wide payer":           with(locked, func(l *types.Log) { l.Topics[3][0] = 1 }),
		"zero payer":           with(locked, func(l *types.Log) { l.Topics[3] = common.Hash{} }),
		"wide beneficiary":     with(locked, func(l *types.Log) { l.Data[0] = 1 }),
		"zero confirmer":       with(locked, func(l *types.Log) { copy(l.Data[32:64], make([]byte, 32)) }),
		"zero amount":          with(locked, func(l *types.Log) { copy(l.Data[64:96], make([]byte, 32)) }),
		"wide deadline":        with(locked, func(l *types.Log) { l.Data[96] = 1 }),
		"deadline past range":  with(locked, func(l *types.Log) { l.Data[120] = 0x80 }),
		"zero window":          with(locked, func(l *types.Log) { copy(l.Data[128:160], make([]byte, 32)) }),
		"zero quote":           with(locked, func(l *types.Log) { copy(l.Data[160:192], make([]byte, 32)) }),
		"trailing data":        with(locked, func(l *types.Log) { l.Data = append(l.Data, make([]byte, 32)...) }),
		"short data":           with(released, func(l *types.Log) { l.Data = l.Data[:64] }),
		"unknown payout":       with(released, func(l *types.Log) { l.Data[95] = 4 }),
		"wide payout":          with(released, func(l *types.Log) { l.Data[64] = 1 }),
		"unknown refund":       with(cancelled, func(l *types.Log) { l.Data[95] = 4 }),
		"no skip reason":       with(skipped, func(l *types.Log) { l.Data[31] = 0 }),
		"unknown skip reason":  with(skipped, func(l *types.Log) { l.Data[31] = 7 }),
		"settings event":       with(skipped, func(l *types.Log) { l.Topics[0] = crypto.Keccak256Hash([]byte("ArbiterChanged(address)")) }),
		"skipped zero order":   with(skipped, func(l *types.Log) { l.Topics[2] = common.Hash{} }),
		"delivered without at": orderLog(t, profile, "OrderDelivered", ids, uint64(0)),
	} {
		if event, err := decoder.Decode(log, types.ReceiptStatusSuccessful); err == nil {
			t.Fatalf("%s accepted: %+v", name, event)
		}
	}
	if _, err := decoder.Decode(locked, types.ReceiptStatusFailed); err == nil {
		t.Fatal("reverted receipt accepted")
	}
	// Anyone may claim zero ids. The log is ignored and does not stop the stream, unlike a malformed lock.
	if decoder.matches(with(skipped, func(l *types.Log) { l.Topics[1] = common.Hash{} })) ||
		!decoder.matches(with(locked, func(l *types.Log) { l.Topics[1] = common.Hash{} })) {
		t.Fatal("zero-id claim and malformed lock handled alike")
	}
	modern := profile
	modern.Version, modern.ABIHash = 2, v2ABIHash
	settlement, err := NewDecoder(modern)
	if err != nil {
		t.Fatal(err)
	}
	if settlement.matches(locked) {
		t.Fatal("order event accepted under the settlement profile")
	}
	modern.ABIHash = escrowABIHash
	if _, err := NewDecoder(modern); !errors.Is(err, ErrUnknownABI) {
		t.Fatalf("version and ABI mismatch accepted: %v", err)
	}
}

func TestOrderLockQuoteWindow(t *testing.T) {
	profile := orderProfile()
	decoder, err := NewDecoder(profile)
	if err != nil {
		t.Fatal(err)
	}
	quote := escrow.PaymentTypesV2MerchantQuote{
		QuoteId: common.Hash{31: 2}, TenantCommitment: common.Hash{31: 3}, BranchCommitment: common.Hash{31: 4},
		OrderId: common.Hash{31: 5}, MerchantId: orderMerchant, ClaimCommitment: common.Hash{31: 8}, IntentCommitment: common.Hash{31: 9},
		Payer: orderPayer, Asset: profile.Asset, Beneficiary: orderBeneficiary, Amount: big.NewInt(6_500_000), Nonce: big.NewInt(11),
		MerchantKeyVersion: 1, BeneficiaryVersion: 1, PayerGeneration: 1, IssuedAt: 1000, ExpiresAt: 1120, Mode: 1, PayerRole: 1,
	}
	contract, err := escrow.OrderescrowMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	bind := func(q escrow.PaymentTypesV2MerchantQuote, to common.Address, at int64, digest string) (Event, error) {
		t.Helper()
		data, err := contract.Pack("lock", q, orderConfirmer, make([]byte, 65), make([]byte, 65))
		if err != nil {
			t.Fatal(err)
		}
		event := Event{Kind: "locked", QuoteDigest: digest}
		err = decoder.bindQuoteWindow(&event, types.NewTx(&types.LegacyTx{To: &to, Data: data}), time.Unix(at, 0))
		return event, err
	}
	digest := orderQuoteDigest(t, profile, quote)
	if event, err := bind(quote, profile.Contract, 1119, digest); err != nil || event.IssuedAt == nil ||
		event.IssuedAt.Unix() != 1000 || event.ExpiresAt.Unix() != 1120 {
		t.Fatalf("direct lock window: %+v %v", event, err)
	}
	for _, at := range []int64{999, 1120} {
		if _, err := bind(quote, profile.Contract, at, digest); err == nil {
			t.Fatalf("lock outside its quote accepted at %d", at)
		}
	}
	if _, err := bind(quote, profile.Contract, 1100, (common.Hash{31: 1}).Hex()); err == nil {
		t.Fatal("different quote digest accepted")
	}
	if event, err := bind(quote, common.HexToAddress("0x9999"), 1100, digest); err != nil || event.IssuedAt != nil {
		t.Fatalf("lock through another contract: %+v %v", event, err)
	}
	odd := quote
	odd.PayerRole = 9 // the contract does not check it
	if event, err := bind(odd, profile.Contract, 1100, (common.Hash{31: 1}).Hex()); err != nil || event.IssuedAt != nil {
		t.Fatalf("quote outside the SDK form stopped the stream: %+v %v", event, err)
	}
	endless := quote
	endless.ExpiresAt = 1 << 63
	if event, err := bind(endless, profile.Contract, 1100, orderQuoteDigest(t, profile, endless)); err != nil || event.IssuedAt != nil {
		t.Fatalf("endless quote stopped the stream: %+v %v", event, err)
	}
}

// orderQuoteDigest is written out from the EIP-712 types in PaymentTypesV2.sol.
func orderQuoteDigest(t *testing.T, profile Profile, quote escrow.PaymentTypesV2MerchantQuote) string {
	t.Helper()
	contract, err := escrow.OrderescrowMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	fields, err := (abi.Arguments{contract.Methods["lock"].Inputs[0]}).Pack(quote)
	if err != nil {
		t.Fatal(err)
	}
	separator := crypto.Keccak256(v1DomainType, v1DomainName, crypto.Keccak256([]byte("2")),
		common.LeftPadBytes(big.NewInt(8283).Bytes(), 32), common.LeftPadBytes(profile.Contract[:], 32))
	quoteType := crypto.Keccak256([]byte("MerchantQuote(bytes32 quoteId,bytes32 tenantCommitment,bytes32 branchCommitment," +
		"bytes32 orderId,bytes32 merchantId,bytes32 claimCommitment,bytes32 intentCommitment,address payer,address asset," +
		"address beneficiary,uint256 amount,uint256 nonce,uint64 merchantKeyVersion,uint64 beneficiaryVersion," +
		"uint64 payerGeneration,uint64 issuedAt,uint64 expiresAt,uint8 mode,uint8 payerRole)"))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, separator, crypto.Keccak256(quoteType, fields)).Hex()
}

type orderFact struct {
	kind, payer, beneficiary, confirmer, amount, quote string
	deliverBy, window, claimableAt, resolveBy          int64
	reason                                             int16
	tx, block, contract, abi, policy                   string
	height                                             int64
	index                                              int32
	included                                           time.Time
}

// orderFacts reads what the operations server reads, with its restricted login.
func orderFacts(t *testing.T, db *pgx.Conn, stream, merchant, order string) []orderFact {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT kind,COALESCE(payer_ref,''),COALESCE(beneficiary_ref,''),COALESCE(confirmer_ref,''),
		COALESCE(amount::text,''),COALESCE(quote_digest,''),COALESCE(deliver_by,0),COALESCE(confirm_window,0),COALESCE(claimable_at,0),
		COALESCE(resolve_by,0),COALESCE(reason,-1::smallint),tx_hash,inclusion_hash,contract_ref,abi_hash,policy_ref,block_height,log_index,included_at
		FROM chainview.escrow_receipt($1,$2,$3) WHERE id IS NOT NULL AND chain_ref='8283'`, stream, merchant, order)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (orderFact, error) {
		var f orderFact
		err := row.Scan(&f.kind, &f.payer, &f.beneficiary, &f.confirmer, &f.amount, &f.quote, &f.deliverBy, &f.window,
			&f.claimableAt, &f.resolveBy, &f.reason, &f.tx, &f.block, &f.contract, &f.abi, &f.policy, &f.height, &f.index, &f.included)
		return f, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func orderReader(t *testing.T, url string, owner *pgxpool.Pool, stream, merchant string) (*pgx.Conn, string) {
	t.Helper()
	ctx := context.Background()
	role, login := fixtureLogin(t, url, owner, "nu54_p07_reconcile")
	if _, err := owner.Exec(ctx, "INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '1 hour')",
		role, stream, merchant); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, login)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	return conn, role
}

func TestPostgresOrderEscrowFacts(t *testing.T) {
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
	const stream = "b0cabaf8-23e5-43d8-b73e-362fa0fb1501"
	const generation = "b0cabaf8-23e5-43d8-b73e-362fa0fb1502"
	profile := orderProfile()
	profile.StreamID, profile.GenerationID = stream, generation
	profile.Manifest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab1501"
	profile.StartHeight, profile.StartHash, profile.CodeHash = 1, common.Hash{31: 101}, common.Hash{31: 99}
	_, login := fixtureLogin(t, url, db, "nu54_p07_ingest")
	repository, err := Open(ctx, login)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Register(ctx, profile); err != nil {
		t.Fatal(err)
	}
	merchant, order := orderMerchant.Hex(), (common.Hash{31: 21}).Hex()
	event := func(kind string, index int32) Event {
		return Event{Kind: kind, Merchant: merchant, Order: order, TxHash: (common.Hash{31: 50}).Hex(), Index: index}
	}
	lock := event("locked", 0)
	lock.Asset, lock.Amount, lock.Payer, lock.Recipient = lower(profile.Asset), "6500000", lower(orderPayer), lower(orderBeneficiary)
	lock.Confirmer, lock.QuoteDigest, lock.DeliverBy, lock.ConfirmWindow = lower(orderConfirmer), (common.Hash{31: 23}).Hex(), 1800007200, 3600
	issued, expires := time.Unix(1800000000, 0).UTC(), time.Unix(1800000002, 0).UTC()
	lock.IssuedAt, lock.ExpiresAt = &issued, &expires
	delivered := event("delivered", 0)
	delivered.ClaimableAt = 1800003602
	// One transaction may carry many facts about one order; the log index orders them.
	disputed, withdrawn, skipped := event("disputed", 3), event("released", 4), event("claim-skipped", 1)
	disputed.ResolveBy = 1801209602
	withdrawn.Asset, withdrawn.Amount, withdrawn.Recipient, withdrawn.Reason = lower(profile.Asset), "6500000", lower(orderBeneficiary), reason(3)
	skipped.Reason = reason(3)
	refunded := event("cancelled", 9)
	refunded.Order = (common.Hash{31: 24}).Hex()
	refunded.Asset, refunded.Amount, refunded.Payer, refunded.Reason = lower(profile.Asset), "10", lower(orderPayer), reason(1)

	block1, block2, block3, block4 := sampleBlock(1, 101, 0), sampleBlock(2, 102, 101), sampleBlock(3, 103, 102), sampleBlock(4, 104, 103)
	block2.Events = []Event{lock}
	block3.Events = []Event{withdrawn, disputed, skipped, refunded}
	block3.Events = append(block3.Events, func() Event { d := delivered; d.Index = 2; return d }())
	source := &fakeChain{blocks: map[int64]Block{1: block1, 2: block2}, final: 1}
	reader := &Reader{Profile: profile, Source: source, DB: repository}
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	activate(t, db, stream, generation)
	server, role := orderReader(t, url, db, stream, merchant)
	if facts := orderFacts(t, server, stream, merchant, order); len(facts) != 0 {
		t.Fatalf("unconfirmed lock exposed: %+v", facts)
	}
	source.blocks[3], source.blocks[4], source.final = block3, block4, 4
	if err := reader.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	facts := orderFacts(t, server, stream, merchant, order)
	var kinds []string
	for _, f := range facts {
		kinds = append(kinds, f.kind)
		if f.contract != lower(profile.Contract) || f.abi != escrowABIHash || f.policy != profile.Manifest {
			t.Fatalf("wrong deployment identity: %+v", f)
		}
	}
	if len(kinds) != 5 || kinds[0] != "locked" || kinds[1] != "claim-skipped" || kinds[2] != "delivered" ||
		kinds[3] != "disputed" || kinds[4] != "released" {
		t.Fatalf("wrong fact order: %v", kinds)
	}
	if f := facts[0]; f.payer != lock.Payer || f.beneficiary != lock.Recipient || f.confirmer != lock.Confirmer || f.amount != "6500000" ||
		f.quote != lock.QuoteDigest || f.deliverBy != lock.DeliverBy || f.window != 3600 || f.claimableAt != 0 || f.reason != -1 ||
		f.height != 2 || f.block != block2.Hash.Hex() || !f.included.Equal(block2.Time) {
		t.Fatalf("wrong lock fact: %+v", f)
	}
	if f := facts[1]; f.reason != 3 || f.amount != "" || f.beneficiary != "" {
		t.Fatalf("wrong skipped claim: %+v", f)
	}
	if facts[2].claimableAt != delivered.ClaimableAt || facts[3].resolveBy != disputed.ResolveBy ||
		facts[4].reason != 3 || facts[4].beneficiary != lower(orderBeneficiary) || facts[4].payer != "" {
		t.Fatalf("wrong later facts: %+v", facts[2:])
	}
	if f := orderFacts(t, server, stream, merchant, refunded.Order); len(f) != 1 || f[0].kind != "cancelled" ||
		f[0].payer != lower(orderPayer) || f[0].beneficiary != "" || f[0].reason != 1 {
		t.Fatalf("wrong cancellation: %+v", f)
	}

	// Scope and other readers.
	if f := orderFacts(t, server, stream, (common.Hash{31: 8}).Hex(), order); len(f) != 0 {
		t.Fatalf("ungranted merchant readable: %+v", f)
	}
	if _, err := server.Exec(ctx, "SELECT * FROM chainview.current_evidence"); err == nil {
		t.Fatal("server login reads past its scoped function")
	}
	for _, group := range []string{"nu54_p07_public", "nu54_p07_explorer", "nu54_p07_read", "nu54_p07_ingest"} {
		_, other := fixtureLogin(t, url, db, group)
		conn, err := pgx.Connect(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, "SELECT * FROM chainview.escrow_receipt($1,$2,$3)", stream, merchant, order); err == nil {
			t.Fatalf("%s reads escrow facts", group)
		}
		conn.Close(ctx)
	}
	for _, view := range []string{"finalized_receipt", "settlement_receipt", "explorer_receipt_detail", "explorer_boundary"} {
		var count int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM chainview."+view+" WHERE stream_id=$1", stream).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s exposes escrow facts: %d %v", view, count, err)
		}
	}
	var ready bool
	if err := server.QueryRow(ctx, "SELECT chainview.reconciliation_ready($1,$2)", stream, merchant).Scan(&ready); err != nil || !ready {
		t.Fatalf("escrow stream not ready: %v %v", ready, err)
	}

	// Incomplete or mixed facts cannot be stored.
	for _, set := range []string{"confirmer_ref=NULL", "deliver_by=NULL", "amount=0", "claimable_at=1", "reason=0", "kind='delivered'", "payer_generation=1"} {
		if _, err := db.Exec(ctx, "UPDATE chainview.observed_event SET "+set+" WHERE generation_id=$1 AND kind='locked'", generation); err == nil {
			t.Fatalf("invalid escrow row accepted: %s", set)
		}
	}

	// Absence: an observed lock, consumed state, or a block before expiry all refuse it.
	unlocked := (common.Hash{31: 30}).Hex()
	if _, err := reader.ProveAbsence(ctx, merchant, order, expires); err == nil {
		t.Fatal("proved absence of a locked order")
	}
	if _, err := reader.ProveAbsence(ctx, merchant, unlocked, block4.Time.Add(time.Second)); err == nil {
		t.Fatal("proved absence before the quote expired")
	}
	if _, err := reader.ProveAbsence(ctx, lower(orderPayer), unlocked, expires); err == nil {
		t.Fatal("accepted a legacy merchant key")
	}
	source.paid = true
	if _, err := reader.ProveAbsence(ctx, merchant, unlocked, expires); err == nil {
		t.Fatal("ignored the order state in the contract")
	}
	source.paid = false
	// A skipped claim names an order but says nothing about a payment.
	id, err := reader.ProveAbsence(ctx, merchant, unlocked, expires)
	if err != nil {
		t.Fatal(err)
	}
	var shown string
	if err := server.QueryRow(ctx, "SELECT evidence_id::text FROM chainview.reconciliation_absence($1,$2,$3)",
		stream, merchant, unlocked).Scan(&shown); err != nil || shown != id {
		t.Fatalf("absence not readable: %s %v", shown, err)
	}

	// Replay rebuilds the same facts and compares the escrow columns too.
	const job = "b0cabaf8-23e5-43d8-b73e-362fa0fb1503"
	if _, err := db.Exec(ctx, "SELECT chainview.start_replay($1,$2,1,now()+interval '1 minute')", stream, job); err != nil {
		t.Fatal(err)
	}
	if err := reader.PollReplay(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(ctx, "SELECT state FROM chainview.replay_job WHERE id=$1", job).Scan(&state); err != nil || state != "matched" {
		t.Fatalf("replay: %s %v", state, err)
	}
	if _, err := db.Exec(ctx, "UPDATE chainview.observed_event SET claimable_at=claimable_at+1 WHERE generation_id=$1 AND kind='delivered'", job); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "SELECT chainview.record_comparison($1,$2,1)", stream, job); err == nil {
		t.Fatal("different delivery time compared as equal")
	}

	if _, err := db.Exec(ctx, "UPDATE chainview.receipt_scope SET valid_until=now() WHERE login_role=$1", role); err != nil {
		t.Fatal(err)
	}
	if f := orderFacts(t, server, stream, merchant, order); len(f) != 0 {
		t.Fatalf("expired scope readable: %+v", f)
	}
	if _, err := db.Exec(ctx, "UPDATE chainview.receipt_scope SET valid_until=now()+interval '1 hour' WHERE login_role=$1", role); err != nil {
		t.Fatal(err)
	}
	// A contradicted confirmed block holds every fact of the stream.
	source.blocks[4] = sampleBlock(4, 144, 103)
	if err := reader.Poll(ctx); !errors.Is(err, ErrFinalizedConflict) {
		t.Fatalf("want finalized conflict, got %v", err)
	}
	if f := orderFacts(t, server, stream, merchant, order); len(f) != 0 {
		t.Fatalf("held stream still published: %+v", f)
	}
	if err := server.QueryRow(ctx, "SELECT chainview.reconciliation_ready($1,$2)", stream, merchant).Scan(&ready); err != nil || ready {
		t.Fatalf("held stream reported ready: %v %v", ready, err)
	}
}
