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
	"github.com/mikeydub/go-gallery/service/socialauth"
	"github.com/mikeydub/go-gallery/service/twitter"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type SocialAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
}

func (s SocialAPI) NewTwitterAuthenticator(userID persist.DBID, authCode string) *socialauth.TwitterAuthenticator {
	return &socialauth.TwitterAuthenticator{
		AuthCode: authCode,
		UserID:   userID,
		Queries:  s.queries,
	}
}

func (api SocialAPI) GetConnectionsPaginate(ctx context.Context, socialProvider persist.SocialProvider, before, after *string, first, last *int, onlyUnfollowing *bool) ([]db.Token, PageInfo, error) {
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

		initialConnections, err = util.Map(following, func(t twitter.TwitterIdentifiers) (model.SocialConnection, error) {
			return model.SocialConnection{
				SocialID:       t.ID,
				SocialType:     persist.SocialProviderTwitter,
				DisplayName:    t.Name,
				SocialUsername: t.Username,
				ProfileImage:   t.ProfileImageURL,
			}, nil
		})
		if err != nil {
			return nil, PageInfo{}, err
		}
	default:
		return nil, PageInfo{}, fmt.Errorf("unsupported social provider: %s", socialProvider)
	}

	queryFunc := func(params boolTimeIDPagingParams) ([]interface{}, error) {

		return nil, nil
	}

	countFunc := func() (int, error) {

		return 0, err
	}

	cursorFunc := func(i interface{}) (bool, time.Time, persist.DBID, error) {

		return false, time.Time{}, "", fmt.Errorf("interface{} is not a token")
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

	return nil, pageInfo, nil
}

func (api SocialAPI) newTwitterAPIForUser(ctx context.Context, userID persist.DBID) (*twitter.API, error) {
	socialAuth, err := api.queries.GetSocialAuthByUserID(ctx, db.GetSocialAuthByUserIDParams{UserID: userID, Provider: persist.SocialProviderTwitter})
	if err != nil {
		return nil, err
	}

	tapi, newSocials, err := twitter.NewAPI(api.queries).WithAuth(ctx, socialAuth.AccessToken.String, socialAuth.RefreshToken.String)
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
