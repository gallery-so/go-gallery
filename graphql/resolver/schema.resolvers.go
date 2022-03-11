package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/publicapi"
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

func (r *galleryCollectionResolver) Gallery(ctx context.Context, obj *model.GalleryCollection) (*model.Gallery, error) {
	gallery, err := dataloader.For(ctx).GalleryByCollectionId.Load(obj.ID)
	if err != nil {
		return nil, err
	}

	return galleryToModel(gallery), nil
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

		fullNft, err := collectionNftToNft(ctx, nft)

		if err == nil {
			nftModel := nftToModel(ctx, r.Resolver, fullNft)

			output[i] = &model.GalleryNft{
				ID:         nft.ID,
				Nft:        &nftModel,
				Collection: obj,
			}
		}
	}

	return output, nil
}

func (r *galleryUserResolver) Galleries(ctx context.Context, obj *model.GalleryUser) ([]*model.Gallery, error) {
	return resolveGalleriesByUserID(ctx, r.Resolver, obj.ID)
}

func (r *membershipOwnerResolver) User(ctx context.Context, obj *model.MembershipOwner) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, r.Resolver, obj.ID)
}

func (r *mutationResolver) CreateCollection(ctx context.Context, input model.CreateCollectionInput) (model.CreateCollectionPayloadOrError, error) {
	api := publicapi.For(ctx)
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.CreateCollectionPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.CreateCollectionPayloadOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	layout := persist.TokenLayout{
		Columns:    persist.NullInt32(input.Layout.Columns),
		Whitespace: input.Layout.Whitespace,
	}

	collection, err := api.Collection.CreateCollection(ctx, input.GalleryID, input.Name, input.CollectorsNote, input.Nfts, layout)

	if err != nil {
		return remapError(err)
	}

	// TODO: Use field collection here, and only query for the collection if it was requested.
	// That also means returning just the ID from the public API and using it here.
	output := model.CreateCollectionPayload{
		Collection: collectionToModel(ctx, *collection),
	}

	return output, nil
}

func (r *mutationResolver) DeleteCollection(ctx context.Context, collectionID persist.DBID) (model.DeleteCollectionPayloadOrError, error) {
	api := publicapi.For(ctx)
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.DeleteCollectionPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.DeleteCollectionPayloadOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	gallery, err := dataloader.For(ctx).GalleryByCollectionId.Load(collectionID)
	if err != nil {
		return nil, err
	}

	galleryID := gallery.ID

	err = api.Collection.DeleteCollection(ctx, collectionID)
	if err != nil {
		return remapError(err)
	}

	dataloader.For(ctx).GalleryByGalleryId.Clear(galleryID)
	gallery, err = dataloader.For(ctx).GalleryByGalleryId.Load(galleryID)

	output := &model.DeleteCollectionPayload{
		Gallery: galleryToModel(gallery),
	}

	return output, nil
}

func (r *mutationResolver) UpdateCollectionInfo(ctx context.Context, input model.UpdateCollectionInfoInput) (model.UpdateCollectionInfoPayloadOrError, error) {
	api := publicapi.For(ctx)
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.UpdateCollectionInfoPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.UpdateCollectionInfoPayloadOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	err := api.Collection.UpdateCollection(ctx, input.CollectionID, input.Name, input.CollectorsNote)

	if err != nil {
		return remapError(err)
	}

	collection, err := dataloader.For(ctx).CollectionByCollectionId.Load(input.CollectionID)
	if err != nil {
		return remapError(err)
	}

	// TODO: field collection
	output := &model.UpdateCollectionInfoPayload{
		Collection: collectionToModel(ctx, collection),
	}

	return output, nil
}

func (r *mutationResolver) UpdateCollectionNfts(ctx context.Context, input model.UpdateCollectionNftsInput) (model.UpdateCollectionNftsPayloadOrError, error) {
	api := publicapi.For(ctx)
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.UpdateCollectionNftsPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.UpdateCollectionNftsPayloadOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	layout := persist.TokenLayout{
		Columns:    persist.NullInt32(input.Layout.Columns),
		Whitespace: input.Layout.Whitespace,
	}

	err := api.Collection.UpdateCollectionNfts(ctx, input.CollectionID, input.Nfts, layout)
	if err != nil {
		return remapError(err)
	}

	collection, err := dataloader.For(ctx).CollectionByCollectionId.Load(input.CollectionID)
	if err != nil {
		return remapError(err)
	}

	// TODO: Field collection
	output := &model.UpdateCollectionNftsPayload{
		Collection: collectionToModel(ctx, collection),
	}

	return output, nil
}

func (r *mutationResolver) UpdateGalleryCollections(ctx context.Context, input model.UpdateGalleryCollectionsInput) (model.UpdateGalleryCollectionsPayloadOrError, error) {
	api := publicapi.For(ctx)
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.UpdateGalleryCollectionsPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.UpdateGalleryCollectionsPayloadOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	err := api.Gallery.UpdateGalleryCollections(ctx, input.GalleryID, input.Collections)
	if err != nil {
		return remapError(err)
	}

	gallery, err := dataloader.For(ctx).GalleryByGalleryId.Load(input.GalleryID)
	if err != nil {
		return remapError(err)
	}

	// TODO: Field collection
	output := &model.UpdateGalleryCollectionsPayload{
		Gallery: galleryToModel(gallery),
	}

	return output, nil
}

func (r *mutationResolver) AddUserAddress(ctx context.Context, address persist.Address, authMechanism model.AuthMechanism) (model.AddUserAddressPayloadOrError, error) {
	api := publicapi.For(ctx)
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.AddUserAddressPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.AddUserAddressPayloadOrError); ok {
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

	err = api.User.AddUserAddress(ctx, address, authenticator)
	if err != nil {
		return remapError(err)
	}

	output := &model.AddUserAddressPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) RemoveUserAddresses(ctx context.Context, addresses []persist.Address) (model.RemoveUserAddressesPayloadOrError, error) {
	api := publicapi.For(ctx)
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.RemoveUserAddressesPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.RemoveUserAddressesPayloadOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	err := api.User.RemoveUserAddresses(ctx, addresses)
	if err != nil {
		return remapError(err)
	}

	output := &model.RemoveUserAddressesPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) UpdateUserInfo(ctx context.Context, input model.UpdateUserInfoInput) (model.UpdateUserInfoPayloadOrError, error) {
	api := publicapi.For(ctx)
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.UpdateUserInfoPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.UpdateUserInfoPayloadOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	err := api.User.UpdateUserInfo(ctx, input.Username, input.Bio)
	if err != nil {
		return remapError(err)
	}

	output := &model.UpdateUserInfoPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) RefreshOpenSeaNfts(ctx context.Context, addresses string) (model.RefreshOpenSeaNftsPayloadOrError, error) {
	api := publicapi.For(ctx)
	gc := util.GinContextFromContext(ctx)

	// Map known errors to GraphQL return types
	remapError := func(err error) (model.RefreshOpenSeaNftsPayloadOrError, error) {
		if errorType, ok := r.errorToGraphqlType(err); ok {
			if returnType, ok := errorType.(model.RefreshOpenSeaNftsPayloadOrError); ok {
				return returnType, nil
			}
		}

		gc.Error(err)
		return nil, err
	}

	err := api.Nft.RefreshOpenSeaNfts(ctx, addresses)
	if err != nil {
		return remapError(err)
	}

	output := &model.RefreshOpenSeaNftsPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) GetAuthNonce(ctx context.Context, address persist.Address) (model.GetAuthNoncePayloadOrError, error) {
	gc := util.GinContextFromContext(ctx)

	authed := auth.GetUserAuthedFromCtx(gc)
	output, err := auth.GetAuthNonce(gc, address, authed, r.Repos.UserRepository, r.Repos.NonceRepository, r.EthClient)

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

func (r *nftResolver) Owner(ctx context.Context, obj *model.Nft) (model.GalleryUserOrWallet, error) {
	return resolveNftOwnerByNftId(ctx, r.Resolver, obj.ID)
}

func (r *ownerAtBlockResolver) Owner(ctx context.Context, obj *model.OwnerAtBlock) (model.GalleryUserOrWallet, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) Viewer(ctx context.Context) (model.ViewerOrError, error) {
	return resolveViewer(ctx), nil
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

func (r *queryResolver) MembershipTiers(ctx context.Context, forceRefresh *bool) ([]*model.MembershipTier, error) {
	api := publicapi.For(ctx)

	refresh := false
	if forceRefresh != nil {
		refresh = *forceRefresh
	}

	tiers, err := api.User.GetMembershipTiers(ctx, refresh)
	if err != nil {
		return nil, err
	}

	output := make([]*model.MembershipTier, len(tiers))
	for i, tier := range tiers {
		tierModel := membershipTierToModel(ctx, tier)
		output[i] = &tierModel
	}

	return output, nil
}

func (r *viewerResolver) User(ctx context.Context, obj *model.Viewer) (*model.GalleryUser, error) {
	gc := util.GinContextFromContext(ctx)
	userID := auth.GetUserIDFromCtx(gc)
	return resolveGalleryUserByUserID(ctx, r.Resolver, userID)
}

func (r *viewerResolver) ViewerGalleries(ctx context.Context, obj *model.Viewer) ([]*model.ViewerGallery, error) {
	gc := util.GinContextFromContext(ctx)
	userID := auth.GetUserIDFromCtx(gc)

	galleries, err := resolveGalleriesByUserID(ctx, r.Resolver, userID)

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

func (r *walletResolver) Nfts(ctx context.Context, obj *model.Wallet) ([]*model.Nft, error) {
	nfts, err := dataloader.For(ctx).NftsByAddress.Load(*obj.Address)

	if err != nil {
		return nil, err
	}

	output := make([]*model.Nft, len(nfts))
	for i, nft := range nfts {
		nftModel := nftToModel(ctx, r.Resolver, nft)
		output[i] = &nftModel
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

// MembershipOwner returns generated.MembershipOwnerResolver implementation.
func (r *Resolver) MembershipOwner() generated.MembershipOwnerResolver {
	return &membershipOwnerResolver{r}
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Nft returns generated.NftResolver implementation.
func (r *Resolver) Nft() generated.NftResolver { return &nftResolver{r} }

// OwnerAtBlock returns generated.OwnerAtBlockResolver implementation.
func (r *Resolver) OwnerAtBlock() generated.OwnerAtBlockResolver { return &ownerAtBlockResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Viewer returns generated.ViewerResolver implementation.
func (r *Resolver) Viewer() generated.ViewerResolver { return &viewerResolver{r} }

// Wallet returns generated.WalletResolver implementation.
func (r *Resolver) Wallet() generated.WalletResolver { return &walletResolver{r} }

type galleryResolver struct{ *Resolver }
type galleryCollectionResolver struct{ *Resolver }
type galleryUserResolver struct{ *Resolver }
type membershipOwnerResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type nftResolver struct{ *Resolver }
type ownerAtBlockResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type viewerResolver struct{ *Resolver }
type walletResolver struct{ *Resolver }
