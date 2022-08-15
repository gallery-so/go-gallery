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
	"strings"
	"sync"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/mediamapper"
	"github.com/sirupsen/logrus"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/qmuntal/gltf"
	"github.com/spf13/viper"
)

var errAlreadyHasMedia = errors.New("token already has preview and thumbnail URLs")

var downloadLock = &sync.Mutex{}

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

var postfixesToMediaTypes = map[string]persist.MediaType{
	"jpg":  persist.MediaTypeImage,
	"jpeg": persist.MediaTypeImage,
	"png":  persist.MediaTypeImage,
	"webp": persist.MediaTypeImage,
	"gif":  persist.MediaTypeGIF,
	"mp4":  persist.MediaTypeVideo,
	"webm": persist.MediaTypeVideo,
	"glb":  persist.MediaTypeAnimation,
	"gltf": persist.MediaTypeAnimation,
	"svg":  persist.MediaTypeSVG,
}

// MakePreviewsForMetadata uses a metadata map to generate media content and cache resized versions of the media content.
func MakePreviewsForMetadata(pCtx context.Context, metadata persist.TokenMetadata, contractAddress string, tokenID persist.TokenID, turi persist.TokenURI, chain persist.Chain, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) (persist.Media, error) {

	name := fmt.Sprintf("%s-%s", contractAddress, tokenID)

	imgURL, vURL := findInitialURLs(metadata, name, turi)

	imgAsURI := persist.TokenURI(imgURL)
	videoAsURI := persist.TokenURI(vURL)

	logger.For(pCtx).WithFields(logrus.Fields{"tokenURI": claspString(turi.String(), 25), "imgURL": claspString(imgURL, 50), "vURL": claspString(vURL, 50), "name": name}).Debug("MakePreviewsForMetadata initial")

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
		logger.For(pCtx).WithFields(logrus.Fields{"tokenURI": claspString(turi.String(), 25), "imgURL": claspString(imgURL, 50), "vURL": claspString(vURL, 50), "name": name}).Debug("MakePreviewsForMetadata vURL valid")
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

	logger.For(pCtx).WithFields(logrus.Fields{"tokenURI": claspString(turi.String(), 25), "imgURL": claspString(imgURL, 25), "vURL": claspString(vURL, 25), "mediaType": mediaType, "name": name}).Debug("MakePreviewsForMetadata mediaType")

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
	if vURL != "" {
		res.ThumbnailURL = persist.NullString(imageURL)
	}
	if vURL != "" {
		logger.For(pCtx).Infof("using vURL %s: %s", name, vURL)
		res.MediaURL = persist.NullString(vURL)
	} else if imageURL != "" && res.ThumbnailURL.String() != imageURL {
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

func getThumbnailURL(pCtx context.Context, tokenBucket string, name string, imgURL string, storageClient *storage.Client) string {
	if storageImageURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("image-%s", name), storageClient); err == nil {
		logger.For(pCtx).Infof("found imageURL for thumbnail %s: %s", name, storageImageURL)
		return storageImageURL
	} else if storageImageURL, err := getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("thumbnail-%s", name), storageClient); err == nil {
		logger.For(pCtx).Infof("found thumbnailURL for %s: %s", name, storageImageURL)
		return storageImageURL
	} else if storageImageURL, err = getMediaServingURL(pCtx, tokenBucket, fmt.Sprintf("svg-%s", name), storageClient); err == nil {
		logger.For(pCtx).Infof("found svg for thumbnail %s: %s", name, storageImageURL)
		return storageImageURL
	} else if imgURL != "" {
		logger.For(pCtx).Infof("using imgURL for thumbnail %s: %s", name, imgURL)
		return imgURL
	}
	return ""
}

func remapIPFS(ipfs string) string {
	if persist.TokenURI(ipfs).Type() != persist.URITypeIPFS {
		return ipfs
	}
	path := util.GetIPFSPath(ipfs)
	return fmt.Sprintf("%s/ipfs/%s", viper.GetString("IPFS_URL"), path)
}

func remapMedia(media persist.Media) persist.Media {
	media.MediaURL = persist.NullString(remapIPFS(media.MediaURL.String()))
	media.ThumbnailURL = persist.NullString(remapIPFS(media.ThumbnailURL.String()))
	return media
}

func findInitialURLs(metadata persist.TokenMetadata, name string, turi persist.TokenURI) (imgURL string, vURL string) {

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

	if it, ok := util.GetValueFromMapUnsafe(metadata, "animation", util.DefaultSearchDepth).(string); ok && it != "" {
		logger.For(nil).Infof("found initial animation url for %s: %s", name, it)
		vURL = it
	} else if it, ok := util.GetValueFromMapUnsafe(metadata, "video", util.DefaultSearchDepth).(string); ok && it != "" {
		logger.For(nil).Infof("found initial video url for %s: %s", name, it)
		vURL = it
	}

	if it, ok := util.GetValueFromMapUnsafe(metadata, "image", util.DefaultSearchDepth).(string); ok && it != "" {
		logger.For(nil).Infof("found initial image url for %s: %s", name, it)
		imgURL = it
	}

	if imgURL == "" && vURL == "" {
		logger.For(nil).Infof("no image url found for %s - using token URI %s", name, turi)
		imgURL = turi.String()
	}
	return imgURL, vURL
}

func cacheRawSvgMedia(ctx context.Context, img []byte, bucket, fileName string, client *storage.Client) error {

	deleteMedia(ctx, bucket, fileName, client)

	sw := client.Bucket(bucket).Object(fileName).NewWriter(ctx)
	_, err := sw.Write(img)
	if err != nil {
		return fmt.Errorf("could not write to bucket %s for %s: %s", bucket, fileName, err)
	}

	if err := sw.Close(); err != nil {
		return err
	}

	logrus.Infof("adding svg to attrs for %s", fileName)
	o := client.Bucket(bucket).Object(fileName)

	// Update the object to set the metadata.
	objectAttrsToUpdate := storage.ObjectAttrsToUpdate{
		ContentType:  "image/svg+xml",
		CacheControl: "no-cache, no-store",
	}
	if _, err := o.Update(ctx, objectAttrsToUpdate); err != nil {
		return err
	}

	err = mediamapper.PurgeImage(ctx, fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, fileName))
	if err != nil {
		logger.For(ctx).Errorf("could not purge image %s: %s", fileName, err)
	}
	return nil
}

func cacheRawAnimationMedia(ctx context.Context, animation []byte, bucket, fileName string, client *storage.Client) error {
	deleteMedia(ctx, bucket, fileName, client)

	sw := client.Bucket(bucket).Object(fileName).NewWriter(ctx)

	writer := gzip.NewWriter(sw)

	_, err := io.Copy(writer, bytes.NewBuffer(animation))
	if err != nil {
		return fmt.Errorf("could not write to bucket %s for %s: %s", bucket, fileName, err)
	}

	if err := writer.Close(); err != nil {
		return err
	}

	if err := sw.Close(); err != nil {
		return err
	}

	o := client.Bucket(bucket).Object(fileName)

	// Update the object to set the metadata.
	objectAttrsToUpdate := storage.ObjectAttrsToUpdate{
		CacheControl: "no-cache, no-store",
	}
	if _, err := o.Update(ctx, objectAttrsToUpdate); err != nil {
		return err
	}

	err = mediamapper.PurgeImage(ctx, fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, fileName))
	if err != nil {
		logger.For(ctx).Errorf("could not purge image %s: %s", fileName, err)
	}
	return nil
}

func cacheRawMedia(ctx context.Context, img []byte, bucket, fileName string, client *storage.Client) error {
	logger.For(ctx).Infof("caching raw media for %s", fileName)

	deleteMedia(ctx, bucket, fileName, client)

	sw := client.Bucket(bucket).Object(fileName).NewWriter(ctx)
	_, err := sw.Write(img)
	if err != nil {
		return fmt.Errorf("could not write to bucket %s for %s: %s", bucket, fileName, err)
	}

	if err := sw.Close(); err != nil {
		return err
	}

	o := client.Bucket(bucket).Object(fileName)

	// Update the object to set the metadata.
	objectAttrsToUpdate := storage.ObjectAttrsToUpdate{
		CacheControl: "no-cache, no-store",
	}
	if _, err := o.Update(ctx, objectAttrsToUpdate); err != nil {
		return err
	}

	err = mediamapper.PurgeImage(ctx, fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, fileName))
	if err != nil {
		logger.For(ctx).Errorf("could not purge image %s: %s", fileName, err)
	}
	return nil
}

func deleteMedia(ctx context.Context, bucket, fileName string, client *storage.Client) {
	client.Bucket(bucket).Object(fileName).Delete(ctx)
}

func getMediaServingURL(pCtx context.Context, bucketID, objectID string, client *storage.Client) (string, error) {
	objectName := fmt.Sprintf("/gs/%s/%s", bucketID, objectID)

	_, err := client.Bucket(bucketID).Object(objectID).Attrs(pCtx)
	if err != nil {
		return "", fmt.Errorf("error getting attrs for %s: %s", objectName, err)
	}
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketID, objectID), nil
}

func downloadAndCache(pCtx context.Context, url, name, ipfsPrefix string, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) (persist.MediaType, error) {

	asURI := persist.TokenURI(url)

	mediaType, _ := PredictMediaType(pCtx, url)

	logger.For(pCtx).Infof("predicted media type for %s: %s", claspString(url, 50), mediaType)

	if mediaType != persist.MediaTypeHTML && asURI.Type() == persist.URITypeIPFSGateway {
		indexAfterGateway := strings.Index(asURI.String(), "/ipfs/")
		path := asURI.String()[indexAfterGateway+len("/ipfs/"):]
		asURI = persist.TokenURI(fmt.Sprintf("ipfs://%s", path))
		logger.For(pCtx).Infof("converted %s to %s", url, asURI)
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
			// delete medias that are stored because the current media should be reflected directly in the metadata, not in GCP
			deleteMedia(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("%s-%s", ipfsPrefix, name), storageClient)
			deleteMedia(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("thumbnail-%s", name), storageClient)
			return mediaType, nil
		}
	}

	if util.Contains([]string{"development", "sandbox-backend", "production"}, strings.ToLower(viper.GetString("ENV"))) {
		downloadLock.Lock()
		defer downloadLock.Unlock()
	}

	bs, err := rpc.GetDataFromURI(pCtx, asURI, ipfsClient, arweaveClient)
	if err != nil {
		return persist.MediaTypeUnknown, fmt.Errorf("could not download %s: %s", url, err)
	}

	logger.For(pCtx).Infof("downloaded %f MB from %s for %s", float64(len(bs))/1024/1024, claspString(url, 50), name)

	if mediaType == persist.MediaTypeUnknown {
		mediaType = GuessMediaType(bs)
	}

	buf := bytes.NewBuffer(bs)

	sniffed := persist.SniffMediaType(bs)
	if mediaType != persist.MediaTypeHTML && mediaType != persist.MediaTypeBase64BMP {
		mediaType = sniffed
	}

	logger.For(pCtx).Infof("sniffed media type for %s: %s", claspString(url, 50), mediaType)

	if mediaType != persist.MediaTypeVideo {
		// only videos get thumbnails, if the NFT was previously a video however, it might still have a thumbnail
		deleteMedia(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("thumbnail-%s", name), storageClient)
	}

	switch mediaType {
	case persist.MediaTypeVideo:
		err := cacheRawMedia(pCtx, bs, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name), storageClient)
		if err != nil {
			return mediaType, err
		}

		videoURL := fmt.Sprintf("https://storage.googleapis.com/%s/video-%s", viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), name)

		jp, err := thumbnailVideo(videoURL)
		if err != nil {
			logger.For(pCtx).Infof("error generating thumbnail for %s: %s", url, err)
			return mediaType, errGeneratingThumbnail{url: url, err: err}
		}
		buf = bytes.NewBuffer(jp)
		logger.For(pCtx).Infof("generated thumbnail for %s - file size %s", url, util.InByteSizeFormat(uint64(buf.Len())))
		return persist.MediaTypeVideo, cacheRawMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("thumbnail-%s", name), storageClient)
	case persist.MediaTypeSVG:
		return persist.MediaTypeSVG, cacheRawSvgMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("svg-%s", name), storageClient)
	case persist.MediaTypeBase64BMP:
		return persist.MediaTypeImage, cacheRawMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name), storageClient)
	default:
		switch asURI.Type() {
		case persist.URITypeIPFS, persist.URITypeArweave:
			if mediaType == persist.MediaTypeHTML && strings.HasPrefix(url, "https://") {
				return mediaType, nil
			}
			logger.For(pCtx).Infof("DECENTRALIZED STORAGE: caching %f mb of raw media with type %s for %s at %s-%s", float64(len(buf.Bytes()))/1024/1024, mediaType, url, ipfsPrefix, name)
			if mediaType == persist.MediaTypeAnimation {
				return mediaType, cacheRawAnimationMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("%s-%s", ipfsPrefix, name), storageClient)
			}
			return mediaType, cacheRawMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("%s-%s", ipfsPrefix, name), storageClient)
		default:
			return mediaType, nil
		}
	}
}

// PredictMediaType guesses the media type of the given URL.
func PredictMediaType(pCtx context.Context, url string) (persist.MediaType, error) {

	spl := strings.Split(url, ".")
	if len(spl) > 1 {
		ext := spl[len(spl)-1]
		ext = strings.Split(ext, "?")[0]
		if t, ok := postfixesToMediaTypes[ext]; ok {
			return t, nil
		}
	}
	asURI := persist.TokenURI(url)
	uriType := asURI.Type()
	logger.For(pCtx).Debugf("predicting media type for %s: %s", url, uriType)
	switch uriType {
	case persist.URITypeBase64JSON, persist.URITypeJSON:
		return persist.MediaTypeJSON, nil
	case persist.URITypeBase64SVG, persist.URITypeSVG:
		return persist.MediaTypeSVG, nil
	case persist.URITypeBase64BMP:
		return persist.MediaTypeBase64BMP, nil
	case persist.URITypeHTTP, persist.URITypeIPFSAPI:
		req, err := http.NewRequestWithContext(pCtx, "GET", url, nil)
		if err != nil {
			return persist.MediaTypeUnknown, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return persist.MediaTypeUnknown, err
		}
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			return persist.MediaTypeUnknown, rpc.ErrHTTP{Status: resp.StatusCode, URL: url}
		}
		contentType := resp.Header.Get("Content-Type")
		return persist.MediaFromContentType(contentType), nil
	case persist.URITypeIPFS:
		path := strings.TrimPrefix(asURI.String(), "ipfs://")
		headers, err := rpc.GetIPFSHeaders(pCtx, path)
		if err != nil {
			return persist.MediaTypeUnknown, err
		}
		contentType := headers.Get("Content-Type")
		return persist.MediaFromContentType(contentType), nil
	}
	return persist.MediaTypeUnknown, nil
}

// GuessMediaType guesses the media type of the given bytes.
func GuessMediaType(bs []byte) persist.MediaType {

	cpy := make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff := bytes.NewBuffer(cpy)
	var doc gltf.Document
	if err := gltf.NewDecoder(cpyBuff).Decode(&doc); err == nil {
		return persist.MediaTypeAnimation
	}
	cpy = make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff = bytes.NewBuffer(cpy)
	if _, err := gif.Decode(cpyBuff); err == nil {
		return persist.MediaTypeGIF
	}
	cpy = make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff = bytes.NewBuffer(cpy)
	if _, _, err := image.Decode(cpyBuff); err == nil {
		return persist.MediaTypeImage
	}
	cpy = make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff = bytes.NewBuffer(cpy)
	if _, err := png.Decode(cpyBuff); err == nil {
		return persist.MediaTypeImage

	}
	cpy = make([]byte, len(bs))
	copy(cpy, bs)
	cpyBuff = bytes.NewBuffer(cpy)
	if _, err := jpeg.Decode(cpyBuff); err == nil {
		return persist.MediaTypeImage
	}
	return persist.MediaTypeUnknown

}

func thumbnailVideo(url string) ([]byte, error) {
	c := exec.Command("ffmpeg", "-seekable", "1", "-i", url, "-ss", "00:00:01.000", "-vframes", "1", "-f", "mjpeg", "pipe:1")
	c.Stderr = os.Stderr
	return c.Output()

}

func claspString(s string, i int) string {
	if len(s) > i {
		return s[:i]
	}
	return s
}

func pipeIOForCmd(c *exec.Cmd, input []byte) ([]byte, error) {

	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}

	go func() {
		defer stdin.Close()
		stdin.Write(input)
	}()

	return c.Output()
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
