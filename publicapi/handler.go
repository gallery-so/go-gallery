package publicapi

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
)

type CollectionAPIHandler struct {
	PublicCollectionAPI
	gc                  *gin.Context
	events              chan<- persist.DBID
	collectionEventRepo persist.CollectionEventRepository
}

func NewCollectionAPIHandler(ctx *gin.Context, collectionAPI PublicCollectionAPI, collectionEventRepo persist.CollectionEventRepository, events chan<- persist.DBID) *CollectionAPIHandler {
	return &CollectionAPIHandler{
		PublicCollectionAPI: collectionAPI,
		gc:                  ctx,
		events:              events,
		collectionEventRepo: collectionEventRepo,
	}
}

func (c CollectionAPIHandler) CreateCollection(ctx context.Context, galleryID persist.DBID, name string, collectorsNote string, nfts []persist.DBID, layout persist.TokenLayout) (*persist.Collection, error) {
	col, err := c.PublicCollectionAPI.CreateCollection(ctx, galleryID, name, collectorsNote, nfts, layout)

	go func() {
		eventID, err := c.collectionEventRepo.Add(ctx, persist.CollectionEventRecord{
			UserID:       auth.GetUserIDFromCtx(c.gc),
			CollectionID: col.ID,
			Type:         persist.CollectionCreatedEvent,
			Data:         persist.CollectionEvent{NFTs: col.NFTs, CollectorsNote: col.CollectorsNote},
		})
		if err != nil {
			// TODO: Log this out.
		}
		c.events <- eventID
	}()

	return col, err
}
