package main

import (
	"net/http"

	"github.com/mikeydub/go-gallery/server"
	"github.com/sirupsen/logrus"
	"google.golang.org/appengine"
)

func main() {
	if appengine.IsAppEngine() {
		logrus.Info("Running in App Engine Mode")
		appengine.Main()
	} else {
		logrus.Info("Running in Default Mode")
		server.Init()
		http.ListenAndServe(":4000", nil)
	}
}
