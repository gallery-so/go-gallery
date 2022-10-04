package media

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/mediamapper"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/qmuntal/gltf"
	"github.com/spf13/viper"
	htransport "google.golang.org/api/transport/http"
)

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

type errGeneratingThumbnail struct {
	err error
	url string
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
}

func NewLocalStorageClient(ctx context.Context, keyPath string) *storage.Client {
	scopes := []string{storage.ScopeFullControl}
	transport, err := htransport.NewTransport(ctx, http.DefaultTransport, option.WithCredentialsFile(keyPath), option.WithScopes(scopes...))
	if err != nil {
		panic(err)
	}
	client, _, err := htransport.NewClient(ctx, option.WithCredentialsFile(keyPath))
	if err != nil {
		panic(err)
	}
	client.Transport = transport
	s, _ := storage.NewClient(ctx, option.WithHTTPClient(client))
	return s
}

func NewStorageClient(ctx context.Context) *storage.Client {
	scopes := []string{storage.ScopeFullControl}
	transport, err := htransport.NewTransport(ctx, tracing.NewTracingTransport(http.DefaultTransport, false, true), option.WithScopes(scopes...))
	if err != nil {
		panic(err)
	}
	client, _, err := htransport.NewClient(ctx)
	if err != nil {
		panic(err)
	}
	client.Transport = transport
	s, _ := storage.NewClient(ctx, option.WithHTTPClient(client))
	return s
}

// MakePreviewsForMetadata uses a metadata map to generate media content and cache resized versions of the media content.
func MakePreviewsForMetadata(pCtx context.Context, metadata persist.TokenMetadata, contractAddress persist.Address, tokenID persist.TokenID, turi persist.TokenURI, chain persist.Chain, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string, imageKeywords, animationKeywords Keywords) (persist.Media, error) {

	name := fmt.Sprintf("%s-%s", contractAddress, tokenID)

	imgURL, vURL := FindImageAndAnimationURLs(pCtx, tokenID, contractAddress, metadata, turi, animationKeywords, imageKeywords, name, true)

	imgAsURI := persist.TokenURI(imgURL)
	videoAsURI := persist.TokenURI(vURL)

	logger.For(pCtx).WithFields(logrus.Fields{"tokenURI": truncateString(turi.String(), 100), "imgURL": truncateString(imgURL, 100), "vURL": truncateString(vURL, 100), "name": name}).Debug("MakePreviewsForMetadata initial")

	var res persist.Media
	var mediaType persist.MediaType
	if imgURL != "" {
		var err error
		mediaType, err = downloadAndCache(pCtx, imgURL, name, "image", ipfsClient, arweaveClient, storageClient)
		if err != nil {
			switch err.(type) {
			case rpc.ErrHTTP:
				if err.(rpc.ErrHTTP).Status == http.StatusNotFound {
					mediaType = persist.MediaTypeInvalid
				} else {
					return persist.Media{}, fmt.Errorf("HTTP error downloading img %s for %s: %s", imgAsURI, name, err)
				}
			case *net.DNSError:
				mediaType = persist.MediaTypeInvalid
				logger.For(pCtx).WithError(err).Warnf("DNS error downloading img %s for %s: %s", imgAsURI, name, err)
			case errGeneratingThumbnail:
				break
			default:
				return persist.Media{}, fmt.Errorf("error downloading img %s of type %s for %s: %s", imgAsURI, imgAsURI.Type(), name, err)
			}
		}
	}
	if vURL != "" {
		logger.For(pCtx).WithFields(logrus.Fields{"tokenURI": truncateString(turi.String(), 25), "imgURL": truncateString(imgURL, 50), "vURL": truncateString(vURL, 50), "name": name}).Debug("MakePreviewsForMetadata vURL valid")
		var err error
		mediaType, err = downloadAndCache(pCtx, vURL, name, "video", ipfsClient, arweaveClient, storageClient)
		if err != nil {
			switch err.(type) {
			case rpc.ErrHTTP:
				if err.(rpc.ErrHTTP).Status == http.StatusNotFound {
					mediaType = persist.MediaTypeInvalid
				} else {
					return persist.Media{}, fmt.Errorf("HTTP error downloading video %s for %s: %s", videoAsURI, name, err)
				}
			case *net.DNSError:
				mediaType = persist.MediaTypeInvalid
				logger.For(pCtx).WithError(err).Warnf("DNS error downloading video %s for %s: %s", videoAsURI, name, err)
			case errGeneratingThumbnail:
				break
			default:
				return persist.Media{}, fmt.Errorf("error downloading video %s of type %s for %s: %s", videoAsURI, videoAsURI.Type(), name, err)
			}
		}
	}

	logger.For(pCtx).WithFields(logrus.Fields{"tokenURI": truncateString(turi.String(), 25), "imgURL": truncateString(imgURL, 25), "vURL": truncateString(vURL, 25), "mediaType": mediaType, "name": name}).Debug("MakePreviewsForMetadata mediaType")

	switch mediaType {
	case persist.MediaTypeImage:
		res = getImageMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL)
	case persist.MediaTypeVideo, persist.MediaTypeAudio, persist.MediaTypeText, persist.MediaTypeAnimation:
		res = getAuxilaryMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL, mediaType)
	case persist.MediaTypeHTML:
		res = getHTMLMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL)
	case persist.MediaTypeGIF:
		res = getGIFMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL)
	case persist.MediaTypeSVG:
		res = getSvgMedia(pCtx, name, tokenBucket, storageClient, vURL, imgURL)
	default:
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
	}

	logger.For(pCtx).Infof("media for %s of type %s: %+v", name, mediaType, res)

	return remapMedia(res), nil
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
	logger.For(pCtx).WithFields(logrus.Fields{"tokenURI": truncateString(tokenBucket, 25), "imgURL": truncateString(imgURL, 50), "vURL": truncateString(vURL, 50), "name": name}).Debug("getAuxilaryMedia")
	if vURL != "" {
		logger.For(pCtx).Infof("using vURL %s: %s", name, vURL)
		res.MediaURL = persist.NullString(vURL)
		res.ThumbnailURL = persist.NullString(imageURL)
	} else if imageURL != "" {
		logger.For(pCtx).Infof("using imageURL for %s: %s", name, imageURL)
		res.MediaURL = persist.NullString(imageURL)
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
	return res
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
	thumb := getThumbnailURL(pCtx, tokenBucket, name, imgURL, storageClient)
	res.ThumbnailURL = persist.NullString(thumb)
	return res
}

func remapPaths(mediaURL string) string {
	switch persist.TokenURI(mediaURL).Type() {
	case persist.URITypeIPFS:
		path := util.GetIPFSPath(mediaURL, false)
		return fmt.Sprintf("%s/ipfs/%s", viper.GetString("IPFS_URL"), path)
	case persist.URITypeArweave:
		// TODO
		return mediaURL
	default:
		return mediaURL
	}

}

func remapMedia(media persist.Media) persist.Media {
	media.MediaURL = persist.NullString(remapPaths(media.MediaURL.String()))
	media.ThumbnailURL = persist.NullString(remapPaths(media.ThumbnailURL.String()))
	return media
}

func FindImageAndAnimationURLs(ctx context.Context, tokenID persist.TokenID, contractAddress persist.Address, metadata persist.TokenMetadata, turi persist.TokenURI, animationKeywords, imageKeywords Keywords, name string, predict bool) (imgURL string, vURL string) {

	if metaMedia, ok := metadata["media"].(map[string]interface{}); ok {
		logger.For(nil).Infof("found media metadata for %s: %s", name, metaMedia)
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
			logger.For(ctx).Infof("found initial animation url for %s with keyword %s: %s", name, keyword, it)
			vURL = it
			break
		}
	}

	for _, keyword := range imageKeywords.ForToken(tokenID, contractAddress) {
		if it, ok := util.GetValueFromMapUnsafe(metadata, keyword, util.DefaultSearchDepth).(string); ok && it != "" && it != vURL {
			logger.For(ctx).Infof("found initial image url for %s with keyword %s: %s", name, keyword, it)
			imgURL = it
			break
		}
	}

	if imgURL == "" && vURL == "" {
		logger.For(ctx).Infof("no image url found for %s - using token URI %s", name, turi)
		imgURL = turi.String()
	}

	logger.For(ctx).Infof("image: %s | video %s", imgURL, vURL)

	if predict {
		return predictTrueURLs(ctx, imgURL, vURL)
	}
	return imgURL, vURL

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

func objectExists(ctx context.Context, o *storage.ObjectHandle) (bool, error) {
	_, err := o.Attrs(ctx)
	if err != nil && err != storage.ErrObjectNotExist {
		return false, fmt.Errorf("could not get object attrs for %s: %s", o.ObjectName(), err)
	}
	return err != storage.ErrObjectNotExist, nil
}

func cacheRawSvgMedia(ctx context.Context, reader io.Reader, bucket, fileName string, client *storage.Client) error {

	o := client.Bucket(bucket).Object(fileName)

	exists, err := objectExists(ctx, o)
	if err != nil {
		return err
	}

	sw := o.NewWriter(ctx)
	_, err = io.Copy(sw, reader)
	if err != nil {
		return fmt.Errorf("could not write to bucket %s for %s: %s", bucket, fileName, err)
	}

	if err := sw.Close(); err != nil {
		return err
	}

	logrus.Infof("adding svg to attrs for %s", fileName)

	// Update the object to set the metadata.
	objectAttrsToUpdate := storage.ObjectAttrsToUpdate{
		ContentType:  "image/svg+xml",
		CacheControl: "no-cache, no-store",
	}
	if _, err := o.Update(ctx, objectAttrsToUpdate); err != nil {
		return err
	}

	if exists {
		err = mediamapper.PurgeImage(ctx, fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, fileName))
		if err != nil {
			logger.For(ctx).Errorf("could not purge image %s: %s", fileName, err)
		}
	}
	return nil
}

func purgeIfExists(ctx context.Context, exists bool, bucket string, fileName string) error {
	if exists {
		err := mediamapper.PurgeImage(ctx, fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, fileName))
		if err != nil {
			logger.For(ctx).Errorf("could not purge image %s: %s", fileName, err)
			return err
		}
	}
	return nil
}

func cacheRawAnimationMedia(ctx context.Context, reader io.Reader, bucket, fileName string, client *storage.Client) error {

	o := client.Bucket(bucket).Object(fileName)

	exists, err := objectExists(ctx, o)
	if err != nil {
		return err
	}

	sw := o.NewWriter(ctx)

	writer := gzip.NewWriter(sw)

	_, err = io.Copy(writer, reader)
	if err != nil {
		return fmt.Errorf("could not write to bucket %s for %s: %s", bucket, fileName, err)
	}

	if err := writer.Close(); err != nil {
		return err
	}

	if err := sw.Close(); err != nil {
		return err
	}

	// Update the object to set the metadata.
	objectAttrsToUpdate := storage.ObjectAttrsToUpdate{
		CacheControl: "no-cache, no-store",
	}
	if _, err := o.Update(ctx, objectAttrsToUpdate); err != nil {
		return err
	}

	purgeIfExists(ctx, exists, bucket, fileName)
	return nil
}

func cacheRawMedia(ctx context.Context, reader io.Reader, bucket, fileName string, contentType string, client *storage.Client) error {
	logger.For(ctx).Infof("caching raw media for %s", fileName)

	timeBeforeCopy := time.Now()
	o := client.Bucket(bucket).Object(fileName)

	exists, err := objectExists(ctx, o)
	if err != nil {
		return err
	}
	sw := o.NewWriter(ctx)
	_, err = io.Copy(sw, reader)
	if err != nil {
		return fmt.Errorf("could not write to bucket %s for %s: %s", bucket, fileName, err)
	}

	if err := sw.Close(); err != nil {
		return err
	}

	logger.For(ctx).Infof("storage copy took %s", time.Since(timeBeforeCopy))

	timeBeforeUpdate := time.Now()

	// Update the object to set the metadata.
	objectAttrsToUpdate := storage.ObjectAttrsToUpdate{
		CacheControl: "no-cache, no-store",
	}
	if contentType != "" {
		objectAttrsToUpdate.ContentType = contentType
	}
	if _, err := o.Update(ctx, objectAttrsToUpdate); err != nil {
		return err
	}

	logger.For(ctx).Infof("storage update took %s", time.Since(timeBeforeUpdate))
	purgeIfExists(ctx, exists, bucket, fileName)
	return nil
}

func thumbnailAndCache(ctx context.Context, videoURL, bucket, fileName string, contentType string, client *storage.Client) error {
	logger.For(ctx).Infof("caching raw media for %s", fileName)

	timeBeforeCopy := time.Now()
	o := client.Bucket(bucket).Object(fileName)

	exists, err := objectExists(ctx, o)
	if err != nil {
		return err
	}
	sw := o.NewWriter(ctx)
	logger.For(ctx).Infof("thumbnailing %s", videoURL)
	err = thumbnailVideoToWriter(videoURL, sw)
	if err != nil {
		return fmt.Errorf("could not write to bucket %s for %s: %s", bucket, fileName, err)
	}

	if err := sw.Close(); err != nil {
		return err
	}

	logger.For(ctx).Infof("storage copy took %s", time.Since(timeBeforeCopy))

	timeBeforeUpdate := time.Now()

	// Update the object to set the metadata.
	objectAttrsToUpdate := storage.ObjectAttrsToUpdate{
		CacheControl: "no-cache, no-store",
	}
	if contentType != "" {
		objectAttrsToUpdate.ContentType = contentType
	}
	if _, err := o.Update(ctx, objectAttrsToUpdate); err != nil {
		return err
	}

	logger.For(ctx).Infof("storage update took %s", time.Since(timeBeforeUpdate))
	purgeIfExists(ctx, exists, bucket, fileName)
	return nil
}

func deleteMedia(ctx context.Context, bucket, fileName string, client *storage.Client) error {
	return client.Bucket(bucket).Object(fileName).Delete(ctx)
}

func getMediaServingURL(pCtx context.Context, bucketID, objectID string, client *storage.Client) (string, error) {
	objectName := fmt.Sprintf("/gs/%s/%s", bucketID, objectID)

	_, err := client.Bucket(bucketID).Object(objectID).Attrs(pCtx)
	if err != nil {
		return "", fmt.Errorf("error getting attrs for %s: %s", objectName, err)
	}
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketID, objectID), nil
}

func downloadAndCache(pCtx context.Context, mediaURL, name, ipfsPrefix string, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) (persist.MediaType, error) {

	asURI := persist.TokenURI(mediaURL)

	timeBeforePredict := time.Now()
	mediaType, contentType, contentLength, _ := PredictMediaType(pCtx, asURI.String())
	logger.For(pCtx).Infof("predicted media type for %s: %s with length %s in %s", truncateString(mediaURL, 50), mediaType, util.InByteSizeFormat(uint64(contentLength)), time.Since(timeBeforePredict))

	if mediaType != persist.MediaTypeHTML && asURI.Type() == persist.URITypeIPFSGateway {
		indexAfterGateway := strings.Index(asURI.String(), "/ipfs/")
		path := asURI.String()[indexAfterGateway+len("/ipfs/"):]
		asURI = persist.TokenURI(fmt.Sprintf("ipfs://%s", path))
		logger.For(pCtx).Infof("converted %s to %s", mediaURL, asURI)
	}

outer:
	switch mediaType {
	case persist.MediaTypeVideo, persist.MediaTypeUnknown, persist.MediaTypeSVG, persist.MediaTypeBase64BMP:
		break outer
	default:
		switch asURI.Type() {
		case persist.URITypeIPFS, persist.URITypeArweave:
			logger.For(pCtx).Infof("uri for %s is of type %s: trying to cache", name, asURI.Type())
			break outer
		default:
			timeBeforeDelete := time.Now()
			// delete medias that are stored because the current media should be reflected directly in the metadata, not in GCP
			deleteMedia(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("%s-%s", ipfsPrefix, name), storageClient)
			deleteMedia(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("thumbnail-%s", name), storageClient)
			logger.For(pCtx).Infof("deleted medias for %s in %s", name, time.Since(timeBeforeDelete))
			return mediaType, nil
		}
	}

	timeBeforeDataReader := time.Now()
	reader, err := rpc.GetDataFromURIAsReader(pCtx, asURI, ipfsClient, arweaveClient)
	if err != nil {
		return persist.MediaTypeUnknown, fmt.Errorf("could not get reader for %s: %s", mediaURL, err)
	}
	logger.For(pCtx).Infof("got reader for %s in %s", name, time.Since(timeBeforeDataReader))
	defer reader.Close()

	if !mediaType.IsValid() {
		timeBeforeSniff := time.Now()
		mediaType, contentType = persist.SniffMediaType(reader.Headers())
		logger.For(pCtx).Infof("sniffed media type for %s: %s in %s", truncateString(mediaURL, 50), mediaType, time.Since(timeBeforeSniff))
	}

	if mediaType != persist.MediaTypeVideo {
		timeBeforeDeleteVideo := time.Now()
		// only videos get thumbnails, if the NFT was previously a video however, it might still have a thumbnail
		deleteMedia(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("thumbnail-%s", name), storageClient)
		logger.For(pCtx).Infof("deleted thumbnail for %s in %s", name, time.Since(timeBeforeDeleteVideo))
	}

	switch mediaType {
	case persist.MediaTypeVideo:
		timeBeforeCacheVideo := time.Now()
		err := cacheRawMedia(pCtx, reader, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name), contentType, storageClient)
		if err != nil {
			return mediaType, err
		}
		logger.For(pCtx).Infof("cached video for %s in %s", name, time.Since(timeBeforeCacheVideo))

		videoURL := fmt.Sprintf("https://storage.googleapis.com/%s/video-%s", viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), name)

		timeBeforeCache := time.Now()
		err = thumbnailAndCache(pCtx, videoURL, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("thumbnail-%s", name), "image/jpeg", storageClient)
		if err != nil {
			return mediaType, err
		}
		logger.For(pCtx).Infof("cached thumbnail for %s in %s", name, time.Since(timeBeforeCache))
		return persist.MediaTypeVideo, nil
	case persist.MediaTypeSVG:
		timeBeforeCache := time.Now()
		err = cacheRawSvgMedia(pCtx, reader, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("svg-%s", name), storageClient)
		if err != nil {
			return mediaType, err
		}
		logger.For(pCtx).Infof("cached svg for %s in %s", name, time.Since(timeBeforeCache))
		return persist.MediaTypeSVG, nil
	case persist.MediaTypeBase64BMP:
		timeBeforeCache := time.Now()
		err = cacheRawMedia(pCtx, reader, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name), contentType, storageClient)
		if err != nil {
			return mediaType, err
		}
		logger.For(pCtx).Infof("cached image for %s in %s", name, time.Since(timeBeforeCache))
		return persist.MediaTypeImage, nil
	default:
		switch asURI.Type() {
		case persist.URITypeIPFS, persist.URITypeArweave:
			if mediaType == persist.MediaTypeHTML && persist.TokenURI(mediaURL).IsPathPrefixed() {
				return mediaType, nil
			}
			logger.For(pCtx).Infof("DECENTRALIZED STORAGE: caching %f mb of raw media with type %s for %s at %s-%s", float64(contentLength)/1024/1024, mediaType, mediaURL, ipfsPrefix, name)
			if mediaType == persist.MediaTypeAnimation {
				timeBeforeCache := time.Now()
				err = cacheRawAnimationMedia(pCtx, reader, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("%s-%s", ipfsPrefix, name), storageClient)
				if err != nil {
					return mediaType, err
				}
				logger.For(pCtx).Infof("cached animation for %s in %s", name, time.Since(timeBeforeCache))
				return mediaType, nil
			}
			timeBeforeCache := time.Now()
			err = cacheRawMedia(pCtx, reader, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("%s-%s", ipfsPrefix, name), contentType, storageClient)
			if err != nil {
				return mediaType, err
			}
			logger.For(pCtx).Infof("cached raw media for %s in %s", name, time.Since(timeBeforeCache))
			return mediaType, nil
		default:
			return mediaType, nil
		}
	}
}

// PredictMediaType guesses the media type of the given URL.
func PredictMediaType(pCtx context.Context, url string) (persist.MediaType, string, int64, error) {

	spl := strings.Split(url, ".")
	if len(spl) > 1 {
		ext := spl[len(spl)-1]
		ext = strings.Split(ext, "?")[0]
		if t, ok := postfixesToMediaTypes[ext]; ok {
			return t.mediaType, t.contentType, 0, nil
		}
	}
	asURI := persist.TokenURI(url)
	uriType := asURI.Type()
	logger.For(pCtx).Debugf("predicting media type for %s: %s", url, uriType)
	switch uriType {
	case persist.URITypeBase64JSON, persist.URITypeJSON:
		return persist.MediaTypeJSON, "application/json", int64(len(asURI.String())), nil
	case persist.URITypeBase64SVG, persist.URITypeSVG:
		return persist.MediaTypeSVG, "image/svg", int64(len(asURI.String())), nil
	case persist.URITypeBase64BMP:
		return persist.MediaTypeBase64BMP, "image/bmp", int64(len(asURI.String())), nil
	case persist.URITypeHTTP, persist.URITypeIPFSAPI:
		req, err := http.NewRequestWithContext(pCtx, "GET", url, nil)
		if err != nil {
			return persist.MediaTypeUnknown, "", 0, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return persist.MediaTypeUnknown, "", 0, err
		}
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return persist.MediaTypeUnknown, "", 0, rpc.ErrHTTP{Status: resp.StatusCode, URL: url}
		}
		contentType := resp.Header.Get("Content-Type")
		contentType = strings.TrimSpace(contentType)
		whereCharset := strings.IndexByte(contentType, ';')
		if whereCharset != -1 {
			contentType = contentType[:whereCharset]
		}
		contentLength := resp.ContentLength
		return persist.MediaFromContentType(contentType), contentType, contentLength, nil
	case persist.URITypeIPFS:
		path := strings.TrimPrefix(asURI.String(), "ipfs://")
		headers, err := rpc.GetIPFSHeaders(pCtx, path)
		if err != nil {
			return persist.MediaTypeUnknown, "", 0, err
		}
		contentType := headers.Get("Content-Type")
		contentType = strings.TrimSpace(contentType)
		whereCharset := strings.IndexByte(contentType, ';')
		if whereCharset != -1 {
			contentType = contentType[:whereCharset]
		}
		contentLength := headers.Get("Content-Length")
		contentLengthInt := 0
		if contentLength != "" {
			contentLengthInt, err = strconv.Atoi(contentLength)
			if err != nil {
				return persist.MediaTypeUnknown, "", 0, err
			}
		}
		return persist.MediaFromContentType(contentType), contentType, int64(contentLengthInt), nil
	}
	return persist.MediaTypeUnknown, "", 0, nil
}

// GuessMediaType guesses the media type of the given bytes.
func GuessMediaType(bs []byte) (persist.MediaType, string) {

	cpy := make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff := bytes.NewBuffer(cpy)
	if _, err := gif.Decode(cpyBuff); err == nil {
		return persist.MediaTypeGIF, "image/gif"
	}
	cpy = make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff = bytes.NewBuffer(cpy)
	if _, _, err := image.Decode(cpyBuff); err == nil {
		return persist.MediaTypeImage, "image"
	}
	cpy = make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff = bytes.NewBuffer(cpy)
	if _, err := png.Decode(cpyBuff); err == nil {
		return persist.MediaTypeImage, "image/png"

	}
	cpy = make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff = bytes.NewBuffer(cpy)
	if _, err := jpeg.Decode(cpyBuff); err == nil {
		return persist.MediaTypeImage, "image/jpeg"
	}
	cpy = make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff = bytes.NewBuffer(cpy)
	var doc gltf.Document
	if err := gltf.NewDecoder(cpyBuff).Decode(&doc); err == nil {
		return persist.MediaTypeAnimation, "model/gltf-binary"
	}
	return persist.MediaTypeUnknown, ""

}

func thumbnailVideoToWriter(url string, writer io.Writer) error {
	c := exec.Command("ffmpeg", "-seekable", "1", "-i", url, "-ss", "00:00:00.000", "-vframes", "1", "-f", "mjpeg", "pipe:1")
	c.Stderr = os.Stderr
	c.Stdout = writer
	return c.Run()
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
	fxHash    = "KT1BJC12dG17CVvPKJ1VYaNnaT5mzfnUTwXv"
	fxHash2   = "KT1KEa8z6vWXDJrVqtMrAeDVzsvxat3kHaCE"
)

func (i TezImageKeywords) ForToken(tokenID persist.TokenID, contract persist.Address) []string {

	switch contract {

	case hicEtNunc:
		return []string{"displayUri", "image", "artifactUri"}
		// fxhash
	case fxHash, fxHash2:
		return []string{"displayUri", "artifactUri", "image", "uri"}
	default:
		return i
	}
}

func (a TezAnimationKeywords) ForToken(tokenID persist.TokenID, contract persist.Address) []string {
	switch contract {
	case fxHash, fxHash2:
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

func (e errGeneratingThumbnail) Error() string {
	return fmt.Sprintf("error generating thumbnail for url %s: %s", e.url, e.err)
}
