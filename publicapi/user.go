package publicapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/spf13/viper"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/go-playground/validator/v10"
	shell "github.com/ipfs/go-ipfs-api"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/emails"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"roci.dev/fracdex"
)

type UserAPI struct {
	repos         *postgres.Repositories
	queries       *db.Queries
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

func (api UserAPI) GetUserById(ctx context.Context, userID persist.DBID) (*db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	user, err := api.loaders.UserByUserID.Load(userID)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserWithPII returns the current user and their associated personally identifiable information
func (api UserAPI) GetUserWithPII(ctx context.Context) (*db.UsersWithPii, error) {
	// Nothing to validate

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, err
	}

	userWithPII, err := api.queries.GetUserWithPIIByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &userWithPII, nil
}

func (api UserAPI) GetUsersByIDs(ctx context.Context, userIDs []persist.DBID, before, after *string, first, last *int) ([]db.User, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userIDs": {userIDs, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {

		users, err := api.queries.GetUsersByIDs(ctx, db.GetUsersByIDsParams{
			Limit:         params.Limit,
			UserIds:       userIDs,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
		if err != nil {
			return nil, err
		}

		interfaces := make([]interface{}, len(users))
		for i, user := range users {
			interfaces[i] = user
		}

		return interfaces, nil
	}

	countFunc := func() (int, error) {
		return len(userIDs), nil
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if user, ok := i.(db.User); ok {
			return user.CreatedAt, user.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an user")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	users := make([]db.User, len(results))
	for i, result := range results {
		if user, ok := result.(db.User); ok {
			users[i] = user
		}
	}

	return users, pageInfo, err
}

func (api UserAPI) GetUserByUsername(ctx context.Context, username string) (*db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
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

func (api UserAPI) GetUserByAddress(ctx context.Context, chainAddress persist.ChainAddress) (*db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"chainAddress": {chainAddress, "required"},
	}); err != nil {
		return nil, err
	}

	chain := chainAddress.Chain()
	user, err := api.loaders.UserByAddress.Load(db.GetUserByAddressBatchParams{
		Chain:   int32(chain),
		Address: persist.Address(chain.NormalizeAddress(chainAddress.Address())),
	})
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (api UserAPI) GetUsersWithTrait(ctx context.Context, trait string) ([]db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"trait": {trait, "required"},
	}); err != nil {
		return nil, err
	}

	users, err := api.loaders.UsersWithTrait.Load(trait)
	if err != nil {
		return nil, err
	}

	return users, nil
}

func (api *UserAPI) GetUserRolesByUserID(ctx context.Context, userID persist.DBID) ([]persist.Role, error) {
	address, tokenIDs := parseAddressTokens(viper.GetString("PREMIUM_CONTRACT_ADDRESS"))
	return api.queries.GetUserRolesByUserId(ctx, db.GetUserRolesByUserIdParams{
		UserID:                userID,
		MembershipAddress:     persist.Address(address),
		MembershipTokenIds:    tokenIDs,
		GrantedMembershipRole: persist.RoleEarlyAccess, // Role granted if user carries a matching token
		Chain:                 persist.ChainETH,
	})
}

// parseAddressTokens returns a contract and tokens from a string encoded as '<address>=[<tokenID>,<tokenID>,...<tokenID>]'.
// It's helpful for parsing contract and tokens passed as environment variables.
func parseAddressTokens(s string) (string, []string) {
	addressTokens := strings.Split(s, "=")
	if len(addressTokens) != 2 {
		panic("invalid address tokens format")
	}
	address, tokens := addressTokens[0], addressTokens[1]
	tokens = strings.TrimLeft(tokens, "[")
	tokens = strings.TrimRight(tokens, "]")
	return address, strings.Split(tokens, ",")
}

func (api UserAPI) PaginateUsersWithRole(ctx context.Context, role persist.Role, before *string, after *string, first *int, last *int) ([]db.User, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"role": {role, "required,role"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params lexicalPagingParams) ([]interface{}, error) {
		keys, err := api.queries.GetUsersWithRolePaginate(ctx, db.GetUsersWithRolePaginateParams{
			Role:          role,
			Limit:         params.Limit,
			CurBeforeKey:  params.CursorBeforeKey,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterKey:   params.CursorAfterKey,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	cursorFunc := func(i interface{}) (string, persist.DBID, error) {
		if row, ok := i.(db.User); ok {
			return row.UsernameIdempotent.String, row.ID, nil
		}
		return "", "", fmt.Errorf("interface{} is not a db.User")
	}

	paginator := lexicalPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	users := make([]db.User, len(results))
	for i, result := range results {
		users[i] = result.(db.User)
	}

	return users, pageInfo, err
}

func (api UserAPI) AddWalletToUser(ctx context.Context, chainAddress persist.ChainAddress, authenticator auth.Authenticator) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
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

	return nil
}

func (api UserAPI) RemoveWalletsFromUser(ctx context.Context, walletIDs []persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
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

	return nil
}

func (api UserAPI) CreateUser(ctx context.Context, authenticator auth.Authenticator, username string, email *persist.Email, bio, galleryName, galleryDesc, galleryPos string) (userID persist.DBID, galleryID persist.DBID, err error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"username": {username, "required,username"},
		"bio":      {bio, "bio"},
	}); err != nil {
		return "", "", err
	}

	if galleryPos == "" {
		first, err := fracdex.KeyBetween("", "")
		if err != nil {
			return "", "", err
		}
		galleryPos = first
	}

	userID, galleryID, err = user.CreateUser(ctx, authenticator, username, email, bio, galleryName, galleryDesc, galleryPos, api.repos.UserRepository, api.repos.GalleryRepository)
	if err != nil {
		return "", "", err
	}

	if email != nil && *email != "" {
		// TODO email validation ahead of time
		err = emails.RequestVerificationEmail(ctx, userID)
		if err != nil {
			return "", "", err
		}
	}

	// Send event
	_, err = dispatchEvent(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		Action:         persist.ActionUserCreated,
		ResourceTypeID: persist.ResourceTypeUser,
		UserID:         userID,
		SubjectID:      userID,
		Data:           persist.EventData{UserBio: bio},
	}, api.validator, nil)
	if err != nil {
		return "", "", err
	}

	return userID, galleryID, err
}

func (api UserAPI) UpdateUserInfo(ctx context.Context, username string, bio string) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
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

	err = user.UpdateUserInfo(ctx, userID, username, bio, api.repos.UserRepository, api.ethClient)
	if err != nil {
		return err
	}

	return nil
}

func (api UserAPI) UpdateUserPrimaryWallet(ctx context.Context, primaryWalletID persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"primaryWalletID": {primaryWalletID, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	err = api.queries.UpdateUserPrimaryWallet(ctx, db.UpdateUserPrimaryWalletParams{WalletID: primaryWalletID, UserID: userID})
	if err != nil {
		return err
	}

	return nil
}

func (api UserAPI) UpdateFeaturedGallery(ctx context.Context, galleryID persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	// query will validate that the gallery belongs to the user
	err = api.queries.UpdateUserFeaturedGallery(ctx, db.UpdateUserFeaturedGalleryParams{GalleryID: galleryID, UserID: userID})
	if err != nil {
		return err
	}

	return nil
}

func (api UserAPI) UpdateUserEmail(ctx context.Context, email persist.Email) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"email": {email, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}
	err = api.queries.UpdateUserEmail(ctx, db.UpdateUserEmailParams{
		UserID:       userID,
		EmailAddress: email,
	})
	if err != nil {
		return err
	}

	err = emails.RequestVerificationEmail(ctx, userID)
	if err != nil {
		return err
	}

	return nil
}

func (api UserAPI) UpdateUserEmailNotificationSettings(ctx context.Context, settings persist.EmailUnsubscriptions) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"settings": {settings, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	// update unsubscriptions

	return emails.UpdateUnsubscriptionsByUserID(ctx, userID, settings)

}

func (api UserAPI) ResendEmailVerification(ctx context.Context) error {

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	err = emails.RequestVerificationEmail(ctx, userID)
	if err != nil {
		return err
	}

	return nil
}

func (api UserAPI) UpdateUserNotificationSettings(ctx context.Context, notificationSettings persist.UserNotificationSettings) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"notification_settings": {notificationSettings, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	return api.queries.UpdateNotificationSettingsByID(ctx, db.UpdateNotificationSettingsByIDParams{ID: userID, NotificationSettings: notificationSettings})
}

func (api UserAPI) GetMembershipTiers(ctx context.Context, forceRefresh bool) ([]persist.MembershipTier, error) {
	// Nothing to validate
	return membership.GetMembershipTiers(ctx, forceRefresh, api.repos.MembershipRepository, api.repos.UserRepository, api.repos.GalleryRepository, api.repos.WalletRepository, api.ethClient, api.ipfsClient, api.arweaveClient, api.storageClient)
}

func (api UserAPI) GetMembershipByMembershipId(ctx context.Context, membershipID persist.DBID) (*db.Membership, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"membershipID": {membershipID, "required"},
	}); err != nil {
		return nil, err
	}

	membership, err := api.loaders.MembershipByMembershipID.Load(membershipID)
	if err != nil {
		return nil, err
	}

	return &membership, nil
}

func (api UserAPI) GetFollowersByUserId(ctx context.Context, userID persist.DBID) ([]db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	if _, err := api.GetUserById(ctx, userID); err != nil {
		return nil, err
	}

	followers, err := api.loaders.FollowersByUserID.Load(userID)
	if err != nil {
		return nil, err
	}

	return followers, nil
}

func (api UserAPI) GetFollowingByUserId(ctx context.Context, userID persist.DBID) ([]db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	if _, err := api.GetUserById(ctx, userID); err != nil {
		return nil, err
	}

	following, err := api.loaders.FollowingByUserID.Load(userID)
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

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
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
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
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

	pushEvent(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(curUserID),
		Action:         persist.ActionUserFollowedUsers,
		ResourceTypeID: persist.ResourceTypeUser,
		UserID:         curUserID,
		SubjectID:      followedUserID,
		Data:           persist.EventData{UserFollowedBack: followedBack, UserRefollowed: refollowed},
	})
}
