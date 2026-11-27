package chainview

import (
	"bytes"
	"errors"
	"math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// orderKinds are the OrderEscrow events the operations server acts on. Settings events are not facts about an order.
var orderKinds = map[string]string{
	"OrderLocked": "locked", "OrderDelivered": "delivered", "OrderDisputed": "disputed",
	"OrderReleased": "released", "OrderCancelled": "cancelled", "ClaimSkipped": "claim-skipped",
}

var unknownOrder = common.Hash{31: 1} // SkipReason.Unknown

var errOrderEvent = errors.New("invalid order escrow event")

// matchesOrder ignores a skipped claim for an order that was never locked, including a zero merchant or order.
// claimBatch is open to anyone and reports any id it is given, so treating a zero id as a malformed receipt
// would let a stranger stop the stream, and recording invented orders would let one fill it.
func (d *Decoder) matchesOrder(log types.Log) bool {
	event, ok := d.orders[log.Topics[0]]
	if !ok {
		return false
	}
	return event.Name != "ClaimSkipped" || len(log.Topics) != 3 ||
		log.Topics[1] != (common.Hash{}) && log.Topics[2] != (common.Hash{}) && common.BytesToHash(log.Data) != unknownOrder
}

// repeatedSkip keeps the first reason a transaction reports for an order, since a batch may name one order many times.
func repeatedSkip(seen map[[2]string]bool, event Event) bool {
	if event.Kind != "claim-skipped" {
		return false
	}
	key := [2]string{event.Merchant, event.Order}
	repeated := seen[key]
	seen[key] = true
	return repeated
}

func (d *Decoder) decodeOrder(log types.Log) (Event, error) {
	event := d.orders[log.Topics[0]]
	fields := event.Inputs.NonIndexed()
	values, err := fields.Unpack(log.Data)
	if err != nil {
		return Event{}, err
	}
	canonical, err := fields.PackValues(values)
	if err != nil || !bytes.Equal(canonical, log.Data) {
		return Event{}, errors.New("noncanonical order escrow encoding")
	}
	if log.Topics[1] == (common.Hash{}) || log.Topics[2] == (common.Hash{}) {
		return Event{}, errOrderEvent
	}
	out := Event{Kind: orderKinds[event.Name], Merchant: log.Topics[1].Hex(), Order: log.Topics[2].Hex(),
		TxHash: log.TxHash.Hex(), Index: int32(log.Index)}
	switch event.Name {
	case "OrderLocked":
		locked, err := d.escrow.ParseOrderLocked(log)
		if err != nil {
			return Event{}, err
		}
		if !wordAddress(log.Topics[3][:]) || locked.Payer == (common.Address{}) || locked.Beneficiary == (common.Address{}) ||
			locked.Confirmer == (common.Address{}) || !positive(locked.Amount) || locked.QuoteDigest == ([32]byte{}) ||
			!unixTime(locked.DeliverBy) || locked.ConfirmWindow == 0 {
			return Event{}, errOrderEvent
		}
		out.Asset, out.Amount = lower(d.profile.Asset), locked.Amount.String()
		out.Payer, out.Recipient, out.Confirmer = lower(locked.Payer), lower(locked.Beneficiary), lower(locked.Confirmer)
		out.DeliverBy, out.ConfirmWindow = int64(locked.DeliverBy), int64(locked.ConfirmWindow)
		out.QuoteDigest = common.Hash(locked.QuoteDigest).Hex()
	case "OrderDelivered":
		delivered, err := d.escrow.ParseOrderDelivered(log)
		if err != nil || !unixTime(delivered.ClaimableAt) {
			return Event{}, errOrderEvent
		}
		out.ClaimableAt = int64(delivered.ClaimableAt)
	case "OrderDisputed":
		disputed, err := d.escrow.ParseOrderDisputed(log)
		if err != nil || !unixTime(disputed.ResolveBy) {
			return Event{}, errOrderEvent
		}
		out.ResolveBy = int64(disputed.ResolveBy)
	case "OrderReleased":
		released, err := d.escrow.ParseOrderReleased(log)
		if err != nil || released.Beneficiary == (common.Address{}) || !positive(released.Amount) || released.Reason > 3 {
			return Event{}, errOrderEvent
		}
		out.Asset, out.Amount, out.Recipient = lower(d.profile.Asset), released.Amount.String(), lower(released.Beneficiary)
		out.Reason = reason(released.Reason)
	case "OrderCancelled":
		cancelled, err := d.escrow.ParseOrderCancelled(log)
		if err != nil || cancelled.Payer == (common.Address{}) || !positive(cancelled.Amount) || cancelled.Reason > 3 {
			return Event{}, errOrderEvent
		}
		out.Asset, out.Amount, out.Payer = lower(d.profile.Asset), cancelled.Amount.String(), lower(cancelled.Payer)
		out.Reason = reason(cancelled.Reason)
	default:
		skipped, err := d.escrow.ParseClaimSkipped(log)
		if err != nil || skipped.Reason == 0 || skipped.Reason > 6 {
			return Event{}, errOrderEvent
		}
		out.Reason = reason(skipped.Reason)
	}
	return out, nil
}

func unixTime(value uint64) bool { return value != 0 && value <= math.MaxInt64 }

func reason(value uint8) *int16 {
	out := int16(value)
	return &out
}
