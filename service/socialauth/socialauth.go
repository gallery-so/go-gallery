package socialauth

import (
	"context"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/twitter"
)

type SocialAuthResult struct {
	ID persist.SocialUserIdentifers
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
		ID: ids,
	}, nil
}
