package userpref

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/james-bowman/sparse"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/util"
)

const contextKey = "personalization.instance"

var ErrNoInputData = errors.New("no personalization input data")

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
}

func (p personalizationMatrices) MarshalBinary() ([]byte, error) {
	var dataBuf []byte
	marshalMatrixTo(&dataBuf, p.userM)
	marshalMatrixTo(&dataBuf, p.ratingM)
	marshalMatrixTo(&dataBuf, p.displayM)
	marshalMatrixTo(&dataBuf, p.simM)
	marshalLookupTo(&dataBuf, p.uL)
	marshalLookupTo(&dataBuf, p.cL)
	return dataBuf, nil
}

func (p *personalizationMatrices) UnmarshalBinary(data []byte) error {
	panic("not implemented")
}

func marshalMatrixTo(buf *[]byte, m *sparse.CSR) {
	byt, err := m.MarshalBinary()
	check(err)
	appendTo(buf, byt)
}

func marshalLookupTo(buf *[]byte, l map[persist.DBID]int) {
	byt, err := json.Marshal(l)
	check(err)
	appendTo(buf, byt)
}

func appendTo(buf *[]byte, byt []byte) {
	tmp := make([]byte, binary.MaxVarintLen64)
	bytesWritten := binary.PutUvarint(tmp, uint64(len(byt)))
	*buf = append(*buf, tmp[:bytesWritten]...)
	*buf = append(*buf, byt...)
}

type Personalization struct {
	// Handles concurrent access to the matrices
	Mu sync.RWMutex
	// LastUpdated is the time the matrices were last updated
	LastUpdated time.Time
	pM          *personalizationMatrices
	q           *db.Queries
	r           *redis.Cache
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
			select {
			case <-ticker.C:
				k.update(ctx)
			}
		}
	}()
}
