package publicapi

import (
	"github.com/gin-gonic/gin"
)

type NftWithDispatch struct {
	PublicNftAPI
	gc *gin.Context
}
