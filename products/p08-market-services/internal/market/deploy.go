package market

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

func loopback(endpoint string) bool {
	target, err := url.Parse(endpoint)
	return err == nil && target.Scheme == "http" && (target.Hostname() == "127.0.0.1" || target.Hostname() == "::1") &&
		target.User == nil && target.RawQuery == "" && target.Fragment == ""
}

func isolatedRPC(ctx context.Context, endpoint string) (*ethclient.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	wire, err := rpc.DialOptions(ctx, endpoint, rpc.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	return ethclient.NewClient(wire), nil
}

func deploymentBytecode(path string) (hexutil.Bytes, error) {
	document, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artifact struct {
		Bytecode struct{ Object hexutil.Bytes }
		Metadata struct {
			Compiler struct{ Version string }
			Settings struct {
				EVMVersion        string
				ViaIR             bool
				Remappings        []string
				CompilationTarget map[string]string
				Libraries         map[string]string
				Metadata          struct {
					BytecodeHash      string
					UseLiteralContent bool
				}
				Optimizer struct {
					Enabled bool
					Runs    int
					Details json.RawMessage
				}
			}
		}
	}
	if err := json.Unmarshal(document, &artifact); err != nil || len(artifact.Bytecode.Object) == 0 {
		return nil, errors.New("invalid Forge market artifact")
	}
	settings := artifact.Metadata.Settings
	if artifact.Metadata.Compiler.Version != "0.8.39+commit.ce47f2c8" ||
		settings.EVMVersion != "prague" || settings.ViaIR || !settings.Optimizer.Enabled ||
		settings.Optimizer.Runs != 200 || len(settings.Optimizer.Details) != 0 || len(settings.Libraries) != 0 ||
		settings.Metadata.BytecodeHash != "ipfs" || settings.Metadata.UseLiteralContent ||
		len(settings.CompilationTarget) != 1 || settings.CompilationTarget["src/Market.sol"] != "Market" ||
		len(settings.Remappings) != 1 || settings.Remappings[0] != "forge-std/=../p06-stablenet-contracts/lib/forge-std/src/" {
		return nil, errors.New("market artifact needs the verified Solidity 0.8.39 build and settings")
	}
	return artifact.Bytecode.Object, nil
}

// Deploy creates a faucet-only venue on a locally routed 8283 fixture.
func Deploy(ctx context.Context, endpoint, artifactPath string, key *ecdsa.PrivateKey, roles [4]common.Address) (Profile, error) {
	if !loopback(endpoint) {
		return Profile{}, errors.New("market deployment requires loopback RPC")
	}
	bytecode, err := deploymentBytecode(artifactPath)
	if err != nil {
		return Profile{}, err
	}
	client, err := isolatedRPC(ctx, endpoint)
	if err != nil {
		return Profile{}, err
	}
	defer client.Close()
	id, err := client.ChainID(ctx)
	if err != nil || id.Cmp(big.NewInt(ChainID)) != 0 {
		return Profile{}, errors.New("refusing to deploy outside isolated 8283")
	}
	genesis, err := client.HeaderByNumber(ctx, big.NewInt(0))
	if err != nil {
		return Profile{}, err
	}
	from := crypto.PubkeyToAddress(key.PublicKey)
	parameters, err := venueABI.Constructor.Inputs.Pack(from, roles[0], roles[1], roles[2], roles[3])
	if err != nil {
		return Profile{}, err
	}
	data := append(bytecode, parameters...)
	receipt, err := submit(ctx, client, key, nil, data)
	if err != nil {
		return Profile{}, err
	}
	if receipt.ContractAddress == (common.Address{}) || receipt.BlockNumber == nil || !receipt.BlockNumber.IsUint64() {
		return Profile{}, errors.New("deployment receipt lacks a contract or height")
	}
	profile := Profile{
		Environment: "p08-isolated-8283-v1", Chain: ChainID, Genesis: genesis.Hash(), Height: receipt.BlockNumber.Uint64(),
		BlockHash: receipt.BlockHash, QuoteSigner: roles[0], Venue: Contract{Address: receipt.ContractAddress},
	}
	chain := Chain{Profile: profile, RPC: client}
	for _, contract := range []struct {
		item   *Contract
		method string
	}{
		{&profile.Venue, ""}, {&profile.Base, "base"}, {&profile.Quote, "quoteToken"},
	} {
		if contract.method != "" {
			values, err := chain.Venue(ctx, receipt.BlockHash, contract.method)
			if err != nil {
				return Profile{}, err
			}
			contract.item.Address = values[0].(common.Address)
		}
		code, err := client.CodeAtHash(ctx, contract.item.Address, receipt.BlockHash)
		if err != nil || len(code) == 0 {
			return Profile{}, errors.New("missing deployed runtime code")
		}
		contract.item.CodeHash = crypto.Keccak256Hash(code)
	}
	chain.Profile = profile
	if err := chain.checkDeployment(ctx); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func submit(ctx context.Context, client *ethclient.Client, key *ecdsa.PrivateKey, to *common.Address, data []byte) (*types.Receipt, error) {
	from := crypto.PubkeyToAddress(key.PublicKey)
	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}
	gas, err := client.EstimateGas(ctx, ethereum.CallMsg{From: from, To: to, Data: data})
	if err != nil {
		return nil, err
	}
	price, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := types.SignTx(types.NewTx(&types.LegacyTx{To: to, Nonce: nonce, GasPrice: price, Gas: gas + gas/5, Data: data}),
		types.LatestSignerForChainID(big.NewInt(ChainID)), key)
	if err != nil {
		return nil, err
	}
	if err := client.SendTransaction(ctx, tx); err != nil {
		return nil, err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		receipt, err := client.TransactionReceipt(ctx, tx.Hash())
		if !errors.Is(err, ethereum.NotFound) {
			if err != nil {
				return nil, err
			}
			if receipt.Status != types.ReceiptStatusSuccessful {
				return nil, fmt.Errorf("transaction %s reverted", tx.Hash())
			}
			return receipt, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
