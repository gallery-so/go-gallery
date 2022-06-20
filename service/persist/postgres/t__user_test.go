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
		Wallets: []persist.Wallet{
			persist.Wallet{
				Address: persist.NullString("0x8914496dc01efcc49a2fa340331fb90969b6f1d2"),
				Chain:   persist.ChainETH,
			},
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
		Wallets: []persist.Wallet{
			persist.Wallet{
				Address: persist.NullString("0x8914496dc01efcc49a2fa340331fb90969b6f1d2"),
				Chain:   persist.ChainETH,
			},
		},
	}

	id, err := userRepo.Create(context.Background(), user)
	a.NoError(err)

	user2, err := userRepo.GetByID(context.Background(), id)
	a.NoError(err)
	a.Equal(id, user2.ID)
	a.Equal(user.Wallets, user2.Wallets)
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
		Wallets: []persist.Wallet{
			persist.Wallet{
				Address: persist.NullString("0x8914496dc01efcc49a2fa340331fb90969b6f1d2"),
				Chain:   persist.ChainETH,
			},
		},
	}

	id, err := userRepo.Create(context.Background(), user)
	a.NoError(err)

	user2, err := userRepo.GetByAddress(context.Background(), user.Wallets[0])
	a.NoError(err)
	a.Equal(id, user2.ID)
	a.Equal(user.Wallets, user2.Wallets)
	a.Equal(user.Username, user2.Username)
}
