package publicapi

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/util"
)

const apiContextKey = "publicapi.api"

type PublicAPI struct {
	repos      *persist.Repositories
	loaders    *dataloader.Loaders
	Collection *CollectionAPI
	Gallery    *GalleryAPI
	User       *UserAPI
	Nft        *NftAPI
}

func AddTo(ctx *gin.Context, repos *persist.Repositories, ethClient *ethclient.Client, pubsub pubsub.PubSub) {
	loaders := dataloader.NewLoaders(ctx, repos)
	api := &PublicAPI{
		repos:      repos,
		loaders:    loaders,
		Collection: &CollectionAPI{repos: repos, loaders: loaders, ethClient: ethClient, pubsub: pubsub},
		Gallery:    &GalleryAPI{repos: repos, loaders: loaders, ethClient: ethClient, pubsub: pubsub},
		User:       &UserAPI{repos: repos, loaders: loaders, ethClient: ethClient, pubsub: pubsub},
		Nft:        &NftAPI{repos: repos, loaders: loaders, ethClient: ethClient, pubsub: pubsub},
	}

	ctx.Set(apiContextKey, api)
}

func For(ctx context.Context) *PublicAPI {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(apiContextKey).(*PublicAPI)
}

func getAuthenticatedUser(ctx context.Context) (persist.DBID, error) {
	gc := util.GinContextFromContext(ctx)
	authError := auth.GetAuthErrorFromCtx(gc)

	if authError != nil {
		return "", authError
	}

	userID := auth.GetUserIDFromCtx(gc)
	return userID, nil
}

type ErrInvalidInput struct {
	Parameter string
	Reason    string
}

func (e ErrInvalidInput) Error() string {
	return fmt.Sprintf("invalid input: parameter '%s': %s", e.Parameter, e.Reason)
}
