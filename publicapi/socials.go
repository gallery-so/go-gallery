package publicapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/socialauth"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/twitter"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type SocialAPI struct {
	repos      *postgres.Repositories
	redis      *redis.Cache
	queries    *db.Queries
	loaders    *dataloader.Loaders
	validator  *validator.Validate
	httpClient *http.Client
	taskClient *task.Client
	neynarAPI  *farcaster.NeynarAPI
}

func (s SocialAPI) NewTwitterAuthenticator(userID persist.DBID, authCode string) *socialauth.TwitterAuthenticator {
	return &socialauth.TwitterAuthenticator{
		AuthCode: authCode,
		UserID:   userID,
		Queries:  s.queries,
		Redis:    s.redis,
	}
}

func (s SocialAPI) NewFarcasterAuthenticator(userID persist.DBID, address persist.Address, withSigner bool) *socialauth.FarcasterAuthenticator {
	return &socialauth.FarcasterAuthenticator{
		HTTPClient: s.httpClient,
		UserID:     userID,
		Queries:    s.queries,
		Address:    address,
		WithSigner: withSigner,
		TaskClient: s.taskClient,
	}
}

func (s SocialAPI) NewLensAuthenticator(userID persist.DBID, address persist.Address, sig string) *socialauth.LensAuthenticator {
	return &socialauth.LensAuthenticator{
		HTTPClient: s.httpClient,
		UserID:     userID,
		Queries:    s.queries,
		Address:    address,
		Signature:  sig,
	}
}

func (api SocialAPI) GetFarcastingFollowingByUserID(ctx context.Context, userID persist.DBID) ([]farcaster.NeynarUser, error) {
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

	// if no rows found, might not be cached yet
	wallets, err := For(ctx).Wallet.GetWalletsByUserID(ctx, userID)

	// check if no wallets
	if err != nil {
		return nil, err
	}

	f, ok := socials[persist.SocialProviderFarcaster]
	if !ok {
		return []farcaster.NeynarUser{}, nil
	}
	fUsers, err := api.neynarAPI.FollowingByUserID(ctx, f.ID)
	return fUsers, err
}

func (api SocialAPI) GetConnectionsPaginate(ctx context.Context, socialProvider persist.SocialProvider, before, after *string, first, last *int, onlyUnfollowing *bool) ([]model.SocialConnection, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"socialProvider": validate.WithTag(socialProvider, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	ouf := false
	if onlyUnfollowing != nil {
		ouf = *onlyUnfollowing
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	var initialConnections []model.SocialConnection
	var socialIDs []string

	switch socialProvider {
	case persist.SocialProviderTwitter:
		tapi, err := api.newTwitterAPIForUser(ctx, userID)
		if err != nil {
			return nil, PageInfo{}, err
		}
		following, err := tapi.GetFollowing(ctx)
		if err != nil {
			return nil, PageInfo{}, err
		}

		initialConnections, _ = util.Map(following, func(t twitter.TwitterIdentifiers) (model.SocialConnection, error) {
			return model.SocialConnection{
				SocialID:       t.ID,
				SocialType:     persist.SocialProviderTwitter,
				DisplayName:    t.Name,
				SocialUsername: t.Username,
				ProfileImage:   t.ProfileImageURL,
			}, nil
		})
		socialIDs, _ = util.Map(following, func(t twitter.TwitterIdentifiers) (string, error) {
			return t.ID, nil
		})
	case persist.SocialProviderFarcaster:
		farcasterFollowing, err := api.GetFarcastingFollowingByUserID(ctx, userID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		initialConnections, _ = util.Map(farcasterFollowing, func(f farcaster.NeynarUser) (model.SocialConnection, error) {
			return model.SocialConnection{
				SocialID:       f.Fid.String(),
				SocialType:     persist.SocialProviderFarcaster,
				DisplayName:    f.DisplayName,
				SocialUsername: f.Username,
				ProfileImage:   f.Pfp.URL,
			}, nil
		})

	default:
		return nil, PageInfo{}, fmt.Errorf("unsupported social provider: %s", socialProvider)
	}

	queryFunc := func(params boolTimeIDPagingParams) ([]model.SocialConnection, error) {
		usernames, _ := util.Map(initialConnections, func(m model.SocialConnection) (string, error) {
			return m.SocialUsername, nil
		})
		displaynames, _ := util.Map(initialConnections, func(m model.SocialConnection) (string, error) {
			return m.DisplayName, nil
		})
		profileImages, _ := util.Map(initialConnections, func(m model.SocialConnection) (string, error) {
			return m.ProfileImage, nil
		})
		results, err := api.queries.GetSocialConnectionsPaginate(ctx, db.GetSocialConnectionsPaginateParams{
			Limit:               params.Limit,
			UserID:              userID,
			SocialIds:           socialIDs,
			SocialUsernames:     usernames,
			SocialDisplaynames:  displaynames,
			Social:              socialProvider.String(),
			SocialProfileImages: profileImages,
			OnlyUnfollowing:     ouf,
			CurBeforeFollowing:  params.CursorBeforeBool,
			CurBeforeTime:       params.CursorBeforeTime,
			CurBeforeID:         params.CursorBeforeID,
			CurAfterFollowing:   params.CursorAfterBool,
			CurAfterTime:        params.CursorAfterTime,
			CurAfterID:          params.CursorAfterID,
			PagingForward:       params.PagingForward,
		})
		if err != nil {
			return nil, fmt.Errorf("error getting social connections: %w", err)
		}
		return util.Map(results, func(r db.GetSocialConnectionsPaginateRow) (model.SocialConnection, error) {
			m := model.SocialConnection{
				GalleryUser:        &model.GalleryUser{Dbid: r.UserID},
				CurrentlyFollowing: r.AlreadyFollowing,
				SocialType:         socialProvider,
				SocialID:           r.SocialID.(string),
				DisplayName:        r.SocialDisplayname.(string),
				SocialUsername:     r.SocialUsername.(string),
				ProfileImage:       r.SocialProfileImage.(string),
				HelperSocialConnectionData: model.HelperSocialConnectionData{
					UserID:        r.UserID,
					UserCreatedAt: r.UserCreatedAt,
				},
			}
			return m, nil
		})
	}

	countFunc := func() (int, error) {

		c, err := api.queries.CountSocialConnections(ctx, db.CountSocialConnectionsParams{
			SocialIds:       socialIDs,
			Social:          socialProvider.String(),
			OnlyUnfollowing: ouf,
			UserID:          userID,
		})
		if err != nil {
			return 0, fmt.Errorf("error counting social connections: %w", err)
		}
		return int(c), nil
	}

	cursorFunc := func(c model.SocialConnection) (bool, time.Time, persist.DBID, error) {
		return c.CurrentlyFollowing, c.UserCreatedAt, c.GalleryUser.Dbid, nil
	}

	paginator := boolTimeIDPaginator[model.SocialConnection]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api SocialAPI) GetConnections(ctx context.Context, socialProvider persist.SocialProvider, onlyUnfollowing *bool) ([]model.SocialConnection, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"socialProvider": validate.WithTag(socialProvider, "required"),
	}); err != nil {
		return nil, err
	}

	ouf := false
	if onlyUnfollowing != nil {
		ouf = *onlyUnfollowing
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, err
	}

	var initialConnections []model.SocialConnection
	var socialIDs []string

	switch socialProvider {
	case persist.SocialProviderTwitter:
		tapi, err := api.newTwitterAPIForUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		following, err := tapi.GetFollowing(ctx)
		if err != nil {
			return nil, err
		}

		initialConnections, _ = util.Map(following, func(t twitter.TwitterIdentifiers) (model.SocialConnection, error) {
			return model.SocialConnection{
				SocialID:       t.ID,
				SocialType:     persist.SocialProviderTwitter,
				DisplayName:    t.Name,
				SocialUsername: t.Username,
				ProfileImage:   t.ProfileImageURL,
			}, nil
		})
		socialIDs, _ = util.Map(following, func(t twitter.TwitterIdentifiers) (string, error) {
			return t.ID, nil
		})

		usernames, _ := util.Map(initialConnections, func(m model.SocialConnection) (string, error) {
			return m.SocialUsername, nil
		})
		displaynames, _ := util.Map(initialConnections, func(m model.SocialConnection) (string, error) {
			return m.DisplayName, nil
		})
		profileImages, _ := util.Map(initialConnections, func(m model.SocialConnection) (string, error) {
			return m.ProfileImage, nil
		})
		results, err := api.queries.GetSocialConnections(ctx, db.GetSocialConnectionsParams{

			UserID:              userID,
			SocialIds:           socialIDs,
			SocialUsernames:     usernames,
			SocialDisplaynames:  displaynames,
			Social:              socialProvider.String(),
			SocialProfileImages: profileImages,
			OnlyUnfollowing:     ouf,
		})
		if err != nil {
			return nil, err
		}
		return util.Map(results, func(r db.GetSocialConnectionsRow) (model.SocialConnection, error) {
			m := model.SocialConnection{
				GalleryUser:        &model.GalleryUser{Dbid: r.UserID},
				CurrentlyFollowing: r.AlreadyFollowing,
				SocialType:         socialProvider,
				SocialID:           r.SocialID.(string),
				DisplayName:        r.SocialDisplayname.(string),
				SocialUsername:     r.SocialUsername.(string),
				ProfileImage:       r.SocialProfileImage.(string),
				HelperSocialConnectionData: model.HelperSocialConnectionData{
					UserID:        r.UserID,
					UserCreatedAt: r.UserCreatedAt,
				},
			}
			return m, nil
		})
	default:
		return nil, fmt.Errorf("unsupported social provider: %s", socialProvider)
	}
}

func (api SocialAPI) newTwitterAPIForUser(ctx context.Context, userID persist.DBID) (*twitter.API, error) {
	socialAuth, err := api.queries.GetSocialAuthByUserID(ctx, db.GetSocialAuthByUserIDParams{UserID: userID, Provider: persist.SocialProviderTwitter})
	if err != nil {
		return nil, fmt.Errorf("error getting social auth: %w", err)
	}

	tapi, newSocials, err := twitter.NewAPI(api.queries, api.redis).WithAuth(ctx, socialAuth.AccessToken.String, socialAuth.RefreshToken.String)
	if newSocials != nil {
		err = api.queries.UpsertSocialOAuth(ctx, db.UpsertSocialOAuthParams{
			ID:           persist.GenerateID(),
			UserID:       userID,
			Provider:     persist.SocialProviderTwitter,
			AccessToken:  util.ToNullString(newSocials.AccessToken, false),
			RefreshToken: util.ToNullString(newSocials.RefreshToken, false),
		})
		if err != nil {
			return nil, fmt.Errorf("error updating social auth: %w", err)
		}
	}
	if err != nil {
		return nil, err
	}

	return tapi, nil
}

func (s *SocialAPI) DisconnectSocialAccount(ctx context.Context, socialType persist.SocialProvider) error {
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}
	return s.queries.RemoveSocialFromUser(ctx, db.RemoveSocialFromUserParams{
		Social: socialType.String(),
		UserID: userID,
	})
}
