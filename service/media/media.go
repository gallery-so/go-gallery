package media

import (
	"bytes"
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
	"sync"
	"time"

	"cloud.google.com/go/storage"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/nfnt/resize"
	"github.com/spf13/viper"

	"github.com/sirupsen/logrus"
	"google.golang.org/appengine/blobstore"
	appimage "google.golang.org/appengine/image"
)

var videoMu = &sync.Mutex{}

var errAlreadyHasMedia = errors.New("token already has preview and thumbnail URLs")

type errUnsupportedURL struct {
	url string
}

type errUnsupportedMediaType struct {
	mediaType persist.MediaType
}

// MakePreviewsForToken finds a token by its token identifier and uses it's data to generate media content and cache resized
// versions of the media content.
func MakePreviewsForToken(pCtx context.Context, contractAddress persist.Address, tokenID persist.TokenID, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell, storageClient *storage.Client) (persist.Media, error) {
	tokens, err := tokenRepo.GetByTokenIdentifiers(pCtx, tokenID, contractAddress, 1, 0)
	if err != nil {
		return persist.Media{}, err
	}
	if len(tokens) < 1 {
		return persist.Media{}, fmt.Errorf("no tokens found for tokenID %s and contractAddress %s", tokenID, contractAddress)
	}

	token := tokens[0]

	if token.Media.PreviewURL != "" && token.Media.ThumbnailURL != "" && token.Media.MediaURL != "" {
		return persist.Media{}, errAlreadyHasMedia
	}
	metadata := token.TokenMetadata
	return MakePreviewsForMetadata(pCtx, metadata, contractAddress, tokenID, token.TokenURI, ipfsClient, storageClient)
}

// MakePreviewsForMetadata uses a metadata map to generate media content and cache resized versions of the media content.
func MakePreviewsForMetadata(pCtx context.Context, metadata persist.TokenMetadata, contractAddress persist.Address, tokenID persist.TokenID, turi persist.TokenURI, ipfsClient *shell.Shell, storageClient *storage.Client) (persist.Media, error) {

	name := fmt.Sprintf("%s-%s", contractAddress, tokenID)

	res := persist.Media{}

	thumbnailURL, err := getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name), storageClient)
	if err == nil {
		res.ThumbnailURL = persist.NullString(thumbnailURL + "=256")
		res.PreviewURL = persist.NullString(thumbnailURL + "=512")
		res.MediaURL = persist.NullString(thumbnailURL + "=s1024")
		res.MediaType = persist.MediaTypeImage
		return res, nil
	}

	thumbnail, err := getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("thumbnail-%s", name), storageClient)
	if err == nil {
		videoURL, err := getVideoURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name), storageClient)
		if err != nil {
			return persist.Media{}, err
		}
		res.ThumbnailURL = persist.NullString(thumbnail + "=256")
		res.PreviewURL = persist.NullString(thumbnail + "=512")
		res.MediaURL = persist.NullString(videoURL)
		res.MediaType = persist.MediaTypeVideo
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

	asURI := persist.TokenURI(imgURL)
	switch asURI.Type() {
	case persist.URITypeBase64SVG:
		res.MediaType = persist.MediaTypeSVG
		data, err := rpc.GetDataFromURI(pCtx, asURI, ipfsClient)
		if err != nil {
			return persist.Media{}, fmt.Errorf("error getting data from base64 svg uri %s: %s", asURI, err)
		}
		res.MediaURL = persist.NullString(data)
		res.PreviewURL = res.MediaURL
		res.ThumbnailURL = res.MediaURL
		return res, nil
	case persist.URITypeSVG:
		res.MediaType = persist.MediaTypeSVG
		res.MediaURL = persist.NullString(asURI.String())
		res.PreviewURL = res.MediaURL
		res.ThumbnailURL = res.MediaURL

		return res, nil
	}

	mediaType, err := downloadAndCache(pCtx, imgURL, name, ipfsClient)
	if err != nil {
		switch err.(type) {
		case rpc.ErrHTTP:
			if err.(rpc.ErrHTTP).Status == http.StatusNotFound {
				mediaType = persist.MediaTypeInvalid
			} else {
				return persist.Media{}, fmt.Errorf("HTTP error downloading img %s: %s", imgURL, err)
			}
		case *net.DNSError:
			mediaType = persist.MediaTypeInvalid
			logrus.WithError(err).Warnf("DNS error downloading img %s: %s", imgURL, err)
		default:
			return persist.Media{}, fmt.Errorf("error downloading img %s: %s", imgURL, err)
		}
	}
	if vURL != "" {
		mediaType, err = downloadAndCache(pCtx, vURL, name, ipfsClient)
		if err != nil {
			switch err.(type) {
			case rpc.ErrHTTP:
				if err.(rpc.ErrHTTP).Status == http.StatusNotFound {
					mediaType = persist.MediaTypeInvalid
				} else {
					return persist.Media{}, fmt.Errorf("HTTP error downloading video %s: %s", vURL, err)
				}
			case *net.DNSError:
				mediaType = persist.MediaTypeInvalid
				logrus.WithError(err).Warnf("DNS error downloading video %s: %s", vURL, err)
			default:
				return persist.Media{}, fmt.Errorf("error downloading video %s: %s", vURL, err)
			}
		}
	}
	res.MediaType = mediaType

	switch mediaType {
	case persist.MediaTypeImage:
		thumbnailURL, err = getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name), storageClient)
		if err == nil {
			res.ThumbnailURL = persist.NullString(thumbnailURL + "=s256")
			res.PreviewURL = persist.NullString(thumbnailURL + "=s512")
			res.MediaURL = persist.NullString(thumbnailURL + "=s1024")
		} else {
			logrus.WithError(err).Error("could not get image serving URL")
		}
	case persist.MediaTypeVideo:
		thumbnailURL, err = getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("thumbnail-%s", name), storageClient)
		if err == nil {
			res.ThumbnailURL = persist.NullString(thumbnailURL + "=s256")
			res.PreviewURL = persist.NullString(thumbnailURL + "=s512")
		} else {
			logrus.WithError(err).Error("could not get image serving URL")
		}
		videoURL, err := getVideoURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name), storageClient)
		if err != nil {
			return persist.Media{}, err
		}
		res.MediaURL = persist.NullString(videoURL)
	default:
		res.MediaURL = persist.NullString(imgURL)
		res.ThumbnailURL = persist.NullString(imgURL)
		res.PreviewURL = persist.NullString(imgURL)
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

func getMediaServingURL(pCtx context.Context, bucketID, objectID string, client *storage.Client) (string, error) {
	objectName := fmt.Sprintf("/gs/%s/%s", bucketID, objectID)

	_, err := client.Bucket(bucketID).Object(objectID).Attrs(pCtx)
	if err != nil {
		return "", fmt.Errorf("error getting attrs for %s: %s", objectName, err)
	}
	key, err := blobstore.BlobKeyForFile(pCtx, objectName)
	if err != nil {
		return "", fmt.Errorf("error getting blob key for %s: %s", objectName, err)
	}
	res, err := appimage.ServingURL(pCtx, key, &appimage.ServingURLOptions{Secure: true})
	if err != nil {
		return "", fmt.Errorf("error getting serving url for %s: %s", objectName, err)
	}
	return res.String(), nil
}

func getVideoURL(pCtx context.Context, bucketID, objectID string, client *storage.Client) (string, error) {
	objectName := fmt.Sprintf("/gs/%s/%s", bucketID, objectID)

	_, err := client.Bucket(bucketID).Object(objectID).Attrs(pCtx)
	if err != nil {
		return "", fmt.Errorf("error getting attrs for %s: %s", objectName, err)
	}

	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketID, objectID), nil
}

func downloadAndCache(pCtx context.Context, url, name string, ipfsClient *shell.Shell) (mediaType persist.MediaType, err error) {

	asURI := persist.TokenURI(url)

	if asURI.Type() == persist.URITypeBase64SVG {
		return persist.MediaTypeBase64SVG, nil
	} else if asURI.Type() == persist.URITypeSVG {
		return persist.MediaTypeSVG, nil
	}

	bs, err := rpc.GetDataFromURI(pCtx, asURI, ipfsClient)
	if err != nil {
		return mediaType, err
	}

	buf := bytes.NewBuffer(bs)
	mediaType = persist.SniffMediaType(bs)

	switch mediaType {
	case persist.MediaTypeVideo:
		scaled, err := scaleVideo(pCtx, bs, -1, 720)
		if err != nil {
			return mediaType, fmt.Errorf("error scaling video: %s", err)
		}
		err = cacheRawMedia(pCtx, scaled, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name))
		if err != nil {
			return mediaType, fmt.Errorf("error caching video: %s", err)
		}

		jp, err := thumbnailVideo(pCtx, bs)
		if err != nil {
			return mediaType, fmt.Errorf("error generating thumbnail: %s", err)
		}
		jpg, err := jpeg.Decode(bytes.NewBuffer(jp))
		if err != nil {
			return mediaType, fmt.Errorf("error decoding thumbnail as jpeg: %s", err)
		}
		jpg = resize.Thumbnail(1024, 1024, jpg, resize.NearestNeighbor)
		buf = &bytes.Buffer{}
		err = jpeg.Encode(buf, jpg, nil)
		if err != nil {
			return mediaType, fmt.Errorf("error encoding thumbnail as jpeg: %s", err)
		}
		return persist.MediaTypeVideo, cacheRawMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("thumbnail-%s", name))
	case persist.MediaTypeGIF:
		// thumbnails a gif, do we need to store the whole gif?
		asGif, err := gif.DecodeAll(bytes.NewReader(buf.Bytes()))
		if err != nil {
			return mediaType, fmt.Errorf("error decoding gif: %s", err)
		}
		buf = &bytes.Buffer{}
		err = jpeg.Encode(buf, resize.Thumbnail(1024, 1024, asGif.Image[0], resize.NearestNeighbor), nil)
		if err != nil {
			return mediaType, fmt.Errorf("error encoding gif thumbnail as jpeg: %s", err)
		}
		return persist.MediaTypeGIF, cacheRawMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	default:
		img, _, err := image.Decode(buf)
		if err != nil {
			logrus.WithError(err).Error("could not decode image")
			if png, err := png.Decode(buf); err == nil {
				img = png
			} else if jpg, err := jpeg.Decode(buf); err == nil {
				img = jpg
			} else {
				return mediaType, fmt.Errorf("could not decode image as jpg or png %s", err.Error())
			}
		}
		img = resize.Thumbnail(1024, 1024, img, resize.NearestNeighbor)
		buf = &bytes.Buffer{}
		err = jpeg.Encode(buf, img, nil)
		if err != nil {
			return mediaType, fmt.Errorf("could not encode image as jpeg: %s", err.Error())
		}
		return persist.MediaTypeImage, cacheRawMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	}
}

func thumbnailVideo(pCtx context.Context, vid []byte) ([]byte, error) {
	c := exec.Command("ffmpeg", "-i", "pipe:0", "-ss", "00:00:01.000", "-vframes", "1", "-f", "singlejpeg", "pipe:1")
	in := bytes.NewReader(vid)
	c.Stdin = in
	out := &bytes.Buffer{}
	c.Stdout = out
	err := c.Run()
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil

}

func scaleVideo(pCtx context.Context, vid []byte, w, h int) ([]byte, error) {
	testFile, err := os.CreateTemp("/tmp", "resize-*.mp4")
	if err != nil {
		return nil, err
	}
	defer os.Remove(testFile.Name())
	defer testFile.Close()

	videoMu.Lock()
	defer videoMu.Unlock()

	c := exec.Command("ffmpeg", "-y", "-i", "pipe:0", "-vf", fmt.Sprintf("scale=%d:%d", w, h), "-f", "mp4", testFile.Name())

	err = pipeIOForCmdNoOut(c, vid)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(testFile)

}

func pipeIOForCmd(c *exec.Cmd, input []byte) ([]byte, error) {
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

	stdin.Write(input)

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

func pipeIOForCmdNoOut(c *exec.Cmd, input []byte) error {

	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	stdin, err := c.StdinPipe()
	if err != nil {
		return err
	}
	err = c.Start()
	if err != nil {
		return err
	}

	_, err = stdin.Write(input)
	if err != nil {
		return err
	}

	err = stdin.Close()
	if err != nil {
		return err
	}

	err = c.Wait()
	if err != nil {
		return err
	}

	return nil
}

func (e errUnsupportedURL) Error() string {
	return fmt.Sprintf("unsupported url %s", e.url)
}

func (e errUnsupportedMediaType) Error() string {
	return fmt.Sprintf("unsupported media type %s", e.mediaType)
}
