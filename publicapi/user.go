package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgtype"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/service/socialauth"
	"github.com/mikeydub/go-gallery/service/user"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/go-playground/validator/v10"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/jinzhu/copier"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/emails"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"roci.dev/fracdex"
)

type UserAPI struct {
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	ipfsClient         *shell.Shell
	arweaveClient      *goar.Client
	storageClient      *storage.Client
	multichainProvider *multichain.Provider
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
func (api UserAPI) GetUserWithPII(ctx context.Context) (*db.PiiUserView, error) {
	// Nothing to validate

	userID, err := getAuthenticatedUserID(ctx)
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
	address, tokenIDs := parseAddressTokens(env.GetString("PREMIUM_CONTRACT_ADDRESS"))
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

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	err = user.AddWalletToUser(ctx, userID, chainAddress, authenticator, api.repos.UserRepository, api.repos.WalletRepository, api.multichainProvider)
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

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	err = user.RemoveWalletsFromUser(ctx, userID, walletIDs, api.repos.UserRepository)
	if err != nil {
		return err
	}

	return nil
}

func (api UserAPI) AddSocialAccountToUser(ctx context.Context, authenticator socialauth.Authenticator, display bool) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"authenticator": {authenticator, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	res, err := authenticator.Authenticate(ctx)
	if err != nil {
		return err
	}

	return api.queries.AddSocialToUser(ctx, db.AddSocialToUserParams{
		UserID: userID,
		Socials: persist.Socials{
			res.Provider: persist.SocialUserIdentifiers{
				Provider: res.Provider,
				ID:       res.ID,
				Display:  display,
				Metadata: res.Metadata,
			},
		},
	})
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

	userID, galleryID, err = user.CreateUser(ctx, authenticator, username, email, bio, galleryName, galleryDesc, galleryPos, api.repos.UserRepository, api.repos.GalleryRepository, api.multichainProvider)
	if err != nil {
		return "", "", err
	}

	err = api.queries.UpdateUserFeaturedGallery(ctx, db.UpdateUserFeaturedGalleryParams{GalleryID: galleryID, UserID: userID})
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

	gc := util.GinContextFromContext(ctx)
	err = api.queries.AddPiiAccountCreationInfo(ctx, db.AddPiiAccountCreationInfoParams{
		UserID:    userID,
		IpAddress: gc.ClientIP(),
	})
	if err != nil {
		logger.For(ctx).Warnf("failed to get IP address for userID %s: %s\n", userID, err)
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

	userID, err := getAuthenticatedUserID(ctx)
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

	userID, err := getAuthenticatedUserID(ctx)
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

	userID, err := getAuthenticatedUserID(ctx)
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

	userID, err := getAuthenticatedUserID(ctx)
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

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	// update unsubscriptions

	return emails.UpdateUnsubscriptionsByUserID(ctx, userID, settings)

}

func (api UserAPI) ResendEmailVerification(ctx context.Context) error {

	userID, err := getAuthenticatedUserID(ctx)
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

	userID, err := getAuthenticatedUserID(ctx)
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

func (api UserAPI) SharedFollowers(ctx context.Context, userID persist.DBID, before, after *string, first, last *int) ([]db.User, PageInfo, error) {
	// Validate
	curUserID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]any, error) {
		keys, err := api.loaders.SharedFollowersByUserIDs.Load(db.GetSharedFollowersBatchPaginateParams{
			Follower:      curUserID,
			Followee:      userID,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
			Limit:         params.Limit,
		})
		if err != nil {
			return nil, err
		}

		results := make([]any, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountSharedFollows(ctx, db.CountSharedFollowsParams{
			Follower: curUserID,
			Followee: userID,
		})
		return int(total), err
	}

	cursorFunc := func(i any) (time.Time, persist.DBID, error) {
		if row, ok := i.(db.GetSharedFollowersBatchPaginateRow); ok {
			return row.FollowedOn, row.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("node is not a db.GetSharedFollowersBatchPaginateRow")
	}

	paginator := sharedFollowersPaginator{
		timeIDPaginator{
			QueryFunc:  queryFunc,
			CursorFunc: cursorFunc,
			CountFunc:  countFunc,
		},
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	users := make([]db.User, len(results))
	for i, result := range results {
		if row, ok := result.(db.GetSharedFollowersBatchPaginateRow); ok {
			var u db.User
			copier.Copy(&u, &row)
			users[i] = u
		}
	}

	return users, pageInfo, nil
}

func (api UserAPI) SharedCommunities(ctx context.Context, userID persist.DBID, before, after *string, first, last *int) ([]db.Contract, PageInfo, error) {
	// Validate
	curUserID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params sharedContractsPaginatorParams) ([]any, error) {
		keys, err := api.loaders.SharedContractsByUserIDs.Load(db.GetSharedContractsBatchPaginateParams{
			UserAID:                   curUserID,
			UserBID:                   userID,
			CurBeforeDisplayedByUserA: params.CursorBeforeDisplayedByUserA,
			CurBeforeDisplayedByUserB: params.CursorBeforeDisplayedByUserB,
			CurBeforeOwnedCount:       int32(params.CursorBeforeOwnedCount),
			CurBeforeContractID:       params.CursorBeforeContractID,
			CurAfterDisplayedByUserA:  params.CursorAfterDisplayedByUserA,
			CurAfterDisplayedByUserB:  params.CursorAfterDisplayedByUserB,
			CurAfterOwnedCount:        int32(params.CursorAfterOwnedCount),
			CurAfterContractID:        params.CursorAfterContractID,
			PagingForward:             params.PagingForward,
			Limit:                     params.Limit,
		})
		if err != nil {
			return nil, err
		}

		results := make([]any, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountSharedContracts(ctx, db.CountSharedContractsParams{
			UserAID: curUserID,
			UserBID: userID,
		})
		return int(total), err
	}

	cursorFunc := func(i any) (bool, bool, int, persist.DBID, error) {
		if row, ok := i.(db.GetSharedContractsBatchPaginateRow); ok {
			return row.DisplayedByUserA, row.DisplayedByUserB, int(row.OwnedCount), row.ID, nil
		}
		return false, false, 0, "", fmt.Errorf("node is not a db.GetSharedContractsBatchPaginateRow")
	}

	paginator := sharedContractsPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	contracts := make([]db.Contract, len(results))
	for i, result := range results {
		if row, ok := result.(db.GetSharedContractsBatchPaginateRow); ok {
			var c db.Contract
			copier.Copy(&c, &row)
			contracts[i] = c
		}
	}

	return contracts, pageInfo, nil
}

func (api UserAPI) CreatedCommunities(ctx context.Context, userID persist.DBID, chains []persist.Chain, before, after *string, first, last *int) ([]db.Contract, PageInfo, error) {
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	// Run a sync to catch new contracts
	err := api.multichainProvider.SyncContractsCreatedByUserID(ctx, userID, chains)
	if err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]any, error) {
		serializedChains := make([]string, len(chains))
		for i, c := range chains {
			serializedChains[i] = strconv.Itoa(int(c))
		}
		keys, err := api.loaders.ContractsLoaderByCreatorID.Load(db.GetCreatedContractsBatchPaginateParams{
			UserID:        userID,
			Chains:        strings.Join(serializedChains, ","),
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
			Limit:         params.Limit,
		})
		if err != nil {
			return nil, err
		}

		results := make([]any, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	cursorFunc := func(node any) (time.Time, persist.DBID, error) {
		if row, ok := node.(db.Contract); ok {
			return row.CreatedAt, row.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("node is not a db.GetSharedContractsBatchPaginateRow")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	contracts := make([]db.Contract, len(results))
	for i, result := range results {
		contracts[i] = result.(db.Contract)
	}

	return nil, pageInfo, nil
}

func (api UserAPI) FollowUser(ctx context.Context, userID persist.DBID) error {
	// Validate
	curUserID, err := getAuthenticatedUserID(ctx)
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

func (api UserAPI) FollowAllSocialConnections(ctx context.Context, socialType persist.SocialProvider) error {
	// Validate
	curUserID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"socialType": {socialType, "required"},
	}); err != nil {
		return err
	}

	var userIDs []string
	switch socialType {
	case persist.SocialProviderTwitter:
		onlyUnfollowing := true
		conns, err := For(ctx).Social.GetConnections(ctx, socialType, &onlyUnfollowing)
		if err != nil {
			return err
		}
		userIDs, _ = util.Map(conns, func(s model.SocialConnection) (string, error) {
			return s.UserID.String(), nil
		})

	default:
		return fmt.Errorf("invalid social type: %s", socialType)
	}

	newIDs, _ := util.Map(userIDs, func(id string) (string, error) {
		return persist.GenerateID().String(), nil
	})

	return api.queries.AddManyFollows(ctx, db.AddManyFollowsParams{
		Ids:       newIDs,
		Follower:  curUserID,
		Followees: userIDs,
	})
}

func (api UserAPI) UnfollowUser(ctx context.Context, userID persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return err
	}

	curUserID, err := getAuthenticatedUserID(ctx)
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

func (api UserAPI) GetUserExperiences(ctx context.Context, userID persist.DBID) ([]*model.UserExperience, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	experiences, err := api.queries.GetUserExperiencesByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	asJSON := map[string]bool{}
	if err := experiences.AssignTo(&asJSON); err != nil {
		return nil, err
	}

	result := make([]*model.UserExperience, len(model.AllUserExperienceType))
	for i, experienceType := range model.AllUserExperienceType {
		result[i] = &model.UserExperience{
			Type:        experienceType,
			Experienced: asJSON[experienceType.String()],
		}
	}
	return result, nil
}

func (api UserAPI) GetSocials(ctx context.Context, userID persist.DBID) (*model.SocialAccounts, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	socials, err := api.queries.GetSocialsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := &model.SocialAccounts{}

	for prov, social := range socials {
		switch prov {
		case persist.SocialProviderTwitter:
			logger.For(ctx).Infof("found twitter social account: %+v", social)
			t := &model.TwitterSocialAccount{
				Type:     prov,
				Display:  social.Display,
				SocialID: social.ID,
			}
			name, ok := social.Metadata["name"].(string)
			if ok {
				t.Name = name
			}
			username, ok := social.Metadata["username"].(string)
			if ok {
				t.Username = username
			}
			profile, ok := social.Metadata["profile_image_url"].(string)
			if ok {
				t.ProfileImageURL = profile
			}
			result.Twitter = t
		default:
			return nil, fmt.Errorf("unknown social provider %s", prov)
		}
	}

	return result, nil
}

func (api UserAPI) GetDisplayedSocials(ctx context.Context, userID persist.DBID) (*model.SocialAccounts, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	socials, err := api.queries.GetSocialsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	for prov, social := range socials {
		if !social.Display {
			delete(socials, prov)
		}
	}

	result := &model.SocialAccounts{}

	for prov, social := range socials {
		switch prov {
		case persist.SocialProviderTwitter:
			logger.For(ctx).Infof("found twitter social account: %+v", social)
			t := &model.TwitterSocialAccount{
				Type:     prov,
				Display:  social.Display,
				SocialID: social.ID,
			}
			name, ok := social.Metadata["name"].(string)
			if ok {
				t.Name = name
			}
			username, ok := social.Metadata["username"].(string)
			if ok {
				t.Username = username
			}
			profile, ok := social.Metadata["profile_image_url"].(string)
			if ok {
				t.ProfileImageURL = profile
			}
			result.Twitter = t
		default:
			return nil, fmt.Errorf("unknown social provider %s", prov)
		}
	}

	return result, nil
}

func (api UserAPI) UpdateUserSocialDisplayed(ctx context.Context, socialType persist.SocialProvider, displayed bool) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"socialType": {socialType, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	socials, err := api.queries.GetSocialsByUserID(ctx, userID)
	if err != nil {
		return err
	}

	social, ok := socials[socialType]
	if !ok {
		return fmt.Errorf("social account not found for user %s and provider %s", userID, socialType)
	}

	social.Display = displayed

	socials[socialType] = social

	return api.queries.UpdateUserSocials(ctx, db.UpdateUserSocialsParams{
		Socials: socials,
		UserID:  userID,
	})
}

func (api UserAPI) UpdateUserExperience(ctx context.Context, experienceType model.UserExperienceType, value bool) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"experienceType": {experienceType, "required"},
	}); err != nil {
		return err
	}

	curUserID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	in := map[string]interface{}{
		experienceType.String(): value,
	}

	marshalled, err := json.Marshal(in)
	if err != nil {
		return err
	}

	return api.queries.UpdateUserExperience(ctx, db.UpdateUserExperienceParams{
		Experience: pgtype.JSONB{
			Bytes:  marshalled,
			Status: pgtype.Present,
		},
		UserID: curUserID,
	})
}

func (api UserAPI) RecommendUsers(ctx context.Context, before, after *string, first, last *int) ([]db.User, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	curUserID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	paginator := positionPaginator{}
	var userIDs []persist.DBID

	// If we have a cursor, we can page through the original set of recommended users
	if before != nil {
		if _, userIDs, err = paginator.decodeCursor(*before); err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		if _, userIDs, err = paginator.decodeCursor(*after); err != nil {
			return nil, PageInfo{}, err
		}
	} else {
		// Otherwise make a new recommendation
		follows, err := api.queries.GetFollowEdgesByUserID(ctx, curUserID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		userIDs, err = recommend.For(ctx).RecommendFromFollowingShuffled(ctx, curUserID, follows)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	positionLookup := map[persist.DBID]int{}
	idsAsString := make([]string, len(userIDs))

	for i, id := range userIDs {
		// Postgres uses 1-based indexing
		positionLookup[id] = i + 1
		idsAsString[i] = id.String()
	}

	paginator.QueryFunc = func(params positionPagingParams) ([]any, error) {
		keys, err := api.queries.GetUsersByPositionPaginate(ctx, db.GetUsersByPositionPaginateParams{
			UserIds:       idsAsString,
			CurBeforePos:  params.CursorBeforePos,
			CurAfterPos:   params.CursorAfterPos,
			PagingForward: params.PagingForward,
			Limit:         params.Limit,
		})
		if err != nil {
			return nil, err
		}

		results := make([]any, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	paginator.CursorFunc = func(node any) (int, []persist.DBID, error) {
		if user, ok := node.(db.User); ok {
			return positionLookup[user.ID], userIDs, nil
		}
		return 0, nil, fmt.Errorf("node is not a db.User")
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	users := make([]db.User, len(results))
	for i, result := range results {
		users[i] = result.(db.User)
	}

	return users, pageInfo, err
}
