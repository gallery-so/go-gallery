// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package contracts

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
)

// RedeemableMetaData contains all meta data concerning the Redeemable contract.
var RedeemableMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"redeemer\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"tokenID\",\"type\":\"uint256\"}],\"name\":\"Redeem\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_tokenID\",\"type\":\"uint256\"}],\"name\":\"isRedeemed\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"tokenID\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"redeemer\",\"type\":\"address\"}],\"name\":\"isRedeemedBy\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_tokenID\",\"type\":\"uint256\"}],\"name\":\"redeem\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// RedeemableABI is the input ABI used to generate the binding from.
// Deprecated: Use RedeemableMetaData.ABI instead.
var RedeemableABI = RedeemableMetaData.ABI

// Redeemable is an auto generated Go binding around an Ethereum contract.
type Redeemable struct {
	RedeemableCaller     // Read-only binding to the contract
	RedeemableTransactor // Write-only binding to the contract
	RedeemableFilterer   // Log filterer for contract events
}

// RedeemableCaller is an auto generated read-only Go binding around an Ethereum contract.
type RedeemableCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RedeemableTransactor is an auto generated write-only Go binding around an Ethereum contract.
type RedeemableTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RedeemableFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type RedeemableFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RedeemableSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type RedeemableSession struct {
	Contract     *Redeemable       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// RedeemableCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type RedeemableCallerSession struct {
	Contract *RedeemableCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// RedeemableTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type RedeemableTransactorSession struct {
	Contract     *RedeemableTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// RedeemableRaw is an auto generated low-level Go binding around an Ethereum contract.
type RedeemableRaw struct {
	Contract *Redeemable // Generic contract binding to access the raw methods on
}

// RedeemableCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type RedeemableCallerRaw struct {
	Contract *RedeemableCaller // Generic read-only contract binding to access the raw methods on
}

// RedeemableTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type RedeemableTransactorRaw struct {
	Contract *RedeemableTransactor // Generic write-only contract binding to access the raw methods on
}

// NewRedeemable creates a new instance of Redeemable, bound to a specific deployed contract.
func NewRedeemable(address common.Address, backend bind.ContractBackend) (*Redeemable, error) {
	contract, err := bindRedeemable(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Redeemable{RedeemableCaller: RedeemableCaller{contract: contract}, RedeemableTransactor: RedeemableTransactor{contract: contract}, RedeemableFilterer: RedeemableFilterer{contract: contract}}, nil
}

// NewRedeemableCaller creates a new read-only instance of Redeemable, bound to a specific deployed contract.
func NewRedeemableCaller(address common.Address, caller bind.ContractCaller) (*RedeemableCaller, error) {
	contract, err := bindRedeemable(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &RedeemableCaller{contract: contract}, nil
}

// NewRedeemableTransactor creates a new write-only instance of Redeemable, bound to a specific deployed contract.
func NewRedeemableTransactor(address common.Address, transactor bind.ContractTransactor) (*RedeemableTransactor, error) {
	contract, err := bindRedeemable(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &RedeemableTransactor{contract: contract}, nil
}

// NewRedeemableFilterer creates a new log filterer instance of Redeemable, bound to a specific deployed contract.
func NewRedeemableFilterer(address common.Address, filterer bind.ContractFilterer) (*RedeemableFilterer, error) {
	contract, err := bindRedeemable(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &RedeemableFilterer{contract: contract}, nil
}

// bindRedeemable binds a generic wrapper to an already deployed contract.
func bindRedeemable(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(RedeemableABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Redeemable *RedeemableRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Redeemable.Contract.RedeemableCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Redeemable *RedeemableRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Redeemable.Contract.RedeemableTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Redeemable *RedeemableRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Redeemable.Contract.RedeemableTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Redeemable *RedeemableCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Redeemable.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Redeemable *RedeemableTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Redeemable.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Redeemable *RedeemableTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Redeemable.Contract.contract.Transact(opts, method, params...)
}

// IsRedeemed is a free data retrieval call binding the contract method 0x32d33cd0.
//
// Solidity: function isRedeemed(uint256 _tokenID) view returns(bool)
func (_Redeemable *RedeemableCaller) IsRedeemed(opts *bind.CallOpts, _tokenID *big.Int) (bool, error) {
	var out []interface{}
	err := _Redeemable.contract.Call(opts, &out, "isRedeemed", _tokenID)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsRedeemed is a free data retrieval call binding the contract method 0x32d33cd0.
//
// Solidity: function isRedeemed(uint256 _tokenID) view returns(bool)
func (_Redeemable *RedeemableSession) IsRedeemed(_tokenID *big.Int) (bool, error) {
	return _Redeemable.Contract.IsRedeemed(&_Redeemable.CallOpts, _tokenID)
}

// IsRedeemed is a free data retrieval call binding the contract method 0x32d33cd0.
//
// Solidity: function isRedeemed(uint256 _tokenID) view returns(bool)
func (_Redeemable *RedeemableCallerSession) IsRedeemed(_tokenID *big.Int) (bool, error) {
	return _Redeemable.Contract.IsRedeemed(&_Redeemable.CallOpts, _tokenID)
}

// IsRedeemedBy is a free data retrieval call binding the contract method 0xc71679b5.
//
// Solidity: function isRedeemedBy(uint256 tokenID, address redeemer) view returns(bool)
func (_Redeemable *RedeemableCaller) IsRedeemedBy(opts *bind.CallOpts, tokenID *big.Int, redeemer common.Address) (bool, error) {
	var out []interface{}
	err := _Redeemable.contract.Call(opts, &out, "isRedeemedBy", tokenID, redeemer)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsRedeemedBy is a free data retrieval call binding the contract method 0xc71679b5.
//
// Solidity: function isRedeemedBy(uint256 tokenID, address redeemer) view returns(bool)
func (_Redeemable *RedeemableSession) IsRedeemedBy(tokenID *big.Int, redeemer common.Address) (bool, error) {
	return _Redeemable.Contract.IsRedeemedBy(&_Redeemable.CallOpts, tokenID, redeemer)
}

// IsRedeemedBy is a free data retrieval call binding the contract method 0xc71679b5.
//
// Solidity: function isRedeemedBy(uint256 tokenID, address redeemer) view returns(bool)
func (_Redeemable *RedeemableCallerSession) IsRedeemedBy(tokenID *big.Int, redeemer common.Address) (bool, error) {
	return _Redeemable.Contract.IsRedeemedBy(&_Redeemable.CallOpts, tokenID, redeemer)
}

// Redeem is a paid mutator transaction binding the contract method 0xdb006a75.
//
// Solidity: function redeem(uint256 _tokenID) returns()
func (_Redeemable *RedeemableTransactor) Redeem(opts *bind.TransactOpts, _tokenID *big.Int) (*types.Transaction, error) {
	return _Redeemable.contract.Transact(opts, "redeem", _tokenID)
}

// Redeem is a paid mutator transaction binding the contract method 0xdb006a75.
//
// Solidity: function redeem(uint256 _tokenID) returns()
func (_Redeemable *RedeemableSession) Redeem(_tokenID *big.Int) (*types.Transaction, error) {
	return _Redeemable.Contract.Redeem(&_Redeemable.TransactOpts, _tokenID)
}

// Redeem is a paid mutator transaction binding the contract method 0xdb006a75.
//
// Solidity: function redeem(uint256 _tokenID) returns()
func (_Redeemable *RedeemableTransactorSession) Redeem(_tokenID *big.Int) (*types.Transaction, error) {
	return _Redeemable.Contract.Redeem(&_Redeemable.TransactOpts, _tokenID)
}

// RedeemableRedeemIterator is returned from FilterRedeem and is used to iterate over the raw logs and unpacked data for Redeem events raised by the Redeemable contract.
type RedeemableRedeemIterator struct {
	Event *RedeemableRedeem // Event containing the contract specifics and raw log

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
func (it *RedeemableRedeemIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RedeemableRedeem)
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
		it.Event = new(RedeemableRedeem)
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
func (it *RedeemableRedeemIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RedeemableRedeemIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RedeemableRedeem represents a Redeem event raised by the Redeemable contract.
type RedeemableRedeem struct {
	Redeemer common.Address
	TokenID  *big.Int
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterRedeem is a free log retrieval operation binding the contract event 0x222838db2794d11532d940e8dec38ae307ed0b63cd97c233322e221f998767a6.
//
// Solidity: event Redeem(address indexed redeemer, uint256 indexed tokenID)
func (_Redeemable *RedeemableFilterer) FilterRedeem(opts *bind.FilterOpts, redeemer []common.Address, tokenID []*big.Int) (*RedeemableRedeemIterator, error) {

	var redeemerRule []interface{}
	for _, redeemerItem := range redeemer {
		redeemerRule = append(redeemerRule, redeemerItem)
	}
	var tokenIDRule []interface{}
	for _, tokenIDItem := range tokenID {
		tokenIDRule = append(tokenIDRule, tokenIDItem)
	}

	logs, sub, err := _Redeemable.contract.FilterLogs(opts, "Redeem", redeemerRule, tokenIDRule)
	if err != nil {
		return nil, err
	}
	return &RedeemableRedeemIterator{contract: _Redeemable.contract, event: "Redeem", logs: logs, sub: sub}, nil
}

// WatchRedeem is a free log subscription operation binding the contract event 0x222838db2794d11532d940e8dec38ae307ed0b63cd97c233322e221f998767a6.
//
// Solidity: event Redeem(address indexed redeemer, uint256 indexed tokenID)
func (_Redeemable *RedeemableFilterer) WatchRedeem(opts *bind.WatchOpts, sink chan<- *RedeemableRedeem, redeemer []common.Address, tokenID []*big.Int) (event.Subscription, error) {

	var redeemerRule []interface{}
	for _, redeemerItem := range redeemer {
		redeemerRule = append(redeemerRule, redeemerItem)
	}
	var tokenIDRule []interface{}
	for _, tokenIDItem := range tokenID {
		tokenIDRule = append(tokenIDRule, tokenIDItem)
	}

	logs, sub, err := _Redeemable.contract.WatchLogs(opts, "Redeem", redeemerRule, tokenIDRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RedeemableRedeem)
				if err := _Redeemable.contract.UnpackLog(event, "Redeem", log); err != nil {
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

// ParseRedeem is a log parse operation binding the contract event 0x222838db2794d11532d940e8dec38ae307ed0b63cd97c233322e221f998767a6.
//
// Solidity: event Redeem(address indexed redeemer, uint256 indexed tokenID)
func (_Redeemable *RedeemableFilterer) ParseRedeem(log types.Log) (*RedeemableRedeem, error) {
	event := new(RedeemableRedeem)
	if err := _Redeemable.contract.UnpackLog(event, "Redeem", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

