package chainview

import (
	"context"
	"math/big"

	v1 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	extended "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/finality"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
)

func (s *RPCSource) generationDeploymentAt(ctx context.Context, profile Profile, anchor Header) bool {
	pinned := s.decoder.profile
	pinned.AssetDecimals = profile.AssetDecimals
	if pinned != profile {
		return false
	}
	var code hexutil.Bytes
	err := s.client.Client().CallContext(ctx, &code, "eth_getCode", profile.Contract,
		rpc.BlockNumberOrHashWithHash(anchor.Hash, true))
	return err == nil && len(code) != 0 && crypto.Keccak256Hash(code) == profile.CodeHash
}

func (s *RPCSource) generationNonceUsed(ctx context.Context, account common.Address, nonce common.Hash, anchor Header) (bool, error) {
	legacy, err := v1.SettlementMetaData.GetAbi()
	if err != nil {
		return false, err
	}
	call, err := legacy.Pack("isNonceUsed", account, new(big.Int).SetBytes(nonce[:]))
	if err != nil {
		return false, err
	}
	raw, err := s.callAt(ctx, s.decoder.profile.Contract, call, anchor.Hash)
	if err != nil || len(raw) != 32 {
		return false, errGenerationUnresolved
	}
	values, err := legacy.Unpack("isNonceUsed", raw)
	if err != nil || len(values) != 1 {
		return false, errGenerationUnresolved
	}
	return values[0].(bool), nil
}

// Candidate logs are hints. A missing candidate yields incomplete. A positive
// requires the actual block transaction and receipt roots plus the exact event.
func (s *RPCSource) generationUses(ctx context.Context, account common.Address, anchor Header, wanted map[common.Hash]bool) (map[common.Hash]generationOccurrence, error) {
	const maxCandidates = 512
	profile := s.decoder.profile
	legacy, err := v1.SettlementMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	extra, err := extended.SettlementextMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	accountTopic := common.BytesToHash(account[:])
	filters := [][][]common.Hash{
		{{legacy.Events["PaymentSettled"].ID}, nil, nil, {accountTopic}},
		{{extra.Events["LimitsChanged"].ID}, {accountTopic}},
	}
	blocks := make(map[int64]common.Hash)
	for _, topics := range filters {
		logs, err := s.client.FilterLogs(ctx, ethereum.FilterQuery{FromBlock: big.NewInt(profile.StartHeight),
			ToBlock: big.NewInt(anchor.Height), Addresses: []common.Address{profile.Contract}, Topics: topics})
		if err != nil || len(logs) > maxCandidates {
			return nil, errGenerationUnresolved
		}
		for _, log := range logs {
			height := int64(log.BlockNumber)
			if height < profile.StartHeight || height > anchor.Height || log.Removed {
				return nil, errGenerationUnresolved
			}
			if hash, exists := blocks[height]; exists && hash != log.BlockHash {
				return nil, errGenerationUnresolved
			}
			blocks[height] = log.BlockHash
		}
	}
	matched := make(map[common.Hash]generationOccurrence, len(wanted))
	for height, hash := range blocks {
		head, err := s.Header(ctx, height)
		if err != nil || head.Hash != hash {
			return nil, errGenerationUnresolved
		}
		block, err := s.client.BlockByHash(ctx, hash)
		if err != nil || block.Hash() != head.Hash {
			return nil, errGenerationUnresolved
		}
		receipts, err := finality.BlockReceipts(ctx, s.client, block)
		if err != nil {
			return nil, err
		}
		for i, receipt := range receipts {
			for _, log := range receipt.Logs {
				use := decodeV1GenerationCall(profile, block.Transactions()[i], *log, receipt.Status, head)
				if use != nil && use.device == account && wanted[use.nonce] {
					if _, duplicate := matched[use.nonce]; duplicate {
						return nil, errGenerationUnresolved
					}
					matched[use.nonce] = generationOccurrence{use: *use, block: head}
				}
			}
		}
	}
	return matched, nil
}
