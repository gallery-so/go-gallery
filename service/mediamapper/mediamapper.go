package mediamapper

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/imgix/imgix-go/v2"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
	"strconv"
	"strings"
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
	gc := util.GinContextFromContext(ctx)
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
	token := viper.GetString("IMGIX_SECRET")
	if token == "" {
		panic(errors.New("IMGIX_SECRET must be set in order to generate image URLs"))
	}

	urlBuilder := imgix.NewURLBuilder(assetDomain, imgix.WithToken(token), imgix.WithLibParam(false))

	defaultParams := []imgix.IxParam{
		imgix.Param("auto", "format", "compress"),
		imgix.Param("fit", "max"),
	}

	thumbnailUrlParams := buildParams(defaultParams, newWidthParam(thumbnailWidth))
	smallUrlParams := buildParams(defaultParams, newWidthParam(smallWidth))
	mediumUrlParams := buildParams(defaultParams, newWidthParam(mediumWidth))
	largeUrlParams := buildParams(defaultParams, newWidthParam(largeWidth))
	srcSetParams := buildParams(defaultParams, newWidthParam(largeWidth))

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

func (u *MediaMapper) buildPreviewImageUrl(sourceUrl string, width int, params []imgix.IxParam) string {
	if sourceUrl == "" {
		return sourceUrl
	}

	sourceUrl = setGoogleWidthParams(sourceUrl, width)
	return u.urlBuilder.CreateURL(sourceUrl, params...)
}

func (u *MediaMapper) GetThumbnailImageUrl(sourceUrl string) string {
	return u.buildPreviewImageUrl(sourceUrl, thumbnailWidth, u.thumbnailUrlParams)
}

func (u *MediaMapper) GetSmallImageUrl(sourceUrl string) string {
	return u.buildPreviewImageUrl(sourceUrl, smallWidth, u.smallUrlParams)
}

func (u *MediaMapper) GetMediumImageUrl(sourceUrl string) string {
	return u.buildPreviewImageUrl(sourceUrl, mediumWidth, u.mediumUrlParams)
}

func (u *MediaMapper) GetLargeImageUrl(sourceUrl string) string {
	return u.buildPreviewImageUrl(sourceUrl, largeWidth, u.largeUrlParams)
}

func (u *MediaMapper) GetSrcSet(sourceUrl string) string {
	return u.urlBuilder.CreateSrcset(sourceUrl, u.srcSetParams)
}
