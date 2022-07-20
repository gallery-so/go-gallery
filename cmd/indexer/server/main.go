package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/mikeydub/go-gallery/indexer"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"google.golang.org/appengine"
)

func main() {
	defer sentryutil.RecoverAndRaise(nil)

	indexer.InitServer()
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":6000", nil)
	}
}
