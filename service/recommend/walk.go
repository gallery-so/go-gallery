package recommend

import (
	"context"
	"math"
	"math/rand"
	"sort"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/tracing"
)

// The parameters below alter the behavior of the random walk.
// The optimal values can be found empirically - these are starting values
// we can tune later.

// totalWalks determines how many walks are started
const totalWalks = 10

// walkLength is the maximum path length for each walk
const walkLength = 200000

// restartRate is the probabilty that a walk restarts from the beginning
const restartRate float64 = 0.25

// nP and nV controls the early stopping condition. A walk terminates when the
// nPth ranked node is visited nV times.
const nP = 500
const nV = 20

// visits keeps the of times a node has been visisted
type visits map[persist.DBID]int

func walkFrom(ctx context.Context, r *Recommender, originID persist.DBID, queryNodes []queryNode) ([]persist.DBID, error) {
	span, ctx := tracing.StartSpan(ctx, "recommend", "walk")
	defer tracing.FinishSpan(span)

	if len(queryNodes) == 0 {
		return []persist.DBID{}, nil
	}

	walks := make(map[queryNode]visits)
	totalVisits := make(visits)
	currentEdges := make(map[persist.DBID]bool)

	for _, node := range queryNodes {
		currentEdges[node.ID] = true
	}

	queryNodes = weightedSample(queryNodes)
	steps := allocateSteps(ctx, r, queryNodes)

	for _, node := range queryNodes {
		walks[node] = walk(ctx, r, currentEdges, originID, node, steps[node.ID])
	}

	for _, walk := range walks {
		for node, count := range walk {
			// We take the square root of the count to boost
			// the signal of nodes that have been visited in more than one walk
			// in the for loop below.
			totalVisits[node] += int(math.Sqrt(float64(count)))
		}
	}

	scored := make([]persist.DBID, len(totalVisits))
	i := 0
	for node, score := range totalVisits {
		scored[i] = node
		// Nodes that have been visited in more than one walk gets boosted, whereas
		// nodes that have been visited by a single walk keeps their original count.
		totalVisits[node] = int(math.Pow(float64(score), 2))
		i++
	}

	sort.Slice(scored, func(i, j int) bool {
		return totalVisits[scored[i]] > totalVisits[scored[j]]
	})

	return scored, nil
}

// walk performs a random walk starting from startNode
func walk(ctx context.Context, r *Recommender, currentEdges map[persist.DBID]bool, originID persist.DBID, startNode queryNode, steps int) visits {
	v := make(visits)
	currentID := startNode.ID
	for i, threshold := 0, 0; i < steps && threshold < nP; i++ {
		// Restart the walk if there aren't neighbors adjacent to node
		nodeNeighbors := r.readNeighbors(ctx, currentID)
		if len(nodeNeighbors) == 0 {
			currentID = startNode.ID
			continue
		}

		// Select a neighbor from a node at random. In the future, we could bias neighbor selection
		// to achieve more personalized results.
		neighborPos := rand.Intn(len(nodeNeighbors))
		currentID = nodeNeighbors[neighborPos]

		// Only count the visit if the selected node is not a node the node
		// is already connected to and not the node we are finding suggestions for.
		if _, isNeighbor := currentEdges[currentID]; !isNeighbor && currentID != originID {
			v[currentID]++
			if v[currentID] >= nV {
				threshold++
			}
		}

		// Randomly restart the walk so walks don't stray too far
		if rand.Float64() < restartRate {
			currentID = startNode.ID
		}
	}

	return v
}

// queryNode represents a starting point on the graph
type queryNode struct {
	ID     persist.DBID
	Weight int
}

// allocateSteps sets the the maximum walk length of each node
func allocateSteps(ctx context.Context, r *Recommender, nodes []queryNode) map[persist.DBID]int {
	queryNodes := make(map[persist.DBID]int)
	scaleFactors := make([]int, len(nodes))
	totalFactors := 0
	metadata := r.readGraphMetadata(ctx)

	for i, n := range nodes {
		indegree := 1 // Node "follows" themself
		indegree += metadata.Indegrees[n.ID]
		// scaleFactor scales the number of walks allocated to a query node by how
		// popular the node is. This scales the number of steps sub-linearly
		// so that there isn't a disproportionate amount of steps allocated to popular nodes.
		scaleFactor := indegree * int(float64(metadata.MaxIndegree)-math.Log(float64(indegree)))
		scaleFactors[i] = scaleFactor
		totalFactors += scaleFactor
	}

	for i, node := range nodes {
		queryNodes[node.ID] = (node.Weight * walkLength * scaleFactors[i]) / totalFactors
	}

	return queryNodes
}

// weightedSample returns a sample of queryNodes, where the probability of a node
// being selected is proportional to its weight.
func weightedSample(nodes []queryNode) []queryNode {
	keys := make([]float64, len(nodes))
	for i, node := range nodes {
		keys[i] = math.Pow(rand.Float64(), (1 / float64(node.Weight)))
	}

	sort.Slice(nodes, func(i, j int) bool {
		return keys[i] > keys[j]
	})

	return nodes[:totalWalks]
}
