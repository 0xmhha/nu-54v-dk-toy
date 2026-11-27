package market

import (
	"errors"
	"math/big"
	"net/http"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/finality"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func (h Handler) approval(w http.ResponseWriter, r *http.Request) {
	var hash common.Hash
	user := r.PathValue("user")
	if hash.UnmarshalText([]byte(r.PathValue("hash"))) != nil || !common.IsHexAddress(user) {
		fail(w, 400, "invalid original approval")
		return
	}
	ctx := r.Context()
	if _, err := h.Chain.Head(ctx); err != nil {
		fail(w, 503, "market unavailable")
		return
	}
	proof, err := finality.CanonicalReceipt(ctx, h.Chain.RPC, hash)
	out := Result{TxHash: hash, State: "pending"}
	switch {
	case errors.Is(err, ethereum.NotFound), errors.Is(err, finality.ErrPending):
		reply(w, 200, out)
		return
	case err != nil:
		fail(w, 409, "could not reconcile token approval")
		return
	}
	tx, receipt := proof.Transaction, proof.Receipt
	to := tx.To()
	p := h.Chain.Profile
	method, err := tokenABI.MethodById(tx.Data())
	sender, signatureErr := types.Sender(types.LatestSignerForChainID(big.NewInt(ChainID)), tx)
	if to == nil || (*to != p.Base.Address && *to != p.Quote.Address) || err != nil || method.Name != "approve" ||
		signatureErr != nil || sender != common.HexToAddress(user) || receipt.BlockNumber.Uint64() < p.Height {
		fail(w, 409, "proof differs from the original token approval")
		return
	}
	values, err := method.Inputs.Unpack(tx.Data()[4:])
	if err != nil || values[0].(common.Address) != p.Venue.Address {
		fail(w, 409, "proof differs from the isolated market allowance")
		return
	}
	out.Block, out.BlockHash = receipt.BlockNumber.Uint64(), receipt.BlockHash
	out.GasFee = legacyGasFee(tx, receipt)
	if receipt.Status != types.ReceiptStatusSuccessful {
		out.State = "rejected"
		reply(w, 200, out)
		return
	}
	event := tokenABI.Events["Approval"]
	for _, log := range receipt.Logs {
		if log.Address != *to || len(log.Topics) != 3 || log.Topics[0] != event.ID ||
			log.Topics[1] != common.BytesToHash(sender.Bytes()) || log.Topics[2] != common.BytesToHash(p.Venue.Address.Bytes()) {
			continue
		}
		fields, err := event.Inputs.NonIndexed().Unpack(log.Data)
		if err == nil && fields[0].(*big.Int).Cmp(values[1].(*big.Int)) == 0 {
			out.State = "confirmed"
			reply(w, 200, out)
			return
		}
	}
	fail(w, 409, "missing finalized allowance event")
}
