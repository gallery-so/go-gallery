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

	return resolveGalleryUserByUserID(ctx, r.Resolver, gallery.OwnerUserID)
}

func (r *galleryResolver) Collections(ctx context.Context, obj *model.Gallery) ([]*model.GalleryCollection, error) {
	return resolveGalleryCollectionsByGalleryID(ctx, r.Resolver, obj.ID)
}

func (r *galleryCollectionResolver) Nfts(ctx context.Context, obj *model.GalleryCollection) ([]*model.GalleryNft, error) {
	collection, err := dataloader.For(ctx).CollectionByCollectionId.Load(obj.ID)

	if err != nil {
		return nil, err
	}

	nfts := collection.NFTs

	output := make([]*model.GalleryNft, len(nfts))
	for i, nft := range nfts {

		// TODO: For resolvers, I don't think we want to use existing SQL queries that recursively fill out an entire
		// Gallery -> Collection -> NFT hierarchy for us. Rather, I think we want to do DB calls as necessary based on
		// what the client asks for. But for now, I'm using the existing SQL stuff to create these GalleryNft objects.
		// Also worth noting: we should get rid of persist.CollectionNFT, which is a struct with a subset of NFT fields
		// that reads from the same NFT database but just grabs less data. With GraphQL, clients will select the data
		// they want.

		genericNft := model.GenericNft{
			ID:                  nft.ID,
			Name:                util.StringToPointer(nft.Name.String()),
			TokenCollectionName: util.StringToPointer(nft.TokenCollectionName.String()),
			Owner:               nil, // handled by dedicated resolver
		}

		output[i] = &model.GalleryNft{
			ID:         nft.ID,
			Nft:        genericNft,
			Collection: obj,
		}
	}

	return output, nil
}

func (r *galleryUserResolver) Galleries(ctx context.Context, obj *model.GalleryUser) ([]*model.Gallery, error) {
	return resolveGalleriesByUserID(ctx, r.Resolver, obj.ID)
}

func (r *genericNftResolver) Owner(ctx context.Context, obj *model.GenericNft) (model.GalleryUserOrWallet, error) {
	return resolveNftOwnerByNftId(ctx, r.Resolver, obj.ID)
}

func (r *imageNftResolver) Owner(ctx context.Context, obj *model.ImageNft) (model.GalleryUserOrWallet, error) {
	return resolveNftOwnerByNftId(ctx, r.Resolver, obj.ID)
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

	userID := auth.GetUserIDFromCtx(gc)
	user, err := resolveGalleryUserByUserID(ctx, r.Resolver, userID)

	if err != nil {
		return remapError(err)
	}

	viewer := &model.Viewer{
		User:            user,
		ViewerGalleries: nil, // handled by dedicated resolver
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

func (r *videoNftResolver) Owner(ctx context.Context, obj *model.VideoNft) (model.GalleryUserOrWallet, error) {
	return resolveNftOwnerByNftId(ctx, r.Resolver, obj.ID)
}

func (r *viewerResolver) ViewerGalleries(ctx context.Context, obj *model.Viewer) ([]*model.ViewerGallery, error) {
	galleries, err := resolveGalleriesByUserID(ctx, r.Resolver, obj.User.ID)

	if err != nil {
		return nil, err
	}

	output := make([]*model.ViewerGallery, len(galleries))
	for i, gallery := range galleries {
		output[i] = &model.ViewerGallery{
			Gallery: gallery,
		}
	}

	return output, nil
}

func (r *walletResolver) Nfts(ctx context.Context, obj *model.Wallet) ([]model.Nft, error) {
	nfts, err := dataloader.For(ctx).NftsByAddress.Load(persist.Address(*obj.Address))

	if err != nil {
		return nil, err
	}

	output := make([]model.Nft, len(nfts))
	for i, nft := range nfts {
		output[i] = nftToNftModel(ctx, r.Resolver, nft)
	}

	return output, nil
}

// Gallery returns generated.GalleryResolver implementation.
func (r *Resolver) Gallery() generated.GalleryResolver { return &galleryResolver{r} }

// GalleryCollection returns generated.GalleryCollectionResolver implementation.
func (r *Resolver) GalleryCollection() generated.GalleryCollectionResolver {
	return &galleryCollectionResolver{r}
}

// GalleryUser returns generated.GalleryUserResolver implementation.
func (r *Resolver) GalleryUser() generated.GalleryUserResolver { return &galleryUserResolver{r} }

// GenericNft returns generated.GenericNftResolver implementation.
func (r *Resolver) GenericNft() generated.GenericNftResolver { return &genericNftResolver{r} }

// ImageNft returns generated.ImageNftResolver implementation.
func (r *Resolver) ImageNft() generated.ImageNftResolver { return &imageNftResolver{r} }

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// VideoNft returns generated.VideoNftResolver implementation.
func (r *Resolver) VideoNft() generated.VideoNftResolver { return &videoNftResolver{r} }

// Viewer returns generated.ViewerResolver implementation.
func (r *Resolver) Viewer() generated.ViewerResolver { return &viewerResolver{r} }

// Wallet returns generated.WalletResolver implementation.
func (r *Resolver) Wallet() generated.WalletResolver { return &walletResolver{r} }

type galleryResolver struct{ *Resolver }
type galleryCollectionResolver struct{ *Resolver }
type galleryUserResolver struct{ *Resolver }
type genericNftResolver struct{ *Resolver }
type imageNftResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type videoNftResolver struct{ *Resolver }
type viewerResolver struct{ *Resolver }
type walletResolver struct{ *Resolver }
