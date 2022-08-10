package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/mikeydub/go-gallery/mediaprocessing"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"google.golang.org/appengine"
)

func main() {
	defer sentryutil.RecoverAndRaise(nil)

	mediaprocessing.InitServer()
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":6500", nil)
	}
}
