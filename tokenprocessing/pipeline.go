package tokenprocessing

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type tokenProcessor struct {
	queries       *coredb.Queries
	ethClient     *ethclient.Client
	mc            *multichain.Provider
	ipfsClient    *shell.Shell
	arweaveClient *goar.Client
	stg           *storage.Client
	tokenBucket   string
	tokenRepo     *postgres.TokenGalleryRepository
}

func NewTokenProcessor(queries *coredb.Queries, ethClient *ethclient.Client, mc *multichain.Provider, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, tokenRepo *postgres.TokenGalleryRepository) *tokenProcessor {
	return &tokenProcessor{
		queries:       queries,
		ethClient:     ethClient,
		mc:            mc,
		ipfsClient:    ipfsClient,
		arweaveClient: arweaveClient,
		stg:           stg,
		tokenBucket:   tokenBucket,
		tokenRepo:     tokenRepo,
	}
}

type tokenProcessingJob struct {
	tp *tokenProcessor

	id           persist.DBID
	key          string
	token        persist.TokenGallery
	contract     persist.ContractGallery
	ownerAddress persist.Address

	cause            persist.ProcessingCause
	pipelineMetadata *persist.PipelineMetadata
}

func (tp *tokenProcessor) ProcessTokenPipeline(c context.Context, t persist.TokenGallery, contract persist.ContractGallery, ownerAddress persist.Address, cause persist.ProcessingCause) error {

	runID := persist.GenerateID()

	job := &tokenProcessingJob{
		id: runID,

		tp:           tp,
		key:          persist.NewTokenIdentifiers(contract.Address, t.TokenID, t.Chain).String(),
		token:        t,
		contract:     contract,
		ownerAddress: ownerAddress,

		cause:            cause,
		pipelineMetadata: new(persist.PipelineMetadata),
	}

	loggerCtx := logger.NewContextWithFields(c, logrus.Fields{
		"tokenDBID":       t.ID,
		"tokenID":         t.TokenID,
		"contractDBID":    t.Contract,
		"contractAddress": contract.Address,
		"chain":           t.Chain,
		"runID":           runID,
	})

	ctx, cancel := context.WithTimeout(loggerCtx, time.Minute*5)
	defer cancel()

	totalTime := time.Now()
	defer func() {
		logger.For(ctx).Infof("token processing job complete: total time for token processing job: %s", time.Since(totalTime))
	}()

	return job.run(ctx)
}

func (tpj *tokenProcessingJob) run(ctx context.Context) error {
	tmedia, err := tpj.createMediaForToken(ctx)
	if err != nil {
		logger.For(ctx).Errorf("error creating media for token: %s", err)
	}

	return tpj.persistResults(ctx, tmedia)
}

func (tpj *tokenProcessingJob) createMediaForToken(ctx context.Context) (coredb.TokenMedia, error) {

	result := coredb.TokenMedia{
		ID:              persist.GenerateID(),
		ContractID:      tpj.token.Contract,
		TokenID:         tpj.token.TokenID,
		Chain:           tpj.token.Chain,
		Active:          true,
		ProcessingJobID: tpj.id,
	}

	result.Metadata = tpj.retrieveMetadata(ctx)

	result.Name, result.Description = tpj.retrieveTokenInfo(ctx, result.Metadata)

	cachedObjects, err := tpj.cacheMediaObjects(ctx, result.Metadata)
	if err != nil {
		isSpam := tpj.contract.IsProviderMarkedSpam || util.GetOptionalValue(tpj.token.IsProviderMarkedSpam, false) || util.GetOptionalValue(tpj.token.IsUserMarkedSpam, false)
		switch err := err.(type) {
		case errNotCacheable:
			// this error is returned for media types that we can not properly render if we cache, but it does not mean that we cached nothing
			// we could have cached a thumbnail or something else alongside the other media.

			first, ok := util.FindFirst(cachedObjects, func(c cachedMediaObject) bool {
				if c.ObjectType == 0 || c.MediaType == "" || c.TokenID == "" || c.ContractAddress == "" {
					return false
				}
				return c.storageURL(tpj.tp.tokenBucket) != "" && (c.ObjectType == objectTypeImage || c.ObjectType == objectTypeSVG || c.ObjectType == objectTypeThumbnail)
			})
			if ok {
				result.Media = tpj.createRawMedia(ctx, err.MediaType, err.URL, first.storageURL(tpj.tp.tokenBucket), cachedObjects)
			} else {
				logger.For(ctx).Errorf("error caching media: %s", err)
				result.Media = persist.Media{MediaType: err.MediaType, MediaURL: persist.NullString(err.URL)}
			}
		case MediaProcessingError:

			if err.AnimationError != nil {
				if errInvalidMedia, ok := err.AnimationError.(errInvalidMedia); ok {
					result.Media = persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(errInvalidMedia.URL)}
				}
			} else if err.ImageError != nil {
				if errInvalidMedia, ok := err.ImageError.(errInvalidMedia); ok {
					result.Media = persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(errInvalidMedia.URL)}
				}
			}
			reportTokenError(ctx, err, tpj.id, tpj.token.Chain, tpj.contract.Address, tpj.token.TokenID, isSpam)
		case errInvalidMedia:
			result.Media = persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(err.URL)}
			reportTokenError(ctx, err, tpj.id, tpj.token.Chain, tpj.contract.Address, tpj.token.TokenID, isSpam)
		default:
			defer persist.TrackStepStatus(&tpj.pipelineMetadata.SetUnknownMediaType, "SetUnknownMediaType")()
			result.Media = persist.Media{MediaType: persist.MediaTypeUnknown}
			reportTokenError(ctx, err, tpj.id, tpj.token.Chain, tpj.contract.Address, tpj.token.TokenID, isSpam)
		}

		if result.Media.MediaType == "" {
			result.Media.MediaType = persist.MediaTypeUnknown
		}

		return result, err
	}

	result.Media = tpj.createMediaFromCachedObjects(ctx, cachedObjects)

	return result, nil
}

func (tpj *tokenProcessingJob) retrieveMetadata(ctx context.Context) persist.TokenMetadata {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.MetadataRetrieval, "MetadataRetrieval")()

	newMetadata := tpj.token.TokenMetadata

	if len(newMetadata) == 0 || tpj.cause == persist.ProcessingCauseRefresh {
		mcMetadata, err := tpj.tp.mc.GetTokenMetadataByTokenIdentifiers(ctx, tpj.contract.Address, tpj.token.TokenID, tpj.ownerAddress, tpj.token.Chain)
		if err != nil {
			logger.For(ctx).Errorf("error getting metadata from chain: %s", err)
			persist.FailStep(&tpj.pipelineMetadata.MetadataRetrieval)
		} else if mcMetadata != nil && len(mcMetadata) > 0 {
			logger.For(ctx).Infof("got metadata from chain: %v", mcMetadata)
			newMetadata = mcMetadata
		}
	}

	if len(newMetadata) == 0 {
		persist.FailStep(&tpj.pipelineMetadata.MetadataRetrieval)
	}

	return newMetadata
}

func (tpj *tokenProcessingJob) retrieveTokenInfo(ctx context.Context, metadata persist.TokenMetadata) (string, string) {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.TokenInfoRetrieval, "TokenInfoRetrieval")()

	name, description := findNameAndDescription(ctx, metadata)

	if name == "" {
		name = tpj.token.Name.String()
	}

	if description == "" {
		description = tpj.token.Description.String()
	}
	return name, description
}

func (tpj *tokenProcessingJob) cacheMediaObjects(ctx context.Context, metadata persist.TokenMetadata) ([]cachedMediaObject, error) {
	return cacheObjectsForMetadata(ctx, metadata, tpj.contract.Address, persist.TokenID(tpj.token.TokenID.String()), tpj.token.TokenURI, tpj.token.Chain, tpj.tp.ipfsClient, tpj.tp.arweaveClient, tpj.tp.stg, tpj.tp.tokenBucket, tpj.pipelineMetadata)
}

func (tpj *tokenProcessingJob) createMediaFromCachedObjects(ctx context.Context, objects []cachedMediaObject) persist.Media {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.CreateMediaFromCachedObjects, "CreateMediaFromCachedObjects")()
	in := map[objectType]cachedMediaObject{}
	for _, obj := range objects {
		in[obj.ObjectType] = obj
	}
	return createMediaFromCachedObjects(ctx, tpj.tp.tokenBucket, in)
}

func (tpj *tokenProcessingJob) createRawMedia(ctx context.Context, mediaType persist.MediaType, animURL, imgURL string, objects []cachedMediaObject) persist.Media {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.CreateRawMedia, "CreateRawMedia")()
	return createRawMedia(ctx, persist.NewTokenIdentifiers(tpj.contract.Address, tpj.token.TokenID, tpj.token.Chain), mediaType, tpj.tp.tokenBucket, animURL, imgURL, objects)
}

func (tpj *tokenProcessingJob) isNewMediaPreferable(ctx context.Context, media persist.Media) bool {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.MediaResultComparison, "MediaResultComparison")()
	if media.IsServable() || (!media.IsServable() && !tpj.token.Media.IsServable()) {
		// if the media is good, it is active
		// if the media is bad but the old media is also bad, it is active
		return true
	}
	// any other case, the media is not active
	return false
}

func (tpj *tokenProcessingJob) persistResults(ctx context.Context, tmetadata coredb.TokenMedia) error {
	if !tpj.isNewMediaPreferable(ctx, tmetadata.Media) {
		tmetadata.Active = false
	}

	return tpj.upsertDB(ctx, tmetadata)

}

func (tpj *tokenProcessingJob) upsertDB(ctx context.Context, tmetadata coredb.TokenMedia) error {

	p := persist.TokenProperties{
		HasMetadata:     tmetadata.Metadata != nil && len(tmetadata.Metadata) > 0,
		HasPrimaryMedia: tmetadata.Media.MediaType.IsValid() && tmetadata.Media.MediaURL != "",
		HasThumbnail:    tmetadata.Media.ThumbnailURL != "",
		HasLiveRender:   tmetadata.Media.LivePreviewURL != "",
		HasDimensions:   tmetadata.Media.Dimensions.Valid(),
		HasName:         tmetadata.Name != "",
		HasDescription:  tmetadata.Description != "",
	}

	job, err := tpj.tp.queries.InsertJob(ctx, coredb.InsertJobParams{
		ProcessingJobID:  tpj.id,
		TokenProperties:  p,
		PipelineMetadata: *tpj.pipelineMetadata,
		ProcessingCause:  tpj.cause,
		ProcessorVersion: "",
	})
	if err != nil {
		logger.For(ctx).Errorf("error inserting job: %s", err)
		return fmt.Errorf("error inserting job: %w", err)
	}

	newID := persist.GenerateID()
	med, err := tpj.tp.queries.UpsertTokenMedia(ctx, coredb.UpsertTokenMediaParams{
		CopyID:          persist.GenerateID(),
		NewID:           newID,
		ContractID:      tpj.token.Contract,
		TokenID:         tpj.token.TokenID,
		Chain:           tpj.token.Chain,
		Metadata:        tmetadata.Metadata,
		Media:           tmetadata.Media,
		Name:            tmetadata.Name,
		Description:     tmetadata.Description,
		ProcessingJobID: job.ID,
		Active:          tmetadata.Active,
	})
	if err != nil {
		logger.For(ctx).Errorf("error upserting token media: %s", err)
		return fmt.Errorf("error upserting token media: %w", err)
	}
	logger.For(ctx).Infof("upserted token media: %s", med.ID)
	if med.Active && newID == med.ID {
		err := tpj.tp.queries.UpdateTokenTokenMediaByTokenIdentifiers(ctx, coredb.UpdateTokenTokenMediaByTokenIdentifiersParams{
			TokenMediaID: med.ID,
			Contract:     tpj.token.Contract,
			TokenID:      tpj.token.TokenID,
			Chain:        tpj.token.Chain,
		})
		if err != nil {
			logger.For(ctx).Errorf("error updating token token_media: %s", err)
			return fmt.Errorf("error updating token token_media: %w", err)
		}
	}
	return nil
}
