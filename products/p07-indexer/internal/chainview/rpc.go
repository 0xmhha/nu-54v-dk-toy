package chainview

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	v1 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/finality"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type RPCSource struct {
	client  *ethclient.Client
	decoder *Decoder
}

func Dial(ctx context.Context, endpoint string, profile Profile) (*RPCSource, error) {
	if err := profile.validate(); err != nil {
		return nil, err
	}
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	source, err := pinnedSource(ctx, client, profile)
	if err != nil {
		client.Close()
	}
	return source, err
}

func pinnedSource(ctx context.Context, client *ethclient.Client, profile Profile) (*RPCSource, error) {
	network, err := client.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if !network.IsUint64() || network.Uint64() != profile.ChainID {
		return nil, ErrDeploymentIdentity
	}
	decoder, err := NewDecoder(profile)
	if err != nil {
		return nil, err
	}
	source := &RPCSource{client: client, decoder: decoder}
	start, err := source.Header(ctx, profile.StartHeight)
	if err != nil {
		return nil, err
	}
	if start.Hash != profile.StartHash {
		return nil, ErrDeploymentIdentity
	}
	var code hexutil.Bytes
	err = client.Client().CallContext(ctx, &code, "eth_getCode", profile.Contract,
		rpc.BlockNumberOrHashWithHash(start.Hash, true))
	if err != nil {
		return nil, fmt.Errorf("deployment code at pinned block: %w", err)
	}
	if len(code) == 0 || crypto.Keccak256Hash(code) != profile.CodeHash {
		return nil, ErrDeploymentIdentity
	}
	return source, nil
}

// VerifyAssetDecimals resolves token units at the configured canonical
// deployment hash. An existing manifest without units can still show raw
// values if its token does not expose decimals.
func (s *RPCSource) VerifyAssetDecimals(ctx context.Context) (*uint8, error) {
	profile := s.decoder.profile
	var output hexutil.Bytes
	query := map[string]any{"to": profile.Asset, "data": "0x313ce567"}
	err := s.client.Client().CallContext(ctx, &output, "eth_call", query,
		rpc.BlockNumberOrHashWithHash(profile.StartHash, true))
	if err != nil || len(output) != 32 || !new(big.Int).SetBytes(output).IsUint64() ||
		new(big.Int).SetBytes(output).Uint64() > math.MaxUint8 {
		if profile.AssetDecimals == nil {
			return nil, nil
		}
		return nil, errors.New("approved token units could not be verified")
	}
	actual := output[31]
	if profile.AssetDecimals != nil && *profile.AssetDecimals != actual {
		return nil, ErrDeploymentIdentity
	}
	return &actual, nil
}

func (s *RPCSource) Close() { s.client.Close() }

func (s *RPCSource) Heads(ctx context.Context) (Header, Header, error) {
	latest, err := s.Header(ctx, int64(rpc.LatestBlockNumber))
	if err != nil {
		return Header{}, Header{}, err
	}
	finalized, err := s.Header(ctx, int64(rpc.FinalizedBlockNumber))
	return latest, finalized, err
}

func (s *RPCSource) Header(ctx context.Context, height int64) (Header, error) {
	value, err := s.client.HeaderByNumber(ctx, big.NewInt(height))
	if err != nil {
		return Header{}, err
	}
	head, err := checkedHeader(value)
	if err == nil && height >= 0 && head.Height != height {
		return Header{}, errors.New("RPC returned different block height")
	}
	return head, err
}

func checkedHeader(value *types.Header) (Header, error) {
	if value == nil || value.Number == nil || !value.Number.IsInt64() || value.Number.Sign() < 0 ||
		value.Time > math.MaxInt64 {
		return Header{}, errors.New("invalid header height or time")
	}
	return Header{Height: value.Number.Int64(), Hash: value.Hash(), Parent: value.ParentHash,
		Time: time.Unix(int64(value.Time), 0).UTC()}, nil
}

func (s *RPCSource) Block(ctx context.Context, expected Header) (Block, error) {
	block, err := s.client.BlockByHash(ctx, expected.Hash)
	if err != nil {
		return Block{}, err
	}
	header, err := checkedHeader(block.Header())
	if err != nil || header != expected {
		return Block{}, errors.New("RPC block contradicted requested header")
	}
	receipts, err := finality.BlockReceipts(ctx, s.client, block)
	if err != nil {
		return Block{}, err
	}
	result := Block{Header: header}
	transactions := block.Transactions()
	for i, receipt := range receipts {
		for _, log := range receipt.Logs {
			if !s.decoder.matches(*log) {
				continue
			}
			event, err := s.decoder.Decode(*log, receipt.Status)
			if err != nil {
				return Block{}, err
			}
			if i >= len(transactions) || transactions[i].Hash() != log.TxHash {
				return Block{}, errors.New("settlement transaction contradicts canonical receipt order")
			}
			if err := s.decoder.bindQuoteWindow(&event, transactions[i], header.Time); err != nil {
				return Block{}, err
			}
			result.Events = append(result.Events, event)
		}
	}
	return result, nil
}

func (s *RPCSource) Paid(ctx context.Context, merchant, order string, anchor Header) (bool, error) {
	address := s.decoder.profile.Contract
	orderID := common.HexToHash(order)
	if s.decoder.profile.Version == 1 {
		abi, err := v1.SettlementMetaData.GetAbi()
		if err != nil {
			return false, err
		}
		call, err := abi.Pack("isPaid", common.HexToAddress(merchant), orderID)
		if err != nil {
			return false, err
		}
		raw, err := s.callAt(ctx, address, call, anchor.Hash)
		if err != nil {
			return false, err
		}
		values, err := abi.Unpack("isPaid", raw)
		if err != nil || len(values) != 1 {
			return false, errors.New("could not verify legacy unconsumed state")
		}
		return values[0].(bool), nil
	}

	abi, err := v2.Settlementv2MetaData.GetAbi()
	if err != nil {
		return false, err
	}
	call, err := abi.Pack("payments", common.HexToHash(merchant), orderID)
	if err != nil {
		return false, err
	}
	raw, err := s.callAt(ctx, address, call, anchor.Hash)
	if err != nil {
		return false, err
	}
	values, err := abi.Unpack("payments", raw)
	if err != nil || len(values) != 5 {
		return false, errors.New("could not verify v2 unconsumed state")
	}
	return values[1].(*big.Int).Sign() != 0, nil
}

func (s *RPCSource) callAt(ctx context.Context, address common.Address, call []byte, at common.Hash) ([]byte, error) {
	var result hexutil.Bytes
	err := s.client.Client().CallContext(ctx, &result, "eth_call",
		map[string]any{"to": address, "data": hexutil.Bytes(call)}, rpc.BlockNumberOrHashWithHash(at, true))
	return result, err
}
