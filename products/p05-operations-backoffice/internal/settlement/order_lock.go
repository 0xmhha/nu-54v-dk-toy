package settlement

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	escrowabi "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

// An order lock archives the board signature followed by the phone key address.
const lockPayloadSize = 65 + common.AddressLength

// verifyLock accepts only the paying board's signature over this exact quote and phone key.
func verifyLock(quote Quote, target Target, confirmer common.Address, signature []byte) (orderescrow.Lock, error) {
	lock, err := orderescrow.NewLockFor(target.Token.OrDollar().Name, quote.Unsigned, confirmer)
	if err != nil {
		return orderescrow.Lock{}, err
	}
	payer, beneficiary, signer := quote.Args.Payer, quote.Args.Beneficiary, quote.Signer
	// The contract refuses the same keys: the store must not confirm receipt for the buyer.
	if confirmer == signer || confirmer == beneficiary || confirmer == payer ||
		confirmer == target.Contract || payer == signer || payer == beneficiary || payer == target.Contract ||
		!orderescrow.SignedBy(lock.Digest, signature, payer) {
		return orderescrow.Lock{}, errors.New("order lock does not match the issued quote")
	}
	return lock, nil
}

func (f Forwarder) verifiedLock(ctx context.Context, req service.Request, tenant, actor string, signature []byte) (
	Quote, Target, string, orderescrow.Lock, common.Address, error,
) {
	fail := func(err error) (Quote, Target, string, orderescrow.Lock, common.Address, error) {
		return Quote{}, Target{}, "", orderescrow.Lock{}, common.Address{}, err
	}
	if target, ok := f.Targets[req.Domain]; ok && !target.Escrow() {
		return fail(service.Capability)
	}
	quote, target, state, err := f.verifiedQuote(ctx, req, tenant, actor)
	if err != nil {
		return fail(err)
	}
	confirmer := common.HexToAddress(req.Body["confirmer"])
	lock, err := verifyLock(quote, target, confirmer, signature)
	if err != nil || rawHex(lock.Digest[:]) != req.Body["lockDigest"] {
		return fail(service.Mismatch)
	}
	return quote, target, state, lock, confirmer, nil
}

// AuthorizeLock derives order authority from the archived quote, like AuthorizeForward.
func (f Forwarder) AuthorizeLock(ctx context.Context, req service.Request, principal service.Principal, signature []byte) (*service.OrderApproval, error) {
	quote, _, _, lock, _, err := f.verifiedLock(ctx, req, principal.Tenant, principal.Subject, signature)
	if err != nil {
		return nil, err
	}
	return &service.OrderApproval{QuoteDigest: rawHex(quote.Digest[:]), ApprovalDigest: rawHex(lock.Digest[:]),
		ValidUntil: min(req.ExpiresAt, principal.ExpiresAt)}, nil
}

// Lock stores the board signature and phone key through the existing payment admission.
func (f Forwarder) Lock(ctx context.Context, bound service.Bound, signature []byte) (string, error) {
	quote, target, state, lock, confirmer, err := f.verifiedLock(ctx, bound.Request, bound.Tenant, bound.Subject, signature)
	if err != nil {
		return "", err
	}
	return f.admit(ctx, bound, quote, target, state, lock.Digest, append(bytes.Clone(signature), confirmer[:]...))
}

// The contract's Payout and Refund values in order.
var (
	payoutNames = [...]string{"receipt", "claim", "arbiter", "dispute-withdrawn"}
	refundNames = [...]string{"store", "undelivered", "arbiter", "dispute-expired"}
)

// EscrowStatus adds what an escrow order's customer or terminal may see. It returns nil for any
// other order, and service.Busy when the order moved after the caller read it.
func (f Forwarder) EscrowStatus(ctx context.Context, bound service.Bound, order commerce.Order) (*commerce.OrderEscrow, error) {
	var revision int64
	var sealedQuote, sealedLock []byte
	var relayHash, paidHash, confirmer, settled, settledHash, pending *string
	var sent bool
	var deliverBy, claimableAt, resolveBy *int64
	var reason *int16
	err := f.Store.db.QueryRow(ctx, "SELECT * FROM ledger.order_escrow_status($1,$2,$3,$4)",
		bound.Tenant, bound.Request.Domain, bound.Subject, bound.Request.Object).
		Scan(&revision, &sealedQuote, &sealedLock, &relayHash, &sent, &paidHash, &confirmer, &deliverBy, &claimableAt, &resolveBy,
			&settled, &reason, &settledHash, &pending)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, sqlError(err)
	}
	if revision != order.Revision {
		return nil, service.Busy
	}
	signed, err := f.QuoteCipher.Open(nil, nil, sealedQuote, quoteAAD(bound.Request.Domain, bound.Request.Object))
	if err != nil || len(signed) != orderescrow.UnsignedSize+65 {
		return nil, service.Unknown
	}
	word := func(index int) []byte { return signed[index*32 : (index+1)*32] }
	view := &commerce.OrderEscrow{Escrow: lower(common.BytesToAddress(word(1))), OrderKey: rawHex(word(5)),
		MerchantID: rawHex(word(6)), Payer: lower(common.BytesToAddress(word(9)))}
	if len(sealedLock) != 0 {
		payload, err := f.RelayCipher.Open(nil, nil, sealedLock, approvalAAD(bound.Request.Object))
		if err != nil || len(payload) != lockPayloadSize {
			return nil, service.Unknown
		}
		view.Confirmer = lower(common.BytesToAddress(payload[65:]))
	}
	switch {
	case order.State == "paid" && confirmer != nil && paidHash != nil && deliverBy != nil:
		view.Stage, view.Confirmer, view.LockTxHash, view.DeliverBy = "locked", *confirmer, paidHash, *deliverBy
		view.ClaimableAt, view.ResolveBy, view.SettledTxHash = claimableAt, resolveBy, settledHash
		switch {
		case settled != nil && *settled == "released" && reason != nil:
			view.Stage, view.Payout = "paid", payoutNames[*reason]
		case settled != nil && *settled == "cancelled" && reason != nil:
			view.Stage, view.Refund = "refunded", refundNames[*reason]
		case resolveBy != nil:
			view.Stage = "disputed"
		case claimableAt != nil:
			view.Stage = "delivered"
		}
		if pending != nil && ((view.Stage == "delivered" && *pending != "withdraw") || (view.Stage == "disputed" && *pending == "withdraw")) {
			view.PendingAction = *pending
		}
	case order.State == "closed":
		view.Stage = "failed"
		if sent { // a transaction that was never sent is not evidence of anything
			view.LockTxHash = relayHash
		}
	case order.State == "checking" && relayHash != nil && (order.Payment == nil || order.Payment.RelayTxHash != nil):
		view.Stage, view.LockTxHash = "submitted", relayHash
	case order.State == "checking":
		view.Stage = "received"
	default:
		return nil, service.Unknown
	}
	return view, nil
}

type escrowSimulator interface {
	simulate(context.Context, common.Address, common.Address, uint64, []byte) error
}

// simulate asks the node whether the call would run now, so one that must revert (already paid, no
// balance, deadline passed) does not spend the store's gas or hold a nonce.
func (c LiveChain) simulate(ctx context.Context, from, to common.Address, gas uint64, data []byte) error {
	_, err := c.PendingCallContract(ctx, ethereum.CallMsg{From: from, To: &to, Gas: gas, Value: new(big.Int), Data: data})
	return err
}

// openLock reads the archived board signature and phone key and checks them against the quote again.
func (w *Worker) openLock(job work, target Target, quote Quote) (orderescrow.Lock, common.Address, []byte, error) {
	payload, err := w.RelayCipher.Open(nil, nil, job.approval, approvalAAD(job.sale))
	if err != nil || len(payload) != lockPayloadSize {
		return orderescrow.Lock{}, common.Address{}, nil, errors.New("archived order lock could not be opened")
	}
	signature, confirmer := payload[:65], common.BytesToAddress(payload[65:])
	lock, err := verifyLock(quote, target, confirmer, signature)
	if err != nil || !bytes.Equal(job.approvalSum, lock.Digest[:]) {
		return orderescrow.Lock{}, common.Address{}, nil, errors.New("archived order lock does not match issued quote")
	}
	return lock, confirmer, signature, nil
}

func (w *Worker) lockCall(job work, target Target, quote Quote) (relayCall, error) {
	lock, confirmer, signature, err := w.openLock(job, target, quote)
	if err != nil {
		return relayCall{}, err
	}
	contract, err := escrowabi.OrderescrowMetaData.GetAbi()
	if err != nil {
		return relayCall{}, err
	}
	data, err := contract.Pack("lock", escrowabi.PaymentTypesV2MerchantQuote(quote.Args), confirmer, quote.MerchantSig, signature)
	return relayCall{quote.expiry(), [32]byte(lock.Digest), data}, err
}

// lockReconciliation is the escrow counterpart of the paid receipt check. The caller holds the stream lock.
func lockReconciliation(ctx context.Context, tx pgx.Tx, worker *Store, w work, target Target, quote Quote) (bool, error) {
	merchant, order := common.Hash(quote.Args.MerchantId).Hex(), common.Hash(quote.Args.OrderId).Hex()
	var id, chain, contract, abi, policy, payer, beneficiary, confirmer, amount, digest, hash, block string
	var deliverBy, window int64
	var index int32
	var included time.Time
	err := tx.QueryRow(ctx, `SELECT id::text,chain_ref,contract_ref,abi_hash,policy_ref,COALESCE(payer_ref,''),
		COALESCE(beneficiary_ref,''),COALESCE(confirmer_ref,''),COALESCE(trim_scale(amount)::text,''),COALESCE(quote_digest,''),
		COALESCE(deliver_by,0),COALESCE(confirm_window,0),tx_hash,inclusion_hash,log_index,included_at
		FROM chainview.escrow_receipt($1,$2,$3) WHERE kind='locked'`, target.Stream, merchant, order).
		Scan(&id, &chain, &contract, &abi, &policy, &payer, &beneficiary, &confirmer, &amount, &digest,
			&deliverBy, &window, &hash, &block, &index, &included)
	if errors.Is(err, pgx.ErrNoRows) {
		if !time.Now().Before(quote.expiry()) {
			return applyAbsence(ctx, tx, worker, w, target, quote)
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if chain != "8283" || contract != lower(target.Contract) || abi != target.ABIHash() || policy != target.Manifest ||
		payer != lower(quote.Args.Payer) || beneficiary != lower(quote.Args.Beneficiary) ||
		amount != quote.Args.Amount.String() || digest != common.Hash(quote.Digest).Hex() ||
		included.Unix() < int64(quote.Args.IssuedAt) || !included.Before(quote.expiry()) ||
		deliverBy <= included.Unix() || window <= 0 ||
		!common.IsHexAddress(confirmer) || strings.ToLower(confirmer) != confirmer || common.HexToAddress(confirmer) == (common.Address{}) {
		return false, errors.New("finalized lock differs from the exact signed quote")
	}
	// The contract checked this phone key under the board's signature, so it is the order's key even
	// when the buyer locked through someone else with another key than the one sent here.
	_, err = worker.db.Exec(ctx, `SELECT ledger.confirm_order_lock($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::text::numeric,$11,$12,$13,$14,$15,$16,$17)`,
		w.sale, quote.Digest[:], target.Stream, id, hash, index, block, included, lower(quote.Args.Asset), amount,
		payer, int64(quote.Args.PayerGeneration), merchant, contract, confirmer, deliverBy, window)
	return err == nil, err
}
