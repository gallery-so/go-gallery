package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
)

const staleCommunityTime = time.Hour * 2

// CommunityRepository represents a repository for interacting with persisted communities
type CommunityRepository struct {
	cache memstore.Cache
	db    *sql.DB

	getInfoStmt             *sql.Stmt
	getUserByAddressStmt    *sql.Stmt
	getWalletByDetailsStmt  *sql.Stmt
	getAddressByDetailsStmt *sql.Stmt
	getAddressByIDStmt      *sql.Stmt
}

// NewCommunityRepository returns a new CommunityRepository
func NewCommunityRepository(db *sql.DB, cache memstore.Cache) *CommunityRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getInfoStmt, err := db.PrepareContext(ctx,
		`SELECT n.OWNER_ADDRESS,n.CONTRACT,n.TOKEN_COLLECTION_NAME,n.CREATOR_ADDRESS,n.IMAGE_PREVIEW_URL
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM nfts n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE n.CONTRACT ->> 'address' = $1 AND g.DELETED = false AND c.DELETED = false AND n.DELETED = false ORDER BY coll_ord,n.nft_ord;`,
	)
	checkNoErr(err)

	getUserByAddressStmt, err := db.PrepareContext(ctx, `SELECT USERNAME FROM users WHERE ADDRESSES @> ARRAY[$1]:: varchar[] AND DELETED = false`)
	checkNoErr(err)

	getWalletByDetailsStmt, err := db.PrepareContext(ctx, `SELECT wallets.ID,wallets.Version,wallets.CREATED_AT,wallets.LAST_UPDATED,wallets.WALLET_TYPE,addresses.ID,addresses.VERSION,addresses.CREATED_AT,addresses.LAST_UPDATED,addresses.ADDRESS_VALUE,addresses.CHAIN FROM wallets LEFT JOIN addresses ON wallets.ADDRESS = addresses.ID WHERE addresses.ADDRESS_VALUE = $1 AND addresses.CHAIN = $2 AND addresses.DELETED = false AND wallets.DELETED = false;`)
	checkNoErr(err)

	getAddressByDetailsStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS_VALUE,CHAIN FROM addresses WHERE ADDRESS_VALUE = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	getAddressByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,CHAIN FROM addresses WHERE ID = $1;`)
	checkNoErr(err)

	return &CommunityRepository{
		cache:                   cache,
		db:                      db,
		getInfoStmt:             getInfoStmt,
		getUserByAddressStmt:    getUserByAddressStmt,
		getWalletByDetailsStmt:  getWalletByDetailsStmt,
		getAddressByDetailsStmt: getAddressByDetailsStmt,
		getAddressByIDStmt:      getAddressByIDStmt,
	}
}

// GetByAddress returns a community by its address
func (c *CommunityRepository) GetByAddress(ctx context.Context, pAddress persist.AddressValue, pChain persist.Chain) (persist.Community, error) {
	var community persist.Community

	bs, err := c.cache.Get(ctx, pAddress.String())
	if err == nil && len(bs) > 0 {
		err = json.Unmarshal(bs, &community)
		if err != nil {
			return persist.Community{}, err
		}
		return community, nil
	}

	var contractAddress persist.Address

	err = c.getAddressByDetailsStmt.QueryRowContext(ctx, pAddress, pChain).Scan(&contractAddress.ID, &contractAddress.Version, &contractAddress.CreationTime, &contractAddress.LastUpdated, &contractAddress.AddressValue, &contractAddress.Chain)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.Community{}, persist.ErrCommunityNotFound{CommunityAddress: pAddress, Chain: pChain}
		}
		return persist.Community{}, err
	}

	community = persist.Community{
		ContractAddress: contractAddress,
	}

	addressIDs := make([]persist.DBID, 0, 20)
	contract := persist.NFTContract{}

	rows, err := c.getInfoStmt.QueryContext(ctx, pAddress)
	if err != nil {
		return persist.Community{}, fmt.Errorf("error getting community info: %w", err)
	}
	defer rows.Close()

	seenAddress := map[persist.DBID]bool{}
	for rows.Next() {
		var addressID persist.DBID
		var creatorAddressID persist.DBID
		err = rows.Scan(&addressID, &contract, &community.Name, &creatorAddressID, &community.PreviewImage)
		if err != nil {
			return persist.Community{}, fmt.Errorf("error scanning community info: %w", err)
		}
		if community.CreatorAddress.ID == "" {
			var creatorAddress persist.Address
			err = c.getAddressByIDStmt.QueryRowContext(ctx, creatorAddressID).Scan(&creatorAddress.ID, &creatorAddress.Version, &creatorAddress.CreationTime, &creatorAddress.LastUpdated, &creatorAddress.AddressValue, &creatorAddress.Chain)
			if err != nil {
				return persist.Community{}, fmt.Errorf("error getting creator address: %w", err)
			}
			community.CreatorAddress = creatorAddress
		}

		if !seenAddress[addressID] {
			addressIDs = append(addressIDs, addressID)
		}
		seenAddress[addressID] = true
	}

	if err = rows.Err(); err != nil {
		return persist.Community{}, fmt.Errorf("error getting community info: %w", err)
	}

	if len(seenAddress) == 0 {
		return persist.Community{}, persist.ErrCommunityNotFound{CommunityAddress: pAddress}
	}

	community.Description = contract.ContractDescription

	if contract.ContractImage.String() != "" {
		community.PreviewImage = contract.ContractImage
	}
	if community.Name.String() == "" {
		community.Name = contract.ContractName
	}

	seenUsername := map[string]bool{}
	community.Owners = make([]persist.CommunityOwner, 0, len(addressIDs))
	for _, addressID := range addressIDs {
		var username persist.NullString
		err := c.getUserByAddressStmt.QueryRowContext(ctx, addressID).Scan(&username)
		if err != nil {
			logrus.Warnf("error getting member of community '%s' by address '%s': %s", pAddress, addressID, err)
			continue
		}

		// Don't include users who haven't picked a username yet
		usernameStr := username.String()
		if usernameStr != "" && !seenUsername[usernameStr] {
			wallet := persist.Wallet{Address: persist.Address{}}
			err := c.getWalletByDetailsStmt.QueryRowContext(ctx, addressID).Scan(&wallet.ID, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated, &wallet.WalletType, &wallet.Address.ID, &wallet.Address.Version, &wallet.Address.CreationTime, &wallet.Address.AddressValue, &wallet.Address.Chain)
			if err != nil {
				logrus.Warnf("error getting wallet of member of community '%s' by address '%s': %s", pAddress, addressID, err)
				continue
			}
			community.Owners = append(community.Owners, persist.CommunityOwner{Wallet: wallet, Username: username})
			seenUsername[usernameStr] = true
		}
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
