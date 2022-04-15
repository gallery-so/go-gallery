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

	getByAddressStmt *sql.Stmt
	getByUserIDStmt  *sql.Stmt
	upsertStmt       *sql.Stmt
}

// NewWalletRepository creates a new postgres repository for interacting with wallets
func NewWalletRepository(db *sql.DB) *WalletRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,CHAIN,USER_ID FROM wallets WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	getByUserIDStmt, err := db.PrepareContext(ctx, `SELECT ID,VERSION,CREATED_AT,LAST_UPDATED,ADDRESS,CHAIN,USER_ID FROM wallets WHERE USER_ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	upsertStmt, err := db.PrepareContext(ctx, `INSERT INTO wallets (ID,VERSION,ADDRESS,CHAIN,USER_ID) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (ADDRESS,CHAIN) DO UPDATE SET VERSION = $2, USER_ID = $5;`)
	checkNoErr(err)

	return &WalletRepository{
		db:               db,
		getByAddressStmt: getByAddressStmt,
		getByUserIDStmt:  getByUserIDStmt,
		upsertStmt:       upsertStmt,
	}
}

// GetByAddress returns a wallet by address and chain
func (w *WalletRepository) GetByAddress(ctx context.Context, addr persist.Address, chain persist.Chain) (persist.Wallet, error) {
	var wallet persist.Wallet
	err := w.getByAddressStmt.QueryRowContext(ctx, addr, chain).Scan(&wallet.ID, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated, &wallet.Address, &wallet.Chain, &wallet.UserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return wallet, persist.ErrWalletNotFoundByAddress{Address: addr, Chain: chain}
		}
		return wallet, err
	}
	return wallet, nil

}

// GetByUserID returns all wallets for a user
func (w *WalletRepository) GetByUserID(ctx context.Context, userID persist.DBID) ([]persist.Wallet, error) {

	res := make([]persist.Wallet, 0, 5)
	rows, err := w.getByUserIDStmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var wallet persist.Wallet
		err := rows.Scan(&wallet.ID, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated, &wallet.Address, &wallet.Chain, &wallet.UserID)
		if err != nil {
			return nil, err
		}
		res = append(res, wallet)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

// Upsert upserts a wallet by its address and chain
func (w *WalletRepository) Upsert(ctx context.Context, addr persist.Address, chain persist.Chain, userID persist.DBID) error {
	_, err := w.upsertStmt.ExecContext(ctx, persist.GenerateID(), 0, addr, userID, chain)
	return err
}
