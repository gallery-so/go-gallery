package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	progressbar "github.com/schollz/progressbar/v3"
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

	logrus.Info("Full migration...")

	// Users migration
	// logrus.Info("Copying users to temp table...")
	// if err := copyUsersToTempTable(pgClient); err != nil {
	// 	panic(err)
	// }
	// logrus.Info("Copying users to temp table... Done")

	// logrus.Info("Clearing addresses column in users table...")
	// if err := clearAddressesColumn(pgClient); err != nil {
	// 	panic(err)
	// }
	// logrus.Info("Clearing addresses column in users table... Done")

	// logrus.Info("Getting all users wallets...")
	// idsToAddresses, err := getAllUsersWallets(pgClient)
	// if err != nil {
	// 	panic(err)
	// }
	// logrus.Info("Getting all users wallets... Done")
	// logrus.Infof("Found %d users", len(idsToAddresses))

	// logrus.Info("Creating wallets and addresses in DB and adding them to users...")
	// if err := createWalletAndAddresses(pgClient, idsToAddresses); err != nil {
	// 	panic(err)
	// }
	// logrus.Info("Creating wallets and addresses in DB and adding them to users... Done")

	// NFTs migration

	nftsChan := make(chan persist.NFT)

	go func() {
		logrus.Info("Migrating NFTs...")
		if err := migrateNFTs(pgClient, rpc.NewEthClient(), nftsChan); err != nil {
			panic(err)
		}
		logrus.Info("Migrating NFTs... Done")
	}()
	logrus.Info("Getting all NFTs...")
	err := getAllNFTs(pgClient, nftsChan)
	if err != nil {
		panic(err)
	}
	logrus.Info("Getting all NFTs... Done")

	logrus.Info("Full migration... Done")
}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")

	viper.AutomaticEnv()
}

func copyUsersToTempTable(pg *sql.DB) error {
	_, err := pg.Exec(`
		DROP TABLE IF EXISTS temp_users;
		CREATE TABLE temp_users AS
			SELECT * FROM users;
	`)
	if err != nil {
		return err
	}
	return nil
}

func getAllUsersWallets(pg *sql.DB) (map[persist.DBID][]persist.AddressValue, error) {

	rows, err := pg.Query(`SELECT ID,ADDRESSES FROM temp_users;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	idToAddress := make(map[persist.DBID][]persist.AddressValue)
	for rows.Next() {
		var id persist.DBID
		var addresses []persist.AddressValue
		err := rows.Scan(&id, pq.Array(&addresses))
		if err != nil {
			return nil, err
		}
		idToAddress[id] = addresses
	}
	return idToAddress, nil
}

func clearAddressesColumn(pg *sql.DB) error {
	_, err := pg.Exec(`UPDATE users SET ADDRESSES = $1;`, []persist.DBID{})
	if err != nil {
		return err
	}
	return nil
}

func createWalletAndAddresses(pg *sql.DB, idsToAddresses map[persist.DBID][]persist.AddressValue) error {
	bar := progressbar.Default(int64(len(idsToAddresses)), "Creating Wallets")
	for id, addresses := range idsToAddresses {
		tx, err := pg.Begin()
		if err != nil {
			return err
		}
		err = func() error {
			defer tx.Rollback()
			userWallets := make([]persist.DBID, len(addresses))
			for i, address := range addresses {
				var addressID persist.DBID
				_, err = tx.Exec(`INSERT INTO addresses (ID,VERSION,ADDRESS_VALUE,CHAIN) VALUES ($1,$2,$3,$4) ON CONFLICT (ADDRESS_VALUE,CHAIN) DO NOTHING;`, persist.GenerateID(), 0, address, persist.ChainETH)
				if err != nil {
					return err
				}
				err = tx.QueryRow(`SELECT ID FROM addresses WHERE ADDRESS_VALUE = $1 AND CHAIN = $2;`, address, persist.ChainETH).Scan(&addressID)
				if err != nil {
					return err
				}
				var walletID persist.DBID
				_, err = tx.Exec(`INSERT INTO wallets (ID,VERSION,ADDRESS,WALLET_TYPE) VALUES ($1,$2,$3,$4) ON CONFLICT (ADDRESS) DO NOTHING;`, persist.GenerateID(), 0, addressID, persist.WalletTypeEOA)
				if err != nil {
					return err
				}
				err = tx.QueryRow(`SELECT ID FROM wallets WHERE ADDRESS = $1;`, addressID).Scan(&walletID)
				if err != nil {
					return err
				}

				userWallets[i] = walletID
			}
			_, err = tx.Exec(`UPDATE users SET ADDRESSES = $1 WHERE ID = $2;`, userWallets, id)
			if err != nil {
				return err
			}
			err = tx.Commit()
			if err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			return err
		}
		bar.Add(1)
	}
	return nil
}

func getAllNFTs(pg *sql.DB, nftsChan chan<- persist.NFT) error {
	rows, err := pg.Query(`SELECT ID,DELETED,VERSION,CREATED_AT,LAST_UPDATED,NAME,DESCRIPTION,EXTERNAL_URL,CREATOR_ADDRESS,CREATOR_NAME,OWNER_ADDRESS,MULTIPLE_OWNERS,CONTRACT,OPENSEA_ID,OPENSEA_TOKEN_ID,IMAGE_URL,IMAGE_THUMBNAIL_URL,IMAGE_PREVIEW_URL,IMAGE_ORIGINAL_URL,ANIMATION_URL,ANIMATION_ORIGINAL_URL,TOKEN_COLLECTION_NAME,COLLECTORS_NOTE FROM nfts;`)
	if err != nil {
		return err
	}
	defer rows.Close()
	defer close(nftsChan)

	for rows.Next() {
		var nft persist.NFT
		err := rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.CreationTime, &nft.LastUpdatedTime, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.TokenCollectionName, &nft.CollectorsNote)
		if err != nil {
			return err
		}
		nftsChan <- nft
	}
	return nil
}

func migrateNFTs(pg *sql.DB, ethClient *ethclient.Client, nfts chan persist.NFT) error {
	ctx := context.Background()
	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return err
	}
	tokenRepo := postgres.NewTokenGalleryRepository(pg, nil)
	for nft := range nfts {
		toUpsert, err := nftToToken(ctx, pg, nft, block)
		if err != nil {
			return err
		}
		if err := tokenRepo.Upsert(ctx, toUpsert); err != nil {
			return err
		}
	}
	return nil
}

// a function that will split an array of NFTs into chunks of size n
func splitNFTs(n int, nfts []persist.NFT) [][]persist.NFT {
	var chunks [][]persist.NFT
	for i := 0; i < len(nfts); i += n {
		end := i + n
		if end > len(nfts) {
			end = len(nfts)
		}
		chunks = append(chunks, nfts[i:end])
	}
	return chunks
}

func nftsToTokens(pg *sql.DB, ethClient *ethclient.Client, nfts []persist.NFT) ([]persist.TokenGallery, error) {
	bar := progressbar.Default(int64(len(nfts)), "Converting NFTs to Tokens")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()
	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Current block number: %d", block)
	tokens := make([]persist.TokenGallery, len(nfts))
	tokensChan := make(chan persist.TokenGallery)
	errChan := make(chan error)
	for _, n := range nfts {
		go func(nft persist.NFT) {
			token, err := nftToToken(ctx, pg, nft, block)
			if err != nil {
				errChan <- err
				return
			}
			tokensChan <- token
		}(n)
	}
	for i := range nfts {
		select {
		case err := <-errChan:
			return nil, err
		case token := <-tokensChan:
			tokens[i] = token
		}
		bar.Add(1)
	}
	return dedupeTokens(tokens), nil
}

func nftToToken(ctx context.Context, pg *sql.DB, nft persist.NFT, block uint64) (persist.TokenGallery, error) {
	var tokenType persist.TokenType
	switch nft.Contract.ContractSchemaName {
	case "ERC1155":
		tokenType = persist.TokenTypeERC1155
	default:
		tokenType = persist.TokenTypeERC721
	}

	metadata := persist.TokenMetadata{
		"name":          nft.Name,
		"description":   nft.Description,
		"image_url":     nft.ImageOriginalURL,
		"animation_url": nft.AnimationOriginalURL,
	}

	med := persist.Media{ThumbnailURL: persist.NullString(nft.ImageThumbnailURL)}
	switch {
	case nft.AnimationURL != "":
		med.MediaURL = persist.NullString(nft.AnimationURL)
		med.MediaType = media.PredictMediaType(ctx, nft.AnimationURL.String())
	case nft.ImageURL != "":
		med.MediaURL = persist.NullString(nft.ImageURL)
		med.MediaType = media.PredictMediaType(ctx, nft.ImageURL.String())
	default:
		med.MediaURL = persist.NullString(nft.ImageThumbnailURL)
		med.MediaType = media.PredictMediaType(ctx, nft.ImageThumbnailURL.String())
	}

	var ownerAddressID persist.DBID
	err := pg.QueryRow(`SELECT ID FROM addresses WHERE ADDRESS_VALUE = $1;`, nft.OwnerAddress).Scan(&ownerAddressID)
	if err != nil && err != sql.ErrNoRows {
		return persist.TokenGallery{}, err
	}

	var walletWithAddressID persist.DBID
	err = pg.QueryRow(`SELECT ID FROM wallets WHERE ADDRESS = $1;`, nft.OwnerAddress).Scan(&walletWithAddressID)
	if err != nil && err != sql.ErrNoRows {
		return persist.TokenGallery{}, err
	}

	var ownerUserID persist.DBID
	err = pg.QueryRow(`SELECT ID FROM users WHERE $1 = ANY(ADDRESSES);`, walletWithAddressID).Scan(&ownerUserID)
	if err != nil && err != sql.ErrNoRows {
		return persist.TokenGallery{}, err
	}

	var contractAddressID persist.DBID
	err = pg.QueryRow(`SELECT ID FROM addresses WHERE ADDRESS_VALUE = $1;`, nft.Contract.ContractAddress).Scan(&contractAddressID)
	if err != nil && err != sql.ErrNoRows {
		return persist.TokenGallery{}, err
	}

	token := persist.TokenGallery{
		TokenType:       tokenType,
		Name:            nft.Name,
		Description:     nft.Description,
		Version:         0,
		CollectorsNote:  nft.CollectorsNote,
		Chain:           persist.ChainETH,
		OwnerAddresses:  []persist.Address{{ID: ownerAddressID}},
		TokenURI:        persist.TokenURI(nft.TokenMetadataURL),
		TokenID:         nft.OpenseaTokenID,
		OwnerUserID:     ownerUserID,
		ContractAddress: persist.Address{ID: contractAddressID},
		ExternalURL:     nft.ExternalURL,
		BlockNumber:     persist.BlockNumber(block),
		TokenMetadata:   metadata,
		Media:           med,
	}
	return token, nil
}

func upsertTokens(pg *sql.DB, tokens []persist.TokenGallery) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	paramsPerRow := 20
	rowsPerQuery := 65535 / paramsPerRow

	if len(tokens) > rowsPerQuery {
		logrus.Debugf("Chunking %d tokens recursively into %d queries", len(tokens), len(tokens)/rowsPerQuery)
		next := tokens[rowsPerQuery:]
		current := tokens[:rowsPerQuery]
		if err := upsertTokens(pg, next); err != nil {
			return err
		}
		tokens = current
	}

	sqlStr := `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNER_ADDRESSES,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES `
	vals := make([]interface{}, 0, len(tokens)*paramsPerRow)
	for i, token := range tokens {
		sqlStr += generateValuesPlaceholders(paramsPerRow, i*paramsPerRow) + ","
		vals = append(vals, persist.GenerateID(), token.CollectorsNote, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerUserID, token.OwnerAddresses, token.OwnershipHistory, token.TokenMetadata, token.ContractAddress, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated)
	}

	sqlStr = sqlStr[:len(sqlStr)-1]

	sqlStr += ` ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS,OWNER_USER_ID) DO UPDATE SET MEDIA = EXCLUDED.MEDIA,TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_USER_ID = EXCLUDED.OWNER_USER_ID,OWNER_ADDRESSES = EXCLUDED.OWNER_ADDRESSES,OWNERSHIP_HISTORY = tokens.OWNERSHIP_HISTORY || EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED WHERE EXCLUDED.BLOCK_NUMBER > tokens.BLOCK_NUMBER`

	_, err := pg.ExecContext(ctx, sqlStr, vals...)
	if err != nil {
		return fmt.Errorf("failed to upsert tokens: %w", err)
	}

	return nil

}

func dedupeTokens(pTokens []persist.TokenGallery) []persist.TokenGallery {
	seen := map[string]persist.TokenGallery{}
	for _, token := range pTokens {
		key := token.ContractAddress.String() + "-" + token.TokenID.String() + "-" + token.OwnerUserID.String()
		if seenToken, ok := seen[key]; ok {
			if seenToken.BlockNumber.Uint64() > token.BlockNumber.Uint64() {
				continue
			}
			seen[key] = token
		}
		seen[key] = token
	}
	result := make([]persist.TokenGallery, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
	}
	return result
}
func generateValuesPlaceholders(l, offset int) string {
	values := "("
	for i := 0; i < l; i++ {
		values += fmt.Sprintf("$%d,", i+1+offset)
	}
	return values[0:len(values)-1] + ")"
}
