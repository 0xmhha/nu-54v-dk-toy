package settlement

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
)

// balanceChain is a chain that holds one coin balance and one token balance, and records the token call.
type balanceChain struct {
	coin, token *big.Int
	err         error
	call        ethereum.CallMsg
	block       *big.Int
	calls       int
}

func (c *balanceChain) BalanceAt(_ context.Context, _ common.Address, block *big.Int) (*big.Int, error) {
	c.block = block
	return c.coin, c.err
}

func (c *balanceChain) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return []byte{1}, nil
}

func (c *balanceChain) CallContract(_ context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	c.call, c.block, c.calls = call, block, c.calls+1
	if c.err != nil {
		return nil, c.err
	}
	return common.LeftPadBytes(c.token.Bytes(), 32), nil
}

// The balance of the chain's coin is the account's own; of any other token it is that token's balanceOf.
func TestAccountBalanceReadsTheSettlementAsset(t *testing.T) {
	account := common.HexToAddress("0x" + strings.Repeat("7", 40))
	token := common.HexToAddress("0x" + strings.Repeat("2", 40))
	chain := &balanceChain{coin: big.NewInt(41), token: big.NewInt(300_000)}
	read := AccountBalance(chain)

	if got, err := read(context.Background(), commerce.StableNetCoin, account); err != nil || got.Int64() != 41 || chain.calls != 0 || chain.block != nil {
		t.Fatalf("coin balance: %v %v (%d token calls, block %v)", got, err, chain.calls, chain.block)
	}
	got, err := read(context.Background(), token, account)
	want := append(common.FromHex("0x70a08231"), common.LeftPadBytes(account[:], 32)...) // balanceOf(address)
	if err != nil || got.Int64() != 300_000 || chain.call.To == nil || *chain.call.To != token || !bytes.Equal(chain.call.Data, want) || chain.block != nil {
		t.Fatalf("token balance: %v %v, call to %v with %x at block %v", got, err, chain.call.To, chain.call.Data, chain.block)
	}
	chain.err = errors.New("rpc down")
	if _, err := read(context.Background(), token, account); err == nil {
		t.Fatal("a failed token read gave a balance")
	}
	if _, err := read(context.Background(), commerce.StableNetCoin, account); err == nil {
		t.Fatal("a failed coin read gave a balance")
	}

	// The forwarder reads the asset of the branch's approved target, and nothing for a branch without one.
	chain.err = nil
	branch, other := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	forwarder := Forwarder{Targets: map[string]Target{branch: {Asset: token}}, Balances: read}
	if asset, balance, err := forwarder.PayerBalance(context.Background(), branch, account); err != nil || asset != token || balance.Int64() != 300_000 {
		t.Fatalf("payer balance: %v %v %v", asset, balance, err)
	}
	if _, balance, err := forwarder.PayerBalance(context.Background(), other, account); err == nil || balance != nil {
		t.Fatalf("a branch without a target has a balance: %v %v", balance, err)
	}
	if _, _, err := (Forwarder{Targets: forwarder.Targets}).PayerBalance(context.Background(), branch, account); err == nil {
		t.Fatal("a forwarder without a chain read a balance")
	}
	var _ commerce.PayerFunds = forwarder
}
