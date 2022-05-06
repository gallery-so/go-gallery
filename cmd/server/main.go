package main

import (
	"github.com/mikeydub/go-gallery/service/logger"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/server"
	"google.golang.org/appengine"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			sentry.CurrentHub().Recover(err)
			sentry.Flush(2 * time.Second)

			// Re-raise error
			panic(err)
		}
	}()

	server.Init()

	if appengine.IsAppEngine() {
		logger.For(nil).Info("Running in App Engine Mode")
		appengine.Main()
	} else {
		logger.For(nil).Info("Running in Default Mode")
		http.ListenAndServe(":4000", nil)
	}
}
