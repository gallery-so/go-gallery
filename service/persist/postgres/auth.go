package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// LoginRepository is a repository for user login attempts
type LoginRepository struct {
	db         *sql.DB
	createStmt *sql.Stmt
}

// NonceRepository is a repository for user nonces
type NonceRepository struct {
	db                    *sql.DB
	getByChainAddressStmt *sql.Stmt
	createStmt            *sql.Stmt
}

// NewLoginRepository creates a new postgres repository for interacting with user login attempts
func NewLoginRepository(db *sql.DB) *LoginRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO login_attempts (ID,USER_EXISTS,ADDRESS,VERSION,NONCE_VALUE,REQUEST_HEADERS,REQUEST_HOST_ADDRESS,SIGNATURE,SIGNATURE_VALID) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING ID`)
	checkNoErr(err)

	return &LoginRepository{db: db, createStmt: createStmt}
}

// NewNonceRepository creates a new postgres repository for interacting with user nonces
func NewNonceRepository(db *sql.DB) *NonceRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getByChainAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,VALUE,ADDRESS,VERSION,DELETED,CREATED_AT,LAST_UPDATED FROM nonces WHERE ADDRESS = $1 AND CHAIN = $2 ORDER BY LAST_UPDATED DESC LIMIT 1`)
	checkNoErr(err)

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO nonces (ID,VALUE,ADDRESS,CHAIN,VERSION,DELETED) VALUES ($1,$2,$3,$4,$5,$6)`)
	checkNoErr(err)

	return &NonceRepository{db: db, getByChainAddressStmt: getByChainAddressStmt, createStmt: createStmt}
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

// Create creates a new login attempt in the DB
func (l *LoginRepository) Create(pCtx context.Context, pAttempt persist.CreateLoginAttemptInput) (persist.DBID, error) {
	var id persist.DBID
	err := l.createStmt.QueryRowContext(pCtx, persist.GenerateID(), pAttempt.UserExists, pAttempt.Address.ID, 0, pAttempt.NonceValue, pAttempt.ReqHeaders, pAttempt.ReqHostAddr, pAttempt.Signature, pAttempt.SignatureValid).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}
