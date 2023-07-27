//go:build norec

package koala

import (
	"context"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

func NewKoala(context.Context, *db.Queries) *Koala                                  { return &Koala{} }
func (k *Koala) RelevanceTo(persist.DBID, db.FeedEntityScoringRow) (float64, error) { return 1, nil }
func (k *Koala) update(context.Context)                                             {}
