package settlement

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"time"

	escrowabi "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/jackc/pgx/v5"
)

var errWouldRevert = errors.New("order escrow call would not execute")

// skipNames are the contract's SkipReason values in order.
var skipNames = [...]string{"", "unknown", "not-delivered", "window-open", "disputed", "settled", "other-beneficiary"}

// escrowAction is one contract call after the lock: a store, buyer, deadline, claim or dispute action.
type escrowAction struct {
	kind                  string
	open                  bool
	created               time.Time
	claimableAt           *int64
	deliverBy, resolveBy  *int64
	confirmer, outcome    *string
	sales                 []string
	quotes                []Quote
	approval              []byte
	ciphers, sums, signed [][]byte
	transfer              *walletTransfer // a wallet return
}

// destination is the contract a job calls.
func destination(job work, target Target) common.Address {
	if job.action != nil && job.action.kind == "return" {
		return target.Asset
	}
	return target.Contract
}

// escrowCalls maps single-order actions to their contract method.
var escrowCalls = map[string]string{"deliver": "markDelivered", "cancel": "cancel", "receipt": "release", "dispute": "dispute",
	"withdraw": "withdrawDispute", "refund-undelivered": "refundUndelivered", "refund-dispute": "refundExpiredDispute", "resolve": "resolve"}

// gasFor bounds one job. claimBatch takes about 15,000 per order, so the bound is twice that.
func gasFor(job work) uint64 {
	if job.action != nil && job.action.kind == "claim" {
		return 200_000 + 30_000*uint64(len(job.action.sales))
	}
	return relayGasLimit
}

func (w *Worker) escrowBranches() []string {
	var branches []string
	for branch, target := range w.Targets {
		if target.Escrow() {
			branches = append(branches, branch)
		}
	}
	slices.Sort(branches)
	return branches
}

func (s *Store) claimAction(ctx context.Context, branches []string) (work, error) {
	var w work
	var a escrowAction
	err := s.db.QueryRow(ctx, `SELECT work_id::text,fence,branch_id::text,kind,is_open,created_at,claimable_at,approval_cipher,deliver_by,
		resolve_by,confirmer,outcome,stream_id::text,relay_nonce,relay_expiry,relay_intent,relay_hash,relay_cipher,relay_state,
		sales::text[],quotes,digests,signers FROM ledger.claim_escrow_action($1::text[]::uuid[])`, branches).
		Scan(&w.id, &w.fence, &w.branch, &a.kind, &a.open, &a.created, &a.claimableAt, &a.approval, &a.deliverBy, &a.resolveBy,
			&a.confirmer, &a.outcome, &w.stream, &w.nonce, &w.expires, &w.intent, &w.hash, &w.raw, &w.state,
			&a.sales, &a.ciphers, &a.sums, &a.signed)
	w.action = &a
	return w, err
}

func (s *Store) releaseSpent(ctx context.Context, w work) error {
	_, err := s.db.Exec(ctx, "SELECT ledger.release_spent_relay($1,$2,$3)", w.id, w.fence, *w.hash)
	return err
}

// actionResult says what one pass established. The job ends only with finished.
type actionResult struct {
	state, outcome *string
	finished       bool
	delay          time.Duration
}

func (w *Worker) stepAction(ctx context.Context, branches []string) (bool, error) {
	job, err := w.Store.claimAction(ctx, branches)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	result, err := w.processAction(ctx, job)
	renew, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stop()
	_, deferErr := w.Store.db.Exec(renew, "SELECT ledger.defer_escrow_action($1,$2,$3,$4,$5,$6)",
		job.id, job.fence, result.state, result.outcome, result.finished, result.delay)
	return true, errors.Join(err, deferErr)
}

func text(value string) *string { return &value }

func (w *Worker) processAction(ctx context.Context, job work) (actionResult, error) {
	retry := actionResult{delay: 2 * time.Second}
	target, exists := w.Targets[job.branch]
	if exists && target.Escrow() && job.action.kind == "return" {
		return w.processReturn(ctx, job, target, retry)
	}
	if !exists || !target.Escrow() || job.stream == nil || *job.stream != target.Stream || len(job.action.sales) == 0 {
		return actionResult{delay: time.Minute}, errors.New("unbound escrow action")
	}
	for i, sale := range job.action.sales {
		raw, err := w.QuoteCipher.Open(nil, nil, job.action.ciphers[i], quoteAAD(job.branch, sale))
		if err != nil {
			return actionResult{delay: time.Minute}, errors.New("sealed quote could not be opened")
		}
		quote, err := unpackQuote(raw, job.action.sums[i], job.action.signed[i], target)
		if err != nil {
			return actionResult{delay: time.Minute}, err
		}
		job.action.quotes = append(job.action.quotes, quote)
	}
	quote := job.action.quotes[0]
	if err := w.adoptIncluded(ctx, &job); err != nil {
		return retry, err
	}
	facts, ok := w.Evidence.(escrowFacts)
	if !ok {
		return retry, errors.New("escrow evidence is not enrolled")
	}
	if job.action.open {
		closed, err := w.readAction(ctx, facts, job, target)
		if closed {
			// The next pass spends what is left of the relay.
			return actionResult{outcome: text("final")}, err
		}
		if err != nil && job.hash == nil {
			return retry, err
		}
		if err != nil { // the signed bytes hold a nonce; keep them moving
			result, sendErr := w.advance(ctx, job, target, quote, retry)
			return result, errors.Join(err, sendErr)
		}
		// Reading the facts may have closed this action.
		open, err := w.Store.actionOpen(ctx, job)
		if err != nil {
			return retry, err
		}
		job.action.open = open
	}
	if job.hash != nil {
		return w.advance(ctx, job, target, quote, retry)
	}
	if !job.action.open {
		return actionResult{finished: true}, nil
	}
	prepared, err := w.prepare(ctx, job, target, quote)
	if errors.Is(err, errWouldRevert) {
		if expired := w.actionExpired(job); expired {
			return actionResult{outcome: text("failed")}, err
		}
		// Try less and less often: the reason may last (a buyer whose token account refuses transfers).
		delay := min(max(w.Now().Sub(job.action.created)/2, 30*time.Second), time.Hour)
		_, noteErr := w.Store.db.Exec(ctx, "SELECT ledger.note_escrow_refusal($1,$2)", job.id, job.fence)
		return actionResult{delay: delay}, errors.Join(err, noteErr)
	}
	if err != nil || !prepared {
		return retry, err
	}
	return actionResult{state: text("prepared")}, nil
}

// actionExpired reports that an unsent action can no longer succeed.
func (w *Worker) actionExpired(job work) bool {
	past := func(at *int64) bool { return at != nil && w.Now().Unix() >= *at }
	switch job.action.kind {
	case "deliver":
		return past(job.action.deliverBy)
	case "dispute":
		return past(job.action.claimableAt)
	case "resolve":
		return past(job.action.resolveBy)
	case "claim":
		return w.Now().Sub(job.action.created) > time.Hour
	}
	return false
}

// advance sends the stored bytes until they are in a finalized block, then decides what they did.
// A nonce is given up only when the chain can no longer include these bytes.
func (w *Worker) advance(ctx context.Context, job work, target Target, quote Quote, retry actionResult) (actionResult, error) {
	receipt, err := w.Chain.TransactionReceipt(ctx, common.HexToHash(*job.hash))
	if err != nil {
		if !errors.Is(err, ethereum.NotFound) {
			return retry, err
		}
		state, _, err := w.resume(ctx, job, target, quote, true)
		return actionResult{state: state, delay: retry.delay}, err
	}
	state := text("observed")
	if receipt.Status == types.ReceiptStatusFailed {
		state = text("reverted")
	}
	final := true
	if source, ok := w.Chain.(finalChain); ok {
		if final, err = source.finalReceipt(ctx, receipt); err != nil {
			return retry, err
		}
	}
	if !final {
		return actionResult{state: state, delay: retry.delay}, nil
	}
	if !job.action.open {
		return actionResult{state: text("final"), finished: true}, nil
	}
	if receipt.Status == types.ReceiptStatusFailed {
		// Something else happened first. The nonce is used, so the action may be judged again.
		return retry, w.Store.releaseSpent(ctx, job)
	}
	return actionResult{state: state, delay: retry.delay}, nil // final on chain, waiting for the indexed fact
}

func (s *Store) actionOpen(ctx context.Context, job work) (bool, error) {
	var open bool
	err := s.db.QueryRow(ctx, "SELECT ledger.escrow_action_open($1,$2)", job.id, job.fence).Scan(&open)
	return open, err
}

type finalChain interface {
	finalNonce(context.Context, common.Address) (uint64, error)
	finalReceipt(context.Context, *types.Receipt) (bool, error)
}

func (c LiveChain) finalNonce(ctx context.Context, account common.Address) (uint64, error) {
	return c.NonceAt(ctx, account, big.NewInt(int64(rpc.FinalizedBlockNumber)))
}

func (c LiveChain) finalReceipt(ctx context.Context, receipt *types.Receipt) (bool, error) {
	head, err := c.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil || receipt.BlockNumber == nil || head.Number.Cmp(receipt.BlockNumber) < 0 {
		return false, err
	}
	block, err := c.HeaderByNumber(ctx, receipt.BlockNumber)
	return err == nil && block.Hash() == receipt.BlockHash, err
}

// actionArgs are the call arguments without a signature, and the key that must sign them (nil if none).
func (w *Worker) actionArgs(job work, target Target) (method string, args []any, digest common.Hash, signer *common.Address, err error) {
	quote, action := job.action.quotes[0], job.action
	merchant, order := quote.Args.MerchantId, quote.Args.OrderId
	if action.kind == "claim" {
		orders := make([][32]byte, len(action.quotes))
		for i, item := range action.quotes {
			if item.Args.MerchantId != merchant {
				return "", nil, digest, nil, errors.New("claim names more than one merchant")
			}
			orders[i] = item.Args.OrderId
		}
		// Orders fixed to another payout account are skipped by the contract and reported.
		return "claimBatch", []any{merchant, quote.Args.Beneficiary, orders}, digest, nil, nil
	}
	method = escrowCalls[action.kind]
	args = []any{merchant, order}
	var signed orderescrow.Action
	var at uint64
	switch action.kind {
	case "deliver", "cancel":
		signed = orderescrow.Delivered
		if action.kind == "cancel" {
			signed = orderescrow.Cancel
		}
		key, enrolled := w.MerchantSigners[job.branch]
		if !enrolled {
			return "", nil, digest, nil, errors.New("store signer is not enrolled")
		}
		signer = &key
	case "receipt", "dispute", "withdraw":
		if action.claimableAt == nil || action.confirmer == nil {
			return "", nil, digest, nil, errors.New("answer identity missing")
		}
		signed, at = orderescrow.BuyerActions[action.kind], uint64(*action.claimableAt)
		key := common.HexToAddress(*action.confirmer)
		signer = &key
	case "resolve":
		if action.outcome == nil {
			return "", nil, digest, nil, errors.New("decision missing")
		}
		args = append(args, *action.outcome == "pay-merchant")
	}
	if signer != nil {
		digest, err = orderescrow.ActionDigest(signed, target.Contract, merchant, order, at)
	}
	return method, args, digest, signer, err
}

func (w *Worker) actionCall(ctx context.Context, job work, target Target, now time.Time) (relayCall, error) {
	if job.action.transfer != nil {
		return job.action.transfer.call(job.id)
	}
	method, args, digest, signer, err := w.actionArgs(job, target)
	if err != nil {
		return relayCall{}, err
	}
	if signer != nil {
		var signature []byte
		if len(job.action.approval) != 0 {
			signature, err = w.RelayCipher.Open(nil, nil, job.action.approval, answerAAD(job.action.sales[0], job.action.kind))
		} else if w.MerchantSigner != nil {
			signature, err = w.MerchantSigner.SignDigest(ctx, digest)
		}
		if err != nil || !orderescrow.SignedBy(digest, signature, *signer) {
			return relayCall{}, errors.New("escrow action signature differs from its enrolled key")
		}
		args = append(args, signature)
	}
	contract, err := escrowabi.OrderescrowMetaData.GetAbi()
	if err != nil {
		return relayCall{}, err
	}
	data, err := contract.Pack(method, args...)
	// These calls have no deadline of their own. The same bytes are sent until the chain takes the nonce.
	expiry := now.Truncate(time.Second).AddDate(10, 0, 0).UTC()
	if job.expires != nil {
		expiry = *job.expires
	}
	return relayCall{expiry, sha256.Sum256([]byte("nu54-escrow-action-v1:" + job.id + ":" + job.action.kind)), data}, err
}

func (w *Worker) validateAction(job work, target Target, data []byte) error {
	if job.action.transfer != nil {
		expected, err := job.action.transfer.call(job.id)
		if err != nil || !bytes.Equal(expected.data, data) {
			return errors.New("stored transaction does not match wallet return")
		}
		return nil
	}
	method, args, digest, signer, err := w.actionArgs(job, target)
	if err != nil {
		return err
	}
	contract, err := escrowabi.OrderescrowMetaData.GetAbi()
	if err != nil {
		return err
	}
	call := contract.Methods[method]
	if len(data) < 4 || !bytes.Equal(data[:4], call.ID) {
		return errors.New("wrong archived escrow call")
	}
	stored, err := call.Inputs.Unpack(data[4:])
	if err != nil {
		return errors.New("invalid archived escrow call")
	}
	if signer != nil {
		signature, ok := stored[len(stored)-1].([]byte)
		if !ok || !orderescrow.SignedBy(digest, signature, *signer) {
			return errors.New("archived escrow signature differs")
		}
		args = append(args, signature)
	}
	expected, err := contract.Pack(method, args...)
	if err != nil || !bytes.Equal(expected, data) {
		return errors.New("stored transaction does not match escrow action")
	}
	return nil
}

func answerAAD(sale, kind string) []byte { return []byte("nu54-order-answer-v1:" + sale + ":" + kind) }

// Answer checks the phone signature over the recorded delivery and stores it for the relay worker.
func (f Forwarder) Answer(ctx context.Context, bound service.Bound, signature []byte) (string, error) {
	req := bound.Request
	var revision int64
	var sealed, approval []byte
	var relay, paid, confirmer, settled, settledHash, pending *string
	var sent bool
	var deliverBy, claimableAt, resolveBy *int64
	var reason *int16
	err := f.Store.db.QueryRow(ctx, "SELECT * FROM ledger.order_escrow_status($1,$2,$3,$4)", bound.Tenant, req.Domain, bound.Subject, req.Object).
		Scan(&revision, &sealed, &approval, &relay, &sent, &paid, &confirmer, &deliverBy, &claimableAt, &resolveBy, &settled, &reason, &settledHash, &pending)
	if err != nil {
		return "", sqlError(err)
	}
	if confirmer == nil {
		return "", service.CheckoutConflict // not locked
	}
	signed, err := f.QuoteCipher.Open(nil, nil, sealed, quoteAAD(req.Domain, req.Object))
	if err != nil || len(signed) != orderescrow.UnsignedSize+65 {
		return "", service.Unknown
	}
	at, err := strconv.ParseInt(req.Body["claimableAt"], 10, 64)
	if err != nil {
		return "", service.BadRequest
	}
	kind := req.Body["action"]
	digest, err := orderescrow.ActionDigest(orderescrow.BuyerActions[kind], common.BytesToAddress(signed[32:64]),
		common.BytesToHash(signed[192:224]), common.BytesToHash(signed[160:192]), uint64(at))
	if err != nil || !orderescrow.SignedBy(digest, signature, common.HexToAddress(*confirmer)) {
		return "", service.Mismatch
	}
	intent, err := hex.DecodeString(bound.IntentDigest)
	if err != nil || len(intent) != 32 {
		return "", service.BadRequest
	}
	var result string
	err = f.Store.db.QueryRow(ctx, "SELECT ledger.store_order_answer($1,$2,$3,$4,$5,$6,$7,$8,$9)::text", bound.Tenant, req.Domain, bound.Subject,
		req.Object, req.IdempotencyKey, intent, kind, at, f.RelayCipher.Seal(nil, nil, signature, answerAAD(req.Object, kind))).Scan(&result)
	return result, sqlError(err)
}

// escrowFacts is the finalized escrow evidence the worker reads besides payment receipts.
type escrowFacts interface {
	orderFacts(context.Context, Target, Quote) ([]escrowFact, error)
	// transactionFacts returns nothing until the indexer has confirmed the transaction's block.
	transactionFacts(context.Context, Target, Quote, *types.Receipt) ([]escrowFact, bool, error)
}

type escrowFact struct {
	order, id, kind, chain, contract, abi, policy string
	payer, beneficiary, amount                    string
	claimableAt, resolveBy                        *int64
	reason                                        *int32
	tx, block                                     string
	index                                         int32
	included                                      time.Time
}

const escrowFactColumns = `id::text,kind,chain_ref,contract_ref,abi_hash,policy_ref,COALESCE(payer_ref,''),COALESCE(beneficiary_ref,''),
	COALESCE(trim_scale(amount)::text,''),claimable_at,resolve_by,reason::integer,tx_hash,inclusion_hash,log_index,included_at`

func (e *EvidenceStore) escrowRows(ctx context.Context, target Target, quote Quote, query string, args ...any) ([]escrowFact, error) {
	var facts []escrowFact
	err := e.lock(ctx, target, quote, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		facts, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (f escrowFact, err error) {
			err = row.Scan(&f.order, &f.id, &f.kind, &f.chain, &f.contract, &f.abi, &f.policy, &f.payer, &f.beneficiary, &f.amount,
				&f.claimableAt, &f.resolveBy, &f.reason, &f.tx, &f.block, &f.index, &f.included)
			return
		})
		return err
	})
	for _, f := range facts {
		if f.chain != "8283" || f.contract != lower(target.Contract) || f.abi != target.ABIHash() || f.policy != target.Manifest {
			return nil, errors.New("finalized escrow fact outside the enrolled deployment")
		}
	}
	return facts, err
}

func (e *EvidenceStore) orderFacts(ctx context.Context, target Target, quote Quote) ([]escrowFact, error) {
	order := common.Hash(quote.Args.OrderId).Hex()
	return e.escrowRows(ctx, target, quote, "SELECT $3::text,"+escrowFactColumns+" FROM chainview.escrow_receipt($1,$2,$3)",
		target.Stream, common.Hash(quote.Args.MerchantId).Hex(), order)
}

func (e *EvidenceStore) transactionFacts(ctx context.Context, target Target, quote Quote, receipt *types.Receipt) ([]escrowFact, bool, error) {
	merchant := common.Hash(quote.Args.MerchantId).Hex()
	var confirmed bool
	if err := e.db.QueryRow(ctx, "SELECT chainview.reconciliation_finalized_block($1,$2,$3,$4)",
		target.Stream, merchant, receipt.BlockNumber.Int64(), receipt.BlockHash.Hex()).Scan(&confirmed); err != nil || !confirmed {
		return nil, false, err
	}
	facts, err := e.escrowRows(ctx, target, quote, "SELECT order_ref,"+escrowFactColumns+" FROM chainview.escrow_transaction($1,$2,$3)",
		target.Stream, merchant, receipt.TxHash.Hex())
	return facts, err == nil, err
}

// record books one finalized event of a locked order.
func (s *Store) recordEvent(ctx context.Context, sale string, target Target, quote Quote, f escrowFact) error {
	var due *int64
	switch f.kind {
	case "delivered":
		due = f.claimableAt
	case "disputed":
		due = f.resolveBy
	case "released", "cancelled":
		if f.amount != quote.Args.Amount.String() || f.reason == nil ||
			(f.kind == "released" && f.beneficiary != lower(quote.Args.Beneficiary)) ||
			(f.kind == "cancelled" && f.payer != lower(quote.Args.Payer)) {
			return errors.New("finalized escrow payout differs from the signed quote")
		}
	default:
		return nil // the lock and skipped claims are recorded elsewhere
	}
	var reason *int32
	var amount *string
	if due == nil {
		reason, amount = f.reason, &f.amount
	}
	_, err := s.db.Exec(ctx, "SELECT ledger.record_escrow_event($1,$2,$3,$4,$5,$6,$7::text::numeric,$8,$9,$10,$11)",
		sale, lower(target.Contract), f.id, f.kind, due, reason, amount, f.tx, f.index, f.block, f.included)
	return err
}

// readAction applies finalized facts. It reports true only for a claim whose every order has an outcome;
// other actions are closed by the event they caused.
func (w *Worker) readAction(ctx context.Context, facts escrowFacts, job work, target Target) (bool, error) {
	if job.action.kind != "claim" {
		return false, w.readOrder(ctx, facts, job.action.sales[0], target, job.action.quotes[0])
	}
	if job.hash == nil {
		return false, nil
	}
	receipt, err := w.Chain.TransactionReceipt(ctx, common.HexToHash(*job.hash))
	if errors.Is(err, ethereum.NotFound) || (err == nil && receipt.Status != types.ReceiptStatusSuccessful) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rows, confirmed, err := facts.transactionFacts(ctx, target, job.action.quotes[0], receipt)
	if err != nil || !confirmed {
		return false, err
	}
	byOrder := map[string]escrowFact{}
	for _, f := range rows {
		byOrder[f.order] = f
	}
	for i, quote := range job.action.quotes {
		f, found := byOrder[common.Hash(quote.Args.OrderId).Hex()]
		outcome := "paid"
		switch {
		case !found || (f.kind != "released" && f.kind != "claim-skipped"):
			outcome = "unknown"
		case f.kind == "released":
			// Read the whole order, so its delivery is on record before its payout.
			if err := w.readOrder(ctx, facts, job.action.sales[i], target, quote); err != nil {
				return false, err
			}
		case f.reason == nil || *f.reason < 1 || int(*f.reason) >= len(skipNames):
			return false, errors.New("claim skipped for an unknown reason")
		default:
			outcome = skipNames[*f.reason]
		}
		if _, err := w.Store.db.Exec(ctx, "SELECT ledger.record_escrow_claim($1,$2,$3,$4)", job.id, job.fence, job.action.sales[i], outcome); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (w *Worker) readOrder(ctx context.Context, facts escrowFacts, sale string, target Target, quote Quote) error {
	rows, err := facts.orderFacts(ctx, target, quote)
	if err != nil {
		return err
	}
	for _, f := range rows {
		if err := w.Store.recordEvent(ctx, sale, target, quote, f); err != nil {
			return err
		}
	}
	return nil
}

// stepFollow reads one open order, so events nobody here sent (a direct claim, a buyer's own call) are booked too.
func (w *Worker) stepFollow(ctx context.Context, branches []string) (bool, error) {
	var sale, branch string
	var cipher, digest, signer []byte
	var stream string
	err := w.Store.db.QueryRow(ctx, `SELECT sale_id::text,branch_id::text,quote_cipher,quote_digest,merchant_signer,stream_id::text
		FROM ledger.claim_escrow_follow($1::text[]::uuid[])`, branches).Scan(&sale, &branch, &cipher, &digest, &signer, &stream)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	target := w.Targets[branch]
	facts, ok := w.Evidence.(escrowFacts)
	if !ok || stream != target.Stream {
		return true, errors.New("unbound escrow order")
	}
	raw, err := w.QuoteCipher.Open(nil, nil, cipher, quoteAAD(branch, sale))
	if err != nil {
		return true, errors.New("sealed quote could not be opened")
	}
	quote, err := unpackQuote(raw, digest, signer, target)
	if err != nil {
		return true, err
	}
	if err := w.readOrder(ctx, facts, sale, target, quote); err != nil {
		return true, fmt.Errorf("escrow order %s: %w", sale, err)
	}
	return true, nil
}

// CheckArbiter refuses a dispute account the enrolled escrow contracts do not accept.
func CheckArbiter(ctx context.Context, client *ethclient.Client, resolver common.Address, targets map[string]Target) error {
	for branch, target := range targets {
		if !target.Escrow() {
			continue
		}
		contract, err := escrowabi.NewOrderescrowCaller(target.Contract, client)
		if err != nil {
			return err
		}
		if arbiter, err := contract.Arbiter(&bind.CallOpts{Context: ctx}); err != nil || arbiter != resolver {
			return fmt.Errorf("the dispute account is not the arbiter of branch %s", branch)
		}
	}
	return nil
}

// CheckOpenContracts refuses to run a branch while it has orders of another payment contract: those
// orders could no longer be confirmed, closed, delivered or refunded.
func (s *Store) CheckOpenContracts(ctx context.Context, branch string, target Target, quotes cipher.AEAD) error {
	rows, err := s.db.Query(ctx, "SELECT sale_id::text,payload_cipher,contract_ref FROM ledger.open_payment_contracts($1)", branch)
	if err != nil {
		return err
	}
	defer rows.Close()
	open := 0
	for rows.Next() {
		var sale string
		var sealed []byte
		var contract *string
		if err := rows.Scan(&sale, &sealed, &contract); err != nil {
			return err
		}
		if contract != nil {
			if !target.Escrow() || *contract != lower(target.Contract) {
				open++
			}
			continue
		}
		// Replacing one SettlementV2 contract by another keeps its existing rules.
		if signed, err := quotes.Open(nil, nil, sealed, quoteAAD(branch, sale)); target.Escrow() &&
			(err != nil || len(signed) < 64 || common.BytesToAddress(signed[32:64]) != target.Contract) {
			open++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if open != 0 {
		return fmt.Errorf("branch %s has %d open orders of another payment contract; finish them before switching", branch, open)
	}
	return nil
}
