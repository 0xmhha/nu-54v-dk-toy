package market

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const ChainID = 8283
const FundingInterval = 300

//go:embed Market.json
var marketABIJSON string

//go:embed LabToken.json
var tokenABIJSON string

var venueABI = parseABI(marketABIJSON)
var tokenABI = parseABI(tokenABIJSON)

func parseABI(document string) abi.ABI {
	result, err := abi.JSON(strings.NewReader(document))
	if err != nil {
		panic(err)
	}
	return result
}

type Contract struct {
	Address  common.Address `json:"address"`
	CodeHash common.Hash    `json:"codeHash"`
}

type Profile struct {
	Environment string         `json:"environment"`
	Chain       uint64         `json:"chainId"`
	Genesis     common.Hash    `json:"genesis"`
	Height      uint64         `json:"deploymentHeight"`
	BlockHash   common.Hash    `json:"deploymentHash"`
	QuoteSigner common.Address `json:"quoteSigner"`
	Venue       Contract       `json:"venue"`
	Base        Contract       `json:"base"`
	Quote       Contract       `json:"quote"`
}

type Chain struct {
	Profile Profile
	RPC     *ethclient.Client
	clock   func() time.Time
}

func Dial(ctx context.Context, endpoint string, p Profile) (*Chain, error) {
	if !loopback(endpoint) {
		return nil, errors.New("P08 RPC requires an isolated loopback node")
	}
	if p.Environment != "p08-isolated-8283-v1" || p.Chain != ChainID || p.Genesis == (common.Hash{}) ||
		p.BlockHash == (common.Hash{}) || p.Height == 0 {
		return nil, errors.New("invalid isolated deployment profile")
	}
	client, err := isolatedRPC(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	chain := &Chain{Profile: p, RPC: client}
	if err = chain.checkDeployment(ctx); err != nil {
		client.Close()
		return nil, err
	}
	return chain, nil
}

func (c *Chain) checkDeployment(ctx context.Context) error {
	genesis, err := c.RPC.HeaderByNumber(ctx, big.NewInt(0))
	if err != nil || genesis.Hash() != c.Profile.Genesis || genesis.Number.Sign() != 0 {
		return errors.New("RPC genesis does not match market deployment")
	}
	deployment, err := c.RPC.HeaderByNumber(ctx, new(big.Int).SetUint64(c.Profile.Height))
	if err != nil || deployment.Hash() != c.Profile.BlockHash || !deployment.Number.IsUint64() ||
		deployment.Number.Uint64() != c.Profile.Height {
		return errors.New("RPC deployment block does not match market deployment")
	}
	if err := c.checkCode(ctx, deployment.Hash()); err != nil {
		return err
	}
	roles, err := c.Venue(ctx, deployment.Hash(), "quoteSigner")
	if err != nil || c.Profile.QuoteSigner == (common.Address{}) || roles[0].(common.Address) != c.Profile.QuoteSigner {
		return errors.New("quote signer differs from the pinned deployment")
	}
	for _, token := range []struct {
		method   string
		ref      *Contract
		decimals uint8
	}{{"base", &c.Profile.Base, 8}, {"quoteToken", &c.Profile.Quote, 6}} {
		data, err := c.call(ctx, deployment.Hash(), c.Profile.Venue.Address, venueABI, token.method)
		if err != nil || data[0].(common.Address) != token.ref.Address {
			return errors.New("venue asset differs from pinned test token")
		}
		data, err = c.call(ctx, deployment.Hash(), token.ref.Address, tokenABI, "decimals")
		if err != nil || data[0].(uint8) != token.decimals {
			return errors.New("unexpected test token decimals")
		}
	}
	return nil
}

func (c *Chain) checkCode(ctx context.Context, at common.Hash) error {
	id, err := c.RPC.ChainID(ctx)
	if err != nil || !id.IsUint64() || id.Uint64() != ChainID {
		return errors.New("RPC chain differs from isolated 8283")
	}
	for _, contract := range []Contract{c.Profile.Venue, c.Profile.Base, c.Profile.Quote} {
		var code hexutil.Bytes
		err := c.RPC.Client().CallContext(ctx, &code, "eth_getCode", contract.Address, rpc.BlockNumberOrHashWithHash(at, true))
		if err != nil || len(code) == 0 || contract.Address == (common.Address{}) || crypto.Keccak256Hash(code) != contract.CodeHash {
			return fmt.Errorf("market runtime code mismatch for %s", contract.Address)
		}
	}
	return nil
}

func (c *Chain) Head(ctx context.Context) (*types.Header, error) {
	head, err := c.RPC.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	if err := c.checkDeployment(ctx); err != nil {
		return nil, err
	}
	if err := c.checkCode(ctx, head.Hash()); err != nil {
		return nil, err
	}
	return head, nil
}

// Quote and submission decisions require a current head. Read-only evidence can
// continue while the market clock is unfit for generating new authorizations.
func (c *Chain) wallTime() time.Time {
	if c.clock != nil {
		return c.clock()
	}
	return time.Now()
}

func (c *Chain) headClock(at uint64) string {
	return tipClock(at, c.wallTime())
}

func (c *Chain) call(ctx context.Context, at common.Hash, to common.Address, contract abi.ABI, method string, args ...any) ([]any, error) {
	input, err := contract.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	var result hexutil.Bytes
	err = c.RPC.Client().CallContext(ctx, &result, "eth_call", map[string]any{"to": to, "data": hexutil.Bytes(input)},
		rpc.BlockNumberOrHashWithHash(at, true))
	if err != nil {
		return nil, err
	}
	return contract.Unpack(method, result)
}

func (c *Chain) Venue(ctx context.Context, at common.Hash, method string, args ...any) ([]any, error) {
	return c.call(ctx, at, c.Profile.Venue.Address, venueABI, method, args...)
}

func (c *Chain) estimate(ctx context.Context, from, to common.Address, data []byte) (uint64, error) {
	return c.RPC.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &to, Data: data})
}
