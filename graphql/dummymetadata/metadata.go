package dummymetadata

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func responseProto(c *gin.Context) string {
	if c.Request.TLS != nil {
		return "https"
	}
	return "http"
}

func replyJSON(key, url string) func(c *gin.Context) {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{key: url})
	}
}

func formatURL(c *gin.Context, endpoint string) string {
	proto := responseProto(c)
	return fmt.Sprintf("%s://%s/%s", proto, c.Request.Host, endpoint)
}

func mediaURL(key, endpoint string) func(c *gin.Context) {
	return func(c *gin.Context) { replyJSON(key, formatURL(c, endpoint))(c) }
}

func base64MetadataHandler(c *gin.Context) {
	asJSON := map[string]string{"image_url": formatURL(c, "media/image")}

	asBytes, err := json.Marshal(asJSON)
	if err != nil {
		panic(err)
	}

	asBase64 := base64.StdEncoding.EncodeToString(asBytes)
	c.Data(http.StatusOK, "application/octet-stream", []byte("data:application/json;base64,"+asBase64))
}
