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

// IERC1155ContractURIMetaData contains all meta data concerning the IERC1155ContractURI contract.
var IERC1155ContractURIMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"contractURI\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// IERC1155ContractURIABI is the input ABI used to generate the binding from.
// Deprecated: Use IERC1155ContractURIMetaData.ABI instead.
var IERC1155ContractURIABI = IERC1155ContractURIMetaData.ABI

// IERC1155ContractURI is an auto generated Go binding around an Ethereum contract.
type IERC1155ContractURI struct {
	IERC1155ContractURICaller     // Read-only binding to the contract
	IERC1155ContractURITransactor // Write-only binding to the contract
	IERC1155ContractURIFilterer   // Log filterer for contract events
}

// IERC1155ContractURICaller is an auto generated read-only Go binding around an Ethereum contract.
type IERC1155ContractURICaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC1155ContractURITransactor is an auto generated write-only Go binding around an Ethereum contract.
type IERC1155ContractURITransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC1155ContractURIFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IERC1155ContractURIFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC1155ContractURISession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IERC1155ContractURISession struct {
	Contract     *IERC1155ContractURI // Generic contract binding to set the session for
	CallOpts     bind.CallOpts        // Call options to use throughout this session
	TransactOpts bind.TransactOpts    // Transaction auth options to use throughout this session
}

// IERC1155ContractURICallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IERC1155ContractURICallerSession struct {
	Contract *IERC1155ContractURICaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts              // Call options to use throughout this session
}

// IERC1155ContractURITransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IERC1155ContractURITransactorSession struct {
	Contract     *IERC1155ContractURITransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts              // Transaction auth options to use throughout this session
}

// IERC1155ContractURIRaw is an auto generated low-level Go binding around an Ethereum contract.
type IERC1155ContractURIRaw struct {
	Contract *IERC1155ContractURI // Generic contract binding to access the raw methods on
}

// IERC1155ContractURICallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IERC1155ContractURICallerRaw struct {
	Contract *IERC1155ContractURICaller // Generic read-only contract binding to access the raw methods on
}

// IERC1155ContractURITransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IERC1155ContractURITransactorRaw struct {
	Contract *IERC1155ContractURITransactor // Generic write-only contract binding to access the raw methods on
}

// NewIERC1155ContractURI creates a new instance of IERC1155ContractURI, bound to a specific deployed contract.
func NewIERC1155ContractURI(address common.Address, backend bind.ContractBackend) (*IERC1155ContractURI, error) {
	contract, err := bindIERC1155ContractURI(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IERC1155ContractURI{IERC1155ContractURICaller: IERC1155ContractURICaller{contract: contract}, IERC1155ContractURITransactor: IERC1155ContractURITransactor{contract: contract}, IERC1155ContractURIFilterer: IERC1155ContractURIFilterer{contract: contract}}, nil
}

// NewIERC1155ContractURICaller creates a new read-only instance of IERC1155ContractURI, bound to a specific deployed contract.
func NewIERC1155ContractURICaller(address common.Address, caller bind.ContractCaller) (*IERC1155ContractURICaller, error) {
	contract, err := bindIERC1155ContractURI(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IERC1155ContractURICaller{contract: contract}, nil
}

// NewIERC1155ContractURITransactor creates a new write-only instance of IERC1155ContractURI, bound to a specific deployed contract.
func NewIERC1155ContractURITransactor(address common.Address, transactor bind.ContractTransactor) (*IERC1155ContractURITransactor, error) {
	contract, err := bindIERC1155ContractURI(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IERC1155ContractURITransactor{contract: contract}, nil
}

// NewIERC1155ContractURIFilterer creates a new log filterer instance of IERC1155ContractURI, bound to a specific deployed contract.
func NewIERC1155ContractURIFilterer(address common.Address, filterer bind.ContractFilterer) (*IERC1155ContractURIFilterer, error) {
	contract, err := bindIERC1155ContractURI(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IERC1155ContractURIFilterer{contract: contract}, nil
}

// bindIERC1155ContractURI binds a generic wrapper to an already deployed contract.
func bindIERC1155ContractURI(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IERC1155ContractURIMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IERC1155ContractURI *IERC1155ContractURIRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IERC1155ContractURI.Contract.IERC1155ContractURICaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IERC1155ContractURI *IERC1155ContractURIRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IERC1155ContractURI.Contract.IERC1155ContractURITransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IERC1155ContractURI *IERC1155ContractURIRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IERC1155ContractURI.Contract.IERC1155ContractURITransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IERC1155ContractURI *IERC1155ContractURICallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IERC1155ContractURI.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IERC1155ContractURI *IERC1155ContractURITransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IERC1155ContractURI.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IERC1155ContractURI *IERC1155ContractURITransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IERC1155ContractURI.Contract.contract.Transact(opts, method, params...)
}

// ContractURI is a free data retrieval call binding the contract method 0xe8a3d485.
//
// Solidity: function contractURI() view returns(string)
func (_IERC1155ContractURI *IERC1155ContractURICaller) ContractURI(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _IERC1155ContractURI.contract.Call(opts, &out, "contractURI")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// ContractURI is a free data retrieval call binding the contract method 0xe8a3d485.
//
// Solidity: function contractURI() view returns(string)
func (_IERC1155ContractURI *IERC1155ContractURISession) ContractURI() (string, error) {
	return _IERC1155ContractURI.Contract.ContractURI(&_IERC1155ContractURI.CallOpts)
}

// ContractURI is a free data retrieval call binding the contract method 0xe8a3d485.
//
// Solidity: function contractURI() view returns(string)
func (_IERC1155ContractURI *IERC1155ContractURICallerSession) ContractURI() (string, error) {
	return _IERC1155ContractURI.Contract.ContractURI(&_IERC1155ContractURI.CallOpts)
}

