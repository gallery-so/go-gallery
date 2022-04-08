package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
)

func (r *collectionResolver) Gallery(ctx context.Context, obj *model.Collection) (*model.Gallery, error) {
	gallery, err := publicapi.For(ctx).Gallery.GetGalleryByCollectionId(ctx, obj.Dbid)

	if err != nil {
		return nil, err
	}

	return galleryToModel(ctx, *gallery), nil
}

func (r *collectionResolver) Nfts(ctx context.Context, obj *model.Collection) ([]*model.CollectionNft, error) {
	nfts, err := publicapi.For(ctx).Nft.GetNftsByCollectionId(ctx, obj.Dbid)

	if err != nil {
		return nil, err
	}

	output := make([]*model.CollectionNft, len(nfts))
	for i, nft := range nfts {
		output[i] = &model.CollectionNft{
			HelperCollectionNftData: model.HelperCollectionNftData{
				NftId:        nft.ID,
				CollectionId: obj.Dbid,
			},
			Nft:        nftToModel(ctx, nft),
			Collection: obj,
		}
	}

	return output, nil
}

func (r *galleryResolver) Owner(ctx context.Context, obj *model.Gallery) (*model.GalleryUser, error) {
	gallery, err := publicapi.For(ctx).Gallery.GetGalleryById(ctx, obj.Dbid)

	if err != nil {
		return nil, err
	}

	return resolveGalleryUserByUserID(ctx, gallery.OwnerUserID)
}

func (r *galleryResolver) Collections(ctx context.Context, obj *model.Gallery) ([]*model.Collection, error) {
	return resolveCollectionsByGalleryID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) Galleries(ctx context.Context, obj *model.GalleryUser) ([]*model.Gallery, error) {
	return resolveGalleriesByUserID(ctx, obj.Dbid)
}

func (r *membershipOwnerResolver) User(ctx context.Context, obj *model.MembershipOwner) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Dbid)
}

func (r *mutationResolver) CreateCollection(ctx context.Context, input model.CreateCollectionInput) (model.CreateCollectionPayloadOrError, error) {
	api := publicapi.For(ctx)

	layout := persist.TokenLayout{
		Columns:    persist.NullInt32(input.Layout.Columns),
		Whitespace: input.Layout.Whitespace,
	}

	collection, err := api.Collection.CreateCollection(ctx, input.GalleryID, input.Name, input.CollectorsNote, input.Nfts, layout)

	if err != nil {
		return nil, err
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

	// Make sure the collection exists before trying to delete it
	_, err := api.Collection.GetCollectionById(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	// Get the collection's parent gallery before deleting the collection
	gallery, err := api.Gallery.GetGalleryByCollectionId(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	err = api.Collection.DeleteCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	// Deleting a collection marks the collection as "deleted" but doesn't alter the gallery,
	// so we don't need to refetch the gallery before returning it here
	output := &model.DeleteCollectionPayload{
		Gallery: galleryToModel(ctx, *gallery),
	}

	return output, nil
}

func (r *mutationResolver) UpdateCollectionInfo(ctx context.Context, input model.UpdateCollectionInfoInput) (model.UpdateCollectionInfoPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.Collection.UpdateCollection(ctx, input.CollectionID, input.Name, input.CollectorsNote)

	if err != nil {
		return nil, err
	}

	collection, err := api.Collection.GetCollectionById(ctx, input.CollectionID)
	if err != nil {
		return nil, err
	}

	// TODO: field collection
	output := &model.UpdateCollectionInfoPayload{
		Collection: collectionToModel(ctx, *collection),
	}

	return output, nil
}

func (r *mutationResolver) UpdateCollectionNfts(ctx context.Context, input model.UpdateCollectionNftsInput) (model.UpdateCollectionNftsPayloadOrError, error) {
	api := publicapi.For(ctx)

	layout := persist.TokenLayout{
		Columns:    persist.NullInt32(input.Layout.Columns),
		Whitespace: input.Layout.Whitespace,
	}

	err := api.Collection.UpdateCollectionNfts(ctx, input.CollectionID, input.Nfts, layout)
	if err != nil {
		return nil, err
	}

	collection, err := api.Collection.GetCollectionById(ctx, input.CollectionID)
	if err != nil {
		return nil, err
	}

	// TODO: Field collection
	output := &model.UpdateCollectionNftsPayload{
		Collection: collectionToModel(ctx, *collection),
	}

	return output, nil
}

func (r *mutationResolver) UpdateGalleryCollections(ctx context.Context, input model.UpdateGalleryCollectionsInput) (model.UpdateGalleryCollectionsPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.Gallery.UpdateGalleryCollections(ctx, input.GalleryID, input.Collections)
	if err != nil {
		return nil, err
	}

	gallery, err := api.Gallery.GetGalleryById(ctx, input.GalleryID)
	if err != nil {
		return nil, err
	}

	// TODO: Field collection
	output := &model.UpdateGalleryCollectionsPayload{
		Gallery: galleryToModel(ctx, *gallery),
	}

	return output, nil
}

func (r *mutationResolver) AddUserAddress(ctx context.Context, address persist.Address, authMechanism model.AuthMechanism) (model.AddUserAddressPayloadOrError, error) {
	api := publicapi.For(ctx)

	authenticator, err := r.authMechanismToAuthenticator(authMechanism)
	if err != nil {
		return nil, err
	}

	err = api.User.AddUserAddress(ctx, address, authenticator)
	if err != nil {
		return nil, err
	}

	output := &model.AddUserAddressPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) RemoveUserAddresses(ctx context.Context, addresses []persist.Address) (model.RemoveUserAddressesPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.User.RemoveUserAddresses(ctx, addresses)
	if err != nil {
		return nil, err
	}

	output := &model.RemoveUserAddressesPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) UpdateUserInfo(ctx context.Context, input model.UpdateUserInfoInput) (model.UpdateUserInfoPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.User.UpdateUserInfo(ctx, input.Username, input.Bio)
	if err != nil {
		return nil, err
	}

	output := &model.UpdateUserInfoPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) RefreshOpenSeaNfts(ctx context.Context, addresses string) (model.RefreshOpenSeaNftsPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.Nft.RefreshOpenSeaNfts(ctx, addresses)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	return output, nil
}

func (r *mutationResolver) CreateUser(ctx context.Context, authMechanism model.AuthMechanism) (model.CreateUserPayloadOrError, error) {
	authenticator, err := r.authMechanismToAuthenticator(authMechanism)
	if err != nil {
		return nil, err
	}

	output, err := user.CreateUser(ctx, authenticator, r.Repos.UserRepository, r.Repos.GalleryRepository)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func (r *mutationResolver) Login(ctx context.Context, authMechanism model.AuthMechanism) (model.LoginPayloadOrError, error) {
	authenticator, err := r.authMechanismToAuthenticator(authMechanism)
	if err != nil {
		return nil, err
	}

	output, err := auth.Login(ctx, authenticator)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func (r *nftResolver) Owner(ctx context.Context, obj *model.Nft) (model.GalleryUserOrWallet, error) {
	return resolveNftOwnerByNftID(ctx, obj.Dbid)
}

func (r *ownerAtBlockResolver) Owner(ctx context.Context, obj *model.OwnerAtBlock) (model.GalleryUserOrWallet, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) Node(ctx context.Context, id model.GqlID) (model.Node, error) {
	return nodeFetcher.GetNodeByGqlID(ctx, id)
}

func (r *queryResolver) Viewer(ctx context.Context) (model.ViewerOrError, error) {
	return resolveViewer(ctx), nil
}

func (r *queryResolver) UserByUsername(ctx context.Context, username string) (model.UserByUsernameOrError, error) {
	return resolveGalleryUserByUsername(ctx, username)
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
		output[i] = persistMembershipTierToModel(ctx, tier)
	}

	return output, nil
}

func (r *queryResolver) CollectionByID(ctx context.Context, id persist.DBID) (model.CollectionByIDOrError, error) {
	return resolveCollectionByCollectionID(ctx, id)
}

func (r *queryResolver) NftByID(ctx context.Context, id persist.DBID) (model.NftByIDOrError, error) {
	return resolveNftByNftID(ctx, id)
}

func (r *queryResolver) CollectionNftByID(ctx context.Context, nftID persist.DBID, collectionID persist.DBID) (model.CollectionNftByIDOrError, error) {
	return resolveCollectionNftByIDs(ctx, nftID, collectionID)
}

func (r *viewerResolver) User(ctx context.Context, obj *model.Viewer) (*model.GalleryUser, error) {
	gc := util.GinContextFromContext(ctx)
	userID := auth.GetUserIDFromCtx(gc)
	return resolveGalleryUserByUserID(ctx, userID)
}

func (r *viewerResolver) ViewerGalleries(ctx context.Context, obj *model.Viewer) ([]*model.ViewerGallery, error) {
	gc := util.GinContextFromContext(ctx)
	userID := auth.GetUserIDFromCtx(gc)

	galleries, err := resolveGalleriesByUserID(ctx, userID)

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
	nfts, err := publicapi.For(ctx).Nft.GetNftsByOwnerAddress(ctx, *obj.Address)

	if err != nil {
		return nil, err
	}

	output := make([]*model.Nft, len(nfts))
	for i, nft := range nfts {
		output[i] = nftToModel(ctx, nft)
	}

	return output, nil
}

// Collection returns generated.CollectionResolver implementation.
func (r *Resolver) Collection() generated.CollectionResolver { return &collectionResolver{r} }

// Gallery returns generated.GalleryResolver implementation.
func (r *Resolver) Gallery() generated.GalleryResolver { return &galleryResolver{r} }

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

type collectionResolver struct{ *Resolver }
type galleryResolver struct{ *Resolver }
type galleryUserResolver struct{ *Resolver }
type membershipOwnerResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type nftResolver struct{ *Resolver }
type ownerAtBlockResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type viewerResolver struct{ *Resolver }
type walletResolver struct{ *Resolver }
