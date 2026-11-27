// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package settlementv2

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
	_ = time.Tick
	_ = context.Background
)

// PaymentTypesV2MerchantQuote is an auto generated low-level Go binding around an user-defined struct.
type PaymentTypesV2MerchantQuote struct {
	QuoteId            [32]byte
	TenantCommitment   [32]byte
	BranchCommitment   [32]byte
	OrderId            [32]byte
	MerchantId         [32]byte
	ClaimCommitment    [32]byte
	IntentCommitment   [32]byte
	Payer              common.Address
	Asset              common.Address
	Beneficiary        common.Address
	Amount             *big.Int
	Nonce              *big.Int
	MerchantKeyVersion uint64
	BeneficiaryVersion uint64
	PayerGeneration    uint64
	IssuedAt           uint64
	ExpiresAt          uint64
	Mode               uint8
	PayerRole          uint8
}

// PaymentTypesV2RefundAuthorization is an auto generated low-level Go binding around an user-defined struct.
type PaymentTypesV2RefundAuthorization struct {
	MerchantId [32]byte
	OrderId    [32]byte
	RefundId   [32]byte
	Amount     *big.Int
	KeyVersion uint64
	ExpiresAt  uint64
}

// Settlementv2MetaData contains all meta data concerning the Settlementv2 contract.
var Settlementv2MetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"asset_\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"governor_\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"directEnabled_\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"CHAIN_ID\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"DIRECT\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"ESCROW\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"MAX_PENDING_ROTATION\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"OWNER\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"RENTER\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"authorizedQuoteDigest\",\"type\":\"bytes32\"}],\"name\":\"approvalDigest\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"asset\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"name\":\"beneficiaries\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"internalType\":\"uint64\",\"name\":\"validFrom\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"validUntil\",\"type\":\"uint64\"},{\"internalType\":\"bool\",\"name\":\"revoked\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"}],\"name\":\"deactivatePayer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"uint64\",\"name\":\"generation\",\"type\":\"uint64\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"depositEscrow\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"directEnabled\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"domainSeparator\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"name\":\"escrowBalance\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"name\":\"escrowWithdrawalAddress\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"fundRefundReserve\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"governor\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"grandfathered\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"name\":\"merchantKeys\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"internalType\":\"uint64\",\"name\":\"validFrom\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"validUntil\",\"type\":\"uint64\"},{\"internalType\":\"bool\",\"name\":\"revoked\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"merchantNonceUsed\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"merchants\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"keyVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"beneficiaryVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"refundKeyVersion\",\"type\":\"uint64\"},{\"internalType\":\"address\",\"name\":\"refundSigner\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"active\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"payerNonceUsed\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"payers\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"generation\",\"type\":\"uint64\"},{\"internalType\":\"uint8\",\"name\":\"role\",\"type\":\"uint8\"},{\"internalType\":\"bool\",\"name\":\"active\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"payments\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"refunded\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"payerGeneration\",\"type\":\"uint64\"},{\"internalType\":\"uint8\",\"name\":\"mode\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"quoteId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"tenantCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"branchCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"claimCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"intentCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"asset\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"merchantKeyVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"beneficiaryVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"payerGeneration\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"issuedAt\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"expiresAt\",\"type\":\"uint64\"},{\"internalType\":\"uint8\",\"name\":\"mode\",\"type\":\"uint8\"},{\"internalType\":\"uint8\",\"name\":\"payerRole\",\"type\":\"uint8\"}],\"internalType\":\"structPaymentTypesV2.MerchantQuote\",\"name\":\"quote\",\"type\":\"tuple\"}],\"name\":\"quoteDigest\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"refundId\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"keyVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"expiresAt\",\"type\":\"uint64\"}],\"internalType\":\"structPaymentTypesV2.RefundAuthorization\",\"name\":\"authorization\",\"type\":\"tuple\"},{\"internalType\":\"bytes\",\"name\":\"signature\",\"type\":\"bytes\"}],\"name\":\"refund\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"refundId\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"keyVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"expiresAt\",\"type\":\"uint64\"}],\"internalType\":\"structPaymentTypesV2.RefundAuthorization\",\"name\":\"authorization\",\"type\":\"tuple\"}],\"name\":\"refundDigest\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"refundIdUsed\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"refundReserve\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"signer\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"refundSigner\",\"type\":\"address\"}],\"name\":\"registerMerchant\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"uint64\",\"name\":\"generation\",\"type\":\"uint64\"},{\"internalType\":\"uint8\",\"name\":\"role\",\"type\":\"uint8\"},{\"internalType\":\"address\",\"name\":\"withdrawalAddress\",\"type\":\"address\"}],\"name\":\"registerPayer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"uint64\",\"name\":\"version\",\"type\":\"uint64\"}],\"name\":\"revokeMerchantKey\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"nextSigner\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"nextBeneficiary\",\"type\":\"address\"},{\"internalType\":\"bytes32[]\",\"name\":\"openDigests\",\"type\":\"bytes32[]\"}],\"name\":\"rotateMerchant\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"signer\",\"type\":\"address\"}],\"name\":\"rotateRefundSigner\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bool\",\"name\":\"active\",\"type\":\"bool\"}],\"name\":\"setMerchantActive\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"quoteId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"tenantCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"branchCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"claimCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"intentCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"asset\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"merchantKeyVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"beneficiaryVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"payerGeneration\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"issuedAt\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"expiresAt\",\"type\":\"uint64\"},{\"internalType\":\"uint8\",\"name\":\"mode\",\"type\":\"uint8\"},{\"internalType\":\"uint8\",\"name\":\"payerRole\",\"type\":\"uint8\"}],\"internalType\":\"structPaymentTypesV2.MerchantQuote\",\"name\":\"quote\",\"type\":\"tuple\"},{\"internalType\":\"bytes\",\"name\":\"merchantSignature\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"payerSignature\",\"type\":\"bytes\"}],\"name\":\"settle\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"totalEscrow\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"totalRefundReserve\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"uint64\",\"name\":\"generation\",\"type\":\"uint64\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"withdrawEscrow\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"keyVersion\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"beneficiaryVersion\",\"type\":\"uint64\"}],\"name\":\"MerchantConfigured\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"uint64\",\"name\":\"keyVersion\",\"type\":\"uint64\"}],\"name\":\"MerchantKeyRevoked\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"asset\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"quoteDigest\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint8\",\"name\":\"mode\",\"type\":\"uint8\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"payerGeneration\",\"type\":\"uint64\"}],\"name\":\"Paid\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"refundId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"asset\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint8\",\"name\":\"mode\",\"type\":\"uint8\"}],\"name\":\"Refunded\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"InsufficientFunds\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidMerchant\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidParameters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidPayer\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidQuote\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidSignature\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidTime\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NonceUsed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"OrderAlreadyPaid\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"Reentered\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"RefundAbovePayment\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"RefundAlreadyUsed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"TokenBalanceMismatch\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"TokenTransferFailed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"Unauthorized\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WrongDomain\",\"type\":\"error\"}]",
}

// Settlementv2ABI is the input ABI used to generate the binding from.
// Deprecated: Use Settlementv2MetaData.ABI instead.
var Settlementv2ABI = Settlementv2MetaData.ABI

// Settlementv2 is an auto generated Go binding around an Ethereum contract.
type Settlementv2 struct {
	Settlementv2Caller     // Read-only binding to the contract
	Settlementv2Transactor // Write-only binding to the contract
	Settlementv2Filterer   // Log filterer for contract events
}

// Settlementv2Caller is an auto generated read-only Go binding around an Ethereum contract.
type Settlementv2Caller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// Settlementv2Transactor is an auto generated write-only Go binding around an Ethereum contract.
type Settlementv2Transactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// Settlementv2Filterer is an auto generated log filtering Go binding around an Ethereum contract events.
type Settlementv2Filterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// Settlementv2Session is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type Settlementv2Session struct {
	Contract     *Settlementv2     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// Settlementv2CallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type Settlementv2CallerSession struct {
	Contract *Settlementv2Caller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// Settlementv2TransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type Settlementv2TransactorSession struct {
	Contract     *Settlementv2Transactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// Settlementv2Raw is an auto generated low-level Go binding around an Ethereum contract.
type Settlementv2Raw struct {
	Contract *Settlementv2 // Generic contract binding to access the raw methods on
}

// Settlementv2CallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type Settlementv2CallerRaw struct {
	Contract *Settlementv2Caller // Generic read-only contract binding to access the raw methods on
}

// Settlementv2TransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type Settlementv2TransactorRaw struct {
	Contract *Settlementv2Transactor // Generic write-only contract binding to access the raw methods on
}

// NewSettlementv2 creates a new instance of Settlementv2, bound to a specific deployed contract.
func NewSettlementv2(address common.Address, backend bind.ContractBackend) (*Settlementv2, error) {
	contract, err := bindSettlementv2(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Settlementv2{Settlementv2Caller: Settlementv2Caller{contract: contract}, Settlementv2Transactor: Settlementv2Transactor{contract: contract}, Settlementv2Filterer: Settlementv2Filterer{contract: contract}}, nil
}

// NewSettlementv2Caller creates a new read-only instance of Settlementv2, bound to a specific deployed contract.
func NewSettlementv2Caller(address common.Address, caller bind.ContractCaller) (*Settlementv2Caller, error) {
	contract, err := bindSettlementv2(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &Settlementv2Caller{contract: contract}, nil
}

// NewSettlementv2Transactor creates a new write-only instance of Settlementv2, bound to a specific deployed contract.
func NewSettlementv2Transactor(address common.Address, transactor bind.ContractTransactor) (*Settlementv2Transactor, error) {
	contract, err := bindSettlementv2(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &Settlementv2Transactor{contract: contract}, nil
}

// NewSettlementv2Filterer creates a new log filterer instance of Settlementv2, bound to a specific deployed contract.
func NewSettlementv2Filterer(address common.Address, filterer bind.ContractFilterer) (*Settlementv2Filterer, error) {
	contract, err := bindSettlementv2(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &Settlementv2Filterer{contract: contract}, nil
}

// bindSettlementv2 binds a generic wrapper to an already deployed contract.
func bindSettlementv2(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := Settlementv2MetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Settlementv2 *Settlementv2Raw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Settlementv2.Contract.Settlementv2Caller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Settlementv2 *Settlementv2Raw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Settlementv2.Contract.Settlementv2Transactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Settlementv2 *Settlementv2Raw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Settlementv2.Contract.Settlementv2Transactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Settlementv2 *Settlementv2CallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Settlementv2.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Settlementv2 *Settlementv2TransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Settlementv2.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Settlementv2 *Settlementv2TransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Settlementv2.Contract.contract.Transact(opts, method, params...)
}

// CHAINID is a free data retrieval call binding the contract method 0x85e1f4d0.
//
// Solidity: function CHAIN_ID() view returns(uint256)
func (_Settlementv2 *Settlementv2Caller) CHAINID(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "CHAIN_ID")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// CHAINID is a free data retrieval call binding the contract method 0x85e1f4d0.
//
// Solidity: function CHAIN_ID() view returns(uint256)
func (_Settlementv2 *Settlementv2Session) CHAINID() (*big.Int, error) {
	return _Settlementv2.Contract.CHAINID(&_Settlementv2.CallOpts)
}

// CHAINID is a free data retrieval call binding the contract method 0x85e1f4d0.
//
// Solidity: function CHAIN_ID() view returns(uint256)
func (_Settlementv2 *Settlementv2CallerSession) CHAINID() (*big.Int, error) {
	return _Settlementv2.Contract.CHAINID(&_Settlementv2.CallOpts)
}

// DIRECT is a free data retrieval call binding the contract method 0xbd89e0a9.
//
// Solidity: function DIRECT() view returns(uint8)
func (_Settlementv2 *Settlementv2Caller) DIRECT(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "DIRECT")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// DIRECT is a free data retrieval call binding the contract method 0xbd89e0a9.
//
// Solidity: function DIRECT() view returns(uint8)
func (_Settlementv2 *Settlementv2Session) DIRECT() (uint8, error) {
	return _Settlementv2.Contract.DIRECT(&_Settlementv2.CallOpts)
}

// DIRECT is a free data retrieval call binding the contract method 0xbd89e0a9.
//
// Solidity: function DIRECT() view returns(uint8)
func (_Settlementv2 *Settlementv2CallerSession) DIRECT() (uint8, error) {
	return _Settlementv2.Contract.DIRECT(&_Settlementv2.CallOpts)
}

// ESCROW is a free data retrieval call binding the contract method 0xe681c4aa.
//
// Solidity: function ESCROW() view returns(uint8)
func (_Settlementv2 *Settlementv2Caller) ESCROW(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "ESCROW")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// ESCROW is a free data retrieval call binding the contract method 0xe681c4aa.
//
// Solidity: function ESCROW() view returns(uint8)
func (_Settlementv2 *Settlementv2Session) ESCROW() (uint8, error) {
	return _Settlementv2.Contract.ESCROW(&_Settlementv2.CallOpts)
}

// ESCROW is a free data retrieval call binding the contract method 0xe681c4aa.
//
// Solidity: function ESCROW() view returns(uint8)
func (_Settlementv2 *Settlementv2CallerSession) ESCROW() (uint8, error) {
	return _Settlementv2.Contract.ESCROW(&_Settlementv2.CallOpts)
}

// MAXPENDINGROTATION is a free data retrieval call binding the contract method 0xb1b70322.
//
// Solidity: function MAX_PENDING_ROTATION() view returns(uint256)
func (_Settlementv2 *Settlementv2Caller) MAXPENDINGROTATION(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "MAX_PENDING_ROTATION")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MAXPENDINGROTATION is a free data retrieval call binding the contract method 0xb1b70322.
//
// Solidity: function MAX_PENDING_ROTATION() view returns(uint256)
func (_Settlementv2 *Settlementv2Session) MAXPENDINGROTATION() (*big.Int, error) {
	return _Settlementv2.Contract.MAXPENDINGROTATION(&_Settlementv2.CallOpts)
}

// MAXPENDINGROTATION is a free data retrieval call binding the contract method 0xb1b70322.
//
// Solidity: function MAX_PENDING_ROTATION() view returns(uint256)
func (_Settlementv2 *Settlementv2CallerSession) MAXPENDINGROTATION() (*big.Int, error) {
	return _Settlementv2.Contract.MAXPENDINGROTATION(&_Settlementv2.CallOpts)
}

// OWNER is a free data retrieval call binding the contract method 0x117803e3.
//
// Solidity: function OWNER() view returns(uint8)
func (_Settlementv2 *Settlementv2Caller) OWNER(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "OWNER")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// OWNER is a free data retrieval call binding the contract method 0x117803e3.
//
// Solidity: function OWNER() view returns(uint8)
func (_Settlementv2 *Settlementv2Session) OWNER() (uint8, error) {
	return _Settlementv2.Contract.OWNER(&_Settlementv2.CallOpts)
}

// OWNER is a free data retrieval call binding the contract method 0x117803e3.
//
// Solidity: function OWNER() view returns(uint8)
func (_Settlementv2 *Settlementv2CallerSession) OWNER() (uint8, error) {
	return _Settlementv2.Contract.OWNER(&_Settlementv2.CallOpts)
}

// RENTER is a free data retrieval call binding the contract method 0x40b438dc.
//
// Solidity: function RENTER() view returns(uint8)
func (_Settlementv2 *Settlementv2Caller) RENTER(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "RENTER")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// RENTER is a free data retrieval call binding the contract method 0x40b438dc.
//
// Solidity: function RENTER() view returns(uint8)
func (_Settlementv2 *Settlementv2Session) RENTER() (uint8, error) {
	return _Settlementv2.Contract.RENTER(&_Settlementv2.CallOpts)
}

// RENTER is a free data retrieval call binding the contract method 0x40b438dc.
//
// Solidity: function RENTER() view returns(uint8)
func (_Settlementv2 *Settlementv2CallerSession) RENTER() (uint8, error) {
	return _Settlementv2.Contract.RENTER(&_Settlementv2.CallOpts)
}

// ApprovalDigest is a free data retrieval call binding the contract method 0x4176aa53.
//
// Solidity: function approvalDigest(bytes32 authorizedQuoteDigest) view returns(bytes32)
func (_Settlementv2 *Settlementv2Caller) ApprovalDigest(opts *bind.CallOpts, authorizedQuoteDigest [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "approvalDigest", authorizedQuoteDigest)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ApprovalDigest is a free data retrieval call binding the contract method 0x4176aa53.
//
// Solidity: function approvalDigest(bytes32 authorizedQuoteDigest) view returns(bytes32)
func (_Settlementv2 *Settlementv2Session) ApprovalDigest(authorizedQuoteDigest [32]byte) ([32]byte, error) {
	return _Settlementv2.Contract.ApprovalDigest(&_Settlementv2.CallOpts, authorizedQuoteDigest)
}

// ApprovalDigest is a free data retrieval call binding the contract method 0x4176aa53.
//
// Solidity: function approvalDigest(bytes32 authorizedQuoteDigest) view returns(bytes32)
func (_Settlementv2 *Settlementv2CallerSession) ApprovalDigest(authorizedQuoteDigest [32]byte) ([32]byte, error) {
	return _Settlementv2.Contract.ApprovalDigest(&_Settlementv2.CallOpts, authorizedQuoteDigest)
}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address)
func (_Settlementv2 *Settlementv2Caller) Asset(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "asset")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address)
func (_Settlementv2 *Settlementv2Session) Asset() (common.Address, error) {
	return _Settlementv2.Contract.Asset(&_Settlementv2.CallOpts)
}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address)
func (_Settlementv2 *Settlementv2CallerSession) Asset() (common.Address, error) {
	return _Settlementv2.Contract.Asset(&_Settlementv2.CallOpts)
}

// Beneficiaries is a free data retrieval call binding the contract method 0xb40573e4.
//
// Solidity: function beneficiaries(bytes32 , uint64 ) view returns(address account, uint64 validFrom, uint64 validUntil, bool revoked)
func (_Settlementv2 *Settlementv2Caller) Beneficiaries(opts *bind.CallOpts, arg0 [32]byte, arg1 uint64) (struct {
	Account    common.Address
	ValidFrom  uint64
	ValidUntil uint64
	Revoked    bool
}, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "beneficiaries", arg0, arg1)

	outstruct := new(struct {
		Account    common.Address
		ValidFrom  uint64
		ValidUntil uint64
		Revoked    bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Account = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.ValidFrom = *abi.ConvertType(out[1], new(uint64)).(*uint64)
	outstruct.ValidUntil = *abi.ConvertType(out[2], new(uint64)).(*uint64)
	outstruct.Revoked = *abi.ConvertType(out[3], new(bool)).(*bool)

	return *outstruct, err

}

// Beneficiaries is a free data retrieval call binding the contract method 0xb40573e4.
//
// Solidity: function beneficiaries(bytes32 , uint64 ) view returns(address account, uint64 validFrom, uint64 validUntil, bool revoked)
func (_Settlementv2 *Settlementv2Session) Beneficiaries(arg0 [32]byte, arg1 uint64) (struct {
	Account    common.Address
	ValidFrom  uint64
	ValidUntil uint64
	Revoked    bool
}, error) {
	return _Settlementv2.Contract.Beneficiaries(&_Settlementv2.CallOpts, arg0, arg1)
}

// Beneficiaries is a free data retrieval call binding the contract method 0xb40573e4.
//
// Solidity: function beneficiaries(bytes32 , uint64 ) view returns(address account, uint64 validFrom, uint64 validUntil, bool revoked)
func (_Settlementv2 *Settlementv2CallerSession) Beneficiaries(arg0 [32]byte, arg1 uint64) (struct {
	Account    common.Address
	ValidFrom  uint64
	ValidUntil uint64
	Revoked    bool
}, error) {
	return _Settlementv2.Contract.Beneficiaries(&_Settlementv2.CallOpts, arg0, arg1)
}

// DirectEnabled is a free data retrieval call binding the contract method 0x4826fa9c.
//
// Solidity: function directEnabled() view returns(bool)
func (_Settlementv2 *Settlementv2Caller) DirectEnabled(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "directEnabled")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// DirectEnabled is a free data retrieval call binding the contract method 0x4826fa9c.
//
// Solidity: function directEnabled() view returns(bool)
func (_Settlementv2 *Settlementv2Session) DirectEnabled() (bool, error) {
	return _Settlementv2.Contract.DirectEnabled(&_Settlementv2.CallOpts)
}

// DirectEnabled is a free data retrieval call binding the contract method 0x4826fa9c.
//
// Solidity: function directEnabled() view returns(bool)
func (_Settlementv2 *Settlementv2CallerSession) DirectEnabled() (bool, error) {
	return _Settlementv2.Contract.DirectEnabled(&_Settlementv2.CallOpts)
}

// DomainSeparator is a free data retrieval call binding the contract method 0xf698da25.
//
// Solidity: function domainSeparator() view returns(bytes32)
func (_Settlementv2 *Settlementv2Caller) DomainSeparator(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "domainSeparator")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// DomainSeparator is a free data retrieval call binding the contract method 0xf698da25.
//
// Solidity: function domainSeparator() view returns(bytes32)
func (_Settlementv2 *Settlementv2Session) DomainSeparator() ([32]byte, error) {
	return _Settlementv2.Contract.DomainSeparator(&_Settlementv2.CallOpts)
}

// DomainSeparator is a free data retrieval call binding the contract method 0xf698da25.
//
// Solidity: function domainSeparator() view returns(bytes32)
func (_Settlementv2 *Settlementv2CallerSession) DomainSeparator() ([32]byte, error) {
	return _Settlementv2.Contract.DomainSeparator(&_Settlementv2.CallOpts)
}

// EscrowBalance is a free data retrieval call binding the contract method 0xcc5931bd.
//
// Solidity: function escrowBalance(address , uint64 ) view returns(uint256)
func (_Settlementv2 *Settlementv2Caller) EscrowBalance(opts *bind.CallOpts, arg0 common.Address, arg1 uint64) (*big.Int, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "escrowBalance", arg0, arg1)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// EscrowBalance is a free data retrieval call binding the contract method 0xcc5931bd.
//
// Solidity: function escrowBalance(address , uint64 ) view returns(uint256)
func (_Settlementv2 *Settlementv2Session) EscrowBalance(arg0 common.Address, arg1 uint64) (*big.Int, error) {
	return _Settlementv2.Contract.EscrowBalance(&_Settlementv2.CallOpts, arg0, arg1)
}

// EscrowBalance is a free data retrieval call binding the contract method 0xcc5931bd.
//
// Solidity: function escrowBalance(address , uint64 ) view returns(uint256)
func (_Settlementv2 *Settlementv2CallerSession) EscrowBalance(arg0 common.Address, arg1 uint64) (*big.Int, error) {
	return _Settlementv2.Contract.EscrowBalance(&_Settlementv2.CallOpts, arg0, arg1)
}

// EscrowWithdrawalAddress is a free data retrieval call binding the contract method 0x5285e62f.
//
// Solidity: function escrowWithdrawalAddress(address , uint64 ) view returns(address)
func (_Settlementv2 *Settlementv2Caller) EscrowWithdrawalAddress(opts *bind.CallOpts, arg0 common.Address, arg1 uint64) (common.Address, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "escrowWithdrawalAddress", arg0, arg1)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// EscrowWithdrawalAddress is a free data retrieval call binding the contract method 0x5285e62f.
//
// Solidity: function escrowWithdrawalAddress(address , uint64 ) view returns(address)
func (_Settlementv2 *Settlementv2Session) EscrowWithdrawalAddress(arg0 common.Address, arg1 uint64) (common.Address, error) {
	return _Settlementv2.Contract.EscrowWithdrawalAddress(&_Settlementv2.CallOpts, arg0, arg1)
}

// EscrowWithdrawalAddress is a free data retrieval call binding the contract method 0x5285e62f.
//
// Solidity: function escrowWithdrawalAddress(address , uint64 ) view returns(address)
func (_Settlementv2 *Settlementv2CallerSession) EscrowWithdrawalAddress(arg0 common.Address, arg1 uint64) (common.Address, error) {
	return _Settlementv2.Contract.EscrowWithdrawalAddress(&_Settlementv2.CallOpts, arg0, arg1)
}

// Governor is a free data retrieval call binding the contract method 0x0c340a24.
//
// Solidity: function governor() view returns(address)
func (_Settlementv2 *Settlementv2Caller) Governor(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "governor")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Governor is a free data retrieval call binding the contract method 0x0c340a24.
//
// Solidity: function governor() view returns(address)
func (_Settlementv2 *Settlementv2Session) Governor() (common.Address, error) {
	return _Settlementv2.Contract.Governor(&_Settlementv2.CallOpts)
}

// Governor is a free data retrieval call binding the contract method 0x0c340a24.
//
// Solidity: function governor() view returns(address)
func (_Settlementv2 *Settlementv2CallerSession) Governor() (common.Address, error) {
	return _Settlementv2.Contract.Governor(&_Settlementv2.CallOpts)
}

// Grandfathered is a free data retrieval call binding the contract method 0x0fc9b68c.
//
// Solidity: function grandfathered(bytes32 , bytes32 ) view returns(bool)
func (_Settlementv2 *Settlementv2Caller) Grandfathered(opts *bind.CallOpts, arg0 [32]byte, arg1 [32]byte) (bool, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "grandfathered", arg0, arg1)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// Grandfathered is a free data retrieval call binding the contract method 0x0fc9b68c.
//
// Solidity: function grandfathered(bytes32 , bytes32 ) view returns(bool)
func (_Settlementv2 *Settlementv2Session) Grandfathered(arg0 [32]byte, arg1 [32]byte) (bool, error) {
	return _Settlementv2.Contract.Grandfathered(&_Settlementv2.CallOpts, arg0, arg1)
}

// Grandfathered is a free data retrieval call binding the contract method 0x0fc9b68c.
//
// Solidity: function grandfathered(bytes32 , bytes32 ) view returns(bool)
func (_Settlementv2 *Settlementv2CallerSession) Grandfathered(arg0 [32]byte, arg1 [32]byte) (bool, error) {
	return _Settlementv2.Contract.Grandfathered(&_Settlementv2.CallOpts, arg0, arg1)
}

// MerchantKeys is a free data retrieval call binding the contract method 0x48310fe5.
//
// Solidity: function merchantKeys(bytes32 , uint64 ) view returns(address account, uint64 validFrom, uint64 validUntil, bool revoked)
func (_Settlementv2 *Settlementv2Caller) MerchantKeys(opts *bind.CallOpts, arg0 [32]byte, arg1 uint64) (struct {
	Account    common.Address
	ValidFrom  uint64
	ValidUntil uint64
	Revoked    bool
}, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "merchantKeys", arg0, arg1)

	outstruct := new(struct {
		Account    common.Address
		ValidFrom  uint64
		ValidUntil uint64
		Revoked    bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Account = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.ValidFrom = *abi.ConvertType(out[1], new(uint64)).(*uint64)
	outstruct.ValidUntil = *abi.ConvertType(out[2], new(uint64)).(*uint64)
	outstruct.Revoked = *abi.ConvertType(out[3], new(bool)).(*bool)

	return *outstruct, err

}

// MerchantKeys is a free data retrieval call binding the contract method 0x48310fe5.
//
// Solidity: function merchantKeys(bytes32 , uint64 ) view returns(address account, uint64 validFrom, uint64 validUntil, bool revoked)
func (_Settlementv2 *Settlementv2Session) MerchantKeys(arg0 [32]byte, arg1 uint64) (struct {
	Account    common.Address
	ValidFrom  uint64
	ValidUntil uint64
	Revoked    bool
}, error) {
	return _Settlementv2.Contract.MerchantKeys(&_Settlementv2.CallOpts, arg0, arg1)
}

// MerchantKeys is a free data retrieval call binding the contract method 0x48310fe5.
//
// Solidity: function merchantKeys(bytes32 , uint64 ) view returns(address account, uint64 validFrom, uint64 validUntil, bool revoked)
func (_Settlementv2 *Settlementv2CallerSession) MerchantKeys(arg0 [32]byte, arg1 uint64) (struct {
	Account    common.Address
	ValidFrom  uint64
	ValidUntil uint64
	Revoked    bool
}, error) {
	return _Settlementv2.Contract.MerchantKeys(&_Settlementv2.CallOpts, arg0, arg1)
}

// MerchantNonceUsed is a free data retrieval call binding the contract method 0x88dff597.
//
// Solidity: function merchantNonceUsed(bytes32 , uint256 ) view returns(bool)
func (_Settlementv2 *Settlementv2Caller) MerchantNonceUsed(opts *bind.CallOpts, arg0 [32]byte, arg1 *big.Int) (bool, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "merchantNonceUsed", arg0, arg1)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// MerchantNonceUsed is a free data retrieval call binding the contract method 0x88dff597.
//
// Solidity: function merchantNonceUsed(bytes32 , uint256 ) view returns(bool)
func (_Settlementv2 *Settlementv2Session) MerchantNonceUsed(arg0 [32]byte, arg1 *big.Int) (bool, error) {
	return _Settlementv2.Contract.MerchantNonceUsed(&_Settlementv2.CallOpts, arg0, arg1)
}

// MerchantNonceUsed is a free data retrieval call binding the contract method 0x88dff597.
//
// Solidity: function merchantNonceUsed(bytes32 , uint256 ) view returns(bool)
func (_Settlementv2 *Settlementv2CallerSession) MerchantNonceUsed(arg0 [32]byte, arg1 *big.Int) (bool, error) {
	return _Settlementv2.Contract.MerchantNonceUsed(&_Settlementv2.CallOpts, arg0, arg1)
}

// Merchants is a free data retrieval call binding the contract method 0x20003022.
//
// Solidity: function merchants(bytes32 ) view returns(uint64 keyVersion, uint64 beneficiaryVersion, uint64 refundKeyVersion, address refundSigner, bool active)
func (_Settlementv2 *Settlementv2Caller) Merchants(opts *bind.CallOpts, arg0 [32]byte) (struct {
	KeyVersion         uint64
	BeneficiaryVersion uint64
	RefundKeyVersion   uint64
	RefundSigner       common.Address
	Active             bool
}, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "merchants", arg0)

	outstruct := new(struct {
		KeyVersion         uint64
		BeneficiaryVersion uint64
		RefundKeyVersion   uint64
		RefundSigner       common.Address
		Active             bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.KeyVersion = *abi.ConvertType(out[0], new(uint64)).(*uint64)
	outstruct.BeneficiaryVersion = *abi.ConvertType(out[1], new(uint64)).(*uint64)
	outstruct.RefundKeyVersion = *abi.ConvertType(out[2], new(uint64)).(*uint64)
	outstruct.RefundSigner = *abi.ConvertType(out[3], new(common.Address)).(*common.Address)
	outstruct.Active = *abi.ConvertType(out[4], new(bool)).(*bool)

	return *outstruct, err

}

// Merchants is a free data retrieval call binding the contract method 0x20003022.
//
// Solidity: function merchants(bytes32 ) view returns(uint64 keyVersion, uint64 beneficiaryVersion, uint64 refundKeyVersion, address refundSigner, bool active)
func (_Settlementv2 *Settlementv2Session) Merchants(arg0 [32]byte) (struct {
	KeyVersion         uint64
	BeneficiaryVersion uint64
	RefundKeyVersion   uint64
	RefundSigner       common.Address
	Active             bool
}, error) {
	return _Settlementv2.Contract.Merchants(&_Settlementv2.CallOpts, arg0)
}

// Merchants is a free data retrieval call binding the contract method 0x20003022.
//
// Solidity: function merchants(bytes32 ) view returns(uint64 keyVersion, uint64 beneficiaryVersion, uint64 refundKeyVersion, address refundSigner, bool active)
func (_Settlementv2 *Settlementv2CallerSession) Merchants(arg0 [32]byte) (struct {
	KeyVersion         uint64
	BeneficiaryVersion uint64
	RefundKeyVersion   uint64
	RefundSigner       common.Address
	Active             bool
}, error) {
	return _Settlementv2.Contract.Merchants(&_Settlementv2.CallOpts, arg0)
}

// PayerNonceUsed is a free data retrieval call binding the contract method 0x39314115.
//
// Solidity: function payerNonceUsed(address , uint256 ) view returns(bool)
func (_Settlementv2 *Settlementv2Caller) PayerNonceUsed(opts *bind.CallOpts, arg0 common.Address, arg1 *big.Int) (bool, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "payerNonceUsed", arg0, arg1)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// PayerNonceUsed is a free data retrieval call binding the contract method 0x39314115.
//
// Solidity: function payerNonceUsed(address , uint256 ) view returns(bool)
func (_Settlementv2 *Settlementv2Session) PayerNonceUsed(arg0 common.Address, arg1 *big.Int) (bool, error) {
	return _Settlementv2.Contract.PayerNonceUsed(&_Settlementv2.CallOpts, arg0, arg1)
}

// PayerNonceUsed is a free data retrieval call binding the contract method 0x39314115.
//
// Solidity: function payerNonceUsed(address , uint256 ) view returns(bool)
func (_Settlementv2 *Settlementv2CallerSession) PayerNonceUsed(arg0 common.Address, arg1 *big.Int) (bool, error) {
	return _Settlementv2.Contract.PayerNonceUsed(&_Settlementv2.CallOpts, arg0, arg1)
}

// Payers is a free data retrieval call binding the contract method 0x4a714c24.
//
// Solidity: function payers(address ) view returns(uint64 generation, uint8 role, bool active)
func (_Settlementv2 *Settlementv2Caller) Payers(opts *bind.CallOpts, arg0 common.Address) (struct {
	Generation uint64
	Role       uint8
	Active     bool
}, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "payers", arg0)

	outstruct := new(struct {
		Generation uint64
		Role       uint8
		Active     bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Generation = *abi.ConvertType(out[0], new(uint64)).(*uint64)
	outstruct.Role = *abi.ConvertType(out[1], new(uint8)).(*uint8)
	outstruct.Active = *abi.ConvertType(out[2], new(bool)).(*bool)

	return *outstruct, err

}

// Payers is a free data retrieval call binding the contract method 0x4a714c24.
//
// Solidity: function payers(address ) view returns(uint64 generation, uint8 role, bool active)
func (_Settlementv2 *Settlementv2Session) Payers(arg0 common.Address) (struct {
	Generation uint64
	Role       uint8
	Active     bool
}, error) {
	return _Settlementv2.Contract.Payers(&_Settlementv2.CallOpts, arg0)
}

// Payers is a free data retrieval call binding the contract method 0x4a714c24.
//
// Solidity: function payers(address ) view returns(uint64 generation, uint8 role, bool active)
func (_Settlementv2 *Settlementv2CallerSession) Payers(arg0 common.Address) (struct {
	Generation uint64
	Role       uint8
	Active     bool
}, error) {
	return _Settlementv2.Contract.Payers(&_Settlementv2.CallOpts, arg0)
}

// Payments is a free data retrieval call binding the contract method 0x9df7c4f9.
//
// Solidity: function payments(bytes32 , bytes32 ) view returns(address payer, uint256 amount, uint256 refunded, uint64 payerGeneration, uint8 mode)
func (_Settlementv2 *Settlementv2Caller) Payments(opts *bind.CallOpts, arg0 [32]byte, arg1 [32]byte) (struct {
	Payer           common.Address
	Amount          *big.Int
	Refunded        *big.Int
	PayerGeneration uint64
	Mode            uint8
}, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "payments", arg0, arg1)

	outstruct := new(struct {
		Payer           common.Address
		Amount          *big.Int
		Refunded        *big.Int
		PayerGeneration uint64
		Mode            uint8
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Payer = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.Amount = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	outstruct.Refunded = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.PayerGeneration = *abi.ConvertType(out[3], new(uint64)).(*uint64)
	outstruct.Mode = *abi.ConvertType(out[4], new(uint8)).(*uint8)

	return *outstruct, err

}

// Payments is a free data retrieval call binding the contract method 0x9df7c4f9.
//
// Solidity: function payments(bytes32 , bytes32 ) view returns(address payer, uint256 amount, uint256 refunded, uint64 payerGeneration, uint8 mode)
func (_Settlementv2 *Settlementv2Session) Payments(arg0 [32]byte, arg1 [32]byte) (struct {
	Payer           common.Address
	Amount          *big.Int
	Refunded        *big.Int
	PayerGeneration uint64
	Mode            uint8
}, error) {
	return _Settlementv2.Contract.Payments(&_Settlementv2.CallOpts, arg0, arg1)
}

// Payments is a free data retrieval call binding the contract method 0x9df7c4f9.
//
// Solidity: function payments(bytes32 , bytes32 ) view returns(address payer, uint256 amount, uint256 refunded, uint64 payerGeneration, uint8 mode)
func (_Settlementv2 *Settlementv2CallerSession) Payments(arg0 [32]byte, arg1 [32]byte) (struct {
	Payer           common.Address
	Amount          *big.Int
	Refunded        *big.Int
	PayerGeneration uint64
	Mode            uint8
}, error) {
	return _Settlementv2.Contract.Payments(&_Settlementv2.CallOpts, arg0, arg1)
}

// QuoteDigest is a free data retrieval call binding the contract method 0xbb3fa477.
//
// Solidity: function quoteDigest((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote) view returns(bytes32)
func (_Settlementv2 *Settlementv2Caller) QuoteDigest(opts *bind.CallOpts, quote PaymentTypesV2MerchantQuote) ([32]byte, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "quoteDigest", quote)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// QuoteDigest is a free data retrieval call binding the contract method 0xbb3fa477.
//
// Solidity: function quoteDigest((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote) view returns(bytes32)
func (_Settlementv2 *Settlementv2Session) QuoteDigest(quote PaymentTypesV2MerchantQuote) ([32]byte, error) {
	return _Settlementv2.Contract.QuoteDigest(&_Settlementv2.CallOpts, quote)
}

// QuoteDigest is a free data retrieval call binding the contract method 0xbb3fa477.
//
// Solidity: function quoteDigest((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote) view returns(bytes32)
func (_Settlementv2 *Settlementv2CallerSession) QuoteDigest(quote PaymentTypesV2MerchantQuote) ([32]byte, error) {
	return _Settlementv2.Contract.QuoteDigest(&_Settlementv2.CallOpts, quote)
}

// RefundDigest is a free data retrieval call binding the contract method 0x5d719bcd.
//
// Solidity: function refundDigest((bytes32,bytes32,bytes32,uint256,uint64,uint64) authorization) view returns(bytes32)
func (_Settlementv2 *Settlementv2Caller) RefundDigest(opts *bind.CallOpts, authorization PaymentTypesV2RefundAuthorization) ([32]byte, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "refundDigest", authorization)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// RefundDigest is a free data retrieval call binding the contract method 0x5d719bcd.
//
// Solidity: function refundDigest((bytes32,bytes32,bytes32,uint256,uint64,uint64) authorization) view returns(bytes32)
func (_Settlementv2 *Settlementv2Session) RefundDigest(authorization PaymentTypesV2RefundAuthorization) ([32]byte, error) {
	return _Settlementv2.Contract.RefundDigest(&_Settlementv2.CallOpts, authorization)
}

// RefundDigest is a free data retrieval call binding the contract method 0x5d719bcd.
//
// Solidity: function refundDigest((bytes32,bytes32,bytes32,uint256,uint64,uint64) authorization) view returns(bytes32)
func (_Settlementv2 *Settlementv2CallerSession) RefundDigest(authorization PaymentTypesV2RefundAuthorization) ([32]byte, error) {
	return _Settlementv2.Contract.RefundDigest(&_Settlementv2.CallOpts, authorization)
}

// RefundIdUsed is a free data retrieval call binding the contract method 0x7412165d.
//
// Solidity: function refundIdUsed(bytes32 , bytes32 , bytes32 ) view returns(bool)
func (_Settlementv2 *Settlementv2Caller) RefundIdUsed(opts *bind.CallOpts, arg0 [32]byte, arg1 [32]byte, arg2 [32]byte) (bool, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "refundIdUsed", arg0, arg1, arg2)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// RefundIdUsed is a free data retrieval call binding the contract method 0x7412165d.
//
// Solidity: function refundIdUsed(bytes32 , bytes32 , bytes32 ) view returns(bool)
func (_Settlementv2 *Settlementv2Session) RefundIdUsed(arg0 [32]byte, arg1 [32]byte, arg2 [32]byte) (bool, error) {
	return _Settlementv2.Contract.RefundIdUsed(&_Settlementv2.CallOpts, arg0, arg1, arg2)
}

// RefundIdUsed is a free data retrieval call binding the contract method 0x7412165d.
//
// Solidity: function refundIdUsed(bytes32 , bytes32 , bytes32 ) view returns(bool)
func (_Settlementv2 *Settlementv2CallerSession) RefundIdUsed(arg0 [32]byte, arg1 [32]byte, arg2 [32]byte) (bool, error) {
	return _Settlementv2.Contract.RefundIdUsed(&_Settlementv2.CallOpts, arg0, arg1, arg2)
}

// RefundReserve is a free data retrieval call binding the contract method 0x6e718073.
//
// Solidity: function refundReserve(bytes32 ) view returns(uint256)
func (_Settlementv2 *Settlementv2Caller) RefundReserve(opts *bind.CallOpts, arg0 [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "refundReserve", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// RefundReserve is a free data retrieval call binding the contract method 0x6e718073.
//
// Solidity: function refundReserve(bytes32 ) view returns(uint256)
func (_Settlementv2 *Settlementv2Session) RefundReserve(arg0 [32]byte) (*big.Int, error) {
	return _Settlementv2.Contract.RefundReserve(&_Settlementv2.CallOpts, arg0)
}

// RefundReserve is a free data retrieval call binding the contract method 0x6e718073.
//
// Solidity: function refundReserve(bytes32 ) view returns(uint256)
func (_Settlementv2 *Settlementv2CallerSession) RefundReserve(arg0 [32]byte) (*big.Int, error) {
	return _Settlementv2.Contract.RefundReserve(&_Settlementv2.CallOpts, arg0)
}

// TotalEscrow is a free data retrieval call binding the contract method 0xa3d89844.
//
// Solidity: function totalEscrow() view returns(uint256)
func (_Settlementv2 *Settlementv2Caller) TotalEscrow(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "totalEscrow")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalEscrow is a free data retrieval call binding the contract method 0xa3d89844.
//
// Solidity: function totalEscrow() view returns(uint256)
func (_Settlementv2 *Settlementv2Session) TotalEscrow() (*big.Int, error) {
	return _Settlementv2.Contract.TotalEscrow(&_Settlementv2.CallOpts)
}

// TotalEscrow is a free data retrieval call binding the contract method 0xa3d89844.
//
// Solidity: function totalEscrow() view returns(uint256)
func (_Settlementv2 *Settlementv2CallerSession) TotalEscrow() (*big.Int, error) {
	return _Settlementv2.Contract.TotalEscrow(&_Settlementv2.CallOpts)
}

// TotalRefundReserve is a free data retrieval call binding the contract method 0x2ad92606.
//
// Solidity: function totalRefundReserve() view returns(uint256)
func (_Settlementv2 *Settlementv2Caller) TotalRefundReserve(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Settlementv2.contract.Call(opts, &out, "totalRefundReserve")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalRefundReserve is a free data retrieval call binding the contract method 0x2ad92606.
//
// Solidity: function totalRefundReserve() view returns(uint256)
func (_Settlementv2 *Settlementv2Session) TotalRefundReserve() (*big.Int, error) {
	return _Settlementv2.Contract.TotalRefundReserve(&_Settlementv2.CallOpts)
}

// TotalRefundReserve is a free data retrieval call binding the contract method 0x2ad92606.
//
// Solidity: function totalRefundReserve() view returns(uint256)
func (_Settlementv2 *Settlementv2CallerSession) TotalRefundReserve() (*big.Int, error) {
	return _Settlementv2.Contract.TotalRefundReserve(&_Settlementv2.CallOpts)
}

// DeactivatePayer is a paid mutator transaction binding the contract method 0x9ed7a570.
//
// Solidity: function deactivatePayer(address payer) returns()
func (_Settlementv2 *Settlementv2Transactor) DeactivatePayer(opts *bind.TransactOpts, payer common.Address) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "deactivatePayer", payer)
}

// DeactivatePayer is a paid mutator transaction binding the contract method 0x9ed7a570.
//
// Solidity: function deactivatePayer(address payer) returns()
func (_Settlementv2 *Settlementv2Session) DeactivatePayer(payer common.Address) (*types.Transaction, error) {
	return _Settlementv2.Contract.DeactivatePayer(&_Settlementv2.TransactOpts, payer)
}

// DeactivatePayer is a paid mutator transaction binding the contract method 0x9ed7a570.
//
// Solidity: function deactivatePayer(address payer) returns()
func (_Settlementv2 *Settlementv2TransactorSession) DeactivatePayer(payer common.Address) (*types.Transaction, error) {
	return _Settlementv2.Contract.DeactivatePayer(&_Settlementv2.TransactOpts, payer)
}

// DepositEscrow is a paid mutator transaction binding the contract method 0x054fa1c2.
//
// Solidity: function depositEscrow(address payer, uint64 generation, uint256 amount) returns()
func (_Settlementv2 *Settlementv2Transactor) DepositEscrow(opts *bind.TransactOpts, payer common.Address, generation uint64, amount *big.Int) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "depositEscrow", payer, generation, amount)
}

// DepositEscrow is a paid mutator transaction binding the contract method 0x054fa1c2.
//
// Solidity: function depositEscrow(address payer, uint64 generation, uint256 amount) returns()
func (_Settlementv2 *Settlementv2Session) DepositEscrow(payer common.Address, generation uint64, amount *big.Int) (*types.Transaction, error) {
	return _Settlementv2.Contract.DepositEscrow(&_Settlementv2.TransactOpts, payer, generation, amount)
}

// DepositEscrow is a paid mutator transaction binding the contract method 0x054fa1c2.
//
// Solidity: function depositEscrow(address payer, uint64 generation, uint256 amount) returns()
func (_Settlementv2 *Settlementv2TransactorSession) DepositEscrow(payer common.Address, generation uint64, amount *big.Int) (*types.Transaction, error) {
	return _Settlementv2.Contract.DepositEscrow(&_Settlementv2.TransactOpts, payer, generation, amount)
}

// FundRefundReserve is a paid mutator transaction binding the contract method 0x4f25d16b.
//
// Solidity: function fundRefundReserve(bytes32 merchantId, uint256 amount) returns()
func (_Settlementv2 *Settlementv2Transactor) FundRefundReserve(opts *bind.TransactOpts, merchantId [32]byte, amount *big.Int) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "fundRefundReserve", merchantId, amount)
}

// FundRefundReserve is a paid mutator transaction binding the contract method 0x4f25d16b.
//
// Solidity: function fundRefundReserve(bytes32 merchantId, uint256 amount) returns()
func (_Settlementv2 *Settlementv2Session) FundRefundReserve(merchantId [32]byte, amount *big.Int) (*types.Transaction, error) {
	return _Settlementv2.Contract.FundRefundReserve(&_Settlementv2.TransactOpts, merchantId, amount)
}

// FundRefundReserve is a paid mutator transaction binding the contract method 0x4f25d16b.
//
// Solidity: function fundRefundReserve(bytes32 merchantId, uint256 amount) returns()
func (_Settlementv2 *Settlementv2TransactorSession) FundRefundReserve(merchantId [32]byte, amount *big.Int) (*types.Transaction, error) {
	return _Settlementv2.Contract.FundRefundReserve(&_Settlementv2.TransactOpts, merchantId, amount)
}

// Refund is a paid mutator transaction binding the contract method 0x0986c87f.
//
// Solidity: function refund((bytes32,bytes32,bytes32,uint256,uint64,uint64) authorization, bytes signature) returns()
func (_Settlementv2 *Settlementv2Transactor) Refund(opts *bind.TransactOpts, authorization PaymentTypesV2RefundAuthorization, signature []byte) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "refund", authorization, signature)
}

// Refund is a paid mutator transaction binding the contract method 0x0986c87f.
//
// Solidity: function refund((bytes32,bytes32,bytes32,uint256,uint64,uint64) authorization, bytes signature) returns()
func (_Settlementv2 *Settlementv2Session) Refund(authorization PaymentTypesV2RefundAuthorization, signature []byte) (*types.Transaction, error) {
	return _Settlementv2.Contract.Refund(&_Settlementv2.TransactOpts, authorization, signature)
}

// Refund is a paid mutator transaction binding the contract method 0x0986c87f.
//
// Solidity: function refund((bytes32,bytes32,bytes32,uint256,uint64,uint64) authorization, bytes signature) returns()
func (_Settlementv2 *Settlementv2TransactorSession) Refund(authorization PaymentTypesV2RefundAuthorization, signature []byte) (*types.Transaction, error) {
	return _Settlementv2.Contract.Refund(&_Settlementv2.TransactOpts, authorization, signature)
}

// RegisterMerchant is a paid mutator transaction binding the contract method 0xb77316a5.
//
// Solidity: function registerMerchant(bytes32 merchantId, address signer, address beneficiary, address refundSigner) returns()
func (_Settlementv2 *Settlementv2Transactor) RegisterMerchant(opts *bind.TransactOpts, merchantId [32]byte, signer common.Address, beneficiary common.Address, refundSigner common.Address) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "registerMerchant", merchantId, signer, beneficiary, refundSigner)
}

// RegisterMerchant is a paid mutator transaction binding the contract method 0xb77316a5.
//
// Solidity: function registerMerchant(bytes32 merchantId, address signer, address beneficiary, address refundSigner) returns()
func (_Settlementv2 *Settlementv2Session) RegisterMerchant(merchantId [32]byte, signer common.Address, beneficiary common.Address, refundSigner common.Address) (*types.Transaction, error) {
	return _Settlementv2.Contract.RegisterMerchant(&_Settlementv2.TransactOpts, merchantId, signer, beneficiary, refundSigner)
}

// RegisterMerchant is a paid mutator transaction binding the contract method 0xb77316a5.
//
// Solidity: function registerMerchant(bytes32 merchantId, address signer, address beneficiary, address refundSigner) returns()
func (_Settlementv2 *Settlementv2TransactorSession) RegisterMerchant(merchantId [32]byte, signer common.Address, beneficiary common.Address, refundSigner common.Address) (*types.Transaction, error) {
	return _Settlementv2.Contract.RegisterMerchant(&_Settlementv2.TransactOpts, merchantId, signer, beneficiary, refundSigner)
}

// RegisterPayer is a paid mutator transaction binding the contract method 0x800e4550.
//
// Solidity: function registerPayer(address payer, uint64 generation, uint8 role, address withdrawalAddress) returns()
func (_Settlementv2 *Settlementv2Transactor) RegisterPayer(opts *bind.TransactOpts, payer common.Address, generation uint64, role uint8, withdrawalAddress common.Address) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "registerPayer", payer, generation, role, withdrawalAddress)
}

// RegisterPayer is a paid mutator transaction binding the contract method 0x800e4550.
//
// Solidity: function registerPayer(address payer, uint64 generation, uint8 role, address withdrawalAddress) returns()
func (_Settlementv2 *Settlementv2Session) RegisterPayer(payer common.Address, generation uint64, role uint8, withdrawalAddress common.Address) (*types.Transaction, error) {
	return _Settlementv2.Contract.RegisterPayer(&_Settlementv2.TransactOpts, payer, generation, role, withdrawalAddress)
}

// RegisterPayer is a paid mutator transaction binding the contract method 0x800e4550.
//
// Solidity: function registerPayer(address payer, uint64 generation, uint8 role, address withdrawalAddress) returns()
func (_Settlementv2 *Settlementv2TransactorSession) RegisterPayer(payer common.Address, generation uint64, role uint8, withdrawalAddress common.Address) (*types.Transaction, error) {
	return _Settlementv2.Contract.RegisterPayer(&_Settlementv2.TransactOpts, payer, generation, role, withdrawalAddress)
}

// RevokeMerchantKey is a paid mutator transaction binding the contract method 0xfe89ccc7.
//
// Solidity: function revokeMerchantKey(bytes32 merchantId, uint64 version) returns()
func (_Settlementv2 *Settlementv2Transactor) RevokeMerchantKey(opts *bind.TransactOpts, merchantId [32]byte, version uint64) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "revokeMerchantKey", merchantId, version)
}

// RevokeMerchantKey is a paid mutator transaction binding the contract method 0xfe89ccc7.
//
// Solidity: function revokeMerchantKey(bytes32 merchantId, uint64 version) returns()
func (_Settlementv2 *Settlementv2Session) RevokeMerchantKey(merchantId [32]byte, version uint64) (*types.Transaction, error) {
	return _Settlementv2.Contract.RevokeMerchantKey(&_Settlementv2.TransactOpts, merchantId, version)
}

// RevokeMerchantKey is a paid mutator transaction binding the contract method 0xfe89ccc7.
//
// Solidity: function revokeMerchantKey(bytes32 merchantId, uint64 version) returns()
func (_Settlementv2 *Settlementv2TransactorSession) RevokeMerchantKey(merchantId [32]byte, version uint64) (*types.Transaction, error) {
	return _Settlementv2.Contract.RevokeMerchantKey(&_Settlementv2.TransactOpts, merchantId, version)
}

// RotateMerchant is a paid mutator transaction binding the contract method 0x94705e56.
//
// Solidity: function rotateMerchant(bytes32 merchantId, address nextSigner, address nextBeneficiary, bytes32[] openDigests) returns()
func (_Settlementv2 *Settlementv2Transactor) RotateMerchant(opts *bind.TransactOpts, merchantId [32]byte, nextSigner common.Address, nextBeneficiary common.Address, openDigests [][32]byte) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "rotateMerchant", merchantId, nextSigner, nextBeneficiary, openDigests)
}

// RotateMerchant is a paid mutator transaction binding the contract method 0x94705e56.
//
// Solidity: function rotateMerchant(bytes32 merchantId, address nextSigner, address nextBeneficiary, bytes32[] openDigests) returns()
func (_Settlementv2 *Settlementv2Session) RotateMerchant(merchantId [32]byte, nextSigner common.Address, nextBeneficiary common.Address, openDigests [][32]byte) (*types.Transaction, error) {
	return _Settlementv2.Contract.RotateMerchant(&_Settlementv2.TransactOpts, merchantId, nextSigner, nextBeneficiary, openDigests)
}

// RotateMerchant is a paid mutator transaction binding the contract method 0x94705e56.
//
// Solidity: function rotateMerchant(bytes32 merchantId, address nextSigner, address nextBeneficiary, bytes32[] openDigests) returns()
func (_Settlementv2 *Settlementv2TransactorSession) RotateMerchant(merchantId [32]byte, nextSigner common.Address, nextBeneficiary common.Address, openDigests [][32]byte) (*types.Transaction, error) {
	return _Settlementv2.Contract.RotateMerchant(&_Settlementv2.TransactOpts, merchantId, nextSigner, nextBeneficiary, openDigests)
}

// RotateRefundSigner is a paid mutator transaction binding the contract method 0xf5ede49e.
//
// Solidity: function rotateRefundSigner(bytes32 merchantId, address signer) returns()
func (_Settlementv2 *Settlementv2Transactor) RotateRefundSigner(opts *bind.TransactOpts, merchantId [32]byte, signer common.Address) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "rotateRefundSigner", merchantId, signer)
}

// RotateRefundSigner is a paid mutator transaction binding the contract method 0xf5ede49e.
//
// Solidity: function rotateRefundSigner(bytes32 merchantId, address signer) returns()
func (_Settlementv2 *Settlementv2Session) RotateRefundSigner(merchantId [32]byte, signer common.Address) (*types.Transaction, error) {
	return _Settlementv2.Contract.RotateRefundSigner(&_Settlementv2.TransactOpts, merchantId, signer)
}

// RotateRefundSigner is a paid mutator transaction binding the contract method 0xf5ede49e.
//
// Solidity: function rotateRefundSigner(bytes32 merchantId, address signer) returns()
func (_Settlementv2 *Settlementv2TransactorSession) RotateRefundSigner(merchantId [32]byte, signer common.Address) (*types.Transaction, error) {
	return _Settlementv2.Contract.RotateRefundSigner(&_Settlementv2.TransactOpts, merchantId, signer)
}

// SetMerchantActive is a paid mutator transaction binding the contract method 0x6976e675.
//
// Solidity: function setMerchantActive(bytes32 merchantId, bool active) returns()
func (_Settlementv2 *Settlementv2Transactor) SetMerchantActive(opts *bind.TransactOpts, merchantId [32]byte, active bool) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "setMerchantActive", merchantId, active)
}

// SetMerchantActive is a paid mutator transaction binding the contract method 0x6976e675.
//
// Solidity: function setMerchantActive(bytes32 merchantId, bool active) returns()
func (_Settlementv2 *Settlementv2Session) SetMerchantActive(merchantId [32]byte, active bool) (*types.Transaction, error) {
	return _Settlementv2.Contract.SetMerchantActive(&_Settlementv2.TransactOpts, merchantId, active)
}

// SetMerchantActive is a paid mutator transaction binding the contract method 0x6976e675.
//
// Solidity: function setMerchantActive(bytes32 merchantId, bool active) returns()
func (_Settlementv2 *Settlementv2TransactorSession) SetMerchantActive(merchantId [32]byte, active bool) (*types.Transaction, error) {
	return _Settlementv2.Contract.SetMerchantActive(&_Settlementv2.TransactOpts, merchantId, active)
}

// Settle is a paid mutator transaction binding the contract method 0x1db7b6d8.
//
// Solidity: function settle((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote, bytes merchantSignature, bytes payerSignature) returns()
func (_Settlementv2 *Settlementv2Transactor) Settle(opts *bind.TransactOpts, quote PaymentTypesV2MerchantQuote, merchantSignature []byte, payerSignature []byte) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "settle", quote, merchantSignature, payerSignature)
}

// Settle is a paid mutator transaction binding the contract method 0x1db7b6d8.
//
// Solidity: function settle((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote, bytes merchantSignature, bytes payerSignature) returns()
func (_Settlementv2 *Settlementv2Session) Settle(quote PaymentTypesV2MerchantQuote, merchantSignature []byte, payerSignature []byte) (*types.Transaction, error) {
	return _Settlementv2.Contract.Settle(&_Settlementv2.TransactOpts, quote, merchantSignature, payerSignature)
}

// Settle is a paid mutator transaction binding the contract method 0x1db7b6d8.
//
// Solidity: function settle((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote, bytes merchantSignature, bytes payerSignature) returns()
func (_Settlementv2 *Settlementv2TransactorSession) Settle(quote PaymentTypesV2MerchantQuote, merchantSignature []byte, payerSignature []byte) (*types.Transaction, error) {
	return _Settlementv2.Contract.Settle(&_Settlementv2.TransactOpts, quote, merchantSignature, payerSignature)
}

// WithdrawEscrow is a paid mutator transaction binding the contract method 0x5decd717.
//
// Solidity: function withdrawEscrow(address payer, uint64 generation, uint256 amount) returns()
func (_Settlementv2 *Settlementv2Transactor) WithdrawEscrow(opts *bind.TransactOpts, payer common.Address, generation uint64, amount *big.Int) (*types.Transaction, error) {
	return _Settlementv2.contract.Transact(opts, "withdrawEscrow", payer, generation, amount)
}

// WithdrawEscrow is a paid mutator transaction binding the contract method 0x5decd717.
//
// Solidity: function withdrawEscrow(address payer, uint64 generation, uint256 amount) returns()
func (_Settlementv2 *Settlementv2Session) WithdrawEscrow(payer common.Address, generation uint64, amount *big.Int) (*types.Transaction, error) {
	return _Settlementv2.Contract.WithdrawEscrow(&_Settlementv2.TransactOpts, payer, generation, amount)
}

// WithdrawEscrow is a paid mutator transaction binding the contract method 0x5decd717.
//
// Solidity: function withdrawEscrow(address payer, uint64 generation, uint256 amount) returns()
func (_Settlementv2 *Settlementv2TransactorSession) WithdrawEscrow(payer common.Address, generation uint64, amount *big.Int) (*types.Transaction, error) {
	return _Settlementv2.Contract.WithdrawEscrow(&_Settlementv2.TransactOpts, payer, generation, amount)
}

// Settlementv2MerchantConfiguredIterator is returned from FilterMerchantConfigured and is used to iterate over the raw logs and unpacked data for MerchantConfigured events raised by the Settlementv2 contract.
type Settlementv2MerchantConfiguredIterator struct {
	Event *Settlementv2MerchantConfigured // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *Settlementv2MerchantConfiguredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Settlementv2MerchantConfigured)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(Settlementv2MerchantConfigured)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *Settlementv2MerchantConfiguredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Settlementv2MerchantConfiguredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Settlementv2MerchantConfigured represents a MerchantConfigured event raised by the Settlementv2 contract.
type Settlementv2MerchantConfigured struct {
	MerchantId         [32]byte
	KeyVersion         uint64
	BeneficiaryVersion uint64
	Raw                types.Log // Blockchain specific contextual infos
}

// FilterMerchantConfigured is a free log retrieval operation binding the contract event 0x01d10a80f364fe6d3708b06e77cbba86de1bcf6f26ac727a2c4a1781ba0f0e8a.
//
// Solidity: event MerchantConfigured(bytes32 indexed merchantId, uint64 keyVersion, uint64 beneficiaryVersion)
func (_Settlementv2 *Settlementv2Filterer) FilterMerchantConfigured(opts *bind.FilterOpts, merchantId [][32]byte) (*Settlementv2MerchantConfiguredIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}

	logs, sub, err := _Settlementv2.contract.FilterLogs(opts, "MerchantConfigured", merchantIdRule)
	if err != nil {
		return nil, err
	}
	return &Settlementv2MerchantConfiguredIterator{contract: _Settlementv2.contract, event: "MerchantConfigured", logs: logs, sub: sub}, nil
}

// WatchMerchantConfigured is a free log subscription operation binding the contract event 0x01d10a80f364fe6d3708b06e77cbba86de1bcf6f26ac727a2c4a1781ba0f0e8a.
//
// Solidity: event MerchantConfigured(bytes32 indexed merchantId, uint64 keyVersion, uint64 beneficiaryVersion)
func (_Settlementv2 *Settlementv2Filterer) WatchMerchantConfigured(opts *bind.WatchOpts, sink chan<- *Settlementv2MerchantConfigured, merchantId [][32]byte) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}

	logs, sub, err := _Settlementv2.contract.WatchLogs(opts, "MerchantConfigured", merchantIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Settlementv2MerchantConfigured)
				if err := _Settlementv2.contract.UnpackLog(event, "MerchantConfigured", log); err != nil {
					// If the signature doesn't match, skip this log.
					if errors.Is(err, bind.ErrEventSignatureMismatch) {
						continue
					}
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseMerchantConfigured is a log parse operation binding the contract event 0x01d10a80f364fe6d3708b06e77cbba86de1bcf6f26ac727a2c4a1781ba0f0e8a.
//
// Solidity: event MerchantConfigured(bytes32 indexed merchantId, uint64 keyVersion, uint64 beneficiaryVersion)
func (_Settlementv2 *Settlementv2Filterer) ParseMerchantConfigured(log types.Log) (*Settlementv2MerchantConfigured, error) {
	event := new(Settlementv2MerchantConfigured)
	if err := _Settlementv2.contract.UnpackLog(event, "MerchantConfigured", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Settlementv2MerchantKeyRevokedIterator is returned from FilterMerchantKeyRevoked and is used to iterate over the raw logs and unpacked data for MerchantKeyRevoked events raised by the Settlementv2 contract.
type Settlementv2MerchantKeyRevokedIterator struct {
	Event *Settlementv2MerchantKeyRevoked // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *Settlementv2MerchantKeyRevokedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Settlementv2MerchantKeyRevoked)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(Settlementv2MerchantKeyRevoked)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *Settlementv2MerchantKeyRevokedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Settlementv2MerchantKeyRevokedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Settlementv2MerchantKeyRevoked represents a MerchantKeyRevoked event raised by the Settlementv2 contract.
type Settlementv2MerchantKeyRevoked struct {
	MerchantId [32]byte
	KeyVersion uint64
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterMerchantKeyRevoked is a free log retrieval operation binding the contract event 0x597c17a7eadd057f67fed8a51abd34a2b14c7b7196b9cdaa51d6fbb9524017ce.
//
// Solidity: event MerchantKeyRevoked(bytes32 indexed merchantId, uint64 indexed keyVersion)
func (_Settlementv2 *Settlementv2Filterer) FilterMerchantKeyRevoked(opts *bind.FilterOpts, merchantId [][32]byte, keyVersion []uint64) (*Settlementv2MerchantKeyRevokedIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var keyVersionRule []interface{}
	for _, keyVersionItem := range keyVersion {
		keyVersionRule = append(keyVersionRule, keyVersionItem)
	}

	logs, sub, err := _Settlementv2.contract.FilterLogs(opts, "MerchantKeyRevoked", merchantIdRule, keyVersionRule)
	if err != nil {
		return nil, err
	}
	return &Settlementv2MerchantKeyRevokedIterator{contract: _Settlementv2.contract, event: "MerchantKeyRevoked", logs: logs, sub: sub}, nil
}

// WatchMerchantKeyRevoked is a free log subscription operation binding the contract event 0x597c17a7eadd057f67fed8a51abd34a2b14c7b7196b9cdaa51d6fbb9524017ce.
//
// Solidity: event MerchantKeyRevoked(bytes32 indexed merchantId, uint64 indexed keyVersion)
func (_Settlementv2 *Settlementv2Filterer) WatchMerchantKeyRevoked(opts *bind.WatchOpts, sink chan<- *Settlementv2MerchantKeyRevoked, merchantId [][32]byte, keyVersion []uint64) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var keyVersionRule []interface{}
	for _, keyVersionItem := range keyVersion {
		keyVersionRule = append(keyVersionRule, keyVersionItem)
	}

	logs, sub, err := _Settlementv2.contract.WatchLogs(opts, "MerchantKeyRevoked", merchantIdRule, keyVersionRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Settlementv2MerchantKeyRevoked)
				if err := _Settlementv2.contract.UnpackLog(event, "MerchantKeyRevoked", log); err != nil {
					// If the signature doesn't match, skip this log.
					if errors.Is(err, bind.ErrEventSignatureMismatch) {
						continue
					}
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseMerchantKeyRevoked is a log parse operation binding the contract event 0x597c17a7eadd057f67fed8a51abd34a2b14c7b7196b9cdaa51d6fbb9524017ce.
//
// Solidity: event MerchantKeyRevoked(bytes32 indexed merchantId, uint64 indexed keyVersion)
func (_Settlementv2 *Settlementv2Filterer) ParseMerchantKeyRevoked(log types.Log) (*Settlementv2MerchantKeyRevoked, error) {
	event := new(Settlementv2MerchantKeyRevoked)
	if err := _Settlementv2.contract.UnpackLog(event, "MerchantKeyRevoked", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Settlementv2PaidIterator is returned from FilterPaid and is used to iterate over the raw logs and unpacked data for Paid events raised by the Settlementv2 contract.
type Settlementv2PaidIterator struct {
	Event *Settlementv2Paid // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *Settlementv2PaidIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Settlementv2Paid)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(Settlementv2Paid)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *Settlementv2PaidIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Settlementv2PaidIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Settlementv2Paid represents a Paid event raised by the Settlementv2 contract.
type Settlementv2Paid struct {
	MerchantId      [32]byte
	OrderId         [32]byte
	Payer           common.Address
	Beneficiary     common.Address
	Asset           common.Address
	Amount          *big.Int
	Nonce           *big.Int
	QuoteDigest     [32]byte
	Mode            uint8
	PayerGeneration uint64
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterPaid is a free log retrieval operation binding the contract event 0x8ef858e4a5aa54b21ac8e7de48a10ab47b1192319da9540a960cc449fd68757f.
//
// Solidity: event Paid(bytes32 indexed merchantId, bytes32 indexed orderId, address indexed payer, address beneficiary, address asset, uint256 amount, uint256 nonce, bytes32 quoteDigest, uint8 mode, uint64 payerGeneration)
func (_Settlementv2 *Settlementv2Filterer) FilterPaid(opts *bind.FilterOpts, merchantId [][32]byte, orderId [][32]byte, payer []common.Address) (*Settlementv2PaidIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}
	var payerRule []interface{}
	for _, payerItem := range payer {
		payerRule = append(payerRule, payerItem)
	}

	logs, sub, err := _Settlementv2.contract.FilterLogs(opts, "Paid", merchantIdRule, orderIdRule, payerRule)
	if err != nil {
		return nil, err
	}
	return &Settlementv2PaidIterator{contract: _Settlementv2.contract, event: "Paid", logs: logs, sub: sub}, nil
}

// WatchPaid is a free log subscription operation binding the contract event 0x8ef858e4a5aa54b21ac8e7de48a10ab47b1192319da9540a960cc449fd68757f.
//
// Solidity: event Paid(bytes32 indexed merchantId, bytes32 indexed orderId, address indexed payer, address beneficiary, address asset, uint256 amount, uint256 nonce, bytes32 quoteDigest, uint8 mode, uint64 payerGeneration)
func (_Settlementv2 *Settlementv2Filterer) WatchPaid(opts *bind.WatchOpts, sink chan<- *Settlementv2Paid, merchantId [][32]byte, orderId [][32]byte, payer []common.Address) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}
	var payerRule []interface{}
	for _, payerItem := range payer {
		payerRule = append(payerRule, payerItem)
	}

	logs, sub, err := _Settlementv2.contract.WatchLogs(opts, "Paid", merchantIdRule, orderIdRule, payerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Settlementv2Paid)
				if err := _Settlementv2.contract.UnpackLog(event, "Paid", log); err != nil {
					// If the signature doesn't match, skip this log.
					if errors.Is(err, bind.ErrEventSignatureMismatch) {
						continue
					}
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParsePaid is a log parse operation binding the contract event 0x8ef858e4a5aa54b21ac8e7de48a10ab47b1192319da9540a960cc449fd68757f.
//
// Solidity: event Paid(bytes32 indexed merchantId, bytes32 indexed orderId, address indexed payer, address beneficiary, address asset, uint256 amount, uint256 nonce, bytes32 quoteDigest, uint8 mode, uint64 payerGeneration)
func (_Settlementv2 *Settlementv2Filterer) ParsePaid(log types.Log) (*Settlementv2Paid, error) {
	event := new(Settlementv2Paid)
	if err := _Settlementv2.contract.UnpackLog(event, "Paid", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Settlementv2RefundedIterator is returned from FilterRefunded and is used to iterate over the raw logs and unpacked data for Refunded events raised by the Settlementv2 contract.
type Settlementv2RefundedIterator struct {
	Event *Settlementv2Refunded // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *Settlementv2RefundedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Settlementv2Refunded)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(Settlementv2Refunded)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *Settlementv2RefundedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Settlementv2RefundedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Settlementv2Refunded represents a Refunded event raised by the Settlementv2 contract.
type Settlementv2Refunded struct {
	MerchantId [32]byte
	OrderId    [32]byte
	RefundId   [32]byte
	Payer      common.Address
	Asset      common.Address
	Amount     *big.Int
	Mode       uint8
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterRefunded is a free log retrieval operation binding the contract event 0x5c026d6872cb44d83c4429c3604d56938b71b245f403a2555d7d6ddcf993aef7.
//
// Solidity: event Refunded(bytes32 indexed merchantId, bytes32 indexed orderId, bytes32 indexed refundId, address payer, address asset, uint256 amount, uint8 mode)
func (_Settlementv2 *Settlementv2Filterer) FilterRefunded(opts *bind.FilterOpts, merchantId [][32]byte, orderId [][32]byte, refundId [][32]byte) (*Settlementv2RefundedIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}
	var refundIdRule []interface{}
	for _, refundIdItem := range refundId {
		refundIdRule = append(refundIdRule, refundIdItem)
	}

	logs, sub, err := _Settlementv2.contract.FilterLogs(opts, "Refunded", merchantIdRule, orderIdRule, refundIdRule)
	if err != nil {
		return nil, err
	}
	return &Settlementv2RefundedIterator{contract: _Settlementv2.contract, event: "Refunded", logs: logs, sub: sub}, nil
}

// WatchRefunded is a free log subscription operation binding the contract event 0x5c026d6872cb44d83c4429c3604d56938b71b245f403a2555d7d6ddcf993aef7.
//
// Solidity: event Refunded(bytes32 indexed merchantId, bytes32 indexed orderId, bytes32 indexed refundId, address payer, address asset, uint256 amount, uint8 mode)
func (_Settlementv2 *Settlementv2Filterer) WatchRefunded(opts *bind.WatchOpts, sink chan<- *Settlementv2Refunded, merchantId [][32]byte, orderId [][32]byte, refundId [][32]byte) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}
	var refundIdRule []interface{}
	for _, refundIdItem := range refundId {
		refundIdRule = append(refundIdRule, refundIdItem)
	}

	logs, sub, err := _Settlementv2.contract.WatchLogs(opts, "Refunded", merchantIdRule, orderIdRule, refundIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Settlementv2Refunded)
				if err := _Settlementv2.contract.UnpackLog(event, "Refunded", log); err != nil {
					// If the signature doesn't match, skip this log.
					if errors.Is(err, bind.ErrEventSignatureMismatch) {
						continue
					}
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRefunded is a log parse operation binding the contract event 0x5c026d6872cb44d83c4429c3604d56938b71b245f403a2555d7d6ddcf993aef7.
//
// Solidity: event Refunded(bytes32 indexed merchantId, bytes32 indexed orderId, bytes32 indexed refundId, address payer, address asset, uint256 amount, uint8 mode)
func (_Settlementv2 *Settlementv2Filterer) ParseRefunded(log types.Log) (*Settlementv2Refunded, error) {
	event := new(Settlementv2Refunded)
	if err := _Settlementv2.contract.UnpackLog(event, "Refunded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
