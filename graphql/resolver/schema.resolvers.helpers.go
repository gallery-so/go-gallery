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
	case auth.ErrAuthenticationFailed:
		mappedErr = model.ErrAuthenticationFailed{Message: message}
	case auth.ErrDoesNotOwnRequiredNFT:
		mappedErr = model.ErrDoesNotOwnRequiredNft{Message: message}
	case persist.ErrUserNotFound:
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

	ethNonceAuth := func(address persist.Address, nonce string, signature string, walletType auth.WalletType) auth.Authenticator {
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

func resolveGalleryUserByUserID(ctx context.Context, r *Resolver, userID persist.DBID) (*model.GalleryUser, error) {
	user, err := dataloader.For(ctx).UserByUserId.Load(userID)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, r, user)
}

func resolveGalleryUserByUsername(ctx context.Context, r *Resolver, username string) (*model.GalleryUser, error) {
	user, err := dataloader.For(ctx).UserByUsername.Load(username)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, r, user)
}

func resolveGalleryUserByAddress(ctx context.Context, r *Resolver, address persist.Address) (*model.GalleryUser, error) {
	user, err := dataloader.For(ctx).UserByAddress.Load(address)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, r, user)
}

func resolveGalleriesByUserID(ctx context.Context, r *Resolver, userID persist.DBID) ([]*model.Gallery, error) {
	galleries, err := dataloader.For(ctx).GalleriesByUserId.Load(userID)

	if err != nil {
		return nil, err
	}

	var output = make([]*model.Gallery, len(galleries))
	for i, gallery := range galleries {
		output[i] = galleryToModel(gallery)
	}

	return output, nil
}

func resolveGalleryCollectionsByGalleryID(ctx context.Context, r *Resolver, galleryID persist.DBID) ([]*model.GalleryCollection, error) {
	// TODO: Update this to query for collections by gallery ID, instead of querying for a user and returning
	// all of their collections. The result is the same right now, since a user only has one gallery.

	gallery, err := dataloader.For(ctx).GalleryByGalleryId.Load(galleryID)
	if err != nil {
		return nil, err
	}

	collections, err := dataloader.For(ctx).CollectionsByUserId.Load(gallery.OwnerUserID)
	if err != nil {
		return nil, err
	}

	var output = make([]*model.GalleryCollection, len(collections))
	for i, collection := range collections {
		version := collection.Version.Int()
		hidden := collection.Hidden.Bool()

		output[i] = &model.GalleryCollection{
			ID:             collection.ID,
			Version:        &version,
			Name:           util.StringToPointer(collection.Name.String()),
			CollectorsNote: util.StringToPointer(collection.CollectorsNote.String()),
			Gallery:        galleryIDToGalleryModel(galleryID),
			Layout:         layoutToModel(ctx, r, collection.Layout),
			Hidden:         &hidden,
			Nfts:           nil, // handled by dedicated resolver
		}
	}

	return output, nil
}

func galleryToModel(gallery persist.Gallery) *model.Gallery {
	return galleryIDToGalleryModel(gallery.ID)
}

func galleryIDToGalleryModel(galleryID persist.DBID) *model.Gallery {
	gallery := model.Gallery{
		ID:          galleryID,
		Owner:       nil, // handled by dedicated resolver
		Collections: nil, // handled by dedicated resolver
	}

	return &gallery
}

func layoutToModel(ctx context.Context, r *Resolver, layout persist.TokenLayout) *model.GalleryCollectionLayout {
	columns := layout.Columns.Int()

	output := model.GalleryCollectionLayout{
		Columns: &columns,
	}

	return &output
}

// userToModel converts a persist.User to a model.User
func userToModel(ctx context.Context, r *Resolver, user persist.User) (*model.GalleryUser, error) {
	gc := util.GinContextFromContext(ctx)
	isAuthenticated := auth.GetUserAuthedFromCtx(gc)

	output := &model.GalleryUser{
		ID:                  user.ID,
		Username:            util.StringToPointer(user.Username.String()),
		Bio:                 util.StringToPointer(user.Bio.String()),
		Wallets:             addressesToModels(ctx, r, user.Addresses),
		Galleries:           nil, // handled by dedicated resolver
		IsAuthenticatedUser: &isAuthenticated,
	}

	return output, nil
}

// addressesToModels converts a slice of persist.Address to a slice of model.Wallet
func addressesToModels(ctx context.Context, r *Resolver, addresses []persist.Address) []*model.Wallet {
	wallets := make([]*model.Wallet, len(addresses))
	for i, address := range addresses {
		wallets[i] = &model.Wallet{
			ID:      "", // TODO: What's a wallet's ID?
			Address: &address,
			Nfts:    nil, // handled by dedicated resolver
		}
	}

	return wallets
}

func resolveNftOwnerByNftId(ctx context.Context, r *Resolver, nftId persist.DBID) (model.GalleryUserOrWallet, error) {
	nft, err := dataloader.For(ctx).NftByNftId.Load(nftId)

	if err != nil {
		return nil, err
	}

	return resolveGalleryUserOrWalletByAddress(ctx, r, nft.OwnerAddress)
}

func resolveGalleryUserOrWalletByAddress(ctx context.Context, r *Resolver, address persist.Address) (model.GalleryUserOrWallet, error) {
	owner, err := dataloader.For(ctx).UserByAddress.Load(address)

	if err == nil {
		return userToModel(ctx, r, owner)
	}

	if _, ok := err.(persist.ErrUserNotFound); ok {
		wallet := model.Wallet{
			ID:      "", // TODO: What's a wallet's ID?
			Address: &address,
			Nfts:    nil, // handled by dedicated resolver
		}

		return wallet, nil
	}

	return nil, err
}

func nftToModel(ctx context.Context, r *Resolver, nft persist.NFT) model.Nft {
	output := model.GenericNft{
		ID:                  nft.ID,
		Name:                util.StringToPointer(nft.Name.String()),
		TokenCollectionName: util.StringToPointer(nft.TokenCollectionName.String()),
		Owner:               nil, // handled by dedicated resolver
	}

	return output
}

func collectionToModel(ctx context.Context, r *Resolver, collection persist.Collection) model.GalleryCollection {
	version := collection.Version.Int()
	hidden := collection.Hidden.Bool()

	// TODO: Start here! Should we be filling this collection's gallery out, or leaving it to a resolver?
	// The Gallery->Collections path currently fills out the Gallery field on each Collection it returns,
	// and switching to a resolver here means switching to a resolver there. Not a big deal, just remember
	// to prime the "GalleryByCollectionId" cache with results from the "collections by gallery" lookup.
	return model.GalleryCollection{
		ID:             collection.ID,
		Version:        &version,
		Name:           util.StringToPointer(collection.Name.String()),
		CollectorsNote: util.StringToPointer(collection.CollectorsNote.String()),
		Gallery:        nil, // TODO: Add SQL query to find gallery parent for collection. // galleryIDToGalleryModel(galleryID),
		Layout:         layoutToModel(ctx, r, collection.Layout),
		Hidden:         &hidden,
		Nfts:           nil, // handled by dedicated resolver
	}
}

func membershipTierToModel(ctx context.Context, membershipTier persist.MembershipTier) model.MembershipTier {
	owners := make([]*model.MembershipOwner, len(membershipTier.Owners))
	for i, owner := range membershipTier.Owners {
		ownerModel := membershipOwnerToModel(ctx, owner)
		owners[i] = &ownerModel
	}

	// TODO: Consider field collection on owners
	return model.MembershipTier{
		ID:       membershipTier.ID,
		Name:     util.StringToPointer(membershipTier.Name.String()),
		AssetURL: util.StringToPointer(membershipTier.AssetURL.String()),
		TokenID:  util.StringToPointer(membershipTier.TokenID.String()),
		Owners:   owners,
	}
}

func membershipOwnerToModel(ctx context.Context, membershipOwner persist.MembershipOwner) model.MembershipOwner {
	previewNfts := make([]*string, len(membershipOwner.PreviewNFTs))
	for i, nft := range membershipOwner.PreviewNFTs {
		previewNfts[i] = util.StringToPointer(nft.String())
	}

	// TODO: Probably want to add "address" here to, assuming it means "the address holding the membership card"
	return model.MembershipOwner{
		ID:          membershipOwner.UserID, // TODO: Not sure this is relevant if we have the user object too
		User:        nil,                    // TODO: Resolve or do field collection now. Resolving is fine if we keep the ID, FC is better if we drop it.
		PreviewNfts: previewNfts,
	}
}

func resolveViewer(ctx context.Context) *model.Viewer {
	viewer := &model.Viewer{
		User:            nil, // handled by dedicated resolver
		ViewerGalleries: nil, // handled by dedicated resolver
	}

	return viewer
}
