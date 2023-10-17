package postgres

import (
	"context"
	"database/sql"
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
	upsertByAddressStmt   *sql.Stmt
	getOwnersStmt         *sql.Stmt
	getUserByWalletIDStmt *sql.Stmt
	getPreviewNFTsStmt    *sql.Stmt
}

// NewContractGalleryRepository creates a new postgres repository for interacting with contracts
func NewContractGalleryRepository(db *sql.DB, queries *db.Queries) *ContractGalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	upsertByAddressStmt, err := db.PrepareContext(ctx, `
		insert into contracts (id,version,address,symbol,name,owner_address,chain,description,profile_image_url) values ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			on conflict (address,chain) where parent_id is null do update set
			version = $2,
			address = $3,
			symbol = coalesce(nullif(contracts.symbol, ''), nullif($4, '')),
			name = coalesce(nullif(contracts.name, ''), nullif($5, '')),
			description = coalesce(nullif(contracts.description, ''), nullif($8, '')),
			profile_image_url = coalesce(nullif(contracts.profile_image_url, ''), nullif($9, '')),
			owner_address = case when nullif(contracts.owner_address, '') is null then $6 else contracts.owner_address end,
			chain = $7
		returning id;
	`)
	checkNoErr(err)

	getOwnersStmt, err := db.PrepareContext(ctx, `
		select unnest(t.owned_by_wallets)
		from galleries g, unnest(g.collections) with ordinality sections(id, idx)
		join (
			select collections.id collection_id, section_tokens.*
			from collections, unnest(collections.nfts) with ordinality section_tokens(token_id, token_idx)
			where not collections.deleted
		) c on sections.id = c.collection_id
		join tokens t on t.id = c.token_id
		join token_definitions td on t.token_definition_id = td.id
		where td.contract_id = $1
			and not g.deleted
			and not t.deleted
			and not td.deleted
		group by unnest(t.owned_by_wallets)
		order by min(sections.idx), min(c.token_idx), unnest(t.owned_by_wallets)
		limit $2 offset $3;`)
	checkNoErr(err)

	getUserByWalletIDStmt, err := db.PrepareContext(ctx, `SELECT ID,USERNAME FROM users WHERE WALLETS @> ARRAY[$1]:: varchar[] AND DELETED = false`)
	checkNoErr(err)

	getPreviewNFTsStmt, err := db.PrepareContext(ctx, `
		select coalesce(nullif(token_medias.media->>'thumbnail_url', ''), nullif(token_medias.media->>'media_url', ''))::varchar
		from tokens, token_definitions, token_medias
		where token_definitions.contract_id = $1
			and tokens.owned_by_wallets && $2
			and tokens.token_definition_id = token_definitions.id
			and token_definitions.token_media_id = token_medias.id
			and (length(token_medias.media->>'thumbnail_url') > 0 or length(token_medias.media->>'media_url') > 0)
			and not tokens.deleted
			and not token_definitions.deleted
			and not token_medias.deleted
		order by tokens.id
		limit 3;
	`)
	checkNoErr(err)

	return &ContractGalleryRepository{db: db, queries: queries, upsertByAddressStmt: upsertByAddressStmt, getOwnersStmt: getOwnersStmt, getUserByWalletIDStmt: getUserByWalletIDStmt, getPreviewNFTsStmt: getPreviewNFTsStmt}
}

// UpsertByAddress upserts the contract with the given address
func (c *ContractGalleryRepository) UpsertByAddress(pCtx context.Context, pAddress persist.Address, pChain persist.Chain, pContract persist.ContractGallery) (contractID persist.DBID, err error) {
	err = c.upsertByAddressStmt.QueryRowContext(pCtx, persist.GenerateID(), pContract.Version, pContract.Address, pContract.Symbol, pContract.Name, pContract.OwnerAddress, pContract.Chain, pContract.Description, pContract.ProfileImageURL).Scan(&contractID)
	if err != nil {
		return "", err
	}

	return contractID, nil
}

// BulkUpsert bulk upserts the contracts by address
func (c *ContractGalleryRepository) BulkUpsert(pCtx context.Context, contracts []db.Contract, canOverwriteOwnerAddress bool) ([]db.Contract, error) {
	if len(contracts) == 0 {
		return []db.Contract{}, nil
	}

	params := db.UpsertParentContractsParams{
		CanOverwriteOwnerAddress: canOverwriteOwnerAddress,
	}

	for i := range contracts {
		c := &contracts[i]
		params.Ids = append(params.Ids, persist.GenerateID().String())
		params.Version = append(params.Version, c.Version.Int32)
		params.Address = append(params.Address, c.Address.String())
		params.Symbol = append(params.Symbol, c.Symbol.String)
		params.Name = append(params.Name, c.Name.String)
		params.OwnerAddress = append(params.OwnerAddress, c.OwnerAddress.String())
		params.Chain = append(params.Chain, int32(c.Chain))
		params.Description = append(params.Description, c.Description.String)
		params.ProfileImageUrl = append(params.ProfileImageUrl, c.ProfileImageUrl.String)
		params.ProviderMarkedSpam = append(params.ProviderMarkedSpam, c.IsProviderMarkedSpam)
	}

	upserted, err := c.queries.UpsertParentContracts(pCtx, params)
	if err != nil {
		return nil, err
	}

	if len(contracts) != len(upserted) {
		panic(fmt.Sprintf("expected %d upserted contracts, got %d", len(contracts), len(upserted)))
	}

	return upserted, nil
}

func (c *ContractGalleryRepository) GetOwnersByAddress(ctx context.Context, contractAddr persist.Address, chain persist.Chain, limit, offset int) ([]persist.TokenHolder, error) {
	panic("not implemented")
	contract, err := c.queries.GetContractByChainAddress(ctx, db.GetContractByChainAddressParams{Address: contractAddr, Chain: chain})
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

		if err != nil {
			logrus.Warnf("error getting preview NFTs of community '%s' by wallet IDs '%s': %s", contractAddr, tokenHolder.WalletIDs, err)
		} else {
			defer rows.Close()
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
