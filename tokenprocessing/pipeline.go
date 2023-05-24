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
	"github.com/mikeydub/go-gallery/service/metric"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/tracing"
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
	mr            metric.MetricReporter
}

func NewTokenProcessor(queries *coredb.Queries, ethClient *ethclient.Client, mc *multichain.Provider, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, tokenRepo *postgres.TokenGalleryRepository, mr metric.MetricReporter) *tokenProcessor {
	return &tokenProcessor{
		queries:       queries,
		ethClient:     ethClient,
		mc:            mc,
		ipfsClient:    ipfsClient,
		arweaveClient: arweaveClient,
		stg:           stg,
		tokenBucket:   tokenBucket,
		tokenRepo:     tokenRepo,
		mr:            mr,
	}
}

type tokenProcessingJob struct {
	tp *tokenProcessor

	id       persist.DBID
	key      string
	token    persist.TokenGallery
	contract persist.ContractGallery

	cause            persist.ProcessingCause
	pipelineMetadata *persist.PipelineMetadata
}

func (tp *tokenProcessor) ProcessTokenPipeline(c context.Context, t persist.TokenGallery, contract persist.ContractGallery, cause persist.ProcessingCause) error {

	runID := persist.GenerateID()

	job := &tokenProcessingJob{
		id: runID,

		tp:       tp,
		key:      persist.NewTokenIdentifiers(contract.Address, t.TokenID, t.Chain).String(),
		token:    t,
		contract: contract,

		cause:            cause,
		pipelineMetadata: new(persist.PipelineMetadata),
	}

	loggerCtx := logger.NewContextWithFields(c, logrus.Fields{
		"tokenDBID":       t.ID,
		"tokenID":         t.TokenID,
		"tokenID_base10":  t.TokenID.Base10String(),
		"contractDBID":    t.Contract,
		"contractAddress": contract.Address,
		"chain":           t.Chain,
		"runID":           runID,
	})

	totalTime := time.Now()

	runResult := job.run(loggerCtx)
	recordPipelineEndState(loggerCtx, tp.mr, &runResult, time.Since(totalTime))
	return runResult.Err
}

type runResult struct {
	MediaType persist.MediaType
	Chain     persist.Chain
	Err       error
	Cause     persist.ProcessingCause
}

func (tpj *tokenProcessingJob) run(ctx context.Context) runResult {
	span, ctx := tracing.StartSpan(ctx, "pipeline.run", fmt.Sprintf("run %s", tpj.id))
	defer tracing.FinishSpan(span)

	logger.For(ctx).Infof("starting token processing pipeline for token %s (tokenDBID: %s)", tpj.key, tpj.token.ID)

	var media coredb.TokenMedia
	var mediaErr error

	func() {
		mediaCtx, cancel := context.WithTimeout(ctx, time.Minute*10)
		defer cancel()
		result := make(chan mediaResult, 1)
		select {
		case result <- tpj.createMediaForToken(mediaCtx):
			r := <-result
			media, mediaErr = r.media, r.err
		case <-mediaCtx.Done():
			mediaErr = mediaCtx.Err()
		}
	}()

	if mediaErr != nil {
		logger.For(ctx).Errorf("failed to create media for token %s", tpj.key)
	}

	persistCtx, cancel := context.WithTimeout(ctx, time.Minute*10)
	defer cancel()

	persistErr := tpj.persistResults(persistCtx, media)

	var err error
	if persistErr != nil && mediaErr != nil {
		err = util.MultiErr{persistErr, mediaErr}
	} else if persistErr != nil {
		err = persistErr
	} else if mediaErr != nil {
		err = mediaErr
	}
	return runResult{
		Chain:     media.Chain,
		MediaType: media.Media.MediaType,
		Err:       err,
		Cause:     tpj.cause,
	}
}

type mediaResult struct {
	media coredb.TokenMedia
	err   error
}

func (tpj *tokenProcessingJob) createMediaForToken(ctx context.Context) mediaResult {

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
		switch typ := err.(type) {
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
				result.Media = tpj.createRawMedia(ctx, typ.MediaType, typ.URL, first.storageURL(tpj.tp.tokenBucket), cachedObjects)
			} else {
				logger.For(ctx).Errorf("error caching media: %s", typ)
				result.Media = persist.Media{MediaType: typ.MediaType, MediaURL: persist.NullString(typ.URL)}
			}

			// We don't want to report this as an error, because it's not really an error
			err = nil
		case MediaProcessingError:

			if typ.AnimationError != nil {
				if errInvalidMedia, ok := typ.AnimationError.(errInvalidMedia); ok {
					result.Media = persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(errInvalidMedia.URL)}
				}
			} else if typ.ImageError != nil {
				if errInvalidMedia, ok := typ.ImageError.(errInvalidMedia); ok {
					result.Media = persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(errInvalidMedia.URL)}
				}
			}
			reportTokenError(ctx, typ, tpj.id, tpj.token.Chain, tpj.contract.Address, tpj.token.TokenID, isSpam)
		case errInvalidMedia:
			result.Media = persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(typ.URL)}
			reportTokenError(ctx, typ, tpj.id, tpj.token.Chain, tpj.contract.Address, tpj.token.TokenID, isSpam)
		default:
			traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.SetUnknownMediaType, "SetUnknownMediaType")
			defer traceCallback()
			result.Media = persist.Media{MediaType: persist.MediaTypeUnknown}
			reportTokenError(ctx, typ, tpj.id, tpj.token.Chain, tpj.contract.Address, tpj.token.TokenID, isSpam)
		}

		if result.Media.MediaType == "" {
			result.Media.MediaType = persist.MediaTypeUnknown
		}

		return mediaResult{result, err}
	}

	result.Media = tpj.createMediaFromCachedObjects(ctx, cachedObjects)

	return mediaResult{result, nil}
}

func (tpj *tokenProcessingJob) retrieveMetadata(ctx context.Context) persist.TokenMetadata {
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.MetadataRetrieval, "MetadataRetrieval")
	defer traceCallback()

	// metadata is a string, it should not take more than a minute to retrieve
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	newMetadata := tpj.token.TokenMetadata

	if len(newMetadata) == 0 || tpj.cause == persist.ProcessingCauseRefresh {
		i, a := tpj.contract.Chain.BaseKeywords()
		fieldRequests := []multichain.FieldRequest[string]{
			{
				FieldNames: append(i, a...),
				Level:      multichain.FieldRequirementLevelOneRequired,
			},
			{
				FieldNames: []string{"name", "description"},
				Level:      multichain.FieldRequirementLevelAllOptional,
			},
		}
		mcMetadata, err := tpj.tp.mc.GetTokenMetadataByTokenIdentifiers(ctx, tpj.contract.Address, tpj.token.TokenID, tpj.token.Chain, fieldRequests)
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
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.TokenInfoRetrieval, "TokenInfoRetrieval")
	defer traceCallback()

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
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.CreateMediaFromCachedObjects, "CreateMediaFromCachedObjects")
	defer traceCallback()

	in := map[objectType]cachedMediaObject{}
	for _, obj := range objects {
		if it, ok := in[obj.ObjectType]; ok {
			if it.MediaType.IsMorePriorityThan(obj.MediaType) {
				in[obj.ObjectType] = obj
			}
			continue
		}
		in[obj.ObjectType] = obj
	}
	return createMediaFromCachedObjects(ctx, tpj.tp.tokenBucket, in)
}

func (tpj *tokenProcessingJob) createRawMedia(ctx context.Context, mediaType persist.MediaType, animURL, imgURL string, objects []cachedMediaObject) persist.Media {
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.CreateRawMedia, "CreateRawMedia")
	defer traceCallback()

	return createRawMedia(ctx, persist.NewTokenIdentifiers(tpj.contract.Address, tpj.token.TokenID, tpj.token.Chain), mediaType, tpj.tp.tokenBucket, animURL, imgURL, objects)
}

func (tpj *tokenProcessingJob) isNewMediaPreferable(ctx context.Context, media persist.Media) bool {
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.MediaResultComparison, "MediaResultComparison")
	defer traceCallback()

	if media.IsServable() {
		// if the media is good, it is active
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
	return tpj.tp.queries.InsertTokenPipelineResults(ctx, coredb.InsertTokenPipelineResultsParams{
		Chain:            tpj.token.Chain,
		ContractID:       tpj.token.Contract,
		TokenID:          tpj.token.TokenID,
		TokenDbid:        tpj.token.ID.String(),
		ProcessingJobID:  tpj.id,
		TokenProperties:  p,
		PipelineMetadata: *tpj.pipelineMetadata,
		ProcessingCause:  tpj.cause,
		ProcessorVersion: "",
		NewMediaID:       persist.GenerateID(),
		Metadata:         tmetadata.Metadata,
		Media:            tmetadata.Media,
		Name:             tmetadata.Name,
		Description:      tmetadata.Description,
		Active:           tmetadata.Active,
		CopyMediaID:      persist.GenerateID(),
	})
}

const (
	// Metrics emitted by the pipeline
	metricPipelineCompleted = "pipeline_completed"
	metricPipelineDuration  = "pipeline_duration"
	metricPipelineErrored   = "pipeline_errored"
	metricPipelineTimedOut  = "pipeline_timedout"
)

func pipelineDurationMetric(d time.Duration) metric.Measure {
	return metric.Measure{Name: metricPipelineDuration, Value: d.Seconds()}
}

func pipelineTimedOutMetric() metric.Measure {
	return metric.Measure{Name: metricPipelineTimedOut}
}

func pipelineCompletedMetric() metric.Measure {
	return metric.Measure{Name: metricPipelineCompleted}
}

func pipelineErroredMetric() metric.Measure {
	return metric.Measure{Name: metricPipelineErrored}
}

func recordPipelineEndState(ctx context.Context, mr metric.MetricReporter, r *runResult, d time.Duration) {
	baseOpts := []any{}

	if r != nil {
		tags := map[string]string{
			"chain":     fmt.Sprintf("%d", r.Chain),
			"mediaType": r.MediaType.String(),
			"cause":     r.Cause.String(),
		}
		baseOpts = append(baseOpts, metric.LogOptions.WithTags(tags))
	}

	if ctx.Err() != nil {
		opts := append(baseOpts, metric.LogOptions.WithLogMessage("pipeline timed out"))
		mr.Record(ctx, pipelineTimedOutMetric(), opts...)
		return
	}

	recordPipelineDuration(ctx, mr, d, baseOpts)

	if r != nil && r.Err != nil {
		opts := append(baseOpts, metric.LogOptions.WithLogMessage("pipeline completed with error: "+r.Err.Error()))
		mr.Record(ctx, pipelineErroredMetric(), opts...)
		return
	}

	opts := append(baseOpts, metric.LogOptions.WithLogMessage("pipeline completed successfully"))
	mr.Record(ctx, pipelineCompletedMetric(), opts...)
}

func recordPipelineDuration(ctx context.Context, mr metric.MetricReporter, d time.Duration, opts []any) {
	opts = append(opts, metric.LogOptions.WithLogMessage(fmt.Sprintf("token processing job complete: total time for token processing job: %s", d)))
	mr.Record(ctx, pipelineDurationMetric(d), opts...)
}