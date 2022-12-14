package main

import (
	_ "net/http/pprof"

	"github.com/mikeydub/go-gallery/indexer/cmd"
)

func main() {
	cmd.Execute()
}
