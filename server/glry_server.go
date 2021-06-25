package server

import (
	// "fmt"
	// log "github.com/sirupsen/logrus"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	gf_rpc_lib "github.com/gloflow/gloflow/go/gf_rpc_lib"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_lib"
)

//-------------------------------------------------------------
func Init(pPortInt int,
	pRuntime *glry_core.Runtime) {

	// HANDLERS
	glry_lib.HandlersInit(pRuntime)

	// SERVER_INIT
	gf_rpc_lib.Server__init(pPortInt)

}
