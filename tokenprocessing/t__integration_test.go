package tokenprocessing

import (
	"testing"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/stretchr/testify/assert"
)

func TestIndexLogs_Success(t *testing.T) {
	// a, db, pgx := setupTest(t)

	// repos := postgres.NewRepositories(db, pgx)

	// ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	// defer cancel()

	// ethClient := rpc.NewEthClient()
	// ipfsShell := rpc.NewIPFSShell()
	// arweaveClient := rpc.NewArweaveClient()
	// stg := newStorageClient(ctx)

	t.Run("it can create image media", func(t *testing.T) {

		// uri, err := rpc.GetTokenURI(ctx, token.TokenType, token.ContractAddress, token.TokenID, ethClient)
		// tokenURIHasExpectedType(t, a, err, uri, persist.URITypeIPFS)

		// metadata, err := rpc.GetMetadataFromURI(ctx, uri, ipfsShell, arweaveClient)
		// mediaHasContent(t, a, err, metadata)

		// predicted, _, _, err := media.PredictMediaType(ctx, metadata["image"].(string))
		// mediaTypeHasExpectedType(t, a, err, persist.MediaTypeImage, predicted)

		// imageData, err := rpc.GetDataFromURI(ctx, persist.TokenURI(metadata["image"].(string)), ipfsShell, arweaveClient)
		// a.NoError(err)
		// a.NotEmpty(imageData)

		// predicted, _ = media.SniffMediaType(imageData)
		// mediaTypeHasExpectedType(t, a, nil, persist.MediaTypeImage, predicted)

		// image, animation := KeywordsForChain(persist.ChainETH, imageKeywords, animationKeywords)
		// obs, err := CacheObjectsForMetadata(ctx, metadata, persist.Address(token.ContractAddress), token.TokenID, uri, persist.ChainETH, ipfsShell, arweaveClient, stg, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), image, animation)
		// med := CreateMediaFromCachedObjects(ctx, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), obs)
		// mediaTypeHasExpectedType(t, a, err, persist.MediaTypeImage, med.MediaType)
		// a.Empty(med.ThumbnailURL)
		// a.NotEmpty(med.MediaURL)

		// mediaType, _, _, err := media.PredictMediaType(ctx, med.MediaURL.String())
		// mediaTypeHasExpectedType(t, a, err, persist.MediaTypeImage, mediaType)
	})

	t.Run("it can create svg media", func(t *testing.T) {
		// token := tokenExistsInDB(t, a, i.tokenRepo, "0x69c40e500b84660cb2ab09cb9614fa2387f95f64", "1")

		// uri, err := rpc.GetTokenURI(ctx, token.TokenType, token.ContractAddress, token.TokenID, ethClient)
		// tokenURIHasExpectedType(t, a, err, uri, persist.URITypeBase64JSON)

		// metadata, err := rpc.GetMetadataFromURI(ctx, uri, ipfsShell, arweaveClient)
		// mediaHasContent(t, a, err, metadata)

		// image, animation := KeywordsForChain(persist.ChainETH, imageKeywords, animationKeywords)
		// obs, err := CacheObjectsForMetadata(ctx, metadata, persist.Address(token.ContractAddress), token.TokenID, uri, persist.ChainETH, ipfsShell, arweaveClient, stg, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), image, animation)
		// med := CreateMediaFromCachedObjects(ctx, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), obs)
		// mediaTypeHasExpectedType(t, a, err, persist.MediaTypeSVG, med.MediaType)
		// a.Empty(med.ThumbnailURL)
		// a.Contains(med.MediaURL.String(), "https://")
	})

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
