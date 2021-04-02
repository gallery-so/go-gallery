package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

import "github.com/mikeydub/go-gallery/internal/db"

type RequestHandlers struct {
	ctx     context.Context
	storage db.Storage
}

func NewRequestHandlers(ctx context.Context, storage db.Storage) *RequestHandlers {
	return &RequestHandlers{ctx: ctx, storage: storage}
}

// this is really handlers
func (s *RequestHandlers) NFTSForUser(w http.ResponseWriter, r *http.Request) {
	nfts, err := s.storage.GetNFTsByUserID(s.ctx, "7bfaafcc-722e-4dce-986f-fe0d9bee2047")
	if err != nil {
		// return 404
		panic(err)
	}
	payload, err := json.Marshal(nfts)
	if err != nil {
		// return empty array
		panic(err)
	}

	fmt.Fprint(w, string(payload))
}

func GenerateRouter(s *RequestHandlers) http.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/nfts", s.NFTSForUser)

	routerWithLogs := handlers.LoggingHandler(os.Stdout, router)
	routerWithCORS := handlers.CORS(
		// handlers.AllowedOrigins([]string{s.WebBaseURL, s.BaseURL}),
		handlers.AllowedMethods([]string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions}),
		handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"}),
	)(routerWithLogs)

	return routerWithCORS
}

func Run(ctx context.Context, storage db.Storage, port int) {
	handlers := NewRequestHandlers(ctx, storage)
	router := GenerateRouter(handlers)

	fmt.Println("Running server")
	log.Fatal(http.ListenAndServe(
		fmt.Sprintf(":%d", port),
		router,
	))
}
