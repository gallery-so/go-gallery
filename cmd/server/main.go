package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mikeydub/go-gallery/service/logger"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"golang.org/x/sync/errgroup"

	"github.com/mikeydub/go-gallery/server"
	"google.golang.org/appengine"
)

func main() {
	defer sentryutil.RecoverAndRaise(nil)

	graceFullShutdown := make(chan os.Signal, 1)
	signal.Notify(graceFullShutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		server.Init()

		if appengine.IsAppEngine() {
			logger.For(nil).Info("Running in App Engine Mode")
			appengine.Main()
		} else {
			logger.For(nil).Info("Running in Default Mode")
			http.ListenAndServe(":4000", nil)
		}
	}()

	<-graceFullShutdown
	errGroup := new(errgroup.Group)
	for _, s := range server.Cleanuppers {
		errGroup.Go(s.Cleanup)
	}
	if err := errGroup.Wait(); err != nil {
		panic(err)
	}
}
