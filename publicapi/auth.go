package publicapi

import (
	"context"
	"github.com/mikeydub/go-gallery/service/persist/postgres"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
)

type AuthAPI struct {
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multiChainProvider *multichain.Provider
}

func (api AuthAPI) NewNonceAuthenticator(chainAddress persist.ChainPubKey, nonce string, signature string, walletType persist.WalletType) auth.Authenticator {
	authenticator := auth.NonceAuthenticator{
		ChainPubKey:        chainAddress,
		Nonce:              nonce,
		Signature:          signature,
		WalletType:         walletType,
		WalletRepo:         api.repos.WalletRepository,
		MultichainProvider: api.multiChainProvider,
		UserRepo:           api.repos.UserRepository,
		NonceRepo:          api.repos.NonceRepository,
		EthClient:          api.ethClient,
	}
	return authenticator
}

func (api AuthAPI) GetAuthNonce(ctx context.Context, chainAddress persist.ChainAddress) (nonce string, userExists bool, err error) {
	return auth.GetAuthNonce(ctx, chainAddress, api.repos.UserRepository, api.repos.NonceRepository, api.repos.WalletRepository, api.repos.EarlyAccessRepository, api.ethClient)
}

func (api AuthAPI) Login(ctx context.Context, authenticator auth.Authenticator) (persist.DBID, error) {
	// Nothing to validate
	return auth.Login(ctx, authenticator)
}

func (api AuthAPI) Logout(ctx context.Context) {
	// Nothing to validate
	auth.Logout(ctx)
}
