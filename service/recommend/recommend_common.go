package recommend

import (
	"context"

	"github.com/gin-gonic/gin"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const contextKey = "recommend.instance"

type saveMsg struct {
	nodeID         persist.DBID
	recommendedIDs []persist.DBID
}

type Recommender struct {
	RecommendFunc  func(context.Context, persist.DBID, []db.Follow) ([]persist.DBID, error)
	LoadFunc       func(context.Context)
	SaveResultFunc func(ctx context.Context, userID persist.DBID, recommendedIDs []persist.DBID) error
	BootstrapFunc  func(ctx context.Context) ([]persist.DBID, error)
	saveCh         chan saveMsg
}

func AddTo(c *gin.Context, r *Recommender) {
	c.Set(contextKey, r)
}

func For(ctx context.Context) *Recommender {
	gc := util.MustGetGinContext(ctx)
	return gc.Value(contextKey).(*Recommender)
}
