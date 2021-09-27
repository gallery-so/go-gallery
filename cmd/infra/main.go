package main

import (
	"context"

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

	indexer := infra.NewIndexer(events, tokenReceive, contractReceive, "test", runtime)

	logrus.Infof("Starting indexer")
	indexer.Start()

	// //-------------
	// // SERVER_INIT
	// log.Fatal(infra.Init(portStr, runtime))

}

func tokenReceive(pCtx context.Context, pToken *persist.Token, pRuntime *runtime.Runtime) error {
	logrus.Infof("tokenReceive: %s", pToken.TokenURI)
	return nil
}
func contractReceive(pCtx context.Context, pContract *persist.Contract, pRuntime *runtime.Runtime) error {
	logrus.Infof("contractReceive: %+v", pContract)
	return nil
}
