//go:build norec

package recommend

import (
	"context"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

func NewRecommender(*db.Queries) *Recommender                         { return &Recommender{} }
func (r *Recommender) Loop(context.Context, *time.Ticker)             {}
func (r *Recommender) readMetadata(ctx context.Context) graphMetadata { panic("not implemented") }

func (r *Recommender) RecommendFromFollowing(context.Context, persist.DBID, []db.Follow) ([]persist.DBID, error) {
	return []persist.DBID{}, nil
}

func (r *Recommender) RecommendFromFollowingShuffled(context.Context, persist.DBID, []db.Follow) ([]persist.DBID, error) {
	return []persist.DBID{}, nil
}

func (r *Recommender) readNeighbors(context.Context, persist.DBID) []persist.DBID {
	return []persist.DBID{}
}
