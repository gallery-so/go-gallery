package main

import (
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/feedbot"
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

	feedbot.Init()
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":4123", nil)
	}
}
