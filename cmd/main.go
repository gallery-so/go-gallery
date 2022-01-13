package main

import (
	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	addr := persist.Address("0xb104371d5a2680fb0d47ea9a3aa2348392454186")
	assets, err := opensea.FetchAssetsForWallet(addr, 0, 0, nil)
	if err != nil {
		logrus.Error(err)
	}
	logrus.Info(len(assets))
}
