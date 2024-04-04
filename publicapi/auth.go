package publicapi

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/service/auth/privy"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/util"
	"time"

	magicclient "github.com/magiclabs/magic-admin-go/client"
	"github.com/magiclabs/magic-admin-go/token"
	"github.com/mikeydub/go-gallery/service/persist/postgres"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/debugtools"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
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
	magicLinkClient    *magicclient.API
	oneTimeLoginCache  *redis.Cache
	authRefreshCache   *redis.Cache
	privyClient        *privy.Client
}

func (api AuthAPI) NewNonceAuthenticator(chainAddress persist.ChainPubKey, nonce string, message string, signature string, walletType persist.WalletType) auth.Authenticator {
	authenticator := auth.NonceAuthenticator{
		ChainPubKey:        chainAddress,
		Nonce:              nonce,
		Message:            message,
		Signature:          signature,
		WalletType:         walletType,
		MultichainProvider: api.multiChainProvider,
		EthClient:          api.ethClient,
		Queries:            api.queries,
	}
	return authenticator
}

func (api AuthAPI) NewDebugAuthenticator(ctx context.Context, debugParams model.DebugAuth) (auth.Authenticator, error) {
	if !debugtools.Enabled || !debugtools.IsDebugEnv() {
		return nil, fmt.Errorf("debug auth is only allowed in local and development environments with debugtools enabled")
	}

	password := util.FromPointer(debugParams.DebugToolsPassword)

	if debugParams.AsUsername == nil {
		if debugParams.ChainAddresses == nil {
			return nil, fmt.Errorf("debug auth failed: either asUsername or chainAddresses must be specified")
		}

		userID := persist.DBID("")
		if debugParams.UserID != nil {
			userID = *debugParams.UserID
		}

		var user *db.User
		u, err := api.queries.GetUserById(ctx, userID)
		if err == nil {
			user = &u
		}

		return debugtools.NewDebugAuthenticator(user, chainAddressPointersToChainAddresses(debugParams.ChainAddresses), password), nil
	}

	if debugParams.UserID != nil || debugParams.ChainAddresses != nil {
		return nil, fmt.Errorf("debug auth failed: asUsername parameter cannot be used in conjunction with userId or chainAddresses parameters")
	}

	username := *debugParams.AsUsername
	if username == "" {
		return nil, fmt.Errorf("debug auth failed: asUsername parameter cannot be empty")
	}

	user, err := api.queries.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("debug auth failed for user '%s': %w", username, err)
	}

	wallets, err := api.queries.GetWalletsByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("debug auth failed for user '%s': %w", username, err)
	}

	var addresses []persist.ChainAddress
	for _, wallet := range wallets {
		addresses = append(addresses, persist.NewChainAddress(wallet.Address, wallet.Chain))
	}

	return debugtools.NewDebugAuthenticator(&user, addresses, password), nil
}

func (api AuthAPI) NewMagicLinkAuthenticator(token token.Token) auth.Authenticator {
	authenticator := auth.MagicLinkAuthenticator{
		Token:       token,
		MagicClient: api.magicLinkClient,
		Queries:     api.queries,
	}
	return authenticator
}

func (api AuthAPI) NewOneTimeLoginTokenAuthenticator(loginToken string) auth.Authenticator {
	authenticator := auth.OneTimeLoginTokenAuthenticator{
		ConsumedTokenCache: api.oneTimeLoginCache,
		Queries:            api.queries,
		LoginToken:         loginToken,
	}
	return authenticator
}

func (api AuthAPI) NewPrivyAuthenticator(authToken string) auth.Authenticator {
	return privy.NewAuthenticator(api.repos.UserRepository, api.queries, api.privyClient, authToken)
}

func chainAddressPointersToChainAddresses(chainAddresses []*persist.ChainAddress) []persist.ChainAddress {
	addresses := make([]persist.ChainAddress, 0, len(chainAddresses))

	for _, address := range chainAddresses {
		if address != nil {
			addresses = append(addresses, *address)
		}
	}

	return addresses
}

func (api AuthAPI) GetAuthNonce(ctx context.Context) (nonce string, message string, err error) {
	return auth.GenerateAuthNonce(ctx, api.queries)
}

func (api AuthAPI) Login(ctx context.Context, authenticator auth.Authenticator) (persist.DBID, error) {
	// Nothing to validate
	return auth.Login(ctx, api.queries, authenticator)
}

func (api AuthAPI) Logout(ctx context.Context) {
	// Nothing to validate
	auth.Logout(ctx, api.queries, api.authRefreshCache)
}

func (api AuthAPI) GenerateQRCodeLoginToken(ctx context.Context) (string, error) {
	// Nothing to validate

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	return auth.GenerateOneTimeLoginToken(ctx, userID, "qr_code", 5*time.Minute)
}

func (api AuthAPI) ForceAuthTokenRefresh(ctx context.Context, userID persist.DBID) error {
	return auth.ForceAuthTokenRefresh(ctx, api.authRefreshCache, userID)
}
