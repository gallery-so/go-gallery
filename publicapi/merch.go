package publicapi

import (
	"context"
	"crypto/x509"
	"fmt"
	"math/big"

	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/spf13/viper"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
)

type MerchAPI struct {
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	storageClient      *storage.Client
	multichainProvider *multichain.Provider
	secrets            *secretmanager.Client
}

func (api MerchAPI) RedeemMerchItems(ctx context.Context, tokenIDs []persist.TokenID, address persist.ChainPubKey, sig string, walletType persist.WalletType) ([]*model.DiscountCode, error) {

	if err := validateFields(api.validator, validationMap{
		"tokenIDs": {tokenIDs, "required"},
		"address":  {address, "required"},
		"sig":      {sig, "required"},
	}); err != nil {
		return nil, err
	}

	// check if user owns tokens

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, err
	}

	merchAddress := viper.GetString("MERCH_CONTRACT_ADDRESS")

	contract, err := api.queries.GetContractByChainAddress(ctx, db.GetContractByChainAddressParams{
		Address: persist.Address(merchAddress),
		Chain:   persist.ChainETH,
	})
	if err != nil {
		return nil, err
	}

	for _, tokenID := range tokenIDs {
		owns, err := api.queries.GetUserOwnsTokenByIdentifiers(ctx, db.GetUserOwnsTokenByIdentifiersParams{
			UserID:   userID,
			TokenHex: tokenID,
			Contract: contract.ID,
		})
		if err != nil {
			return nil, err
		}
		if !owns {
			return nil, fmt.Errorf("user does not own token %v", tokenID)
		}
	}

	// verify signature

	// user should have signed the tokenIDs in place of the usual nonce
	valid, err := api.multichainProvider.VerifySignature(ctx, sig, fmt.Sprintf("%v", tokenIDs), address, walletType)
	if err != nil {
		return nil, err
	}

	if !valid {
		return nil, auth.ErrSignatureInvalid
	}

	// redeem and return codes in DB

	discountCodes, err := api.queries.RedeemMerchMany(ctx, tokenIDs)
	if err != nil {
		return nil, err
	}

	result := make([]*model.DiscountCode, 0, len(discountCodes))
	for _, discountCode := range discountCodes {
		if discountCode.DiscountCode.Valid {
			result = append(result, &model.DiscountCode{Code: discountCode.DiscountCode.String, TokenID: discountCode.TokenID.String()})
		}
	}

	// redeem tokens on chain

	mer, err := contracts.NewMerch(common.HexToAddress(merchAddress), api.ethClient)

	privateKey, err := api.secrets.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: "backend-eth-private-key",
	})

	key, err := x509.ParseECPrivateKey(privateKey.Payload.Data)
	if err != nil {
		return nil, err
	}

	chainID, err := api.ethClient.ChainID(ctx)
	if err != nil {
		return nil, err
	}

	keytransactor, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return nil, err
	}

	asBigs := make([]*big.Int, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		asBigs = append(asBigs, tokenID.BigInt())
	}

	tx, err := mer.Redeem(keytransactor, asBigs)
	if err != nil {
		return nil, err
	}

	err = api.ethClient.SendTransaction(ctx, tx)
	if err != nil {
		logger.For(ctx).Errorf("failed to send transaction: %v", err)
	}

	return result, nil
}
