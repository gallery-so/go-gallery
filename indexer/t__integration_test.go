package indexer

import (
	"context"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/stretchr/testify/assert"
)

func TestIndexLogs_Success(t *testing.T) {
	a, db, pgx, pgx2 := setupTest(t)
	i := newMockIndexer(db, pgx, pgx2)

	// Run the Indexer
	i.catchUp(sentry.SetHubOnContext(context.Background(), sentry.CurrentHub()), eventsToTopics(i.eventHashes))

	// ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	// defer cancel()

	// ethClient := rpc.NewEthClient()

	t.Run("it updates its state", func(t *testing.T) {
		a.EqualValues(testBlockTo-blocksPerLogsCall, i.lastSyncedChunk)
	})

	t.Run("it saves contracts to the db", func(t *testing.T) {
		for _, address := range expectedContracts() {
			contract := contractExistsInDB(t, a, i.contractRepo, address)
			a.NotEmpty(contract.ID)
			a.Equal(address, contract.Address)
		}
	})

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
