package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
)

const staleCommunityTime = time.Hour * 2

// CommunityRepository represents a repository for interacting with persisted communities
type CommunityRepository struct {
	cache memstore.Cache
	db    *sql.DB

	getInfoStmt          *sql.Stmt
	getUserByAddressStmt *sql.Stmt
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

	return &CommunityRepository{
		cache:                cache,
		db:                   db,
		getInfoStmt:          getInfoStmt,
		getUserByAddressStmt: getUserByAddressStmt,
	}
}

// GetByAddress returns a community by its address
func (c *CommunityRepository) GetByAddress(ctx context.Context, pAddress persist.Address) (persist.Community, error) {
	var community persist.Community

	bs, err := c.cache.Get(ctx, pAddress.String())
	if err == nil && len(bs) > 0 {
		err = json.Unmarshal(bs, &community)
		if err != nil {
			return persist.Community{}, err
		}
		return community, nil
	}

	community = persist.Community{ContractAddress: pAddress}

	addresses := make([]persist.Address, 0, 20)
	contract := persist.NFTContract{}

	rows, err := c.getInfoStmt.QueryContext(ctx, pAddress)
	if err != nil {
		return persist.Community{}, fmt.Errorf("error getting community info: %w", err)
	}
	defer rows.Close()

	seen := map[persist.Address]bool{}
	for rows.Next() {
		var address persist.Address
		err = rows.Scan(&address, &contract, &community.Name, &community.CreatorAddress, &community.PreviewImage)
		if err != nil {
			return persist.Community{}, fmt.Errorf("error scanning community info: %w", err)
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

	community.Description = contract.ContractDescription

	if contract.ContractImage.String() != "" {
		community.PreviewImage = contract.ContractImage
	}
	if community.Name.String() == "" {
		community.Name = contract.ContractName
	}

	community.Owners = make([]persist.CommunityOwner, 0, len(addresses))
	for _, address := range addresses {
		var username persist.NullString
		err := c.getUserByAddressStmt.QueryRowContext(ctx, address).Scan(&username)
		if err != nil {
			return persist.Community{}, fmt.Errorf("error getting user by address: %w", err)
		}
		community.Owners = append(community.Owners, persist.CommunityOwner{Address: address, Username: username})
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
