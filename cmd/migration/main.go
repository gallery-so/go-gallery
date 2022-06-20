package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
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
var badMedias int64 = 0

func main() {
	setDefaults()
	run()
}

func run() {

	start := time.Now()

	pgClient := postgres.NewClient()

	logrus.Info("Full migration...")

	// Users migration

	// if err := copyBack(pgClient); err != nil {
	// 	panic(err)
	// }

	// if err := copyUsersToTempTable(pgClient); err != nil {
	// 	panic(err)
	// }

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

	// NFTs migration

	logrus.Info("Creating wallets and addresses in DB and adding them to users... Done")

	var count int
	pgClient.QueryRow("SELECT COUNT(*) FROM nfts;").Scan(&count)
	logrus.Infof("Found %d NFTs", count)
	nftsChan := make(chan persist.NFT)

	allUserIDs, err := getAllUserIDs(pgClient)
	if err != nil {
		panic(err)
	}

	go func() {
		logrus.Info("Getting all NFTs...")
		if err := getAllNFTs(pgClient, allUserIDs, nftsChan); err != nil {
			panic(err)
		}
		logrus.Info("Getting all NFTs... Done")
	}()
	logrus.Info("Migrating NFTs...")
	if err := migrateNFTs(pgClient, rpc.NewEthClient(), nftsChan); err != nil {
		panic(err)
	}
	logrus.Info("Migrating NFTs... Done")
	logrus.Infof("Full migration... Done in %s", time.Since(start))
	logrus.Infof("Found %d bad NFTs", badMedias)
}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	viper.SetDefault("REDIS_URL", "localhost:6379")

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

func getAllUserIDs(pg *sql.DB) ([]persist.DBID, error) {
	rows, err := pg.Query(`SELECT ID FROM users WHERE DELETED = false;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []persist.DBID
	for rows.Next() {
		var id persist.DBID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
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

func getAllNFTs(pg *sql.DB, users []persist.DBID, nftsChan chan<- persist.NFT) error {
	defer close(nftsChan)
	for _, user := range users {
		err := func() error {
			rows, err := pg.Query(`SELECT n.ID,n.DELETED,n.VERSION,n.CREATED_AT,n.LAST_UPDATED,n.NAME,n.DESCRIPTION,n.EXTERNAL_URL,n.CREATOR_ADDRESS,n.CREATOR_NAME,n.OWNER_ADDRESS,n.MULTIPLE_OWNERS,n.CONTRACT,n.OPENSEA_ID,n.OPENSEA_TOKEN_ID,n.IMAGE_URL,n.IMAGE_THUMBNAIL_URL,n.IMAGE_PREVIEW_URL,n.IMAGE_ORIGINAL_URL,n.ANIMATION_URL,n.ANIMATION_ORIGINAL_URL,n.TOKEN_COLLECTION_NAME,n.COLLECTORS_NOTE FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll AND c.DELETED = false
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM nfts n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE g.OWNER_USER_ID = $1 AND g.DELETED = false ORDER BY coll_ord,n.nft_ord;`, user)
			if err != nil {
				return err
			}

			defer rows.Close()

			for rows.Next() {
				var nft persist.NFT
				err := rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.CreationTime, &nft.LastUpdatedTime, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.TokenCollectionName, &nft.CollectorsNote)
				if err != nil {
					return err
				}
				nftsChan <- nft
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

type contractUpsert struct {
	contractAddress string
	contractName    string
	contractSymbol  string
	creatorAddress  string
	backChan        chan persist.DBID
}

func migrateNFTs(pg *sql.DB, ethClient *ethclient.Client, nfts <-chan persist.NFT) error {

	pg.Exec(`TRUNCATE tokens;`)

	ctx := context.Background()
	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return err
	}

	toUpsertChan := make(chan persist.TokenGallery)
	contractsChan := make(chan contractUpsert)
	errChan := make(chan error)
	wp := workerpool.New(500)
	go func() {
		defer close(toUpsertChan)
		contracts := &sync.Map{}
		for nft := range nfts {
			n := nft
			f := func() {
				innerCtx, cancel := context.WithTimeout(ctx, time.Second*30)
				defer cancel()
				defer func() {
					if r := recover(); r != nil {
						errChan <- fmt.Errorf("panic inside goroutine: %v - nft: %s", r, n.ID)
					}
				}()
				normalized := n.Contract.ContractAddress.String()
				contractID, ok := contracts.Load(normalized)
				if !ok {
					var newContractID persist.DBID
					err := pg.QueryRow(`SELECT ID FROM contracts WHERE ADDRESS = $1 AND CHAIN = 0;`, normalized).Scan(&newContractID)
					if err != nil {
						backChan := make(chan persist.DBID)
						toUpsertContract := contractUpsert{
							contractAddress: normalized,
							contractName:    n.Contract.ContractName.String(),
							contractSymbol:  n.Contract.ContractSymbol.String(),
							creatorAddress:  n.CreatorAddress.String(),
							backChan:        backChan,
						}
						contractsChan <- toUpsertContract
						contractID = <-backChan
					}
					contractID = newContractID
					contracts.Store(normalized, contractID)
				}

				token, err := nftToToken(innerCtx, pg, n, contractID.(persist.DBID), block)
				if err != nil {
					errChan <- err
					return
				}
				toUpsertChan <- token
			}

			wp.Submit(f)
		}
		wp.StopWait()
	}()

	perUpsert := 1000

	tokens := make([]persist.TokenGallery, perUpsert)
	i := 0
	j := 0
	bar := progressbar.Default(int64(perUpsert), "Prepping Upsert")

	for {
		select {
		case toUpsert, ok := <-toUpsertChan:
			if i == perUpsert || !ok {
				deduped := dedupeTokens(tokens)
				err = upsertTokens(pg, deduped)
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
		case contractUpsert := <-contractsChan:
			func() {
				defer close(contractUpsert.backChan)
				_, err := pg.Exec(`INSERT INTO contracts (ID,ADDRESS,NAME,SYMBOL,CREATOR_ADDRESS,CHAIN) VALUES ($1,$2,$3,$4,$5,0) ON CONFLICT (ADDRESS,CHAIN) DO UPDATE SET NAME = EXCLUDED.NAME, SYMBOL = EXCLUDED.SYMBOL, CREATOR_ADDRESS = EXCLUDED.CREATOR_ADDRESS;`, persist.GenerateID(), contractUpsert.contractAddress, contractUpsert.contractName, contractUpsert.contractSymbol, contractUpsert.creatorAddress)
				if err != nil {
					logrus.Errorf("error inserting contract %s: %s", contractUpsert.contractAddress, err)
				}
				var contractID persist.DBID
				if err := pg.QueryRow(`SELECT ID FROM contracts WHERE ADDRESS = $1 AND CHAIN = 0;`, contractUpsert.contractAddress).Scan(&contractID); err != nil {
					logrus.Errorf("error retrieving contract %s: %s", contractUpsert.contractAddress, err)
				}
				contractUpsert.backChan <- contractID
			}()
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

func nftToToken(ctx context.Context, pg *sql.DB, nft persist.NFT, contractID persist.DBID, block uint64) (persist.TokenGallery, error) {
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

	med := persist.Media{ThumbnailURL: persist.NullString(firstNonEmptyString(nft.ImageURL.String(), nft.ImagePreviewURL.String(), nft.ImageThumbnailURL.String()))}
	var err error
	switch {
	case nft.AnimationURL != "":
		med.MediaURL = persist.NullString(nft.AnimationURL)
		med.MediaType, err = media.PredictMediaType(ctx, nft.AnimationURL.String())

	case nft.AnimationOriginalURL != "":
		med.MediaURL = persist.NullString(nft.AnimationOriginalURL)
		med.MediaType, err = media.PredictMediaType(ctx, nft.AnimationOriginalURL.String())

	case nft.ImageURL != "":
		med.MediaURL = persist.NullString(nft.ImageURL)
		med.MediaType, err = media.PredictMediaType(ctx, nft.ImageURL.String())
	case nft.ImageOriginalURL != "":
		med.MediaURL = persist.NullString(nft.ImageOriginalURL)
		med.MediaType, err = media.PredictMediaType(ctx, nft.ImageOriginalURL.String())

	default:
		med.MediaURL = persist.NullString(nft.ImageThumbnailURL)
		med.MediaType, err = media.PredictMediaType(ctx, nft.ImageThumbnailURL.String())
	}
	if err != nil {
		atomic.AddInt64(&badMedias, 1)
		// logrus.Errorf("error predicting media type for %s: %s", med.MediaURL, err)
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
		Quantity:         "1",
		OwnershipHistory: []persist.AddressAtBlock{},
		CollectorsNote:   nft.CollectorsNote,
		Chain:            persist.ChainETH,
		OwnedByWallets:   []persist.Wallet{{ID: walletID}},
		TokenURI:         persist.TokenURI(nft.TokenMetadataURL),
		TokenID:          nft.OpenseaTokenID,
		OwnerUserID:      ownerUserID,
		Contract:         contractID,
		ExternalURL:      nft.ExternalURL,
		BlockNumber:      persist.BlockNumber(block),
		TokenMetadata:    metadata,
		Media:            med,
		CreationTime:     nft.CreationTime,
		Deleted:          nft.Deleted,
		LastUpdated:      nft.LastUpdatedTime,
	}
	return token, nil
}

func communityToContract(community persist.Community) persist.ContractGallery {
	return persist.ContractGallery{
		Chain:          community.Chain,
		Address:        community.ContractAddress,
		Name:           community.Name,
		CreatorAddress: community.CreatorAddress,
	}
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

	sqlStr := `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES `
	vals := make([]interface{}, 0, len(tokens)*paramsPerRow)
	for i, token := range tokens {
		sqlStr += generateValuesPlaceholders(paramsPerRow, i*paramsPerRow) + ","
		vals = append(vals, token.ID, token.CollectorsNote, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerUserID, token.OwnedByWallets, token.OwnershipHistory, token.TokenMetadata, token.Contract, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated)
	}

	sqlStr = sqlStr[:len(sqlStr)-1]

	sqlStr += ` ON CONFLICT DO NOTHING;`

	_, err := pg.ExecContext(ctx, sqlStr, vals...)
	if err != nil {
		return fmt.Errorf("failed to upsert tokens: %w", err)
	}

	return nil

}

type tokenUniqueIdentifiers struct {
	TokenID     persist.TokenID
	Chain       persist.Chain
	Contract    persist.DBID
	OwnerUserID persist.DBID
}

func dedupeTokens(pTokens []persist.TokenGallery) []persist.TokenGallery {
	seen := map[tokenUniqueIdentifiers]persist.TokenGallery{}
	for _, token := range pTokens {
		key := tokenUniqueIdentifiers{TokenID: token.TokenID, Chain: token.Chain, Contract: token.Contract, OwnerUserID: token.OwnerUserID}
		if seenToken, ok := seen[key]; ok {
			if seenToken.BlockNumber.Uint64() > token.BlockNumber.Uint64() {
				continue
			}
			seen[key] = token
		} else {
			seen[key] = token
		}
	}
	seenIDs := map[persist.DBID]bool{}
	result := []persist.TokenGallery{}
	i := 0
	for _, v := range seen {
		if _, ok := seenIDs[v.ID]; !ok {
			seenIDs[v.ID] = true
			result = append(result, v)
		}
		i++
	}
	return result
}

func upsertContracts(pg *sql.DB, pContracts []persist.ContractGallery) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	if len(pContracts) == 0 {
		return nil
	}
	sqlStr := `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,CREATOR_ADDRESS,CHAIN) VALUES `
	vals := make([]interface{}, 0, len(pContracts)*7)
	for i, contract := range pContracts {
		sqlStr += generateValuesPlaceholders(7, i*7)
		vals = append(vals, persist.GenerateID(), 0, contract.Address, contract.Symbol, contract.Name, contract.CreatorAddress, contract.Chain)
		sqlStr += ","
	}
	sqlStr = sqlStr[:len(sqlStr)-1]
	sqlStr += ` ON CONFLICT (ADDRESS, CHAIN) DO UPDATE SET SYMBOL = EXCLUDED.SYMBOL,NAME = EXCLUDED.NAME,CREATOR_ADDRESS = EXCLUDED.CREATOR_ADDRESS,CHAIN = EXCLUDED.CHAIN;`
	_, err := pg.ExecContext(ctx, sqlStr, vals...)
	if err != nil {
		return fmt.Errorf("error bulk upserting contracts: %v - SQL: %s -- VALS: %+v", err, sqlStr, vals)
	}

	return nil
}
func dedupeContracts(pContracts []persist.ContractGallery) []persist.ContractGallery {
	seen := map[persist.Address]persist.ContractGallery{}
	for _, contract := range pContracts {
		seen[contract.Address] = contract
	}
	result := make([]persist.ContractGallery, 0, len(seen))
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

func firstNonEmptyString(strings ...string) string {
	for _, s := range strings {
		if s != "" {
			return s
		}
	}
	return ""
}
