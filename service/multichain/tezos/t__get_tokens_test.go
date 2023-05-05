package tezos

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
)

func TestGetTokensForWallet_Success(t *testing.T) {
	a := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*45)
	defer cancel()

	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()
	storage := newStorageClient(ctx)
	p := NewProvider(env.GetString("TEZOS_API_URL"), env.GetString("TOKEN_PROCESSING_URL"), env.GetString("IPFS_GATEWAY_URL"), http.DefaultClient, ipfsClient, arweaveClient, storage, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"))

	powerUsers := []persist.Address{"tz1hyNv7RBzNPGLpKfdwHRc6NhLW6VbzXP3N", "tz1YHsinBJHMj1YFN7UrCsVAgTcaJCH86PjK", "tz1bPMztWzs449CuEmVTY3BprhHMtm4NUQPJ"}

	for _, addr := range powerUsers {
		tokens, _, err := p.GetTokensByWalletAddress(ctx, addr, 50, 0)
		a.NoError(err)
		a.NotEmpty(tokens)
	}
}
