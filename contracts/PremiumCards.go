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
	_ = abi.ConvertType
)

// PremiumCardsMetaData contains all meta data concerning the PremiumCards contract.
var PremiumCardsMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address[]\",\"name\":\"_to\",\"type\":\"address[]\"},{\"internalType\":\"uint256\",\"name\":\"_id\",\"type\":\"uint256\"}],\"name\":\"mintToMany\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// PremiumCardsABI is the input ABI used to generate the binding from.
// Deprecated: Use PremiumCardsMetaData.ABI instead.
var PremiumCardsABI = PremiumCardsMetaData.ABI

// PremiumCards is an auto generated Go binding around an Ethereum contract.
type PremiumCards struct {
	PremiumCardsCaller     // Read-only binding to the contract
	PremiumCardsTransactor // Write-only binding to the contract
	PremiumCardsFilterer   // Log filterer for contract events
}

// PremiumCardsCaller is an auto generated read-only Go binding around an Ethereum contract.
type PremiumCardsCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// PremiumCardsTransactor is an auto generated write-only Go binding around an Ethereum contract.
type PremiumCardsTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// PremiumCardsFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type PremiumCardsFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// PremiumCardsSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type PremiumCardsSession struct {
	Contract     *PremiumCards     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// PremiumCardsCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type PremiumCardsCallerSession struct {
	Contract *PremiumCardsCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// PremiumCardsTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type PremiumCardsTransactorSession struct {
	Contract     *PremiumCardsTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// PremiumCardsRaw is an auto generated low-level Go binding around an Ethereum contract.
type PremiumCardsRaw struct {
	Contract *PremiumCards // Generic contract binding to access the raw methods on
}

// PremiumCardsCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type PremiumCardsCallerRaw struct {
	Contract *PremiumCardsCaller // Generic read-only contract binding to access the raw methods on
}

// PremiumCardsTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type PremiumCardsTransactorRaw struct {
	Contract *PremiumCardsTransactor // Generic write-only contract binding to access the raw methods on
}

// NewPremiumCards creates a new instance of PremiumCards, bound to a specific deployed contract.
func NewPremiumCards(address common.Address, backend bind.ContractBackend) (*PremiumCards, error) {
	contract, err := bindPremiumCards(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &PremiumCards{PremiumCardsCaller: PremiumCardsCaller{contract: contract}, PremiumCardsTransactor: PremiumCardsTransactor{contract: contract}, PremiumCardsFilterer: PremiumCardsFilterer{contract: contract}}, nil
}

// NewPremiumCardsCaller creates a new read-only instance of PremiumCards, bound to a specific deployed contract.
func NewPremiumCardsCaller(address common.Address, caller bind.ContractCaller) (*PremiumCardsCaller, error) {
	contract, err := bindPremiumCards(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &PremiumCardsCaller{contract: contract}, nil
}

// NewPremiumCardsTransactor creates a new write-only instance of PremiumCards, bound to a specific deployed contract.
func NewPremiumCardsTransactor(address common.Address, transactor bind.ContractTransactor) (*PremiumCardsTransactor, error) {
	contract, err := bindPremiumCards(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &PremiumCardsTransactor{contract: contract}, nil
}

// NewPremiumCardsFilterer creates a new log filterer instance of PremiumCards, bound to a specific deployed contract.
func NewPremiumCardsFilterer(address common.Address, filterer bind.ContractFilterer) (*PremiumCardsFilterer, error) {
	contract, err := bindPremiumCards(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &PremiumCardsFilterer{contract: contract}, nil
}

// bindPremiumCards binds a generic wrapper to an already deployed contract.
func bindPremiumCards(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := PremiumCardsMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_PremiumCards *PremiumCardsRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _PremiumCards.Contract.PremiumCardsCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_PremiumCards *PremiumCardsRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _PremiumCards.Contract.PremiumCardsTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_PremiumCards *PremiumCardsRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _PremiumCards.Contract.PremiumCardsTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_PremiumCards *PremiumCardsCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _PremiumCards.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_PremiumCards *PremiumCardsTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _PremiumCards.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_PremiumCards *PremiumCardsTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _PremiumCards.Contract.contract.Transact(opts, method, params...)
}

// MintToMany is a paid mutator transaction binding the contract method 0x8a0b0360.
//
// Solidity: function mintToMany(address[] _to, uint256 _id) returns()
func (_PremiumCards *PremiumCardsTransactor) MintToMany(opts *bind.TransactOpts, _to []common.Address, _id *big.Int) (*types.Transaction, error) {
	return _PremiumCards.contract.Transact(opts, "mintToMany", _to, _id)
}

// MintToMany is a paid mutator transaction binding the contract method 0x8a0b0360.
//
// Solidity: function mintToMany(address[] _to, uint256 _id) returns()
func (_PremiumCards *PremiumCardsSession) MintToMany(_to []common.Address, _id *big.Int) (*types.Transaction, error) {
	return _PremiumCards.Contract.MintToMany(&_PremiumCards.TransactOpts, _to, _id)
}

// MintToMany is a paid mutator transaction binding the contract method 0x8a0b0360.
//
// Solidity: function mintToMany(address[] _to, uint256 _id) returns()
func (_PremiumCards *PremiumCardsTransactorSession) MintToMany(_to []common.Address, _id *big.Int) (*types.Transaction, error) {
	return _PremiumCards.Contract.MintToMany(&_PremiumCards.TransactOpts, _to, _id)
}

