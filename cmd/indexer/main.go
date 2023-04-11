package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/indexer/cmd"
)

func main() {
	if os.Getenv("K_SERVICE") != "" {
		indexer.SetDefaults()
		indexer.ValidateEnv()
		indexer.Init(nil, nil, false, true)
		fmt.Println("Running in Default Mode on port 4000")
		http.ListenAndServe(":4000", nil)
	} else {
		cmd.Execute()
	}
}
