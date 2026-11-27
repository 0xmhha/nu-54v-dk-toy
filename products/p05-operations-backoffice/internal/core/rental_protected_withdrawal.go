package core

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
)

var errProtectedWithdrawalEvidence = errors.New("protected withdrawal evidence is indeterminate")

var (
	withdrawalExecutedTopic = crypto.Keccak256Hash([]byte("WithdrawalExecuted(address,uint256)"))
	withdrawalTransferTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	withdrawalHeldTopic     = crypto.Keccak256Hash([]byte("TransferHeld(uint256,address,address,uint256)"))
	withdrawalAcceptedTopic = crypto.Keccak256Hash([]byte("TransferAccepted(uint256,address,address,uint256)"))
)

// ProtectedWithdrawalEvidence identifies one original close and its canonical
// v1 payout, if any. An observed held token transfer is not a payout.
type ProtectedWithdrawalEvidence struct {
	State           string
	CloseHash       common.Hash
	PayoutHash      common.Hash
	AcceptanceHash  common.Hash
	Amount          *big.Int
	WithdrawAfter   uint64
	FinalizedHeight int64
	FinalizedHash   common.Hash
}

func (c *Chain) ObserveProtectedWithdrawal(ctx context.Context, device, destination common.Address, closeHash common.Hash) (ProtectedWithdrawalEvidence, error) {
	indeterminate := ProtectedWithdrawalEvidence{State: "withdrawal-indeterminate", CloseHash: closeHash}
	final, err := c.client.HeaderByNumber(ctx, c.read)
	if err != nil || final.Number == nil || !final.Number.IsInt64() {
		return indeterminate, errProtectedWithdrawalEvidence
	}
	closeHeader, closeReceipt, err := c.protectedWithdrawalReceipt(ctx, final, closeHash)
	if err != nil {
		return indeterminate, err
	}
	ext, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
	if err != nil {
		return indeterminate, err
	}
	state, err := ext.AccountOf(&bind.CallOpts{Context: ctx, BlockHash: final.Hash()}, device)
	if err != nil || state.WithdrawAddress != destination || state.ClosedAt == nil || state.ClosedAt.Sign() == 0 ||
		!state.ClosedAt.IsUint64() || state.ClosedAt.Uint64() != closeHeader.Time ||
		state.Balance == nil || state.PendingWithdrawal == nil || state.WithdrawAfter == nil || !state.WithdrawAfter.IsUint64() {
		return indeterminate, errProtectedWithdrawalEvidence
	}
	requested, err := c.protectedCloseIntent(ext, closeReceipt, device)
	if err != nil {
		return indeterminate, err
	}
	out := ProtectedWithdrawalEvidence{CloseHash: closeHash, Amount: requested, FinalizedHeight: final.Number.Int64(), FinalizedHash: final.Hash()}
	switch {
	case requested.Sign() == 0 && state.Balance.Sign() == 0 && state.PendingWithdrawal.Sign() == 0:
		out.State = "withdrawal-no-payout"
	case requested.Sign() > 0 && state.Balance.Cmp(requested) == 0 && state.PendingWithdrawal.Cmp(requested) == 0:
		out.WithdrawAfter = state.WithdrawAfter.Uint64()
		if out.WithdrawAfter == 0 {
			return indeterminate, errProtectedWithdrawalEvidence
		}
		out.State = "withdrawal-ready"
		if final.Time < out.WithdrawAfter {
			out.State = "withdrawal-delay"
		}
	case requested.Sign() > 0 && state.Balance.Sign() == 0 && state.PendingWithdrawal.Sign() == 0:
		out, err = c.protectedWithdrawalPayout(ctx, final, closeHeader.Number, device, destination, out)
		if err != nil {
			return indeterminate, err
		}
	default:
		// A late or unresolved settlement changed the amount. Do not infer
		// either clearance or payment from the resulting account balance.
		return indeterminate, errProtectedWithdrawalEvidence
	}
	current, err := c.client.HeaderByNumber(ctx, final.Number)
	if err != nil || current.Hash() != final.Hash() {
		return indeterminate, errProtectedWithdrawalEvidence
	}
	return out, nil
}

func (c *Chain) protectedCloseIntent(ext *settlementext.Settlementext, receipt *types.Receipt, device common.Address) (*big.Int, error) {
	var requested *big.Int
	closes := 0
	for _, entry := range receipt.Logs {
		if entry == nil || entry.Address != c.dep.Settlement {
			continue
		}
		if closed, err := ext.ParseAccountClosed(*entry); err == nil && closed.Device == device {
			closes++
		}
		if intent, err := ext.ParseWithdrawalRequested(*entry); err == nil && intent.Device == device {
			if requested != nil {
				return nil, errProtectedWithdrawalEvidence
			}
			requested = intent.Amount
		}
	}
	if closes != 1 || requested == nil {
		return nil, errProtectedWithdrawalEvidence
	}
	return requested, nil
}

// Untrusted log filters and single-receipt indexes only locate candidates. Both
// ordered trie commitments are checked against the canonical block header.
func (c *Chain) protectedWithdrawalReceipt(ctx context.Context, final *types.Header, hash common.Hash) (*types.Header, *types.Receipt, error) {
	hint, err := c.client.TransactionReceipt(ctx, hash)
	if err != nil || hint == nil || hint.TxHash != hash || hint.BlockNumber == nil || hint.BlockNumber.Cmp(final.Number) > 0 {
		return nil, nil, errProtectedWithdrawalEvidence
	}
	header, err := c.client.HeaderByNumber(ctx, hint.BlockNumber)
	if err != nil || header.Hash() != hint.BlockHash {
		return nil, nil, errProtectedWithdrawalEvidence
	}
	block, err := c.client.BlockByHash(ctx, header.Hash())
	if err != nil || block.Hash() != header.Hash() || types.DeriveSha(block.Transactions(), trie.NewStackTrie(nil)) != header.TxHash {
		return nil, nil, errProtectedWithdrawalEvidence
	}
	receipts, err := c.client.BlockReceipts(ctx, rpc.BlockNumberOrHashWithHash(header.Hash(), true))
	if err != nil || len(receipts) != len(block.Transactions()) {
		return nil, nil, errProtectedWithdrawalEvidence
	}
	for _, receipt := range receipts {
		if receipt == nil {
			return nil, nil, errProtectedWithdrawalEvidence
		}
	}
	if types.DeriveSha(types.Receipts(receipts), trie.NewStackTrie(nil)) != header.ReceiptHash {
		return nil, nil, errProtectedWithdrawalEvidence
	}
	for index, tx := range block.Transactions() {
		if tx.Hash() == hash && receipts[index].Status == types.ReceiptStatusSuccessful {
			return header, receipts[index], nil
		}
	}
	return nil, nil, errProtectedWithdrawalEvidence
}

func (c *Chain) protectedWithdrawalPayout(ctx context.Context, final *types.Header, from *big.Int, device, destination common.Address,
	out ProtectedWithdrawalEvidence) (ProtectedWithdrawalEvidence, error) {
	deviceTopic := common.BytesToHash(device.Bytes())
	candidates, err := c.client.FilterLogs(ctx, ethereum.FilterQuery{FromBlock: from, ToBlock: final.Number,
		Addresses: []common.Address{c.dep.Settlement}, Topics: [][]common.Hash{{withdrawalExecutedTopic}, {deviceTopic}}})
	if err != nil || len(candidates) != 1 {
		return out, errProtectedWithdrawalEvidence
	}
	payout := candidates[0].TxHash
	header, receipt, err := c.protectedWithdrawalReceipt(ctx, final, payout)
	if err != nil || header.Number.Cmp(from) < 0 || matchingWithdrawalLogs(receipt, c.dep.Settlement,
		[]common.Hash{withdrawalExecutedTopic, deviceTopic}, out.Amount) != 1 {
		return out, errProtectedWithdrawalEvidence
	}
	out.PayoutHash = payout
	source, recipient := common.BytesToHash(c.dep.Settlement.Bytes()), common.BytesToHash(destination.Bytes())
	direct := matchingWithdrawalLogs(receipt, c.dep.Token, []common.Hash{withdrawalTransferTopic, source, recipient}, out.Amount)
	var heldID common.Hash
	holds := 0
	for _, log := range receipt.Logs {
		if log != nil && len(log.Topics) == 4 && matchingWithdrawalLog(log, c.dep.Token,
			[]common.Hash{withdrawalHeldTopic, log.Topics[1], source, recipient}, out.Amount) {
			heldID = log.Topics[1]
			holds++
		}
	}
	switch {
	case direct == 1 && holds == 0:
		out.State = "withdrawal-received"
		return out, nil
	case direct == 0 && holds == 1:
		return c.protectedWithdrawalAcceptance(ctx, final, header.Number, heldID, source, recipient, out)
	default:
		return out, errProtectedWithdrawalEvidence
	}
}

func (c *Chain) protectedWithdrawalAcceptance(ctx context.Context, final *types.Header, from *big.Int, id, source, recipient common.Hash,
	out ProtectedWithdrawalEvidence) (ProtectedWithdrawalEvidence, error) {
	topics := []common.Hash{withdrawalAcceptedTopic, id, source, recipient}
	filter := make([][]common.Hash, len(topics))
	for i, topic := range topics {
		filter[i] = []common.Hash{topic}
	}
	candidates, err := c.client.FilterLogs(ctx, ethereum.FilterQuery{FromBlock: from, ToBlock: final.Number,
		Addresses: []common.Address{c.dep.Token}, Topics: filter})
	if err != nil || len(candidates) > 1 {
		return out, errProtectedWithdrawalEvidence
	}
	if len(candidates) == 0 {
		token, err := testusdc.NewTestusdc(c.dep.Token, c.client)
		if err != nil {
			return out, errProtectedWithdrawalEvidence
		}
		held, err := token.HeldTransfer(&bind.CallOpts{Context: ctx, BlockHash: final.Hash()}, id.Big())
		if err != nil || held.Amount == nil || held.HeldAt == nil || held.HeldAt.Sign() == 0 ||
			held.From != c.dep.Settlement || common.BytesToHash(held.To.Bytes()) != recipient || held.Amount.Cmp(out.Amount) != 0 {
			return out, errProtectedWithdrawalEvidence
		}
		out.State = "withdrawal-held"
		return out, nil
	}
	hash := candidates[0].TxHash
	header, receipt, err := c.protectedWithdrawalReceipt(ctx, final, hash)
	if err != nil || header.Number.Cmp(from) < 0 || matchingWithdrawalLogs(receipt, c.dep.Token, topics, out.Amount) != 1 ||
		matchingWithdrawalLogs(receipt, c.dep.Token, []common.Hash{withdrawalTransferTopic, common.BytesToHash(c.dep.Token.Bytes()), recipient}, out.Amount) != 1 {
		return out, errProtectedWithdrawalEvidence
	}
	out.State, out.AcceptanceHash = "withdrawal-received", hash
	return out, nil
}

func matchingWithdrawalLogs(receipt *types.Receipt, address common.Address, topics []common.Hash, amount *big.Int) int {
	count := 0
	for _, log := range receipt.Logs {
		if matchingWithdrawalLog(log, address, topics, amount) {
			count++
		}
	}
	return count
}

func matchingWithdrawalLog(log *types.Log, address common.Address, topics []common.Hash, amount *big.Int) bool {
	if log == nil || log.Address != address || len(topics) != len(log.Topics) || len(log.Data) != 32 ||
		new(big.Int).SetBytes(log.Data).Cmp(amount) != 0 {
		return false
	}
	for i, topic := range topics {
		if log.Topics[i] != topic {
			return false
		}
	}
	return true
}

// PrepareProtectedWithdrawal creates the exact transaction without transmitting
// it. The caller persists its hash before calling PublishProtectedWithdrawal.
func (c *Chain) PrepareProtectedWithdrawal(ctx context.Context, signer *ecdsa.PrivateKey, device common.Address) (*types.Transaction, error) {
	ext, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
	if err != nil {
		return nil, err
	}
	opts, err := c.Transactor(ctx, signer)
	if err != nil {
		return nil, err
	}
	opts.NoSend = true
	return ext.ExecuteWithdrawal(opts, device)
}

func (c *Chain) PublishProtectedWithdrawal(ctx context.Context, tx *types.Transaction) error {
	return c.client.SendTransaction(ctx, tx)
}

// ProtectedWithdrawalUnseen is conservative: a known pending/mined original,
// anomalous provider answer or a consumed/reserved signer nonce bars sending.
func (c *Chain) ProtectedWithdrawalUnseen(ctx context.Context, signer common.Address, nonce uint64, hash common.Hash) (bool, error) {
	receipt, err := c.client.TransactionReceipt(ctx, hash)
	if err == nil || receipt != nil {
		return false, nil
	}
	if !errors.Is(err, ethereum.NotFound) {
		return false, err
	}
	tx, _, err := c.client.TransactionByHash(ctx, hash)
	if err == nil || tx != nil {
		return false, nil
	}
	if !errors.Is(err, ethereum.NotFound) {
		return false, err
	}
	final, err := c.client.HeaderByNumber(ctx, c.read)
	if err != nil {
		return false, err
	}
	finalNonce, err := c.rentalCloseNonceAt(ctx, signer, final)
	if err != nil || finalNonce > nonce {
		return false, err
	}
	pending, err := c.client.PendingNonceAt(ctx, signer)
	return err == nil && pending == nonce, err
}

func (c *Chain) DecodeProtectedWithdrawal(raw []byte, device, signer common.Address, nonce uint64, expected common.Hash) (*types.Transaction, error) {
	var tx types.Transaction
	if len(raw) == 0 || len(raw) > 1024 || tx.UnmarshalBinary(raw) != nil || tx.Hash() != expected || tx.Nonce() != nonce ||
		tx.To() == nil || *tx.To() != c.dep.Settlement || tx.Value().Sign() != 0 {
		return nil, errProtectedWithdrawalEvidence
	}
	chain := new(big.Int).SetUint64(c.dep.ChainID)
	sender, err := types.Sender(types.LatestSignerForChainID(chain), &tx)
	if err != nil || !tx.Protected() || tx.ChainId().Cmp(chain) != 0 || sender != signer || c.dep.ChainID != 8283 {
		return nil, errProtectedWithdrawalEvidence
	}
	abi, err := settlementext.SettlementextMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	want, err := abi.Pack("executeWithdrawal", device)
	if err != nil || !bytes.Equal(want, tx.Data()) {
		return nil, errProtectedWithdrawalEvidence
	}
	return &tx, nil
}
