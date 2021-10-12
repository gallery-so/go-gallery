package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/spf13/viper"

	"github.com/nfnt/resize"
	"github.com/sirupsen/logrus"
	ffmpeg "github.com/u2takey/ffmpeg-go"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	appimage "google.golang.org/appengine/image"
)

func makePreviewsForToken(pCtx context.Context, contractAddress, tokenID string, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell) (*persist.Media, error) {
	tokens, err := tokenRepo.GetByNFTIdentifiers(pCtx, tokenID, contractAddress)
	if err != nil {
		return nil, err
	}
	if len(tokens) > 1 {
		return nil, errors.New("too many tokens returned for one token ID and contract address")
	}

	token := tokens[0]

	if token.Media.PreviewURL != "" && token.Media.ThumbnailURL != "" {
		return nil, errors.New("token already has preview and thumbnail URLs")
	}
	metadata := token.TokenMetadata
	return makePreviewsForMetadata(pCtx, metadata, contractAddress, tokenID, token.TokenURI, ipfsClient)
}

func makePreviewsForMetadata(pCtx context.Context, metadata map[string]interface{}, contractAddress, tokenID, turi string, ipfsClient *shell.Shell) (*persist.Media, error) {

	name := fmt.Sprintf("%s-%s", contractAddress, tokenID)

	res := &persist.Media{}

	imageURL, err := getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("image-%s", name))
	if err == nil {
		res.ThumbnailURL = imageURL + "=s96"
		res.PreviewURL = imageURL + "=s256"
		res.MediaURL = imageURL + "=s1024"
	}

	videoURL, err := getMediaServingURL(pCtx, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), fmt.Sprintf("video-%s", name))
	if err == nil {
		res.MediaURL = videoURL
	}

	if res.MediaURL != "" {
		return res, nil
	}

	imgURL := ""
	vURL := ""

	if it, ok := metadata["animation_url"]; ok && it != nil {
		vURL = it.(string)
	} else if it, ok := metadata["video_url"]; ok && it != nil {
		vURL = it.(string)
	}
	if it, ok := metadata["image"]; ok && it != nil {
		imgURL = it.(string)
	} else if it, ok := metadata["image_url"]; ok && it != nil {
		imgURL = it.(string)
	}

	if imgURL == "" {
		imgURL = turi
	}

	mediaType, err := downloadAndCache(pCtx, imgURL, name, ipfsClient)
	if err != nil {
		return nil, err
	}
	if vURL != "" {
		mt, _ := downloadAndCache(pCtx, vURL, name, ipfsClient)
		mediaType = mt
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
		return fmt.Errorf("Could not write file: %v", err)
	}

	if err := sw.Close(); err != nil {
		return fmt.Errorf("Could not put file: %v", err)
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

	// TODO handle when url is ipfs
	buf := &bytes.Buffer{}

	if strings.HasPrefix(url, "ipfs://") {
		res, err := ipfsClient.Cat(strings.TrimPrefix(url, "ipfs://"))
		if err != nil {
			return "", err
		}
		_, err = io.Copy(buf, res)
		if err != nil {
			return "", err
		}
	} else if strings.HasPrefix(url, "http") {
		hc := &http.Client{
			Timeout: time.Second * 5,
		}

		req, err := hc.Get(url)
		if err != nil {
			return "", err
		}
		defer req.Body.Close()

		_, err = io.Copy(buf, req.Body)
		if err != nil {
			return "", err
		}
	} else {
		return "", errors.New("unsupported url")
	}

	contentType := persist.SniffMediaType(buf.Bytes())
	logrus.Debugf("contentType: %s", contentType)
	switch contentType {
	case persist.MediaTypeImage:
		img, _, err := image.Decode(buf)
		if err != nil {
			return "", err
		}
		img = resize.Thumbnail(1024, 1024, img, resize.NearestNeighbor)
		buf = &bytes.Buffer{}
		jpeg.Encode(buf, img, nil)
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

		scaled, err := scaleVideo(pCtx, file.Name(), 640, 480)
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
		return "", errors.New("unsupported media type")
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
