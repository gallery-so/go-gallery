package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mikeydub/go-gallery/service/logger"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"

	"github.com/mikeydub/go-gallery/server"
	"google.golang.org/appengine"
)

func main() {
	defer sentryutil.RecoverAndRaise(nil)

	graceFullShutdown := make(chan os.Signal, 1)
	signal.Notify(graceFullShutdown, os.Interrupt, syscall.SIGTERM)

	server.Init()
	if appengine.IsAppEngine() {
		logger.For(nil).Info("Running in App Engine Mode")
		appengine.Main()
	} else {
		logger.For(nil).Info("Running in Default Mode")
		http.ListenAndServe(":4000", nil)
	}

}
