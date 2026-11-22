package settlement

import (
	"context"
	"encoding/hex"
	"math/big"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// RentalEscrow uses the existing P05 deployment review and P06 ledger.
type RentalEscrow struct {
	Client  *ethclient.Client
	Targets map[string]Target
	Reviews map[string]DeploymentReview
}

func (e RentalEscrow) Deployment(ctx context.Context, branch string) ([]byte, bool) {
	target, ok := e.Targets[branch]
	if !ok || e.Client == nil || !ReviewedDeploymentCurrent(ctx, e.Client, target, e.Reviews[branch]) {
		return nil, false
	}
	digest, err := hex.DecodeString(target.Manifest)
	return digest, err == nil && len(digest) == 32
}

func (e RentalEscrow) Check(ctx context.Context, branch string, payer common.Address, generation uint64, amount *big.Int) ([]byte, time.Time, bool) {
	manifest, ok := e.Deployment(ctx, branch)
	if !ok || generation == 0 || amount == nil || amount.Sign() <= 0 {
		return nil, time.Time{}, false
	}
	target := e.Targets[branch]
	head, err := e.Client.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil || head == nil || head.Number == nil {
		return nil, time.Time{}, false
	}
	review := e.Reviews[branch]
	hash := head.Hash()
	if relation := head.Number.Cmp(big.NewInt(review.StartHeight)); relation < 0 ||
		(relation == 0 && hash.Hex() != review.StartHash) {
		return nil, time.Time{}, false
	}
	contract, err := v2.NewSettlementv2Caller(target.Contract, e.Client)
	if err != nil {
		return nil, time.Time{}, false
	}
	opts := &bind.CallOpts{Context: ctx, BlockHash: hash}
	code, err := e.Client.CodeAtHash(ctx, target.Contract, hash)
	if err != nil || crypto.Keccak256Hash(code) != target.CodeHash {
		return nil, time.Time{}, false
	}
	asset, err := contract.Asset(opts)
	if err != nil || asset != target.Asset {
		return nil, time.Time{}, false
	}
	registered, err := contract.Payers(opts, payer)
	if err != nil || !registered.Active || registered.Role != 2 || registered.Generation != generation {
		return nil, time.Time{}, false
	}
	balance, err := contract.EscrowBalance(opts, payer, generation)
	if err != nil || balance == nil || balance.Cmp(amount) < 0 {
		return nil, time.Time{}, false
	}
	canonical, err := e.Client.HeaderByNumber(ctx, head.Number)
	if err != nil || canonical == nil || canonical.Hash() != hash {
		return nil, time.Time{}, false
	}
	return manifest, time.Now().Add(RentalEscrowProofLifetime), true
}

// RentalEscrowProofLifetime stays below the 15-second maximum that wallets accept
// (verifyPresentedRentalEscrow), leaving two seconds for a phone clock behind this server.
const RentalEscrowProofLifetime = 13 * time.Second
