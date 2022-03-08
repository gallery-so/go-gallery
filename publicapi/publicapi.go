package publicapi

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

const apiContextKey = "publicapi.api"

type PublicAPI struct {
	repos      *persist.Repositories
	loaders    *dataloader.Loaders
	validator  *validator.Validate
	Collection PublicCollectionAPI
	Gallery    PublicGalleryAPI
	User       PublicUserAPI
	Nft        PublicNftAPI
}

type PublicCollectionAPI interface {
	CreateCollection(ctx context.Context, galleryID persist.DBID, name string, collectorsNote string, nfts []persist.DBID, layout persist.TokenLayout) (*persist.Collection, error)
	DeleteCollection(ctx context.Context, collectionID persist.DBID) error
	UpdateCollectionNfts(ctx context.Context, collectionID persist.DBID, nfts []persist.DBID, layout persist.TokenLayout) error
	UpdateCollection(ctx context.Context, collectionID persist.DBID, name string, collectorsNote string) error
}

type PublicGalleryAPI interface {
	UpdateGalleryCollections(ctx context.Context, galleryID persist.DBID, collections []persist.DBID) error
}

type PublicUserAPI interface {
	AddUserAddress(ctx context.Context, address persist.Address, authenticator auth.Authenticator) error
	RemoveUserAddresses(ctx context.Context, addresses []persist.Address) error
	UpdateUserInfo(ctx context.Context, username, bio string) error
	GetMembershipTiers(ctx context.Context, forceRefresh bool) ([]persist.MembershipTier, error)
}

type PublicNftAPI interface {
	RefreshOpenSeaNfts(ctx context.Context, addresses string) error
}

func AddTo(ctx *gin.Context, repos *persist.Repositories, ethClient *ethclient.Client, pubsub pubsub.PubSub) {
	loaders := dataloader.NewLoaders(ctx, repos)
	validator := newValidator()
	collection := CollectionWithDispatch{PublicCollectionAPI: &CollectionAPI{repos: repos, loaders: loaders, validator: validator, ethClient: ethClient, pubsub: pubsub}, gc: ctx}
	user := UserWithDispatch{PublicUserAPI: &UserAPI{repos: repos, loaders: loaders, validator: validator, ethClient: ethClient, pubsub: pubsub}, gc: ctx}
	nft := NftWithDispatch{PublicNftAPI: &NftAPI{repos: repos, loaders: loaders, validator: validator, ethClient: ethClient, pubsub: pubsub}, gc: ctx}
	api := &PublicAPI{
		repos:      repos,
		loaders:    loaders,
		validator:  validator,
		Collection: collection,
		Gallery:    &GalleryAPI{repos: repos, loaders: loaders, validator: validator, ethClient: ethClient, pubsub: pubsub},
		User:       user,
		Nft:        nft,
	}

	ctx.Set(apiContextKey, api)
}

func For(ctx context.Context) *PublicAPI {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(apiContextKey).(*PublicAPI)
}

func newValidator() *validator.Validate {
	v := validator.New()
	validate.RegisterCustomValidators(v)
	return v
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

type validationMap map[string]struct {
	value interface{}
	tag   string
}

func validateFields(validator *validator.Validate, fields validationMap) error {
	validationErr := ErrInvalidInput{}
	foundErrors := false

	for k, v := range fields {
		err := validator.Var(v.value, v.tag)
		if err != nil {
			foundErrors = true
			validationErr.Append(k, err.Error())
		}
	}

	if foundErrors {
		return validationErr
	}

	return nil
}

type ErrInvalidInput struct {
	Parameters []string
	Reasons    []string
}

func (e *ErrInvalidInput) Append(parameter string, reason string) {
	e.Parameters = append(e.Parameters, parameter)
	e.Reasons = append(e.Reasons, reason)
}

func (e ErrInvalidInput) Error() string {
	str := "invalid input:\n"

	for i, _ := range e.Parameters {
		str += fmt.Sprintf("    parameter: %s, reason: %s\n", e.Parameters[i], e.Reasons[i])
	}

	return str
}
