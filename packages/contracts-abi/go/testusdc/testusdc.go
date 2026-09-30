// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package testusdc

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

// TestusdcMetaData contains all meta data concerning the Testusdc contract.
var TestusdcMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"owner_\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"RETURN_DELAY\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"}],\"name\":\"acceptTransfer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"allowance\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"approve\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"balanceOf\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"decimals\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"}],\"name\":\"heldTransfer\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"heldAt\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"mint\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"name\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"nextHeldId\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"refusalMode\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"}],\"name\":\"rejectTransfer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"}],\"name\":\"returnToSender\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bool\",\"name\":\"enabled\",\"type\":\"bool\"}],\"name\":\"setRefusalMode\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"optedOut\",\"type\":\"bool\"}],\"name\":\"setTrustOptOut\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"trusted\",\"type\":\"bool\"}],\"name\":\"setTrustedSender\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"symbol\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"totalSupply\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"transfer\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"transferFrom\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"trustOptOut\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"trustedSender\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"wouldHold\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"Approval\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"enabled\",\"type\":\"bool\"}],\"name\":\"RefusalModeSet\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"Transfer\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"TransferAccepted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"TransferHeld\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"TransferRejected\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"TransferReturned\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"optedOut\",\"type\":\"bool\"}],\"name\":\"TrustOptOutSet\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"trusted\",\"type\":\"bool\"}],\"name\":\"TrustedSenderSet\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"AmountTooLarge\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InsufficientAllowance\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InsufficientBalance\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidRecipient\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotOwner\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotRecipient\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"RecipientRefusing\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ReturnNotYetAllowed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"UnknownHeldTransfer\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ZeroAddress\",\"type\":\"error\"}]",
}

// TestusdcABI is the input ABI used to generate the binding from.
// Deprecated: Use TestusdcMetaData.ABI instead.
var TestusdcABI = TestusdcMetaData.ABI

// Testusdc is an auto generated Go binding around an Ethereum contract.
type Testusdc struct {
	TestusdcCaller     // Read-only binding to the contract
	TestusdcTransactor // Write-only binding to the contract
	TestusdcFilterer   // Log filterer for contract events
}

// TestusdcCaller is an auto generated read-only Go binding around an Ethereum contract.
type TestusdcCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TestusdcTransactor is an auto generated write-only Go binding around an Ethereum contract.
type TestusdcTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TestusdcFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type TestusdcFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TestusdcSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type TestusdcSession struct {
	Contract     *Testusdc         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// TestusdcCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type TestusdcCallerSession struct {
	Contract *TestusdcCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// TestusdcTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type TestusdcTransactorSession struct {
	Contract     *TestusdcTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// TestusdcRaw is an auto generated low-level Go binding around an Ethereum contract.
type TestusdcRaw struct {
	Contract *Testusdc // Generic contract binding to access the raw methods on
}

// TestusdcCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type TestusdcCallerRaw struct {
	Contract *TestusdcCaller // Generic read-only contract binding to access the raw methods on
}

// TestusdcTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type TestusdcTransactorRaw struct {
	Contract *TestusdcTransactor // Generic write-only contract binding to access the raw methods on
}

// NewTestusdc creates a new instance of Testusdc, bound to a specific deployed contract.
func NewTestusdc(address common.Address, backend bind.ContractBackend) (*Testusdc, error) {
	contract, err := bindTestusdc(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Testusdc{TestusdcCaller: TestusdcCaller{contract: contract}, TestusdcTransactor: TestusdcTransactor{contract: contract}, TestusdcFilterer: TestusdcFilterer{contract: contract}}, nil
}

// NewTestusdcCaller creates a new read-only instance of Testusdc, bound to a specific deployed contract.
func NewTestusdcCaller(address common.Address, caller bind.ContractCaller) (*TestusdcCaller, error) {
	contract, err := bindTestusdc(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &TestusdcCaller{contract: contract}, nil
}

// NewTestusdcTransactor creates a new write-only instance of Testusdc, bound to a specific deployed contract.
func NewTestusdcTransactor(address common.Address, transactor bind.ContractTransactor) (*TestusdcTransactor, error) {
	contract, err := bindTestusdc(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &TestusdcTransactor{contract: contract}, nil
}

// NewTestusdcFilterer creates a new log filterer instance of Testusdc, bound to a specific deployed contract.
func NewTestusdcFilterer(address common.Address, filterer bind.ContractFilterer) (*TestusdcFilterer, error) {
	contract, err := bindTestusdc(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &TestusdcFilterer{contract: contract}, nil
}

// bindTestusdc binds a generic wrapper to an already deployed contract.
func bindTestusdc(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := TestusdcMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Testusdc *TestusdcRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Testusdc.Contract.TestusdcCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Testusdc *TestusdcRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Testusdc.Contract.TestusdcTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Testusdc *TestusdcRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Testusdc.Contract.TestusdcTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Testusdc *TestusdcCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Testusdc.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Testusdc *TestusdcTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Testusdc.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Testusdc *TestusdcTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Testusdc.Contract.contract.Transact(opts, method, params...)
}

// RETURNDELAY is a free data retrieval call binding the contract method 0x6038a3e5.
//
// Solidity: function RETURN_DELAY() view returns(uint256)
func (_Testusdc *TestusdcCaller) RETURNDELAY(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "RETURN_DELAY")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// RETURNDELAY is a free data retrieval call binding the contract method 0x6038a3e5.
//
// Solidity: function RETURN_DELAY() view returns(uint256)
func (_Testusdc *TestusdcSession) RETURNDELAY() (*big.Int, error) {
	return _Testusdc.Contract.RETURNDELAY(&_Testusdc.CallOpts)
}

// RETURNDELAY is a free data retrieval call binding the contract method 0x6038a3e5.
//
// Solidity: function RETURN_DELAY() view returns(uint256)
func (_Testusdc *TestusdcCallerSession) RETURNDELAY() (*big.Int, error) {
	return _Testusdc.Contract.RETURNDELAY(&_Testusdc.CallOpts)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address , address ) view returns(uint256)
func (_Testusdc *TestusdcCaller) Allowance(opts *bind.CallOpts, arg0 common.Address, arg1 common.Address) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "allowance", arg0, arg1)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address , address ) view returns(uint256)
func (_Testusdc *TestusdcSession) Allowance(arg0 common.Address, arg1 common.Address) (*big.Int, error) {
	return _Testusdc.Contract.Allowance(&_Testusdc.CallOpts, arg0, arg1)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address , address ) view returns(uint256)
func (_Testusdc *TestusdcCallerSession) Allowance(arg0 common.Address, arg1 common.Address) (*big.Int, error) {
	return _Testusdc.Contract.Allowance(&_Testusdc.CallOpts, arg0, arg1)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address ) view returns(uint256)
func (_Testusdc *TestusdcCaller) BalanceOf(opts *bind.CallOpts, arg0 common.Address) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "balanceOf", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address ) view returns(uint256)
func (_Testusdc *TestusdcSession) BalanceOf(arg0 common.Address) (*big.Int, error) {
	return _Testusdc.Contract.BalanceOf(&_Testusdc.CallOpts, arg0)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address ) view returns(uint256)
func (_Testusdc *TestusdcCallerSession) BalanceOf(arg0 common.Address) (*big.Int, error) {
	return _Testusdc.Contract.BalanceOf(&_Testusdc.CallOpts, arg0)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_Testusdc *TestusdcCaller) Decimals(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "decimals")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_Testusdc *TestusdcSession) Decimals() (uint8, error) {
	return _Testusdc.Contract.Decimals(&_Testusdc.CallOpts)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_Testusdc *TestusdcCallerSession) Decimals() (uint8, error) {
	return _Testusdc.Contract.Decimals(&_Testusdc.CallOpts)
}

// HeldTransfer is a free data retrieval call binding the contract method 0x4f41094f.
//
// Solidity: function heldTransfer(uint256 id) view returns(address from, address to, uint256 amount, uint256 heldAt)
func (_Testusdc *TestusdcCaller) HeldTransfer(opts *bind.CallOpts, id *big.Int) (struct {
	From   common.Address
	To     common.Address
	Amount *big.Int
	HeldAt *big.Int
}, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "heldTransfer", id)

	outstruct := new(struct {
		From   common.Address
		To     common.Address
		Amount *big.Int
		HeldAt *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.From = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.To = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)
	outstruct.Amount = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.HeldAt = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// HeldTransfer is a free data retrieval call binding the contract method 0x4f41094f.
//
// Solidity: function heldTransfer(uint256 id) view returns(address from, address to, uint256 amount, uint256 heldAt)
func (_Testusdc *TestusdcSession) HeldTransfer(id *big.Int) (struct {
	From   common.Address
	To     common.Address
	Amount *big.Int
	HeldAt *big.Int
}, error) {
	return _Testusdc.Contract.HeldTransfer(&_Testusdc.CallOpts, id)
}

// HeldTransfer is a free data retrieval call binding the contract method 0x4f41094f.
//
// Solidity: function heldTransfer(uint256 id) view returns(address from, address to, uint256 amount, uint256 heldAt)
func (_Testusdc *TestusdcCallerSession) HeldTransfer(id *big.Int) (struct {
	From   common.Address
	To     common.Address
	Amount *big.Int
	HeldAt *big.Int
}, error) {
	return _Testusdc.Contract.HeldTransfer(&_Testusdc.CallOpts, id)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_Testusdc *TestusdcCaller) Name(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "name")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_Testusdc *TestusdcSession) Name() (string, error) {
	return _Testusdc.Contract.Name(&_Testusdc.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_Testusdc *TestusdcCallerSession) Name() (string, error) {
	return _Testusdc.Contract.Name(&_Testusdc.CallOpts)
}

// NextHeldId is a free data retrieval call binding the contract method 0x3bae582e.
//
// Solidity: function nextHeldId() view returns(uint256)
func (_Testusdc *TestusdcCaller) NextHeldId(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "nextHeldId")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// NextHeldId is a free data retrieval call binding the contract method 0x3bae582e.
//
// Solidity: function nextHeldId() view returns(uint256)
func (_Testusdc *TestusdcSession) NextHeldId() (*big.Int, error) {
	return _Testusdc.Contract.NextHeldId(&_Testusdc.CallOpts)
}

// NextHeldId is a free data retrieval call binding the contract method 0x3bae582e.
//
// Solidity: function nextHeldId() view returns(uint256)
func (_Testusdc *TestusdcCallerSession) NextHeldId() (*big.Int, error) {
	return _Testusdc.Contract.NextHeldId(&_Testusdc.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Testusdc *TestusdcCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Testusdc *TestusdcSession) Owner() (common.Address, error) {
	return _Testusdc.Contract.Owner(&_Testusdc.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Testusdc *TestusdcCallerSession) Owner() (common.Address, error) {
	return _Testusdc.Contract.Owner(&_Testusdc.CallOpts)
}

// RefusalMode is a free data retrieval call binding the contract method 0x1fd9c246.
//
// Solidity: function refusalMode(address ) view returns(bool)
func (_Testusdc *TestusdcCaller) RefusalMode(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "refusalMode", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// RefusalMode is a free data retrieval call binding the contract method 0x1fd9c246.
//
// Solidity: function refusalMode(address ) view returns(bool)
func (_Testusdc *TestusdcSession) RefusalMode(arg0 common.Address) (bool, error) {
	return _Testusdc.Contract.RefusalMode(&_Testusdc.CallOpts, arg0)
}

// RefusalMode is a free data retrieval call binding the contract method 0x1fd9c246.
//
// Solidity: function refusalMode(address ) view returns(bool)
func (_Testusdc *TestusdcCallerSession) RefusalMode(arg0 common.Address) (bool, error) {
	return _Testusdc.Contract.RefusalMode(&_Testusdc.CallOpts, arg0)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_Testusdc *TestusdcCaller) Symbol(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "symbol")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_Testusdc *TestusdcSession) Symbol() (string, error) {
	return _Testusdc.Contract.Symbol(&_Testusdc.CallOpts)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_Testusdc *TestusdcCallerSession) Symbol() (string, error) {
	return _Testusdc.Contract.Symbol(&_Testusdc.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_Testusdc *TestusdcCaller) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "totalSupply")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_Testusdc *TestusdcSession) TotalSupply() (*big.Int, error) {
	return _Testusdc.Contract.TotalSupply(&_Testusdc.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_Testusdc *TestusdcCallerSession) TotalSupply() (*big.Int, error) {
	return _Testusdc.Contract.TotalSupply(&_Testusdc.CallOpts)
}

// TrustOptOut is a free data retrieval call binding the contract method 0x7bbf2fda.
//
// Solidity: function trustOptOut(address , address ) view returns(bool)
func (_Testusdc *TestusdcCaller) TrustOptOut(opts *bind.CallOpts, arg0 common.Address, arg1 common.Address) (bool, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "trustOptOut", arg0, arg1)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// TrustOptOut is a free data retrieval call binding the contract method 0x7bbf2fda.
//
// Solidity: function trustOptOut(address , address ) view returns(bool)
func (_Testusdc *TestusdcSession) TrustOptOut(arg0 common.Address, arg1 common.Address) (bool, error) {
	return _Testusdc.Contract.TrustOptOut(&_Testusdc.CallOpts, arg0, arg1)
}

// TrustOptOut is a free data retrieval call binding the contract method 0x7bbf2fda.
//
// Solidity: function trustOptOut(address , address ) view returns(bool)
func (_Testusdc *TestusdcCallerSession) TrustOptOut(arg0 common.Address, arg1 common.Address) (bool, error) {
	return _Testusdc.Contract.TrustOptOut(&_Testusdc.CallOpts, arg0, arg1)
}

// TrustedSender is a free data retrieval call binding the contract method 0x3f6ba415.
//
// Solidity: function trustedSender(address ) view returns(bool)
func (_Testusdc *TestusdcCaller) TrustedSender(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "trustedSender", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// TrustedSender is a free data retrieval call binding the contract method 0x3f6ba415.
//
// Solidity: function trustedSender(address ) view returns(bool)
func (_Testusdc *TestusdcSession) TrustedSender(arg0 common.Address) (bool, error) {
	return _Testusdc.Contract.TrustedSender(&_Testusdc.CallOpts, arg0)
}

// TrustedSender is a free data retrieval call binding the contract method 0x3f6ba415.
//
// Solidity: function trustedSender(address ) view returns(bool)
func (_Testusdc *TestusdcCallerSession) TrustedSender(arg0 common.Address) (bool, error) {
	return _Testusdc.Contract.TrustedSender(&_Testusdc.CallOpts, arg0)
}

// WouldHold is a free data retrieval call binding the contract method 0x502fb65f.
//
// Solidity: function wouldHold(address sender, address account) view returns(bool)
func (_Testusdc *TestusdcCaller) WouldHold(opts *bind.CallOpts, sender common.Address, account common.Address) (bool, error) {
	var out []interface{}
	err := _Testusdc.contract.Call(opts, &out, "wouldHold", sender, account)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// WouldHold is a free data retrieval call binding the contract method 0x502fb65f.
//
// Solidity: function wouldHold(address sender, address account) view returns(bool)
func (_Testusdc *TestusdcSession) WouldHold(sender common.Address, account common.Address) (bool, error) {
	return _Testusdc.Contract.WouldHold(&_Testusdc.CallOpts, sender, account)
}

// WouldHold is a free data retrieval call binding the contract method 0x502fb65f.
//
// Solidity: function wouldHold(address sender, address account) view returns(bool)
func (_Testusdc *TestusdcCallerSession) WouldHold(sender common.Address, account common.Address) (bool, error) {
	return _Testusdc.Contract.WouldHold(&_Testusdc.CallOpts, sender, account)
}

// AcceptTransfer is a paid mutator transaction binding the contract method 0x274fae7c.
//
// Solidity: function acceptTransfer(uint256 id) returns()
func (_Testusdc *TestusdcTransactor) AcceptTransfer(opts *bind.TransactOpts, id *big.Int) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "acceptTransfer", id)
}

// AcceptTransfer is a paid mutator transaction binding the contract method 0x274fae7c.
//
// Solidity: function acceptTransfer(uint256 id) returns()
func (_Testusdc *TestusdcSession) AcceptTransfer(id *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.AcceptTransfer(&_Testusdc.TransactOpts, id)
}

// AcceptTransfer is a paid mutator transaction binding the contract method 0x274fae7c.
//
// Solidity: function acceptTransfer(uint256 id) returns()
func (_Testusdc *TestusdcTransactorSession) AcceptTransfer(id *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.AcceptTransfer(&_Testusdc.TransactOpts, id)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 amount) returns(bool)
func (_Testusdc *TestusdcTransactor) Approve(opts *bind.TransactOpts, spender common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "approve", spender, amount)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 amount) returns(bool)
func (_Testusdc *TestusdcSession) Approve(spender common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.Approve(&_Testusdc.TransactOpts, spender, amount)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 amount) returns(bool)
func (_Testusdc *TestusdcTransactorSession) Approve(spender common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.Approve(&_Testusdc.TransactOpts, spender, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_Testusdc *TestusdcTransactor) Mint(opts *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "mint", to, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_Testusdc *TestusdcSession) Mint(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.Mint(&_Testusdc.TransactOpts, to, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_Testusdc *TestusdcTransactorSession) Mint(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.Mint(&_Testusdc.TransactOpts, to, amount)
}

// RejectTransfer is a paid mutator transaction binding the contract method 0x12effc32.
//
// Solidity: function rejectTransfer(uint256 id) returns()
func (_Testusdc *TestusdcTransactor) RejectTransfer(opts *bind.TransactOpts, id *big.Int) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "rejectTransfer", id)
}

// RejectTransfer is a paid mutator transaction binding the contract method 0x12effc32.
//
// Solidity: function rejectTransfer(uint256 id) returns()
func (_Testusdc *TestusdcSession) RejectTransfer(id *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.RejectTransfer(&_Testusdc.TransactOpts, id)
}

// RejectTransfer is a paid mutator transaction binding the contract method 0x12effc32.
//
// Solidity: function rejectTransfer(uint256 id) returns()
func (_Testusdc *TestusdcTransactorSession) RejectTransfer(id *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.RejectTransfer(&_Testusdc.TransactOpts, id)
}

// ReturnToSender is a paid mutator transaction binding the contract method 0x745ea1fe.
//
// Solidity: function returnToSender(uint256 id) returns()
func (_Testusdc *TestusdcTransactor) ReturnToSender(opts *bind.TransactOpts, id *big.Int) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "returnToSender", id)
}

// ReturnToSender is a paid mutator transaction binding the contract method 0x745ea1fe.
//
// Solidity: function returnToSender(uint256 id) returns()
func (_Testusdc *TestusdcSession) ReturnToSender(id *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.ReturnToSender(&_Testusdc.TransactOpts, id)
}

// ReturnToSender is a paid mutator transaction binding the contract method 0x745ea1fe.
//
// Solidity: function returnToSender(uint256 id) returns()
func (_Testusdc *TestusdcTransactorSession) ReturnToSender(id *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.ReturnToSender(&_Testusdc.TransactOpts, id)
}

// SetRefusalMode is a paid mutator transaction binding the contract method 0x35dac11d.
//
// Solidity: function setRefusalMode(bool enabled) returns()
func (_Testusdc *TestusdcTransactor) SetRefusalMode(opts *bind.TransactOpts, enabled bool) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "setRefusalMode", enabled)
}

// SetRefusalMode is a paid mutator transaction binding the contract method 0x35dac11d.
//
// Solidity: function setRefusalMode(bool enabled) returns()
func (_Testusdc *TestusdcSession) SetRefusalMode(enabled bool) (*types.Transaction, error) {
	return _Testusdc.Contract.SetRefusalMode(&_Testusdc.TransactOpts, enabled)
}

// SetRefusalMode is a paid mutator transaction binding the contract method 0x35dac11d.
//
// Solidity: function setRefusalMode(bool enabled) returns()
func (_Testusdc *TestusdcTransactorSession) SetRefusalMode(enabled bool) (*types.Transaction, error) {
	return _Testusdc.Contract.SetRefusalMode(&_Testusdc.TransactOpts, enabled)
}

// SetTrustOptOut is a paid mutator transaction binding the contract method 0x01bd3193.
//
// Solidity: function setTrustOptOut(address sender, bool optedOut) returns()
func (_Testusdc *TestusdcTransactor) SetTrustOptOut(opts *bind.TransactOpts, sender common.Address, optedOut bool) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "setTrustOptOut", sender, optedOut)
}

// SetTrustOptOut is a paid mutator transaction binding the contract method 0x01bd3193.
//
// Solidity: function setTrustOptOut(address sender, bool optedOut) returns()
func (_Testusdc *TestusdcSession) SetTrustOptOut(sender common.Address, optedOut bool) (*types.Transaction, error) {
	return _Testusdc.Contract.SetTrustOptOut(&_Testusdc.TransactOpts, sender, optedOut)
}

// SetTrustOptOut is a paid mutator transaction binding the contract method 0x01bd3193.
//
// Solidity: function setTrustOptOut(address sender, bool optedOut) returns()
func (_Testusdc *TestusdcTransactorSession) SetTrustOptOut(sender common.Address, optedOut bool) (*types.Transaction, error) {
	return _Testusdc.Contract.SetTrustOptOut(&_Testusdc.TransactOpts, sender, optedOut)
}

// SetTrustedSender is a paid mutator transaction binding the contract method 0xca208adb.
//
// Solidity: function setTrustedSender(address sender, bool trusted) returns()
func (_Testusdc *TestusdcTransactor) SetTrustedSender(opts *bind.TransactOpts, sender common.Address, trusted bool) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "setTrustedSender", sender, trusted)
}

// SetTrustedSender is a paid mutator transaction binding the contract method 0xca208adb.
//
// Solidity: function setTrustedSender(address sender, bool trusted) returns()
func (_Testusdc *TestusdcSession) SetTrustedSender(sender common.Address, trusted bool) (*types.Transaction, error) {
	return _Testusdc.Contract.SetTrustedSender(&_Testusdc.TransactOpts, sender, trusted)
}

// SetTrustedSender is a paid mutator transaction binding the contract method 0xca208adb.
//
// Solidity: function setTrustedSender(address sender, bool trusted) returns()
func (_Testusdc *TestusdcTransactorSession) SetTrustedSender(sender common.Address, trusted bool) (*types.Transaction, error) {
	return _Testusdc.Contract.SetTrustedSender(&_Testusdc.TransactOpts, sender, trusted)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 amount) returns(bool)
func (_Testusdc *TestusdcTransactor) Transfer(opts *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "transfer", to, amount)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 amount) returns(bool)
func (_Testusdc *TestusdcSession) Transfer(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.Transfer(&_Testusdc.TransactOpts, to, amount)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 amount) returns(bool)
func (_Testusdc *TestusdcTransactorSession) Transfer(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.Transfer(&_Testusdc.TransactOpts, to, amount)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 amount) returns(bool)
func (_Testusdc *TestusdcTransactor) TransferFrom(opts *bind.TransactOpts, from common.Address, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.contract.Transact(opts, "transferFrom", from, to, amount)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 amount) returns(bool)
func (_Testusdc *TestusdcSession) TransferFrom(from common.Address, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.TransferFrom(&_Testusdc.TransactOpts, from, to, amount)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 amount) returns(bool)
func (_Testusdc *TestusdcTransactorSession) TransferFrom(from common.Address, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc.Contract.TransferFrom(&_Testusdc.TransactOpts, from, to, amount)
}

// TestusdcApprovalIterator is returned from FilterApproval and is used to iterate over the raw logs and unpacked data for Approval events raised by the Testusdc contract.
type TestusdcApprovalIterator struct {
	Event *TestusdcApproval // Event containing the contract specifics and raw log

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
func (it *TestusdcApprovalIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestusdcApproval)
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
		it.Event = new(TestusdcApproval)
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
func (it *TestusdcApprovalIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestusdcApprovalIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestusdcApproval represents a Approval event raised by the Testusdc contract.
type TestusdcApproval struct {
	Owner   common.Address
	Spender common.Address
	Value   *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterApproval is a free log retrieval operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_Testusdc *TestusdcFilterer) FilterApproval(opts *bind.FilterOpts, owner []common.Address, spender []common.Address) (*TestusdcApprovalIterator, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var spenderRule []interface{}
	for _, spenderItem := range spender {
		spenderRule = append(spenderRule, spenderItem)
	}

	logs, sub, err := _Testusdc.contract.FilterLogs(opts, "Approval", ownerRule, spenderRule)
	if err != nil {
		return nil, err
	}
	return &TestusdcApprovalIterator{contract: _Testusdc.contract, event: "Approval", logs: logs, sub: sub}, nil
}

// WatchApproval is a free log subscription operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_Testusdc *TestusdcFilterer) WatchApproval(opts *bind.WatchOpts, sink chan<- *TestusdcApproval, owner []common.Address, spender []common.Address) (event.Subscription, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var spenderRule []interface{}
	for _, spenderItem := range spender {
		spenderRule = append(spenderRule, spenderItem)
	}

	logs, sub, err := _Testusdc.contract.WatchLogs(opts, "Approval", ownerRule, spenderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestusdcApproval)
				if err := _Testusdc.contract.UnpackLog(event, "Approval", log); err != nil {
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

// ParseApproval is a log parse operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_Testusdc *TestusdcFilterer) ParseApproval(log types.Log) (*TestusdcApproval, error) {
	event := new(TestusdcApproval)
	if err := _Testusdc.contract.UnpackLog(event, "Approval", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TestusdcRefusalModeSetIterator is returned from FilterRefusalModeSet and is used to iterate over the raw logs and unpacked data for RefusalModeSet events raised by the Testusdc contract.
type TestusdcRefusalModeSetIterator struct {
	Event *TestusdcRefusalModeSet // Event containing the contract specifics and raw log

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
func (it *TestusdcRefusalModeSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestusdcRefusalModeSet)
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
		it.Event = new(TestusdcRefusalModeSet)
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
func (it *TestusdcRefusalModeSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestusdcRefusalModeSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestusdcRefusalModeSet represents a RefusalModeSet event raised by the Testusdc contract.
type TestusdcRefusalModeSet struct {
	Account common.Address
	Enabled bool
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterRefusalModeSet is a free log retrieval operation binding the contract event 0xd501b806045698ed46fc71b8184d1049e7ab1464be5827633af82fc47aa4d02a.
//
// Solidity: event RefusalModeSet(address indexed account, bool enabled)
func (_Testusdc *TestusdcFilterer) FilterRefusalModeSet(opts *bind.FilterOpts, account []common.Address) (*TestusdcRefusalModeSetIterator, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _Testusdc.contract.FilterLogs(opts, "RefusalModeSet", accountRule)
	if err != nil {
		return nil, err
	}
	return &TestusdcRefusalModeSetIterator{contract: _Testusdc.contract, event: "RefusalModeSet", logs: logs, sub: sub}, nil
}

// WatchRefusalModeSet is a free log subscription operation binding the contract event 0xd501b806045698ed46fc71b8184d1049e7ab1464be5827633af82fc47aa4d02a.
//
// Solidity: event RefusalModeSet(address indexed account, bool enabled)
func (_Testusdc *TestusdcFilterer) WatchRefusalModeSet(opts *bind.WatchOpts, sink chan<- *TestusdcRefusalModeSet, account []common.Address) (event.Subscription, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _Testusdc.contract.WatchLogs(opts, "RefusalModeSet", accountRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestusdcRefusalModeSet)
				if err := _Testusdc.contract.UnpackLog(event, "RefusalModeSet", log); err != nil {
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

// ParseRefusalModeSet is a log parse operation binding the contract event 0xd501b806045698ed46fc71b8184d1049e7ab1464be5827633af82fc47aa4d02a.
//
// Solidity: event RefusalModeSet(address indexed account, bool enabled)
func (_Testusdc *TestusdcFilterer) ParseRefusalModeSet(log types.Log) (*TestusdcRefusalModeSet, error) {
	event := new(TestusdcRefusalModeSet)
	if err := _Testusdc.contract.UnpackLog(event, "RefusalModeSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TestusdcTransferIterator is returned from FilterTransfer and is used to iterate over the raw logs and unpacked data for Transfer events raised by the Testusdc contract.
type TestusdcTransferIterator struct {
	Event *TestusdcTransfer // Event containing the contract specifics and raw log

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
func (it *TestusdcTransferIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestusdcTransfer)
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
		it.Event = new(TestusdcTransfer)
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
func (it *TestusdcTransferIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestusdcTransferIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestusdcTransfer represents a Transfer event raised by the Testusdc contract.
type TestusdcTransfer struct {
	From  common.Address
	To    common.Address
	Value *big.Int
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterTransfer is a free log retrieval operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_Testusdc *TestusdcFilterer) FilterTransfer(opts *bind.FilterOpts, from []common.Address, to []common.Address) (*TestusdcTransferIterator, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.FilterLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &TestusdcTransferIterator{contract: _Testusdc.contract, event: "Transfer", logs: logs, sub: sub}, nil
}

// WatchTransfer is a free log subscription operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_Testusdc *TestusdcFilterer) WatchTransfer(opts *bind.WatchOpts, sink chan<- *TestusdcTransfer, from []common.Address, to []common.Address) (event.Subscription, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.WatchLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestusdcTransfer)
				if err := _Testusdc.contract.UnpackLog(event, "Transfer", log); err != nil {
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

// ParseTransfer is a log parse operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_Testusdc *TestusdcFilterer) ParseTransfer(log types.Log) (*TestusdcTransfer, error) {
	event := new(TestusdcTransfer)
	if err := _Testusdc.contract.UnpackLog(event, "Transfer", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TestusdcTransferAcceptedIterator is returned from FilterTransferAccepted and is used to iterate over the raw logs and unpacked data for TransferAccepted events raised by the Testusdc contract.
type TestusdcTransferAcceptedIterator struct {
	Event *TestusdcTransferAccepted // Event containing the contract specifics and raw log

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
func (it *TestusdcTransferAcceptedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestusdcTransferAccepted)
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
		it.Event = new(TestusdcTransferAccepted)
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
func (it *TestusdcTransferAcceptedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestusdcTransferAcceptedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestusdcTransferAccepted represents a TransferAccepted event raised by the Testusdc contract.
type TestusdcTransferAccepted struct {
	Id     *big.Int
	From   common.Address
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTransferAccepted is a free log retrieval operation binding the contract event 0x36d5947138c317fdfe6c54a68c6ee9786d30f033fde86488b466cea291e3b98e.
//
// Solidity: event TransferAccepted(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) FilterTransferAccepted(opts *bind.FilterOpts, id []*big.Int, from []common.Address, to []common.Address) (*TestusdcTransferAcceptedIterator, error) {

	var idRule []interface{}
	for _, idItem := range id {
		idRule = append(idRule, idItem)
	}
	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.FilterLogs(opts, "TransferAccepted", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &TestusdcTransferAcceptedIterator{contract: _Testusdc.contract, event: "TransferAccepted", logs: logs, sub: sub}, nil
}

// WatchTransferAccepted is a free log subscription operation binding the contract event 0x36d5947138c317fdfe6c54a68c6ee9786d30f033fde86488b466cea291e3b98e.
//
// Solidity: event TransferAccepted(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) WatchTransferAccepted(opts *bind.WatchOpts, sink chan<- *TestusdcTransferAccepted, id []*big.Int, from []common.Address, to []common.Address) (event.Subscription, error) {

	var idRule []interface{}
	for _, idItem := range id {
		idRule = append(idRule, idItem)
	}
	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.WatchLogs(opts, "TransferAccepted", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestusdcTransferAccepted)
				if err := _Testusdc.contract.UnpackLog(event, "TransferAccepted", log); err != nil {
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

// ParseTransferAccepted is a log parse operation binding the contract event 0x36d5947138c317fdfe6c54a68c6ee9786d30f033fde86488b466cea291e3b98e.
//
// Solidity: event TransferAccepted(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) ParseTransferAccepted(log types.Log) (*TestusdcTransferAccepted, error) {
	event := new(TestusdcTransferAccepted)
	if err := _Testusdc.contract.UnpackLog(event, "TransferAccepted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TestusdcTransferHeldIterator is returned from FilterTransferHeld and is used to iterate over the raw logs and unpacked data for TransferHeld events raised by the Testusdc contract.
type TestusdcTransferHeldIterator struct {
	Event *TestusdcTransferHeld // Event containing the contract specifics and raw log

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
func (it *TestusdcTransferHeldIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestusdcTransferHeld)
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
		it.Event = new(TestusdcTransferHeld)
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
func (it *TestusdcTransferHeldIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestusdcTransferHeldIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestusdcTransferHeld represents a TransferHeld event raised by the Testusdc contract.
type TestusdcTransferHeld struct {
	Id     *big.Int
	From   common.Address
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTransferHeld is a free log retrieval operation binding the contract event 0x55880df1f18b7149bf5aa7ee3957601319fe18920786b6d5b9964d972f318585.
//
// Solidity: event TransferHeld(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) FilterTransferHeld(opts *bind.FilterOpts, id []*big.Int, from []common.Address, to []common.Address) (*TestusdcTransferHeldIterator, error) {

	var idRule []interface{}
	for _, idItem := range id {
		idRule = append(idRule, idItem)
	}
	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.FilterLogs(opts, "TransferHeld", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &TestusdcTransferHeldIterator{contract: _Testusdc.contract, event: "TransferHeld", logs: logs, sub: sub}, nil
}

// WatchTransferHeld is a free log subscription operation binding the contract event 0x55880df1f18b7149bf5aa7ee3957601319fe18920786b6d5b9964d972f318585.
//
// Solidity: event TransferHeld(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) WatchTransferHeld(opts *bind.WatchOpts, sink chan<- *TestusdcTransferHeld, id []*big.Int, from []common.Address, to []common.Address) (event.Subscription, error) {

	var idRule []interface{}
	for _, idItem := range id {
		idRule = append(idRule, idItem)
	}
	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.WatchLogs(opts, "TransferHeld", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestusdcTransferHeld)
				if err := _Testusdc.contract.UnpackLog(event, "TransferHeld", log); err != nil {
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

// ParseTransferHeld is a log parse operation binding the contract event 0x55880df1f18b7149bf5aa7ee3957601319fe18920786b6d5b9964d972f318585.
//
// Solidity: event TransferHeld(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) ParseTransferHeld(log types.Log) (*TestusdcTransferHeld, error) {
	event := new(TestusdcTransferHeld)
	if err := _Testusdc.contract.UnpackLog(event, "TransferHeld", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TestusdcTransferRejectedIterator is returned from FilterTransferRejected and is used to iterate over the raw logs and unpacked data for TransferRejected events raised by the Testusdc contract.
type TestusdcTransferRejectedIterator struct {
	Event *TestusdcTransferRejected // Event containing the contract specifics and raw log

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
func (it *TestusdcTransferRejectedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestusdcTransferRejected)
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
		it.Event = new(TestusdcTransferRejected)
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
func (it *TestusdcTransferRejectedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestusdcTransferRejectedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestusdcTransferRejected represents a TransferRejected event raised by the Testusdc contract.
type TestusdcTransferRejected struct {
	Id     *big.Int
	From   common.Address
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTransferRejected is a free log retrieval operation binding the contract event 0x7300b9ea4d855f42b47170fdc5530a70cc7d53bf42aedfc6572392b07c3b867e.
//
// Solidity: event TransferRejected(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) FilterTransferRejected(opts *bind.FilterOpts, id []*big.Int, from []common.Address, to []common.Address) (*TestusdcTransferRejectedIterator, error) {

	var idRule []interface{}
	for _, idItem := range id {
		idRule = append(idRule, idItem)
	}
	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.FilterLogs(opts, "TransferRejected", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &TestusdcTransferRejectedIterator{contract: _Testusdc.contract, event: "TransferRejected", logs: logs, sub: sub}, nil
}

// WatchTransferRejected is a free log subscription operation binding the contract event 0x7300b9ea4d855f42b47170fdc5530a70cc7d53bf42aedfc6572392b07c3b867e.
//
// Solidity: event TransferRejected(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) WatchTransferRejected(opts *bind.WatchOpts, sink chan<- *TestusdcTransferRejected, id []*big.Int, from []common.Address, to []common.Address) (event.Subscription, error) {

	var idRule []interface{}
	for _, idItem := range id {
		idRule = append(idRule, idItem)
	}
	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.WatchLogs(opts, "TransferRejected", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestusdcTransferRejected)
				if err := _Testusdc.contract.UnpackLog(event, "TransferRejected", log); err != nil {
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

// ParseTransferRejected is a log parse operation binding the contract event 0x7300b9ea4d855f42b47170fdc5530a70cc7d53bf42aedfc6572392b07c3b867e.
//
// Solidity: event TransferRejected(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) ParseTransferRejected(log types.Log) (*TestusdcTransferRejected, error) {
	event := new(TestusdcTransferRejected)
	if err := _Testusdc.contract.UnpackLog(event, "TransferRejected", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TestusdcTransferReturnedIterator is returned from FilterTransferReturned and is used to iterate over the raw logs and unpacked data for TransferReturned events raised by the Testusdc contract.
type TestusdcTransferReturnedIterator struct {
	Event *TestusdcTransferReturned // Event containing the contract specifics and raw log

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
func (it *TestusdcTransferReturnedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestusdcTransferReturned)
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
		it.Event = new(TestusdcTransferReturned)
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
func (it *TestusdcTransferReturnedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestusdcTransferReturnedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestusdcTransferReturned represents a TransferReturned event raised by the Testusdc contract.
type TestusdcTransferReturned struct {
	Id     *big.Int
	From   common.Address
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTransferReturned is a free log retrieval operation binding the contract event 0x7eff2ba5bd86e7ce15c22347700c00018a65e80056451e47da00bfc2231d1bdc.
//
// Solidity: event TransferReturned(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) FilterTransferReturned(opts *bind.FilterOpts, id []*big.Int, from []common.Address, to []common.Address) (*TestusdcTransferReturnedIterator, error) {

	var idRule []interface{}
	for _, idItem := range id {
		idRule = append(idRule, idItem)
	}
	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.FilterLogs(opts, "TransferReturned", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &TestusdcTransferReturnedIterator{contract: _Testusdc.contract, event: "TransferReturned", logs: logs, sub: sub}, nil
}

// WatchTransferReturned is a free log subscription operation binding the contract event 0x7eff2ba5bd86e7ce15c22347700c00018a65e80056451e47da00bfc2231d1bdc.
//
// Solidity: event TransferReturned(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) WatchTransferReturned(opts *bind.WatchOpts, sink chan<- *TestusdcTransferReturned, id []*big.Int, from []common.Address, to []common.Address) (event.Subscription, error) {

	var idRule []interface{}
	for _, idItem := range id {
		idRule = append(idRule, idItem)
	}
	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc.contract.WatchLogs(opts, "TransferReturned", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestusdcTransferReturned)
				if err := _Testusdc.contract.UnpackLog(event, "TransferReturned", log); err != nil {
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

// ParseTransferReturned is a log parse operation binding the contract event 0x7eff2ba5bd86e7ce15c22347700c00018a65e80056451e47da00bfc2231d1bdc.
//
// Solidity: event TransferReturned(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc *TestusdcFilterer) ParseTransferReturned(log types.Log) (*TestusdcTransferReturned, error) {
	event := new(TestusdcTransferReturned)
	if err := _Testusdc.contract.UnpackLog(event, "TransferReturned", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TestusdcTrustOptOutSetIterator is returned from FilterTrustOptOutSet and is used to iterate over the raw logs and unpacked data for TrustOptOutSet events raised by the Testusdc contract.
type TestusdcTrustOptOutSetIterator struct {
	Event *TestusdcTrustOptOutSet // Event containing the contract specifics and raw log

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
func (it *TestusdcTrustOptOutSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestusdcTrustOptOutSet)
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
		it.Event = new(TestusdcTrustOptOutSet)
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
func (it *TestusdcTrustOptOutSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestusdcTrustOptOutSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestusdcTrustOptOutSet represents a TrustOptOutSet event raised by the Testusdc contract.
type TestusdcTrustOptOutSet struct {
	Account  common.Address
	Sender   common.Address
	OptedOut bool
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterTrustOptOutSet is a free log retrieval operation binding the contract event 0xb4ebf946b338521727979a53e3fa0aaf42f117988596cd5184d395493a919567.
//
// Solidity: event TrustOptOutSet(address indexed account, address indexed sender, bool optedOut)
func (_Testusdc *TestusdcFilterer) FilterTrustOptOutSet(opts *bind.FilterOpts, account []common.Address, sender []common.Address) (*TestusdcTrustOptOutSetIterator, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _Testusdc.contract.FilterLogs(opts, "TrustOptOutSet", accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return &TestusdcTrustOptOutSetIterator{contract: _Testusdc.contract, event: "TrustOptOutSet", logs: logs, sub: sub}, nil
}

// WatchTrustOptOutSet is a free log subscription operation binding the contract event 0xb4ebf946b338521727979a53e3fa0aaf42f117988596cd5184d395493a919567.
//
// Solidity: event TrustOptOutSet(address indexed account, address indexed sender, bool optedOut)
func (_Testusdc *TestusdcFilterer) WatchTrustOptOutSet(opts *bind.WatchOpts, sink chan<- *TestusdcTrustOptOutSet, account []common.Address, sender []common.Address) (event.Subscription, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _Testusdc.contract.WatchLogs(opts, "TrustOptOutSet", accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestusdcTrustOptOutSet)
				if err := _Testusdc.contract.UnpackLog(event, "TrustOptOutSet", log); err != nil {
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

// ParseTrustOptOutSet is a log parse operation binding the contract event 0xb4ebf946b338521727979a53e3fa0aaf42f117988596cd5184d395493a919567.
//
// Solidity: event TrustOptOutSet(address indexed account, address indexed sender, bool optedOut)
func (_Testusdc *TestusdcFilterer) ParseTrustOptOutSet(log types.Log) (*TestusdcTrustOptOutSet, error) {
	event := new(TestusdcTrustOptOutSet)
	if err := _Testusdc.contract.UnpackLog(event, "TrustOptOutSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TestusdcTrustedSenderSetIterator is returned from FilterTrustedSenderSet and is used to iterate over the raw logs and unpacked data for TrustedSenderSet events raised by the Testusdc contract.
type TestusdcTrustedSenderSetIterator struct {
	Event *TestusdcTrustedSenderSet // Event containing the contract specifics and raw log

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
func (it *TestusdcTrustedSenderSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestusdcTrustedSenderSet)
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
		it.Event = new(TestusdcTrustedSenderSet)
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
func (it *TestusdcTrustedSenderSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestusdcTrustedSenderSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestusdcTrustedSenderSet represents a TrustedSenderSet event raised by the Testusdc contract.
type TestusdcTrustedSenderSet struct {
	Sender  common.Address
	Trusted bool
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterTrustedSenderSet is a free log retrieval operation binding the contract event 0xc6d8fe9203493650347355f292641f1329830533307341fc68759d417e654765.
//
// Solidity: event TrustedSenderSet(address indexed sender, bool trusted)
func (_Testusdc *TestusdcFilterer) FilterTrustedSenderSet(opts *bind.FilterOpts, sender []common.Address) (*TestusdcTrustedSenderSetIterator, error) {

	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _Testusdc.contract.FilterLogs(opts, "TrustedSenderSet", senderRule)
	if err != nil {
		return nil, err
	}
	return &TestusdcTrustedSenderSetIterator{contract: _Testusdc.contract, event: "TrustedSenderSet", logs: logs, sub: sub}, nil
}

// WatchTrustedSenderSet is a free log subscription operation binding the contract event 0xc6d8fe9203493650347355f292641f1329830533307341fc68759d417e654765.
//
// Solidity: event TrustedSenderSet(address indexed sender, bool trusted)
func (_Testusdc *TestusdcFilterer) WatchTrustedSenderSet(opts *bind.WatchOpts, sink chan<- *TestusdcTrustedSenderSet, sender []common.Address) (event.Subscription, error) {

	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _Testusdc.contract.WatchLogs(opts, "TrustedSenderSet", senderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestusdcTrustedSenderSet)
				if err := _Testusdc.contract.UnpackLog(event, "TrustedSenderSet", log); err != nil {
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

// ParseTrustedSenderSet is a log parse operation binding the contract event 0xc6d8fe9203493650347355f292641f1329830533307341fc68759d417e654765.
//
// Solidity: event TrustedSenderSet(address indexed sender, bool trusted)
func (_Testusdc *TestusdcFilterer) ParseTrustedSenderSet(log types.Log) (*TestusdcTrustedSenderSet, error) {
	event := new(TestusdcTrustedSenderSet)
	if err := _Testusdc.contract.UnpackLog(event, "TrustedSenderSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
