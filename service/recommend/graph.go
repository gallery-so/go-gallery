package recommend

import (
	"context"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/tracing"
)

type graph struct {
	Neighbors map[persist.DBID][]persist.DBID
	Metadata  graphMetadata `json:"metadata"`
}

type graphMetadata struct {
	Indegrees    map[persist.DBID]int
	MaxIndegree  int `json:"max_indegree"`
	MaxOutdegree int `json:"max_outdegree"`
	TotalEdges   int `json:"total_edges"`
}

func generateGraph(ctx context.Context, queries *db.Queries) (*graph, error) {
	span, ctx := tracing.StartSpan(ctx, "recommend", "generateGraph")
	defer tracing.FinishSpan(span)

	follows, err := queries.GetFollowGraphSource(ctx)
	if err != nil {
		return nil, err
	}

	neighbors := map[persist.DBID][]persist.DBID{}

	// Store additional metadata which gets used in the algorithm
	// for calculating node weights
	var inDegrees = make(map[persist.DBID]int)

	for _, f := range follows {
		if _, ok := neighbors[f.Follower]; !ok {
			neighbors[f.Follower] = []persist.DBID{}
		}
		neighbors[f.Follower] = append(neighbors[f.Follower], f.Followee)
		inDegrees[f.Followee]++
	}

	return &graph{
		Neighbors: neighbors,
		Metadata: graphMetadata{
			Indegrees: inDegrees,
		},
	}, nil
}
