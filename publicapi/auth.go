package publicapi

import (
	"context"
	"fmt"

	magicclient "github.com/magiclabs/magic-admin-go/client"
	"github.com/magiclabs/magic-admin-go/token"
	"github.com/mikeydub/go-gallery/env"
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

func (api AuthAPI) NewDebugAuthenticator(ctx context.Context, debugParams model.DebugAuth) (auth.Authenticator, error) {
	if !debugtools.Enabled || env.Get[string](ctx, "ENV") != "local" {
		return nil, fmt.Errorf("debug auth is only allowed in local environments with debugtools enabled")
	}

	if debugParams.AsUsername == nil {
		if debugParams.ChainAddresses == nil {
			return nil, fmt.Errorf("debug auth failed: either asUsername or chainAddresses must be specified")
		}

		userID := persist.DBID("")
		if debugParams.UserID != nil {
			userID = *debugParams.UserID
		}

		var user *persist.User
		dbUser, err := api.repos.UserRepository.GetByID(ctx, userID)
		if err == nil {
			user = &dbUser
		}

		return debugtools.NewDebugAuthenticator(user, chainAddressPointersToChainAddresses(debugParams.ChainAddresses)), nil
	}

	if debugParams.UserID != nil || debugParams.ChainAddresses != nil {
		return nil, fmt.Errorf("debug auth failed: asUsername parameter cannot be used in conjunction with userId or chainAddresses parameters")
	}

	username := *debugParams.AsUsername
	if username == "" {
		return nil, fmt.Errorf("debug auth failed: asUsername parameter cannot be empty")
	}

	user, err := api.repos.UserRepository.GetByUsername(ctx, username)
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

	return debugtools.NewDebugAuthenticator(&user, addresses), nil
}

func (api AuthAPI) NewMagicLinkAuthenticator(token token.Token) auth.Authenticator {
	authenticator := auth.MagicLinkAuthenticator{
		Token:       token,
		MagicClient: api.magicLinkClient,
		UserRepo:    api.repos.UserRepository,
	}
	return authenticator
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
