package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/mikeydub/go-gallery.git/utils"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

func initializeRouter(g *galleryController) http.Handler {
	router := mux.NewRouter()

	router.HandleFunc("/alive", handleHealthcheck)
	router.HandleFunc("/login", g.handleLogin).Methods("GET")

	routerWithLogs := handlers.LoggingHandler(os.Stdout, router)
	routerWithCORS := handlers.CORS(
		handlers.AllowedOrigins([]string{g.appConfig.WebBaseURL, g.appConfig.BaseURL}),
		handlers.AllowedMethods([]string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions}),
		handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"}),
	)(routerWithLogs)

	return routerWithCORS
}

func handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	fmt.Println("💙")
	utils.RespondWithBody(w, "💙")
}