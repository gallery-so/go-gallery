package postgres

import (
	"database/sql"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func setupTest(t *testing.T) (*assert.Assertions, *sql.DB) {
	viper.Set("POSTGRES_HOST", "0.0.0.0")
	viper.Set("POSTGRES_PORT", 5432)
	viper.Set("POSTGRES_USER", "postgres")
	viper.Set("POSTGRES_PASSWORD", "")
	viper.Set("POSTGRES_DB", "postgres")
	viper.Set("ENV", "local")

	db := NewClient()

	t.Cleanup(func() {
		defer db.Close()
		dropSQL := `TRUNCATE users, nfts, collections, galleries;`
		_, err := db.Exec(dropSQL)
		if err != nil {
			t.Logf("error dropping tables: %v", err)
		}
	})

	return assert.New(t), db
}
