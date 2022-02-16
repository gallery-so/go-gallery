package graphql

// schema.resolvers.go gets updated when generating gqlgen bindings and should not contain
// helper functions. schema.resolvers.helpers.go is a companion file that can contain
// helper functions without interfering with code generation.

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
)

var errNoAuthMechanismFound = fmt.Errorf("no auth mechanism found")

// errorToGraphqlType converts a golang error to its matching type from our GraphQL schema.
// If no matching type is found, ok will return false
func (r *Resolver) errorToGraphqlType(err error) (gqlError model.Error, ok bool) {
	message := err.Error()
	var mappedErr model.Error = nil

	switch err.(type) {
	case auth.ErrSignatureVerificationFailed:
		mappedErr = model.ErrSignatureVerificationFailed{Message: message}
	case auth.ErrAddressDoesNotOwnRequiredNFT:
		mappedErr = model.ErrAddressDoesNotOwnRequiredNft{Message: message}
	case auth.ErrUserNotFound:
		mappedErr = model.ErrUserNotFound{Message: message}
	case user.ErrUserAlreadyExists:
		mappedErr = model.ErrUserAlreadyExists{Message: message}
	}

	if mappedErr != nil {
		return mappedErr, true
	}

	return nil, false
}

// authMechanismToAuthenticator takes a GraphQL AuthMechanism and returns an Authenticator that can be used for auth
func (r *Resolver) authMechanismToAuthenticator(m model.AuthMechanism) (auth.Authenticator, error) {

	ethNonceAuth := func(address string, nonce string, signature string, walletType auth.WalletType) auth.Authenticator {
		authenticator := auth.EthereumNonceAuthenticator{
			Address:    address,
			Nonce:      nonce,
			Signature:  signature,
			WalletType: walletType,
			UserRepo:   r.Repos.UserRepository,
			NonceRepo:  r.Repos.NonceRepository,
			EthClient:  r.EthClient,
		}
		return authenticator
	}

	if m.EthereumEoa != nil {
		return ethNonceAuth(m.EthereumEoa.Address, m.EthereumEoa.Nonce, m.EthereumEoa.Signature, auth.WalletTypeEOA), nil
	}

	if m.GnosisSafe != nil {
		return ethNonceAuth(m.GnosisSafe.Address, m.GnosisSafe.Nonce, m.GnosisSafe.Signature, auth.WalletTypeGnosis), nil
	}

	return nil, errNoAuthMechanismFound
}

func resolveGalleryUserByUserId(ctx context.Context, r *Resolver, userID string) (*model.GalleryUser, error) {
	user, err := dataloader.For(ctx).UserByUserId.Load(userID)

	if err != nil {
		return nil, err
	}

	return userToUserModel(ctx, r, user)
}

func resolveGalleryUserByUsername(ctx context.Context, r *Resolver, username string) (*model.GalleryUser, error) {
	user, err := dataloader.For(ctx).UserByUsername.Load(username)

	if err != nil {
		return nil, err
	}

	return userToUserModel(ctx, r, user)
}

func resolveGalleryUserByAddress(ctx context.Context, r *Resolver, address string) (*model.GalleryUser, error) {
	user, err := dataloader.For(ctx).UserByAddress.Load(address)

	if err != nil {
		return nil, err
	}

	return userToUserModel(ctx, r, user)
}

// userToUserModel converts a persist.User to a model.User
func userToUserModel(ctx context.Context, r *Resolver, user persist.User) (*model.GalleryUser, error) {
	gc := util.GinContextFromContext(ctx)
	isAuthenticated := auth.GetUserAuthedFromCtx(gc)

	output := &model.GalleryUser{
		ID:                  user.ID.String(),
		Username:            util.StringToPointer(user.Username.String()),
		Bio:                 util.StringToPointer(user.Bio.String()),
		Wallets:             addressesToWalletModels(ctx, r, user.Addresses),
		Galleries:           nil, // handled by dedicated resolver
		IsAuthenticatedUser: &isAuthenticated,
	}

	return output, nil
}

// addressesToWalletModels converts a slice of persist.Address to a slice of model.Wallet
func addressesToWalletModels(ctx context.Context, r *Resolver, addresses []persist.Address) []*model.Wallet {
	wallets := make([]*model.Wallet, len(addresses))
	for i, address := range addresses {
		wallets[i] = &model.Wallet{
			ID:      "", // TODO: What's a wallet's ID?
			Address: util.StringToPointer(address.String()),
			Nfts:    nil, // handled by dedicated resolver
		}
	}

	return wallets
}
