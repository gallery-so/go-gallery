package koala

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/james-bowman/sparse"
	"gonum.org/v1/gonum/mat"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var ErrNoInputData = errors.New("no personalization input data")
var contextKey = "personalization.instance"
var socialWeight = 1.0
var relevanceWeight = 2.0
var similarityWeight = 1.0

// sharedContracts are excluded from the personalization metrix because of their low specificity
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

type Koala struct {
	// userM is a matrix of size u x u where a non-zero value at m[i][j] denotes an edge from user i to user j
	userM *sparse.CSR
	// ratingM is a matrix of size u x k where the value at m[i][j] is how many held tokens of community k are displayed by user j
	ratingM *sparse.CSR
	// displayM is matrix of size k x k where the value at m[i][j] is how many tokens of community i are displayed with community j
	displayM *sparse.CSR
	// simM is a matrix of size u x u where the value at m[i][j] is a combined value of follows and common tokens displayed by user i and user j
	simM *sparse.CSR
	// lookup of user id to index in the matrix
	uL map[persist.DBID]int
	// lookup of contract id to index in the matrix
	cL map[persist.DBID]int
	mu sync.Mutex
	q  *db.Queries
}

func NewKoala(ctx context.Context, q *db.Queries) *Koala {
	k := &Koala{q: q}
	newMatrices(ctx, k)
	return k
}

func newMatrices(ctx context.Context, k *Koala) {
	uL := readUserLabels(ctx, k.q)
	ratingM, displayM, cL := readContractMatrices(ctx, k.q, uL)
	userM := readUserMatrix(ctx, k.q, uL)
	simM := toSimMatrix(ratingM, userM)
	k.userM = userM
	k.ratingM = ratingM
	k.displayM = displayM
	k.simM = simM
	k.uL = uL
	k.cL = cL
}

func AddTo(c *gin.Context, k *Koala) {
	c.Set(contextKey, k)
}

func For(ctx context.Context) *Koala {
	gc := util.MustGetGinContext(ctx)
	return gc.Value(contextKey).(*Koala)
}

func (k *Koala) RelevanceTo(userID persist.DBID, post db.Post) (topS float64, err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(post.ContractIds) == 0 {
		return k.scoreFeatures(userID, post.ActorID, "")
	}
	var scored bool
	var s float64
	for _, contractID := range post.ContractIds {
		s, err = k.scoreFeatures(userID, post.ActorID, contractID)
		if err == nil && s > topS {
			topS = s
			scored = true
		}
	}
	if scored {
		return topS, nil
	}
	return 0, err
}

// Loop is the main event loop that updates the personalization matrices
func (k *Koala) Loop(ctx context.Context, ticker *time.Ticker) {
	go func() {
		for {
			select {
			case <-ticker.C:
				k.mu.Lock()
				defer k.mu.Unlock()
				newMatrices(ctx, k)
			}
		}
	}()
}

func (k *Koala) scoreFeatures(viewerID, queryID, contractID persist.DBID) (float64, error) {
	vIdx, vOK := k.uL[viewerID]
	qIdx, qOK := k.uL[queryID]
	cIdx, cOK := k.cL[contractID]
	// Nothing to compute
	if (!vOK || !qOK) && !cOK {
		return 0, ErrNoInputData
	}
	// If vIdx or qIdx not in the matrix, compute the relevance score
	if (!vOK || !qOK) && cOK {
		return calcRelevanceScore(k.ratingM, k.displayM, vIdx, cIdx), nil
	}
	// If cIdx not in the matrix, compute the social and similarity scores
	if (vOK && qOK) && !cOK {
		socialScore := calcSocialScore(k.userM, vIdx, qIdx)
		similarityScore := calcSimilarityScore(k.simM, vIdx, qIdx)
		return ((socialWeight * socialScore) + (similarityWeight * similarityScore)) / 2, nil
	}
	socialScore := calcSocialScore(k.userM, vIdx, qIdx)
	relevanceScore := calcRelevanceScore(k.ratingM, k.displayM, vIdx, cIdx)
	similarityScore := calcSimilarityScore(k.simM, vIdx, qIdx)
	score := ((socialWeight * socialScore) + (relevanceWeight * relevanceScore) + (similarityWeight * similarityScore)) / 3
	return score, nil
}

// calcSocialScore determines if vIdx is in the same friend circle as qIdx by running a DFS on userM
func calcSocialScore(userM *sparse.CSR, vIdx, qIdx int) float64 {
	n, _ := userM.Dims()
	return dfs(userM, n, vIdx, qIdx)
}

// calcRelavanceScore computes the relevance of cIdx to vIdx's held tokens
func calcRelevanceScore(ratingM, displayM *sparse.CSR, vIdx, cIdx int) float64 {
	var top float64
	v := ratingM.RowView(vIdx).(*sparse.Vector)
	v.DoNonZero(func(i int, j int, v float64) {
		s := jaccardIndex(displayM, i, cIdx)
		if s > top {
			top = s
		}
	})
	return top
}

// calcSimilarityScore computes the similarity of vIdx and qIdx based on their interactions with other users
func calcSimilarityScore(simM *sparse.CSR, vIdx, qIdx int) float64 {
	v1 := simM.RowView(vIdx).(*sparse.Vector)
	v2 := simM.RowView(qIdx).(*sparse.Vector)
	return cosineSimilarity(v1, v2)
}

func dfs(m *sparse.CSR, n, vIdx, qIdx int) float64 {
	visits := make([]int, n)
	s := stack{}
	s.Push(vIdx)
	for len(s) > 0 {
		cur := s.Pop()
		if cur == qIdx {
			return 1
		}
		if visits[cur] == 0 {
			visits[cur] = 1
			neighbors := m.RowView(cur).(*sparse.Vector)
			neighbors.DoNonZero(func(i int, j int, v float64) {
				if visits[i] == 0 {
					s.Push(i)
				}
			})
		}
	}
	return 0
}

// jaccardIndex computes overlap divided by the union of two sets
func jaccardIndex(m *sparse.CSR, a, b int) float64 {
	return m.At(a, b) / (m.At(a, a) + m.At(b, b) - m.At(a, b))
}

func cosineSimilarity(v1, v2 mat.Vector) float64 {
	return sparse.Dot(v1, v2) / (sparse.Norm(v1, 2) * sparse.Norm(v2, 2))
}

type stack []int

func (s *stack) Pop() int {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[:n-1]
	return x
}

func (s *stack) Push(i int) {
	*s = append(*s, i)
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
	contracts, err := q.GetDisplayedContracts(ctx, excludedContracts)
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
	displayMT := displayM.T().(*sparse.CSR).ToCSR()
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
	r := make([]string, len(m))
	i := 0
	for k := range m {
		r[i] = k.String()
		i++
	}
	return r
}
