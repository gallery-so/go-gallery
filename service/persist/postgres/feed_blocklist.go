package postgres

import (
	"context"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/persist"
)

type FeedBlocklistRepository struct {
	Queries *sqlc.Queries
}

func (r *FeedBlocklistRepository) IsBlocked(ctx context.Context, userID persist.DBID, action persist.Action) (bool, error) {
	return r.Queries.IsFeedUserActionBlocked(ctx, sqlc.IsFeedUserActionBlockedParams{
		UserID: userID,
		Action: action,
	})
}
