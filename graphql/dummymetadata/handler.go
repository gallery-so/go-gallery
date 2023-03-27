package dummymetadata

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/util"
)

func handlersInitServer(router *gin.Engine) *gin.Engine {
	mediaGroup := router.Group("/media")
	mediaGroup.StaticFile("/image", util.MustFindFile("./static/test_image.png"))
	mediaGroup.StaticFile("/video", util.MustFindFile("./static/test_video.mp4"))
	mediaGroup.StaticFile("/iframe", util.MustFindFile("./static/test_iframe.html"))
	mediaGroup.StaticFile("/gif", util.MustFindFile("./static/test_gif.gif"))
	mediaGroup.GET("/bad", errorReply(http.StatusInternalServerError, "bad media"))
	mediaGroup.GET("/notfound", errorReply(http.StatusNotFound, "media not found"))
	mediaGroup.StaticFile("/svg", util.MustFindFile("./static/test_svg.svg"))
	mediaGroup.StaticFile("/animation", util.MustFindFile("./static/test_animation.glb"))
	mediaGroup.StaticFile("/pdf", util.MustFindFile("./static/test_pdf.pdf"))
	mediaGroup.GET("/text", func(c *gin.Context) { c.String(http.StatusOK, "I love Gallery") })
	mediaGroup.StaticFile("/badimage", util.MustFindFile("./static/test_bad_image.png"))

	metadataGroup := router.Group("/metadata")
	metadataGroup.GET("/image", mediaURL("image_url", "media/image"))
	metadataGroup.GET("/video", mediaURL("video_url", "media/video"))
	metadataGroup.GET("/iframe", mediaURL("animation_url", "media/iframe"))
	metadataGroup.GET("/gif", mediaURL("animation_url", "media/gif"))
	metadataGroup.GET("/bad", errorReply(http.StatusInternalServerError, "bad metadata"))
	metadataGroup.GET("/notfound", errorReply(http.StatusNotFound, "metadata not found"))
	metadataGroup.GET("/media/bad", mediaURL("image_url", "media/bad"))
	metadataGroup.GET("/media/notfound", mediaURL("image_url", "media/notfound"))
	metadataGroup.GET("/svg", mediaURL("image_url", "media/svg"))
	metadataGroup.GET("/base64svg", base64MetadataSVGHandler)
	metadataGroup.GET("/base64", base64MetadataHandler)
	metadataGroup.GET("/media/ipfs", replyJSON("image_url", "ipfs://QmPH5gEDMd78t83ZWniD5gNJSA9r4pa5Z7AXjKbJNWK8jU"))
	metadataGroup.GET("/media/dnsbad", replyJSON("image_url", "https://bad.domain.com/image.png"))
	metadataGroup.GET("/differentkeyword", mediaURL("image", "media/image"))
	metadataGroup.GET("/wrongkeyword", mediaURL("image_url", "media/video"))
	metadataGroup.GET("/animation", mediaURL("animation_url", "media/animation"))
	metadataGroup.GET("/pdf", mediaURL("animation_url", "media/pdf"))
	metadataGroup.GET("/text", mediaURL("animation_url", "media/text"))
	metadataGroup.GET("/badimage", mediaURL("image_url", "media/badimage"))

	return router
}

func errorReply(statusCode int, errorString string) func(c *gin.Context) {
	return func(c *gin.Context) {
		c.JSON(statusCode, gin.H{"error": errorString})
	}
}
