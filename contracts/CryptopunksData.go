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

// CryptopunksDataMetaData contains all meta data concerning the CryptopunksData contract.
var CryptopunksDataMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"uint16\",\"name\":\"index\",\"type\":\"uint16\"}],\"name\":\"punkAttributes\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"text\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint16\",\"name\":\"index\",\"type\":\"uint16\"}],\"name\":\"punkImage\",\"outputs\":[{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint16\",\"name\":\"index\",\"type\":\"uint16\"}],\"name\":\"punkImageSvg\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"svg\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// CryptopunksDataABI is the input ABI used to generate the binding from.
// Deprecated: Use CryptopunksDataMetaData.ABI instead.
var CryptopunksDataABI = CryptopunksDataMetaData.ABI

// CryptopunksData is an auto generated Go binding around an Ethereum contract.
type CryptopunksData struct {
	CryptopunksDataCaller     // Read-only binding to the contract
	CryptopunksDataTransactor // Write-only binding to the contract
	CryptopunksDataFilterer   // Log filterer for contract events
}

// CryptopunksDataCaller is an auto generated read-only Go binding around an Ethereum contract.
type CryptopunksDataCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CryptopunksDataTransactor is an auto generated write-only Go binding around an Ethereum contract.
type CryptopunksDataTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CryptopunksDataFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type CryptopunksDataFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CryptopunksDataSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type CryptopunksDataSession struct {
	Contract     *CryptopunksData  // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// CryptopunksDataCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type CryptopunksDataCallerSession struct {
	Contract *CryptopunksDataCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts          // Call options to use throughout this session
}

// CryptopunksDataTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type CryptopunksDataTransactorSession struct {
	Contract     *CryptopunksDataTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts          // Transaction auth options to use throughout this session
}

// CryptopunksDataRaw is an auto generated low-level Go binding around an Ethereum contract.
type CryptopunksDataRaw struct {
	Contract *CryptopunksData // Generic contract binding to access the raw methods on
}

// CryptopunksDataCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type CryptopunksDataCallerRaw struct {
	Contract *CryptopunksDataCaller // Generic read-only contract binding to access the raw methods on
}

// CryptopunksDataTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type CryptopunksDataTransactorRaw struct {
	Contract *CryptopunksDataTransactor // Generic write-only contract binding to access the raw methods on
}

// NewCryptopunksData creates a new instance of CryptopunksData, bound to a specific deployed contract.
func NewCryptopunksData(address common.Address, backend bind.ContractBackend) (*CryptopunksData, error) {
	contract, err := bindCryptopunksData(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &CryptopunksData{CryptopunksDataCaller: CryptopunksDataCaller{contract: contract}, CryptopunksDataTransactor: CryptopunksDataTransactor{contract: contract}, CryptopunksDataFilterer: CryptopunksDataFilterer{contract: contract}}, nil
}

// NewCryptopunksDataCaller creates a new read-only instance of CryptopunksData, bound to a specific deployed contract.
func NewCryptopunksDataCaller(address common.Address, caller bind.ContractCaller) (*CryptopunksDataCaller, error) {
	contract, err := bindCryptopunksData(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &CryptopunksDataCaller{contract: contract}, nil
}

// NewCryptopunksDataTransactor creates a new write-only instance of CryptopunksData, bound to a specific deployed contract.
func NewCryptopunksDataTransactor(address common.Address, transactor bind.ContractTransactor) (*CryptopunksDataTransactor, error) {
	contract, err := bindCryptopunksData(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &CryptopunksDataTransactor{contract: contract}, nil
}

// NewCryptopunksDataFilterer creates a new log filterer instance of CryptopunksData, bound to a specific deployed contract.
func NewCryptopunksDataFilterer(address common.Address, filterer bind.ContractFilterer) (*CryptopunksDataFilterer, error) {
	contract, err := bindCryptopunksData(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &CryptopunksDataFilterer{contract: contract}, nil
}

// bindCryptopunksData binds a generic wrapper to an already deployed contract.
func bindCryptopunksData(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(CryptopunksDataABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CryptopunksData *CryptopunksDataRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CryptopunksData.Contract.CryptopunksDataCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CryptopunksData *CryptopunksDataRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CryptopunksData.Contract.CryptopunksDataTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CryptopunksData *CryptopunksDataRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CryptopunksData.Contract.CryptopunksDataTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CryptopunksData *CryptopunksDataCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CryptopunksData.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CryptopunksData *CryptopunksDataTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CryptopunksData.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CryptopunksData *CryptopunksDataTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CryptopunksData.Contract.contract.Transact(opts, method, params...)
}

// PunkAttributes is a free data retrieval call binding the contract method 0x76dfe297.
//
// Solidity: function punkAttributes(uint16 index) view returns(string text)
func (_CryptopunksData *CryptopunksDataCaller) PunkAttributes(opts *bind.CallOpts, index uint16) (string, error) {
	var out []interface{}
	err := _CryptopunksData.contract.Call(opts, &out, "punkAttributes", index)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// PunkAttributes is a free data retrieval call binding the contract method 0x76dfe297.
//
// Solidity: function punkAttributes(uint16 index) view returns(string text)
func (_CryptopunksData *CryptopunksDataSession) PunkAttributes(index uint16) (string, error) {
	return _CryptopunksData.Contract.PunkAttributes(&_CryptopunksData.CallOpts, index)
}

// PunkAttributes is a free data retrieval call binding the contract method 0x76dfe297.
//
// Solidity: function punkAttributes(uint16 index) view returns(string text)
func (_CryptopunksData *CryptopunksDataCallerSession) PunkAttributes(index uint16) (string, error) {
	return _CryptopunksData.Contract.PunkAttributes(&_CryptopunksData.CallOpts, index)
}

// PunkImage is a free data retrieval call binding the contract method 0x3e5e0a96.
//
// Solidity: function punkImage(uint16 index) view returns(bytes)
func (_CryptopunksData *CryptopunksDataCaller) PunkImage(opts *bind.CallOpts, index uint16) ([]byte, error) {
	var out []interface{}
	err := _CryptopunksData.contract.Call(opts, &out, "punkImage", index)

	if err != nil {
		return *new([]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([]byte)).(*[]byte)

	return out0, err

}

// PunkImage is a free data retrieval call binding the contract method 0x3e5e0a96.
//
// Solidity: function punkImage(uint16 index) view returns(bytes)
func (_CryptopunksData *CryptopunksDataSession) PunkImage(index uint16) ([]byte, error) {
	return _CryptopunksData.Contract.PunkImage(&_CryptopunksData.CallOpts, index)
}

// PunkImage is a free data retrieval call binding the contract method 0x3e5e0a96.
//
// Solidity: function punkImage(uint16 index) view returns(bytes)
func (_CryptopunksData *CryptopunksDataCallerSession) PunkImage(index uint16) ([]byte, error) {
	return _CryptopunksData.Contract.PunkImage(&_CryptopunksData.CallOpts, index)
}

// PunkImageSvg is a free data retrieval call binding the contract method 0x74beb047.
//
// Solidity: function punkImageSvg(uint16 index) view returns(string svg)
func (_CryptopunksData *CryptopunksDataCaller) PunkImageSvg(opts *bind.CallOpts, index uint16) (string, error) {
	var out []interface{}
	err := _CryptopunksData.contract.Call(opts, &out, "punkImageSvg", index)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// PunkImageSvg is a free data retrieval call binding the contract method 0x74beb047.
//
// Solidity: function punkImageSvg(uint16 index) view returns(string svg)
func (_CryptopunksData *CryptopunksDataSession) PunkImageSvg(index uint16) (string, error) {
	return _CryptopunksData.Contract.PunkImageSvg(&_CryptopunksData.CallOpts, index)
}

// PunkImageSvg is a free data retrieval call binding the contract method 0x74beb047.
//
// Solidity: function punkImageSvg(uint16 index) view returns(string svg)
func (_CryptopunksData *CryptopunksDataCallerSession) PunkImageSvg(index uint16) (string, error) {
	return _CryptopunksData.Contract.PunkImageSvg(&_CryptopunksData.CallOpts, index)
}
