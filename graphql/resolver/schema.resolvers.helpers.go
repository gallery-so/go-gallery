package graphql

// schema.resolvers.go gets updated when generating gqlgen bindings and should not contain
// helper functions. schema.resolvers.helpers.go is a companion file that can contain
// helper functions without interfering with code generation.

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/user"
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
	case user.ErrUserExistsWithAddress:
		mappedErr = model.ErrUserExistsWithAddress{Message: message}
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

func (r *mutationResolver) CreateUserWithEthereum(ctx context.Context, address string, nonce string, signature string, walletType auth.WalletType) (model.CreateUserPayload, error) {
	gc := GinContextFromContext(ctx)

	output, err := user.CreateUser(ctx, address, nonce, signature, walletType, r.Repos.UserRepository, r.Repos.NonceRepository, r.Repos.GalleryRepository, r.EthClient)
	if err != nil {
		// Map known errors to GraphQL return types
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.CreateUserPayload); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	auth.SetJWTCookie(gc, *output.JwtToken)
	return output, nil
}
