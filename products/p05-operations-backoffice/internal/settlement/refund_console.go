package settlement

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
)

func (h FinanceHTTP) consoleRefund(ctx context.Context, actor service.Principal, branch, sale string) (map[string]any, error) {
	var data []byte
	if err := h.Store.db.QueryRow(ctx, "SELECT ledger.console_refunds($1,$2,$3,$4)",
		actor.Tenant, branch, actor.Subject, sale).Scan(&data); err != nil {
		return nil, err
	}
	var view map[string]any
	err := json.Unmarshal(data, &view)
	return view, err
}

func (h FinanceHTTP) currentRefundFunds(ctx context.Context, branch, merchant string) (*big.Int, error) {
	target, ok := h.Targets[branch]
	if !ok || merchant == "" || h.Merchants[branch] != merchant || h.Funds == nil {
		return nil, service.Forbidden
	}
	deadline, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	return h.Funds.RefundReserve(deadline, target, common.HexToHash(merchant))
}

func unreservedRefundFunds(balance *big.Int, committed string) string {
	queued, ok := new(big.Int).SetString(committed, 10)
	if !ok || queued.Sign() < 0 || balance.Cmp(queued) < 0 {
		return "0"
	}
	return new(big.Int).Sub(balance, queued).String()
}

type rentalRefundChain interface {
	RefundWithdrawal(context.Context, Target, common.Address, uint64) (common.Address, error)
}

func (h FinanceHTTP) refundRoute(ctx context.Context, branch string, view map[string]any) string {
	withdrawal, rental := view["rentalWithdrawal"].(string)
	if !rental || withdrawal == "" {
		if view["rentalPayer"] == true {
			return "rental-held"
		}
		return "original"
	}
	source, ok := h.Funds.(rentalRefundChain)
	target, configured := h.Targets[branch]
	payer, _ := view["payer"].(string)
	generation, _ := view["payerGeneration"].(string)
	g, err := strconv.ParseUint(generation, 10, 64)
	if !ok || !configured || !common.IsHexAddress(payer) || err != nil || g == 0 {
		return "rental-held"
	}
	probe, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	actual, err := source.RefundWithdrawal(probe, target, common.HexToAddress(payer), g)
	if err != nil || actual == (common.Address{}) || lower(actual) != withdrawal {
		return "rental-held"
	}
	return "rental-verified"
}

func (c LiveChain) RefundWithdrawal(ctx context.Context, target Target, payer common.Address, generation uint64) (common.Address, error) {
	header, err := c.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil {
		return common.Address{}, err
	}
	code, err := c.CodeAtHash(ctx, target.Contract, header.Hash())
	if err != nil || len(code) == 0 || crypto.Keccak256Hash(code) != target.CodeHash {
		return common.Address{}, service.Forbidden
	}
	caller, err := v2.NewSettlementv2Caller(target.Contract, c.Client)
	if err != nil {
		return common.Address{}, err
	}
	// This mapping is immutable for an existing authority generation. The
	// settlement contract refunds that generation even after payer rotation.
	return caller.EscrowWithdrawalAddress(&bind.CallOpts{Context: ctx, BlockHash: header.Hash()}, payer, generation)
}

func (h FinanceHTTP) refundBlocked(ctx context.Context, w http.ResponseWriter, code string) {
	if err := h.Store.db.CheckRead(ctx); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
