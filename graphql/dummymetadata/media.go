package dummymetadata

import (
	"encoding/base64"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/util"
)

func base64SvgHandler(c *gin.Context) {
	bs, err := os.ReadFile(util.MustFindFile("./static/test_svg.svg"))
	if err != nil {
		panic(err)
	}

	c.Data(200, "application/octet-stream", []byte(base64.StdEncoding.EncodeToString(bs)))
}
