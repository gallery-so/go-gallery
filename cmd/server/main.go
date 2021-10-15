package main

import (
	"net/http"
	"os"

	_ "github.com/mikeydub/go-gallery/server"
	"github.com/sirupsen/logrus"
	"google.golang.org/appengine"
)

func main() {
	if os.Getenv("SERVER_SOFTWARE") != "" {
		logrus.Info("Running in App Engine Mode")
		appengine.Main()
	} else {
		logrus.Info("Running in Default Mode")
		http.ListenAndServe(":4000", nil)
	}
}
