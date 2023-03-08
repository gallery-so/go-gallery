package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

type updateTokenMedia struct {
	TokenDBID persist.DBID
	Media     persist.Media
}

func main() {

	setDefaults()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Took %s", elapsed)
	}()
	ctx := context.Background()
	pg := postgres.NewPgxClient()
	stg := media.NewStorageClient(ctx)
	ffMu := &sync.Mutex{}

	rows, err := pg.Query(ctx, `select tokens.id, tokens.media, contracts.address, tokens.token_id from tokens join contracts on contracts.id = tokens.contract where tokens.deleted = false and tokens.media is not null and not tokens.media->>'media_type' = '' and not tokens.media->>'media_url' = '' and not tokens.media->>'media_type' = 'unknown' and not tokens.media->>'media_type' = 'invalid' order by tokens.last_updated desc;`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	results := make(chan updateTokenMedia)

	wp := workerpool.New(50)
	for rows.Next() {
		var tokenDBID persist.DBID
		var media persist.Media
		var contractAddress string
		var tokenID string
		err := rows.Scan(&tokenDBID, &media, &contractAddress, &tokenID)
		if err != nil {
			logger.For(nil).Errorf("failed to scan row: %s", err)
			continue
		}

		if media.Dimensions.Height != 0 && media.Dimensions.Width != 0 {
			logger.For(nil).Infof("skipping %s (%s %s-%s) as it already has dimensions", media.MediaURL, tokenDBID, contractAddress, tokenID)
			continue
		}

		if media.MediaURL == "" {
			logger.For(nil).Infof("skipping %s (%s %s-%s) as it has no media url", media.MediaURL, tokenDBID, contractAddress, tokenID)
			continue
		}

		if media.MediaType == "" || media.MediaType == persist.MediaTypeInvalid || media.MediaType == persist.MediaTypeUnknown {
			logger.For(nil).Infof("skipping %s (%s %s-%s) as it has an unsupported media type: %s", media.MediaURL, tokenDBID, contractAddress, tokenID, media.MediaType)
			continue
		}

		wp.Submit(func() {
			logger.For(nil).Infof("processing %s (%s %s-%s)", media.MediaURL, tokenDBID, contractAddress, tokenID)
			switch media.MediaType {
			case persist.MediaTypeSVG:
				dims, err := getSvgDimensions(ctx, media.MediaURL.String())
				if err != nil {
					logger.For(nil).Errorf("failed to get dimensions for %s: %s", media.MediaURL, err)
					return
				}
				media.Dimensions = dims
			case persist.MediaTypeHTML:
				dims, err := getHTMLDimensions(ctx, media.MediaURL.String())
				if err != nil {
					logger.For(nil).Errorf("failed to get dimensions for %s: %s", media.MediaURL, err)
					return
				}
				media.Dimensions = dims

			case persist.MediaTypeVideo:
				if strings.HasSuffix(media.MediaURL.String(), "https://storage.googleapis.com") {
					name := fmt.Sprintf("%s-%s", contractAddress, tokenID)
					func() {
						ffMu.Lock()
						defer ffMu.Unlock()
						err := createLiveRenderAndCache(ctx, media.MediaURL.String(), viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), name, stg)
						if err != nil {
							logger.For(nil).Errorf("failed to create live render for %s: %s", media.MediaURL, err)
						}
					}()
				}
				fallthrough
			default:
				dims, err := getMediaDimensions(media.MediaURL.String())
				if err != nil {
					logger.For(nil).Errorf("failed to get dimensions for %s: %s", media.MediaURL, err)
					return
				}
				media.Dimensions = dims
			}

			logger.For(nil).Infof("got dimensions for %s (%s %s-%s): %+v", media.MediaURL, tokenDBID, contractAddress, tokenID, media.Dimensions)

			results <- updateTokenMedia{
				TokenDBID: tokenDBID,
				Media:     media,
			}

			logger.For(nil).Infof("finished processing %s (%s %s-%s)", media.MediaURL, tokenDBID, contractAddress, tokenID)

		})
	}

	updateWp := workerpool.New(100)

	for result := range results {
		up := result
		updateWp.Submit(func() {
			if up.Media.Dimensions.Height == 0 || up.Media.Dimensions.Width == 0 {
				logger.For(nil).Errorf("not updating %s (%s) as it has no dimensions", up.Media.MediaURL, up.TokenDBID)
				return
			}
			logger.For(nil).Infof("updating %s (%s)", up.Media.MediaURL, up.TokenDBID)
			_, err := pg.Exec(ctx, `update tokens set media = $1 where id = $2;`, up.Media, up.TokenDBID)
			if err != nil {
				logger.For(nil).Errorf("failed to update token %s: %s", up.TokenDBID, err)
			}

			logger.For(nil).Infof("updated %s (%s)", up.Media.MediaURL, up.TokenDBID)
		})
	}

	wp.StopWait()
	updateWp.StopWait()
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "dev-token-content")

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("backend", fi)
		util.LoadEncryptedEnvFile(envFile)
	}
}

type dimensions struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

func getMediaDimensions(url string) (persist.Dimensions, error) {
	outBuf := &bytes.Buffer{}
	c := exec.Command("ffprobe", "-hide_banner", "-loglevel", "error", "-show_streams", url, "-print_format", "json")
	c.Stderr = os.Stderr
	c.Stdout = outBuf
	err := c.Run()
	if err != nil {
		return persist.Dimensions{}, err
	}

	var d dimensions
	err = json.Unmarshal(outBuf.Bytes(), &d)
	if err != nil {
		return persist.Dimensions{}, fmt.Errorf("failed to unmarshal ffprobe output: %w", err)
	}

	if len(d.Streams) == 0 {
		return persist.Dimensions{}, fmt.Errorf("no streams found in ffprobe output: %w", err)
	}

	dims := persist.Dimensions{
		Width:  d.Streams[0].Width,
		Height: d.Streams[0].Height,
	}

	logger.For(nil).Debugf("got dimensions %+v for %s", dims, url)
	return dims, nil
}

type svgDimensions struct {
	XMLName xml.Name `xml:"svg"`
	Width   string   `xml:"width,attr"`
	Height  string   `xml:"height,attr"`
	Viewbox string   `xml:"viewBox,attr"`
}

func getSvgDimensions(ctx context.Context, url string) (persist.Dimensions, error) {
	buf := &bytes.Buffer{}
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

func createLiveRenderAndCache(ctx context.Context, videoURL, bucket, name string, client *storage.Client) error {

	fileName := fmt.Sprintf("liverender-%s", name)
	logger.For(ctx).Infof("caching live render media for '%s'", fileName)

	timeBeforeCopy := time.Now()

	sw := newObjectWriter(ctx, client, bucket, fileName, "video/mp4")

	logger.For(ctx).Infof("creating live render for %s", videoURL)
	if err := createLiveRenderPreviewVideo(videoURL, sw); err != nil {
		return fmt.Errorf("could not write to bucket %s for '%s': %s", bucket, fileName, err)
	}

	if err := sw.Close(); err != nil {
		return err
	}

	logger.For(ctx).Infof("storage copy took %s", time.Since(timeBeforeCopy))

	return nil
}

func createLiveRenderPreviewVideo(videoURL string, writer io.Writer) error {
	c := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", videoURL, "-ss", "00:00:00.000", "-t", "00:00:05.000", "-filter:v", "scale=720:-1", "-movflags", "frag_keyframe+empty_moov", "-c:a", "copy", "-f", "mp4", "pipe:1", "</dev/null")
	c.Stderr = os.Stderr
	c.Stdout = writer
	return c.Run()
}

func newObjectWriter(ctx context.Context, client *storage.Client, bucket, fileName, contentType string) *storage.Writer {
	writer := client.Bucket(bucket).Object(fileName).NewWriter(ctx)
	writer.ObjectAttrs.ContentType = contentType
	writer.CacheControl = "no-cache, no-store"
	return writer
}
