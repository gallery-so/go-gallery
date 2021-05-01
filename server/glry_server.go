package server

import (
	"fmt"
	// log "github.com/sirupsen/logrus"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	gfrpclib "github.com/gloflow/gloflow/go/gf_rpc_lib"
	"github.com/mikeydub/go-gallery/db"
)

//-------------------------------------------------------------
func Init(pPortInt int,
	pDB         *db.DB, 
	pRuntimeSys *gfcore.Runtime_sys) {


	// HANDLERS
	initHandlers(pRuntimeSys)


	// SERVER_INIT
	gfrpclib.Server__init(fmt.Sprintf("%d", pPortInt))

	

	

}