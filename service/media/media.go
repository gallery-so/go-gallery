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

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/mediamapper"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
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

func (e errNoDataFromReader) Error() string {
	return fmt.Sprintf("no data from reader: %s (url: %s)", e.err, e.url)
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

	return storageClient
}

// MakePreviewsForMetadata uses a metadata map to generate media content and cache resized versions of the media content.
func MakePreviewsForMetadata(pCtx context.Context, metadata persist.TokenMetadata, contractAddress persist.Address, tokenID persist.TokenID, tokenURI persist.TokenURI, chain persist.Chain, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string, imageKeywords, animationKeywords Keywords) (persist.Media, error) {
	name := fmt.Sprintf("%s-%s", contractAddress, tokenID)
	imgURL, vURL := FindImageAndAnimationURLs(pCtx, tokenID, contractAddress, metadata, tokenURI, animationKeywords, imageKeywords, true)
	logger.For(pCtx).Infof("got imgURL=%s;videoURL=%s", imgURL, vURL)

	var (
		imgCh, vidCh         chan cacheResult
		imgResult, vidResult cacheResult
		mediaType            persist.MediaType
		res                  persist.Media
	)

	tids := persist.NewTokenIdentifiers(contractAddress, tokenID, chain)
	if vURL != "" {
		vidCh = downloadMediaFromURL(pCtx, tids, storageClient, arweaveClient, ipfsClient, "video", vURL, name, tokenBucket)
	}
	if imgURL != "" {
		imgCh = downloadMediaFromURL(pCtx, tids, storageClient, arweaveClient, ipfsClient, "image", imgURL, name, tokenBucket)
	}

	if vidCh != nil {
		vidResult = <-vidCh
	}
	if imgCh != nil {
		imgResult = <-imgCh
	}

	// Neither download worked
	if (vidResult.err != nil && vidResult.mediaType == "") && (imgResult.err != nil && imgResult.mediaType == "") {
		return persist.Media{}, vidResult.err // Just use the video error
	}

	if imgResult.mediaType != "" {
		mediaType = imgResult.mediaType
	}
	if vidResult.mediaType != "" {
		mediaType = vidResult.mediaType
	}

	if asString, ok := metadata["media_type"].(string); !mediaType.IsValid() && ok && asString != "" {
		mediaType = persist.MediaType(RawFormatToMediaType(asString))
	}

	if asString, ok := metadata["format"].(string); !mediaType.IsValid() && ok && asString != "" {
		mediaType = persist.MediaType(RawFormatToMediaType(asString))
	}

	pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{"mediaType": mediaType})
	logger.For(pCtx).Infof("using '%s' as the mediaType", mediaType)

	deleteCtx, cancel := context.WithTimeout(pCtx, 25*time.Second)

	deletePool := pool.New().WithContext(deleteCtx)

	// if nothing was cached in the image step and the image step did process an image type, delete the now stale cached image
	if !imgResult.cached && imgResult.mediaType.IsImageLike() {
		logger.For(pCtx).Debug("imgResult not cached, deleting cached version if any")

		deletePool.Go(func(ctx context.Context) error {
			return deleteMedia(ctx, tokenBucket, fmt.Sprintf("image-%s", name), storageClient)
		})
	}

	// if nothing was cached in the image step and the image step did process an image type, delete the now stale cached live render
	if !imgResult.cached && imgResult.mediaType.IsAnimationLike() {
		logger.For(pCtx).Debug("imgResult not cached, deleting cached version if any")

		deletePool.Go(func(ctx context.Context) error {
			return deleteMedia(ctx, tokenBucket, fmt.Sprintf("liverender-%s", name), storageClient)
		})
	}
	// if nothing was cached in the video step and the video step did process a video type, delete the now stale cached video
	if !vidResult.cached && vidResult.mediaType.IsAnimationLike() {
		logger.For(pCtx).Debug("vidResult not cached, deleting cached version if any")

		deletePool.Go(func(ctx context.Context) error {
			return deleteMedia(ctx, tokenBucket, fmt.Sprintf("video-%s", name), storageClient)
		})
		deletePool.Go(func(ctx context.Context) error {
			return deleteMedia(ctx, tokenBucket, fmt.Sprintf("liverender-%s", name), storageClient)
		})
	}

	// if something was cached but neither media type is animation type, we can assume that there was nothing thumbnailed therefore any thumbnail or liverender is stale
	if (imgResult.cached || vidResult.cached) && (!imgResult.mediaType.IsAnimationLike() && !vidResult.mediaType.IsAnimationLike()) {
		logger.For(pCtx).Debug("neither cached, deleting thumbnail if any")

		deletePool.Go(func(ctx context.Context) error {
			return deleteMedia(ctx, tokenBucket, fmt.Sprintf("thumbnail-%s", name), storageClient)
		})
		deletePool.Go(func(ctx context.Context) error {
			return deleteMedia(ctx, tokenBucket, fmt.Sprintf("liverender-%s", name), storageClient)
		})
	}

	func() {
		defer cancel()
		deletePool.Wait()
	}()

	// imgURL does not work, but vidURL does, don't try to use imgURL
	if _, ok := imgResult.err.(errNoDataFromReader); ok && (vidResult.cached && vidResult.mediaType.IsAnimationLike()) {
		imgURL = ""
	}

	switch mediaType {
	case persist.MediaTypeImage:
		res = getImageMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL)
	case persist.MediaTypeVideo, persist.MediaTypeAudio, persist.MediaTypeText, persist.MediaTypePDF, persist.MediaTypeAnimation:
		res = getAuxilaryMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL, mediaType)
	case persist.MediaTypeHTML:
		res = getHTMLMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL)
	case persist.MediaTypeGIF:
		res = getGIFMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL)
	case persist.MediaTypeSVG:
		res = getSvgMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL)
	default:
		res = getRawMedia(pCtx, mediaType, name, vURL, imgURL)
	}

	logger.For(pCtx).Infof("media for %s of type %s: %+v", name, mediaType, res)
	return res, nil
}

type cacheResult struct {
	mediaType persist.MediaType
	cached    bool
	err       error
}

func downloadMediaFromURL(ctx context.Context, tids persist.TokenIdentifiers, storageClient *storage.Client, arweaveClient *goar.Client, ipfsClient *shell.Shell, urlType, mediaURL, name, bucket string) chan cacheResult {
	resultCh := make(chan cacheResult)
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"tokenURIType": persist.TokenURI(mediaURL).Type(),
		"urlType":      urlType,
		"mediaURL":     mediaURL,
	})

	go func() {
		mediaType, cached, err := downloadAndCache(ctx, tids, mediaURL, name, urlType, ipfsClient, arweaveClient, storageClient, bucket, false)
		if err == nil {
			resultCh <- cacheResult{mediaType, cached, err}
			return
		}

		switch caught := err.(type) {
		case rpc.ErrHTTP:
			if err.(rpc.ErrHTTP).Status == http.StatusNotFound {
				resultCh <- cacheResult{persist.MediaTypeInvalid, cached, err}
			} else {
				resultCh <- cacheResult{mediaType, cached, err}
			}
		case *net.DNSError:
			resultCh <- cacheResult{persist.MediaTypeInvalid, cached, err}
		case *googleapi.Error:
			panic(fmt.Errorf("googleAPI error %s: %s", caught, err))
		default:
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
			resultCh <- cacheResult{mediaType, cached, err}
		}
	}()

	return resultCh
}

func getAuxilaryMedia(pCtx context.Context, name, tokenBucket string, storageClient *storage.Client, vURL string, imgURL string, mediaType persist.MediaType) persist.Media {
	res := persist.Media{
		MediaType: mediaType,
	}
	videoURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("video-%s", name), storageClient)
	if err == nil {
		vURL = videoURL
	}
	imageURL := getThumbnailURL(pCtx, tokenBucket, name, imgURL, storageClient)
	if vURL != "" {
		logger.For(pCtx).Infof("using vURL %s: %s", name, vURL)
		res.MediaURL = persist.NullString(vURL)
		res.ThumbnailURL = persist.NullString(imageURL)
	} else if imageURL != "" {
		logger.For(pCtx).Infof("using imageURL for %s: %s", name, imageURL)
		res.MediaURL = persist.NullString(imageURL)
	}

	res = remapMedia(res)

	res.Dimensions, err = getMediaDimensions(pCtx, res.MediaURL.String())
	if err != nil {
		logger.For(pCtx).Errorf("failed to get dimensions for %s: %v", name, err)
	}

	if mediaType == persist.MediaTypeVideo {
		liveRenderURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("liverender-%s", name), storageClient)
		if err != nil {
			logger.For(pCtx).Errorf("failed to get live render URL for %s: %v", name, err)
		} else {
			res.LivePreviewURL = persist.NullString(liveRenderURL)
		}
	}

	return res
}

func getGIFMedia(pCtx context.Context, name, tokenBucket string, storageClient *storage.Client, vURL string, imgURL string) persist.Media {
	res := persist.Media{
		MediaType: persist.MediaTypeGIF,
	}
	videoURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("video-%s", name), storageClient)
	if err == nil {
		vURL = videoURL
	}
	imageURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("image-%s", name), storageClient)
	if err == nil {
		logger.For(pCtx).Infof("found imageURL for %s: %s", name, imageURL)
		imgURL = imageURL
	}
	res.ThumbnailURL = persist.NullString(getThumbnailURL(pCtx, tokenBucket, name, imgURL, storageClient))
	if vURL != "" {
		logger.For(pCtx).Infof("using vURL %s: %s", name, vURL)
		res.MediaURL = persist.NullString(vURL)
		if imgURL != "" && res.ThumbnailURL.String() == "" {
			res.ThumbnailURL = persist.NullString(imgURL)
		}
	} else if imgURL != "" {
		logger.For(pCtx).Infof("using imgURL for %s: %s", name, imgURL)
		res.MediaURL = persist.NullString(imgURL)
	}

	res = remapMedia(res)

	res.Dimensions, err = getMediaDimensions(pCtx, res.MediaURL.String())
	if err != nil {
		logger.For(pCtx).Errorf("failed to get dimensions for %s: %v", name, err)
	}

	return res
}

func getSvgMedia(pCtx context.Context, name, tokenBucket string, storageClient *storage.Client, vURL, imgURL string) persist.Media {
	res := persist.Media{
		MediaType: persist.MediaTypeSVG,
	}
	imageURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("svg-%s", name), storageClient)
	if err == nil {
		logger.For(pCtx).Infof("found svgURL for svg %s: %s", name, imageURL)
		res.MediaURL = persist.NullString(imageURL)
	} else {
		if vURL != "" {
			logger.For(pCtx).Infof("using vURL for svg %s: %s", name, vURL)
			res.MediaURL = persist.NullString(vURL)
			if imgURL != "" {
				res.ThumbnailURL = persist.NullString(imgURL)
			}
		} else if imgURL != "" {
			logger.For(pCtx).Infof("using imgURL for svg %s: %s", name, imgURL)
			res.MediaURL = persist.NullString(imgURL)
		}
	}

	res = remapMedia(res)

	res.Dimensions, err = getSvgDimensions(pCtx, res.MediaURL.String())
	if err != nil {
		logger.For(pCtx).Errorf("failed to get dimensions for svg %s: %v", name, err)
	}

	return res
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

func getImageMedia(pCtx context.Context, name, tokenBucket string, storageClient *storage.Client, vURL, imgURL string) persist.Media {
	res := persist.Media{
		MediaType: persist.MediaTypeImage,
	}
	imageURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("image-%s", name), storageClient)
	if err == nil {
		logger.For(pCtx).Infof("found imageURL for %s: %s", name, imageURL)
		res.MediaURL = persist.NullString(imageURL)
	} else {
		if vURL != "" {
			logger.For(pCtx).Infof("using vURL for %s: %s", name, vURL)
			res.MediaURL = persist.NullString(vURL)
			if imgURL != "" {
				res.ThumbnailURL = persist.NullString(imgURL)
			}
		} else if imgURL != "" {
			logger.For(pCtx).Infof("using imgURL for %s: %s", name, imgURL)
			res.MediaURL = persist.NullString(imgURL)
		}
	}

	res = remapMedia(res)

	res.Dimensions, err = getMediaDimensions(pCtx, res.MediaURL.String())
	if err != nil {
		logger.For(pCtx).Errorf("failed to get dimensions for %s: %v", name, err)
	}

	return res
}

func getHTMLMedia(pCtx context.Context, name, tokenBucket string, storageClient *storage.Client, vURL, imgURL string) persist.Media {
	res := persist.Media{
		MediaType: persist.MediaTypeHTML,
	}

	if vURL != "" {
		logger.For(pCtx).Infof("using vURL for %s: %s", name, vURL)
		res.MediaURL = persist.NullString(vURL)
	} else if imgURL != "" {
		logger.For(pCtx).Infof("using imgURL for %s: %s", name, imgURL)
		res.MediaURL = persist.NullString(imgURL)
	}
	res.ThumbnailURL = persist.NullString(getThumbnailURL(pCtx, tokenBucket, name, imgURL, storageClient))

	res = remapMedia(res)

	dimensions, err := getHTMLDimensions(pCtx, res.MediaURL.String())
	if err != nil {
		logger.For(pCtx).Errorf("failed to get dimensions for %s: %v", name, err)
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

func getRawMedia(pCtx context.Context, mediaType persist.MediaType, name, vURL, imgURL string) persist.Media {
	var res persist.Media
	res.MediaType = mediaType
	if vURL != "" {
		logger.For(pCtx).Infof("using vURL for %s: %s", name, vURL)
		res.MediaURL = persist.NullString(vURL)
		if imgURL != "" {
			res.ThumbnailURL = persist.NullString(imgURL)
		}
	} else if imgURL != "" {
		logger.For(pCtx).Infof("using imgURL for %s: %s", name, imgURL)
		res.MediaURL = persist.NullString(imgURL)
	}

	res = remapMedia(res)

	dimensions, err := getMediaDimensions(pCtx, res.MediaURL.String())
	if err != nil {
		logger.For(pCtx).Errorf("failed to get dimensions for %s: %v", name, err)
	}
	res.Dimensions = dimensions
	return res
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

func persistToStorage(ctx context.Context, client *storage.Client, reader io.Reader, bucket, fileName string, contentType *string, contentLength *int64) error {
	writer := newObjectWriter(ctx, client, bucket, fileName, contentType, contentLength)
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
	}, shouldRetryUpload, retry.DefaultRetry)
}

func cacheRawMedia(ctx context.Context, reader io.Reader, bucket, fileName string, contentType *string, contentLength *int64, client *storage.Client) error {
	err := persistToStorage(ctx, client, reader, bucket, fileName, contentType, contentLength)
	go purgeIfExists(context.Background(), bucket, fileName, client)
	return err
}

func cacheRawSvgMedia(ctx context.Context, reader io.Reader, bucket, name string, contentLength *int64, client *storage.Client) error {
	return cacheRawMedia(ctx, reader, bucket, fmt.Sprintf("svg-%s", name), util.ToPointer("image/svg+xml"), contentLength, client)
}

func cacheRawVideoMedia(ctx context.Context, reader io.Reader, bucket, name string, contentType *string, contentLength *int64, client *storage.Client) error {
	return cacheRawMedia(ctx, reader, bucket, fmt.Sprintf("video-%s", name), contentType, contentLength, client)
}

func cacheRawImageMedia(ctx context.Context, reader io.Reader, bucket, name string, contentType *string, contentLength *int64, client *storage.Client) error {
	return cacheRawMedia(ctx, reader, bucket, fmt.Sprintf("image-%s", name), contentType, contentLength, client)
}

func cacheRawAnimationMedia(ctx context.Context, reader io.Reader, bucket, fileName string, client *storage.Client) error {
	sw := newObjectWriter(ctx, client, bucket, fileName, nil, nil)
	writer := gzip.NewWriter(sw)

	err := retryWriteToCloudStorage(ctx, writer, reader)
	if err != nil {
		return fmt.Errorf("could not write to bucket %s for %s: %s", bucket, fileName, err)
	}

	if err := writer.Close(); err != nil {
		return err
	}

	if err := sw.Close(); err != nil {
		return err
	}

	go purgeIfExists(context.Background(), bucket, fileName, client)
	return nil
}

func thumbnailAndCache(ctx context.Context, videoURL, bucket, name string, client *storage.Client) error {

	fileName := fmt.Sprintf("thumbnail-%s", name)
	logger.For(ctx).Infof("caching thumbnail for '%s'", fileName)

	timeBeforeCopy := time.Now()

	sw := newObjectWriter(ctx, client, bucket, fileName, util.ToPointer("image/jpeg"), nil)

	logger.For(ctx).Infof("thumbnailing %s", videoURL)
	if err := thumbnailVideoToWriter(ctx, videoURL, sw); err != nil {
		return fmt.Errorf("could not thumbnail to bucket %s for '%s': %s", bucket, fileName, err)
	}

	if err := sw.Close(); err != nil {
		return err
	}

	logger.For(ctx).Infof("storage copy took %s", time.Since(timeBeforeCopy))

	go purgeIfExists(context.Background(), bucket, fileName, client)

	return nil
}

func createLiveRenderAndCache(ctx context.Context, videoURL, bucket, name string, client *storage.Client) error {

	fileName := fmt.Sprintf("liverender-%s", name)
	logger.For(ctx).Infof("caching live render media for '%s'", fileName)

	timeBeforeCopy := time.Now()

	sw := newObjectWriter(ctx, client, bucket, fileName, util.ToPointer("video/mp4"), nil)

	logger.For(ctx).Infof("creating live render for %s", videoURL)
	if err := createLiveRenderPreviewVideo(ctx, videoURL, sw); err != nil {
		return fmt.Errorf("could not live render to bucket %s for '%s': %s", bucket, fileName, err)
	}

	if err := sw.Close(); err != nil {
		return err
	}

	logger.For(ctx).Infof("storage copy took %s", time.Since(timeBeforeCopy))

	go purgeIfExists(context.Background(), bucket, fileName, client)

	return nil
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

func downloadAndCache(pCtx context.Context, tids persist.TokenIdentifiers, mediaURL, name, ipfsPrefix string, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, bucket string, isRecursive bool) (persist.MediaType, bool, error) {
	asURI := persist.TokenURI(mediaURL)
	timeBeforePredict := time.Now()
	mediaType, contentType, contentLength, _ := PredictMediaType(pCtx, asURI.String())
	pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{
		"mediaType":   mediaType,
		"contentType": contentType,
		"mediaURL":    mediaURL,
	})
	contentLengthStr := "nil"
	if contentLength != nil {
		contentLengthStr = util.InByteSizeFormat(uint64(util.FromPointer(contentLength)))
	}
	pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{
		"contentLength": contentLength,
	})
	logger.For(pCtx).Infof("predicted media type from '%s' as '%s' with length %s in %s", mediaURL, mediaType, contentLengthStr, time.Since(timeBeforePredict))

outer:
	switch mediaType {
	case persist.MediaTypeVideo, persist.MediaTypeUnknown, persist.MediaTypeSVG, persist.MediaTypeBase64BMP:
		break outer
	default:
		switch asURI.Type() {
		case persist.URITypeIPFS, persist.URITypeArweave:
			logger.For(pCtx).Infof("uri for '%s' is of type '%s', trying to cache", name, asURI.Type())
			break outer
		default:
			logger.For(pCtx).Infof("skipping caching of mediaType '%s' and uriType '%s'", mediaType, asURI.Type())
			return mediaType, false, nil
		}
	}

	timeBeforeDataReader := time.Now()
	reader, err := rpc.GetDataFromURIAsReader(pCtx, asURI, ipfsClient, arweaveClient)
	if err != nil {

		if !isRecursive && tids.Chain == persist.ChainETH {
			logger.For(pCtx).Infof("failed to get data from uri '%s' for '%s', trying opensea", mediaURL, name)
			// if token is ETH, backup to asking opensea
			assets, err := opensea.FetchAssetsForTokenIdentifiers(pCtx, persist.EthereumAddress(tids.ContractAddress), opensea.TokenID(tids.TokenID.Base10String()))
			if err != nil || len(assets) == 0 {
				// no data from opensea, return error
				return mediaType, false, errNoDataFromReader{err: err, url: mediaURL}
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

				logger.For(pCtx).Infof("got reader for %s from opensea in %s (%s)", name, time.Since(timeBeforeDataReader), firstNonEmptyURL)
				return downloadAndCache(pCtx, tids, firstNonEmptyURL, name, ipfsPrefix, ipfsClient, arweaveClient, storageClient, bucket, true)
			}
		}
		return mediaType, false, errNoDataFromReader{err: err, url: mediaURL}

	}
	logger.For(pCtx).Infof("got reader for %s in %s", name, time.Since(timeBeforeDataReader))
	defer reader.Close()

	if !mediaType.IsValid() {
		timeBeforeSniff := time.Now()
		bytesToSniff, _ := reader.Headers()
		mediaType, *contentType = persist.SniffMediaType(bytesToSniff)
		logger.For(pCtx).Infof("sniffed media type for %s: %s in %s", truncateString(mediaURL, 50), mediaType, time.Since(timeBeforeSniff))
	}

	switch mediaType {
	case persist.MediaTypeVideo:
		timeBeforeCache := time.Now()

		videoURL := fmt.Sprintf("https://storage.googleapis.com/%s/video-%s", bucket, name)
		err := cacheRawVideoMedia(pCtx, reader, bucket, name, contentType, contentLength, storageClient)
		if err != nil {
			return mediaType, false, err
		}

		if err := thumbnailAndCache(pCtx, videoURL, bucket, name, storageClient); err != nil {
			logger.For(pCtx).Errorf("could not create thumbnail for %s: %s", name, err)
		}

		if err := createLiveRenderAndCache(pCtx, videoURL, bucket, name, storageClient); err != nil {
			logger.For(pCtx).Errorf("could not create live render for %s: %s", name, err)
		}

		logger.For(pCtx).Infof("cached video for %s in %s", name, time.Since(timeBeforeCache))
		return persist.MediaTypeVideo, true, nil
	case persist.MediaTypeSVG:
		timeBeforeCache := time.Now()
		err = cacheRawSvgMedia(pCtx, reader, bucket, name, contentLength, storageClient)
		if err != nil {
			return mediaType, false, err
		}
		logger.For(pCtx).Infof("cached svg for %s in %s", name, time.Since(timeBeforeCache))
		return persist.MediaTypeSVG, true, nil
	case persist.MediaTypeBase64BMP:
		timeBeforeCache := time.Now()
		err = cacheRawImageMedia(pCtx, reader, bucket, name, contentType, contentLength, storageClient)
		if err != nil {
			return mediaType, false, err
		}
		logger.For(pCtx).Infof("cached image for %s in %s", name, time.Since(timeBeforeCache))
		return persist.MediaTypeImage, true, nil
	}

	switch asURI.Type() {
	case persist.URITypeIPFS, persist.URITypeArweave:
		if mediaType == persist.MediaTypeHTML && asURI.IsPathPrefixed() {
			return mediaType, false, nil
		}

		asMb := 0.0
		if contentLength != nil && *contentLength > 0 {
			asMb = float64(*contentLength) / 1024 / 1024
		}
		logger.For(pCtx).Infof("caching %.2f mb of raw media with type '%s' for '%s' at '%s-%s'", asMb, mediaType, mediaURL, ipfsPrefix, name)

		if mediaType == persist.MediaTypeAnimation {
			timeBeforeCache := time.Now()
			err = cacheRawAnimationMedia(pCtx, reader, bucket, fmt.Sprintf("%s-%s", ipfsPrefix, name), storageClient)
			if err != nil {
				return mediaType, false, err
			}
			logger.For(pCtx).Infof("cached animation for %s in %s", name, time.Since(timeBeforeCache))
			return mediaType, true, nil
		}
		timeBeforeCache := time.Now()
		err = cacheRawMedia(pCtx, reader, bucket, fmt.Sprintf("%s-%s", ipfsPrefix, name), contentType, contentLength, storageClient)
		if err != nil {
			return mediaType, false, err
		}
		logger.For(pCtx).Infof("cached raw media for %s in %s", name, time.Since(timeBeforeCache))
		return mediaType, true, nil
	}

	return mediaType, false, nil
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

func newObjectWriter(ctx context.Context, client *storage.Client, bucket, fileName string, contentType *string, contentLength *int64) *storage.Writer {
	writer := client.Bucket(bucket).Object(fileName).NewWriter(ctx)
	if contentType != nil {
		writer.ObjectAttrs.ContentType = *contentType
	}
	writer.ObjectAttrs.CacheControl = "no-cache, no-store"
	writer.ChunkSize = 4 * 1024 * 1024 // 4MB
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

func shouldRetryUpload(err error) bool {
	if gerr, ok := err.(*googleapi.Error); ok {
		// Retriable error codes: https://cloud.google.com/storage/docs/json_api/v1/status-codes
		return gerr.Code == 500 || gerr.Code == 502 || gerr.Code == 503 || gerr.Code == 504
	}

	return err == io.ErrUnexpectedEOF
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
