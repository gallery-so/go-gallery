package tokenprocessing

import (
	"context"
	"errors"
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

func (tp *tokenProcessor) ProcessTokenPipeline(c context.Context, t persist.TokenGallery, contract persist.ContractGallery, cause persist.ProcessingCause) (coredb.TokenMedia, error) {

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

	media, err := job.run(loggerCtx)
	recordPipelineEndState(loggerCtx, tp.mr, media, err, time.Since(totalTime), cause)

	if err != nil {
		reportJobError(c, err, *job)
	}

	return media, err
}

// ErrBadToken is an error indicating that there is an issue with the token itself
type ErrBadToken struct {
	Err error
}

func (m ErrBadToken) Error() string {
	return fmt.Sprintf("issue with token: %s", m.Err)
}

func (e ErrBadToken) Unwrap() error {
	return e.Err
}

func (tpj *tokenProcessingJob) run(ctx context.Context) (coredb.TokenMedia, error) {
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

	persistCtx, cancel := context.WithTimeout(ctx, time.Minute*10)
	defer cancel()

	if err := tpj.persistResults(persistCtx, media); err != nil {
		return media, err
	}

	return media, mediaErr
}

type mediaResult struct {
	media coredb.TokenMedia
	err   error
}

func (tpj *tokenProcessingJob) createMediaForToken(ctx context.Context) mediaResult {
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.CreateMedia, "CreateMedia")
	defer traceCallback()

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

	tokenMedia, err := tpj.cacheMediaObjects(ctx, result.Metadata)
	result.Media = tokenMedia

	// Wrap the error to indicate that the token is bad to callers
	if util.ErrorAs[errNoMediaURLs](err) || util.ErrorAs[errInvalidMedia](err) {
		err = ErrBadToken{err}
	}

	return mediaResult{result, err}
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
			logger.For(ctx).Warnf("error getting metadata from chain: %s", err)
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

func (tpj *tokenProcessingJob) cacheMediaObjects(ctx context.Context, metadata persist.TokenMetadata) (persist.Media, error) {
	imgURL, animURL, err := findImageAndAnimationURLs(ctx, tpj.token.TokenID, tpj.contract.Address, tpj.token.Chain, metadata, tpj.token.TokenURI, tpj.token.Chain != persist.ChainETH, tpj.pipelineMetadata)
	if err != nil {
		return persist.Media{MediaType: persist.MediaTypeUnknown}, err
	}

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"imgURL":  imgURL,
		"animURL": animURL,
	})

	var (
		imgCh, animCh         chan cacheResult
		imgResult, animResult cacheResult
		downloadSuccess       bool
	)

	if animURL != "" {
		animCh = cacheAnimationObjects(ctx, animURL, metadata, tpj)
	}
	if imgURL != "" {
		imgCh = cacheImageObjects(ctx, imgURL, metadata, tpj)
	}

	if animCh != nil {
		animResult = <-animCh
		if isCacheResultValid(animResult.err, len(animResult.cachedObjects)) {
			downloadSuccess = true
		}
	}
	if imgCh != nil {
		imgResult = <-imgCh
		if isCacheResultValid(imgResult.err, len(imgResult.cachedObjects)) {
			downloadSuccess = true
		}
	}

	// If we have at least one successful download, we can create media from it
	if downloadSuccess {
		return createMediaFromResults(ctx, tpj, animResult, imgResult), nil
	}

	// Try to use OpenSea as a fallback
	if imgResult.err != nil || animResult.err != nil {
		traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.NothingCachedWithErrors, "NothingCachedWithErrors")
		defer traceCallback()

		openseaObjects, err := cacheOpenSeaObjects(ctx, tpj)

		if isCacheResultValid(err, len(openseaObjects)) {
			// Fail nothing cached with errors because we were able to get media from opensea
			persist.FailStep(&tpj.pipelineMetadata.NothingCachedWithErrors)
			logger.For(ctx).Warn("using media from OpenSea instead")
			return tpj.createMediaFromCachedObjects(ctx, openseaObjects), nil
		}
	}

	// At this point even OpenSea failed, so we need to return invalid media
	if invalidErr, ok := animResult.err.(errInvalidMedia); ok {
		return persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(invalidErr.URL)}, invalidErr
	}
	if invalidErr, ok := imgResult.err.(errInvalidMedia); ok {
		return persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(invalidErr.URL)}, invalidErr
	}
	if animResult.err != nil {
		return persist.Media{MediaType: persist.MediaTypeUnknown}, animResult.err
	}
	if imgResult.err != nil {
		return persist.Media{MediaType: persist.MediaTypeUnknown}, imgResult.err
	}

	// We somehow didn't cache media without getting an error anywhere
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.NothingCachedWithoutErrors, "NothingCachedWithoutErrors")
	defer traceCallback()
	logger.For(ctx).Fatal("failed to cache media, and no error occurred in the process")

	tid := persist.NewTokenIdentifiers(tpj.contract.Address, tpj.token.TokenID, tpj.token.Chain)
	return persist.Media{MediaType: persist.MediaTypeUnknown}, errNoCachedObjects{tid}
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

func recordPipelineEndState(ctx context.Context, mr metric.MetricReporter, tokenMedia coredb.TokenMedia, err error, d time.Duration, cause persist.ProcessingCause) {
	baseOpts := append([]any{}, metric.LogOptions.WithTags(map[string]string{
		"chain":      fmt.Sprintf("%d", tokenMedia.Chain),
		"mediaType":  tokenMedia.Media.MediaType.String(),
		"cause":      cause.String(),
		"isBadToken": fmt.Sprintf("%t", isBadTokenErr(err)),
	}))

	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		mr.Record(ctx, pipelineTimedOutMetric(), append(baseOpts,
			metric.LogOptions.WithLogMessage("pipeline timed out"),
		)...)
		return
	}

	mr.Record(ctx, pipelineDurationMetric(d), append(baseOpts,
		metric.LogOptions.WithLogMessage(fmt.Sprintf("pipeline finished (took: %s)", d)),
	)...)

	if err != nil {
		mr.Record(ctx, pipelineErroredMetric(), append(baseOpts,
			metric.LogOptions.WithLevel(logrus.ErrorLevel),
			metric.LogOptions.WithLogMessage("pipeline completed with error: "+err.Error()),
		)...)
		return
	}

	mr.Record(ctx, pipelineCompletedMetric(), append(baseOpts,
		metric.LogOptions.WithLogMessage("pipeline completed successfully"),
	)...)
}
