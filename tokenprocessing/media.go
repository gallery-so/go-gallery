package tokenprocessing

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/mediamapper"
	"github.com/sirupsen/logrus"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
)

func init() {
	env.RegisterValidation("IPFS_URL", "required")
}

type errNoDataFromReader struct {
	err error
	url string
}

type errNoDataFromOpensea struct {
	err error
}

type errNotCacheable struct {
	URL       string
	MediaType persist.MediaType
}

type errInvalidMedia struct {
	err error
	URL string
}

type errStoreObjectFailed struct {
	bucket string
	object cachedMediaObject
	err    error
}

func (e errNoDataFromReader) Error() string {
	return fmt.Sprintf("no data from reader: %s (url: %s)", e.err, e.url)
}

func (e errNoDataFromReader) Unwrap() error {
	return e.err
}

func (e errNotCacheable) Error() string {
	return fmt.Sprintf("unsupported media for caching: %s (mediaURL: %s)", e.MediaType, e.URL)
}

func (e errInvalidMedia) Error() string {
	return fmt.Sprintf("invalid media: %s (url: %s)", e.err, e.URL)
}

func (e errInvalidMedia) Unwrap() error {
	return e.err
}

func (e errNoDataFromOpensea) Error() string {
	msg := "no data from opensea"
	if e.err != nil {
		msg += ": " + e.err.Error()
	}
	return msg
}

func (e errNoDataFromOpensea) Unwrap() error {
	return e.err
}

func (e errStoreObjectFailed) Error() string {
	return fmt.Sprintf("failed to write object to key: %s/%s: %s", e.bucket, e.object.fileName(), e.err)
}

func (e errStoreObjectFailed) Unwrap() error {
	return e.err
}

type cachePipelineMetadata struct {
	ContentHeaderValueRetrieval  *persist.PipelineStepStatus
	ReaderRetrieval              *persist.PipelineStepStatus
	OpenseaFallback              *persist.PipelineStepStatus
	DetermineMediaTypeWithReader *persist.PipelineStepStatus
	AnimationGzip                *persist.PipelineStepStatus
	SVGRasterize                 *persist.PipelineStepStatus
	StoreGCP                     *persist.PipelineStepStatus
	ThumbnailGCP                 *persist.PipelineStepStatus
	LiveRenderGCP                *persist.PipelineStepStatus
}

func createRawMedia(pCtx context.Context, tids persist.TokenIdentifiers, mediaType persist.MediaType, tokenBucket, animURL, imgURL string, objects []cachedMediaObject) persist.Media {
	switch mediaType {
	case persist.MediaTypeHTML:
		return getHTMLMedia(pCtx, tids, tokenBucket, animURL, imgURL, objects)
	default:
		panic(fmt.Sprintf("media type %s should be cached", mediaType))
	}
}

func createUncachedMedia(ctx context.Context, job *tokenProcessingJob, url string, mediaType persist.MediaType, objects []cachedMediaObject) persist.Media {
	first, ok := findFirstImageObject(objects)
	if ok {
		return job.createRawMedia(ctx, mediaType, url, first.storageURL(job.tp.tokenBucket), objects)
	}
	return persist.Media{MediaType: mediaType, MediaURL: persist.NullString(url)}
}

func mustCreateMediaFromErr(ctx context.Context, err error, job *tokenProcessingJob) persist.Media {
	if bErr, ok := err.(ErrBadToken); ok {
		return mustCreateMediaFromErr(ctx, bErr.Unwrap(), job)
	}
	if util.ErrorIs[errInvalidMedia](err) {
		invalidErr := err.(errInvalidMedia)
		return persist.Media{MediaType: persist.MediaTypeInvalid, MediaURL: persist.NullString(invalidErr.URL)}
	}
	if err != nil {
		return persist.Media{MediaType: persist.MediaTypeUnknown}
	}
	// We somehow didn't cache media without getting an error anywhere
	closing, _ := persist.TrackStepStatus(ctx, &job.pipelineMetadata.NothingCachedWithoutErrors, "NothingCachedWithoutErrors")
	// Fail NothingCachedWithErrors because we didn't get any errors
	persist.FailStep(&job.pipelineMetadata.NothingCachedWithErrors)
	closing()
	panic("failed to cache media, and no error occurred in the process")
}

func createMediaFromResults(ctx context.Context, job *tokenProcessingJob, animResult, imgResult, pfpResult cacheResult) persist.Media {
	objects := append(animResult.cachedObjects, imgResult.cachedObjects...)
	objects = append(objects, pfpResult.cachedObjects...)

	if notCacheableErr, ok := animResult.err.(errNotCacheable); ok {
		return createUncachedMedia(ctx, job, notCacheableErr.URL, notCacheableErr.MediaType, objects)
	}

	if notCacheableErr, ok := imgResult.err.(errNotCacheable); ok {
		return createUncachedMedia(ctx, job, notCacheableErr.URL, notCacheableErr.MediaType, objects)
	}

	return job.createMediaFromCachedObjects(ctx, objects)
}

func createMediaFromCachedObjects(ctx context.Context, tokenBucket string, objects map[objectType]cachedMediaObject) persist.Media {
	var primaryObject cachedMediaObject

	if obj, ok := objects[objectTypeAnimation]; ok {
		primaryObject = obj
	} else if obj, ok := objects[objectTypeSVG]; ok {
		primaryObject = obj
	} else if obj, ok := objects[objectTypeImage]; ok {
		primaryObject = obj
	} else {
		logger.For(ctx).Errorf("no primary object found for cached objects: %+v", objects)
	}

	var thumbnailObject *cachedMediaObject
	var liveRenderObject = util.MapFindOrNil(objects, objectTypeLiveRender)
	var profileImageObject = util.MapFindOrNil(objects, objectTypeProfileImage)

	if primaryObject.ObjectType == objectTypeAnimation || primaryObject.ObjectType == objectTypeSVG {
		// animations should have a thumbnail that could be an image or svg or thumbnail
		// thumbnail take top priority, then the other image types that could have been cached

		if obj, ok := objects[objectTypeImage]; ok {
			thumbnailObject = &obj
		} else if obj, ok := objects[objectTypeSVG]; ok && primaryObject.ObjectType != objectTypeSVG {
			thumbnailObject = &obj
		} else if obj, ok := objects[objectTypeThumbnail]; ok {
			thumbnailObject = &obj
		}

	} else {
		// it's not an animation, meaning its image like, so we only use a thumbnail when there explicitly is one
		if obj, ok := objects[objectTypeThumbnail]; ok {
			thumbnailObject = &obj
		}
	}

	result := persist.Media{
		MediaURL:  persist.NullString(primaryObject.storageURL(tokenBucket)),
		MediaType: primaryObject.MediaType,
	}

	if thumbnailObject != nil {
		result.ThumbnailURL = persist.NullString(thumbnailObject.storageURL(tokenBucket))
	}

	if liveRenderObject != nil {
		result.LivePreviewURL = persist.NullString(liveRenderObject.storageURL(tokenBucket))
	}

	if profileImageObject != nil {
		result.ProfileImageURL = persist.NullString(profileImageObject.storageURL(tokenBucket))
	}

	var err error
	switch result.MediaType {
	case persist.MediaTypeSVG:
		result.Dimensions, err = getSvgDimensions(ctx, result.MediaURL.String())
	default:
		result.Dimensions, err = getMediaDimensions(ctx, result.MediaURL.String())
	}

	if err != nil {
		logger.For(ctx).Warnf("failed to get dimensions for media: %s", err)
	}

	return result
}

func findFirstImageObject(objects []cachedMediaObject) (cachedMediaObject, bool) {
	return util.FindFirst(objects, func(c cachedMediaObject) bool {
		if c.ObjectType == 0 || c.MediaType == "" || c.TokenID == "" || c.ContractAddress == "" {
			return false
		}
		return c.ObjectType == objectTypeImage || c.ObjectType == objectTypeSVG || c.ObjectType == objectTypeThumbnail
	})
}

type cacheResult struct {
	cachedObjects []cachedMediaObject
	err           error
}

// IsSuccess returns true if objects were cached and no error occurred or if the error is errNotCacheable
// IsSuccess evaluates to false for the zero value of cacheResult
func (c cacheResult) IsSuccess() bool {
	if util.ErrorIs[errNotCacheable](c.err) {
		return true
	}
	return c.err == nil && len(c.cachedObjects) > 0
}

type svgDimensions struct {
	XMLName xml.Name `xml:"svg"`
	Width   string   `xml:"width,attr"`
	Height  string   `xml:"height,attr"`
	Viewbox string   `xml:"viewBox,attr"`
}

func getSvgDimensions(ctx context.Context, url string) (persist.Dimensions, error) {
	buf := bytes.NewBuffer(nil)
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
			if obj.ObjectType == objectTypeThumbnail {
				res.ThumbnailURL = persist.NullString(obj.storageURL(tokenBucket))
				break
			} else if obj.ObjectType == objectTypeImage || obj.ObjectType == objectTypeSVG {
				res.ThumbnailURL = persist.NullString(obj.storageURL(tokenBucket))
			}
		}

		for _, obj := range cachedObjects {
			if obj.ObjectType == objectTypeLiveRender {
				res.LivePreviewURL = persist.NullString(obj.storageURL(tokenBucket))
				break
			}
		}
	}

	dimensions, err := getHTMLDimensions(pCtx, res.MediaURL.String())
	if err != nil {
		logger.For(pCtx).Warnf("failed to get dimensions for %s: %v", tids, err)
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

func findImageAndAnimationURLs(ctx context.Context, metadata persist.TokenMetadata, imgKeywords, animKeywords []string, pMeta *persist.PipelineMetadata) (media.ImageURL, media.AnimationURL, error) {
	traceCallback, ctx := persist.TrackStepStatus(ctx, &pMeta.MediaURLsRetrieval, "MediaURLsRetrieval")
	defer traceCallback()

	imgURL, vURL, err := media.FindImageAndAnimationURLs(ctx, metadata, imgKeywords, animKeywords)
	if err != nil {
		persist.FailStep(&pMeta.MediaURLsRetrieval)
	}

	return imgURL, vURL, err
}

func findProfileImageURL(metadata persist.TokenMetadata, profileImageKey string) media.ImageURL {
	k := metadata[profileImageKey]
	if k == nil {
		return ""
	}
	urlStr, ok := k.(string)
	if !ok {
		return ""
	}
	return media.ImageURL(urlStr)
}

func findNameAndDescription(metadata persist.TokenMetadata) (string, string) {
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
			logger.For(ctx).Warnf("could not purge file %s: %s", fileName, err)
		}
	}

	return nil
}

func persistToStorage(ctx context.Context, client *storage.Client, reader io.Reader, bucket string, object cachedMediaObject, metadata map[string]string) error {
	writer := newObjectWriter(ctx, client, bucket, object.fileName(), object.ContentLength,
		objAttrsOpts.WithContentType(object.ContentType),
		objAttrsOpts.WithCustomMetadata(metadata),
	)
	if written, err := io.Copy(writer, util.NewLoggingReader(ctx, reader, reader.(io.WriterTo))); err != nil {
		if object.ContentLength != nil {
			logger.For(ctx).Errorf("wrote %d out of %d bytes before error: %s", written, *object.ContentLength, err)
		} else {
			logger.For(ctx).Errorf("wrote %d out of an unknown number of bytes before error: %s", written, err)
		}
		return errStoreObjectFailed{err: err, bucket: bucket, object: object}
	}
	return writer.Close()
}

type objectType int

const (
	objectTypeUnknown objectType = iota
	objectTypeImage
	objectTypeAnimation
	objectTypeThumbnail
	objectTypeLiveRender
	objectTypeSVG
	objectTypeProfileImage
)

func (o objectType) String() string {
	switch o {
	case objectTypeImage:
		return "image"
	case objectTypeAnimation:
		return "animation"
	case objectTypeThumbnail:
		return "thumbnail"
	case objectTypeLiveRender:
		return "liverender"
	case objectTypeSVG:
		return "svg"
	case objectTypeProfileImage:
		return "pfp"
	case objectTypeUnknown:
		return "unknown"
	default:
		panic(fmt.Sprintf("unknown object type: %d", o))
	}
}

// nonOverridableObjectTypes are object types that have a specific purpose and should not be overriden to other types
var nonOverridableObjectTypes = map[objectType]bool{
	objectTypeThumbnail:    true,
	objectTypeLiveRender:   true,
	objectTypeProfileImage: true,
}

// mediaTypeToObjectTypeLookup are default mappings from media type to object type
var mediaTypeToObjectTypeLookup = map[persist.MediaType]objectType{
	persist.MediaTypeVideo:     objectTypeAnimation,
	persist.MediaTypeImage:     objectTypeImage,
	persist.MediaTypeGIF:       objectTypeImage,
	persist.MediaTypeSVG:       objectTypeSVG,
	persist.MediaTypeAnimation: objectTypeAnimation,
}

// mediaTypeToObjectType returns the corresponding objectType from mediaType. If startType is an non-overridable type, startType is returned.
// A token's metadata can be inaccurate e.g. a video asset is stored under "image_url". In such cases, we want to correct the initial objectType
// to what the object type is from downloading the asset and figuring out the media type.
func mediaTypeToObjectType(mediaType persist.MediaType, startType objectType) objectType {
	if _, nonOverride := nonOverridableObjectTypes[startType]; nonOverride {
		return startType
	}
	if oType, ok := mediaTypeToObjectTypeLookup[mediaType]; ok {
		return oType
	}
	return startType
}

type cachedMediaObject struct {
	MediaType       persist.MediaType
	TokenID         persist.TokenID
	ContractAddress persist.Address
	Chain           persist.Chain
	ContentType     string
	ContentLength   *int64
	ObjectType      objectType
}

func (m cachedMediaObject) fileName() string {
	if m.ObjectType.String() == "" || m.TokenID == "" || m.ContractAddress == "" {
		panic(fmt.Sprintf("invalid media object: %+v", m))
	}
	return fmt.Sprintf("%d-%s-%s-%s", m.Chain, m.TokenID, m.ContractAddress, m.ObjectType)
}

func (m cachedMediaObject) storageURL(tokenBucket string) string {
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", tokenBucket, m.fileName())
}

func cacheRawMedia(ctx context.Context, reader *util.FileHeaderReader, tids persist.TokenIdentifiers, mediaType persist.MediaType, contentLength *int64, contentType string, oType objectType, bucket, ogURL string, client *storage.Client, subMeta *cachePipelineMetadata) (cachedMediaObject, error) {
	traceCallback, ctx := persist.TrackStepStatus(ctx, subMeta.StoreGCP, "StoreGCP")
	defer traceCallback()

	object := cachedMediaObject{
		MediaType:       mediaType,
		TokenID:         tids.TokenID,
		ContractAddress: tids.ContractAddress,
		Chain:           tids.Chain,
		ContentType:     contentType,
		ContentLength:   contentLength,
		ObjectType:      mediaTypeToObjectType(mediaType, oType),
	}

	err := persistToStorage(ctx, client, reader, bucket, object, map[string]string{
		"originalURL": truncateString(ogURL, 100),
		"mediaType":   mediaType.String(),
	})
	if err != nil {
		persist.FailStep(subMeta.StoreGCP)
		return cachedMediaObject{}, err
	}

	purgeIfExists(ctx, bucket, object.fileName(), client)
	return object, err
}

func cacheRawAnimationMedia(ctx context.Context, reader *util.FileHeaderReader, tids persist.TokenIdentifiers, mediaType persist.MediaType, oType objectType, bucket, ogURL string, client *storage.Client, subMeta *cachePipelineMetadata) (cachedMediaObject, error) {
	traceCallback, ctx := persist.TrackStepStatus(ctx, subMeta.AnimationGzip, "AnimationGzip")
	defer traceCallback()

	object := cachedMediaObject{
		MediaType:       mediaType,
		TokenID:         tids.TokenID,
		ContractAddress: tids.ContractAddress,
		Chain:           tids.Chain,
		ObjectType:      mediaTypeToObjectType(mediaType, oType),
	}

	sw := newObjectWriter(ctx, client, bucket, object.fileName(), nil,
		objAttrsOpts.WithContentEncoding("gzip"),
		objAttrsOpts.WithCustomMetadata(map[string]string{"originalURL": truncateString(ogURL, 100), "mediaType": mediaType.String()}),
	)
	writer := gzip.NewWriter(sw)

	written, err := io.Copy(writer, util.NewLoggingReader(ctx, reader, reader))
	if err != nil {
		if object.ContentLength != nil {
			logger.For(ctx).Errorf("wrote %d out of %d bytes before error: %s", written, *object.ContentLength, err)
		} else {
			logger.For(ctx).Errorf("wrote %d out of an unknown number of bytes before error: %s", written, err)
		}
		persist.FailStep(subMeta.AnimationGzip)
		return cachedMediaObject{}, errStoreObjectFailed{err: err, bucket: bucket, object: object}
	}

	if err := writer.Close(); err != nil {
		persist.FailStep(subMeta.AnimationGzip)
		return cachedMediaObject{}, err
	}

	if err := sw.Close(); err != nil {
		persist.FailStep(subMeta.AnimationGzip)
		return cachedMediaObject{}, err
	}

	purgeIfExists(ctx, bucket, object.fileName(), client)
	return object, nil
}

type rasterizeResponse struct {
	PNG string  `json:"png"`
	GIF *string `json:"gif"`
}

func rasterizeAndCacheSVGMedia(ctx context.Context, svgURL string, tids persist.TokenIdentifiers, bucket, ogURL string, httpClient *http.Client, client *storage.Client, subMeta *cachePipelineMetadata) ([]cachedMediaObject, error) {
	traceCallback, ctx := persist.TrackStepStatus(ctx, subMeta.SVGRasterize, "SVGRasterize")
	defer traceCallback()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/rasterize?url=%s", env.GetString("RASTERIZER_URL"), svgURL), nil)
	if err != nil {
		persist.FailStep(subMeta.SVGRasterize)
		return nil, err
	}
	idToken, _ := metadata.Get(fmt.Sprintf("instance/service-accounts/default/identity?audience=%s", env.GetString("RASTERIZER_URL")))
	if idToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", idToken))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		persist.FailStep(subMeta.SVGRasterize)
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		persist.FailStep(subMeta.SVGRasterize)
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rasterizer returned non-200 status code: %d (%s)", resp.StatusCode, string(bs))
	}

	var rasterizeResp rasterizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&rasterizeResp); err != nil {
		persist.FailStep(subMeta.SVGRasterize)
		return nil, err
	}

	objects := make([]cachedMediaObject, 0, 2)

	pngData := rasterizeResp.PNG

	data, err := base64.StdEncoding.DecodeString(string(pngData))
	if err != nil {
		persist.FailStep(subMeta.SVGRasterize)
		return nil, fmt.Errorf("could not decode base64 data: %s", err)
	}

	pngObject := cachedMediaObject{
		MediaType:       persist.MediaTypeImage,
		ContentType:     "image/png",
		TokenID:         tids.TokenID,
		ContractAddress: tids.ContractAddress,
		Chain:           tids.Chain,
		ContentLength:   util.ToPointer(int64(len(data))),
		ObjectType:      mediaTypeToObjectType(persist.MediaTypeImage, objectTypeThumbnail),
	}

	sw := newObjectWriter(ctx, client, bucket, pngObject.fileName(), pngObject.ContentLength,
		objAttrsOpts.WithContentType(pngObject.ContentType),
		objAttrsOpts.WithCustomMetadata(map[string]string{"originalURL": truncateString(ogURL, 100), "mediaType": persist.MediaTypeImage.String()}),
	)

	_, err = sw.Write(data)
	if err != nil {
		persist.FailStep(subMeta.SVGRasterize)
		return nil, fmt.Errorf("could not write to bucket %s for %s: %s", bucket, pngObject.fileName(), err)
	}

	if err := sw.Close(); err != nil {
		persist.FailStep(subMeta.SVGRasterize)
		return nil, err
	}

	purgeIfExists(ctx, bucket, pngObject.fileName(), client)

	objects = append(objects, pngObject)

	if rasterizeResp.GIF != nil {
		gifData := *rasterizeResp.GIF

		data, err := base64.StdEncoding.DecodeString(string(gifData))
		if err != nil {
			persist.FailStep(subMeta.SVGRasterize)
			return nil, fmt.Errorf("could not decode base64 data: %s", err)
		}

		gifObject := cachedMediaObject{
			MediaType:       persist.MediaTypeGIF,
			TokenID:         tids.TokenID,
			ContractAddress: tids.ContractAddress,
			Chain:           tids.Chain,
			ContentType:     "image/gif",
			ContentLength:   util.ToPointer(int64(len(data))),
			ObjectType:      mediaTypeToObjectType(persist.MediaTypeGIF, objectTypeLiveRender),
		}

		sw := newObjectWriter(ctx, client, bucket, gifObject.fileName(), gifObject.ContentLength,
			objAttrsOpts.WithContentType(gifObject.ContentType),
			objAttrsOpts.WithCustomMetadata(map[string]string{"originalURL": truncateString(ogURL, 100), "mediaType": persist.MediaTypeGIF.String()}),
		)

		_, err = sw.Write(data)
		if err != nil {
			persist.FailStep(subMeta.SVGRasterize)
			return nil, fmt.Errorf("could not write to bucket %s for %s: %s", bucket, gifObject.fileName(), err)
		}

		if err := sw.Close(); err != nil {
			persist.FailStep(subMeta.AnimationGzip)
			return nil, err
		}

		purgeIfExists(ctx, bucket, gifObject.fileName(), client)

		objects = append(objects, gifObject)

	}

	return objects, nil
}

func thumbnailAndCache(ctx context.Context, tids persist.TokenIdentifiers, videoURL, bucket string, client *storage.Client, subMeta *cachePipelineMetadata) (cachedMediaObject, error) {
	traceCallback, ctx := persist.TrackStepStatus(ctx, subMeta.ThumbnailGCP, "ThumbnailGCP")
	defer traceCallback()

	obj := cachedMediaObject{
		ObjectType:      mediaTypeToObjectType(persist.MediaTypeImage, objectTypeThumbnail),
		MediaType:       persist.MediaTypeImage,
		TokenID:         tids.TokenID,
		ContractAddress: tids.ContractAddress,
		Chain:           tids.Chain,
		ContentType:     "image/png",
	}

	logger.For(ctx).Infof("caching thumbnail for '%s'", obj.fileName())

	timeBeforeCopy := time.Now()

	sw := newObjectWriter(ctx, client, bucket, obj.fileName(), nil,
		objAttrsOpts.WithContentType("image/jpeg"),
		objAttrsOpts.WithCustomMetadata(map[string]string{"thumbnailedURL": videoURL}),
	)

	logger.For(ctx).Infof("thumbnailing %s", videoURL)
	if err := thumbnailVideoToWriter(ctx, videoURL, sw); err != nil {
		persist.FailStep(subMeta.ThumbnailGCP)
		return cachedMediaObject{}, errStoreObjectFailed{err: err, bucket: bucket, object: obj}
	}

	if err := sw.Close(); err != nil {
		persist.FailStep(subMeta.ThumbnailGCP)
		return cachedMediaObject{}, err
	}

	logger.For(ctx).Infof("storage copy took %s", time.Since(timeBeforeCopy))

	purgeIfExists(ctx, bucket, obj.fileName(), client)

	return obj, nil
}

func createLiveRenderAndCache(ctx context.Context, tids persist.TokenIdentifiers, videoURL, bucket string, client *storage.Client, subMeta *cachePipelineMetadata) (cachedMediaObject, error) {

	traceCallback, ctx := persist.TrackStepStatus(ctx, subMeta.LiveRenderGCP, "LiveRenderGCP")
	defer traceCallback()

	obj := cachedMediaObject{
		ObjectType:      mediaTypeToObjectType(persist.MediaTypeVideo, objectTypeLiveRender),
		MediaType:       persist.MediaTypeVideo,
		TokenID:         tids.TokenID,
		ContractAddress: tids.ContractAddress,
		Chain:           tids.Chain,
		ContentType:     "video/mp4",
	}

	logger.For(ctx).Infof("caching live render media for '%s'", obj.fileName())

	timeBeforeCopy := time.Now()

	sw := newObjectWriter(ctx, client, bucket, obj.fileName(), nil,
		objAttrsOpts.WithContentType("video/mp4"),
		objAttrsOpts.WithCustomMetadata(map[string]string{"liveRenderedURL": videoURL}),
	)

	logger.For(ctx).Infof("creating live render for %s", videoURL)
	if err := createLiveRenderPreviewVideo(ctx, videoURL, sw); err != nil {
		persist.FailStep(subMeta.LiveRenderGCP)
		return cachedMediaObject{}, errStoreObjectFailed{err: err, bucket: bucket, object: obj}
	}

	if err := sw.Close(); err != nil {
		persist.FailStep(subMeta.LiveRenderGCP)
		return cachedMediaObject{}, err
	}

	logger.For(ctx).Infof("storage copy took %s", time.Since(timeBeforeCopy))

	purgeIfExists(ctx, bucket, obj.fileName(), client)

	return obj, nil
}

func readerFromURL(ctx context.Context, mediaURL string, mediaType persist.MediaType, ipfsClient *shell.Shell, arweaveClient *goar.Client, subMeta *cachePipelineMetadata) (*util.FileHeaderReader, persist.MediaType, error) {
	traceCallback, ctx := persist.TrackStepStatus(ctx, subMeta.ReaderRetrieval, "ReaderRetrieval")
	defer traceCallback()

	reader, mediaType, err := rpc.GetDataFromURIAsReader(ctx, persist.TokenURI(mediaURL), mediaType, ipfsClient, arweaveClient, util.MB, time.Minute, true)
	if err != nil {
		persist.FailStep(subMeta.ReaderRetrieval)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return reader, mediaType, err
		}
		// The reader is and always will be invalid
		if util.ErrorIs[util.ErrHTTP](err) || util.ErrorIs[*net.DNSError](err) || util.ErrorIs[*url.Error](err) {
			return reader, mediaType, errInvalidMedia{URL: mediaURL, err: err}
		}
		return reader, mediaType, errNoDataFromReader{err: err, url: mediaURL}
	}

	return reader, mediaType, nil
}

func cacheObjectsFromURL(pCtx context.Context, tids persist.TokenIdentifiers, mediaURL string, oType objectType, httpClient *http.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, bucket string, subMeta *cachePipelineMetadata) ([]cachedMediaObject, error) {

	asURI := persist.TokenURI(mediaURL)
	timeBeforePredict := time.Now()
	mediaType, contentType, contentLength := func() (persist.MediaType, string, *int64) {
		traceCallback, pCtx := persist.TrackStepStatus(pCtx, subMeta.ContentHeaderValueRetrieval, "ContentHeaderValueRetrieval")
		defer traceCallback()
		mediaType, contentType, contentLength, err := media.PredictMediaType(pCtx, asURI.String())
		if err != nil {
			persist.FailStep(subMeta.ContentHeaderValueRetrieval)
		}
		pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{
			"predictedMediaType":   mediaType,
			"predictedContentType": contentType,
		})

		contentLengthStr := "nil"
		if contentLength != nil {
			contentLengthStr = util.InByteSizeFormat(uint64(util.FromPointer(contentLength)))
		}
		pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{
			"contentLength": contentLengthStr,
		})
		logger.For(pCtx).Infof("predicted media type from '%s' as '%s' with length %s in %s", mediaURL, mediaType, contentLengthStr, time.Since(timeBeforePredict))
		return mediaType, contentType, contentLength
	}()

	if mediaType == persist.MediaTypeHTML {
		return nil, errNotCacheable{URL: mediaURL, MediaType: mediaType}
	}

	timeBeforeDataReader := time.Now()

	reader, mediaType, err := readerFromURL(pCtx, mediaURL, mediaType, ipfsClient, arweaveClient, subMeta)
	if err != nil {
		return nil, err
	}

	logger.For(pCtx).Infof("got reader for %s in %s", tids, time.Since(timeBeforeDataReader))

	defer reader.Close()

	if !mediaType.IsValid() {
		func() {
			traceCallback, pCtx := persist.TrackStepStatus(pCtx, subMeta.DetermineMediaTypeWithReader, "DetermineMediaTypeWithReader")
			defer traceCallback()

			timeBeforeSniff := time.Now()
			bytesToSniff, err := reader.Headers()
			if err != nil {
				persist.FailStep(subMeta.DetermineMediaTypeWithReader)
				logger.For(pCtx).Errorf("could not get headers for %s: %s", mediaURL, err)
				return
			}
			mediaType, contentType = media.SniffMediaType(bytesToSniff)
			logger.For(pCtx).Infof("sniffed media type for %s: %s in %s", truncateString(mediaURL, 50), mediaType, time.Since(timeBeforeSniff))
		}()
	}

	if mediaType == persist.MediaTypeHTML {
		return nil, errNotCacheable{URL: mediaURL, MediaType: mediaType}
	}

	asMb := 0.0
	if contentLength != nil && *contentLength > 0 {
		asMb = float64(*contentLength) / 1024 / 1024
	}

	pCtx = logger.NewContextWithFields(pCtx, logrus.Fields{
		"finalMediaType":   mediaType,
		"finalContentType": contentType,
		"mb":               asMb,
	})

	logger.For(pCtx).Infof("caching %.2f mb of raw media with type '%s' for '%s' at '%s-%s'", asMb, mediaType, mediaURL, tids, oType)

	if mediaType == persist.MediaTypeAnimation {
		timeBeforeCache := time.Now()
		obj, err := cacheRawAnimationMedia(pCtx, reader, tids, mediaType, oType, bucket, mediaURL, storageClient, subMeta)
		if err != nil {
			logger.For(pCtx).Errorf("could not cache animation: %s", err)
			return nil, err
		}
		logger.For(pCtx).Infof("cached animation for %s in %s", tids, time.Since(timeBeforeCache))
		return []cachedMediaObject{obj}, nil
	}

	timeBeforeCache := time.Now()
	obj, err := cacheRawMedia(pCtx, reader, tids, mediaType, contentLength, contentType, oType, bucket, mediaURL, storageClient, subMeta)
	if err != nil {
		return nil, err
	}
	logger.For(pCtx).Infof("cached media for %s in %s", tids, time.Since(timeBeforeCache))

	result := []cachedMediaObject{obj}
	if mediaType == persist.MediaTypeVideo {
		videoURL := obj.storageURL(bucket)
		thumbObj, err := thumbnailAndCache(pCtx, tids, videoURL, bucket, storageClient, subMeta)
		if err != nil {
			logger.For(pCtx).Errorf("could not create thumbnail for %s: %s", tids, err)
		} else {
			result = append(result, thumbObj)
		}

		liveObj, err := createLiveRenderAndCache(pCtx, tids, videoURL, bucket, storageClient, subMeta)
		if err != nil {
			logger.For(pCtx).Errorf("could not create live render for %s: %s", tids, err)
		} else {
			result = append(result, liveObj)
		}

	} else if mediaType == persist.MediaTypeSVG && tids.ContractAddress != eth.PunkAddress {
		timeBeforeCache := time.Now()
		obj, err := rasterizeAndCacheSVGMedia(pCtx, obj.storageURL(bucket), tids, bucket, mediaURL, httpClient, storageClient, subMeta)
		if err != nil {
			logger.For(pCtx).Errorf("could not cache svg rasterization: %s", err)
			// still return the original object as svg
			return result, nil
		}
		logger.For(pCtx).Infof("cached animation for %s in %s", tids, time.Since(timeBeforeCache))
		return append(result, obj...), nil
	}

	return result, nil
}

func thumbnailVideoToWriter(ctx context.Context, url string, writer io.Writer) error {
	c := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-i", url, "-ss", "00:00:00.000", "-vframes", "1", "-f", "mjpeg", "pipe:1")
	errBuf := new(bytes.Buffer)
	c.Stderr = errBuf
	c.Stdout = writer
	err := c.Run()
	if _, ok := isExitErr(err); ok {
		return errors.New(errBuf.String())
	}
	return err
}

func createLiveRenderPreviewVideo(ctx context.Context, videoURL string, writer io.Writer) error {
	c := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-i", videoURL, "-ss", "00:00:00.000", "-t", "00:00:05.000", "-filter:v", "scale=720:trunc(ow/a/2)*2", "-c:a", "aac", "-shortest", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1")
	errBuf := new(bytes.Buffer)
	c.Stderr = errBuf
	c.Stdout = writer
	err := c.Run()
	if _, ok := isExitErr(err); ok {
		return errors.New(errBuf.String())
	}
	return nil
}

type dimensions struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

func getMediaDimensions(ctx context.Context, url string) (persist.Dimensions, error) {
	c := exec.CommandContext(ctx, "ffprobe", "-hide_banner", "-loglevel", "error", "-show_streams", url, "-print_format", "json")
	outBuf, err := c.Output()
	if err != nil {
		err = errFromExitErr(err)
		logger.For(ctx).Warnf("failed to get dimensions for %s: %s", url, err)
		return getMediaDimensionsBackup(ctx, url)
	}

	var d dimensions
	err = json.Unmarshal(outBuf, &d)
	if err != nil {
		return persist.Dimensions{}, fmt.Errorf("failed to unmarshal ffprobe output: %w", err)
	}

	if len(d.Streams) == 0 {
		return getMediaDimensionsBackup(ctx, url)
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

	if dims.Width == 0 || dims.Height == 0 {
		return getMediaDimensionsBackup(ctx, url)
	}

	logger.For(ctx).Debugf("got dimensions %+v for %s", dims, url)
	return dims, nil
}

func getMediaDimensionsBackup(ctx context.Context, url string) (persist.Dimensions, error) {
	curlCmd := exec.CommandContext(ctx, "curl", "-s", url)
	identifyCmd := exec.CommandContext(ctx, "identify", "-format", "%wx%h", "-")

	curlCmdStdout, err := curlCmd.StdoutPipe()
	if err != nil {
		return persist.Dimensions{}, fmt.Errorf("failed to create pipe for curl stdout: %w", err)
	}

	identifyCmd.Stdin = curlCmdStdout

	err = curlCmd.Start()
	if err != nil {
		return persist.Dimensions{}, fmt.Errorf("failed to start curl command: %w", err)
	}

	outBuf, err := identifyCmd.Output()
	if err != nil {
		return persist.Dimensions{}, errFromExitErr(err)
	}

	err = curlCmd.Wait()
	if err != nil {
		return persist.Dimensions{}, fmt.Errorf("curl command exited with error: %w", err)
	}

	dimensionsStr := string(outBuf)
	dimensionsSplit := strings.Split(dimensionsStr, "x")
	if len(dimensionsSplit) != 2 {
		return persist.Dimensions{}, fmt.Errorf("unexpected output format from identify: %s", dimensionsStr)
	}

	width, err := strconv.Atoi(dimensionsSplit[0])
	if err != nil {
		return persist.Dimensions{}, fmt.Errorf("failed to parse width: %w", err)
	}

	height, err := strconv.Atoi(dimensionsSplit[1])
	if err != nil {
		return persist.Dimensions{}, fmt.Errorf("failed to parse height: %w", err)
	}

	dims := persist.Dimensions{
		Width:  width,
		Height: height,
	}

	return dims, nil
}

func truncateString(s string, i int) string {
	asRunes := []rune(s)
	if len(asRunes) > i {
		return string(asRunes[:i])
	}
	return s
}

func newObjectWriter(ctx context.Context, client *storage.Client, bucket, fileName string, contentLength *int64, opts ...func(*storage.ObjectAttrs)) *storage.Writer {
	writer := client.Bucket(bucket).Object(fileName).NewWriter(ctx)
	writer.ProgressFunc = func(written int64) {
		logger.For(ctx).Infof("wrote %s to %s", util.InByteSizeFormat(uint64(written)), fileName)
	}
	writer.ObjectAttrs.CacheControl = "no-cache, no-store"
	writer.ChunkSize = contentLengthToChunkSize(contentLength)
	writer.ChunkRetryDeadline = 5 * time.Minute
	for _, opt := range opts {
		opt(&writer.ObjectAttrs)
	}
	return writer
}

type objectAttrsOptions struct{}

var objAttrsOpts objectAttrsOptions

// WithContentType sets the Content-Type header of the object
func (objectAttrsOptions) WithContentType(typ string) func(*storage.ObjectAttrs) {
	return func(a *storage.ObjectAttrs) {
		a.ContentType = typ
	}
}

// WithCustomMetadata sets custom metadata on the object
func (objectAttrsOptions) WithCustomMetadata(m map[string]string) func(*storage.ObjectAttrs) {
	return func(a *storage.ObjectAttrs) {
		a.Metadata = m
	}
}

// WithContentEncoding sets the Content-Encoding header of the object
func (objectAttrsOptions) WithContentEncoding(enc string) func(*storage.ObjectAttrs) {
	return func(a *storage.ObjectAttrs) {
		a.ContentEncoding = enc
	}
}

func contentLengthToChunkSize(contentLength *int64) int {
	if contentLength == nil {
		return 4 * 1024 * 1024
	}
	cl := *contentLength
	if cl < 4*1024*1024 {
		return int(cl)
	} else if cl > 8*1024*1024 && cl < 32*1024*1024 {
		return 8 * 1024 * 1024
	} else if cl > 32*1024*1024 {
		return 16 * 1024 * 1024
	}
	return 4 * 1024 * 1024
}

func errFromExitErr(err error) error {
	if exitErr, ok := isExitErr(err); ok {
		return errors.New(string(exitErr.Stderr))
	}
	return err
}

func isExitErr(err error) (*exec.ExitError, bool) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr, true
	}
	return nil, false
}
