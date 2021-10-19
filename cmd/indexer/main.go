package main

import (
	"net/http"

	_ "github.com/mikeydub/go-gallery/indexer"
	"google.golang.org/appengine"
)

func main() {
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":4000", nil)
	}
}
