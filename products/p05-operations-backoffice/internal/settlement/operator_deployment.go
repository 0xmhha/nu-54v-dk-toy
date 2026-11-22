package settlement

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"math/big"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type DeploymentReview struct {
	Status             string  `json:"status"`
	Chain              uint64  `json:"chain,omitempty"`
	Contract           string  `json:"contract,omitempty"`
	Asset              string  `json:"asset,omitempty"`
	Manifest           string  `json:"manifest,omitempty"`
	ABI                string  `json:"abi,omitempty"`
	RuntimeCodeHash    string  `json:"runtimeCodeHash,omitempty"`
	StartHeight        int64   `json:"startHeight,omitempty"`
	StartHash          string  `json:"startHash,omitempty"`
	GasPayer           string  `json:"gasPayer,omitempty"`
	BeneficiaryVersion *uint64 `json:"beneficiaryVersion,omitempty"`
}

func deploymentIdentity(target Target, gas common.Address) DeploymentReview {
	view := DeploymentReview{Status: "unverified", Chain: 8283, Contract: lower(target.Contract), Asset: lower(target.Asset),
		Manifest: target.Manifest, ABI: ABIHash, RuntimeCodeHash: target.CodeHash.Hex()}
	if gas != (common.Address{}) {
		view.GasPayer = lower(gas)
	}
	return view
}

func operatorDeploymentRuntimeMatches(ctx context.Context, client *ethclient.Client, target Target) bool {
	chain, err := client.ChainID(ctx)
	if err != nil || !chain.IsUint64() || chain.Uint64() != 8283 {
		return false
	}
	code, err := client.CodeAt(ctx, target.Contract, nil)
	return err == nil && len(code) != 0 && crypto.Keccak256Hash(code) == target.CodeHash
}

func (h FinanceHTTP) publicDeploymentCurrent(ctx context.Context, target Target, approved DeploymentReview) bool {
	chain, ok := h.Funds.(LiveChain)
	return ok && ReviewedDeploymentCurrent(ctx, chain.Client, target, approved)
}

func ReviewedDeploymentCurrent(ctx context.Context, client *ethclient.Client, target Target, approved DeploymentReview) bool {
	if approved.Status != "verified" || approved.StartHeight <= 0 ||
		!operatorDeploymentRuntimeMatches(ctx, client, target) {
		return false
	}
	header, err := client.HeaderByNumber(ctx, big.NewInt(approved.StartHeight))
	return err == nil && header != nil && header.Hash().Hex() == approved.StartHash
}

func VerifyOperatorDeployment(ctx context.Context, client *ethclient.Client, target Target, gas common.Address,
	profiles []deployment.Profile, key ed25519.PublicKey, signatures map[string]string, requireSignature bool) DeploymentReview {
	invalid := DeploymentReview{Status: "invalid"}
	if !operatorDeploymentRuntimeMatches(ctx, client, target) {
		return invalid
	}
	view := deploymentIdentity(target, gas)
	if !requireSignature {
		return view
	}
	var signed *deployment.Profile
	for _, profile := range profiles {
		if profile.StreamID != target.Stream {
			continue
		}
		// A P05 target must name one signed deployment. Generation swaps remain
		// P07 operations; supply one corresponding approved profile for P05.
		if signed != nil {
			return invalid
		}
		signed = &profile
	}
	if signed == nil || signed.Version != 2 || signed.ChainID != view.Chain || signed.Contract != target.Contract ||
		signed.Asset != target.Asset || signed.Manifest != target.Manifest || signed.CodeHash != target.CodeHash ||
		signed.ABIHash != ABIHash || !financeRef.MatchString(signed.GenerationID) || signed.StartHeight <= 0 ||
		signed.StartHash == (common.Hash{}) || (signed.AssetDecimals != nil && *signed.AssetDecimals != 6) {
		return invalid
	}
	sig, err := hex.DecodeString(signatures[signed.GenerationID])
	if err != nil || !deployment.Verify(*signed, key, sig) {
		return invalid
	}
	height := big.NewInt(signed.StartHeight)
	header, err := client.HeaderByNumber(ctx, height)
	if err != nil || header.Hash() != signed.StartHash {
		return invalid
	}
	pinnedCode, err := client.CodeAtHash(ctx, signed.Contract, signed.StartHash)
	if err != nil || len(pinnedCode) == 0 || crypto.Keccak256Hash(pinnedCode) != signed.CodeHash {
		return invalid
	}
	caller, err := v2.NewSettlementv2Caller(target.Contract, client)
	if err != nil {
		return invalid
	}
	at := &bind.CallOpts{Context: ctx, BlockHash: signed.StartHash}
	chainField := common.LeftPadBytes(new(big.Int).SetUint64(signed.ChainID).Bytes(), 32)
	contractField := common.LeftPadBytes(signed.Contract.Bytes(), 32)
	domain, err := caller.DomainSeparator(at)
	if err != nil || common.Hash(domain) != domainHash(append(chainField, contractField...)) {
		return invalid
	}
	asset, err := caller.Asset(at)
	if err != nil || asset != signed.Asset {
		return invalid
	}
	token, err := testusdc.NewTestusdcCaller(asset, client)
	if err != nil {
		return invalid
	}
	units, err := token.Decimals(at)
	if err != nil || units != 6 {
		return invalid
	}
	view.Status, view.StartHeight, view.StartHash = "verified", signed.StartHeight, signed.StartHash.Hex()
	return view
}

func (h FinanceHTTP) deploymentReview(ctx context.Context, branch string, target Target) DeploymentReview {
	view, configured := h.Deployments[branch]
	if !configured {
		return deploymentIdentity(target, h.GasAccount)
	}
	chain, ok := h.Funds.(LiveChain)
	if !ok || view.Status != "verified" || h.Merchants[branch] == "" {
		return view
	}
	contract, err := v2.NewSettlementv2Caller(target.Contract, chain.Client)
	if err != nil {
		return view
	}
	block := &bind.CallOpts{Context: ctx, BlockNumber: big.NewInt(int64(rpc.FinalizedBlockNumber))}
	merchant := common.HexToHash(h.Merchants[branch])
	entry, err := contract.Merchants(block, merchant)
	if err == nil && entry.Active && entry.BeneficiaryVersion > 0 {
		view.BeneficiaryVersion = &entry.BeneficiaryVersion
	}
	return view
}
