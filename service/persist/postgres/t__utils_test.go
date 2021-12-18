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

	c := NewPostgresClient()

	t.Cleanup(func() {
		defer c.Close()
		dropSQL := `TRUNCATE users, nfts, collections, galleries;`
		_, err := c.Exec(dropSQL)
		if err != nil {
			t.Logf("error dropping tables: %v", err)
		}
	})

	return assert.New(t), c
}
