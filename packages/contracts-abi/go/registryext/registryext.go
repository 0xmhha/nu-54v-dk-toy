// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package registryext

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

// RegistryextMetaData contains all meta data concerning the Registryext contract.
var RegistryextMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"cancelPayoutChange\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"merchantStatus\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"active\",\"type\":\"bool\"},{\"internalType\":\"address\",\"name\":\"payout\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"payoutChangeDelay\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"pendingPayoutOf\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"payout\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"effectiveAt\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"payout\",\"type\":\"address\"}],\"name\":\"requestPayoutChange\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"}],\"name\":\"PayoutChangeCancelled\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"merchant\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"payout\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"effectiveAt\",\"type\":\"uint256\"}],\"name\":\"PayoutChangeQueued\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"InvalidParameters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NoPendingPayoutChange\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotAdminOrMerchant\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"PayoutChangeRequired\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"UnknownMerchant\",\"type\":\"error\"}]",
}

// RegistryextABI is the input ABI used to generate the binding from.
// Deprecated: Use RegistryextMetaData.ABI instead.
var RegistryextABI = RegistryextMetaData.ABI

// Registryext is an auto generated Go binding around an Ethereum contract.
type Registryext struct {
	RegistryextCaller     // Read-only binding to the contract
	RegistryextTransactor // Write-only binding to the contract
	RegistryextFilterer   // Log filterer for contract events
}

// RegistryextCaller is an auto generated read-only Go binding around an Ethereum contract.
type RegistryextCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RegistryextTransactor is an auto generated write-only Go binding around an Ethereum contract.
type RegistryextTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RegistryextFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type RegistryextFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RegistryextSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type RegistryextSession struct {
	Contract     *Registryext      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// RegistryextCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type RegistryextCallerSession struct {
	Contract *RegistryextCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// RegistryextTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type RegistryextTransactorSession struct {
	Contract     *RegistryextTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// RegistryextRaw is an auto generated low-level Go binding around an Ethereum contract.
type RegistryextRaw struct {
	Contract *Registryext // Generic contract binding to access the raw methods on
}

// RegistryextCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type RegistryextCallerRaw struct {
	Contract *RegistryextCaller // Generic read-only contract binding to access the raw methods on
}

// RegistryextTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type RegistryextTransactorRaw struct {
	Contract *RegistryextTransactor // Generic write-only contract binding to access the raw methods on
}

// NewRegistryext creates a new instance of Registryext, bound to a specific deployed contract.
func NewRegistryext(address common.Address, backend bind.ContractBackend) (*Registryext, error) {
	contract, err := bindRegistryext(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Registryext{RegistryextCaller: RegistryextCaller{contract: contract}, RegistryextTransactor: RegistryextTransactor{contract: contract}, RegistryextFilterer: RegistryextFilterer{contract: contract}}, nil
}

// NewRegistryextCaller creates a new read-only instance of Registryext, bound to a specific deployed contract.
func NewRegistryextCaller(address common.Address, caller bind.ContractCaller) (*RegistryextCaller, error) {
	contract, err := bindRegistryext(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &RegistryextCaller{contract: contract}, nil
}

// NewRegistryextTransactor creates a new write-only instance of Registryext, bound to a specific deployed contract.
func NewRegistryextTransactor(address common.Address, transactor bind.ContractTransactor) (*RegistryextTransactor, error) {
	contract, err := bindRegistryext(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &RegistryextTransactor{contract: contract}, nil
}

// NewRegistryextFilterer creates a new log filterer instance of Registryext, bound to a specific deployed contract.
func NewRegistryextFilterer(address common.Address, filterer bind.ContractFilterer) (*RegistryextFilterer, error) {
	contract, err := bindRegistryext(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &RegistryextFilterer{contract: contract}, nil
}

// bindRegistryext binds a generic wrapper to an already deployed contract.
func bindRegistryext(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := RegistryextMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Registryext *RegistryextRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Registryext.Contract.RegistryextCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Registryext *RegistryextRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Registryext.Contract.RegistryextTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Registryext *RegistryextRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Registryext.Contract.RegistryextTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Registryext *RegistryextCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Registryext.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Registryext *RegistryextTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Registryext.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Registryext *RegistryextTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Registryext.Contract.contract.Transact(opts, method, params...)
}

// MerchantStatus is a free data retrieval call binding the contract method 0x235ce474.
//
// Solidity: function merchantStatus(address merchant) view returns(bool active, address payout)
func (_Registryext *RegistryextCaller) MerchantStatus(opts *bind.CallOpts, merchant common.Address) (struct {
	Active bool
	Payout common.Address
}, error) {
	var out []interface{}
	err := _Registryext.contract.Call(opts, &out, "merchantStatus", merchant)

	outstruct := new(struct {
		Active bool
		Payout common.Address
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Active = *abi.ConvertType(out[0], new(bool)).(*bool)
	outstruct.Payout = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)

	return *outstruct, err

}

// MerchantStatus is a free data retrieval call binding the contract method 0x235ce474.
//
// Solidity: function merchantStatus(address merchant) view returns(bool active, address payout)
func (_Registryext *RegistryextSession) MerchantStatus(merchant common.Address) (struct {
	Active bool
	Payout common.Address
}, error) {
	return _Registryext.Contract.MerchantStatus(&_Registryext.CallOpts, merchant)
}

// MerchantStatus is a free data retrieval call binding the contract method 0x235ce474.
//
// Solidity: function merchantStatus(address merchant) view returns(bool active, address payout)
func (_Registryext *RegistryextCallerSession) MerchantStatus(merchant common.Address) (struct {
	Active bool
	Payout common.Address
}, error) {
	return _Registryext.Contract.MerchantStatus(&_Registryext.CallOpts, merchant)
}

// PayoutChangeDelay is a free data retrieval call binding the contract method 0x92c12c75.
//
// Solidity: function payoutChangeDelay() view returns(uint256)
func (_Registryext *RegistryextCaller) PayoutChangeDelay(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Registryext.contract.Call(opts, &out, "payoutChangeDelay")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// PayoutChangeDelay is a free data retrieval call binding the contract method 0x92c12c75.
//
// Solidity: function payoutChangeDelay() view returns(uint256)
func (_Registryext *RegistryextSession) PayoutChangeDelay() (*big.Int, error) {
	return _Registryext.Contract.PayoutChangeDelay(&_Registryext.CallOpts)
}

// PayoutChangeDelay is a free data retrieval call binding the contract method 0x92c12c75.
//
// Solidity: function payoutChangeDelay() view returns(uint256)
func (_Registryext *RegistryextCallerSession) PayoutChangeDelay() (*big.Int, error) {
	return _Registryext.Contract.PayoutChangeDelay(&_Registryext.CallOpts)
}

// PendingPayoutOf is a free data retrieval call binding the contract method 0x439ad1a9.
//
// Solidity: function pendingPayoutOf(address merchant) view returns(address payout, uint256 effectiveAt)
func (_Registryext *RegistryextCaller) PendingPayoutOf(opts *bind.CallOpts, merchant common.Address) (struct {
	Payout      common.Address
	EffectiveAt *big.Int
}, error) {
	var out []interface{}
	err := _Registryext.contract.Call(opts, &out, "pendingPayoutOf", merchant)

	outstruct := new(struct {
		Payout      common.Address
		EffectiveAt *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Payout = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.EffectiveAt = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// PendingPayoutOf is a free data retrieval call binding the contract method 0x439ad1a9.
//
// Solidity: function pendingPayoutOf(address merchant) view returns(address payout, uint256 effectiveAt)
func (_Registryext *RegistryextSession) PendingPayoutOf(merchant common.Address) (struct {
	Payout      common.Address
	EffectiveAt *big.Int
}, error) {
	return _Registryext.Contract.PendingPayoutOf(&_Registryext.CallOpts, merchant)
}

// PendingPayoutOf is a free data retrieval call binding the contract method 0x439ad1a9.
//
// Solidity: function pendingPayoutOf(address merchant) view returns(address payout, uint256 effectiveAt)
func (_Registryext *RegistryextCallerSession) PendingPayoutOf(merchant common.Address) (struct {
	Payout      common.Address
	EffectiveAt *big.Int
}, error) {
	return _Registryext.Contract.PendingPayoutOf(&_Registryext.CallOpts, merchant)
}

// CancelPayoutChange is a paid mutator transaction binding the contract method 0x23da5ffa.
//
// Solidity: function cancelPayoutChange(address merchant) returns()
func (_Registryext *RegistryextTransactor) CancelPayoutChange(opts *bind.TransactOpts, merchant common.Address) (*types.Transaction, error) {
	return _Registryext.contract.Transact(opts, "cancelPayoutChange", merchant)
}

// CancelPayoutChange is a paid mutator transaction binding the contract method 0x23da5ffa.
//
// Solidity: function cancelPayoutChange(address merchant) returns()
func (_Registryext *RegistryextSession) CancelPayoutChange(merchant common.Address) (*types.Transaction, error) {
	return _Registryext.Contract.CancelPayoutChange(&_Registryext.TransactOpts, merchant)
}

// CancelPayoutChange is a paid mutator transaction binding the contract method 0x23da5ffa.
//
// Solidity: function cancelPayoutChange(address merchant) returns()
func (_Registryext *RegistryextTransactorSession) CancelPayoutChange(merchant common.Address) (*types.Transaction, error) {
	return _Registryext.Contract.CancelPayoutChange(&_Registryext.TransactOpts, merchant)
}

// RequestPayoutChange is a paid mutator transaction binding the contract method 0x018d9e89.
//
// Solidity: function requestPayoutChange(address merchant, address payout) returns()
func (_Registryext *RegistryextTransactor) RequestPayoutChange(opts *bind.TransactOpts, merchant common.Address, payout common.Address) (*types.Transaction, error) {
	return _Registryext.contract.Transact(opts, "requestPayoutChange", merchant, payout)
}

// RequestPayoutChange is a paid mutator transaction binding the contract method 0x018d9e89.
//
// Solidity: function requestPayoutChange(address merchant, address payout) returns()
func (_Registryext *RegistryextSession) RequestPayoutChange(merchant common.Address, payout common.Address) (*types.Transaction, error) {
	return _Registryext.Contract.RequestPayoutChange(&_Registryext.TransactOpts, merchant, payout)
}

// RequestPayoutChange is a paid mutator transaction binding the contract method 0x018d9e89.
//
// Solidity: function requestPayoutChange(address merchant, address payout) returns()
func (_Registryext *RegistryextTransactorSession) RequestPayoutChange(merchant common.Address, payout common.Address) (*types.Transaction, error) {
	return _Registryext.Contract.RequestPayoutChange(&_Registryext.TransactOpts, merchant, payout)
}

// RegistryextPayoutChangeCancelledIterator is returned from FilterPayoutChangeCancelled and is used to iterate over the raw logs and unpacked data for PayoutChangeCancelled events raised by the Registryext contract.
type RegistryextPayoutChangeCancelledIterator struct {
	Event *RegistryextPayoutChangeCancelled // Event containing the contract specifics and raw log

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
func (it *RegistryextPayoutChangeCancelledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RegistryextPayoutChangeCancelled)
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
		it.Event = new(RegistryextPayoutChangeCancelled)
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
func (it *RegistryextPayoutChangeCancelledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RegistryextPayoutChangeCancelledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RegistryextPayoutChangeCancelled represents a PayoutChangeCancelled event raised by the Registryext contract.
type RegistryextPayoutChangeCancelled struct {
	Merchant common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterPayoutChangeCancelled is a free log retrieval operation binding the contract event 0xbd977badcdca76a5cda5ddc4387b8f90feb04cb8437c652386cb7e41656e4cc0.
//
// Solidity: event PayoutChangeCancelled(address indexed merchant)
func (_Registryext *RegistryextFilterer) FilterPayoutChangeCancelled(opts *bind.FilterOpts, merchant []common.Address) (*RegistryextPayoutChangeCancelledIterator, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}

	logs, sub, err := _Registryext.contract.FilterLogs(opts, "PayoutChangeCancelled", merchantRule)
	if err != nil {
		return nil, err
	}
	return &RegistryextPayoutChangeCancelledIterator{contract: _Registryext.contract, event: "PayoutChangeCancelled", logs: logs, sub: sub}, nil
}

// WatchPayoutChangeCancelled is a free log subscription operation binding the contract event 0xbd977badcdca76a5cda5ddc4387b8f90feb04cb8437c652386cb7e41656e4cc0.
//
// Solidity: event PayoutChangeCancelled(address indexed merchant)
func (_Registryext *RegistryextFilterer) WatchPayoutChangeCancelled(opts *bind.WatchOpts, sink chan<- *RegistryextPayoutChangeCancelled, merchant []common.Address) (event.Subscription, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}

	logs, sub, err := _Registryext.contract.WatchLogs(opts, "PayoutChangeCancelled", merchantRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RegistryextPayoutChangeCancelled)
				if err := _Registryext.contract.UnpackLog(event, "PayoutChangeCancelled", log); err != nil {
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

// ParsePayoutChangeCancelled is a log parse operation binding the contract event 0xbd977badcdca76a5cda5ddc4387b8f90feb04cb8437c652386cb7e41656e4cc0.
//
// Solidity: event PayoutChangeCancelled(address indexed merchant)
func (_Registryext *RegistryextFilterer) ParsePayoutChangeCancelled(log types.Log) (*RegistryextPayoutChangeCancelled, error) {
	event := new(RegistryextPayoutChangeCancelled)
	if err := _Registryext.contract.UnpackLog(event, "PayoutChangeCancelled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// RegistryextPayoutChangeQueuedIterator is returned from FilterPayoutChangeQueued and is used to iterate over the raw logs and unpacked data for PayoutChangeQueued events raised by the Registryext contract.
type RegistryextPayoutChangeQueuedIterator struct {
	Event *RegistryextPayoutChangeQueued // Event containing the contract specifics and raw log

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
func (it *RegistryextPayoutChangeQueuedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RegistryextPayoutChangeQueued)
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
		it.Event = new(RegistryextPayoutChangeQueued)
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
func (it *RegistryextPayoutChangeQueuedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RegistryextPayoutChangeQueuedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RegistryextPayoutChangeQueued represents a PayoutChangeQueued event raised by the Registryext contract.
type RegistryextPayoutChangeQueued struct {
	Merchant    common.Address
	Payout      common.Address
	EffectiveAt *big.Int
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterPayoutChangeQueued is a free log retrieval operation binding the contract event 0x289193c3f258c27166f3727736c6534fe4323ccf3ea4941c2eb013b6640c7302.
//
// Solidity: event PayoutChangeQueued(address indexed merchant, address payout, uint256 effectiveAt)
func (_Registryext *RegistryextFilterer) FilterPayoutChangeQueued(opts *bind.FilterOpts, merchant []common.Address) (*RegistryextPayoutChangeQueuedIterator, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}

	logs, sub, err := _Registryext.contract.FilterLogs(opts, "PayoutChangeQueued", merchantRule)
	if err != nil {
		return nil, err
	}
	return &RegistryextPayoutChangeQueuedIterator{contract: _Registryext.contract, event: "PayoutChangeQueued", logs: logs, sub: sub}, nil
}

// WatchPayoutChangeQueued is a free log subscription operation binding the contract event 0x289193c3f258c27166f3727736c6534fe4323ccf3ea4941c2eb013b6640c7302.
//
// Solidity: event PayoutChangeQueued(address indexed merchant, address payout, uint256 effectiveAt)
func (_Registryext *RegistryextFilterer) WatchPayoutChangeQueued(opts *bind.WatchOpts, sink chan<- *RegistryextPayoutChangeQueued, merchant []common.Address) (event.Subscription, error) {

	var merchantRule []interface{}
	for _, merchantItem := range merchant {
		merchantRule = append(merchantRule, merchantItem)
	}

	logs, sub, err := _Registryext.contract.WatchLogs(opts, "PayoutChangeQueued", merchantRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RegistryextPayoutChangeQueued)
				if err := _Registryext.contract.UnpackLog(event, "PayoutChangeQueued", log); err != nil {
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

// ParsePayoutChangeQueued is a log parse operation binding the contract event 0x289193c3f258c27166f3727736c6534fe4323ccf3ea4941c2eb013b6640c7302.
//
// Solidity: event PayoutChangeQueued(address indexed merchant, address payout, uint256 effectiveAt)
func (_Registryext *RegistryextFilterer) ParsePayoutChangeQueued(log types.Log) (*RegistryextPayoutChangeQueued, error) {
	event := new(RegistryextPayoutChangeQueued)
	if err := _Registryext.contract.UnpackLog(event, "PayoutChangeQueued", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
