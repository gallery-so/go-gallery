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

	getInfoStmt                 *sql.Stmt
	getUserByAddressStmt        *sql.Stmt
	getUserByWalletIDStmt       *sql.Stmt
	getContractStmt             *sql.Stmt
	getWalletByChainAddressStmt *sql.Stmt
	getPreviewNFTsStmt          *sql.Stmt
}

// NewCommunityTokenRepository returns a new CommunityRepository
func NewCommunityTokenRepository(db *sql.DB, cache memstore.Cache) *CommunityTokenRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getInfoStmt, err := db.PrepareContext(ctx,
		`SELECT n.OWNER_ADDRESSES,n.DESCRIPTION,n.MEDIA
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM tokens n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE n.CONTRACT_ADDRESS = $1 AND g.DELETED = false AND c.DELETED = false AND n.DELETED = false ORDER BY coll_ord,n.nft_ord;`,
	)
	checkNoErr(err)

	getUserByWalletIDStmt, err := db.PrepareContext(ctx, `SELECT ID,USERNAME FROM users WHERE ADDRESSES @> ARRAY[$1]:: varchar[] AND DELETED = false`)
	checkNoErr(err)

	getContractStmt, err := db.PrepareContext(ctx, `SELECT NAME,CREATOR_ADDRESS FROM contracts WHERE ADDRESS = $1`)
	checkNoErr(err)

	getPreviewNFTsStmt, err := db.PrepareContext(ctx, `SELECT MEDIA->>'thumbnail_url' FROM tokens WHERE CONTRACT_ADDRESS = $1 AND DELETED = false AND OWNER_ADDRESSES && $2 AND LENGTH(MEDIA->>'thumbnail_url') > 0 ORDER BY ID LIMIT 3`)
	checkNoErr(err)

	return &CommunityTokenRepository{
		cache:                 cache,
		db:                    db,
		getInfoStmt:           getInfoStmt,
		getUserByWalletIDStmt: getUserByWalletIDStmt,
		getContractStmt:       getContractStmt,
		getPreviewNFTsStmt:    getPreviewNFTsStmt,
	}
}

// GetByAddress returns a community by its address
func (c *CommunityTokenRepository) GetByAddress(ctx context.Context, pCommunityAddress persist.ChainAddress, forceRefresh bool) (persist.Community, error) {
	var community persist.Community

	if !forceRefresh {
		bs, err := c.cache.Get(ctx, pCommunityAddress.Address().String())
		if err == nil && len(bs) > 0 {
			err = json.Unmarshal(bs, &community)
			if err != nil {
				return persist.Community{}, err
			}
			return community, nil
		}
	}

	community = persist.Community{
		ContractAddress: pCommunityAddress.Address(),
	}

	hasDescription := true

	// TODO-EZRA: I think this is actually "wallets" now
	walletIDs := make([]persist.DBID, 0, 20)

	rows, err := c.getInfoStmt.QueryContext(ctx, pCommunityAddress.Address())
	if err != nil {
		return persist.Community{}, fmt.Errorf("error getting community info: %w", err)
	}
	defer rows.Close()

	seen := map[persist.DBID]bool{}

	for rows.Next() {
		tempDesc := community.Description

		var wallets []persist.DBID
		var media persist.Media
		err = rows.Scan(pq.Array(&wallets), &community.Description, &media)
		if err != nil {
			return persist.Community{}, fmt.Errorf("error scanning community info: %w", err)
		}

		if tempDesc != "" && hasDescription && tempDesc != community.Description {
			hasDescription = false
		}

		if media.ThumbnailURL != "" && community.PreviewImage == "" {
			community.PreviewImage = media.ThumbnailURL
		}

		for _, id := range wallets {
			if !seen[id] {
				walletIDs = append(walletIDs, id)
			}

			seen[id] = true
		}
	}

	if err = rows.Err(); err != nil {
		return persist.Community{}, fmt.Errorf("error getting community info: %w", err)
	}

	if len(seen) == 0 {
		return persist.Community{}, persist.ErrCommunityNotFound{CommunityAddress: pCommunityAddress}
	}

	if !hasDescription {
		community.Description = ""
	}

	err = c.getContractStmt.QueryRowContext(ctx, pCommunityAddress.Address()).Scan(&community.Name, &community.CreatorAddress)
	if err != nil {
		return persist.Community{}, fmt.Errorf("error getting community contract: %w", err)
	}

	tokenHolders := map[persist.DBID]*persist.TokenHolder{}
	for _, walletID := range walletIDs {
		var username persist.NullString
		var userID persist.DBID
		err := c.getUserByWalletIDStmt.QueryRowContext(ctx, walletID).Scan(&userID, &username)
		if err != nil {
			logrus.Warnf("error getting member of community '%s' by wallet ID '%s': %s", pCommunityAddress, walletID, err)
			continue
		}

		if username.String() == "" {
			continue
		}

		if tokenHolder, ok := tokenHolders[userID]; ok {
			tokenHolder.WalletIDs = append(tokenHolder.WalletIDs, walletID)
		} else {
			tokenHolders[userID] = &persist.TokenHolder{
				UserID:      userID,
				WalletIDs:   []persist.DBID{walletID},
				PreviewNFTs: nil,
			}
		}
	}

	community.Owners = make([]persist.TokenHolder, 0, len(tokenHolders))

	for _, tokenHolder := range tokenHolders {
		previewNFTs := make([]persist.NullString, 0, 3)

		rows, err = c.getPreviewNFTsStmt.QueryContext(ctx, pCommunityAddress.Address(), pq.Array(tokenHolder.WalletIDs))
		defer rows.Close()

		if err != nil {
			logrus.Warnf("error getting preview NFTs of community '%s' by wallet IDs '%s': %s", pCommunityAddress, tokenHolder.WalletIDs, err)
		} else {
			for rows.Next() {
				var imageURL persist.NullString
				err = rows.Scan(&imageURL)
				if err != nil {
					logrus.Warnf("error scanning preview NFT of community '%s' by wallet IDs '%s': %s", pCommunityAddress, tokenHolder.WalletIDs, err)
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
	err = c.cache.Set(ctx, pCommunityAddress.Address().String(), bs, staleCommunityTime)
	if err != nil {
		return persist.Community{}, err
	}

	return community, nil

}
