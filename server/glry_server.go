package server

import (
	// "fmt"
	// log "github.com/sirupsen/logrus"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	gfrpclib "github.com/gloflow/gloflow/go/gf_rpc_lib"
	"github.com/mikeydub/go-gallery/core"
)

//-------------------------------------------------------------
func Init(pPortInt int,
	pRuntime *core.Runtime) {

	// HANDLERS
	initHandlers(pRuntime)
	



	// SERVER_INIT
	gfrpclib.Server__init(pPortInt)
	
	


}