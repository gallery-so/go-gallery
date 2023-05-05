package main

import (
	"net/http"

	"github.com/mikeydub/go-gallery/feedbot"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"google.golang.org/appengine"
)

func main() {
	defer sentryutil.RecoverAndRaise(nil)

	feedbot.Init()
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":4123", nil)
	}
}
