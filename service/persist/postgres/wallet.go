package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// WalletRepository is a repository for wallets
type WalletRepository struct {
	db *sql.DB

	getByAddressDetailStmt  *sql.Stmt
	insertStmt              *sql.Stmt
	insertAddressStmt       *sql.Stmt
	getByAddressStmt        *sql.Stmt
	getAddressByDetailsStmt *sql.Stmt
}

// NewWalletRepository creates a new postgres repository for interacting with wallets
func NewWalletRepository(db *sql.DB) *WalletRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	insertStmt, err := db.PrepareContext(ctx, `INSERT INTO wallets (ID,VERSION,ADDRESS,WALLET_TYPE) VALUES ($1,$2,$3,$4) ON CONFLICT (ADDRESS) DO NOTHING;`)
	checkNoErr(err)

	insertAddressStmt, err := db.PrepareContext(ctx, `INSERT INTO addresses (ID,VERSION,ADDRESS_VALUE,CHAIN) VALUES ($1,$2,$3,$4) ON CONFLICT (ADDRESS_VALUE,CHAIN) DO NOTHING`)
	checkNoErr(err)

	getByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,WALLET_TYPE FROM wallets WHERE ADDRESS = (SELECT ID FROM addresses WHERE ID = $1 AND DELETED = false);`)
	checkNoErr(err)

	getByAddressByDetailsStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,WALLET_TYPE FROM wallets WHERE ADDRESS = (SELECT ID FROM addresses WHERE ADDRESS_VALUE = $1 AND CHAIN = $2 AND DELETED = false);`)
	checkNoErr(err)

	getAddressByDetailsStmt, err := db.PrepareContext(ctx, `SELECT ID FROM addresses WHERE ADDRESS_VALUE = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	return &WalletRepository{
		db:                      db,
		getByAddressDetailStmt:  getByAddressByDetailsStmt,
		getByAddressStmt:        getByAddressStmt,
		insertStmt:              insertStmt,
		insertAddressStmt:       insertAddressStmt,
		getAddressByDetailsStmt: getAddressByDetailsStmt,
	}
}

// GetByAddress returns a wallet by address and chain
func (w *WalletRepository) GetByAddress(ctx context.Context, pAddressID persist.DBID) (persist.Wallet, error) {
	var wallet persist.Wallet
	err := w.getByAddressStmt.QueryRowContext(ctx, pAddressID).Scan(&wallet.ID, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated, &wallet.Address.ID, &wallet.Address.AddressValue, &wallet.Address.Chain)
	if err != nil {
		if err == sql.ErrNoRows {
			return wallet, persist.ErrWalletNotFoundByAddress{Address: pAddressID}
		}
		return wallet, err
	}
	return wallet, nil

}

// GetByAddressDetails returns a wallet by address and chain
func (w *WalletRepository) GetByAddressDetails(ctx context.Context, addr persist.AddressValue, chain persist.Chain) (persist.Wallet, error) {
	var wallet persist.Wallet
	err := w.getByAddressDetailStmt.QueryRowContext(ctx, addr, chain).Scan(&wallet.ID, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated, &wallet.Address.ID, &wallet.Address.AddressValue, &wallet.Address.Chain)
	if err != nil {
		if err == sql.ErrNoRows {
			return wallet, persist.ErrWalletNotFoundByAddressDetails{Address: addr, Chain: chain}
		}
		return wallet, err
	}
	return wallet, nil

}

// Insert inserts a wallet by its address and chain
func (w *WalletRepository) Insert(ctx context.Context, addr persist.AddressValue, chain persist.Chain, walletType persist.WalletType) (persist.DBID, error) {

	_, err := w.insertAddressStmt.ExecContext(ctx, persist.GenerateID(), 0, addr, chain, walletType)
	if err != nil {
		return "", err
	}
	var addressID persist.DBID
	err = w.getAddressByDetailsStmt.QueryRowContext(ctx, addr, chain).Scan(&addressID)
	if err != nil {
		return "", err
	}

	_, err = w.insertStmt.ExecContext(ctx, persist.GenerateID(), 0, addressID, walletType)
	if err != nil {
		return "", err
	}

	wa, err := w.GetByAddressDetails(ctx, addr, chain)
	if err != nil {
		return "", err
	}

	return wa.ID, nil
}
