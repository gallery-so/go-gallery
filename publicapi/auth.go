package publicapi

import (
	"context"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
)

type AuthAPI struct {
	repos     *persist.Repositories
	queries   *sqlc.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api AuthAPI) NewEthereumNonceAuthenticator(address persist.Address, nonce string, signature string, walletType auth.WalletType) auth.Authenticator {
	authenticator := auth.EthereumNonceAuthenticator{
		Address:    address,
		Nonce:      nonce,
		Signature:  signature,
		WalletType: walletType,
		UserRepo:   api.repos.UserRepository,
		NonceRepo:  api.repos.NonceRepository,
		EthClient:  api.ethClient,
	}
	return authenticator
}

func (api AuthAPI) Login(ctx context.Context, authenticator auth.Authenticator) (persist.DBID, error) {
	// Nothing to validate
	return auth.Login(ctx, authenticator)
}

func (api AuthAPI) Logout(ctx context.Context) {
	// Nothing to validate
	auth.Logout(ctx)
}
