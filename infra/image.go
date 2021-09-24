package infra

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/gif"
	"image/jpeg"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/sirupsen/logrus"
	ffmpeg "github.com/u2takey/ffmpeg-go"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/image"
)

func makePreviewsForToken(pCtx context.Context, contractAddress, tokenID string, pRuntime *runtime.Runtime) error {
	tokens, err := persist.TokenGetByNFTIdentifiers(pCtx, tokenID, contractAddress, pRuntime)
	if err != nil {
		return err
	}
	if len(tokens) > 1 {
		return errors.New("too many tokens returned for one token ID and contract address")
	}

	token := tokens[0]

	if token.PreviewURL != "" && token.ThumbnailURL != "" {
		return errors.New("token already has preview and thumbnail URLs")
	}
	name := fmt.Sprintf("%s-%s", contractAddress, tokenID)

	url := ""

	if it, ok := token.TokenMetadata["image"]; ok {
		url = it.(string)
	} else if it, ok := token.TokenMetadata["image_url"]; ok {
		url = it.(string)
	} else if it, ok := token.TokenMetadata["video_url"]; ok {
		url = it.(string)
	} else if it, ok := token.TokenMetadata["animation_url"]; ok {
		url = it.(string)
	}

	if url == "" {
		url = token.TokenURI
	}

	err = downloadAndCache(pCtx, url, name, pRuntime)
	if err != nil {
		return err
	}
	update := &persist.TokenUpdateImageURLsInput{}

	imageURL, err := getMediaServingURL(pCtx, pRuntime.Config.GCloudTokenContentBucket, fmt.Sprintf("image-%s", name))
	if err == nil {
		update.ThumbnailURL = imageURL + "=s256"
		update.PreviewURL = imageURL + "=s1024"
	}

	videoURL, err := getMediaServingURL(pCtx, pRuntime.Config.GCloudTokenContentBucket, fmt.Sprintf("video-%s", name))
	if err == nil {
		update.VideoURL = videoURL
	}

	logrus.WithFields(logrus.Fields{"token": token.ID, "servingURL": imageURL}).Info("processImagesForToken")

	return persist.TokenUpdateByID(pCtx, token.ID, update, pRuntime)
}

func cacheRawMedia(pCtx context.Context, image []byte, bucket, fileName string) error {

	ctx, cancel := context.WithTimeout(pCtx, 2*time.Second)
	defer cancel()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}

	sw := client.Bucket(bucket).Object(fileName).NewWriter(ctx)
	if _, err := sw.Write(image); err != nil {
		return fmt.Errorf("Could not write file: %v", err)
	}

	if err := sw.Close(); err != nil {
		return fmt.Errorf("Could not put file: %v", err)
	}
	return nil
}

func getMediaServingURL(pCtx context.Context, bucketID, objectID string) (string, error) {
	objectName := fmt.Sprintf("gs/%s/%s", bucketID, objectID)
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
	res, err := image.ServingURL(pCtx, appengine.BlobKey(key), &image.ServingURLOptions{Secure: true})
	if err != nil {
		return "", err
	}
	return res.String(), nil
}

func downloadAndCache(pCtx context.Context, url, name string, pRuntime *runtime.Runtime) error {

	// TODO handle when url is ipfs
	buf := &bytes.Buffer{}

	if strings.HasPrefix(url, "ipfs://") {
		res, err := pRuntime.IPFS.Cat(strings.TrimPrefix(url, "ipfs://"))
		if err != nil {
			return err
		}
		_, err = io.Copy(buf, res)
		if err != nil {
			return err
		}
	} else if strings.HasPrefix(url, "http") {
		hc := &http.Client{
			Timeout: time.Second * 2,
		}

		req, err := hc.Get(url)
		if err != nil {
			return err
		}
		defer req.Body.Close()

		_, err = io.Copy(buf, req.Body)
		if err != nil {
			return err
		}
	} else {
		return errors.New("unsupported url")
	}

	contentType := http.DetectContentType(buf.Bytes()[:512])

	if strings.HasPrefix(contentType, "image/") {
		if strings.HasPrefix(contentType, "image/gif") {
			// thumbnails a gif, do we need to store the whole gif?
			asGif, err := gif.DecodeAll(bytes.NewReader(buf.Bytes()))
			if err != nil {
				return err
			}
			buf = &bytes.Buffer{}
			err = jpeg.Encode(buf, asGif.Image[0], &jpeg.Options{Quality: 30})
			if err != nil {
				return err
			}
		}
		return cacheRawMedia(pCtx, buf.Bytes(), pRuntime.Config.GCloudTokenContentBucket, fmt.Sprintf("image-%s", name))
	} else if strings.HasPrefix(contentType, "video/") {
		// thumbnails the video, do we need to store a whole video?
		file, err := ioutil.TempFile("/tmp", "")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())

		_, err = file.Write(buf.Bytes())
		if err != nil {
			return err
		}

		scaled, err := scaleVideo(pCtx, file.Name(), 640, 480)
		if err != nil {
			return err
		}
		cacheRawMedia(pCtx, scaled, pRuntime.Config.GCloudTokenContentBucket, fmt.Sprintf("video-%s", name))

		jp, err := readVidFrameAsJpeg(file.Name(), 1)
		if err != nil {
			return err
		}
		return cacheRawMedia(pCtx, jp, pRuntime.Config.GCloudTokenContentBucket, fmt.Sprintf("image-%s", name))
	} else {
		return fmt.Errorf("Unsupported content type: %s", contentType)
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
