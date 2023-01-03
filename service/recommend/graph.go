package recommend

import (
	"context"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/tracing"
)

type adjacencyList map[persist.DBID][]persist.DBID

type graph struct {
	Neighbors adjacencyList `json:"neighbors"`
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

	neighbors := adjacencyList{}

	// Store additional metadata which gets used in the algorithm
	// for calculating node weights
	var inDegrees = make(map[persist.DBID]int)
	var maxIndegree int
	var maxOutdegree int
	var totalEdges int

	max := func(a, b int) int {
		if a > b {
			return a
		}
		return b
	}

	for _, f := range follows {
		if _, ok := neighbors[f.Follower]; !ok {
			neighbors[f.Follower] = []persist.DBID{}
		}
		neighbors[f.Follower] = append(neighbors[f.Follower], f.Followee)
		inDegrees[f.Followee]++
		totalEdges++
		maxIndegree = max(maxIndegree, inDegrees[f.Followee])
		maxOutdegree = max(maxOutdegree, len(neighbors[f.Follower]))
	}

	return &graph{
		Neighbors: neighbors,
		Metadata: graphMetadata{
			Indegrees:    inDegrees,
			MaxIndegree:  maxIndegree,
			MaxOutdegree: maxOutdegree,
			TotalEdges:   totalEdges,
		},
	}, nil
}
