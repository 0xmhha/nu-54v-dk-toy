package settlement

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"sort"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/jackc/pgx/v5"
)

type PayoutWorker struct {
	Store       *Store
	Client      *ethclient.Client
	Signer      Signer
	Governor    common.Address
	Profiles    map[string]commerce.Profile
	MaxGasPrice *big.Int
}

type payoutWork struct {
	key, phase                string
	next, previous, hash, raw []byte
	oldVersion                uint64
	frozen                    [][]byte
}

func (w *PayoutWorker) Step(ctx context.Context) (bool, error) {
	branches := make([]string, 0, len(w.Profiles))
	for branch := range w.Profiles {
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	for _, branch := range branches {
		profile := w.Profiles[branch]
		var address []byte
		var version *uint64
		if err := w.Store.db.QueryRow(ctx, "SELECT address,version FROM ledger.active_payout($1)", branch).Scan(&address, &version); err != nil {
			return false, err
		}
		if version != nil && *version >= profile.BeneficiaryVersion {
			profile.Beneficiary, profile.BeneficiaryVersion = common.BytesToAddress(address), *version
		}
		digest := profile.Digest()
		var job payoutWork
		err := w.Store.db.QueryRow(ctx, `SELECT request_key::text,new_address,old_address,old_version,phase,
			frozen_hashes,tx_hash,signed_tx FROM ledger.claim_payout($1,$2)`, branch, digest[:]).
			Scan(&job.key, &job.next, &job.previous, &job.oldVersion, &job.phase, &job.frozen, &job.hash, &job.raw)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return false, err
		}
		return w.process(ctx, branch, profile, job)
	}
	return false, nil
}

func (w *PayoutWorker) process(ctx context.Context, branch string, profile commerce.Profile, job payoutWork) (bool, error) {
	if job.oldVersion != profile.BeneficiaryVersion || common.BytesToAddress(job.previous) != profile.Beneficiary {
		return false, errors.New("payout predecessor differs from the issuance profile")
	}
	next := common.BytesToAddress(job.next)
	contract, err := v2.NewSettlementv2(profile.Contract, w.Client)
	if err != nil {
		return false, err
	}
	codec, err := v2.Settlementv2MetaData.GetAbi()
	if err != nil {
		return false, err
	}
	open := make([][32]byte, len(job.frozen))
	for i, digest := range job.frozen {
		if len(digest) != 32 {
			return false, errors.New("invalid frozen quote digest")
		}
		open[i] = [32]byte(digest)
	}
	data, err := codec.Pack("rotateMerchant", profile.MerchantID, profile.Signer, next, open)
	if err != nil {
		return false, err
	}
	signer := types.LatestSignerForChainID(big.NewInt(8283))
	if job.phase == "frozen" {
		if err := w.verifyPredecessor(ctx, contract, profile); err != nil {
			return false, err
		}
		from := w.Governor
		price, err := w.Client.SuggestGasPrice(ctx)
		if err != nil || price == nil || price.Sign() <= 0 || price.Cmp(w.MaxGasPrice) > 0 {
			return false, errors.New("payout gas price outside configured limit")
		}
		nonce, err := w.Client.PendingNonceAt(ctx, from)
		if err != nil {
			return false, err
		}
		gas, err := w.Client.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &profile.Contract, Data: data})
		if err != nil || gas > 10_000_000 {
			return false, errors.New("rotateMerchant simulation failed or exceeds gas limit")
		}
		unsigned := types.NewTx(&types.LegacyTx{Nonce: nonce, GasPrice: price, Gas: gas + gas/10, To: &profile.Contract, Value: big.NewInt(0), Data: data})
		hash := signer.Hash(unsigned)
		raw, err := w.Signer.SignDigest(ctx, hash)
		if err != nil || !matchesSigner(hash, raw, from) {
			return false, errors.New("payout signer differs from enrolled governor")
		}
		raw[64] -= 27
		signed, err := unsigned.WithSignature(signer, raw)
		if err != nil {
			return false, err
		}
		wire, err := signed.MarshalBinary()
		if err != nil {
			return false, err
		}
		var accepted bool
		err = w.Store.db.QueryRow(ctx, "SELECT ledger.remember_payout_transaction($1,$2,$3,$4)",
			branch, job.key, signed.Hash().Bytes(), wire).Scan(&accepted)
		return accepted, err
	}

	var signed types.Transaction
	if err := signed.UnmarshalBinary(job.raw); err != nil || signed.Hash() != common.BytesToHash(job.hash) ||
		signed.To() == nil || *signed.To() != profile.Contract || signed.Value().Sign() != 0 || !bytes.Equal(signed.Data(), data) {
		return false, errors.New("archived rotation differs from the covered intent")
	}
	from, err := types.Sender(signer, &signed)
	if err != nil || from != w.Governor || !signed.Protected() {
		return false, errors.New("archived rotation signer differs")
	}
	receipt, err := w.Client.TransactionReceipt(ctx, signed.Hash())
	if errors.Is(err, ethereum.NotFound) {
		_, pending, lookupErr := w.Client.TransactionByHash(ctx, signed.Hash())
		if errors.Is(lookupErr, ethereum.NotFound) {
			// Exact persisted bytes only. Any reply failure keeps the same hash.
			return false, w.Client.SendTransaction(ctx, &signed)
		}
		if lookupErr != nil {
			return false, lookupErr
		}
		_ = pending
		return false, nil
	}
	if err != nil {
		return false, err
	}
	final, err := w.Client.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil || final == nil || final.Number.Cmp(receipt.BlockNumber) < 0 {
		return false, err
	}
	canonical, err := w.Client.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil || canonical == nil || canonical.Hash() != receipt.BlockHash || receipt.Status != types.ReceiptStatusSuccessful {
		return false, errors.New("rotation receipt missing from finalized canonical chain or failed")
	}
	call := &bind.CallOpts{Context: ctx, BlockNumber: final.Number}
	merchant, err := contract.Merchants(call, profile.MerchantID)
	if err != nil || !merchant.Active || merchant.KeyVersion != profile.KeyVersion ||
		merchant.BeneficiaryVersion != profile.BeneficiaryVersion+1 {
		return false, errors.New("finalized payout registry version differs")
	}
	beneficiary, err := contract.Beneficiaries(call, profile.MerchantID, merchant.BeneficiaryVersion)
	if err != nil || beneficiary.Revoked || beneficiary.Account != next || beneficiary.ValidFrom != canonical.Time {
		return false, errors.New("finalized beneficiary differs from the independently authorized address")
	}
	profile.Beneficiary, profile.BeneficiaryVersion = next, merchant.BeneficiaryVersion
	digest := profile.Digest()
	var confirmed bool
	err = w.Store.db.QueryRow(ctx, "SELECT ledger.confirm_payout($1,$2,$3,$4,$5,$6)", branch, job.key,
		signed.Hash().Bytes(), receipt.BlockHash.Bytes(), digest[:], time.Unix(int64(beneficiary.ValidFrom), 0)).Scan(&confirmed)
	return confirmed, err
}

func (w *PayoutWorker) verifyPredecessor(ctx context.Context, contract *v2.Settlementv2, profile commerce.Profile) error {
	opts := &bind.CallOpts{Context: ctx}
	governor, err := contract.Governor(opts)
	if err != nil || governor != w.Governor {
		return errors.New("payout registry governor differs")
	}
	merchant, err := contract.Merchants(opts, profile.MerchantID)
	if err != nil || !merchant.Active || merchant.KeyVersion != profile.KeyVersion ||
		merchant.BeneficiaryVersion != profile.BeneficiaryVersion {
		return errors.New("payout registry predecessor versions differ")
	}
	beneficiary, err := contract.Beneficiaries(opts, profile.MerchantID, merchant.BeneficiaryVersion)
	if err != nil || beneficiary.Revoked || beneficiary.Account != profile.Beneficiary {
		return errors.New("payout registry predecessor address differs")
	}
	key, err := contract.MerchantKeys(opts, profile.MerchantID, merchant.KeyVersion)
	if err != nil || key.Revoked || key.Account != profile.Signer {
		return errors.New("payout registry quotation signer differs")
	}
	return nil
}
