package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"os"
	"time"

	"cloud.google.com/go/storage"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"

	"github.com/nfnt/resize"
	"github.com/sirupsen/logrus"
	ffmpeg "github.com/u2takey/ffmpeg-go"
	"google.golang.org/appengine"
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
	res, err := appimage.ServingURL(pCtx, appengine.BlobKey(key), &appimage.ServingURLOptions{Secure: true})
	if err != nil {
		return "", err
	}
	return res.String(), nil
}

func downloadAndCache(pCtx context.Context, url, name string, ipfsClient *shell.Shell) (persist.MediaType, error) {

	bs, err := indexer.GetDataFromURI(persist.TokenURI(url), ipfsClient)
	if err != nil {
		return "", fmt.Errorf("could not get data from url %s: %s", url, err.Error())
	}

	buf := bytes.NewBuffer(bs)
	contentType := persist.SniffMediaType(bs)
	switch contentType {
	case persist.MediaTypeImage:
		img, _, err := image.Decode(buf)
		if err != nil {
			logrus.WithError(err).Error("could not decode image")
			if png, err := png.Decode(buf); err == nil {
				img = png
			} else if jpg, err := jpeg.Decode(buf); err == nil {
				img = jpg
			} else {
				return "", fmt.Errorf("could not decode image as jpg or png %s", err.Error())
			}
		}
		img = resize.Thumbnail(1024, 1024, img, resize.NearestNeighbor)
		buf = &bytes.Buffer{}
		err = jpeg.Encode(buf, img, nil)
		if err != nil {
			return "", fmt.Errorf("could not encode image as jpeg: %s", err.Error())
		}
		return persist.MediaTypeImage, cacheRawMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	case persist.MediaTypeVideo:
		// thumbnails the video, do we need to store a whole video?
		file, err := ioutil.TempFile("/tmp", "")
		if err != nil {
			return "", err
		}
		defer os.Remove(file.Name())

		_, err = file.Write(buf.Bytes())
		if err != nil {
			return "", err
		}

		scaled, err := scaleVideo(pCtx, file.Name(), 1280, 720)
		if err != nil {
			return "", err
		}
		cacheRawMedia(pCtx, scaled, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name))

		jp, err := readVidFrameAsJpeg(file.Name(), 1)
		if err != nil {
			return "", err
		}
		jpg, err := jpeg.Decode(bytes.NewBuffer(jp))
		if err != nil {
			return "", err
		}
		jpg = resize.Thumbnail(1024, 1024, jpg, resize.NearestNeighbor)
		buf = &bytes.Buffer{}
		jpeg.Encode(buf, jpg, nil)
		return persist.MediaTypeVideo, cacheRawMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	case persist.MediaTypeGIF:
		// thumbnails a gif, do we need to store the whole gif?
		asGif, err := gif.DecodeAll(bytes.NewReader(buf.Bytes()))
		if err != nil {
			return "", err
		}
		buf = &bytes.Buffer{}
		err = jpeg.Encode(buf, resize.Thumbnail(1024, 1024, asGif.Image[0], resize.NearestNeighbor), nil)
		if err != nil {
			return "", err
		}
		return persist.MediaTypeGIF, cacheRawMedia(pCtx, buf.Bytes(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	default:
		return persist.MediaTypeUnknown, errUnsupportedMediaType{contentType}
	}

}

func readVidFrameAsJpeg(inFileName string, frameNum int) ([]byte, error) {
	buf := &bytes.Buffer{}
	err := ffmpeg.Input(inFileName).
		Filter("select", ffmpeg.Args{fmt.Sprintf("gte(n,%d)", frameNum)}).
		Output("pipe:", ffmpeg.KwArgs{"vframes": 1, "format": "image2", "vcodec": "mjpeg"}).
		WithOutput(buf, os.Stdout).
		Run()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func scaleVideo(pCtx context.Context, inFileName string, w, h float64) ([]byte, error) {
	buf := &bytes.Buffer{}
	err := ffmpeg.Input(inFileName).
		Filter("scale", ffmpeg.Args{fmt.Sprintf("%f:%f", w, h)}).
		Output("pipe:", ffmpeg.KwArgs{"vcodec": "mjpeg"}).
		WithOutput(buf, os.Stdout).
		Run()
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
