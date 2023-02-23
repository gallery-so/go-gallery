package socialauth

import (
	"context"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/twitter"
	"github.com/mikeydub/go-gallery/util"
)

type SocialAuthResult struct {
	Provider persist.SocialProvider `json:"provider,required" binding:"required"`
	ID       string                 `json:"id,required" binding:"required"`
	Metadata map[string]interface{} `json:"metadata"`
}

type Authenticator interface {
	Authenticate(context.Context) (*SocialAuthResult, error)
}

type TwitterAuthenticator struct {
	Queries *coredb.Queries
	Redis   *redis.Cache

	UserID   persist.DBID
	AuthCode string
}

func (a TwitterAuthenticator) Authenticate(ctx context.Context) (*SocialAuthResult, error) {
	tAPI := twitter.NewAPI(a.Queries, a.Redis)

	ids, access, err := tAPI.GetAuthedUserFromCode(ctx, a.AuthCode)
	if err != nil {
		return nil, err
	}

	err = a.Queries.UpsertSocialOAuth(ctx, coredb.UpsertSocialOAuthParams{
		ID:           persist.GenerateID(),
		UserID:       a.UserID,
		Provider:     persist.SocialProviderTwitter,
		AccessToken:  util.ToNullString(access.AccessToken),
		RefreshToken: util.ToNullString(access.RefreshToken),
	})
	if err != nil {
		return nil, err
	}

	return &SocialAuthResult{
		Provider: persist.SocialProviderTwitter,
		ID:       ids.ID,
		Metadata: map[string]interface{}{
			"username":          ids.Username,
			"name":              ids.Name,
			"profile_image_url": ids.ProfileImageURL,
		},
	}, nil
}
