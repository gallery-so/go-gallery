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

		makeErrNotAuthorized := func(e string, c model.AuthorizationError) model.ErrNotAuthorized {
			return model.ErrNotAuthorized{
				Message: fmt.Sprintf("authorization failed: %s", e),
				Cause:   c,
			}
		}

		if authError := auth.GetAuthErrorFromCtx(gc); authError != nil {
			var gqlModel model.AuthorizationError
			errorMsg := authError.Error()

			switch authError {
			case auth.ErrNoCookie:
				gqlModel = model.ErrNoCookie{Message: errorMsg}
				addError(ctx, authError, gqlModel)
			case auth.ErrInvalidJWT:
				gqlModel = model.ErrInvalidToken{Message: errorMsg}
				addError(ctx, authError, gqlModel)
			default:
				return nil, authError
			}

			return makeErrNotAuthorized(errorMsg, gqlModel), nil
		}

		userID := auth.GetUserIDFromCtx(gc)
		if userID == "" {
			panic(fmt.Errorf("userID is empty, but no auth error occurred"))
		}

		if viper.GetBool("REQUIRE_NFTS") {
			user, err := dataloader.For(ctx).UserByUserId.Load(userID)

			if err != nil {
				return nil, err
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
				errorMsg := auth.ErrDoesNotOwnRequiredNFT{}.Error()
				nftErr := model.ErrDoesNotOwnRequiredNft{Message: errorMsg}

				return makeErrNotAuthorized(errorMsg, nftErr), nil
			}
		}

		return next(ctx)
	}
}
