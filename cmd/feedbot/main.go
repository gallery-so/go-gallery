package main

import (
	"net/http"

	"github.com/mikeydub/go-gallery/feedbot"
	"google.golang.org/appengine"
)

func main() {
	feedbot.Init()
	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		http.ListenAndServe(":4123", nil)
	}
}
