package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

// CommunityTokenRepository represents a repository for interacting with persisted communities
type CommunityTokenRepository struct {
	cache memstore.Cache
	db    *sql.DB

	getInfoStmt            *sql.Stmt
	getUserByAddressStmt   *sql.Stmt
	getContractStmt        *sql.Stmt
	getWalletByDetailsStmt *sql.Stmt
	getPreviewNFTsStmt     *sql.Stmt
}

// NewCommunityTokenRepository returns a new CommunityRepository
func NewCommunityTokenRepository(db *sql.DB, cache memstore.Cache) *CommunityTokenRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// TODO-EZRA: I think this needs to be updated
	getInfoStmt, err := db.PrepareContext(ctx,
		`SELECT n.OWNER_ADDRESS,n.DESCRIPTION,n.MEDIA
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM nfts n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE n.CONTRACT_ADDRESS = $1 AND g.DELETED = false AND c.DELETED = false AND n.DELETED = false ORDER BY coll_ord,n.nft_ord;`,
	)
	checkNoErr(err)

	getUserByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,USERNAME FROM users WHERE ADDRESSES @> ARRAY[$1]:: varchar[] AND DELETED = false`)
	checkNoErr(err)

	getUserByWalletIDStmt, err := db.PrepareContext(ctx, `SELECT ID,USERNAME FROM users WHERE WALLETS @> ARRAY[$1]:: varchar[] AND DELETED = false`)
	checkNoErr(err)

	getContractStmt, err := db.PrepareContext(ctx, `SELECT NAME,CREATOR_ADDRESS FROM contracts WHERE ADDRESS = $1`)
	checkNoErr(err)

	getWalletByDetailsStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,CHAIN,WALLET_TYPE FROM wallets WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	getPreviewNFTsStmt, err := db.PrepareContext(ctx, `SELECT MEDIA->>'thumbnail_url' FROM tokens WHERE CONTRACT_ADDRESS = $1 AND DELETED = false AND OWNER_ADDRESSES && $2 AND LENGTH(MEDIA->>'thumbnail_url') > 0 ORDER BY ID LIMIT 3`)
	checkNoErr(err)

	return &CommunityTokenRepository{
		cache:                  cache,
		db:                     db,
		getInfoStmt:            getInfoStmt,
		getUserByAddressStmt:   getUserByAddressStmt,
		getContractStmt:        getContractStmt,
		getWalletByDetailsStmt: getWalletByDetailsStmt,
		getPreviewNFTsStmt:     getPreviewNFTsStmt,
	}
}

// GetByAddress returns a community by its address
func (c *CommunityTokenRepository) GetByAddress(ctx context.Context, pAddress persist.Address, pChain persist.Chain, forceRefresh bool) (persist.Community, error) {
	var community persist.Community

	if !forceRefresh {
		bs, err := c.cache.Get(ctx, pAddress.String())
		if err == nil && len(bs) > 0 {
			err = json.Unmarshal(bs, &community)
			if err != nil {
				return persist.Community{}, err
			}
			return community, nil
		}
	}

	community = persist.Community{
		ContractAddress: pAddress,
	}

	hasDescription := true

	addresses := make([]persist.Address, 0, 20)

	rows, err := c.getInfoStmt.QueryContext(ctx, pAddress)
	if err != nil {
		return persist.Community{}, fmt.Errorf("error getting community info: %w", err)
	}
	defer rows.Close()

	seen := map[persist.Address]bool{}

	for rows.Next() {
		tempDesc := community.Description

		var address persist.Address
		var media persist.Media
		err = rows.Scan(&address, &community.Description, &media)
		if err != nil {
			return persist.Community{}, fmt.Errorf("error scanning community info: %w", err)
		}

		if tempDesc != "" && hasDescription && tempDesc != community.Description {
			hasDescription = false
		}

		if media.ThumbnailURL != "" && community.PreviewImage == "" {
			community.PreviewImage = media.ThumbnailURL
		}

		if !seen[address] {
			addresses = append(addresses, address)
		}

		seen[address] = true
	}

	if err = rows.Err(); err != nil {
		return persist.Community{}, fmt.Errorf("error getting community info: %w", err)
	}

	if len(seen) == 0 {
		return persist.Community{}, persist.ErrCommunityNotFound{CommunityAddress: pAddress}
	}

	if !hasDescription {
		community.Description = ""
	}

	err = c.getContractStmt.QueryRowContext(ctx, pAddress).Scan(&community.Name, &community.CreatorAddress)
	if err != nil {
		return persist.Community{}, fmt.Errorf("error getting community contract: %w", err)
	}

	tokenHolders := map[persist.DBID]*persist.TokenHolder{}
	for _, address := range addresses {
		wallet := persist.Wallet{}
		err = c.getWalletByDetailsStmt.QueryRowContext(ctx, address).Scan(&wallet.ID, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated, &wallet.Address, &wallet.Chain, &wallet.WalletType)
		if err != nil {
			logrus.Warnf("error getting wallet of member of community '%s' by address '%s': %s", pAddress, address, err)
			continue
		}

		var username persist.NullString
		var userID persist.DBID
		err := c.getUserByWalletIDStmt.QueryRowContext(ctx, wallet.ID).Scan(&userID, &username)
		if err != nil {
			logrus.Warnf("error getting member of community '%s' by wallet ID '%s': %s", pAddress, address, err)
			continue
		}

		if username.String() == "" {
			continue
		}

		if tokenHolder, ok := tokenHolders[userID]; ok {
			tokenHolder.WalletIDs = append(tokenHolder.WalletIDs, wallet.ID)
		} else {
			tokenHolders[userID] = &persist.TokenHolder{
				UserID:      userID,
				WalletIDs:   []persist.DBID{wallet.ID},
				PreviewNFTs: nil,
			}
		}
	}

	community.Owners = make([]persist.TokenHolder, 0, len(tokenHolders))

	for _, tokenHolder := range tokenHolders {
		previewNFTs := make([]persist.NullString, 0, 3)

		rows, err = c.getPreviewNFTsStmt.QueryContext(ctx, pAddress, pq.Array(tokenHolder.WalletIDs))
		defer rows.Close()

		if err != nil {
			logrus.Warnf("error getting preview NFTs of community '%s' by addresses '%s': %s", pAddress, tokenHolder.WalletIDs, err)
		} else {
			for rows.Next() {
				var imageURL persist.NullString
				err = rows.Scan(&imageURL)
				if err != nil {
					logrus.Warnf("error scanning preview NFT of community '%s' by addresses '%s': %s", pAddress, tokenHolder.WalletIDs, err)
					continue
				}
				previewNFTs = append(previewNFTs, imageURL)
			}
		}

		tokenHolder.PreviewNFTs = previewNFTs
		community.Owners = append(community.Owners, *tokenHolder)
	}

	community.LastUpdated = persist.LastUpdatedTime(time.Now())

	bs, err := json.Marshal(community)
	if err != nil {
		return persist.Community{}, err
	}
	err = c.cache.Set(ctx, pAddress.String(), bs, staleCommunityTime)
	if err != nil {
		return persist.Community{}, err
	}

	return community, nil

}
