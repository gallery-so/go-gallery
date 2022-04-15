package eth

import "github.com/mikeydub/go-gallery/service/persist"

func ToWalletAddresses(addresses []persist.EthereumAddress) []persist.Wallet {
	res := make([]persist.Wallet, len(addresses))
	for i, v := range addresses {
		res[i] = ToWalletAddress(v)
	}
	return res
}

func ToWalletAddress(address persist.EthereumAddress) persist.Wallet {
	return persist.Wallet{
		Address: persist.Address(address.String()),
		Chain:   persist.ChainETH,
	}

}
