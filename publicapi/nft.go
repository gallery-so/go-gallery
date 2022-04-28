package publicapi

import (
	"context"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/event"
	nftservice "github.com/mikeydub/go-gallery/service/nft"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

type NftAPI struct {
	repos     *persist.Repositories
	queries   *sqlc.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

// ErrOpenSeaRefreshFailed is a generic error that wraps all other OpenSea sync failures.
// Should be removed once we stop using OpenSea to sync NFTs.
type ErrOpenSeaRefreshFailed struct {
	Message string
}

func (e ErrOpenSeaRefreshFailed) Error() string {
	return e.Message
}

func (api NftAPI) GetNftById(ctx context.Context, nftID persist.DBID) (*sqlc.Nft, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"nftID": {nftID, "required"},
	}); err != nil {
		return nil, err
	}

	nft, err := api.loaders.NftByNftId.Load(nftID)
	if err != nil {
		return nil, err
	}

	return &nft, nil
}

func (api NftAPI) GetNftsByCollectionId(ctx context.Context, collectionID persist.DBID) ([]sqlc.Nft, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return nil, err
	}

	nfts, err := api.loaders.NftsByCollectionId.Load(collectionID)
	if err != nil {
		return nil, err
	}

	return nfts, nil
}

func (api NftAPI) GetNftsByOwnerAddress(ctx context.Context, ownerAddress persist.Address) ([]sqlc.Nft, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"ownerAddress": {ownerAddress, "required,eth_addr"},
	}); err != nil {
		return nil, err
	}

	nfts, err := api.loaders.NftsByOwnerAddress.Load(ownerAddress)
	if err != nil {
		return nil, err
	}

	return nfts, nil
}

func (api NftAPI) RefreshOpenSeaNfts(ctx context.Context, addresses string) error {
	// No validation to do here -- addresses is an optional comma-separated list of addresses

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	err = nftservice.GetOpenseaNFTs(ctx, userID, addresses, api.repos.NftRepository, api.repos.UserRepository, api.repos.CollectionRepository, api.repos.GalleryRepository, api.repos.BackupRepository)
	if err != nil {
		// Wrap all OpenSea sync failures in a generic type that can be returned to the frontend as an expected error type
		return ErrOpenSeaRefreshFailed{Message: err.Error()}
	}

	api.loaders.ClearAllCaches()

	return nil
}

func (api NftAPI) UpdateNftInfo(ctx context.Context, nftID persist.DBID, collectionID persist.DBID, collectorsNote string) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"nftID":          {nftID, "required"},
		"collectorsNote": {collectorsNote, "nft_note"},
	}); err != nil {
		return err
	}

	// Sanitize
	collectorsNote = validate.SanitizationPolicy.Sanitize(collectorsNote)

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	update := persist.NFTUpdateInfoInput{
		CollectorsNote: persist.NullString(collectorsNote),
	}

	err = api.repos.NftRepository.UpdateByID(ctx, nftID, userID, update)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()

	// Send event
	nftData := persist.NftEvent{CollectionID: collectionID, CollectorsNote: persist.NullString(collectorsNote)}
	dispatchNftEvent(ctx, persist.NftCollectorsNoteAddedEvent, userID, nftID, nftData)

	return nil
}

func dispatchNftEvent(ctx context.Context, eventCode persist.EventCode, userID persist.DBID, nftID persist.DBID, nftData persist.NftEvent) {
	gc := util.GinContextFromContext(ctx)
	nftHandlers := event.For(gc).Nft
	evt := persist.NftEventRecord{
		UserID: userID,
		NftID:  nftID,
		Code:   eventCode,
		Data:   nftData,
	}

	nftHandlers.Dispatch(ctx, evt)
}
