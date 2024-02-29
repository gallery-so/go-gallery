package media

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/rpc/arweave"
	"github.com/mikeydub/go-gallery/service/rpc/ipfs"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

type AnimationURL string
type ImageURL string

var ErrNoMediaURLs = errors.New("no media URLs found in metadata")

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

var gltfFields = []string{"scene", "scenes", "nodes", "meshes", "accessors", "bufferViews", "buffers", "materials", "textures", "images", "samplers", "cameras", "skins", "animations", "extensions", "extras"}

type mediaWithContentType struct {
	mediaType   persist.MediaType
	contentType string
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

type contentTypeLengthTuple struct {
	contentType string
	length      int64
}

type mediaPrediction struct {
	mediaType   persist.MediaType
	contentType string
	length      *int64
}

// PredictMediaType guesses the media type of the given URL.
func PredictMediaType(ctx context.Context, url string) (persist.MediaType, string, *int64, error) {
	f := func() (persist.MediaType, string, *int64, error) {
		spl := strings.Split(url, ".")
		if len(spl) > 1 {
			ext := spl[len(spl)-1]
			ext = strings.Split(ext, "?")[0]
			if t, ok := postfixesToMediaTypes[ext]; ok {
				return t.mediaType, t.contentType, nil, nil
			}
		}
		asURI := persist.TokenURI(url)
		lenURI := int64(len(asURI.String()))
		uriType := asURI.Type()
		logger.For(ctx).Debugf("predicting media type for %s with URI type %s", url, uriType)
		mediaType := uriType.ToMediaType()
		if mediaType.IsValid() {
			return mediaType, mediaType.ToContentType(), &lenURI, nil
		}
		switch uriType {
		case persist.URITypeIPFS:
			header, err := ipfs.GetHeader(ctx, strings.TrimPrefix(asURI.String(), "ipfs://"))
			if err != nil {
				return persist.MediaTypeUnknown, "", nil, err
			}
			contentType := parseContentType(header)
			contentLength, err := parseContentLength(header)
			return MediaFromContentType(contentType), contentType, &contentLength, err
		case persist.URITypeIPFSGateway:

			ctl, err := util.FirstNonErrorWithValue(ctx, true, retry.HTTPErrNotFound, func(ctx context.Context) (contentTypeLengthTuple, error) {
				header, err := ipfs.GetHeader(ctx, util.GetURIPath(asURI.String(), false))
				if err != nil {
					return contentTypeLengthTuple{}, err
				}
				contentType := parseContentType(header)
				contentLength, err := parseContentLength(header)
				return contentTypeLengthTuple{contentType: contentType, length: contentLength}, err
			}, func(ctx context.Context) (contentTypeLengthTuple, error) {
				header, err := rpc.GetHTTPHeaders(ctx, url)
				if err != nil {
					return contentTypeLengthTuple{}, err
				}
				contentType := parseContentType(header)
				contentLength, err := parseContentLength(header)
				return contentTypeLengthTuple{contentType: contentType, length: contentLength}, err
			})

			if err != nil {
				return persist.MediaTypeUnknown, "", nil, err
			}

			return MediaFromContentType(ctl.contentType), ctl.contentType, &ctl.length, nil
		case persist.URITypeArweave:
			url = arweave.BestGatewayNodeFrom(asURI.String())
			fallthrough
		case persist.URITypeHTTP, persist.URITypeIPFSAPI, persist.URITypeArweaveGateway:
			header, err := rpc.GetHTTPHeaders(ctx, url)
			if err != nil {
				return persist.MediaTypeUnknown, "", nil, err
			}
			contentType := parseContentType(header)
			contentLength, err := parseContentLength(header)
			return MediaFromContentType(contentType), contentType, &contentLength, err
		}
		return persist.MediaTypeUnknown, "", nil, nil
	}

	errChan := make(chan error)
	resultChan := make(chan mediaPrediction)
	go func() {
		defer close(errChan)
		defer close(resultChan)
		mediaType, contentType, length, err := f()
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- mediaPrediction{mediaType: mediaType, contentType: contentType, length: length}
	}()

	select {
	case <-ctx.Done():
		return persist.MediaTypeUnknown, "", nil, ctx.Err()
	case err := <-errChan:
		return persist.MediaTypeUnknown, "", nil, err
	case result := <-resultChan:
		return result.mediaType, result.contentType, result.length, nil
	}
}

type svgXML struct {
	XMLName xml.Name `xml:"svg"`
}

// SniffMediaType will attempt to detect the media type for a given array of bytes
func SniffMediaType(buf []byte) (persist.MediaType, string) {

	var asXML svgXML
	if err := xml.Unmarshal(buf, &asXML); err == nil {
		return persist.MediaTypeSVG, "image/svg+xml"
	}

	contentType := http.DetectContentType(buf)
	contentType = strings.TrimSpace(contentType)
	whereCharset := strings.IndexByte(contentType, ';')
	if whereCharset != -1 {
		contentType = contentType[:whereCharset]
	}
	if contentType == "application/octet-stream" || contentType == "text/plain" {
		// fallback of http.DetectContentType
		if strings.EqualFold(string(buf[:4]), "glTF") {
			return persist.MediaTypeAnimation, "model/gltf+binary"
		}

		if strings.HasPrefix(strings.TrimSpace(string(buf[:20])), "{") && util.ContainsAnyString(strings.TrimSpace(string(buf)), gltfFields...) {
			return persist.MediaTypeAnimation, "model/gltf+json"
		}
	}
	return MediaFromContentType(contentType), contentType
}

// MediaFromContentType will attempt to convert a content type to a media type
func MediaFromContentType(contentType string) persist.MediaType {
	contentType = strings.TrimSpace(contentType)
	whereCharset := strings.IndexByte(contentType, ';')
	if whereCharset != -1 {
		contentType = contentType[:whereCharset]
	}

	splt := strings.Split(contentType, "/")

	typ, subType := splt[0], ""

	if len(splt) == 2 {
		subType = splt[1]
	}

	switch typ {
	case "image":
		switch subType {
		case "svg", "svg+xml":
			return persist.MediaTypeSVG
		case "gif":
			return persist.MediaTypeGIF
		default:
			return persist.MediaTypeImage
		}
	case "video":
		return persist.MediaTypeVideo
	case "audio":
		return persist.MediaTypeAudio
	case "text":
		switch subType {
		case "html":
			return persist.MediaTypeHTML
		default:
			return persist.MediaTypeText
		}
	case "application":
		switch subType {
		case "pdf":
			return persist.MediaTypePDF
		}
		fallthrough
	default:
		return persist.MediaTypeUnknown
	}
}

// PredictMediaURLs finds the image and animation URLs from a token's metadata and accesses them to predict the true URLs
func PredictMediaURLs(ctx context.Context, metadata persist.TokenMetadata, imgKeywords []string, animKeywords []string) (imgURL ImageURL, animURL AnimationURL, err error) {
	imgURL, animURL, err = FindMediaURLs(metadata, imgKeywords, animKeywords)
	if err != nil {
		return
	}
	imgURL, animURL = predictTrueURLs(ctx, imgURL, animURL)
	return imgURL, animURL, nil
}

// FindMediaURLsChain finds the image and animation URLs from a token's metadata, using the chain's base keywords
func FindMediaURLsChain(metadata persist.TokenMetadata, chain persist.Chain) (imgURL ImageURL, animURL AnimationURL, err error) {
	imgK, animK := chain.BaseKeywords()
	return FindMediaURLs(metadata, imgK, animK)
}

// FindMediaURLs finds the image and animation URLs from a token's metadata
func FindMediaURLs(metadata persist.TokenMetadata, imgKeywords []string, animKeywords []string) (imgURL ImageURL, animURL AnimationURL, err error) {
	_, imgURL, _, animURL, err = FindMediaURLsKeys(metadata, imgKeywords, animKeywords)
	return imgURL, animURL, err
}

// FindMediaURLsKeysChain finds the image and animation URLs and corresponding keywords from a token's metadata, using the chain's base keywords
func FindMediaURLsKeysChain(metadata persist.TokenMetadata, chain persist.Chain) (imgKey string, imgURL ImageURL, animKey string, animURL AnimationURL, err error) {
	imgK, animK := chain.BaseKeywords()
	return FindMediaURLsKeys(metadata, imgK, animK)
}

// FindMediaURLsKeys finds the image and animation URLs and corresponding keywords from a token's metadata
func FindMediaURLsKeys(metadata persist.TokenMetadata, imgKeywords []string, animKeywords []string) (imgKey string, imgURL ImageURL, animKey string, animURL AnimationURL, err error) {
	if metaMedia, ok := metadata["media"].(map[string]any); ok {
		var mediaType persist.MediaType

		if mime, ok := metaMedia["mimeType"].(string); ok {
			mediaType = MediaFromContentType(mime)
		}
		if uri, ok := metaMedia["uri"].(string); ok {
			switch mediaType {
			case persist.MediaTypeImage, persist.MediaTypeSVG, persist.MediaTypeGIF:
				imgURL = ImageURL(uri)
			default:
				animURL = AnimationURL(uri)
			}
		}
	}

	for _, keyword := range imgKeywords {
		if it, ok := util.GetValueFromMapUnsafe(metadata, keyword, util.DefaultSearchDepth).(string); ok && it != "" {
			imgURL = ImageURL(it)
			imgKey = keyword
			break
		}
	}

	for _, keyword := range animKeywords {
		if it, ok := util.GetValueFromMapUnsafe(metadata, keyword, util.DefaultSearchDepth).(string); ok && string(it) != "" && AnimationURL(it) != animURL {
			animURL = AnimationURL(it)
			animKey = keyword
			break
		}
	}

	if imgURL == "" && animURL == "" {
		return "", "", "", "", ErrNoMediaURLs
	}

	return imgKey, imgURL, animKey, animURL, nil
}

func predictTrueURLs(ctx context.Context, curImg ImageURL, curV AnimationURL) (ImageURL, AnimationURL) {
	imgMediaType, _, _, err := PredictMediaType(ctx, string(curImg))
	if err != nil {
		return curImg, curV
	}
	vMediaType, _, _, err := PredictMediaType(ctx, string(curV))
	if err != nil {
		return curImg, curV
	}

	if imgMediaType.IsAnimationLike() && !vMediaType.IsAnimationLike() {
		return ImageURL(curV), AnimationURL(curImg)
	}

	if !imgMediaType.IsValid() || !vMediaType.IsValid() {
		return curImg, curV
	}

	if imgMediaType.IsMorePriorityThan(vMediaType) {
		return ImageURL(curV), AnimationURL(curImg)
	}

	return curImg, curV
}

func parseContentLength(h http.Header) (int64, error) {
	contentLength := h.Get("Content-Length")
	if contentLength != "" {
		contentLengthInt, err := strconv.Atoi(contentLength)
		if err != nil {
			return 0, err
		}
		return int64(contentLengthInt), nil
	}
	return 0, nil
}

func parseContentType(h http.Header) string {
	contentType := h.Get("Content-Type")
	contentType = strings.TrimSpace(contentType)
	whereCharset := strings.IndexByte(contentType, ';')
	if whereCharset != -1 {
		contentType = contentType[:whereCharset]
	}
	return contentType
}
