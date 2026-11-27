package settlement

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc3009"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/jackc/pgx/v5"
)

// walletTransfer is the board's signed transfer of its balance home or to another address. Its own deadline ends it.
type walletTransfer struct {
	payer, to   common.Address
	amount      *big.Int
	validBefore int64
	nonce       common.Hash
	signature   []byte
}

func (t walletTransfer) call(id string) (relayCall, error) {
	token, err := testusdc3009.Testusdc3009MetaData.GetAbi()
	if err != nil || len(t.signature) != 65 {
		return relayCall{}, errors.New("invalid wallet return")
	}
	data, err := token.Pack("transferWithAuthorization", t.payer, t.to, t.amount, new(big.Int), big.NewInt(t.validBefore), [32]byte(t.nonce),
		t.signature[64], [32]byte(t.signature[:32]), [32]byte(t.signature[32:64]))
	return relayCall{time.Unix(t.validBefore, 0).UTC(), [32]byte(t.nonce), data}, err
}

func returnAAD(payer common.Address, nonce common.Hash) []byte {
	return []byte("nu54-wallet-return-v1:" + lower(payer) + ":" + nonce.Hex())
}

func (f Forwarder) escrowTarget(req service.Request, subject string) (Target, error) {
	if req.Object != subject {
		return Target{}, service.Forbidden
	}
	target, ok := f.Targets[req.Domain]
	if !ok || !target.Escrow() {
		return Target{}, service.Capability
	}
	return target, nil
}

// EnrollPayer accepts the board account that proves itself for this customer, store and request.
func (f Forwarder) EnrollPayer(ctx context.Context, bound service.Bound, signature []byte) error {
	req := bound.Request
	if _, err := f.escrowTarget(req, bound.Subject); err != nil {
		return err
	}
	payer := common.HexToAddress(req.Body["payer"])
	generation, err := strconv.ParseUint(req.Body["generation"], 10, 64)
	if err != nil || generation == 0 || generation > math.MaxInt64 {
		return service.BadRequest
	}
	challenge := orderescrow.EnrollChallenge(bound.Tenant, req.Domain, bound.Subject, payer, generation, req.IdempotencyKey)
	if !orderescrow.SignedBy(orderescrow.AccountProofDigest(payer, challenge), signature, payer) {
		return service.Mismatch
	}
	intent, err := hex.DecodeString(bound.IntentDigest)
	if err != nil || len(intent) != 32 {
		return service.BadRequest
	}
	_, err = f.Store.db.Exec(ctx, "SELECT ledger.enroll_payer($1,$2,$3,$4,$5,$6,$7)", bound.Tenant, req.Domain, bound.Subject,
		req.IdempotencyKey, intent, payer[:], int64(generation))
	return sqlError(err)
}

func (f Forwarder) EnrolledPayer(ctx context.Context, bound service.Bound) (*commerce.PayerEnrollment, error) {
	if _, err := f.escrowTarget(bound.Request, bound.Subject); err != nil {
		return nil, err
	}
	var payer []byte
	var generation, expires int64
	err := f.Store.db.QueryRow(ctx, "SELECT * FROM ledger.enrolled_payer($1,$2,$3)", bound.Tenant, bound.Request.Domain, bound.Subject).
		Scan(&payer, &generation, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, sqlError(err)
	}
	return &commerce.PayerEnrollment{Payer: lower(common.BytesToAddress(payer)), Generation: strconv.FormatInt(generation, 10), ExpiresAt: expires}, nil
}

// ReturnWallet stores the board's transfer home for the relay worker.
func (f Forwarder) ReturnWallet(ctx context.Context, bound service.Bound, signature []byte) error {
	req := bound.Request
	target, err := f.escrowTarget(req, bound.Subject)
	if err != nil {
		return err
	}
	return f.storeTransfer(ctx, bound, target, signature, func(args []any) error {
		_, err := f.Store.db.Exec(ctx, `SELECT ledger.store_wallet_return($1,$2,$3,$4,$5,$6,$7,$8,$9::text::numeric,$10,$11,$12,$13)`, args...)
		return err
	})
}

// SendWallet stores the board's transfer to any address for the relay worker.
func (f Forwarder) SendWallet(ctx context.Context, bound service.Bound, signature []byte) error {
	req := bound.Request
	target, err := f.escrowTarget(req, bound.Subject)
	if err != nil {
		return err
	}
	// The board signs the store's token only, and no other token may spend the store's gas.
	if target.SendsPerDay < 1 || common.HexToAddress(req.Body["token"]) != target.Asset {
		return service.Capability
	}
	if to := common.HexToAddress(req.Body["to"]); to == (common.Address{}) || (target.Sponsor != (common.Address{}) && to == target.Sponsor) {
		return service.BadRequest
	}
	return f.storeTransfer(ctx, bound, target, signature, func(args []any) error {
		_, err := f.Store.db.Exec(ctx, `SELECT ledger.store_wallet_send($1,$2,$3,$4,$5,$6,$7,$8,$9::text::numeric,$10,$11,$12,$13,$14)`,
			append(args, target.SendsPerDay)...)
		return err
	})
}

// WalletSends lists the customer's transfers at a store that relays them.
func (f Forwarder) WalletSends(ctx context.Context, bound service.Bound) (*commerce.WalletSends, error) {
	target, err := f.escrowTarget(bound.Request, bound.Subject)
	if err != nil {
		return nil, err
	}
	if target.SendsPerDay < 1 {
		return nil, service.Capability
	}
	var raw []byte
	if err := f.Store.db.QueryRow(ctx, "SELECT ledger.wallet_send_status($1,$2,$3)", bound.Tenant, bound.Request.Domain, bound.Subject).Scan(&raw); err != nil {
		return nil, sqlError(err)
	}
	var held struct {
		Used  int32
		Sends []commerce.WalletSend
	}
	if err := json.Unmarshal(raw, &held); err != nil {
		return nil, err
	}
	return &commerce.WalletSends{PerDay: target.SendsPerDay, Remaining: max(0, target.SendsPerDay-held.Used), Sends: held.Sends}, nil
}

// storeTransfer checks a board-signed transfer and hands it to the ledger.
func (f Forwarder) storeTransfer(ctx context.Context, bound service.Bound, target Target, signature []byte, store func([]any) error) error {
	req := bound.Request
	payer, to := common.HexToAddress(req.Body["payer"]), common.HexToAddress(req.Body["to"])
	amount, ok := new(big.Int).SetString(req.Body["amount"], 10)
	validBefore, err := strconv.ParseInt(req.Body["validBefore"], 10, 64)
	if err != nil || !ok || to == payer || to == target.Asset || to == target.Contract {
		return service.BadRequest
	}
	nonce := common.HexToHash(req.Body["nonce"])
	digest, err := orderescrow.ReturnDigestFor(target.Token.OrDollar().Name, target.Asset, payer, to, amount, uint64(validBefore), nonce)
	if err != nil {
		return service.BadRequest
	}
	if !orderescrow.SignedBy(digest, signature, payer) {
		return service.Mismatch
	}
	intent, err := hex.DecodeString(bound.IntentDigest)
	if err != nil || len(intent) != 32 {
		return service.BadRequest
	}
	// The store's gas commitment covers this payer and branch, as it does for a payment.
	until := time.Now().Add(time.Minute)
	if f.GasSponsors != nil {
		profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, MerchantID: f.MerchantIDs[req.Domain]}
		witness, available := f.GasSponsors(ctx, req.Domain, profile, payer, time.Now())
		until = time.Time{}
		if available {
			until = time.Unix(witness.ExpiresAt, 0)
		}
	}
	return sqlError(store([]any{bound.Tenant, req.Domain, bound.Subject, req.IdempotencyKey, intent, payer[:], to[:], lower(target.Asset), amount.String(), validBefore,
		nonce[:], f.RelayCipher.Seal(nil, nil, signature, returnAAD(payer, nonce)), until}))
}

func (f Forwarder) WalletReturnStatus(ctx context.Context, bound service.Bound) (*commerce.WalletReturn, error) {
	if _, err := f.escrowTarget(bound.Request, bound.Subject); err != nil {
		return nil, err
	}
	var payer, to []byte
	view := commerce.WalletReturn{}
	err := f.Store.db.QueryRow(ctx, "SELECT * FROM ledger.wallet_return_status($1,$2,$3)", bound.Tenant, bound.Request.Domain, bound.Subject).
		Scan(&payer, &to, &view.Amount, &view.ValidBefore, &view.State, &view.TxHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	view.Payer, view.To = lower(common.BytesToAddress(payer)), lower(common.BytesToAddress(to))
	return &view, sqlError(err)
}

type authorizationChain interface {
	// authorization reads one token authorization in the finalized block: used or not, and the block's time.
	authorization(context.Context, common.Address, common.Address, common.Hash) (bool, int64, error)
	// authorizationUse finds the transaction that used it between two times. Nil means no log shows it.
	authorizationUse(context.Context, common.Address, common.Address, common.Hash, int64, int64) (*common.Hash, error)
}

func (c LiveChain) authorization(ctx context.Context, asset, payer common.Address, nonce common.Hash) (bool, int64, error) {
	head, err := c.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil {
		return false, 0, err
	}
	token, err := testusdc3009.NewTestusdc3009Caller(asset, c.Client)
	if err != nil {
		return false, 0, err
	}
	used, err := token.AuthorizationState(&bind.CallOpts{Context: ctx, BlockNumber: head.Number}, payer, nonce)
	return used, int64(head.Time), err
}

// firstBlockAt returns the first block at or after a time, up to the given head.
func (c LiveChain) firstBlockAt(ctx context.Context, at int64, head uint64) (uint64, error) {
	low, high := uint64(0), head
	for low < high {
		middle := low + (high-low)/2
		header, err := c.HeaderByNumber(ctx, new(big.Int).SetUint64(middle))
		if err != nil {
			return 0, err
		}
		if int64(header.Time) < at {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low, nil
}

func (c LiveChain) authorizationUse(ctx context.Context, asset, payer common.Address, nonce common.Hash, from, until int64) (*common.Hash, error) {
	head, err := c.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil {
		return nil, err
	}
	token, err := testusdc3009.NewTestusdc3009Filterer(asset, c.Client)
	if err != nil {
		return nil, err
	}
	// The authorization is bound to a few minutes, so the search is short however late it starts.
	first, err := c.firstBlockAt(ctx, from, head.Number.Uint64())
	if err != nil {
		return nil, err
	}
	last, err := c.firstBlockAt(ctx, until, head.Number.Uint64())
	if err != nil {
		return nil, err
	}
	size := uint64(1000)
	for start := first; start <= last; {
		end := min(start+size-1, last)
		logs, err := token.FilterAuthorizationUsed(&bind.FilterOpts{Context: ctx, Start: start, End: &end}, []common.Address{payer}, [][32]byte{nonce})
		if err != nil {
			if size == 1 || ctx.Err() != nil {
				return nil, err
			}
			size /= 2 // the node may allow a smaller range
			continue
		}
		found := logs.Next()
		err = logs.Error()
		logs.Close()
		if err != nil {
			return nil, err
		}
		if found {
			return &logs.Event.Raw.TxHash, nil
		}
		start = end + 1
	}
	return nil, nil
}

// processReturn is final only for a transaction that moved the money in a finalized block, and failed
// only when a finalized block after the deadline shows the authorization unused.
func (w *Worker) processReturn(ctx context.Context, job work, target Target, retry actionResult) (actionResult, error) {
	var payer, to, nonce, sealed []byte
	var asset, amount string
	var validBefore int64
	err := w.Store.db.QueryRow(ctx, "SELECT * FROM ledger.wallet_return_job($1,$2)", job.id, job.fence).
		Scan(&payer, &to, &asset, &amount, &validBefore, &nonce, &sealed)
	if err != nil {
		return retry, err
	}
	merchant, enrolled := w.MerchantIDs[job.branch]
	source, ok := w.Chain.(authorizationChain)
	if asset != lower(target.Asset) || !enrolled || !ok {
		return actionResult{delay: time.Minute}, errors.New("unbound wallet return")
	}
	transfer := &walletTransfer{payer: common.BytesToAddress(payer), to: common.BytesToAddress(to), validBefore: validBefore, nonce: common.BytesToHash(nonce)}
	transfer.amount, _ = new(big.Int).SetString(amount, 10)
	transfer.signature, err = w.RelayCipher.Open(nil, nil, sealed, returnAAD(transfer.payer, transfer.nonce))
	if err != nil {
		return actionResult{delay: time.Minute}, errors.New("sealed wallet return could not be opened")
	}
	job.action.transfer = transfer
	if err := w.adoptIncluded(ctx, &job); err != nil {
		return retry, err
	}
	used, finalTime, err := source.authorization(ctx, target.Asset, transfer.payer, transfer.nonce)
	if err != nil {
		return retry, err
	}
	over := finalTime >= validBefore
	if job.action.open {
		switch {
		case used:
			// The board signs a return for minutes; an hour before its deadline covers every clock involved.
			hash, err := source.authorizationUse(ctx, target.Asset, transfer.payer, transfer.nonce, validBefore-3600, validBefore)
			if err != nil {
				return retry, err
			}
			if hash == nil {
				if !over || w.Now().Sub(job.action.created) < time.Hour {
					return actionResult{delay: time.Minute}, errors.New("wallet return was used by a transaction the node cannot show")
				}
				// The money moved, so the account must not stay blocked for the missing log.
				return actionResult{outcome: text("final")}, errors.New("wallet return is used and its transaction stays unknown")
			}
			if _, err := w.Store.db.Exec(ctx, "SELECT ledger.record_wallet_return($1,$2,$3)", job.id, job.fence, hash.Hex()); err != nil {
				return retry, err
			}
			return actionResult{outcome: text("final")}, nil
		case over:
			// Nothing signed here can run any more, so the job ends and its nonce may be reused.
			return actionResult{outcome: text("failed"), finished: true}, nil
		}
	}
	// The stream gate needs the store's merchant, not an order.
	gate := Quote{}
	gate.Args.MerchantId = merchant
	if job.hash != nil {
		if !job.action.open && over {
			// Nothing here can run any more. The expired attempt gives its nonce back on the next reservation.
			return actionResult{finished: true}, nil
		}
		return w.advance(ctx, job, target, gate, retry)
	}
	if !job.action.open {
		return actionResult{finished: true}, nil
	}
	if w.Now().Unix() >= validBefore {
		return retry, nil // waiting for the finalized proof that it never ran
	}
	prepared, err := w.prepare(ctx, job, target, gate)
	if errors.Is(err, errWouldRevert) {
		return actionResult{delay: min(max(w.Now().Sub(job.action.created)/2, 5*time.Second), time.Minute)}, err
	}
	if err != nil || !prepared {
		return retry, err
	}
	return actionResult{state: text("prepared")}, nil
}

// escrowEvidence marks an escrow payment verified when the indexed lock is the recorded one.
func (h FinanceHTTP) escrowEvidence(ctx context.Context, target Target, quote Quote, hash, block string, index int32, out map[string]any) {
	facts, err := h.Evidence.orderFacts(ctx, target, quote)
	out["status"] = "held"
	if err != nil {
		return
	}
	for _, fact := range facts {
		if fact.kind != "locked" {
			continue
		}
		if fact.tx != hash || fact.block != block || fact.index != index || fact.amount != quote.Args.Amount.String() ||
			fact.beneficiary != lower(quote.Args.Beneficiary) || fact.payer != lower(quote.Args.Payer) ||
			fact.included.Unix() < int64(quote.Args.IssuedAt) || !fact.included.Before(quote.expiry()) {
			out["status"] = "invalid"
			return
		}
		out["status"], out["transaction"], out["block"], out["log"] = "verified", fact.tx, fact.block, fact.index
		out["includedAt"], out["includedBeforeExpiry"] = fact.included.UTC(), true
		return
	}
}
