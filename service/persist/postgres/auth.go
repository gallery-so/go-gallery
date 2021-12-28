package postgres

import (
	"context"
	"database/sql"

	"github.com/mikeydub/go-gallery/service/persist"
)

// LoginRepository is a repository for user login attempts
type LoginRepository struct {
	db *sql.DB
}

// NonceRepository is a repository for user nonces
type NonceRepository struct {
	db *sql.DB
}

// NewLoginRepository creates a new postgres repository for interacting with user login attempts
func NewLoginRepository(db *sql.DB) *LoginRepository {
	return &LoginRepository{db: db}
}

// NewNonceRepository creates a new postgres repository for interacting with user nonces
func NewNonceRepository(db *sql.DB) *NonceRepository {
	return &NonceRepository{db: db}
}

// Get returns a nonce from the DB by its address
func (n *NonceRepository) Get(pCtx context.Context, pAddress persist.Address) (persist.UserNonce, error) {
	sqlStr := `SELECT ID,VALUE,ADDRESS,VERSION,DELETED,CREATED_AT,LAST_UPDATED FROM nonces WHERE ADDRESS = $1`
	var nonce persist.UserNonce
	err := n.db.QueryRowContext(pCtx, sqlStr, pAddress).Scan(&nonce.ID, &nonce.Value, &nonce.Address, &nonce.Version, &nonce.Deleted, &nonce.CreationTime, &nonce.LastUpdated)
	if err != nil {
		return persist.UserNonce{}, err
	}
	return nonce, nil
}

// Create creates a new nonce in the DB
func (n *NonceRepository) Create(pCtx context.Context, pNonce persist.UserNonce) error {
	sqlStr := `INSERT INTO nonces (ID,VALUE,ADDRESS,VERSION,DELETED) VALUES ($1,$2,$3,$4,$5)`
	_, err := n.db.ExecContext(pCtx, sqlStr, pNonce.ID, pNonce.Value, pNonce.Address, pNonce.Version, pNonce.Deleted)
	return err
}

// Create creates a new login attempt in the DB
func (l *LoginRepository) Create(pCtx context.Context, pAttempt persist.UserLoginAttempt) (persist.DBID, error) {
	sqlStr := `INSERT INTO login_attempts (ID,USER_EXISTS,ADDRESS,VERSION,NONCE_VALUE,REQUEST_HEADERS,REQUEST_HOST_ADDRESS,SIGNATURE,SIGNATURE_VALID) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING ID`
	var id persist.DBID
	err := l.db.QueryRowContext(pCtx, sqlStr, pAttempt.ID, pAttempt.UserExists, pAttempt.Address, pAttempt.Version, pAttempt.NonceValue, pAttempt.ReqHeaders, pAttempt.ReqHostAddr, pAttempt.Signature).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}
