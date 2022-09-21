package main

import (
	"net/http"
	_ "net/http/pprof"

	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/tokenprocessing"
	"google.golang.org/appengine"
)

func main() {
	defer sentryutil.RecoverAndRaise(nil)

	tokenprocessing.InitServer()
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":6500", nil)
	}
}
