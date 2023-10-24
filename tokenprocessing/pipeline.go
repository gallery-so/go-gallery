package tokenprocessing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/jackc/pgtype"
	"github.com/sirupsen/logrus"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/metric"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
)

type tokenProcessor struct {
	queries       *coredb.Queries
	httpClient    *http.Client
	mc            *multichain.Provider
	ipfsClient    *shell.Shell
	arweaveClient *goar.Client
	stg           *storage.Client
	tokenBucket   string
	mr            metric.MetricReporter
}

func NewTokenProcessor(queries *coredb.Queries, httpClient *http.Client, mc *multichain.Provider, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, mr metric.MetricReporter) *tokenProcessor {
	return &tokenProcessor{
		queries:       queries,
		mc:            mc,
		httpClient:    httpClient,
		ipfsClient:    ipfsClient,
		arweaveClient: arweaveClient,
		stg:           stg,
		tokenBucket:   tokenBucket,
		mr:            mr,
	}
}

type tokenProcessingJob struct {
	tp               *tokenProcessor
	id               persist.DBID
	token            persist.TokenIdentifiers
	contract         persist.ContractIdentifiers
	cause            persist.ProcessingCause
	pipelineMetadata *persist.PipelineMetadata
	// profileImageKey is an optional key in the metadata that the pipeline should also process as a profile image.
	// The pipeline only looks at the root level of the metadata for the key and will also not fail if the key is missing
	// or if processing media for the key fails.
	profileImageKey string
	// refreshMetadata is an optional flag that indicates that the pipeline should check for new metadata when enabled
	refreshMetadata bool
	// defaultMetadata is metadata to use to retrieve media from. If not set or refreshMetadata is enabled, the pipeline will try to get new metadata.
	defaultMetadata persist.TokenMetadata
	// isSpamJob is an optional flag that indicates that the token/contract is designated as spam. It's currently used to exclude events from Sentry. isSpamJob bool
	isSpamJob bool
}

type PipelineOption func(*tokenProcessingJob)

type pOpts struct{}

var PipelineOpts pOpts

func (pOpts) WithProfileImageKey(key string) PipelineOption {
	return func(j *tokenProcessingJob) {
		j.profileImageKey = key
	}
}

func (pOpts) WithRefreshMetadata() PipelineOption {
	return func(j *tokenProcessingJob) {
		j.refreshMetadata = true
	}
}

func (pOpts) WithMetadata(t persist.TokenMetadata) PipelineOption {
	return func(j *tokenProcessingJob) {
		j.defaultMetadata = t
	}
}

func (pOpts) WithIsSpamJob(isSpamJob bool) PipelineOption {
	return func(j *tokenProcessingJob) {
		j.isSpamJob = isSpamJob
	}
}

func (tp *tokenProcessor) ProcessTokenPipeline(ctx context.Context, token persist.TokenIdentifiers, contract persist.ContractIdentifiers, cause persist.ProcessingCause, opts ...PipelineOption) (coredb.TokenMedia, error) {
	runID := persist.GenerateID()

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"runID": runID})

	job := &tokenProcessingJob{
		id:               runID,
		tp:               tp,
		token:            token,
		contract:         contract,
		cause:            cause,
		pipelineMetadata: new(persist.PipelineMetadata),
	}

	for _, opt := range opts {
		opt(job)
	}

	startTime := time.Now()
	media, err := job.Run(ctx)
	recordPipelineEndState(ctx, tp.mr, job.token.Chain, media, err, time.Since(startTime), cause)

	if err != nil {
		reportJobError(ctx, err, *job)
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

// Run runs the pipeline, returning the media that was created by the run.
func (tpj *tokenProcessingJob) Run(ctx context.Context) (coredb.TokenMedia, error) {
	span, ctx := tracing.StartSpan(ctx, "pipeline.run", fmt.Sprintf("run %s", tpj.id))
	defer tracing.FinishSpan(span)

	logger.For(ctx).Infof("starting token processing pipeline for token %s", tpj.token.String())

	mediaCtx, cancel := context.WithTimeout(ctx, time.Minute*10)
	defer cancel()

	media, metadata, mediaErr := tpj.createMediaForToken(mediaCtx)

	saved, err := tpj.persistResults(ctx, media, metadata)
	if err != nil {
		return saved, err
	}

	return saved, mediaErr
}

func (tpj *tokenProcessingJob) createMediaForToken(ctx context.Context) (persist.Media, persist.TokenMetadata, error) {
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.CreateMedia, "CreateMedia")
	defer traceCallback()

	metadata := tpj.retrieveMetadata(ctx)
	newMedia, err := tpj.cacheMediaObjects(ctx, metadata)

	// Wrap the error to indicate that the token is bad to callers
	if errors.Is(err, media.ErrNoMediaURLs) || util.ErrorAs[errInvalidMedia](err) {
		err = ErrBadToken{err}
	}

	return newMedia, metadata, err
}

func (tpj *tokenProcessingJob) retrieveMetadata(ctx context.Context) persist.TokenMetadata {
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.MetadataRetrieval, "MetadataRetrieval")
	defer traceCallback()

	if len(tpj.defaultMetadata) > 0 && !tpj.refreshMetadata {
		return tpj.defaultMetadata
	}

	// metadata is a string, it should not take more than a minute to retrieve
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

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

	newMetadata, err := tpj.tp.mc.GetTokenMetadataByTokenIdentifiers(ctx, tpj.contract.ContractAddress, tpj.token.TokenID, tpj.token.Chain, fieldRequests)
	if err != nil {
		logger.For(ctx).Warnf("error getting metadata from chain: %s", err)
		persist.FailStep(&tpj.pipelineMetadata.MetadataRetrieval)
	} else if len(newMetadata) > 0 {
		logger.For(ctx).Infof("got metadata from chain: %v", newMetadata)
	}

	if len(newMetadata) == 0 {
		persist.FailStep(&tpj.pipelineMetadata.MetadataRetrieval)
	}

	return newMetadata
}

func (tpj *tokenProcessingJob) cacheMediaObjects(ctx context.Context, metadata persist.TokenMetadata) (persist.Media, error) {
	imgURL, animURL, err := findImageAndAnimationURLs(ctx, tpj.contract.ContractAddress, tpj.token.Chain, metadata, tpj.pipelineMetadata)
	if err != nil {
		return persist.Media{MediaType: persist.MediaTypeUnknown}, err
	}

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"imgURL":  imgURL,
		"animURL": animURL,
	})

	var (
		imgCh, animCh, pfpCh             chan cacheResult
		imgResult, animResult, pfpResult cacheResult
		downloadSuccess                  bool
	)

	if animURL != "" {
		animCh = cacheAnimationObjects(ctx, animURL, tpj)
	}
	if imgURL != "" {
		imgCh = cacheImageObjects(ctx, imgURL, tpj)
	}
	if tpj.profileImageKey != "" {
		pfpCh, err = cacheProfileImageObjects(ctx, tpj, metadata)
		if err != nil {
			logger.For(ctx).Errorf("error caching profile image source: %s", err)
		}
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
	if pfpCh != nil {
		pfpResult = <-pfpCh
	}

	// If we have at least one successful download, we can create media from it
	if downloadSuccess {
		return createMediaFromResults(ctx, tpj, animResult, imgResult, pfpResult), nil
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
	traceCallback, _ := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.NothingCachedWithoutErrors, "NothingCachedWithoutErrors")
	defer traceCallback()

	panic("failed to cache media, and no error occurred in the process")
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

	return createRawMedia(ctx, persist.NewTokenIdentifiers(tpj.contract.ContractAddress, tpj.token.TokenID, tpj.token.Chain), mediaType, tpj.tp.tokenBucket, animURL, imgURL, objects)
}

func (tpj *tokenProcessingJob) activeStatus(ctx context.Context, media persist.Media) bool {
	traceCallback, _ := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.MediaResultComparison, "MediaResultComparison")
	defer traceCallback()
	return media.IsServable()
}

func toJSONB(v any) (pgtype.JSONB, error) {
	var j pgtype.JSONB
	err := j.Set(v)
	return j, err
}

func (tpj *tokenProcessingJob) persistResults(ctx context.Context, media persist.Media, metadata persist.TokenMetadata) (coredb.TokenMedia, error) {
	newMedia, err := toJSONB(media)
	if err != nil {
		return coredb.TokenMedia{}, err
	}

	newMetadata, err := toJSONB(metadata)
	if err != nil {
		return coredb.TokenMedia{}, err
	}

	name, description := findNameAndDescription(metadata)

	params := coredb.InsertTokenPipelineResultsParams{
		ProcessingJobID:  tpj.id,
		PipelineMetadata: *tpj.pipelineMetadata,
		ProcessingCause:  tpj.cause,
		ProcessorVersion: "",
		RetiringMediaID:  persist.GenerateID(),
		Chain:            tpj.token.Chain,
		ContractAddress:  tpj.contract.ContractAddress,
		TokenID:          tpj.token.TokenID,
		NewMediaIsActive: tpj.activeStatus(ctx, media),
		NewMediaID:       persist.GenerateID(),
		NewMedia:         newMedia,
		NewMetadata:      newMetadata,
		NewName:          util.ToNullString(name, true),
		NewDescription:   util.ToNullString(description, true),
	}

	params.TokenProperties = persist.TokenProperties{
		HasMetadata:     len(metadata) > 0,
		HasPrimaryMedia: media.MediaType.IsValid() && media.MediaURL != "",
		HasThumbnail:    media.ThumbnailURL != "",
		HasLiveRender:   media.LivePreviewURL != "",
		HasDimensions:   media.Dimensions.Valid(),
		HasName:         params.NewName.String != "",
		HasDescription:  params.NewDescription.String != "",
	}

	r, err := tpj.tp.queries.InsertTokenPipelineResults(ctx, params)
	return r.TokenMedia, err
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

func recordPipelineEndState(ctx context.Context, mr metric.MetricReporter, chain persist.Chain, tokenMedia coredb.TokenMedia, err error, d time.Duration, cause persist.ProcessingCause) {
	baseOpts := append([]any{}, metric.LogOptions.WithTags(map[string]string{
		"chain":      fmt.Sprintf("%d", chain),
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
