package main

import (
	"github.com/mikeydub/go-gallery.git/utils"
	"net/http"
	"fmt"
)

func main() {
	appConfig := getApplicationConfig()

	httpClient := utils.CreateHTTPClient()

	gc := newGalleryController(
		appConfig,
		httpClient,
	)

	router := initializeRouter(gc)

	http.ListenAndServe(
		fmt.Sprintf(":%s", appConfig.Port),
		router,
	)
}
