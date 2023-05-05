package mediamapper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/imgix/imgix-go/v2"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
)

const contextKey = "mediamapper.instance"

const assetDomain = "assets.gallery.so"

const (
	thumbnailWidth = 64
	smallWidth     = 204
	mediumWidth    = 340
	largeWidth     = 1024
)

type MediaMapper struct {
	urlBuilder         imgix.URLBuilder
	thumbnailUrlParams []imgix.IxParam
	smallUrlParams     []imgix.IxParam
	mediumUrlParams    []imgix.IxParam
	largeUrlParams     []imgix.IxParam
	srcSetParams       []imgix.IxParam
}

func AddTo(c *gin.Context) {
	c.Set(contextKey, NewMediaMapper())
}

func For(ctx context.Context) *MediaMapper {
	gc := util.MustGetGinContext(ctx)
	return gc.Value(contextKey).(*MediaMapper)
}

func buildParams(defaults []imgix.IxParam, other ...imgix.IxParam) []imgix.IxParam {
	var output []imgix.IxParam

	for _, p := range defaults {
		output = append(output, p)
	}

	for _, p := range other {
		output = append(output, p)
	}

	return output
}

func newWidthParam(width int) imgix.IxParam {
	return imgix.Param("w", strconv.Itoa(width))
}

func NewMediaMapper() *MediaMapper {
	token := env.GetString("IMGIX_SECRET")
	if token == "" {
		// panic(errors.New("IMGIX_SECRET must be set in order to generate image URLs"))
		logger.For(nil).Error("IMGIX_SECRET must be set in order to generate image URLs")
		return nil
	}

	urlBuilder := imgix.NewURLBuilder(assetDomain, imgix.WithToken(token), imgix.WithLibParam(false))

	thumbnailUrlParams := buildParams(getDefaultParams(), newWidthParam(thumbnailWidth))
	smallUrlParams := buildParams(getDefaultParams(), newWidthParam(smallWidth))
	mediumUrlParams := buildParams(getDefaultParams(), newWidthParam(mediumWidth))
	largeUrlParams := buildParams(getDefaultParams(), newWidthParam(largeWidth))
	srcSetParams := buildParams(getDefaultParams(), newWidthParam(largeWidth))

	return &MediaMapper{
		urlBuilder:         urlBuilder,
		thumbnailUrlParams: thumbnailUrlParams,
		smallUrlParams:     smallUrlParams,
		mediumUrlParams:    mediumUrlParams,
		largeUrlParams:     largeUrlParams,
		srcSetParams:       srcSetParams,
	}
}

// googleusercontent URLs appear to return a fairly low resolution image if no size parameters are
// appended to the URL, which means we might end up trying upscale a low-resolution image. To fix
// that, we check for googleusercontent URLs and append our target width as a parameter.
func setGoogleWidthParams(sourceUrl string, width int) string {
	if strings.HasPrefix(sourceUrl, "https://lh3.googleusercontent.com/") {
		return fmt.Sprintf("%s=w%d", sourceUrl, width)
	}

	return sourceUrl
}

func getDefaultParams() []imgix.IxParam {
	return []imgix.IxParam{
		imgix.Param("auto", "format", "compress"),
		imgix.Param("fit", "max"),
	}
}

func (u *MediaMapper) buildPreviewImageUrl(sourceUrl string, width int, params []imgix.IxParam, options ...Option) string {
	if sourceUrl == "" {
		return sourceUrl
	}

	sourceUrl = setGoogleWidthParams(sourceUrl, width)
	params = applyOptions(params, options)

	return u.urlBuilder.CreateURL(sourceUrl, params...)
}

func (u *MediaMapper) buildSrcSet(sourceUrl string, params []imgix.IxParam, options ...Option) string {
	if sourceUrl == "" {
		return sourceUrl
	}

	params = applyOptions(params, options)

	return u.urlBuilder.CreateSrcset(sourceUrl, params)
}

func applyOptions(params []imgix.IxParam, options []Option) []imgix.IxParam {
	// Options will append to the params slice, so we need to make a copy of it
	// to ensure we're not modifying a shared set of params
	if len(options) > 0 {
		p := make([]imgix.IxParam, len(params))
		copy(p, params)
		params = p

		for _, option := range options {
			option(&params)
		}
	}

	return params
}

type Option func(*[]imgix.IxParam)

func WithStaticImage() Option {
	return func(params *[]imgix.IxParam) {
		*params = append(*params, imgix.Param("frame", "1"))
	}
}

func (u *MediaMapper) GetThumbnailImageUrl(sourceUrl string, options ...Option) string {
	return u.buildPreviewImageUrl(sourceUrl, thumbnailWidth, u.thumbnailUrlParams, options...)
}

func (u *MediaMapper) GetSmallImageUrl(sourceUrl string, options ...Option) string {
	return u.buildPreviewImageUrl(sourceUrl, smallWidth, u.smallUrlParams, options...)
}

func (u *MediaMapper) GetMediumImageUrl(sourceUrl string, options ...Option) string {
	return u.buildPreviewImageUrl(sourceUrl, mediumWidth, u.mediumUrlParams, options...)
}

func (u *MediaMapper) GetLargeImageUrl(sourceUrl string, options ...Option) string {
	return u.buildPreviewImageUrl(sourceUrl, largeWidth, u.largeUrlParams, options...)
}

func (u *MediaMapper) GetSrcSet(sourceUrl string, options ...Option) string {
	return u.buildSrcSet(sourceUrl, u.srcSetParams, options...)
}

func (u *MediaMapper) GetBlurhash(sourceUrl string) *string {
	url := u.urlBuilder.CreateURL(sourceUrl, imgix.Param("fm", "blurhash"))

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, bytes.NewBuffer([]byte{}))
	req.Header.Set("Accept", "*/*")

	if err != nil {
		return nil
	}

	response, err := http.DefaultClient.Do(req)

	if err != nil {
		return nil
	}

	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil
	}

	responseBytes, _ := io.ReadAll(response.Body)
	responseString := string(responseBytes)

	return &responseString
}

func (u *MediaMapper) GetAspectRatio(sourceUrl string) *float64 {
	url := u.urlBuilder.CreateURL(sourceUrl, buildParams(getDefaultParams(), imgix.Param("fm", "json"))...)

	rawResponse, err := http.Get(url)

	if err != nil {
		return nil
	}

	if rawResponse.StatusCode != 200 {
		return nil
	}

	type ImgixJsonResponse struct {
		PixelWidth  float64
		PixelHeight float64
	}

	var imgixJsonResponse ImgixJsonResponse
	rseponseBytes, err := io.ReadAll(rawResponse.Body)

	json.Unmarshal(rseponseBytes, &imgixJsonResponse)

	// Make sure we handle the case where there is no image to make sure we don't get infinity
	if imgixJsonResponse.PixelHeight == 0 || imgixJsonResponse.PixelWidth == 0 {
		return nil
	}

	aspectRatio := imgixJsonResponse.PixelWidth / imgixJsonResponse.PixelHeight
	return &aspectRatio
}

func PurgeImage(ctx context.Context, u string) error {
	// '{ "data": { "attributes": { "url": "<url-to-purge>" }, "type": "purges" } }'
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"attributes": map[string]interface{}{
				"url": fmt.Sprintf("https://%s/%s", assetDomain, url.QueryEscape(u)),
			},
			"type": "purges",
		},
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.imgix.com/api/v1/purge", bytes.NewBuffer(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Authorization", "Bearer "+env.GetString("IMGIX_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected response status code: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
