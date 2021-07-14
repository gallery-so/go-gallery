package server

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_lib"
	log "github.com/sirupsen/logrus"
)

//-------------------------------------------------------------
func Init(pPortInt int,
	pRuntime *glry_core.Runtime) {

	log.Info("initializing server...")

	pRuntime.Router = gin.Default()

	// HANDLERS
	glry_lib.HandlersInit(pRuntime)

	if err := pRuntime.Router.Run(fmt.Sprintf(":%d", pPortInt)); err != nil {
		panic(err)
	}
}
