package indexer

import (
	"context"
	"database/sql"
	"math/big"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/ory/dockertest"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
)

func setupTest(t *testing.T) (*assert.Assertions, *sql.DB) {

	setDefaults("../_local/app-local-indexer-server.yaml")
	pg, pgUnpatch := docker.InitPostgresIndexer()

	db := postgres.NewClient()
	err := migrate.RunMigration(db, "./db/migrations/indexer")
	if err != nil {
		t.Fatalf("failed to seed db: %s", err)
	}

	t.Cleanup(func() {
		defer db.Close()
		defer pgUnpatch()
		for _, r := range []*dockertest.Resource{pg} {
			if err := r.Close(); err != nil {
				t.Fatalf("could not purge resource: %s", err)
			}
		}
	})

	return assert.New(t), db
}

var htmlLogs = []types.Log{{
	Address:     common.HexToAddress("0x0c2ee19b2a89943066c2dc7f1bddcc907f614033"),
	Topics:      []common.Hash{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x0000000000000000000000009a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"), common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000d9")},
	Data:        []byte{},
	BlockNumber: 1,
}, {
	Address:     common.HexToAddress("0x059edd72cd353df5106d2b9cc5ab83a52287ac3a"),
	Topics:      []common.Hash{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x0000000000000000000000009a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"), common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001")},
	Data:        []byte{},
	BlockNumber: 1,
}}
var ipfsLogs = []types.Log{{
	Address:     common.HexToAddress("0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d"),
	Topics:      []common.Hash{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x0000000000000000000000009a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"), common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001")},
	Data:        []byte{},
	BlockNumber: 2,
}}

var customHandlerLogs = []types.Log{{
	Address:     common.HexToAddress("0xd4e4078ca3495de5b1d4db434bebc5a986197782"),
	Topics:      []common.Hash{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x0000000000000000000000009a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"), common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001")},
	Data:        []byte{},
	BlockNumber: 22,
}}
var svgLogs = []types.Log{{Address: common.HexToAddress("0x69c40e500b84660cb2ab09cb9614fa2387f95f64"),
	Topics:      []common.Hash{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x0000000000000000000000009a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"), common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001")},
	Data:        []byte{},
	BlockNumber: 3,
}}

var allLogs = append(append(append(append(htmlLogs, ipfsLogs...), customHandlerLogs...), svgLogs...))

func newMockIndexer(db *sql.DB) *indexer {
	start := uint64(0)
	end := uint64(100)
	i := newIndexer(nil, nil, nil, nil, postgres.NewTokenRepository(db), postgres.NewContractRepository(db), persist.ChainETH, []eventHash{transferBatchEventHash, transferEventHash, transferSingleEventHash}, func(ctx context.Context, curBlock, nextBlock *big.Int, topics [][]common.Hash) ([]types.Log, error) {
		transferAgainLogs := []types.Log{{
			Address:     common.HexToAddress("0x0c2ee19b2a89943066c2dc7f1bddcc907f614033"),
			Topics:      []common.Hash{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), common.HexToHash("0x0000000000000000000000009a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"), common.HexToHash("0x0000000000000000000000008914496dc01efcc49a2fa340331fb90969b6f1d2"), common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000d9")},
			Data:        []byte{},
			BlockNumber: 51,
		}}
		if curBlock.Uint64() == 0 {
			return allLogs, nil
		}
		return transferAgainLogs, nil

	}, &start, &end)
	return i
}

func newStorageClient(ctx context.Context) *storage.Client {
	stg, err := storage.NewClient(ctx, option.WithCredentialsFile("../_deploy/service-key-dev.json"))
	if err != nil {
		panic(err)
	}
	return stg
}
