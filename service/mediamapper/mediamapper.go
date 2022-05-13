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

type MediaMapper struct {
	urlBuilder      imgix.URLBuilder
	smallUrlParams  []imgix.IxParam
	mediumUrlParams []imgix.IxParam
	largeUrlParams  []imgix.IxParam
	srcSetParams    []imgix.IxParam
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

	urlBuilder := imgix.NewURLBuilder("assets.gallery.so", imgix.WithToken(token), imgix.WithLibParam(false))

	// TODO: Decide on ideal parameters for all permutations below

	smallUrlParams := []imgix.IxParam{
		imgix.Param("w", "100"),
		imgix.Param("auto", "format", "compress"),
	}

	mediumUrlParams := []imgix.IxParam{
		imgix.Param("w", "500"),
		imgix.Param("auto", "format", "compress"),
	}

	largeUrlParams := []imgix.IxParam{
		imgix.Param("w", "1000"),
		imgix.Param("auto", "format", "compress"),
	}

	srcSetParams := []imgix.IxParam{
		imgix.Param("auto", "format", "compress"),
	}

	return &MediaMapper{
		urlBuilder:      urlBuilder,
		smallUrlParams:  smallUrlParams,
		mediumUrlParams: mediumUrlParams,
		largeUrlParams:  largeUrlParams,
		srcSetParams:    srcSetParams,
	}
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
