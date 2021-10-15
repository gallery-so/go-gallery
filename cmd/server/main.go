package main

import (
	"net/http"
	"os"
	"strings"

	_ "github.com/mikeydub/go-gallery/server"
	"github.com/sirupsen/logrus"
	"google.golang.org/appengine"
)

func main() {
	if strings.HasSuffix(os.Getenv("SERVER_SOFTWARE"), "Google App Engine/") {
		logrus.Info("Running in App Engine Mode")
		appengine.Main()
	} else {
		logrus.Info("Running in Default Mode")
		http.ListenAndServe(":8080", nil)
	}
}
