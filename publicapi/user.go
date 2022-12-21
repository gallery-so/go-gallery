package publicapi

import (
	"context"
	"fmt"
	"time"

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
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type UserAPI struct {
	LoggedInUserID                  func(context.Context) persist.DBID
	LoggedInUserWithPII             func(context.Context) (*db.UsersWithPii, error)
	IsUserLoggedIn                  func(context.Context) bool
	UserByID                        func(context.Context, persist.DBID) (*db.User, error)
	UserByUsername                  func(context.Context, string) (*db.User, error)
	UserByAddress                   func(context.Context, persist.ChainAddress) (*db.User, error)
	UsersWithTrait                  func(context.Context, string) ([]db.User, error)
	PaginateUsersWithIDs            func(ctx context.Context, userIDs []persist.DBID, before, after *string, first, last *int) ([]db.User, PageInfo, error)
	PaginateUsersWithRole           func(ctx context.Context, role persist.Role, before, after *string, first, last *int) ([]db.User, PageInfo, error)
	FollowersByUserID               func(context.Context, persist.DBID) ([]db.User, error)
	FollowingByUserID               func(context.Context, persist.DBID) ([]db.User, error)
	MembershipTiers                 func(ctx context.Context, forceRefresh bool) ([]persist.MembershipTier, error)
	MembershipByID                  func(context.Context, persist.DBID) (*db.Membership, error)
	AddWallet                       func(context.Context, persist.ChainAddress, auth.Authenticator) error
	RemoveWallets                   func(context.Context, []persist.DBID) error
	CreateUser                      func(ctx context.Context, authenticator auth.Authenticator, username string, email *persist.Email, bio string) (userID, galleryID persist.DBID, err error)
	UpdateUserInfo                  func(ctx context.Context, username, bio string) error
	UpdateEmail                     func(context.Context, persist.Email) error
	UpdateEmailNotificationSettings func(context.Context, persist.EmailUnsubscriptions) error
	UpdateNotificationSettings      func(context.Context, persist.UserNotificationSettings) error
	ResendEmailVerification         func(context.Context) error
	FollowUser                      func(context.Context, persist.DBID) error
	UnfollowUser                    func(context.Context, persist.DBID) error
}

func NewUserAPI(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) *UserAPI {
	userByIDFunc := userByID(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient)
	return &UserAPI{
		LoggedInUserID:                  loggedInUserID(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		LoggedInUserWithPII:             loggedInUserWithPII(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		IsUserLoggedIn:                  isUserLoggedIn(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		UserByID:                        userByIDFunc,
		UserByUsername:                  userByUsername(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		UserByAddress:                   userByAddress(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		UsersWithTrait:                  usersWithTrait(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		PaginateUsersWithIDs:            usersByIDs(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		PaginateUsersWithRole:           usersWithRole(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		FollowersByUserID:               followersByUserID(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient, userByIDFunc),
		FollowingByUserID:               followingByUserID(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient, userByIDFunc),
		MembershipTiers:                 membershipTiers(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		MembershipByID:                  membershipByID(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		AddWallet:                       addWallet(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		RemoveWallets:                   removeWallets(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		CreateUser:                      createUser(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		UpdateUserInfo:                  updateInfo(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		UpdateEmail:                     updateEmail(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		UpdateEmailNotificationSettings: updateEmailNotificationSettings(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		UpdateNotificationSettings:      updateNotificationSettings(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		ResendEmailVerification:         resendEmailVerification(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
		FollowUser:                      followUser(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient, userByIDFunc),
		UnfollowUser:                    unfollowUser(repos, queries, loaders, validator, ethClient, ipfsClient, arweaveClient, storageClient),
	}
}

func loggedInUserID(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context) persist.DBID {
	return func(ctx context.Context) persist.DBID {
		gc := util.GinContextFromContext(ctx)
		return auth.GetUserIDFromCtx(gc)
	}
}

func isUserLoggedIn(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context) bool {
	return func(ctx context.Context) bool {
		gc := util.GinContextFromContext(ctx)
		return auth.GetUserAuthedFromCtx(gc)
	}
}

func userByID(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, userID persist.DBID) (*db.User, error) {
	return func(ctx context.Context, userID persist.DBID) (*db.User, error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"userID": {userID, "required"},
		}); err != nil {
			return nil, err
		}

		user, err := loaders.UserByUserID.Load(userID)
		if err != nil {
			return nil, err
		}

		return &user, nil
	}
}

// loggedInUserWithPII returns the current user and their associated personally identifiable information
func loggedInUserWithPII(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context) (*db.UsersWithPii, error) {
	return func(ctx context.Context) (*db.UsersWithPii, error) {
		// Nothing to validate

		userID, err := getAuthenticatedUser(ctx)
		if err != nil {
			return nil, err
		}

		userWithPII, err := queries.GetUserWithPIIByID(ctx, userID)
		if err != nil {
			return nil, err
		}

		return &userWithPII, nil
	}
}

func usersByIDs(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, userIDs []persist.DBID, before, after *string, first, last *int) ([]db.User, PageInfo, error) {
	return func(ctx context.Context, userIDs []persist.DBID, before, after *string, first, last *int) ([]db.User, PageInfo, error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"userIDs": {userIDs, "required"},
		}); err != nil {
			return nil, PageInfo{}, err
		}

		if err := validatePaginationParams(validator, first, last); err != nil {
			return nil, PageInfo{}, err
		}

		queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
			users, err := queries.GetUsersByIDs(ctx, db.GetUsersByIDsParams{
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
}

func userByUsername(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, username string) (*db.User, error) {
	return func(ctx context.Context, username string) (*db.User, error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"username": {username, "required"},
		}); err != nil {
			return nil, err
		}

		user, err := loaders.UserByUsername.Load(username)
		if err != nil {
			return nil, err
		}

		return &user, nil
	}
}

func userByAddress(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, chainAddress persist.ChainAddress) (*db.User, error) {
	return func(ctx context.Context, chainAddress persist.ChainAddress) (*db.User, error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"chainAddress": {chainAddress, "required"},
		}); err != nil {
			return nil, err
		}

		chain := chainAddress.Chain()
		user, err := loaders.UserByAddress.Load(db.GetUserByAddressBatchParams{
			Chain:   int32(chain),
			Address: persist.Address(chain.NormalizeAddress(chainAddress.Address())),
		})
		if err != nil {
			return nil, err
		}

		return &user, nil
	}
}

func usersWithTrait(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, trait string) ([]db.User, error) {
	return func(ctx context.Context, trait string) ([]db.User, error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"trait": {trait, "required"},
		}); err != nil {
			return nil, err
		}

		users, err := loaders.UsersWithTrait.Load(trait)
		if err != nil {
			return nil, err
		}

		return users, nil
	}
}

func usersWithRole(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, role persist.Role, before *string, after *string, first *int, last *int) ([]db.User, PageInfo, error) {
	return func(ctx context.Context, role persist.Role, before *string, after *string, first *int, last *int) ([]db.User, PageInfo, error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"role": {role, "required,role"},
		}); err != nil {
			return nil, PageInfo{}, err
		}

		if err := validatePaginationParams(validator, first, last); err != nil {
			return nil, PageInfo{}, err
		}

		queryFunc := func(params lexicalPagingParams) ([]interface{}, error) {
			keys, err := queries.GetUsersWithRolePaginate(ctx, db.GetUsersWithRolePaginateParams{
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
}

func addWallet(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, chainAddress persist.ChainAddress, authenticator auth.Authenticator) error {
	return func(ctx context.Context, chainAddress persist.ChainAddress, authenticator auth.Authenticator) error {
		// Validate
		if err := validateFields(validator, validationMap{
			"chainAddress":  {chainAddress, "required"},
			"authenticator": {authenticator, "required"},
		}); err != nil {
			return err
		}

		userID, err := getAuthenticatedUser(ctx)
		if err != nil {
			return err
		}

		err = user.AddWalletToUser(ctx, userID, chainAddress, authenticator, repos.UserRepository, repos.WalletRepository)
		if err != nil {
			return err
		}

		return nil
	}
}

func removeWallets(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, walletIDs []persist.DBID) error {
	return func(ctx context.Context, walletIDs []persist.DBID) error {
		// Validate
		if err := validateFields(validator, validationMap{
			"walletIDs": {walletIDs, "required,unique,dive,required"},
		}); err != nil {
			return err
		}

		userID, err := getAuthenticatedUser(ctx)
		if err != nil {
			return err
		}

		err = user.RemoveWalletsFromUser(ctx, userID, walletIDs, repos.UserRepository)
		if err != nil {
			return err
		}

		return nil
	}
}

func createUser(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, authenticator auth.Authenticator, username string, email *persist.Email, bio string) (userID persist.DBID, galleryID persist.DBID, err error) {
	return func(ctx context.Context, authenticator auth.Authenticator, username string, email *persist.Email, bio string) (userID persist.DBID, galleryID persist.DBID, err error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"username": {username, "required,username"},
			"bio":      {bio, "bio"},
		}); err != nil {
			return "", "", err
		}

		userID, galleryID, err = user.CreateUser(ctx, authenticator, username, email, bio, repos.UserRepository, repos.GalleryRepository)
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
		}, validator, nil)
		if err != nil {
			return "", "", err
		}

		return userID, galleryID, err
	}
}

func updateInfo(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, username string, bio string) error {
	return func(ctx context.Context, username string, bio string) error {
		// Validate
		if err := validateFields(validator, validationMap{
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

		err = user.UpdateUserInfo(ctx, userID, username, bio, repos.UserRepository, ethClient)
		if err != nil {
			return err
		}

		return nil
	}
}

func updateEmail(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, email persist.Email) error {
	return func(ctx context.Context, email persist.Email) error {
		// Validate
		if err := validateFields(validator, validationMap{
			"email": {email, "required"},
		}); err != nil {
			return err
		}

		userID, err := getAuthenticatedUser(ctx)
		if err != nil {
			return err
		}
		err = queries.UpdateUserEmail(ctx, db.UpdateUserEmailParams{
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
}

func updateEmailNotificationSettings(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, settings persist.EmailUnsubscriptions) error {
	return func(ctx context.Context, settings persist.EmailUnsubscriptions) error {
		// Validate
		if err := validateFields(validator, validationMap{
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
}

func resendEmailVerification(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context) error {
	return func(ctx context.Context) error {

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
}

func updateNotificationSettings(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, notificationSettings persist.UserNotificationSettings) error {
	return func(ctx context.Context, notificationSettings persist.UserNotificationSettings) error {
		// Validate
		if err := validateFields(validator, validationMap{
			"notification_settings": {notificationSettings, "required"},
		}); err != nil {
			return err
		}

		userID, err := getAuthenticatedUser(ctx)
		if err != nil {
			return err
		}

		return queries.UpdateNotificationSettingsByID(ctx, db.UpdateNotificationSettingsByIDParams{ID: userID, NotificationSettings: notificationSettings})
	}
}

func membershipTiers(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, forceRefresh bool) ([]persist.MembershipTier, error) {
	return func(ctx context.Context, forceRefresh bool) ([]persist.MembershipTier, error) {
		// Nothing to validate
		return membership.GetMembershipTiers(ctx, forceRefresh, repos.MembershipRepository, repos.UserRepository, repos.GalleryRepository, repos.WalletRepository, ethClient, ipfsClient, arweaveClient, storageClient)
	}
}

func membershipByID(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, membershipID persist.DBID) (*db.Membership, error) {
	return func(ctx context.Context, membershipID persist.DBID) (*db.Membership, error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"membershipID": {membershipID, "required"},
		}); err != nil {
			return nil, err
		}

		membership, err := loaders.MembershipByMembershipID.Load(membershipID)
		if err != nil {
			return nil, err
		}

		return &membership, nil
	}
}

func followersByUserID(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, userByID func(context.Context, persist.DBID) (*db.User, error)) func(ctx context.Context, userID persist.DBID) ([]db.User, error) {
	return func(ctx context.Context, userID persist.DBID) ([]db.User, error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"userID": {userID, "required"},
		}); err != nil {
			return nil, err
		}

		if _, err := userByID(ctx, userID); err != nil {
			return nil, err
		}

		followers, err := loaders.FollowersByUserID.Load(userID)
		if err != nil {
			return nil, err
		}

		return followers, nil
	}
}

func followingByUserID(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, userByID func(context.Context, persist.DBID) (*db.User, error)) func(ctx context.Context, userID persist.DBID) ([]db.User, error) {
	return func(ctx context.Context, userID persist.DBID) ([]db.User, error) {
		// Validate
		if err := validateFields(validator, validationMap{
			"userID": {userID, "required"},
		}); err != nil {
			return nil, err
		}

		if _, err := userByID(ctx, userID); err != nil {
			return nil, err
		}

		following, err := loaders.FollowingByUserID.Load(userID)
		if err != nil {
			return nil, err
		}

		return following, nil
	}
}

func followUser(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, userByID func(context.Context, persist.DBID) (*db.User, error)) func(ctx context.Context, userID persist.DBID) error {
	return func(ctx context.Context, userID persist.DBID) error {
		// Validate
		curUserID, err := getAuthenticatedUser(ctx)
		if err != nil {
			return err
		}

		if err := validateFields(validator, validationMap{
			"userID": {userID, fmt.Sprintf("required,ne=%s", curUserID)},
		}); err != nil {
			return err
		}

		if _, err := userByID(ctx, userID); err != nil {
			return err
		}

		refollowed, err := repos.UserRepository.AddFollower(ctx, curUserID, userID)
		if err != nil {
			return err
		}

		// Send event
		go dispatchFollowEventToFeed(sentryutil.NewSentryHubGinContext(ctx), repos, curUserID, userID, refollowed)

		return nil
	}
}

func unfollowUser(repos *postgres.Repositories, queries *db.Queries, loaders *dataloader.Loaders, validator *validator.Validate, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) func(ctx context.Context, userID persist.DBID) error {
	return func(ctx context.Context, userID persist.DBID) error {
		// Validate
		if err := validateFields(validator, validationMap{
			"userID": {userID, "required"},
		}); err != nil {
			return err
		}

		curUserID, err := getAuthenticatedUser(ctx)
		if err != nil {
			return err
		}

		return repos.UserRepository.RemoveFollower(ctx, curUserID, userID)
	}
}

func dispatchFollowEventToFeed(ctx context.Context, repos *postgres.Repositories, curUserID persist.DBID, followedUserID persist.DBID, refollowed bool) {
	followedBack, err := repos.UserRepository.UserFollowsUser(ctx, followedUserID, curUserID)

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
