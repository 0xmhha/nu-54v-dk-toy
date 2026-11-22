package market

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/finality"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type Position struct {
	Owner         common.Address `json:"owner"`
	Long          bool           `json:"long"`
	Closed        bool           `json:"closed"`
	InitialMargin string         `json:"initialMargin"`
	Margin        string         `json:"margin"`
	Size          string         `json:"size"`
	EntryPrice    string         `json:"entryPrice"`
	Revision      string         `json:"revision"`
	NextPeriod    string         `json:"nextPeriod"`
}

func (c *Chain) Position(ctx context.Context, at common.Hash, id *big.Int) (Position, error) {
	values, err := c.Venue(ctx, at, "positions", id)
	if err != nil {
		return Position{}, err
	}
	txt := func(index int) string { return values[index].(*big.Int).String() }
	return Position{
		Owner: values[0].(common.Address), Long: values[1].(bool), Closed: values[2].(bool), InitialMargin: txt(3),
		Margin: txt(4), Size: txt(5), EntryPrice: txt(6), Revision: txt(7), NextPeriod: txt(8),
	}, nil
}

type Result struct {
	TxHash     common.Hash `json:"txHash"`
	State      string      `json:"state"`
	Block      uint64      `json:"block,omitempty"`
	BlockHash  common.Hash `json:"blockHash,omitempty"`
	GasFee     string      `json:"gasFee,omitempty"`
	Action     string      `json:"action,omitempty"`
	First      string      `json:"first,omitempty"`
	Second     string      `json:"second,omitempty"`
	AddedBase  string      `json:"addedBase,omitempty"`
	AddedQuote string      `json:"addedQuote,omitempty"`
}

func (c *Chain) Result(ctx context.Context, hash, digest common.Hash) (Result, error) {
	if hash == (common.Hash{}) || digest == (common.Hash{}) {
		return Result{}, errors.New("original quote and transaction hashes required")
	}
	if _, err := c.Head(ctx); err != nil {
		return Result{}, err
	}
	result := Result{TxHash: hash, State: "pending"}
	proof, err := finality.CanonicalReceipt(ctx, c.RPC, hash)
	if errors.Is(err, ethereum.NotFound) || errors.Is(err, finality.ErrPending) {
		return result, nil
	}
	if err != nil {
		return Result{}, err
	}
	tx, receipt := proof.Transaction, proof.Receipt
	if tx.To() == nil || *tx.To() != c.Profile.Venue.Address || receipt.BlockNumber.Uint64() < c.Profile.Height {
		return Result{}, errors.New("proof transaction differs from market")
	}
	input := tx.Data()
	method, err := venueABI.MethodById(input)
	if err != nil || method.Name != "execute" {
		return Result{}, errors.New("proof is not a user execution")
	}
	values, err := method.Inputs.Unpack(input[4:])
	if err != nil {
		return Result{}, err
	}
	decoded, ok := abiOrder(values[0])
	if !ok {
		return Result{}, errors.New("invalid proof quote tuple")
	}
	want := quoteDigest(c.Profile, decoded)
	sender, signatureErr := types.Sender(types.LatestSignerForChainID(big.NewInt(ChainID)), tx)
	if digest != want || signatureErr != nil || sender != decoded.User {
		return Result{}, errors.New("execution hash differs from original quote")
	}
	result.Block, result.BlockHash = receipt.BlockNumber.Uint64(), receipt.BlockHash
	result.GasFee = legacyGasFee(tx, receipt)
	if receipt.Status == types.ReceiptStatusFailed {
		result.State = "rejected"
		return result, nil
	}
	event := venueABI.Events["Executed"]
	liquidity := venueABI.Events["LiquidityAdded"]
	for _, log := range receipt.Logs {
		if log.Address != c.Profile.Venue.Address || len(log.Topics) < 2 || log.Topics[1] != digest {
			continue
		}
		switch log.Topics[0] {
		case event.ID:
			if result.State == "confirmed" || len(log.Topics) != 4 || log.Topics[3].Big().Cmp(big.NewInt(int64(decoded.Action))) != 0 ||
				log.Topics[2] != common.BytesToHash(decoded.User.Bytes()) {
				return Result{}, errors.New("ambiguous market execution event")
			}
			fields, err := event.Inputs.NonIndexed().Unpack(log.Data)
			if err != nil {
				return Result{}, err
			}
			result.State = "confirmed"
			result.Action = actions[decoded.Action]
			result.First = fields[0].(*big.Int).String()
			result.Second = fields[1].(*big.Int).String()
		case liquidity.ID:
			if decoded.Action != 2 || result.AddedBase != "" || len(log.Topics) != 3 ||
				log.Topics[2] != common.BytesToHash(decoded.User.Bytes()) {
				return Result{}, errors.New("ambiguous liquidity event")
			}
			fields, err := liquidity.Inputs.NonIndexed().Unpack(log.Data)
			if err != nil {
				return Result{}, err
			}
			result.AddedBase = fields[0].(*big.Int).String()
			result.AddedQuote = fields[1].(*big.Int).String()
		}
	}
	if result.State != "confirmed" || (decoded.Action == 2 && result.AddedBase == "") {
		return Result{}, fmt.Errorf("market receipt missing action evidence for %s", hash)
	}
	return result, nil
}

func legacyGasFee(tx *types.Transaction, receipt *types.Receipt) string {
	if tx.Type() != types.LegacyTxType {
		return ""
	}
	return new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), tx.GasPrice()).String()
}
