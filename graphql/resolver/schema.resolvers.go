package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
)

func (r *collectionResolver) Gallery(ctx context.Context, obj *model.Collection) (*model.Gallery, error) {
	gallery, err := publicapi.For(ctx).Gallery.GetGalleryByCollectionId(ctx, obj.Dbid)

	if err != nil {
		return nil, err
	}

	return galleryToModel(ctx, *gallery), nil
}

func (r *collectionResolver) Tokens(ctx context.Context, obj *model.Collection) ([]*model.CollectionToken, error) {
	tokens, err := publicapi.For(ctx).Token.GetTokensByCollectionId(ctx, obj.Dbid)

	if err != nil {
		return nil, err
	}

	output := make([]*model.CollectionToken, len(tokens))
	for i, token := range tokens {
		output[i] = &model.CollectionToken{
			HelperCollectionTokenData: model.HelperCollectionTokenData{
				TokenId:      token.ID,
				CollectionId: obj.Dbid,
			},
			Token:      tokenToModel(ctx, token),
			Collection: obj,
		}
	}

	return output, nil
}

func (r *followUserPayloadResolver) User(ctx context.Context, obj *model.FollowUserPayload) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.User.Dbid)
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

func (r *galleryUserResolver) Tokens(ctx context.Context, obj *model.GalleryUser) ([]*model.Token, error) {
	return resolveTokensByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) Wallets(ctx context.Context, obj *model.GalleryUser) ([]*model.Wallet, error) {
	return resolveWalletsByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) Galleries(ctx context.Context, obj *model.GalleryUser) ([]*model.Gallery, error) {
	return resolveGalleriesByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) Followers(ctx context.Context, obj *model.GalleryUser) ([]*model.GalleryUser, error) {
	return resolveFollowersByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) Following(ctx context.Context, obj *model.GalleryUser) ([]*model.GalleryUser, error) {
	return resolveFollowingByUserID(ctx, obj.Dbid)
}

func (r *mutationResolver) AddUserWallet(ctx context.Context, chainAddress persist.ChainAddress, authMechanism model.AuthMechanism) (model.AddUserWalletPayloadOrError, error) {
	api := publicapi.For(ctx)

	authenticator, err := r.authMechanismToAuthenticator(ctx, authMechanism)
	if err != nil {
		return nil, err
	}

	err = api.User.AddWalletToUser(ctx, chainAddress, authenticator)
	if err != nil {
		return nil, err
	}

	output := &model.AddUserWalletPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) RemoveUserWallets(ctx context.Context, walletIds []persist.DBID) (model.RemoveUserWalletsPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.User.RemoveWalletsFromUser(ctx, walletIds)
	if err != nil {
		return nil, err
	}

	output := &model.RemoveUserWalletsPayload{
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

	output := &model.UpdateGalleryCollectionsPayload{
		Gallery: galleryToModel(ctx, *gallery),
	}

	return output, nil
}

func (r *mutationResolver) CreateCollection(ctx context.Context, input model.CreateCollectionInput) (model.CreateCollectionPayloadOrError, error) {
	api := publicapi.For(ctx)

	layout := persist.TokenLayout{
		Columns:    persist.NullInt32(input.Layout.Columns),
		Whitespace: input.Layout.Whitespace,
	}

	collection, err := api.Collection.CreateCollection(ctx, input.GalleryID, input.Name, input.CollectorsNote, input.Tokens, layout)

	if err != nil {
		return nil, err
	}

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

	err := api.Collection.UpdateCollectionInfo(ctx, input.CollectionID, input.Name, input.CollectorsNote)

	if err != nil {
		return nil, err
	}

	collection, err := api.Collection.GetCollectionById(ctx, input.CollectionID)
	if err != nil {
		return nil, err
	}

	output := &model.UpdateCollectionInfoPayload{
		Collection: collectionToModel(ctx, *collection),
	}

	return output, nil
}

func (r *mutationResolver) UpdateCollectionTokens(ctx context.Context, input model.UpdateCollectionTokensInput) (model.UpdateCollectionTokensPayloadOrError, error) {
	api := publicapi.For(ctx)

	layout := persist.TokenLayout{
		Columns:    persist.NullInt32(input.Layout.Columns),
		Whitespace: input.Layout.Whitespace,
	}

	err := api.Collection.UpdateCollectionTokens(ctx, input.CollectionID, input.Tokens, layout)
	if err != nil {
		return nil, err
	}

	collection, err := api.Collection.GetCollectionById(ctx, input.CollectionID)
	if err != nil {
		return nil, err
	}

	output := &model.UpdateCollectionTokensPayload{
		Collection: collectionToModel(ctx, *collection),
	}

	return output, nil
}

func (r *mutationResolver) UpdateCollectionHidden(ctx context.Context, input model.UpdateCollectionHiddenInput) (model.UpdateCollectionHiddenPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.Collection.UpdateCollectionHidden(ctx, input.CollectionID, input.Hidden)
	if err != nil {
		return nil, err
	}

	collection, err := api.Collection.GetCollectionById(ctx, input.CollectionID)
	if err != nil {
		return nil, err
	}

	output := &model.UpdateCollectionHiddenPayload{
		Collection: collectionToModel(ctx, *collection),
	}

	return output, nil
}

func (r *mutationResolver) UpdateTokenInfo(ctx context.Context, input model.UpdateTokenInfoInput) (model.UpdateTokenInfoPayloadOrError, error) {
	api := publicapi.For(ctx)

	collectionID := persist.DBID("")
	if input.CollectionID != nil {
		collectionID = *input.CollectionID
	}

	err := api.Token.UpdateTokenInfo(ctx, input.TokenID, collectionID, input.CollectorsNote)
	if err != nil {
		return nil, err
	}

	token, err := api.Token.GetTokenById(ctx, input.TokenID)
	if err != nil {
		return nil, err
	}

	output := &model.UpdateTokenInfoPayload{
		Token: tokenToModel(ctx, *token),
	}

	return output, nil
}

func (r *mutationResolver) SyncTokens(ctx context.Context) (model.SyncTokensPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.Token.SyncTokens(ctx)
	if err != nil {
		return nil, err
	}

	output := &model.SyncTokensPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) RefreshToken(ctx context.Context, tokenID persist.DBID) (model.RefreshTokenPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.Token.RefreshToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}

	token, err := resolveTokenByTokenID(ctx, tokenID)
	if err != nil {
		return nil, err
	}

	output := &model.RefreshTokenPayload{
		Token: token,
	}

	return output, nil
}

func (r *mutationResolver) RefreshContract(ctx context.Context, contractID persist.DBID) (model.RefreshContractPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.Contract.RefreshContract(ctx, contractID)
	if err != nil {
		return nil, err
	}

	contract, err := resolveContractByContractID(ctx, contractID)
	if err != nil {
		return nil, err
	}

	output := &model.RefreshContractPayload{
		Contract: contract,
	}

	return output, nil
}

func (r *mutationResolver) GetAuthNonce(ctx context.Context, chainAddress persist.ChainAddress) (model.GetAuthNoncePayloadOrError, error) {
	nonce, userExists, err := publicapi.For(ctx).Auth.GetAuthNonce(ctx, chainAddress)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	output := &model.AuthNonce{
		Nonce:      &nonce,
		UserExists: &userExists,
	}

	return output, nil
}

func (r *mutationResolver) CreateUser(ctx context.Context, authMechanism model.AuthMechanism) (model.CreateUserPayloadOrError, error) {
	authenticator, err := r.authMechanismToAuthenticator(ctx, authMechanism)
	if err != nil {
		return nil, err
	}

	userID, galleryID, err := publicapi.For(ctx).User.CreateUser(ctx, authenticator)
	if err != nil {
		return nil, err
	}

	output := &model.CreateUserPayload{
		UserID:    &userID,
		GalleryID: &galleryID,
		Viewer:    resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) Login(ctx context.Context, authMechanism model.AuthMechanism) (model.LoginPayloadOrError, error) {
	authenticator, err := r.authMechanismToAuthenticator(ctx, authMechanism)
	if err != nil {
		return nil, err
	}

	userId, err := publicapi.For(ctx).Auth.Login(ctx, authenticator)
	if err != nil {
		return nil, err
	}

	output := &model.LoginPayload{
		UserID: &userId,
		Viewer: resolveViewer(ctx),
	}
	return output, nil
}

func (r *mutationResolver) Logout(ctx context.Context) (*model.LogoutPayload, error) {
	publicapi.For(ctx).Auth.Logout(ctx)

	output := &model.LogoutPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) FollowUser(ctx context.Context, userID persist.DBID) (model.FollowUserPayloadOrError, error) {
	err := publicapi.For(ctx).User.FollowUser(ctx, userID)

	if err != nil {
		return nil, err
	}

	output := &model.FollowUserPayload{
		Viewer: resolveViewer(ctx),
		User: &model.GalleryUser{
			Dbid: userID, // remaining fields handled by dedicated resolver
		},
	}

	return output, err
}

func (r *mutationResolver) UnfollowUser(ctx context.Context, userID persist.DBID) (model.UnfollowUserPayloadOrError, error) {
	err := publicapi.For(ctx).User.UnfollowUser(ctx, userID)

	if err != nil {
		return nil, err
	}

	output := &model.UnfollowUserPayload{
		Viewer: resolveViewer(ctx),
		User: &model.GalleryUser{
			Dbid: userID, // remaining fields handled by dedicated resolver
		},
	}

	return output, err
}

func (r *ownerAtBlockResolver) Owner(ctx context.Context, obj *model.OwnerAtBlock) (model.GalleryUserOrAddress, error) {
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

func (r *queryResolver) UserByID(ctx context.Context, id persist.DBID) (model.UserByIDOrError, error) {
	return resolveGalleryUserByUserID(ctx, id)
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

func (r *queryResolver) TokenByID(ctx context.Context, id persist.DBID) (model.TokenByIDOrError, error) {
	return resolveTokenByTokenID(ctx, id)
}

func (r *queryResolver) CollectionTokenByID(ctx context.Context, tokenID persist.DBID, collectionID persist.DBID) (model.CollectionTokenByIDOrError, error) {
	return resolveCollectionTokenByIDs(ctx, tokenID, collectionID)
}

func (r *queryResolver) CommunityByAddress(ctx context.Context, communityAddress persist.ChainAddress, forceRefresh *bool) (model.CommunityByAddressOrError, error) {
	refresh := false
	if forceRefresh != nil {
		refresh = *forceRefresh
	}

	return resolveCommunityByContractAddress(ctx, communityAddress, refresh)
}

func (r *queryResolver) GeneralAllowlist(ctx context.Context) ([]*persist.ChainAddress, error) {
	return resolveGeneralAllowlist(ctx)
}

func (r *tokenResolver) Owner(ctx context.Context, obj *model.Token) (*model.GalleryUser, error) {
	return resolveTokenOwnerByTokenID(ctx, obj.Dbid)
}

func (r *tokenResolver) OwnedByWallets(ctx context.Context, obj *model.Token) ([]*model.Wallet, error) {
	token, err := publicapi.For(ctx).Token.GetTokenById(ctx, obj.Dbid)
	if err != nil {
		return nil, err
	}

	wallets := make([]*model.Wallet, len(token.OwnedByWallets))
	for i, walletID := range token.OwnedByWallets {
		wallets[i], err = resolveWalletByWalletID(ctx, walletID)
		if err != nil {
			sentryutil.ReportError(ctx, err)
		}
	}

	return wallets, nil
}

func (r *tokenResolver) Contract(ctx context.Context, obj *model.Token) (*model.Contract, error) {
	return resolveContractByTokenID(ctx, obj.Dbid)
}

func (r *tokenHolderResolver) Wallets(ctx context.Context, obj *model.TokenHolder) ([]*model.Wallet, error) {
	wallets := make([]*model.Wallet, 0, len(obj.WalletIds))
	for _, id := range obj.WalletIds {
		wallet, err := resolveWalletByWalletID(ctx, id)
		if err == nil {
			wallets = append(wallets, wallet)
		}
	}

	return wallets, nil
}

func (r *tokenHolderResolver) User(ctx context.Context, obj *model.TokenHolder) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.UserId)
}

func (r *unfollowUserPayloadResolver) User(ctx context.Context, obj *model.UnfollowUserPayload) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.User.Dbid)
}

func (r *viewerResolver) User(ctx context.Context, obj *model.Viewer) (*model.GalleryUser, error) {
	userID := publicapi.For(ctx).User.GetLoggedInUserId(ctx)
	return resolveGalleryUserByUserID(ctx, userID)
}

func (r *viewerResolver) ViewerGalleries(ctx context.Context, obj *model.Viewer) ([]*model.ViewerGallery, error) {
	userID := publicapi.For(ctx).User.GetLoggedInUserId(ctx)
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

func (r *walletResolver) Tokens(ctx context.Context, obj *model.Wallet) ([]*model.Token, error) {
	return resolveTokensByWalletID(ctx, obj.Dbid)
}

func (r *chainAddressInputResolver) Address(ctx context.Context, obj *persist.ChainAddress, data persist.Address) error {
	return obj.GQLSetAddressFromResolver(data)
}

func (r *chainAddressInputResolver) Chain(ctx context.Context, obj *persist.ChainAddress, data persist.Chain) error {
	return obj.GQLSetChainFromResolver(data)
}

// Collection returns generated.CollectionResolver implementation.
func (r *Resolver) Collection() generated.CollectionResolver { return &collectionResolver{r} }

// FollowUserPayload returns generated.FollowUserPayloadResolver implementation.
func (r *Resolver) FollowUserPayload() generated.FollowUserPayloadResolver {
	return &followUserPayloadResolver{r}
}

// Gallery returns generated.GalleryResolver implementation.
func (r *Resolver) Gallery() generated.GalleryResolver { return &galleryResolver{r} }

// GalleryUser returns generated.GalleryUserResolver implementation.
func (r *Resolver) GalleryUser() generated.GalleryUserResolver { return &galleryUserResolver{r} }

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// OwnerAtBlock returns generated.OwnerAtBlockResolver implementation.
func (r *Resolver) OwnerAtBlock() generated.OwnerAtBlockResolver { return &ownerAtBlockResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Token returns generated.TokenResolver implementation.
func (r *Resolver) Token() generated.TokenResolver { return &tokenResolver{r} }

// TokenHolder returns generated.TokenHolderResolver implementation.
func (r *Resolver) TokenHolder() generated.TokenHolderResolver { return &tokenHolderResolver{r} }

// UnfollowUserPayload returns generated.UnfollowUserPayloadResolver implementation.
func (r *Resolver) UnfollowUserPayload() generated.UnfollowUserPayloadResolver {
	return &unfollowUserPayloadResolver{r}
}

// Viewer returns generated.ViewerResolver implementation.
func (r *Resolver) Viewer() generated.ViewerResolver { return &viewerResolver{r} }

// Wallet returns generated.WalletResolver implementation.
func (r *Resolver) Wallet() generated.WalletResolver { return &walletResolver{r} }

// ChainAddressInput returns generated.ChainAddressInputResolver implementation.
func (r *Resolver) ChainAddressInput() generated.ChainAddressInputResolver {
	return &chainAddressInputResolver{r}
}

type collectionResolver struct{ *Resolver }
type followUserPayloadResolver struct{ *Resolver }
type galleryResolver struct{ *Resolver }
type galleryUserResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type ownerAtBlockResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type tokenResolver struct{ *Resolver }
type tokenHolderResolver struct{ *Resolver }
type unfollowUserPayloadResolver struct{ *Resolver }
type viewerResolver struct{ *Resolver }
type walletResolver struct{ *Resolver }
type chainAddressInputResolver struct{ *Resolver }
