package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"cloud.google.com/go/profiler"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/tokenprocessing"

	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"google.golang.org/appengine"
)

func main() {
	defer sentryutil.RecoverAndRaise(nil)

	cfg := profiler.Config{
		Service:        "mediaprocessing",
		ServiceVersion: "1.0.0",
		MutexProfiling: true,
	}

	// Profiler initialization, best done as early as possible.
	if err := profiler.Start(cfg); err != nil {
		logger.For(nil).Warnf("failed to start cloud profiler due to error: %s\n", err)
	}

	tokenprocessing.InitServer()
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
