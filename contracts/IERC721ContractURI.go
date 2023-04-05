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

// IERC721ContractURIMetaData contains all meta data concerning the IERC721ContractURI contract.
var IERC721ContractURIMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"contractURI\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// IERC721ContractURIABI is the input ABI used to generate the binding from.
// Deprecated: Use IERC721ContractURIMetaData.ABI instead.
var IERC721ContractURIABI = IERC721ContractURIMetaData.ABI

// IERC721ContractURI is an auto generated Go binding around an Ethereum contract.
type IERC721ContractURI struct {
	IERC721ContractURICaller     // Read-only binding to the contract
	IERC721ContractURITransactor // Write-only binding to the contract
	IERC721ContractURIFilterer   // Log filterer for contract events
}

// IERC721ContractURICaller is an auto generated read-only Go binding around an Ethereum contract.
type IERC721ContractURICaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC721ContractURITransactor is an auto generated write-only Go binding around an Ethereum contract.
type IERC721ContractURITransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC721ContractURIFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IERC721ContractURIFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC721ContractURISession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IERC721ContractURISession struct {
	Contract     *IERC721ContractURI // Generic contract binding to set the session for
	CallOpts     bind.CallOpts       // Call options to use throughout this session
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// IERC721ContractURICallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IERC721ContractURICallerSession struct {
	Contract *IERC721ContractURICaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts             // Call options to use throughout this session
}

// IERC721ContractURITransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IERC721ContractURITransactorSession struct {
	Contract     *IERC721ContractURITransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts             // Transaction auth options to use throughout this session
}

// IERC721ContractURIRaw is an auto generated low-level Go binding around an Ethereum contract.
type IERC721ContractURIRaw struct {
	Contract *IERC721ContractURI // Generic contract binding to access the raw methods on
}

// IERC721ContractURICallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IERC721ContractURICallerRaw struct {
	Contract *IERC721ContractURICaller // Generic read-only contract binding to access the raw methods on
}

// IERC721ContractURITransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IERC721ContractURITransactorRaw struct {
	Contract *IERC721ContractURITransactor // Generic write-only contract binding to access the raw methods on
}

// NewIERC721ContractURI creates a new instance of IERC721ContractURI, bound to a specific deployed contract.
func NewIERC721ContractURI(address common.Address, backend bind.ContractBackend) (*IERC721ContractURI, error) {
	contract, err := bindIERC721ContractURI(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IERC721ContractURI{IERC721ContractURICaller: IERC721ContractURICaller{contract: contract}, IERC721ContractURITransactor: IERC721ContractURITransactor{contract: contract}, IERC721ContractURIFilterer: IERC721ContractURIFilterer{contract: contract}}, nil
}

// NewIERC721ContractURICaller creates a new read-only instance of IERC721ContractURI, bound to a specific deployed contract.
func NewIERC721ContractURICaller(address common.Address, caller bind.ContractCaller) (*IERC721ContractURICaller, error) {
	contract, err := bindIERC721ContractURI(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IERC721ContractURICaller{contract: contract}, nil
}

// NewIERC721ContractURITransactor creates a new write-only instance of IERC721ContractURI, bound to a specific deployed contract.
func NewIERC721ContractURITransactor(address common.Address, transactor bind.ContractTransactor) (*IERC721ContractURITransactor, error) {
	contract, err := bindIERC721ContractURI(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IERC721ContractURITransactor{contract: contract}, nil
}

// NewIERC721ContractURIFilterer creates a new log filterer instance of IERC721ContractURI, bound to a specific deployed contract.
func NewIERC721ContractURIFilterer(address common.Address, filterer bind.ContractFilterer) (*IERC721ContractURIFilterer, error) {
	contract, err := bindIERC721ContractURI(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IERC721ContractURIFilterer{contract: contract}, nil
}

// bindIERC721ContractURI binds a generic wrapper to an already deployed contract.
func bindIERC721ContractURI(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IERC721ContractURIMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IERC721ContractURI *IERC721ContractURIRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IERC721ContractURI.Contract.IERC721ContractURICaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IERC721ContractURI *IERC721ContractURIRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IERC721ContractURI.Contract.IERC721ContractURITransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IERC721ContractURI *IERC721ContractURIRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IERC721ContractURI.Contract.IERC721ContractURITransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IERC721ContractURI *IERC721ContractURICallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IERC721ContractURI.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IERC721ContractURI *IERC721ContractURITransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IERC721ContractURI.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IERC721ContractURI *IERC721ContractURITransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IERC721ContractURI.Contract.contract.Transact(opts, method, params...)
}

// ContractURI is a free data retrieval call binding the contract method 0xe8a3d485.
//
// Solidity: function contractURI() view returns(string)
func (_IERC721ContractURI *IERC721ContractURICaller) ContractURI(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _IERC721ContractURI.contract.Call(opts, &out, "contractURI")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// ContractURI is a free data retrieval call binding the contract method 0xe8a3d485.
//
// Solidity: function contractURI() view returns(string)
func (_IERC721ContractURI *IERC721ContractURISession) ContractURI() (string, error) {
	return _IERC721ContractURI.Contract.ContractURI(&_IERC721ContractURI.CallOpts)
}

// ContractURI is a free data retrieval call binding the contract method 0xe8a3d485.
//
// Solidity: function contractURI() view returns(string)
func (_IERC721ContractURI *IERC721ContractURICallerSession) ContractURI() (string, error) {
	return _IERC721ContractURI.Contract.ContractURI(&_IERC721ContractURI.CallOpts)
}

