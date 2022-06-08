package postgres

import (
	"context"
	"database/sql"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
	"strings"
	"time"
)

type EarlyAccessRepository struct {
	db                    *sql.DB
	existsByAddressesStmt *sql.Stmt
}

func NewEarlyAccessRepository(db *sql.DB) *EarlyAccessRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	existsByAddressesStmt, err := db.PrepareContext(ctx, `SELECT EXISTS(SELECT 1 FROM early_access WHERE address = ANY($1));`)
	checkNoErr(err)

	return &EarlyAccessRepository{
		db:                    db,
		existsByAddressesStmt: existsByAddressesStmt,
	}
}

func (u *EarlyAccessRepository) IsAllowedByAddresses(ctx context.Context, addresses []persist.Address) (bool, error) {
	lowerAddresses := make([]string, len(addresses))
	for i, address := range addresses {
		lowerAddresses[i] = strings.ToLower(address.String())
	}

	var allowed bool
	err := u.existsByAddressesStmt.QueryRowContext(ctx, pq.Array(addresses)).Scan(&allowed)
	if err != nil {
		return false, err
	}

	return allowed, nil
}
