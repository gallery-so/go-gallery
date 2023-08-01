//go:build norec

package userpref

import (
	"context"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

func NewPersonalization(context.Context, *db.Queries) *Personalization { return &Personalization{} }
func (k *Personalization) RelevanceTo(persist.DBID, db.FeedEntityScoringRow) (float64, error) {
	return 1, nil
}
func (k *Personalization) update(context.Context) {}
