package memstore

import (
	"testing"

	"github.com/mikeydub/go-gallery/docker"
	"github.com/stretchr/testify/assert"
)

func setupTest(t *testing.T) *assert.Assertions {
	rd, rdUnpatch := docker.InitRedis()

	t.Cleanup(func() {
		defer rdUnpatch()
		if err := rd.Close(); err != nil {
			t.Fatalf("could not purge resource: %s", err)
		}
	})

	return assert.New(t)
}
