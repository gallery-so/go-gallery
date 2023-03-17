package indexer

import (
	"context"
	"database/sql"
	"math/big"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/jackc/pgx/v4/pgxpool"
	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/indexer/refresh"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
)

var (
	testBlockFrom            = 0
	testBlockTo              = 100
	testAddress              = "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"
	galleryMembershipAddress = "0xe01569ca9b39e55bc7c0dfa09f05fa15cb4c7698"
	ensAddress               = "0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85"
	contribAddress           = "0xda3845b44736b57e05ee80fc011a52a9c777423a" // Jarrel's address with a contributor card in it
)

var allLogs = func() []types.Log {
	logs := htmlLogs
	logs = append(logs, ipfsLogs...)
	logs = append(logs, customHandlerLogs...)
	logs = append(logs, svgLogs...)
	logs = append(logs, erc1155Logs...)
	logs = append(logs, ensLogs...)
	return logs
}()

func setupTest(t *testing.T) (*assert.Assertions, *sql.DB, *pgxpool.Pool) {
	SetDefaults()
	LoadConfigFile("indexer-server", "local")
	ValidateEnv()

	r, err := docker.StartPostgresIndexer()
	if err != nil {
		t.Fatal(err)
	}

	hostAndPort := strings.Split(r.GetHostPort("5432/tcp"), ":")
	t.Setenv("POSTGRES_HOST", hostAndPort[0])
	t.Setenv("POSTGRES_PORT", hostAndPort[1])

	db := postgres.MustCreateClient()
	pgx := postgres.NewPgxClient()
	migrate, err := migrate.RunMigration(db, "./db/migrations/indexer")
	if err != nil {
		t.Fatalf("failed to seed db: %s", err)
	}
	t.Cleanup(func() {
		migrate.Close()
		r.Close()
	})

	return assert.New(t), db, pgx
}

func newMockIndexer(db *sql.DB, pool *pgxpool.Pool) *indexer {
	start := uint64(testBlockFrom)
	end := uint64(testBlockTo)
	rpcEnabled = true
	ethClient := rpc.NewEthSocketClient()
	storageClient := newStorageClient(context.Background())
	bucket := storageClient.Bucket(env.Get[string](context.Background(), "GCLOUD_TOKEN_LOGS_BUCKET"))

	i := newIndexer(ethClient, nil, nil, nil, postgres.NewTokenRepository(db), postgres.NewContractRepository(db), refresh.AddressFilterRepository{Bucket: bucket}, persist.ChainETH, defaultTransferEvents, func(ctx context.Context, curBlock, nextBlock *big.Int, topics [][]common.Hash) ([]types.Log, error) {
		transferAgainLogs := []types.Log{{
			Address:     common.HexToAddress("0x0c2ee19b2a89943066c2dc7f1bddcc907f614033"),
			Topics:      []common.Hash{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), common.HexToHash(testAddress), common.HexToHash("0x0000000000000000000000008914496dc01efcc49a2fa340331fb90969b6f1d2"), common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000d9")},
			Data:        []byte{},
			BlockNumber: 51,
			TxIndex:     1,
		}}
		if curBlock.Uint64() == 0 {
			return allLogs, nil
		}
		return transferAgainLogs, nil
	}, &start, &end)
	return i
}

func newStorageClient(ctx context.Context) *storage.Client {
	stg, err := storage.NewClient(ctx, option.WithCredentialsJSON(util.LoadEncryptedServiceKey("secrets/dev/service-key-dev.json")))
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
		Address: common.HexToAddress(galleryMembershipAddress),
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
var ensLogs = []types.Log{
	{
		Address: common.HexToAddress(ensAddress),
		Topics: []common.Hash{
			common.HexToHash(string(transferEventHash)),
			common.HexToHash(persist.ZeroAddress.String()),
			common.HexToHash(testAddress),
			common.HexToHash("0xc1cb7903f69821967b365cce775cd62d694cd7ae7cfe00efe1917a55fdae2bb7"),
		},
		BlockNumber: 42,
	},
	{
		Address: common.HexToAddress(ensAddress),
		Topics: []common.Hash{
			common.HexToHash(string(transferEventHash)),
			common.HexToHash(persist.ZeroAddress.String()),
			common.HexToHash(testAddress),
			// Leading zero in token ID
			common.HexToHash("0x08c111a4e7c31becd720bde47f538417068e102d45b7732f24cfeda9e2b22a45"),
		},
		BlockNumber: 42,
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
	persist.NewTokenIdentifiers(persist.Address(galleryMembershipAddress), "0", 0): persist.Token{
		BlockNumber:     5,
		OwnerAddress:    persist.EthereumAddress(contribAddress),
		ContractAddress: persist.EthereumAddress(galleryMembershipAddress),
		TokenType:       persist.TokenTypeERC1155,
		TokenID:         "0",
		Quantity:        "1",
	},
	persist.NewTokenIdentifiers(persist.Address(ensAddress), "c1cb7903f69821967b365cce775cd62d694cd7ae7cfe00efe1917a55fdae2bb7", 0): persist.Token{
		BlockNumber:     42,
		OwnerAddress:    persist.EthereumAddress(testAddress),
		ContractAddress: persist.EthereumAddress(ensAddress),
		TokenType:       persist.TokenTypeERC721,
		TokenID:         "c1cb7903f69821967b365cce775cd62d694cd7ae7cfe00efe1917a55fdae2bb7",
		Quantity:        "1",
	},
	persist.NewTokenIdentifiers(persist.Address(ensAddress), "8c111a4e7c31becd720bde47f538417068e102d45b7732f24cfeda9e2b22a45", 0): persist.Token{
		BlockNumber:     42,
		OwnerAddress:    persist.EthereumAddress(testAddress),
		ContractAddress: persist.EthereumAddress(ensAddress),
		TokenType:       persist.TokenTypeERC721,
		TokenID:         "8c111a4e7c31becd720bde47f538417068e102d45b7732f24cfeda9e2b22a45",
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
