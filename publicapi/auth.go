package publicapi

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type AuthAPI struct {
	repos             *persist.Repositories
	queries           *sqlc.Queries
	loaders           *dataloader.Loaders
	validator         *validator.Validate
	ethClient         *ethclient.Client
	multiChainProvier *multichain.Provider
}

func (api AuthAPI) NewNonceAuthenticator(chainAddress persist.ChainAddress, nonce string, signature string, walletType persist.WalletType) auth.Authenticator {
	authenticator := auth.NonceAuthenticator{
		ChainAddress:       chainAddress,
		Nonce:              nonce,
		Signature:          signature,
		WalletType:         walletType,
		WalletRepo:         api.repos.WalletRepository,
		MultichainProvider: api.multiChainProvier,
		UserRepo:           api.repos.UserRepository,
		NonceRepo:          api.repos.NonceRepository,
		EthClient:          api.ethClient,
	}
	return authenticator
}

func (api AuthAPI) GetAuthNonce(ctx context.Context, chainAddress persist.ChainAddress) (nonce string, userExists bool, err error) {
	gc := util.GinContextFromContext(ctx)
	authed := auth.GetUserAuthedFromCtx(gc)

	return auth.GetAuthNonce(ctx, chainAddress, authed, api.repos.UserRepository, api.repos.NonceRepository, api.repos.WalletRepository, api.ethClient)
}

func (api AuthAPI) Login(ctx context.Context, authenticator auth.Authenticator) (persist.DBID, error) {
	// Nothing to validate
	return auth.Login(ctx, authenticator)
}

func (api AuthAPI) Logout(ctx context.Context) {
	// Nothing to validate
	auth.Logout(ctx)
}
