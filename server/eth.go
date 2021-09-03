package server

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/contracts"
)

func hasAnyNFT(contractAddress string, userAddr string) (bool, error) {
	// TODO use alchemy URL
	client, err := ethclient.Dial("https://rinkeby.infura.io")
	if err != nil {
		return false, err
	}

	addr := common.HexToAddress(userAddr)

	contract := common.HexToAddress(contractAddress)
	instance, err := contracts.NewContracts(contract, client)
	if err != nil {
		return false, err
	}

	call, err := instance.BalanceOf(&bind.CallOpts{From: addr}, addr)
	if err != nil {
		return false, err
	}

	switch {
	case call.IsInt64():
		return call.Int64() > 0, nil
	case call.IsUint64():
		return call.Uint64() > 0, nil
	default:
		return false, errors.New("could not get balanceOf address for contract")
	}

}
func hasNFT(contractAddress string, id string, userAddr string) (bool, error) {
	// TODO use alchemy URL
	client, err := ethclient.Dial("https://rinkeby.infura.io")
	if err != nil {
		return false, err
	}

	addr := common.HexToAddress(userAddr)

	contract := common.HexToAddress(contractAddress)
	instance, err := contracts.NewContracts(contract, client)
	if err != nil {
		return false, err
	}

	bigIntID := &big.Int{}
	bigIntID, _ = bigIntID.SetString(id, 10)

	call, err := instance.OwnerOf(&bind.CallOpts{From: addr}, bigIntID)
	if err != nil {
		return false, err
	}

	return call.String() == addr.String(), nil

}
