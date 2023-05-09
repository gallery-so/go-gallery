package media

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
)

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
	contentType *string
	length      *int64
}

// PredictMediaType guesses the media type of the given URL.
func PredictMediaType(ctx context.Context, url string) (persist.MediaType, *string, *int64, error) {
	// predicting is not critical, so we can afford to give it a timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	f := func() (persist.MediaType, *string, *int64, error) {
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
		logger.For(ctx).Debugf("predicting media type for %s with URI type %s", url, uriType)
		switch uriType {
		case persist.URITypeBase64JSON, persist.URITypeJSON:
			return persist.MediaTypeJSON, util.ToPointer("application/json"), &lenURI, nil
		case persist.URITypeBase64SVG, persist.URITypeSVG:
			return persist.MediaTypeSVG, util.ToPointer("image/svg+xml"), &lenURI, nil
		case persist.URITypeBase64BMP:
			return persist.MediaTypeBase64BMP, util.ToPointer("image/bmp"), &lenURI, nil
		case persist.URITypeBase64PNG:
			return persist.MediaTypeBase64PNG, util.ToPointer("image/png"), &lenURI, nil
		case persist.URITypeBase64HTML:
			return persist.MediaTypeHTML, util.ToPointer("text/html"), &lenURI, nil
		case persist.URITypeIPFS:
			contentType, contentLength, err := rpc.GetIPFSHeaders(ctx, strings.TrimPrefix(asURI.String(), "ipfs://"))
			if err != nil {
				return persist.MediaTypeUnknown, nil, nil, err
			}
			return MediaFromContentType(contentType), &contentType, &contentLength, nil
		case persist.URITypeIPFSGateway:

			ctl, err := util.FirstNonErrorWithValue(ctx, true, rpc.HTTPErrIsForceClose, func(ctx context.Context) (contentTypeLengthTuple, error) {
				contentType, contentLength, err := rpc.GetIPFSHeaders(ctx, util.GetURIPath(asURI.String(), false))
				if err != nil {
					return contentTypeLengthTuple{}, err
				}
				return contentTypeLengthTuple{contentType: contentType, length: contentLength}, nil
			}, func(ctx context.Context) (contentTypeLengthTuple, error) {
				contentType, contentLength, err := rpc.GetHTTPHeaders(ctx, url)
				if err != nil {
					return contentTypeLengthTuple{}, err
				}
				return contentTypeLengthTuple{contentType: contentType, length: contentLength}, nil
			})

			if err != nil {
				return persist.MediaTypeUnknown, nil, nil, err
			}

			return MediaFromContentType(ctl.contentType), &ctl.contentType, &ctl.length, nil
		case persist.URITypeArweave:
			path := util.GetURIPath(asURI.String(), false)
			url = fmt.Sprintf("https://arweave.net/%s", path)
			fallthrough
		case persist.URITypeHTTP, persist.URITypeIPFSAPI, persist.URITypeArweaveGateway:
			contentType, contentLength, err := rpc.GetHTTPHeaders(ctx, url)
			if err != nil {
				return persist.MediaTypeUnknown, nil, nil, err
			}
			return MediaFromContentType(contentType), &contentType, &contentLength, nil
		}
		return persist.MediaTypeUnknown, nil, nil, nil
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
		return persist.MediaTypeUnknown, nil, nil, ctx.Err()
	case err := <-errChan:
		return persist.MediaTypeUnknown, nil, nil, err
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
	spl := strings.Split(contentType, "/")

	switch spl[0] {
	case "image":
		switch spl[1] {
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
		switch spl[1] {
		case "html":
			return persist.MediaTypeHTML
		default:
			return persist.MediaTypeText
		}
	case "application":
		switch spl[1] {
		case "pdf":
			return persist.MediaTypePDF
		}
		fallthrough
	default:
		return persist.MediaTypeUnknown
	}
}
