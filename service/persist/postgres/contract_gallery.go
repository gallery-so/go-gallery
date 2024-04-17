package postgres

import (
	"context"
	"database/sql"
	"fmt"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

// ContractGalleryRepository represents a contract repository in the postgres database
type ContractGalleryRepository struct {
	db      *sql.DB
	queries *db.Queries
}

// NewContractGalleryRepository creates a new postgres repository for interacting with contracts
func NewContractGalleryRepository(db *sql.DB, queries *db.Queries) *ContractGalleryRepository {
	return &ContractGalleryRepository{db: db, queries: queries}
}

func (c *ContractGalleryRepository) Upsert(pCtx context.Context, contract db.Contract, canOverwriteOwnerAddress bool) (db.Contract, error) {
	upserted, err := c.BulkUpsert(pCtx, []db.Contract{contract}, canOverwriteOwnerAddress)
	if err != nil {
		return db.Contract{}, err
	}
	return upserted[0], nil
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
		params.L1Chain = append(params.L1Chain, int32(c.Chain.L1Chain()))
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
