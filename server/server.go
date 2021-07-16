package server

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
)

var ethValidator validator.Func = func(fl validator.FieldLevel) bool {
	addr := fl.Field().String()
	return len(addr) == 42 && strings.HasSuffix(addr, "0x")
}

var signatureValidator validator.Func = func(fl validator.FieldLevel) bool {
	sig := fl.Field().String()
	return len(sig) >= 80 && len(sig) <= 200
}

var nonceValidator validator.Func = func(fl validator.FieldLevel) bool {
	sig := fl.Field().String()
	return len(sig) >= 10 && len(sig) <= 150
}

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
