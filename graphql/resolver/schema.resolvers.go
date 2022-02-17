package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
)

func (r *galleryResolver) Owner(ctx context.Context, obj *model.Gallery) (*model.GalleryUser, error) {
	gallery, err := dataloader.For(ctx).GalleryByGalleryId.Load(obj.ID)

	if err != nil {
		return nil, err
	}

	return resolveGalleryUserByUserId(ctx, r.Resolver, gallery.OwnerUserID.String())
}

func (r *galleryUserResolver) Galleries(ctx context.Context, obj *model.GalleryUser) ([]*model.Gallery, error) {
	galleries, err := dataloader.For(ctx).GalleriesByUserId.Load(obj.ID)

	if err != nil {
		return nil, err
	}

	// Oof, more remapping, huh?
	//if err != nil {
	//	return remapError(err)
	//}

	var output = make([]*model.Gallery, len(galleries))
	for i, gallery := range galleries {
		output[i] = &model.Gallery{
			ID:          gallery.ID.String(),
			Owner:       nil, // Handled by resolver
			Collections: nil, // TODO, should also be handled by a resolver
		}
	}

	return output, nil
}

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

func (r *mutationResolver) GetAuthNonce(ctx context.Context, address string) (model.GetAuthNoncePayloadOrError, error) {
	gc := util.GinContextFromContext(ctx)

	authed := auth.GetUserAuthedFromCtx(gc)
	output, err := auth.GetAuthNonce(gc, persist.Address(address), authed, r.Repos.UserRepository, r.Repos.NonceRepository, r.EthClient)

	if err != nil {
		// Map known errors to GraphQL return types
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.GetAuthNoncePayloadOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	return output, nil
}

func (r *mutationResolver) CreateUser(ctx context.Context, authMechanism model.AuthMechanism) (model.CreateUserPayloadOrError, error) {
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.CreateUserPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.CreateUserPayloadOrError); ok {
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

func (r *mutationResolver) Login(ctx context.Context, authMechanism model.AuthMechanism) (model.LoginPayloadOrError, error) {
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.LoginPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.LoginPayloadOrError); ok {
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

func (r *queryResolver) Viewer(ctx context.Context) (model.ViewerOrError, error) {
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.ViewerOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.ViewerOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	userID := auth.GetUserIDFromCtx(gc).String() // TODO: Is there a case where a user has an ID but isn't authed?
	user, err := resolveGalleryUserByUserId(ctx, r.Resolver, userID)

	if err != nil {
		return remapError(err)
	}

	// TODO: Check auth first!
	viewer := &model.Viewer{
		User:          user,
		Wallets:       user.Wallets, // TODO: Is this field necessary?
		ViewerGallery: nil,
	}

	return viewer, nil
}

func (r *queryResolver) UserByUsername(ctx context.Context, username string) (model.UserByUsernameOrError, error) {
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.UserByUsernameOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.UserByUsernameOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	user, err := resolveGalleryUserByUsername(ctx, r.Resolver, username)

	if err != nil {
		return remapError(err)
	}

	return user, nil
}

func (r *queryResolver) MembershipTiers(ctx context.Context) ([]*model.MembershipTier, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *walletResolver) Nfts(ctx context.Context, obj *model.Wallet) ([]model.Nft, error) {
	return []model.Nft{model.ImageNft{ID: "abcdef"}}, nil
}

// Gallery returns generated.GalleryResolver implementation.
func (r *Resolver) Gallery() generated.GalleryResolver { return &galleryResolver{r} }

// GalleryUser returns generated.GalleryUserResolver implementation.
func (r *Resolver) GalleryUser() generated.GalleryUserResolver { return &galleryUserResolver{r} }

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Wallet returns generated.WalletResolver implementation.
func (r *Resolver) Wallet() generated.WalletResolver { return &walletResolver{r} }

type galleryResolver struct{ *Resolver }
type galleryUserResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type walletResolver struct{ *Resolver }
