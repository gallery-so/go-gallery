package main

import (
	"github.com/mikeydub/go-gallery/infra"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/sirupsen/logrus"
)

func main() {

	config := runtime.ConfigLoad()
	// portStr := config.InfraPort

	// RUNTIME
	runtime, err := runtime.GetRuntime(config)
	if err != nil {
		panic(err.Error())
	}
	events := []infra.EventHash{infra.TransferBatchEventHash, infra.TransferEventHash, infra.TransferSingleEventHash}

	indexer := infra.NewIndexer(persist.Chain(runtime.Config.Chain), events, "stats.json", runtime)

	logrus.Infof("Starting indexer")
	indexer.Start()

}
