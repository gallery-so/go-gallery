package graphql

import (
	"context"
	"fmt"
	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

func AuthRequiredDirectiveHandler(ethClient *ethclient.Client) func(ctx context.Context, obj interface{}, next gqlgen.Resolver) (res interface{}, err error) {
	return func(ctx context.Context, obj interface{}, next gqlgen.Resolver) (res interface{}, err error) {
		gc := util.GinContextFromContext(ctx)

		if authError := auth.GetAuthErrorFromCtx(gc); authError != nil {
			var errorType model.AuthFailureType

			switch authError {
			case auth.ErrNoCookie:
				errorType = model.AuthFailureTypeNoCookie
			case auth.ErrInvalidJWT:
				errorType = model.AuthFailureTypeInvalidToken
			default:
				errorType = model.AuthFailureTypeInternalError
			}

			notAuthorized := model.ErrNotAuthorized{
				Message:   authError.Error(),
				ErrorType: errorType,
			}

			return notAuthorized, nil
		}

		userID := auth.GetUserIDFromCtx(gc)
		if userID == "" {
			panic(fmt.Errorf("userID is empty, but no auth error occurred"))
		}

		if viper.GetBool("REQUIRE_NFTS") {
			user, err := dataloader.For(ctx).UserByUserId.Load(userID.String())
			if err != nil {
				notAuthorized := model.ErrNotAuthorized{Message: err.Error(), ErrorType: model.AuthFailureTypeInternalError}
				return notAuthorized, nil
			}

			has := false
			for _, addr := range user.Addresses {
				allowlist := auth.GetAllowlistContracts()
				for k, v := range allowlist {
					if found, _ := eth.HasNFTs(gc, k, v, addr, ethClient); found {
						has = true
						break
					}
				}
			}
			if !has {
				notAuthorized := model.ErrNotAuthorized{Message: err.Error(), ErrorType: model.AuthFailureTypeDoesNotOwnRequiredNft}
				return notAuthorized, nil
			}
		}

		return next(ctx)
	}
}
