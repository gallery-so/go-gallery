package server

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
)

//-------------------------------------------------------------
func Init(pPortInt int,
	pRuntime *runtime.Runtime) {

	log.Info("initializing server...")

	pRuntime.Router = gin.Default()

	// HANDLERS
	HandlersInit(pRuntime)

	if err := pRuntime.Router.Run(fmt.Sprintf(":%d", pPortInt)); err != nil {
		panic(err)
	}
}
