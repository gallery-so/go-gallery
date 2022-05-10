package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// AddressRepository is a repository for addresses
type AddressRepository struct {
	db *sql.DB

	insertAddressStmt       *sql.Stmt
	getByIDStmt             *sql.Stmt
	getAddressByDetailsStmt *sql.Stmt
}

// NewAddressRepository creates a new postgres repository for interacting with addresses
func NewAddressRepository(db *sql.DB) *AddressRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	insertAddressStmt, err := db.PrepareContext(ctx, `INSERT INTO addresses (ID,VERSION,ADDRESS_VALUE,CHAIN) VALUES ($1,$2,$3,$4) ON CONFLICT (ADDRESS_VALUE,CHAIN) DO NOTHING`)
	checkNoErr(err)

	getAddressByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS_VALUE,CHAIN FROM addresses addresses WHERE ID = $1;`)
	checkNoErr(err)

	getAddressByDetailsStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS_VALUE,CHAIN FROM addresses addresses WHERE ADDRESS_VALUE = $1 AND CHAIN = $2;`)
	checkNoErr(err)

	return &AddressRepository{
		db:                      db,
		getByIDStmt:             getAddressByIDStmt,
		insertAddressStmt:       insertAddressStmt,
		getAddressByDetailsStmt: getAddressByDetailsStmt,
	}
}

// GetByDetails returns the address with the given details
func (a *AddressRepository) GetByDetails(ctx context.Context, addr persist.AddressValue, chain persist.Chain) (persist.Address, error) {
	var address persist.Address
	err := a.getAddressByDetailsStmt.QueryRowContext(ctx, addr, chain).Scan(&address.ID, &address.Version, &address.CreationTime, &address.LastUpdated, &address.AddressValue, &address.Chain)
	if err != nil {
		if err == sql.ErrNoRows {
			_, err = a.insertAddressStmt.ExecContext(ctx, persist.GenerateID(), 0, addr, chain)
			if err != nil {
				return persist.Address{}, err
			}
			err = a.getAddressByDetailsStmt.QueryRowContext(ctx, addr, chain).Scan(&address.ID, &address.Version, &address.CreationTime, &address.LastUpdated, &address.AddressValue, &address.Chain)
			if err != nil {
				return persist.Address{}, err
			}
			return address, persist.ErrAddressNotFoundByDetails{
				Address: addr,
				Chain:   chain,
			}
		}
		return persist.Address{}, err
	}
	return address, nil
}

// GetByID returns the address with the given ID
func (a *AddressRepository) GetByID(ctx context.Context, id persist.DBID) (persist.Address, error) {
	var address persist.Address
	err := a.getByIDStmt.QueryRowContext(ctx, id).Scan(&address.ID, &address.Version, &address.CreationTime, &address.LastUpdated, &address.AddressValue, &address.Chain)
	if err != nil {
		return persist.Address{}, err
	}
	return address, nil
}

// Insert inserts the address into the database
func (a *AddressRepository) Insert(ctx context.Context, address persist.AddressValue, chain persist.Chain) error {
	_, err := a.insertAddressStmt.ExecContext(ctx, persist.GenerateID(), 0, address, chain)
	return err
}
