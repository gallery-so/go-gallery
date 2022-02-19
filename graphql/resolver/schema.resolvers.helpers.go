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
	case auth.ErrSignatureVerificationFailed:
		mappedErr = model.ErrSignatureVerificationFailed{Message: message}
	case auth.ErrAddressDoesNotOwnRequiredNFT:
		mappedErr = model.ErrAddressDoesNotOwnRequiredNft{Message: message}
	case auth.ErrUserNotFound:
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

func resolveGalleryUserByUserID(ctx context.Context, r *Resolver, userID string) (*model.GalleryUser, error) {
	user, err := dataloader.For(ctx).UserByUserId.Load(userID)

	if err != nil {
		return nil, err
	}

	return userToUserModel(ctx, r, user)
}

func resolveGalleryUserByUsername(ctx context.Context, r *Resolver, username string) (*model.GalleryUser, error) {
	user, err := dataloader.For(ctx).UserByUsername.Load(username)

	if err != nil {
		return nil, err
	}

	return userToUserModel(ctx, r, user)
}

func resolveGalleryUserByAddress(ctx context.Context, r *Resolver, address string) (*model.GalleryUser, error) {
	user, err := dataloader.For(ctx).UserByAddress.Load(address)

	if err != nil {
		return nil, err
	}

	return userToUserModel(ctx, r, user)
}

func resolveGalleriesByUserID(ctx context.Context, r *Resolver, userID string) ([]*model.Gallery, error) {
	galleries, err := dataloader.For(ctx).GalleriesByUserId.Load(userID)

	if err != nil {
		return nil, err
	}

	var output = make([]*model.Gallery, len(galleries))
	for i, gallery := range galleries {
		output[i] = galleryToGalleryModel(gallery)
	}

	return output, nil
}

func resolveGalleryCollectionsByGalleryID(ctx context.Context, r *Resolver, galleryID string) ([]*model.GalleryCollection, error) {
	// TODO: Update this to query for collections by gallery ID, instead of querying for a user and returning
	// all of their collections. The result is the same right now, since a user only has one gallery.

	gallery, err := dataloader.For(ctx).GalleryByGalleryId.Load(galleryID)
	if err != nil {
		return nil, err
	}

	collections, err := dataloader.For(ctx).CollectionsByUserId.Load(gallery.OwnerUserID.String())
	if err != nil {
		return nil, err
	}

	var output = make([]*model.GalleryCollection, len(collections))
	for i, collection := range collections {
		// TODO: Do we store any 64-bit types (e.g. NFT Opensea token ID) that need special handling?
		version := int(collection.Version.Int64())
		hidden := collection.Hidden.Bool()

		output[i] = &model.GalleryCollection{
			ID:             collection.ID.String(),
			Version:        &version,
			Name:           util.StringToPointer(collection.Name.String()),
			CollectorsNote: util.StringToPointer(collection.CollectorsNote.String()),
			Gallery:        galleryIDToGalleryModel(galleryID),
			Layout:         layoutToLayoutModel(ctx, r, collection.Layout),
			Hidden:         &hidden,
			Nfts:           nil, // TODO: Delegate to a resolver
		}
	}

	return output, nil
}

func galleryToGalleryModel(gallery persist.Gallery) *model.Gallery {
	return galleryIDToGalleryModel(gallery.ID.String())
}

func galleryIDToGalleryModel(galleryID string) *model.Gallery {
	gallery := model.Gallery{
		ID:          galleryID,
		Owner:       nil, // handled by dedicated resolver
		Collections: nil, // handled by dedicated resolver
	}

	return &gallery
}

func layoutToLayoutModel(ctx context.Context, r *Resolver, layout persist.TokenLayout) *model.GalleryCollectionLayout {
	columns := int(layout.Columns.Int64())

	output := model.GalleryCollectionLayout{
		Columns: &columns,
	}

	return &output
}

// userToUserModel converts a persist.User to a model.User
func userToUserModel(ctx context.Context, r *Resolver, user persist.User) (*model.GalleryUser, error) {
	gc := util.GinContextFromContext(ctx)
	isAuthenticated := auth.GetUserAuthedFromCtx(gc)

	output := &model.GalleryUser{
		ID:                  user.ID.String(),
		Username:            util.StringToPointer(user.Username.String()),
		Bio:                 util.StringToPointer(user.Bio.String()),
		Wallets:             addressesToWalletModels(ctx, r, user.Addresses),
		Galleries:           nil, // handled by dedicated resolver
		IsAuthenticatedUser: &isAuthenticated,
	}

	return output, nil
}

// addressesToWalletModels converts a slice of persist.Address to a slice of model.Wallet
func addressesToWalletModels(ctx context.Context, r *Resolver, addresses []persist.Address) []*model.Wallet {
	wallets := make([]*model.Wallet, len(addresses))
	for i, address := range addresses {
		wallets[i] = &model.Wallet{
			ID:      "", // TODO: What's a wallet's ID?
			Address: util.StringToPointer(address.String()),
			Nfts:    nil, // handled by dedicated resolver
		}
	}

	return wallets
}
