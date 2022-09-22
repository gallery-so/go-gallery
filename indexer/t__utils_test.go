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
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/ory/dockertest"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
)

var (
	testBlockFrom  = 0
	testBlockTo    = 100
	testAddress    = "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"
	contribAddress = "0xda3845b44736b57e05ee80fc011a52a9c777423a" // Jarrel's address with a contributor card in it
)

var allLogs = func() []types.Log {
	logs := htmlLogs
	logs = append(logs, ipfsLogs...)
	logs = append(logs, customHandlerLogs...)
	logs = append(logs, svgLogs...)
	logs = append(logs, erc1155Logs...)
	return logs
}()

func setupTest(t *testing.T) (*assert.Assertions, *sql.DB) {
	fi, err := util.FindFile("_local/app-local-indexer-server.yaml", 4)
	if err != nil {
		panic(err)
	}
	setDefaults(fi)
	pg, pgUnpatch := docker.InitPostgresIndexer()

	db := postgres.NewClient()
	err = migrate.RunMigration(db, "./db/migrations/indexer")
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

func newMockIndexer(db *sql.DB) *indexer {
	start := uint64(testBlockFrom)
	end := uint64(testBlockTo)

	rpcEnabled = true
	ethClient := rpc.NewEthSocketClient()

	i := newIndexer(ethClient, nil, nil, nil, postgres.NewTokenRepository(db), postgres.NewContractRepository(db), persist.ChainETH, []eventHash{transferBatchEventHash, transferEventHash, transferSingleEventHash}, func(ctx context.Context, curBlock, nextBlock *big.Int, topics [][]common.Hash) ([]types.Log, error) {
		transferAgainLogs := []types.Log{{
			Address:     common.HexToAddress("0x0c2ee19b2a89943066c2dc7f1bddcc907f614033"),
			Topics:      []common.Hash{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), common.HexToHash(testAddress), common.HexToHash("0x0000000000000000000000008914496dc01efcc49a2fa340331fb90969b6f1d2"), common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000d9")},
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

var htmlLogs = []types.Log{
	{
		Address: common.HexToAddress("0x0c2ee19b2a89943066c2dc7f1bddcc907f614033"),
		Topics: []common.Hash{
			common.HexToHash(string(transferEventHash)),
			common.HexToHash(persist.ZeroAddress.String()),
			common.HexToHash(testAddress),
			common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000d9"),
		},
		BlockNumber: 1,
	},
	{
		Address: common.HexToAddress("0x059edd72cd353df5106d2b9cc5ab83a52287ac3a"),
		Topics: []common.Hash{
			common.HexToHash(string(transferEventHash)),
			common.HexToHash(persist.ZeroAddress.String()),
			common.HexToHash(testAddress),
			common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
		},
		BlockNumber: 1,
	},
}
var ipfsLogs = []types.Log{
	{
		Address: common.HexToAddress("0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d"),
		Topics: []common.Hash{common.HexToHash(
			string(transferEventHash)),
			common.HexToHash(persist.ZeroAddress.String()),
			common.HexToHash(testAddress),
			common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
		},
		BlockNumber: 2,
	},
}
var customHandlerLogs = []types.Log{
	{
		Address: common.HexToAddress("0xd4e4078ca3495de5b1d4db434bebc5a986197782"),
		Topics: []common.Hash{
			common.HexToHash(string(transferEventHash)),
			common.HexToHash(persist.ZeroAddress.String()),
			common.HexToHash(testAddress),
			common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
		},
		BlockNumber: 22,
	},
}
var svgLogs = []types.Log{
	{
		Address: common.HexToAddress("0x69c40e500b84660cb2ab09cb9614fa2387f95f64"),
		Topics: []common.Hash{
			common.HexToHash(string(transferEventHash)),
			common.HexToHash(persist.ZeroAddress.String()),
			common.HexToHash(testAddress),
			common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
		},
		BlockNumber: 3,
	},
}
var erc1155Logs = []types.Log{
	{
		Address: common.HexToAddress("0xe01569ca9b39e55bc7c0dfa09f05fa15cb4c7698"),
		Topics: []common.Hash{
			common.HexToHash(string(transferSingleEventHash)),
			common.HexToHash(persist.ZeroAddress.String()),
			common.HexToHash(persist.ZeroAddress.String()),
			common.HexToHash(contribAddress),
		},
		Data:        common.Hex2Bytes("00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001"),
		BlockNumber: 5,
	},
}

type expectedTokenResults map[persist.TokenIdentifiers]persist.Token

var expectedResults expectedTokenResults = expectedTokenResults{
	persist.NewTokenIdentifiers("0x059edd72cd353df5106d2b9cc5ab83a52287ac3a", "1", 0): {
		BlockNumber:     1,
		OwnerAddress:    persist.EthereumAddress(testAddress),
		ContractAddress: persist.EthereumAddress("0x059edd72cd353df5106d2b9cc5ab83a52287ac3a"),
		TokenType:       persist.TokenTypeERC721,
		TokenID:         "1",
		Quantity:        "1",
	},
	persist.NewTokenIdentifiers("0x69c40e500b84660cb2ab09cb9614fa2387f95f64", "1", 0): persist.Token{
		BlockNumber:     3,
		OwnerAddress:    persist.EthereumAddress(testAddress),
		ContractAddress: persist.EthereumAddress("0x69c40e500b84660cb2ab09cb9614fa2387f95f64"),
		TokenType:       persist.TokenTypeERC721,
		TokenID:         "1",
		Quantity:        "1",
	},
	persist.NewTokenIdentifiers("0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d", "1", 0): persist.Token{
		BlockNumber:     2,
		OwnerAddress:    persist.EthereumAddress(testAddress),
		ContractAddress: persist.EthereumAddress("0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d"),
		TokenType:       persist.TokenTypeERC721,
		TokenID:         "1",
		Quantity:        "1",
	},
	persist.NewTokenIdentifiers("0xd4e4078ca3495de5b1d4db434bebc5a986197782", "1", 0): persist.Token{
		BlockNumber:     22,
		OwnerAddress:    persist.EthereumAddress(testAddress),
		ContractAddress: persist.EthereumAddress("0xd4e4078ca3495de5b1d4db434bebc5a986197782"),
		TokenType:       persist.TokenTypeERC721,
		TokenID:         "1",
		Quantity:        "1",
	},
	persist.NewTokenIdentifiers("0x0c2ee19b2a89943066c2dc7f1bddcc907f614033", "d9", 0): persist.Token{
		BlockNumber:     51,
		OwnerAddress:    persist.EthereumAddress("0x8914496dc01efcc49a2fa340331fb90969b6f1d2"),
		ContractAddress: persist.EthereumAddress("0x0c2ee19b2a89943066c2dc7f1bddcc907f614033"),
		TokenType:       persist.TokenTypeERC721,
		TokenID:         "d9",
		Quantity:        "1",
	},
	persist.NewTokenIdentifiers("0xe01569ca9b39e55bc7c0dfa09f05fa15cb4c7698", "0", 0): persist.Token{
		BlockNumber:     5,
		OwnerAddress:    persist.EthereumAddress(contribAddress),
		ContractAddress: persist.EthereumAddress("0xe01569ca9b39e55bc7c0dfa09f05fa15cb4c7698"),
		TokenType:       persist.TokenTypeERC1155,
		TokenID:         "0",
		Quantity:        "1",
	},
}

func expectedTokensForAddress(address persist.EthereumAddress) int {
	count := 0
	for _, token := range expectedResults {
		if token.OwnerAddress == address {
			count++
		}
	}
	return count
}

func expectedContracts() []persist.EthereumAddress {
	contracts := make([]persist.EthereumAddress, 0, len(expectedResults))
	seen := map[persist.EthereumAddress]struct{}{}
	for _, token := range expectedResults {
		if _, ok := seen[token.ContractAddress]; !ok {
			seen[token.ContractAddress] = struct{}{}
			contracts = append(contracts, token.ContractAddress)
		}
	}
	return contracts
}
