package recommend

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const contextKey = "recommend.instance"

func AddTo(c *gin.Context, r *Recommender) {
	c.Set(contextKey, r)
}

func For(ctx context.Context) *Recommender {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(contextKey).(*Recommender)
}

type Recommender struct {
	currentGraph   *graph
	bootstrapFunc  func(ctx context.Context) ([]persist.DBID, error)
	loadFunc       func(context.Context) *graph
	saveResultFunc func(ctx context.Context, userID persist.DBID, recommendedIDs []persist.DBID) error
	readCh         chan any
	updateCh       chan updateMsg
}

type updateMsg struct {
	nodeID    persist.DBID
	neighbors []persist.DBID
}

type readNeighborsMsg struct {
	nodeID  persist.DBID
	replyCh chan []persist.DBID
}

type readMetadataMsg struct {
	replyCh chan graphMetadata
}

func NewRecommender(queries *db.Queries) *Recommender {
	loadFunc := func(ctx context.Context) *graph {
		g, err := generateGraph(ctx, queries)
		if err != nil {
			panic(err)
		}
		return g
	}

	saveResultFunc := func(ctx context.Context, userID persist.DBID, recommendedIDs []persist.DBID) error {
		params := db.UpdatedRecommendationResultsParams{}
		for _, id := range recommendedIDs {
			params.ID = append(params.ID, persist.GenerateID().String())
			params.UserID = append(params.UserID, userID.String())
			params.RecommendedUserID = append(params.RecommendedUserID, id.String())
		}
		return queries.UpdatedRecommendationResults(ctx, params)
	}

	bootstrapFunc := func(ctx context.Context) ([]persist.DBID, error) {
		recommendedIDs, err := queries.GetTopRecommendedUserIDs(ctx)
		if err != nil {
			return nil, err
		}
		shuffle(recommendedIDs)
		return recommendedIDs, nil
	}

	return &Recommender{
		bootstrapFunc:  bootstrapFunc,
		loadFunc:       loadFunc,
		saveResultFunc: saveResultFunc,
		readCh:         make(chan any),
		updateCh:       make(chan updateMsg),
	}
}

// UsersFromFollowing suggest users based on a given user's follows sorted in descending order.
func (r *Recommender) RecommendFromFollowing(ctx context.Context, userID persist.DBID, followingIDs []persist.DBID, followedTimes []time.Time) ([]persist.DBID, error) {
	if len(followingIDs) != len(followedTimes) {
		return nil, errors.New("`followingIDs` length not equal to `followedTimes` length")
	}

	// If the user isn't following anyone, generate a recommendation from history.
	if len(followingIDs) == 0 {
		return r.bootstrapFunc(ctx)
	}

	// Update graph with refreshed data
	r.updateNeighbors(userID, followingIDs)

	var latestFollow time.Time
	for _, t := range followedTimes {
		if t.Sub(latestFollow) > 0 {
			t = latestFollow
		}
	}

	// Discrete weighting function that weights recent follows more.
	score := func(i int) int {
		oneWeek := time.Hour * 24 * 7
		recency := latestFollow.Sub(followedTimes[i])
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

	followingNodes := make([]queryNode, len(followingIDs))
	for i, id := range followingIDs {
		followingNodes[i] = queryNode{
			ID:     id,
			Weight: score(i),
		}
	}

	recommendedIDs, err := walkFrom(ctx, r, userID, followingNodes)
	if err != nil {
		return nil, err
	}

	go func() {
		if err := r.saveResultFunc(context.Background(), userID, recommendedIDs); err != nil {
			logger.For(nil).Errorf("failed to save recommendation: %s", err)
		}
	}()

	return recommendedIDs[:100], nil
}

// UsersFromFollowingShuffled re-orders suggestions to generate more diverse results.
func (r *Recommender) RecommendFromFollowingShuffled(ctx context.Context, userID persist.DBID, followingIDs []persist.DBID, followedTimes []time.Time) ([]persist.DBID, error) {
	ids, err := r.RecommendFromFollowing(ctx, userID, followingIDs, followedTimes)
	if err != nil {
		return nil, err
	}
	shuffle(ids)
	return ids, err
}

// Run is the main event loop that controls read and write operations to the current graph.
func (r *Recommender) Run(ctx context.Context, ticker *time.Ticker) {
	r.currentGraph = r.loadFunc(ctx)
	for {
		select {
		case <-ticker.C:
			r.currentGraph = r.loadFunc(ctx)
		case update := <-r.updateCh:
			r.currentGraph.Neighbors[update.nodeID] = update.neighbors
		case msg := <-r.readCh:
			switch m := msg.(type) {
			case readNeighborsMsg:
				m.replyCh <- r.currentGraph.Neighbors[m.nodeID]
			case readMetadataMsg:
				m.replyCh <- r.currentGraph.Metadata
			default:
				panic("unknown read request")
			}
		}
	}
}

func (r *Recommender) readNeighbors(ctx context.Context, nodeID persist.DBID) []persist.DBID {
	msg := readNeighborsMsg{nodeID: nodeID, replyCh: make(chan []persist.DBID)}
	r.readCh <- msg
	response := <-msg.replyCh
	return response
}

func (r *Recommender) readGraphMetadata(ctx context.Context) graphMetadata {
	msg := readMetadataMsg{make(chan graphMetadata)}
	r.readCh <- msg
	response := <-msg.replyCh
	return response
}

func (r *Recommender) updateNeighbors(node persist.DBID, neighbors []persist.DBID) {
	r.updateCh <- updateMsg{node, neighbors}
}

// shuffle shuffles IDs within partitions so that results are in a similar order.
func shuffle(ids []persist.DBID) {
	partitionSize := 10
	partition := make([]persist.DBID, partitionSize)

	for i := 0; i < len(ids); i += partitionSize {
		partition = ids[i : i+partitionSize]
		rand.Shuffle(partitionSize, func(i, j int) {
			partition[i], partition[j] = partition[j], partition[i]
		})
		copy(ids[i:i+partitionSize], partition)
	}
}
