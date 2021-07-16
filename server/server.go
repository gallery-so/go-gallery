package server

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
)

var shortStringValidator validator.Func = func(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	return len(s) > 4 && len(s) < 50
}

var mediumStringValidator validator.Func = func(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	return len(s) < 500
}

//-------------------------------------------------------------
func Init(pPortInt int,
	pRuntime *runtime.Runtime) {

	log.Info("initializing server...")

	pRuntime.Router = gin.Default()

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterValidation("short_string", shortStringValidator)
		v.RegisterValidation("medium_string", mediumStringValidator)
	}

	// HANDLERS
	HandlersInit(pRuntime)

	if err := pRuntime.Router.Run(fmt.Sprintf(":%d", pPortInt)); err != nil {
		panic(err)
	}
}
