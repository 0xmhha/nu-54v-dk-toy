package settlement

import (
	"bytes"
	"context"
	"math/big"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/finality"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type RevertedAttempt struct {
	Transaction       string `json:"transaction"`
	Block             string `json:"block"`
	BlockHeight       uint64 `json:"blockHeight"`
	TransactionIndex  uint   `json:"transactionIndex"`
	GasUsed           uint64 `json:"gasUsed"`
	EffectivePriceWei string `json:"effectivePriceWei"`
	FeeWei            string `json:"feeWei"`
	GasPayer          string `json:"gasPayer"`
	PrincipalDelta    string `json:"principalDelta"`
	OrderConsumed     bool   `json:"orderConsumed"`
}

func (h FinanceHTTP) revertedPayment(ctx context.Context, target Target, quote Quote, relayHash string) *RevertedAttempt {
	chain, ok := h.Funds.(LiveChain)
	if !ok || h.Evidence == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	receipt, err := finality.CanonicalReceipt(ctx, chain.Client, common.HexToHash(relayHash))
	if err != nil || receipt.Receipt.Status != types.ReceiptStatusFailed || !receipt.Receipt.BlockNumber.IsInt64() {
		return nil
	}
	tx := receipt.Transaction
	// The normal relay owns legacy EIP-155 transactions. Their signed gas
	// price is the effective price for this receipt.
	if tx.Type() != types.LegacyTxType || !tx.Protected() || tx.ChainId().Cmp(big.NewInt(8283)) != 0 ||
		tx.To() == nil || *tx.To() != target.Contract || tx.Value().Sign() != 0 || !exactPaymentCall(tx.Data(), quote) {
		return nil
	}
	price := tx.GasPrice()
	if receipt.Receipt.EffectiveGasPrice != nil && price.Cmp(receipt.Receipt.EffectiveGasPrice) != 0 {
		return nil
	}
	payer, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8283)), tx)
	if err != nil {
		return nil
	}
	var anchored bool
	err = h.Evidence.db.QueryRow(ctx, "SELECT chainview.reconciliation_finalized_block($1,$2,$3,$4)",
		target.Stream, common.Hash(quote.Args.MerchantId).Hex(), receipt.Receipt.BlockNumber.Int64(), receipt.Receipt.BlockHash.Hex()).Scan(&anchored)
	if err != nil || !anchored {
		return nil
	}
	// A status-zero receipt proven against the block receipt root rolls
	// back this call's state and token changes only. It says nothing about
	// an independent later payment or quote-expiry evidence.
	gas := receipt.Receipt.GasUsed
	fee := new(big.Int).Mul(new(big.Int).SetUint64(gas), price)
	return &RevertedAttempt{Transaction: relayHash, Block: receipt.Receipt.BlockHash.Hex(), BlockHeight: receipt.Receipt.BlockNumber.Uint64(),
		TransactionIndex: receipt.Receipt.TransactionIndex, GasUsed: gas, EffectivePriceWei: price.String(),
		FeeWei: fee.String(), GasPayer: lower(payer), PrincipalDelta: "0", OrderConsumed: false}
}

func exactPaymentCall(data []byte, quote Quote) bool {
	codec, err := v2.Settlementv2MetaData.GetAbi()
	if err != nil {
		return false
	}
	method := codec.Methods["settle"]
	if len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
		return false
	}
	args, err := method.Inputs.Unpack(data[4:])
	if err != nil || len(args) != 3 {
		return false
	}
	approval, ok := args[2].([]byte)
	if !ok || !matchesSigner(quote.ApprovalHash, approval, quote.Args.Payer) {
		return false
	}
	expected, err := codec.Pack("settle", quote.Args, quote.MerchantSig, approval)
	return err == nil && bytes.Equal(expected, data)
}
