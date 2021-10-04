package server

import (
	"context"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/runtime"
)

const ensContractAddress = "0xFaC7BEA255a6990f749363002136aF6556b31e04"

func hasAnyNFT(pCtx context.Context, contractAddress string, userAddr string, pRuntime *runtime.Runtime) (bool, error) {
	client, err := ethclient.Dial("wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	if err != nil {
		return false, err
	}

	addr := common.HexToAddress(userAddr)

	contract := common.HexToAddress(contractAddress)
	instance, err := contracts.NewIERC721Caller(contract, client)
	if err != nil {
		return false, err
	}

	call, err := instance.BalanceOf(&bind.CallOpts{From: addr, Context: pCtx}, addr)
	if err != nil {
		return false, err
	}

	return call.Cmp(new(big.Int).SetUint64(0)) == 1, nil
}
func hasNFT(pCtx context.Context, contractAddress string, id string, userAddr string, pRuntime *runtime.Runtime) (bool, error) {
	client, err := ethclient.Dial("wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	if err != nil {
		return false, err
	}

	addr := common.HexToAddress(userAddr)

	contract := common.HexToAddress(contractAddress)
	instance, err := contracts.NewIERC721Caller(contract, client)
	if err != nil {
		return false, err
	}

	bigIntID := &big.Int{}
	bigIntID, _ = bigIntID.SetString(id, 10)

	call, err := instance.OwnerOf(&bind.CallOpts{From: addr, Context: pCtx}, bigIntID)
	if err != nil {
		return false, err
	}

	return call.String() == addr.String(), nil

}

func resolvesENS(pCtx context.Context, ens string, userAddr string, pRuntime *runtime.Runtime) (bool, error) {
	client, err := ethclient.Dial("wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	if err != nil {
		return false, err
	}

	addr := common.HexToAddress(userAddr)

	contract := common.HexToAddress(ensContractAddress)
	instance, err := contracts.NewIENSCaller(contract, client)
	if err != nil {
		return false, err
	}

	nh := namehash(ens)
	asBytes32 := [32]byte{}
	for i := 0; i < len(nh); i++ {
		asBytes32[i] = nh[i]
	}

	call, err := instance.Resolver(&bind.CallOpts{From: addr, Context: pCtx}, asBytes32)
	if err != nil {
		return false, err
	}

	return call.String() == addr.String(), nil

}

// function that computes the namehash for a given ENS domain
func namehash(name string) common.Hash {
	node := common.Hash{}

	if len(name) > 0 {
		labels := strings.Split(name, ".")

		for i := len(labels) - 1; i >= 0; i-- {
			labelSha := crypto.Keccak256Hash([]byte(labels[i]))
			node = crypto.Keccak256Hash(node.Bytes(), labelSha.Bytes())
		}
	}

	return node
}
