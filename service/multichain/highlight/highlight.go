package highlight

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/shurcooL/graphql"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
)

const (
	baseURL                  = "https://api.highlight.xyz:8080"
	txnCodeReverted          = "TX_REVERTED"
	txnCodeCancelled         = "TX_CANCELLED"
	claimCodeMintUnavailable = "MINT_UNAVAILABLE"
)

var ErrHighlightChainNotSupported = errors.New("chain is not supported by highlight")

type ErrHighlightTxnFailed struct {
	StatusCode   string
	CollectionID string
	Chain        persist.Chain
	Address      persist.Address
}

func (e ErrHighlightTxnFailed) Error() string {
	return fmt.Sprintf("failed to execute mint transaction for collectionID=%s; chain=%d; address=%s: %s", e.CollectionID, e.Chain, e.Address, e.StatusCode)
}

type ErrHighlightCollectionMintUnavailable struct{ CollectionID string }

func (e ErrHighlightCollectionMintUnavailable) Error() string {
	return fmt.Sprintf("collectionID=%s is unavailable for minting", e.CollectionID)
}

type Provider struct {
	httpClient *http.Client
	gql        *graphql.Client
	appID      string
	appSecret  string
}

func NewProvider(c *http.Client) *Provider {
	appID := env.GetString("HIGHLIGHT_APP_ID")
	if appID == "" {
		panic("HIGHLIGHT_APP_ID is not set")
	}
	appSecret := env.GetString("HIGHLIGHT_APP_SECRET")
	if appSecret == "" {
		panic("HIGHLIGHT_APP_SECRET is not set")
	}
	return &Provider{
		gql:       graphql.NewClient(baseURL, c),
		appID:     appID,
		appSecret: appSecret,
	}
}

func (api *Provider) ClaimMint(ctx context.Context, collectionID string, qty int, recipient persist.ChainAddress) (string, error) {
	if recipient.Chain().L1Chain() != persist.L1Chain(persist.ChainETH) {
		return "", ErrHighlightChainNotSupported
	}

	var m struct {
		HighlightMint struct {
			ClaimId     string
			ClaimStatus string
		} `graphql:"highlightMint($collectionId: String!, $data: GaslessClaimAppInput!)"`
	}

	err := api.gql.Mutate(ctx, &m, map[string]any{
		"collectionId": graphql.String(collectionID),
		"data": struct {
			NumTokensToMint graphql.Int
			MintRecipient   graphql.String
		}{
			NumTokensToMint: graphql.Int(qty),
			MintRecipient:   graphql.String(recipient.Address()),
		},
	})
	if err != nil {
		if err.Error() == claimCodeMintUnavailable {
			err = ErrHighlightCollectionMintUnavailable{CollectionID: collectionID}
		}
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		return "", err
	}

	switch m.HighlightMint.ClaimStatus {
	case txnCodeReverted, txnCodeCancelled:
		err := ErrHighlightTxnFailed{
			StatusCode:   m.HighlightMint.ClaimStatus,
			CollectionID: collectionID,
			Chain:        recipient.Chain(),
			Address:      recipient.Address(),
		}
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		return "", err
	default:
		return m.HighlightMint.ClaimId, nil
	}
}
