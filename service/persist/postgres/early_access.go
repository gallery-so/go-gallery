package postgres

import (
	"context"
	"database/sql"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

type EarlyAccessRepository struct {
	db                    *sql.DB
	queries               *db.Queries
	existsByAddressesStmt *sql.Stmt
}

func NewEarlyAccessRepository(db *sql.DB, queries *db.Queries) *EarlyAccessRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	existsByAddressesStmt, err := db.PrepareContext(ctx, `SELECT EXISTS(SELECT 1 FROM early_access WHERE address = ANY($1));`)
	checkNoErr(err)

	return &EarlyAccessRepository{
		db:                    db,
		queries:               queries,
		existsByAddressesStmt: existsByAddressesStmt,
	}
}

func (u *EarlyAccessRepository) IsAllowedByAddresses(ctx context.Context, chainAddresses []persist.ChainAddress) (bool, error) {
	addresses := make([]string, len(chainAddresses))
	for i, chainAddress := range chainAddresses {
		addresses[i] = chainAddress.Address().String()
	}

	var allowed bool
	err := u.existsByAddressesStmt.QueryRowContext(ctx, pq.Array(addresses)).Scan(&allowed)
	if err != nil {
		return false, err
	}

	return allowed, nil
}
