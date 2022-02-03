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

func (r *mutationResolver) GetLoginNonce(ctx context.Context, address string) (*model.GetLoginNoncePayload, error) {
	gc, err := GinContextFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: This currently gets its value from the AuthOptional middleware applied to the entire GraphQL endpoint,
	// but that won't suffice when we transition auth-only endpoints to GraphQL too.
	authed := gc.GetBool(auth.AuthContextKey)

	output, err := auth.GetLoginNonce(gc, persist.Address(address), authed, r.Repos.UserRepository, r.Repos.NonceRepository, r.EthClient)
	if err != nil {
		// TODO: Map errors to GraphQL types
		//status := http.StatusInternalServerError
		//if _, ok := err.(persist.ErrNonceNotFoundForAddress); ok {
		//	status = http.StatusNotFound
		//}

		gc.Error(err)
		return nil, err
	}

	return output, nil
}

func (r *mutationResolver) LoginWithEoa(ctx context.Context, address string, nonce string, signature string) (*model.LoginPayload, error) {
	gc, err := GinContextFromContext(ctx)
	if err != nil {
		return nil, err
	}

	output, err := auth.LoginAndRecordAttempt(ctx, address, nonce, signature, auth.WalletTypeEOA, gc.Request, r.Repos.UserRepository, r.Repos.NonceRepository, r.Repos.LoginRepository, r.EthClient)
	if err != nil {
		gc.Error(err)
		return nil, err
	}

	auth.SetJWTCookie(gc, *output.JwtToken)

	return output, nil
}

func (r *mutationResolver) LoginWithSmartContract(ctx context.Context, address string, nonce string, signature string) (*model.LoginPayload, error) {
	gc, err := GinContextFromContext(ctx)
	if err != nil {
		return nil, err
	}

	output, err := auth.LoginAndRecordAttempt(ctx, address, nonce, signature, auth.WalletTypeGnosis, gc.Request, r.Repos.UserRepository, r.Repos.NonceRepository, r.Repos.LoginRepository, r.EthClient)
	if err != nil {
		gc.Error(err)
		return nil, err
	}

	auth.SetJWTCookie(gc, *output.JwtToken)

	return output, nil
}

func (r *queryResolver) Viewer(ctx context.Context) (model.ViewerPayload, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) UserByUsername(ctx context.Context, username *string) (model.GalleryByUserPayload, error) {
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
