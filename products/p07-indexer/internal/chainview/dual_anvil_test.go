package chainview

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/registry"
	v1 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func postBothVersions(t *testing.T, ctx context.Context, client *ethclient.Client, asset, modern common.Address) (Profile, int64) {
	t.Helper()
	synthetic := func(value byte) (*bind.TransactOpts, func(common.Hash) []byte) {
		key, err := crypto.ToECDSA(append(make([]byte, 31), value))
		if err != nil {
			t.Fatal(err)
		}
		auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
		if err != nil {
			t.Fatal(err)
		}
		auth.Context, auth.GasLimit = ctx, 10_000_000
		return auth, func(hash common.Hash) []byte {
			sig, err := crypto.Sign(hash[:], key)
			if err != nil {
				t.Fatal(err)
			}
			sig[64] += 27
			return sig
		}
	}
	operator, _ := synthetic(100)
	payer, signPayer := synthetic(102)
	merchantSigner, signMerchant := synthetic(101)
	beneficiary, _ := synthetic(104)
	mined := func(tx *types.Transaction, err error) *types.Receipt {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := bind.WaitMined(ctx, client, tx)
		if err != nil || receipt.Status != 1 {
			t.Fatalf("synthetic payment or deployment reverted: %v", err)
		}
		return receipt
	}
	deploy := func(name string, args ...any) (common.Address, *types.Receipt) {
		t.Helper()
		path := filepath.Join(cmp.Or(os.Getenv("FOUNDRY_OUT"), "../../../../products/p06-stablenet-contracts/out"), name+".sol", name+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var artifact struct {
			ABI      json.RawMessage
			Bytecode struct{ Object hexutil.Bytes }
		}
		if err := json.Unmarshal(raw, &artifact); err != nil {
			t.Fatal(err)
		}
		contract, err := abi.JSON(bytes.NewReader(artifact.ABI))
		if err != nil {
			t.Fatal(err)
		}
		address, tx, _, err := bind.DeployContract(operator, contract, artifact.Bytecode.Object, client, args...)
		return address, mined(tx, err)
	}

	registryAddress, _ := deploy("MerchantRegistry", operator.From, big.NewInt(86400))
	address, deployment := deploy("PaymentSettlement", asset, registryAddress, operator.From,
		big.NewInt(50), big.NewInt(200), big.NewInt(3600), big.NewInt(120))
	reg, err := registry.NewRegistry(registryAddress, client)
	if err != nil {
		t.Fatal(err)
	}
	old, err := v1.NewSettlement(address, client)
	if err != nil {
		t.Fatal(err)
	}
	token, err := testusdc.NewTestusdc(asset, client)
	if err != nil {
		t.Fatal(err)
	}
	newProtocol, err := v2.NewSettlementv2(modern, client)
	if err != nil {
		t.Fatal(err)
	}
	mined(reg.RegisterMerchant(operator, merchantSigner.From, beneficiary.From))
	mined(token.Mint(operator, operator.From, big.NewInt(35)))
	mined(token.Approve(operator, address, big.NewInt(35)))
	mined(old.DepositFor(operator, payer.From, big.NewInt(35), payer.From))

	head, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := v1.PaymentTypesPaymentAuthorization{
		ChainId: big.NewInt(8283), ContractAddress: address, Merchant: merchantSigner.From, Payout: beneficiary.From,
		Token: asset, Amount: big.NewInt(35), OrderId: common.Hash{31: 71}, Nonce: big.NewInt(78), Expiry: head.Time + 60,
	}
	legacy, err := v1.SettlementMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := abi.NewType("bytes32", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	typeHash := crypto.Keccak256Hash([]byte("PaymentAuthorization(uint256 chainId,address contract,address merchant,address payout,address token,uint256 amount,bytes32 orderId,uint256 nonce,uint64 expiry)"))
	canonical, err := (abi.Arguments{{Type: fixed}, legacy.Methods["settle"].Inputs[0]}).Pack(typeHash, a)
	if err != nil {
		t.Fatal(err)
	}
	separator, err := old.DomainSeparator(nil)
	if err != nil {
		t.Fatal(err)
	}
	oldDigest := crypto.Keccak256Hash([]byte{0x19, 0x01}, separator[:], crypto.Keccak256(canonical))
	q := v2.PaymentTypesV2MerchantQuote{
		QuoteId: common.Hash{31: 61}, TenantCommitment: common.Hash{31: 62}, BranchCommitment: common.Hash{31: 63},
		OrderId: common.Hash{31: 64}, MerchantId: common.Hash{31: 50}, ClaimCommitment: common.Hash{31: 65}, IntentCommitment: common.Hash{31: 66},
		Payer: payer.From, Asset: asset, Beneficiary: beneficiary.From, Amount: big.NewInt(30), Nonce: big.NewInt(92),
		MerchantKeyVersion: 1, BeneficiaryVersion: 1, PayerGeneration: 1, IssuedAt: head.Time, ExpiresAt: head.Time + 120, Mode: 1, PayerRole: 2,
	}
	digest, err := newProtocol.QuoteDigest(nil, q)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := newProtocol.ApprovalDigest(nil, digest)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, args ...any) {
		t.Helper()
		if err := client.Client().CallContext(ctx, nil, method, args...); err != nil {
			t.Fatal(err)
		}
	}
	nonce, err := client.PendingNonceAt(ctx, operator.From)
	if err != nil {
		t.Fatal(err)
	}
	call("evm_setAutomine", false)
	call("evm_setNextBlockTimestamp", head.Time+1)
	operator.Nonce = new(big.Int).SetUint64(nonce)
	txV1, err := old.Settle(operator, a, signPayer(oldDigest))
	if err != nil {
		t.Fatal(err)
	}
	operator.Nonce = new(big.Int).SetUint64(nonce + 1)
	txV2, err := newProtocol.Settle(operator, q, signMerchant(digest), signPayer(approval))
	if err != nil {
		t.Fatal(err)
	}
	call("anvil_mine", 1)
	call("evm_setAutomine", true)
	v1Receipt, v2Receipt := mined(txV1, nil), mined(txV2, nil)
	if v1Receipt.BlockHash != v2Receipt.BlockHash {
		t.Fatal("v1/v2 payments did not share the test block")
	}
	code, err := client.CodeAt(ctx, address, deployment.BlockNumber)
	if err != nil || len(code) == 0 {
		t.Fatalf("legacy code unavailable: %v", err)
	}
	return Profile{StreamID: "b0cabaf8-23e5-43d8-b73e-362fa0fb14a2", GenerationID: "b0cabaf8-23e5-43d8-b73e-362fa0fb14a3",
		Version: 1, ChainID: 8283, Contract: address, Asset: asset, ABIHash: v1ABIHash, CodeHash: crypto.Keccak256Hash(code),
		StartHeight: deployment.BlockNumber.Int64(), StartHash: deployment.BlockHash,
		Manifest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abce00"}, v1Receipt.BlockNumber.Int64()
}
