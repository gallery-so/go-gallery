package userpref

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/james-bowman/sparse"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/store"
	"github.com/mikeydub/go-gallery/util"
)

const (
	contextKey    = "personalization.instance"
	gcpObjectName = "personalization_matrices.bin.gz"
)

var ErrNoInputData = errors.New("no personalization input data")

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

type personalizationMatrices struct {
	// userM is a matrix of size u x u where a non-zero value at m[i][j] is an edge from user i to user j
	userM *sparse.CSR
	// ratingM is a matrix of size u x k where the value at m[i][j] is how many held tokens of community j are displayed by user i
	ratingM *sparse.CSR
	// displayM is matrix of size k x k where the value at m[i][j] is how many tokens of community i are displayed with community j
	displayM *sparse.CSR
	// simM is a matrix of size u x u where the value at m[i][j] is a combined value of follows and common tokens displayed by user i and user j
	simM *sparse.CSR
	// lookup of user ID to index in the matrix
	uL map[persist.DBID]int
	// lookup of contract ID to index in the matrix
	cL map[persist.DBID]int
	// lastUpdated is the time the matrices were last updated
	lastUpdated time.Time
}

func (p personalizationMatrices) MarshalBinary() ([]byte, error) {
	var buf []byte
	appendTime(&buf, p.lastUpdated)
	appendMatrix(&buf, p.userM)
	appendMatrix(&buf, p.ratingM)
	appendMatrix(&buf, p.displayM)
	appendMatrix(&buf, p.simM)
	appendJSON(&buf, p.uL)
	appendJSON(&buf, p.cL)
	return buf, nil
}

func (p *personalizationMatrices) UnmarshalBinary(data []byte) {
	r := bytes.NewReader(data)
	t := readTime(r)
	userM := readMatrix(r)
	ratingM := readMatrix(r)
	displayM := readMatrix(r)
	simM := readMatrix(r)
	uL := readJSON(r)
	cL := readJSON(r)
	p.lastUpdated = t
	p.userM = userM
	p.ratingM = ratingM
	p.displayM = displayM
	p.simM = simM
	p.uL = uL
	p.cL = cL
}

func (p *personalizationMatrices) UnmarshalBinaryFrom(r io.Reader) {
	b, err := io.ReadAll(r)
	check(err)
	p.UnmarshalBinary(b)
}

func appendMatrix(buf *[]byte, m *sparse.CSR) {
	byt, err := m.MarshalBinary()
	check(err)
	appendTo(buf, byt)
}

func appendJSON(buf *[]byte, l map[persist.DBID]int) {
	byt, err := json.Marshal(l)
	check(err)
	appendTo(buf, byt)
}

func appendTime(buf *[]byte, t time.Time) {
	byt, err := t.MarshalBinary()
	check(err)
	appendTo(buf, byt)
}

func readMatrix(r *bytes.Reader) *sparse.CSR {
	buf := readTo(r)
	var m sparse.CSR
	err := m.UnmarshalBinary(buf)
	check(err)
	return &m
}

func readJSON(r *bytes.Reader) map[persist.DBID]int {
	buf := readTo(r)
	var l map[persist.DBID]int
	err := json.Unmarshal(buf, &l)
	check(err)
	return l
}

func readTime(r *bytes.Reader) time.Time {
	buf := readTo(r)
	t := time.Time{}
	err := t.UnmarshalBinary(buf)
	check(err)
	return t
}

func appendTo(buf *[]byte, byt []byte) {
	tmp := make([]byte, binary.MaxVarintLen64)
	bytesWritten := binary.PutUvarint(tmp, uint64(len(byt)))
	*buf = append(*buf, tmp[:bytesWritten]...)
	*buf = append(*buf, byt...)
}

func readTo(r *bytes.Reader) []byte {
	l, err := binary.ReadUvarint(r)
	check(err)
	buf := make([]byte, l)
	_, err = r.Read(buf)
	check(err)
	return buf
}

type Personalization struct {
	Mu sync.RWMutex
	pM *personalizationMatrices
	q  *db.Queries
	b  store.BucketStorer
}

func AddTo(c *gin.Context, k *Personalization) {
	c.Set(contextKey, k)
}

func For(ctx context.Context) *Personalization {
	gc := util.MustGetGinContext(ctx)
	return gc.Value(contextKey).(*Personalization)
}

// Loop is the main event loop that updates the personalization matrices
func (k *Personalization) Loop(ctx context.Context, ticker *time.Ticker) {
	go func() {
		for {
			<-ticker.C
			k.update(ctx)
		}
	}()
}

func NewPersonalization(ctx context.Context, q *db.Queries, c *storage.Client) *Personalization {
	b := store.NewBucketStorer(c, env.GetString("GCLOUD_USER_PREF_BUCKET"))
	k := &Personalization{q: q, b: b}
	k.update(ctx)
	return k
}

func (p *Personalization) RelevanceTo(userID persist.DBID, e db.FeedEntityScore) (float64, error) {
	// We don't have personalization data for this user yet
	_, vOK := p.pM.uL[userID]
	if !vOK {
		return 0, ErrNoInputData
	}

	var relevanceScore float64

	for _, contractID := range e.ContractIds {
		if relevanceScore == 1 {
			break
		}
		if s, _ := p.scoreRelevance(userID, contractID); s > relevanceScore {
			relevanceScore = s
		}
	}

	edgeScore, _ := p.scoreEdge(userID, e.ActorID)
	return relevanceScore + edgeScore, nil
}

func (p *Personalization) scoreEdge(viewerID, queryID persist.DBID) (float64, error) {
	vIdx, vOK := p.pM.uL[viewerID]
	qIdx, qOK := p.pM.uL[queryID]
	if !vOK || !qOK {
		return 0, ErrNoInputData
	}
	socialScore := calcSocialScore(p.pM.userM, vIdx, qIdx)
	similarityScore := calcSimilarityScore(p.pM.simM, vIdx, qIdx)
	return socialScore + similarityScore, nil
}

func (p *Personalization) scoreRelevance(viewerID, contractID persist.DBID) (float64, error) {
	vIdx, vOK := p.pM.uL[viewerID]
	cIdx, cOK := p.pM.cL[contractID]
	if !vOK || !cOK {
		return 0, ErrNoInputData
	}
	return calcRelevanceScore(p.pM.ratingM, p.pM.displayM, vIdx, cIdx), nil
}

func readMatrices(ctx context.Context, q *db.Queries) personalizationMatrices {
	uL := readUserLabels(ctx, q)
	ratingM, displayM, cL := readContractMatrices(ctx, q, uL)
	userM := readUserMatrix(ctx, q, uL)
	simM := toSimMatrix(ratingM, userM)
	return personalizationMatrices{
		userM:       userM,
		ratingM:     ratingM,
		displayM:    displayM,
		simM:        simM,
		uL:          uL,
		cL:          cL,
		lastUpdated: time.Now(),
	}
}

func (p *Personalization) updateMatrices(m *personalizationMatrices) {
	p.Mu.Lock()
	defer p.Mu.Unlock()
	p.pM = m
}

func (p *Personalization) update(ctx context.Context) {
	curObj, err := p.b.Metadata(ctx, gcpObjectName)
	if err != nil && err != storage.ErrObjectNotExist {
		panic(err)
	}

	if err == storage.ErrObjectNotExist {
		p.updateCache(ctx)
		return
	}

	if p.pM == nil {
		logger.For(ctx).Infof("no data loaded, reading from cache")
		p.readCache(ctx)
		return
	}

	staleAt := p.pM.lastUpdated.Add(time.Hour)

	if staleAt.After(time.Now()) {
		logger.For(ctx).Infof("data is still fresh, skipping update")
		return
	}

	if curObj.Updated.Before(staleAt) {
		p.updateCache(ctx)
		return
	}

	p.readCache(ctx)
}

func (p *Personalization) readCache(ctx context.Context) {
	logger.For(ctx).Infof("personalization data is stale, reading from cache")
	now := time.Now()
	r, err := p.b.NewReader(ctx, gcpObjectName)
	check(err)
	defer r.Close()
	var m personalizationMatrices
	m.UnmarshalBinaryFrom(r)
	p.updateMatrices(&m)
	logger.For(ctx).Infof("took %s to read from cache", time.Since(now))
}

func (p *Personalization) updateCache(ctx context.Context) {
	logger.For(ctx).Infof("no data found in cache, updating the cache")
	now := time.Now()
	m := readMatrices(ctx, p.q)
	b, err := m.MarshalBinary()
	check(err)
	_, err = p.b.WriteGzip(ctx, gcpObjectName, b, store.ObjAttrsOptions.WithContentType("application/octet-stream"))
	check(err)
	p.updateMatrices(&m)
	logger.For(ctx).Infof("took %s to update the cache", time.Since(now))
}

// calcSocialScore determines if vIdx is in the same friend circle as qIdx by running a bfs on userM
func calcSocialScore(userM *sparse.CSR, vIdx, qIdx int) float64 {
	score := bfs(userM, vIdx, qIdx)
	return score
}

// calcRelavanceScore computes the relevance of cIdx to vIdx's held tokens
func calcRelevanceScore(ratingM, displayM *sparse.CSR, vIdx, cIdx int) float64 {
	var t float64
	v := ratingM.RowView(vIdx).(*sparse.Vector)
	contractCardinality := displayM.At(cIdx, cIdx)
	v.DoNonZero(func(i int, j int, v float64) {
		// Max score is 1, we can't exit early but we can skip the calculation
		if t == 1 {
			return
		}
		if s := overlapIndex(displayM.At(i, cIdx), displayM.At(i, i), contractCardinality); s > t {
			t = s
		}
	})
	return t
}

// calcSimilarityScore computes the similarity of vIdx and qIdx based on their interactions with other users
func calcSimilarityScore(simM *sparse.CSR, vIdx, qIdx int) float64 {
	return overlapIndex(simM.At(vIdx, qIdx), simM.At(vIdx, vIdx), simM.At(qIdx, qIdx))
}

// idLookup uses a slice to lookup a value by its index
// Its purpose is to avoid using a map because indexing a slice is suprisingly much quicker than a map lookup.
// It should be pretty space efficient: with one million users, it should take: 1 million users * 1 byte = 1MB of memory.
type idLookup struct {
	l []uint8
}

func newIdLookup() idLookup {
	return idLookup{l: make([]uint8, 1)}
}

func (n idLookup) Get(idx int) uint8 {
	if idx >= len(n.l) {
		return 0
	}
	return n.l[idx]
}

func (n *idLookup) Set(idx int, i uint8) {
	appendVal(&n.l, idx, i)
}

func extendBy[T any](s *[]T, i int) {
	if newSize := i + 1; newSize > cap(*s) {
		cp := make([]T, newSize*2)
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
		if depth.Get(cur) > 3 {
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

func overlapIndex(unionAB, cardA, cardB float64) float64 {
	minCard := cardA
	if cardB < cardA {
		minCard = cardB
	}
	return unionAB / minCard
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
