//go:build !norec
// +build !norec

package koala

import (
	"context"
	"fmt"

	"github.com/james-bowman/sparse"
	"gonum.org/v1/gonum/mat"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

// sharedContracts are excluded because of their low specificity
var sharedContracts map[persist.ChainAddress]bool = map[persist.ChainAddress]bool{
	persist.NewChainAddress("KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton", persist.ChainTezos):     true, // hic et nunc NFTs
	persist.NewChainAddress("KT1GBZmSxmnKJXGMdMLbugPfLyUPmuLSMwKS", persist.ChainTezos):     true, // Tezos Domains NameRegistry
	persist.NewChainAddress("0x495f947276749ce646f68ac8c248420045cb7b5e", persist.ChainETH): true, // OpenSea Shared Storefront
	persist.NewChainAddress("0xd07dc4262bcdbf85190c01c996b4c06a461d2430", persist.ChainETH): true, // Rarible 1155
	persist.NewChainAddress("0x60f80121c31a0d46b5279700f9df786054aa5ee5", persist.ChainETH): true, // Rarible V2
	persist.NewChainAddress("0xb66a603f4cfe17e3d27b87a8bfcad319856518b8", persist.ChainETH): true, // Rarible
	persist.NewChainAddress("0xabb3738f04dc2ec20f4ae4462c3d069d02ae045b", persist.ChainETH): true, // KnownOriginDigitalAsset
	persist.NewChainAddress("0xfbeef911dc5821886e1dda71586d90ed28174b7d", persist.ChainETH): true, // Known Origin
	persist.NewChainAddress("0xb932a70a57673d89f4acffbe830e8ed7f75fb9e0", persist.ChainETH): true, // SuperRare
	persist.NewChainAddress("0x2a46f2ffd99e19a89476e2f62270e0a35bbf0756", persist.ChainETH): true, // MakersTokenV2
	persist.NewChainAddress("0x22c1f6050e56d2876009903609a2cc3fef83b415", persist.ChainETH): true, // POAP
}
var sharedContractsStr = keys(sharedContracts)

func NewKoala(ctx context.Context, q *db.Queries) *Koala {
	k := &Koala{q: q}
	k.update(ctx)
	return k
}

func (k *Koala) RelevanceTo(userID persist.DBID, e db.FeedEntityScoringRow) (float64, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if len(e.ContractIds) == 0 {
		return k.scoreEdge(userID, e.ActorID)
	}

	var relevanceScore float64

	for _, contractID := range e.ContractIds {
		if relevanceScore == 1 {
			break
		}
		if s, _ := k.scoreRelevance(userID, contractID); s > relevanceScore {
			relevanceScore = s
		}
	}

	edgeScore, err := k.scoreEdge(userID, e.ActorID)
	if err != nil {
		return relevanceScore, err
	}

	return relevanceScore * edgeScore, nil
}

func (k *Koala) scoreEdge(viewerID, queryID persist.DBID) (float64, error) {
	vIdx, vOK := k.uL[viewerID]
	qIdx, qOK := k.uL[queryID]
	if !vOK || !qOK {
		return 0, ErrNoInputData
	}
	socialScore := calcSocialScore(k.userM, vIdx, qIdx)
	similarityScore := calcSimilarityScore(k.simM, vIdx, qIdx)
	return socialScore + similarityScore, nil
}

func (k *Koala) scoreRelevance(viewerID, contractID persist.DBID) (float64, error) {
	vIdx, vOK := k.uL[viewerID]
	cIdx, cOK := k.cL[contractID]
	if !vOK || !cOK {
		return 0, ErrNoInputData
	}
	return calcRelevanceScore(k.ratingM, k.displayM, vIdx, cIdx), nil
}

func readMatrices(ctx context.Context, q *db.Queries) (userM, ratingM, displayM, simM *sparse.CSR, uL, cL map[persist.DBID]int) {
	uL = readUserLabels(ctx, q)
	ratingM, displayM, cL = readContractMatrices(ctx, q, uL)
	userM = readUserMatrix(ctx, q, uL)
	simM = toSimMatrix(ratingM, userM)
	return userM, ratingM, displayM, simM, uL, cL
}

func (k *Koala) update(ctx context.Context) {
	userM, ratingM, displayM, simM, uL, cL := readMatrices(ctx, k.q)
	k.mu.Lock()
	defer k.mu.Unlock()
	k.userM = userM
	k.ratingM = ratingM
	k.displayM = displayM
	k.simM = simM
	k.uL = uL
	k.cL = cL
}

// calcSocialScore determines if vIdx is in the same friend circle as qIdx by running a bfs on userM
func calcSocialScore(userM *sparse.CSR, vIdx, qIdx int) float64 {
	score := bfs(userM, vIdx, qIdx)
	return score
}

// calcRelavanceScore computes the relevance of cIdx to vIdx's held tokens
func calcRelevanceScore(ratingM, displayM *sparse.CSR, vIdx, cIdx int) float64 {
	var t float64
	cDisplayed := displayM.At(cIdx, cIdx)
	v := ratingM.RowView(vIdx).(*sparse.Vector)
	v.DoNonZero(func(i int, j int, v float64) {
		// Max score is 1, we can't exit early but we can skip the calculation
		if t == 1 {
			return
		}
		if s := jaccardIndex(displayM, i, cIdx, cDisplayed); s > t {
			t = s
		}
	})
	// 0.02 is the average overlap of displayed tokens between two communities. It's interpretation is
	// on average 2% of the the tokens any two communities are displayed together. This is an approximation,
	// a better way would be to calculate the average overlap of this community to all other communities, since
	// it is possible that a community is shared frequently with all other communities, but this would require
	// computing the score for every community.
	//
	// Dividing by this value creates an index where a score above 0.02 over indexes and is a value greater than
	// 1. This means that the a score of 1 gets scaled to 50. This is a pretty massive boost, and we may want to clamp
	// this value to some max.
	return t / 0.02
}

// calcSimilarityScore computes the similarity of vIdx and qIdx based on their interactions with other users
func calcSimilarityScore(simM *sparse.CSR, vIdx, qIdx int) float64 {
	v1 := simM.RowView(vIdx).(*sparse.Vector)
	v2 := simM.RowView(qIdx).(*sparse.Vector)
	score := cosineSimilarity(v1, v2)
	return score
}

// idLookup uses a slice to lookup a value by its index
// Its purpose is to avoid using a map because indexing a slice is suprisingly much quicker than a map lookup.
// It should be pretty space efficient: with one million users, it should take only 1 million * 1 byte = 1MB of memory.
type idLookup struct {
	l []int8
}

func newIdLookup() idLookup {
	return idLookup{l: make([]int8, 1)}
}

func (n idLookup) Get(idx int) int8 {
	if idx >= len(n.l) {
		return 0
	}
	return n.l[idx]
}

func (n *idLookup) Set(idx int, i int8) {
	appendVal(&n.l, idx, i)
}

func extendBy[T any](s *[]T, i int) {
	if newSize := i + 1; newSize > cap(*s) {
		cp := make([]T, newSize*2, newSize*2)
		copy(cp, *s)
		*s = cp
	}
}

func appendVal[T any](s *[]T, i int, v T) {
	extendBy(s, i)
	(*s)[i] = v
}

func bfs(m *sparse.CSR, vIdx, qIdx int) float64 {
	q := queue{}
	q.Push(vIdx)
	depth := newIdLookup()
	visited := newIdLookup()
	for len(q) > 0 {
		cur := q.Pop()
		if cur == qIdx {
			return 1
		}
		if depth.Get(cur) > 4 {
			return 0
		}
		neighbors := m.RowView(cur).(*sparse.Vector)
		neighbors.DoNonZero(func(i int, j int, v float64) {
			if visited.Get(i) == 0 {
				visited.Set(i, 1)
				depth.Set(i, depth.Get(cur)+1)
				q.Push(i)
			}
		})
	}
	return 0
}

// jaccardIndex computes intersection divided by the union of two sets
func jaccardIndex(m *sparse.CSR, a int, b int, cVal float64) float64 {
	u := m.At(a, b)
	return u / (m.At(a, a) + cVal - u)
}

func cosineSimilarity(v1, v2 mat.Vector) float64 {
	return sparse.Dot(v1, v2) / (sparse.Norm(v1, 2) * sparse.Norm(v2, 2))
}

type queue []int

func (q *queue) Push(i int) {
	*q = append(*q, i)
}

func (q *queue) Pop() int {
	old := *q
	x := old[0]
	old[0] = 0
	*q = old[1:]
	return x
}

func readUserLabels(ctx context.Context, q *db.Queries) map[persist.DBID]int {
	labels, err := q.GetUserLabels(ctx)
	check(err)
	uL := make(map[persist.DBID]int, len(labels))
	for i, u := range labels {
		uL[u] = i
	}
	return uL
}

func readContractMatrices(ctx context.Context, q *db.Queries, uL map[persist.DBID]int) (ratingM, displayM *sparse.CSR, cL map[persist.DBID]int) {
	excludedContracts := make([]string, len(sharedContracts))
	i := 0
	for c := range sharedContracts {
		excludedContracts[i] = c.String()
		i++
	}
	contracts, err := q.GetContractLabels(ctx, excludedContracts)
	check(err)
	var cIdx int
	cL = make(map[persist.DBID]int, len(contracts))
	userToDisplayed := make(map[persist.DBID][]persist.DBID)
	for _, c := range contracts {
		if _, ok := cL[c.ContractID]; !ok {
			cL[c.ContractID] = cIdx
			cIdx++
		}
		userToDisplayed[c.UserID] = append(userToDisplayed[c.UserID], c.ContractID)
	}
	dok := sparse.NewDOK(len(uL), cIdx)
	for uID, uIdx := range uL {
		for _, contractID := range userToDisplayed[uID] {
			cIdx := mustGet(cL, contractID)
			dok.Set(uIdx, cIdx, 1)
		}
		// If a user doesn't own any tokens, add a dummy row so the number of rows is equal to uL
		if len(userToDisplayed[uID]) == 0 {
			dok.Set(uIdx, 0, 0)
		}
	}
	ratingM = dok.ToCSR()
	displayM = dok.ToCSR()
	displayMT := displayM.T().(*sparse.CSC).ToCSR()
	displayMMT := &sparse.CSR{}
	displayMMT.Mul(displayMT, displayM)
	return ratingM, displayMMT, cL
}

func readUserMatrix(ctx context.Context, q *db.Queries, uL map[persist.DBID]int) *sparse.CSR {
	follows, err := q.GetFollowGraphSource(ctx)
	check(err)
	dok := sparse.NewDOK(len(uL), len(uL))
	userAdj := make(map[persist.DBID][]persist.DBID)
	for _, f := range follows {
		userAdj[f.Follower] = append(userAdj[f.Follower], f.Followee)
	}
	for uID, uIdx := range uL {
		for _, followee := range userAdj[uID] {
			followeeIdx := mustGet(uL, followee)
			dok.Set(uIdx, followeeIdx, 1)
		}
		// If a user isn't connected to anyone, add a dummy row so the matrix is the same size as uL
		if len(userAdj[uID]) == 0 {
			dok.Set(uIdx, 0, 0)
		}
	}
	return dok.ToCSR()
}

func toSimMatrix(ratingM, userM *sparse.CSR) *sparse.CSR {
	rRows, _ := ratingM.Dims()
	uRows, _ := userM.Dims()
	checkOK(rRows == uRows, "number of rows in rating matrix is not equal to number of rows in user matrix")
	ratingMT := ratingM.T().(*sparse.CSC).ToCSR()
	ratingMMT := &sparse.CSR{}
	ratingMMT.Mul(ratingM, ratingMT)
	simM := &sparse.CSR{}
	simM.Add(userM, ratingMMT)
	return simM
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func checkOK(ok bool, msg string) {
	if !ok {
		panic(msg)
	}
}

func mustGet[K comparable, T any](m map[K]T, k K) T {
	v, ok := m[k]
	if !ok {
		panic(fmt.Sprintf("key %v not found in map", k))
	}
	return v
}

func keys(m map[persist.ChainAddress]bool) []string {
	l := make([]string, len(m))
	i := 0
	for k := range m {
		l[i] = k.String()
		i++
	}
	return l
}
