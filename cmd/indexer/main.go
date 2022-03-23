package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/mikeydub/go-gallery/indexer"
	"google.golang.org/appengine"
)

func main() {
	indexer.Init()
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":4000", nil)
	}
}
