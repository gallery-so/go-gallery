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

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/platform"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
)

type tokenProcessor struct {
	queries        *db.Queries
	httpClient     *http.Client
	metadataFinder *MetadataFinder
	ipfsClient     *shell.Shell
	arweaveClient  *goar.Client
	stg            *storage.Client
	tokenBucket    string
}

func NewTokenProcessor(queries *db.Queries, httpClient *http.Client, metadataFinder *MetadataFinder, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string) *tokenProcessor {
	return &tokenProcessor{
		queries:        queries,
		metadataFinder: metadataFinder,
		httpClient:     httpClient,
		ipfsClient:     ipfsClient,
		arweaveClient:  arweaveClient,
		stg:            stg,
		tokenBucket:    tokenBucket,
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
	// startingMetadata is starting metadata to use to process media from. If empty or refreshMetadata is set, then the pipeline will try to get new metadata.
	startingMetadata persist.TokenMetadata
	// isSpamJob indicates that the job is processing a spam token. It's currently used to exclude events from Sentry.
	isSpamJob bool
	// isFxhash indicates that the job is processing a fxhash token.
	isFxhash bool
	// requireImage indicates that the pipeline should return an error if an image URL is present but an image wasn't cached.
	requireImage bool
	// requireFxHashSigned indicates that the pipeline should return an error if the token is FxHash but it isn't signed yet.
	requireFxHashSigned bool
	//fxHashIsSignedF is called to determine if a token is signed by Fxhash. It's currently used to determine if the token should be retried at a later time if it is not signed yet.
	fxHashIsSignedF func(persist.TokenMetadata) bool
	// imgKeywords are fields in the token's metadata that the pipeline should treat as images. If imgKeywords is empty, the chain's default keywords are used instead.
	imgKeywords []string
	// animKeywords are fields in the token's metadata that the pipeline should treat as animations. If animKeywords is empty, the chain's default keywords are used instead.
	animKeywords []string
	// placeHolderImageURL is an image URL that is downloaded from if processing from metadata fails
	placeHolderImageURL string
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

func (pOpts) WithStartingMetadata(t persist.TokenMetadata) PipelineOption {
	return func(j *tokenProcessingJob) {
		j.startingMetadata = t
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

func (pOpts) WithRequireProhibitionimage(td db.TokenDefinition, c db.Contract) PipelineOption {
	return func(j *tokenProcessingJob) {
		if platform.IsProhibition(c.Chain, c.Address) {
			j.requireImage = true
			j.refreshMetadata = true
		}
	}
}

func (pOpts) WithRequireFxHashSigned(td db.TokenDefinition, c db.Contract) PipelineOption {
	return func(j *tokenProcessingJob) {
		if td.IsFxhash {
			j.isFxhash = true
			j.requireFxHashSigned = true
			j.fxHashIsSignedF = func(m persist.TokenMetadata) bool { return platform.IsFxhashSigned(td, c, m) }
			j.requireImage = true
			j.refreshMetadata = true
		}
	}
}

func (pOpts) WithRequireHighlightImage(td db.TokenDefinition, c db.Contract) PipelineOption {
	return func(j *tokenProcessingJob) {
		if isHighlight(td) {
			j.requireImage = true
			j.refreshMetadata = true
		}
	}
}

func (pOpts) WithKeywords(td db.TokenDefinition) PipelineOption {
	return func(j *tokenProcessingJob) {
		j.imgKeywords, j.animKeywords = platform.KeywordsFor(td)
	}
}

func (pOpts) WithPlaceholderImageURL(u string) PipelineOption {
	return func(j *tokenProcessingJob) {
		j.placeHolderImageURL = u
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

// ErrRequiredSignedToken indicates that the token isn't signed
var ErrRequiredSignedToken = errors.New("token isn't signed")

func (tp *tokenProcessor) ProcessToken(ctx context.Context, token persist.TokenIdentifiers, contract persist.ContractIdentifiers, cause persist.ProcessingCause, opts ...PipelineOption) (db.TokenMedia, error) {
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

	if len(job.imgKeywords) == 0 {
		k, _ := token.Chain.BaseKeywords()
		job.imgKeywords = k
	}

	if len(job.animKeywords) == 0 {
		_, k := token.Chain.BaseKeywords()
		job.animKeywords = k
	}

	media, err := job.Run(ctx)

	if err != nil {
		reportJobError(ctx, err, *job)
	}

	return media, err
}

// Run runs the pipeline, returning the media that was created by the run.
func (tpj *tokenProcessingJob) Run(ctx context.Context) (db.TokenMedia, error) {
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

func wrapWithBadTokenErr(err error) error {
	if errors.Is(err, media.ErrNoMediaURLs) || util.ErrorIs[errInvalidMedia](err) || util.ErrorIs[errNoDataFromReader](err) || errors.Is(err, ErrRequiredSignedToken) {
		err = tokenmanage.ErrBadToken{Err: err}
	}
	return err
}

func (tpj *tokenProcessingJob) createErrFromResults(animResult cacheResult, imgResult cacheResult, metadata persist.TokenMetadata, requireImg, requireSigned bool) error {
	if requireImg && !imgResult.IsSuccess() {
		return ErrImageResultRequired{Err: wrapWithBadTokenErr(imgResult.err)}
	}
	if requireSigned && !tpj.fxHashIsSignedF(metadata) {
		return wrapWithBadTokenErr(ErrRequiredSignedToken)
	}
	if animResult.IsSuccess() || imgResult.IsSuccess() {
		return nil
	}
	if imgResult.err != nil {
		return wrapWithBadTokenErr(imgResult.err)
	}
	return wrapWithBadTokenErr(animResult.err)
}

func (tpj *tokenProcessingJob) urlsToDownload(ctx context.Context, metadata persist.TokenMetadata) (imgURL media.ImageURL, pfpURL media.ImageURL, animURL media.AnimationURL, err error) {
	pfpURL = findProfileImageURL(metadata, tpj.profileImageKey)
	imgURL, animURL, err = findImageAndAnimationURLs(ctx, metadata, tpj.imgKeywords, tpj.animKeywords, tpj.pipelineMetadata)
	imgURL = media.ImageURL(rpc.RewriteURIToHTTP(string(imgURL), tpj.isFxhash))
	pfpURL = media.ImageURL(rpc.RewriteURIToHTTP(string(pfpURL), tpj.isFxhash))
	animURL = media.AnimationURL(rpc.RewriteURIToHTTP(string(animURL), tpj.isFxhash))
	return imgURL, pfpURL, animURL, err
}

func (tpj *tokenProcessingJob) createMediaForToken(ctx context.Context) (persist.Media, persist.TokenMetadata, error) {
	var (
		imgURL     media.ImageURL
		pfpURL     media.ImageURL
		animURL    media.AnimationURL
		metadata   persist.TokenMetadata
		tokenMedia persist.Media = persist.Media{MediaType: persist.MediaTypeUnknown}
		err        error
	)

	if !tpj.refreshMetadata {
		imgURL, pfpURL, animURL, err = tpj.urlsToDownload(ctx, tpj.startingMetadata)
		if err != nil && !errors.Is(err, media.ErrNoMediaURLs) {
			return tokenMedia, tpj.startingMetadata, wrapWithBadTokenErr(err)
		}
	}

	func() {
		metadataCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.MetadataRetrieval, "MetadataRetrieval")
		defer metadataCallback()

		metadata, err = tpj.tp.metadataFinder.GetMetadata(ctx, tpj.token)
		if err != nil {
			return
		}

		// try to get URLs from new metadata
		imgURL, pfpURL, animURL, err = tpj.urlsToDownload(ctx, metadata)
		if err == nil || (err != nil && !errors.Is(err, media.ErrNoMediaURLs)) {
			return
		}

		// use the starting metadata
		metadata = tpj.startingMetadata
		imgURL, pfpURL, animURL, err = tpj.urlsToDownload(ctx, metadata)
	}()

	if err != nil {
		return tokenMedia, metadata, wrapWithBadTokenErr(err)
	}

	tokenMedia, err = tpj.cacheMediaFromURLs(ctx, imgURL, pfpURL, animURL, metadata,
		tpj.requireImage && imgURL != "",
		tpj.requireFxHashSigned,
	)
	return tokenMedia, metadata, err
}

func (tpj *tokenProcessingJob) cacheFromURL(ctx context.Context, tids persist.TokenIdentifiers, defaultObjectType objectType, mediaURL string, subMeta *cachePipelineMetadata) chan cacheResult {
	resultCh := make(chan cacheResult)
	go func() {
		cachedObjects, err := cacheObjectsFromURL(ctx, tids, mediaURL, defaultObjectType, tpj.tp.httpClient, tpj.tp.ipfsClient, tpj.tp.arweaveClient, tpj.tp.stg, tpj.tp.tokenBucket, subMeta)
		resultCh <- cacheResult{cachedObjects, err}
	}()
	return resultCh
}

func (tpj *tokenProcessingJob) cacheMediaFromOriginalURLs(ctx context.Context, imgURL media.ImageURL, pfpURL media.ImageURL, animURL media.AnimationURL) (imgResult, pfpResult, animResult cacheResult) {
	imgRunMetadata := &cachePipelineMetadata{
		ContentHeaderValueRetrieval:  &tpj.pipelineMetadata.ImageContentHeaderValueRetrieval,
		ReaderRetrieval:              &tpj.pipelineMetadata.ImageReaderRetrieval,
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
		DetermineMediaTypeWithReader: &tpj.pipelineMetadata.AnimationDetermineMediaTypeWithReader,
		AnimationGzip:                &tpj.pipelineMetadata.AnimationAnimationGzip,
		SVGRasterize:                 &tpj.pipelineMetadata.AnimationSVGRasterize,
		StoreGCP:                     &tpj.pipelineMetadata.AnimationStoreGCP,
		ThumbnailGCP:                 &tpj.pipelineMetadata.AnimationThumbnailGCP,
		LiveRenderGCP:                &tpj.pipelineMetadata.AnimationLiveRenderGCP,
	}
	return tpj.cacheMediaSources(ctx, imgURL, pfpURL, animURL, imgRunMetadata, pfpRunMetadata, animRunMetadata)
}

func (tpj *tokenProcessingJob) cacheMediaFromPlaceholder(ctx context.Context) (imgResult, animResult cacheResult) {
	if tpj.placeHolderImageURL == "" {
		return cacheResult{}, cacheResult{}
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

	imgResult, _, animResult = tpj.cacheMediaSources(ctx, media.ImageURL(tpj.placeHolderImageURL), "", "", imgRunMetadata, nil, nil)
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

func (tpj *tokenProcessingJob) cacheMediaFromURLs(ctx context.Context, imgURL, pfpURL media.ImageURL, animURL media.AnimationURL, metadata persist.TokenMetadata, requireImg, requireSigned bool) (m persist.Media, err error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"imgURL":        imgURL,
		"pfpURL":        pfpURL,
		"animURL":       animURL,
		"requireImg":    requireImg,
		"requireSigned": requireSigned,
	})

	traceCallback, ctx := persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.CreateMedia, "CreateMedia")
	defer traceCallback()

	imgResult, pfpResult, animResult := tpj.cacheMediaFromOriginalURLs(ctx, imgURL, pfpURL, animURL)

	if (!requireImg && animResult.IsSuccess()) || imgResult.IsSuccess() {
		err = tpj.createErrFromResults(animResult, imgResult, metadata, requireImg, requireSigned)
		return createMediaFromResults(ctx, tpj, animResult, imgResult, pfpResult), err
	}

	// If there is a placeholder URL available, use that instead.
	placeHolderImgResult, placeHolderAnimResult := tpj.cacheMediaFromPlaceholder(ctx)
	if !imgResult.IsSuccess() && placeHolderImgResult.IsSuccess() {
		imgResult = placeHolderImgResult
	}
	if !animResult.IsSuccess() && placeHolderAnimResult.IsSuccess() {
		animResult = placeHolderAnimResult
	}

	if animResult.IsSuccess() || imgResult.IsSuccess() {
		err = tpj.createErrFromResults(animResult, imgResult, metadata, requireImg, requireSigned)
		return createMediaFromResults(ctx, tpj, animResult, imgResult, pfpResult), err
	}

	traceCallback, ctx = persist.TrackStepStatus(ctx, &tpj.pipelineMetadata.NothingCachedWithErrors, "NothingCachedWithErrors")
	defer traceCallback()

	// At this point we don't have a way to make media so we return an error
	err = tpj.createErrFromResults(animResult, imgResult, metadata, requireImg, requireSigned)
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

func (tpj *tokenProcessingJob) persistResults(ctx context.Context, media persist.Media, metadata persist.TokenMetadata) (db.TokenMedia, error) {
	newMedia, err := toJSONB(media)
	if err != nil {
		return db.TokenMedia{}, err
	}

	newMetadata, err := toJSONB(metadata)
	if err != nil {
		return db.TokenMedia{}, err
	}

	name, description := findNameAndDescription(metadata)

	params := db.InsertTokenPipelineResultsParams{
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
