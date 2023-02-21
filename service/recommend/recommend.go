package recommend

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const contextKey = "recommend.instance"

var metadataKey = struct{}{}

var currentGraph sync.Map

func AddTo(c *gin.Context, r *Recommender) {
	c.Set(contextKey, r)
}

func For(ctx context.Context) *Recommender {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(contextKey).(*Recommender)
}

type Recommender struct {
	RecommendFunc  func(context.Context, persist.DBID, []db.Follow) ([]persist.DBID, error)
	LoadFunc       func(context.Context)
	SaveResultFunc func(ctx context.Context, userID persist.DBID, recommendedIDs []persist.DBID) error
	BootstrapFunc  func(ctx context.Context) ([]persist.DBID, error)
	saveCh         chan saveMsg
}

func NewRecommender(queries *db.Queries) *Recommender {
	r := &Recommender{saveCh: make(chan saveMsg)}

	r.LoadFunc = func(ctx context.Context) {
		g, err := generateGraph(ctx, queries)
		if err != nil {
			panic(err)
		}
		currentGraph.Store(metadataKey, g.Metadata)
		for node, neighbors := range g.Neighbors {
			currentGraph.Store(node, neighbors)
		}
	}

	r.SaveResultFunc = func(ctx context.Context, userID persist.DBID, recommendedIDs []persist.DBID) error {
		params := db.UpdatedRecommendationResultsParams{}
		for _, id := range recommendedIDs {
			params.ID = append(params.ID, persist.GenerateID().String())
			params.UserID = append(params.UserID, userID.String())
			params.RecommendedUserID = append(params.RecommendedUserID, id.String())
			params.RecommendedCount = append(params.RecommendedCount, 1)
		}
		return queries.UpdatedRecommendationResults(ctx, params)
	}

	r.BootstrapFunc = func(ctx context.Context) ([]persist.DBID, error) {
		recommendedIDs, err := queries.GetTopRecommendedUserIDs(ctx)
		if err != nil {
			return nil, err
		}
		shuffle(recommendedIDs)
		return recommendedIDs, nil
	}

	r.RecommendFunc = func(ctx context.Context, userID persist.DBID, follows []db.Follow) ([]persist.DBID, error) {
		var latestFollow time.Time
		queryNodes := make([]queryNode, len(follows))

		for _, f := range follows {
			if f.LastUpdated.Sub(latestFollow) > 0 {
				latestFollow = f.LastUpdated
			}
		}

		// Discrete weighting function that weights recent follows more
		score := func(f db.Follow) int {
			oneWeek := time.Hour * 24 * 7
			recency := latestFollow.Sub(f.LastUpdated)
			switch {
			case recency < oneWeek:
				return 8
			case recency < 2*oneWeek:
				return 4
			case recency < 4*oneWeek:
				return 2
			default:
				return 1
			}
		}

		for i, f := range follows {
			queryNodes[i] = queryNode{ID: f.Followee, Weight: score(f)}
		}

		// Create a new source of randomness for each thread
		// because the global source requires synchronization to use
		rng := rand.New(rand.NewSource(time.Now().Unix()))

		return walkFrom(ctx, r, userID, queryNodes, rng)
	}

	return r
}

// UsersFromFollowingShuffled re-orders ranked suggestions to generate more diverse results.
func (r *Recommender) RecommendFromFollowingShuffled(ctx context.Context, userID persist.DBID, follows []db.Follow) ([]persist.DBID, error) {
	recommendedIDs, err := r.RecommendFromFollowing(ctx, userID, follows)
	if err != nil {
		return nil, err
	}
	shuffle(recommendedIDs)
	return recommendedIDs, err
}

// UsersFromFollowing suggest users based on a given user's follows sorted in descending order.
func (r *Recommender) RecommendFromFollowing(ctx context.Context, userID persist.DBID, follows []db.Follow) ([]persist.DBID, error) {
	if len(follows) == 0 {
		return r.BootstrapFunc(ctx)
	}

	recommendedIDs, err := r.RecommendFunc(ctx, userID, follows)
	if err != nil {
		return nil, err
	}

	if len(recommendedIDs) == 0 {
		return r.BootstrapFunc(ctx)
	}

	if len(recommendedIDs) > 100 {
		recommendedIDs = recommendedIDs[:100]
	}

	go func() { r.saveCh <- saveMsg{userID, recommendedIDs} }()
	return recommendedIDs, nil
}

// Run is the main event loop that manages access to the currently loaded graph
func (r *Recommender) Run(ctx context.Context, ticker *time.Ticker) {
	r.LoadFunc(ctx)
	go func() {
		for {
			select {
			case <-ticker.C:
				r.LoadFunc(ctx)
			case msg := <-r.saveCh:
				if err := r.SaveResultFunc(ctx, msg.nodeID, msg.recommendedIDs); err != nil {
					logger.For(ctx).Errorf("failed to save recommendation: %s", err)
				}
			}
		}
	}()
}

type saveMsg struct {
	nodeID         persist.DBID
	recommendedIDs []persist.DBID
}

func (r *Recommender) readNeighbors(ctx context.Context, nodeID persist.DBID) []persist.DBID {
	val, ok := currentGraph.Load(nodeID)
	if !ok {
		return []persist.DBID{}
	}
	return val.([]persist.DBID)
}

func (r *Recommender) readMetadata(ctx context.Context) graphMetadata {
	val, ok := currentGraph.Load(metadataKey)
	if !ok {
		panic("no metadata for graph!")
	}
	return val.(graphMetadata)
}

// shuffle shuffles IDs within partitions so that results are in a similar order.
func shuffle(ids []persist.DBID) {
	if len(ids) <= 1 {
		return
	}

	size := 24
	for len(ids) < size {
		size /= 2
	}

	subset := make([]persist.DBID, size)

	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		subset = ids[i:end]
		rand.Shuffle(end-i, func(i, j int) { subset[i], subset[j] = subset[j], subset[i] })
		copy(ids[i:end], subset)
	}
}
