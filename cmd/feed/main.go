package main

import (
	"net/http"

	"github.com/mikeydub/go-gallery/feed"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"google.golang.org/appengine"
)

func main() {
	defer sentryutil.RecoverAndRaise(nil)

	feed.Init()
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":4124", nil)
	}
}
