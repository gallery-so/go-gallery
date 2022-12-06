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

// MerchMetaData contains all meta data concerning the Merch contract.
var MerchMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"tokenId\",\"type\":\"uint256\"}],\"name\":\"isRedeemed\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256[]\",\"name\":\"tokenIDs\",\"type\":\"uint256[]\"}],\"name\":\"redeem\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"tokenId\",\"type\":\"uint256\"}],\"name\":\"tokenURI\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// MerchABI is the input ABI used to generate the binding from.
// Deprecated: Use MerchMetaData.ABI instead.
var MerchABI = MerchMetaData.ABI

// Merch is an auto generated Go binding around an Ethereum contract.
type Merch struct {
	MerchCaller     // Read-only binding to the contract
	MerchTransactor // Write-only binding to the contract
	MerchFilterer   // Log filterer for contract events
}

// MerchCaller is an auto generated read-only Go binding around an Ethereum contract.
type MerchCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MerchTransactor is an auto generated write-only Go binding around an Ethereum contract.
type MerchTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MerchFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type MerchFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MerchSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type MerchSession struct {
	Contract     *Merch            // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// MerchCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type MerchCallerSession struct {
	Contract *MerchCaller  // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// MerchTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type MerchTransactorSession struct {
	Contract     *MerchTransactor  // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// MerchRaw is an auto generated low-level Go binding around an Ethereum contract.
type MerchRaw struct {
	Contract *Merch // Generic contract binding to access the raw methods on
}

// MerchCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type MerchCallerRaw struct {
	Contract *MerchCaller // Generic read-only contract binding to access the raw methods on
}

// MerchTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type MerchTransactorRaw struct {
	Contract *MerchTransactor // Generic write-only contract binding to access the raw methods on
}

// NewMerch creates a new instance of Merch, bound to a specific deployed contract.
func NewMerch(address common.Address, backend bind.ContractBackend) (*Merch, error) {
	contract, err := bindMerch(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Merch{MerchCaller: MerchCaller{contract: contract}, MerchTransactor: MerchTransactor{contract: contract}, MerchFilterer: MerchFilterer{contract: contract}}, nil
}

// NewMerchCaller creates a new read-only instance of Merch, bound to a specific deployed contract.
func NewMerchCaller(address common.Address, caller bind.ContractCaller) (*MerchCaller, error) {
	contract, err := bindMerch(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &MerchCaller{contract: contract}, nil
}

// NewMerchTransactor creates a new write-only instance of Merch, bound to a specific deployed contract.
func NewMerchTransactor(address common.Address, transactor bind.ContractTransactor) (*MerchTransactor, error) {
	contract, err := bindMerch(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &MerchTransactor{contract: contract}, nil
}

// NewMerchFilterer creates a new log filterer instance of Merch, bound to a specific deployed contract.
func NewMerchFilterer(address common.Address, filterer bind.ContractFilterer) (*MerchFilterer, error) {
	contract, err := bindMerch(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &MerchFilterer{contract: contract}, nil
}

// bindMerch binds a generic wrapper to an already deployed contract.
func bindMerch(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(MerchABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Merch *MerchRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Merch.Contract.MerchCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Merch *MerchRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Merch.Contract.MerchTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Merch *MerchRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Merch.Contract.MerchTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Merch *MerchCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Merch.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Merch *MerchTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Merch.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Merch *MerchTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Merch.Contract.contract.Transact(opts, method, params...)
}

// IsRedeemed is a free data retrieval call binding the contract method 0x32d33cd0.
//
// Solidity: function isRedeemed(uint256 tokenId) view returns(bool)
func (_Merch *MerchCaller) IsRedeemed(opts *bind.CallOpts, tokenId *big.Int) (bool, error) {
	var out []interface{}
	err := _Merch.contract.Call(opts, &out, "isRedeemed", tokenId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsRedeemed is a free data retrieval call binding the contract method 0x32d33cd0.
//
// Solidity: function isRedeemed(uint256 tokenId) view returns(bool)
func (_Merch *MerchSession) IsRedeemed(tokenId *big.Int) (bool, error) {
	return _Merch.Contract.IsRedeemed(&_Merch.CallOpts, tokenId)
}

// IsRedeemed is a free data retrieval call binding the contract method 0x32d33cd0.
//
// Solidity: function isRedeemed(uint256 tokenId) view returns(bool)
func (_Merch *MerchCallerSession) IsRedeemed(tokenId *big.Int) (bool, error) {
	return _Merch.Contract.IsRedeemed(&_Merch.CallOpts, tokenId)
}

// TokenURI is a free data retrieval call binding the contract method 0xc87b56dd.
//
// Solidity: function tokenURI(uint256 tokenId) view returns(string)
func (_Merch *MerchCaller) TokenURI(opts *bind.CallOpts, tokenId *big.Int) (string, error) {
	var out []interface{}
	err := _Merch.contract.Call(opts, &out, "tokenURI", tokenId)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// TokenURI is a free data retrieval call binding the contract method 0xc87b56dd.
//
// Solidity: function tokenURI(uint256 tokenId) view returns(string)
func (_Merch *MerchSession) TokenURI(tokenId *big.Int) (string, error) {
	return _Merch.Contract.TokenURI(&_Merch.CallOpts, tokenId)
}

// TokenURI is a free data retrieval call binding the contract method 0xc87b56dd.
//
// Solidity: function tokenURI(uint256 tokenId) view returns(string)
func (_Merch *MerchCallerSession) TokenURI(tokenId *big.Int) (string, error) {
	return _Merch.Contract.TokenURI(&_Merch.CallOpts, tokenId)
}

// Redeem is a paid mutator transaction binding the contract method 0xf9afb26a.
//
// Solidity: function redeem(uint256[] tokenIDs) returns()
func (_Merch *MerchTransactor) Redeem(opts *bind.TransactOpts, tokenIDs []*big.Int) (*types.Transaction, error) {
	return _Merch.contract.Transact(opts, "redeem", tokenIDs)
}

// Redeem is a paid mutator transaction binding the contract method 0xf9afb26a.
//
// Solidity: function redeem(uint256[] tokenIDs) returns()
func (_Merch *MerchSession) Redeem(tokenIDs []*big.Int) (*types.Transaction, error) {
	return _Merch.Contract.Redeem(&_Merch.TransactOpts, tokenIDs)
}

// Redeem is a paid mutator transaction binding the contract method 0xf9afb26a.
//
// Solidity: function redeem(uint256[] tokenIDs) returns()
func (_Merch *MerchTransactorSession) Redeem(tokenIDs []*big.Int) (*types.Transaction, error) {
	return _Merch.Contract.Redeem(&_Merch.TransactOpts, tokenIDs)
}

