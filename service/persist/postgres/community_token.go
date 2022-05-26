package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

// CommunityTokenRepository represents a repository for interacting with persisted communities
type CommunityTokenRepository struct {
	cache memstore.Cache
	db    *sql.DB

	getInfoStmt             *sql.Stmt
	getUserByAddressStmt    *sql.Stmt
	getContractStmt         *sql.Stmt
	getWalletByDetailsStmt  *sql.Stmt
	getAddressByDetailsStmt *sql.Stmt
	getAddressByIDStmt      *sql.Stmt
}

// NewCommunityTokenRepository returns a new CommunityRepository
func NewCommunityTokenRepository(db *sql.DB, cache memstore.Cache) *CommunityTokenRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getInfoStmt, err := db.PrepareContext(ctx,
		`SELECT n.OWNER_ADDRESS,n.DESCRIPTION,n.MEDIA
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM nfts n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE n.CONTRACT_ADDRESS = $1 AND g.DELETED = false AND c.DELETED = false AND n.DELETED = false ORDER BY coll_ord,n.nft_ord;`,
	)
	checkNoErr(err)

	getUserByAddressStmt, err := db.PrepareContext(ctx, `SELECT USERNAME FROM users WHERE ADDRESSES @> ARRAY[$1]:: varchar[] AND DELETED = false`)
	checkNoErr(err)

	getContractStmt, err := db.PrepareContext(ctx, `SELECT NAME,CREATOR_ADDRESS FROM contracts WHERE ADDRESS = $1`)
	checkNoErr(err)

	getWalletByDetailsStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,CHAIN,WALLET_TYPE FROM wallets WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	getAddressByDetailsStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS_VALUE,CHAIN FROM addresses WHERE ADDRESS_VALUE = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	getAddressByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,CHAIN FROM addresses WHERE ID = $1;`)
	checkNoErr(err)

	return &CommunityTokenRepository{
		cache:                   cache,
		db:                      db,
		getInfoStmt:             getInfoStmt,
		getUserByAddressStmt:    getUserByAddressStmt,
		getContractStmt:         getContractStmt,
		getWalletByDetailsStmt:  getWalletByDetailsStmt,
		getAddressByDetailsStmt: getAddressByDetailsStmt,
		getAddressByIDStmt:      getAddressByIDStmt,
	}
}

// GetByAddress returns a community by its address
func (c *CommunityTokenRepository) GetByAddress(ctx context.Context, pAddress persist.Address, pChain persist.Chain) (persist.Community, error) {
	var community persist.Community

	bs, err := c.cache.Get(ctx, pAddress.String())
	if err == nil && len(bs) > 0 {
		err = json.Unmarshal(bs, &community)
		if err != nil {
			return persist.Community{}, err
		}
		return community, nil
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

	community.Owners = make([]persist.CommunityOwner, 0, len(addresses))
	for _, address := range addresses {
		var username persist.NullString
		err := c.getUserByAddressStmt.QueryRowContext(ctx, address).Scan(&username)
		if err != nil {
			return persist.Community{}, fmt.Errorf("error getting user by address: %w", err)
		}
		wallet := persist.Wallet{}
		err = c.getWalletByDetailsStmt.QueryRowContext(ctx, address).Scan(&wallet.ID, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated, &wallet.Address, &wallet.Chain, &wallet.WalletType)
		if err != nil {
			logrus.Warnf("error getting wallet of member of community '%s' by address '%s': %s", pAddress, address, err)
			continue
		}
		community.Owners = append(community.Owners, persist.CommunityOwner{Wallet: wallet, Username: username})
	}

	community.LastUpdated = persist.LastUpdatedTime(time.Now())

	bs, err = json.Marshal(community)
	if err != nil {
		return persist.Community{}, err
	}
	err = c.cache.Set(ctx, pAddress.String(), bs, staleCommunityTime)
	if err != nil {
		return persist.Community{}, err
	}

	return community, nil

}
