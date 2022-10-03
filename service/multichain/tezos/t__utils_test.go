package tezos

import (
	"context"
	"fmt"
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
	fi, err := util.FindFile("_deploy/service-key-dev.json", 4)
	if err != nil {
		panic(err)
	}
	stg, err := storage.NewClient(ctx, option.WithCredentialsFile(fi))
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

	// Tests can run from directories deeper in the source tree, so we need to search parent directories to find this config file
	path, err := util.FindFile("_local/app-local-backend.yaml", 5)
	if err != nil {
		panic(err)
	}

	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Sprintf("error reading viper config: %s\nmake sure your _local directory is decrypted and up-to-date", err))
	}

}
