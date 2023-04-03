package publicapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/validate"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

const (
	maxTokensPerCollection         = 1000
	maxSectionsPerCollection       = 100
	currentCollectionSchemaVersion = 1
)

type CollectionAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api CollectionAPI) GetCollectionById(ctx context.Context, collectionID persist.DBID) (*db.Collection, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return nil, err
	}

	collection, err := api.loaders.CollectionByCollectionID.Load(collectionID)
	if err != nil {
		return nil, err
	}

	return &collection, nil
}

func (api CollectionAPI) GetCollectionsByIds(ctx context.Context, collectionIDs []persist.DBID) ([]*db.Collection, []error) {
	collectionThunk := func(collectionID persist.DBID) func() (db.Collection, error) {
		// Validate
		if err := validate.ValidateFields(api.validator, validate.ValidationMap{
			"collectionID": {collectionID, "required"},
		}); err != nil {
			return func() (db.Collection, error) { return db.Collection{}, err }
		}

		return api.loaders.CollectionByCollectionID.LoadThunk(collectionID)
	}

	// A "thunk" will add this request to a batch, and then return a function that will block to fetch
	// data when called. By creating all of the thunks first (without invoking the functions they return),
	// we're setting up a batch that will eventually fetch all of these requests at the same time when
	// their functions are invoked. "LoadAll" would accomplish something similar, but wouldn't let us
	// validate each collectionID parameter first.
	thunks := make([]func() (db.Collection, error), len(collectionIDs))

	for i, collectionID := range collectionIDs {
		thunks[i] = collectionThunk(collectionID)
	}

	collections := make([]*db.Collection, len(collectionIDs))
	errors := make([]error, len(collectionIDs))

	for i, _ := range collectionIDs {
		collection, err := thunks[i]()
		if err == nil {
			collections[i] = &collection
		} else {
			errors[i] = err
		}
	}

	return collections, errors
}

func (api CollectionAPI) GetCollectionsByGalleryId(ctx context.Context, galleryID persist.DBID) ([]db.Collection, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return nil, err
	}

	collections, err := api.loaders.CollectionsByGalleryID.Load(galleryID)
	if err != nil {
		return nil, err
	}

	return collections, nil
}

func (api CollectionAPI) CreateCollection(ctx context.Context, galleryID persist.DBID, name string, collectorsNote string, tokens []persist.DBID, layout persist.TokenLayout, tokenSettings map[persist.DBID]persist.CollectionTokenSettings, caption *string) (*db.Collection, *db.FeedEvent, error) {
	fieldsToValidate := validate.ValidationMap{
		"galleryID":      {galleryID, "required"},
		"name":           {name, "collection_name"},
		"collectorsNote": {collectorsNote, "collection_note"},
		"tokens":         {tokens, fmt.Sprintf("required,unique,min=1,max=%d", maxTokensPerCollection)},
		"sections":       {layout.Sections, fmt.Sprintf("unique,sorted_asc,lte=%d,min=1,max=%d,len=%d,dive,gte=0,lte=%d", len(tokens), maxSectionsPerCollection, len(layout.SectionLayout), len(tokens)-1)},
	}

	// Trim and optimistically sanitize the input while we're at it.
	var trimmedCaption string
	if caption != nil {
		trimmedCaption = strings.TrimSpace(*caption)
		fieldsToValidate["caption"] = validate.ValWithTags{trimmedCaption, fmt.Sprintf("required,caption")}
		cleaned := validate.SanitizationPolicy.Sanitize(trimmedCaption)
		caption = &cleaned
	}

	// Validate
	if err := validate.ValidateFields(api.validator, fieldsToValidate); err != nil {
		return nil, nil, err
	}

	if err := api.validator.Struct(validate.CollectionTokenSettingsParams{
		Tokens:        tokens,
		TokenSettings: tokenSettings,
	}); err != nil {
		return nil, nil, err
	}

	layout, err := persist.ValidateLayout(layout, tokens)
	if err != nil {
		return nil, nil, err
	}

	// Sanitize
	name = validate.SanitizationPolicy.Sanitize(strings.TrimSpace(name))
	collectorsNote = validate.SanitizationPolicy.Sanitize(collectorsNote)

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, nil, err
	}

	err = api.repos.TokenRepository.TokensAreOwnedByUser(ctx, userID, tokens)
	if err != nil {
		return nil, nil, err
	}

	collection := persist.CollectionDB{
		OwnerUserID:    userID,
		Tokens:         tokens,
		GalleryID:      galleryID,
		Layout:         layout,
		Name:           persist.NullString(name),
		CollectorsNote: persist.NullString(collectorsNote),
		TokenSettings:  tokenSettings,
		Version:        currentCollectionSchemaVersion,
	}

	collectionID, err := api.repos.CollectionRepository.Create(ctx, collection)
	if err != nil {
		return nil, nil, err
	}

	err = api.repos.GalleryRepository.AddCollections(ctx, galleryID, userID, []persist.DBID{collectionID})
	if err != nil {
		return nil, nil, err
	}

	createdCollection, err := api.loaders.CollectionByCollectionID.Load(collectionID)
	if err != nil {
		return nil, nil, err
	}

	// Send event
	feedEvent, err := dispatchEvent(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		Action:         persist.ActionCollectionCreated,
		ResourceTypeID: persist.ResourceTypeCollection,
		CollectionID:   collectionID,
		GalleryID:      galleryID,
		SubjectID:      collectionID,
		Data:           persist.EventData{CollectionTokenIDs: createdCollection.Nfts, CollectionCollectorsNote: collectorsNote},
	}, api.validator, caption)
	if err != nil {
		return nil, nil, err
	}

	return &createdCollection, feedEvent, nil
}

func (api CollectionAPI) DeleteCollection(ctx context.Context, collectionID persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	err = api.repos.CollectionRepository.Delete(ctx, collectionID, userID)
	if err != nil {
		return err
	}

	return nil
}

func (api CollectionAPI) UpdateCollectionInfo(ctx context.Context, collectionID persist.DBID, name string, collectorsNote string) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID":   {collectionID, "required"},
		"name":           {name, "collection_name"},
		"collectorsNote": {collectorsNote, "collection_note"},
	}); err != nil {
		return err
	}

	// Sanitize
	name = validate.SanitizationPolicy.Sanitize(name)
	collectorsNote = validate.SanitizationPolicy.Sanitize(collectorsNote)

	userID, err := getAuthenticatedUserID(ctx)
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

	galleryID, err := api.queries.GetGalleryIDByCollectionID(ctx, collectionID)
	if err != nil {
		return err
	}

	// Send event
	_, err = dispatchEvent(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		Action:         persist.ActionCollectorsNoteAddedToCollection,
		ResourceTypeID: persist.ResourceTypeCollection,
		CollectionID:   collectionID,
		GalleryID:      galleryID,
		SubjectID:      collectionID,
		Data:           persist.EventData{CollectionCollectorsNote: collectorsNote},
	}, api.validator, nil)

	return err
}

func (api CollectionAPI) UpdateCollectionTokens(ctx context.Context, collectionID persist.DBID, tokens []persist.DBID, layout persist.TokenLayout, tokenSettings map[persist.DBID]persist.CollectionTokenSettings, caption *string) (*db.FeedEvent, error) {
	fieldsToValidate := validate.ValidationMap{
		"collectionID": {collectionID, "required"},
		"tokens":       {tokens, fmt.Sprintf("required,unique,min=1,max=%d", maxTokensPerCollection)},
		"sections":     {layout.Sections, fmt.Sprintf("unique,sorted_asc,lte=%d,min=1,max=%d,len=%d,dive,gte=0,lte=%d", len(tokens), maxSectionsPerCollection, len(layout.SectionLayout), len(tokens)-1)},
	}

	// Trim and optimistically sanitize the input while we're at it.
	var trimmedCaption string
	if caption != nil {
		trimmedCaption = strings.TrimSpace(*caption)
		fieldsToValidate["caption"] = validate.ValWithTags{trimmedCaption, fmt.Sprintf("required,caption")}
		cleaned := validate.SanitizationPolicy.Sanitize(trimmedCaption)
		caption = &cleaned
	}

	// Validate
	if err := validate.ValidateFields(api.validator, fieldsToValidate); err != nil {
		return nil, err
	}

	if err := api.validator.Struct(validate.CollectionTokenSettingsParams{
		Tokens:        tokens,
		TokenSettings: tokenSettings,
	}); err != nil {
		return nil, err
	}

	layout, err := persist.ValidateLayout(layout, tokens)
	if err != nil {
		return nil, err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = api.repos.TokenRepository.TokensAreOwnedByUser(ctx, userID, tokens)
	if err != nil {
		return nil, err
	}

	update := persist.CollectionUpdateTokensInput{
		Tokens:        tokens,
		Layout:        layout,
		TokenSettings: tokenSettings,
		Version:       currentCollectionSchemaVersion,
	}

	err = api.repos.CollectionRepository.UpdateTokens(ctx, collectionID, userID, update)
	if err != nil {
		return nil, err
	}

	galleryID, err := api.queries.GetGalleryIDByCollectionID(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	// Send event
	return dispatchEvent(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		Action:         persist.ActionTokensAddedToCollection,
		ResourceTypeID: persist.ResourceTypeCollection,
		CollectionID:   collectionID,
		GalleryID:      galleryID,
		SubjectID:      collectionID,
		Data:           persist.EventData{CollectionTokenIDs: tokens},
		Caption:        persist.StrPtrToNullStr(caption),
	}, api.validator, caption)
}

func (api CollectionAPI) UpdateCollectionHidden(ctx context.Context, collectionID persist.DBID, hidden bool) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	update := persist.CollectionUpdateHiddenInput{Hidden: persist.NullBool(hidden)}

	err = api.repos.CollectionRepository.Update(ctx, collectionID, userID, update)
	if err != nil {
		return err
	}

	return nil
}

// UpdateCollectionGallery updates the gallery of a collection and returns the ID of the old gallery.
func (api CollectionAPI) UpdateCollectionGallery(ctx context.Context, collectionID, galleryID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID": {collectionID, "required"},
		"galleryID":    {galleryID, "required"},
	}); err != nil {
		return "", err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	// check ownership
	if ownsCollection, err := api.queries.UserOwnsCollection(ctx, db.UserOwnsCollectionParams{
		ID:          collectionID,
		OwnerUserID: userID,
	}); err != nil {
		return "", err
	} else if !ownsCollection {
		return "", fmt.Errorf("user does not own collection: %s", collectionID)
	}

	if ownsGallery, err := api.queries.UserOwnsGallery(ctx, db.UserOwnsGalleryParams{
		ID:          galleryID,
		OwnerUserID: userID,
	}); err != nil {
		return "", err
	} else if !ownsGallery {
		return "", fmt.Errorf("user does not own gallery: %s", galleryID)
	}

	tx, err := api.repos.BeginTx(ctx)
	if err != nil {
		return "", err
	}

	defer tx.Rollback(ctx)

	q := api.queries.WithTx(tx)

	curCol, err := q.GetCollectionById(ctx, collectionID)
	if err != nil {
		return "", err
	}

	if err := q.UpdateCollectionGallery(ctx, db.UpdateCollectionGalleryParams{
		GalleryID: galleryID,
		ID:        collectionID,
	}); err != nil {
		return "", err
	}

	if err := q.AddCollectionToGallery(ctx, db.AddCollectionToGalleryParams{
		GalleryID:    galleryID,
		CollectionID: collectionID,
	}); err != nil {
		return "", err
	}

	if err := q.RemoveCollectionFromGallery(ctx, db.RemoveCollectionFromGalleryParams{
		GalleryID:    curCol.GalleryID,
		CollectionID: collectionID,
	}); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	return curCol.GalleryID, nil
}
