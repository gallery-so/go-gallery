package indexer

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestIndexLogs_Success(t *testing.T) {
	a, db := setupTest(t)
	i := newMockIndexer(db)
	i.Start(configureRootContext())

	tokenRepo := postgres.NewTokenRepository(db)
	tokens, _, err := tokenRepo.GetByWallet(context.Background(), "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5", -1, 0)
	a.NoError(err)
	a.Len(tokens, 4)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ethClient := rpc.NewEthClient()
	ipfsShell := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()
	stg := newStorageClient(ctx)

	uri := testBase64JSONURI(ctx, ethClient, a)

	uri = testIPFSURI(uri, err, ctx, ethClient, a)

	metadata := testIPFSMetadataRetrieval(ctx, uri, ipfsShell, arweaveClient, a)

	testPredictIPFSMediaType(ctx, metadata, a)

	testGetImageData(ctx, metadata, ipfsShell, arweaveClient, a)

	testCreateImageMedia(ctx, metadata, uri, ipfsShell, arweaveClient, stg, a)

	testCreateSVGMedia(ctx, ethClient, a, ipfsShell, arweaveClient, stg)
}

func testCreateSVGMedia(ctx context.Context, ethClient *ethclient.Client, a *assert.Assertions, ipfsShell *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) {
	svgURI, err := rpc.GetTokenURI(ctx, persist.TokenTypeERC721, "0x69c40e500b84660cb2ab09cb9614fa2387f95f64", "1", ethClient)
	a.NoError(err)
	a.Equal(svgURI.Type(), persist.URITypeBase64JSON)
	svgMetadata, err := rpc.GetMetadataFromURI(ctx, svgURI, ipfsShell, arweaveClient)
	a.NoError(err)
	a.NotEmpty(svgMetadata["image"])
	med, err := media.MakePreviewsForMetadata(ctx, svgMetadata, "0x69c40e500b84660cb2ab09cb9614fa2387f95f64", "1", svgURI, persist.ChainETH, ipfsShell, arweaveClient, stg, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), imageKeywords, animationKeywords)
	a.NoError(err)
	a.Equal(persist.MediaTypeSVG, med.MediaType)
	a.Empty(med.ThumbnailURL)
	a.Contains(med.MediaURL.String(), "https://")
}

func testCreateImageMedia(ctx context.Context, metadata persist.TokenMetadata, uri persist.TokenURI, ipfsShell *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, a *assert.Assertions) {
	med, err := media.MakePreviewsForMetadata(ctx, metadata, "0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d", "1", uri, persist.ChainETH, ipfsShell, arweaveClient, stg, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), imageKeywords, animationKeywords)
	a.NoError(err)
	a.Equal(persist.MediaTypeImage, med.MediaType)
	a.Empty(med.ThumbnailURL)
	a.NotEmpty(med.MediaURL)
	mediaType, _, err := media.PredictMediaType(ctx, med.MediaURL.String())
	a.NoError(err)
	a.Equal(persist.MediaTypeImage, mediaType)
}

func testGetImageData(ctx context.Context, metadata persist.TokenMetadata, ipfsShell *shell.Shell, arweaveClient *goar.Client, a *assert.Assertions) {
	imageData, err := rpc.GetDataFromURI(ctx, persist.TokenURI(metadata["image"].(string)), ipfsShell, arweaveClient)
	a.NoError(err)
	a.NotEmpty(imageData)
	mediaType, _ := persist.SniffMediaType(imageData)
	a.Equal(persist.MediaTypeImage, mediaType)
}

func testPredictIPFSMediaType(ctx context.Context, metadata persist.TokenMetadata, a *assert.Assertions) {
	predicted, _, err := media.PredictMediaType(ctx, metadata["image"].(string))
	a.NoError(err)
	a.Equal(predicted, persist.MediaTypeImage)
}

func testIPFSMetadataRetrieval(ctx context.Context, uri persist.TokenURI, ipfsShell *shell.Shell, arweaveClient *goar.Client, a *assert.Assertions) persist.TokenMetadata {
	metadata, err := rpc.GetMetadataFromURI(ctx, uri, ipfsShell, arweaveClient)
	a.NoError(err)
	a.NotEmpty(metadata["image"])
	return metadata
}

func testIPFSURI(uri persist.TokenURI, err error, ctx context.Context, ethClient *ethclient.Client, a *assert.Assertions) persist.TokenURI {
	uri, err = rpc.GetTokenURI(ctx, persist.TokenTypeERC721, "0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d", "0", ethClient)
	a.NoError(err)
	a.Equal(uri.Type(), persist.URITypeIPFS)
	return uri
}

func testBase64JSONURI(ctx context.Context, ethClient *ethclient.Client, a *assert.Assertions) persist.TokenURI {
	uri, err := rpc.GetTokenURI(ctx, persist.TokenTypeERC721, "0x0c2ee19b2a89943066c2dc7f1bddcc907f614033", "d9", ethClient)
	a.NoError(err)
	a.Equal(uri.Type(), persist.URITypeBase64JSON)
	return uri
}
