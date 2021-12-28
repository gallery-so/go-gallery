package postgres

import (
	"context"
	"database/sql"

	"github.com/mikeydub/go-gallery/service/persist"
)

// ContractRepository represents a contract repository in the postgres database
type ContractRepository struct {
	db *sql.DB
}

// NewContractRepository creates a new postgres repository for interacting with contracts
func NewContractRepository(db *sql.DB) *ContractRepository {
	return &ContractRepository{db: db}
}

// GetByAddress returns the contract with the given address
func (c *ContractRepository) GetByAddress(pCtx context.Context, pAddress persist.Address) (persist.Contract, error) {
	sqlStr := `SELECT ID,VERSION,DELETED,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,LATEST_BLOCK FROM contracts WHERE ADDRESS = $1`
	contract := persist.Contract{}
	err := c.db.QueryRowContext(pCtx, sqlStr, pAddress).Scan(&contract.ID, &contract.Version, &contract.Deleted, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.LatestBlock)
	if err != nil {
		return persist.Contract{}, err
	}

	return contract, nil
}

// UpsertByAddress upserts the contract with the given address
func (c *ContractRepository) UpsertByAddress(pCtx context.Context, pAddress persist.Address, pContract persist.Contract) error {
	sqlStr := `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,LATEST_BLOCK) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (ADDRESS) DO UPDATE SET VERSION = $2,ADDRESS = $3,SYMBOL = $4,NAME = $5,LATEST_BLOCK = $6`
	_, err := c.db.ExecContext(pCtx, sqlStr, persist.GenerateID(), pContract.Version, pContract.Address, pContract.Symbol, pContract.Name, pContract.LatestBlock)
	if err != nil {
		return err
	}

	return nil
}

// BulkUpsert bulk upserts the contracts by address
func (c *ContractRepository) BulkUpsert(pCtx context.Context, pContracts []persist.Contract) error {
	sqlStr := `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,LATEST_BLOCK) VALUES `
	vals := make([]interface{}, 0, len(pContracts)*7)
	for i, contract := range pContracts {
		if i > 0 {
			sqlStr += `,`
		}
		sqlStr += `($1,$2,$3,$4,$5,$6)`
		vals = append(vals, contract.ID, contract.Version, contract.Address, contract.Symbol, contract.Name, contract.LatestBlock)
	}
	sqlStr += ` ON CONFLICT (ADDRESS) DO UPDATE SET VERSION = EXCLUDED.VERSION,ADDRESS = EXCLUDED.ADDRESS,SYMBOL = EXCLUDED.SYMBOL,NAME = EXCLUDED.NAME,LATEST_BLOCK = EXCLUDED.LATEST_BLOCK`
	_, err := c.db.ExecContext(pCtx, sqlStr, vals...)
	if err != nil {
		return err
	}

	return nil
}
