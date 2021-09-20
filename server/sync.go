package server

import (
	"context"

	"github.com/mikeydub/go-gallery/infra"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/sirupsen/logrus"
)

func getAndSyncTokens(pCtx context.Context, pUserID persist.DBID, pWalletAddress string, pSkipDB bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {

	tokens, err := infra.GetTokensForWallet(pCtx, pWalletAddress, 1, pSkipDB, pRuntime)
	if err != nil {
		return nil, err
	}

	go func() {
		tokenIDs := make([]persist.DBID, len(tokens))
		for i, token := range tokens {
			tokenIDs[i] = token.ID
		}
		err := persist.TokensClaim(pCtx, pUserID, tokenIDs, pRuntime)
		if err != nil {
			logrus.WithError(err).Error("failed to claim tokens")
		}
	}()

	return tokens, nil
}
