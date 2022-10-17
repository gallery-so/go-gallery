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

// CryptopunksMetaData contains all meta data concerning the Cryptopunks contract.
var CryptopunksMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_punkIndex\",\"type\":\"uint256\"}],\"name\":\"punkIndexToAddress\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// CryptopunksABI is the input ABI used to generate the binding from.
// Deprecated: Use CryptopunksMetaData.ABI instead.
var CryptopunksABI = CryptopunksMetaData.ABI

// Cryptopunks is an auto generated Go binding around an Ethereum contract.
type Cryptopunks struct {
	CryptopunksCaller     // Read-only binding to the contract
	CryptopunksTransactor // Write-only binding to the contract
	CryptopunksFilterer   // Log filterer for contract events
}

// CryptopunksCaller is an auto generated read-only Go binding around an Ethereum contract.
type CryptopunksCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CryptopunksTransactor is an auto generated write-only Go binding around an Ethereum contract.
type CryptopunksTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CryptopunksFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type CryptopunksFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CryptopunksSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type CryptopunksSession struct {
	Contract     *Cryptopunks      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// CryptopunksCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type CryptopunksCallerSession struct {
	Contract *CryptopunksCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// CryptopunksTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type CryptopunksTransactorSession struct {
	Contract     *CryptopunksTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// CryptopunksRaw is an auto generated low-level Go binding around an Ethereum contract.
type CryptopunksRaw struct {
	Contract *Cryptopunks // Generic contract binding to access the raw methods on
}

// CryptopunksCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type CryptopunksCallerRaw struct {
	Contract *CryptopunksCaller // Generic read-only contract binding to access the raw methods on
}

// CryptopunksTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type CryptopunksTransactorRaw struct {
	Contract *CryptopunksTransactor // Generic write-only contract binding to access the raw methods on
}

// NewCryptopunks creates a new instance of Cryptopunks, bound to a specific deployed contract.
func NewCryptopunks(address common.Address, backend bind.ContractBackend) (*Cryptopunks, error) {
	contract, err := bindCryptopunks(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Cryptopunks{CryptopunksCaller: CryptopunksCaller{contract: contract}, CryptopunksTransactor: CryptopunksTransactor{contract: contract}, CryptopunksFilterer: CryptopunksFilterer{contract: contract}}, nil
}

// NewCryptopunksCaller creates a new read-only instance of Cryptopunks, bound to a specific deployed contract.
func NewCryptopunksCaller(address common.Address, caller bind.ContractCaller) (*CryptopunksCaller, error) {
	contract, err := bindCryptopunks(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &CryptopunksCaller{contract: contract}, nil
}

// NewCryptopunksTransactor creates a new write-only instance of Cryptopunks, bound to a specific deployed contract.
func NewCryptopunksTransactor(address common.Address, transactor bind.ContractTransactor) (*CryptopunksTransactor, error) {
	contract, err := bindCryptopunks(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &CryptopunksTransactor{contract: contract}, nil
}

// NewCryptopunksFilterer creates a new log filterer instance of Cryptopunks, bound to a specific deployed contract.
func NewCryptopunksFilterer(address common.Address, filterer bind.ContractFilterer) (*CryptopunksFilterer, error) {
	contract, err := bindCryptopunks(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &CryptopunksFilterer{contract: contract}, nil
}

// bindCryptopunks binds a generic wrapper to an already deployed contract.
func bindCryptopunks(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(CryptopunksABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Cryptopunks *CryptopunksRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Cryptopunks.Contract.CryptopunksCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Cryptopunks *CryptopunksRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Cryptopunks.Contract.CryptopunksTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Cryptopunks *CryptopunksRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Cryptopunks.Contract.CryptopunksTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Cryptopunks *CryptopunksCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Cryptopunks.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Cryptopunks *CryptopunksTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Cryptopunks.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Cryptopunks *CryptopunksTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Cryptopunks.Contract.contract.Transact(opts, method, params...)
}

// PunkIndexToAddress is a free data retrieval call binding the contract method 0x58178168.
//
// Solidity: function punkIndexToAddress(uint256 _punkIndex) view returns(address)
func (_Cryptopunks *CryptopunksCaller) PunkIndexToAddress(opts *bind.CallOpts, _punkIndex *big.Int) (common.Address, error) {
	var out []interface{}
	err := _Cryptopunks.contract.Call(opts, &out, "punkIndexToAddress", _punkIndex)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// PunkIndexToAddress is a free data retrieval call binding the contract method 0x58178168.
//
// Solidity: function punkIndexToAddress(uint256 _punkIndex) view returns(address)
func (_Cryptopunks *CryptopunksSession) PunkIndexToAddress(_punkIndex *big.Int) (common.Address, error) {
	return _Cryptopunks.Contract.PunkIndexToAddress(&_Cryptopunks.CallOpts, _punkIndex)
}

// PunkIndexToAddress is a free data retrieval call binding the contract method 0x58178168.
//
// Solidity: function punkIndexToAddress(uint256 _punkIndex) view returns(address)
func (_Cryptopunks *CryptopunksCallerSession) PunkIndexToAddress(_punkIndex *big.Int) (common.Address, error) {
	return _Cryptopunks.Contract.PunkIndexToAddress(&_Cryptopunks.CallOpts, _punkIndex)
}
