// Package chain reads StableNet over JSON-RPC for the indexer.
package chain

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// RPC is the indexer's chain reader.
type RPC struct{ c *ethclient.Client }

func Dial(ctx context.Context, url string) (*RPC, error) {
	c, err := ethclient.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}
	return &RPC{c}, nil
}

// Finalized is the number of the finalized block.
func (r *RPC) Finalized(ctx context.Context) (uint64, error) {
	h, err := r.c.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil {
		return 0, fmt.Errorf("finalized header: %w", err)
	}
	return h.Number.Uint64(), nil
}

func (r *RPC) Logs(ctx context.Context, contract common.Address, topic0 common.Hash, from, to uint64) ([]types.Log, error) {
	return r.c.FilterLogs(ctx, ethereum.FilterQuery{
		Addresses: []common.Address{contract}, Topics: [][]common.Hash{{topic0}},
		FromBlock: new(big.Int).SetUint64(from), ToBlock: new(big.Int).SetUint64(to),
	})
}

func (r *RPC) BlockTime(ctx context.Context, n uint64) (uint64, error) {
	h, err := r.c.HeaderByNumber(ctx, new(big.Int).SetUint64(n))
	if err != nil {
		return 0, fmt.Errorf("header %d: %w", n, err)
	}
	return h.Time, nil
}
