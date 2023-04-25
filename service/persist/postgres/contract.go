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
	updateByAddressStmt *sql.Stmt
	ownedByAddressStmt  *sql.Stmt
}

// NewContractRepository creates a new postgres repository for interacting with contracts
func NewContractRepository(db *sql.DB) *ContractRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,OWNER_ADDRESS FROM contracts WHERE ADDRESS = $1 AND DELETED = false;`)
	checkNoErr(err)

	upsertByAddressStmt, err := db.PrepareContext(ctx, `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,OWNER_ADDRESS, CREATOR_ADDRESS) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (ADDRESS) DO UPDATE SET VERSION = $2,ADDRESS = $3,SYMBOL = $4,NAME = $5,LATEST_BLOCK = $6,OWNER_ADDRESS = $7, CREATOR_ADDRESS = $8;`)
	checkNoErr(err)

	updateByAddressStmt, err := db.PrepareContext(ctx, `UPDATE contracts SET NAME = $2, SYMBOL = $3, OWNER_ADDRESS = $4, CREATOR_ADDRESS = $5, LATEST_BLOCK = $6, LAST_UPDATED = $7 WHERE ADDRESS = $1;`)
	checkNoErr(err)

	ownedByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,OWNER_ADDRESS FROM contracts WHERE OWNER_ADDRESS = $1 AND DELETED = false;`)
	checkNoErr(err)

	return &ContractRepository{db: db, getByAddressStmt: getByAddressStmt, upsertByAddressStmt: upsertByAddressStmt, updateByAddressStmt: updateByAddressStmt, ownedByAddressStmt: ownedByAddressStmt}
}

// GetByAddress returns the contract with the given address
func (c *ContractRepository) GetByAddress(pCtx context.Context, pAddress persist.EthereumAddress) (persist.Contract, error) {
	contract := persist.Contract{}
	err := c.getByAddressStmt.QueryRowContext(pCtx, pAddress).Scan(&contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.LatestBlock, &contract.OwnerAddress)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.Contract{}, persist.ErrContractNotFoundByAddress{
				Address: pAddress,
			}
		}
		return persist.Contract{}, err
	}

	return contract, nil
}

// UpsertByAddress upserts the contract with the given address
func (c *ContractRepository) UpsertByAddress(pCtx context.Context, pAddress persist.EthereumAddress, pContract persist.Contract) error {
	_, err := c.upsertByAddressStmt.ExecContext(pCtx, persist.GenerateID(), pContract.Version, pContract.Address, pContract.Symbol, pContract.Name, pContract.LatestBlock, pContract.OwnerAddress, pContract.CreatorAddress)
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
	sqlStr := `INSERT INTO contracts (ID,VERSION,ADDRESS,SYMBOL,NAME,LATEST_BLOCK,OWNER_ADDRESS,CREATOR_ADDRESS) VALUES `
	vals := make([]interface{}, 0, len(pContracts)*8)
	for i, contract := range pContracts {
		sqlStr += generateValuesPlaceholders(8, i*8, nil)
		vals = append(vals, persist.GenerateID(), contract.Version, contract.Address, contract.Symbol, contract.Name, contract.LatestBlock, contract.OwnerAddress, contract.CreatorAddress)
		sqlStr += ","
	}
	sqlStr = sqlStr[:len(sqlStr)-1]
	sqlStr += ` ON CONFLICT (ADDRESS) DO UPDATE SET SYMBOL = EXCLUDED.SYMBOL,NAME = EXCLUDED.NAME,LATEST_BLOCK = EXCLUDED.LATEST_BLOCK,OWNER_ADDRESS = EXCLUDED.OWNER_ADDRESS, CREATOR_ADDRESS = EXCLUDED.CREATOR_ADDRESS;`
	_, err := c.db.ExecContext(pCtx, sqlStr, vals...)
	if err != nil {
		return fmt.Errorf("error bulk upserting contracts: %v - SQL: %s -- VALS: %+v", err, sqlStr, vals)
	}

	return nil
}

// UpdateByAddress updates the given contract's metadata fields by its address field.
func (c *ContractRepository) UpdateByAddress(ctx context.Context, addr persist.EthereumAddress, up persist.ContractUpdateInput) error {
	if _, err := c.updateByAddressStmt.ExecContext(ctx, addr, up.Name, up.Symbol, up.OwnerAddress, up.CreatorAddress, up.LatestBlock, persist.LastUpdatedTime{}); err != nil {
		return err
	}
	return nil
}

// GetContractsOwnedByAddress returns all contracts owned by the given address
func (c *ContractRepository) GetContractsOwnedByAddress(ctx context.Context, addr persist.EthereumAddress) ([]persist.Contract, error) {
	rows, err := c.ownedByAddressStmt.QueryContext(ctx, addr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	contracts := []persist.Contract{}
	for rows.Next() {
		contract := persist.Contract{}
		if err := rows.Scan(&contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.LatestBlock, &contract.OwnerAddress); err != nil {
			return nil, err
		}
		contracts = append(contracts, contract)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contracts, nil
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
