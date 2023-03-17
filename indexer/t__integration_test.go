package indexer

import (
	"context"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/stretchr/testify/assert"
)

func TestIndexLogs_Success(t *testing.T) {
	a, db, pgx := setupTest(t)
	i := newMockIndexer(db, pgx)

	// Run the Indexer
	i.catchUp(sentry.SetHubOnContext(context.Background(), sentry.CurrentHub()), eventsToTopics(i.eventHashes))

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ethClient := rpc.NewEthClient()
	ipfsShell := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()
	stg := newStorageClient(ctx)

	t.Run("it updates its state", func(t *testing.T) {
		a.EqualValues(testBlockTo-blocksPerLogsCall, i.lastSyncedChunk)
	})

	t.Run("it saves ERC-721s to the db", func(t *testing.T) {
		tokens := addressHasTokensInDB(t, a, i.tokenRepo, persist.EthereumAddress(testAddress), expectedTokensForAddress(persist.EthereumAddress(testAddress)))
		for _, token := range tokens {
			tokenMatchesExpected(t, a, token)
		}
	})

	t.Run("it saves ERC-1155s to the db", func(t *testing.T) {
		tokens := addressHasTokensInDB(t, a, i.tokenRepo, persist.EthereumAddress(contribAddress), expectedTokensForAddress(persist.EthereumAddress(contribAddress)))
		for _, token := range tokens {
			tokenMatchesExpected(t, a, token)
		}
	})

	t.Run("it saves contracts to the db", func(t *testing.T) {
		for _, address := range expectedContracts() {
			contract := contractExistsInDB(t, a, i.contractRepo, address)
			a.NotEmpty(contract.ID)
			a.Equal(address, contract.Address)
		}
	})

	t.Run("it can parse base64 uris", func(t *testing.T) {
		token := tokenExistsInDB(t, a, i.tokenRepo, "0x0c2ee19b2a89943066c2dc7f1bddcc907f614033", "d9")
		uri, err := rpc.GetTokenURI(ctx, persist.TokenTypeERC721, token.ContractAddress, token.TokenID, ethClient)
		tokenURIHasExpectedType(t, a, err, uri, persist.URITypeBase64JSON)
	})

	t.Run("it can create image media", func(t *testing.T) {
		token := tokenExistsInDB(t, a, i.tokenRepo, "0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d", "1")
		uri, err := rpc.GetTokenURI(ctx, token.TokenType, token.ContractAddress, token.TokenID, ethClient)
		tokenURIHasExpectedType(t, a, err, uri, persist.URITypeIPFS)

		metadata, err := rpc.GetMetadataFromURI(ctx, uri, ipfsShell, arweaveClient)
		mediaHasContent(t, a, err, metadata)

		predicted, _, _, err := media.PredictMediaType(ctx, metadata["image"].(string))
		mediaTypeHasExpectedType(t, a, err, persist.MediaTypeImage, predicted)

		imageData, err := rpc.GetDataFromURI(ctx, persist.TokenURI(metadata["image"].(string)), ipfsShell, arweaveClient)
		a.NoError(err)
		a.NotEmpty(imageData)

		predicted, _ = persist.SniffMediaType(imageData)
		mediaTypeHasExpectedType(t, a, nil, persist.MediaTypeImage, predicted)

		image, animation := media.KeywordsForChain(persist.ChainETH, imageKeywords, animationKeywords)
		med, err := media.MakePreviewsForMetadata(ctx, metadata, persist.Address(token.ContractAddress), token.TokenID, uri, persist.ChainETH, ipfsShell, arweaveClient, stg, env.Get[string](ctx, "GCLOUD_TOKEN_CONTENT_BUCKET"), image, animation)
		mediaTypeHasExpectedType(t, a, err, persist.MediaTypeImage, med.MediaType)
		a.Empty(med.ThumbnailURL)
		a.NotEmpty(med.MediaURL)

		mediaType, _, _, err := media.PredictMediaType(ctx, med.MediaURL.String())
		mediaTypeHasExpectedType(t, a, err, persist.MediaTypeImage, mediaType)
	})

	t.Run("it can create svg media", func(t *testing.T) {
		token := tokenExistsInDB(t, a, i.tokenRepo, "0x69c40e500b84660cb2ab09cb9614fa2387f95f64", "1")

		uri, err := rpc.GetTokenURI(ctx, token.TokenType, token.ContractAddress, token.TokenID, ethClient)
		tokenURIHasExpectedType(t, a, err, uri, persist.URITypeBase64JSON)

		metadata, err := rpc.GetMetadataFromURI(ctx, uri, ipfsShell, arweaveClient)
		mediaHasContent(t, a, err, metadata)

		image, animation := media.KeywordsForChain(persist.ChainETH, imageKeywords, animationKeywords)
		med, err := media.MakePreviewsForMetadata(ctx, metadata, persist.Address(token.ContractAddress), token.TokenID, uri, persist.ChainETH, ipfsShell, arweaveClient, stg, env.Get[string](ctx, "GCLOUD_TOKEN_CONTENT_BUCKET"), image, animation)
		mediaTypeHasExpectedType(t, a, err, persist.MediaTypeSVG, med.MediaType)
		a.Empty(med.ThumbnailURL)
		a.Contains(med.MediaURL.String(), "https://")
	})

}

func tokenExistsInDB(t *testing.T, a *assert.Assertions, tokenRepo persist.TokenRepository, address persist.EthereumAddress, tokenID persist.TokenID) persist.Token {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := tokenRepo.GetByTokenIdentifiers(ctx, tokenID, address, 0, -1)
	a.NoError(err)
	a.Len(tokens, 1)
	return tokens[0]
}

func contractExistsInDB(t *testing.T, a *assert.Assertions, contractRepo persist.ContractRepository, address persist.EthereumAddress) persist.Contract {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	contract, err := contractRepo.GetByAddress(ctx, address)
	a.NoError(err)
	return contract
}

func addressHasTokensInDB(t *testing.T, a *assert.Assertions, tokenRepo persist.TokenRepository, address persist.EthereumAddress, expected int) []persist.Token {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, _, err := tokenRepo.GetByWallet(ctx, address, -1, 0)
	a.NoError(err)
	a.Len(tokens, expected)
	return tokens
}

func tokenURIHasExpectedType(t *testing.T, a *assert.Assertions, err error, uri persist.TokenURI, expectedType persist.URIType) {
	t.Helper()
	a.NoError(err)
	a.Equal(expectedType, uri.Type())
}

func mediaHasContent(t *testing.T, a *assert.Assertions, err error, metadata persist.TokenMetadata) {
	t.Helper()
	a.NoError(err)
	a.NotEmpty(metadata["image"])
}

func mediaTypeHasExpectedType(t *testing.T, a *assert.Assertions, err error, expected, actual persist.MediaType) {
	t.Helper()
	a.NoError(err)
	a.Equal(expected, actual)
}

func tokenMatchesExpected(t *testing.T, a *assert.Assertions, actual persist.Token) {
	t.Helper()
	id := persist.NewTokenIdentifiers(persist.Address(actual.ContractAddress), actual.TokenID, actual.Chain)
	expected, ok := expectedResults[id]
	if !ok {
		t.Fatalf("tokenID=%s not in expected results", id)
	}
	a.NotEmpty(actual.ID)
	a.Equal(expected.BlockNumber, actual.BlockNumber)
	a.Equal(expected.OwnerAddress, actual.OwnerAddress)
	a.Equal(expected.ContractAddress, actual.ContractAddress)
	a.Equal(expected.TokenType, actual.TokenType)
	a.Equal(expected.TokenID, actual.TokenID)
	a.Equal(expected.Quantity, actual.Quantity)
}
