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
}

func (w *Worker) Step(ctx context.Context) (bool, error) {
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
	done, err := w.Evidence.reconciliation(ctx, w.Store, job, target, quote)
	if errors.Is(err, ErrEvidenceHeld) && job.hash != nil {
		_, _, lookupErr := w.resume(ctx, job, target, quote, false)
		return nil, false, errors.Join(err, lookupErr)
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
	pending, err := w.Chain.PendingNonceAt(ctx, w.GasAccount)
	if err != nil {
		return false, err
	}
	nonce, err := w.Store.reserve(ctx, job, lower(w.GasAccount), pending, candidate.intent, candidate.expiry)
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
	const gasLimit = 500000
	tx := types.NewTx(&types.LegacyTx{Nonce: nonce, GasPrice: price, Gas: gasLimit, To: &target.Contract, Value: new(big.Int), Data: candidate.data})
	signer := types.LatestSignerForChainID(big.NewInt(8283))
	digest := signer.Hash(tx)
	sig, err := w.GasSigner.SignDigest(ctx, digest)
	if err != nil || !matchesSigner(digest, sig, w.GasAccount) {
		return false, errors.New("relay signer differed from configured sender")
	}
	sig[64] -= 27
	signed, err := tx.WithSignature(signer, sig)
	if err != nil {
		return false, err
	}
	wire, err := signed.MarshalBinary()
	if err != nil {
		return false, err
	}
	hash := signed.Hash().Hex()
	sealed := w.RelayCipher.Seal(nil, nil, wire, relayAAD(job.id, hash))
	if err := w.Store.save(ctx, job, candidate.intent, hash, sealed); err != nil {
		return false, err
	}
	return true, nil
}

func (w *Worker) makeCall(ctx context.Context, job work, target Target, quote Quote, now time.Time) (relayCall, error) {
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
	if err != nil || sender != w.GasAccount || tx.To() == nil || *tx.To() != target.Contract ||
		tx.Value().Sign() != 0 || tx.Hash().Hex() != *job.hash || job.nonce == nil ||
		strconv.FormatUint(tx.Nonce(), 10) != *job.nonce || job.expires == nil {
		return nil, false, errors.New("sealed relay transaction metadata differs")
	}
	canonical, err := tx.MarshalBinary()
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, false, errors.New("sealed relay bytes not canonical")
	}
	if err := w.validateCall(job, target, quote, tx.Data()); err != nil {
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
	_, _, err = w.Chain.TransactionByHash(ctx, tx.Hash())
	if err == nil {
		s := "broadcast"
		return &s, false, nil
	}
	if !errors.Is(err, ethereum.NotFound) {
		return nil, false, err
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
	if err := w.Chain.SendTransaction(ctx, &tx); err != nil {
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
