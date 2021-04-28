package db

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4/pgxpool"
)

//-------------------------------------------------------------
type Storage interface {
	GetNFTsByUserID(ctx context.Context, userID string) ([]*NFT, error)
	Cleanup()
}

type DB struct {
	pool *pgxpool.Pool
}

//-------------------------------------------------------------
func NewDB(ctx context.Context, uri string) (*DB, error) {
	pool, err := pgxpool.Connect(ctx, uri)
	if err != nil {
		return nil, err
	}

	return &DB{pool: pool}, nil
}

//-------------------------------------------------------------
func (db *DB) GetNFTsByUserID(ctx context.Context, userID string) ([]*NFT, error) {
	var nfts []*NFT

	query := `
SELECT
	id,
	user_id,
	image_url,
--	description
	name,
	collection_name,
	position,
	external_url,
	created_date,
	creator_address,
	contract_address,
--	token_id,
	hidden,
	image_thumbnail_url,
	image_preview_url
FROM nfts
WHERE user_id='%s'
`
	err := pgxscan.Select(ctx, db.pool, &nfts, fmt.Sprintf(query, userID))
	if err != nil {
		return nil, err
	}

	return nfts, nil
}

//-------------------------------------------------------------
func (db *DB) Cleanup() {
	db.pool.Close()
}
