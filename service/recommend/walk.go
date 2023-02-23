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
const totalWalks = 30

// walkSteps is the maximum number steps that can be taken for each walk
const walkSteps = 200000

// The max number degrees away from the start node before a walk resets
const maxDepth = 10

func walkFrom(ctx context.Context, r *Recommender, originID persist.DBID, queryNodes []queryNode, rng *rand.Rand) ([]persist.DBID, error) {
	span, ctx := tracing.StartSpan(ctx, "recommend", "walk")
	defer tracing.FinishSpan(span)

	if len(queryNodes) == 0 {
		return []persist.DBID{}, nil
	}

	walks := make(map[queryNode]map[persist.DBID]int)
	totalVisits := make(map[persist.DBID]int)
	currentEdges := make(map[persist.DBID]bool)

	for _, node := range queryNodes {
		currentEdges[node.ID] = true
	}

	queryNodes = weightedSample(queryNodes, rng)
	steps := allocateSteps(ctx, r, queryNodes)

	for _, node := range queryNodes {
		walks[node] = walk(ctx, r, currentEdges, originID, node.ID, steps[node.ID], rng)
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
func walk(ctx context.Context, r *Recommender, currentEdges map[persist.DBID]bool, originID persist.DBID, startID persist.DBID, steps int, rng *rand.Rand) map[persist.DBID]int {
	neighbors := r.readNeighbors(ctx, startID)

	if len(neighbors) == 0 {
		return map[persist.DBID]int{}
	}

	type edge struct{ ID, Next persist.DBID }

	visited := make(map[edge]bool)
	visits := make(map[persist.DBID]int)
	current := startID
	depth := 0
	for i := 0; i < steps; i++ {
		neighbors = r.readNeighbors(ctx, current)
		if len(neighbors) == 0 {
			current = startID
			depth = 0
			continue
		}
		currentEdge := edge{current, neighbors[rng.Intn(len(neighbors))]}
		current = currentEdge.Next
		if !visited[currentEdge] {
			visited[currentEdge] = true
			if current != originID && !currentEdges[current] {
				visits[current]++
			}
		}
		depth++
		if depth > maxDepth {
			current = startID
			depth = 0
		}
	}

	return visits
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
	metadata := r.readMetadata(ctx)
	totalFactors := 0
	maxIndegree := 0

	for _, n := range nodes {
		if metadata.Indegrees[n.ID] > maxIndegree {
			maxIndegree = metadata.Indegrees[n.ID]
		}
	}

	for i, n := range nodes {
		indegree := metadata.Indegrees[n.ID]
		// scaleFactor scales the number of walks allocated to a query node by how
		// popular the node is. This scales the number of steps sub-linearly
		// so that there isn't a disproportionate amount of steps allocated to popular nodes.
		scaleFactor := indegree * int(float64(maxIndegree)-math.Log(float64(indegree)))
		scaleFactors[i] = scaleFactor
		totalFactors += scaleFactor
	}

	for i, node := range nodes {
		ratio := float64(scaleFactors[i]) / float64(totalFactors)
		queryNodes[node.ID] = int(float64(node.Weight) * float64(walkSteps) * ratio)
	}

	return queryNodes
}

// weightedSample returns a sample of queryNodes, where the probability of selecting
// a node is proportional to its weight. This uses a seemingly magic algorithm called A-Res:
// https://en.wikipedia.org/wiki/Reservoir_sampling#Algorithm_A-Res
func weightedSample(nodes []queryNode, rng *rand.Rand) []queryNode {
	if len(nodes) < totalWalks {
		return nodes
	}

	keys := make(map[queryNode]float64)
	for _, node := range nodes {
		keys[node] = math.Pow(rng.Float64(), (1 / float64(node.Weight)))
	}

	sort.Slice(nodes, func(i, j int) bool {
		return keys[nodes[i]] > keys[nodes[j]]
	})

	return nodes[:totalWalks]
}
