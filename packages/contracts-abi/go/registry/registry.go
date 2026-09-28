// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package registry

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

// RegistryMetaData contains all meta data concerning the Registry contract.
var RegistryMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"admin\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"isActive\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"payoutOf\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"payout\",\"type\":\"address\"}],\"name\":\"registerMerchant\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"revokeMerchant\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"payout\",\"type\":\"address\"}],\"name\":\"MerchantRegistered\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"MerchantRevoked\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"NotRegistryAdmin\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ZeroAddress\",\"type\":\"error\"}]",
}

// RegistryABI is the input ABI used to generate the binding from.
// Deprecated: Use RegistryMetaData.ABI instead.
var RegistryABI = RegistryMetaData.ABI

// Registry is an auto generated Go binding around an Ethereum contract.
type Registry struct {
	RegistryCaller     // Read-only binding to the contract
	RegistryTransactor // Write-only binding to the contract
	RegistryFilterer   // Log filterer for contract events
}

// RegistryCaller is an auto generated read-only Go binding around an Ethereum contract.
type RegistryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RegistryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type RegistryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RegistryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type RegistryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RegistrySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type RegistrySession struct {
	Contract     *Registry         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// RegistryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type RegistryCallerSession struct {
	Contract *RegistryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// RegistryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type RegistryTransactorSession struct {
	Contract     *RegistryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// RegistryRaw is an auto generated low-level Go binding around an Ethereum contract.
type RegistryRaw struct {
	Contract *Registry // Generic contract binding to access the raw methods on
}

// RegistryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type RegistryCallerRaw struct {
	Contract *RegistryCaller // Generic read-only contract binding to access the raw methods on
}

// RegistryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type RegistryTransactorRaw struct {
	Contract *RegistryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewRegistry creates a new instance of Registry, bound to a specific deployed contract.
func NewRegistry(address common.Address, backend bind.ContractBackend) (*Registry, error) {
	contract, err := bindRegistry(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Registry{RegistryCaller: RegistryCaller{contract: contract}, RegistryTransactor: RegistryTransactor{contract: contract}, RegistryFilterer: RegistryFilterer{contract: contract}}, nil
}

// NewRegistryCaller creates a new read-only instance of Registry, bound to a specific deployed contract.
func NewRegistryCaller(address common.Address, caller bind.ContractCaller) (*RegistryCaller, error) {
	contract, err := bindRegistry(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &RegistryCaller{contract: contract}, nil
}

// NewRegistryTransactor creates a new write-only instance of Registry, bound to a specific deployed contract.
func NewRegistryTransactor(address common.Address, transactor bind.ContractTransactor) (*RegistryTransactor, error) {
	contract, err := bindRegistry(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &RegistryTransactor{contract: contract}, nil
}

// NewRegistryFilterer creates a new log filterer instance of Registry, bound to a specific deployed contract.
func NewRegistryFilterer(address common.Address, filterer bind.ContractFilterer) (*RegistryFilterer, error) {
	contract, err := bindRegistry(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &RegistryFilterer{contract: contract}, nil
}

// bindRegistry binds a generic wrapper to an already deployed contract.
func bindRegistry(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := RegistryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Registry *RegistryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Registry.Contract.RegistryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Registry *RegistryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Registry.Contract.RegistryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Registry *RegistryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Registry.Contract.RegistryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Registry *RegistryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Registry.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Registry *RegistryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Registry.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Registry *RegistryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Registry.Contract.contract.Transact(opts, method, params...)
}

// Admin is a free data retrieval call binding the contract method 0xf851a440.
//
// Solidity: function admin() view returns(address)
func (_Registry *RegistryCaller) Admin(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "admin")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Admin is a free data retrieval call binding the contract method 0xf851a440.
//
// Solidity: function admin() view returns(address)
func (_Registry *RegistrySession) Admin() (common.Address, error) {
	return _Registry.Contract.Admin(&_Registry.CallOpts)
}

// Admin is a free data retrieval call binding the contract method 0xf851a440.
//
// Solidity: function admin() view returns(address)
func (_Registry *RegistryCallerSession) Admin() (common.Address, error) {
	return _Registry.Contract.Admin(&_Registry.CallOpts)
}

// IsActive is a free data retrieval call binding the contract method 0x9f8a13d7.
//
// Solidity: function isActive(address merchant) view returns(bool)
func (_Registry *RegistryCaller) IsActive(opts *bind.CallOpts, merchant common.Address) (bool, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "isActive", merchant)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsActive is a free data retrieval call binding the contract method 0x9f8a13d7.
//
// Solidity: function isActive(address merchant) view returns(bool)
func (_Registry *RegistrySession) IsActive(merchant common.Address) (bool, error) {
	return _Registry.Contract.IsActive(&_Registry.CallOpts, merchant)
}

// IsActive is a free data retrieval call binding the contract method 0x9f8a13d7.
//
// Solidity: function isActive(address merchant) view returns(bool)
func (_Registry *RegistryCallerSession) IsActive(merchant common.Address) (bool, error) {
	return _Registry.Contract.IsActive(&_Registry.CallOpts, merchant)
}

// PayoutOf is a free data retrieval call binding the contract method 0x6da61d1e.
//
// Solidity: function payoutOf(address merchant) view returns(address)
func (_Registry *RegistryCaller) PayoutOf(opts *bind.CallOpts, merchant common.Address) (common.Address, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "payoutOf", merchant)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// PayoutOf is a free data retrieval call binding the contract method 0x6da61d1e.
//
// Solidity: function payoutOf(address merchant) view returns(address)
func (_Registry *RegistrySession) PayoutOf(merchant common.Address) (common.Address, error) {
	return _Registry.Contract.PayoutOf(&_Registry.CallOpts, merchant)
}

// PayoutOf is a free data retrieval call binding the contract method 0x6da61d1e.
//
// Solidity: function payoutOf(address merchant) view returns(address)
func (_Registry *RegistryCallerSession) PayoutOf(merchant common.Address) (common.Address, error) {
	return _Registry.Contract.PayoutOf(&_Registry.CallOpts, merchant)
}

// RegisterMerchant is a paid mutator transaction binding the contract method 0xda4e2f0e.
//
// Solidity: function registerMerchant(address merchant, address payout) returns()
func (_Registry *RegistryTransactor) RegisterMerchant(opts *bind.TransactOpts, merchant common.Address, payout common.Address) (*types.Transaction, error) {
	return _Registry.contract.Transact(opts, "registerMerchant", merchant, payout)
}

// RegisterMerchant is a paid mutator transaction binding the contract method 0xda4e2f0e.
//
// Solidity: function registerMerchant(address merchant, address payout) returns()
func (_Registry *RegistrySession) RegisterMerchant(merchant common.Address, payout common.Address) (*types.Transaction, error) {
	return _Registry.Contract.RegisterMerchant(&_Registry.TransactOpts, merchant, payout)
}

// RegisterMerchant is a paid mutator transaction binding the contract method 0xda4e2f0e.
//
// Solidity: function registerMerchant(address merchant, address payout) returns()
func (_Registry *RegistryTransactorSession) RegisterMerchant(merchant common.Address, payout common.Address) (*types.Transaction, error) {
	return _Registry.Contract.RegisterMerchant(&_Registry.TransactOpts, merchant, payout)
}

// RevokeMerchant is a paid mutator transaction binding the contract method 0x819a701c.
//
// Solidity: function revokeMerchant(address merchant) returns()
func (_Registry *RegistryTransactor) RevokeMerchant(opts *bind.TransactOpts, merchant common.Address) (*types.Transaction, error) {
	return _Registry.contract.Transact(opts, "revokeMerchant", merchant)
}

// RevokeMerchant is a paid mutator transaction binding the contract method 0x819a701c.
//
// Solidity: function revokeMerchant(address merchant) returns()
func (_Registry *RegistrySession) RevokeMerchant(merchant common.Address) (*types.Transaction, error) {
	return _Registry.Contract.RevokeMerchant(&_Registry.TransactOpts, merchant)
}

// RevokeMerchant is a paid mutator transaction binding the contract method 0x819a701c.
//
// Solidity: function revokeMerchant(address merchant) returns()
func (_Registry *RegistryTransactorSession) RevokeMerchant(merchant common.Address) (*types.Transaction, error) {
	return _Registry.Contract.RevokeMerchant(&_Registry.TransactOpts, merchant)
}

// RegistryMerchantRegisteredIterator is returned from FilterMerchantRegistered and is used to iterate over the raw logs and unpacked data for MerchantRegistered events raised by the Registry contract.
type RegistryMerchantRegisteredIterator struct {
	Event *RegistryMerchantRegistered // Event containing the contract specifics and raw log

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
func (it *RegistryMerchantRegisteredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RegistryMerchantRegistered)
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
		it.Event = new(RegistryMerchantRegistered)
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
func (it *RegistryMerchantRegisteredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RegistryMerchantRegisteredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RegistryMerchantRegistered represents a MerchantRegistered event raised by the Registry contract.
type RegistryMerchantRegistered struct {
	Merchant common.Address
	Payout   common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterMerchantRegistered is a free log retrieval operation binding the contract event 0x5279479dfe30a6e77c7e2920ac5dc774f863f301001473d1164f1e84c1bd0f0a.
//
// Solidity: event MerchantRegistered(address indexed merchant, address payout)
func (_Registry *RegistryFilterer) FilterMerchantRegistered(opts *bind.FilterOpts, merchant []common.Address) (*RegistryMerchantRegisteredIterator, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}

	logs, sub, err := _Registry.contract.FilterLogs(opts, "MerchantRegistered", merchantRule)
	if err != nil {
		return nil, err
	}
	return &RegistryMerchantRegisteredIterator{contract: _Registry.contract, event: "MerchantRegistered", logs: logs, sub: sub}, nil
}

// WatchMerchantRegistered is a free log subscription operation binding the contract event 0x5279479dfe30a6e77c7e2920ac5dc774f863f301001473d1164f1e84c1bd0f0a.
//
// Solidity: event MerchantRegistered(address indexed merchant, address payout)
func (_Registry *RegistryFilterer) WatchMerchantRegistered(opts *bind.WatchOpts, sink chan<- *RegistryMerchantRegistered, merchant []common.Address) (event.Subscription, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}

	logs, sub, err := _Registry.contract.WatchLogs(opts, "MerchantRegistered", merchantRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RegistryMerchantRegistered)
				if err := _Registry.contract.UnpackLog(event, "MerchantRegistered", log); err != nil {
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

// ParseMerchantRegistered is a log parse operation binding the contract event 0x5279479dfe30a6e77c7e2920ac5dc774f863f301001473d1164f1e84c1bd0f0a.
//
// Solidity: event MerchantRegistered(address indexed merchant, address payout)
func (_Registry *RegistryFilterer) ParseMerchantRegistered(log types.Log) (*RegistryMerchantRegistered, error) {
	event := new(RegistryMerchantRegistered)
	if err := _Registry.contract.UnpackLog(event, "MerchantRegistered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// RegistryMerchantRevokedIterator is returned from FilterMerchantRevoked and is used to iterate over the raw logs and unpacked data for MerchantRevoked events raised by the Registry contract.
type RegistryMerchantRevokedIterator struct {
	Event *RegistryMerchantRevoked // Event containing the contract specifics and raw log

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
func (it *RegistryMerchantRevokedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RegistryMerchantRevoked)
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
		it.Event = new(RegistryMerchantRevoked)
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
func (it *RegistryMerchantRevokedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RegistryMerchantRevokedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RegistryMerchantRevoked represents a MerchantRevoked event raised by the Registry contract.
type RegistryMerchantRevoked struct {
	Merchant common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterMerchantRevoked is a free log retrieval operation binding the contract event 0x1bc5b06af60efc2e1bf6905dcc4fa4bb9153c3a9b61a0aa9568c573cbe01076b.
//
// Solidity: event MerchantRevoked(address indexed merchant)
func (_Registry *RegistryFilterer) FilterMerchantRevoked(opts *bind.FilterOpts, merchant []common.Address) (*RegistryMerchantRevokedIterator, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}

	logs, sub, err := _Registry.contract.FilterLogs(opts, "MerchantRevoked", merchantRule)
	if err != nil {
		return nil, err
	}
	return &RegistryMerchantRevokedIterator{contract: _Registry.contract, event: "MerchantRevoked", logs: logs, sub: sub}, nil
}

// WatchMerchantRevoked is a free log subscription operation binding the contract event 0x1bc5b06af60efc2e1bf6905dcc4fa4bb9153c3a9b61a0aa9568c573cbe01076b.
//
// Solidity: event MerchantRevoked(address indexed merchant)
func (_Registry *RegistryFilterer) WatchMerchantRevoked(opts *bind.WatchOpts, sink chan<- *RegistryMerchantRevoked, merchant []common.Address) (event.Subscription, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}

	logs, sub, err := _Registry.contract.WatchLogs(opts, "MerchantRevoked", merchantRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RegistryMerchantRevoked)
				if err := _Registry.contract.UnpackLog(event, "MerchantRevoked", log); err != nil {
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

// ParseMerchantRevoked is a log parse operation binding the contract event 0x1bc5b06af60efc2e1bf6905dcc4fa4bb9153c3a9b61a0aa9568c573cbe01076b.
//
// Solidity: event MerchantRevoked(address indexed merchant)
func (_Registry *RegistryFilterer) ParseMerchantRevoked(log types.Log) (*RegistryMerchantRevoked, error) {
	event := new(RegistryMerchantRevoked)
	if err := _Registry.contract.UnpackLog(event, "MerchantRevoked", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
