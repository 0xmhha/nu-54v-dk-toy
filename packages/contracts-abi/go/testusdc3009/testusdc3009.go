// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package testusdc3009

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

// Testusdc3009MetaData contains all meta data concerning the Testusdc3009 contract.
var Testusdc3009MetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"owner_\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"DOMAIN_SEPARATOR\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"RECEIVE_WITH_AUTHORIZATION_TYPEHASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"RETURN_DELAY\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"TRANSFER_WITH_AUTHORIZATION_TYPEHASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"}],\"name\":\"acceptTransfer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"allowance\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"approve\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"authorizationState\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"balanceOf\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"decimals\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"}],\"name\":\"heldTransfer\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"heldAt\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"mint\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"name\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"nextHeldId\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"validAfter\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"validBefore\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"nonce\",\"type\":\"bytes32\"},{\"internalType\":\"uint8\",\"name\":\"v\",\"type\":\"uint8\"},{\"internalType\":\"bytes32\",\"name\":\"r\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"s\",\"type\":\"bytes32\"}],\"name\":\"receiveWithAuthorization\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"refusalMode\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"}],\"name\":\"rejectTransfer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"}],\"name\":\"returnToSender\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bool\",\"name\":\"enabled\",\"type\":\"bool\"}],\"name\":\"setRefusalMode\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"optedOut\",\"type\":\"bool\"}],\"name\":\"setTrustOptOut\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"trusted\",\"type\":\"bool\"}],\"name\":\"setTrustedSender\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"symbol\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"totalSupply\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"transfer\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"transferFrom\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"validAfter\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"validBefore\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"nonce\",\"type\":\"bytes32\"},{\"internalType\":\"uint8\",\"name\":\"v\",\"type\":\"uint8\"},{\"internalType\":\"bytes32\",\"name\":\"r\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"s\",\"type\":\"bytes32\"}],\"name\":\"transferWithAuthorization\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"trustOptOut\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"trustedSender\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"wouldHold\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"Approval\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"authorizer\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"nonce\",\"type\":\"bytes32\"}],\"name\":\"AuthorizationUsed\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"enabled\",\"type\":\"bool\"}],\"name\":\"RefusalModeSet\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"Transfer\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"TransferAccepted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"TransferHeld\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"TransferRejected\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"id\",\"type\":\"uint256\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"TransferReturned\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"optedOut\",\"type\":\"bool\"}],\"name\":\"TrustOptOutSet\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"trusted\",\"type\":\"bool\"}],\"name\":\"TrustedSenderSet\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"AmountTooLarge\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"AuthorizationAlreadyUsed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"AuthorizationExpired\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"AuthorizationNotYetValid\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"CallerNotRecipient\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InsufficientAllowance\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InsufficientBalance\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidAuthorization\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidRecipient\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotOwner\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotRecipient\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"RecipientRefusing\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ReturnNotYetAllowed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"UnknownHeldTransfer\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ZeroAddress\",\"type\":\"error\"}]",
}

// Testusdc3009ABI is the input ABI used to generate the binding from.
// Deprecated: Use Testusdc3009MetaData.ABI instead.
var Testusdc3009ABI = Testusdc3009MetaData.ABI

// Testusdc3009 is an auto generated Go binding around an Ethereum contract.
type Testusdc3009 struct {
	Testusdc3009Caller     // Read-only binding to the contract
	Testusdc3009Transactor // Write-only binding to the contract
	Testusdc3009Filterer   // Log filterer for contract events
}

// Testusdc3009Caller is an auto generated read-only Go binding around an Ethereum contract.
type Testusdc3009Caller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// Testusdc3009Transactor is an auto generated write-only Go binding around an Ethereum contract.
type Testusdc3009Transactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// Testusdc3009Filterer is an auto generated log filtering Go binding around an Ethereum contract events.
type Testusdc3009Filterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// Testusdc3009Session is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type Testusdc3009Session struct {
	Contract     *Testusdc3009     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// Testusdc3009CallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type Testusdc3009CallerSession struct {
	Contract *Testusdc3009Caller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// Testusdc3009TransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type Testusdc3009TransactorSession struct {
	Contract     *Testusdc3009Transactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// Testusdc3009Raw is an auto generated low-level Go binding around an Ethereum contract.
type Testusdc3009Raw struct {
	Contract *Testusdc3009 // Generic contract binding to access the raw methods on
}

// Testusdc3009CallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type Testusdc3009CallerRaw struct {
	Contract *Testusdc3009Caller // Generic read-only contract binding to access the raw methods on
}

// Testusdc3009TransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type Testusdc3009TransactorRaw struct {
	Contract *Testusdc3009Transactor // Generic write-only contract binding to access the raw methods on
}

// NewTestusdc3009 creates a new instance of Testusdc3009, bound to a specific deployed contract.
func NewTestusdc3009(address common.Address, backend bind.ContractBackend) (*Testusdc3009, error) {
	contract, err := bindTestusdc3009(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009{Testusdc3009Caller: Testusdc3009Caller{contract: contract}, Testusdc3009Transactor: Testusdc3009Transactor{contract: contract}, Testusdc3009Filterer: Testusdc3009Filterer{contract: contract}}, nil
}

// NewTestusdc3009Caller creates a new read-only instance of Testusdc3009, bound to a specific deployed contract.
func NewTestusdc3009Caller(address common.Address, caller bind.ContractCaller) (*Testusdc3009Caller, error) {
	contract, err := bindTestusdc3009(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009Caller{contract: contract}, nil
}

// NewTestusdc3009Transactor creates a new write-only instance of Testusdc3009, bound to a specific deployed contract.
func NewTestusdc3009Transactor(address common.Address, transactor bind.ContractTransactor) (*Testusdc3009Transactor, error) {
	contract, err := bindTestusdc3009(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009Transactor{contract: contract}, nil
}

// NewTestusdc3009Filterer creates a new log filterer instance of Testusdc3009, bound to a specific deployed contract.
func NewTestusdc3009Filterer(address common.Address, filterer bind.ContractFilterer) (*Testusdc3009Filterer, error) {
	contract, err := bindTestusdc3009(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009Filterer{contract: contract}, nil
}

// bindTestusdc3009 binds a generic wrapper to an already deployed contract.
func bindTestusdc3009(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := Testusdc3009MetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Testusdc3009 *Testusdc3009Raw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Testusdc3009.Contract.Testusdc3009Caller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Testusdc3009 *Testusdc3009Raw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Testusdc3009.Contract.Testusdc3009Transactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Testusdc3009 *Testusdc3009Raw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Testusdc3009.Contract.Testusdc3009Transactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Testusdc3009 *Testusdc3009CallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Testusdc3009.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Testusdc3009 *Testusdc3009TransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Testusdc3009.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Testusdc3009 *Testusdc3009TransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Testusdc3009.Contract.contract.Transact(opts, method, params...)
}

// DOMAINSEPARATOR is a free data retrieval call binding the contract method 0x3644e515.
//
// Solidity: function DOMAIN_SEPARATOR() view returns(bytes32)
func (_Testusdc3009 *Testusdc3009Caller) DOMAINSEPARATOR(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "DOMAIN_SEPARATOR")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// DOMAINSEPARATOR is a free data retrieval call binding the contract method 0x3644e515.
//
// Solidity: function DOMAIN_SEPARATOR() view returns(bytes32)
func (_Testusdc3009 *Testusdc3009Session) DOMAINSEPARATOR() ([32]byte, error) {
	return _Testusdc3009.Contract.DOMAINSEPARATOR(&_Testusdc3009.CallOpts)
}

// DOMAINSEPARATOR is a free data retrieval call binding the contract method 0x3644e515.
//
// Solidity: function DOMAIN_SEPARATOR() view returns(bytes32)
func (_Testusdc3009 *Testusdc3009CallerSession) DOMAINSEPARATOR() ([32]byte, error) {
	return _Testusdc3009.Contract.DOMAINSEPARATOR(&_Testusdc3009.CallOpts)
}

// RECEIVEWITHAUTHORIZATIONTYPEHASH is a free data retrieval call binding the contract method 0x7f2eecc3.
//
// Solidity: function RECEIVE_WITH_AUTHORIZATION_TYPEHASH() view returns(bytes32)
func (_Testusdc3009 *Testusdc3009Caller) RECEIVEWITHAUTHORIZATIONTYPEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "RECEIVE_WITH_AUTHORIZATION_TYPEHASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// RECEIVEWITHAUTHORIZATIONTYPEHASH is a free data retrieval call binding the contract method 0x7f2eecc3.
//
// Solidity: function RECEIVE_WITH_AUTHORIZATION_TYPEHASH() view returns(bytes32)
func (_Testusdc3009 *Testusdc3009Session) RECEIVEWITHAUTHORIZATIONTYPEHASH() ([32]byte, error) {
	return _Testusdc3009.Contract.RECEIVEWITHAUTHORIZATIONTYPEHASH(&_Testusdc3009.CallOpts)
}

// RECEIVEWITHAUTHORIZATIONTYPEHASH is a free data retrieval call binding the contract method 0x7f2eecc3.
//
// Solidity: function RECEIVE_WITH_AUTHORIZATION_TYPEHASH() view returns(bytes32)
func (_Testusdc3009 *Testusdc3009CallerSession) RECEIVEWITHAUTHORIZATIONTYPEHASH() ([32]byte, error) {
	return _Testusdc3009.Contract.RECEIVEWITHAUTHORIZATIONTYPEHASH(&_Testusdc3009.CallOpts)
}

// RETURNDELAY is a free data retrieval call binding the contract method 0x6038a3e5.
//
// Solidity: function RETURN_DELAY() view returns(uint256)
func (_Testusdc3009 *Testusdc3009Caller) RETURNDELAY(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "RETURN_DELAY")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// RETURNDELAY is a free data retrieval call binding the contract method 0x6038a3e5.
//
// Solidity: function RETURN_DELAY() view returns(uint256)
func (_Testusdc3009 *Testusdc3009Session) RETURNDELAY() (*big.Int, error) {
	return _Testusdc3009.Contract.RETURNDELAY(&_Testusdc3009.CallOpts)
}

// RETURNDELAY is a free data retrieval call binding the contract method 0x6038a3e5.
//
// Solidity: function RETURN_DELAY() view returns(uint256)
func (_Testusdc3009 *Testusdc3009CallerSession) RETURNDELAY() (*big.Int, error) {
	return _Testusdc3009.Contract.RETURNDELAY(&_Testusdc3009.CallOpts)
}

// TRANSFERWITHAUTHORIZATIONTYPEHASH is a free data retrieval call binding the contract method 0xa0cc6a68.
//
// Solidity: function TRANSFER_WITH_AUTHORIZATION_TYPEHASH() view returns(bytes32)
func (_Testusdc3009 *Testusdc3009Caller) TRANSFERWITHAUTHORIZATIONTYPEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "TRANSFER_WITH_AUTHORIZATION_TYPEHASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// TRANSFERWITHAUTHORIZATIONTYPEHASH is a free data retrieval call binding the contract method 0xa0cc6a68.
//
// Solidity: function TRANSFER_WITH_AUTHORIZATION_TYPEHASH() view returns(bytes32)
func (_Testusdc3009 *Testusdc3009Session) TRANSFERWITHAUTHORIZATIONTYPEHASH() ([32]byte, error) {
	return _Testusdc3009.Contract.TRANSFERWITHAUTHORIZATIONTYPEHASH(&_Testusdc3009.CallOpts)
}

// TRANSFERWITHAUTHORIZATIONTYPEHASH is a free data retrieval call binding the contract method 0xa0cc6a68.
//
// Solidity: function TRANSFER_WITH_AUTHORIZATION_TYPEHASH() view returns(bytes32)
func (_Testusdc3009 *Testusdc3009CallerSession) TRANSFERWITHAUTHORIZATIONTYPEHASH() ([32]byte, error) {
	return _Testusdc3009.Contract.TRANSFERWITHAUTHORIZATIONTYPEHASH(&_Testusdc3009.CallOpts)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address , address ) view returns(uint256)
func (_Testusdc3009 *Testusdc3009Caller) Allowance(opts *bind.CallOpts, arg0 common.Address, arg1 common.Address) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "allowance", arg0, arg1)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address , address ) view returns(uint256)
func (_Testusdc3009 *Testusdc3009Session) Allowance(arg0 common.Address, arg1 common.Address) (*big.Int, error) {
	return _Testusdc3009.Contract.Allowance(&_Testusdc3009.CallOpts, arg0, arg1)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address , address ) view returns(uint256)
func (_Testusdc3009 *Testusdc3009CallerSession) Allowance(arg0 common.Address, arg1 common.Address) (*big.Int, error) {
	return _Testusdc3009.Contract.Allowance(&_Testusdc3009.CallOpts, arg0, arg1)
}

// AuthorizationState is a free data retrieval call binding the contract method 0xe94a0102.
//
// Solidity: function authorizationState(address , bytes32 ) view returns(bool)
func (_Testusdc3009 *Testusdc3009Caller) AuthorizationState(opts *bind.CallOpts, arg0 common.Address, arg1 [32]byte) (bool, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "authorizationState", arg0, arg1)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// AuthorizationState is a free data retrieval call binding the contract method 0xe94a0102.
//
// Solidity: function authorizationState(address , bytes32 ) view returns(bool)
func (_Testusdc3009 *Testusdc3009Session) AuthorizationState(arg0 common.Address, arg1 [32]byte) (bool, error) {
	return _Testusdc3009.Contract.AuthorizationState(&_Testusdc3009.CallOpts, arg0, arg1)
}

// AuthorizationState is a free data retrieval call binding the contract method 0xe94a0102.
//
// Solidity: function authorizationState(address , bytes32 ) view returns(bool)
func (_Testusdc3009 *Testusdc3009CallerSession) AuthorizationState(arg0 common.Address, arg1 [32]byte) (bool, error) {
	return _Testusdc3009.Contract.AuthorizationState(&_Testusdc3009.CallOpts, arg0, arg1)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address ) view returns(uint256)
func (_Testusdc3009 *Testusdc3009Caller) BalanceOf(opts *bind.CallOpts, arg0 common.Address) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "balanceOf", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address ) view returns(uint256)
func (_Testusdc3009 *Testusdc3009Session) BalanceOf(arg0 common.Address) (*big.Int, error) {
	return _Testusdc3009.Contract.BalanceOf(&_Testusdc3009.CallOpts, arg0)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address ) view returns(uint256)
func (_Testusdc3009 *Testusdc3009CallerSession) BalanceOf(arg0 common.Address) (*big.Int, error) {
	return _Testusdc3009.Contract.BalanceOf(&_Testusdc3009.CallOpts, arg0)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_Testusdc3009 *Testusdc3009Caller) Decimals(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "decimals")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_Testusdc3009 *Testusdc3009Session) Decimals() (uint8, error) {
	return _Testusdc3009.Contract.Decimals(&_Testusdc3009.CallOpts)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_Testusdc3009 *Testusdc3009CallerSession) Decimals() (uint8, error) {
	return _Testusdc3009.Contract.Decimals(&_Testusdc3009.CallOpts)
}

// HeldTransfer is a free data retrieval call binding the contract method 0x4f41094f.
//
// Solidity: function heldTransfer(uint256 id) view returns(address from, address to, uint256 amount, uint256 heldAt)
func (_Testusdc3009 *Testusdc3009Caller) HeldTransfer(opts *bind.CallOpts, id *big.Int) (struct {
	From   common.Address
	To     common.Address
	Amount *big.Int
	HeldAt *big.Int
}, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "heldTransfer", id)

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
func (_Testusdc3009 *Testusdc3009Session) HeldTransfer(id *big.Int) (struct {
	From   common.Address
	To     common.Address
	Amount *big.Int
	HeldAt *big.Int
}, error) {
	return _Testusdc3009.Contract.HeldTransfer(&_Testusdc3009.CallOpts, id)
}

// HeldTransfer is a free data retrieval call binding the contract method 0x4f41094f.
//
// Solidity: function heldTransfer(uint256 id) view returns(address from, address to, uint256 amount, uint256 heldAt)
func (_Testusdc3009 *Testusdc3009CallerSession) HeldTransfer(id *big.Int) (struct {
	From   common.Address
	To     common.Address
	Amount *big.Int
	HeldAt *big.Int
}, error) {
	return _Testusdc3009.Contract.HeldTransfer(&_Testusdc3009.CallOpts, id)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_Testusdc3009 *Testusdc3009Caller) Name(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "name")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_Testusdc3009 *Testusdc3009Session) Name() (string, error) {
	return _Testusdc3009.Contract.Name(&_Testusdc3009.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_Testusdc3009 *Testusdc3009CallerSession) Name() (string, error) {
	return _Testusdc3009.Contract.Name(&_Testusdc3009.CallOpts)
}

// NextHeldId is a free data retrieval call binding the contract method 0x3bae582e.
//
// Solidity: function nextHeldId() view returns(uint256)
func (_Testusdc3009 *Testusdc3009Caller) NextHeldId(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "nextHeldId")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// NextHeldId is a free data retrieval call binding the contract method 0x3bae582e.
//
// Solidity: function nextHeldId() view returns(uint256)
func (_Testusdc3009 *Testusdc3009Session) NextHeldId() (*big.Int, error) {
	return _Testusdc3009.Contract.NextHeldId(&_Testusdc3009.CallOpts)
}

// NextHeldId is a free data retrieval call binding the contract method 0x3bae582e.
//
// Solidity: function nextHeldId() view returns(uint256)
func (_Testusdc3009 *Testusdc3009CallerSession) NextHeldId() (*big.Int, error) {
	return _Testusdc3009.Contract.NextHeldId(&_Testusdc3009.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Testusdc3009 *Testusdc3009Caller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Testusdc3009 *Testusdc3009Session) Owner() (common.Address, error) {
	return _Testusdc3009.Contract.Owner(&_Testusdc3009.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Testusdc3009 *Testusdc3009CallerSession) Owner() (common.Address, error) {
	return _Testusdc3009.Contract.Owner(&_Testusdc3009.CallOpts)
}

// RefusalMode is a free data retrieval call binding the contract method 0x1fd9c246.
//
// Solidity: function refusalMode(address ) view returns(bool)
func (_Testusdc3009 *Testusdc3009Caller) RefusalMode(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "refusalMode", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// RefusalMode is a free data retrieval call binding the contract method 0x1fd9c246.
//
// Solidity: function refusalMode(address ) view returns(bool)
func (_Testusdc3009 *Testusdc3009Session) RefusalMode(arg0 common.Address) (bool, error) {
	return _Testusdc3009.Contract.RefusalMode(&_Testusdc3009.CallOpts, arg0)
}

// RefusalMode is a free data retrieval call binding the contract method 0x1fd9c246.
//
// Solidity: function refusalMode(address ) view returns(bool)
func (_Testusdc3009 *Testusdc3009CallerSession) RefusalMode(arg0 common.Address) (bool, error) {
	return _Testusdc3009.Contract.RefusalMode(&_Testusdc3009.CallOpts, arg0)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_Testusdc3009 *Testusdc3009Caller) Symbol(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "symbol")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_Testusdc3009 *Testusdc3009Session) Symbol() (string, error) {
	return _Testusdc3009.Contract.Symbol(&_Testusdc3009.CallOpts)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_Testusdc3009 *Testusdc3009CallerSession) Symbol() (string, error) {
	return _Testusdc3009.Contract.Symbol(&_Testusdc3009.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_Testusdc3009 *Testusdc3009Caller) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "totalSupply")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_Testusdc3009 *Testusdc3009Session) TotalSupply() (*big.Int, error) {
	return _Testusdc3009.Contract.TotalSupply(&_Testusdc3009.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_Testusdc3009 *Testusdc3009CallerSession) TotalSupply() (*big.Int, error) {
	return _Testusdc3009.Contract.TotalSupply(&_Testusdc3009.CallOpts)
}

// TrustOptOut is a free data retrieval call binding the contract method 0x7bbf2fda.
//
// Solidity: function trustOptOut(address , address ) view returns(bool)
func (_Testusdc3009 *Testusdc3009Caller) TrustOptOut(opts *bind.CallOpts, arg0 common.Address, arg1 common.Address) (bool, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "trustOptOut", arg0, arg1)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// TrustOptOut is a free data retrieval call binding the contract method 0x7bbf2fda.
//
// Solidity: function trustOptOut(address , address ) view returns(bool)
func (_Testusdc3009 *Testusdc3009Session) TrustOptOut(arg0 common.Address, arg1 common.Address) (bool, error) {
	return _Testusdc3009.Contract.TrustOptOut(&_Testusdc3009.CallOpts, arg0, arg1)
}

// TrustOptOut is a free data retrieval call binding the contract method 0x7bbf2fda.
//
// Solidity: function trustOptOut(address , address ) view returns(bool)
func (_Testusdc3009 *Testusdc3009CallerSession) TrustOptOut(arg0 common.Address, arg1 common.Address) (bool, error) {
	return _Testusdc3009.Contract.TrustOptOut(&_Testusdc3009.CallOpts, arg0, arg1)
}

// TrustedSender is a free data retrieval call binding the contract method 0x3f6ba415.
//
// Solidity: function trustedSender(address ) view returns(bool)
func (_Testusdc3009 *Testusdc3009Caller) TrustedSender(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "trustedSender", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// TrustedSender is a free data retrieval call binding the contract method 0x3f6ba415.
//
// Solidity: function trustedSender(address ) view returns(bool)
func (_Testusdc3009 *Testusdc3009Session) TrustedSender(arg0 common.Address) (bool, error) {
	return _Testusdc3009.Contract.TrustedSender(&_Testusdc3009.CallOpts, arg0)
}

// TrustedSender is a free data retrieval call binding the contract method 0x3f6ba415.
//
// Solidity: function trustedSender(address ) view returns(bool)
func (_Testusdc3009 *Testusdc3009CallerSession) TrustedSender(arg0 common.Address) (bool, error) {
	return _Testusdc3009.Contract.TrustedSender(&_Testusdc3009.CallOpts, arg0)
}

// WouldHold is a free data retrieval call binding the contract method 0x502fb65f.
//
// Solidity: function wouldHold(address sender, address account) view returns(bool)
func (_Testusdc3009 *Testusdc3009Caller) WouldHold(opts *bind.CallOpts, sender common.Address, account common.Address) (bool, error) {
	var out []interface{}
	err := _Testusdc3009.contract.Call(opts, &out, "wouldHold", sender, account)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// WouldHold is a free data retrieval call binding the contract method 0x502fb65f.
//
// Solidity: function wouldHold(address sender, address account) view returns(bool)
func (_Testusdc3009 *Testusdc3009Session) WouldHold(sender common.Address, account common.Address) (bool, error) {
	return _Testusdc3009.Contract.WouldHold(&_Testusdc3009.CallOpts, sender, account)
}

// WouldHold is a free data retrieval call binding the contract method 0x502fb65f.
//
// Solidity: function wouldHold(address sender, address account) view returns(bool)
func (_Testusdc3009 *Testusdc3009CallerSession) WouldHold(sender common.Address, account common.Address) (bool, error) {
	return _Testusdc3009.Contract.WouldHold(&_Testusdc3009.CallOpts, sender, account)
}

// AcceptTransfer is a paid mutator transaction binding the contract method 0x274fae7c.
//
// Solidity: function acceptTransfer(uint256 id) returns()
func (_Testusdc3009 *Testusdc3009Transactor) AcceptTransfer(opts *bind.TransactOpts, id *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "acceptTransfer", id)
}

// AcceptTransfer is a paid mutator transaction binding the contract method 0x274fae7c.
//
// Solidity: function acceptTransfer(uint256 id) returns()
func (_Testusdc3009 *Testusdc3009Session) AcceptTransfer(id *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.AcceptTransfer(&_Testusdc3009.TransactOpts, id)
}

// AcceptTransfer is a paid mutator transaction binding the contract method 0x274fae7c.
//
// Solidity: function acceptTransfer(uint256 id) returns()
func (_Testusdc3009 *Testusdc3009TransactorSession) AcceptTransfer(id *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.AcceptTransfer(&_Testusdc3009.TransactOpts, id)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 amount) returns(bool)
func (_Testusdc3009 *Testusdc3009Transactor) Approve(opts *bind.TransactOpts, spender common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "approve", spender, amount)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 amount) returns(bool)
func (_Testusdc3009 *Testusdc3009Session) Approve(spender common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.Approve(&_Testusdc3009.TransactOpts, spender, amount)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 amount) returns(bool)
func (_Testusdc3009 *Testusdc3009TransactorSession) Approve(spender common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.Approve(&_Testusdc3009.TransactOpts, spender, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_Testusdc3009 *Testusdc3009Transactor) Mint(opts *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "mint", to, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_Testusdc3009 *Testusdc3009Session) Mint(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.Mint(&_Testusdc3009.TransactOpts, to, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_Testusdc3009 *Testusdc3009TransactorSession) Mint(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.Mint(&_Testusdc3009.TransactOpts, to, amount)
}

// ReceiveWithAuthorization is a paid mutator transaction binding the contract method 0xef55bec6.
//
// Solidity: function receiveWithAuthorization(address from, address to, uint256 value, uint256 validAfter, uint256 validBefore, bytes32 nonce, uint8 v, bytes32 r, bytes32 s) returns()
func (_Testusdc3009 *Testusdc3009Transactor) ReceiveWithAuthorization(opts *bind.TransactOpts, from common.Address, to common.Address, value *big.Int, validAfter *big.Int, validBefore *big.Int, nonce [32]byte, v uint8, r [32]byte, s [32]byte) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "receiveWithAuthorization", from, to, value, validAfter, validBefore, nonce, v, r, s)
}

// ReceiveWithAuthorization is a paid mutator transaction binding the contract method 0xef55bec6.
//
// Solidity: function receiveWithAuthorization(address from, address to, uint256 value, uint256 validAfter, uint256 validBefore, bytes32 nonce, uint8 v, bytes32 r, bytes32 s) returns()
func (_Testusdc3009 *Testusdc3009Session) ReceiveWithAuthorization(from common.Address, to common.Address, value *big.Int, validAfter *big.Int, validBefore *big.Int, nonce [32]byte, v uint8, r [32]byte, s [32]byte) (*types.Transaction, error) {
	return _Testusdc3009.Contract.ReceiveWithAuthorization(&_Testusdc3009.TransactOpts, from, to, value, validAfter, validBefore, nonce, v, r, s)
}

// ReceiveWithAuthorization is a paid mutator transaction binding the contract method 0xef55bec6.
//
// Solidity: function receiveWithAuthorization(address from, address to, uint256 value, uint256 validAfter, uint256 validBefore, bytes32 nonce, uint8 v, bytes32 r, bytes32 s) returns()
func (_Testusdc3009 *Testusdc3009TransactorSession) ReceiveWithAuthorization(from common.Address, to common.Address, value *big.Int, validAfter *big.Int, validBefore *big.Int, nonce [32]byte, v uint8, r [32]byte, s [32]byte) (*types.Transaction, error) {
	return _Testusdc3009.Contract.ReceiveWithAuthorization(&_Testusdc3009.TransactOpts, from, to, value, validAfter, validBefore, nonce, v, r, s)
}

// RejectTransfer is a paid mutator transaction binding the contract method 0x12effc32.
//
// Solidity: function rejectTransfer(uint256 id) returns()
func (_Testusdc3009 *Testusdc3009Transactor) RejectTransfer(opts *bind.TransactOpts, id *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "rejectTransfer", id)
}

// RejectTransfer is a paid mutator transaction binding the contract method 0x12effc32.
//
// Solidity: function rejectTransfer(uint256 id) returns()
func (_Testusdc3009 *Testusdc3009Session) RejectTransfer(id *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.RejectTransfer(&_Testusdc3009.TransactOpts, id)
}

// RejectTransfer is a paid mutator transaction binding the contract method 0x12effc32.
//
// Solidity: function rejectTransfer(uint256 id) returns()
func (_Testusdc3009 *Testusdc3009TransactorSession) RejectTransfer(id *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.RejectTransfer(&_Testusdc3009.TransactOpts, id)
}

// ReturnToSender is a paid mutator transaction binding the contract method 0x745ea1fe.
//
// Solidity: function returnToSender(uint256 id) returns()
func (_Testusdc3009 *Testusdc3009Transactor) ReturnToSender(opts *bind.TransactOpts, id *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "returnToSender", id)
}

// ReturnToSender is a paid mutator transaction binding the contract method 0x745ea1fe.
//
// Solidity: function returnToSender(uint256 id) returns()
func (_Testusdc3009 *Testusdc3009Session) ReturnToSender(id *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.ReturnToSender(&_Testusdc3009.TransactOpts, id)
}

// ReturnToSender is a paid mutator transaction binding the contract method 0x745ea1fe.
//
// Solidity: function returnToSender(uint256 id) returns()
func (_Testusdc3009 *Testusdc3009TransactorSession) ReturnToSender(id *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.ReturnToSender(&_Testusdc3009.TransactOpts, id)
}

// SetRefusalMode is a paid mutator transaction binding the contract method 0x35dac11d.
//
// Solidity: function setRefusalMode(bool enabled) returns()
func (_Testusdc3009 *Testusdc3009Transactor) SetRefusalMode(opts *bind.TransactOpts, enabled bool) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "setRefusalMode", enabled)
}

// SetRefusalMode is a paid mutator transaction binding the contract method 0x35dac11d.
//
// Solidity: function setRefusalMode(bool enabled) returns()
func (_Testusdc3009 *Testusdc3009Session) SetRefusalMode(enabled bool) (*types.Transaction, error) {
	return _Testusdc3009.Contract.SetRefusalMode(&_Testusdc3009.TransactOpts, enabled)
}

// SetRefusalMode is a paid mutator transaction binding the contract method 0x35dac11d.
//
// Solidity: function setRefusalMode(bool enabled) returns()
func (_Testusdc3009 *Testusdc3009TransactorSession) SetRefusalMode(enabled bool) (*types.Transaction, error) {
	return _Testusdc3009.Contract.SetRefusalMode(&_Testusdc3009.TransactOpts, enabled)
}

// SetTrustOptOut is a paid mutator transaction binding the contract method 0x01bd3193.
//
// Solidity: function setTrustOptOut(address sender, bool optedOut) returns()
func (_Testusdc3009 *Testusdc3009Transactor) SetTrustOptOut(opts *bind.TransactOpts, sender common.Address, optedOut bool) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "setTrustOptOut", sender, optedOut)
}

// SetTrustOptOut is a paid mutator transaction binding the contract method 0x01bd3193.
//
// Solidity: function setTrustOptOut(address sender, bool optedOut) returns()
func (_Testusdc3009 *Testusdc3009Session) SetTrustOptOut(sender common.Address, optedOut bool) (*types.Transaction, error) {
	return _Testusdc3009.Contract.SetTrustOptOut(&_Testusdc3009.TransactOpts, sender, optedOut)
}

// SetTrustOptOut is a paid mutator transaction binding the contract method 0x01bd3193.
//
// Solidity: function setTrustOptOut(address sender, bool optedOut) returns()
func (_Testusdc3009 *Testusdc3009TransactorSession) SetTrustOptOut(sender common.Address, optedOut bool) (*types.Transaction, error) {
	return _Testusdc3009.Contract.SetTrustOptOut(&_Testusdc3009.TransactOpts, sender, optedOut)
}

// SetTrustedSender is a paid mutator transaction binding the contract method 0xca208adb.
//
// Solidity: function setTrustedSender(address sender, bool trusted) returns()
func (_Testusdc3009 *Testusdc3009Transactor) SetTrustedSender(opts *bind.TransactOpts, sender common.Address, trusted bool) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "setTrustedSender", sender, trusted)
}

// SetTrustedSender is a paid mutator transaction binding the contract method 0xca208adb.
//
// Solidity: function setTrustedSender(address sender, bool trusted) returns()
func (_Testusdc3009 *Testusdc3009Session) SetTrustedSender(sender common.Address, trusted bool) (*types.Transaction, error) {
	return _Testusdc3009.Contract.SetTrustedSender(&_Testusdc3009.TransactOpts, sender, trusted)
}

// SetTrustedSender is a paid mutator transaction binding the contract method 0xca208adb.
//
// Solidity: function setTrustedSender(address sender, bool trusted) returns()
func (_Testusdc3009 *Testusdc3009TransactorSession) SetTrustedSender(sender common.Address, trusted bool) (*types.Transaction, error) {
	return _Testusdc3009.Contract.SetTrustedSender(&_Testusdc3009.TransactOpts, sender, trusted)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 amount) returns(bool)
func (_Testusdc3009 *Testusdc3009Transactor) Transfer(opts *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "transfer", to, amount)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 amount) returns(bool)
func (_Testusdc3009 *Testusdc3009Session) Transfer(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.Transfer(&_Testusdc3009.TransactOpts, to, amount)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 amount) returns(bool)
func (_Testusdc3009 *Testusdc3009TransactorSession) Transfer(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.Transfer(&_Testusdc3009.TransactOpts, to, amount)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 amount) returns(bool)
func (_Testusdc3009 *Testusdc3009Transactor) TransferFrom(opts *bind.TransactOpts, from common.Address, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "transferFrom", from, to, amount)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 amount) returns(bool)
func (_Testusdc3009 *Testusdc3009Session) TransferFrom(from common.Address, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.TransferFrom(&_Testusdc3009.TransactOpts, from, to, amount)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 amount) returns(bool)
func (_Testusdc3009 *Testusdc3009TransactorSession) TransferFrom(from common.Address, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Testusdc3009.Contract.TransferFrom(&_Testusdc3009.TransactOpts, from, to, amount)
}

// TransferWithAuthorization is a paid mutator transaction binding the contract method 0xe3ee160e.
//
// Solidity: function transferWithAuthorization(address from, address to, uint256 value, uint256 validAfter, uint256 validBefore, bytes32 nonce, uint8 v, bytes32 r, bytes32 s) returns()
func (_Testusdc3009 *Testusdc3009Transactor) TransferWithAuthorization(opts *bind.TransactOpts, from common.Address, to common.Address, value *big.Int, validAfter *big.Int, validBefore *big.Int, nonce [32]byte, v uint8, r [32]byte, s [32]byte) (*types.Transaction, error) {
	return _Testusdc3009.contract.Transact(opts, "transferWithAuthorization", from, to, value, validAfter, validBefore, nonce, v, r, s)
}

// TransferWithAuthorization is a paid mutator transaction binding the contract method 0xe3ee160e.
//
// Solidity: function transferWithAuthorization(address from, address to, uint256 value, uint256 validAfter, uint256 validBefore, bytes32 nonce, uint8 v, bytes32 r, bytes32 s) returns()
func (_Testusdc3009 *Testusdc3009Session) TransferWithAuthorization(from common.Address, to common.Address, value *big.Int, validAfter *big.Int, validBefore *big.Int, nonce [32]byte, v uint8, r [32]byte, s [32]byte) (*types.Transaction, error) {
	return _Testusdc3009.Contract.TransferWithAuthorization(&_Testusdc3009.TransactOpts, from, to, value, validAfter, validBefore, nonce, v, r, s)
}

// TransferWithAuthorization is a paid mutator transaction binding the contract method 0xe3ee160e.
//
// Solidity: function transferWithAuthorization(address from, address to, uint256 value, uint256 validAfter, uint256 validBefore, bytes32 nonce, uint8 v, bytes32 r, bytes32 s) returns()
func (_Testusdc3009 *Testusdc3009TransactorSession) TransferWithAuthorization(from common.Address, to common.Address, value *big.Int, validAfter *big.Int, validBefore *big.Int, nonce [32]byte, v uint8, r [32]byte, s [32]byte) (*types.Transaction, error) {
	return _Testusdc3009.Contract.TransferWithAuthorization(&_Testusdc3009.TransactOpts, from, to, value, validAfter, validBefore, nonce, v, r, s)
}

// Testusdc3009ApprovalIterator is returned from FilterApproval and is used to iterate over the raw logs and unpacked data for Approval events raised by the Testusdc3009 contract.
type Testusdc3009ApprovalIterator struct {
	Event *Testusdc3009Approval // Event containing the contract specifics and raw log

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
func (it *Testusdc3009ApprovalIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009Approval)
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
		it.Event = new(Testusdc3009Approval)
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
func (it *Testusdc3009ApprovalIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009ApprovalIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009Approval represents a Approval event raised by the Testusdc3009 contract.
type Testusdc3009Approval struct {
	Owner   common.Address
	Spender common.Address
	Value   *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterApproval is a free log retrieval operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_Testusdc3009 *Testusdc3009Filterer) FilterApproval(opts *bind.FilterOpts, owner []common.Address, spender []common.Address) (*Testusdc3009ApprovalIterator, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var spenderRule []interface{}
	for _, spenderItem := range spender {
		spenderRule = append(spenderRule, spenderItem)
	}

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "Approval", ownerRule, spenderRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009ApprovalIterator{contract: _Testusdc3009.contract, event: "Approval", logs: logs, sub: sub}, nil
}

// WatchApproval is a free log subscription operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_Testusdc3009 *Testusdc3009Filterer) WatchApproval(opts *bind.WatchOpts, sink chan<- *Testusdc3009Approval, owner []common.Address, spender []common.Address) (event.Subscription, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var spenderRule []interface{}
	for _, spenderItem := range spender {
		spenderRule = append(spenderRule, spenderItem)
	}

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "Approval", ownerRule, spenderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009Approval)
				if err := _Testusdc3009.contract.UnpackLog(event, "Approval", log); err != nil {
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
func (_Testusdc3009 *Testusdc3009Filterer) ParseApproval(log types.Log) (*Testusdc3009Approval, error) {
	event := new(Testusdc3009Approval)
	if err := _Testusdc3009.contract.UnpackLog(event, "Approval", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Testusdc3009AuthorizationUsedIterator is returned from FilterAuthorizationUsed and is used to iterate over the raw logs and unpacked data for AuthorizationUsed events raised by the Testusdc3009 contract.
type Testusdc3009AuthorizationUsedIterator struct {
	Event *Testusdc3009AuthorizationUsed // Event containing the contract specifics and raw log

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
func (it *Testusdc3009AuthorizationUsedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009AuthorizationUsed)
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
		it.Event = new(Testusdc3009AuthorizationUsed)
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
func (it *Testusdc3009AuthorizationUsedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009AuthorizationUsedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009AuthorizationUsed represents a AuthorizationUsed event raised by the Testusdc3009 contract.
type Testusdc3009AuthorizationUsed struct {
	Authorizer common.Address
	Nonce      [32]byte
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterAuthorizationUsed is a free log retrieval operation binding the contract event 0x98de503528ee59b575ef0c0a2576a82497bfc029a5685b209e9ec333479b10a5.
//
// Solidity: event AuthorizationUsed(address indexed authorizer, bytes32 indexed nonce)
func (_Testusdc3009 *Testusdc3009Filterer) FilterAuthorizationUsed(opts *bind.FilterOpts, authorizer []common.Address, nonce [][32]byte) (*Testusdc3009AuthorizationUsedIterator, error) {

	var authorizerRule []interface{}
	for _, authorizerItem := range authorizer {
		authorizerRule = append(authorizerRule, authorizerItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "AuthorizationUsed", authorizerRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009AuthorizationUsedIterator{contract: _Testusdc3009.contract, event: "AuthorizationUsed", logs: logs, sub: sub}, nil
}

// WatchAuthorizationUsed is a free log subscription operation binding the contract event 0x98de503528ee59b575ef0c0a2576a82497bfc029a5685b209e9ec333479b10a5.
//
// Solidity: event AuthorizationUsed(address indexed authorizer, bytes32 indexed nonce)
func (_Testusdc3009 *Testusdc3009Filterer) WatchAuthorizationUsed(opts *bind.WatchOpts, sink chan<- *Testusdc3009AuthorizationUsed, authorizer []common.Address, nonce [][32]byte) (event.Subscription, error) {

	var authorizerRule []interface{}
	for _, authorizerItem := range authorizer {
		authorizerRule = append(authorizerRule, authorizerItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "AuthorizationUsed", authorizerRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009AuthorizationUsed)
				if err := _Testusdc3009.contract.UnpackLog(event, "AuthorizationUsed", log); err != nil {
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

// ParseAuthorizationUsed is a log parse operation binding the contract event 0x98de503528ee59b575ef0c0a2576a82497bfc029a5685b209e9ec333479b10a5.
//
// Solidity: event AuthorizationUsed(address indexed authorizer, bytes32 indexed nonce)
func (_Testusdc3009 *Testusdc3009Filterer) ParseAuthorizationUsed(log types.Log) (*Testusdc3009AuthorizationUsed, error) {
	event := new(Testusdc3009AuthorizationUsed)
	if err := _Testusdc3009.contract.UnpackLog(event, "AuthorizationUsed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Testusdc3009RefusalModeSetIterator is returned from FilterRefusalModeSet and is used to iterate over the raw logs and unpacked data for RefusalModeSet events raised by the Testusdc3009 contract.
type Testusdc3009RefusalModeSetIterator struct {
	Event *Testusdc3009RefusalModeSet // Event containing the contract specifics and raw log

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
func (it *Testusdc3009RefusalModeSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009RefusalModeSet)
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
		it.Event = new(Testusdc3009RefusalModeSet)
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
func (it *Testusdc3009RefusalModeSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009RefusalModeSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009RefusalModeSet represents a RefusalModeSet event raised by the Testusdc3009 contract.
type Testusdc3009RefusalModeSet struct {
	Account common.Address
	Enabled bool
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterRefusalModeSet is a free log retrieval operation binding the contract event 0xd501b806045698ed46fc71b8184d1049e7ab1464be5827633af82fc47aa4d02a.
//
// Solidity: event RefusalModeSet(address indexed account, bool enabled)
func (_Testusdc3009 *Testusdc3009Filterer) FilterRefusalModeSet(opts *bind.FilterOpts, account []common.Address) (*Testusdc3009RefusalModeSetIterator, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "RefusalModeSet", accountRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009RefusalModeSetIterator{contract: _Testusdc3009.contract, event: "RefusalModeSet", logs: logs, sub: sub}, nil
}

// WatchRefusalModeSet is a free log subscription operation binding the contract event 0xd501b806045698ed46fc71b8184d1049e7ab1464be5827633af82fc47aa4d02a.
//
// Solidity: event RefusalModeSet(address indexed account, bool enabled)
func (_Testusdc3009 *Testusdc3009Filterer) WatchRefusalModeSet(opts *bind.WatchOpts, sink chan<- *Testusdc3009RefusalModeSet, account []common.Address) (event.Subscription, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "RefusalModeSet", accountRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009RefusalModeSet)
				if err := _Testusdc3009.contract.UnpackLog(event, "RefusalModeSet", log); err != nil {
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
func (_Testusdc3009 *Testusdc3009Filterer) ParseRefusalModeSet(log types.Log) (*Testusdc3009RefusalModeSet, error) {
	event := new(Testusdc3009RefusalModeSet)
	if err := _Testusdc3009.contract.UnpackLog(event, "RefusalModeSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Testusdc3009TransferIterator is returned from FilterTransfer and is used to iterate over the raw logs and unpacked data for Transfer events raised by the Testusdc3009 contract.
type Testusdc3009TransferIterator struct {
	Event *Testusdc3009Transfer // Event containing the contract specifics and raw log

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
func (it *Testusdc3009TransferIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009Transfer)
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
		it.Event = new(Testusdc3009Transfer)
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
func (it *Testusdc3009TransferIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009TransferIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009Transfer represents a Transfer event raised by the Testusdc3009 contract.
type Testusdc3009Transfer struct {
	From  common.Address
	To    common.Address
	Value *big.Int
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterTransfer is a free log retrieval operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_Testusdc3009 *Testusdc3009Filterer) FilterTransfer(opts *bind.FilterOpts, from []common.Address, to []common.Address) (*Testusdc3009TransferIterator, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009TransferIterator{contract: _Testusdc3009.contract, event: "Transfer", logs: logs, sub: sub}, nil
}

// WatchTransfer is a free log subscription operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_Testusdc3009 *Testusdc3009Filterer) WatchTransfer(opts *bind.WatchOpts, sink chan<- *Testusdc3009Transfer, from []common.Address, to []common.Address) (event.Subscription, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009Transfer)
				if err := _Testusdc3009.contract.UnpackLog(event, "Transfer", log); err != nil {
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
func (_Testusdc3009 *Testusdc3009Filterer) ParseTransfer(log types.Log) (*Testusdc3009Transfer, error) {
	event := new(Testusdc3009Transfer)
	if err := _Testusdc3009.contract.UnpackLog(event, "Transfer", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Testusdc3009TransferAcceptedIterator is returned from FilterTransferAccepted and is used to iterate over the raw logs and unpacked data for TransferAccepted events raised by the Testusdc3009 contract.
type Testusdc3009TransferAcceptedIterator struct {
	Event *Testusdc3009TransferAccepted // Event containing the contract specifics and raw log

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
func (it *Testusdc3009TransferAcceptedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009TransferAccepted)
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
		it.Event = new(Testusdc3009TransferAccepted)
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
func (it *Testusdc3009TransferAcceptedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009TransferAcceptedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009TransferAccepted represents a TransferAccepted event raised by the Testusdc3009 contract.
type Testusdc3009TransferAccepted struct {
	Id     *big.Int
	From   common.Address
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTransferAccepted is a free log retrieval operation binding the contract event 0x36d5947138c317fdfe6c54a68c6ee9786d30f033fde86488b466cea291e3b98e.
//
// Solidity: event TransferAccepted(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc3009 *Testusdc3009Filterer) FilterTransferAccepted(opts *bind.FilterOpts, id []*big.Int, from []common.Address, to []common.Address) (*Testusdc3009TransferAcceptedIterator, error) {

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

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "TransferAccepted", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009TransferAcceptedIterator{contract: _Testusdc3009.contract, event: "TransferAccepted", logs: logs, sub: sub}, nil
}

// WatchTransferAccepted is a free log subscription operation binding the contract event 0x36d5947138c317fdfe6c54a68c6ee9786d30f033fde86488b466cea291e3b98e.
//
// Solidity: event TransferAccepted(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc3009 *Testusdc3009Filterer) WatchTransferAccepted(opts *bind.WatchOpts, sink chan<- *Testusdc3009TransferAccepted, id []*big.Int, from []common.Address, to []common.Address) (event.Subscription, error) {

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

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "TransferAccepted", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009TransferAccepted)
				if err := _Testusdc3009.contract.UnpackLog(event, "TransferAccepted", log); err != nil {
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
func (_Testusdc3009 *Testusdc3009Filterer) ParseTransferAccepted(log types.Log) (*Testusdc3009TransferAccepted, error) {
	event := new(Testusdc3009TransferAccepted)
	if err := _Testusdc3009.contract.UnpackLog(event, "TransferAccepted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Testusdc3009TransferHeldIterator is returned from FilterTransferHeld and is used to iterate over the raw logs and unpacked data for TransferHeld events raised by the Testusdc3009 contract.
type Testusdc3009TransferHeldIterator struct {
	Event *Testusdc3009TransferHeld // Event containing the contract specifics and raw log

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
func (it *Testusdc3009TransferHeldIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009TransferHeld)
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
		it.Event = new(Testusdc3009TransferHeld)
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
func (it *Testusdc3009TransferHeldIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009TransferHeldIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009TransferHeld represents a TransferHeld event raised by the Testusdc3009 contract.
type Testusdc3009TransferHeld struct {
	Id     *big.Int
	From   common.Address
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTransferHeld is a free log retrieval operation binding the contract event 0x55880df1f18b7149bf5aa7ee3957601319fe18920786b6d5b9964d972f318585.
//
// Solidity: event TransferHeld(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc3009 *Testusdc3009Filterer) FilterTransferHeld(opts *bind.FilterOpts, id []*big.Int, from []common.Address, to []common.Address) (*Testusdc3009TransferHeldIterator, error) {

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

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "TransferHeld", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009TransferHeldIterator{contract: _Testusdc3009.contract, event: "TransferHeld", logs: logs, sub: sub}, nil
}

// WatchTransferHeld is a free log subscription operation binding the contract event 0x55880df1f18b7149bf5aa7ee3957601319fe18920786b6d5b9964d972f318585.
//
// Solidity: event TransferHeld(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc3009 *Testusdc3009Filterer) WatchTransferHeld(opts *bind.WatchOpts, sink chan<- *Testusdc3009TransferHeld, id []*big.Int, from []common.Address, to []common.Address) (event.Subscription, error) {

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

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "TransferHeld", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009TransferHeld)
				if err := _Testusdc3009.contract.UnpackLog(event, "TransferHeld", log); err != nil {
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
func (_Testusdc3009 *Testusdc3009Filterer) ParseTransferHeld(log types.Log) (*Testusdc3009TransferHeld, error) {
	event := new(Testusdc3009TransferHeld)
	if err := _Testusdc3009.contract.UnpackLog(event, "TransferHeld", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Testusdc3009TransferRejectedIterator is returned from FilterTransferRejected and is used to iterate over the raw logs and unpacked data for TransferRejected events raised by the Testusdc3009 contract.
type Testusdc3009TransferRejectedIterator struct {
	Event *Testusdc3009TransferRejected // Event containing the contract specifics and raw log

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
func (it *Testusdc3009TransferRejectedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009TransferRejected)
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
		it.Event = new(Testusdc3009TransferRejected)
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
func (it *Testusdc3009TransferRejectedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009TransferRejectedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009TransferRejected represents a TransferRejected event raised by the Testusdc3009 contract.
type Testusdc3009TransferRejected struct {
	Id     *big.Int
	From   common.Address
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTransferRejected is a free log retrieval operation binding the contract event 0x7300b9ea4d855f42b47170fdc5530a70cc7d53bf42aedfc6572392b07c3b867e.
//
// Solidity: event TransferRejected(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc3009 *Testusdc3009Filterer) FilterTransferRejected(opts *bind.FilterOpts, id []*big.Int, from []common.Address, to []common.Address) (*Testusdc3009TransferRejectedIterator, error) {

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

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "TransferRejected", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009TransferRejectedIterator{contract: _Testusdc3009.contract, event: "TransferRejected", logs: logs, sub: sub}, nil
}

// WatchTransferRejected is a free log subscription operation binding the contract event 0x7300b9ea4d855f42b47170fdc5530a70cc7d53bf42aedfc6572392b07c3b867e.
//
// Solidity: event TransferRejected(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc3009 *Testusdc3009Filterer) WatchTransferRejected(opts *bind.WatchOpts, sink chan<- *Testusdc3009TransferRejected, id []*big.Int, from []common.Address, to []common.Address) (event.Subscription, error) {

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

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "TransferRejected", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009TransferRejected)
				if err := _Testusdc3009.contract.UnpackLog(event, "TransferRejected", log); err != nil {
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
func (_Testusdc3009 *Testusdc3009Filterer) ParseTransferRejected(log types.Log) (*Testusdc3009TransferRejected, error) {
	event := new(Testusdc3009TransferRejected)
	if err := _Testusdc3009.contract.UnpackLog(event, "TransferRejected", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Testusdc3009TransferReturnedIterator is returned from FilterTransferReturned and is used to iterate over the raw logs and unpacked data for TransferReturned events raised by the Testusdc3009 contract.
type Testusdc3009TransferReturnedIterator struct {
	Event *Testusdc3009TransferReturned // Event containing the contract specifics and raw log

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
func (it *Testusdc3009TransferReturnedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009TransferReturned)
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
		it.Event = new(Testusdc3009TransferReturned)
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
func (it *Testusdc3009TransferReturnedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009TransferReturnedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009TransferReturned represents a TransferReturned event raised by the Testusdc3009 contract.
type Testusdc3009TransferReturned struct {
	Id     *big.Int
	From   common.Address
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTransferReturned is a free log retrieval operation binding the contract event 0x7eff2ba5bd86e7ce15c22347700c00018a65e80056451e47da00bfc2231d1bdc.
//
// Solidity: event TransferReturned(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc3009 *Testusdc3009Filterer) FilterTransferReturned(opts *bind.FilterOpts, id []*big.Int, from []common.Address, to []common.Address) (*Testusdc3009TransferReturnedIterator, error) {

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

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "TransferReturned", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009TransferReturnedIterator{contract: _Testusdc3009.contract, event: "TransferReturned", logs: logs, sub: sub}, nil
}

// WatchTransferReturned is a free log subscription operation binding the contract event 0x7eff2ba5bd86e7ce15c22347700c00018a65e80056451e47da00bfc2231d1bdc.
//
// Solidity: event TransferReturned(uint256 indexed id, address indexed from, address indexed to, uint256 amount)
func (_Testusdc3009 *Testusdc3009Filterer) WatchTransferReturned(opts *bind.WatchOpts, sink chan<- *Testusdc3009TransferReturned, id []*big.Int, from []common.Address, to []common.Address) (event.Subscription, error) {

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

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "TransferReturned", idRule, fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009TransferReturned)
				if err := _Testusdc3009.contract.UnpackLog(event, "TransferReturned", log); err != nil {
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
func (_Testusdc3009 *Testusdc3009Filterer) ParseTransferReturned(log types.Log) (*Testusdc3009TransferReturned, error) {
	event := new(Testusdc3009TransferReturned)
	if err := _Testusdc3009.contract.UnpackLog(event, "TransferReturned", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Testusdc3009TrustOptOutSetIterator is returned from FilterTrustOptOutSet and is used to iterate over the raw logs and unpacked data for TrustOptOutSet events raised by the Testusdc3009 contract.
type Testusdc3009TrustOptOutSetIterator struct {
	Event *Testusdc3009TrustOptOutSet // Event containing the contract specifics and raw log

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
func (it *Testusdc3009TrustOptOutSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009TrustOptOutSet)
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
		it.Event = new(Testusdc3009TrustOptOutSet)
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
func (it *Testusdc3009TrustOptOutSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009TrustOptOutSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009TrustOptOutSet represents a TrustOptOutSet event raised by the Testusdc3009 contract.
type Testusdc3009TrustOptOutSet struct {
	Account  common.Address
	Sender   common.Address
	OptedOut bool
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterTrustOptOutSet is a free log retrieval operation binding the contract event 0xb4ebf946b338521727979a53e3fa0aaf42f117988596cd5184d395493a919567.
//
// Solidity: event TrustOptOutSet(address indexed account, address indexed sender, bool optedOut)
func (_Testusdc3009 *Testusdc3009Filterer) FilterTrustOptOutSet(opts *bind.FilterOpts, account []common.Address, sender []common.Address) (*Testusdc3009TrustOptOutSetIterator, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "TrustOptOutSet", accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009TrustOptOutSetIterator{contract: _Testusdc3009.contract, event: "TrustOptOutSet", logs: logs, sub: sub}, nil
}

// WatchTrustOptOutSet is a free log subscription operation binding the contract event 0xb4ebf946b338521727979a53e3fa0aaf42f117988596cd5184d395493a919567.
//
// Solidity: event TrustOptOutSet(address indexed account, address indexed sender, bool optedOut)
func (_Testusdc3009 *Testusdc3009Filterer) WatchTrustOptOutSet(opts *bind.WatchOpts, sink chan<- *Testusdc3009TrustOptOutSet, account []common.Address, sender []common.Address) (event.Subscription, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "TrustOptOutSet", accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009TrustOptOutSet)
				if err := _Testusdc3009.contract.UnpackLog(event, "TrustOptOutSet", log); err != nil {
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
func (_Testusdc3009 *Testusdc3009Filterer) ParseTrustOptOutSet(log types.Log) (*Testusdc3009TrustOptOutSet, error) {
	event := new(Testusdc3009TrustOptOutSet)
	if err := _Testusdc3009.contract.UnpackLog(event, "TrustOptOutSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// Testusdc3009TrustedSenderSetIterator is returned from FilterTrustedSenderSet and is used to iterate over the raw logs and unpacked data for TrustedSenderSet events raised by the Testusdc3009 contract.
type Testusdc3009TrustedSenderSetIterator struct {
	Event *Testusdc3009TrustedSenderSet // Event containing the contract specifics and raw log

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
func (it *Testusdc3009TrustedSenderSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(Testusdc3009TrustedSenderSet)
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
		it.Event = new(Testusdc3009TrustedSenderSet)
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
func (it *Testusdc3009TrustedSenderSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *Testusdc3009TrustedSenderSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// Testusdc3009TrustedSenderSet represents a TrustedSenderSet event raised by the Testusdc3009 contract.
type Testusdc3009TrustedSenderSet struct {
	Sender  common.Address
	Trusted bool
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterTrustedSenderSet is a free log retrieval operation binding the contract event 0xc6d8fe9203493650347355f292641f1329830533307341fc68759d417e654765.
//
// Solidity: event TrustedSenderSet(address indexed sender, bool trusted)
func (_Testusdc3009 *Testusdc3009Filterer) FilterTrustedSenderSet(opts *bind.FilterOpts, sender []common.Address) (*Testusdc3009TrustedSenderSetIterator, error) {

	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _Testusdc3009.contract.FilterLogs(opts, "TrustedSenderSet", senderRule)
	if err != nil {
		return nil, err
	}
	return &Testusdc3009TrustedSenderSetIterator{contract: _Testusdc3009.contract, event: "TrustedSenderSet", logs: logs, sub: sub}, nil
}

// WatchTrustedSenderSet is a free log subscription operation binding the contract event 0xc6d8fe9203493650347355f292641f1329830533307341fc68759d417e654765.
//
// Solidity: event TrustedSenderSet(address indexed sender, bool trusted)
func (_Testusdc3009 *Testusdc3009Filterer) WatchTrustedSenderSet(opts *bind.WatchOpts, sink chan<- *Testusdc3009TrustedSenderSet, sender []common.Address) (event.Subscription, error) {

	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _Testusdc3009.contract.WatchLogs(opts, "TrustedSenderSet", senderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(Testusdc3009TrustedSenderSet)
				if err := _Testusdc3009.contract.UnpackLog(event, "TrustedSenderSet", log); err != nil {
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
func (_Testusdc3009 *Testusdc3009Filterer) ParseTrustedSenderSet(log types.Log) (*Testusdc3009TrustedSenderSet, error) {
	event := new(Testusdc3009TrustedSenderSet)
	if err := _Testusdc3009.contract.UnpackLog(event, "TrustedSenderSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
