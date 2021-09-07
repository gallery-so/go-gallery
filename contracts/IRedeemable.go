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

// IRedeemableMetaData contains all meta data concerning the IRedeemable contract.
var IRedeemableMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_tokenID\",\"type\":\"uint256\"}],\"name\":\"isRedeemed\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"tokenID\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"redeemer\",\"type\":\"address\"}],\"name\":\"isRedeemedBy\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// IRedeemableABI is the input ABI used to generate the binding from.
// Deprecated: Use IRedeemableMetaData.ABI instead.
var IRedeemableABI = IRedeemableMetaData.ABI

// IRedeemable is an auto generated Go binding around an Ethereum contract.
type IRedeemable struct {
	IRedeemableCaller     // Read-only binding to the contract
	IRedeemableTransactor // Write-only binding to the contract
	IRedeemableFilterer   // Log filterer for contract events
}

// IRedeemableCaller is an auto generated read-only Go binding around an Ethereum contract.
type IRedeemableCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IRedeemableTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IRedeemableTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IRedeemableFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IRedeemableFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IRedeemableSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IRedeemableSession struct {
	Contract     *IRedeemable      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IRedeemableCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IRedeemableCallerSession struct {
	Contract *IRedeemableCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// IRedeemableTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IRedeemableTransactorSession struct {
	Contract     *IRedeemableTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// IRedeemableRaw is an auto generated low-level Go binding around an Ethereum contract.
type IRedeemableRaw struct {
	Contract *IRedeemable // Generic contract binding to access the raw methods on
}

// IRedeemableCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IRedeemableCallerRaw struct {
	Contract *IRedeemableCaller // Generic read-only contract binding to access the raw methods on
}

// IRedeemableTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IRedeemableTransactorRaw struct {
	Contract *IRedeemableTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIRedeemable creates a new instance of IRedeemable, bound to a specific deployed contract.
func NewIRedeemable(address common.Address, backend bind.ContractBackend) (*IRedeemable, error) {
	contract, err := bindIRedeemable(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IRedeemable{IRedeemableCaller: IRedeemableCaller{contract: contract}, IRedeemableTransactor: IRedeemableTransactor{contract: contract}, IRedeemableFilterer: IRedeemableFilterer{contract: contract}}, nil
}

// NewIRedeemableCaller creates a new read-only instance of IRedeemable, bound to a specific deployed contract.
func NewIRedeemableCaller(address common.Address, caller bind.ContractCaller) (*IRedeemableCaller, error) {
	contract, err := bindIRedeemable(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IRedeemableCaller{contract: contract}, nil
}

// NewIRedeemableTransactor creates a new write-only instance of IRedeemable, bound to a specific deployed contract.
func NewIRedeemableTransactor(address common.Address, transactor bind.ContractTransactor) (*IRedeemableTransactor, error) {
	contract, err := bindIRedeemable(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IRedeemableTransactor{contract: contract}, nil
}

// NewIRedeemableFilterer creates a new log filterer instance of IRedeemable, bound to a specific deployed contract.
func NewIRedeemableFilterer(address common.Address, filterer bind.ContractFilterer) (*IRedeemableFilterer, error) {
	contract, err := bindIRedeemable(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IRedeemableFilterer{contract: contract}, nil
}

// bindIRedeemable binds a generic wrapper to an already deployed contract.
func bindIRedeemable(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(IRedeemableABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IRedeemable *IRedeemableRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IRedeemable.Contract.IRedeemableCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IRedeemable *IRedeemableRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IRedeemable.Contract.IRedeemableTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IRedeemable *IRedeemableRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IRedeemable.Contract.IRedeemableTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IRedeemable *IRedeemableCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IRedeemable.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IRedeemable *IRedeemableTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IRedeemable.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IRedeemable *IRedeemableTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IRedeemable.Contract.contract.Transact(opts, method, params...)
}

// IsRedeemed is a free data retrieval call binding the contract method 0x32d33cd0.
//
// Solidity: function isRedeemed(uint256 _tokenID) view returns(bool)
func (_IRedeemable *IRedeemableCaller) IsRedeemed(opts *bind.CallOpts, _tokenID *big.Int) (bool, error) {
	var out []interface{}
	err := _IRedeemable.contract.Call(opts, &out, "isRedeemed", _tokenID)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsRedeemed is a free data retrieval call binding the contract method 0x32d33cd0.
//
// Solidity: function isRedeemed(uint256 _tokenID) view returns(bool)
func (_IRedeemable *IRedeemableSession) IsRedeemed(_tokenID *big.Int) (bool, error) {
	return _IRedeemable.Contract.IsRedeemed(&_IRedeemable.CallOpts, _tokenID)
}

// IsRedeemed is a free data retrieval call binding the contract method 0x32d33cd0.
//
// Solidity: function isRedeemed(uint256 _tokenID) view returns(bool)
func (_IRedeemable *IRedeemableCallerSession) IsRedeemed(_tokenID *big.Int) (bool, error) {
	return _IRedeemable.Contract.IsRedeemed(&_IRedeemable.CallOpts, _tokenID)
}

// IsRedeemedBy is a free data retrieval call binding the contract method 0xc71679b5.
//
// Solidity: function isRedeemedBy(uint256 tokenID, address redeemer) view returns(bool)
func (_IRedeemable *IRedeemableCaller) IsRedeemedBy(opts *bind.CallOpts, tokenID *big.Int, redeemer common.Address) (bool, error) {
	var out []interface{}
	err := _IRedeemable.contract.Call(opts, &out, "isRedeemedBy", tokenID, redeemer)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsRedeemedBy is a free data retrieval call binding the contract method 0xc71679b5.
//
// Solidity: function isRedeemedBy(uint256 tokenID, address redeemer) view returns(bool)
func (_IRedeemable *IRedeemableSession) IsRedeemedBy(tokenID *big.Int, redeemer common.Address) (bool, error) {
	return _IRedeemable.Contract.IsRedeemedBy(&_IRedeemable.CallOpts, tokenID, redeemer)
}

// IsRedeemedBy is a free data retrieval call binding the contract method 0xc71679b5.
//
// Solidity: function isRedeemedBy(uint256 tokenID, address redeemer) view returns(bool)
func (_IRedeemable *IRedeemableCallerSession) IsRedeemedBy(tokenID *big.Int, redeemer common.Address) (bool, error) {
	return _IRedeemable.Contract.IsRedeemedBy(&_IRedeemable.CallOpts, tokenID, redeemer)
}

