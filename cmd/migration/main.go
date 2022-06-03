package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gammazero/workerpool"
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

	if err := copyBack(pgClient); err != nil {
		panic(err)
	}

	if err := copyUsersToTempTable(pgClient); err != nil {
		panic(err)
	}

	logrus.Info("Getting all users wallets...")
	idsToAddresses, err := getAllUsersWallets(pgClient)
	if err != nil {
		panic(err)
	}
	logrus.Info("Getting all users wallets... Done")
	logrus.Infof("Found %d users", len(idsToAddresses))

	logrus.Info("Creating wallets and addresses in DB and adding them to users...")
	if err := createWalletAndAddresses(pgClient, idsToAddresses); err != nil {
		panic(err)
	}
	logrus.Info("Creating wallets and addresses in DB and adding them to users... Done")

	// NFTs migration

	nftsChan := make(chan persist.NFT)

	go func() {
		logrus.Info("Getting all NFTs...")
		if err := getAllNFTs(pgClient, nftsChan); err != nil {
			panic(err)
		}
		logrus.Info("Getting all NFTs... Done")
	}()
	logrus.Info("Migrating NFTs...")
	if err := migrateNFTs(pgClient, rpc.NewEthClient(), nftsChan); err != nil {
		panic(err)
	}
	logrus.Info("Migrating NFTs... Done")

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

func copyBack(pg *sql.DB) error {
	var sel int
	pg.QueryRow(`SELECT 1 FROM temp_users WHERE CARDINALITY(WALLETS) > 0;`).Scan(&sel)
	if sel > 0 {
		logrus.Info("Found temp_users with addresses... copying back to original table")
		_, err := pg.Exec(`
		UPDATE users u SET WALLETS = (SELECT WALLETS FROM temp_users WHERE ID = u.ID)
	`)
		if err != nil {
			return err
		}
	}
	return nil
}

func copyUsersToTempTable(pg *sql.DB) error {
	var sel int
	err := pg.QueryRow(`SELECT 1 FROM temp_users WHERE DELETED = false;`).Scan(&sel)
	if err != nil && sel == 0 {
		logrus.Info("Copying users to temp table...")
		_, err = pg.Exec(`
		CREATE TABLE temp_users AS
			SELECT * FROM users;
		`)
		if err != nil {
			return err
		}
	}
	return nil
}

func getAllUsersWallets(pg *sql.DB) (map[persist.DBID][]persist.Address, error) {

	rows, err := pg.Query(`SELECT ID,WALLETS FROM temp_users;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	idToAddress := make(map[persist.DBID][]persist.Address)
	for rows.Next() {
		var id persist.DBID
		var addresses []persist.Address
		err := rows.Scan(&id, pq.Array(&addresses))
		if err != nil {
			return nil, err
		}
		idToAddress[id] = addresses
	}
	return idToAddress, nil
}

func createWalletAndAddresses(pg *sql.DB, idsToAddresses map[persist.DBID][]persist.Address) error {
	pg.Exec(`TRUNCATE wallets;`)
	bar := progressbar.Default(int64(len(idsToAddresses)), "Creating Wallets")
	for id, addresses := range idsToAddresses {

		userWallets := make([]persist.DBID, len(addresses))
		for i, address := range addresses {
			walletID := persist.GenerateID()
			_, err := pg.Exec(`INSERT INTO wallets (ID,VERSION,ADDRESS,WALLET_TYPE,CHAIN) VALUES ($1,$2,$3,$4,0) ON CONFLICT DO NOTHING;`, walletID, 0, address, persist.WalletTypeEOA)
			if err != nil {
				return err
			}

			userWallets[i] = walletID
		}
		_, err := pg.Exec(`UPDATE users SET WALLETS = $1 WHERE ID = $2;`, userWallets, id)
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

func migrateNFTs(pg *sql.DB, ethClient *ethclient.Client, nfts <-chan persist.NFT) error {
	ctx := context.Background()
	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return err
	}

	toUpsertChan := make(chan persist.TokenGallery)
	errChan := make(chan error)
	go func() {
		defer close(toUpsertChan)
		wp := workerpool.New(1000)
		for nft := range nfts {
			n := nft
			f := func() {
				toUpsert, err := nftToToken(ctx, pg, n, block)
				if err != nil {
					errChan <- err
					return
				}
				toUpsertChan <- toUpsert
			}

			wp.Submit(f)
		}
		wp.StopWait()
	}()

	perUpsert := 2000

	tokens := make([]persist.TokenGallery, perUpsert)
	i := 0
	j := 0
	bar := progressbar.Default(int64(perUpsert), "Prepping Upsert")

	for {
		select {
		case toUpsert, ok := <-toUpsertChan:
			if i == perUpsert || !ok {
				err = upsertTokens(pg, dedupeTokens(tokens))
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
				tokens = make([]persist.TokenGallery, perUpsert)
				i = 0
				j++
				bar = progressbar.Default(int64(perUpsert), fmt.Sprintf("Prepping Upsert"))
				logrus.Infof("Upserted NFTs %d", j)
			}
			tokens[i] = toUpsert
			bar.Add(1)
			i++
		case err := <-errChan:
			return err
		}
	}
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
	var quantity persist.HexString
	switch nft.Contract.ContractSchemaName {
	case "ERC1155":
		tokenType = persist.TokenTypeERC1155
		quantity = "1"
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
	var err error
	switch {
	case nft.AnimationURL != "":
		med.MediaURL = persist.NullString(nft.AnimationURL)
		med.MediaType, err = media.PredictMediaType(ctx, nft.AnimationURL.String())

	case nft.AnimationOriginalURL != "":
		med.MediaURL = persist.NullString(nft.AnimationOriginalURL)
		med.MediaType, _ = media.PredictMediaType(ctx, nft.AnimationOriginalURL.String())

	case nft.ImageURL != "":
		med.MediaURL = persist.NullString(nft.ImageURL)
		med.MediaType, _ = media.PredictMediaType(ctx, nft.ImageURL.String())
	case nft.ImageOriginalURL != "":
		med.MediaURL = persist.NullString(nft.ImageOriginalURL)
		med.MediaType, _ = media.PredictMediaType(ctx, nft.ImageOriginalURL.String())

	default:
		med.MediaURL = persist.NullString(nft.ImageThumbnailURL)
		med.MediaType, _ = media.PredictMediaType(ctx, nft.ImageThumbnailURL.String())
	}
	if err != nil {
		logrus.Infof("Error predicting media type for %v: %s", nft, err)
	}

	var walletID persist.DBID
	err = pg.QueryRow(`SELECT ID FROM wallets WHERE ADDRESS = $1;`, nft.OwnerAddress).Scan(&walletID)
	if err != nil && err != sql.ErrNoRows {
		return persist.TokenGallery{}, err
	}

	var ownerUserID persist.DBID
	err = pg.QueryRow(`SELECT ID FROM users WHERE $1 = ANY(WALLETS);`, walletID).Scan(&ownerUserID)
	if err != nil && err != sql.ErrNoRows {
		return persist.TokenGallery{}, err
	}

	token := persist.TokenGallery{
		ID:               nft.ID,
		TokenType:        tokenType,
		Name:             nft.Name,
		Description:      nft.Description,
		Version:          0,
		Quantity:         quantity,
		OwnershipHistory: []persist.AddressAtBlock{},
		CollectorsNote:   nft.CollectorsNote,
		Chain:            persist.ChainETH,
		OwnedByWallets:   []persist.Wallet{{ID: walletID}},
		TokenURI:         persist.TokenURI(nft.TokenMetadataURL),
		TokenID:          nft.OpenseaTokenID,
		OwnerUserID:      ownerUserID,
		ContractAddress:  persist.Address(nft.Contract.ContractAddress),
		ExternalURL:      nft.ExternalURL,
		BlockNumber:      persist.BlockNumber(block),
		TokenMetadata:    metadata,
		Media:            med,
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

	sqlStr := `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES `
	vals := make([]interface{}, 0, len(tokens)*paramsPerRow)
	for i, token := range tokens {
		sqlStr += generateValuesPlaceholders(paramsPerRow, i*paramsPerRow) + ","
		vals = append(vals, token.ID, token.CollectorsNote, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerUserID, token.OwnedByWallets, token.OwnershipHistory, token.TokenMetadata, token.ContractAddress, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated)
	}

	sqlStr = sqlStr[:len(sqlStr)-1]

	sqlStr += ` ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS,OWNER_USER_ID) DO UPDATE SET MEDIA = EXCLUDED.MEDIA,TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_USER_ID = EXCLUDED.OWNER_USER_ID,OWNED_BY_WALLETS = EXCLUDED.OWNED_BY_WALLETS,OWNERSHIP_HISTORY = tokens.OWNERSHIP_HISTORY || EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED WHERE EXCLUDED.BLOCK_NUMBER > tokens.BLOCK_NUMBER`

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
