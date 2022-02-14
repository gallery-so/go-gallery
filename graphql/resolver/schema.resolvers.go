package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
)

func (r *mutationResolver) CreateCollection(ctx context.Context, input model.CreateCollectionInput) (*model.CreateCollectionPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) DeleteCollection(ctx context.Context, collectionID *int) (*model.DeleteCollectionPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) UpdateCollectionInfo(ctx context.Context, input model.UpdateCollectionInfoInput) (*model.UpdateCollectionInfoPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) UpdateCollectionNfts(ctx context.Context, input model.UpdateCollectionNftsInput) (*model.UpdateCollectionNftsPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) UpdateGalleryCollections(ctx context.Context, input *model.UpdateGalleryCollectionsInput) (*model.UpdateGalleryCollectionsPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) RemoveUserAddress(ctx context.Context, address string) (*model.RemoveUserAddressPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) UpdateUserInfo(ctx context.Context, input *model.UpdateUserInfoInput) (*model.UpdateUserInfoPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) RefreshOpenSeaNfts(ctx context.Context) (*model.RefreshOpenSeaNftsPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) GetAuthNonce(ctx context.Context, address string) (model.GetAuthNoncePayload, error) {
	gc := util.GinContextFromContext(ctx)

	// TODO: This currently gets its value from the AuthOptional middleware applied to the entire GraphQL endpoint,
	// but that won't suffice when we transition auth-only endpoints to GraphQL too.
	authed := gc.GetBool(auth.AuthContextKey)

	output, err := auth.GetAuthNonce(gc, persist.Address(address), authed, r.Repos.UserRepository, r.Repos.NonceRepository, r.EthClient)

	if err != nil {
		// Map known errors to GraphQL return types
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.GetAuthNoncePayload); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	return output, nil
}

func (r *mutationResolver) CreateUser(ctx context.Context, authMechanism model.AuthMechanism) (model.CreateUserPayload, error) {
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.CreateUserPayload, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.CreateUserPayload); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	authenticator, err := r.authMechanismToAuthenticator(authMechanism)
	if err != nil {
		return remapError(err)
	}

	output, err := user.CreateUser(ctx, authenticator, r.Repos.UserRepository, r.Repos.GalleryRepository)
	if err != nil {
		return remapError(err)
	}

	return output, nil
}

func (r *mutationResolver) Login(ctx context.Context, authMechanism model.AuthMechanism) (model.LoginPayload, error) {
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.LoginPayload, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.LoginPayload); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	authenticator, err := r.authMechanismToAuthenticator(authMechanism)
	if err != nil {
		return remapError(err)
	}

	output, err := auth.Login(ctx, authenticator)
	if err != nil {
		return remapError(err)
	}

	return output, nil
}

func (r *queryResolver) Viewer(ctx context.Context) (model.ViewerPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) UserByUsername(ctx context.Context, username string) (model.UserByUsernamePayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) MembershipTiers(ctx context.Context) ([]*model.MembershipTier, error) {
	panic(fmt.Errorf("not implemented"))
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
