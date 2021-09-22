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
	"gitlab.com/opennota/screengen"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/image"
)

func processImagesForToken(pCtx context.Context, contractAddress, tokenID string, pRuntime *runtime.Runtime) error {
	tokens, err := persist.TokenGetByTokenID(pCtx, tokenID, contractAddress, pRuntime)
	if err != nil {
		return err
	}
	if len(tokens) > 0 {
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

	res, err := downloadFromURL(pCtx, url)
	if err != nil {
		return err
	}
	err = cacheRawImage(pCtx, res, pRuntime.Config.GCloudTokenContentBucket, name)
	if err != nil {
		return err
	}

	servingURL, err := getImageServingURL(pCtx, pRuntime.Config.GCloudTokenContentBucket, name)
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{"token": token.ID, "servingURL": servingURL}).Info("processImagesForToken")

	update := &persist.TokenUpdateImageURLsInput{
		ThumbnailURL: servingURL,
		PreviewURL:   servingURL,
	}

	return persist.TokenUpdateByID(pCtx, token.ID, update, pRuntime)
}

func cacheRawImage(pCtx context.Context, image []byte, bucket, fileName string) error {

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

func getImageServingURL(pCtx context.Context, bucketID, objectID string) (string, error) {
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

func downloadFromURL(pCtx context.Context, url string) ([]byte, error) {

	httpClient := &http.Client{
		Timeout: time.Second * 2,
	}

	req, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer req.Body.Close()

	buf := &bytes.Buffer{}
	_, err = io.Copy(buf, req.Body)
	if err != nil {
		return nil, err
	}

	contentType := http.DetectContentType(buf.Bytes()[:512])
	if strings.HasPrefix(contentType, "image/") {
		if strings.HasPrefix(contentType, "image/gif") {
			// thumbnails a gif, do we need to store the whole gif?
			asGif, err := gif.DecodeAll(bytes.NewReader(buf.Bytes()))
			if err != nil {
				return nil, err
			}
			buf = &bytes.Buffer{}
			err = jpeg.Encode(buf, asGif.Image[0], &jpeg.Options{Quality: 30})
			if err != nil {
				return nil, err
			}
		}
		return buf.Bytes(), nil
	} else if strings.HasPrefix(contentType, "video/") {
		// thumbnails the video, do we need to store a whole video?
		file, err := ioutil.TempFile("/tmp", "")
		if err != nil {
			return nil, err
		}
		defer os.Remove(file.Name())

		_, err = file.Write(buf.Bytes())
		if err != nil {
			return nil, err
		}

		gen, err := screengen.NewGenerator(file.Name())
		if err != nil {
			return nil, err
		}
		defer gen.Close()

		img, err := gen.ImageWxH(gen.Duration/2, 256, 256)
		if err != nil {
			return nil, err
		}
		buf := &bytes.Buffer{}
		err = jpeg.Encode(buf, img, &jpeg.Options{Quality: 30})
		return buf.Bytes(), nil
	} else {
		return nil, fmt.Errorf("Unsupported content type: %s", contentType)
	}
}
