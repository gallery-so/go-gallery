package publicapi

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

type CollectionAPIHandler struct {
	PublicCollectionAPI
	gc                  *gin.Context
	events              chan<- event.EventMessage
	collectionEventRepo persist.CollectionEventRepository
}

func NewCollectionAPIHandler(ctx *gin.Context, collectionAPI PublicCollectionAPI, collectionEventRepo persist.CollectionEventRepository, events chan<- event.EventMessage) *CollectionAPIHandler {
	return &CollectionAPIHandler{
		PublicCollectionAPI: collectionAPI,
		gc:                  ctx,
		events:              events,
		collectionEventRepo: collectionEventRepo,
	}
}

type errFailedToAddToCollectionEventRepo struct {
	err error
}

func (e errFailedToAddToCollectionEventRepo) Error() string {
	return fmt.Sprintf("failed to add to collection event repository: %s", e.err)
}

// Forwarding calls to the wrapped API.

func (c CollectionAPIHandler) CreateCollection(ctx context.Context, galleryID persist.DBID, name string, collectorsNote string, nfts []persist.DBID, layout persist.TokenLayout) (*persist.Collection, error) {
	col, err := c.PublicCollectionAPI.CreateCollection(ctx, galleryID, name, collectorsNote, nfts, layout)
	if err != nil {
		return col, err
	}

	go func() {
		nftIDs := make([]persist.DBID, len(col.NFTs))
		for i, nft := range col.NFTs {
			nftIDs[i] = nft.ID
		}
		record := persist.CollectionEventRecord{
			UserID:       auth.GetUserIDFromCtx(c.gc),
			CollectionID: col.ID,
			Code:         persist.CollectionCreatedEvent,
			Data:         persist.CollectionEvent{NFTs: nftIDs, CollectorsNote: col.CollectorsNote},
		}
		eventID, err := c.collectionEventRepo.Add(ctx, record)
		if err != nil {
			logrus.Error(errFailedToAddToCollectionEventRepo{err})
			return
		}
		c.events <- event.EventMessage{ID: eventID, EventCode: persist.CollectionCreatedEvent}
	}()

	return col, err
}

func (c CollectionAPIHandler) UpdateCollection(ctx context.Context, collectionID persist.DBID, name string, collectorsNote string) error {
	err := c.PublicCollectionAPI.UpdateCollection(ctx, collectionID, name, collectorsNote)
	if err != nil {
		return err
	}

	go func() {
		record := persist.CollectionEventRecord{
			UserID:       auth.GetUserIDFromCtx(c.gc),
			CollectionID: collectionID,
			Code:         persist.CollectionCollectorsNoteAdded,
			Data:         persist.CollectionEvent{CollectorsNote: persist.NullString(collectorsNote)},
		}
		eventID, err := c.collectionEventRepo.Add(ctx, record)
		if err != nil {
			logrus.Error(errFailedToAddToCollectionEventRepo{err})
			return
		}
		c.events <- event.EventMessage{ID: eventID, EventCode: persist.CollectionCreatedEvent}
	}()

	return err
}

func (c CollectionAPIHandler) UpdateCollectionNfts(ctx context.Context, collectionID persist.DBID, nfts []persist.DBID, layout persist.TokenLayout) error {
	err := c.PublicCollectionAPI.UpdateCollectionNfts(ctx, collectionID, nfts, layout)
	if err != nil {
		return err
	}

	go func() {
		record := persist.CollectionEventRecord{
			UserID:       auth.GetUserIDFromCtx(c.gc),
			CollectionID: collectionID,
			Code:         persist.CollectionTokensAdded,
			Data:         persist.CollectionEvent{NFTs: nfts},
		}
		eventID, err := c.collectionEventRepo.Add(ctx, record)
		if err != nil {
			logrus.Error(errFailedToAddToCollectionEventRepo{err})
			return
		}
		c.events <- event.EventMessage{ID: eventID, EventCode: persist.CollectionCreatedEvent}
	}()

	return err
}
