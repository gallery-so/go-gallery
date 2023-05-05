package main

import (
	"net/http"

	"github.com/mikeydub/go-gallery/admin"
	"github.com/sirupsen/logrus"
	"google.golang.org/appengine"
)

func main() {
	admin.Init()
	if appengine.IsAppEngine() {
		logrus.Info("Running in App Engine Mode")
		appengine.Main()
	} else {
		logrus.Info("Running in Default Mode")
		http.ListenAndServe(":4000", nil)
	}
}
