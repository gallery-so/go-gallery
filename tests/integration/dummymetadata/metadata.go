package dummymetadata

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

func imageMetadataHandler(c *gin.Context) {

	mediaURL := fmt.Sprintf("%s/media/image", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}
	c.JSON(200, gin.H{
		"image_url": mediaURL,
	})
}

func videoMetadataHandler(c *gin.Context) {

	mediaURL := fmt.Sprintf("%s/media/image", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}
	c.JSON(200, gin.H{
		"video_url": mediaURL,
	})
}

func iframeMetadataHandler(c *gin.Context) {

	mediaURL := fmt.Sprintf("%s/media/iframe", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}
	c.JSON(200, gin.H{
		"animation_url": mediaURL,
	})
}

func gifMetadataHandler(c *gin.Context) {

	mediaURL := fmt.Sprintf("%s/media/gif", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}
	c.JSON(200, gin.H{
		"animation_url": mediaURL,
	})
}

func badMetadataHandler(c *gin.Context) {
	c.JSON(500, gin.H{
		"error": "bad metadata",
	})
}

func metadataNotFoundHandler(c *gin.Context) {
	c.JSON(404, gin.H{
		"error": "metadata not found",
	})
}

func badMediaMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/bad", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}
	c.JSON(200, gin.H{
		"image_url": mediaURL,
	})
}

func mediaNotFoundMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/notfound", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}
	c.JSON(200, gin.H{
		"image_url": mediaURL,
	})
}

func base64MetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/base64", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}
	asJSON := map[string]string{
		"image_url": mediaURL,
	}

	asBytes, err := json.Marshal(asJSON)
	if err != nil {
		panic(err)
	}

	asBase64 := base64.StdEncoding.EncodeToString(asBytes)

	c.Data(200, "application/octet-stream", []byte(asBase64))
}

func svgMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/svg", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}

	c.JSON(200, gin.H{
		"image_url": mediaURL,
	})
}

func base64SvgMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/base64svg", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}

	c.JSON(200, gin.H{
		"image_url": mediaURL,
	})
}

func ipfsMediaMetadataHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"image_url": "ipfs://QmPH5gEDMd78t83ZWniD5gNJSA9r4pa5Z7AXjKbJNWK8jU",
	})
}

func differentKeywordMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/image", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}

	c.JSON(200, gin.H{
		"image": mediaURL,
	})
}

func wrongKeywordMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/video", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}

	c.JSON(200, gin.H{
		"image_url": mediaURL,
	})
}

func badDNSMediaMetadataHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"image_url": "https://bad.domain.com/image.png",
	})
}

func animationMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/animation", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}

	c.JSON(200, gin.H{
		"animation_url": mediaURL,
	})
}

func pdfMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/pdf", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}

	c.JSON(200, gin.H{
		"animation_url": mediaURL,
	})
}

func textMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/text", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}

	c.JSON(200, gin.H{
		"animation_url": mediaURL,
	})
}

func badImageMetadataHandler(c *gin.Context) {
	mediaURL := fmt.Sprintf("%s/media/badimage", c.Request.Host)

	if !c.Request.URL.IsAbs() {
		mediaURL = fmt.Sprintf("%s://%s", c.Request.URL.Scheme, mediaURL)
	}

	c.JSON(200, gin.H{
		"image_url": mediaURL,
	})
}
