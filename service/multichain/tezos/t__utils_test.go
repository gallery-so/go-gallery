package tezos

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
)

func setupTest(t *testing.T) *assert.Assertions {

	setDefaults()

	return assert.New(t)
}

func newStorageClient(ctx context.Context) *storage.Client {
	stg, err := storage.NewClient(ctx, option.WithCredentialsJSON(util.LoadEncryptedServiceKey("secrets/dev/service-key-dev.json")))
	if err != nil {
		panic(err)
	}
	return stg
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("PORT", 4000)
	viper.SetDefault("IPFS_URL", "https://gallery.infura-ipfs.io")
	viper.SetDefault("IPFS_API_URL", "https://ipfs.infura.io:5001")
	viper.SetDefault("IPFS_PROJECT_ID", "")
	viper.SetDefault("IPFS_PROJECT_SECRET", "")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "dev-token-content")
	viper.SetDefault("INDEXER_HOST", "http://localhost:6000")
	viper.SetDefault("TOKEN_PROCESSING_URL", "http://localhost:6500")
	viper.SetDefault("TEZOS_API_URL", "https://api.tzkt.io")

	viper.AutomaticEnv()

	envFile := util.ResolveEnvFile("backend", "local")
	util.LoadEncryptedEnvFile(envFile)
}
