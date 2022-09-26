package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

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
		port := "6500"
		if it := os.Getenv("PORT"); it != "" {
			port = it
		}
		http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	}
}
