package server

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
)

// CoreInit initializes core server functionality. This is abstracted
// so the test server can also utilize it
func CoreInit(pRuntime *runtime.Runtime) *gin.Engine {
	log.Info("initializing server...")

	pRuntime.Router = gin.Default()
	pRuntime.Router.Use(handleCORS(pRuntime.Config))

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		log.Info("registering validation")
		v.RegisterValidation("short_string", shortStringValidator)
		v.RegisterValidation("medium_string", mediumStringValidator)
		v.RegisterValidation("eth_addr", ethValidator)
		v.RegisterValidation("nonce", nonceValidator)
		v.RegisterValidation("signature", signatureValidator)
		v.RegisterValidation("username", usernameValidator)
	}

	return handlersInit(pRuntime)
}

func Init(pPortInt int,
	pRuntime *runtime.Runtime) {

	CoreInit(pRuntime)

	if err := pRuntime.Router.Run(fmt.Sprintf(":%d", pPortInt)); err != nil {
		panic(err)
	}
}
