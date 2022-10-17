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

// ZoraMetaData contains all meta data concerning the Zora contract.
var ZoraMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"tokenId\",\"type\":\"uint256\"}],\"name\":\"tokenMetadataURI\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"tokenId\",\"type\":\"uint256\"}],\"name\":\"tokenURI\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// ZoraABI is the input ABI used to generate the binding from.
// Deprecated: Use ZoraMetaData.ABI instead.
var ZoraABI = ZoraMetaData.ABI

// Zora is an auto generated Go binding around an Ethereum contract.
type Zora struct {
	ZoraCaller     // Read-only binding to the contract
	ZoraTransactor // Write-only binding to the contract
	ZoraFilterer   // Log filterer for contract events
}

// ZoraCaller is an auto generated read-only Go binding around an Ethereum contract.
type ZoraCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ZoraTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ZoraTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ZoraFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ZoraFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ZoraSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ZoraSession struct {
	Contract     *Zora             // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ZoraCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ZoraCallerSession struct {
	Contract *ZoraCaller   // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// ZoraTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ZoraTransactorSession struct {
	Contract     *ZoraTransactor   // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ZoraRaw is an auto generated low-level Go binding around an Ethereum contract.
type ZoraRaw struct {
	Contract *Zora // Generic contract binding to access the raw methods on
}

// ZoraCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ZoraCallerRaw struct {
	Contract *ZoraCaller // Generic read-only contract binding to access the raw methods on
}

// ZoraTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ZoraTransactorRaw struct {
	Contract *ZoraTransactor // Generic write-only contract binding to access the raw methods on
}

// NewZora creates a new instance of Zora, bound to a specific deployed contract.
func NewZora(address common.Address, backend bind.ContractBackend) (*Zora, error) {
	contract, err := bindZora(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Zora{ZoraCaller: ZoraCaller{contract: contract}, ZoraTransactor: ZoraTransactor{contract: contract}, ZoraFilterer: ZoraFilterer{contract: contract}}, nil
}

// NewZoraCaller creates a new read-only instance of Zora, bound to a specific deployed contract.
func NewZoraCaller(address common.Address, caller bind.ContractCaller) (*ZoraCaller, error) {
	contract, err := bindZora(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ZoraCaller{contract: contract}, nil
}

// NewZoraTransactor creates a new write-only instance of Zora, bound to a specific deployed contract.
func NewZoraTransactor(address common.Address, transactor bind.ContractTransactor) (*ZoraTransactor, error) {
	contract, err := bindZora(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ZoraTransactor{contract: contract}, nil
}

// NewZoraFilterer creates a new log filterer instance of Zora, bound to a specific deployed contract.
func NewZoraFilterer(address common.Address, filterer bind.ContractFilterer) (*ZoraFilterer, error) {
	contract, err := bindZora(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ZoraFilterer{contract: contract}, nil
}

// bindZora binds a generic wrapper to an already deployed contract.
func bindZora(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(ZoraABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Zora *ZoraRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Zora.Contract.ZoraCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Zora *ZoraRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Zora.Contract.ZoraTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Zora *ZoraRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Zora.Contract.ZoraTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Zora *ZoraCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Zora.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Zora *ZoraTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Zora.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Zora *ZoraTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Zora.Contract.contract.Transact(opts, method, params...)
}

// TokenMetadataURI is a free data retrieval call binding the contract method 0x157c3df9.
//
// Solidity: function tokenMetadataURI(uint256 tokenId) view returns(string)
func (_Zora *ZoraCaller) TokenMetadataURI(opts *bind.CallOpts, tokenId *big.Int) (string, error) {
	var out []interface{}
	err := _Zora.contract.Call(opts, &out, "tokenMetadataURI", tokenId)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// TokenMetadataURI is a free data retrieval call binding the contract method 0x157c3df9.
//
// Solidity: function tokenMetadataURI(uint256 tokenId) view returns(string)
func (_Zora *ZoraSession) TokenMetadataURI(tokenId *big.Int) (string, error) {
	return _Zora.Contract.TokenMetadataURI(&_Zora.CallOpts, tokenId)
}

// TokenMetadataURI is a free data retrieval call binding the contract method 0x157c3df9.
//
// Solidity: function tokenMetadataURI(uint256 tokenId) view returns(string)
func (_Zora *ZoraCallerSession) TokenMetadataURI(tokenId *big.Int) (string, error) {
	return _Zora.Contract.TokenMetadataURI(&_Zora.CallOpts, tokenId)
}

// TokenURI is a free data retrieval call binding the contract method 0xc87b56dd.
//
// Solidity: function tokenURI(uint256 tokenId) view returns(string)
func (_Zora *ZoraCaller) TokenURI(opts *bind.CallOpts, tokenId *big.Int) (string, error) {
	var out []interface{}
	err := _Zora.contract.Call(opts, &out, "tokenURI", tokenId)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// TokenURI is a free data retrieval call binding the contract method 0xc87b56dd.
//
// Solidity: function tokenURI(uint256 tokenId) view returns(string)
func (_Zora *ZoraSession) TokenURI(tokenId *big.Int) (string, error) {
	return _Zora.Contract.TokenURI(&_Zora.CallOpts, tokenId)
}

// TokenURI is a free data retrieval call binding the contract method 0xc87b56dd.
//
// Solidity: function tokenURI(uint256 tokenId) view returns(string)
func (_Zora *ZoraCallerSession) TokenURI(tokenId *big.Int) (string, error) {
	return _Zora.Contract.TokenURI(&_Zora.CallOpts, tokenId)
}
