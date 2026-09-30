// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package settlementext

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

// IPaymentSettlementExtensionsAccountView is an auto generated low-level Go binding around an user-defined struct.
type IPaymentSettlementExtensionsAccountView struct {
	Balance           *big.Int
	ClosedAt          *big.Int
	WithdrawAddress   common.Address
	PendingWithdrawal *big.Int
	WithdrawAfter     *big.Int
	PerPaymentLimit   *big.Int
	DailyLimit        *big.Int
	WindowStart       *big.Int
	WindowSpent       *big.Int
}

// PaymentTypesLimitChange is an auto generated low-level Go binding around an user-defined struct.
type PaymentTypesLimitChange struct {
	ChainId         *big.Int
	ContractAddress common.Address
	PerPaymentLimit *big.Int
	DailyLimit      *big.Int
	Nonce           *big.Int
	Expiry          uint64
}

// SettlementextMetaData contains all meta data concerning the Settlementext contract.
var SettlementextMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"}],\"name\":\"accountOf\",\"outputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"balance\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"closedAt\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"withdrawAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"pendingWithdrawal\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"withdrawAfter\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"perPaymentLimit\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"dailyLimit\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"windowStart\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"windowSpent\",\"type\":\"uint256\"}],\"internalType\":\"structIPaymentSettlementExtensions.AccountView\",\"name\":\"\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"}],\"name\":\"cancelWithdrawal\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"cashOutFor\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"}],\"name\":\"closeAccount\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"}],\"name\":\"executeWithdrawal\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"recoverSurplus\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"requestWithdrawal\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"contractAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"perPaymentLimit\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"dailyLimit\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"expiry\",\"type\":\"uint64\"}],\"internalType\":\"structPaymentTypes.LimitChange\",\"name\":\"change\",\"type\":\"tuple\"},{\"internalType\":\"bytes\",\"name\":\"sig\",\"type\":\"bytes\"}],\"name\":\"setLimits\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"surplus\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"totalOwed\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"withdrawalDelay\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"}],\"name\":\"AccountClosed\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"perPayment\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"daily\",\"type\":\"uint256\"}],\"name\":\"LimitsChanged\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"SurplusRecovered\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"}],\"name\":\"WithdrawalCancelled\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"WithdrawalExecuted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"WithdrawalRequested\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"NoPendingWithdrawal\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WithdrawAddressMismatch\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WithdrawalMatured\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WithdrawalNotReady\",\"type\":\"error\"}]",
}

// SettlementextABI is the input ABI used to generate the binding from.
// Deprecated: Use SettlementextMetaData.ABI instead.
var SettlementextABI = SettlementextMetaData.ABI

// Settlementext is an auto generated Go binding around an Ethereum contract.
type Settlementext struct {
	SettlementextCaller     // Read-only binding to the contract
	SettlementextTransactor // Write-only binding to the contract
	SettlementextFilterer   // Log filterer for contract events
}

// SettlementextCaller is an auto generated read-only Go binding around an Ethereum contract.
type SettlementextCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SettlementextTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SettlementextTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SettlementextFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SettlementextFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SettlementextSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SettlementextSession struct {
	Contract     *Settlementext    // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// SettlementextCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SettlementextCallerSession struct {
	Contract *SettlementextCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts        // Call options to use throughout this session
}

// SettlementextTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SettlementextTransactorSession struct {
	Contract     *SettlementextTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts        // Transaction auth options to use throughout this session
}

// SettlementextRaw is an auto generated low-level Go binding around an Ethereum contract.
type SettlementextRaw struct {
	Contract *Settlementext // Generic contract binding to access the raw methods on
}

// SettlementextCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SettlementextCallerRaw struct {
	Contract *SettlementextCaller // Generic read-only contract binding to access the raw methods on
}

// SettlementextTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SettlementextTransactorRaw struct {
	Contract *SettlementextTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSettlementext creates a new instance of Settlementext, bound to a specific deployed contract.
func NewSettlementext(address common.Address, backend bind.ContractBackend) (*Settlementext, error) {
	contract, err := bindSettlementext(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Settlementext{SettlementextCaller: SettlementextCaller{contract: contract}, SettlementextTransactor: SettlementextTransactor{contract: contract}, SettlementextFilterer: SettlementextFilterer{contract: contract}}, nil
}

// NewSettlementextCaller creates a new read-only instance of Settlementext, bound to a specific deployed contract.
func NewSettlementextCaller(address common.Address, caller bind.ContractCaller) (*SettlementextCaller, error) {
	contract, err := bindSettlementext(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SettlementextCaller{contract: contract}, nil
}

// NewSettlementextTransactor creates a new write-only instance of Settlementext, bound to a specific deployed contract.
func NewSettlementextTransactor(address common.Address, transactor bind.ContractTransactor) (*SettlementextTransactor, error) {
	contract, err := bindSettlementext(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SettlementextTransactor{contract: contract}, nil
}

// NewSettlementextFilterer creates a new log filterer instance of Settlementext, bound to a specific deployed contract.
func NewSettlementextFilterer(address common.Address, filterer bind.ContractFilterer) (*SettlementextFilterer, error) {
	contract, err := bindSettlementext(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SettlementextFilterer{contract: contract}, nil
}

// bindSettlementext binds a generic wrapper to an already deployed contract.
func bindSettlementext(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := SettlementextMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Settlementext *SettlementextRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Settlementext.Contract.SettlementextCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Settlementext *SettlementextRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Settlementext.Contract.SettlementextTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Settlementext *SettlementextRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Settlementext.Contract.SettlementextTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Settlementext *SettlementextCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Settlementext.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Settlementext *SettlementextTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Settlementext.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Settlementext *SettlementextTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Settlementext.Contract.contract.Transact(opts, method, params...)
}

// AccountOf is a free data retrieval call binding the contract method 0x8086b8ba.
//
// Solidity: function accountOf(address device) view returns((uint256,uint256,address,uint256,uint256,uint256,uint256,uint256,uint256))
func (_Settlementext *SettlementextCaller) AccountOf(opts *bind.CallOpts, device common.Address) (IPaymentSettlementExtensionsAccountView, error) {
	var out []interface{}
	err := _Settlementext.contract.Call(opts, &out, "accountOf", device)

	if err != nil {
		return *new(IPaymentSettlementExtensionsAccountView), err
	}

	out0 := *abi.ConvertType(out[0], new(IPaymentSettlementExtensionsAccountView)).(*IPaymentSettlementExtensionsAccountView)

	return out0, err

}

// AccountOf is a free data retrieval call binding the contract method 0x8086b8ba.
//
// Solidity: function accountOf(address device) view returns((uint256,uint256,address,uint256,uint256,uint256,uint256,uint256,uint256))
func (_Settlementext *SettlementextSession) AccountOf(device common.Address) (IPaymentSettlementExtensionsAccountView, error) {
	return _Settlementext.Contract.AccountOf(&_Settlementext.CallOpts, device)
}

// AccountOf is a free data retrieval call binding the contract method 0x8086b8ba.
//
// Solidity: function accountOf(address device) view returns((uint256,uint256,address,uint256,uint256,uint256,uint256,uint256,uint256))
func (_Settlementext *SettlementextCallerSession) AccountOf(device common.Address) (IPaymentSettlementExtensionsAccountView, error) {
	return _Settlementext.Contract.AccountOf(&_Settlementext.CallOpts, device)
}

// Surplus is a free data retrieval call binding the contract method 0x13888565.
//
// Solidity: function surplus() view returns(uint256)
func (_Settlementext *SettlementextCaller) Surplus(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Settlementext.contract.Call(opts, &out, "surplus")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Surplus is a free data retrieval call binding the contract method 0x13888565.
//
// Solidity: function surplus() view returns(uint256)
func (_Settlementext *SettlementextSession) Surplus() (*big.Int, error) {
	return _Settlementext.Contract.Surplus(&_Settlementext.CallOpts)
}

// Surplus is a free data retrieval call binding the contract method 0x13888565.
//
// Solidity: function surplus() view returns(uint256)
func (_Settlementext *SettlementextCallerSession) Surplus() (*big.Int, error) {
	return _Settlementext.Contract.Surplus(&_Settlementext.CallOpts)
}

// TotalOwed is a free data retrieval call binding the contract method 0xe7fa9f7d.
//
// Solidity: function totalOwed() view returns(uint256)
func (_Settlementext *SettlementextCaller) TotalOwed(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Settlementext.contract.Call(opts, &out, "totalOwed")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalOwed is a free data retrieval call binding the contract method 0xe7fa9f7d.
//
// Solidity: function totalOwed() view returns(uint256)
func (_Settlementext *SettlementextSession) TotalOwed() (*big.Int, error) {
	return _Settlementext.Contract.TotalOwed(&_Settlementext.CallOpts)
}

// TotalOwed is a free data retrieval call binding the contract method 0xe7fa9f7d.
//
// Solidity: function totalOwed() view returns(uint256)
func (_Settlementext *SettlementextCallerSession) TotalOwed() (*big.Int, error) {
	return _Settlementext.Contract.TotalOwed(&_Settlementext.CallOpts)
}

// WithdrawalDelay is a free data retrieval call binding the contract method 0xa7ab6961.
//
// Solidity: function withdrawalDelay() view returns(uint256)
func (_Settlementext *SettlementextCaller) WithdrawalDelay(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Settlementext.contract.Call(opts, &out, "withdrawalDelay")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// WithdrawalDelay is a free data retrieval call binding the contract method 0xa7ab6961.
//
// Solidity: function withdrawalDelay() view returns(uint256)
func (_Settlementext *SettlementextSession) WithdrawalDelay() (*big.Int, error) {
	return _Settlementext.Contract.WithdrawalDelay(&_Settlementext.CallOpts)
}

// WithdrawalDelay is a free data retrieval call binding the contract method 0xa7ab6961.
//
// Solidity: function withdrawalDelay() view returns(uint256)
func (_Settlementext *SettlementextCallerSession) WithdrawalDelay() (*big.Int, error) {
	return _Settlementext.Contract.WithdrawalDelay(&_Settlementext.CallOpts)
}

// CancelWithdrawal is a paid mutator transaction binding the contract method 0x3e0db869.
//
// Solidity: function cancelWithdrawal(address device) returns()
func (_Settlementext *SettlementextTransactor) CancelWithdrawal(opts *bind.TransactOpts, device common.Address) (*types.Transaction, error) {
	return _Settlementext.contract.Transact(opts, "cancelWithdrawal", device)
}

// CancelWithdrawal is a paid mutator transaction binding the contract method 0x3e0db869.
//
// Solidity: function cancelWithdrawal(address device) returns()
func (_Settlementext *SettlementextSession) CancelWithdrawal(device common.Address) (*types.Transaction, error) {
	return _Settlementext.Contract.CancelWithdrawal(&_Settlementext.TransactOpts, device)
}

// CancelWithdrawal is a paid mutator transaction binding the contract method 0x3e0db869.
//
// Solidity: function cancelWithdrawal(address device) returns()
func (_Settlementext *SettlementextTransactorSession) CancelWithdrawal(device common.Address) (*types.Transaction, error) {
	return _Settlementext.Contract.CancelWithdrawal(&_Settlementext.TransactOpts, device)
}

// CashOutFor is a paid mutator transaction binding the contract method 0x1830c2e9.
//
// Solidity: function cashOutFor(address merchant) returns()
func (_Settlementext *SettlementextTransactor) CashOutFor(opts *bind.TransactOpts, merchant common.Address) (*types.Transaction, error) {
	return _Settlementext.contract.Transact(opts, "cashOutFor", merchant)
}

// CashOutFor is a paid mutator transaction binding the contract method 0x1830c2e9.
//
// Solidity: function cashOutFor(address merchant) returns()
func (_Settlementext *SettlementextSession) CashOutFor(merchant common.Address) (*types.Transaction, error) {
	return _Settlementext.Contract.CashOutFor(&_Settlementext.TransactOpts, merchant)
}

// CashOutFor is a paid mutator transaction binding the contract method 0x1830c2e9.
//
// Solidity: function cashOutFor(address merchant) returns()
func (_Settlementext *SettlementextTransactorSession) CashOutFor(merchant common.Address) (*types.Transaction, error) {
	return _Settlementext.Contract.CashOutFor(&_Settlementext.TransactOpts, merchant)
}

// CloseAccount is a paid mutator transaction binding the contract method 0xdd336b94.
//
// Solidity: function closeAccount(address device) returns()
func (_Settlementext *SettlementextTransactor) CloseAccount(opts *bind.TransactOpts, device common.Address) (*types.Transaction, error) {
	return _Settlementext.contract.Transact(opts, "closeAccount", device)
}

// CloseAccount is a paid mutator transaction binding the contract method 0xdd336b94.
//
// Solidity: function closeAccount(address device) returns()
func (_Settlementext *SettlementextSession) CloseAccount(device common.Address) (*types.Transaction, error) {
	return _Settlementext.Contract.CloseAccount(&_Settlementext.TransactOpts, device)
}

// CloseAccount is a paid mutator transaction binding the contract method 0xdd336b94.
//
// Solidity: function closeAccount(address device) returns()
func (_Settlementext *SettlementextTransactorSession) CloseAccount(device common.Address) (*types.Transaction, error) {
	return _Settlementext.Contract.CloseAccount(&_Settlementext.TransactOpts, device)
}

// ExecuteWithdrawal is a paid mutator transaction binding the contract method 0x7deb9260.
//
// Solidity: function executeWithdrawal(address device) returns()
func (_Settlementext *SettlementextTransactor) ExecuteWithdrawal(opts *bind.TransactOpts, device common.Address) (*types.Transaction, error) {
	return _Settlementext.contract.Transact(opts, "executeWithdrawal", device)
}

// ExecuteWithdrawal is a paid mutator transaction binding the contract method 0x7deb9260.
//
// Solidity: function executeWithdrawal(address device) returns()
func (_Settlementext *SettlementextSession) ExecuteWithdrawal(device common.Address) (*types.Transaction, error) {
	return _Settlementext.Contract.ExecuteWithdrawal(&_Settlementext.TransactOpts, device)
}

// ExecuteWithdrawal is a paid mutator transaction binding the contract method 0x7deb9260.
//
// Solidity: function executeWithdrawal(address device) returns()
func (_Settlementext *SettlementextTransactorSession) ExecuteWithdrawal(device common.Address) (*types.Transaction, error) {
	return _Settlementext.Contract.ExecuteWithdrawal(&_Settlementext.TransactOpts, device)
}

// RecoverSurplus is a paid mutator transaction binding the contract method 0x0e57c69a.
//
// Solidity: function recoverSurplus(address to, uint256 amount) returns()
func (_Settlementext *SettlementextTransactor) RecoverSurplus(opts *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Settlementext.contract.Transact(opts, "recoverSurplus", to, amount)
}

// RecoverSurplus is a paid mutator transaction binding the contract method 0x0e57c69a.
//
// Solidity: function recoverSurplus(address to, uint256 amount) returns()
func (_Settlementext *SettlementextSession) RecoverSurplus(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Settlementext.Contract.RecoverSurplus(&_Settlementext.TransactOpts, to, amount)
}

// RecoverSurplus is a paid mutator transaction binding the contract method 0x0e57c69a.
//
// Solidity: function recoverSurplus(address to, uint256 amount) returns()
func (_Settlementext *SettlementextTransactorSession) RecoverSurplus(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Settlementext.Contract.RecoverSurplus(&_Settlementext.TransactOpts, to, amount)
}

// RequestWithdrawal is a paid mutator transaction binding the contract method 0xda95ebf7.
//
// Solidity: function requestWithdrawal(address device, uint256 amount) returns()
func (_Settlementext *SettlementextTransactor) RequestWithdrawal(opts *bind.TransactOpts, device common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Settlementext.contract.Transact(opts, "requestWithdrawal", device, amount)
}

// RequestWithdrawal is a paid mutator transaction binding the contract method 0xda95ebf7.
//
// Solidity: function requestWithdrawal(address device, uint256 amount) returns()
func (_Settlementext *SettlementextSession) RequestWithdrawal(device common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Settlementext.Contract.RequestWithdrawal(&_Settlementext.TransactOpts, device, amount)
}

// RequestWithdrawal is a paid mutator transaction binding the contract method 0xda95ebf7.
//
// Solidity: function requestWithdrawal(address device, uint256 amount) returns()
func (_Settlementext *SettlementextTransactorSession) RequestWithdrawal(device common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Settlementext.Contract.RequestWithdrawal(&_Settlementext.TransactOpts, device, amount)
}

// SetLimits is a paid mutator transaction binding the contract method 0xde26c6f2.
//
// Solidity: function setLimits((uint256,address,uint256,uint256,uint256,uint64) change, bytes sig) returns()
func (_Settlementext *SettlementextTransactor) SetLimits(opts *bind.TransactOpts, change PaymentTypesLimitChange, sig []byte) (*types.Transaction, error) {
	return _Settlementext.contract.Transact(opts, "setLimits", change, sig)
}

// SetLimits is a paid mutator transaction binding the contract method 0xde26c6f2.
//
// Solidity: function setLimits((uint256,address,uint256,uint256,uint256,uint64) change, bytes sig) returns()
func (_Settlementext *SettlementextSession) SetLimits(change PaymentTypesLimitChange, sig []byte) (*types.Transaction, error) {
	return _Settlementext.Contract.SetLimits(&_Settlementext.TransactOpts, change, sig)
}

// SetLimits is a paid mutator transaction binding the contract method 0xde26c6f2.
//
// Solidity: function setLimits((uint256,address,uint256,uint256,uint256,uint64) change, bytes sig) returns()
func (_Settlementext *SettlementextTransactorSession) SetLimits(change PaymentTypesLimitChange, sig []byte) (*types.Transaction, error) {
	return _Settlementext.Contract.SetLimits(&_Settlementext.TransactOpts, change, sig)
}

// SettlementextAccountClosedIterator is returned from FilterAccountClosed and is used to iterate over the raw logs and unpacked data for AccountClosed events raised by the Settlementext contract.
type SettlementextAccountClosedIterator struct {
	Event *SettlementextAccountClosed // Event containing the contract specifics and raw log

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
func (it *SettlementextAccountClosedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SettlementextAccountClosed)
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
		it.Event = new(SettlementextAccountClosed)
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
func (it *SettlementextAccountClosedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SettlementextAccountClosedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SettlementextAccountClosed represents a AccountClosed event raised by the Settlementext contract.
type SettlementextAccountClosed struct {
	Device common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterAccountClosed is a free log retrieval operation binding the contract event 0xa29911196d428d7968f8bde7515181a391bfa16e26042f789f3f2da7665e25de.
//
// Solidity: event AccountClosed(address indexed device)
func (_Settlementext *SettlementextFilterer) FilterAccountClosed(opts *bind.FilterOpts, device []common.Address) (*SettlementextAccountClosedIterator, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.FilterLogs(opts, "AccountClosed", deviceRule)
	if err != nil {
		return nil, err
	}
	return &SettlementextAccountClosedIterator{contract: _Settlementext.contract, event: "AccountClosed", logs: logs, sub: sub}, nil
}

// WatchAccountClosed is a free log subscription operation binding the contract event 0xa29911196d428d7968f8bde7515181a391bfa16e26042f789f3f2da7665e25de.
//
// Solidity: event AccountClosed(address indexed device)
func (_Settlementext *SettlementextFilterer) WatchAccountClosed(opts *bind.WatchOpts, sink chan<- *SettlementextAccountClosed, device []common.Address) (event.Subscription, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.WatchLogs(opts, "AccountClosed", deviceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SettlementextAccountClosed)
				if err := _Settlementext.contract.UnpackLog(event, "AccountClosed", log); err != nil {
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

// ParseAccountClosed is a log parse operation binding the contract event 0xa29911196d428d7968f8bde7515181a391bfa16e26042f789f3f2da7665e25de.
//
// Solidity: event AccountClosed(address indexed device)
func (_Settlementext *SettlementextFilterer) ParseAccountClosed(log types.Log) (*SettlementextAccountClosed, error) {
	event := new(SettlementextAccountClosed)
	if err := _Settlementext.contract.UnpackLog(event, "AccountClosed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SettlementextLimitsChangedIterator is returned from FilterLimitsChanged and is used to iterate over the raw logs and unpacked data for LimitsChanged events raised by the Settlementext contract.
type SettlementextLimitsChangedIterator struct {
	Event *SettlementextLimitsChanged // Event containing the contract specifics and raw log

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
func (it *SettlementextLimitsChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SettlementextLimitsChanged)
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
		it.Event = new(SettlementextLimitsChanged)
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
func (it *SettlementextLimitsChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SettlementextLimitsChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SettlementextLimitsChanged represents a LimitsChanged event raised by the Settlementext contract.
type SettlementextLimitsChanged struct {
	Device     common.Address
	PerPayment *big.Int
	Daily      *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterLimitsChanged is a free log retrieval operation binding the contract event 0x40673dd94c74259f5630bc8b77c27dce7b9dad44aa1a9e15acbac2dcf8643a08.
//
// Solidity: event LimitsChanged(address indexed device, uint256 perPayment, uint256 daily)
func (_Settlementext *SettlementextFilterer) FilterLimitsChanged(opts *bind.FilterOpts, device []common.Address) (*SettlementextLimitsChangedIterator, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.FilterLogs(opts, "LimitsChanged", deviceRule)
	if err != nil {
		return nil, err
	}
	return &SettlementextLimitsChangedIterator{contract: _Settlementext.contract, event: "LimitsChanged", logs: logs, sub: sub}, nil
}

// WatchLimitsChanged is a free log subscription operation binding the contract event 0x40673dd94c74259f5630bc8b77c27dce7b9dad44aa1a9e15acbac2dcf8643a08.
//
// Solidity: event LimitsChanged(address indexed device, uint256 perPayment, uint256 daily)
func (_Settlementext *SettlementextFilterer) WatchLimitsChanged(opts *bind.WatchOpts, sink chan<- *SettlementextLimitsChanged, device []common.Address) (event.Subscription, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.WatchLogs(opts, "LimitsChanged", deviceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SettlementextLimitsChanged)
				if err := _Settlementext.contract.UnpackLog(event, "LimitsChanged", log); err != nil {
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

// ParseLimitsChanged is a log parse operation binding the contract event 0x40673dd94c74259f5630bc8b77c27dce7b9dad44aa1a9e15acbac2dcf8643a08.
//
// Solidity: event LimitsChanged(address indexed device, uint256 perPayment, uint256 daily)
func (_Settlementext *SettlementextFilterer) ParseLimitsChanged(log types.Log) (*SettlementextLimitsChanged, error) {
	event := new(SettlementextLimitsChanged)
	if err := _Settlementext.contract.UnpackLog(event, "LimitsChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SettlementextSurplusRecoveredIterator is returned from FilterSurplusRecovered and is used to iterate over the raw logs and unpacked data for SurplusRecovered events raised by the Settlementext contract.
type SettlementextSurplusRecoveredIterator struct {
	Event *SettlementextSurplusRecovered // Event containing the contract specifics and raw log

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
func (it *SettlementextSurplusRecoveredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SettlementextSurplusRecovered)
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
		it.Event = new(SettlementextSurplusRecovered)
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
func (it *SettlementextSurplusRecoveredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SettlementextSurplusRecoveredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SettlementextSurplusRecovered represents a SurplusRecovered event raised by the Settlementext contract.
type SettlementextSurplusRecovered struct {
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterSurplusRecovered is a free log retrieval operation binding the contract event 0x8d062d570e15e665f57873e5745c84e024c3b6a4c94cfcda6af54a9ccd416af7.
//
// Solidity: event SurplusRecovered(address indexed to, uint256 amount)
func (_Settlementext *SettlementextFilterer) FilterSurplusRecovered(opts *bind.FilterOpts, to []common.Address) (*SettlementextSurplusRecoveredIterator, error) {

	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Settlementext.contract.FilterLogs(opts, "SurplusRecovered", toRule)
	if err != nil {
		return nil, err
	}
	return &SettlementextSurplusRecoveredIterator{contract: _Settlementext.contract, event: "SurplusRecovered", logs: logs, sub: sub}, nil
}

// WatchSurplusRecovered is a free log subscription operation binding the contract event 0x8d062d570e15e665f57873e5745c84e024c3b6a4c94cfcda6af54a9ccd416af7.
//
// Solidity: event SurplusRecovered(address indexed to, uint256 amount)
func (_Settlementext *SettlementextFilterer) WatchSurplusRecovered(opts *bind.WatchOpts, sink chan<- *SettlementextSurplusRecovered, to []common.Address) (event.Subscription, error) {

	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Settlementext.contract.WatchLogs(opts, "SurplusRecovered", toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SettlementextSurplusRecovered)
				if err := _Settlementext.contract.UnpackLog(event, "SurplusRecovered", log); err != nil {
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

// ParseSurplusRecovered is a log parse operation binding the contract event 0x8d062d570e15e665f57873e5745c84e024c3b6a4c94cfcda6af54a9ccd416af7.
//
// Solidity: event SurplusRecovered(address indexed to, uint256 amount)
func (_Settlementext *SettlementextFilterer) ParseSurplusRecovered(log types.Log) (*SettlementextSurplusRecovered, error) {
	event := new(SettlementextSurplusRecovered)
	if err := _Settlementext.contract.UnpackLog(event, "SurplusRecovered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SettlementextWithdrawalCancelledIterator is returned from FilterWithdrawalCancelled and is used to iterate over the raw logs and unpacked data for WithdrawalCancelled events raised by the Settlementext contract.
type SettlementextWithdrawalCancelledIterator struct {
	Event *SettlementextWithdrawalCancelled // Event containing the contract specifics and raw log

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
func (it *SettlementextWithdrawalCancelledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SettlementextWithdrawalCancelled)
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
		it.Event = new(SettlementextWithdrawalCancelled)
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
func (it *SettlementextWithdrawalCancelledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SettlementextWithdrawalCancelledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SettlementextWithdrawalCancelled represents a WithdrawalCancelled event raised by the Settlementext contract.
type SettlementextWithdrawalCancelled struct {
	Device common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterWithdrawalCancelled is a free log retrieval operation binding the contract event 0xc51fdb96728de385ec7859819e3997bc618362ef0dbca0ad051d856866cda3db.
//
// Solidity: event WithdrawalCancelled(address indexed device)
func (_Settlementext *SettlementextFilterer) FilterWithdrawalCancelled(opts *bind.FilterOpts, device []common.Address) (*SettlementextWithdrawalCancelledIterator, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.FilterLogs(opts, "WithdrawalCancelled", deviceRule)
	if err != nil {
		return nil, err
	}
	return &SettlementextWithdrawalCancelledIterator{contract: _Settlementext.contract, event: "WithdrawalCancelled", logs: logs, sub: sub}, nil
}

// WatchWithdrawalCancelled is a free log subscription operation binding the contract event 0xc51fdb96728de385ec7859819e3997bc618362ef0dbca0ad051d856866cda3db.
//
// Solidity: event WithdrawalCancelled(address indexed device)
func (_Settlementext *SettlementextFilterer) WatchWithdrawalCancelled(opts *bind.WatchOpts, sink chan<- *SettlementextWithdrawalCancelled, device []common.Address) (event.Subscription, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.WatchLogs(opts, "WithdrawalCancelled", deviceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SettlementextWithdrawalCancelled)
				if err := _Settlementext.contract.UnpackLog(event, "WithdrawalCancelled", log); err != nil {
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

// ParseWithdrawalCancelled is a log parse operation binding the contract event 0xc51fdb96728de385ec7859819e3997bc618362ef0dbca0ad051d856866cda3db.
//
// Solidity: event WithdrawalCancelled(address indexed device)
func (_Settlementext *SettlementextFilterer) ParseWithdrawalCancelled(log types.Log) (*SettlementextWithdrawalCancelled, error) {
	event := new(SettlementextWithdrawalCancelled)
	if err := _Settlementext.contract.UnpackLog(event, "WithdrawalCancelled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SettlementextWithdrawalExecutedIterator is returned from FilterWithdrawalExecuted and is used to iterate over the raw logs and unpacked data for WithdrawalExecuted events raised by the Settlementext contract.
type SettlementextWithdrawalExecutedIterator struct {
	Event *SettlementextWithdrawalExecuted // Event containing the contract specifics and raw log

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
func (it *SettlementextWithdrawalExecutedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SettlementextWithdrawalExecuted)
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
		it.Event = new(SettlementextWithdrawalExecuted)
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
func (it *SettlementextWithdrawalExecutedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SettlementextWithdrawalExecutedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SettlementextWithdrawalExecuted represents a WithdrawalExecuted event raised by the Settlementext contract.
type SettlementextWithdrawalExecuted struct {
	Device common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterWithdrawalExecuted is a free log retrieval operation binding the contract event 0x1131c2afe9ce72c59360d085f89bfdd088fcf4a3c93eec6cc5ff42f5d6b0801f.
//
// Solidity: event WithdrawalExecuted(address indexed device, uint256 amount)
func (_Settlementext *SettlementextFilterer) FilterWithdrawalExecuted(opts *bind.FilterOpts, device []common.Address) (*SettlementextWithdrawalExecutedIterator, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.FilterLogs(opts, "WithdrawalExecuted", deviceRule)
	if err != nil {
		return nil, err
	}
	return &SettlementextWithdrawalExecutedIterator{contract: _Settlementext.contract, event: "WithdrawalExecuted", logs: logs, sub: sub}, nil
}

// WatchWithdrawalExecuted is a free log subscription operation binding the contract event 0x1131c2afe9ce72c59360d085f89bfdd088fcf4a3c93eec6cc5ff42f5d6b0801f.
//
// Solidity: event WithdrawalExecuted(address indexed device, uint256 amount)
func (_Settlementext *SettlementextFilterer) WatchWithdrawalExecuted(opts *bind.WatchOpts, sink chan<- *SettlementextWithdrawalExecuted, device []common.Address) (event.Subscription, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.WatchLogs(opts, "WithdrawalExecuted", deviceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SettlementextWithdrawalExecuted)
				if err := _Settlementext.contract.UnpackLog(event, "WithdrawalExecuted", log); err != nil {
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

// ParseWithdrawalExecuted is a log parse operation binding the contract event 0x1131c2afe9ce72c59360d085f89bfdd088fcf4a3c93eec6cc5ff42f5d6b0801f.
//
// Solidity: event WithdrawalExecuted(address indexed device, uint256 amount)
func (_Settlementext *SettlementextFilterer) ParseWithdrawalExecuted(log types.Log) (*SettlementextWithdrawalExecuted, error) {
	event := new(SettlementextWithdrawalExecuted)
	if err := _Settlementext.contract.UnpackLog(event, "WithdrawalExecuted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SettlementextWithdrawalRequestedIterator is returned from FilterWithdrawalRequested and is used to iterate over the raw logs and unpacked data for WithdrawalRequested events raised by the Settlementext contract.
type SettlementextWithdrawalRequestedIterator struct {
	Event *SettlementextWithdrawalRequested // Event containing the contract specifics and raw log

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
func (it *SettlementextWithdrawalRequestedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SettlementextWithdrawalRequested)
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
		it.Event = new(SettlementextWithdrawalRequested)
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
func (it *SettlementextWithdrawalRequestedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SettlementextWithdrawalRequestedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SettlementextWithdrawalRequested represents a WithdrawalRequested event raised by the Settlementext contract.
type SettlementextWithdrawalRequested struct {
	Device common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterWithdrawalRequested is a free log retrieval operation binding the contract event 0xe670e4e82118d22a1f9ee18920455ebc958bae26a90a05d31d3378788b1b0e44.
//
// Solidity: event WithdrawalRequested(address indexed device, uint256 amount)
func (_Settlementext *SettlementextFilterer) FilterWithdrawalRequested(opts *bind.FilterOpts, device []common.Address) (*SettlementextWithdrawalRequestedIterator, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.FilterLogs(opts, "WithdrawalRequested", deviceRule)
	if err != nil {
		return nil, err
	}
	return &SettlementextWithdrawalRequestedIterator{contract: _Settlementext.contract, event: "WithdrawalRequested", logs: logs, sub: sub}, nil
}

// WatchWithdrawalRequested is a free log subscription operation binding the contract event 0xe670e4e82118d22a1f9ee18920455ebc958bae26a90a05d31d3378788b1b0e44.
//
// Solidity: event WithdrawalRequested(address indexed device, uint256 amount)
func (_Settlementext *SettlementextFilterer) WatchWithdrawalRequested(opts *bind.WatchOpts, sink chan<- *SettlementextWithdrawalRequested, device []common.Address) (event.Subscription, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlementext.contract.WatchLogs(opts, "WithdrawalRequested", deviceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SettlementextWithdrawalRequested)
				if err := _Settlementext.contract.UnpackLog(event, "WithdrawalRequested", log); err != nil {
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

// ParseWithdrawalRequested is a log parse operation binding the contract event 0xe670e4e82118d22a1f9ee18920455ebc958bae26a90a05d31d3378788b1b0e44.
//
// Solidity: event WithdrawalRequested(address indexed device, uint256 amount)
func (_Settlementext *SettlementextFilterer) ParseWithdrawalRequested(log types.Log) (*SettlementextWithdrawalRequested, error) {
	event := new(SettlementextWithdrawalRequested)
	if err := _Settlementext.contract.UnpackLog(event, "WithdrawalRequested", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
