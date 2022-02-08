package graphql

// schema.resolvers.go gets updated when generating gqlgen bindings and should not contain
// helper functions. schema.resolvers.helpers.go is a companion file that can contain
// helper functions without interfering with code generation.

import (
	"context"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/user"
)

func (r *Resolver) ErrorToGraphqlType(err error) (model.Error, bool) {
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

func (r *mutationResolver) CreateUserWithEthereum(ctx context.Context, address string, nonce string, signature string, walletType auth.WalletType) (model.CreateUserPayload, error) {
	gc := GinContextFromContext(ctx)

	output, err := user.CreateUser(ctx, address, nonce, signature, walletType, r.Repos.UserRepository, r.Repos.NonceRepository, r.Repos.GalleryRepository, r.EthClient)
	if err != nil {
		// Map known errors to GraphQL return types
		if errorType, ok := r.ErrorToGraphqlType(err); ok {
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

func (r *mutationResolver) LoginWithEthereum(ctx context.Context, address string, nonce string, signature string, walletType auth.WalletType) (model.LoginPayload, error) {
	gc := GinContextFromContext(ctx)

	output, err := auth.LoginAndRecordAttempt(ctx, address, nonce, signature, walletType, gc.Request, r.Repos.UserRepository, r.Repos.NonceRepository, r.Repos.LoginRepository, r.EthClient)
	if err != nil {
		// Map known errors to GraphQL return types
		if errorType, ok := r.ErrorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.LoginPayload); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	auth.SetJWTCookie(gc, *output.JwtToken)
	return output, nil
}
