// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package gassponsor

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

// GasSponsorRequest is an auto generated low-level Go binding around an user-defined struct.
type GasSponsorRequest struct {
	User        common.Address
	Target      common.Address
	Data        []byte
	Gas         *big.Int
	ValidBefore uint64
	Nonce       [32]byte
}

// GassponsorMetaData contains all meta data concerning the Gassponsor contract.
var GassponsorMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"owner_\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"DOMAIN_SEPARATOR\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"MAX_LIFE\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"REQUEST_TYPEHASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"bytes4\",\"name\":\"\",\"type\":\"bytes4\"}],\"name\":\"actions\",\"outputs\":[{\"internalType\":\"enumGasSponsor.Kind\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"},{\"internalType\":\"uint256\",\"name\":\"gas\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"validBefore\",\"type\":\"uint64\"},{\"internalType\":\"bytes32\",\"name\":\"nonce\",\"type\":\"bytes32\"}],\"internalType\":\"structGasSponsor.Request\",\"name\":\"request\",\"type\":\"tuple\"},{\"internalType\":\"bytes\",\"name\":\"signature\",\"type\":\"bytes\"}],\"name\":\"execute\",\"outputs\":[{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"relays\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"},{\"internalType\":\"uint256\",\"name\":\"gas\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"validBefore\",\"type\":\"uint64\"},{\"internalType\":\"bytes32\",\"name\":\"nonce\",\"type\":\"bytes32\"}],\"internalType\":\"structGasSponsor.Request\",\"name\":\"request\",\"type\":\"tuple\"}],\"name\":\"requestDigest\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bytes4[]\",\"name\":\"selectors\",\"type\":\"bytes4[]\"},{\"internalType\":\"enumGasSponsor.Kind\",\"name\":\"kind\",\"type\":\"uint8\"}],\"name\":\"setActions\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"relay\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"allowed\",\"type\":\"bool\"}],\"name\":\"setRelay\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"name\":\"sponsor\",\"outputs\":[{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"used\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"bytes4\",\"name\":\"selector\",\"type\":\"bytes4\"},{\"indexed\":false,\"internalType\":\"enumGasSponsor.Kind\",\"name\":\"kind\",\"type\":\"uint8\"}],\"name\":\"ActionSet\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"relay\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"allowed\",\"type\":\"bool\"}],\"name\":\"RelaySet\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"relay\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"bytes4\",\"name\":\"selector\",\"type\":\"bytes4\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"nonce\",\"type\":\"bytes32\"}],\"name\":\"Sponsored\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"ActionNotAllowed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidParameters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidSignature\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotEnoughGas\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotOwner\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotRelay\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"Reentered\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"RequestExpired\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"RequestTooLong\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"RequestUsed\",\"type\":\"error\"}]",
}

// GassponsorABI is the input ABI used to generate the binding from.
// Deprecated: Use GassponsorMetaData.ABI instead.
var GassponsorABI = GassponsorMetaData.ABI

// Gassponsor is an auto generated Go binding around an Ethereum contract.
type Gassponsor struct {
	GassponsorCaller     // Read-only binding to the contract
	GassponsorTransactor // Write-only binding to the contract
	GassponsorFilterer   // Log filterer for contract events
}

// GassponsorCaller is an auto generated read-only Go binding around an Ethereum contract.
type GassponsorCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GassponsorTransactor is an auto generated write-only Go binding around an Ethereum contract.
type GassponsorTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GassponsorFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type GassponsorFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GassponsorSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type GassponsorSession struct {
	Contract     *Gassponsor       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// GassponsorCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type GassponsorCallerSession struct {
	Contract *GassponsorCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// GassponsorTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type GassponsorTransactorSession struct {
	Contract     *GassponsorTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// GassponsorRaw is an auto generated low-level Go binding around an Ethereum contract.
type GassponsorRaw struct {
	Contract *Gassponsor // Generic contract binding to access the raw methods on
}

// GassponsorCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type GassponsorCallerRaw struct {
	Contract *GassponsorCaller // Generic read-only contract binding to access the raw methods on
}

// GassponsorTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type GassponsorTransactorRaw struct {
	Contract *GassponsorTransactor // Generic write-only contract binding to access the raw methods on
}

// NewGassponsor creates a new instance of Gassponsor, bound to a specific deployed contract.
func NewGassponsor(address common.Address, backend bind.ContractBackend) (*Gassponsor, error) {
	contract, err := bindGassponsor(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Gassponsor{GassponsorCaller: GassponsorCaller{contract: contract}, GassponsorTransactor: GassponsorTransactor{contract: contract}, GassponsorFilterer: GassponsorFilterer{contract: contract}}, nil
}

// NewGassponsorCaller creates a new read-only instance of Gassponsor, bound to a specific deployed contract.
func NewGassponsorCaller(address common.Address, caller bind.ContractCaller) (*GassponsorCaller, error) {
	contract, err := bindGassponsor(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &GassponsorCaller{contract: contract}, nil
}

// NewGassponsorTransactor creates a new write-only instance of Gassponsor, bound to a specific deployed contract.
func NewGassponsorTransactor(address common.Address, transactor bind.ContractTransactor) (*GassponsorTransactor, error) {
	contract, err := bindGassponsor(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &GassponsorTransactor{contract: contract}, nil
}

// NewGassponsorFilterer creates a new log filterer instance of Gassponsor, bound to a specific deployed contract.
func NewGassponsorFilterer(address common.Address, filterer bind.ContractFilterer) (*GassponsorFilterer, error) {
	contract, err := bindGassponsor(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &GassponsorFilterer{contract: contract}, nil
}

// bindGassponsor binds a generic wrapper to an already deployed contract.
func bindGassponsor(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := GassponsorMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Gassponsor *GassponsorRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Gassponsor.Contract.GassponsorCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Gassponsor *GassponsorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Gassponsor.Contract.GassponsorTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Gassponsor *GassponsorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Gassponsor.Contract.GassponsorTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Gassponsor *GassponsorCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Gassponsor.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Gassponsor *GassponsorTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Gassponsor.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Gassponsor *GassponsorTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Gassponsor.Contract.contract.Transact(opts, method, params...)
}

// DOMAINSEPARATOR is a free data retrieval call binding the contract method 0x3644e515.
//
// Solidity: function DOMAIN_SEPARATOR() view returns(bytes32)
func (_Gassponsor *GassponsorCaller) DOMAINSEPARATOR(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Gassponsor.contract.Call(opts, &out, "DOMAIN_SEPARATOR")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// DOMAINSEPARATOR is a free data retrieval call binding the contract method 0x3644e515.
//
// Solidity: function DOMAIN_SEPARATOR() view returns(bytes32)
func (_Gassponsor *GassponsorSession) DOMAINSEPARATOR() ([32]byte, error) {
	return _Gassponsor.Contract.DOMAINSEPARATOR(&_Gassponsor.CallOpts)
}

// DOMAINSEPARATOR is a free data retrieval call binding the contract method 0x3644e515.
//
// Solidity: function DOMAIN_SEPARATOR() view returns(bytes32)
func (_Gassponsor *GassponsorCallerSession) DOMAINSEPARATOR() ([32]byte, error) {
	return _Gassponsor.Contract.DOMAINSEPARATOR(&_Gassponsor.CallOpts)
}

// MAXLIFE is a free data retrieval call binding the contract method 0x6fa9d0d0.
//
// Solidity: function MAX_LIFE() view returns(uint64)
func (_Gassponsor *GassponsorCaller) MAXLIFE(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _Gassponsor.contract.Call(opts, &out, "MAX_LIFE")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// MAXLIFE is a free data retrieval call binding the contract method 0x6fa9d0d0.
//
// Solidity: function MAX_LIFE() view returns(uint64)
func (_Gassponsor *GassponsorSession) MAXLIFE() (uint64, error) {
	return _Gassponsor.Contract.MAXLIFE(&_Gassponsor.CallOpts)
}

// MAXLIFE is a free data retrieval call binding the contract method 0x6fa9d0d0.
//
// Solidity: function MAX_LIFE() view returns(uint64)
func (_Gassponsor *GassponsorCallerSession) MAXLIFE() (uint64, error) {
	return _Gassponsor.Contract.MAXLIFE(&_Gassponsor.CallOpts)
}

// REQUESTTYPEHASH is a free data retrieval call binding the contract method 0x9e39b73e.
//
// Solidity: function REQUEST_TYPEHASH() view returns(bytes32)
func (_Gassponsor *GassponsorCaller) REQUESTTYPEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Gassponsor.contract.Call(opts, &out, "REQUEST_TYPEHASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// REQUESTTYPEHASH is a free data retrieval call binding the contract method 0x9e39b73e.
//
// Solidity: function REQUEST_TYPEHASH() view returns(bytes32)
func (_Gassponsor *GassponsorSession) REQUESTTYPEHASH() ([32]byte, error) {
	return _Gassponsor.Contract.REQUESTTYPEHASH(&_Gassponsor.CallOpts)
}

// REQUESTTYPEHASH is a free data retrieval call binding the contract method 0x9e39b73e.
//
// Solidity: function REQUEST_TYPEHASH() view returns(bytes32)
func (_Gassponsor *GassponsorCallerSession) REQUESTTYPEHASH() ([32]byte, error) {
	return _Gassponsor.Contract.REQUESTTYPEHASH(&_Gassponsor.CallOpts)
}

// Actions is a free data retrieval call binding the contract method 0xc7619cec.
//
// Solidity: function actions(address , bytes4 ) view returns(uint8)
func (_Gassponsor *GassponsorCaller) Actions(opts *bind.CallOpts, arg0 common.Address, arg1 [4]byte) (uint8, error) {
	var out []interface{}
	err := _Gassponsor.contract.Call(opts, &out, "actions", arg0, arg1)

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// Actions is a free data retrieval call binding the contract method 0xc7619cec.
//
// Solidity: function actions(address , bytes4 ) view returns(uint8)
func (_Gassponsor *GassponsorSession) Actions(arg0 common.Address, arg1 [4]byte) (uint8, error) {
	return _Gassponsor.Contract.Actions(&_Gassponsor.CallOpts, arg0, arg1)
}

// Actions is a free data retrieval call binding the contract method 0xc7619cec.
//
// Solidity: function actions(address , bytes4 ) view returns(uint8)
func (_Gassponsor *GassponsorCallerSession) Actions(arg0 common.Address, arg1 [4]byte) (uint8, error) {
	return _Gassponsor.Contract.Actions(&_Gassponsor.CallOpts, arg0, arg1)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Gassponsor *GassponsorCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Gassponsor.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Gassponsor *GassponsorSession) Owner() (common.Address, error) {
	return _Gassponsor.Contract.Owner(&_Gassponsor.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Gassponsor *GassponsorCallerSession) Owner() (common.Address, error) {
	return _Gassponsor.Contract.Owner(&_Gassponsor.CallOpts)
}

// Relays is a free data retrieval call binding the contract method 0xcb8d406f.
//
// Solidity: function relays(address ) view returns(bool)
func (_Gassponsor *GassponsorCaller) Relays(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var out []interface{}
	err := _Gassponsor.contract.Call(opts, &out, "relays", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// Relays is a free data retrieval call binding the contract method 0xcb8d406f.
//
// Solidity: function relays(address ) view returns(bool)
func (_Gassponsor *GassponsorSession) Relays(arg0 common.Address) (bool, error) {
	return _Gassponsor.Contract.Relays(&_Gassponsor.CallOpts, arg0)
}

// Relays is a free data retrieval call binding the contract method 0xcb8d406f.
//
// Solidity: function relays(address ) view returns(bool)
func (_Gassponsor *GassponsorCallerSession) Relays(arg0 common.Address) (bool, error) {
	return _Gassponsor.Contract.Relays(&_Gassponsor.CallOpts, arg0)
}

// RequestDigest is a free data retrieval call binding the contract method 0x6602c23c.
//
// Solidity: function requestDigest((address,address,bytes,uint256,uint64,bytes32) request) view returns(bytes32)
func (_Gassponsor *GassponsorCaller) RequestDigest(opts *bind.CallOpts, request GasSponsorRequest) ([32]byte, error) {
	var out []interface{}
	err := _Gassponsor.contract.Call(opts, &out, "requestDigest", request)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// RequestDigest is a free data retrieval call binding the contract method 0x6602c23c.
//
// Solidity: function requestDigest((address,address,bytes,uint256,uint64,bytes32) request) view returns(bytes32)
func (_Gassponsor *GassponsorSession) RequestDigest(request GasSponsorRequest) ([32]byte, error) {
	return _Gassponsor.Contract.RequestDigest(&_Gassponsor.CallOpts, request)
}

// RequestDigest is a free data retrieval call binding the contract method 0x6602c23c.
//
// Solidity: function requestDigest((address,address,bytes,uint256,uint64,bytes32) request) view returns(bytes32)
func (_Gassponsor *GassponsorCallerSession) RequestDigest(request GasSponsorRequest) ([32]byte, error) {
	return _Gassponsor.Contract.RequestDigest(&_Gassponsor.CallOpts, request)
}

// Used is a free data retrieval call binding the contract method 0x41a1c110.
//
// Solidity: function used(address , bytes32 ) view returns(bool)
func (_Gassponsor *GassponsorCaller) Used(opts *bind.CallOpts, arg0 common.Address, arg1 [32]byte) (bool, error) {
	var out []interface{}
	err := _Gassponsor.contract.Call(opts, &out, "used", arg0, arg1)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// Used is a free data retrieval call binding the contract method 0x41a1c110.
//
// Solidity: function used(address , bytes32 ) view returns(bool)
func (_Gassponsor *GassponsorSession) Used(arg0 common.Address, arg1 [32]byte) (bool, error) {
	return _Gassponsor.Contract.Used(&_Gassponsor.CallOpts, arg0, arg1)
}

// Used is a free data retrieval call binding the contract method 0x41a1c110.
//
// Solidity: function used(address , bytes32 ) view returns(bool)
func (_Gassponsor *GassponsorCallerSession) Used(arg0 common.Address, arg1 [32]byte) (bool, error) {
	return _Gassponsor.Contract.Used(&_Gassponsor.CallOpts, arg0, arg1)
}

// Execute is a paid mutator transaction binding the contract method 0xf830b447.
//
// Solidity: function execute((address,address,bytes,uint256,uint64,bytes32) request, bytes signature) returns(bytes)
func (_Gassponsor *GassponsorTransactor) Execute(opts *bind.TransactOpts, request GasSponsorRequest, signature []byte) (*types.Transaction, error) {
	return _Gassponsor.contract.Transact(opts, "execute", request, signature)
}

// Execute is a paid mutator transaction binding the contract method 0xf830b447.
//
// Solidity: function execute((address,address,bytes,uint256,uint64,bytes32) request, bytes signature) returns(bytes)
func (_Gassponsor *GassponsorSession) Execute(request GasSponsorRequest, signature []byte) (*types.Transaction, error) {
	return _Gassponsor.Contract.Execute(&_Gassponsor.TransactOpts, request, signature)
}

// Execute is a paid mutator transaction binding the contract method 0xf830b447.
//
// Solidity: function execute((address,address,bytes,uint256,uint64,bytes32) request, bytes signature) returns(bytes)
func (_Gassponsor *GassponsorTransactorSession) Execute(request GasSponsorRequest, signature []byte) (*types.Transaction, error) {
	return _Gassponsor.Contract.Execute(&_Gassponsor.TransactOpts, request, signature)
}

// SetActions is a paid mutator transaction binding the contract method 0x9175dfb7.
//
// Solidity: function setActions(address target, bytes4[] selectors, uint8 kind) returns()
func (_Gassponsor *GassponsorTransactor) SetActions(opts *bind.TransactOpts, target common.Address, selectors [][4]byte, kind uint8) (*types.Transaction, error) {
	return _Gassponsor.contract.Transact(opts, "setActions", target, selectors, kind)
}

// SetActions is a paid mutator transaction binding the contract method 0x9175dfb7.
//
// Solidity: function setActions(address target, bytes4[] selectors, uint8 kind) returns()
func (_Gassponsor *GassponsorSession) SetActions(target common.Address, selectors [][4]byte, kind uint8) (*types.Transaction, error) {
	return _Gassponsor.Contract.SetActions(&_Gassponsor.TransactOpts, target, selectors, kind)
}

// SetActions is a paid mutator transaction binding the contract method 0x9175dfb7.
//
// Solidity: function setActions(address target, bytes4[] selectors, uint8 kind) returns()
func (_Gassponsor *GassponsorTransactorSession) SetActions(target common.Address, selectors [][4]byte, kind uint8) (*types.Transaction, error) {
	return _Gassponsor.Contract.SetActions(&_Gassponsor.TransactOpts, target, selectors, kind)
}

// SetRelay is a paid mutator transaction binding the contract method 0x4e82449d.
//
// Solidity: function setRelay(address relay, bool allowed) returns()
func (_Gassponsor *GassponsorTransactor) SetRelay(opts *bind.TransactOpts, relay common.Address, allowed bool) (*types.Transaction, error) {
	return _Gassponsor.contract.Transact(opts, "setRelay", relay, allowed)
}

// SetRelay is a paid mutator transaction binding the contract method 0x4e82449d.
//
// Solidity: function setRelay(address relay, bool allowed) returns()
func (_Gassponsor *GassponsorSession) SetRelay(relay common.Address, allowed bool) (*types.Transaction, error) {
	return _Gassponsor.Contract.SetRelay(&_Gassponsor.TransactOpts, relay, allowed)
}

// SetRelay is a paid mutator transaction binding the contract method 0x4e82449d.
//
// Solidity: function setRelay(address relay, bool allowed) returns()
func (_Gassponsor *GassponsorTransactorSession) SetRelay(relay common.Address, allowed bool) (*types.Transaction, error) {
	return _Gassponsor.Contract.SetRelay(&_Gassponsor.TransactOpts, relay, allowed)
}

// Sponsor is a paid mutator transaction binding the contract method 0xea1eb800.
//
// Solidity: function sponsor(address target, bytes data) returns(bytes)
func (_Gassponsor *GassponsorTransactor) Sponsor(opts *bind.TransactOpts, target common.Address, data []byte) (*types.Transaction, error) {
	return _Gassponsor.contract.Transact(opts, "sponsor", target, data)
}

// Sponsor is a paid mutator transaction binding the contract method 0xea1eb800.
//
// Solidity: function sponsor(address target, bytes data) returns(bytes)
func (_Gassponsor *GassponsorSession) Sponsor(target common.Address, data []byte) (*types.Transaction, error) {
	return _Gassponsor.Contract.Sponsor(&_Gassponsor.TransactOpts, target, data)
}

// Sponsor is a paid mutator transaction binding the contract method 0xea1eb800.
//
// Solidity: function sponsor(address target, bytes data) returns(bytes)
func (_Gassponsor *GassponsorTransactorSession) Sponsor(target common.Address, data []byte) (*types.Transaction, error) {
	return _Gassponsor.Contract.Sponsor(&_Gassponsor.TransactOpts, target, data)
}

// GassponsorActionSetIterator is returned from FilterActionSet and is used to iterate over the raw logs and unpacked data for ActionSet events raised by the Gassponsor contract.
type GassponsorActionSetIterator struct {
	Event *GassponsorActionSet // Event containing the contract specifics and raw log

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
func (it *GassponsorActionSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GassponsorActionSet)
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
		it.Event = new(GassponsorActionSet)
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
func (it *GassponsorActionSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GassponsorActionSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GassponsorActionSet represents a ActionSet event raised by the Gassponsor contract.
type GassponsorActionSet struct {
	Target   common.Address
	Selector [4]byte
	Kind     uint8
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterActionSet is a free log retrieval operation binding the contract event 0x218295e8180c2841862428ada645ff98055c98e0a8ecceefc2ee63f513d91b06.
//
// Solidity: event ActionSet(address indexed target, bytes4 indexed selector, uint8 kind)
func (_Gassponsor *GassponsorFilterer) FilterActionSet(opts *bind.FilterOpts, target []common.Address, selector [][4]byte) (*GassponsorActionSetIterator, error) {

	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}
	var selectorRule []interface{}
	for _, selectorItem := range selector {
		selectorRule = append(selectorRule, selectorItem)
	}

	logs, sub, err := _Gassponsor.contract.FilterLogs(opts, "ActionSet", targetRule, selectorRule)
	if err != nil {
		return nil, err
	}
	return &GassponsorActionSetIterator{contract: _Gassponsor.contract, event: "ActionSet", logs: logs, sub: sub}, nil
}

// WatchActionSet is a free log subscription operation binding the contract event 0x218295e8180c2841862428ada645ff98055c98e0a8ecceefc2ee63f513d91b06.
//
// Solidity: event ActionSet(address indexed target, bytes4 indexed selector, uint8 kind)
func (_Gassponsor *GassponsorFilterer) WatchActionSet(opts *bind.WatchOpts, sink chan<- *GassponsorActionSet, target []common.Address, selector [][4]byte) (event.Subscription, error) {

	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}
	var selectorRule []interface{}
	for _, selectorItem := range selector {
		selectorRule = append(selectorRule, selectorItem)
	}

	logs, sub, err := _Gassponsor.contract.WatchLogs(opts, "ActionSet", targetRule, selectorRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GassponsorActionSet)
				if err := _Gassponsor.contract.UnpackLog(event, "ActionSet", log); err != nil {
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

// ParseActionSet is a log parse operation binding the contract event 0x218295e8180c2841862428ada645ff98055c98e0a8ecceefc2ee63f513d91b06.
//
// Solidity: event ActionSet(address indexed target, bytes4 indexed selector, uint8 kind)
func (_Gassponsor *GassponsorFilterer) ParseActionSet(log types.Log) (*GassponsorActionSet, error) {
	event := new(GassponsorActionSet)
	if err := _Gassponsor.contract.UnpackLog(event, "ActionSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// GassponsorRelaySetIterator is returned from FilterRelaySet and is used to iterate over the raw logs and unpacked data for RelaySet events raised by the Gassponsor contract.
type GassponsorRelaySetIterator struct {
	Event *GassponsorRelaySet // Event containing the contract specifics and raw log

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
func (it *GassponsorRelaySetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GassponsorRelaySet)
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
		it.Event = new(GassponsorRelaySet)
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
func (it *GassponsorRelaySetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GassponsorRelaySetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GassponsorRelaySet represents a RelaySet event raised by the Gassponsor contract.
type GassponsorRelaySet struct {
	Relay   common.Address
	Allowed bool
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterRelaySet is a free log retrieval operation binding the contract event 0xf1a9c06ecd6e47e4f5e6a4197adb647f1b139f771d37607de489ab3d30562315.
//
// Solidity: event RelaySet(address indexed relay, bool allowed)
func (_Gassponsor *GassponsorFilterer) FilterRelaySet(opts *bind.FilterOpts, relay []common.Address) (*GassponsorRelaySetIterator, error) {

	var relayRule []interface{}
	for _, relayItem := range relay {
		relayRule = append(relayRule, relayItem)
	}

	logs, sub, err := _Gassponsor.contract.FilterLogs(opts, "RelaySet", relayRule)
	if err != nil {
		return nil, err
	}
	return &GassponsorRelaySetIterator{contract: _Gassponsor.contract, event: "RelaySet", logs: logs, sub: sub}, nil
}

// WatchRelaySet is a free log subscription operation binding the contract event 0xf1a9c06ecd6e47e4f5e6a4197adb647f1b139f771d37607de489ab3d30562315.
//
// Solidity: event RelaySet(address indexed relay, bool allowed)
func (_Gassponsor *GassponsorFilterer) WatchRelaySet(opts *bind.WatchOpts, sink chan<- *GassponsorRelaySet, relay []common.Address) (event.Subscription, error) {

	var relayRule []interface{}
	for _, relayItem := range relay {
		relayRule = append(relayRule, relayItem)
	}

	logs, sub, err := _Gassponsor.contract.WatchLogs(opts, "RelaySet", relayRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GassponsorRelaySet)
				if err := _Gassponsor.contract.UnpackLog(event, "RelaySet", log); err != nil {
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

// ParseRelaySet is a log parse operation binding the contract event 0xf1a9c06ecd6e47e4f5e6a4197adb647f1b139f771d37607de489ab3d30562315.
//
// Solidity: event RelaySet(address indexed relay, bool allowed)
func (_Gassponsor *GassponsorFilterer) ParseRelaySet(log types.Log) (*GassponsorRelaySet, error) {
	event := new(GassponsorRelaySet)
	if err := _Gassponsor.contract.UnpackLog(event, "RelaySet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// GassponsorSponsoredIterator is returned from FilterSponsored and is used to iterate over the raw logs and unpacked data for Sponsored events raised by the Gassponsor contract.
type GassponsorSponsoredIterator struct {
	Event *GassponsorSponsored // Event containing the contract specifics and raw log

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
func (it *GassponsorSponsoredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GassponsorSponsored)
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
		it.Event = new(GassponsorSponsored)
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
func (it *GassponsorSponsoredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GassponsorSponsoredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GassponsorSponsored represents a Sponsored event raised by the Gassponsor contract.
type GassponsorSponsored struct {
	Relay    common.Address
	Target   common.Address
	Selector [4]byte
	User     common.Address
	Nonce    [32]byte
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterSponsored is a free log retrieval operation binding the contract event 0xb3bb70d0d151761d68d146070c47284f59b783d84372a9758a24f819a6cbcd56.
//
// Solidity: event Sponsored(address indexed relay, address indexed target, bytes4 indexed selector, address user, bytes32 nonce)
func (_Gassponsor *GassponsorFilterer) FilterSponsored(opts *bind.FilterOpts, relay []common.Address, target []common.Address, selector [][4]byte) (*GassponsorSponsoredIterator, error) {

	var relayRule []interface{}
	for _, relayItem := range relay {
		relayRule = append(relayRule, relayItem)
	}
	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}
	var selectorRule []interface{}
	for _, selectorItem := range selector {
		selectorRule = append(selectorRule, selectorItem)
	}

	logs, sub, err := _Gassponsor.contract.FilterLogs(opts, "Sponsored", relayRule, targetRule, selectorRule)
	if err != nil {
		return nil, err
	}
	return &GassponsorSponsoredIterator{contract: _Gassponsor.contract, event: "Sponsored", logs: logs, sub: sub}, nil
}

// WatchSponsored is a free log subscription operation binding the contract event 0xb3bb70d0d151761d68d146070c47284f59b783d84372a9758a24f819a6cbcd56.
//
// Solidity: event Sponsored(address indexed relay, address indexed target, bytes4 indexed selector, address user, bytes32 nonce)
func (_Gassponsor *GassponsorFilterer) WatchSponsored(opts *bind.WatchOpts, sink chan<- *GassponsorSponsored, relay []common.Address, target []common.Address, selector [][4]byte) (event.Subscription, error) {

	var relayRule []interface{}
	for _, relayItem := range relay {
		relayRule = append(relayRule, relayItem)
	}
	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}
	var selectorRule []interface{}
	for _, selectorItem := range selector {
		selectorRule = append(selectorRule, selectorItem)
	}

	logs, sub, err := _Gassponsor.contract.WatchLogs(opts, "Sponsored", relayRule, targetRule, selectorRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GassponsorSponsored)
				if err := _Gassponsor.contract.UnpackLog(event, "Sponsored", log); err != nil {
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

// ParseSponsored is a log parse operation binding the contract event 0xb3bb70d0d151761d68d146070c47284f59b783d84372a9758a24f819a6cbcd56.
//
// Solidity: event Sponsored(address indexed relay, address indexed target, bytes4 indexed selector, address user, bytes32 nonce)
func (_Gassponsor *GassponsorFilterer) ParseSponsored(log types.Log) (*GassponsorSponsored, error) {
	event := new(GassponsorSponsored)
	if err := _Gassponsor.contract.UnpackLog(event, "Sponsored", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
