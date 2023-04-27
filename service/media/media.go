package media

import (
	"context"
	"encoding/xml"
	"net/http"
	"strings"

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
		return MediaFromContentType(contentType), &contentType, &contentLength, nil
	case persist.URITypeIPFSGateway:
		contentType, contentLength, err := rpc.GetIPFSHeaders(pCtx, util.GetURIPath(asURI.String(), false))
		if err == nil {
			return MediaFromContentType(contentType), &contentType, &contentLength, nil
		} else if err != nil {
			logger.For(pCtx).Errorf("could not get IPFS headers for %s: %s", url, err)
		}
		fallthrough
	case persist.URITypeHTTP, persist.URITypeIPFSAPI:
		contentType, contentLength, err := rpc.GetHTTPHeaders(pCtx, url)
		if err != nil {
			return persist.MediaTypeUnknown, nil, nil, err
		}
		return MediaFromContentType(contentType), &contentType, &contentLength, nil
	}
	return persist.MediaTypeUnknown, nil, nil, nil
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
