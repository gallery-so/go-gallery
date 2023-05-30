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

func (tp *tokenProcessor) ProcessTokenPipeline(c context.Context, t persist.TokenGallery, contract persist.ContractGallery, cause persist.ProcessingCause) (TokenRunResult, error) {

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

	mediaResult, pipelineErr := job.run(loggerCtx)
	recordPipelineEndState(loggerCtx, tp.mr, &mediaResult, time.Since(totalTime), cause)

	if mediaResult.Err != nil {
		logger.For(c).Errorf("pipeline encountered an error while handling media for token(chain=%d, contract=%d, tokenID=%s): %s", job.token.Chain, job.contract.Address, job.token.TokenID, mediaResult.Err)
		reportJobError(c, mediaResult.Err, *job)
	}

	if pipelineErr != nil {
		logger.For(c).Errorf("pipeline execution error occurred for token(chain=%s, contract=%d, tokenID=%s): %s", job.token.Chain, job.contract.Address, job.token.TokenID, pipelineErr)
		reportJobError(c, pipelineErr, *job)
	}

	return mediaResult, pipelineErr
}

// TokenRunResult is the result of running the token processing pipeline
type TokenRunResult struct {
	TokenMedia coredb.TokenMedia
	Err        error
}

// ErrMediaProcessing is an error that occurs when handling media for a token
type ErrMediaProcessing struct {
	Err error
}

func (m ErrMediaProcessing) Error() string {
	return fmt.Sprintf("error occurred in processing: %s", m.Err)
}

func (e ErrMediaProcessing) Unwrap() error {
	return e.Err
}

// ErrPipelineStep is used to differentiate between errors that occur because of
// the pipeline versus errors that occur in the media processing
type ErrPipelineStep struct {
	Err error
}

func (e ErrPipelineStep) Error() string {
	return e.Err.Error()
}

func (e ErrPipelineStep) Unwrap() error {
	return e.Err
}

func (tpj *tokenProcessingJob) run(ctx context.Context) (TokenRunResult, error) {
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

	pipelineErr := tpj.persistResults(persistCtx, media)

	if mediaErr != nil {
		mediaErr = ErrMediaProcessing{mediaErr}
	}

	if pipelineErr != nil {
		pipelineErr = ErrPipelineStep{pipelineErr}
	}

	return TokenRunResult{TokenMedia: media, Err: mediaErr}, pipelineErr
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

	tokenMedia, err := tpj.cacheMediaObjects(ctx, result.Metadata)
	if err != nil {
		tokenMedia.MediaType = persist.MediaTypeUnknown
	}

	return mediaResult{result, err}
}

func (tpj *tokenProcessingJob) retrieveMetadata(ctx context.Context) persist.TokenMetadata {
	traceCallback, ctx := trackStepStatus(ctx, &tpj.pipelineMetadata.MetadataRetrieval, "MetadataRetrieval")
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
			failStep(&tpj.pipelineMetadata.MetadataRetrieval)
		} else if mcMetadata != nil && len(mcMetadata) > 0 {
			logger.For(ctx).Infof("got metadata from chain: %v", mcMetadata)
			newMetadata = mcMetadata
		}
	}

	if len(newMetadata) == 0 {
		failStep(&tpj.pipelineMetadata.MetadataRetrieval)
	}

	return newMetadata
}

func (tpj *tokenProcessingJob) retrieveTokenInfo(ctx context.Context, metadata persist.TokenMetadata) (string, string) {
	traceCallback, ctx := trackStepStatus(ctx, &tpj.pipelineMetadata.TokenInfoRetrieval, "TokenInfoRetrieval")
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
	imgURL, animURL, err := findImageAndAnimationURLs(ctx, tpj.token.TokenID, tpj.contract.Address, tpj.token.Chain, tpj.token.TokenMetadata, tpj.token.TokenURI, tpj.token.Chain != persist.ChainETH, tpj.pipelineMetadata)
	if err != nil {
		return persist.Media{}, err
	}

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"imgURL":  imgURL,
		"animURL": animURL,
	})

	logger.For(ctx).Infof("found media URLs")

	var (
		imgCh, animCh         chan cacheResult
		imgResult, animResult cacheResult
		downloadSuccess       bool
	)

	if animURL != "" {
		animCh = cacheAnimationObjects(ctx, imgURL, metadata, tpj)
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

	if downloadSuccess {
		return createMediaFromResults(ctx, tpj, animResult, imgResult), nil
	} else {
		traceCallback, ctx := trackStepStatus(ctx, &tpj.pipelineMetadata.NothingCachedWithErrors, "NothingCachedWithErrors")
		defer traceCallback()

		openseaObjects, err := cacheOpenSeaObjects(ctx, tpj)

		if isCacheResultValid(err, len(openseaObjects)) {
			// Fail nothing cached with errors because we were able to get media from opensea
			failStep(&tpj.pipelineMetadata.NothingCachedWithErrors)
			logger.For(ctx).Warn("using media from OpenSea instead")
			return tpj.createMediaFromCachedObjects(ctx, openseaObjects), nil
		}

		logger.For(ctx).Errorf("failed to cache media from OpenSea: %s", err)
	}

	// At this point even OpenSea failed, so we need to return invalid media
	if invalidErr, ok := animResult.err.(errInvalidMedia); ok {
		return persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(invalidErr.URL)}, invalidErr
	}
	if invalidErr, ok := imgResult.err.(errInvalidMedia); ok {
		return persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(invalidErr.URL)}, invalidErr
	}

	// TODO: panic check thing?

	return persist.Media{MediaType: persist.MediaTypeUnknown}, util.MultiErr{animResult.err, imgResult.err}
}

func (tpj *tokenProcessingJob) createMediaFromCachedObjects(ctx context.Context, objects []cachedMediaObject) persist.Media {
	traceCallback, ctx := trackStepStatus(ctx, &tpj.pipelineMetadata.CreateMediaFromCachedObjects, "CreateMediaFromCachedObjects")
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
	traceCallback, ctx := trackStepStatus(ctx, &tpj.pipelineMetadata.CreateRawMedia, "CreateRawMedia")
	defer traceCallback()

	return createRawMedia(ctx, persist.NewTokenIdentifiers(tpj.contract.Address, tpj.token.TokenID, tpj.token.Chain), mediaType, tpj.tp.tokenBucket, animURL, imgURL, objects)
}

func (tpj *tokenProcessingJob) isNewMediaPreferable(ctx context.Context, media persist.Media) bool {
	traceCallback, ctx := trackStepStatus(ctx, &tpj.pipelineMetadata.MediaResultComparison, "MediaResultComparison")
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

func trackStepStatus(ctx context.Context, status *persist.PipelineStepStatus, name string) (func(), context.Context) {
	span, ctx := tracing.StartSpan(ctx, "pipeline.step", name)

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"pipelineStep": name,
	})

	startTime := time.Now()

	if status == nil {
		started := persist.PipelineStepStatusStarted
		status = &started
	}
	*status = persist.PipelineStepStatusStarted

	go func() {
		for {
			<-time.After(5 * time.Second)
			if status == nil || *status == persist.PipelineStepStatusSuccess || *status == persist.PipelineStepStatusError {
				return
			}
			logger.For(ctx).Infof("still on [%s] (taken: %s)", name, time.Since(startTime))
		}
	}()

	return func() {
		defer tracing.FinishSpan(span)
		if *status == persist.PipelineStepStatusError {
			logger.For(ctx).Infof("failed [%s] (took: %s)", name, time.Since(startTime))
			return
		}
		*status = persist.PipelineStepStatusSuccess
		logger.For(ctx).Infof("succeeded [%s] (took: %s)", name, time.Since(startTime))
	}, ctx

}

func failStep(status *persist.PipelineStepStatus) {
	if status == nil {
		errored := persist.PipelineStepStatusError
		status = &errored
	}
	*status = persist.PipelineStepStatusError
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

func recordPipelineEndState(ctx context.Context, mr metric.MetricReporter, r *TokenRunResult, d time.Duration, cause persist.ProcessingCause) {
	baseOpts := []any{}

	if r != nil {
		baseOpts = append(baseOpts, metric.LogOptions.WithTags(map[string]string{
			"chain":     fmt.Sprintf("%d", r.TokenMedia.Chain),
			"mediaType": r.TokenMedia.Media.MediaType.String(),
			"cause":     cause.String(),
		}))
	}

	if ctx.Err() != nil {
		mr.Record(ctx, pipelineTimedOutMetric(), append(baseOpts,
			metric.LogOptions.WithLogMessage("pipeline timed out"),
		)...)
		return
	}

	mr.Record(ctx, pipelineDurationMetric(d), append(baseOpts,
		metric.LogOptions.WithLogMessage(fmt.Sprintf("pipeline finished (took: %s)", d)),
	)...)

	if r != nil && r.Err != nil {
		mr.Record(ctx, pipelineErroredMetric(), append(baseOpts,
			metric.LogOptions.WithLogMessage("pipeline completed with error: "+r.Err.Error()),
		)...)
		return
	}

	mr.Record(ctx, pipelineCompletedMetric(), append(baseOpts,
		metric.LogOptions.WithLogMessage("pipeline completed successfully"),
	)...)
}
