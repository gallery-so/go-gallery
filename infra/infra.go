package infra

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
)

// CoreInit initializes core server functionality. This is abstracted
// so the test server can also utilize it
func CoreInit(pRuntime *runtime.Runtime) *gin.Engine {
	log.Info("initializing server...")

	pRuntime.Router = gin.Default()
	pRuntime.Router.Use(handleCORS(pRuntime.Config))

	// if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
	// 	log.Info("registering validation")

	// }

	return handlersInit(pRuntime)
}

// Init initializes the server
func Init(pPortInt int,
	pRuntime *runtime.Runtime) {

	CoreInit(pRuntime)

	if err := pRuntime.Router.Run(fmt.Sprintf(":%d", pPortInt)); err != nil {
		panic(err)
	}
}
