package mediamapper

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/imgix/imgix-go/v2"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const contextKey = "mediamapper.instance"

const assetDomain = "assets.gallery.so"

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

func NewMediaMapper() *MediaMapper {
	token := viper.GetString("IMGIX_SECRET")
	if token == "" {
		panic(errors.New("IMGIX_SECRET must be set in order to generate image URLs"))
	}

	urlBuilder := imgix.NewURLBuilder(assetDomain, imgix.WithToken(token), imgix.WithLibParam(false))

	defaultAutoParam := imgix.Param("auto", "format", "compress")

	thumbnailUrlParams := []imgix.IxParam{
		imgix.Param("w", "64"),
		defaultAutoParam,
	}

	smallUrlParams := []imgix.IxParam{
		imgix.Param("w", "204"),
		defaultAutoParam,
	}

	mediumUrlParams := []imgix.IxParam{
		imgix.Param("w", "340"),
		defaultAutoParam,
	}

	largeUrlParams := []imgix.IxParam{
		imgix.Param("w", "1024"),
		defaultAutoParam,
	}

	srcSetParams := []imgix.IxParam{
		imgix.Param("w", "204"),
		defaultAutoParam,
	}

	return &MediaMapper{
		urlBuilder:         urlBuilder,
		thumbnailUrlParams: thumbnailUrlParams,
		smallUrlParams:     smallUrlParams,
		mediumUrlParams:    mediumUrlParams,
		largeUrlParams:     largeUrlParams,
		srcSetParams:       srcSetParams,
	}
}

func (u *MediaMapper) GetThumbnailImageUrl(sourceUrl string) string {
	return u.urlBuilder.CreateURL(sourceUrl, u.thumbnailUrlParams...)
}

func (u *MediaMapper) GetSmallImageUrl(sourceUrl string) string {
	return u.urlBuilder.CreateURL(sourceUrl, u.smallUrlParams...)
}

func (u *MediaMapper) GetMediumImageUrl(sourceUrl string) string {
	return u.urlBuilder.CreateURL(sourceUrl, u.mediumUrlParams...)
}

func (u *MediaMapper) GetLargeImageUrl(sourceUrl string) string {
	return u.urlBuilder.CreateURL(sourceUrl, u.largeUrlParams...)
}

func (u *MediaMapper) GetSrcSet(sourceUrl string) string {
	return u.urlBuilder.CreateSrcset(sourceUrl, u.srcSetParams)
}
