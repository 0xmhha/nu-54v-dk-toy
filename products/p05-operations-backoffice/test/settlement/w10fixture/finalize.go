// Package w10fixture drives the exact installed-order settlement on a private Anvil chain.
package w10fixture

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type orderStore interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Settlement struct {
	DB             orderStore
	Chain          *ethclient.Client
	Step           func(context.Context) (bool, error)
	Profile        commerce.Profile
	Payer, Sponsor common.Address
	BeforeMine     func(string) error
}

func (s Settlement) Finalize(t testing.TB, ctx context.Context, sale string, amount *big.Int) (result string, failure error) {
	db, chain := s.DB, s.Chain
	profile := s.Profile
	phase := "original-admission"
	defer func() {
		if failure == nil {
			return
		}
		code := "assertion"
		var sql *pgconn.PgError
		switch {
		case errors.As(failure, &sql):
			code = sql.Code
		case errors.Is(failure, pgx.ErrNoRows):
			code = "no-row"
		case errors.Is(failure, context.DeadlineExceeded):
			code = "deadline"
		case errors.Is(failure, ethereum.NotFound):
			code = "transaction-not-found"
		}
		t.Logf("installed W10 original settlement phase=%s code=%s", phase, code)
	}()
	var issued time.Time
	if err := db.QueryRow(ctx, `SELECT q.issued_at FROM ledger.quote_work q
		JOIN ledger.settlement_submission p ON p.sale_id=q.sale_id WHERE q.sale_id=$1`, sale).Scan(&issued); err != nil {
		return "", err
	}
	phase = "starting-balances"
	balances, err := ReadBalances(ctx, chain, profile, s.Payer, s.Sponsor)
	if err != nil {
		return "", err
	}
	phase = "chain-time"
	if err := chain.Client().CallContext(ctx, nil, "evm_setAutomine", false); err != nil {
		return "", err
	}
	header, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return "", err
	}
	if err := chain.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", max(int64(header.Time)+1, issued.Unix()+1)); err != nil {
		return "", err
	}
	step := func() error {
		var eligible bool
		err := db.QueryRow(ctx, `WITH original AS (
			UPDATE ledger.settlement_work SET retry_at=now() WHERE id=$1 AND sale_id=$1 AND NOT finished RETURNING id
		) SELECT EXISTS(SELECT FROM original) AND NOT EXISTS (
			SELECT FROM ledger.settlement_work WHERE id<>$1 AND NOT finished AND retry_at<=now()
		)`, sale).Scan(&eligible)
		if err != nil {
			return err
		}
		if !eligible {
			return errors.New("isolated worker does not have precisely the original settlement work")
		}
		moved, err := s.Step(ctx)
		if err != nil {
			return err
		}
		if !moved {
			return errors.New("original settlement work unavailable")
		}
		return nil
	}
	phase = "prepare-original-relay"
	if err := step(); err != nil {
		return "", err
	}
	phase = "broadcast-original-relay"
	if err := step(); err != nil {
		return "", err
	}
	phase = "read-original-relay"
	var hash, state string
	if err := db.QueryRow(ctx, "SELECT tx_hash,state FROM ledger.relay_attempt WHERE work_id=$1", sale).Scan(&hash, &state); err != nil {
		return "", err
	}
	if state != "broadcast" {
		phase = "original-relay-not-broadcast"
		return "", errors.New("the exact signed attempt was not broadcast")
	}
	if s.BeforeMine != nil {
		if err := s.BeforeMine(hash); err != nil {
			return "", err
		}
	}
	phase = "mine-original-relay"
	if err := chain.Client().CallContext(ctx, nil, "evm_mine"); err != nil {
		return "", err
	}
	txHash := common.HexToHash(hash)
	receipt, err := bind.WaitMinedHash(ctx, chain, txHash)
	if err != nil {
		return "", err
	}
	phase = "read-mined-original-relay"
	tx, pending, err := chain.TransactionByHash(ctx, txHash)
	if err != nil {
		return "", err
	}
	if pending {
		return "", errors.New("original relay remains pending after mining")
	}
	from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8283)), tx)
	if err != nil || from != s.Sponsor || tx.To() == nil || *tx.To() != profile.Contract {
		return "", errors.New("unexpected sender for original relay")
	}
	phase = "validate-original-receipt"
	if receipt.Status != types.ReceiptStatusSuccessful || receipt.EffectiveGasPrice == nil {
		return "", errors.New("original relay did not complete its principal transfer")
	}
	phase = "principal-and-fee"
	after, err := ReadBalances(ctx, chain, profile, s.Payer, s.Sponsor)
	if err != nil {
		return "", err
	}
	fee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
	if new(big.Int).Sub(after.Beneficiary, balances.Beneficiary).Cmp(amount) != 0 ||
		new(big.Int).Sub(balances.Escrow, after.Escrow).Cmp(amount) != 0 ||
		after.PayerGas.Cmp(balances.PayerGas) != 0 || fee.Sign() <= 0 ||
		new(big.Int).Sub(balances.SponsorGas, after.SponsorGas).Cmp(fee) != 0 {
		return "", errors.New("original principal or sponsored gas did not reconcile")
	}
	phase = "finalized-indexer-evidence"
	for range 5 {
		if err := chain.Client().CallContext(ctx, nil, "evm_mine"); err != nil {
			return "", err
		}
	}
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		var finalized bool
		err := db.QueryRow(ctx, "SELECT EXISTS (SELECT FROM chainview.finalized_receipt WHERE tx_hash=$1 AND kind='paid')", hash).Scan(&finalized)
		if err != nil {
			return "", err
		}
		if finalized {
			phase = "book-original-payment"
			return hash, step()
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-tick.C:
		}
	}
}

type Balances struct {
	Beneficiary, Escrow, PayerGas, SponsorGas *big.Int
}

func ReadBalances(ctx context.Context, chain *ethclient.Client, profile commerce.Profile,
	payer, sponsor common.Address) (Balances, error) {
	var state Balances
	token, err := testusdc.NewTestusdcCaller(profile.Asset, chain)
	if err != nil {
		return state, err
	}
	contract, err := v2.NewSettlementv2Caller(profile.Contract, chain)
	if err != nil {
		return state, err
	}
	opts := &bind.CallOpts{Context: ctx}
	beneficiary, recipientErr := token.BalanceOf(opts, profile.Beneficiary)
	escrow, escrowErr := contract.EscrowBalance(opts, payer, 1)
	payerGas, payerErr := chain.BalanceAt(ctx, payer, nil)
	sponsorGas, sponsorErr := chain.BalanceAt(ctx, sponsor, nil)
	return Balances{beneficiary, escrow, payerGas, sponsorGas},
		errors.Join(recipientErr, escrowErr, payerErr, sponsorErr)
}
