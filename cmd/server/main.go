package main

import (
	"net/http"
	"os"
	"strings"

	_ "github.com/mikeydub/go-gallery/server"
	"google.golang.org/appengine"
)

func main() {
	if strings.HasSuffix(os.Getenv("SERVER_SOFTWARE"), "Google App Engine/") {
		appengine.Main()
	} else {
		http.ListenAndServe(":8080", nil)
	}
}
