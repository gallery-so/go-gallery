package tokenprocessing

import (
	"context"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
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

func newTokenProcessor(queries *coredb.Queries, ethClient *ethclient.Client, mc *multichain.Provider, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, tokenRepo *postgres.TokenGalleryRepository) *tokenProcessor {
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

	id                persist.DBID
	key               string
	token             persist.TokenGallery
	contract          persist.ContractGallery
	ownerAddress      persist.Address
	imageKeywords     []string
	animationKeywords []string
	cause             persist.ProcessingCause
	pipelineMetadata  *persist.PipelineMetadata
}

func (tp *tokenProcessor) processTokenPipeline(c context.Context, key string, t persist.TokenGallery, contract persist.ContractGallery, ownerAddress persist.Address, imageKeywords, animationKeywords []string, cause persist.ProcessingCause) error {

	runID := persist.GenerateID()

	job := &tokenProcessingJob{
		id: runID,

		tp:                tp,
		key:               key,
		token:             t,
		contract:          contract,
		ownerAddress:      ownerAddress,
		imageKeywords:     imageKeywords,
		animationKeywords: animationKeywords,
		cause:             cause,
		pipelineMetadata:  new(persist.PipelineMetadata),
	}

	loggerCtx := logger.NewContextWithFields(c, logrus.Fields{
		"tokenDBID":       t.ID,
		"tokenID":         t.TokenID,
		"contractDBID":    t.Contract,
		"contractAddress": contract.Address,
		"chain":           t.Chain,
		"runID":           runID,
	})

	ctx, cancel := context.WithTimeout(loggerCtx, time.Minute*10)
	defer cancel()

	totalTime := time.Now()
	defer func() {
		logger.For(ctx).Infof("total time for token processing job: %s", time.Since(totalTime))
	}()

	return job.run(ctx)
}

func (tpj *tokenProcessingJob) run(ctx context.Context) error {
	toInsert, err := tpj.createMediaForToken(ctx)
	if err != nil {
		logger.For(ctx).Errorf("error creating media for token: %s", err)
	}

	return tpj.persistMedia(ctx, toInsert)
}

func (tpj *tokenProcessingJob) createMediaForToken(ctx context.Context) (coredb.TokenMedium, error) {
	defer func() {
		if r := recover(); r != nil {
			logger.For(ctx).Errorf("panic in createMediaForToken: %s", r)
		}
	}()

	result := coredb.TokenMedium{
		ID:              persist.GenerateID(),
		Contract:        tpj.token.Contract,
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
		reportTokenError(ctx, err, tpj.id, tpj.token.Chain, tpj.contract.Address, tpj.token.TokenID, isSpam)
		switch err.(type) {
		case media.ErrNotCacheable:
			errNotCacheable := err.(media.ErrNotCacheable)
			first, _ := util.FindFirst(cachedObjects, func(c media.CachedMediaObject) bool {
				return c.StorageURL(tpj.tp.tokenBucket) != ""
			})
			result.Media = tpj.createRawMedia(ctx, errNotCacheable.MediaType, errNotCacheable.URL, first.StorageURL(tpj.tp.tokenBucket), cachedObjects)
		case media.MediaProcessingError:
			errMediaProcessing := err.(media.MediaProcessingError)
			if errMediaProcessing.AnimationError != nil {
				errInvalidMedia := err.(media.ErrInvalidMedia)
				result.Media = persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(errInvalidMedia.URL)}
			} else if errMediaProcessing.ImageError != nil {
				errInvalidMedia := err.(media.ErrInvalidMedia)
				result.Media = persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(errInvalidMedia.URL)}
			}
		case media.ErrInvalidMedia:
			errInvalidMedia := err.(media.ErrInvalidMedia)
			result.Media = persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(errInvalidMedia.URL)}
		}

		return result, err
	}

	result.Media = tpj.createMediaFromCachedObjects(ctx, cachedObjects)

	return result, nil
}

func (tpj *tokenProcessingJob) retrieveMetadata(ctx context.Context) persist.TokenMetadata {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.MetadataRetrieval)

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
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.TokenInfoRetrieval)

	name, description := media.FindNameAndDescription(ctx, metadata)

	if name == "" {
		name = tpj.token.Name.String()
	}

	if description == "" {
		description = tpj.token.Description.String()
	}
	return name, description
}

func (tpj *tokenProcessingJob) cacheMediaObjects(ctx context.Context, metadata persist.TokenMetadata) ([]media.CachedMediaObject, error) {
	image, anim := media.KeywordsForChain(tpj.token.Chain, tpj.imageKeywords, tpj.animationKeywords)
	return media.CacheObjectsForMetadata(ctx, metadata, tpj.contract.Address, persist.TokenID(tpj.token.TokenID.String()), tpj.token.TokenURI, tpj.token.Chain, tpj.tp.ipfsClient, tpj.tp.arweaveClient, tpj.tp.stg, tpj.tp.tokenBucket, image, anim)
}

func (tpj *tokenProcessingJob) createMediaFromCachedObjects(ctx context.Context, objects []media.CachedMediaObject) persist.Media {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.CreateMediaFromCachedObjects)
	return media.CreateMediaFromCachedObjects(ctx, tpj.tp.tokenBucket, objects)
}

func (tpj *tokenProcessingJob) createRawMedia(ctx context.Context, mediaType persist.MediaType, animURL, imgURL string, objects []media.CachedMediaObject) persist.Media {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.CreateRawMedia)
	return media.CreateRawMedia(ctx, persist.NewTokenIdentifiers(tpj.contract.Address, tpj.token.TokenID, tpj.token.Chain), mediaType, tpj.tp.tokenBucket, animURL, imgURL, objects)
}

func (tpj *tokenProcessingJob) isNewMediaPreferable(ctx context.Context, media persist.Media) bool {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.MediaResultComparison)
	return !tpj.token.Media.IsServable() && media.IsServable()
}

func (tpj *tokenProcessingJob) persistMedia(ctx context.Context, tmetadata coredb.TokenMedium) error {
	if !tpj.isNewMediaPreferable(ctx, tmetadata.Media) {
		tmetadata.Active = false
	}

	errGroup, ctx := errgroup.WithContext(ctx)

	errGroup.Go(func() error {
		return tpj.updateTokenMetadataDB(ctx, tmetadata)

	})
	errGroup.Go(func() error {
		return tpj.updateJobDB(ctx, tmetadata)
	})

	return errGroup.Wait()
}

func (tpj *tokenProcessingJob) updateTokenMetadataDB(ctx context.Context, tmetadata coredb.TokenMedium) error {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.UpdateTokenMetadataDB)
	if !tmetadata.Active {
		err := tpj.tp.queries.InsertTokenMedia(ctx, coredb.InsertTokenMediaParams{
			ID:              tmetadata.ID,
			Contract:        tmetadata.Contract,
			TokenID:         tmetadata.TokenID,
			Chain:           tmetadata.Chain,
			Metadata:        tmetadata.Metadata,
			Media:           tmetadata.Media,
			Name:            tmetadata.Name,
			Description:     tmetadata.Description,
			ProcessingJobID: tmetadata.ProcessingJobID,
		})
		if err != nil {
			persist.FailStep(&tpj.pipelineMetadata.UpdateTokenMetadataDB)
			return err
		}
	}
	exists, err := tpj.tp.queries.IsExistsTokenMediaByTokenIdentifers(ctx, coredb.IsExistsTokenMediaByTokenIdentifersParams{
		Contract: tpj.token.Contract,
		TokenID:  tpj.token.TokenID,
		Chain:    tpj.token.Chain,
	})
	if err != nil {
		persist.FailStep(&tpj.pipelineMetadata.UpdateTokenMetadataDB)
		return err
	}

	if exists {
		return tpj.tp.queries.UpdateTokenMediaByTokenIdentifiers(ctx, coredb.UpdateTokenMediaByTokenIdentifiersParams{
			ID:              tmetadata.ID,
			Contract:        tpj.token.Contract,
			TokenID:         tmetadata.TokenID,
			Chain:           tpj.token.Chain,
			Metadata:        tmetadata.Metadata,
			Media:           tmetadata.Media,
			Name:            tmetadata.Name,
			Description:     tmetadata.Description,
			ProcessingJobID: tmetadata.ProcessingJobID,
		})
	}
	err = tpj.tp.queries.InsertTokenMedia(ctx, coredb.InsertTokenMediaParams{
		ID:              tmetadata.ID,
		Contract:        tmetadata.Contract,
		TokenID:         tmetadata.TokenID,
		Chain:           tmetadata.Chain,
		Metadata:        tmetadata.Metadata,
		Media:           tmetadata.Media,
		Name:            tmetadata.Name,
		Description:     tmetadata.Description,
		ProcessingJobID: tmetadata.ProcessingJobID,
	})
	if err != nil {
		persist.FailStep(&tpj.pipelineMetadata.UpdateTokenMetadataDB)
		return err
	}
	err = tpj.tp.queries.UpdateTokenTokenMediaByTokenIdentifiers(ctx, coredb.UpdateTokenTokenMediaByTokenIdentifiersParams{
		TokenMedia: persist.DBIDToNullStr(tmetadata.ID),
		Contract:   tpj.token.Contract,
		TokenID:    tpj.token.TokenID,
		Chain:      tpj.token.Chain,
	})
	if err != nil {
		persist.FailStep(&tpj.pipelineMetadata.UpdateTokenMetadataDB)
		return err
	}
	return nil
}

func (tpj *tokenProcessingJob) updateJobDB(ctx context.Context, tmetadata coredb.TokenMedium) error {
	defer persist.TrackStepStatus(&tpj.pipelineMetadata.UpdateJobDB)
	p := persist.TokenProperties{}
	if tmetadata.Metadata != nil && len(tmetadata.Metadata) > 0 {
		p.HasMetadata = true
	}
	if tmetadata.Media.MediaType.IsValid() && tmetadata.Media.MediaURL != "" {
		p.HasPrimaryMedia = true
	}
	if tmetadata.Media.ThumbnailURL != "" {
		p.HasThumbnail = true
	}
	if tmetadata.Media.LivePreviewURL != "" {
		p.HasLiveRender = true
	}
	if tmetadata.Media.Dimensions.Valid() {
		p.HasDimensions = true
	}

	err := tpj.tp.queries.InsertTokenProcessingJob(ctx, coredb.InsertTokenProcessingJobParams{
		ID:               tpj.id,
		TokenProperties:  p,
		PipelineMetadata: *tpj.pipelineMetadata,
		ProcessingCause:  tpj.cause,
		ProcessorVersion: "",
	})
	if err != nil {
		persist.FailStep(&tpj.pipelineMetadata.UpdateJobDB)
		return err
	}
	return nil
}
