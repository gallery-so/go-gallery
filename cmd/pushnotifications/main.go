package main

import (
	"fmt"
	"github.com/mikeydub/go-gallery/pushnotifications"
	"net/http"
	_ "net/http/pprof"
	"os"

	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
)

func main() {
	defer sentryutil.RecoverAndRaise(nil)

	pushnotifications.InitServer()
	port := "6600"
	if it := os.Getenv("PORT"); it != "" {
		port = it
	}
	http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}
