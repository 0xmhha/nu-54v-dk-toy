package core

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
)

type RentalCloseTransaction struct {
	Hash   common.Hash
	Nonce  uint64
	Signed []byte
}

type RentalCloseChainState string

const (
	RentalCloseNotObserved RentalCloseChainState = "not-observed"
	RentalCloseSubmitted   RentalCloseChainState = "submitted"
	RentalCloseConfirmed   RentalCloseChainState = "confirmed"
	RentalCloseReverted    RentalCloseChainState = "reverted"
	RentalCloseSuperseded  RentalCloseChainState = "superseded"
)

func (c *Chain) PrepareRentalClose(ctx context.Context, operator *ecdsa.PrivateKey, device common.Address) (RentalCloseTransaction, error) {
	contract, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
	if err != nil {
		return RentalCloseTransaction{}, err
	}
	opts, err := c.Transactor(ctx, operator)
	if err != nil {
		return RentalCloseTransaction{}, err
	}
	opts.NoSend = true
	tx, err := contract.CloseAccount(opts, device)
	if err != nil {
		return RentalCloseTransaction{}, err
	}
	data, err := tx.MarshalBinary()
	if err != nil {
		return RentalCloseTransaction{}, err
	}
	return RentalCloseTransaction{Hash: tx.Hash(), Nonce: tx.Nonce(), Signed: data}, nil
}

func parseRentalCloseTransaction(deployment Deployment, signer, device common.Address, raw []byte) (*types.Transaction, error) {
	var tx types.Transaction
	if len(raw) > 1024 || len(raw) == 0 || tx.UnmarshalBinary(raw) != nil {
		return nil, errors.New("invalid protected close transaction")
	}
	abi, err := settlementext.SettlementextMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	data, err := abi.Pack("closeAccount", device)
	if err != nil {
		return nil, err
	}
	chain := new(big.Int).SetUint64(deployment.ChainID)
	sender, err := types.Sender(types.LatestSignerForChainID(chain), &tx)
	if err != nil || deployment.ChainID != 8283 || signer == (common.Address{}) || device == (common.Address{}) ||
		!tx.Protected() || tx.ChainId().Cmp(chain) != 0 || sender != signer || tx.To() == nil || *tx.To() != deployment.Settlement ||
		tx.Value().Sign() != 0 || !bytes.Equal(tx.Data(), data) {
		return nil, errors.New("protected close transaction scope differs")
	}
	return &tx, nil
}

// InspectRentalClose uses the original hash before considering a retry or
// treating a nonce as consumed by a different finalized transaction.
func (c *Chain) InspectRentalClose(ctx context.Context, signer, device common.Address, candidate RentalCloseTransaction) (RentalCloseChainState, error) {
	receipt, err := c.client.TransactionReceipt(ctx, candidate.Hash)
	if err != nil && !errors.Is(err, ethereum.NotFound) {
		return RentalCloseNotObserved, err
	}
	final, err := c.client.HeaderByNumber(ctx, c.read)
	if err != nil {
		return RentalCloseNotObserved, err
	}
	currentFinal := func() bool {
		checked, err := c.client.HeaderByNumber(ctx, final.Number)
		return err == nil && checked.Hash() == final.Hash()
	}
	if receipt != nil {
		if receipt.TxHash != candidate.Hash || receipt.BlockNumber == nil || receipt.BlockHash == (common.Hash{}) {
			return RentalCloseNotObserved, errors.New("protected close receipt identity differs")
		}
		mined, err := c.client.HeaderByNumber(ctx, receipt.BlockNumber)
		if err != nil && !errors.Is(err, ethereum.NotFound) {
			return RentalCloseNotObserved, err
		}
		if mined != nil && mined.Hash() == receipt.BlockHash {
			if receipt.BlockNumber.Cmp(final.Number) > 0 {
				return RentalCloseSubmitted, nil
			}
			verified, err := c.finalizedRentalCloseReceipt(ctx, mined, candidate)
			if err != nil || !currentFinal() {
				return RentalCloseNotObserved, errors.New("protected close final receipt could not be verified")
			}
			receipt = verified
			if receipt.Status == types.ReceiptStatusFailed {
				return RentalCloseReverted, nil
			}
			ext, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
			if err != nil {
				return RentalCloseNotObserved, err
			}
			for _, event := range receipt.Logs {
				if event == nil || event.Address != c.dep.Settlement {
					continue
				}
				closed, err := ext.ParseAccountClosed(*event)
				if err == nil && closed.Device == device && receipt.Status == types.ReceiptStatusSuccessful {
					return RentalCloseConfirmed, nil
				}
			}
			return RentalCloseNotObserved, errors.New("final receipt did not prove the original account close")
		}
	}
	found, pending, err := c.client.TransactionByHash(ctx, candidate.Hash)
	if err == nil {
		if found == nil || found.Hash() != candidate.Hash {
			return RentalCloseNotObserved, errors.New("protected close lookup returned a different transaction")
		}
		if pending || receipt == nil {
			return RentalCloseSubmitted, nil
		}
		// A stale noncanonical receipt is never a finalized observation.
	} else if !errors.Is(err, ethereum.NotFound) {
		return RentalCloseNotObserved, err
	}
	nonce, err := c.rentalCloseNonceAt(ctx, signer, final)
	if err != nil {
		return RentalCloseNotObserved, err
	}
	if nonce > candidate.Nonce {
		consuming, err := c.finalizedRentalCloseNonceUse(ctx, signer, candidate.Nonce, final)
		if err != nil {
			return RentalCloseNotObserved, err
		}
		if !currentFinal() {
			return RentalCloseNotObserved, errRentalCloseNonceEvidence
		}
		if consuming == candidate.Hash {
			// The canonical body includes the original, but an unavailable
			// receipt cannot establish success or failure. Keep its bytes.
			return RentalCloseSubmitted, nil
		}
		return RentalCloseSuperseded, nil
	}
	if !currentFinal() {
		return RentalCloseNotObserved, errRentalCloseNonceEvidence
	}
	return RentalCloseNotObserved, nil
}

// Ethereum authenticates receipt status and event data at the transaction's
// position in the block. RPC-provided receipt metadata is only a lookup hint.
func (c *Chain) finalizedRentalCloseReceipt(ctx context.Context, header *types.Header, candidate RentalCloseTransaction) (*types.Receipt, error) {
	invalid := errors.New("invalid finalized protected close receipt commitments")
	hash := header.Hash()
	body, err := c.client.BlockByHash(ctx, hash)
	if err != nil || body.Hash() != hash || types.DeriveSha(body.Transactions(), trie.NewStackTrie(nil)) != header.TxHash {
		return nil, invalid
	}
	receipts, err := c.client.BlockReceipts(ctx, rpc.BlockNumberOrHashWithHash(hash, true))
	if err != nil || len(receipts) != len(body.Transactions()) {
		return nil, invalid
	}
	for _, receipt := range receipts {
		if receipt == nil {
			return nil, invalid
		}
	}
	if types.DeriveSha(types.Receipts(receipts), trie.NewStackTrie(nil)) != header.ReceiptHash {
		return nil, invalid
	}
	current, err := c.client.HeaderByNumber(ctx, header.Number)
	if err != nil || current.Hash() != hash {
		return nil, invalid
	}
	for index, tx := range body.Transactions() {
		if tx.Hash() == candidate.Hash && tx.Nonce() == candidate.Nonce {
			return receipts[index], nil
		}
	}
	return nil, invalid
}

func (c *Chain) PublishRentalClose(ctx context.Context, signer, device common.Address, candidate RentalCloseTransaction) error {
	tx, err := parseRentalCloseTransaction(c.dep, signer, device, candidate.Signed)
	if err != nil || tx.Hash() != candidate.Hash || tx.Nonce() != candidate.Nonce {
		return errors.New("original protected close transaction differs")
	}
	return c.client.SendTransaction(ctx, tx)
}
