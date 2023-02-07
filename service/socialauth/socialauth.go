package socialauth

import (
	"context"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/twitter"
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

	UserID   persist.DBID
	AuthCode string
}

func (a TwitterAuthenticator) Authenticate(ctx context.Context) (*SocialAuthResult, error) {
	tAPI := twitter.NewAPI(a.Queries)

	ids, err := tAPI.GetAuthedUserFromCode(ctx, a.UserID, a.AuthCode)
	if err != nil {
		return nil, err
	}

	return &SocialAuthResult{
		Provider: persist.SocialProviderTwitter,
		ID:       ids.ID,
		Metadata: map[string]interface{}{
			"username": ids.Username,
			"name":     ids.Name,
		},
	}, nil
}
