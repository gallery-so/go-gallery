package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var bigZero = big.NewInt(0)

func main() {
	setDefaults()
	run()
}

func run() {

	pgClient := postgres.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tokenRepo := postgres.NewTokenRepository(pgClient)
	nftRepo := postgres.NewNFTRepository(pgClient, nil, nil)

	ethClient := rpc.NewEthClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	userIDs := getAllUsers(ctx, pgClient)

	usersToNewCollections := getNewCollections(ctx, pgClient, userIDs, nftRepo, tokenRepo, ethClient, ipfsClient, arweaveClient)

	updateCollections(ctx, pgClient, usersToNewCollections)
}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("OPENSEA_API_KEY", "")
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	viper.SetDefault("IPFS_URL", "https://ipfs.io")

	viper.AutomaticEnv()
}

func updateCollections(ctx context.Context, pgClient *sql.DB, usersToNewCollections map[persist.DBID]map[persist.DBID][]persist.DBID) {
	for userID, newCollections := range usersToNewCollections {
		logrus.Infof("Updating %d collections for user %s", len(newCollections), userID)
		for coll, nfts := range newCollections {
			logrus.Infof("Updating collection %s with %d nfts for user %s", coll, len(nfts), userID)
			_, err := pgClient.ExecContext(ctx, `UPDATE collections_v2 SET NFTS = $2 WHERE ID = $1`, coll, pq.Array(nfts))
			if err != nil {
				panic(err)
			}
		}
	}
}

func getNewCollections(ctx context.Context, pgClient *sql.DB, userIDs map[persist.DBID][]persist.Address, nftRepo *postgres.NFTRepository, tokenRepo *postgres.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) map[persist.DBID]map[persist.DBID][]persist.DBID {
	usersToNewCollections := map[persist.DBID]map[persist.DBID][]persist.DBID{}

	for userID, addresses := range userIDs {
		logrus.Infof("Processing user %s with addresses %v", userID, addresses)
		res, err := pgClient.QueryContext(ctx, `SELECT ID, NFTS FROM collections WHERE OWNER_USER_ID = $1 AND DELETED = false;`, userID)
		if err != nil {
			panic(err)
		}
		collsToNFTs := map[persist.DBID][]persist.DBID{}
		for res.Next() {
			var nftIDs []persist.DBID
			var collID persist.DBID
			if err = res.Scan(&collID, pq.Array(&nftIDs)); err != nil {
				panic(err)
			}
			collsToNFTs[collID] = nftIDs
		}
		if err := res.Err(); err != nil {
			panic(err)
		}
		newCollsToNFTs := map[persist.DBID][]persist.DBID{}
		for coll, nftIDs := range collsToNFTs {
			newCollsToNFTs[coll] = make([]persist.DBID, 0, 10)
			logrus.Infof("Processing collection %s with %d nfts for user %s", coll, len(nftIDs), userID)
			for _, nftID := range nftIDs {
				fullNFT, err := nftRepo.GetByID(ctx, nftID)
				if err != nil {
					if _, ok := err.(persist.ErrNFTNotFoundByID); !ok {
						panic(err)
					} else {
						logrus.Infof("NFT %s not found for collection %s", nftID, coll)
					}
				}

				tokenEquivelents, err := tokenRepo.GetByTokenIdentifiers(ctx, fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, -1, -1)
				if err != nil {
					if _, ok := err.(persist.ErrTokenNotFoundByIdentifiers); !ok {
						panic(err)
					} else {
						logrus.Infof("Token equi not found for %s-%s in collection %s. Making token...", fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, coll)
						tokenEquivelents, err = nftToTokens(ctx, fullNFT, addresses, ethClient, ipfsClient, arweaveClient)
						if err != nil {
							logrus.Errorf("Error making token for %s-%s in collection %s: %s", fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, coll, err)
							continue
						}
					}
				}
				for _, token := range tokenEquivelents {
					if containsAddress(token.OwnerAddress, addresses) {
						logrus.Infof("token %s-%s is owned by %s", token.ContractAddress, token.TokenID, token.OwnerAddress)
						newCollsToNFTs[coll] = append(newCollsToNFTs[coll], token.ID)
					}
				}
			}
		}
		usersToNewCollections[userID] = newCollsToNFTs
	}
	return usersToNewCollections
}

func getAllUsers(ctx context.Context, pgClient *sql.DB) map[persist.DBID][]persist.Address {

	res, err := pgClient.QueryContext(ctx, `SELECT ID,ADDRESSES FROM users WHERE DELETED = false;`)
	if err != nil {
		panic(err)
	}

	result := map[persist.DBID][]persist.Address{}
	for res.Next() {
		var id persist.DBID
		var addresses []persist.Address
		if err = res.Scan(&id, pq.Array(&addresses)); err != nil {
			panic(err)
		}
		if _, ok := result[id]; !ok {
			result[id] = make([]persist.Address, 0, 3)
		}
		result[id] = append(result[id], addresses...)
	}
	return result
}

func nftToTokens(ctx context.Context, nft persist.NFT, addresses []persist.Address, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) ([]persist.Token, error) {

	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	allTokens := make([]persist.Token, 0, 5)
	asURI := persist.TokenURI(nft.ImageURL)
	media := persist.Media{}

	bs, err := rpc.GetDataFromURI(ctx, asURI, ipfsClient, arweaveClient)
	if err == nil {
		mediaType := persist.SniffMediaType(bs)
		if mediaType != persist.MediaTypeUnknown {
			media = persist.Media{
				MediaURL:     persist.NullString(nft.ImageURL),
				ThumbnailURL: persist.NullString(nft.ImagePreviewURL),
				MediaType:    mediaType,
			}
		}
	}

	uri := persist.TokenURI(strings.ReplaceAll(nft.TokenMetadataURL.String(), "{id}", nft.OpenseaTokenID.ToUint256String()))
	metadata, _ := rpc.GetMetadataFromURI(ctx, uri, ipfsClient, arweaveClient)
	t := persist.Token{
		CollectorsNote:  nft.CollectorsNote,
		TokenMetadata:   metadata,
		Media:           media,
		TokenURI:        uri,
		Chain:           persist.ChainETH,
		TokenID:         nft.OpenseaTokenID,
		OwnerAddress:    nft.OwnerAddress,
		ContractAddress: nft.Contract.ContractAddress,
		BlockNumber:     persist.BlockNumber(block),
		OwnershipHistory: []persist.AddressAtBlock{
			{
				Address: persist.ZeroAddress,
				Block:   persist.BlockNumber(block - 1),
			},
		},
		ExternalURL: nft.ExternalURL,
		Description: nft.Description,
		Name:        nft.Name,
		Quantity:    "1",
	}
	switch nft.Contract.ContractSchemaName {
	case "ERC721", "CRYPTOPUNKS":
		t.TokenType = persist.TokenTypeERC721
		allTokens = append(allTokens, t)
	case "ERC1155":
		t.TokenType = persist.TokenTypeERC1155
		ierc1155, err := contracts.NewIERC1155Caller(t.ContractAddress.Address(), ethClient)
		if err != nil {
			return nil, err
		}

		for _, addr := range addresses {
			new := t
			bal, err := ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, addr.Address(), t.TokenID.BigInt())
			if err != nil {
				return nil, err
			}
			if bal.Cmp(bigZero) > 0 {
				new.OwnerAddress = addr
				new.Quantity = persist.HexString(bal.Text(16))

				allTokens = append(allTokens, new)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported token type: %s", nft.Contract.ContractSchemaName)
	}
	return allTokens, nil
}

func containsAddress(addr persist.Address, addrs []persist.Address) bool {
	for _, a := range addrs {
		if addr.String() == a.String() {
			return true
		}
	}
	return false
}
