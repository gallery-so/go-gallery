package publicapi

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

const maxNftsPerCollection = 1000

type CollectionAPI struct {
	repos     *persist.Repositories
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
	pubsub    pubsub.PubSub
}

func (api CollectionAPI) GetCollection(ctx context.Context, collectionID persist.DBID) (*persist.Collection, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return nil, err
	}

	collection, err := api.repos.CollectionRepository.GetByID(ctx, collectionID, false)

	return &collection, err
}

func (api CollectionAPI) CreateCollection(ctx context.Context, galleryID persist.DBID, name string, collectorsNote string, nfts []persist.DBID, layout persist.TokenLayout) (*persist.Collection, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"galleryID":      {galleryID, "required"},
		"name":           {name, "collection_name"},
		"collectorsNote": {collectorsNote, "collection_note"},
		"nfts":           {nfts, fmt.Sprintf("required,unique,max=%d", maxNftsPerCollection)},
	}); err != nil {
		return nil, err
	}

	layout, err := persist.ValidateLayout(layout, nfts)
	if err != nil {
		return nil, err
	}

	// Sanitize
	name = validate.SanitizationPolicy.Sanitize(name)
	collectorsNote = validate.SanitizationPolicy.Sanitize(collectorsNote)

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, err
	}

	collection := persist.CollectionDB{
		OwnerUserID:    userID,
		NFTs:           nfts,
		Layout:         layout,
		Name:           persist.NullString(name),
		CollectorsNote: persist.NullString(collectorsNote),
	}

	collectionID, err := api.repos.CollectionRepository.Create(ctx, collection)
	if err != nil {
		return nil, err
	}

	err = api.repos.GalleryRepository.AddCollections(ctx, galleryID, userID, []persist.DBID{collectionID})
	if err != nil {
		return nil, err
	}

	// TODO: Get a shallow collection instead of a fully unnested one. Can we roll these into a single struct with
	// multiple fields (nftIds, nfts) and assume it's not hydrated if nfts is null? And then maybe include a parameter
	// for whether to hydrate the hierarchy or not?
	createdCollection, err := dataloader.For(ctx).CollectionByCollectionId.Load(collectionID)
	if err != nil {
		return nil, err
	}

	// Send event
	nftIDs := make([]persist.DBID, len(createdCollection.NFTs))
	for i, nft := range createdCollection.NFTs {
		nftIDs[i] = nft.ID
	}
	collectionData := persist.CollectionEvent{NFTs: nftIDs, CollectorsNote: createdCollection.CollectorsNote}
	dispatchCollectionEvent(ctx, persist.CollectionCreatedEvent, userID, createdCollection.ID, collectionData)

	return &createdCollection, nil
}

func (api CollectionAPI) DeleteCollection(ctx context.Context, collectionID persist.DBID) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	return api.repos.CollectionRepository.Delete(ctx, collectionID, userID)
}

func (api CollectionAPI) UpdateCollection(ctx context.Context, collectionID persist.DBID, name string, collectorsNote string) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID":   {collectionID, "required"},
		"name":           {name, "required,collection_name"},
		"collectorsNote": {collectorsNote, "required,collection_note"},
	}); err != nil {
		return err
	}

	// Sanitize
	name = validate.SanitizationPolicy.Sanitize(name)
	collectorsNote = validate.SanitizationPolicy.Sanitize(collectorsNote)

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	update := persist.CollectionUpdateInfoInput{
		Name:           persist.NullString(name),
		CollectorsNote: persist.NullString(collectorsNote),
	}

	// Send event
	collectionData := persist.CollectionEvent{CollectorsNote: persist.NullString(collectorsNote)}
	dispatchCollectionEvent(ctx, persist.CollectionCollectorsNoteAdded, userID, collectionID, collectionData)

	return api.repos.CollectionRepository.Update(ctx, collectionID, userID, update)
}

func (api CollectionAPI) UpdateCollectionNfts(ctx context.Context, collectionID persist.DBID, nfts []persist.DBID, layout persist.TokenLayout) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionID, "required"},
		"nfts":         {nfts, fmt.Sprintf("required,unique,max=%d", maxNftsPerCollection)},
	}); err != nil {
		return err
	}

	layout, err := persist.ValidateLayout(layout, nfts)
	if err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	update := persist.CollectionUpdateNftsInput{NFTs: nfts, Layout: layout}

	err = api.repos.CollectionRepository.UpdateNFTs(ctx, collectionID, userID, update)
	if err != nil {
		return err
	}

	backupGalleriesForUser(ctx, userID, api.repos)

	// Send event
	collectionData := persist.CollectionEvent{NFTs: nfts}
	dispatchCollectionEvent(ctx, persist.CollectionTokensAdded, userID, collectionID, collectionData)

	return nil
}

func dispatchCollectionEvent(ctx context.Context, eventCode persist.EventCode, userID persist.DBID, collectionID persist.DBID, collectionData persist.CollectionEvent) {
	gc := util.GinContextFromContext(ctx)
	collectionHandlers := event.For(gc).Collection
	evt := persist.CollectionEventRecord{
		UserID:       userID,
		CollectionID: collectionID,
		Code:         eventCode,
		Data:         collectionData,
	}

	collectionHandlers.Dispatch(evt)
}
