package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

// ContractGalleryRepository represents a contract repository in the postgres database
type ContractGalleryRepository struct {
	db                    *sql.DB
	queries               *db.Queries
	getByIDStmt           *sql.Stmt
	getByAddressStmt      *sql.Stmt
	getByAddressesStmt    *sql.Stmt
	upsertByAddressStmt   *sql.Stmt
	getOwnersStmt         *sql.Stmt
	getUserByWalletIDStmt *sql.Stmt
	getPreviewNFTsStmt    *sql.Stmt
}

// NewContractGalleryRepository creates a new postgres repository for interacting with contracts
func NewContractGalleryRepository(db *sql.DB, queries *db.Queries) *ContractGalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,OWNER_ADDRESS,CHAIN,IS_PROVIDER_MARKED_SPAM FROM contracts WHERE ID = $1;`)
	checkNoErr(err)

	getByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,OWNER_ADDRESS,CHAIN,IS_PROVIDER_MARKED_SPAM FROM contracts WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	getByAddressesStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,OWNER_ADDRESS,CHAIN,IS_PROVIDER_MARKED_SPAM FROM contracts WHERE ADDRESS = ANY($1) AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	upsertByAddressStmt, err := db.PrepareContext(ctx, `
		insert into contracts (id,version,address,symbol,name,owner_address,chain) values ($1,$2,$3,$4,$5,$6,$7)
			on conflict (address,chain) where parent_id is null do update set
			version = $2,
			address = $3,
			symbol = $4,
			owner_address = case when nullif(contracts.owner_address, '') is null then $6 else contracts.owner_address end,
			chain = $7;
	`)
	checkNoErr(err)

	getOwnersStmt, err := db.PrepareContext(ctx,
		`SELECT n.OWNED_BY_WALLETS
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM tokens n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE n.CONTRACT = $1 AND g.DELETED = false AND c.DELETED = false AND n.DELETED = false ORDER BY coll_ord,n.nft_ord LIMIT $2 OFFSET $3;`,
	)
	checkNoErr(err)

	getUserByWalletIDStmt, err := db.PrepareContext(ctx, `SELECT ID,USERNAME FROM users WHERE WALLETS @> ARRAY[$1]:: varchar[] AND DELETED = false`)
	checkNoErr(err)

	getPreviewNFTsStmt, err := db.PrepareContext(ctx, `SELECT MEDIA->>'thumbnail_url' FROM tokens WHERE CONTRACT = $1 AND DELETED = false AND OWNED_BY_WALLETS && $2 AND LENGTH(MEDIA->>'thumbnail_url') > 0 ORDER BY ID LIMIT 3`)
	checkNoErr(err)

	return &ContractGalleryRepository{db: db, queries: queries, getByIDStmt: getByIDStmt, getByAddressStmt: getByAddressStmt, upsertByAddressStmt: upsertByAddressStmt, getByAddressesStmt: getByAddressesStmt, getOwnersStmt: getOwnersStmt, getUserByWalletIDStmt: getUserByWalletIDStmt, getPreviewNFTsStmt: getPreviewNFTsStmt}
}

func (c *ContractGalleryRepository) GetByID(ctx context.Context, id persist.DBID) (persist.ContractGallery, error) {
	contract := persist.ContractGallery{}
	err := c.getByIDStmt.QueryRowContext(ctx, id).Scan(&contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.OwnerAddress, &contract.Chain, &contract.IsProviderMarkedSpam)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return persist.ContractGallery{}, persist.ErrContractNotFoundByID{ID: id}
		}
		return persist.ContractGallery{}, err
	}

	return contract, nil
}

// GetByAddress returns the contract with the given address
func (c *ContractGalleryRepository) GetByAddress(pCtx context.Context, pAddress persist.Address, pChain persist.Chain) (persist.ContractGallery, error) {
	contract := persist.ContractGallery{}
	err := c.getByAddressStmt.QueryRowContext(pCtx, pAddress, pChain).Scan(&contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.OwnerAddress, &contract.Chain, &contract.IsProviderMarkedSpam)
	if err != nil {
		return persist.ContractGallery{}, err
	}

	return contract, nil
}

// GetByAddresses returns the contract with the given address
func (c *ContractGalleryRepository) GetByAddresses(pCtx context.Context, pAddresses []persist.Address, pChain persist.Chain) ([]persist.ContractGallery, error) {
	res := []persist.ContractGallery{}
	rows, err := c.getByAddressesStmt.QueryContext(pCtx, pAddresses, pChain)
	if err != nil {
		return res, err
	}
	defer rows.Close()

	for rows.Next() {
		var contract persist.ContractGallery
		err := rows.Scan(&contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.OwnerAddress, &contract.Chain, &contract.IsProviderMarkedSpam)
		if err != nil {
			return res, err
		}
		res = append(res, contract)
	}

	if err := rows.Err(); err != nil {
		return res, err
	}

	return res, nil
}

// UpsertByAddress upserts the contract with the given address
func (c *ContractGalleryRepository) UpsertByAddress(pCtx context.Context, pAddress persist.Address, pChain persist.Chain, pContract persist.ContractGallery) error {
	_, err := c.upsertByAddressStmt.ExecContext(pCtx, persist.GenerateID(), pContract.Version, pContract.Address, pContract.Symbol, pContract.Name, pContract.OwnerAddress, pContract.Chain)
	if err != nil {
		return err
	}

	return nil
}

// BulkUpsert bulk upserts the contracts by address
func (c *ContractGalleryRepository) BulkUpsert(pCtx context.Context, pContracts []persist.ContractGallery, canOverwriteOwnerAddress bool) ([]persist.ContractGallery, error) {
	if len(pContracts) == 0 {
		return []persist.ContractGallery{}, nil
	}

	contracts := removeDuplicateContractsGallery(pContracts)
	params := db.UpsertParentContractsParams{
		CanOverwriteOwnerAddress: canOverwriteOwnerAddress,
	}

	for i := range contracts {
		c := &contracts[i]
		params.Ids = append(params.Ids, persist.GenerateID().String())
		params.Version = append(params.Version, c.Version.Int32())
		params.Address = append(params.Address, c.Address.String())
		params.Symbol = append(params.Symbol, c.Symbol.String())
		params.Name = append(params.Name, c.Name.String())
		params.OwnerAddress = append(params.OwnerAddress, c.OwnerAddress.String())
		params.Chain = append(params.Chain, int32(c.Chain))
		params.Description = append(params.Description, c.Description.String())
	}

	upserted, err := c.queries.UpsertParentContracts(pCtx, params)
	if err != nil {
		return nil, err
	}

	if len(contracts) != len(upserted) {
		panic(fmt.Sprintf("expected %d upserted contracts, got %d", len(contracts), len(upserted)))
	}

	// Update contracts with the existing data if the contract already exists.
	// The remaining fields are skipped because the upsert logic ensures that the existing row is
	// always replaced with the newer row.
	for i := range contracts {
		c := &contracts[i]
		(*c).ID = upserted[i].ID
		(*c).CreationTime = upserted[i].CreatedAt
		(*c).CreatorAddress = upserted[i].CreatorAddress
		(*c).ParentID = upserted[i].ParentID
	}

	return contracts, nil
}

func (c *ContractGalleryRepository) GetOwnersByAddress(ctx context.Context, contractAddr persist.Address, chain persist.Chain, limit, offset int) ([]persist.TokenHolder, error) {
	contract, err := c.GetByAddress(ctx, contractAddr, chain)
	if err != nil {
		return nil, err
	}

	walletIDs := make([]persist.DBID, 0, 20)
	rows, err := c.getOwnersStmt.QueryContext(ctx, contract.ID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("error getting owners: %w", err)
	}
	defer rows.Close()

	seen := map[persist.DBID]bool{}

	for rows.Next() {

		var wallets []persist.DBID

		err = rows.Scan(pq.Array(&wallets))
		if err != nil {
			return nil, fmt.Errorf("error scanning owners: %w", err)
		}

		for _, id := range wallets {
			if !seen[id] {
				walletIDs = append(walletIDs, id)
			}

			seen[id] = true
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error getting owners: %w", err)
	}

	if len(seen) == 0 {
		return []persist.TokenHolder{}, nil
	}

	tokenHolders := map[persist.DBID]*persist.TokenHolder{}
	for _, walletID := range walletIDs {
		var username persist.NullString
		var userID persist.DBID
		err := c.getUserByWalletIDStmt.QueryRowContext(ctx, walletID).Scan(&userID, &username)
		if err != nil {
			logrus.Warnf("error getting member of community '%s' by wallet ID '%s': %s", contractAddr, walletID, err)
			continue
		}

		if username.String() == "" {
			continue
		}

		if tokenHolder, ok := tokenHolders[userID]; ok {
			tokenHolder.WalletIDs = append(tokenHolder.WalletIDs, walletID)
		} else {
			tokenHolders[userID] = &persist.TokenHolder{
				UserID:        userID,
				WalletIDs:     []persist.DBID{walletID},
				PreviewTokens: nil,
			}
		}
	}

	result := make([]persist.TokenHolder, 0, len(tokenHolders))

	for _, tokenHolder := range tokenHolders {
		previewNFTs := make([]persist.NullString, 0, 3)

		rows, err = c.getPreviewNFTsStmt.QueryContext(ctx, contract.ID, pq.Array(tokenHolder.WalletIDs))
		defer rows.Close()

		if err != nil {
			logrus.Warnf("error getting preview NFTs of community '%s' by wallet IDs '%s': %s", contractAddr, tokenHolder.WalletIDs, err)
		} else {
			for rows.Next() {
				var imageURL persist.NullString
				err = rows.Scan(&imageURL)
				if err != nil {
					logrus.Warnf("error scanning preview NFT of community '%s' by wallet IDs '%s': %s", contractAddr, tokenHolder.WalletIDs, err)
					continue
				}
				previewNFTs = append(previewNFTs, imageURL)
			}
		}

		tokenHolder.PreviewTokens = previewNFTs
		result = append(result, *tokenHolder)
	}

	return result, nil

}

func removeDuplicateContractsGallery(pContracts []persist.ContractGallery) []persist.ContractGallery {
	if len(pContracts) == 0 {
		return pContracts
	}
	unique := map[persist.Address]bool{}
	result := make([]persist.ContractGallery, 0, len(pContracts))
	for _, v := range pContracts {
		if unique[v.Address] {
			continue
		}
		result = append(result, v)
		unique[v.Address] = true
	}
	return result
}
