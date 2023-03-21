package dummymetadata

import (
	"encoding/base64"
	"os"

	"github.com/gin-gonic/gin"
)

func imageHandler(c *gin.Context) {
	c.File("./static/test_image.png")
}

func videoHandler(c *gin.Context) {
	c.File("./static/test_video.mp4")
}

func iframeHandler(c *gin.Context) {
	c.File("./static/test_iframe.html")
}

func gifHandler(c *gin.Context) {
	c.File("./static/test_gif.gif")
}

func badMediaHandler(c *gin.Context) {
	c.JSON(500, gin.H{
		"error": "bad media",
	})
}

func mediaNotFoundHandler(c *gin.Context) {
	c.JSON(404, gin.H{
		"error": "media not found",
	})
}

func svgHandler(c *gin.Context) {
	c.File("./static/test_svg.svg")
}

func base64SvgHandler(c *gin.Context) {

	bs, err := os.ReadFile("./static/test_svg.svg")
	if err != nil {
		panic(err)
	}

	c.Data(200, "application/octet-stream", []byte(base64.StdEncoding.EncodeToString(bs)))
}

func animationHandler(c *gin.Context) {
	c.File("./static/test_animation.glb")
}

func pdfHandler(c *gin.Context) {
	c.File("./static/test_pdf.pdf")
}

func textHandler(c *gin.Context) {
	c.String(200, "I love Gallery!")
}

func badImageHandler(c *gin.Context) {
	c.File("./static/test_bad_image.png")
}
