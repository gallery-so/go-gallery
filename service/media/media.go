package media

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/googleapis/gax-go/v2"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/mediamapper"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	htransport "google.golang.org/api/transport/http"
)

func init() {
	env.RegisterValidation("IPFS_URL", "required")
}

var errAlreadyHasMedia = errors.New("token already has preview and thumbnail URLs")

type Keywords interface {
	ForToken(tokenID persist.TokenID, contract persist.Address) []string
}

type DefaultKeywords []string
type TezImageKeywords []string
type TezAnimationKeywords []string

type errUnsupportedURL struct {
	url string
}

type errUnsupportedMediaType struct {
	mediaType persist.MediaType
}

type errNoDataFromReader struct {
	err error
	url string
}

type errNotCacheable struct {
	url       string
	mediaType persist.MediaType
}

type errInvalidMedia struct {
	err error
	url string
}

type errNoCachedObjects struct {
	tids persist.TokenIdentifiers
}

type errNoMediaURLs struct {
	metadata persist.TokenMetadata
	tokenURI persist.TokenURI
	tids     persist.TokenIdentifiers
}

func (e errNoDataFromReader) Error() string {
	return fmt.Sprintf("no data from reader: %s (url: %s)", e.err, e.url)
}

func (e errNotCacheable) Error() string {
	return fmt.Sprintf("unsupported media for caching: %s (mediaURL: %s)", e.mediaType, e.url)
}

func (e errInvalidMedia) Error() string {
	return fmt.Sprintf("invalid media: %s (url: %s)", e.err, e.url)
}

func (e errNoMediaURLs) Error() string {
	return fmt.Sprintf("no media URLs found in metadata: %s (metadata: %+v, tokenURI: %s)", e.tids, e.metadata, e.tokenURI)
}

func (e errNoCachedObjects) Error() string {
	return fmt.Sprintf("no cached objects found for tids: %s", e.tids)
}

type mediaWithContentType struct {
	mediaType   persist.MediaType
	contentType string
}

var postfixesToMediaTypes = map[string]mediaWithContentType{
	"jpg":  {persist.MediaTypeImage, "image/jpeg"},
	"jpeg": {persist.MediaTypeImage, "image/jpeg"},
	"png":  {persist.MediaTypeImage, "image/png"},
	"webp": {persist.MediaTypeImage, "image/webp"},
	"gif":  {persist.MediaTypeGIF, "image/gif"},
	"mp4":  {persist.MediaTypeVideo, "video/mp4"},
	"webm": {persist.MediaTypeVideo, "video/webm"},
	"glb":  {persist.MediaTypeAnimation, "model/gltf-binary"},
	"gltf": {persist.MediaTypeAnimation, "model/gltf+json"},
	"svg":  {persist.MediaTypeImage, "image/svg+xml"},
	"pdf":  {persist.MediaTypePDF, "application/pdf"},
	"html": {persist.MediaTypeHTML, "text/html"},
}

func NewStorageClient(ctx context.Context) *storage.Client {
	opts := append([]option.ClientOption{}, option.WithScopes([]string{storage.ScopeFullControl}...))

	if env.GetString("ENV") == "local" {
		fi, err := util.LoadEncryptedServiceKeyOrError("./secrets/dev/service-key-dev.json")
		if err != nil {
			logger.For(ctx).WithError(err).Error("failed to find service key file (local), running without storage client")
			return nil
		}
		opts = append(opts, option.WithCredentialsJSON(fi))
	}

	transport, err := htransport.NewTransport(ctx, tracing.NewTracingTransport(http.DefaultTransport, false), opts...)
	if err != nil {
		panic(err)
	}

	client, _, err := htransport.NewClient(ctx)
	if err != nil {
		panic(err)
	}
	client.Transport = transport

	storageClient, err := storage.NewClient(ctx, option.WithHTTPClient(client))
	if err != nil {
		panic(err)
	}

	storageClient.SetRetry(storage.WithPolicy(storage.RetryAlways), storage.WithBackoff(gax.Backoff{Initial: 100 * time.Millisecond, Max: 10 * time.Second, Multiplier: 1.3}), storage.WithErrorFunc(storage.ShouldRetry))

	return storageClient
}

// MakePreviewsForMetadata uses a metadata map to generate media content and cache resized versions of the media content.
func MakePreviewsForMetadata(pCtx context.Context, metadata persist.TokenMetadata, contractAddress persist.Address, tokenID persist.TokenID, tokenURI persist.TokenURI, chain persist.Chain, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string, imageKeywords, animationKeywords Keywords) (persist.Media, error) {

	pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{
		"contractAddress": contractAddress,
		"tokenID":         tokenID,
		"chain":           chain,
	})

	imgURL, animURL := FindImageAndAnimationURLs(pCtx, tokenID, contractAddress, metadata, tokenURI, animationKeywords, imageKeywords, true)

	tids := persist.NewTokenIdentifiers(contractAddress, tokenID, chain)
	if imgURL == "" && animURL == "" {
		return persist.Media{
			MediaType: persist.MediaTypeInvalid,
		}, errNoMediaURLs{metadata: metadata, tokenURI: tokenURI, tids: tids}
	}

	logger.For(pCtx).Infof("got imgURL=%s;animURL=%s", imgURL, animURL)

	var (
		imgCh, animCh         chan cacheResult
		imgResult, animResult cacheResult
	)

	if animURL != "" {
		animCh = asyncCacheObjectsForURL(pCtx, tids, storageClient, arweaveClient, ipfsClient, ObjectTypeAnimation, animURL, tokenBucket)
	}
	if imgURL != "" {
		imgCh = asyncCacheObjectsForURL(pCtx, tids, storageClient, arweaveClient, ipfsClient, ObjectTypeImage, imgURL, tokenBucket)
	}

	if animCh != nil {
		animResult = <-animCh
	}
	if imgCh != nil {
		imgResult = <-imgCh
	}

	objects := append(animResult.cachedObjects, imgResult.cachedObjects...)

	// we expectedly did not cache the given media type
	if notCacheableErr, ok := animResult.err.(errNotCacheable); ok {
		return createRawMedia(pCtx, tids, notCacheableErr.mediaType, tokenBucket, animURL, imgURL, objects), nil
	} else if notCacheableErr, ok := imgResult.err.(errNotCacheable); ok {
		return createRawMedia(pCtx, tids, notCacheableErr.mediaType, tokenBucket, animURL, imgURL, objects), nil
	}

	// neither download worked, unexpectedly
	if (animResult.err != nil && len(animResult.cachedObjects) == 0) && (imgResult.err != nil && len(imgResult.cachedObjects) == 0) {

		if invalidMediaErr, ok := animResult.err.(errInvalidMedia); ok {
			return persist.Media{
				MediaURL:  persist.NullString(invalidMediaErr.url),
				MediaType: persist.MediaTypeInvalid,
			}, util.MultiErr{animResult.err, imgResult.err}
		}
		if invalidMediaErr, ok := imgResult.err.(errInvalidMedia); ok {
			return persist.Media{
				MediaURL:  persist.NullString(invalidMediaErr.url),
				MediaType: persist.MediaTypeInvalid,
			}, util.MultiErr{animResult.err, imgResult.err}
		}

		return persist.Media{}, util.MultiErr{animResult.err, imgResult.err}
	}

	// we should never get here, the caching should always return at least one object or an error saying why it didn't
	if len(objects) == 0 {
		return persist.Media{
			MediaType: persist.MediaTypeInvalid,
		}, errNoCachedObjects{tids: tids}
	}

	res := createMediaFromCachedObjects(pCtx, tokenBucket, objects)

	logger.For(pCtx).Infof("media for %s: %+v", tids, res)
	return res, nil
}

func createRawMedia(pCtx context.Context, tids persist.TokenIdentifiers, mediaType persist.MediaType, tokenBucket, animURL, imgURL string, objects []cachedMediaObject) persist.Media {
	switch mediaType {
	case persist.MediaTypeHTML:
		return getHTMLMedia(pCtx, tids, tokenBucket, animURL, imgURL, objects)
	default:
		panic(fmt.Sprintf("media type %s should be cached", mediaType))
	}

}

func createMediaFromCachedObjects(ctx context.Context, tokenBucket string, objects []cachedMediaObject) persist.Media {
	var primaryObject cachedMediaObject
	for _, obj := range objects {
		switch obj.objectType {
		case ObjectTypeAnimation:
			primaryObject = obj
			break
		case ObjectTypeImage, ObjectTypeSVG:
			primaryObject = obj
		}
	}

	var thumbnailObject *cachedMediaObject
	var liveRenderObject *cachedMediaObject
	if primaryObject.objectType == ObjectTypeAnimation {
		for _, obj := range objects {
			if obj.objectType == ObjectTypeImage || obj.objectType == ObjectTypeSVG {
				thumbnailObject = &obj
			} else if obj.objectType == ObjectTypeThumbnail {
				thumbnailObject = &obj
				break
			}
		}
	} else {
		for _, obj := range objects {
			if obj.objectType == ObjectTypeThumbnail {
				thumbnailObject = &obj
				break
			}
		}
	}

	for _, obj := range objects {
		if obj.objectType == ObjectTypeLiveRender {
			liveRenderObject = &obj
			break
		}
	}

	result := persist.Media{
		MediaURL:  persist.NullString(primaryObject.storageURL(tokenBucket)),
		MediaType: primaryObject.mediaType,
	}

	if thumbnailObject != nil {
		result.ThumbnailURL = persist.NullString(thumbnailObject.storageURL(tokenBucket))
	}

	if liveRenderObject != nil {
		result.LivePreviewURL = persist.NullString(liveRenderObject.storageURL(tokenBucket))
	}

	var err error
	switch result.MediaType {
	case persist.MediaTypeSVG:
		result.Dimensions, err = getSvgDimensions(ctx, result.MediaURL.String())
	default:
		result.Dimensions, err = getMediaDimensions(ctx, result.MediaURL.String())
	}

	if err != nil {
		logger.For(ctx).WithError(err).Error("failed to get dimensions for media")
	}

	return result
}

type cacheResult struct {
	cachedObjects []cachedMediaObject
	err           error
}

func asyncCacheObjectsForURL(ctx context.Context, tids persist.TokenIdentifiers, storageClient *storage.Client, arweaveClient *goar.Client, ipfsClient *shell.Shell, defaultObjectType objectType, mediaURL, bucket string) chan cacheResult {
	resultCh := make(chan cacheResult)
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"tokenURIType":      persist.TokenURI(mediaURL).Type(),
		"defaultObjectType": defaultObjectType,
		"mediaURL":          mediaURL,
	})

	go func() {
		cachedObjects, err := cacheObjectsFromURL(ctx, tids, mediaURL, defaultObjectType, ipfsClient, arweaveClient, storageClient, bucket, false)
		if err == nil {
			resultCh <- cacheResult{cachedObjects, err}
			return
		}

		switch caught := err.(type) {
		case *googleapi.Error:
			panic(fmt.Errorf("googleAPI error %s: %s", caught, err))
		default:
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
			resultCh <- cacheResult{cachedObjects, err}
		}
	}()

	return resultCh
}

type svgDimensions struct {
	XMLName xml.Name `xml:"svg"`
	Width   string   `xml:"width,attr"`
	Height  string   `xml:"height,attr"`
	Viewbox string   `xml:"viewBox,attr"`
}

func getSvgDimensions(ctx context.Context, url string) (persist.Dimensions, error) {
	buf := &bytes.Buffer{}
	if strings.HasPrefix(url, "http") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return persist.Dimensions{}, err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return persist.Dimensions{}, err
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return persist.Dimensions{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		_, err = io.Copy(buf, resp.Body)
		if err != nil {
			return persist.Dimensions{}, err
		}
	} else {
		buf = bytes.NewBufferString(url)
	}

	if bytes.HasSuffix(buf.Bytes(), []byte(`<!-- Generated by SVGo -->`)) {
		buf = bytes.NewBuffer(bytes.TrimSuffix(buf.Bytes(), []byte(`<!-- Generated by SVGo -->`)))
	}

	var s svgDimensions
	if err := xml.NewDecoder(buf).Decode(&s); err != nil {
		return persist.Dimensions{}, err
	}

	if (s.Width == "" || s.Height == "") && s.Viewbox == "" {
		return persist.Dimensions{}, fmt.Errorf("no dimensions found for %s", url)
	}

	if s.Viewbox != "" {
		parts := strings.Split(s.Viewbox, " ")
		if len(parts) != 4 {
			return persist.Dimensions{}, fmt.Errorf("invalid viewbox for %s", url)
		}
		s.Width = parts[2]
		s.Height = parts[3]

	}

	width, err := strconv.Atoi(s.Width)
	if err != nil {
		return persist.Dimensions{}, err
	}

	height, err := strconv.Atoi(s.Height)
	if err != nil {
		return persist.Dimensions{}, err
	}

	return persist.Dimensions{
		Width:  width,
		Height: height,
	}, nil
}

func getHTMLMedia(pCtx context.Context, tids persist.TokenIdentifiers, tokenBucket, vURL, imgURL string, cachedObjects []cachedMediaObject) persist.Media {
	res := persist.Media{
		MediaType: persist.MediaTypeHTML,
	}

	if vURL != "" {
		logger.For(pCtx).Infof("using vURL for %s: %s", tids, vURL)
		res.MediaURL = persist.NullString(vURL)
	} else if imgURL != "" {
		logger.For(pCtx).Infof("using imgURL for %s: %s", tids, imgURL)
		res.MediaURL = persist.NullString(imgURL)
	}
	if len(cachedObjects) > 0 {
		for _, obj := range cachedObjects {
			if obj.objectType == ObjectTypeThumbnail {
				res.ThumbnailURL = persist.NullString(obj.storageURL(tokenBucket))
				break
			} else if obj.objectType == ObjectTypeImage {
				res.ThumbnailURL = persist.NullString(obj.storageURL(tokenBucket))
			}
		}
	}
	res = remapMedia(res)

	dimensions, err := getHTMLDimensions(pCtx, res.MediaURL.String())
	if err != nil {
		logger.For(pCtx).Errorf("failed to get dimensions for %s: %v", tids, err)
	}

	res.Dimensions = dimensions

	return res
}

type iframeDimensions struct {
	XMLName xml.Name `xml:"iframe"`
	Width   string   `xml:"width,attr"`
	Height  string   `xml:"height,attr"`
}

func getHTMLDimensions(ctx context.Context, url string) (persist.Dimensions, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return persist.Dimensions{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return persist.Dimensions{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return persist.Dimensions{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var s iframeDimensions
	if err := xml.NewDecoder(resp.Body).Decode(&s); err != nil {
		return persist.Dimensions{}, err
	}

	if s.Width == "" || s.Height == "" {
		return persist.Dimensions{}, fmt.Errorf("no dimensions found for %s", url)
	}

	width, err := strconv.Atoi(s.Width)
	if err != nil {
		return persist.Dimensions{}, err
	}

	height, err := strconv.Atoi(s.Height)
	if err != nil {
		return persist.Dimensions{}, err
	}

	return persist.Dimensions{
		Width:  width,
		Height: height,
	}, nil

}

func remapPaths(mediaURL string) string {
	switch persist.TokenURI(mediaURL).Type() {
	case persist.URITypeIPFS, persist.URITypeIPFSAPI:
		path := util.GetURIPath(mediaURL, false)
		return fmt.Sprintf("%s/ipfs/%s", env.GetString("IPFS_URL"), path)
	case persist.URITypeArweave:
		path := util.GetURIPath(mediaURL, false)
		return fmt.Sprintf("https://arweave.net/%s", path)
	default:
		return mediaURL
	}

}

func remapMedia(media persist.Media) persist.Media {
	media.MediaURL = persist.NullString(remapPaths(strings.TrimSpace(media.MediaURL.String())))
	media.ThumbnailURL = persist.NullString(remapPaths(strings.TrimSpace(media.ThumbnailURL.String())))
	if !persist.TokenURI(media.ThumbnailURL).IsRenderable() {
		media.ThumbnailURL = persist.NullString("")
	}
	return media
}

func FindImageAndAnimationURLs(ctx context.Context, tokenID persist.TokenID, contractAddress persist.Address, metadata persist.TokenMetadata, tokenURI persist.TokenURI, animationKeywords, imageKeywords Keywords, predict bool) (imgURL string, vURL string) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"tokenID": tokenID, "contractAddress": contractAddress})
	if metaMedia, ok := metadata["media"].(map[string]any); ok {
		logger.For(ctx).Debugf("found media metadata: %s", metaMedia)
		var mediaType persist.MediaType

		if mime, ok := metaMedia["mimeType"].(string); ok {
			mediaType = persist.MediaFromContentType(mime)
		}
		if uri, ok := metaMedia["uri"].(string); ok {
			switch mediaType {
			case persist.MediaTypeImage, persist.MediaTypeSVG, persist.MediaTypeGIF:
				imgURL = uri
			default:
				vURL = uri
			}
		}
	}

	for _, keyword := range animationKeywords.ForToken(tokenID, contractAddress) {
		if it, ok := util.GetValueFromMapUnsafe(metadata, keyword, util.DefaultSearchDepth).(string); ok && it != "" {
			logger.For(ctx).Debugf("found initial animation url from '%s': %s", keyword, it)
			vURL = it
			break
		}
	}

	for _, keyword := range imageKeywords.ForToken(tokenID, contractAddress) {
		if it, ok := util.GetValueFromMapUnsafe(metadata, keyword, util.DefaultSearchDepth).(string); ok && it != "" && it != vURL {
			logger.For(ctx).Debugf("found initial image url from '%s': %s", keyword, it)
			imgURL = it
			break
		}
	}

	if imgURL == "" && vURL == "" {
		logger.For(ctx).Debugf("no image url found, using token URI: %s", tokenURI)
		imgURL = tokenURI.String()
	}

	if predict {
		return predictTrueURLs(ctx, imgURL, vURL)
	}
	return imgURL, vURL

}

func FindNameAndDescription(ctx context.Context, metadata persist.TokenMetadata) (string, string) {
	name, ok := util.GetValueFromMapUnsafe(metadata, "name", util.DefaultSearchDepth).(string)
	if !ok {
		name = ""
	}

	description, ok := util.GetValueFromMapUnsafe(metadata, "description", util.DefaultSearchDepth).(string)
	if !ok {
		description = ""
	}

	return name, description
}

func predictTrueURLs(ctx context.Context, curImg, curV string) (string, string) {
	imgMediaType, _, _, err := PredictMediaType(ctx, curImg)
	if err != nil {
		return curImg, curV
	}
	vMediaType, _, _, err := PredictMediaType(ctx, curV)
	if err != nil {
		return curImg, curV
	}

	if imgMediaType.IsAnimationLike() && !vMediaType.IsAnimationLike() {
		return curV, curImg
	}

	if !imgMediaType.IsValid() || !vMediaType.IsValid() {
		return curImg, curV
	}

	if imgMediaType.IsMorePriorityThan(vMediaType) {
		return curV, curImg
	}

	return curImg, curV
}

func getThumbnailURL(pCtx context.Context, tokenBucket string, name string, imgURL string, storageClient *storage.Client) string {
	if storageImageURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("image-%s", name), storageClient); err == nil {
		logger.For(pCtx).Infof("found imageURL for thumbnail %s: %s", name, storageImageURL)
		return storageImageURL
	} else if storageImageURL, err = getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("svg-%s", name), storageClient); err == nil {
		logger.For(pCtx).Infof("found svg for thumbnail %s: %s", name, storageImageURL)
		return storageImageURL
	} else if imgURL != "" && persist.TokenURI(imgURL).IsRenderable() {
		logger.For(pCtx).Infof("using imgURL for thumbnail %s: %s", name, imgURL)
		return imgURL
	} else if storageImageURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("thumbnail-%s", name), storageClient); err == nil {
		logger.For(pCtx).Infof("found thumbnailURL for %s: %s", name, storageImageURL)
		return storageImageURL
	}
	return ""
}

func objectExists(ctx context.Context, client *storage.Client, bucket, fileName string) (bool, error) {
	objHandle := client.Bucket(bucket).Object(fileName)
	_, err := objHandle.Attrs(ctx)
	if err != nil && err != storage.ErrObjectNotExist {
		return false, fmt.Errorf("could not get object attrs for %s: %s", objHandle.ObjectName(), err)
	}
	return err != storage.ErrObjectNotExist, nil
}

func purgeIfExists(ctx context.Context, bucket string, fileName string, client *storage.Client) error {
	exists, err := objectExists(ctx, client, bucket, fileName)
	if err != nil {
		return err
	}
	if exists {
		if err := mediamapper.PurgeImage(ctx, fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, fileName)); err != nil {
			logger.For(ctx).WithError(err).Errorf("could not purge file %s", fileName)
		}
	}

	return nil
}

func persistToStorage(ctx context.Context, client *storage.Client, reader io.Reader, bucket, fileName string, contentType *string, contentLength *int64, metadata map[string]string) error {
	writer := newObjectWriter(ctx, client, bucket, fileName, contentType, contentLength, metadata)
	if err := retryWriteToCloudStorage(ctx, writer, reader); err != nil {
		return fmt.Errorf("could not write to bucket %s for %s: %s", bucket, fileName, err)
	}
	return writer.Close()
}

func retryWriteToCloudStorage(ctx context.Context, writer io.Writer, reader io.Reader) error {
	return retry.RetryFunc(ctx, func(ctx context.Context) error {
		if _, err := io.Copy(writer, reader); err != nil {
			return err
		}
		return nil
	}, storage.ShouldRetry, retry.DefaultRetry)
}

type objectType int

const (
	ObjectTypeUnknown objectType = iota
	ObjectTypeImage
	ObjectTypeAnimation
	ObjectTypeThumbnail
	ObjectTypeLiveRender
	ObjectTypeSVG
)

func (o objectType) String() string {
	switch o {
	case ObjectTypeImage:
		return "image"
	case ObjectTypeAnimation:
		return "animation"
	case ObjectTypeThumbnail:
		return "thumbnail"
	case ObjectTypeLiveRender:
		return "liverender"
	case ObjectTypeSVG:
		return "svg"
	default:
		panic(fmt.Sprintf("unknown object type: %d", o))
	}
}

type cachedMediaObject struct {
	mediaType       persist.MediaType
	tokenID         persist.TokenID
	contractAddress persist.Address
	chain           persist.Chain
	contentType     *string
	contentLength   *int64
	objectType      objectType
}

func (m cachedMediaObject) fileName() string {
	if m.objectType.String() == "" || m.tokenID == "" || m.contractAddress == "" {
		panic(fmt.Sprintf("invalid media object: %+v", m))
	}
	return fmt.Sprintf("%d-%s-%s-%s", m.chain, m.tokenID, m.contractAddress, m.objectType)
}

func (m cachedMediaObject) storageURL(tokenBucket string) string {
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", tokenBucket, m.fileName())
}

func cacheRawMedia(ctx context.Context, reader io.Reader, tids persist.TokenIdentifiers, mediaType persist.MediaType, contentLength *int64, contentType *string, defaultObjectType objectType, bucket, ogURL string, client *storage.Client) (cachedMediaObject, error) {

	var objectType objectType
	switch mediaType {
	case persist.MediaTypeVideo:
		objectType = ObjectTypeAnimation
	case persist.MediaTypeSVG:
		objectType = ObjectTypeSVG
	case persist.MediaTypeBase64BMP:
		objectType = ObjectTypeImage
	default:
		objectType = defaultObjectType
	}
	object := cachedMediaObject{
		mediaType:       mediaType,
		tokenID:         tids.TokenID,
		contractAddress: tids.ContractAddress,
		chain:           tids.Chain,
		contentType:     contentType,
		contentLength:   contentLength,
		objectType:      objectType,
	}
	err := persistToStorage(ctx, client, reader, bucket, object.fileName(), object.contentType, object.contentLength,
		map[string]string{
			"originalURL": ogURL,
			"mediaType":   mediaType.String(),
		})
	go purgeIfExists(context.Background(), bucket, object.fileName(), client)
	return object, err
}

func cacheRawAnimationMedia(ctx context.Context, reader io.Reader, tids persist.TokenIdentifiers, mediaType persist.MediaType, bucket, ogURL string, client *storage.Client) (cachedMediaObject, error) {

	object := cachedMediaObject{
		mediaType:       mediaType,
		tokenID:         tids.TokenID,
		contractAddress: tids.ContractAddress,
		chain:           tids.Chain,
		objectType:      ObjectTypeAnimation,
	}

	sw := newObjectWriter(ctx, client, bucket, object.fileName(), nil, nil, map[string]string{
		"originalURL": ogURL,
		"mediaType":   mediaType.String(),
	})
	writer := gzip.NewWriter(sw)

	err := retryWriteToCloudStorage(ctx, writer, reader)
	if err != nil {
		return cachedMediaObject{}, fmt.Errorf("could not write to bucket %s for %s: %s", bucket, object.fileName(), err)
	}

	if err := writer.Close(); err != nil {
		return cachedMediaObject{}, err
	}

	if err := sw.Close(); err != nil {
		return cachedMediaObject{}, err
	}

	go purgeIfExists(context.Background(), bucket, object.fileName(), client)
	return object, nil
}

func thumbnailAndCache(ctx context.Context, tids persist.TokenIdentifiers, videoURL, bucket string, client *storage.Client) (cachedMediaObject, error) {

	obj := cachedMediaObject{
		objectType:      ObjectTypeThumbnail,
		mediaType:       persist.MediaTypeImage,
		tokenID:         tids.TokenID,
		contractAddress: tids.ContractAddress,
		chain:           tids.Chain,
		contentType:     util.ToPointer("image/png"),
	}

	logger.For(ctx).Infof("caching thumbnail for '%s'", obj.fileName())

	timeBeforeCopy := time.Now()

	sw := newObjectWriter(ctx, client, bucket, obj.fileName(), util.ToPointer("image/jpeg"), nil, map[string]string{
		"thumbnailedURL": videoURL,
	})

	logger.For(ctx).Infof("thumbnailing %s", videoURL)
	if err := thumbnailVideoToWriter(ctx, videoURL, sw); err != nil {
		return cachedMediaObject{}, fmt.Errorf("could not thumbnail to bucket %s for '%s': %s", bucket, obj.fileName(), err)
	}

	if err := sw.Close(); err != nil {
		return cachedMediaObject{}, err
	}

	logger.For(ctx).Infof("storage copy took %s", time.Since(timeBeforeCopy))

	go purgeIfExists(context.Background(), bucket, obj.fileName(), client)

	return obj, nil
}

func createLiveRenderAndCache(ctx context.Context, tids persist.TokenIdentifiers, videoURL, bucket string, client *storage.Client) (cachedMediaObject, error) {

	obj := cachedMediaObject{
		objectType:      ObjectTypeLiveRender,
		mediaType:       persist.MediaTypeImage,
		tokenID:         tids.TokenID,
		contractAddress: tids.ContractAddress,
		chain:           tids.Chain,
		contentType:     util.ToPointer("video/mp4"),
	}

	logger.For(ctx).Infof("caching live render media for '%s'", obj.fileName())

	timeBeforeCopy := time.Now()

	sw := newObjectWriter(ctx, client, bucket, obj.fileName(), util.ToPointer("video/mp4"), nil, map[string]string{
		"liveRenderedURL": videoURL,
	})

	logger.For(ctx).Infof("creating live render for %s", videoURL)
	if err := createLiveRenderPreviewVideo(ctx, videoURL, sw); err != nil {
		return cachedMediaObject{}, fmt.Errorf("could not live render to bucket %s for '%s': %s", bucket, obj.fileName(), err)
	}

	if err := sw.Close(); err != nil {
		return cachedMediaObject{}, err
	}

	logger.For(ctx).Infof("storage copy took %s", time.Since(timeBeforeCopy))

	go purgeIfExists(context.Background(), bucket, obj.fileName(), client)

	return obj, nil
}

func deleteMedia(ctx context.Context, bucket, fileName string, client *storage.Client) error {
	return client.Bucket(bucket).Object(fileName).Delete(ctx)
}

func getMediaServingURL(pCtx context.Context, bucketID, objectID string, client *storage.Client) (string, error) {
	if exists, err := objectExists(pCtx, client, bucketID, objectID); err != nil || !exists {
		objectName := fmt.Sprintf("/gs/%s/%s", bucketID, objectID)
		return "", fmt.Errorf("failed to check if object %s exists: %s", objectName, err)
	}
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketID, objectID), nil
}

func cacheObjectsFromURL(pCtx context.Context, tids persist.TokenIdentifiers, mediaURL string, defaultObjectType objectType, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, bucket string, isRecursive bool) ([]cachedMediaObject, error) {

	asURI := persist.TokenURI(mediaURL)
	timeBeforePredict := time.Now()
	mediaType, contentType, contentLength, _ := PredictMediaType(pCtx, asURI.String())
	pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{
		"predictedMediaType":   mediaType,
		"predictedContentType": contentType,
	})

	contentLengthStr := "nil"
	if contentLength != nil {
		contentLengthStr = util.InByteSizeFormat(uint64(util.FromPointer(contentLength)))
	}
	pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{
		"contentLength": contentLength,
	})
	logger.For(pCtx).Infof("predicted media type from '%s' as '%s' with length %s in %s", mediaURL, mediaType, contentLengthStr, time.Since(timeBeforePredict))

	if mediaType == persist.MediaTypeHTML {
		return nil, errNotCacheable{url: mediaURL, mediaType: mediaType}
	}

	timeBeforeDataReader := time.Now()
	reader, err := rpc.GetDataFromURIAsReader(pCtx, asURI, ipfsClient, arweaveClient)
	if err != nil {

		// the reader is and always will be invalid
		switch caught := err.(type) {
		case rpc.ErrHTTP:
			if caught.Status == http.StatusNotFound {
				return nil, errInvalidMedia{url: mediaURL, err: err}
			}
		case *net.DNSError:
			return nil, errInvalidMedia{url: mediaURL, err: err}
		}

		// if we're not already recursive, try opensea for ethereum tokens
		if !isRecursive && tids.Chain == persist.ChainETH {
			logger.For(pCtx).Infof("failed to get data from uri '%s' for '%s', trying opensea", mediaURL, tids)
			// if token is ETH, backup to asking opensea
			assets, err := opensea.FetchAssetsForTokenIdentifiers(pCtx, persist.EthereumAddress(tids.ContractAddress), opensea.TokenID(tids.TokenID.Base10String()))
			if err != nil || len(assets) == 0 {
				// no data from opensea, return error
				return nil, errNoDataFromReader{err: err, url: mediaURL}
			}

			for _, asset := range assets {
				// does this asset have any valid URLs?
				firstNonEmptyURL, ok := util.FindFirst([]string{asset.AnimationURL, asset.ImageURL, asset.ImagePreviewURL, asset.ImageOriginalURL, asset.ImageThumbnailURL}, func(s string) bool {
					return s != ""
				})
				if !ok {
					continue
				}

				reader, err = rpc.GetDataFromURIAsReader(pCtx, persist.TokenURI(firstNonEmptyURL), ipfsClient, arweaveClient)
				if err != nil {
					continue
				}

				logger.For(pCtx).Infof("got reader for %s from opensea in %s (%s)", tids, time.Since(timeBeforeDataReader), firstNonEmptyURL)
				return cacheObjectsFromURL(pCtx, tids, firstNonEmptyURL, defaultObjectType, ipfsClient, arweaveClient, storageClient, bucket, true)
			}
		}

		return nil, errNoDataFromReader{err: err, url: mediaURL}
	}
	logger.For(pCtx).Infof("got reader for %s in %s", tids, time.Since(timeBeforeDataReader))
	defer reader.Close()

	if !mediaType.IsValid() {
		timeBeforeSniff := time.Now()
		bytesToSniff, _ := reader.Headers()
		var contentType = util.ToPointer("")
		mediaType, *contentType = persist.SniffMediaType(bytesToSniff)
		logger.For(pCtx).Infof("sniffed media type for %s: %s in %s", truncateString(mediaURL, 50), mediaType, time.Since(timeBeforeSniff))
	}

	pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{
		"finalMediaType":   mediaType,
		"finalContentType": contentType,
	})

	asMb := 0.0
	if contentLength != nil && *contentLength > 0 {
		asMb = float64(*contentLength) / 1024 / 1024
	}
	logger.For(pCtx).Infof("caching %.2f mb of raw media with type '%s' for '%s' at '%s-%s'", asMb, mediaType, mediaURL, defaultObjectType, tids)

	if mediaType == persist.MediaTypeAnimation {
		timeBeforeCache := time.Now()
		obj, err := cacheRawAnimationMedia(pCtx, reader, tids, mediaType, bucket, mediaURL, storageClient)
		if err != nil {
			return nil, err
		}
		logger.For(pCtx).Infof("cached animation for %s in %s", tids, time.Since(timeBeforeCache))
		return []cachedMediaObject{obj}, nil
	}

	timeBeforeCache := time.Now()
	obj, err := cacheRawMedia(pCtx, reader, tids, mediaType, contentLength, contentType, defaultObjectType, bucket, mediaURL, storageClient)
	if err != nil {
		return nil, err
	}
	logger.For(pCtx).Infof("cached media for %s in %s", tids, time.Since(timeBeforeCache))

	result := []cachedMediaObject{obj}
	if mediaType == persist.MediaTypeVideo {
		videoURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, obj.fileName())
		thumbObj, err := thumbnailAndCache(pCtx, tids, videoURL, bucket, storageClient)
		if err != nil {
			logger.For(pCtx).Errorf("could not create thumbnail for %s: %s", tids, err)
		} else {
			result = append(result, thumbObj)
		}

		liveObj, err := createLiveRenderAndCache(pCtx, tids, videoURL, bucket, storageClient)
		if err != nil {
			logger.For(pCtx).Errorf("could not create live render for %s: %s", tids, err)
		} else {
			result = append(result, liveObj)
		}

	}

	return result, nil
}

// PredictMediaType guesses the media type of the given URL.
func PredictMediaType(pCtx context.Context, url string) (persist.MediaType, *string, *int64, error) {

	spl := strings.Split(url, ".")
	if len(spl) > 1 {
		ext := spl[len(spl)-1]
		ext = strings.Split(ext, "?")[0]
		if t, ok := postfixesToMediaTypes[ext]; ok {
			return t.mediaType, &t.contentType, nil, nil
		}
	}
	asURI := persist.TokenURI(url)
	lenURI := int64(len(asURI.String()))
	uriType := asURI.Type()
	logger.For(pCtx).Debugf("predicting media type for %s with URI type %s", url, uriType)
	switch uriType {
	case persist.URITypeBase64JSON, persist.URITypeJSON:
		return persist.MediaTypeJSON, util.ToPointer("application/json"), &lenURI, nil
	case persist.URITypeBase64SVG, persist.URITypeSVG:
		return persist.MediaTypeSVG, util.ToPointer("image/svg"), &lenURI, nil
	case persist.URITypeBase64BMP:
		return persist.MediaTypeBase64BMP, util.ToPointer("image/bmp"), &lenURI, nil
	case persist.URITypeBase64PNG:
		return persist.MediaTypeBase64PNG, util.ToPointer("image/png"), &lenURI, nil
	case persist.URITypeIPFS:
		contentType, contentLength, err := rpc.GetIPFSHeaders(pCtx, strings.TrimPrefix(asURI.String(), "ipfs://"))
		if err != nil {
			return persist.MediaTypeUnknown, nil, nil, err
		}
		return persist.MediaFromContentType(contentType), &contentType, &contentLength, nil
	case persist.URITypeIPFSGateway:
		contentType, contentLength, err := rpc.GetIPFSHeaders(pCtx, util.GetURIPath(asURI.String(), false))
		if err == nil {
			return persist.MediaFromContentType(contentType), &contentType, &contentLength, nil
		} else if err != nil {
			logger.For(pCtx).Errorf("could not get IPFS headers for %s: %s", url, err)
		}
		fallthrough
	case persist.URITypeHTTP, persist.URITypeIPFSAPI:
		contentType, contentLength, err := rpc.GetHTTPHeaders(pCtx, url)
		if err != nil {
			return persist.MediaTypeUnknown, nil, nil, err
		}
		return persist.MediaFromContentType(contentType), &contentType, &contentLength, nil
	}
	return persist.MediaTypeUnknown, nil, nil, nil
}

func thumbnailVideoToWriter(ctx context.Context, url string, writer io.Writer) error {
	c := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-i", url, "-ss", "00:00:00.000", "-vframes", "1", "-f", "mjpeg", "pipe:1")
	c.Stderr = os.Stderr
	c.Stdout = writer
	return c.Run()
}

func createLiveRenderPreviewVideo(ctx context.Context, videoURL string, writer io.Writer) error {
	c := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-i", videoURL, "-ss", "00:00:00.000", "-t", "00:00:05.000", "-filter:v", "scale=720:-1", "-movflags", "frag_keyframe+empty_moov", "-c:a", "copy", "-f", "mp4", "pipe:1")
	c.Stderr = os.Stderr
	c.Stdout = writer
	return c.Run()
}

type dimensions struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

type errNoStreams struct {
	url string
	err error
}

func (e errNoStreams) Error() string {
	return fmt.Sprintf("no streams in %s: %s", e.url, e.err)
}

func getMediaDimensions(ctx context.Context, url string) (persist.Dimensions, error) {
	outBuf := &bytes.Buffer{}
	c := exec.CommandContext(ctx, "ffprobe", "-hide_banner", "-loglevel", "error", "-show_streams", url, "-print_format", "json")
	c.Stderr = os.Stderr
	c.Stdout = outBuf
	err := c.Run()
	if err != nil {
		return persist.Dimensions{}, err
	}

	var d dimensions
	err = json.Unmarshal(outBuf.Bytes(), &d)
	if err != nil {
		return persist.Dimensions{}, fmt.Errorf("failed to unmarshal ffprobe output: %w", err)
	}

	if len(d.Streams) == 0 {
		return persist.Dimensions{}, fmt.Errorf("no streams found in ffprobe output: %w", err)
	}

	dims := persist.Dimensions{}

	for _, s := range d.Streams {
		if s.Height == 0 || s.Width == 0 {
			continue
		}
		dims = persist.Dimensions{
			Width:  s.Width,
			Height: s.Height,
		}
		break
	}

	logger.For(ctx).Debugf("got dimensions %+v for %s", dims, url)
	return dims, nil
}

func truncateString(s string, i int) string {
	asRunes := []rune(s)
	if len(asRunes) > i {
		return string(asRunes[:i])
	}
	return s
}

func (d DefaultKeywords) ForToken(tokenID persist.TokenID, contract persist.Address) []string {
	return d
}

const (
	hicEtNunc = "KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton"
	fxHash    = "KT1KEa8z6vWXDJrVqtMrAeDVzsvxat3kHaCE"
	fxHash2   = "KT1U6EHmNxJTkvaWJ4ThczG4FSDaHC21ssvi"
)

func IsHicEtNunc(contract persist.Address) bool {
	return contract == hicEtNunc
}

func IsFxHash(contract persist.Address) bool {
	return contract == fxHash || contract == fxHash2
}

func (i TezImageKeywords) ForToken(tokenID persist.TokenID, contract persist.Address) []string {
	switch {
	case IsHicEtNunc(contract):
		return []string{"artifactUri", "displayUri", "image"}
	case IsFxHash(contract):
		return []string{"displayUri", "artifactUri", "image", "uri"}
	default:
		return i
	}
}

func (a TezAnimationKeywords) ForToken(tokenID persist.TokenID, contract persist.Address) []string {
	switch {
	case IsFxHash(contract):
		return []string{"artifactUri", "displayUri"}
	default:
		return a
	}
}

func KeywordsForChain(chain persist.Chain, imageKeywords []string, animationKeywords []string) (Keywords, Keywords) {
	switch chain {
	case persist.ChainTezos:
		return TezImageKeywords(imageKeywords), TezAnimationKeywords(animationKeywords)
	default:
		return DefaultKeywords(imageKeywords), DefaultKeywords(animationKeywords)
	}
}

func (e errUnsupportedURL) Error() string {
	return fmt.Sprintf("unsupported url %s", e.url)
}

func (e errUnsupportedMediaType) Error() string {
	return fmt.Sprintf("unsupported media type %s", e.mediaType)
}

func newObjectWriter(ctx context.Context, client *storage.Client, bucket, fileName string, contentType *string, contentLength *int64, objMetadata map[string]string) *storage.Writer {
	writer := client.Bucket(bucket).Object(fileName).NewWriter(ctx)
	if contentType != nil {
		writer.ObjectAttrs.ContentType = *contentType
	}
	writer.ObjectAttrs.Metadata = objMetadata
	writer.ObjectAttrs.CacheControl = "no-cache, no-store"
	writer.ChunkSize = 4 * 1024 * 1024 // 4MB
	writer.ChunkRetryDeadline = 1 * time.Minute
	if contentLength != nil {
		cl := *contentLength
		if cl < 4*1024*1024 {
			writer.ChunkSize = int(cl)
		} else if cl > 32*1024*1024 {
			writer.ChunkSize = 8 * 1024 * 1024
		}
	}
	return writer
}

func RawFormatToMediaType(format string) persist.MediaType {
	switch format {
	case "jpeg", "png", "image", "jpg", "webp":
		return persist.MediaTypeImage
	case "gif":
		return persist.MediaTypeGIF
	case "video", "mp4", "quicktime":
		return persist.MediaTypeVideo
	case "audio", "mp3", "wav":
		return persist.MediaTypeAudio
	case "pdf":
		return persist.MediaTypePDF
	case "html", "iframe":
		return persist.MediaTypeHTML
	case "svg", "svg+xml":
		return persist.MediaTypeSVG
	default:
		return persist.MediaTypeUnknown
	}
}
