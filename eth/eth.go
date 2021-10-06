package eth

import (
	"context"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/spf13/viper"
)

var contractsClient *ethclient.Client
var contractAddress common.Address

func init() {
	viper.SetDefault("CONTRACT_ADDRESS", "0x970b6AFD5EcDCB4001dB8dBf5E2702e86c857E54")
	viper.SetDefault("CONTRACT_INTERACTION_URL", "https://eth-kovan.alchemyapi.io/v2/lZc9uHY6g2ak1jnEkrOkkopylNJXvE76")
	viper.AutomaticEnv()
	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}
	contractsClient = client
	contractAddress = common.HexToAddress(viper.GetString("CONTRACT_ADDRESS"))
}

const ensContractAddress = "0xFaC7BEA255a6990f749363002136aF6556b31e04"

// HasNFT checks if a wallet address has a given NFT
func HasNFT(pCtx context.Context, id string, userAddr string) (bool, error) {

	addr := common.HexToAddress(userAddr)

	instance, err := contracts.NewIERC1155Caller(contractAddress, contractsClient)
	if err != nil {
		return false, err
	}

	bigIntID := &big.Int{}
	bigIntID, _ = bigIntID.SetString(id, 10)

	call, err := instance.BalanceOf(&bind.CallOpts{From: addr, Context: pCtx}, addr, bigIntID)
	if err != nil {
		return false, err
	}

	return call.Cmp(big.NewInt(0)) > 0, nil

}

// HasNFTs checks if a wallet address has a given set of NFTs
func HasNFTs(pCtx context.Context, ids []string, userAddr string) (bool, error) {

	addr := common.HexToAddress(userAddr)

	instance, err := contracts.NewIERC1155Caller(contractAddress, contractsClient)
	if err != nil {
		return false, err
	}

	bigIntIDs := make([]*big.Int, len(ids))
	addrs := make([]common.Address, len(ids))
	for i := 0; i < len(ids); i++ {
		asBigInt := &big.Int{}
		bigIntIDs[i], _ = asBigInt.SetString(ids[i], 10)
		addrs[i] = addr
	}

	call, err := instance.BalanceOfBatch(&bind.CallOpts{From: addr, Context: pCtx}, addrs, bigIntIDs)
	if err != nil {
		return false, err
	}
	for _, v := range call {
		if v.Cmp(big.NewInt(0)) > 0 {
			return true, nil
		}
	}

	return false, nil

}

// ResolvesENS checks if an ENS resolves to a given address
func ResolvesENS(pCtx context.Context, ens string, userAddr string) (bool, error) {

	addr := common.HexToAddress(userAddr)

	instance, err := contracts.NewIENSCaller(contractAddress, contractsClient)
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
