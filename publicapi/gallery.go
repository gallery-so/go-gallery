package publicapi

import (
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"

	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/spf13/viper"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
)

const maxCollectionsPerGallery = 1000

type GalleryAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api GalleryAPI) CreateGallery(ctx context.Context, name, description *string, position string) (db.Gallery, error) {

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"name":        {name, "max=200"},
		"description": {description, "max=600"},
		"position":    {position, "required"},
	}); err != nil {
		return db.Gallery{}, err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return db.Gallery{}, err
	}

	gallery, err := api.repos.GalleryRepository.Create(ctx, db.GalleryRepoCreateParams{
		GalleryID:   persist.GenerateID(),
		Name:        util.FromPointer(name),
		Description: util.FromPointer(description),
		Position:    position,
		OwnerUserID: userID,
	})
	if err != nil {
		return db.Gallery{}, err
	}

	return gallery, nil
}

func (api GalleryAPI) UpdateGallery(ctx context.Context, update model.UpdateGalleryInput) (db.Gallery, error) {

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID":           {update.GalleryID, "required"},
		"name":                {update.Name, "max=200"},
		"description":         {update.Description, "max=600"},
		"deleted_collections": {update.DeletedCollections, "unique"},
		"created_collections": {update.CreatedCollections, "created_collections"},
	}); err != nil {
		return db.Gallery{}, err
	}

	curGal, err := api.loaders.GalleryByGalleryID.Load(update.GalleryID)
	if err != nil {
		return db.Gallery{}, err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return db.Gallery{}, err
	}

	if curGal.OwnerUserID != userID {
		return db.Gallery{}, fmt.Errorf("user %s is not the owner of gallery %s", userID, update.GalleryID)
	}

	tx, err := api.repos.BeginTx(ctx)
	if err != nil {
		return db.Gallery{}, err
	}
	defer tx.Rollback(ctx)

	q := api.queries.WithTx(tx)

	// then delete collections
	if len(update.DeletedCollections) > 0 {
		err = q.DeleteCollections(ctx, util.StringersToStrings(update.DeletedCollections))
		if err != nil {
			return db.Gallery{}, err
		}
	}

	// update collections

	if len(update.UpdatedCollections) > 0 {
		err = updateCollectionsInfoAndTokens(ctx, q, update.UpdatedCollections)
		if err != nil {
			return db.Gallery{}, err
		}
	}

	// create collections
	mappedIDs := make(map[persist.DBID]persist.DBID)
	for _, c := range update.CreatedCollections {
		collectionID, err := q.CreateCollection(ctx, db.CreateCollectionParams{
			ID:             persist.GenerateID(),
			Name:           persist.StrToNullStr(&c.Name),
			CollectorsNote: persist.StrToNullStr(&c.CollectorsNote),
			OwnerUserID:    curGal.OwnerUserID,
			GalleryID:      update.GalleryID,
			Layout:         modelToTokenLayout(c.Layout),
			Hidden:         c.Hidden,
			Nfts:           c.Tokens,
			TokenSettings:  modelToTokenSettings(c.TokenSettings),
		})
		if err != nil {
			return db.Gallery{}, err
		}
		mappedIDs[c.GivenID] = collectionID
	}

	// order collections
	for i, c := range update.Order {
		if newID, ok := mappedIDs[c]; ok {
			update.Order[i] = newID
		}
	}

	params := db.UpdateGalleryParams{
		GalleryID: update.GalleryID,
	}

	asList := persist.DBIDList(update.Order)

	setConditionalValue(update.Name, &params.Name, &params.NameUpdated)
	setConditionalValue(update.Description, &params.Description, &params.DescriptionUpdated)
	setConditionalValue(&asList, &params.Collections, &params.CollectionsUpdated)

	err = q.UpdateGallery(ctx, params)
	if err != nil {
		return db.Gallery{}, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return db.Gallery{}, err
	}

	newGall, err := api.loaders.GalleryByGalleryID.Load(update.GalleryID)
	if err != nil {
		return db.Gallery{}, err
	}

	return newGall, nil
}

func updateCollectionsInfoAndTokens(ctx context.Context, q *db.Queries, update []*model.UpdateCollectionInput) error {
	dbids, err := util.Map(update, func(u *model.UpdateCollectionInput) (string, error) {
		return u.Dbid.String(), nil
	})
	if err != nil {
		return err
	}

	collectorNotes, err := util.Map(update, func(u *model.UpdateCollectionInput) (string, error) {
		return u.CollectorsNote, nil
	})
	if err != nil {
		return err
	}

	layouts, err := util.Map(update, func(u *model.UpdateCollectionInput) (pgtype.JSONB, error) {
		b, err := json.Marshal(modelToTokenLayout(u.Layout))
		if err != nil {
			return pgtype.JSONB{
				Status: pgtype.Null,
			}, err
		}

		return pgtype.JSONB{
			Bytes:  b,
			Status: pgtype.Present,
		}, nil
	})
	if err != nil {
		return err
	}

	tokenSettings, err := util.Map(update, func(u *model.UpdateCollectionInput) (pgtype.JSONB, error) {
		settings := modelToTokenSettings(u.TokenSettings)
		b, err := json.Marshal(settings)
		if err != nil {
			return pgtype.JSONB{
				Status: pgtype.Null,
			}, err
		}
		return pgtype.JSONB{
			Bytes:  b,
			Status: pgtype.Present,
		}, nil
	})
	if err != nil {
		return err
	}

	hiddens, err := util.Map(update, func(u *model.UpdateCollectionInput) (bool, error) {
		return u.Hidden, nil
	})
	if err != nil {
		return err
	}

	names, err := util.Map(update, func(u *model.UpdateCollectionInput) (string, error) {
		return u.Name, nil
	})
	if err != nil {
		return err
	}

	err = q.UpdateCollectionsInfo(ctx, db.UpdateCollectionsInfoParams{
		Ids:             dbids,
		Names:           names,
		CollectorsNotes: collectorNotes,
		Layouts:         layouts,
		TokenSettings:   tokenSettings,
		Hidden:          hiddens,
	})
	if err != nil {
		return err
	}

	for _, collection := range update {
		err = q.UpdateCollectionTokens(ctx, db.UpdateCollectionTokensParams{
			ID:   collection.Dbid,
			Nfts: collection.Tokens,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (api GalleryAPI) DeleteGallery(ctx context.Context, galleryID persist.DBID) error {

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	err = api.repos.GalleryRepository.Delete(ctx, db.GalleryRepoDeleteParams{
		GalleryID:   galleryID,
		OwnerUserID: userID,
	})
	if err != nil {
		return err
	}

	return nil
}

func (api GalleryAPI) GetGalleryById(ctx context.Context, galleryID persist.DBID) (*db.Gallery, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return nil, err
	}

	gallery, err := api.loaders.GalleryByGalleryID.Load(galleryID)
	if err != nil {
		return nil, err
	}

	return &gallery, nil
}

func (api GalleryAPI) GetViewerGalleryById(ctx context.Context, galleryID persist.DBID) (*db.Gallery, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return nil, err
	}

	userID, err := getAuthenticatedUser(ctx)

	if err != nil {
		return nil, persist.ErrGalleryNotFound{ID: galleryID}
	}

	gallery, err := api.loaders.GalleryByGalleryID.Load(galleryID)
	if err != nil {
		return nil, err
	}

	if userID != gallery.OwnerUserID {
		return nil, persist.ErrGalleryNotFound{ID: galleryID}
	}

	return &gallery, nil
}

func (api GalleryAPI) GetGalleryByCollectionId(ctx context.Context, collectionID persist.DBID) (*db.Gallery, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return nil, err
	}

	gallery, err := api.loaders.GalleryByCollectionID.Load(collectionID)
	if err != nil {
		return nil, err
	}

	return &gallery, nil
}

func (api GalleryAPI) GetGalleriesByUserId(ctx context.Context, userID persist.DBID) ([]db.Gallery, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	galleries, err := api.loaders.GalleriesByUserID.Load(userID)
	if err != nil {
		return nil, err
	}

	return galleries, nil
}

func (api GalleryAPI) GetTokenPreviewsByGalleryID(ctx context.Context, galleryID persist.DBID) ([]persist.Media, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return nil, err
	}

	previews, err := api.queries.GetGalleryTokenMediasByGalleryID(ctx, galleryID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	medias := make([]persist.Media, len(previews))
	for i, preview := range previews {
		var media persist.Media
		err = preview.AssignTo(&media)
		if err != nil {
			return nil, err
		}
		medias[i] = media
	}

	return medias, nil
}

func (api GalleryAPI) UpdateGalleryCollections(ctx context.Context, galleryID persist.DBID, collections []persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID":   {galleryID, "required"},
		"collections": {collections, fmt.Sprintf("required,unique,max=%d", maxCollectionsPerGallery)},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	update := persist.GalleryTokenUpdateInput{Collections: collections}

	err = api.repos.GalleryRepository.Update(ctx, galleryID, userID, update)
	if err != nil {
		return err
	}

	return nil
}

func (api GalleryAPI) UpdateGalleryInfo(ctx context.Context, galleryID persist.DBID, name, description *string) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID":   {galleryID, "required"},
		"name":        {name, "max=200"},
		"description": {description, "max=600"},
	}); err != nil {
		return err
	}

	var nullName, nullDesc string
	if name != nil {
		nullName = *name
	}
	if description != nil {
		nullDesc = *description
	}

	err := api.queries.UpdateGalleryInfo(ctx, db.UpdateGalleryInfoParams{
		ID:          galleryID,
		Name:        nullName,
		Description: nullDesc,
	})
	if err != nil {
		return err
	}
	return nil
}

func (api GalleryAPI) UpdateGalleryHidden(ctx context.Context, galleryID persist.DBID, hidden bool) (coredb.Gallery, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return db.Gallery{}, err
	}

	gallery, err := api.queries.UpdateGalleryHidden(ctx, db.UpdateGalleryHiddenParams{
		ID:     galleryID,
		Hidden: hidden,
	})
	if err != nil {
		return db.Gallery{}, err
	}

	return gallery, nil
}

func (api GalleryAPI) UpdateGalleryPositions(ctx context.Context, positions []*model.GalleryPositionInput) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"positions": {positions, "required,min=1"},
	}); err != nil {
		return err
	}

	ids := make([]string, len(positions))
	pos := make([]string, len(positions))
	for i, position := range positions {
		ids[i] = position.GalleryID.String()
		pos[i] = position.Position
	}

	err := api.queries.UpdateGalleryPositions(ctx, db.UpdateGalleryPositionsParams{
		GalleryIds: ids,
		Positions:  pos,
	})
	if err != nil {
		return err
	}

	return nil
}

func (api GalleryAPI) ViewGallery(ctx context.Context, galleryID persist.DBID) (db.Gallery, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return db.Gallery{}, err
	}

	gallery, err := api.loaders.GalleryByGalleryID.Load(galleryID)
	if err != nil {
		return db.Gallery{}, err
	}

	gc := util.GinContextFromContext(ctx)

	if auth.GetUserAuthedFromCtx(gc) {
		userID, err := getAuthenticatedUser(ctx)
		if err != nil {
			return db.Gallery{}, err
		}

		// if gallery.OwnerUserID != userID {
		// only view gallery if the user hasn't already viewed it in this most recent notification period

		_, err = dispatchEvent(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(userID),
			ResourceTypeID: persist.ResourceTypeGallery,
			SubjectID:      galleryID,
			Action:         persist.ActionViewedGallery,
			GalleryID:      galleryID,
		}, api.validator, nil)
		if err != nil {
			return db.Gallery{}, err
		}
		// }
	} else {
		_, err := dispatchEvent(ctx, db.Event{
			ResourceTypeID: persist.ResourceTypeGallery,
			SubjectID:      galleryID,
			Action:         persist.ActionViewedGallery,
			GalleryID:      galleryID,
			ExternalID:     persist.StrToNullStr(getExternalID(ctx)),
		}, api.validator, nil)
		if err != nil {
			return db.Gallery{}, err
		}
	}

	return gallery, nil
}

func getExternalID(ctx context.Context) *string {
	gc := util.GinContextFromContext(ctx)
	if ip := net.ParseIP(gc.ClientIP()); ip != nil && !ip.IsPrivate() {
		hash := sha256.New()
		hash.Write([]byte(viper.GetString("BACKEND_SECRET") + ip.String()))
		res, _ := hash.(encoding.BinaryMarshaler).MarshalBinary()
		externalID := base64.StdEncoding.EncodeToString(res)
		return &externalID
	}
	return nil
}

func modelToTokenLayout(u *model.CollectionLayoutInput) persist.TokenLayout {
	sectionLayout := make([]persist.CollectionSectionLayout, len(u.SectionLayout))
	for i, layout := range u.SectionLayout {
		sectionLayout[i] = persist.CollectionSectionLayout{
			Columns:    persist.NullInt32(layout.Columns),
			Whitespace: layout.Whitespace,
		}
	}
	return persist.TokenLayout{
		Sections:      persist.StandardizeCollectionSections(u.Sections),
		SectionLayout: sectionLayout,
	}
}

func modelToTokenSettings(u []*model.CollectionTokenSettingsInput) map[persist.DBID]persist.CollectionTokenSettings {
	settings := make(map[persist.DBID]persist.CollectionTokenSettings)
	for _, tokenSetting := range u {
		settings[tokenSetting.TokenID] = persist.CollectionTokenSettings{RenderLive: tokenSetting.RenderLive}
	}
	return settings
}
