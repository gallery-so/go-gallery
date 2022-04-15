package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// ContractGalleryRepository represents a contract repository in the postgres database
type ContractGalleryRepository struct {
	db                  *sql.DB
	getByAddressStmt    *sql.Stmt
	upsertByAddressStmt *sql.Stmt
}

// NewContractGalleryRepository creates a new postgres repository for interacting with contracts
func NewContractGalleryRepository(db *sql.DB) *ContractGalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,CREATOR_ADDRESS,CHAIN FROM contracts WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	upsertByAddressStmt, err := db.PrepareContext(ctx, `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,CREATOR_ADDRESS,CHAIN) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (ADDRESS,CHAIN) DO UPDATE SET VERSION = $2, ADDRESS = $3, SYMBOL = $4, NAME = $5, CREATOR_ADDRESS = $6, CHAIN = $7;`)
	checkNoErr(err)

	return &ContractGalleryRepository{db: db, getByAddressStmt: getByAddressStmt, upsertByAddressStmt: upsertByAddressStmt}
}

// GetByAddress returns the contract with the given address
func (c *ContractGalleryRepository) GetByAddress(pCtx context.Context, pAddress persist.Address, pChain persist.Chain) (persist.ContractGallery, error) {
	contract := persist.ContractGallery{}
	err := c.getByAddressStmt.QueryRowContext(pCtx, pAddress, pChain).Scan(&contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.CreatorAddress, &contract.Chain)
	if err != nil {
		return persist.ContractGallery{}, err
	}

	return contract, nil
}

// UpsertByAddress upserts the contract with the given address
func (c *ContractGalleryRepository) UpsertByAddress(pCtx context.Context, pAddress persist.EthereumAddress, pContract persist.ContractGallery) error {
	_, err := c.upsertByAddressStmt.ExecContext(pCtx, persist.GenerateID(), pContract.Version, pContract.Address, pContract.Symbol, pContract.Name, pContract.CreatorAddress, pContract.Chain)
	if err != nil {
		return err
	}

	return nil
}

// BulkUpsert bulk upserts the contracts by address
func (c *ContractGalleryRepository) BulkUpsert(pCtx context.Context, pContracts []persist.ContractGallery) error {
	if len(pContracts) == 0 {
		return nil
	}
	pContracts = removeDuplicateContractsGallery(pContracts)
	sqlStr := `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,CREATOR_ADDRESS,CHAIN) VALUES `
	vals := make([]interface{}, 0, len(pContracts)*7)
	for i, contract := range pContracts {
		sqlStr += generateValuesPlaceholders(7, i*7)
		vals = append(vals, persist.GenerateID(), contract.Version, contract.Address, contract.Symbol, contract.Name, contract.CreatorAddress)
		sqlStr += ","
	}
	sqlStr = sqlStr[:len(sqlStr)-1]
	sqlStr += ` ON CONFLICT (ADDRESS,CHAIN) DO UPDATE SET SYMBOL = EXCLUDED.SYMBOL,NAME = EXCLUDED.NAME,CREATOR_ADDRESS = EXCLUDED.CREATOR_ADDRESS,CHAIN = EXCLUDED.CHAIN;`
	_, err := c.db.ExecContext(pCtx, sqlStr, vals...)
	if err != nil {
		return fmt.Errorf("error bulk upserting contracts: %v - SQL: %s -- VALS: %+v", err, sqlStr, vals)
	}

	return nil
}

func removeDuplicates(pContracts []persist.Contract) []persist.Contract {
	if len(pContracts) == 0 {
		return pContracts
	}
	unique := map[persist.EthereumAddress]bool{}
	result := make([]persist.Contract, 0, len(pContracts))
	for _, v := range pContracts {
		if unique[v.Address] {
			continue
		}
		result = append(result, v)
		unique[v.Address] = true
	}
	return result
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
