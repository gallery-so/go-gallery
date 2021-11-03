package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"cloud.google.com/go/storage"
	"github.com/bakape/thumbnailer"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"

	"github.com/sirupsen/logrus"
	"google.golang.org/appengine/blobstore"
	appimage "google.golang.org/appengine/image"
)

var errAlreadyHasMedia = errors.New("token already has preview and thumbnail URLs")

type errUnsupportedURL struct {
	url string
}

type errUnsupportedMediaType struct {
	mediaType persist.MediaType
}

func makePreviewsForToken(pCtx context.Context, contractAddress persist.Address, tokenID persist.TokenID, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell) (*persist.Media, error) {
	tokens, err := tokenRepo.GetByTokenIdentifiers(pCtx, tokenID, contractAddress)
	if err != nil {
		return nil, err
	}
	if len(tokens) < 1 {
		return nil, fmt.Errorf("no tokens found for tokenID %s and contractAddress %s", tokenID, contractAddress)
	}

	token := tokens[0]

	if token.Media.PreviewURL != "" && token.Media.ThumbnailURL != "" && token.Media.MediaURL != "" {
		return nil, errAlreadyHasMedia
	}
	metadata := token.TokenMetadata
	return makePreviewsForMetadata(pCtx, metadata, contractAddress, tokenID, token.TokenURI, ipfsClient)
}

func makePreviewsForMetadata(pCtx context.Context, metadata persist.TokenMetadata, contractAddress persist.Address, tokenID persist.TokenID, turi persist.TokenURI, ipfsClient *shell.Shell) (*persist.Media, error) {

	name := fmt.Sprintf("%s-%s", contractAddress, tokenID)

	res := &persist.Media{}

	imageURL, err := getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	if err == nil {
		res.ThumbnailURL = imageURL + "=s96"
		res.PreviewURL = imageURL + "=s256"
		res.MediaURL = imageURL + "=s1024"
		res.MediaType = persist.MediaTypeImage
	}

	videoURL, err := getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name))
	if err == nil {
		res.MediaURL = videoURL
		res.MediaType = persist.MediaTypeVideo
	}

	if res.MediaURL != "" {
		return res, nil
	}

	imgURL := ""
	vURL := ""

	if it, ok := util.GetValueFromMapUnsafe(metadata, "animation", util.DefaultSearchDepth).(string); ok {
		vURL = it
	} else if it, ok := util.GetValueFromMapUnsafe(metadata, "video", util.DefaultSearchDepth).(string); ok {
		vURL = it
	}

	if it, ok := util.GetValueFromMapUnsafe(metadata, "image", util.DefaultSearchDepth).(string); ok {
		imgURL = it
	}

	if imgURL == "" {
		imgURL = turi.String()
	}

	mediaType, err := downloadAndCache(pCtx, imgURL, name, ipfsClient)
	if err != nil {
		return nil, err
	}
	if vURL != "" {
		mediaType, err = downloadAndCache(pCtx, vURL, name, ipfsClient)
		if err != nil {
			return nil, err
		}
	}
	res.MediaType = mediaType

	imageURL, err = getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	if err == nil {
		res.ThumbnailURL = imageURL + "=s96"
		res.PreviewURL = imageURL + "=s256"
		res.MediaURL = imageURL + "=s1024"
	} else {
		logrus.WithError(err).Error("could not get image serving URL")
	}

	videoURL, err = getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name))
	if err == nil {
		res.MediaURL = videoURL
	} else {
		logrus.WithError(err).Error("could not get video serving URL")
	}

	return res, nil
}

func cacheRawMedia(pCtx context.Context, img []byte, bucket, fileName string) error {

	ctx, cancel := context.WithTimeout(pCtx, 2*time.Second)
	defer cancel()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}

	sw := client.Bucket(bucket).Object(fileName).NewWriter(ctx)
	if _, err := sw.Write(img); err != nil {
		return err
	}

	if err := sw.Close(); err != nil {
		return err
	}
	return nil
}

func getMediaServingURL(pCtx context.Context, bucketID, objectID string) (string, error) {
	objectName := fmt.Sprintf("/gs/%s/%s", bucketID, objectID)

	key, err := blobstore.BlobKeyForFile(pCtx, objectName)
	if err != nil {
		return "", err
	}
	client, err := storage.NewClient(pCtx)
	if err != nil {
		return "", err
	}
	_, err = client.Bucket(bucketID).Object(objectID).Attrs(pCtx)
	if err != nil {
		return "", err
	}
	res, err := appimage.ServingURL(pCtx, key, &appimage.ServingURLOptions{Secure: true})
	if err != nil {
		return "", err
	}
	return res.String(), nil
}

func downloadAndCache(pCtx context.Context, url, name string, ipfsClient *shell.Shell) (persist.MediaType, error) {

	bs, err := indexer.GetDataFromURI(pCtx, persist.TokenURI(url), ipfsClient)
	if err != nil {
		return "", fmt.Errorf("could not get data from url %s: %s", url, err.Error())
	}

	contentType := persist.SniffMediaType(bs)
	thumbnailDimensions := thumbnailer.Dims{Width: 1024, Height: 1024}

	thumbnailOptions := thumbnailer.Options{JPEGQuality: 100, ThumbDims: thumbnailDimensions}

	_, thumb, err := thumbnailer.ProcessBuffer(bs, thumbnailOptions)
	if err != nil {
		return "", fmt.Errorf("could not process video: %s", err.Error())
	}
	switch contentType {
	case persist.MediaTypeImage, persist.MediaTypeGIF:
		return persist.MediaTypeImage, cacheRawMedia(pCtx, thumb.Data, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	case persist.MediaTypeVideo:

		scaled, err := scaleVideo(pCtx, bs, -1, 720)
		if err != nil {
			return "", err
		}
		err = cacheRawMedia(pCtx, scaled, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name))
		if err != nil {
			return "", err
		}

		return persist.MediaTypeVideo, cacheRawMedia(pCtx, thumb.Data, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	default:
		return persist.MediaTypeUnknown, errUnsupportedMediaType{contentType}
	}

}

func scaleVideo(pCtx context.Context, vid []byte, w, h int) ([]byte, error) {
	c := exec.Command("ffmpeg", "-i", "pipe:0", "-vf", fmt.Sprintf("scale=%d:%d", w, h), "pipe:1")

	buf := bytes.NewBuffer(make([]byte, 5*1024*1024))
	c.Stdout = buf
	c.Stderr = os.Stderr

	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	err = c.Start()
	if err != nil {
		return nil, err
	}

	_, err = stdin.Write(vid)
	if err != nil {
		return nil, err
	}

	err = stdin.Close()
	if err != nil {
		return nil, err
	}

	err = c.Wait()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (e errUnsupportedURL) Error() string {
	return fmt.Sprintf("unsupported url %s", e.url)
}

func (e errUnsupportedMediaType) Error() string {
	return fmt.Sprintf("unsupported media type %s", e.mediaType)
}
