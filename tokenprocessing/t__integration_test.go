package tokenprocessing

import (
	"context"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/stretchr/testify/assert"
)

func TestIndexLogs_Success(t *testing.T) {
	a, db, pgx := setupTest(t)

	repo := postgres.NewTokenGalleryRepository(db, coredb.New(pgx))

	t.Run("it can handle ens", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		token := tokenExistsInDB(t, a, repo, persist.Address(ensAddress), "c1cb7903f69821967b365cce775cd62d694cd7ae7cfe00efe1917a55fdae2bb7")
		uri, metadata, err := ens(ctx, token.TokenURI, "", token.TokenID, nil, nil, nil)
		a.NoError(err)
		a.NotEmpty(uri)
		a.NotEmpty(metadata["name"])

		token = tokenExistsInDB(t, a, repo, persist.Address(ensAddress), "8c111a4e7c31becd720bde47f538417068e102d45b7732f24cfeda9e2b22a45")
		uri, metadata, err = ens(ctx, token.TokenURI, "", token.TokenID, nil, nil, nil)
		a.NoError(err)
		a.NotEmpty(uri)
		a.NotEmpty(metadata["name"])
	})
}

func tokenExistsInDB(t *testing.T, a *assert.Assertions, tokenRepo persist.TokenGalleryRepository, address persist.Address, tokenID persist.TokenID) persist.TokenGallery {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := tokenRepo.GetByTokenIdentifiers(ctx, tokenID, address, persist.ChainETH, 0, -1)
	a.NoError(err)
	a.Len(tokens, 1)
	return tokens[0]
}
