package features

import (
	"context"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type getUserFeaturesInput struct {
	UserID persist.DBID `json:"user_id" binding:"required"`
}

type getUserFeaturesOutput struct {
	Features []*persist.FeatureFlag `json:"features"`
}

func getUserFeatures(userRepo persist.UserRepository, featureRepo persist.FeatureFlagRepository, accessRepo persist.AccessRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input getUserFeaturesInput
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := input.UserID
		access, err := accessRepo.GetByUserID(c, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		features, err := featureRepo.GetByRequiredTokens(c, access.RequiredTokensOwned)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, getUserFeaturesOutput{Features: features})

		// start a background process to update just in case websockets have not been able to keep track
		go func(pCtx context.Context) {

			ctx, cancel := context.WithTimeout(pCtx, time.Minute)
			defer cancel()

			user, err := userRepo.GetByID(ctx, userID)
			if err != nil {
				logrus.WithError(err).Error("failed to get user by ID")
				return
			}

			allFeatures, err := featureRepo.GetAll(ctx)
			if err != nil {
				logrus.WithError(err).Error("failed to get all features")
				return
			}

			tis := map[persist.TokenIdentifiers]uint64{}

			currentBlock, err := ethClient.BlockNumber(ctx)
			if err != nil {
				logrus.WithError(err).Error("failed to get current block")
				return
			}

			for _, feature := range allFeatures {
				// TODO erc20
				switch feature.TokenType {
				case persist.TokenTypeERC1155:
					address, tokenID := feature.RequiredToken.GetParts()
					ca, err := contracts.NewIERC1155Caller(address.Address(), ethClient)
					if err != nil {
						logrus.WithError(err).Error("failed to initialize ERC1155 caller")
						return
					}
					totalBal := new(big.Int)
					for _, address := range user.Addresses {
						bal, err := ca.BalanceOf(&bind.CallOpts{
							Context: ctx,
						}, address.Address(), tokenID.BigInt())
						if err == nil {
							totalBal.Add(totalBal, bal)
						}
					}
					tis[feature.RequiredToken] = totalBal.Uint64()
				case persist.TokenTypeERC721:
					address, tokenID := feature.RequiredToken.GetParts()
					ca, err := contracts.NewIERC721Caller(address.Address(), ethClient)
					if err != nil {
						logrus.WithError(err).Error("failed to initialize ERC1155 caller")
						return
					}
					isOwner := 0
					for _, address := range user.Addresses {
						owner, err := ca.OwnerOf(&bind.CallOpts{
							Context: ctx,
						}, tokenID.BigInt())
						if err == nil && strings.EqualFold(owner.Hex(), address.Address().Hex()) {
							isOwner = 1
							break
						}
					}
					tis[feature.RequiredToken] = uint64(isOwner)
				}
			}

			err = accessRepo.UpdateRequiredTokensByUserID(ctx, userID, tis, persist.BlockNumber(currentBlock))
			if err != nil {
				logrus.WithError(err).Error("failed to update required tokens")
				return
			}

		}(c.Copy())
	}
}
