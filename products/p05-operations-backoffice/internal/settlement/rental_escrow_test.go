package settlement

import (
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type rentalEscrowRPC struct {
	code       hexutil.Bytes
	start      *types.Header
	finalized  *types.Header
	asset      common.Address
	payer      common.Address
	target     common.Address
	generation uint64
}

func (*rentalEscrowRPC) ChainId() hexutil.Uint64 { return 8283 }

func (f *rentalEscrowRPC) GetCode(account common.Address, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	if account != f.target || block.BlockHash != nil && *block.BlockHash != f.finalized.Hash() {
		return nil, errors.New("unexpected code target or block")
	}
	return f.code, nil
}

func (f *rentalEscrowRPC) GetBlockByNumber(number rpc.BlockNumber, _ bool) (*types.Header, error) {
	switch number {
	case rpc.FinalizedBlockNumber:
		return f.finalized, nil
	case rpc.BlockNumber(f.start.Number.Int64()):
		return f.start, nil
	case rpc.BlockNumber(f.finalized.Number.Int64()):
		return f.finalized, nil
	default:
		return nil, errors.New("unexpected state block")
	}
}

func (f *rentalEscrowRPC) Call(args map[string]any, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	if block.BlockHash == nil || *block.BlockHash != f.finalized.Hash() || args["to"] != strings.ToLower(f.target.Hex()) {
		return nil, errors.New("escrow lookup did not use the original finalized deployment")
	}
	raw, ok := args["input"].(string)
	if !ok {
		return nil, errors.New("missing ABI input")
	}
	input, err := hexutil.Decode(raw)
	if err != nil || len(input) < 4 {
		return nil, errors.New("invalid ABI input")
	}
	abi, err := v2.Settlementv2MetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	method, err := abi.MethodById(input[:4])
	if err != nil {
		return nil, err
	}
	values, err := method.Inputs.Unpack(input[4:])
	if err != nil {
		return nil, err
	}
	switch method.Name {
	case "asset":
		return method.Outputs.Pack(f.asset)
	case "payers":
		if len(values) == 1 && values[0] == f.payer {
			return method.Outputs.Pack(f.generation, uint8(2), true)
		}
	case "escrowBalance":
		if len(values) == 2 && values[0] == f.payer && values[1] == f.generation {
			return method.Outputs.Pack(big.NewInt(6500000))
		}
	}
	return nil, errors.New("unexpected escrow payer or generation")
}

func TestRentalEscrowUsesReviewedFinalizedGeneration(t *testing.T) {
	header := func(number uint64, variant string) *types.Header {
		return &types.Header{Number: new(big.Int).SetUint64(number), Difficulty: big.NewInt(0), GasLimit: 30000000, Extra: []byte(variant)}
	}
	for _, test := range []struct {
		name      string
		finalized *types.Header
		amount    int64
		available bool
	}{
		{"before_reviewed_origin", header(3, ""), 6500000, false},
		{"at_reviewed_origin", header(4, ""), 6500000, true},
		{"different_hash_at_reviewed_height", header(4, "different"), 6500000, false},
		{"later_finalized", header(9, ""), 6500000, true},
		{"one_unit_short", header(4, ""), 6500001, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			code := hexutil.Bytes{0x00}
			fixture := &rentalEscrowRPC{
				code: code, start: header(4, ""), finalized: test.finalized, generation: 7,
				asset: common.HexToAddress("0xa0a0"), target: common.HexToAddress("0xb0b0"), payer: common.HexToAddress("0xc0c0"),
			}
			server := rpc.NewServer()
			t.Cleanup(server.Stop)
			if err := server.RegisterName("eth", fixture); err != nil {
				t.Fatal(err)
			}
			client := ethclient.NewClient(rpc.DialInProc(server))
			t.Cleanup(client.Close)
			const branch = "2bea322f-1b48-4c62-9922-f687a5e19b3d"
			manifest := strings.Repeat("aa", 32)
			resolver := RentalEscrow{Client: client, Targets: map[string]Target{
				branch: {Contract: fixture.target, Asset: fixture.asset, CodeHash: crypto.Keccak256Hash(code), Manifest: manifest},
			}, Reviews: map[string]DeploymentReview{
				branch: {Status: "verified", StartHeight: 4, StartHash: fixture.start.Hash().Hex()},
			}}
			deployment, expiry, available := resolver.Check(t.Context(), branch, fixture.payer, fixture.generation, big.NewInt(test.amount))
			if available != test.available {
				t.Fatalf("escrow availability = %t, want %t", available, test.available)
			}
			if available && (hex.EncodeToString(deployment) != manifest || time.Until(expiry) <= 0 || time.Until(expiry) > 15*time.Second) {
				t.Fatal("invalid deployment proof or lifetime")
			}
		})
	}
}
