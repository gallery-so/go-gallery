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
	Indegrees map[persist.DBID]int `json:"indegrees"`
}

func generateGraph(ctx context.Context, queries *db.Queries) (*graph, error) {
	span, ctx := tracing.StartSpan(ctx, "recommend", "generateGraph")
	defer tracing.FinishSpan(span)

	follows, err := queries.GetFollowGraphSource(ctx)
	if err != nil {
		return nil, err
	}

	neighbors := map[persist.DBID][]persist.DBID{}

	var inDegrees = make(map[persist.DBID]int)

	for _, f := range follows {
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
