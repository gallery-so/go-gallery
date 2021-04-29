package main

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/config"
	"github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/server"
)

//-------------------------------------------------------------
func main() {
	ctx := context.Background()
	cfg := config.LoadConfig()

	storage, err := db.NewDB(ctx, cfg.PostgresURI)
	if err != nil {
		fmt.Printf("Error acquiring database connection: %v\n", err)
		panic(err)
	}

	server.Run(ctx, storage, cfg.Port)
}
