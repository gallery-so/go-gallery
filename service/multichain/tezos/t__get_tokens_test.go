package tezos

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
)

func TestGetTokensForWallet_Success(t *testing.T) {
	a := setupTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*45)
	defer cancel()

	p := NewProvider(env.GetString("TEZOS_API_URL"), http.DefaultClient)

	powerUsers := []persist.Address{"tz1hyNv7RBzNPGLpKfdwHRc6NhLW6VbzXP3N", "tz1YHsinBJHMj1YFN7UrCsVAgTcaJCH86PjK", "tz1bPMztWzs449CuEmVTY3BprhHMtm4NUQPJ"}

	for _, addr := range powerUsers {
		tokens, _, err := p.GetTokensByWalletAddress(ctx, addr)
		a.NoError(err)
		a.NotEmpty(tokens)
	}
}
