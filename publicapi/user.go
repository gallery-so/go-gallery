package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gallery-so/fracdex"
	"github.com/go-playground/validator/v10"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/emails"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc/ipfs"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/socialauth"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

var ErrProfileImageTooManySources = errors.New("too many profile image sources provided")
var ErrProfileImageUnknownSource = errors.New("unknown profile image source to use")
var ErrProfileImageNotTokenOwner = errors.New("user is not an owner of the token")
var ErrProfileImageNotWalletOwner = errors.New("user is not the owner of the wallet")

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
	taskClient         *task.Client
	cache              *redis.Cache
}

func (api UserAPI) GetLoggedInUserId(ctx context.Context) persist.DBID {
	gc := util.MustGetGinContext(ctx)
	return auth.GetUserIDFromCtx(gc)
}

func (api UserAPI) IsUserLoggedIn(ctx context.Context) bool {
	gc := util.MustGetGinContext(ctx)
	return auth.GetUserAuthedFromCtx(gc)
}

func (api UserAPI) GetUserById(ctx context.Context, userID persist.DBID) (*db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, err
	}

	user, err := api.loaders.GetUserByIdBatch.Load(userID)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (api UserAPI) GetUserByVerifiedEmailAddress(ctx context.Context, emailAddress persist.Email) (*db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"emailAddress": validate.WithTag(emailAddress, "required"),
	}); err != nil {
		return nil, err
	}

	// Intentionally using queries here instead of a dataloader. Caching a user by email address is tricky
	// because the key (email address) isn't part of the user object, and this method isn't currently invoked
	// in a way that would benefit from dataloaders or caching anyway.
	user, err := api.queries.GetUserByVerifiedEmailAddress(ctx, emailAddress.String())

	if err != nil {
		if err == pgx.ErrNoRows {
			err = persist.ErrUserNotFound{Email: emailAddress}
		}
		return nil, err
	}

	return &user, nil
}

func (api UserAPI) VerifiedEmailAddressExists(ctx context.Context, emailAddress persist.Email) (bool, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"emailAddress": validate.WithTag(emailAddress, "required"),
	}); err != nil {
		return false, err
	}

	// Intentionally using queries here instead of a dataloader. Caching a user by email address is tricky
	// because the key (email address) isn't part of the user object, and this method isn't currently invoked
	// in a way that would benefit from dataloaders or caching anyway.
	_, err := api.queries.GetUserByVerifiedEmailAddress(ctx, emailAddress.String())

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return true, nil
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
		"userIDs": validate.WithTag(userIDs, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]db.User, error) {
		return api.queries.GetUsersByIDs(ctx, db.GetUsersByIDsParams{
			Limit:         params.Limit,
			UserIds:       userIDs,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		return len(userIDs), nil
	}

	cursorFunc := func(u db.User) (time.Time, persist.DBID, error) {
		return u.CreatedAt, u.ID, nil
	}

	paginator := timeIDPaginator[db.User]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api UserAPI) paginatorFromCursorStr(ctx context.Context, curStr string) (positionPaginator[db.User], error) {
	cur := cursors.NewPositionCursor()
	err := cur.Unpack(curStr)
	if err != nil {
		return positionPaginator[db.User]{}, err
	}
	return api.paginatorFromCursor(ctx, cur), nil
}

func (api UserAPI) paginatorFromCursor(ctx context.Context, c *positionCursor) positionPaginator[db.User] {
	return api.paginatorWithQuery(c, func(p positionPagingParams) ([]db.User, error) {
		params := db.GetUsersByPositionPaginateBatchParams{
			UserIds: util.MapWithoutError(c.IDs, func(id persist.DBID) string { return id.String() }),
			// Postgres uses 1-based indexing
			CurBeforePos: p.CursorBeforePos + 1,
			CurAfterPos:  p.CursorAfterPos + 1,
		}
		return api.loaders.GetUsersByPositionPaginateBatch.Load(params)
	})
}

func (api UserAPI) paginatorFromResults(ctx context.Context, c *positionCursor, users []db.User) positionPaginator[db.User] {
	queryF := func(positionPagingParams) ([]db.User, error) { return users, nil }
	return api.paginatorWithQuery(c, queryF)
}

func (api UserAPI) paginatorWithQuery(c *positionCursor, queryF func(positionPagingParams) ([]db.User, error)) positionPaginator[db.User] {
	var paginator positionPaginator[db.User]
	paginator.QueryFunc = queryF
	paginator.CursorFunc = func(u db.User) (int64, []persist.DBID, error) { return c.Positions[u.ID], c.IDs, nil }
	return paginator
}

func (api UserAPI) GetUserByUsername(ctx context.Context, username string) (*db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"username": validate.WithTag(username, "required"),
	}); err != nil {
		return nil, err
	}

	user, err := api.loaders.GetUserByUsernameBatch.Load(username)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (api UserAPI) GetUserByAddress(ctx context.Context, chainAddress persist.ChainAddress) (*db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"chainAddress": validate.WithTag(chainAddress, "required"),
	}); err != nil {
		return nil, err
	}

	chain := chainAddress.Chain()
	user, err := api.loaders.GetUserByAddressAndL1Batch.Load(db.GetUserByAddressAndL1BatchParams{
		L1Chain: chain.L1Chain(),
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
		"trait": validate.WithTag(trait, "required"),
	}); err != nil {
		return nil, err
	}

	users, err := api.loaders.GetUsersWithTraitBatch.Load(trait)
	if err != nil {
		return nil, err
	}

	return users, nil
}

func (api *UserAPI) OptInForRoles(ctx context.Context, roles []persist.Role) (*db.User, error) {
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"roles": validate.WithTag(roles, "required,min=1,unique,dive,role,opt_in_role"),
	}); err != nil {
		return nil, err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, err
	}

	// The opt_in_role validator already checks this, but let's be explicit about not letting
	// users opt in for the admin role.
	for _, role := range roles {
		if role == persist.RoleAdmin {
			err = errors.New("cannot opt in for admin role")
			sentryutil.ReportError(ctx, err)
			return nil, err
		}
	}

	newRoles := util.MapWithoutError(roles, func(role persist.Role) string { return string(role) })
	ids := util.MapWithoutError(roles, func(role persist.Role) string { return persist.GenerateID().String() })

	err = api.queries.AddUserRoles(ctx, db.AddUserRolesParams{
		UserID: userID,
		Ids:    ids,
		Roles:  newRoles,
	})

	if err != nil {
		return nil, err
	}

	// Even though the user's roles have changed in the database, it could take a while before
	// the new roles are reflected in their auth token. Forcing an auth token refresh will
	// make the roles appear immediately.
	err = For(ctx).Auth.ForceAuthTokenRefresh(ctx, userID)
	if err != nil {
		logger.For(ctx).Errorf("error forcing auth token refresh for user %s: %s", userID, err)
	}

	user, err := api.queries.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &user, err
}

func (api *UserAPI) OptOutForRoles(ctx context.Context, roles []persist.Role) (*db.User, error) {
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"roles": validate.WithTag(roles, "required,min=1,unique,dive,role,opt_in_role"),
	}); err != nil {
		return nil, err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, err
	}

	// The opt_in_role validator already checks this, but let's be explicit about not letting
	// users opt out of the admin role.
	for _, role := range roles {
		if role == persist.RoleAdmin {
			err := errors.New("cannot opt out of admin role")
			sentryutil.ReportError(ctx, err)
			return nil, err
		}
	}

	err = api.queries.DeleteUserRoles(ctx, db.DeleteUserRolesParams{
		Roles:  roles,
		UserID: userID,
	})

	if err != nil {
		return nil, err
	}

	// Even though the user's roles have changed in the database, it could take a while before
	// the new roles are reflected in their auth token. Forcing an auth token refresh will
	// make the roles appear immediately.
	err = For(ctx).Auth.ForceAuthTokenRefresh(ctx, userID)
	if err != nil {
		logger.For(ctx).Errorf("error forcing auth token refresh for user %s: %s", userID, err)
	}

	user, err := api.queries.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &user, err
}

func (api *UserAPI) GetUserRolesByUserID(ctx context.Context, userID persist.DBID) ([]persist.Role, error) {
	return auth.RolesByUserID(ctx, api.queries, userID)
}

func (api *UserAPI) UserIsAdmin(ctx context.Context) bool {
	for _, role := range getUserRoles(ctx) {
		if role == persist.RoleAdmin {
			return true
		}
	}
	return false
}

func (api UserAPI) PaginateUsersWithRole(ctx context.Context, role persist.Role, before *string, after *string, first *int, last *int) ([]db.User, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"role": validate.WithTag(role, "required,role"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params lexicalPagingParams) ([]db.User, error) {
		return api.queries.GetUsersWithRolePaginate(ctx, db.GetUsersWithRolePaginateParams{
			Role:          role,
			Limit:         params.Limit,
			CurBeforeKey:  params.CursorBeforeKey,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterKey:   params.CursorAfterKey,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	cursorFunc := func(u db.User) (string, persist.DBID, error) {
		return u.UsernameIdempotent.String, u.ID, nil
	}

	paginator := lexicalPaginator[db.User]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api UserAPI) AddWalletToUser(ctx context.Context, chainAddress persist.ChainAddress, authenticator auth.Authenticator) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"chainAddress":  validate.WithTag(chainAddress, "required"),
		"authenticator": validate.WithTag(authenticator, "required"),
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	err = user.AddWalletToUser(ctx, userID, chainAddress, authenticator, api.repos.UserRepository, api.multichainProvider)
	if err != nil {
		return err
	}

	err = api.taskClient.CreateTaskForAutosocialProcessUsers(ctx, task.AutosocialProcessUsersMessage{
		Users: map[persist.DBID]map[persist.SocialProvider][]persist.ChainAddress{
			userID: {
				persist.SocialProviderFarcaster: []persist.ChainAddress{chainAddress},
				persist.SocialProviderLens:      []persist.ChainAddress{chainAddress},
			},
		},
	})
	if err != nil {
		logger.For(ctx).WithError(err).Error("failed to create task for autosocial process users")
	}
	return nil
}

func (api UserAPI) RemoveWalletsFromUser(ctx context.Context, walletIDs []persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"walletIDs": validate.WithTag(walletIDs, "required,unique,dive,required"),
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	removedIDs, removalErr := user.RemoveWalletsFromUser(ctx, userID, walletIDs, api.repos.UserRepository)

	// If any wallet IDs were successfully removed, we need to process those removals, even if we also
	// encountered an error.
	if len(removedIDs) > 0 {
		walletRemovalMessage := task.TokenProcessingWalletRemovalMessage{
			UserID:    userID,
			WalletIDs: removedIDs,
		}

		if err := api.taskClient.CreateTaskForWalletRemoval(ctx, walletRemovalMessage); err != nil {
			// Just log the error here. No need to return it -- the actual wallet removal DID succeed,
			// but tokens owned by the affected wallets won't be updated until the user's next sync.
			logger.For(ctx).WithError(err).Error("failed to create task to process wallet removal")
		}
	}

	return removalErr
}

func (api UserAPI) AddSocialAccountToUser(ctx context.Context, authenticator socialauth.Authenticator, display bool) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"authenticator": validate.WithTag(authenticator, "required"),
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
		"username": validate.WithTag(username, "required,username"),
		"bio":      validate.WithTag(bio, "bio"),
	}); err != nil {
		return "", "", err
	}

	createUserParams, err := createNewUserParamsWithAuth(ctx, authenticator, username, bio, email)
	if err != nil {
		return "", "", err
	}

	createGalleryParams, err := createNewUserGalleryParams(galleryName, galleryDesc, galleryPos)
	if err != nil {
		return "", "", err
	}

	tx, err := api.repos.BeginTx(ctx)
	if err != nil {
		return "", "", err
	}
	queries := api.queries.WithTx(tx)
	defer tx.Rollback(ctx)

	userID, galleryID, err = user.CreateUser(ctx, createUserParams, createGalleryParams, api.repos.UserRepository, queries)
	if err != nil {
		return "", "", err
	}

	err = queries.UpdateUserFeaturedGallery(ctx, db.UpdateUserFeaturedGalleryParams{GalleryID: galleryID, UserID: userID})
	if err != nil {
		return "", "", err
	}

	gc := util.MustGetGinContext(ctx)
	err = queries.AddPiiAccountCreationInfo(ctx, db.AddPiiAccountCreationInfoParams{
		UserID:    userID,
		IpAddress: gc.ClientIP(),
	})
	if err != nil {
		logger.For(ctx).Warnf("failed to get IP address for userID %s: %s\n", userID, err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return "", "", err
	}

	if createUserParams.EmailStatus == persist.EmailVerificationStatusUnverified && email != nil {
		if err := emails.RequestVerificationEmail(ctx, userID); err != nil {
			// Just the log the error since the user can verify their email later
			logger.For(ctx).Warnf("failed to send verification email: %s", err)
		}
	}

	if createUserParams.EmailStatus == persist.EmailVerificationStatusVerified {
		if err := api.taskClient.CreateTaskForAddingEmailToMailingList(ctx, task.AddEmailToMailingListMessage{UserID: userID}); err != nil {
			// Report error to Sentry since there's not another way to subscribe the user to the mailing list
			sentryutil.ReportError(ctx, err)
			logger.For(ctx).Warnf("failed to send mailing list subscription task: %s", err)
		}
	}

	// Send event
	err = event.Dispatch(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		Action:         persist.ActionUserCreated,
		ResourceTypeID: persist.ResourceTypeUser,
		UserID:         userID,
		SubjectID:      userID,
		Data:           persist.EventData{UserBio: bio},
	})
	if err != nil {
		logger.For(ctx).Errorf("failed to dispatch event: %s", err)
	}

	err = api.taskClient.CreateTaskForAutosocialProcessUsers(ctx, task.AutosocialProcessUsersMessage{
		Users: map[persist.DBID]map[persist.SocialProvider][]persist.ChainAddress{
			userID: {
				persist.SocialProviderFarcaster: []persist.ChainAddress{createUserParams.ChainAddress},
				persist.SocialProviderLens:      []persist.ChainAddress{createUserParams.ChainAddress},
			},
		},
	})
	if err != nil {
		logger.For(ctx).Errorf("failed to create task for autosocial process users: %s", err)
	}

	return userID, galleryID, nil
}

func (api UserAPI) UpdateUserInfo(ctx context.Context, username string, bio string) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"username": validate.WithTag(username, "required,username"),
		"bio":      validate.WithTag(bio, "bio"),
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
		"primaryWalletID": validate.WithTag(primaryWalletID, "required"),
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
		"galleryID": validate.WithTag(galleryID, "required"),
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
		"email": validate.WithTag(email, "required"),
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}
	err = api.queries.UpdateUserUnverifiedEmail(ctx, db.UpdateUserUnverifiedEmailParams{
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
		"settings": validate.WithTag(settings, "required"),
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

func (api UserAPI) GetCurrentUserEmailNotificationSettings(ctx context.Context) (persist.EmailUnsubscriptions, error) {

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return persist.EmailUnsubscriptions{}, err
	}

	// update unsubscriptions

	return emails.GetCurrentUnsubscriptionsByUserID(ctx, userID)

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
		"notification_settings": validate.WithTag(notificationSettings, "required"),
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
		"membershipID": validate.WithTag(membershipID, "required"),
	}); err != nil {
		return nil, err
	}

	membership, err := api.loaders.GetMembershipByMembershipIdBatch.Load(membershipID)
	if err != nil {
		return nil, err
	}

	return &membership, nil
}

func (api UserAPI) GetFollowersByUserId(ctx context.Context, userID persist.DBID) ([]db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, err
	}

	if _, err := api.GetUserById(ctx, userID); err != nil {
		return nil, err
	}

	followers, err := api.loaders.GetFollowersByUserIdBatch.Load(userID)
	if err != nil {
		return nil, err
	}

	return followers, nil
}

func (api UserAPI) GetFollowingByUserId(ctx context.Context, userID persist.DBID) ([]db.User, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, err
	}

	if _, err := api.GetUserById(ctx, userID); err != nil {
		return nil, err
	}

	following, err := api.loaders.GetFollowingByUserIdBatch.Load(userID)
	if err != nil {
		return nil, err
	}

	return following, nil
}

func (api UserAPI) SharedFollowers(ctx context.Context, userID persist.DBID, before, after *string, first, last *int) ([]db.User, PageInfo, error) {
	curUserID, _ := getAuthenticatedUserID(ctx)

	// If the user is not logged in, return an empty list of users
	if curUserID == "" {
		return []db.User{}, PageInfo{}, nil
	}

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]db.GetSharedFollowersBatchPaginateRow, error) {
		return api.loaders.GetSharedFollowersBatchPaginate.Load(db.GetSharedFollowersBatchPaginateParams{
			Follower:      curUserID,
			Followee:      userID,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
			Limit:         params.Limit,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountSharedFollows(ctx, db.CountSharedFollowsParams{
			Follower: curUserID,
			Followee: userID,
		})
		return int(total), err
	}

	cursorFunc := func(r db.GetSharedFollowersBatchPaginateRow) (time.Time, persist.DBID, error) {
		return r.FollowedOn, r.User.ID, nil
	}

	paginator := sharedFollowersPaginator[db.GetSharedFollowersBatchPaginateRow]{
		timeIDPaginator[db.GetSharedFollowersBatchPaginateRow]{
			QueryFunc:  queryFunc,
			CursorFunc: cursorFunc,
			CountFunc:  countFunc,
		},
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)
	users := util.MapWithoutError(results, func(r db.GetSharedFollowersBatchPaginateRow) db.User { return r.User })
	return users, pageInfo, err
}

func (api UserAPI) SharedCommunities(ctx context.Context, userID persist.DBID, before, after *string, first, last *int) ([]db.Community, PageInfo, error) {
	// Validate
	curUserID, _ := getAuthenticatedUserID(ctx)

	// If the user is not logged in, return an empty list of users
	if curUserID == "" {
		return []db.Community{}, PageInfo{}, nil
	}

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params sharedCommunitiesPaginatorParams) ([]db.GetSharedCommunitiesBatchPaginateRow, error) {
		return api.loaders.GetSharedCommunitiesBatchPaginate.Load(db.GetSharedCommunitiesBatchPaginateParams{
			UserAID:                   curUserID,
			UserBID:                   userID,
			CurBeforeDisplayedByUserA: params.CursorBeforeDisplayedByUserA,
			CurBeforeDisplayedByUserB: params.CursorBeforeDisplayedByUserB,
			CurBeforeOwnedCount:       int32(params.CursorBeforeOwnedCount),
			CurBeforeContractID:       params.CursorBeforeCommunityID,
			CurAfterDisplayedByUserA:  params.CursorAfterDisplayedByUserA,
			CurAfterDisplayedByUserB:  params.CursorAfterDisplayedByUserB,
			CurAfterOwnedCount:        int32(params.CursorAfterOwnedCount),
			CurAfterContractID:        params.CursorAfterCommunityID,
			PagingForward:             params.PagingForward,
			Limit:                     params.Limit,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountSharedCommunities(ctx, db.CountSharedCommunitiesParams{
			UserAID: curUserID,
			UserBID: userID,
		})
		return int(total), err
	}

	cursorFunc := func(r db.GetSharedCommunitiesBatchPaginateRow) (bool, bool, int64, persist.DBID, error) {
		return r.DisplayedByUserA, r.DisplayedByUserB, int64(r.OwnedCount), r.Community.ID, nil
	}

	paginator := sharedCommunitiesPaginator[db.GetSharedCommunitiesBatchPaginateRow]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)
	communities := util.MapWithoutError(results, func(r db.GetSharedCommunitiesBatchPaginateRow) db.Community { return r.Community })
	return communities, pageInfo, err
}

func (api UserAPI) CreatedCommunities(ctx context.Context, userID persist.DBID, includeChains []persist.Chain, before, after *string, first, last *int) ([]db.Contract, PageInfo, error) {
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]db.Contract, error) {
		serializedChains := make([]string, len(includeChains))
		for i, c := range includeChains {
			serializedChains[i] = strconv.Itoa(int(c))
		}
		return api.loaders.GetCreatedContractsBatchPaginate.Load(db.GetCreatedContractsBatchPaginateParams{
			UserID:           userID,
			Chains:           strings.Join(serializedChains, ","),
			CurBeforeTime:    params.CursorBeforeTime,
			CurBeforeID:      params.CursorBeforeID,
			CurAfterTime:     params.CursorAfterTime,
			CurAfterID:       params.CursorAfterID,
			PagingForward:    params.PagingForward,
			Limit:            params.Limit,
			IncludeAllChains: len(includeChains) == 0,
		})
	}

	cursorFunc := func(c db.Contract) (time.Time, persist.DBID, error) {
		return c.CreatedAt, c.ID, nil
	}

	paginator := timeIDPaginator[db.Contract]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api UserAPI) FollowUser(ctx context.Context, userID persist.DBID) error {
	// Validate
	curUserID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, fmt.Sprintf("required,ne=%s", curUserID)),
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
		"socialType": validate.WithTag(socialType, "required"),
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

func (api UserAPI) FollowAllOnboardingRecommendations(ctx context.Context, curStr *string) error {
	curUserID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	var usersToFollow []persist.DBID

	if curStr != nil {
		cursor := cursors.NewPositionCursor()
		if err := cursor.Unpack(*curStr); err != nil {
			// Just log the error and continue
			sentryutil.ReportError(ctx, err)
		} else {
			usersToFollow = cursor.IDs
		}
	}

	// cursor wasn't provided or is invalid
	if len(usersToFollow) == 0 {
		usersToFollow, err = GetOnboardingUserRecommendationsBootstrap(api.queries)(ctx)
		if err != nil {
			return err
		}
	}

	ids := make([]string, len(usersToFollow))
	userIDs := make([]string, len(usersToFollow))
	for i, id := range usersToFollow {
		ids[i] = persist.GenerateID().String()
		userIDs[i] = id.String()
	}

	return api.queries.AddManyFollows(ctx, db.AddManyFollowsParams{
		Ids:       ids,
		Follower:  curUserID,
		Followees: userIDs,
	})
}

func (api UserAPI) UnfollowUser(ctx context.Context, userID persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
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

	event.PushEvent(ctx, db.Event{
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
		"userID": validate.WithTag(userID, "required"),
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
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, err
	}

	socials, err := api.queries.GetSocialsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := &model.SocialAccounts{}

	for prov, social := range socials {
		assignSocialToModel(ctx, prov, social, result)
	}

	return result, nil
}

func (api UserAPI) GetDisplayedSocials(ctx context.Context, userID persist.DBID) (*model.SocialAccounts, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, err
	}

	socials, err := api.queries.GetSocialsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := &model.SocialAccounts{}

	for prov, social := range socials {
		if !social.Display {
			continue
		}
		assignSocialToModel(ctx, prov, social, result)

	}

	return result, nil
}

func assignSocialToModel(ctx context.Context, prov persist.SocialProvider, social persist.SocialUserIdentifiers, result *model.SocialAccounts) {
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
		scope, ok := social.Metadata["scope"].(string)
		if ok {
			t.Scope = scope
		}
		result.Twitter = t
	case persist.SocialProviderFarcaster:
		logger.For(ctx).Infof("found farcaster social account: %+v", social)
		f := &model.FarcasterSocialAccount{
			Type:     prov,
			Display:  social.Display,
			SocialID: social.ID,
		}
		name, ok := social.Metadata["name"].(string)
		if ok {
			f.Name = name
		}
		username, ok := social.Metadata["username"].(string)
		if ok {
			f.Username = username
		}
		profile, ok := social.Metadata["profile_image_url"].(string)
		if ok {
			f.ProfileImageURL = profile
		}
		bio, ok := social.Metadata["bio"].(string)
		if ok {
			f.Bio = bio
		}
		approvalURL, ok := social.Metadata["approval_url"].(string)
		if ok {
			f.ApprovalURL = &approvalURL
		}
		signerStatus, ok := social.Metadata["signer_status"].(string)
		if ok {
			f.SignerStatus = &signerStatus
		}
		result.Farcaster = f
	case persist.SocialProviderLens:
		logger.For(ctx).Infof("found lens social account: %+v", social)
		l := &model.LensSocialAccount{
			Type:     prov,
			Display:  social.Display,
			SocialID: social.ID,
		}
		name, ok := social.Metadata["name"].(string)
		if ok {
			l.Name = name
		}
		username, ok := social.Metadata["username"].(string)
		if ok {
			l.Username = username
		}
		profile, ok := social.Metadata["profile_image_url"].(string)
		if ok {
			l.ProfileImageURL = profile
		}
		bio, ok := social.Metadata["bio"].(string)
		if ok {
			l.Bio = bio
		}
		signtatureApproved, ok := social.Metadata["signature_approved"].(bool)
		if ok {
			l.SignatureApproved = signtatureApproved
		}
		result.Lens = l
	default:
		logger.For(ctx).Errorf("unknown social provider %s", prov)
	}
}

func (api UserAPI) UpdateUserSocialDisplayed(ctx context.Context, socialType persist.SocialProvider, displayed bool) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"socialType": validate.WithTag(socialType, "required"),
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
		"experienceType": validate.WithTag(experienceType, "required"),
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

// GetOnboardingUserRecommendationsBootstrap returns a function that when called returns a list of recommended users.
func GetOnboardingUserRecommendationsBootstrap(q *db.Queries) func(ctx context.Context) ([]persist.DBID, error) {
	return func(ctx context.Context) ([]persist.DBID, error) {
		return newDBIDCache(redis.SocialCache, "onboarding_user_recommendations", 24*time.Hour, func(ctx context.Context) ([]persist.DBID, error) {
			users, err := q.GetOnboardingUserRecommendations(ctx, 100)
			return util.MapWithoutError(users, func(u db.User) persist.DBID { return u.ID }), err
		}).Load(ctx)
	}
}

func (api UserAPI) GetSuggestedUsers(ctx context.Context, before, after *string, first, last *int) ([]db.User, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	viewerID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	var paginator positionPaginator[db.User]

	// If we have a cursor, we can page through the original set of recommended users
	if before != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *before)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *after)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else {
		// Otherwise make a new recommendation
		userIDs, err := GetOnboardingUserRecommendationsBootstrap(api.queries)(ctx)
		if err != nil {
			return nil, PageInfo{}, err
		}

		follows, err := api.queries.GetFollowEdgesByUserID(ctx, viewerID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		// Make personalized recommendations
		if len(follows) > 0 {
			personalizedIDs, err := recommend.For(ctx).RecommendFromFollowing(ctx, viewerID, follows)
			if err != nil {
				return nil, PageInfo{}, err
			}
			freq := make(map[persist.DBID]int)
			for _, id := range personalizedIDs {
				freq[id] += 2
			}
			for _, id := range userIDs {
				freq[id] += 1
			}
			userIDs = append(userIDs, personalizedIDs...)
			userIDs = util.Dedupe(userIDs, true)
			sort.Slice(userIDs, func(i, j int) bool { return freq[userIDs[i]] > freq[userIDs[j]] })
		}

		users, err := api.loaders.GetUsersByPositionPersonalizedBatch.Load(db.GetUsersByPositionPersonalizedBatchParams{
			ViewerID: viewerID,
			UserIds:  util.MapWithoutError(userIDs, func(id persist.DBID) string { return id.String() }),
		})
		if err != nil {
			return nil, PageInfo{}, err
		}

		recommend.Shuffle(users, 8)

		cursorPositions := make(map[persist.DBID]int64, len(users))
		cursorIDs := make([]persist.DBID, len(users))
		for i, u := range users {
			cursorPositions[u.ID] = int64(i)
			cursorIDs[i] = u.ID
		}

		cursor := cursors.NewPositionCursor()
		cursor.IDs = cursorIDs
		cursor.Positions = cursorPositions
		paginator = api.paginatorFromResults(ctx, cursor, users)
	}

	return paginator.paginate(before, after, first, last)
}

func (api UserAPI) GetSuggestedUsersFarcaster(ctx context.Context, before, after *string, first, last *int) ([]db.User, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	viewerID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	var paginator positionPaginator[db.User]

	// If we have a cursor, we can page through the original set of recommended users
	if before != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *before)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *after)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else {
		// Otherwise make a new recommendation
		fUsers, err := For(ctx).Social.GetFarcastingFollowingByUserID(ctx, viewerID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		connectionRank, err := api.queries.GetFarcasterConnections(ctx, db.GetFarcasterConnectionsParams{
			Fids:   util.MapWithoutError(fUsers, func(u farcaster.NeynarUser) string { return u.Fid.String() }),
			UserID: viewerID,
		})
		if err != nil {
			return nil, PageInfo{}, err
		}

		recommend.Shuffle(connectionRank, 8)

		cursorPositions := make(map[persist.DBID]int64, len(connectionRank))
		cursorIDs := make([]persist.DBID, len(connectionRank))
		users := make([]db.User, len(connectionRank))

		for i, u := range connectionRank {
			cursorPositions[u.User.ID] = int64(i)
			cursorIDs[i] = u.User.ID
			users[i] = u.User
		}

		cursor := cursors.NewPositionCursor()
		cursor.IDs = cursorIDs
		cursor.Positions = cursorPositions
		paginator = api.paginatorFromResults(ctx, cursor, users)
	}

	return paginator.paginate(before, after, first, last)
}

// CreatePushTokenForUser adds a push token to a user, or returns the existing push token if it's already been
// added to this user. If the token can't be added because it belongs to another user, an error is returned.
func (api UserAPI) CreatePushTokenForUser(ctx context.Context, pushToken string) (db.PushNotificationToken, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"pushToken": validate.WithTag(pushToken, "required,min=1,max=255"),
	}); err != nil {
		return db.PushNotificationToken{}, err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return db.PushNotificationToken{}, err
	}

	// Does the token already exist?
	token, err := api.queries.GetPushTokenByPushToken(ctx, pushToken)

	if err == nil {
		// If the token exists and belongs to the current user, return it. Attempting to re-add
		// a token that you've already registered is a no-op.
		if token.UserID == userID {
			return token, nil
		}

		// Otherwise, the token belongs to another user. Return an error.
		return db.PushNotificationToken{}, persist.ErrPushTokenBelongsToAnotherUser{PushToken: pushToken}
	}

	// ErrNoRows is expected and means we can continue with creating the token. If we see any other
	// error, return it.
	if err != pgx.ErrNoRows {
		return db.PushNotificationToken{}, err
	}

	token, err = api.queries.CreatePushTokenForUser(ctx, db.CreatePushTokenForUserParams{
		ID:        persist.GenerateID(),
		UserID:    userID,
		PushToken: pushToken,
	})

	if err != nil {
		return db.PushNotificationToken{}, err
	}

	return token, nil
}

// DeletePushTokenByPushToken removes a push token from a user, or does nothing if the token doesn't exist.
// If the token can't be removed because it belongs to another user, an error is returned.
func (api UserAPI) DeletePushTokenByPushToken(ctx context.Context, pushToken string) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"pushToken": validate.WithTag(pushToken, "required,min=1,max=255"),
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	existingToken, err := api.queries.GetPushTokenByPushToken(ctx, pushToken)
	if err == nil {
		// If the token exists and belongs to the current user, let them delete it.
		if existingToken.UserID == userID {
			return api.queries.DeletePushTokensByIDs(ctx, []persist.DBID{existingToken.ID})
		}

		// Otherwise, the token belongs to another user. Return an error.
		return persist.ErrPushTokenBelongsToAnotherUser{PushToken: pushToken}
	}

	// ErrNoRows is okay and means the token doesn't exist. Unregistering it is a no-op
	// and doesn't return an error.
	if err == pgx.ErrNoRows {
		return nil
	}

	return err
}

// SetProfileImage sets the profile image for the current user.
func (api UserAPI) SetProfileImage(ctx context.Context, tokenID *persist.DBID, walletAddress *persist.ChainAddress) error {
	// Validate
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	// Too many inputs provided
	if tokenID != nil && walletAddress != nil {
		return ErrProfileImageTooManySources
	}

	// Set the profile image to reference a token
	if tokenID != nil {
		t, err := For(ctx).Token.GetTokenById(ctx, *tokenID)
		if err != nil {
			return err
		}

		if t.OwnerUserID != userID || (!t.IsHolderToken && !t.IsCreatorToken) {
			return ErrProfileImageNotTokenOwner
		}

		return api.queries.SetProfileImageToToken(ctx, db.SetProfileImageToTokenParams{
			TokenSourceType: persist.ProfileImageSourceToken,
			ProfileID:       persist.GenerateID(),
			UserID:          userID,
			TokenID:         t.ID,
		})
	}

	// Set the profile image to reference an ENS avatar
	if walletAddress != nil {
		// Validate
		if err := validate.ValidateFields(api.validator, validate.ValidationMap{
			"chain":   validate.WithTag(walletAddress.Chain(), fmt.Sprintf("eq=%d", persist.ChainETH)),
			"address": validate.WithTag(walletAddress.Address(), "required"),
		}); err != nil {
			return err
		}

		wallets, err := api.loaders.GetWalletsByUserIDBatch.Load(userID)
		if err != nil {
			return err
		}

		for _, w := range wallets {
			if w.Chain == walletAddress.Chain() && w.Address == walletAddress.Address() {
				// Found the wallet
				addr := persist.EthereumAddress(w.Address)

				r, domain, err := eth.EnsAvatarRecordFor(ctx, api.ethClient, addr)
				if err != nil {
					return err
				}

				// Confirm wallet owns the token
				if t, ok := r.(eth.EnsTokenRecord); ok {
					isOwner, err := eth.IsOwner(ctx, api.ethClient, addr, t)
					if err != nil {
						return err
					}
					if !isOwner {
						return ErrProfileImageNotTokenOwner
					}
				}

				uri, err := uriFromRecord(ctx, api.multichainProvider, r)
				if err != nil {
					// Couldn't parse the URI, but tokenprocessing may support it so don't error out
					if errors.Is(err, eth.ErrUnknownEnsAvatarURI) {
						uri = ""
					} else {
						return err
					}
				}

				pfp, err := api.queries.SetProfileImageToENS(ctx, db.SetProfileImageToENSParams{
					EnsSourceType: persist.ProfileImageSourceENS,
					ProfileID:     persist.GenerateID(),
					UserID:        userID,
					WalletID:      w.ID,
					EnsAvatarUri:  util.ToNullString(uri, true),
					EnsDomain:     util.ToNullString(domain, true),
				})
				if err != nil {
					return err
				}

				// Manually prime the PFP loader
				api.loaders.GetProfileImageByIdBatch.Prime(db.GetProfileImageByIdBatchParams{
					ID:              pfp.ProfileImage.ID,
					EnsSourceType:   persist.ProfileImageSourceENS,
					TokenSourceType: persist.ProfileImageSourceToken,
				}, pfp.ProfileImage)
				return nil
			}
		}
		return ErrProfileImageNotWalletOwner
	}

	return ErrProfileImageUnknownSource
}

func (api UserAPI) RemoveProfileImage(ctx context.Context) error {
	// Validate
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}
	return api.queries.RemoveProfileImage(ctx, userID)
}

func (api UserAPI) GetProfileImageByUserID(ctx context.Context, userID persist.DBID) (db.ProfileImage, error) {
	// Validate
	user, err := api.GetUserById(ctx, userID)
	if err != nil {
		return db.ProfileImage{}, err
	}
	if user.ProfileImageID == "" {
		return db.ProfileImage{}, nil
	}
	return api.loaders.GetProfileImageByIdBatch.Load(db.GetProfileImageByIdBatchParams{
		ID:              user.ProfileImageID,
		TokenSourceType: persist.ProfileImageSourceToken,
		EnsSourceType:   persist.ProfileImageSourceENS,
	})
}

type EnsAvatar struct {
	WalletID persist.DBID
	Domain   string
	URI      string
}

// GetPotentialENSProfileImageByUserID returns the an ENS profile image for a user based on their set of wallets
func (api UserAPI) GetPotentialENSProfileImageByUserID(ctx context.Context, userID persist.DBID) (a EnsAvatar, err error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return a, err
	}

	// Check if profile images have been processed
	pfp, err := api.queries.GetPotentialENSProfileImageByUserId(ctx, db.GetPotentialENSProfileImageByUserIdParams{
		EnsAddress: eth.EnsAddress,
		Chain:      persist.ChainETH,
		UserID:     userID,
	})
	if err == nil {
		// Validate that the name is valid
		domain, err := eth.NormalizeDomain(pfp.TokenDefinition.Name.String)
		if err == nil {
			return EnsAvatar{
				WalletID: pfp.Wallet.ID,
				Domain:   domain,
				URI:      string(pfp.TokenMedia.Media.ProfileImageURL),
			}, nil
		}
	}

	// Otherwise check for ENS profile images
	wallets, err := api.queries.GetEthereumWalletsForEnsProfileImagesByUserID(ctx, userID)
	if err != nil {
		return a, err
	}

	errs := make([]error, 0)

	for _, w := range wallets {
		addr := persist.EthereumAddress(w.Address)

		r, domain, err := eth.EnsAvatarRecordFor(ctx, api.ethClient, addr)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if r == nil {
			continue
		}

		// Confirm wallet owns the token
		if t, ok := r.(eth.EnsTokenRecord); ok {
			isOwner, err := eth.IsOwner(ctx, api.ethClient, addr, t)
			if err != nil {
				return a, err
			}
			if !isOwner {
				continue
			}
		}

		uri, err := uriFromRecord(ctx, api.multichainProvider, r)
		if err != nil {
			return a, err
		}

		return EnsAvatar{WalletID: w.ID, Domain: domain, URI: uri}, nil
	}

	if len(errs) > 0 {
		return a, errs[0]
	}

	return a, nil
}

func (api UserAPI) IsMemberOfCommunity(ctx context.Context, userID persist.DBID, communityID persist.DBID) (bool, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID":      validate.WithTag(userID, "required"),
		"communityID": validate.WithTag(communityID, "required"),
	}); err != nil {
		return false, err
	}
	return api.queries.IsMemberOfCommunity(ctx, db.IsMemberOfCommunityParams{
		UserID:      userID,
		CommunityID: communityID,
	})
}

func (api UserAPI) BlockUser(ctx context.Context, userID persist.DBID) error {
	// Validate
	viewerID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, fmt.Sprintf("required,ne=%s", viewerID)),
	}); err != nil {
		return err
	}
	_, err = api.queries.BlockUser(ctx, db.BlockUserParams{
		ID:            persist.GenerateID(),
		UserID:        viewerID,
		BlockedUserID: userID,
	})
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return persist.ErrUserNotFound{UserID: userID}
	}
	return err
}

func (api UserAPI) UnblockUser(ctx context.Context, userID persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return err
	}
	viewerID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}
	return api.queries.UnblockUser(ctx, db.UnblockUserParams{UserID: viewerID, BlockedUserID: userID})
}

func (api UserAPI) SetPersona(ctx context.Context, persona persist.Persona) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"persona": validate.WithTag(persona, "required,persona"),
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	err = api.queries.SetPersonaByUserID(ctx, db.SetPersonaByUserIDParams{
		UserID:  userID,
		Persona: persona,
	})

	if err != nil {
		return err
	}

	return nil
}

func uriFromRecord(ctx context.Context, mc *multichain.Provider, r eth.AvatarRecord) (uri string, err error) {
	switch u := r.(type) {
	case nil:
		return "", nil
	case eth.EnsHttpRecord:
		return standardizeURI(u.URL), nil
	case eth.EnsIpfsRecord:
		return standardizeURI(u.URL), nil
	case eth.EnsTokenRecord:
		uri, err = uriFromEnsTokenRecord(ctx, mc, u)
		return standardizeURI(uri), err
	default:
		return "", eth.ErrUnknownEnsAvatarURI
	}
}

func uriFromEnsTokenRecord(ctx context.Context, mc *multichain.Provider, r eth.EnsTokenRecord) (string, error) {
	chain, contractAddr, _, tokenID, err := eth.TokenInfoFor(r)
	if err != nil {
		return "", err
	}

	// Fetch the metadata and return the appropriate profile image source
	metadata, err := mc.GetTokenMetadataByTokenIdentifiers(ctx, contractAddr, tokenID, chain)
	if err != nil {
		return "", err
	}

	imageURL, _, err := media.FindMediaURLsChain(metadata, chain)
	if err != nil {
		if errors.Is(err, media.ErrNoMediaURLs) {
			return "", nil
		}
		return "", err
	}

	return standardizeURI(string(imageURL)), nil
}

func standardizeURI(u string) string {
	if strings.HasPrefix(u, "ipfs://") {
		return ipfs.DefaultGatewayFrom(u)
	}
	return u
}

func createNewUserParamsWithAuth(ctx context.Context, authenticator auth.Authenticator, username string, bio string, email *persist.Email) (persist.CreateUserInput, error) {
	authResult, err := authenticator.Authenticate(ctx)
	if err != nil && !util.ErrorIs[persist.ErrUserNotFound](err) {
		return persist.CreateUserInput{}, auth.ErrAuthenticationFailed{WrappedErr: err}
	}

	if authResult.User != nil && !authResult.User.Universal.Bool() {
		if _, ok := authenticator.(auth.MagicLinkAuthenticator); ok {
			// TODO: We currently only use MagicLink for email, but we may use it for other login methods like SMS later,
			// so this error may not always be applicable in the future.
			return persist.CreateUserInput{}, auth.ErrEmailAlreadyUsed
		}
		return persist.CreateUserInput{}, persist.ErrUserAlreadyExists{Authenticator: authenticator.GetDescription()}
	}

	var wallet auth.AuthenticatedAddress

	if len(authResult.Addresses) > 0 {
		// TODO: This currently takes the first authenticated address returned by the authenticator and creates
		// the user's account based on that address. This works because the only auth mechanism we have is nonce-based
		// auth and that supplies a single address. In the future, a user may authenticate in a way that makes
		// multiple authenticated addresses available for initial user creation, and we may want to add all of
		// those addresses to the user's account here.
		wallet = authResult.Addresses[0]
	}

	params := persist.CreateUserInput{
		Username:     username,
		Bio:          bio,
		Email:        email,
		EmailStatus:  persist.EmailVerificationStatusUnverified,
		ChainAddress: wallet.ChainAddress,
		WalletType:   wallet.WalletType,
		PrivyDID:     authResult.PrivyDID,
	}

	// Override input email with verified email if available
	if authResult.Email != nil {
		params.Email = authResult.Email
		params.EmailStatus = persist.EmailVerificationStatusVerified
	}

	return params, nil
}

func createNewUserGalleryParams(galleryName, galleryDesc, galleryPos string) (params db.GalleryRepoCreateParams, err error) {
	params = db.GalleryRepoCreateParams{
		GalleryID:   persist.GenerateID(),
		Name:        galleryName,
		Description: galleryDesc,
		Position:    galleryPos,
	}

	if params.Position == "" {
		params.Position, err = fracdex.KeyBetween("", "")
		if err != nil {
			return db.GalleryRepoCreateParams{}, err
		}
	}

	return params, nil
}
