package main

import (
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/server"
	"github.com/sirupsen/logrus"
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
		logrus.Info("Running in App Engine Mode")
		appengine.Main()
	} else {
		logrus.Info("Running in Default Mode")
		http.ListenAndServe(":4000", nil)
	}
}
