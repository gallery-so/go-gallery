package server

import (
	"context"

	"github.com/mikeydub/go-gallery/infra"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/sirupsen/logrus"
)

func getAndSyncTokens(pCtx context.Context, pUserID persist.DBID, pWalletAddresses []string, pSkipDB bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {

	// TODO magic numbers and index
	tokens, err := infra.GetTokensForWallet(pCtx, pWalletAddresses[0], 1, 50, pSkipDB, pRuntime)
	if err != nil {
		return nil, err
	}

	go func() {
		tokenIDs := make([]persist.DBID, len(tokens))
		for i, token := range tokens {
			tokenIDs[i] = token.ID
		}
		err := persist.CollClaimNFTs(pCtx, pUserID, pWalletAddresses, &persist.CollectionUpdateNftsInput{Nfts: tokenIDs}, pRuntime)
		if err != nil {
			logrus.WithError(err).Error("failed to claim tokens")
		}
	}()

	return tokens, nil
}
