// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package settlement

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

// PaymentTypesPaymentAuthorization is an auto generated low-level Go binding around an user-defined struct.
type PaymentTypesPaymentAuthorization struct {
	ChainId         *big.Int
	ContractAddress common.Address
	Merchant        common.Address
	Payout          common.Address
	Token           common.Address
	Amount          *big.Int
	OrderId         [32]byte
	Nonce           *big.Int
	Expiry          uint64
}

// SettlementMetaData contains all meta data concerning the Settlement contract.
var SettlementMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"}],\"name\":\"balanceOf\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"cashOut\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"withdrawAddress\",\"type\":\"address\"}],\"name\":\"depositFor\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"domainSeparator\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"}],\"name\":\"isNonceUsed\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"}],\"name\":\"isPaid\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"merchantBalance\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"operator\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"registry\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"contractAddress\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"payout\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"expiry\",\"type\":\"uint64\"}],\"internalType\":\"structPaymentTypes.PaymentAuthorization\",\"name\":\"auth\",\"type\":\"tuple\"},{\"internalType\":\"bytes\",\"name\":\"sig\",\"type\":\"bytes\"}],\"name\":\"settle\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"token\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"payout\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"CashedOut\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"Deposited\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"orderId\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"device\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"}],\"name\":\"PaymentSettled\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"AccountInactive\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"Expired\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InsufficientBalance\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"MerchantForged\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"MerchantRevoked\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NonceReplayed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotOperator\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"OrderAlreadyPaid\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"OverCap\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"TransferFailed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WrongDomain\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ZeroAddress\",\"type\":\"error\"}]",
}

// SettlementABI is the input ABI used to generate the binding from.
// Deprecated: Use SettlementMetaData.ABI instead.
var SettlementABI = SettlementMetaData.ABI

// Settlement is an auto generated Go binding around an Ethereum contract.
type Settlement struct {
	SettlementCaller     // Read-only binding to the contract
	SettlementTransactor // Write-only binding to the contract
	SettlementFilterer   // Log filterer for contract events
}

// SettlementCaller is an auto generated read-only Go binding around an Ethereum contract.
type SettlementCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SettlementTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SettlementTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SettlementFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SettlementFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SettlementSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SettlementSession struct {
	Contract     *Settlement       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// SettlementCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SettlementCallerSession struct {
	Contract *SettlementCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// SettlementTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SettlementTransactorSession struct {
	Contract     *SettlementTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// SettlementRaw is an auto generated low-level Go binding around an Ethereum contract.
type SettlementRaw struct {
	Contract *Settlement // Generic contract binding to access the raw methods on
}

// SettlementCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SettlementCallerRaw struct {
	Contract *SettlementCaller // Generic read-only contract binding to access the raw methods on
}

// SettlementTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SettlementTransactorRaw struct {
	Contract *SettlementTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSettlement creates a new instance of Settlement, bound to a specific deployed contract.
func NewSettlement(address common.Address, backend bind.ContractBackend) (*Settlement, error) {
	contract, err := bindSettlement(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Settlement{SettlementCaller: SettlementCaller{contract: contract}, SettlementTransactor: SettlementTransactor{contract: contract}, SettlementFilterer: SettlementFilterer{contract: contract}}, nil
}

// NewSettlementCaller creates a new read-only instance of Settlement, bound to a specific deployed contract.
func NewSettlementCaller(address common.Address, caller bind.ContractCaller) (*SettlementCaller, error) {
	contract, err := bindSettlement(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SettlementCaller{contract: contract}, nil
}

// NewSettlementTransactor creates a new write-only instance of Settlement, bound to a specific deployed contract.
func NewSettlementTransactor(address common.Address, transactor bind.ContractTransactor) (*SettlementTransactor, error) {
	contract, err := bindSettlement(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SettlementTransactor{contract: contract}, nil
}

// NewSettlementFilterer creates a new log filterer instance of Settlement, bound to a specific deployed contract.
func NewSettlementFilterer(address common.Address, filterer bind.ContractFilterer) (*SettlementFilterer, error) {
	contract, err := bindSettlement(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SettlementFilterer{contract: contract}, nil
}

// bindSettlement binds a generic wrapper to an already deployed contract.
func bindSettlement(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := SettlementMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Settlement *SettlementRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Settlement.Contract.SettlementCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Settlement *SettlementRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Settlement.Contract.SettlementTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Settlement *SettlementRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Settlement.Contract.SettlementTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Settlement *SettlementCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Settlement.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Settlement *SettlementTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Settlement.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Settlement *SettlementTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Settlement.Contract.contract.Transact(opts, method, params...)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address device) view returns(uint256)
func (_Settlement *SettlementCaller) BalanceOf(opts *bind.CallOpts, device common.Address) (*big.Int, error) {
	var out []interface{}
	err := _Settlement.contract.Call(opts, &out, "balanceOf", device)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address device) view returns(uint256)
func (_Settlement *SettlementSession) BalanceOf(device common.Address) (*big.Int, error) {
	return _Settlement.Contract.BalanceOf(&_Settlement.CallOpts, device)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address device) view returns(uint256)
func (_Settlement *SettlementCallerSession) BalanceOf(device common.Address) (*big.Int, error) {
	return _Settlement.Contract.BalanceOf(&_Settlement.CallOpts, device)
}

// DomainSeparator is a free data retrieval call binding the contract method 0xf698da25.
//
// Solidity: function domainSeparator() view returns(bytes32)
func (_Settlement *SettlementCaller) DomainSeparator(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Settlement.contract.Call(opts, &out, "domainSeparator")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// DomainSeparator is a free data retrieval call binding the contract method 0xf698da25.
//
// Solidity: function domainSeparator() view returns(bytes32)
func (_Settlement *SettlementSession) DomainSeparator() ([32]byte, error) {
	return _Settlement.Contract.DomainSeparator(&_Settlement.CallOpts)
}

// DomainSeparator is a free data retrieval call binding the contract method 0xf698da25.
//
// Solidity: function domainSeparator() view returns(bytes32)
func (_Settlement *SettlementCallerSession) DomainSeparator() ([32]byte, error) {
	return _Settlement.Contract.DomainSeparator(&_Settlement.CallOpts)
}

// IsNonceUsed is a free data retrieval call binding the contract method 0xcab7e8eb.
//
// Solidity: function isNonceUsed(address device, uint256 nonce) view returns(bool)
func (_Settlement *SettlementCaller) IsNonceUsed(opts *bind.CallOpts, device common.Address, nonce *big.Int) (bool, error) {
	var out []interface{}
	err := _Settlement.contract.Call(opts, &out, "isNonceUsed", device, nonce)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsNonceUsed is a free data retrieval call binding the contract method 0xcab7e8eb.
//
// Solidity: function isNonceUsed(address device, uint256 nonce) view returns(bool)
func (_Settlement *SettlementSession) IsNonceUsed(device common.Address, nonce *big.Int) (bool, error) {
	return _Settlement.Contract.IsNonceUsed(&_Settlement.CallOpts, device, nonce)
}

// IsNonceUsed is a free data retrieval call binding the contract method 0xcab7e8eb.
//
// Solidity: function isNonceUsed(address device, uint256 nonce) view returns(bool)
func (_Settlement *SettlementCallerSession) IsNonceUsed(device common.Address, nonce *big.Int) (bool, error) {
	return _Settlement.Contract.IsNonceUsed(&_Settlement.CallOpts, device, nonce)
}

// IsPaid is a free data retrieval call binding the contract method 0x92a43afc.
//
// Solidity: function isPaid(address merchant, bytes32 orderId) view returns(bool)
func (_Settlement *SettlementCaller) IsPaid(opts *bind.CallOpts, merchant common.Address, orderId [32]byte) (bool, error) {
	var out []interface{}
	err := _Settlement.contract.Call(opts, &out, "isPaid", merchant, orderId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsPaid is a free data retrieval call binding the contract method 0x92a43afc.
//
// Solidity: function isPaid(address merchant, bytes32 orderId) view returns(bool)
func (_Settlement *SettlementSession) IsPaid(merchant common.Address, orderId [32]byte) (bool, error) {
	return _Settlement.Contract.IsPaid(&_Settlement.CallOpts, merchant, orderId)
}

// IsPaid is a free data retrieval call binding the contract method 0x92a43afc.
//
// Solidity: function isPaid(address merchant, bytes32 orderId) view returns(bool)
func (_Settlement *SettlementCallerSession) IsPaid(merchant common.Address, orderId [32]byte) (bool, error) {
	return _Settlement.Contract.IsPaid(&_Settlement.CallOpts, merchant, orderId)
}

// MerchantBalance is a free data retrieval call binding the contract method 0x4cce98f6.
//
// Solidity: function merchantBalance(address merchant) view returns(uint256)
func (_Settlement *SettlementCaller) MerchantBalance(opts *bind.CallOpts, merchant common.Address) (*big.Int, error) {
	var out []interface{}
	err := _Settlement.contract.Call(opts, &out, "merchantBalance", merchant)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MerchantBalance is a free data retrieval call binding the contract method 0x4cce98f6.
//
// Solidity: function merchantBalance(address merchant) view returns(uint256)
func (_Settlement *SettlementSession) MerchantBalance(merchant common.Address) (*big.Int, error) {
	return _Settlement.Contract.MerchantBalance(&_Settlement.CallOpts, merchant)
}

// MerchantBalance is a free data retrieval call binding the contract method 0x4cce98f6.
//
// Solidity: function merchantBalance(address merchant) view returns(uint256)
func (_Settlement *SettlementCallerSession) MerchantBalance(merchant common.Address) (*big.Int, error) {
	return _Settlement.Contract.MerchantBalance(&_Settlement.CallOpts, merchant)
}

// Operator is a free data retrieval call binding the contract method 0x570ca735.
//
// Solidity: function operator() view returns(address)
func (_Settlement *SettlementCaller) Operator(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Settlement.contract.Call(opts, &out, "operator")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Operator is a free data retrieval call binding the contract method 0x570ca735.
//
// Solidity: function operator() view returns(address)
func (_Settlement *SettlementSession) Operator() (common.Address, error) {
	return _Settlement.Contract.Operator(&_Settlement.CallOpts)
}

// Operator is a free data retrieval call binding the contract method 0x570ca735.
//
// Solidity: function operator() view returns(address)
func (_Settlement *SettlementCallerSession) Operator() (common.Address, error) {
	return _Settlement.Contract.Operator(&_Settlement.CallOpts)
}

// Registry is a free data retrieval call binding the contract method 0x7b103999.
//
// Solidity: function registry() view returns(address)
func (_Settlement *SettlementCaller) Registry(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Settlement.contract.Call(opts, &out, "registry")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Registry is a free data retrieval call binding the contract method 0x7b103999.
//
// Solidity: function registry() view returns(address)
func (_Settlement *SettlementSession) Registry() (common.Address, error) {
	return _Settlement.Contract.Registry(&_Settlement.CallOpts)
}

// Registry is a free data retrieval call binding the contract method 0x7b103999.
//
// Solidity: function registry() view returns(address)
func (_Settlement *SettlementCallerSession) Registry() (common.Address, error) {
	return _Settlement.Contract.Registry(&_Settlement.CallOpts)
}

// Token is a free data retrieval call binding the contract method 0xfc0c546a.
//
// Solidity: function token() view returns(address)
func (_Settlement *SettlementCaller) Token(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Settlement.contract.Call(opts, &out, "token")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Token is a free data retrieval call binding the contract method 0xfc0c546a.
//
// Solidity: function token() view returns(address)
func (_Settlement *SettlementSession) Token() (common.Address, error) {
	return _Settlement.Contract.Token(&_Settlement.CallOpts)
}

// Token is a free data retrieval call binding the contract method 0xfc0c546a.
//
// Solidity: function token() view returns(address)
func (_Settlement *SettlementCallerSession) Token() (common.Address, error) {
	return _Settlement.Contract.Token(&_Settlement.CallOpts)
}

// CashOut is a paid mutator transaction binding the contract method 0x793cd71e.
//
// Solidity: function cashOut() returns()
func (_Settlement *SettlementTransactor) CashOut(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Settlement.contract.Transact(opts, "cashOut")
}

// CashOut is a paid mutator transaction binding the contract method 0x793cd71e.
//
// Solidity: function cashOut() returns()
func (_Settlement *SettlementSession) CashOut() (*types.Transaction, error) {
	return _Settlement.Contract.CashOut(&_Settlement.TransactOpts)
}

// CashOut is a paid mutator transaction binding the contract method 0x793cd71e.
//
// Solidity: function cashOut() returns()
func (_Settlement *SettlementTransactorSession) CashOut() (*types.Transaction, error) {
	return _Settlement.Contract.CashOut(&_Settlement.TransactOpts)
}

// DepositFor is a paid mutator transaction binding the contract method 0xc8820f6c.
//
// Solidity: function depositFor(address device, uint256 amount, address withdrawAddress) returns()
func (_Settlement *SettlementTransactor) DepositFor(opts *bind.TransactOpts, device common.Address, amount *big.Int, withdrawAddress common.Address) (*types.Transaction, error) {
	return _Settlement.contract.Transact(opts, "depositFor", device, amount, withdrawAddress)
}

// DepositFor is a paid mutator transaction binding the contract method 0xc8820f6c.
//
// Solidity: function depositFor(address device, uint256 amount, address withdrawAddress) returns()
func (_Settlement *SettlementSession) DepositFor(device common.Address, amount *big.Int, withdrawAddress common.Address) (*types.Transaction, error) {
	return _Settlement.Contract.DepositFor(&_Settlement.TransactOpts, device, amount, withdrawAddress)
}

// DepositFor is a paid mutator transaction binding the contract method 0xc8820f6c.
//
// Solidity: function depositFor(address device, uint256 amount, address withdrawAddress) returns()
func (_Settlement *SettlementTransactorSession) DepositFor(device common.Address, amount *big.Int, withdrawAddress common.Address) (*types.Transaction, error) {
	return _Settlement.Contract.DepositFor(&_Settlement.TransactOpts, device, amount, withdrawAddress)
}

// Settle is a paid mutator transaction binding the contract method 0x7e717651.
//
// Solidity: function settle((uint256,address,address,address,address,uint256,bytes32,uint256,uint64) auth, bytes sig) returns()
func (_Settlement *SettlementTransactor) Settle(opts *bind.TransactOpts, auth PaymentTypesPaymentAuthorization, sig []byte) (*types.Transaction, error) {
	return _Settlement.contract.Transact(opts, "settle", auth, sig)
}

// Settle is a paid mutator transaction binding the contract method 0x7e717651.
//
// Solidity: function settle((uint256,address,address,address,address,uint256,bytes32,uint256,uint64) auth, bytes sig) returns()
func (_Settlement *SettlementSession) Settle(auth PaymentTypesPaymentAuthorization, sig []byte) (*types.Transaction, error) {
	return _Settlement.Contract.Settle(&_Settlement.TransactOpts, auth, sig)
}

// Settle is a paid mutator transaction binding the contract method 0x7e717651.
//
// Solidity: function settle((uint256,address,address,address,address,uint256,bytes32,uint256,uint64) auth, bytes sig) returns()
func (_Settlement *SettlementTransactorSession) Settle(auth PaymentTypesPaymentAuthorization, sig []byte) (*types.Transaction, error) {
	return _Settlement.Contract.Settle(&_Settlement.TransactOpts, auth, sig)
}

// SettlementCashedOutIterator is returned from FilterCashedOut and is used to iterate over the raw logs and unpacked data for CashedOut events raised by the Settlement contract.
type SettlementCashedOutIterator struct {
	Event *SettlementCashedOut // Event containing the contract specifics and raw log

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
func (it *SettlementCashedOutIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SettlementCashedOut)
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
		it.Event = new(SettlementCashedOut)
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
func (it *SettlementCashedOutIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SettlementCashedOutIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SettlementCashedOut represents a CashedOut event raised by the Settlement contract.
type SettlementCashedOut struct {
	Merchant common.Address
	Payout   common.Address
	Amount   *big.Int
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterCashedOut is a free log retrieval operation binding the contract event 0x9d83ea6cc47b1e7f87d6bc7ff3e803baf544a6e9116ca5071d051040928d1c05.
//
// Solidity: event CashedOut(address indexed merchant, address indexed payout, uint256 amount)
func (_Settlement *SettlementFilterer) FilterCashedOut(opts *bind.FilterOpts, merchant []common.Address, payout []common.Address) (*SettlementCashedOutIterator, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}
	var payoutRule []interface{}
	for _, payoutItem := range payout {
		payoutRule = append(payoutRule, payoutItem)
	}

	logs, sub, err := _Settlement.contract.FilterLogs(opts, "CashedOut", merchantRule, payoutRule)
	if err != nil {
		return nil, err
	}
	return &SettlementCashedOutIterator{contract: _Settlement.contract, event: "CashedOut", logs: logs, sub: sub}, nil
}

// WatchCashedOut is a free log subscription operation binding the contract event 0x9d83ea6cc47b1e7f87d6bc7ff3e803baf544a6e9116ca5071d051040928d1c05.
//
// Solidity: event CashedOut(address indexed merchant, address indexed payout, uint256 amount)
func (_Settlement *SettlementFilterer) WatchCashedOut(opts *bind.WatchOpts, sink chan<- *SettlementCashedOut, merchant []common.Address, payout []common.Address) (event.Subscription, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}
	var payoutRule []interface{}
	for _, payoutItem := range payout {
		payoutRule = append(payoutRule, payoutItem)
	}

	logs, sub, err := _Settlement.contract.WatchLogs(opts, "CashedOut", merchantRule, payoutRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SettlementCashedOut)
				if err := _Settlement.contract.UnpackLog(event, "CashedOut", log); err != nil {
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

// ParseCashedOut is a log parse operation binding the contract event 0x9d83ea6cc47b1e7f87d6bc7ff3e803baf544a6e9116ca5071d051040928d1c05.
//
// Solidity: event CashedOut(address indexed merchant, address indexed payout, uint256 amount)
func (_Settlement *SettlementFilterer) ParseCashedOut(log types.Log) (*SettlementCashedOut, error) {
	event := new(SettlementCashedOut)
	if err := _Settlement.contract.UnpackLog(event, "CashedOut", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SettlementDepositedIterator is returned from FilterDeposited and is used to iterate over the raw logs and unpacked data for Deposited events raised by the Settlement contract.
type SettlementDepositedIterator struct {
	Event *SettlementDeposited // Event containing the contract specifics and raw log

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
func (it *SettlementDepositedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SettlementDeposited)
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
		it.Event = new(SettlementDeposited)
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
func (it *SettlementDepositedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SettlementDepositedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SettlementDeposited represents a Deposited event raised by the Settlement contract.
type SettlementDeposited struct {
	Device common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterDeposited is a free log retrieval operation binding the contract event 0x2da466a7b24304f47e87fa2e1e5a81b9831ce54fec19055ce277ca2f39ba42c4.
//
// Solidity: event Deposited(address indexed device, uint256 amount)
func (_Settlement *SettlementFilterer) FilterDeposited(opts *bind.FilterOpts, device []common.Address) (*SettlementDepositedIterator, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlement.contract.FilterLogs(opts, "Deposited", deviceRule)
	if err != nil {
		return nil, err
	}
	return &SettlementDepositedIterator{contract: _Settlement.contract, event: "Deposited", logs: logs, sub: sub}, nil
}

// WatchDeposited is a free log subscription operation binding the contract event 0x2da466a7b24304f47e87fa2e1e5a81b9831ce54fec19055ce277ca2f39ba42c4.
//
// Solidity: event Deposited(address indexed device, uint256 amount)
func (_Settlement *SettlementFilterer) WatchDeposited(opts *bind.WatchOpts, sink chan<- *SettlementDeposited, device []common.Address) (event.Subscription, error) {

	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlement.contract.WatchLogs(opts, "Deposited", deviceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SettlementDeposited)
				if err := _Settlement.contract.UnpackLog(event, "Deposited", log); err != nil {
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

// ParseDeposited is a log parse operation binding the contract event 0x2da466a7b24304f47e87fa2e1e5a81b9831ce54fec19055ce277ca2f39ba42c4.
//
// Solidity: event Deposited(address indexed device, uint256 amount)
func (_Settlement *SettlementFilterer) ParseDeposited(log types.Log) (*SettlementDeposited, error) {
	event := new(SettlementDeposited)
	if err := _Settlement.contract.UnpackLog(event, "Deposited", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SettlementPaymentSettledIterator is returned from FilterPaymentSettled and is used to iterate over the raw logs and unpacked data for PaymentSettled events raised by the Settlement contract.
type SettlementPaymentSettledIterator struct {
	Event *SettlementPaymentSettled // Event containing the contract specifics and raw log

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
func (it *SettlementPaymentSettledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SettlementPaymentSettled)
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
		it.Event = new(SettlementPaymentSettled)
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
func (it *SettlementPaymentSettledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SettlementPaymentSettledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SettlementPaymentSettled represents a PaymentSettled event raised by the Settlement contract.
type SettlementPaymentSettled struct {
	Merchant common.Address
	OrderId  [32]byte
	Device   common.Address
	Amount   *big.Int
	Nonce    *big.Int
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterPaymentSettled is a free log retrieval operation binding the contract event 0xeef4300dbd9217414481ce2ade0c4791c3b47c75b0c0d4b6bbe5fbb05260195f.
//
// Solidity: event PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)
func (_Settlement *SettlementFilterer) FilterPaymentSettled(opts *bind.FilterOpts, merchant []common.Address, orderId [][32]byte, device []common.Address) (*SettlementPaymentSettledIterator, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}
	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlement.contract.FilterLogs(opts, "PaymentSettled", merchantRule, orderIdRule, deviceRule)
	if err != nil {
		return nil, err
	}
	return &SettlementPaymentSettledIterator{contract: _Settlement.contract, event: "PaymentSettled", logs: logs, sub: sub}, nil
}

// WatchPaymentSettled is a free log subscription operation binding the contract event 0xeef4300dbd9217414481ce2ade0c4791c3b47c75b0c0d4b6bbe5fbb05260195f.
//
// Solidity: event PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)
func (_Settlement *SettlementFilterer) WatchPaymentSettled(opts *bind.WatchOpts, sink chan<- *SettlementPaymentSettled, merchant []common.Address, orderId [][32]byte, device []common.Address) (event.Subscription, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}
	var orderIdRule []interface{}
	for _, orderIdItem := range orderId {
		orderIdRule = append(orderIdRule, orderIdItem)
	}
	var deviceRule []interface{}
	for _, deviceItem := range device {
		deviceRule = append(deviceRule, deviceItem)
	}

	logs, sub, err := _Settlement.contract.WatchLogs(opts, "PaymentSettled", merchantRule, orderIdRule, deviceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SettlementPaymentSettled)
				if err := _Settlement.contract.UnpackLog(event, "PaymentSettled", log); err != nil {
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

// ParsePaymentSettled is a log parse operation binding the contract event 0xeef4300dbd9217414481ce2ade0c4791c3b47c75b0c0d4b6bbe5fbb05260195f.
//
// Solidity: event PaymentSettled(address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce)
func (_Settlement *SettlementFilterer) ParsePaymentSettled(log types.Log) (*SettlementPaymentSettled, error) {
	event := new(SettlementPaymentSettled)
	if err := _Settlement.contract.UnpackLog(event, "PaymentSettled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
