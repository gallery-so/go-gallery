package publicapi

import (
	"context"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
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

func (api AuthAPI) GetLoggedInUserId(ctx context.Context) persist.DBID {
	gc := util.GinContextFromContext(ctx)
	return auth.GetUserIDFromCtx(gc)
}

func (api AuthAPI) IsUserLoggedIn(ctx context.Context) bool {
	gc := util.GinContextFromContext(ctx)
	return auth.GetUserAuthedFromCtx(gc)
}

func (api AuthAPI) GetAuthNonce(ctx context.Context, address persist.Address) (nonce string, userExists bool, err error) {
	gc := util.GinContextFromContext(ctx)
	authed := auth.GetUserAuthedFromCtx(gc)

	return auth.GetAuthNonce(ctx, address, authed, api.repos.UserRepository, api.repos.NonceRepository, api.ethClient)
}

func (api AuthAPI) Login(ctx context.Context, authenticator auth.Authenticator) (persist.DBID, error) {
	// Nothing to validate
	return auth.Login(ctx, authenticator)
}

func (api AuthAPI) Logout(ctx context.Context) {
	// Nothing to validate
	auth.Logout(ctx)
}
