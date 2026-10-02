// Package ingest moves finalized PaymentSettled logs into the store (P07 design 2).
//
//  1. read the finalized block F; if the cursor C has reached it, wait one poll period
//  2. eth_getLogs{settlement, [topic0], C+1 .. min(F, C+1000)}
//  3. decode merchant, orderId, device (topics) and amount, nonce (data), read block times
//  4. store the rows and move the cursor to the range end in one transaction
//
// Any RPC or decode error leaves the cursor where it was and the range is read again
// (P07-NFR-02). Only finalized blocks are read, so there is no reorg handling.
package ingest

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/receipt"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/store"
)

// Topic0 is keccak256 of the full PaymentSettled signature [N08].
var Topic0 = crypto.Keccak256Hash([]byte("PaymentSettled(address,bytes32,address,uint256,uint256)"))

// MaxRange is the most blocks one eth_getLogs call covers.
const MaxRange = 1000

// Chain is what ingestion reads.
type Chain interface {
	Finalized(ctx context.Context) (uint64, error)
	Logs(ctx context.Context, contract common.Address, topic0 common.Hash, from, to uint64) ([]types.Log, error)
	BlockTime(ctx context.Context, n uint64) (uint64, error)
}

// Sink stores a range.
type Sink interface {
	Cursor(ctx context.Context, start uint64) (uint64, error)
	Commit(ctx context.Context, rows []store.Row, to uint64) error
}

// Ingester runs the loop and reports how far behind it is.
type Ingester struct {
	Chain    Chain
	Sink     Sink
	Contract common.Address
	// Start is the cursor before the first ingest: the block before the settlement deployment.
	Start uint64
	Poll  time.Duration

	mu        sync.Mutex
	finalized uint64
	cursor    uint64
	polled    bool
}

var _ receipt.Freshness = (*Ingester)(nil)

// Lag is finalized minus cursor at the last poll.
func (in *Ingester) Lag() (uint64, uint64, bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if !in.polled || in.cursor > in.finalized {
		return 0, in.cursor, in.polled
	}
	return in.finalized - in.cursor, in.cursor, true
}

// Decode turns one log into a row; logs of another contract or event are refused (P07-FR-01).
func Decode(contract common.Address, l types.Log, blockTime uint64) (store.Row, error) {
	if l.Address != contract || len(l.Topics) != 4 || l.Topics[0] != Topic0 || len(l.Data) != 64 {
		return store.Row{}, fmt.Errorf("not a PaymentSettled log of %s (tx %s, index %d)", contract.Hex(), l.TxHash.Hex(), l.Index)
	}
	return store.Row{
		TxHash:      l.TxHash.Hex(),
		LogIndex:    l.Index,
		Merchant:    common.BytesToAddress(l.Topics[1].Bytes()).Hex(),
		OrderID:     l.Topics[2].Hex(),
		Device:      common.BytesToAddress(l.Topics[3].Bytes()).Hex(),
		Amount:      new(big.Int).SetBytes(l.Data[:32]).String(),
		Nonce:       new(big.Int).SetBytes(l.Data[32:]).String(),
		BlockNumber: l.BlockNumber,
		BlockTime:   blockTime,
	}, nil
}

// Step ingests at most one range. It returns true when it stored up to the finalized head.
func (in *Ingester) Step(ctx context.Context) (caughtUp bool, err error) {
	cursor, err := in.Sink.Cursor(ctx, in.Start)
	if err != nil {
		return false, err
	}
	fin, err := in.Chain.Finalized(ctx)
	if err != nil {
		return false, err
	}
	in.mu.Lock()
	in.finalized, in.cursor, in.polled = fin, cursor, true
	in.mu.Unlock()
	if cursor >= fin {
		return true, nil
	}
	from, to := cursor+1, min(fin, cursor+MaxRange)
	logs, err := in.Chain.Logs(ctx, in.Contract, Topic0, from, to)
	if err != nil {
		return false, err
	}
	times := map[uint64]uint64{}
	rows := make([]store.Row, 0, len(logs))
	for _, l := range logs {
		if l.Removed {
			continue // never on finalized blocks; ignored if a node sends one
		}
		t, ok := times[l.BlockNumber]
		if !ok {
			if t, err = in.Chain.BlockTime(ctx, l.BlockNumber); err != nil {
				return false, err
			}
			times[l.BlockNumber] = t
		}
		row, err := Decode(in.Contract, l, t)
		if err != nil {
			return false, err
		}
		rows = append(rows, row)
	}
	if err := in.Sink.Commit(ctx, rows, to); err != nil {
		return false, err
	}
	in.mu.Lock()
	in.cursor = to
	in.mu.Unlock()
	return to == fin, nil
}

// Run polls until the context ends; errors are logged and the same range is read again.
func (in *Ingester) Run(ctx context.Context) {
	for {
		caughtUp, err := in.Step(ctx)
		if err != nil {
			log.Printf("ingest: %v (cursor kept, retrying)", err)
		}
		if err == nil && !caughtUp {
			continue // more ranges to read now
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(in.Poll):
		}
	}
}
