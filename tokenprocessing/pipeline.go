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
	"google.golang.org/api/googleapi"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/metric"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
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
	// defaultMetadata is starting metadata to use to process media from. If empty or refreshMetadata is set, then the pipeline will try to get new metadata.
	defaultMetadata persist.TokenMetadata
	// isSpamJob indicates that the job is processing a spam token. It's currently used to exclude events from Sentry.
	isSpamJob bool
	// requireImage indicates that the pipeline should return an error if an image URL is present but an image wasn't cached.
	requireImage bool
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

func (pOpts) WithRequireImage() PipelineOption {
	return func(j *tokenProcessingJob) {
		j.requireImage = true
	}
}

type ErrImageResultRequired struct{ Err error }

func (e ErrImageResultRequired) Unwrap() error { return e.Err }
func (e ErrImageResultRequired) Error() string {
	msg := "failed to process required image"
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// ErrBadToken is an error indicating that there is an issue with the token itself
type ErrBadToken struct{ Err error }

func (e ErrBadToken) Unwrap() error { return e.Err }
func (m ErrBadToken) Error() string { return fmt.Sprintf("issue with token: %s", m.Err) }

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

func findURLsToDownloadFrom(ctx context.Context, tpj *tokenProcessingJob, metadata persist.TokenMetadata) (imgURL media.ImageURL, pfpURL media.ImageURL, animURL media.AnimationURL, err error) {
	pfpURL = findProfileImageURL(metadata, tpj.profileImageKey)
	imgURL, animURL, err = findImageAndAnimationURLs(ctx, tpj.token.ContractAddress, tpj.token.Chain, metadata, tpj.pipelineMetadata)
	return imgURL, pfpURL, animURL, err
}

func wrapWithBadTokenErr(err error) error {
	if errors.Is(err, media.ErrNoMediaURLs) || util.ErrorAs[errInvalidMedia](err) || util.ErrorAs[errNoDataFromReader](err) {
		err = ErrBadToken{Err: err}
	}
	return err
}

func cacheResultsToErr(animResult cacheResult, imgResult cacheResult, imageRequired bool) error {
	if imageRequired && !imgResult.IsSuccess() {
		return ErrImageResultRequired{Err: wrapWithBadTokenErr(imgResult.err)}
	}
	if animResult.IsSuccess() || imgResult.IsSuccess() {
		return nil
	}
	if imgResult.err != nil {
		return wrapWithBadTokenErr(imgResult.err)
	}
	return wrapWithBadTokenErr(animResult.err)
}

func (tpj *tokenProcessingJob) createMediaForToken(ctx context.Context) (persist.Media, persist.TokenMetadata, error) {
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.CreateMedia, "CreateMedia")
	defer traceCallback()
	metadata := tpj.retrieveMetadata(ctx)
	imgURL, pfpURL, animURL, err := findURLsToDownloadFrom(ctx, tpj, metadata)
	if err != nil {
		return persist.Media{MediaType: persist.MediaTypeUnknown}, metadata, err
	}
	newMedia, err := tpj.cacheMediaFromURLs(ctx, imgURL, pfpURL, animURL)
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

func (tpj *tokenProcessingJob) cacheFromURL(ctx context.Context, tids persist.TokenIdentifiers, defaultObjectType objectType, mediaURL string, subMeta *cachePipelineMetadata) chan cacheResult {
	resultCh := make(chan cacheResult)
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"tokenURIType":      persist.TokenURI(mediaURL).Type(),
		"defaultObjectType": defaultObjectType,
		"mediaURL":          mediaURL,
	})

	go func() {
		cachedObjects, err := cacheObjectsFromURL(ctx, tids, mediaURL, defaultObjectType, tpj.tp.httpClient, tpj.tp.ipfsClient, tpj.tp.arweaveClient, tpj.tp.stg, tpj.tp.tokenBucket, subMeta)
		if err == nil {
			resultCh <- cacheResult{cachedObjects, err}
			return
		}
		switch caught := err.(type) {
		case *googleapi.Error:
			if caught.Code == http.StatusTooManyRequests {
				logger.For(ctx).Infof("rate limited by google, retrying in 30 seconds")
				time.Sleep(time.Second * 30)
				cachedObjects, err = cacheObjectsFromURL(ctx, tids, mediaURL, defaultObjectType, tpj.tp.httpClient, tpj.tp.ipfsClient, tpj.tp.arweaveClient, tpj.tp.stg, tpj.tp.tokenBucket, subMeta)
			}
			resultCh <- cacheResult{cachedObjects, err}
		default:
			resultCh <- cacheResult{cachedObjects, err}
		}
	}()

	return resultCh
}

func (tpj *tokenProcessingJob) cacheMediaFromOriginalURLs(ctx context.Context, imgURL media.ImageURL, pfpURL media.ImageURL, animURL media.AnimationURL) (imgResult, pfpResult, animResult cacheResult) {
	imgRunMetadata := &cachePipelineMetadata{
		ContentHeaderValueRetrieval:  &tpj.pipelineMetadata.ImageContentHeaderValueRetrieval,
		ReaderRetrieval:              &tpj.pipelineMetadata.ImageReaderRetrieval,
		OpenseaFallback:              &tpj.pipelineMetadata.ImageOpenseaFallback,
		DetermineMediaTypeWithReader: &tpj.pipelineMetadata.ImageDetermineMediaTypeWithReader,
		AnimationGzip:                &tpj.pipelineMetadata.ImageAnimationGzip,
		SVGRasterize:                 &tpj.pipelineMetadata.ImageSVGRasterize,
		StoreGCP:                     &tpj.pipelineMetadata.ImageStoreGCP,
		ThumbnailGCP:                 &tpj.pipelineMetadata.ImageThumbnailGCP,
		LiveRenderGCP:                &tpj.pipelineMetadata.ImageLiveRenderGCP,
	}
	pfpRunMetadata := &cachePipelineMetadata{
		ContentHeaderValueRetrieval:  &tpj.pipelineMetadata.ProfileImageContentHeaderValueRetrieval,
		ReaderRetrieval:              &tpj.pipelineMetadata.ProfileImageReaderRetrieval,
		OpenseaFallback:              &tpj.pipelineMetadata.ProfileImageOpenseaFallback,
		DetermineMediaTypeWithReader: &tpj.pipelineMetadata.ProfileImageDetermineMediaTypeWithReader,
		AnimationGzip:                &tpj.pipelineMetadata.ProfileImageAnimationGzip,
		SVGRasterize:                 &tpj.pipelineMetadata.ProfileImageSVGRasterize,
		StoreGCP:                     &tpj.pipelineMetadata.ProfileImageStoreGCP,
		ThumbnailGCP:                 &tpj.pipelineMetadata.ProfileImageThumbnailGCP,
		LiveRenderGCP:                &tpj.pipelineMetadata.ProfileImageLiveRenderGCP,
	}
	animRunMetadata := &cachePipelineMetadata{
		ContentHeaderValueRetrieval:  &tpj.pipelineMetadata.AnimationContentHeaderValueRetrieval,
		ReaderRetrieval:              &tpj.pipelineMetadata.AnimationReaderRetrieval,
		OpenseaFallback:              &tpj.pipelineMetadata.AnimationOpenseaFallback,
		DetermineMediaTypeWithReader: &tpj.pipelineMetadata.AnimationDetermineMediaTypeWithReader,
		AnimationGzip:                &tpj.pipelineMetadata.AnimationAnimationGzip,
		SVGRasterize:                 &tpj.pipelineMetadata.AnimationSVGRasterize,
		StoreGCP:                     &tpj.pipelineMetadata.AnimationStoreGCP,
		ThumbnailGCP:                 &tpj.pipelineMetadata.AnimationThumbnailGCP,
		LiveRenderGCP:                &tpj.pipelineMetadata.AnimationLiveRenderGCP,
	}
	return tpj.cacheMediaSources(ctx, imgURL, pfpURL, animURL, imgRunMetadata, pfpRunMetadata, animRunMetadata)
}

func (tpj *tokenProcessingJob) cacheMediaFromOpenSeaAssetURLs(ctx context.Context) (imgResult, animResult cacheResult) {
	tID := persist.NewTokenIdentifiers(tpj.token.ContractAddress, tpj.token.TokenID, tpj.token.Chain)
	assets, err := opensea.FetchAssetsForTokenIdentifiers(ctx, persist.EthereumAddress(tID.ContractAddress), opensea.TokenID(tID.TokenID.Base10String()))
	if err != nil || len(assets) == 0 {
		result := cacheResult{err: errNoDataFromOpensea{err}}
		return result, result
	}
	imgRunMetadata := &cachePipelineMetadata{
		ContentHeaderValueRetrieval:  &tpj.pipelineMetadata.AlternateImageContentHeaderValueRetrieval,
		ReaderRetrieval:              &tpj.pipelineMetadata.AlternateImageReaderRetrieval,
		DetermineMediaTypeWithReader: &tpj.pipelineMetadata.AlternateImageDetermineMediaTypeWithReader,
		AnimationGzip:                &tpj.pipelineMetadata.AlternateImageAnimationGzip,
		SVGRasterize:                 &tpj.pipelineMetadata.AlternateImageSVGRasterize,
		StoreGCP:                     &tpj.pipelineMetadata.AlternateImageStoreGCP,
		ThumbnailGCP:                 &tpj.pipelineMetadata.AlternateImageThumbnailGCP,
		LiveRenderGCP:                &tpj.pipelineMetadata.AlternateImageLiveRenderGCP,
	}
	animRunMetadata := &cachePipelineMetadata{
		ContentHeaderValueRetrieval:  &tpj.pipelineMetadata.AlternateAnimationContentHeaderValueRetrieval,
		ReaderRetrieval:              &tpj.pipelineMetadata.AlternateAnimationReaderRetrieval,
		DetermineMediaTypeWithReader: &tpj.pipelineMetadata.AlternateAnimationDetermineMediaTypeWithReader,
		AnimationGzip:                &tpj.pipelineMetadata.AlternateAnimationAnimationGzip,
		SVGRasterize:                 &tpj.pipelineMetadata.AlternateAnimationSVGRasterize,
		StoreGCP:                     &tpj.pipelineMetadata.AlternateAnimationStoreGCP,
		ThumbnailGCP:                 &tpj.pipelineMetadata.AlternateAnimationThumbnailGCP,
		LiveRenderGCP:                &tpj.pipelineMetadata.AlternateAnimationLiveRenderGCP,
	}
	for _, asset := range assets {
		animURL := media.AnimationURL(util.FirstNonEmptyString(
			asset.AnimationURL,
			asset.AnimationOriginalURL,
		))
		imgURL := media.ImageURL(util.FirstNonEmptyString(
			asset.ImageURL,
			asset.ImagePreviewURL,
			asset.ImageOriginalURL,
			asset.ImageThumbnailURL,
		))
		imgResult, _, animResult = tpj.cacheMediaSources(ctx, imgURL, "", animURL, imgRunMetadata, nil, animRunMetadata)
		if animResult.IsSuccess() || imgResult.IsSuccess() {
			return imgResult, animResult
		}
	}
	return imgResult, animResult
}

func (tpj *tokenProcessingJob) cacheMediaSources(
	ctx context.Context,
	imgURL media.ImageURL,
	pfpURL media.ImageURL,
	animURL media.AnimationURL,
	imgRunMetadata *cachePipelineMetadata,
	pfpRunMetadata *cachePipelineMetadata,
	animRunMetadata *cachePipelineMetadata,
) (imgResult, pfpResult, animResult cacheResult) {
	var imgCh, pfpCh, animCh chan cacheResult

	if imgURL != "" {
		imgCh = tpj.cacheFromURL(ctx, tpj.token, objectTypeImage, string(imgURL), imgRunMetadata)
	}
	if pfpURL != "" {
		pfpCh = tpj.cacheFromURL(ctx, tpj.token, objectTypeProfileImage, string(pfpURL), pfpRunMetadata)
	}
	if animURL != "" {
		animCh = tpj.cacheFromURL(ctx, tpj.token, objectTypeAnimation, string(animURL), animRunMetadata)
	}

	if imgCh != nil {
		imgResult = <-imgCh
	}
	if pfpCh != nil {
		pfpResult = <-pfpCh
		if pfpResult.err != nil {
			logger.For(ctx).Errorf("error caching profile image source: %s", pfpResult.err)
		}
	}
	if animCh != nil {
		animResult = <-animCh
	}

	return imgResult, pfpResult, animResult
}

func (tpj *tokenProcessingJob) cacheMediaFromURLs(ctx context.Context, imgURL, pfpURL media.ImageURL, animURL media.AnimationURL) (m persist.Media, err error) {
	imgRequired := tpj.requireImage && imgURL != ""
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"imgURL": imgURL, "pfpURL": pfpURL, "animURL": animURL, "imgRequired": imgRequired})

	imgResult, pfpResult, animResult := tpj.cacheMediaFromOriginalURLs(ctx, imgURL, pfpURL, animURL)

	if (!imgRequired && animResult.IsSuccess()) || imgResult.IsSuccess() {
		err = cacheResultsToErr(animResult, imgResult, imgRequired)
		return createMediaFromResults(ctx, tpj, animResult, imgResult, pfpResult), err
	}

	// Our OpenSea provider only supports Ethereum currently. If we do use OpenSea, we prioritize our results over theirs.
	// If OpenSea also fails, then we keep our results so we can report on the original error.
	if tpj.token.Chain == persist.ChainETH {
		osImgResult, osAnimResult := tpj.cacheMediaFromOpenSeaAssetURLs(ctx)
		if !imgResult.IsSuccess() && osImgResult.IsSuccess() {
			imgResult = osImgResult
		}
		if !animResult.IsSuccess() && osAnimResult.IsSuccess() {
			animResult = osAnimResult
		}
	}

	// Now check if we got any result from OpenSea
	if animResult.IsSuccess() || imgResult.IsSuccess() {
		err = cacheResultsToErr(animResult, imgResult, imgRequired)
		return createMediaFromResults(ctx, tpj, animResult, imgResult, pfpResult), err
	}

	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.NothingCachedWithErrors, "NothingCachedWithErrors")
	defer traceCallback()

	// At this point we don't have a way to make media so we return an error
	err = cacheResultsToErr(animResult, imgResult, imgRequired)
	return mustCreateMediaFromErr(ctx, err, tpj), err
}

func (tpj *tokenProcessingJob) createMediaFromCachedObjects(ctx context.Context, objects []cachedMediaObject) persist.Media {
	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.CreateMediaFromCachedObjects, "CreateMediaFromCachedObjects")
	defer traceCallback()

	in := map[objectType]cachedMediaObject{}

	for _, obj := range objects {
		cur, ok := in[obj.ObjectType]

		if !ok {
			in[obj.ObjectType] = obj
			continue
		}

		if obj.MediaType.IsMorePriorityThan(cur.MediaType) {
			in[obj.ObjectType] = obj
		}
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
