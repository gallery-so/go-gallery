package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	ens "github.com/wealdtech/go-ens/v3"
)

var bigZero = big.NewInt(0)

func main() {
	setDefaults()
	run()
}

func run() {

	pgClient := postgres.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour*12)
	defer cancel()

	galleryRepo := postgres.NewGalleryRepository(pgClient, nil)
	tokenRepo := postgres.NewTokenGalleryRepository(pgClient, nil)
	nftRepo := postgres.NewNFTRepository(pgClient, galleryRepo)
	userRepo := postgres.NewUserRepository(pgClient)
	collectionRepo := postgres.NewCollectionRepository(pgClient, galleryRepo)
	backupRepo := postgres.NewBackupRepository(pgClient)

	ethClient := rpc.NewEthClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	userIDs := getAllUsers(ctx, pgClient)

	usersToNewCollections := getNewCollections(ctx, pgClient, userIDs, nftRepo, userRepo, collectionRepo, tokenRepo, galleryRepo, backupRepo, ethClient, ipfsClient, arweaveClient)

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

type userIDCollsTuple struct {
	userID         persist.DBID
	newCollsToNFTs map[persist.DBID][]persist.DBID
}

func getNewCollections(ctx context.Context, pgClient *sql.DB, userIDs map[persist.DBID][]persist.Wallet, nftRepo *postgres.NFTRepository, userRepo persist.UserRepository, collRepo persist.CollectionRepository, tokenRepo *postgres.TokenGalleryRepository, galleryRepo *postgres.GalleryRepository, backupRepo *postgres.BackupRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) map[persist.DBID]map[persist.DBID][]persist.DBID {
	usersToNewCollections := map[persist.DBID]map[persist.DBID][]persist.DBID{}
	receivedColls := make(chan userIDCollsTuple)

	wp := workerpool.New(10)
	go func() {
		for u, addrs := range userIDs {
			userID := u
			addresses := addrs
			for i, addr := range addresses {
				if strings.ContainsAny(addr.String(), ".eth") {
					resolved, err := ens.Resolve(ethClient, addr.String())
					if err != nil {
						logrus.Errorf("Error resolving ens address %s: %s", addr.String(), err)
						continue
					}
					addresses[i] = persist.Wallet{
						Address: persist.NullString(strings.ToLower(resolved.Hex())),
						Chain:   persist.ChainETH,
					}
				}
			}
			wp.Submit(func() {
				c, cancel := context.WithTimeout(ctx, time.Minute*30)
				defer cancel()
				logrus.Infof("Processing user %s with addresses %v", userID, addresses)
				res, err := pgClient.QueryContext(c, `SELECT ID, NFTS FROM collections WHERE OWNER_USER_ID = $1 AND DELETED = false;`, userID)
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
						fullNFT, err := nftRepo.GetByID(c, nftID)
						if err != nil {
							logrus.Errorf("Error getting nft %s: %s", nftID, err)
							continue
						}
						if strings.ContainsAny(fullNFT.OwnerAddress.String(), ".eth") {
							addr, err := ens.Resolve(ethClient, fullNFT.OwnerAddress.String())
							if err != nil {
								logrus.Errorf("Error resolving ens address %s: %s", fullNFT.OwnerAddress.String(), err)
								continue
							}
							fullNFT.OwnerAddress = persist.EthereumAddress(strings.ToLower(addr.Hex()))
						}

						if fullNFT.Contract.ContractAddress == "" {
							logrus.Infof("NFT %s has no contract address", nftID)
							assets, err := opensea.FetchAssets(c, fullNFT.OwnerAddress, "", opensea.TokenID(fullNFT.OpenseaTokenID.String()), 0, 0, nil)
							if err != nil {
								logrus.Errorf("Error fetching contract address for NFT %s: %d assets found - err %s", nftID, len(assets), err)
							} else {
								matchingAsset, err := findMatchingAsset(assets, fullNFT)
								if err != nil {
									logrus.Errorf("Error finding matching asset for NFT %s: %s", nftID, err)
									err = opensea.UpdateAssetsForAcc(c, userID, addresses, nftRepo, userRepo, collRepo, galleryRepo, backupRepo)
									if err != nil {
										logrus.Errorf("Error updating assets for user %s: %s", userID, err)
									} else {
										fullNFT, err = nftRepo.GetByID(c, nftID)
										if err != nil {
											logrus.Errorf("Error fetching NFT %s after updating assets: %s", nftID, err)
										} else {
											if fullNFT.Contract.ContractAddress == "" {
												logrus.Errorf("NFT %s still has no contract address", nftID)

											}
										}
									}
								}
								logrus.Infof("Found contract address %s for NFT %s", matchingAsset.Contract.ContractAddress, nftID)
								fullNFT.Contract = matchingAsset.Contract

							}
						}

						if fullNFT.OpenseaTokenID == "" {
							assets, err := opensea.FetchAssets(c, fullNFT.OwnerAddress, fullNFT.Contract.ContractAddress, "", 0, 0, nil)
							if err != nil {
								logrus.Errorf("Error fetching token ID for NFT %s: %d assets found - err %s", nftID, len(assets), err)
							} else {
								matchingAsset, err := findMatchingAsset(assets, fullNFT)
								if err != nil {
									logrus.Errorf("Error finding matching asset for NFT %s: %s", nftID, err)
									err = opensea.UpdateAssetsForAcc(c, userID, addresses, nftRepo, userRepo, collRepo, galleryRepo, backupRepo)
									if err != nil {
										logrus.Errorf("Error updating assets for user %s: %s", userID, err)
									} else {
										fullNFT, err = nftRepo.GetByID(c, nftID)
										if err != nil {
											logrus.Errorf("Error fetching NFT %s after updating assets: %s", nftID, err)
										} else {
											if fullNFT.OpenseaTokenID == "" {
												logrus.Errorf("NFT %s still has no token ID", nftID)
											}
										}
									}
								}
								logrus.Infof("Found token ID %s for NFT %s", matchingAsset.TokenID.ToBase16(), nftID)
								fullNFT.OpenseaTokenID = persist.TokenID(matchingAsset.TokenID.ToBase16())
							}
						}

						var tokenEquivelents []persist.TokenGallery
						if fullNFT.OpenseaTokenID == "" && fullNFT.Contract.ContractAddress != "" {
							logrus.Warnf("NFT %s has no token ID and has a contract address", nftID)
							tokenEquivelents, err = tokenRepo.GetByContract(c, fullNFT.Contract.ContractAddress, -1, -1)
							if err != nil {
								if len(tokenEquivelents) == 0 {
									tokenEquivelents, err = tokenRepo.GetByWallet(c, fullNFT.OwnerAddress, -1, -1)
								}
							}
							if err == nil {
								tokenEquivelents = findMatchingTokens(tokenEquivelents, fullNFT)
							}
						} else if fullNFT.OpenseaTokenID != "" && fullNFT.Contract.ContractAddress == "" {
							tokenEquivelents, err = tokenRepo.GetByTokenID(c, fullNFT.OpenseaTokenID, -1, -1)
							if err != nil {
								asBase10, ok := big.NewInt(0).SetString(fullNFT.OpenseaTokenID.String(), 10)
								if ok {
									tokenEquivelents, err = tokenRepo.GetByTokenID(c, persist.TokenID(asBase10.Text(16)), -1, -1)
								}
								if len(tokenEquivelents) == 0 {
									tokenEquivelents, err = tokenRepo.GetByWallet(c, fullNFT.OwnerAddress, -1, -1)
								}
							}
							if err == nil {
								tokenEquivelents = findMatchingTokens(tokenEquivelents, fullNFT)
							}
						} else if fullNFT.OpenseaTokenID != "" && fullNFT.Contract.ContractAddress != "" {
							tokenEquivelents, err = tokenRepo.GetByTokenIdentifiers(c, fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, -1, -1)
							if err != nil {
								asBase10, ok := big.NewInt(0).SetString(fullNFT.OpenseaTokenID.String(), 10)
								if ok {
									tokenEquivelents, err = tokenRepo.GetByTokenIdentifiers(c, persist.TokenID(asBase10.Text(16)), fullNFT.Contract.ContractAddress, -1, -1)
								}
								if len(tokenEquivelents) == 0 {
									tokens, err := tokenRepo.GetByWallet(c, fullNFT.OwnerAddress, -1, -1)
									if err == nil {
										tokenEquivelents = findMatchingTokens(tokens, fullNFT)
									}
								}
							}
						} else {
							logrus.Errorf("NFT %s has no token ID and no contract address", nftID)
							continue
						}

						if err != nil {
							logrus.Warnf("Token equivalent not found for %s-%s in collection %s. Making token...", fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, coll)
							tokenEquivelents, err = nftToTokens(c, fullNFT, addresses, ethClient, ipfsClient, arweaveClient)
							if err != nil {
								logrus.Errorf("Error making token for %s-%s in collection %s: %s", fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, coll, err)
								continue
							}
							if len(tokenEquivelents) == 0 {
								logrus.Errorf("No token equivalent found for %s-%s in collection %s", fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, coll)
								continue
							}
							logrus.Warnf("Upserting token equivalent for %s-%s in collection %s: %+v", fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, coll, tokenEquivelents)
							err = tokenRepo.BulkUpsert(c, tokenEquivelents)
							if err != nil {
								logrus.Errorf("Error upserting token equivalents for %s-%s in collection %s: %s", fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, coll, err)
								continue
							}
							tokenEquivelents, err = tokenRepo.GetByTokenIdentifiers(c, fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, -1, -1)
							if err != nil {
								asBase10, _ := big.NewInt(0).SetString(fullNFT.OpenseaTokenID.String(), 10)
								tokenEquivelents, err = tokenRepo.GetByTokenIdentifiers(c, persist.TokenID(asBase10.Text(16)), fullNFT.Contract.ContractAddress, -1, -1)
								if err != nil {
									if len(tokenEquivelents) == 0 {
										tokens, err := tokenRepo.GetByWallet(c, fullNFT.OwnerAddress, -1, -1)
										if err == nil {
											tokenEquivelents = findMatchingTokens(tokens, fullNFT)
										}
									}
								}
							}
						}

						if len(tokenEquivelents) == 0 {
							logrus.Errorf("No token equivalent found for %s-%s in collection %s", fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, coll)
							continue
						}

						for _, token := range tokenEquivelents {
							if containsEthAddress(token.OwnerAddress, addresses) {
								logrus.Infof("token %s-%s is owned by %s", token.ContractAddress, token.TokenID, token.OwnerAddress)
								newCollsToNFTs[coll] = append(newCollsToNFTs[coll], token.ID)
							}
						}
					}
				}
				receivedColls <- userIDCollsTuple{userID, newCollsToNFTs}
			})
		}
	}()
	for i := 0; i < len(userIDs); i++ {
		select {
		case tuple := <-receivedColls:
			if tuple.newCollsToNFTs != nil && tuple.userID != "" {
				usersToNewCollections[tuple.userID] = tuple.newCollsToNFTs
			}
		case <-ctx.Done():
			panic("context cancelled")
		}
	}
	return usersToNewCollections
}

func getAllUsers(ctx context.Context, pgClient *sql.DB) map[persist.DBID][]persist.EthereumAddress {
	c, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	res, err := pgClient.QueryContext(c, `SELECT ID,ADDRESSES FROM users WHERE DELETED = false ORDER BY CREATED_AT DESC;`)
	if err != nil {
		panic(err)
	}

	result := map[persist.DBID][]persist.EthereumAddress{}
	for res.Next() {
		var id persist.DBID
		var addresses []persist.EthereumAddress
		if err = res.Scan(&id, pq.Array(&addresses)); err != nil {
			panic(err)
		}
		if _, ok := result[id]; !ok {
			result[id] = make([]persist.EthereumAddress, 0, 3)
		}
		result[id] = append(result[id], addresses...)
	}
	return result
}

func nftToTokens(ctx context.Context, nft persist.NFT, addresses []persist.EthereumAddress, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) ([]persist.TokenGallery, error) {

	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	allTokens := make([]persist.TokenGallery, 0, 5)
	asURI := persist.TokenURI(nft.ImageURL)
	media := persist.Media{}

	bs, err := rpc.GetDataFromURI(ctx, asURI, ipfsClient, arweaveClient)
	if err == nil {
		mediaType := persist.SniffMediaType(bs)
		if mediaType != persist.MediaTypeUnknown {
			media.MediaURL = persist.NullString(nft.ImageURL)
			media.ThumbnailURL = persist.NullString(nft.ImagePreviewURL)
			media.MediaType = mediaType
		}
	}

	uri := persist.TokenURI(nft.TokenMetadataURL.String()).ReplaceID(nft.OpenseaTokenID)
	metadata, _ := rpc.GetMetadataFromURI(ctx, uri, ipfsClient, arweaveClient)
	t := persist.TokenGallery{
		CollectorsNote:  nft.CollectorsNote,
		TokenMetadata:   metadata,
		Media:           media,
		TokenURI:        uri,
		Chain:           persist.ChainETH,
		TokenID:         nft.OpenseaTokenID,
		OwnerAddress:    nft.OwnerAddress,
		ContractAddress: nft.Contract.ContractAddress,
		BlockNumber:     persist.BlockNumber(block),
		ExternalURL:     nft.ExternalURL,
		Description:     nft.Description,
		Name:            nft.Name,
		Quantity:        "1",
	}
	switch nft.Contract.ContractSchemaName {
	case "ERC1155":
		t.TokenType = persist.TokenTypeERC1155
		ierc1155, err := contracts.NewIERC1155Caller(t.ContractAddress.Address(), ethClient)
		if err != nil {
			return nil, fmt.Errorf("error getting ERC1155 contract: %s", err)
		}
		for _, addr := range addresses {
			new := t
			bal, err := ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, addr.Address(), t.TokenID.BigInt())
			if err != nil {
				return nil, fmt.Errorf("error getting balance of %s for %s-%s: %s", addr.Address(), t.ContractAddress, t.TokenID, err)
			}
			if bal.Cmp(bigZero) > 0 {
				new.OwnerAddress = addr
				new.Quantity = persist.HexString(bal.Text(16))

				allTokens = append(allTokens, new)
			}
		}
	default:
		t.TokenType = persist.TokenTypeERC721
		t.OwnershipHistory = []persist.AddressAtBlock{
			{
				Address: persist.ZeroAddress,
				Block:   persist.BlockNumber(block - 1),
			},
		}
		allTokens = append(allTokens, t)
	}

	return allTokens, nil
}

func containsEthAddress(addr persist.EthereumAddress, addrs []persist.EthereumAddress) bool {
	for _, a := range addrs {
		if addr.String() == a.String() {
			return true
		}
	}
	return false
}

var errNoMatchingAsset = errors.New("no matching asset")

func findMatchingAsset(assets []opensea.Asset, pNFT persist.NFT) (opensea.Asset, error) {
	logrus.Infof("finding matching asset for %s-%s using %d assets", pNFT.Contract.ContractAddress, pNFT.OpenseaTokenID, len(assets))
	for _, a := range assets {
		switch {
		case a.ID == int(pNFT.OpenseaID.Int64()):
			return a, nil
		case a.TokenID.ToBase16() == pNFT.OpenseaTokenID.String() && a.Contract.ContractAddress.String() == pNFT.Contract.ContractAddress.String():
			return a, nil
		case a.Name == pNFT.Name.String() && a.Description == pNFT.Description.String():
			return a, nil
		case a.TokenMetadataURL == pNFT.TokenMetadataURL.String():
			return a, nil
		}
	}
	return opensea.Asset{}, errNoMatchingAsset
}
func findMatchingTokens(tokens []persist.TokenGallery, pNFT persist.NFT) []persist.TokenGallery {
	result := make([]persist.TokenGallery, 0, 10)
	logrus.Infof("finding matching asset for %s-%s using %d assets", pNFT.Contract.ContractAddress, pNFT.OpenseaTokenID, len(tokens))
	for _, t := range tokens {
		if t.OwnerAddress.String() != pNFT.OwnerAddress.String() {
			continue
		}
		switch {
		case t.TokenID.String() == pNFT.OpenseaTokenID.String() && t.ContractAddress.String() == pNFT.Contract.ContractAddress.String():
			result = append(result, t)
		case t.Name.String() == pNFT.Name.String():
			result = append(result, t)
		case t.TokenURI.String() == pNFT.TokenMetadataURL.String():
			result = append(result, t)
		case t.Media.MediaURL == pNFT.ImageURL, t.Media.MediaURL == pNFT.ImagePreviewURL, t.Media.MediaURL == pNFT.ImageOriginalURL, t.Media.MediaURL == pNFT.ImageThumbnailURL, t.Media.ThumbnailURL == pNFT.ImageThumbnailURL, t.Media.MediaURL == pNFT.ImageURL:
			result = append(result, t)
		case t.Media.MediaURL == pNFT.AnimationURL, t.Media.MediaURL == pNFT.AnimationOriginalURL:
			result = append(result, t)
		case t.ExternalURL == pNFT.ExternalURL:
			result = append(result, t)
		case t.Description.String() == pNFT.Description.String():
			result = append(result, t)
		}
	}
	return result
}
