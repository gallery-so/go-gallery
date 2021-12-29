package postgres

import (
	"context"
	"testing"

	"github.com/mikeydub/go-gallery/service/persist"
)

func TestUserCreate_Success(t *testing.T) {
	a, db := setupTest(t)

	userRepo := NewUserRepository(db)

	user := persist.User{
		Deleted:            false,
		Version:            1,
		Username:           "username",
		UsernameIdempotent: "username-idempotent",
		Addresses: []persist.Address{
			"0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
		},
	}

	_, err := userRepo.Create(context.Background(), user)
	a.NoError(err)
}

func TestUserGetByID_Success(t *testing.T) {
	a, db := setupTest(t)

	userRepo := NewUserRepository(db)

	user := persist.User{
		Deleted:            false,
		Version:            1,
		Username:           "username",
		UsernameIdempotent: "username-idempotent",
		Addresses: []persist.Address{
			"0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
		},
	}

	id, err := userRepo.Create(context.Background(), user)
	a.NoError(err)

	user2, err := userRepo.GetByID(context.Background(), id)
	a.NoError(err)
	a.Equal(id, user2.ID)
	a.Equal(user.Addresses, user2.Addresses)
	a.Equal(user.Username, user2.Username)
}

func TestUserGetByAddress_Success(t *testing.T) {
	a, db := setupTest(t)

	userRepo := NewUserRepository(db)

	user := persist.User{
		Deleted:            false,
		Version:            1,
		Username:           "username",
		UsernameIdempotent: "username-idempotent",
		Addresses: []persist.Address{
			"0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
		},
	}

	id, err := userRepo.Create(context.Background(), user)
	a.NoError(err)

	user2, err := userRepo.GetByAddress(context.Background(), user.Addresses[0])
	a.NoError(err)
	a.Equal(id, user2.ID)
	a.Equal(user.Addresses, user2.Addresses)
	a.Equal(user.Username, user2.Username)
}
