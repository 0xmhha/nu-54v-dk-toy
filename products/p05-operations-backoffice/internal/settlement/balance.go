package settlement

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc3009"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
)

// ChainBalances is what AccountBalance asks of a chain client; *ethclient.Client is one.
type ChainBalances interface {
	bind.ContractCaller
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
}

// AccountBalance reads what an account holds of a token at the chain's latest block: the account's own balance for
// the chain's coin, and the token's balanceOf for any other token.
func AccountBalance(client ChainBalances) func(ctx context.Context, token, account common.Address) (*big.Int, error) {
	return func(ctx context.Context, token, account common.Address) (*big.Int, error) {
		if token == commerce.StableNetCoin {
			return client.BalanceAt(ctx, account, nil)
		}
		caller, err := testusdc3009.NewTestusdc3009Caller(token, client)
		if err != nil {
			return nil, err
		}
		return caller.BalanceOf(&bind.CallOpts{Context: ctx}, account)
	}
}

// PayerBalance is the customer account's balance of the asset the branch is paid in (commerce.PayerFunds). A
// branch without an approved settlement target has no asset to read.
func (f Forwarder) PayerBalance(ctx context.Context, branch string, payer common.Address) (common.Address, *big.Int, error) {
	target, ok := f.Targets[branch]
	if !ok || f.Balances == nil {
		return common.Address{}, nil, service.Capability
	}
	balance, err := f.Balances(ctx, target.Asset, payer)
	return target.Asset, balance, err
}
