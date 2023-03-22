package dummymetadata

import (
	"github.com/gin-gonic/gin"
)

func handlersInitServer(router *gin.Engine) *gin.Engine {
	mediaGroup := router.Group("/media")
	mediaGroup.GET("/image", imageHandler)
	mediaGroup.GET("/video", videoHandler)
	mediaGroup.GET("/iframe", iframeHandler)
	mediaGroup.GET("/gif", gifHandler)
	mediaGroup.GET("/bad", badMediaHandler)
	mediaGroup.GET("/notfound", mediaNotFoundHandler)
	mediaGroup.GET("/svg", svgHandler)
	mediaGroup.GET("/base64svg", base64SvgHandler)
	mediaGroup.GET("/animation", animationHandler)
	mediaGroup.GET("/pdf", pdfHandler)
	mediaGroup.GET("/text", textHandler)
	mediaGroup.GET("/badimage", badImageHandler)

	metadataGroup := router.Group("/metadata")
	metadataGroup.GET("/image", imageMetadataHandler)
	metadataGroup.GET("/video", videoMetadataHandler)
	metadataGroup.GET("/iframe", iframeMetadataHandler)
	metadataGroup.GET("/gif", gifMetadataHandler)
	metadataGroup.GET("/bad", badMetadataHandler)
	metadataGroup.GET("/notfound", metadataNotFoundHandler)
	metadataGroup.GET("/media/bad", badMediaMetadataHandler)
	metadataGroup.GET("/media/notfound", mediaNotFoundMetadataHandler)
	metadataGroup.GET("/svg", svgMetadataHandler)
	metadataGroup.GET("/base64svg", base64SvgMetadataHandler)
	metadataGroup.GET("/base64", base64MetadataHandler)
	metadataGroup.GET("/media/ipfs", ipfsMediaMetadataHandler)
	metadataGroup.GET("/media/dnsbad", badDNSMediaMetadataHandler)
	metadataGroup.GET("/differentkeyword", differentKeywordMetadataHandler)
	metadataGroup.GET("/wrongkeyword", wrongKeywordMetadataHandler)
	metadataGroup.GET("/animation", animationMetadataHandler)
	metadataGroup.GET("/pdf", pdfMetadataHandler)
	metadataGroup.GET("/text", textMetadataHandler)
	metadataGroup.GET("/badimage", badImageMetadataHandler)

	return router
}
