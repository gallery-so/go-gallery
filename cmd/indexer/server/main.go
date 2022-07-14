package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/mikeydub/go-gallery/indexer"
	"google.golang.org/appengine"
)

func main() {
	indexer.InitServer()
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":6000", nil)
	}
}
