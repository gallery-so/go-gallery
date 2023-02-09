package publicapi

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
)

type CardAPI struct {
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
	secrets            *secretmanager.Client
}

func (api *CardAPI) MintPremiumCardToWallet(ctx context.Context, input model.MintCardToWalletInput) (string, error) {

	cardAddress := viper.GetString("CARDS_CONTRACT_ADDRESS")

	cards, err := contracts.NewPremiumCards(common.HexToAddress(cardAddress), api.ethClient)
	if err != nil {
		return "", fmt.Errorf("failed to instantiate a Cards contract: %w", err)
	}

	chainID, err := api.ethClient.ChainID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get chain ID: %w", err)
	}

	if chainID.Cmp(big.NewInt(1)) == 0 && viper.GetString("ENV") == "production" {
		privateKey := viper.GetString("ETH_PRIVATE_KEY")

		key, err := crypto.HexToECDSA(privateKey)
		if err != nil {
			return "", err
		}

		auth, err := bind.NewKeyedTransactorWithChainID(key, chainID)
		if err != nil {
			return "", fmt.Errorf("failed to create authorized transactor: %w", err)
		}
		auth.Context = ctx

		addresses, err := util.Map(input.WalletAddresses, func(addr persist.Address) (common.Address, error) {
			return common.HexToAddress(addr.String()), nil
		})
		tid, ok := big.NewInt(0).SetString(input.TokenID, 16)
		if !ok {
			return "", fmt.Errorf("failed to parse token id: %w", err)
		}
		tx, err := cards.MintToMany(auth, addresses, tid)
		if err != nil {
			return "", fmt.Errorf("failed to mint to wallet: %w", err)
		}

		return tx.Hash().String(), nil
	}

	return "", errors.New("failed to mint to wallet, env is not production or chain id is not 1")
}
