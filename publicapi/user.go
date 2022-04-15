package publicapi

import (
	"context"

	"github.com/mikeydub/go-gallery/db/sqlc"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/go-playground/validator/v10"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type UserAPI struct {
	repos         *persist.Repositories
	queries       *sqlc.Queries
	loaders       *dataloader.Loaders
	validator     *validator.Validate
	ethClient     *ethclient.Client
	ipfsClient    *shell.Shell
	arweaveClient *goar.Client
	storageClient *storage.Client
}

func (api UserAPI) GetUserById(ctx context.Context, userID persist.DBID) (*sqlc.User, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	user, err := api.loaders.UserByUserId.Load(userID)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (api UserAPI) GetUserByUsername(ctx context.Context, username string) (*sqlc.User, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"username": {username, "required"},
	}); err != nil {
		return nil, err
	}

	user, err := api.loaders.UserByUsername.Load(username)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (api UserAPI) GetUserByAddress(ctx context.Context, address persist.EthereumAddress) (*sqlc.User, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"address": {address, "required,eth_addr"},
	}); err != nil {
		return nil, err
	}

	user, err := api.loaders.UserByAddress.Load(address)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (api UserAPI) AddUserAddress(ctx context.Context, address persist.EthereumAddress, authenticator auth.Authenticator) error {
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

	err = user.AddAddressToUser(ctx, userID, address, authenticator, api.repos.UserRepository)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()
	return nil
}

func (api UserAPI) RemoveUserAddresses(ctx context.Context, addresses []persist.EthereumAddress) error {
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

	err = user.RemoveAddressesFromUser(ctx, userID, addresses, api.repos.UserRepository)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()
	return nil
}

func (api UserAPI) UpdateUserInfo(ctx context.Context, username string, bio string) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"username": {username, "required,username"},
		"bio":      {bio, "required,bio"},
	}); err != nil {
		return err
	}

	// Sanitize
	bio = validate.SanitizationPolicy.Sanitize(bio)

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	err = user.UpdateUser(ctx, userID, username, bio, api.repos.UserRepository, api.ethClient)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()

	// Send event
	userData := persist.UserEvent{Username: username, Bio: persist.NullString(bio)}
	dispatchUserEvent(ctx, persist.UserCreatedEvent, userID, userData)

	return nil
}

func (api UserAPI) GetMembershipTiers(ctx context.Context, forceRefresh bool) ([]persist.MembershipTier, error) {
	return membership.GetMembershipTiers(ctx, forceRefresh, api.repos.MembershipRepository, api.repos.UserRepository, api.repos.GalleryRepository, api.ethClient, api.ipfsClient, api.arweaveClient, api.storageClient)
}

func (api UserAPI) GetMembershipByMembershipId(ctx context.Context, membershipID persist.DBID) (*sqlc.Membership, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"membershipID": {membershipID, "required"},
	}); err != nil {
		return nil, err
	}

	membership, err := api.loaders.MembershipByMembershipId.Load(membershipID)
	if err != nil {
		return nil, err
	}

	return &membership, nil
}

func dispatchUserEvent(ctx context.Context, eventCode persist.EventCode, userID persist.DBID, userData persist.UserEvent) {
	gc := util.GinContextFromContext(ctx)
	userHandlers := event.For(gc).User
	evt := persist.UserEventRecord{
		UserID: userID,
		Code:   eventCode,
		Data:   userData,
	}

	userHandlers.Dispatch(evt)
}
