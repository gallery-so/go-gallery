package postgres

import (
	"context"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

type FeedBlocklistRepository struct {
	Queries *db.Queries
}

func (r *FeedBlocklistRepository) IsBlocked(ctx context.Context, userID persist.DBID, action persist.Action) (bool, error) {
	return r.Queries.IsFeedUserActionBlocked(ctx, db.IsFeedUserActionBlockedParams{
		UserID: userID,
		Action: action,
	})
}
