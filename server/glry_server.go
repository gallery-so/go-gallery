package server

import (
	// "fmt"
	// log "github.com/sirupsen/logrus"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_lib"
)

//-------------------------------------------------------------
func Init(pPortInt int,
	pRuntime *glry_core.Runtime) {

	pRuntime.Router = gin.Default()

	// HANDLERS
	glry_lib.HandlersInit(pRuntime)

	if err := pRuntime.Router.Run(fmt.Sprintf(":%d", pPortInt)); err != nil {
		panic(err)
	}

}
