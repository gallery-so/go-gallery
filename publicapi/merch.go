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

const (
	merchTypeTShirt = iota
	merchTypeHat
	merchTypeCard
)

var uriToMerchType = map[string]int{
	"ipfs://QmSWiQSXkxXhaoMJ2m9goR9DVXnyijdozE57jEsAwqLNZY": merchTypeTShirt,
	"ipfs://QmVXF8H7Xcnqr4oQXGEtoMCMnah8d6fBZuQQ5tcv9nL8Po": merchTypeHat,
	"ipfs://QmSPdA9Gg8xAdVxWvUyGkdFKQ8YMVYnGjYcr3cGMcBH1ae": merchTypeCard,
}

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

	// check for duplicate tokenIDs
	seen := map[persist.TokenID]bool{}
	for _, tokenID := range tokenIDs {
		if !seen[tokenID] {
			seen[tokenID] = true
		} else {
			return nil, fmt.Errorf("duplicate token ID %v", tokenID)
		}
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
		return nil, fmt.Errorf("failed to get contract by address %s: %w", merchAddress, err)
	}

	for _, tokenID := range tokenIDs {
		owns, err := api.queries.GetUserOwnsTokenByIdentifiers(ctx, db.GetUserOwnsTokenByIdentifiersParams{
			UserID:   userID,
			TokenHex: tokenID,
			Contract: contract.ID,
			Chain:    persist.ChainETH,
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

	mer, err := contracts.NewMerch(common.HexToAddress(merchAddress), api.ethClient)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate a Merch contract: %w", err)
	}

	// redeem and return codes in DB

	redeemed := map[persist.TokenID]bool{}

	result := make([]*model.DiscountCode, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {

		t, err := mer.IsRedeemed(&bind.CallOpts{Context: ctx}, tokenID.BigInt())
		if err != nil {
			return nil, fmt.Errorf("failed to check if token %v is redeemed: %w", tokenID, err)
		}
		redeemed[tokenID] = t

		uri, err := mer.TokenURI(&bind.CallOpts{Context: ctx}, tokenID.BigInt())
		if err != nil {
			return nil, fmt.Errorf("failed to get token URI for token %v: %w", tokenID, err)
		}

		objectType, ok := uriToMerchType[uri]
		if ok {

			// first check if the token ID has already been redeemed, then just return the code
			discountCode, err := api.queries.GetMerchDiscountCodeByTokenID(ctx, tokenID)
			if err != nil {
				logger.For(ctx).Debugf("failed to get discount code for token %v: %v", tokenID, err)
				// if not, redeem it
				discountCode, err = api.queries.RedeemMerch(ctx, db.RedeemMerchParams{
					TokenHex:   tokenID,
					ObjectType: int32(objectType),
				})
				if err != nil {
					return nil, fmt.Errorf("failed to redeem token %v: %w", tokenID, err)
				}
			} else {
				// override what the blockchain says about redeemed status, if the DB says it is redeemed then there is probably
				// a transaction in progress for redeeming it, so don't kick off another redeem transaction for this token
				redeemed[tokenID] = true
			}

			if discountCode.Valid {
				result = append(result, &model.DiscountCode{Code: discountCode.String, TokenID: tokenID.String()})
			}
		} else {
			logger.For(ctx).Errorf("unknown merch type for %v", uri)
		}

	}

	// redeem tokens on chain

	chainID, err := api.ethClient.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID: %w", err)
	}

	if chainID.Cmp(big.NewInt(1)) == 0 && viper.GetString("ENV") != "production" {
		// if we're on mainnet but not on production, don't actually redeem
		return result, nil
	}

	privateKey, err := api.secrets.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: "backend-eth-private-key",
	})
	if err != nil {
		return nil, err
	}

	key, err := x509.ParseECPrivateKey(privateKey.Payload.Data)
	if err != nil {
		return nil, err
	}

	auth, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create authorized transactor: %w", err)
	}
	auth.Context = ctx

	asBigs := make([]*big.Int, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if redeemed[tokenID] {
			// if the token is redeemed, don't try to redeem it again
			continue
		}
		asBigs = append(asBigs, tokenID.BigInt())
	}

	tx, err := mer.Redeem(auth, asBigs)
	if err != nil {
		return nil, fmt.Errorf("failed to redeem tokens: %w", err)
	}

	logger.For(ctx).Infof("redeemed merch items with tx: %s", tx.Hash())

	return result, nil
}
