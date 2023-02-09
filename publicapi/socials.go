package publicapi

import (
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/socialauth"
)

type SocialAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
}

func (s *SocialAPI) NewTwitterAuthenticator(userID persist.DBID, authCode string) *socialauth.TwitterAuthenticator {
	return &socialauth.TwitterAuthenticator{
		AuthCode: authCode,
		UserID:   userID,
		Queries:  s.queries,
	}
}
