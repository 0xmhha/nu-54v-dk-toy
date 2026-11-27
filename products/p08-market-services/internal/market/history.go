package market

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"sort"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/finality"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

type PositionEvent struct {
	Kind       string      `json:"kind"`
	TxHash     common.Hash `json:"txHash"`
	Block      uint64      `json:"block"`
	Period     string      `json:"period,omitempty"`
	Version    string      `json:"priceVersion,omitempty"`
	Payment    string      `json:"payment,omitempty"`
	Margin     string      `json:"margin,omitempty"`
	Payout     string      `json:"payout,omitempty"`
	FundingGap string      `json:"fundingGap,omitempty"`
}

func (h Handler) history(w http.ResponseWriter, r *http.Request) {
	id, err := units(r.PathValue("id"))
	if err != nil || id.Sign() == 0 {
		fail(w, 400, "invalid position")
		return
	}
	events, err := h.Chain.History(r.Context(), id)
	if err != nil {
		fail(w, 409, "position history could not be reconciled")
		return
	}
	reply(w, 200, events)
}

func (c *Chain) History(ctx context.Context, id *big.Int) ([]PositionEvent, error) {
	if _, err := c.Head(ctx); err != nil {
		return nil, err
	}
	head, err := c.RPC.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil || !head.Number.IsUint64() {
		return nil, errors.New("finalized head unavailable")
	}
	events := make([]PositionEvent, 0, 32)
	if head.Number.Uint64() < c.Profile.Height {
		return events, nil
	}
	start := c.Profile.Height
	if later := head.Number.Uint64(); later-start > 4096 {
		start = later - 4096
	}
	opened := venueABI.Events["PositionOpened"]
	funded := venueABI.Events["PositionFunded"]
	closed := venueABI.Events["PositionClosed"]
	logs, err := c.RPC.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(start), ToBlock: head.Number, Addresses: []common.Address{c.Profile.Venue.Address},
		Topics: [][]common.Hash{{opened.ID, funded.ID, closed.ID}, {common.BigToHash(id)}},
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].BlockNumber == logs[j].BlockNumber {
			return logs[i].Index > logs[j].Index
		}
		return logs[i].BlockNumber > logs[j].BlockNumber
	})
	for _, hint := range logs[:min(len(logs), 32)] {
		if len(hint.Topics) != 3 {
			return nil, errors.New("invalid position log hint")
		}
		proof, err := finality.CanonicalReceipt(ctx, c.RPC, hint.TxHash)
		if err != nil {
			return nil, err
		}
		if proof.Receipt.BlockNumber.Cmp(head.Number) > 0 || proof.Receipt.Status != types.ReceiptStatusSuccessful ||
			proof.Receipt.BlockHash != hint.BlockHash {
			return nil, errors.New("position hint differs from canonical receipt")
		}
		found := false
		for _, log := range proof.Receipt.Logs {
			if log.Index != hint.Index || log.Address != c.Profile.Venue.Address ||
				len(log.Topics) != 3 || log.Topics[1] != common.BigToHash(id) ||
				log.Topics[0] != hint.Topics[0] {
				continue
			}
			event := PositionEvent{TxHash: hint.TxHash, Block: proof.Receipt.BlockNumber.Uint64()}
			switch log.Topics[0] {
			case opened.ID:
				values, err := opened.Inputs.NonIndexed().Unpack(log.Data)
				if err != nil {
					return nil, err
				}
				event.Kind, event.Margin = "opened", values[1].(*big.Int).String()
			case funded.ID:
				values, err := funded.Inputs.NonIndexed().Unpack(log.Data)
				if err != nil {
					return nil, err
				}
				event.Kind, event.Period = "funded", log.Topics[2].Big().String()
				event.Version, event.Payment, event.Margin = values[1].(*big.Int).String(), values[2].(*big.Int).String(), values[3].(*big.Int).String()
			case closed.ID:
				values, err := closed.Inputs.NonIndexed().Unpack(log.Data)
				if err != nil {
					return nil, err
				}
				event.Kind = "closed"
				if log.Topics[2].Big().Cmp(big.NewInt(1)) == 0 {
					event.Kind = "liquidated"
				}
				event.Version, event.Payout, event.FundingGap = values[1].(*big.Int).String(), values[2].(*big.Int).String(), values[3].(*big.Int).String()
			}
			events = append(events, event)
			found = true
		}
		if !found {
			return nil, errors.New("position hint has no canonical event")
		}
	}
	return events, nil
}
