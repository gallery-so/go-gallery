package publicapi

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

const maxNftsPerCollection = 1000

type CollectionAPI struct {
	repos     *persist.Repositories
	queries   *sqlc.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api CollectionAPI) GetCollectionById(ctx context.Context, collectionID persist.DBID) (*sqlc.Collection, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return nil, err
	}

	collection, err := api.loaders.CollectionByCollectionId.Load(collectionID)
	if err != nil {
		return nil, err
	}

	return &collection, nil
}

func (api CollectionAPI) GetCollectionsByGalleryId(ctx context.Context, galleryID persist.DBID) ([]sqlc.Collection, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return nil, err
	}

	collections, err := api.loaders.CollectionsByGalleryId.Load(galleryID)
	if err != nil {
		return nil, err
	}

	return collections, nil
}

func (api CollectionAPI) CreateCollection(ctx context.Context, galleryID persist.DBID, name string, collectorsNote string, nfts []persist.DBID, layout persist.TokenLayout) (*sqlc.Collection, error) {
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

	api.loaders.ClearAllCaches()

	createdCollection, err := api.loaders.CollectionByCollectionId.Load(collectionID)
	if err != nil {
		return nil, err
	}

	// Send event
	collectionData := persist.CollectionEvent{NFTs: createdCollection.Nfts, CollectorsNote: persist.NullString(createdCollection.CollectorsNote.String)}
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

	err = api.repos.CollectionRepository.Delete(ctx, collectionID, userID)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()

	return nil
}

func (api CollectionAPI) UpdateCollectionInfo(ctx context.Context, collectionID persist.DBID, name string, collectorsNote string) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID":   {collectionID, "required"},
		"name":           {name, "collection_name"},
		"collectorsNote": {collectorsNote, "collection_note"},
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

	err = api.repos.CollectionRepository.Update(ctx, collectionID, userID, update)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()

	// Send event
	collectionData := persist.CollectionEvent{CollectorsNote: persist.NullString(collectorsNote)}
	dispatchCollectionEvent(ctx, persist.CollectionCollectorsNoteAdded, userID, collectionID, collectionData)

	return nil
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

	api.loaders.ClearAllCaches()
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
