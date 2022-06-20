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
	db                  *sql.DB
	getByAddressStmt    *sql.Stmt
	upsertByAddressStmt *sql.Stmt
}

// NewContractRepository creates a new postgres repository for interacting with contracts
func NewContractRepository(db *sql.DB) *ContractRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,CREATOR_ADDRESS FROM contracts WHERE ADDRESS = $1 AND DELETED = false;`)
	checkNoErr(err)

	upsertByAddressStmt, err := db.PrepareContext(ctx, `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,CREATOR_ADDRESS) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (ADDRESS) DO UPDATE SET VERSION = $2,ADDRESS = $3,SYMBOL = $4,NAME = $5,LATEST_BLOCK = $6,CREATOR_ADDRESS = $7;`)
	checkNoErr(err)

	return &ContractRepository{db: db, getByAddressStmt: getByAddressStmt, upsertByAddressStmt: upsertByAddressStmt}
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
		sqlStr += generateValuesPlaceholders(7, i*7)
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
