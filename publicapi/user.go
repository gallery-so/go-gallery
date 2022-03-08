package publicapi

import (
	"context"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/validate"
)

type UserAPI struct {
	repos     *persist.Repositories
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
	pubsub    pubsub.PubSub
}

func (api UserAPI) AddUserAddress(ctx context.Context, address persist.Address, authenticator auth.Authenticator) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"address":       {address, "required,eth_addr"},
		"authenticator": {authenticator, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	return user.AddAddressToUser(ctx, userID, address, authenticator, api.repos.UserRepository, api.pubsub)
}

func (api UserAPI) RemoveUserAddresses(ctx context.Context, addresses []persist.Address) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"addresses": {addresses, "required,unique,dive,required,eth_addr"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	return user.RemoveAddressesFromUser(ctx, userID, addresses, api.repos.UserRepository)
}

func (api UserAPI) UpdateUserInfo(ctx context.Context, username string, bio string) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"username": {username, "required,username"},
		"bio":      {bio, "required,medium"},
	}); err != nil {
		return err
	}

	// Sanitize
	bio = validate.SanitizationPolicy.Sanitize(bio)

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	return user.UpdateUser(ctx, userID, username, bio, api.repos.UserRepository, api.ethClient)
}

func (api UserAPI) GetMembershipTiers(ctx context.Context, forceRefresh bool) ([]persist.MembershipTier, error) {
	return membership.GetMembershipTiers(ctx, forceRefresh, api.repos.MembershipRepository, api.repos.UserRepository, api.repos.GalleryRepository, api.ethClient)
}
