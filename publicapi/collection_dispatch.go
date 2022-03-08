package publicapi

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/persist"
)

type CollectionWithDispatch struct {
	PublicCollectionAPI
	gc *gin.Context
}

// Forwarding methods to wrapped API
func (c CollectionWithDispatch) CreateCollection(ctx context.Context, galleryID persist.DBID, name string, collectorsNote string, nfts []persist.DBID, layout persist.TokenLayout) (*persist.Collection, error) {
	col, err := c.PublicCollectionAPI.CreateCollection(ctx, galleryID, name, collectorsNote, nfts, layout)
	if err != nil {
		return col, err
	}

	nftIDs := make([]persist.DBID, len(col.NFTs))
	for i, nft := range col.NFTs {
		nftIDs[i] = nft.ID
	}
	evt := persist.CollectionEventRecord{
		UserID:       auth.GetUserIDFromCtx(c.gc),
		CollectionID: col.ID,
		Code:         persist.CollectionCreatedEvent,
		Data:         persist.CollectionEvent{NFTs: nftIDs, CollectorsNote: col.CollectorsNote},
	}

	collectionHandlers := event.For(c.gc).Collection
	collectionHandlers.Dispatch(evt)

	return col, err
}

func (c CollectionWithDispatch) UpdateCollection(ctx context.Context, collectionID persist.DBID, name string, collectorsNote string) error {
	err := c.PublicCollectionAPI.UpdateCollection(ctx, collectionID, name, collectorsNote)
	if err != nil {
		return err
	}

	evt := persist.CollectionEventRecord{
		UserID:       auth.GetUserIDFromCtx(c.gc),
		CollectionID: collectionID,
		Code:         persist.CollectionCollectorsNoteAdded,
		Data:         persist.CollectionEvent{CollectorsNote: persist.NullString(collectorsNote)},
	}

	collectionHandlers := event.For(c.gc).Collection
	collectionHandlers.Dispatch(evt)

	return err
}

func (c CollectionWithDispatch) UpdateCollectionNfts(ctx context.Context, collectionID persist.DBID, nfts []persist.DBID, layout persist.TokenLayout) error {
	err := c.PublicCollectionAPI.UpdateCollectionNfts(ctx, collectionID, nfts, layout)
	if err != nil {
		return err
	}

	evt := persist.CollectionEventRecord{
		UserID:       auth.GetUserIDFromCtx(c.gc),
		CollectionID: collectionID,
		Code:         persist.CollectionTokensAdded,
		Data:         persist.CollectionEvent{NFTs: nfts},
	}

	collectionHandlers := event.For(c.gc).Collection
	collectionHandlers.Dispatch(evt)

	return err
}
