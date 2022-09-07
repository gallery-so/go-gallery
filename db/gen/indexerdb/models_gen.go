// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.14.0

package indexerdb

import (
	"database/sql"
	"time"

	"github.com/jackc/pgtype"
	"github.com/mikeydub/go-gallery/service/persist"
)

type AddressFilter struct {
	ID          persist.DBID
	FromBlock   persist.BlockNumber
	ToBlock     persist.BlockNumber
	BloomFilter []byte
	CreatedAt   time.Time
	LastUpdated time.Time
	Deleted     bool
}

type Contract struct {
	ID             persist.DBID
	Deleted        bool
	Version        sql.NullInt32
	CreatedAt      time.Time
	LastUpdated    time.Time
	Name           sql.NullString
	Symbol         sql.NullString
	Address        sql.NullString
	CreatorAddress sql.NullString
	Chain          sql.NullInt32
	LatestBlock    sql.NullInt64
}

type Token struct {
	ID               persist.DBID
	Deleted          bool
	Version          sql.NullInt32
	CreatedAt        time.Time
	LastUpdated      time.Time
	Name             sql.NullString
	Description      sql.NullString
	ContractAddress  sql.NullString
	Media            pgtype.JSONB
	OwnerAddress     sql.NullString
	TokenUri         sql.NullString
	TokenType        sql.NullString
	TokenID          sql.NullString
	Quantity         sql.NullString
	OwnershipHistory []pgtype.JSONB
	TokenMetadata    pgtype.JSONB
	ExternalUrl      sql.NullString
	BlockNumber      sql.NullInt64
	Chain            sql.NullInt32
	IsSpam           sql.NullBool
}
