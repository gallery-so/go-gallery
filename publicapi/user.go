package publicapi

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/go-playground/validator/v10"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
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

func (api UserAPI) GetLoggedInUserId(ctx context.Context) persist.DBID {
	gc := util.GinContextFromContext(ctx)
	return auth.GetUserIDFromCtx(gc)
}

func (api UserAPI) IsUserLoggedIn(ctx context.Context) bool {
	gc := util.GinContextFromContext(ctx)
	return auth.GetUserAuthedFromCtx(gc)
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

func (api UserAPI) AddWalletToUser(ctx context.Context, chainAddress persist.ChainAddress, authenticator auth.Authenticator) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"chainAddress":  {chainAddress, "required"},
		"authenticator": {authenticator, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	err = user.AddWalletToUser(ctx, userID, chainAddress, authenticator, api.repos.UserRepository, api.repos.WalletRepository)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()
	return nil
}

func (api UserAPI) RemoveWalletsFromUser(ctx context.Context, walletIDs []persist.DBID) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"walletIDs": {walletIDs, "required,unique,dive,required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	err = user.RemoveWalletsFromUser(ctx, userID, walletIDs, api.repos.UserRepository)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()
	return nil
}

func (api UserAPI) CreateUser(ctx context.Context, authenticator auth.Authenticator, username string, bio string) (userID persist.DBID, galleryID persist.DBID, err error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"username": {username, "required,username"},
		"bio":      {bio, "bio"},
	}); err != nil {
		return "", "", err
	}

	userID, galleryID, err = user.CreateUser(ctx, authenticator, username, bio, api.repos.UserRepository, api.repos.GalleryRepository)

	// Send event
	dispatchEventToFeed(ctx, sqlc.Event{
		ActorID:        userID,
		Action:         persist.ActionUserCreated,
		ResourceTypeID: persist.ResourceTypeUser,
		UserID:         userID,
		SubjectID:      userID,
		Data:           persist.EventData{UserBio: bio},
	})

	return userID, galleryID, err
}

func (api UserAPI) UpdateUserInfo(ctx context.Context, username string, bio string) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"username": {username, "required,username"},
		"bio":      {bio, "bio"},
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

	return nil
}

func (api UserAPI) GetMembershipTiers(ctx context.Context, forceRefresh bool) ([]persist.MembershipTier, error) {
	// Nothing to validate
	return membership.GetMembershipTiers(ctx, forceRefresh, api.repos.MembershipRepository, api.repos.UserRepository, api.repos.GalleryRepository, api.repos.WalletRepository, api.ethClient, api.ipfsClient, api.arweaveClient, api.storageClient)
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

func (api UserAPI) GetCommunityByContractAddress(ctx context.Context, contractAddress persist.ChainAddress, forceRefresh bool) (*persist.Community, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractAddress": {contractAddress, "required"},
	}); err != nil {
		return nil, err
	}

	community, err := api.repos.CommunityRepository.GetByAddress(ctx, contractAddress, forceRefresh)
	if err != nil {
		return nil, err
	}

	return &community, nil
}

func (api UserAPI) GetFollowersByUserId(ctx context.Context, userID persist.DBID) ([]sqlc.User, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	if _, err := api.GetUserById(ctx, userID); err != nil {
		return nil, err
	}

	followers, err := api.loaders.FollowersByUserId.Load(userID)
	if err != nil {
		return nil, err
	}

	return followers, nil
}

func (api UserAPI) GetFollowingByUserId(ctx context.Context, userID persist.DBID) ([]sqlc.User, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	if _, err := api.GetUserById(ctx, userID); err != nil {
		return nil, err
	}

	following, err := api.loaders.FollowingByUserId.Load(userID)
	if err != nil {
		return nil, err
	}

	return following, nil
}

func (api UserAPI) FollowUser(ctx context.Context, userID persist.DBID) error {
	// Validate
	curUserID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	if err := validateFields(api.validator, validationMap{
		"userID": {userID, fmt.Sprintf("required,ne=%s", curUserID)},
	}); err != nil {
		return err
	}

	if _, err := api.GetUserById(ctx, userID); err != nil {
		return err
	}

	refollowed, err := api.repos.UserRepository.AddFollower(ctx, curUserID, userID)
	if err != nil {
		return err
	}

	// Send event
	go dispatchFollowEventToFeed(sentryutil.NewSentryHubGinContext(ctx), api, curUserID, userID, refollowed)

	return nil
}

func (api UserAPI) UnfollowUser(ctx context.Context, userID persist.DBID) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return err
	}

	curUserID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	return api.repos.UserRepository.RemoveFollower(ctx, curUserID, userID)
}

func dispatchFollowEventToFeed(ctx context.Context, api UserAPI, curUserID persist.DBID, followedUserID persist.DBID, refollowed bool) {
	followedBack, err := api.repos.UserRepository.UserFollowsUser(ctx, followedUserID, curUserID)

	if err != nil {
		sentryutil.ReportError(ctx, err)
		return
	}

	pushFeedEvent(ctx, sqlc.Event{
		ActorID:        curUserID,
		Action:         persist.ActionUserFollowedUsers,
		ResourceTypeID: persist.ResourceTypeUser,
		UserID:         curUserID,
		SubjectID:      followedUserID,
		Data:           persist.EventData{UserFollowedBack: followedBack, UserRefollowed: refollowed},
	})
}
