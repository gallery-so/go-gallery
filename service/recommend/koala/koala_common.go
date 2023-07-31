package koala

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/james-bowman/sparse"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const contextKey = "personalization.instance"

var ErrNoInputData = errors.New("no personalization input data")

type Koala struct {
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
	Mu sync.RWMutex
	q  *db.Queries
}

func AddTo(c *gin.Context, k *Koala) {
	c.Set(contextKey, k)
}

func For(ctx context.Context) *Koala {
	gc := util.MustGetGinContext(ctx)
	return gc.Value(contextKey).(*Koala)
}

// Loop is the main event loop that updates the personalization matrices
func (k *Koala) Loop(ctx context.Context, ticker *time.Ticker) {
	go func() {
		for {
			select {
			case <-ticker.C:
				k.update(ctx)
			}
		}
	}()
}
