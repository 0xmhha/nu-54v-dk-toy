package settlement

import (
	"bytes"
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
)

// relayGasLimit covers settle, refund and OrderEscrow.lock, whose first use of every slot takes
// about 259,000 (checked by TestLoopbackOrderEscrow).
const relayGasLimit = 500000

type Signer interface {
	SignDigest(context.Context, [32]byte) ([]byte, error)
}

type Chain interface {
	PendingNonceAt(context.Context, common.Address) (uint64, error)
	SuggestGasPrice(context.Context) (*big.Int, error)
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
	TransactionByHash(context.Context, common.Hash) (*types.Transaction, bool, error)
	SendTransaction(context.Context, *types.Transaction) error
	RefundReserve(context.Context, Target, [32]byte) (*big.Int, error)
}

type LiveChain struct{ *ethclient.Client }

func (c LiveChain) RefundReserve(ctx context.Context, target Target, merchant [32]byte) (*big.Int, error) {
	caller, err := v2.NewSettlementv2Caller(target.Contract, c.Client)
	if err != nil {
		return nil, err
	}
	// The pinned contract enforces this reserve again during execution.
	return caller.RefundReserve(&bind.CallOpts{Context: ctx}, merchant)
}

type evidenceSource interface {
	reconciliation(context.Context, *Store, work, Target, Quote) (bool, error)
	deliveryEpoch(context.Context, string, string, func(context.Context, string) error) error
	requestAbsence(context.Context, Target, Quote) error
}

type Worker struct {
	Store                    *Store
	Evidence                 evidenceSource
	Chain                    Chain
	Targets                  map[string]Target
	QuoteCipher, RelayCipher cipher.AEAD
	GasAccount               common.Address
	GasSigner, RefundSigner  Signer
	MaxGasPrice              *big.Int
	Now                      func() time.Time
	// Escrow branches only: the key that signs store actions, with the address each branch expects,
	// and the account that settles disputes.
	MerchantSigner  Signer
	MerchantSigners map[string]common.Address
	Resolver        common.Address
	ResolverSigner  Signer
	MerchantIDs     map[string]common.Hash
	// Faucet is nil where no store offers test tokens.
	Faucet *Faucet
	turn   int
}

// account is the sender of the job's transaction.
func (w *Worker) account(job work) (common.Address, Signer, error) {
	if job.action != nil && job.action.kind == "resolve" {
		if w.Resolver == (common.Address{}) || w.ResolverSigner == nil {
			return common.Address{}, nil, errors.New("dispute account is not enrolled")
		}
		return w.Resolver, w.ResolverSigner, nil
	}
	if job.action != nil && job.action.kind == "faucet" {
		if w.Faucet == nil || job.action.faucet == nil || w.Faucet.Account != job.action.faucet.sender {
			return common.Address{}, nil, errors.New("faucet account is not enrolled")
		}
		return w.Faucet.Account, w.Faucet.Signer, nil
	}
	return w.GasAccount, w.GasSigner, nil
}

// Step takes one job. Payments, escrow actions and escrow reading take turns, so none waits for another.
func (w *Worker) Step(ctx context.Context) (bool, error) {
	branches := w.escrowBranches()
	if len(branches) == 0 {
		return w.stepPayment(ctx)
	}
	queues := []func(context.Context) (bool, error){w.stepPayment,
		func(ctx context.Context) (bool, error) { return w.stepAction(ctx, branches) },
		func(ctx context.Context) (bool, error) { return w.stepFollow(ctx, branches) }}
	for range queues {
		queue := queues[w.turn%len(queues)]
		w.turn++
		if moved, err := queue(ctx); moved || err != nil {
			return moved, err
		}
	}
	return false, nil
}

func (w *Worker) stepPayment(ctx context.Context) (bool, error) {
	job, err := w.Store.claim(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	state, finished, err := w.process(ctx, job)
	if errors.Is(err, errRefundReopened) {
		return true, nil
	}
	renew, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stop()
	return true, errors.Join(err, w.Store.deferWork(renew, job, state, finished))
}

func (w *Worker) process(ctx context.Context, job work) (*string, bool, error) {
	target, exists := w.Targets[job.branch]
	if !exists || (job.stream != nil && *job.stream != target.Stream) {
		return nil, false, errors.New("unbound settlement deployment")
	}
	raw, err := w.QuoteCipher.Open(nil, nil, job.quote, quoteAAD(job.branch, job.sale))
	if err != nil {
		return nil, false, errors.New("sealed quote could not be opened")
	}
	quote, err := unpackQuote(raw, job.digest, job.signer, target)
	if err != nil {
		return nil, false, err
	}
	if target.Escrow() && job.refund != nil {
		return nil, false, errors.New("an escrow order has no settlement refund")
	}
	if err := w.adoptIncluded(ctx, &job); err != nil {
		return nil, false, err
	}
	done, err := w.Evidence.reconciliation(ctx, w.Store, job, target, quote)
	if err == nil && !done && job.refund == nil && !w.Now().Before(quote.expiry()) {
		err = w.Evidence.requestAbsence(ctx, target, quote)
	}
	if errors.Is(err, ErrEvidenceHeld) && job.hash != nil {
		_, _, lookupErr := w.resume(ctx, job, target, quote, false)
		return nil, false, errors.Join(err, lookupErr)
	}
	if err != nil && job.hash != nil {
		// The signed bytes hold a nonce that later jobs need. An unreadable fact must not keep them unsent.
		state, _, sendErr := w.resume(ctx, job, target, quote, true)
		return state, false, errors.Join(err, sendErr)
	}
	if err != nil || done {
		if done {
			s := "final"
			return &s, true, err
		}
		return nil, false, err
	}
	// An issued quote without a payer submission can reconcile externally paid
	// or expired orders, but cannot authorize an outbound transaction itself.
	if job.refund == nil && len(job.approval) == 0 {
		return nil, false, nil
	}
	if job.hash != nil {
		return w.resume(ctx, job, target, quote, true)
	}
	prepared, err := w.prepare(ctx, job, target, quote)
	if err != nil || !prepared {
		return nil, false, err
	}
	// Release the preparation lease after custody commits. An immediately
	// eligible second claim can send; a crash here still holds the exact bytes.
	state := "prepared"
	return &state, false, nil
}

type relayCall struct {
	expiry time.Time
	intent [32]byte
	data   []byte
}

func (w *Worker) prepare(ctx context.Context, job work, target Target, quote Quote) (bool, error) {
	now := w.Now()
	candidate, err := w.makeCall(ctx, job, target, quote, now)
	if err != nil {
		return false, err
	}
	if !now.Before(candidate.expiry) {
		return false, nil
	}
	if job.nonce != nil && (!bytes.Equal(job.intent, candidate.intent[:]) || !job.expires.Equal(candidate.expiry)) {
		return false, errors.New("reserved signed intent differs")
	}
	account, gasSigner, err := w.account(job)
	if err != nil {
		return false, err
	}
	gas := gasFor(job)
	if target.sponsored(job) {
		gas += sponsorGas
	}
	to, data, err := relayTo(job, target, candidate.data)
	if err != nil {
		return false, err
	}
	if target.Escrow() {
		// Before any nonce is held: a call that would revert stays unsent.
		if source, ok := w.Chain.(escrowSimulator); ok && job.nonce == nil {
			if err := source.simulate(ctx, account, to, gas, data); err != nil {
				return false, fmt.Errorf("%w: %w", errWouldRevert, err)
			}
		}
	}
	pending, err := w.Chain.PendingNonceAt(ctx, account)
	if err != nil {
		return false, err
	}
	nonce, err := w.Store.reserve(ctx, job, lower(account), pending, candidate.intent, candidate.expiry)
	if err != nil {
		return false, err
	}
	price, err := w.Chain.SuggestGasPrice(ctx)
	if err != nil {
		return false, err
	}
	if price == nil || price.Sign() <= 0 || price.Cmp(w.MaxGasPrice) > 0 {
		return false, errors.New("gas quote outside configured limits")
	}
	_, hash, sealed, err := w.signRelay(ctx, job.id, account, gasSigner, &types.LegacyTx{Nonce: nonce, GasPrice: w.roomyPrice(price), Gas: gas,
		To: &to, Value: new(big.Int), Data: data})
	if err != nil {
		return false, err
	}
	if err := w.Store.save(ctx, job, candidate.intent, hash, sealed); err != nil {
		return false, err
	}
	return true, nil
}

// roomyPrice leaves room for the base fee to rise before the transaction is included.
func (w *Worker) roomyPrice(suggested *big.Int) *big.Int {
	price := new(big.Int).Lsh(suggested, 1)
	if price.Cmp(w.MaxGasPrice) > 0 {
		price.Set(w.MaxGasPrice)
	}
	return price
}

func (w *Worker) signRelay(ctx context.Context, id string, account common.Address, gasSigner Signer, unsigned *types.LegacyTx) (*types.Transaction, string, []byte, error) {
	tx := types.NewTx(unsigned)
	signer := types.LatestSignerForChainID(big.NewInt(8283))
	digest := signer.Hash(tx)
	sig, err := gasSigner.SignDigest(ctx, digest)
	if err != nil || !matchesSigner(digest, sig, account) {
		return nil, "", nil, errors.New("relay signer differed from configured sender")
	}
	sig[64] -= 27
	signed, err := tx.WithSignature(signer, sig)
	if err != nil {
		return nil, "", nil, err
	}
	wire, err := signed.MarshalBinary()
	if err != nil {
		return nil, "", nil, err
	}
	hash := signed.Hash().Hex()
	return signed, hash, w.RelayCipher.Seal(nil, nil, wire, relayAAD(id, hash)), nil
}

// adoptIncluded makes an earlier version the attempt again once the chain has included it.
func (w *Worker) adoptIncluded(ctx context.Context, job *work) error {
	if job.hash == nil {
		return nil
	}
	var versions []string
	if err := w.Store.db.QueryRow(ctx, "SELECT ledger.relay_versions($1,$2)", job.id, job.fence).Scan(&versions); err != nil {
		return err
	}
	for _, hash := range versions {
		_, err := w.Chain.TransactionReceipt(ctx, common.HexToHash(hash))
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		if err != nil {
			return err
		}
		var raw []byte
		if err := w.Store.db.QueryRow(ctx, "SELECT ledger.adopt_included_relay($1,$2,$3)", job.id, job.fence, hash).Scan(&raw); err != nil {
			return err
		}
		if raw == nil {
			return errors.New("included relay version is missing")
		}
		job.hash, job.raw = &hash, raw
		return nil
	}
	return nil
}

// replace signs the same nonce and call again at a higher price once the node asks for more than the
// stored price. Both versions stay on record, because either may be included.
func (w *Worker) replace(ctx context.Context, job *work, tx *types.Transaction, account common.Address, gasSigner Signer) (*types.Transaction, error) {
	suggested, err := w.Chain.SuggestGasPrice(ctx)
	if err != nil || suggested == nil || suggested.Cmp(tx.GasPrice()) <= 0 {
		return nil, err
	}
	// Nodes accept a replacement only at least a tenth above the old price; an eighth is safe.
	price := new(big.Int).Add(tx.GasPrice(), new(big.Int).Rsh(tx.GasPrice(), 3))
	price.Add(price, big.NewInt(1))
	if price.Cmp(w.MaxGasPrice) > 0 {
		return nil, nil // the configured limit stands; the stored bytes wait for the fee to fall
	}
	if roomy := w.roomyPrice(suggested); roomy.Cmp(price) > 0 {
		price = roomy
	}
	signed, hash, sealed, err := w.signRelay(ctx, job.id, account, gasSigner, &types.LegacyTx{Nonce: tx.Nonce(), GasPrice: price, Gas: tx.Gas(),
		To: tx.To(), Value: new(big.Int), Data: tx.Data()})
	if err != nil {
		return nil, err
	}
	if _, err := w.Store.db.Exec(ctx, "SELECT ledger.replace_signed_relay($1,$2,$3,$4,$5)", job.id, job.fence, *job.hash, hash, sealed); err != nil {
		return nil, err
	}
	job.hash, job.raw = &hash, sealed
	return signed, nil
}

func (w *Worker) makeCall(ctx context.Context, job work, target Target, quote Quote, now time.Time) (relayCall, error) {
	if job.action != nil {
		return w.actionCall(ctx, job, target, now)
	}
	if target.Escrow() {
		return w.lockCall(job, target, quote)
	}
	contract, err := v2.Settlementv2MetaData.GetAbi()
	if err != nil {
		return relayCall{}, err
	}
	if job.refund == nil {
		raw, err := w.RelayCipher.Open(nil, nil, job.approval, approvalAAD(job.sale))
		if err != nil || !bytes.Equal(job.approvalSum, quote.ApprovalHash[:]) || !matchesSigner(quote.ApprovalHash, raw, quote.Args.Payer) {
			return relayCall{}, errors.New("archived payer approval does not match issued quote")
		}
		data, err := contract.Pack("settle", quote.Args, quote.MerchantSig, raw)
		return relayCall{quote.expiry(), [32]byte(quote.ApprovalHash), data}, err
	}
	if job.refundAmount == nil || len(job.refundKey) != 32 {
		return relayCall{}, errors.New("refund identity missing")
	}
	amount, ok := new(big.Int).SetString(*job.refundAmount, 10)
	if !ok || amount.Sign() <= 0 {
		return relayCall{}, errors.New("invalid refund amount")
	}
	expiry := now.Truncate(time.Second).Add(90 * time.Second)
	if job.expires != nil {
		expiry = *job.expires
	}
	if !now.Before(expiry) {
		return relayCall{expiry: expiry}, nil
	}
	available, err := w.Chain.RefundReserve(ctx, target, quote.Args.MerchantId)
	if err != nil {
		return relayCall{}, err
	}
	if available.Cmp(amount) < 0 {
		return relayCall{}, errors.New("merchant refund reserve is below the approved amount")
	}
	authorization := v2.PaymentTypesV2RefundAuthorization{
		MerchantId: quote.Args.MerchantId, OrderId: quote.Args.OrderId, RefundId: common.BytesToHash(job.refundKey),
		Amount: amount, KeyVersion: target.RefundVersion, ExpiresAt: uint64(expiry.Unix()),
	}
	digest, err := hashRefund(contract, target, authorization)
	if err != nil {
		return relayCall{}, err
	}
	sig, err := w.RefundSigner.SignDigest(ctx, digest)
	if err != nil || !matchesSigner(digest, sig, target.RefundSigner) {
		return relayCall{}, errors.New("refund signer differed from enrolled authority")
	}
	data, err := contract.Pack("refund", authorization, sig)
	return relayCall{expiry, [32]byte(digest), data}, err
}

func hashRefund(contract *abi.ABI, target Target, authorization v2.PaymentTypesV2RefundAuthorization) (common.Hash, error) {
	fixed, err := abi.NewType("bytes32", "", nil)
	if err != nil {
		return common.Hash{}, err
	}
	typeHash := crypto.Keccak256Hash([]byte("RefundAuthorization(bytes32 merchantId,bytes32 orderId,bytes32 refundId,uint256 amount,uint64 keyVersion,uint64 expiresAt)"))
	encoded, err := (abi.Arguments{{Type: fixed}, contract.Methods["refund"].Inputs[0]}).Pack(typeHash, authorization)
	if err != nil {
		return common.Hash{}, err
	}
	fields := append(common.LeftPadBytes(big.NewInt(8283).Bytes(), 32), common.LeftPadBytes(target.Contract.Bytes(), 32)...)
	separator := domainHash(fields)
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, separator[:], crypto.Keccak256(encoded)), nil
}

func (w *Worker) resume(ctx context.Context, job work, target Target, quote Quote, allowBroadcast bool) (*string, bool, error) {
	raw, err := w.RelayCipher.Open(nil, nil, job.raw, relayAAD(job.id, *job.hash))
	if err != nil {
		return nil, false, errors.New("sealed relay bytes did not authenticate")
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return nil, false, err
	}
	sender, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8283)), &tx)
	account, gasSigner, accountErr := w.account(job)
	if err != nil || accountErr != nil || sender != account || tx.To() == nil ||
		tx.Value().Sign() != 0 || tx.Hash().Hex() != *job.hash || job.nonce == nil ||
		strconv.FormatUint(tx.Nonce(), 10) != *job.nonce || job.expires == nil {
		return nil, false, errors.New("sealed relay transaction metadata differs")
	}
	canonical, err := tx.MarshalBinary()
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, false, errors.New("sealed relay bytes not canonical")
	}
	call, err := relayed(job, target, *tx.To(), tx.Data())
	if err != nil {
		return nil, false, err
	}
	if err := w.validateCall(job, target, quote, call); err != nil {
		return nil, false, err
	}
	receipt, err := w.Chain.TransactionReceipt(ctx, tx.Hash())
	if err == nil {
		state := "observed"
		if receipt.Status == types.ReceiptStatusFailed {
			state = "reverted"
			if job.refund != nil && allowBroadcast {
				return &state, false, w.establishRefundFailure(ctx, job, target, quote, receipt)
			}
		}
		return &state, false, nil
	}
	if !errors.Is(err, ethereum.NotFound) {
		return nil, false, err
	}
	sending := &tx
	if allowBroadcast && w.Now().Before(*job.expires) {
		higher, err := w.replace(ctx, &job, &tx, account, gasSigner)
		if err != nil {
			return nil, false, err
		}
		if higher != nil {
			sending = higher
		}
	}
	if sending == &tx {
		_, _, err = w.Chain.TransactionByHash(ctx, tx.Hash())
		if err == nil {
			s := "broadcast"
			return &s, false, nil
		}
		if !errors.Is(err, ethereum.NotFound) {
			return nil, false, err
		}
	}
	if source, ok := w.Chain.(finalChain); ok {
		used, err := source.finalNonce(ctx, account)
		if err != nil {
			return nil, false, err
		}
		if used > tx.Nonce() {
			// The nonce is used for good. Look at every version once more: one of them may have been included
			// since it was last looked up. Only if none was did another transaction take the nonce.
			before := *job.hash
			if err := w.adoptIncluded(ctx, &job); err != nil || *job.hash != before {
				return nil, false, err
			}
			if _, err := w.Chain.TransactionReceipt(ctx, common.HexToHash(*job.hash)); !errors.Is(err, ethereum.NotFound) {
				return nil, false, err
			}
			if job.action != nil && job.action.faucet != nil {
				return nil, false, errFaucetSpent
			}
			return nil, false, w.Store.releaseSpent(ctx, job)
		}
	}
	if !allowBroadcast || !w.Now().Before(*job.expires) {
		return nil, false, nil
	}
	merchant := common.Hash(quote.Args.MerchantId).Hex()
	var admitted string
	err = w.Evidence.deliveryEpoch(ctx, target.Stream, merchant, func(ctx context.Context, epoch string) error {
		admitted = epoch
		_, err := w.Store.db.Exec(ctx, "SELECT ledger.authorize_relay_send($1,$2,$3,$4)", job.id, job.fence, *job.hash, epoch)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	// Persisted admission is the last authorization point before RPC. The
	// transaction is immutable and no database lock is held during the call.
	if err := w.Chain.SendTransaction(ctx, sending); err != nil {
		return nil, false, err // persisted unknown, exact-hash lookup on recovery
	}
	err = w.Evidence.deliveryEpoch(ctx, target.Stream, merchant, func(ctx context.Context, epoch string) error {
		if epoch != admitted {
			return ErrEvidenceHeld
		}
		_, err := w.Store.db.Exec(ctx, "SELECT ledger.ack_relay_send($1,$2,$3,$4)", job.id, job.fence, *job.hash, epoch)
		return err
	})
	return nil, false, err
}

func (w *Worker) validateCall(job work, target Target, quote Quote, data []byte) error {
	if job.action != nil {
		return w.validateAction(job, target, data)
	}
	if target.Escrow() {
		expected, err := w.lockCall(job, target, quote)
		if err != nil || !bytes.Equal(expected.data, data) || !bytes.Equal(job.intent, expected.intent[:]) {
			return errors.New("stored transaction does not match order lock")
		}
		return nil
	}
	contract, err := v2.Settlementv2MetaData.GetAbi()
	if err != nil {
		return err
	}
	if job.refund == nil {
		payerSig, err := w.RelayCipher.Open(nil, nil, job.approval, approvalAAD(job.sale))
		if err != nil {
			return err
		}
		expected, err := contract.Pack("settle", quote.Args, quote.MerchantSig, payerSig)
		if err != nil || !bytes.Equal(expected, data) || !bytes.Equal(job.intent, quote.ApprovalHash[:]) {
			return errors.New("stored transaction does not match payment approval")
		}
		return nil
	}
	method := contract.Methods["refund"]
	if len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
		return errors.New("wrong archived refund selector")
	}
	args, err := method.Inputs.Unpack(data[4:])
	if err != nil || len(args) != 2 {
		return errors.New("invalid archived refund calldata")
	}
	authorization := *abi.ConvertType(args[0], new(v2.PaymentTypesV2RefundAuthorization)).(*v2.PaymentTypesV2RefundAuthorization)
	sig := args[1].([]byte)
	digest, err := hashRefund(contract, target, authorization)
	if err != nil || job.refundAmount == nil || authorization.Amount.String() != *job.refundAmount ||
		authorization.MerchantId != quote.Args.MerchantId || authorization.OrderId != quote.Args.OrderId ||
		authorization.RefundId != common.BytesToHash(job.refundKey) || authorization.KeyVersion != target.RefundVersion ||
		authorization.ExpiresAt != uint64(job.expires.Unix()) || !bytes.Equal(digest[:], job.intent) ||
		!matchesSigner(digest, sig, target.RefundSigner) {
		return fmt.Errorf("archived refund authorization differs")
	}
	return nil
}
