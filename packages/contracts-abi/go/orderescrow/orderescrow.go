// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package orderescrow

import (
	"errors"
	"math/big"
	"strings"

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

// OrderescrowMetaData contains all meta data concerning the Orderescrow contract.
var OrderescrowMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"asset_\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"governor_\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"arbiter_\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"CANCEL_TYPEHASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"CHAIN_ID\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"DELIVERY_TYPEHASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"DISPUTE_TYPEHASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"DISPUTE_WINDOW\",\"outputs\":[{\"internalType\":\"uint32\",\"name\":\"\",\"type\":\"uint32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"LOCK_TYPEHASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"MAX_BATCH\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"MAX_WINDOW\",\"outputs\":[{\"internalType\":\"uint32\",\"name\":\"\",\"type\":\"uint32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"MIN_WINDOW\",\"outputs\":[{\"internalType\":\"uint32\",\"name\":\"\",\"type\":\"uint32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"RECEIPT_TYPEHASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"WITHDRAW_DISPUTE_TYPEHASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"typehash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"actionDigest\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"arbiter\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"asset\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"typehash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"buyerDigest\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"signature\",\"type\":\"bytes\"}],\"name\":\"cancel\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"claim\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"bytes32[]\",\"name\":\"orderIds\",\"type\":\"bytes32[]\"}],\"name\":\"claimBatch\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"total\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"claimStatus\",\"outputs\":[{\"internalType\":\"enumOrderEscrow.SkipReason\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"signer\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"uint32\",\"name\":\"confirmWindow\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"deliveryWindow\",\"type\":\"uint32\"}],\"name\":\"configureMerchant\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"signature\",\"type\":\"bytes\"}],\"name\":\"dispute\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"domainSeparator\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"governor\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"quoteId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"tenantCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"branchCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"claimCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"intentCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"asset\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"merchantKeyVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"beneficiaryVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"payerGeneration\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"issuedAt\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"expiresAt\",\"type\":\"uint64\"},{\"internalType\":\"uint8\",\"name\":\"mode\",\"type\":\"uint8\"},{\"internalType\":\"uint8\",\"name\":\"payerRole\",\"type\":\"uint8\"}],\"internalType\":\"structPaymentTypesV2.MerchantQuote\",\"name\":\"quote\",\"type\":\"tuple\"},{\"internalType\":\"address\",\"name\":\"confirmer\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"merchantSignature\",\"type\":\"bytes\"},{\"internalType\":\"bytes\",\"name\":\"payerSignature\",\"type\":\"bytes\"}],\"name\":\"lock\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"quoteId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"tenantCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"branchCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"claimCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"intentCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"asset\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"merchantKeyVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"beneficiaryVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"payerGeneration\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"issuedAt\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"expiresAt\",\"type\":\"uint64\"},{\"internalType\":\"uint8\",\"name\":\"mode\",\"type\":\"uint8\"},{\"internalType\":\"uint8\",\"name\":\"payerRole\",\"type\":\"uint8\"}],\"internalType\":\"structPaymentTypesV2.MerchantQuote\",\"name\":\"quote\",\"type\":\"tuple\"},{\"internalType\":\"address\",\"name\":\"confirmer\",\"type\":\"address\"}],\"name\":\"lockNonce\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"signature\",\"type\":\"bytes\"}],\"name\":\"markDelivered\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"merchants\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"signer\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"uint64\",\"name\":\"keyVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"beneficiaryVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint32\",\"name\":\"confirmWindow\",\"type\":\"uint32\"},{\"internalType\":\"uint32\",\"name\":\"deliveryWindow\",\"type\":\"uint32\"},{\"internalType\":\"bool\",\"name\":\"active\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"orders\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"confirmer\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"deliverBy\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"claimableAt\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"resolveBy\",\"type\":\"uint64\"},{\"internalType\":\"uint32\",\"name\":\"confirmWindow\",\"type\":\"uint32\"},{\"internalType\":\"enumOrderEscrow.State\",\"name\":\"state\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"quoteId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"tenantCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"branchCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"claimCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"intentCommitment\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"asset\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"merchantKeyVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"beneficiaryVersion\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"payerGeneration\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"issuedAt\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"expiresAt\",\"type\":\"uint64\"},{\"internalType\":\"uint8\",\"name\":\"mode\",\"type\":\"uint8\"},{\"internalType\":\"uint8\",\"name\":\"payerRole\",\"type\":\"uint8\"}],\"internalType\":\"structPaymentTypesV2.MerchantQuote\",\"name\":\"quote\",\"type\":\"tuple\"}],\"name\":\"quoteDigest\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"refundExpiredDispute\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"refundUndelivered\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"signature\",\"type\":\"bytes\"}],\"name\":\"release\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bool\",\"name\":\"payMerchant\",\"type\":\"bool\"}],\"name\":\"resolve\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"next\",\"type\":\"address\"}],\"name\":\"setArbiter\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bool\",\"name\":\"active\",\"type\":\"bool\"}],\"name\":\"setMerchantActive\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"totalHeld\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"signature\",\"type\":\"bytes\"}],\"name\":\"withdrawDispute\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"arbiter\",\"type\":\"address\"}],\"name\":\"ArbiterChanged\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"enumOrderEscrow.SkipReason\",\"name\":\"reason\",\"type\":\"uint8\"}],\"name\":\"ClaimSkipped\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"active\",\"type\":\"bool\"}],\"name\":\"MerchantActive\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"signer\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"keyVersion\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"beneficiaryVersion\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint32\",\"name\":\"confirmWindow\",\"type\":\"uint32\"},{\"indexed\":false,\"internalType\":\"uint32\",\"name\":\"deliveryWindow\",\"type\":\"uint32\"}],\"name\":\"MerchantConfigured\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"enumOrderEscrow.Refund\",\"name\":\"reason\",\"type\":\"uint8\"}],\"name\":\"OrderCancelled\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"claimableAt\",\"type\":\"uint64\"}],\"name\":\"OrderDelivered\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"resolveBy\",\"type\":\"uint64\"}],\"name\":\"OrderDisputed\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"payer\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"confirmer\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"deliverBy\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint32\",\"name\":\"confirmWindow\",\"type\":\"uint32\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"quoteDigest\",\"type\":\"bytes32\"}],\"name\":\"OrderLocked\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"merchantId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"beneficiary\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"enumOrderEscrow.Payout\",\"name\":\"reason\",\"type\":\"uint8\"}],\"name\":\"OrderReleased\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"InvalidMerchant\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidParameters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidQuote\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidSignature\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidState\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidTime\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"OrderAlreadyPaid\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"Reentered\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"TokenBalanceMismatch\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"TokenTransferFailed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"Unauthorized\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WrongDomain\",\"type\":\"error\"}]",
}

// OrderescrowABI is the input ABI used to generate the binding from.
// Deprecated: Use OrderescrowMetaData.ABI instead.
var OrderescrowABI = OrderescrowMetaData.ABI

// Orderescrow is an auto generated Go binding around an Ethereum contract.
type Orderescrow struct {
	OrderescrowCaller     // Read-only binding to the contract
	OrderescrowTransactor // Write-only binding to the contract
	OrderescrowFilterer   // Log filterer for contract events
}

// OrderescrowCaller is an auto generated read-only Go binding around an Ethereum contract.
type OrderescrowCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OrderescrowTransactor is an auto generated write-only Go binding around an Ethereum contract.
type OrderescrowTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OrderescrowFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type OrderescrowFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OrderescrowSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type OrderescrowSession struct {
	Contract     *Orderescrow      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// OrderescrowCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type OrderescrowCallerSession struct {
	Contract *OrderescrowCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// OrderescrowTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type OrderescrowTransactorSession struct {
	Contract     *OrderescrowTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// OrderescrowRaw is an auto generated low-level Go binding around an Ethereum contract.
type OrderescrowRaw struct {
	Contract *Orderescrow // Generic contract binding to access the raw methods on
}

// OrderescrowCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type OrderescrowCallerRaw struct {
	Contract *OrderescrowCaller // Generic read-only contract binding to access the raw methods on
}

// OrderescrowTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type OrderescrowTransactorRaw struct {
	Contract *OrderescrowTransactor // Generic write-only contract binding to access the raw methods on
}

// NewOrderescrow creates a new instance of Orderescrow, bound to a specific deployed contract.
func NewOrderescrow(address common.Address, backend bind.ContractBackend) (*Orderescrow, error) {
	contract, err := bindOrderescrow(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Orderescrow{OrderescrowCaller: OrderescrowCaller{contract: contract}, OrderescrowTransactor: OrderescrowTransactor{contract: contract}, OrderescrowFilterer: OrderescrowFilterer{contract: contract}}, nil
}

// NewOrderescrowCaller creates a new read-only instance of Orderescrow, bound to a specific deployed contract.
func NewOrderescrowCaller(address common.Address, caller bind.ContractCaller) (*OrderescrowCaller, error) {
	contract, err := bindOrderescrow(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &OrderescrowCaller{contract: contract}, nil
}

// NewOrderescrowTransactor creates a new write-only instance of Orderescrow, bound to a specific deployed contract.
func NewOrderescrowTransactor(address common.Address, transactor bind.ContractTransactor) (*OrderescrowTransactor, error) {
	contract, err := bindOrderescrow(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &OrderescrowTransactor{contract: contract}, nil
}

// NewOrderescrowFilterer creates a new log filterer instance of Orderescrow, bound to a specific deployed contract.
func NewOrderescrowFilterer(address common.Address, filterer bind.ContractFilterer) (*OrderescrowFilterer, error) {
	contract, err := bindOrderescrow(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &OrderescrowFilterer{contract: contract}, nil
}

// bindOrderescrow binds a generic wrapper to an already deployed contract.
func bindOrderescrow(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := OrderescrowMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Orderescrow *OrderescrowRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Orderescrow.Contract.OrderescrowCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Orderescrow *OrderescrowRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Orderescrow.Contract.OrderescrowTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Orderescrow *OrderescrowRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Orderescrow.Contract.OrderescrowTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Orderescrow *OrderescrowCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Orderescrow.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Orderescrow *OrderescrowTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Orderescrow.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Orderescrow *OrderescrowTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Orderescrow.Contract.contract.Transact(opts, method, params...)
}

// CANCELTYPEHASH is a free data retrieval call binding the contract method 0x73fca6ea.
//
// Solidity: function CANCEL_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) CANCELTYPEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "CANCEL_TYPEHASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// CANCELTYPEHASH is a free data retrieval call binding the contract method 0x73fca6ea.
//
// Solidity: function CANCEL_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowSession) CANCELTYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.CANCELTYPEHASH(&_Orderescrow.CallOpts)
}

// CANCELTYPEHASH is a free data retrieval call binding the contract method 0x73fca6ea.
//
// Solidity: function CANCEL_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) CANCELTYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.CANCELTYPEHASH(&_Orderescrow.CallOpts)
}

// CHAINID is a free data retrieval call binding the contract method 0x85e1f4d0.
//
// Solidity: function CHAIN_ID() view returns(uint256)
func (_Orderescrow *OrderescrowCaller) CHAINID(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "CHAIN_ID")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// CHAINID is a free data retrieval call binding the contract method 0x85e1f4d0.
//
// Solidity: function CHAIN_ID() view returns(uint256)
func (_Orderescrow *OrderescrowSession) CHAINID() (*big.Int, error) {
	return _Orderescrow.Contract.CHAINID(&_Orderescrow.CallOpts)
}

// CHAINID is a free data retrieval call binding the contract method 0x85e1f4d0.
//
// Solidity: function CHAIN_ID() view returns(uint256)
func (_Orderescrow *OrderescrowCallerSession) CHAINID() (*big.Int, error) {
	return _Orderescrow.Contract.CHAINID(&_Orderescrow.CallOpts)
}

// DELIVERYTYPEHASH is a free data retrieval call binding the contract method 0x7d1d8323.
//
// Solidity: function DELIVERY_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) DELIVERYTYPEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "DELIVERY_TYPEHASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// DELIVERYTYPEHASH is a free data retrieval call binding the contract method 0x7d1d8323.
//
// Solidity: function DELIVERY_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowSession) DELIVERYTYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.DELIVERYTYPEHASH(&_Orderescrow.CallOpts)
}

// DELIVERYTYPEHASH is a free data retrieval call binding the contract method 0x7d1d8323.
//
// Solidity: function DELIVERY_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) DELIVERYTYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.DELIVERYTYPEHASH(&_Orderescrow.CallOpts)
}

// DISPUTETYPEHASH is a free data retrieval call binding the contract method 0x284d2d68.
//
// Solidity: function DISPUTE_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) DISPUTETYPEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "DISPUTE_TYPEHASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// DISPUTETYPEHASH is a free data retrieval call binding the contract method 0x284d2d68.
//
// Solidity: function DISPUTE_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowSession) DISPUTETYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.DISPUTETYPEHASH(&_Orderescrow.CallOpts)
}

// DISPUTETYPEHASH is a free data retrieval call binding the contract method 0x284d2d68.
//
// Solidity: function DISPUTE_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) DISPUTETYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.DISPUTETYPEHASH(&_Orderescrow.CallOpts)
}

// DISPUTEWINDOW is a free data retrieval call binding the contract method 0xf585dc57.
//
// Solidity: function DISPUTE_WINDOW() view returns(uint32)
func (_Orderescrow *OrderescrowCaller) DISPUTEWINDOW(opts *bind.CallOpts) (uint32, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "DISPUTE_WINDOW")

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// DISPUTEWINDOW is a free data retrieval call binding the contract method 0xf585dc57.
//
// Solidity: function DISPUTE_WINDOW() view returns(uint32)
func (_Orderescrow *OrderescrowSession) DISPUTEWINDOW() (uint32, error) {
	return _Orderescrow.Contract.DISPUTEWINDOW(&_Orderescrow.CallOpts)
}

// DISPUTEWINDOW is a free data retrieval call binding the contract method 0xf585dc57.
//
// Solidity: function DISPUTE_WINDOW() view returns(uint32)
func (_Orderescrow *OrderescrowCallerSession) DISPUTEWINDOW() (uint32, error) {
	return _Orderescrow.Contract.DISPUTEWINDOW(&_Orderescrow.CallOpts)
}

// LOCKTYPEHASH is a free data retrieval call binding the contract method 0xb2924620.
//
// Solidity: function LOCK_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) LOCKTYPEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "LOCK_TYPEHASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// LOCKTYPEHASH is a free data retrieval call binding the contract method 0xb2924620.
//
// Solidity: function LOCK_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowSession) LOCKTYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.LOCKTYPEHASH(&_Orderescrow.CallOpts)
}

// LOCKTYPEHASH is a free data retrieval call binding the contract method 0xb2924620.
//
// Solidity: function LOCK_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) LOCKTYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.LOCKTYPEHASH(&_Orderescrow.CallOpts)
}

// MAXBATCH is a free data retrieval call binding the contract method 0x950bff9f.
//
// Solidity: function MAX_BATCH() view returns(uint256)
func (_Orderescrow *OrderescrowCaller) MAXBATCH(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "MAX_BATCH")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MAXBATCH is a free data retrieval call binding the contract method 0x950bff9f.
//
// Solidity: function MAX_BATCH() view returns(uint256)
func (_Orderescrow *OrderescrowSession) MAXBATCH() (*big.Int, error) {
	return _Orderescrow.Contract.MAXBATCH(&_Orderescrow.CallOpts)
}

// MAXBATCH is a free data retrieval call binding the contract method 0x950bff9f.
//
// Solidity: function MAX_BATCH() view returns(uint256)
func (_Orderescrow *OrderescrowCallerSession) MAXBATCH() (*big.Int, error) {
	return _Orderescrow.Contract.MAXBATCH(&_Orderescrow.CallOpts)
}

// MAXWINDOW is a free data retrieval call binding the contract method 0xdeb96634.
//
// Solidity: function MAX_WINDOW() view returns(uint32)
func (_Orderescrow *OrderescrowCaller) MAXWINDOW(opts *bind.CallOpts) (uint32, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "MAX_WINDOW")

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// MAXWINDOW is a free data retrieval call binding the contract method 0xdeb96634.
//
// Solidity: function MAX_WINDOW() view returns(uint32)
func (_Orderescrow *OrderescrowSession) MAXWINDOW() (uint32, error) {
	return _Orderescrow.Contract.MAXWINDOW(&_Orderescrow.CallOpts)
}

// MAXWINDOW is a free data retrieval call binding the contract method 0xdeb96634.
//
// Solidity: function MAX_WINDOW() view returns(uint32)
func (_Orderescrow *OrderescrowCallerSession) MAXWINDOW() (uint32, error) {
	return _Orderescrow.Contract.MAXWINDOW(&_Orderescrow.CallOpts)
}

// MINWINDOW is a free data retrieval call binding the contract method 0x52b65c4b.
//
// Solidity: function MIN_WINDOW() view returns(uint32)
func (_Orderescrow *OrderescrowCaller) MINWINDOW(opts *bind.CallOpts) (uint32, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "MIN_WINDOW")

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// MINWINDOW is a free data retrieval call binding the contract method 0x52b65c4b.
//
// Solidity: function MIN_WINDOW() view returns(uint32)
func (_Orderescrow *OrderescrowSession) MINWINDOW() (uint32, error) {
	return _Orderescrow.Contract.MINWINDOW(&_Orderescrow.CallOpts)
}

// MINWINDOW is a free data retrieval call binding the contract method 0x52b65c4b.
//
// Solidity: function MIN_WINDOW() view returns(uint32)
func (_Orderescrow *OrderescrowCallerSession) MINWINDOW() (uint32, error) {
	return _Orderescrow.Contract.MINWINDOW(&_Orderescrow.CallOpts)
}

// RECEIPTTYPEHASH is a free data retrieval call binding the contract method 0x95850f59.
//
// Solidity: function RECEIPT_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) RECEIPTTYPEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "RECEIPT_TYPEHASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// RECEIPTTYPEHASH is a free data retrieval call binding the contract method 0x95850f59.
//
// Solidity: function RECEIPT_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowSession) RECEIPTTYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.RECEIPTTYPEHASH(&_Orderescrow.CallOpts)
}

// RECEIPTTYPEHASH is a free data retrieval call binding the contract method 0x95850f59.
//
// Solidity: function RECEIPT_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) RECEIPTTYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.RECEIPTTYPEHASH(&_Orderescrow.CallOpts)
}

// WITHDRAWDISPUTETYPEHASH is a free data retrieval call binding the contract method 0xebd01856.
//
// Solidity: function WITHDRAW_DISPUTE_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) WITHDRAWDISPUTETYPEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "WITHDRAW_DISPUTE_TYPEHASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// WITHDRAWDISPUTETYPEHASH is a free data retrieval call binding the contract method 0xebd01856.
//
// Solidity: function WITHDRAW_DISPUTE_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowSession) WITHDRAWDISPUTETYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.WITHDRAWDISPUTETYPEHASH(&_Orderescrow.CallOpts)
}

// WITHDRAWDISPUTETYPEHASH is a free data retrieval call binding the contract method 0xebd01856.
//
// Solidity: function WITHDRAW_DISPUTE_TYPEHASH() view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) WITHDRAWDISPUTETYPEHASH() ([32]byte, error) {
	return _Orderescrow.Contract.WITHDRAWDISPUTETYPEHASH(&_Orderescrow.CallOpts)
}

// ActionDigest is a free data retrieval call binding the contract method 0x8c02c250.
//
// Solidity: function actionDigest(bytes32 typehash, bytes32 merchantId, bytes32 orderId) view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) ActionDigest(opts *bind.CallOpts, typehash [32]byte, merchantId [32]byte, orderId [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "actionDigest", typehash, merchantId, orderId)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ActionDigest is a free data retrieval call binding the contract method 0x8c02c250.
//
// Solidity: function actionDigest(bytes32 typehash, bytes32 merchantId, bytes32 orderId) view returns(bytes32)
func (_Orderescrow *OrderescrowSession) ActionDigest(typehash [32]byte, merchantId [32]byte, orderId [32]byte) ([32]byte, error) {
	return _Orderescrow.Contract.ActionDigest(&_Orderescrow.CallOpts, typehash, merchantId, orderId)
}

// ActionDigest is a free data retrieval call binding the contract method 0x8c02c250.
//
// Solidity: function actionDigest(bytes32 typehash, bytes32 merchantId, bytes32 orderId) view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) ActionDigest(typehash [32]byte, merchantId [32]byte, orderId [32]byte) ([32]byte, error) {
	return _Orderescrow.Contract.ActionDigest(&_Orderescrow.CallOpts, typehash, merchantId, orderId)
}

// Arbiter is a free data retrieval call binding the contract method 0xfe25e00a.
//
// Solidity: function arbiter() view returns(address)
func (_Orderescrow *OrderescrowCaller) Arbiter(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "arbiter")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Arbiter is a free data retrieval call binding the contract method 0xfe25e00a.
//
// Solidity: function arbiter() view returns(address)
func (_Orderescrow *OrderescrowSession) Arbiter() (common.Address, error) {
	return _Orderescrow.Contract.Arbiter(&_Orderescrow.CallOpts)
}

// Arbiter is a free data retrieval call binding the contract method 0xfe25e00a.
//
// Solidity: function arbiter() view returns(address)
func (_Orderescrow *OrderescrowCallerSession) Arbiter() (common.Address, error) {
	return _Orderescrow.Contract.Arbiter(&_Orderescrow.CallOpts)
}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address)
func (_Orderescrow *OrderescrowCaller) Asset(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "asset")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address)
func (_Orderescrow *OrderescrowSession) Asset() (common.Address, error) {
	return _Orderescrow.Contract.Asset(&_Orderescrow.CallOpts)
}

// Asset is a free data retrieval call binding the contract method 0x38d52e0f.
//
// Solidity: function asset() view returns(address)
func (_Orderescrow *OrderescrowCallerSession) Asset() (common.Address, error) {
	return _Orderescrow.Contract.Asset(&_Orderescrow.CallOpts)
}

// BuyerDigest is a free data retrieval call binding the contract method 0x884a735f.
//
// Solidity: function buyerDigest(bytes32 typehash, bytes32 merchantId, bytes32 orderId) view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) BuyerDigest(opts *bind.CallOpts, typehash [32]byte, merchantId [32]byte, orderId [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "buyerDigest", typehash, merchantId, orderId)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// BuyerDigest is a free data retrieval call binding the contract method 0x884a735f.
//
// Solidity: function buyerDigest(bytes32 typehash, bytes32 merchantId, bytes32 orderId) view returns(bytes32)
func (_Orderescrow *OrderescrowSession) BuyerDigest(typehash [32]byte, merchantId [32]byte, orderId [32]byte) ([32]byte, error) {
	return _Orderescrow.Contract.BuyerDigest(&_Orderescrow.CallOpts, typehash, merchantId, orderId)
}

// BuyerDigest is a free data retrieval call binding the contract method 0x884a735f.
//
// Solidity: function buyerDigest(bytes32 typehash, bytes32 merchantId, bytes32 orderId) view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) BuyerDigest(typehash [32]byte, merchantId [32]byte, orderId [32]byte) ([32]byte, error) {
	return _Orderescrow.Contract.BuyerDigest(&_Orderescrow.CallOpts, typehash, merchantId, orderId)
}

// ClaimStatus is a free data retrieval call binding the contract method 0x9e812993.
//
// Solidity: function claimStatus(bytes32 merchantId, bytes32 orderId) view returns(uint8)
func (_Orderescrow *OrderescrowCaller) ClaimStatus(opts *bind.CallOpts, merchantId [32]byte, orderId [32]byte) (uint8, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "claimStatus", merchantId, orderId)

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// ClaimStatus is a free data retrieval call binding the contract method 0x9e812993.
//
// Solidity: function claimStatus(bytes32 merchantId, bytes32 orderId) view returns(uint8)
func (_Orderescrow *OrderescrowSession) ClaimStatus(merchantId [32]byte, orderId [32]byte) (uint8, error) {
	return _Orderescrow.Contract.ClaimStatus(&_Orderescrow.CallOpts, merchantId, orderId)
}

// ClaimStatus is a free data retrieval call binding the contract method 0x9e812993.
//
// Solidity: function claimStatus(bytes32 merchantId, bytes32 orderId) view returns(uint8)
func (_Orderescrow *OrderescrowCallerSession) ClaimStatus(merchantId [32]byte, orderId [32]byte) (uint8, error) {
	return _Orderescrow.Contract.ClaimStatus(&_Orderescrow.CallOpts, merchantId, orderId)
}

// DomainSeparator is a free data retrieval call binding the contract method 0xf698da25.
//
// Solidity: function domainSeparator() view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) DomainSeparator(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "domainSeparator")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// DomainSeparator is a free data retrieval call binding the contract method 0xf698da25.
//
// Solidity: function domainSeparator() view returns(bytes32)
func (_Orderescrow *OrderescrowSession) DomainSeparator() ([32]byte, error) {
	return _Orderescrow.Contract.DomainSeparator(&_Orderescrow.CallOpts)
}

// DomainSeparator is a free data retrieval call binding the contract method 0xf698da25.
//
// Solidity: function domainSeparator() view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) DomainSeparator() ([32]byte, error) {
	return _Orderescrow.Contract.DomainSeparator(&_Orderescrow.CallOpts)
}

// Governor is a free data retrieval call binding the contract method 0x0c340a24.
//
// Solidity: function governor() view returns(address)
func (_Orderescrow *OrderescrowCaller) Governor(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "governor")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Governor is a free data retrieval call binding the contract method 0x0c340a24.
//
// Solidity: function governor() view returns(address)
func (_Orderescrow *OrderescrowSession) Governor() (common.Address, error) {
	return _Orderescrow.Contract.Governor(&_Orderescrow.CallOpts)
}

// Governor is a free data retrieval call binding the contract method 0x0c340a24.
//
// Solidity: function governor() view returns(address)
func (_Orderescrow *OrderescrowCallerSession) Governor() (common.Address, error) {
	return _Orderescrow.Contract.Governor(&_Orderescrow.CallOpts)
}

// LockNonce is a free data retrieval call binding the contract method 0x6eb33e4f.
//
// Solidity: function lockNonce((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote, address confirmer) view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) LockNonce(opts *bind.CallOpts, quote PaymentTypesV2MerchantQuote, confirmer common.Address) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "lockNonce", quote, confirmer)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// LockNonce is a free data retrieval call binding the contract method 0x6eb33e4f.
//
// Solidity: function lockNonce((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote, address confirmer) view returns(bytes32)
func (_Orderescrow *OrderescrowSession) LockNonce(quote PaymentTypesV2MerchantQuote, confirmer common.Address) ([32]byte, error) {
	return _Orderescrow.Contract.LockNonce(&_Orderescrow.CallOpts, quote, confirmer)
}

// LockNonce is a free data retrieval call binding the contract method 0x6eb33e4f.
//
// Solidity: function lockNonce((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote, address confirmer) view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) LockNonce(quote PaymentTypesV2MerchantQuote, confirmer common.Address) ([32]byte, error) {
	return _Orderescrow.Contract.LockNonce(&_Orderescrow.CallOpts, quote, confirmer)
}

// Merchants is a free data retrieval call binding the contract method 0x20003022.
//
// Solidity: function merchants(bytes32 ) view returns(address signer, address beneficiary, uint64 keyVersion, uint64 beneficiaryVersion, uint32 confirmWindow, uint32 deliveryWindow, bool active)
func (_Orderescrow *OrderescrowCaller) Merchants(opts *bind.CallOpts, arg0 [32]byte) (struct {
	Signer             common.Address
	Beneficiary        common.Address
	KeyVersion         uint64
	BeneficiaryVersion uint64
	ConfirmWindow      uint32
	DeliveryWindow     uint32
	Active             bool
}, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "merchants", arg0)

	outstruct := new(struct {
		Signer             common.Address
		Beneficiary        common.Address
		KeyVersion         uint64
		BeneficiaryVersion uint64
		ConfirmWindow      uint32
		DeliveryWindow     uint32
		Active             bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Signer = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.Beneficiary = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)
	outstruct.KeyVersion = *abi.ConvertType(out[2], new(uint64)).(*uint64)
	outstruct.BeneficiaryVersion = *abi.ConvertType(out[3], new(uint64)).(*uint64)
	outstruct.ConfirmWindow = *abi.ConvertType(out[4], new(uint32)).(*uint32)
	outstruct.DeliveryWindow = *abi.ConvertType(out[5], new(uint32)).(*uint32)
	outstruct.Active = *abi.ConvertType(out[6], new(bool)).(*bool)

	return *outstruct, err

}

// Merchants is a free data retrieval call binding the contract method 0x20003022.
//
// Solidity: function merchants(bytes32 ) view returns(address signer, address beneficiary, uint64 keyVersion, uint64 beneficiaryVersion, uint32 confirmWindow, uint32 deliveryWindow, bool active)
func (_Orderescrow *OrderescrowSession) Merchants(arg0 [32]byte) (struct {
	Signer             common.Address
	Beneficiary        common.Address
	KeyVersion         uint64
	BeneficiaryVersion uint64
	ConfirmWindow      uint32
	DeliveryWindow     uint32
	Active             bool
}, error) {
	return _Orderescrow.Contract.Merchants(&_Orderescrow.CallOpts, arg0)
}

// Merchants is a free data retrieval call binding the contract method 0x20003022.
//
// Solidity: function merchants(bytes32 ) view returns(address signer, address beneficiary, uint64 keyVersion, uint64 beneficiaryVersion, uint32 confirmWindow, uint32 deliveryWindow, bool active)
func (_Orderescrow *OrderescrowCallerSession) Merchants(arg0 [32]byte) (struct {
	Signer             common.Address
	Beneficiary        common.Address
	KeyVersion         uint64
	BeneficiaryVersion uint64
	ConfirmWindow      uint32
	DeliveryWindow     uint32
	Active             bool
}, error) {
	return _Orderescrow.Contract.Merchants(&_Orderescrow.CallOpts, arg0)
}

// Orders is a free data retrieval call binding the contract method 0x894b127f.
//
// Solidity: function orders(bytes32 , bytes32 ) view returns(address payer, address beneficiary, address confirmer, uint256 amount, uint64 deliverBy, uint64 claimableAt, uint64 resolveBy, uint32 confirmWindow, uint8 state)
func (_Orderescrow *OrderescrowCaller) Orders(opts *bind.CallOpts, arg0 [32]byte, arg1 [32]byte) (struct {
	Payer         common.Address
	Beneficiary   common.Address
	Confirmer     common.Address
	Amount        *big.Int
	DeliverBy     uint64
	ClaimableAt   uint64
	ResolveBy     uint64
	ConfirmWindow uint32
	State         uint8
}, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "orders", arg0, arg1)

	outstruct := new(struct {
		Payer         common.Address
		Beneficiary   common.Address
		Confirmer     common.Address
		Amount        *big.Int
		DeliverBy     uint64
		ClaimableAt   uint64
		ResolveBy     uint64
		ConfirmWindow uint32
		State         uint8
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Payer = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.Beneficiary = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)
	outstruct.Confirmer = *abi.ConvertType(out[2], new(common.Address)).(*common.Address)
	outstruct.Amount = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.DeliverBy = *abi.ConvertType(out[4], new(uint64)).(*uint64)
	outstruct.ClaimableAt = *abi.ConvertType(out[5], new(uint64)).(*uint64)
	outstruct.ResolveBy = *abi.ConvertType(out[6], new(uint64)).(*uint64)
	outstruct.ConfirmWindow = *abi.ConvertType(out[7], new(uint32)).(*uint32)
	outstruct.State = *abi.ConvertType(out[8], new(uint8)).(*uint8)

	return *outstruct, err

}

// Orders is a free data retrieval call binding the contract method 0x894b127f.
//
// Solidity: function orders(bytes32 , bytes32 ) view returns(address payer, address beneficiary, address confirmer, uint256 amount, uint64 deliverBy, uint64 claimableAt, uint64 resolveBy, uint32 confirmWindow, uint8 state)
func (_Orderescrow *OrderescrowSession) Orders(arg0 [32]byte, arg1 [32]byte) (struct {
	Payer         common.Address
	Beneficiary   common.Address
	Confirmer     common.Address
	Amount        *big.Int
	DeliverBy     uint64
	ClaimableAt   uint64
	ResolveBy     uint64
	ConfirmWindow uint32
	State         uint8
}, error) {
	return _Orderescrow.Contract.Orders(&_Orderescrow.CallOpts, arg0, arg1)
}

// Orders is a free data retrieval call binding the contract method 0x894b127f.
//
// Solidity: function orders(bytes32 , bytes32 ) view returns(address payer, address beneficiary, address confirmer, uint256 amount, uint64 deliverBy, uint64 claimableAt, uint64 resolveBy, uint32 confirmWindow, uint8 state)
func (_Orderescrow *OrderescrowCallerSession) Orders(arg0 [32]byte, arg1 [32]byte) (struct {
	Payer         common.Address
	Beneficiary   common.Address
	Confirmer     common.Address
	Amount        *big.Int
	DeliverBy     uint64
	ClaimableAt   uint64
	ResolveBy     uint64
	ConfirmWindow uint32
	State         uint8
}, error) {
	return _Orderescrow.Contract.Orders(&_Orderescrow.CallOpts, arg0, arg1)
}

// QuoteDigest is a free data retrieval call binding the contract method 0xbb3fa477.
//
// Solidity: function quoteDigest((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote) view returns(bytes32)
func (_Orderescrow *OrderescrowCaller) QuoteDigest(opts *bind.CallOpts, quote PaymentTypesV2MerchantQuote) ([32]byte, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "quoteDigest", quote)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// QuoteDigest is a free data retrieval call binding the contract method 0xbb3fa477.
//
// Solidity: function quoteDigest((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote) view returns(bytes32)
func (_Orderescrow *OrderescrowSession) QuoteDigest(quote PaymentTypesV2MerchantQuote) ([32]byte, error) {
	return _Orderescrow.Contract.QuoteDigest(&_Orderescrow.CallOpts, quote)
}

// QuoteDigest is a free data retrieval call binding the contract method 0xbb3fa477.
//
// Solidity: function quoteDigest((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote) view returns(bytes32)
func (_Orderescrow *OrderescrowCallerSession) QuoteDigest(quote PaymentTypesV2MerchantQuote) ([32]byte, error) {
	return _Orderescrow.Contract.QuoteDigest(&_Orderescrow.CallOpts, quote)
}

// TotalHeld is a free data retrieval call binding the contract method 0xf43cc773.
//
// Solidity: function totalHeld() view returns(uint256)
func (_Orderescrow *OrderescrowCaller) TotalHeld(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Orderescrow.contract.Call(opts, &out, "totalHeld")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalHeld is a free data retrieval call binding the contract method 0xf43cc773.
//
// Solidity: function totalHeld() view returns(uint256)
func (_Orderescrow *OrderescrowSession) TotalHeld() (*big.Int, error) {
	return _Orderescrow.Contract.TotalHeld(&_Orderescrow.CallOpts)
}

// TotalHeld is a free data retrieval call binding the contract method 0xf43cc773.
//
// Solidity: function totalHeld() view returns(uint256)
func (_Orderescrow *OrderescrowCallerSession) TotalHeld() (*big.Int, error) {
	return _Orderescrow.Contract.TotalHeld(&_Orderescrow.CallOpts)
}

// Cancel is a paid mutator transaction binding the contract method 0x184890b4.
//
// Solidity: function cancel(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactor) Cancel(opts *bind.TransactOpts, merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "cancel", merchantId, orderId, signature)
}

// Cancel is a paid mutator transaction binding the contract method 0x184890b4.
//
// Solidity: function cancel(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowSession) Cancel(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Cancel(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// Cancel is a paid mutator transaction binding the contract method 0x184890b4.
//
// Solidity: function cancel(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactorSession) Cancel(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Cancel(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// Claim is a paid mutator transaction binding the contract method 0x84cc9dfb.
//
// Solidity: function claim(bytes32 merchantId, bytes32 orderId) returns()
func (_Orderescrow *OrderescrowTransactor) Claim(opts *bind.TransactOpts, merchantId [32]byte, orderId [32]byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "claim", merchantId, orderId)
}

// Claim is a paid mutator transaction binding the contract method 0x84cc9dfb.
//
// Solidity: function claim(bytes32 merchantId, bytes32 orderId) returns()
func (_Orderescrow *OrderescrowSession) Claim(merchantId [32]byte, orderId [32]byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Claim(&_Orderescrow.TransactOpts, merchantId, orderId)
}

// Claim is a paid mutator transaction binding the contract method 0x84cc9dfb.
//
// Solidity: function claim(bytes32 merchantId, bytes32 orderId) returns()
func (_Orderescrow *OrderescrowTransactorSession) Claim(merchantId [32]byte, orderId [32]byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Claim(&_Orderescrow.TransactOpts, merchantId, orderId)
}

// ClaimBatch is a paid mutator transaction binding the contract method 0xe93e4c3d.
//
// Solidity: function claimBatch(bytes32 merchantId, address beneficiary, bytes32[] orderIds) returns(uint256 total)
func (_Orderescrow *OrderescrowTransactor) ClaimBatch(opts *bind.TransactOpts, merchantId [32]byte, beneficiary common.Address, orderIds [][32]byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "claimBatch", merchantId, beneficiary, orderIds)
}

// ClaimBatch is a paid mutator transaction binding the contract method 0xe93e4c3d.
//
// Solidity: function claimBatch(bytes32 merchantId, address beneficiary, bytes32[] orderIds) returns(uint256 total)
func (_Orderescrow *OrderescrowSession) ClaimBatch(merchantId [32]byte, beneficiary common.Address, orderIds [][32]byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.ClaimBatch(&_Orderescrow.TransactOpts, merchantId, beneficiary, orderIds)
}

// ClaimBatch is a paid mutator transaction binding the contract method 0xe93e4c3d.
//
// Solidity: function claimBatch(bytes32 merchantId, address beneficiary, bytes32[] orderIds) returns(uint256 total)
func (_Orderescrow *OrderescrowTransactorSession) ClaimBatch(merchantId [32]byte, beneficiary common.Address, orderIds [][32]byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.ClaimBatch(&_Orderescrow.TransactOpts, merchantId, beneficiary, orderIds)
}

// ConfigureMerchant is a paid mutator transaction binding the contract method 0x8c9dd892.
//
// Solidity: function configureMerchant(bytes32 merchantId, address signer, address beneficiary, uint32 confirmWindow, uint32 deliveryWindow) returns()
func (_Orderescrow *OrderescrowTransactor) ConfigureMerchant(opts *bind.TransactOpts, merchantId [32]byte, signer common.Address, beneficiary common.Address, confirmWindow uint32, deliveryWindow uint32) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "configureMerchant", merchantId, signer, beneficiary, confirmWindow, deliveryWindow)
}

// ConfigureMerchant is a paid mutator transaction binding the contract method 0x8c9dd892.
//
// Solidity: function configureMerchant(bytes32 merchantId, address signer, address beneficiary, uint32 confirmWindow, uint32 deliveryWindow) returns()
func (_Orderescrow *OrderescrowSession) ConfigureMerchant(merchantId [32]byte, signer common.Address, beneficiary common.Address, confirmWindow uint32, deliveryWindow uint32) (*types.Transaction, error) {
	return _Orderescrow.Contract.ConfigureMerchant(&_Orderescrow.TransactOpts, merchantId, signer, beneficiary, confirmWindow, deliveryWindow)
}

// ConfigureMerchant is a paid mutator transaction binding the contract method 0x8c9dd892.
//
// Solidity: function configureMerchant(bytes32 merchantId, address signer, address beneficiary, uint32 confirmWindow, uint32 deliveryWindow) returns()
func (_Orderescrow *OrderescrowTransactorSession) ConfigureMerchant(merchantId [32]byte, signer common.Address, beneficiary common.Address, confirmWindow uint32, deliveryWindow uint32) (*types.Transaction, error) {
	return _Orderescrow.Contract.ConfigureMerchant(&_Orderescrow.TransactOpts, merchantId, signer, beneficiary, confirmWindow, deliveryWindow)
}

// Dispute is a paid mutator transaction binding the contract method 0xe890ca8a.
//
// Solidity: function dispute(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactor) Dispute(opts *bind.TransactOpts, merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "dispute", merchantId, orderId, signature)
}

// Dispute is a paid mutator transaction binding the contract method 0xe890ca8a.
//
// Solidity: function dispute(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowSession) Dispute(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Dispute(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// Dispute is a paid mutator transaction binding the contract method 0xe890ca8a.
//
// Solidity: function dispute(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactorSession) Dispute(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Dispute(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// Lock is a paid mutator transaction binding the contract method 0x8e1d68f0.
//
// Solidity: function lock((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote, address confirmer, bytes merchantSignature, bytes payerSignature) returns()
func (_Orderescrow *OrderescrowTransactor) Lock(opts *bind.TransactOpts, quote PaymentTypesV2MerchantQuote, confirmer common.Address, merchantSignature []byte, payerSignature []byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "lock", quote, confirmer, merchantSignature, payerSignature)
}

// Lock is a paid mutator transaction binding the contract method 0x8e1d68f0.
//
// Solidity: function lock((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote, address confirmer, bytes merchantSignature, bytes payerSignature) returns()
func (_Orderescrow *OrderescrowSession) Lock(quote PaymentTypesV2MerchantQuote, confirmer common.Address, merchantSignature []byte, payerSignature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Lock(&_Orderescrow.TransactOpts, quote, confirmer, merchantSignature, payerSignature)
}

// Lock is a paid mutator transaction binding the contract method 0x8e1d68f0.
//
// Solidity: function lock((bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,bytes32,address,address,address,uint256,uint256,uint64,uint64,uint64,uint64,uint64,uint8,uint8) quote, address confirmer, bytes merchantSignature, bytes payerSignature) returns()
func (_Orderescrow *OrderescrowTransactorSession) Lock(quote PaymentTypesV2MerchantQuote, confirmer common.Address, merchantSignature []byte, payerSignature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Lock(&_Orderescrow.TransactOpts, quote, confirmer, merchantSignature, payerSignature)
}

// MarkDelivered is a paid mutator transaction binding the contract method 0xad4172b1.
//
// Solidity: function markDelivered(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactor) MarkDelivered(opts *bind.TransactOpts, merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "markDelivered", merchantId, orderId, signature)
}

// MarkDelivered is a paid mutator transaction binding the contract method 0xad4172b1.
//
// Solidity: function markDelivered(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowSession) MarkDelivered(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.MarkDelivered(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// MarkDelivered is a paid mutator transaction binding the contract method 0xad4172b1.
//
// Solidity: function markDelivered(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactorSession) MarkDelivered(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.MarkDelivered(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// RefundExpiredDispute is a paid mutator transaction binding the contract method 0x642275d1.
//
// Solidity: function refundExpiredDispute(bytes32 merchantId, bytes32 orderId) returns()
func (_Orderescrow *OrderescrowTransactor) RefundExpiredDispute(opts *bind.TransactOpts, merchantId [32]byte, orderId [32]byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "refundExpiredDispute", merchantId, orderId)
}

// RefundExpiredDispute is a paid mutator transaction binding the contract method 0x642275d1.
//
// Solidity: function refundExpiredDispute(bytes32 merchantId, bytes32 orderId) returns()
func (_Orderescrow *OrderescrowSession) RefundExpiredDispute(merchantId [32]byte, orderId [32]byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.RefundExpiredDispute(&_Orderescrow.TransactOpts, merchantId, orderId)
}

// RefundExpiredDispute is a paid mutator transaction binding the contract method 0x642275d1.
//
// Solidity: function refundExpiredDispute(bytes32 merchantId, bytes32 orderId) returns()
func (_Orderescrow *OrderescrowTransactorSession) RefundExpiredDispute(merchantId [32]byte, orderId [32]byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.RefundExpiredDispute(&_Orderescrow.TransactOpts, merchantId, orderId)
}

// RefundUndelivered is a paid mutator transaction binding the contract method 0x7e24cee6.
//
// Solidity: function refundUndelivered(bytes32 merchantId, bytes32 orderId) returns()
func (_Orderescrow *OrderescrowTransactor) RefundUndelivered(opts *bind.TransactOpts, merchantId [32]byte, orderId [32]byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "refundUndelivered", merchantId, orderId)
}

// RefundUndelivered is a paid mutator transaction binding the contract method 0x7e24cee6.
//
// Solidity: function refundUndelivered(bytes32 merchantId, bytes32 orderId) returns()
func (_Orderescrow *OrderescrowSession) RefundUndelivered(merchantId [32]byte, orderId [32]byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.RefundUndelivered(&_Orderescrow.TransactOpts, merchantId, orderId)
}

// RefundUndelivered is a paid mutator transaction binding the contract method 0x7e24cee6.
//
// Solidity: function refundUndelivered(bytes32 merchantId, bytes32 orderId) returns()
func (_Orderescrow *OrderescrowTransactorSession) RefundUndelivered(merchantId [32]byte, orderId [32]byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.RefundUndelivered(&_Orderescrow.TransactOpts, merchantId, orderId)
}

// Release is a paid mutator transaction binding the contract method 0x142f178c.
//
// Solidity: function release(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactor) Release(opts *bind.TransactOpts, merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "release", merchantId, orderId, signature)
}

// Release is a paid mutator transaction binding the contract method 0x142f178c.
//
// Solidity: function release(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowSession) Release(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Release(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// Release is a paid mutator transaction binding the contract method 0x142f178c.
//
// Solidity: function release(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactorSession) Release(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.Release(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// Resolve is a paid mutator transaction binding the contract method 0xcf578307.
//
// Solidity: function resolve(bytes32 merchantId, bytes32 orderId, bool payMerchant) returns()
func (_Orderescrow *OrderescrowTransactor) Resolve(opts *bind.TransactOpts, merchantId [32]byte, orderId [32]byte, payMerchant bool) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "resolve", merchantId, orderId, payMerchant)
}

// Resolve is a paid mutator transaction binding the contract method 0xcf578307.
//
// Solidity: function resolve(bytes32 merchantId, bytes32 orderId, bool payMerchant) returns()
func (_Orderescrow *OrderescrowSession) Resolve(merchantId [32]byte, orderId [32]byte, payMerchant bool) (*types.Transaction, error) {
	return _Orderescrow.Contract.Resolve(&_Orderescrow.TransactOpts, merchantId, orderId, payMerchant)
}

// Resolve is a paid mutator transaction binding the contract method 0xcf578307.
//
// Solidity: function resolve(bytes32 merchantId, bytes32 orderId, bool payMerchant) returns()
func (_Orderescrow *OrderescrowTransactorSession) Resolve(merchantId [32]byte, orderId [32]byte, payMerchant bool) (*types.Transaction, error) {
	return _Orderescrow.Contract.Resolve(&_Orderescrow.TransactOpts, merchantId, orderId, payMerchant)
}

// SetArbiter is a paid mutator transaction binding the contract method 0x1c9c088a.
//
// Solidity: function setArbiter(address next) returns()
func (_Orderescrow *OrderescrowTransactor) SetArbiter(opts *bind.TransactOpts, next common.Address) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "setArbiter", next)
}

// SetArbiter is a paid mutator transaction binding the contract method 0x1c9c088a.
//
// Solidity: function setArbiter(address next) returns()
func (_Orderescrow *OrderescrowSession) SetArbiter(next common.Address) (*types.Transaction, error) {
	return _Orderescrow.Contract.SetArbiter(&_Orderescrow.TransactOpts, next)
}

// SetArbiter is a paid mutator transaction binding the contract method 0x1c9c088a.
//
// Solidity: function setArbiter(address next) returns()
func (_Orderescrow *OrderescrowTransactorSession) SetArbiter(next common.Address) (*types.Transaction, error) {
	return _Orderescrow.Contract.SetArbiter(&_Orderescrow.TransactOpts, next)
}

// SetMerchantActive is a paid mutator transaction binding the contract method 0x6976e675.
//
// Solidity: function setMerchantActive(bytes32 merchantId, bool active) returns()
func (_Orderescrow *OrderescrowTransactor) SetMerchantActive(opts *bind.TransactOpts, merchantId [32]byte, active bool) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "setMerchantActive", merchantId, active)
}

// SetMerchantActive is a paid mutator transaction binding the contract method 0x6976e675.
//
// Solidity: function setMerchantActive(bytes32 merchantId, bool active) returns()
func (_Orderescrow *OrderescrowSession) SetMerchantActive(merchantId [32]byte, active bool) (*types.Transaction, error) {
	return _Orderescrow.Contract.SetMerchantActive(&_Orderescrow.TransactOpts, merchantId, active)
}

// SetMerchantActive is a paid mutator transaction binding the contract method 0x6976e675.
//
// Solidity: function setMerchantActive(bytes32 merchantId, bool active) returns()
func (_Orderescrow *OrderescrowTransactorSession) SetMerchantActive(merchantId [32]byte, active bool) (*types.Transaction, error) {
	return _Orderescrow.Contract.SetMerchantActive(&_Orderescrow.TransactOpts, merchantId, active)
}

// WithdrawDispute is a paid mutator transaction binding the contract method 0xe2696cfd.
//
// Solidity: function withdrawDispute(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactor) WithdrawDispute(opts *bind.TransactOpts, merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.contract.Transact(opts, "withdrawDispute", merchantId, orderId, signature)
}

// WithdrawDispute is a paid mutator transaction binding the contract method 0xe2696cfd.
//
// Solidity: function withdrawDispute(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowSession) WithdrawDispute(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.WithdrawDispute(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// WithdrawDispute is a paid mutator transaction binding the contract method 0xe2696cfd.
//
// Solidity: function withdrawDispute(bytes32 merchantId, bytes32 orderId, bytes signature) returns()
func (_Orderescrow *OrderescrowTransactorSession) WithdrawDispute(merchantId [32]byte, orderId [32]byte, signature []byte) (*types.Transaction, error) {
	return _Orderescrow.Contract.WithdrawDispute(&_Orderescrow.TransactOpts, merchantId, orderId, signature)
}

// OrderescrowArbiterChangedIterator is returned from FilterArbiterChanged and is used to iterate over the raw logs and unpacked data for ArbiterChanged events raised by the Orderescrow contract.
type OrderescrowArbiterChangedIterator struct {
	Event *OrderescrowArbiterChanged // Event containing the contract specifics and raw log

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
func (it *OrderescrowArbiterChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(OrderescrowArbiterChanged)
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
		it.Event = new(OrderescrowArbiterChanged)
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
func (it *OrderescrowArbiterChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *OrderescrowArbiterChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// OrderescrowArbiterChanged represents a ArbiterChanged event raised by the Orderescrow contract.
type OrderescrowArbiterChanged struct {
	Arbiter common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterArbiterChanged is a free log retrieval operation binding the contract event 0xed09dd09a633c9b0b1e25be13667a88772e82a16f68172a8f7ba0f22b303fef8.
//
// Solidity: event ArbiterChanged(address indexed arbiter)
func (_Orderescrow *OrderescrowFilterer) FilterArbiterChanged(opts *bind.FilterOpts, arbiter []common.Address) (*OrderescrowArbiterChangedIterator, error) {

	var arbiterRule []interface{}
	for _, arbiterItem := range arbiter {
		arbiterRule = append(arbiterRule, arbiterItem)
	}

	logs, sub, err := _Orderescrow.contract.FilterLogs(opts, "ArbiterChanged", arbiterRule)
	if err != nil {
		return nil, err
	}
	return &OrderescrowArbiterChangedIterator{contract: _Orderescrow.contract, event: "ArbiterChanged", logs: logs, sub: sub}, nil
}

// WatchArbiterChanged is a free log subscription operation binding the contract event 0xed09dd09a633c9b0b1e25be13667a88772e82a16f68172a8f7ba0f22b303fef8.
//
// Solidity: event ArbiterChanged(address indexed arbiter)
func (_Orderescrow *OrderescrowFilterer) WatchArbiterChanged(opts *bind.WatchOpts, sink chan<- *OrderescrowArbiterChanged, arbiter []common.Address) (event.Subscription, error) {

	var arbiterRule []interface{}
	for _, arbiterItem := range arbiter {
		arbiterRule = append(arbiterRule, arbiterItem)
	}

	logs, sub, err := _Orderescrow.contract.WatchLogs(opts, "ArbiterChanged", arbiterRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(OrderescrowArbiterChanged)
				if err := _Orderescrow.contract.UnpackLog(event, "ArbiterChanged", log); err != nil {
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

// ParseArbiterChanged is a log parse operation binding the contract event 0xed09dd09a633c9b0b1e25be13667a88772e82a16f68172a8f7ba0f22b303fef8.
//
// Solidity: event ArbiterChanged(address indexed arbiter)
func (_Orderescrow *OrderescrowFilterer) ParseArbiterChanged(log types.Log) (*OrderescrowArbiterChanged, error) {
	event := new(OrderescrowArbiterChanged)
	if err := _Orderescrow.contract.UnpackLog(event, "ArbiterChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// OrderescrowClaimSkippedIterator is returned from FilterClaimSkipped and is used to iterate over the raw logs and unpacked data for ClaimSkipped events raised by the Orderescrow contract.
type OrderescrowClaimSkippedIterator struct {
	Event *OrderescrowClaimSkipped // Event containing the contract specifics and raw log

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
func (it *OrderescrowClaimSkippedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(OrderescrowClaimSkipped)
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
		it.Event = new(OrderescrowClaimSkipped)
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
func (it *OrderescrowClaimSkippedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *OrderescrowClaimSkippedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// OrderescrowClaimSkipped represents a ClaimSkipped event raised by the Orderescrow contract.
type OrderescrowClaimSkipped struct {
	MerchantId [32]byte
	OrderId    [32]byte
	Reason     uint8
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterClaimSkipped is a free log retrieval operation binding the contract event 0xfdb7acac774f1995e587c9b6ee709adf95307274bf3dc09290157d125d87a93c.
//
// Solidity: event ClaimSkipped(bytes32 indexed merchantId, bytes32 indexed orderId, uint8 reason)
func (_Orderescrow *OrderescrowFilterer) FilterClaimSkipped(opts *bind.FilterOpts, merchantId [][32]byte, orderId [][32]byte) (*OrderescrowClaimSkippedIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.FilterLogs(opts, "ClaimSkipped", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return &OrderescrowClaimSkippedIterator{contract: _Orderescrow.contract, event: "ClaimSkipped", logs: logs, sub: sub}, nil
}

// WatchClaimSkipped is a free log subscription operation binding the contract event 0xfdb7acac774f1995e587c9b6ee709adf95307274bf3dc09290157d125d87a93c.
//
// Solidity: event ClaimSkipped(bytes32 indexed merchantId, bytes32 indexed orderId, uint8 reason)
func (_Orderescrow *OrderescrowFilterer) WatchClaimSkipped(opts *bind.WatchOpts, sink chan<- *OrderescrowClaimSkipped, merchantId [][32]byte, orderId [][32]byte) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.WatchLogs(opts, "ClaimSkipped", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(OrderescrowClaimSkipped)
				if err := _Orderescrow.contract.UnpackLog(event, "ClaimSkipped", log); err != nil {
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

// ParseClaimSkipped is a log parse operation binding the contract event 0xfdb7acac774f1995e587c9b6ee709adf95307274bf3dc09290157d125d87a93c.
//
// Solidity: event ClaimSkipped(bytes32 indexed merchantId, bytes32 indexed orderId, uint8 reason)
func (_Orderescrow *OrderescrowFilterer) ParseClaimSkipped(log types.Log) (*OrderescrowClaimSkipped, error) {
	event := new(OrderescrowClaimSkipped)
	if err := _Orderescrow.contract.UnpackLog(event, "ClaimSkipped", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// OrderescrowMerchantActiveIterator is returned from FilterMerchantActive and is used to iterate over the raw logs and unpacked data for MerchantActive events raised by the Orderescrow contract.
type OrderescrowMerchantActiveIterator struct {
	Event *OrderescrowMerchantActive // Event containing the contract specifics and raw log

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
func (it *OrderescrowMerchantActiveIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(OrderescrowMerchantActive)
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
		it.Event = new(OrderescrowMerchantActive)
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
func (it *OrderescrowMerchantActiveIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *OrderescrowMerchantActiveIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// OrderescrowMerchantActive represents a MerchantActive event raised by the Orderescrow contract.
type OrderescrowMerchantActive struct {
	MerchantId [32]byte
	Active     bool
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterMerchantActive is a free log retrieval operation binding the contract event 0x5348c6b0ac196c50c0177f366a918c3a8d88f7a6ba306a562b98d131365eddf5.
//
// Solidity: event MerchantActive(bytes32 indexed merchantId, bool active)
func (_Orderescrow *OrderescrowFilterer) FilterMerchantActive(opts *bind.FilterOpts, merchantId [][32]byte) (*OrderescrowMerchantActiveIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}

	logs, sub, err := _Orderescrow.contract.FilterLogs(opts, "MerchantActive", merchantIdRule)
	if err != nil {
		return nil, err
	}
	return &OrderescrowMerchantActiveIterator{contract: _Orderescrow.contract, event: "MerchantActive", logs: logs, sub: sub}, nil
}

// WatchMerchantActive is a free log subscription operation binding the contract event 0x5348c6b0ac196c50c0177f366a918c3a8d88f7a6ba306a562b98d131365eddf5.
//
// Solidity: event MerchantActive(bytes32 indexed merchantId, bool active)
func (_Orderescrow *OrderescrowFilterer) WatchMerchantActive(opts *bind.WatchOpts, sink chan<- *OrderescrowMerchantActive, merchantId [][32]byte) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}

	logs, sub, err := _Orderescrow.contract.WatchLogs(opts, "MerchantActive", merchantIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(OrderescrowMerchantActive)
				if err := _Orderescrow.contract.UnpackLog(event, "MerchantActive", log); err != nil {
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

// ParseMerchantActive is a log parse operation binding the contract event 0x5348c6b0ac196c50c0177f366a918c3a8d88f7a6ba306a562b98d131365eddf5.
//
// Solidity: event MerchantActive(bytes32 indexed merchantId, bool active)
func (_Orderescrow *OrderescrowFilterer) ParseMerchantActive(log types.Log) (*OrderescrowMerchantActive, error) {
	event := new(OrderescrowMerchantActive)
	if err := _Orderescrow.contract.UnpackLog(event, "MerchantActive", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// OrderescrowMerchantConfiguredIterator is returned from FilterMerchantConfigured and is used to iterate over the raw logs and unpacked data for MerchantConfigured events raised by the Orderescrow contract.
type OrderescrowMerchantConfiguredIterator struct {
	Event *OrderescrowMerchantConfigured // Event containing the contract specifics and raw log

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
func (it *OrderescrowMerchantConfiguredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(OrderescrowMerchantConfigured)
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
		it.Event = new(OrderescrowMerchantConfigured)
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
func (it *OrderescrowMerchantConfiguredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *OrderescrowMerchantConfiguredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// OrderescrowMerchantConfigured represents a MerchantConfigured event raised by the Orderescrow contract.
type OrderescrowMerchantConfigured struct {
	MerchantId         [32]byte
	Signer             common.Address
	Beneficiary        common.Address
	KeyVersion         uint64
	BeneficiaryVersion uint64
	ConfirmWindow      uint32
	DeliveryWindow     uint32
	Raw                types.Log // Blockchain specific contextual infos
}

// FilterMerchantConfigured is a free log retrieval operation binding the contract event 0xaed915b94fb985cb6c5120ad9973c48702d305de3080cac92c843bceecc604ad.
//
// Solidity: event MerchantConfigured(bytes32 indexed merchantId, address signer, address beneficiary, uint64 keyVersion, uint64 beneficiaryVersion, uint32 confirmWindow, uint32 deliveryWindow)
func (_Orderescrow *OrderescrowFilterer) FilterMerchantConfigured(opts *bind.FilterOpts, merchantId [][32]byte) (*OrderescrowMerchantConfiguredIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}

	logs, sub, err := _Orderescrow.contract.FilterLogs(opts, "MerchantConfigured", merchantIdRule)
	if err != nil {
		return nil, err
	}
	return &OrderescrowMerchantConfiguredIterator{contract: _Orderescrow.contract, event: "MerchantConfigured", logs: logs, sub: sub}, nil
}

// WatchMerchantConfigured is a free log subscription operation binding the contract event 0xaed915b94fb985cb6c5120ad9973c48702d305de3080cac92c843bceecc604ad.
//
// Solidity: event MerchantConfigured(bytes32 indexed merchantId, address signer, address beneficiary, uint64 keyVersion, uint64 beneficiaryVersion, uint32 confirmWindow, uint32 deliveryWindow)
func (_Orderescrow *OrderescrowFilterer) WatchMerchantConfigured(opts *bind.WatchOpts, sink chan<- *OrderescrowMerchantConfigured, merchantId [][32]byte) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}

	logs, sub, err := _Orderescrow.contract.WatchLogs(opts, "MerchantConfigured", merchantIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(OrderescrowMerchantConfigured)
				if err := _Orderescrow.contract.UnpackLog(event, "MerchantConfigured", log); err != nil {
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

// ParseMerchantConfigured is a log parse operation binding the contract event 0xaed915b94fb985cb6c5120ad9973c48702d305de3080cac92c843bceecc604ad.
//
// Solidity: event MerchantConfigured(bytes32 indexed merchantId, address signer, address beneficiary, uint64 keyVersion, uint64 beneficiaryVersion, uint32 confirmWindow, uint32 deliveryWindow)
func (_Orderescrow *OrderescrowFilterer) ParseMerchantConfigured(log types.Log) (*OrderescrowMerchantConfigured, error) {
	event := new(OrderescrowMerchantConfigured)
	if err := _Orderescrow.contract.UnpackLog(event, "MerchantConfigured", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// OrderescrowOrderCancelledIterator is returned from FilterOrderCancelled and is used to iterate over the raw logs and unpacked data for OrderCancelled events raised by the Orderescrow contract.
type OrderescrowOrderCancelledIterator struct {
	Event *OrderescrowOrderCancelled // Event containing the contract specifics and raw log

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
func (it *OrderescrowOrderCancelledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(OrderescrowOrderCancelled)
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
		it.Event = new(OrderescrowOrderCancelled)
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
func (it *OrderescrowOrderCancelledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *OrderescrowOrderCancelledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// OrderescrowOrderCancelled represents a OrderCancelled event raised by the Orderescrow contract.
type OrderescrowOrderCancelled struct {
	MerchantId [32]byte
	OrderId    [32]byte
	Payer      common.Address
	Amount     *big.Int
	Reason     uint8
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterOrderCancelled is a free log retrieval operation binding the contract event 0x71b45f1e02b9464b2d614ecb13b4bc3847838603f206dcd62d20b1f4208366bb.
//
// Solidity: event OrderCancelled(bytes32 indexed merchantId, bytes32 indexed orderId, address payer, uint256 amount, uint8 reason)
func (_Orderescrow *OrderescrowFilterer) FilterOrderCancelled(opts *bind.FilterOpts, merchantId [][32]byte, orderId [][32]byte) (*OrderescrowOrderCancelledIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.FilterLogs(opts, "OrderCancelled", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return &OrderescrowOrderCancelledIterator{contract: _Orderescrow.contract, event: "OrderCancelled", logs: logs, sub: sub}, nil
}

// WatchOrderCancelled is a free log subscription operation binding the contract event 0x71b45f1e02b9464b2d614ecb13b4bc3847838603f206dcd62d20b1f4208366bb.
//
// Solidity: event OrderCancelled(bytes32 indexed merchantId, bytes32 indexed orderId, address payer, uint256 amount, uint8 reason)
func (_Orderescrow *OrderescrowFilterer) WatchOrderCancelled(opts *bind.WatchOpts, sink chan<- *OrderescrowOrderCancelled, merchantId [][32]byte, orderId [][32]byte) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.WatchLogs(opts, "OrderCancelled", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(OrderescrowOrderCancelled)
				if err := _Orderescrow.contract.UnpackLog(event, "OrderCancelled", log); err != nil {
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

// ParseOrderCancelled is a log parse operation binding the contract event 0x71b45f1e02b9464b2d614ecb13b4bc3847838603f206dcd62d20b1f4208366bb.
//
// Solidity: event OrderCancelled(bytes32 indexed merchantId, bytes32 indexed orderId, address payer, uint256 amount, uint8 reason)
func (_Orderescrow *OrderescrowFilterer) ParseOrderCancelled(log types.Log) (*OrderescrowOrderCancelled, error) {
	event := new(OrderescrowOrderCancelled)
	if err := _Orderescrow.contract.UnpackLog(event, "OrderCancelled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// OrderescrowOrderDeliveredIterator is returned from FilterOrderDelivered and is used to iterate over the raw logs and unpacked data for OrderDelivered events raised by the Orderescrow contract.
type OrderescrowOrderDeliveredIterator struct {
	Event *OrderescrowOrderDelivered // Event containing the contract specifics and raw log

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
func (it *OrderescrowOrderDeliveredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(OrderescrowOrderDelivered)
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
		it.Event = new(OrderescrowOrderDelivered)
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
func (it *OrderescrowOrderDeliveredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *OrderescrowOrderDeliveredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// OrderescrowOrderDelivered represents a OrderDelivered event raised by the Orderescrow contract.
type OrderescrowOrderDelivered struct {
	MerchantId  [32]byte
	OrderId     [32]byte
	ClaimableAt uint64
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterOrderDelivered is a free log retrieval operation binding the contract event 0x79aad851f126425c1471ad8f2bdbcb0612add90a3842f0b467553e6b7fc8c85c.
//
// Solidity: event OrderDelivered(bytes32 indexed merchantId, bytes32 indexed orderId, uint64 claimableAt)
func (_Orderescrow *OrderescrowFilterer) FilterOrderDelivered(opts *bind.FilterOpts, merchantId [][32]byte, orderId [][32]byte) (*OrderescrowOrderDeliveredIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.FilterLogs(opts, "OrderDelivered", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return &OrderescrowOrderDeliveredIterator{contract: _Orderescrow.contract, event: "OrderDelivered", logs: logs, sub: sub}, nil
}

// WatchOrderDelivered is a free log subscription operation binding the contract event 0x79aad851f126425c1471ad8f2bdbcb0612add90a3842f0b467553e6b7fc8c85c.
//
// Solidity: event OrderDelivered(bytes32 indexed merchantId, bytes32 indexed orderId, uint64 claimableAt)
func (_Orderescrow *OrderescrowFilterer) WatchOrderDelivered(opts *bind.WatchOpts, sink chan<- *OrderescrowOrderDelivered, merchantId [][32]byte, orderId [][32]byte) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.WatchLogs(opts, "OrderDelivered", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(OrderescrowOrderDelivered)
				if err := _Orderescrow.contract.UnpackLog(event, "OrderDelivered", log); err != nil {
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

// ParseOrderDelivered is a log parse operation binding the contract event 0x79aad851f126425c1471ad8f2bdbcb0612add90a3842f0b467553e6b7fc8c85c.
//
// Solidity: event OrderDelivered(bytes32 indexed merchantId, bytes32 indexed orderId, uint64 claimableAt)
func (_Orderescrow *OrderescrowFilterer) ParseOrderDelivered(log types.Log) (*OrderescrowOrderDelivered, error) {
	event := new(OrderescrowOrderDelivered)
	if err := _Orderescrow.contract.UnpackLog(event, "OrderDelivered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// OrderescrowOrderDisputedIterator is returned from FilterOrderDisputed and is used to iterate over the raw logs and unpacked data for OrderDisputed events raised by the Orderescrow contract.
type OrderescrowOrderDisputedIterator struct {
	Event *OrderescrowOrderDisputed // Event containing the contract specifics and raw log

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
func (it *OrderescrowOrderDisputedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(OrderescrowOrderDisputed)
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
		it.Event = new(OrderescrowOrderDisputed)
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
func (it *OrderescrowOrderDisputedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *OrderescrowOrderDisputedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// OrderescrowOrderDisputed represents a OrderDisputed event raised by the Orderescrow contract.
type OrderescrowOrderDisputed struct {
	MerchantId [32]byte
	OrderId    [32]byte
	ResolveBy  uint64
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterOrderDisputed is a free log retrieval operation binding the contract event 0x65af27387495d085e9bce4a80312c0d0b087fd6190d65d06d4da71282a9bce39.
//
// Solidity: event OrderDisputed(bytes32 indexed merchantId, bytes32 indexed orderId, uint64 resolveBy)
func (_Orderescrow *OrderescrowFilterer) FilterOrderDisputed(opts *bind.FilterOpts, merchantId [][32]byte, orderId [][32]byte) (*OrderescrowOrderDisputedIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.FilterLogs(opts, "OrderDisputed", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return &OrderescrowOrderDisputedIterator{contract: _Orderescrow.contract, event: "OrderDisputed", logs: logs, sub: sub}, nil
}

// WatchOrderDisputed is a free log subscription operation binding the contract event 0x65af27387495d085e9bce4a80312c0d0b087fd6190d65d06d4da71282a9bce39.
//
// Solidity: event OrderDisputed(bytes32 indexed merchantId, bytes32 indexed orderId, uint64 resolveBy)
func (_Orderescrow *OrderescrowFilterer) WatchOrderDisputed(opts *bind.WatchOpts, sink chan<- *OrderescrowOrderDisputed, merchantId [][32]byte, orderId [][32]byte) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.WatchLogs(opts, "OrderDisputed", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(OrderescrowOrderDisputed)
				if err := _Orderescrow.contract.UnpackLog(event, "OrderDisputed", log); err != nil {
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

// ParseOrderDisputed is a log parse operation binding the contract event 0x65af27387495d085e9bce4a80312c0d0b087fd6190d65d06d4da71282a9bce39.
//
// Solidity: event OrderDisputed(bytes32 indexed merchantId, bytes32 indexed orderId, uint64 resolveBy)
func (_Orderescrow *OrderescrowFilterer) ParseOrderDisputed(log types.Log) (*OrderescrowOrderDisputed, error) {
	event := new(OrderescrowOrderDisputed)
	if err := _Orderescrow.contract.UnpackLog(event, "OrderDisputed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// OrderescrowOrderLockedIterator is returned from FilterOrderLocked and is used to iterate over the raw logs and unpacked data for OrderLocked events raised by the Orderescrow contract.
type OrderescrowOrderLockedIterator struct {
	Event *OrderescrowOrderLocked // Event containing the contract specifics and raw log

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
func (it *OrderescrowOrderLockedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(OrderescrowOrderLocked)
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
		it.Event = new(OrderescrowOrderLocked)
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
func (it *OrderescrowOrderLockedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *OrderescrowOrderLockedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// OrderescrowOrderLocked represents a OrderLocked event raised by the Orderescrow contract.
type OrderescrowOrderLocked struct {
	MerchantId    [32]byte
	OrderId       [32]byte
	Payer         common.Address
	Beneficiary   common.Address
	Confirmer     common.Address
	Amount        *big.Int
	DeliverBy     uint64
	ConfirmWindow uint32
	QuoteDigest   [32]byte
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOrderLocked is a free log retrieval operation binding the contract event 0xa700cf81ade0877ab0f11ee19c0cd98e7ba8a78111cbcdc94100c124849fb4c0.
//
// Solidity: event OrderLocked(bytes32 indexed merchantId, bytes32 indexed orderId, address indexed payer, address beneficiary, address confirmer, uint256 amount, uint64 deliverBy, uint32 confirmWindow, bytes32 quoteDigest)
func (_Orderescrow *OrderescrowFilterer) FilterOrderLocked(opts *bind.FilterOpts, merchantId [][32]byte, orderId [][32]byte, payer []common.Address) (*OrderescrowOrderLockedIterator, error) {

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

	logs, sub, err := _Orderescrow.contract.FilterLogs(opts, "OrderLocked", merchantIdRule, orderIdRule, payerRule)
	if err != nil {
		return nil, err
	}
	return &OrderescrowOrderLockedIterator{contract: _Orderescrow.contract, event: "OrderLocked", logs: logs, sub: sub}, nil
}

// WatchOrderLocked is a free log subscription operation binding the contract event 0xa700cf81ade0877ab0f11ee19c0cd98e7ba8a78111cbcdc94100c124849fb4c0.
//
// Solidity: event OrderLocked(bytes32 indexed merchantId, bytes32 indexed orderId, address indexed payer, address beneficiary, address confirmer, uint256 amount, uint64 deliverBy, uint32 confirmWindow, bytes32 quoteDigest)
func (_Orderescrow *OrderescrowFilterer) WatchOrderLocked(opts *bind.WatchOpts, sink chan<- *OrderescrowOrderLocked, merchantId [][32]byte, orderId [][32]byte, payer []common.Address) (event.Subscription, error) {

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

	logs, sub, err := _Orderescrow.contract.WatchLogs(opts, "OrderLocked", merchantIdRule, orderIdRule, payerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(OrderescrowOrderLocked)
				if err := _Orderescrow.contract.UnpackLog(event, "OrderLocked", log); err != nil {
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

// ParseOrderLocked is a log parse operation binding the contract event 0xa700cf81ade0877ab0f11ee19c0cd98e7ba8a78111cbcdc94100c124849fb4c0.
//
// Solidity: event OrderLocked(bytes32 indexed merchantId, bytes32 indexed orderId, address indexed payer, address beneficiary, address confirmer, uint256 amount, uint64 deliverBy, uint32 confirmWindow, bytes32 quoteDigest)
func (_Orderescrow *OrderescrowFilterer) ParseOrderLocked(log types.Log) (*OrderescrowOrderLocked, error) {
	event := new(OrderescrowOrderLocked)
	if err := _Orderescrow.contract.UnpackLog(event, "OrderLocked", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// OrderescrowOrderReleasedIterator is returned from FilterOrderReleased and is used to iterate over the raw logs and unpacked data for OrderReleased events raised by the Orderescrow contract.
type OrderescrowOrderReleasedIterator struct {
	Event *OrderescrowOrderReleased // Event containing the contract specifics and raw log

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
func (it *OrderescrowOrderReleasedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(OrderescrowOrderReleased)
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
		it.Event = new(OrderescrowOrderReleased)
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
func (it *OrderescrowOrderReleasedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *OrderescrowOrderReleasedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// OrderescrowOrderReleased represents a OrderReleased event raised by the Orderescrow contract.
type OrderescrowOrderReleased struct {
	MerchantId  [32]byte
	OrderId     [32]byte
	Beneficiary common.Address
	Amount      *big.Int
	Reason      uint8
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterOrderReleased is a free log retrieval operation binding the contract event 0xed92ca5efe9422051b3694c2cd05344630aeb19fa3f92d98589f96b08dcec633.
//
// Solidity: event OrderReleased(bytes32 indexed merchantId, bytes32 indexed orderId, address beneficiary, uint256 amount, uint8 reason)
func (_Orderescrow *OrderescrowFilterer) FilterOrderReleased(opts *bind.FilterOpts, merchantId [][32]byte, orderId [][32]byte) (*OrderescrowOrderReleasedIterator, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.FilterLogs(opts, "OrderReleased", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return &OrderescrowOrderReleasedIterator{contract: _Orderescrow.contract, event: "OrderReleased", logs: logs, sub: sub}, nil
}

// WatchOrderReleased is a free log subscription operation binding the contract event 0xed92ca5efe9422051b3694c2cd05344630aeb19fa3f92d98589f96b08dcec633.
//
// Solidity: event OrderReleased(bytes32 indexed merchantId, bytes32 indexed orderId, address beneficiary, uint256 amount, uint8 reason)
func (_Orderescrow *OrderescrowFilterer) WatchOrderReleased(opts *bind.WatchOpts, sink chan<- *OrderescrowOrderReleased, merchantId [][32]byte, orderId [][32]byte) (event.Subscription, error) {

	var merchantIdRule []interface{}
	for _, merchantIdItem := range merchantId {
		merchantIdRule = append(merchantIdRule, merchantIdItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}

	logs, sub, err := _Orderescrow.contract.WatchLogs(opts, "OrderReleased", merchantIdRule, orderIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(OrderescrowOrderReleased)
				if err := _Orderescrow.contract.UnpackLog(event, "OrderReleased", log); err != nil {
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

// ParseOrderReleased is a log parse operation binding the contract event 0xed92ca5efe9422051b3694c2cd05344630aeb19fa3f92d98589f96b08dcec633.
//
// Solidity: event OrderReleased(bytes32 indexed merchantId, bytes32 indexed orderId, address beneficiary, uint256 amount, uint8 reason)
func (_Orderescrow *OrderescrowFilterer) ParseOrderReleased(log types.Log) (*OrderescrowOrderReleased, error) {
	event := new(OrderescrowOrderReleased)
	if err := _Orderescrow.contract.UnpackLog(event, "OrderReleased", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
