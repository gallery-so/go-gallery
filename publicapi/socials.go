package publicapi

import (
	"context"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/socialauth"
	"github.com/mikeydub/go-gallery/service/twitter"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type SocialAPI struct {
	repos     *postgres.Repositories
	redis     *redis.Cache
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
}

func (s SocialAPI) NewTwitterAuthenticator(userID persist.DBID, authCode string) *socialauth.TwitterAuthenticator {
	return &socialauth.TwitterAuthenticator{
		AuthCode: authCode,
		UserID:   userID,
		Queries:  s.queries,
		Redis:    s.redis,
	}
}

func (api SocialAPI) GetConnectionsPaginate(ctx context.Context, socialProvider persist.SocialProvider, before, after *string, first, last *int, onlyUnfollowing *bool) ([]model.SocialConnection, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"socialProvider": {socialProvider, "required"},
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
	default:
		return nil, PageInfo{}, fmt.Errorf("unsupported social provider: %s", socialProvider)
	}

	queryFunc := func(params boolTimeIDPagingParams) ([]interface{}, error) {
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
			Limit:               params.Limit,
			UserID:              userID,
			SocialIds:           socialIDs,
			SocialUsernames:     usernames,
			SocialDisplaynames:  displaynames,
			Column1:             socialProvider.String(),
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
			return nil, err
		}
		return util.Map(results, func(r db.GetSocialConnectionsRow) (interface{}, error) {
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
					UserCreatedAt: persist.CreationTime(r.UserCreatedAt),
				},
			}
			return m, nil
		})
	}

	countFunc := func() (int, error) {

		c, err := api.queries.CountSocialConnections(ctx, db.CountSocialConnectionsParams{
			SocialIds:       socialIDs,
			Column1:         socialProvider.String(),
			OnlyUnfollowing: ouf,
			UserID:          userID,
		})
		return int(c), err
	}

	cursorFunc := func(i interface{}) (bool, time.Time, persist.DBID, error) {
		if conn, ok := i.(model.SocialConnection); ok {
			return conn.CurrentlyFollowing, conn.UserCreatedAt.Time(), conn.GalleryUser.Dbid, nil
		}
		return false, time.Time{}, "", fmt.Errorf("interface{} is not a social connection")
	}

	paginator := boolTimeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	if err != nil {
		return nil, PageInfo{}, err
	}

	connections, _ := util.Map(results, func(i interface{}) (model.SocialConnection, error) {
		return i.(model.SocialConnection), nil
	})

	return connections, pageInfo, nil
}

func (api SocialAPI) newTwitterAPIForUser(ctx context.Context, userID persist.DBID) (*twitter.API, error) {
	socialAuth, err := api.queries.GetSocialAuthByUserID(ctx, db.GetSocialAuthByUserIDParams{UserID: userID, Provider: persist.SocialProviderTwitter})
	if err != nil {
		return nil, err
	}

	tapi, newSocials, err := twitter.NewAPI(api.queries, api.redis).WithAuth(ctx, socialAuth.AccessToken.String, socialAuth.RefreshToken.String)
	if err != nil {
		return nil, err
	}

	if newSocials != nil {
		err = api.queries.UpsertSocialOAuth(ctx, coredb.UpsertSocialOAuthParams{
			ID:           persist.GenerateID(),
			UserID:       userID,
			Provider:     persist.SocialProviderTwitter,
			AccessToken:  util.ToNullString(newSocials.AccessToken),
			RefreshToken: util.ToNullString(newSocials.RefreshToken),
		})
		if err != nil {
			return nil, err
		}
	}
	return tapi, nil
}
