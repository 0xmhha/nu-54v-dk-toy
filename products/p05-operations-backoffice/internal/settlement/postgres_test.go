package settlement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func scratchDB(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("P05_SETTLEMENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL with P05 migrations")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.ConnConfig.Database, "nu54_settlement_") {
		t.Fatal("use a disposable nu54_settlement_* database")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db, dsn
}

func testLogin(t *testing.T, db *pgxpool.Pool, dsn, capability string) (string, string) {
	t.Helper()
	ctx := context.Background()
	name := fmt.Sprintf("s10_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := db.Exec(ctx, "CREATE ROLE "+quoted+" LOGIN"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(context.Background(), "DROP ROLE "+quoted); err != nil {
			t.Error(err)
		}
	})
	if _, err := db.Exec(ctx, "GRANT "+pgx.Identifier{capability}.Sanitize()+" TO "+quoted); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	uri := url.URL{Scheme: "postgres", User: url.User(name), Path: "/" + cfg.ConnConfig.Database, RawQuery: url.Values{
		"host": {cfg.ConnConfig.Host}, "port": {strconv.Itoa(int(cfg.ConnConfig.Port))}, "sslmode": {"disable"},
	}.Encode()}
	return name, uri.String()
}

func fixtureUUID(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := db.QueryRow(context.Background(), "SELECT gen_random_uuid()::text").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func fixtureExec(t *testing.T, db *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

type fixtureSigner byte

func (s fixtureSigner) Address() common.Address {
	key, _ := crypto.ToECDSA(append(make([]byte, 31), byte(s)))
	return crypto.PubkeyToAddress(key.PublicKey)
}

func (s fixtureSigner) SignDigest(_ context.Context, digest [32]byte) ([]byte, error) {
	key, _ := crypto.ToECDSA(append(make([]byte, 31), byte(s)))
	signature, err := crypto.Sign(digest[:], key)
	if err == nil {
		signature[64] += 27
	}
	return signature, err
}

type seededOrder struct {
	Tenant, Branch, Sale, Actor string
	CipherKey                   [32]byte
	Quote                       Quote
	Signed                      []byte
	Presentation                *commerce.Presentation
	QuoteAttempts               int
	Order                       commerce.QuoteOrder
	Target                      Target
}

func seedOrder(t *testing.T, db *pgxpool.Pool, target Target, issued time.Time, amount string, nonce byte, branchOwner ...seededOrder) seededOrder {
	return seedPayerOrder(t, db, target, issued, amount, nonce, fixtureSigner(102), 2, branchOwner...)
}

func seedPayerOrder(t *testing.T, db *pgxpool.Pool, target Target, issued time.Time, amount string, nonce byte,
	payer fixtureSigner, payerRole byte, branchOwner ...seededOrder) seededOrder {
	s := seedPayerPreview(t, db, target, amount, payer, branchOwner...)
	return seedPayerIssue(t, db, s, issued, amount, nonce, payerRole, 120*time.Second)
}

func seedPayerPreview(t *testing.T, db *pgxpool.Pool, target Target, amount string, payer fixtureSigner, branchOwner ...seededOrder) seededOrder {
	t.Helper()
	s := seededOrder{Tenant: fixtureUUID(t, db), Branch: fixtureUUID(t, db), Sale: fixtureUUID(t, db),
		Actor: fixtureUUID(t, db), CipherKey: [32]byte{31: 53}, Target: target}
	if len(branchOwner) == 1 {
		s.Tenant, s.Branch = branchOwner[0].Tenant, branchOwner[0].Branch
		if payer.Address() == branchOwner[0].Order.Payer {
			s.Actor = branchOwner[0].Actor
		}
	} else {
		fixtureExec(t, db, "INSERT INTO ledger.branch(id,tenant_ref,timezone,close_local) VALUES($1,$2,'Asia/Seoul','04:00')", s.Branch, s.Tenant)
		fixtureExec(t, db, `INSERT INTO ledger.command_rule VALUES($1,'fixture','fixture','fixture',interval '5 minutes')`, s.Branch)
	}
	fixtureExec(t, db, `INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until)
		VALUES($1,$2,'customer',now()+interval '1 hour') ON CONFLICT DO NOTHING`, s.Branch, s.Actor)
	fixtureExec(t, db, `INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,state,asset_ref,amount)
		VALUES($1,$2,'pos',gen_random_uuid(),'preview',$3,$4::text::numeric)`, s.Sale, s.Branch, lower(target.Asset), amount)
	s.Order = commerce.QuoteOrder{Payer: payer.Address(), Claim: [32]byte{31: 3}}
	return s
}

func seedPayerIssue(t *testing.T, db *pgxpool.Pool, s seededOrder, issued time.Time, amount string, nonce, payerRole byte, lifetime time.Duration) seededOrder {
	t.Helper()
	ctx := context.Background()
	target := s.Target
	fixtureExec(t, db, "UPDATE ledger.sale SET state='issued' WHERE id=$1 AND state='preview'", s.Sale)
	// Multiple loopback scenarios share one settlement database. Salt the
	// fixture's role-specific words with its unique original sale reference.
	fixtureWord := func(role string) [32]byte {
		return sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%d", role, s.Sale, nonce)))
	}
	q := commerce.QuoteOrder{Tenant: s.Tenant, Branch: s.Branch, Sale: s.Sale, Payer: s.Order.Payer,
		QuoteID: fixtureWord("quote"), OrderID: fixtureWord("order"), Claim: [32]byte{31: 3}, Intent: [32]byte{31: 4},
		Nonce: fixtureWord("nonce"), PayerGeneration: 1, PayerRole: payerRole, Amount: amount, Issued: issued, Expires: issued.Add(lifetime)}
	profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
		Signer: fixtureSigner(101).Address(), MerchantID: [32]byte{31: 50}, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}}
	signed, digest, err := commerce.SignQuote(ctx, q, profile, fixtureSigner(101), issued)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := commerce.NewCipher(s.CipherKey[:])
	if err != nil {
		t.Fatal(err)
	}
	encrypted := cipher.Seal(nil, nil, signed, quoteAAD(s.Branch, s.Sale))
	fixtureExec(t, db, `INSERT INTO ledger.quote_work(sale_id,actor_id,claim_digest,quote_id,order_id,nonce,expected_revision,
		profile_version,merchant_signer,payer,payer_generation,payer_role,issued_at,expires_at,fence,lease_until,payload_cipher,signed_digest)
		VALUES($1,$2,$3,$4,$5,$6,0,1,$7,$8,1,$13,$9,$10,1,$9,$11,$12)`, s.Sale, s.Actor, q.Claim[:], q.QuoteID[:],
		q.OrderID[:], q.Nonce[:], profile.Signer.Bytes(), q.Payer.Bytes(), q.Issued, q.Expires, encrypted, digest[:], payerRole)
	fixtureExec(t, db, `INSERT INTO ledger.quote_outbox(sale_id,quote_digest) VALUES($1,$2)`, s.Sale, digest[:])
	s.Quote, err = unpackQuote(signed, digest[:], profile.Signer.Bytes(), target)
	if err != nil {
		t.Fatal(err)
	}
	s.Order, s.Signed = q, signed
	return s
}

func TestPostgresRefundCapacityFinanceRolesAndDayCutoff(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := context.Background()
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x2222")}
	// Seoul 03:59 and 04:00 straddle the configured accounting day.
	issued := time.Date(2026, 11, 20, 18, 58, 30, 0, time.UTC)
	s := seedOrder(t, db, target, issued, "100", 91)
	txHash, blockHash := (common.Hash{31: 12}).Hex(), (common.Hash{31: 13}).Hex()
	fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),$4,0,$5,$6,$7,100,$8,1,1,$9)`,
		s.Sale, s.Quote.Digest[:], target.Stream, txHash, blockHash, issued.Add(30*time.Second), lower(target.Asset), lower(s.Order.Payer), common.Hash(s.Quote.Args.MerchantId).Hex())
	requester, approver := fixtureUUID(t, db), fixtureUUID(t, db)
	for _, grant := range [][2]string{{requester, "refund_request"}, {requester, "refund_approve"}, {approver, "refund_approve"}, {approver, "day_close"}} {
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,$3,now()+interval '1 hour')", s.Branch, grant[0], grant[1])
	}
	_, financeDSN := testLogin(t, db, dsn, "nu54_p05_finance")
	store, err := Open(ctx, financeDSN, "nu54_p05_finance")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	type result struct {
		id     string
		amount int
		err    error
	}
	races := make(chan result, 2)
	var wait sync.WaitGroup
	for _, quantity := range []int{70, 50} {
		key := fixtureUUID(t, db)
		wait.Go(func() {
			r := result{amount: quantity}
			r.err = store.db.QueryRow(ctx, `SELECT ledger.reserve_console_refund($1,$2,$3,$4,$5,$6::text::numeric,'partial_service')::text`,
				s.Tenant, s.Branch, requester, s.Sale, key, strconv.Itoa(quantity)).Scan(&r.id)
			races <- r
		})
	}
	wait.Wait()
	close(races)
	var winner result
	var refusals int
	for result := range races {
		if result.err == nil {
			winner = result
		} else if service.CodeOf(sqlError(result.err)) == service.Conflict {
			refusals++
		} else {
			t.Fatal(result.err)
		}
	}
	if winner.id == "" || refusals != 1 {
		t.Fatalf("refund race: %+v conflicts=%d", winner, refusals)
	}
	call := `SELECT ledger.approve_refund($1,$2,$3,$4,$5)`
	if _, err := store.db.Exec(ctx, call, s.Tenant, s.Branch, requester, s.Sale, winner.id); service.CodeOf(sqlError(err)) != service.Forbidden {
		t.Fatalf("self approval was not denied: %v", err)
	}
	if _, err := store.db.Exec(ctx, call, s.Tenant, s.Branch, approver, s.Sale, winner.id); err != nil {
		t.Fatal(err)
	}
	finish := func(payer string) error {
		_, err := db.Exec(ctx, `SELECT ledger.confirm_refund($1,$2,gen_random_uuid(),$3,0,$4,$5,$6,$7::text::numeric,$8,1,
			(SELECT chain_refund_id FROM ledger.refund WHERE id=$1))`, winner.id, target.Stream,
			(common.Hash{31: 14}).Hex(), blockHash, issued.Add(95*time.Second), lower(target.Asset), strconv.Itoa(winner.amount), payer)
		return err
	}
	fixtureExec(t, db, "SELECT ledger.mark_refund_inflight($1)", winner.id)
	if err := finish(lower(common.HexToAddress("0xaabb"))); service.CodeOf(sqlError(err)) != service.Conflict {
		t.Fatalf("wrong refund destination accepted: %v", err)
	}
	if err := finish(lower(s.Order.Payer)); err != nil {
		t.Fatal(err)
	}
	if err := finish(lower(s.Order.Payer)); err != nil {
		t.Fatal(err)
	}
	var paidDay, refundDay, balanced string
	err = db.QueryRow(ctx, `SELECT min(business_day)::text,max(business_day)::text,
		sum(CASE side WHEN 'debit' THEN amount ELSE -amount END)::text FROM ledger.posting WHERE sale_id=$1`,
		s.Sale).Scan(&paidDay, &refundDay, &balanced)
	if err != nil || paidDay != "2026-11-20" || refundDay != "2026-11-21" || balanced != "0" {
		t.Fatalf("business-day/ledger mismatch %s %s %s %v", paidDay, refundDay, balanced, err)
	}
	if _, err := store.db.Exec(ctx, `SELECT ledger.close_business_day($1,$2,$3,$4::text::date,$5,100,0,gen_random_uuid())`,
		s.Tenant, s.Branch, approver, paidDay, lower(target.Asset)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(ctx, `SELECT ledger.close_business_day($1,$2,$3,$4::text::date,$5,0,$6::text::numeric,gen_random_uuid())`,
		s.Tenant, s.Branch, approver, refundDay, lower(target.Asset), strconv.Itoa(winner.amount)); err != nil {
		t.Fatal(err)
	}
	thirty := seedOrder(t, db, target, issued, "30", 93, s)
	fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),$4,0,$5,$6,$7,30,$8,1,1,$9)`,
		thirty.Sale, thirty.Quote.Digest[:], target.Stream, (common.Hash{31: 92}).Hex(), blockHash, issued.Add(30*time.Second),
		lower(target.Asset), lower(thirty.Order.Payer), common.Hash(thirty.Quote.Args.MerchantId).Hex())
	conflicts := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		key := fixtureUUID(t, db)
		wait.Go(func() {
			<-start
			_, err := store.db.Exec(ctx, `SELECT ledger.reserve_console_refund($1,$2,$3,$4,$5,20,'partial_service')`,
				thirty.Tenant, thirty.Branch, requester, thirty.Sale, key)
			conflicts <- err
		})
	}
	close(start)
	wait.Wait()
	won, denied := 0, 0
	for range 2 {
		switch err := <-conflicts; {
		case err == nil:
			won++
		case service.CodeOf(sqlError(err)) == service.Conflict:
			denied++
		default:
			t.Fatal(err)
		}
	}
	var reserved string
	if err := db.QueryRow(ctx, "SELECT reserved::text FROM ledger.payment WHERE sale_id=$1", thirty.Sale).Scan(&reserved); err != nil || reserved != "20" || won != 1 || denied != 1 {
		t.Fatalf("20+20 against principal 30: won=%d denied=%d reserved=%s: %v", won, denied, reserved, err)
	}

}

type unavailableChain struct{ sent [][]byte }

func (*unavailableChain) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return 4, nil
}
func (*unavailableChain) SuggestGasPrice(context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}
func (*unavailableChain) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	return nil, ethereum.NotFound
}
func (*unavailableChain) TransactionByHash(context.Context, common.Hash) (*types.Transaction, bool, error) {
	return nil, false, ethereum.NotFound
}
func (c *unavailableChain) SendTransaction(_ context.Context, tx *types.Transaction) error {
	raw, _ := tx.MarshalBinary()
	c.sent = append(c.sent, raw)
	if len(c.sent) == 1 {
		return errors.New("synthetic response failure")
	}
	return nil
}
func (*unavailableChain) RefundReserve(context.Context, Target, [32]byte) (*big.Int, error) {
	return big.NewInt(100), nil
}

type liveTestEvidence struct{}

func (liveTestEvidence) reconciliation(context.Context, *Store, work, Target, Quote) (bool, error) {
	return false, nil
}
func (liveTestEvidence) requestAbsence(context.Context, Target, Quote) error { return nil }
func (liveTestEvidence) lock(_ context.Context, _ Target, _ Quote, f func(pgx.Tx) error) error {
	return f(nil)
}

func TestPostgresRelayCustodyAndDeliveryRecovery(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := context.Background()
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x3333"), Contract: common.HexToAddress("0x4444")}
	order := seedOrder(t, db, target, time.Now().UTC().Truncate(time.Second).Add(-time.Second), "30", 92)
	qCipher, _ := commerce.NewCipher(order.CipherKey[:])
	relayKey := [32]byte{31: 95}
	relayCipher, _ := commerce.NewCipher(relayKey[:])
	_, commerceDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	commerceDB, err := Open(ctx, commerceDSN, "nu54_p05_commerce")
	if err != nil {
		t.Fatal(err)
	}
	defer commerceDB.Close()
	req := service.Request{Version: 2, Contract: "C3", Operation: "payment.forward", Domain: order.Branch, Object: order.Sale,
		RequestID: fixtureUUID(t, db), IdempotencyKey: fixtureUUID(t, db), ExpiresAt: time.Now().Unix() + 30,
		Body: map[string]string{"quoteDigest": rawHex(order.Quote.Digest[:]), "approvalDigest": rawHex(order.Quote.ApprovalHash[:])}}
	principal := service.Principal{Role: "customer", Tenant: order.Tenant, Subject: order.Actor, ExpiresAt: req.ExpiresAt,
		Grants: []service.Grant{{Domain: order.Branch, Object: order.Sale, Operations: []string{"C3/payment.forward"}}}}
	approve, _ := fixtureSigner(102).SignDigest(ctx, order.Quote.ApprovalHash)
	f := Forwarder{Store: commerceDB, QuoteCipher: qCipher, RelayCipher: relayCipher, Targets: map[string]Target{order.Branch: target}}
	principal.Order, err = f.AuthorizeForward(ctx, req, principal, approve)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := service.Authorize(req, principal, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Forward(ctx, bound, append(bytes.Clone(approve[:64]), 0)); service.CodeOf(err) != service.Mismatch {
		t.Fatalf("invalid approval accepted: %v", err)
	}
	for range 2 {
		if id, err := f.Forward(ctx, bound, approve); err != nil || id != order.Sale {
			t.Fatalf("payment forwarding: %s %v", id, err)
		}
	}
	_, workerDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	workerDB, err := Open(ctx, workerDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer workerDB.Close()
	chain := &unavailableChain{}
	w := Worker{Store: workerDB, Evidence: liveTestEvidence{}, Chain: chain, Targets: f.Targets, QuoteCipher: qCipher, RelayCipher: relayCipher,
		GasAccount: fixtureSigner(110).Address(), GasSigner: fixtureSigner(110), MaxGasPrice: big.NewInt(2), Now: time.Now}
	// Make this fixture the next eligible work, independently of prior tests.
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now()+interval '1 day' WHERE id<>$1", order.Sale)
	run := func() error {
		fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now() WHERE id=$1", order.Sale)
		ok, err := w.Step(ctx)
		if !ok {
			t.Fatal("ordered relay work missing", err)
		}
		return err
	}
	if err := run(); err != nil || len(chain.sent) != 0 {
		t.Fatalf("signing must commit custody without network send: %v", err)
	}
	var sealed []byte
	if err := db.QueryRow(ctx, "SELECT tx_cipher FROM ledger.relay_attempt WHERE work_id=$1", order.Sale).Scan(&sealed); err != nil || len(sealed) == 0 {
		t.Fatal("missing encrypted tx", err)
	}
	older := seedOrder(t, db, target, time.Now().Add(-5*time.Second), "30", 94, order)
	fixtureExec(t, db, "UPDATE ledger.settlement_work SET retry_at=now()-interval '5 minutes' WHERE id=$1", older.Sale)
	if worked, err := w.Step(ctx); !worked || err == nil || len(chain.sent) != 1 {
		t.Fatalf("newly signed bytes must be immediately due ahead of older unsigned work: %v", err)
	}
	if worked, err := w.Step(ctx); !worked || err != nil {
		t.Fatalf("old unpaid work made no progress after signed handoff: %v", err)
	}
	var oldFence int64
	if err := db.QueryRow(ctx, "SELECT fence FROM ledger.settlement_work WHERE id=$1", older.Sale).Scan(&oldFence); err != nil || oldFence != 1 {
		t.Fatalf("old work was not claimed after signed handoff: %d %v", oldFence, err)
	}
	w = Worker{Store: workerDB, Evidence: liveTestEvidence{}, Chain: chain, Targets: f.Targets, QuoteCipher: qCipher, RelayCipher: relayCipher,
		GasAccount: fixtureSigner(110).Address(), GasSigner: fixtureSigner(110), MaxGasPrice: big.NewInt(2), Now: time.Now}
	if err := run(); err != nil || len(chain.sent) != 2 || !bytes.Equal(chain.sent[0], chain.sent[1]) || bytes.Equal(sealed, chain.sent[0]) {
		t.Fatalf("resume failed to transmit identical authenticated bytes: %v", err)
	}
	fixtureExec(t, db, `SELECT ledger.confirm_payment($1,$2,$3,gen_random_uuid(),$4,0,$5,$6,$7,30,$8,1,1,$9)`,
		order.Sale, order.Quote.Digest[:], target.Stream, (common.Hash{31: 21}).Hex(),
		(common.Hash{31: 22}).Hex(), order.Order.Issued, lower(target.Asset), lower(order.Order.Payer), common.Hash(order.Quote.Args.MerchantId).Hex())
	_, dispatchDSN := testLogin(t, db, dsn, "nu54_p05_dispatch")
	dispatched, err := Open(ctx, dispatchDSN, "nu54_p05_dispatch")
	if err != nil {
		t.Fatal(err)
	}
	defer dispatched.Close()
	var event Delivery
	if err := db.QueryRow(ctx, `SELECT event_id::text,effect_key::text FROM ledger.fulfillment_outbox
		WHERE sale_id=$1 AND kind='paid'`, order.Sale).Scan(&event.EventID, &event.EffectKey); err != nil {
		t.Fatal(err)
	}
	var posts, gets int
	var acceptedDigest string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := "unknown"
		if r.Method == http.MethodPost {
			posts++
			var body json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&body)
			hash := sha256.Sum256(body)
			acceptedDigest = hex.EncodeToString(hash[:])
			// Downstream committed the effect, then returned a transport failure.
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		gets++
		if gets == 2 {
			status = "delivered"
		}
		if r.URL.Query().Get("payloadDigest") != acceptedDigest {
			t.Error("lookup did not retain payload digest")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"eventId": event.EventID, "effectKey": event.EffectKey,
			"status": status, "payloadDigest": acceptedDigest})
	}))
	defer server.Close()
	fixtureExec(t, db, "UPDATE ledger.fulfillment_outbox SET lease_until=now()+interval '1 day' WHERE event_id<>$1", event.EventID)
	for range 3 {
		fixtureExec(t, db, "UPDATE ledger.fulfillment_outbox SET lease_until=NULL WHERE event_id=$1", event.EventID)
		dispatch := Dispatcher{Evidence: liveTestEvidence{}, Store: dispatched, Client: server.Client(), Targets: map[string]string{order.Branch: server.URL}}
		if ok, err := dispatch.Step(ctx); err != nil || !ok {
			t.Fatalf("dispatch retry: %v", err)
		}
	}
	var inbox int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.fulfillment_inbox WHERE event_id=$1", event.EventID).Scan(&inbox); err != nil || inbox != 1 || posts != 1 || gets != 2 {
		t.Fatalf("unknown result caused redispatch or absent dedupe: inbox=%d posts=%d gets=%d %v", inbox, posts, gets, err)
	}
}

func (liveTestEvidence) deliveryEpoch(ctx context.Context, _, _ string, action func(context.Context, string) error) error {
	return action(ctx, strings.Repeat("a", 64))
}
