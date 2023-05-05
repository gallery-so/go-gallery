package postgres

import (
	"context"
	"database/sql"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// NonceRepository is a repository for user nonces
type NonceRepository struct {
	db                    *sql.DB
	queries               *db.Queries
	getByChainAddressStmt *sql.Stmt
	createStmt            *sql.Stmt
}

// NewNonceRepository creates a new postgres repository for interacting with user nonces
func NewNonceRepository(db *sql.DB, queries *db.Queries) *NonceRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getByChainAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VALUE,ADDRESS,VERSION,DELETED,CREATED_AT,LAST_UPDATED FROM nonces WHERE ADDRESS = $1 AND CHAIN = $2 ORDER BY LAST_UPDATED DESC LIMIT 1`)
	checkNoErr(err)

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO nonces (ID,VALUE,ADDRESS,CHAIN,VERSION,DELETED) VALUES ($1,$2,$3,$4,$5,$6)`)
	checkNoErr(err)

	return &NonceRepository{db: db, queries: queries, getByChainAddressStmt: getByChainAddressStmt, createStmt: createStmt}
}

// Get returns a nonce from the DB by its address
func (n *NonceRepository) Get(pCtx context.Context, pChainAddress persist.ChainAddress) (persist.UserNonce, error) {
	var nonce persist.UserNonce
	err := n.getByChainAddressStmt.QueryRowContext(pCtx, pChainAddress.Address(), pChainAddress.Chain()).Scan(&nonce.ID, &nonce.Value, &nonce.Address, &nonce.Version, &nonce.Deleted, &nonce.CreationTime, &nonce.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.UserNonce{}, persist.ErrNonceNotFoundForAddress{ChainAddress: pChainAddress}
		}
		return persist.UserNonce{}, err
	}
	return nonce, nil
}

// Create creates a new nonce in the DB
func (n *NonceRepository) Create(pCtx context.Context, pNonceValue string, pChainAddress persist.ChainAddress) error {
	_, err := n.createStmt.ExecContext(pCtx, persist.GenerateID(), pNonceValue, pChainAddress.Address(), pChainAddress.Chain(), 0, false)
	return err
}
