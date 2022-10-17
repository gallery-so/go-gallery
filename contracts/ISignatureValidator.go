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

// ISignatureValidatorMetaData contains all meta data concerning the ISignatureValidator contract.
var ISignatureValidatorMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"_data\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"_signature\",\"type\":\"bytes\"}],\"name\":\"isValidSignature\",\"outputs\":[{\"internalType\":\"bytes4\",\"name\":\"\",\"type\":\"bytes4\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// ISignatureValidatorABI is the input ABI used to generate the binding from.
// Deprecated: Use ISignatureValidatorMetaData.ABI instead.
var ISignatureValidatorABI = ISignatureValidatorMetaData.ABI

// ISignatureValidator is an auto generated Go binding around an Ethereum contract.
type ISignatureValidator struct {
	ISignatureValidatorCaller     // Read-only binding to the contract
	ISignatureValidatorTransactor // Write-only binding to the contract
	ISignatureValidatorFilterer   // Log filterer for contract events
}

// ISignatureValidatorCaller is an auto generated read-only Go binding around an Ethereum contract.
type ISignatureValidatorCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ISignatureValidatorTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ISignatureValidatorTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ISignatureValidatorFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ISignatureValidatorFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ISignatureValidatorSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ISignatureValidatorSession struct {
	Contract     *ISignatureValidator // Generic contract binding to set the session for
	CallOpts     bind.CallOpts        // Call options to use throughout this session
	TransactOpts bind.TransactOpts    // Transaction auth options to use throughout this session
}

// ISignatureValidatorCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ISignatureValidatorCallerSession struct {
	Contract *ISignatureValidatorCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts              // Call options to use throughout this session
}

// ISignatureValidatorTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ISignatureValidatorTransactorSession struct {
	Contract     *ISignatureValidatorTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts              // Transaction auth options to use throughout this session
}

// ISignatureValidatorRaw is an auto generated low-level Go binding around an Ethereum contract.
type ISignatureValidatorRaw struct {
	Contract *ISignatureValidator // Generic contract binding to access the raw methods on
}

// ISignatureValidatorCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ISignatureValidatorCallerRaw struct {
	Contract *ISignatureValidatorCaller // Generic read-only contract binding to access the raw methods on
}

// ISignatureValidatorTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ISignatureValidatorTransactorRaw struct {
	Contract *ISignatureValidatorTransactor // Generic write-only contract binding to access the raw methods on
}

// NewISignatureValidator creates a new instance of ISignatureValidator, bound to a specific deployed contract.
func NewISignatureValidator(address common.Address, backend bind.ContractBackend) (*ISignatureValidator, error) {
	contract, err := bindISignatureValidator(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ISignatureValidator{ISignatureValidatorCaller: ISignatureValidatorCaller{contract: contract}, ISignatureValidatorTransactor: ISignatureValidatorTransactor{contract: contract}, ISignatureValidatorFilterer: ISignatureValidatorFilterer{contract: contract}}, nil
}

// NewISignatureValidatorCaller creates a new read-only instance of ISignatureValidator, bound to a specific deployed contract.
func NewISignatureValidatorCaller(address common.Address, caller bind.ContractCaller) (*ISignatureValidatorCaller, error) {
	contract, err := bindISignatureValidator(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ISignatureValidatorCaller{contract: contract}, nil
}

// NewISignatureValidatorTransactor creates a new write-only instance of ISignatureValidator, bound to a specific deployed contract.
func NewISignatureValidatorTransactor(address common.Address, transactor bind.ContractTransactor) (*ISignatureValidatorTransactor, error) {
	contract, err := bindISignatureValidator(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ISignatureValidatorTransactor{contract: contract}, nil
}

// NewISignatureValidatorFilterer creates a new log filterer instance of ISignatureValidator, bound to a specific deployed contract.
func NewISignatureValidatorFilterer(address common.Address, filterer bind.ContractFilterer) (*ISignatureValidatorFilterer, error) {
	contract, err := bindISignatureValidator(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ISignatureValidatorFilterer{contract: contract}, nil
}

// bindISignatureValidator binds a generic wrapper to an already deployed contract.
func bindISignatureValidator(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(ISignatureValidatorABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ISignatureValidator *ISignatureValidatorRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ISignatureValidator.Contract.ISignatureValidatorCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ISignatureValidator *ISignatureValidatorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ISignatureValidator.Contract.ISignatureValidatorTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ISignatureValidator *ISignatureValidatorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ISignatureValidator.Contract.ISignatureValidatorTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ISignatureValidator *ISignatureValidatorCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ISignatureValidator.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ISignatureValidator *ISignatureValidatorTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ISignatureValidator.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ISignatureValidator *ISignatureValidatorTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ISignatureValidator.Contract.contract.Transact(opts, method, params...)
}

// IsValidSignature is a free data retrieval call binding the contract method 0x1626ba7e.
//
// Solidity: function isValidSignature(bytes32 _data, bytes _signature) view returns(bytes4)
func (_ISignatureValidator *ISignatureValidatorCaller) IsValidSignature(opts *bind.CallOpts, _data [32]byte, _signature []byte) ([4]byte, error) {
	var out []interface{}
	err := _ISignatureValidator.contract.Call(opts, &out, "isValidSignature", _data, _signature)

	if err != nil {
		return *new([4]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([4]byte)).(*[4]byte)

	return out0, err

}

// IsValidSignature is a free data retrieval call binding the contract method 0x1626ba7e.
//
// Solidity: function isValidSignature(bytes32 _data, bytes _signature) view returns(bytes4)
func (_ISignatureValidator *ISignatureValidatorSession) IsValidSignature(_data [32]byte, _signature []byte) ([4]byte, error) {
	return _ISignatureValidator.Contract.IsValidSignature(&_ISignatureValidator.CallOpts, _data, _signature)
}

// IsValidSignature is a free data retrieval call binding the contract method 0x1626ba7e.
//
// Solidity: function isValidSignature(bytes32 _data, bytes _signature) view returns(bytes4)
func (_ISignatureValidator *ISignatureValidatorCallerSession) IsValidSignature(_data [32]byte, _signature []byte) ([4]byte, error) {
	return _ISignatureValidator.Contract.IsValidSignature(&_ISignatureValidator.CallOpts, _data, _signature)
}
