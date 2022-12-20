package publicapi

import (
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
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

	if err := validateFields(api.validator, validationMap{
		"name":        {name, "max=200"},
		"description": {description, "max=600"},
		"position":    {position, "required"},
	}); err != nil {
		return db.Gallery{}, err
	}

	var nullName, nullDesc string
	if name != nil {
		nullName = *name
	}
	if description != nil {
		nullDesc = *description
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return db.Gallery{}, err
	}

	gallery, err := api.repos.GalleryRepository.Create(ctx, db.GalleryRepoCreateParams{
		GalleryID:   persist.GenerateID(),
		Name:        nullName,
		Description: nullDesc,
		Position:    position,
		OwnerUserID: userID,
	})
	if err != nil {
		return db.Gallery{}, err
	}

	return gallery, nil
}

func (api GalleryAPI) DeleteGallery(ctx context.Context, galleryID persist.DBID) error {

	if err := validateFields(api.validator, validationMap{
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
	if err := validateFields(api.validator, validationMap{
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

func (api GalleryAPI) GetGalleryByCollectionId(ctx context.Context, collectionID persist.DBID) (*db.Gallery, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
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
	if err := validateFields(api.validator, validationMap{
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

func (api GalleryAPI) GetTokenPreviewsByGalleryID(ctx context.Context, galleryID persist.DBID) ([]string, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return nil, err
	}

	previews, err := api.queries.GetGalleryTokenPreviewsByID(ctx, galleryID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	asString := make([]string, len(previews))
	for i, preview := range previews {
		asString[i] = preview.(string)
	}

	return asString, nil
}

func (api GalleryAPI) UpdateGalleryCollections(ctx context.Context, galleryID persist.DBID, collections []persist.DBID) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
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
	if err := validateFields(api.validator, validationMap{
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
	if err := validateFields(api.validator, validationMap{
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
	if err := validateFields(api.validator, validationMap{
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
	if err := validateFields(api.validator, validationMap{
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

	// It's possible that there are multiple X-Forwaded-For
	// headers in the request so we first combine it into a single slice.
	forwarded := make([]string, 0)
	for _, header := range gc.Request.Header["X-Forwarded-For"] {
		header = strings.ReplaceAll(header, " ", "")
		proxied := strings.Split(header, ",")
		forwarded = append(forwarded, proxied...)
	}

	// Find the first valid, non-private IP that is
	// closest to the client from left to right.
	for _, address := range forwarded {
		if ip := net.ParseIP(address); ip != nil && !IsPrivate(ip) {
			hash := sha256.New()
			hash.Write([]byte(viper.GetString("BACKEND_SECRET") + ip.String()))
			res, _ := hash.(encoding.BinaryMarshaler).MarshalBinary()
			externalID := base64.StdEncoding.EncodeToString(res)
			return &externalID
		}
	}

	return nil
}

// The below code is modified from the IsPrivate() method of the 'net' standard library package
// available in go versions > 1.17.
//
// TODO: Remove when backend is upgraded from 1.16 in favor of ip.IsPrivate()
//
// IsPrivate reports whether ip is a private address, according to
// RFC 1918 (IPv4 addresses) and RFC 4193 (IPv6 addresses).
func IsPrivate(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		// Following RFC 1918, Section 3. Private Address Space which says:
		//   The Internet Assigned Numbers Authority (IANA) has reserved the
		//   following three blocks of the IP address space for private internets:
		//     10.0.0.0        -   10.255.255.255  (10/8 prefix)
		//     172.16.0.0      -   172.31.255.255  (172.16/12 prefix)
		//     192.168.0.0     -   192.168.255.255 (192.168/16 prefix)
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1]&0xf0 == 16) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	// Following RFC 4193, Section 8. IANA Considerations which says:
	//   The IANA has assigned the FC00::/7 prefix to "Unique Local Unicast".
	IPv6Len := 16
	return len(ip) == IPv6Len && ip[0]&0xfe == 0xfc
}
