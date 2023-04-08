package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// ContractRepository represents a contract repository in the postgres database
type ContractRepository struct {
	db                          *sql.DB
	getByAddressStmt            *sql.Stmt
	upsertByAddressStmt         *sql.Stmt
	updateByAddressStmt         *sql.Stmt
	getMetadataByAddressStmt    *sql.Stmt
	updateMetadataByAddressStmt *sql.Stmt
}

// NewContractRepository creates a new postgres repository for interacting with contracts
func NewContractRepository(db *sql.DB) *ContractRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,CREATOR_ADDRESS FROM contracts WHERE ADDRESS = $1 AND DELETED = false;`)
	checkNoErr(err)

	getMetadataByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,CREATOR_ADDRESS,CONTRACT_URI,CONTRACT_METADATA FROM contracts WHERE ADDRESS = $1 AND DELETED = false;`)
	checkNoErr(err)

	upsertByAddressStmt, err := db.PrepareContext(ctx, `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,CREATOR_ADDRESS) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (ADDRESS) DO UPDATE SET VERSION = $2,ADDRESS = $3,SYMBOL = $4,NAME = $5,LATEST_BLOCK = $6,CREATOR_ADDRESS = $7;`)
	checkNoErr(err)

	updateByAddressStmt, err := db.PrepareContext(ctx, `UPDATE contracts SET NAME = $2, SYMBOL = $3, CREATOR_ADDRESS = $4, LATEST_BLOCK = $5, LAST_UPDATED = $6 WHERE ADDRESS = $1;`)
	checkNoErr(err)

	updateMetadataByAddressStmt, err := db.PrepareContext(ctx, `UPDATE contracts SET NAME = $2, SYMBOL = $3, CREATOR_ADDRESS = $4, LATEST_BLOCK = $5, LAST_UPDATED = $6, CONTRACT_URI = $7, CONTRACT_METADATA = $8 WHERE ADDRESS = $1;`)
	checkNoErr(err)

	return &ContractRepository{db: db, getByAddressStmt: getByAddressStmt, upsertByAddressStmt: upsertByAddressStmt, updateByAddressStmt: updateByAddressStmt, getMetadataByAddressStmt: getMetadataByAddressStmt, updateMetadataByAddressStmt: updateMetadataByAddressStmt}
}

// GetByAddress returns the contract with the given address
func (c *ContractRepository) GetByAddress(pCtx context.Context, pAddress persist.EthereumAddress) (persist.Contract, error) {
	contract := persist.Contract{}
	err := c.getByAddressStmt.QueryRowContext(pCtx, pAddress).Scan(&contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.LatestBlock, &contract.CreatorAddress)
	if err != nil {
		return persist.Contract{}, err
	}

	return contract, nil
}

func (c *ContractRepository) GetAllByAddress(pCtx context.Context, pAddress persist.EthereumAddress) (persist.Contract, error) {
	contract := persist.Contract{}
	err := c.getByAddressStmt.QueryRowContext(pCtx, pAddress).Scan(&contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.LatestBlock, &contract.CreatorAddress, &contract.ContractURI, &contract.ContractMetadata)
	if err != nil {
		return persist.Contract{}, err
	}

	return contract, nil
}

// GetByAddress returns the contract with the given address
func (c *ContractRepository) GetMetadataByAddress(pCtx context.Context, pAddress persist.EthereumAddress) (persist.Contract, error) {
	contract := persist.Contract{}
	err := c.getMetadataByAddressStmt.QueryRowContext(pCtx, pAddress).Scan(&contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.LatestBlock, &contract.CreatorAddress)
	if err != nil {
		return persist.Contract{}, err
	}

	return contract, nil
}

// UpsertByAddress upserts the contract with the given address
func (c *ContractRepository) UpsertByAddress(pCtx context.Context, pAddress persist.EthereumAddress, pContract persist.Contract) error {
	_, err := c.upsertByAddressStmt.ExecContext(pCtx, persist.GenerateID(), pContract.Version, pContract.Address, pContract.Symbol, pContract.Name, pContract.LatestBlock, pContract.CreatorAddress)
	if err != nil {
		return err
	}

	return nil
}

// BulkUpsert bulk upserts the contracts by address
func (c *ContractRepository) BulkUpsert(pCtx context.Context, pContracts []persist.Contract) error {
	if len(pContracts) == 0 {
		return nil
	}
	pContracts = removeDuplicateContracts(pContracts)
	sqlStr := `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,CREATOR_ADDRESS) VALUES `
	vals := make([]interface{}, 0, len(pContracts)*7)
	for i, contract := range pContracts {
		sqlStr += generateValuesPlaceholders(7, i*7, nil)
		vals = append(vals, persist.GenerateID(), contract.Version, contract.Address, contract.Symbol, contract.Name, contract.LatestBlock, contract.CreatorAddress)
		sqlStr += ","
	}
	sqlStr = sqlStr[:len(sqlStr)-1]
	sqlStr += ` ON CONFLICT (ADDRESS) DO UPDATE SET SYMBOL = EXCLUDED.SYMBOL,NAME = EXCLUDED.NAME,LATEST_BLOCK = EXCLUDED.LATEST_BLOCK,CREATOR_ADDRESS = EXCLUDED.CREATOR_ADDRESS;`
	_, err := c.db.ExecContext(pCtx, sqlStr, vals...)
	if err != nil {
		return fmt.Errorf("error bulk upserting contracts: %v - SQL: %s -- VALS: %+v", err, sqlStr, vals)
	}

	return nil
}

// UpdateByAddress updates the given contract's metadata fields by its address field.
func (c *ContractRepository) UpdateByAddress(ctx context.Context, addr persist.EthereumAddress, up persist.ContractUpdateInput) error {
	if _, err := c.updateByAddressStmt.ExecContext(ctx, addr, up.Name, up.Symbol, up.CreatorAddress, up.LatestBlock, persist.LastUpdatedTime{}); err != nil {
		return err
	}
	return nil
}

func (c *ContractRepository) UpdateMetadataByAddress(ctx context.Context, addr persist.EthereumAddress, up persist.Contract) error {
	if _, err := c.updateMetadataByAddressStmt.ExecContext(ctx, addr, up.Name, up.Symbol, up.CreatorAddress, up.LatestBlock, persist.LastUpdatedTime{}, up.ContractURI, up.ContractMetadata); err != nil {
		return err
	}
	return nil
}

func removeDuplicateContracts(pContracts []persist.Contract) []persist.Contract {
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
