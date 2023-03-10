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
	"time"

	"github.com/gammazero/workerpool"
	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
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

	var totalTokenCount int

	err := pg.QueryRow(ctx, `select count(*) from tokens where tokens.deleted = false and tokens.media is not null and not tokens.media->>'media_type' = '' and not tokens.media->>'media_url' = '' and not tokens.media->>'media_type' = 'unknown' and not tokens.media->>'media_type' = 'invalid';`).Scan(&totalTokenCount)
	if err != nil {
		logrus.Errorf("error getting total token count: %v", err)
		panic(err)
	}

	var limit int
	var offset int

	var rows pgx.Rows

	if viper.GetString("CLOUD_RUN_JOB") != "" {
		logrus.Infof("running as cloud run job")

		jobIndex := viper.GetInt("CLOUD_RUN_TASK_INDEX")
		jobCount := viper.GetInt("CLOUD_RUN_TASK_COUNT")

		// given the totalTokenCount, and the jobCount, we can calculate the offset and limit for this job
		// we want to evenly distribute the work across the jobs
		// so we can calculate the limit by dividing the totalTokenCount by the jobCount
		// and the offset by multiplying the jobIndex by the limit

		limit = totalTokenCount / jobCount
		offset = jobIndex * limit

		logrus.Infof("jobIndex: %d, jobCount: %d, totalTokenCount: %d, limit: %d, offset: %d", jobIndex, jobCount, totalTokenCount, limit, offset)

		rows, err = pg.Query(ctx, `select tokens.id, tokens.media from tokens join contracts on contracts.id = tokens.contract where tokens.deleted = false and tokens.media is not null and not tokens.media->>'media_type' = '' and not tokens.media->>'media_url' = '' and not tokens.media->>'media_type' = 'unknown' and not tokens.media->>'media_type' = 'invalid' order by tokens.last_updated desc limit $1 offset $2;`, limit, offset)
	} else {
		logrus.Infof("running as local job")
		limit = 1000
		offset = 120000
		rows, err = pg.Query(ctx, `select tokens.id, tokens.media from tokens join contracts on contracts.id = tokens.contract where tokens.deleted = false and tokens.media is not null and not tokens.media->>'media_type' = '' and not tokens.media->>'media_url' = '' and not tokens.media->>'media_type' = 'unknown' and not tokens.media->>'media_type' = 'invalid' order by tokens.last_updated desc limit $1 offset $2;`, limit, offset)
	}

	logrus.Info("querying for tokens...")

	if err != nil {
		logrus.Errorf("error getting tokens: %v", err)
		panic(err)
	}
	defer rows.Close()

	results := make(chan updateTokenMedia)

	wp := workerpool.New(100)

	logrus.Infof("processing (%d) tokens...", totalTokenCount)

	totalTokens := 0

	for ; rows.Next(); totalTokens++ {

		var tokenDBID persist.DBID
		var media persist.Media

		err := rows.Scan(&tokenDBID, &media)
		if err != nil {
			logrus.Errorf("failed to scan row: %s", err)
			continue
		}

		logrus.Infof("found %s (%s)", media.MediaURL, tokenDBID)

		if media.Dimensions.Height != 0 && media.Dimensions.Width != 0 {
			logrus.Infof("skipping %s (%s) as it already has dimensions", media.MediaURL, tokenDBID)
			continue
		}

		if media.MediaURL == "" {
			logrus.Infof("skipping %s (%s) as it has no media url", media.MediaURL, tokenDBID)
			continue
		}

		if media.MediaType == "" || media.MediaType == persist.MediaTypeInvalid || media.MediaType == persist.MediaTypeUnknown {
			logrus.Infof("skipping %s (%s) as it has an unsupported media type: %s", media.MediaURL, tokenDBID, media.MediaType)
			continue
		}

		wp.Submit(func() {

			logrus.Infof("processing %s (%s)", media.MediaURL, tokenDBID)
			switch media.MediaType {
			case persist.MediaTypeSVG:
				dims, err := getSvgDimensions(ctx, media.MediaURL.String())
				if err != nil {
					logrus.Errorf("failed to get dimensions for %s: %s", media.MediaURL, err)
					return
				}
				media.Dimensions = dims
			case persist.MediaTypeHTML:
				dims, err := getHTMLDimensions(ctx, media.MediaURL.String())
				if err != nil {
					logrus.Errorf("failed to get dimensions for %s: %s", media.MediaURL, err)
					return
				}
				media.Dimensions = dims

			default:

				func() {
					timeoutContext, cancel := context.WithTimeout(ctx, 30*time.Second)
					defer cancel()
					dims, err := getMediaDimensions(timeoutContext, media.MediaURL.String())
					if err != nil {
						logrus.Errorf("failed to get dimensions for %s: %s", media.MediaURL, err)
						return
					}
					media.Dimensions = dims
				}()
			}

			logrus.Infof("got dimensions for %s (%s): %+v", media.MediaURL, tokenDBID, media.Dimensions)

			results <- updateTokenMedia{
				TokenDBID: tokenDBID,
				Media:     media,
			}

			logrus.Infof("finished processing %s (%s)", media.MediaURL, tokenDBID)

		})
	}

	logrus.Infof("finished kicking off processes for %d tokens", totalTokens)

	go func() {
		defer close(results)
		wp.StopWait()
		logrus.Info("finished processing all tokens")
	}()

	logrus.Info("updating tokens...")

	allResults := make([]updateTokenMedia, 0, limit)
	for result := range results {
		up := result

		if up.Media.Dimensions.Height == 0 || up.Media.Dimensions.Width == 0 {
			logrus.Errorf("not updating %s (%s) as it has no dimensions", up.Media.MediaURL, up.TokenDBID)
			continue
		}
		logrus.Infof("updating %s (%s)", up.Media.MediaURL, up.TokenDBID)

		allResults = append(allResults, up)
	}

	logrus.Infof("preparing to update %d tokens with dimension data", len(allResults))

	allIDs, _ := util.Map(allResults, func(up updateTokenMedia) (persist.DBID, error) {
		return up.TokenDBID, nil
	})

	allMedias, _ := util.Map(allResults, func(up updateTokenMedia) (persist.Media, error) {
		return up.Media, nil
	})

	_, err = pg.Exec(ctx, `update tokens set media = t.media from (select unnest($1::varchar[]) as id, unnest($2::jsonb[]) as media) as t where tokens.id = t.id;`, allIDs, allMedias)
	if err != nil {
		logrus.Errorf("failed to update tokens: %s", err)
		panic(err)
	}

	logrus.Infof("finished backfilling tokens")
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "dev-token-content")
	viper.SetDefault("CLOUD_RUN_JOB", "")
	viper.SetDefault("CLOUD_RUN_TASK_INDEX", 0)
	viper.SetDefault("CLOUD_RUN_TASK_COUNT", 1)

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" {
		logrus.Info("running in non-local environment, skipping environment configuration")
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

func getMediaDimensions(ctx context.Context, url string) (persist.Dimensions, error) {
	outBuf := &bytes.Buffer{}
	c := exec.CommandContext(ctx, "ffprobe", "-hide_banner", "-loglevel", "error", "-show_streams", url, "-print_format", "json")
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

	logrus.Debugf("got dimensions %+v for %s", dims, url)
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
