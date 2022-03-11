package graphql

// schema.resolvers.go gets updated when generating gqlgen bindings and should not contain
// helper functions. schema.resolvers.helpers.go is a companion file that can contain
// helper functions without interfering with code generation.

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
	"path/filepath"
	"strings"
)

var errNoAuthMechanismFound = fmt.Errorf("no auth mechanism found")

// errorToGraphqlType converts a golang error to its matching type from our GraphQL schema.
// If no matching type is found, ok will return false
func (r *Resolver) errorToGraphqlType(err error) (gqlError model.Error, ok bool) {
	message := err.Error()
	var mappedErr model.Error = nil

	// TODO: Add model.ErrNotAuthorized mapping once auth handling is moved to the publicapi layer

	switch err.(type) {
	case auth.ErrAuthenticationFailed:
		mappedErr = model.ErrAuthenticationFailed{Message: message}
	case auth.ErrDoesNotOwnRequiredNFT:
		mappedErr = model.ErrDoesNotOwnRequiredNft{Message: message}
	case persist.ErrUserNotFound:
		mappedErr = model.ErrUserNotFound{Message: message}
	case user.ErrUserAlreadyExists:
		mappedErr = model.ErrUserAlreadyExists{Message: message}
	case publicapi.ErrInvalidInput:
		validationErr, _ := err.(publicapi.ErrInvalidInput)
		mappedErr = model.ErrInvalidInput{Message: message, Parameters: validationErr.Parameters, Reasons: validationErr.Reasons}
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
		// GnosisSafe passes an empty signature
		return ethNonceAuth(m.GnosisSafe.Address, m.GnosisSafe.Nonce, "0x", auth.WalletTypeGnosis), nil
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
			Layout:         layoutToModel(ctx, collection.Layout),
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

func layoutToModel(ctx context.Context, layout persist.TokenLayout) *model.GalleryCollectionLayout {
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

func getUrlExtension(url string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(url), "."))
}

func getMediaForNft(nft persist.NFT) model.MediaSubtype {
	ext := getUrlExtension(*getFirstNonEmptyString(nft.AnimationURL.String(), nft.ImageURL.String(), nft.ImageOriginalURL.String()))

	switch ext {
	case "svg":
	case "gif":
	case "jpg":
	case "jpeg":
	case "png":
		return getImageMedia(nft)
	case "mp4":
		return getVideoMedia(nft)
	case "mp3":
	case "wav":
		return getAudioMedia(nft)
	case "html":
		return getHtmlMedia(nft)
	case "glb":
		return getUnknownMedia(nft)
	}
	// Note: default in v1 frontend mapping was "animation"
	return getUnknownMedia(nft)
}

func getFirstNonEmptyString(strings ...string) *string {
	for _, str := range strings {
		if str != "" {
			return &str
		}
	}

	empty := ""
	return &empty
}

func getPreviewUrls(nft persist.NFT) *model.PreviewURLSet {
	return &model.PreviewURLSet{
		Raw:    getFirstNonEmptyString(nft.ImageOriginalURL.String(), nft.AnimationURL.String()),
		Small:  getFirstNonEmptyString(nft.ImageThumbnailURL.String(), nft.AnimationURL.String()),
		Medium: getFirstNonEmptyString(nft.ImagePreviewURL.String(), nft.AnimationURL.String()),
		Large:  getFirstNonEmptyString(nft.ImageURL.String(), nft.AnimationURL.String()),
	}
}

func getImageMedia(nft persist.NFT) model.ImageMedia {
	imageUrls := model.ImageURLSet{
		Raw:    getFirstNonEmptyString(nft.ImageOriginalURL.String(), nft.AnimationURL.String()),
		Small:  getFirstNonEmptyString(nft.ImageThumbnailURL.String(), nft.AnimationURL.String()),
		Medium: getFirstNonEmptyString(nft.ImagePreviewURL.String(), nft.AnimationURL.String()),
		Large:  getFirstNonEmptyString(nft.ImageURL.String(), nft.AnimationURL.String()),
	}

	return model.ImageMedia{
		PreviewUrls:       getPreviewUrls(nft),
		MediaURL:          getFirstNonEmptyString(nft.ImageOriginalURL.String(), nft.ImageURL.String()),
		MediaType:         nil,
		ContentRenderUrls: &imageUrls,
	}
}

func getVideoMedia(nft persist.NFT) model.VideoMedia {
	videoUrls := model.VideoURLSet{
		Raw:    util.StringToPointer(nft.AnimationOriginalURL.String()),
		Small:  util.StringToPointer(nft.AnimationURL.String()),
		Medium: util.StringToPointer(nft.AnimationURL.String()),
		Large:  util.StringToPointer(nft.AnimationURL.String()),
	}

	return model.VideoMedia{
		PreviewUrls:       getPreviewUrls(nft),
		MediaURL:          getFirstNonEmptyString(nft.AnimationOriginalURL.String(), nft.AnimationURL.String()),
		MediaType:         nil,
		ContentRenderUrls: &videoUrls,
	}
}

func getAudioMedia(nft persist.NFT) model.AudioMedia {
	return model.AudioMedia{
		PreviewUrls:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalURL.String(), nft.AnimationURL.String()),
		MediaType:        nil,
		ContentRenderURL: util.StringToPointer(nft.AnimationURL.String()),
	}
}

func getTextMedia(nft persist.NFT) model.TextMedia {
	return model.TextMedia{
		PreviewUrls:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalURL.String(), nft.AnimationURL.String()),
		MediaType:        nil,
		ContentRenderURL: util.StringToPointer(nft.AnimationURL.String()),
	}
}

func getHtmlMedia(nft persist.NFT) model.HTMLMedia {
	return model.HTMLMedia{
		PreviewUrls:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalURL.String(), nft.AnimationURL.String()),
		MediaType:        nil,
		ContentRenderURL: util.StringToPointer(nft.AnimationURL.String()),
	}
}

func getJsonMedia(nft persist.NFT) model.JSONMedia {
	return model.JSONMedia{
		PreviewUrls:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalURL.String(), nft.AnimationURL.String()),
		MediaType:        nil,
		ContentRenderURL: util.StringToPointer(nft.AnimationURL.String()),
	}
}

func getUnknownMedia(nft persist.NFT) model.UnknownMedia {
	return model.UnknownMedia{}
}

func getInvalidMedia(nft persist.NFT) model.InvalidMedia {
	return model.InvalidMedia{}
}

// TODO: Temporary helper method. VERY SLOW. Will be replaced by optimized lookups before being used in production.
func collectionNftToNft(ctx context.Context, nft persist.CollectionNFT) (persist.NFT, error) {
	return dataloader.For(ctx).NftByNftId.Load(nft.ID)
}

func nftToModel(ctx context.Context, r *Resolver, nft persist.NFT) model.Nft {
	creationTime := nft.CreationTime.Time()
	lastUpdated := nft.LastUpdatedTime.Time()
	chainEthereum := model.ChainEthereum

	output := model.Nft{
		ID:               nft.ID,
		CreationTime:     &creationTime,
		LastUpdated:      &lastUpdated,
		CollectorsNote:   util.StringToPointer(nft.CollectorsNote.String()),
		Media:            getMediaForNft(nft),
		TokenType:        nil,            // TODO: later
		Chain:            &chainEthereum, // Everything's Ethereum right now
		Name:             util.StringToPointer(nft.Name.String()),
		Description:      util.StringToPointer(nft.Description.String()),
		TokenURI:         nil, // TODO: later
		TokenID:          util.StringToPointer(nft.OpenseaTokenID.String()),
		Quantity:         nil, // TODO: later
		Owner:            nil, // handled by dedicated resolver
		OwnershipHistory: nil, // TODO: later
		TokenMetadata:    nil, // TODO: later
		ContractAddress:  &nft.Contract.ContractAddress,
		ExternalURL:      util.StringToPointer(nft.ExternalURL.String()),
		BlockNumber:      nil, // TODO: later
	}

	return output
}

func collectionToModel(ctx context.Context, collection persist.Collection) *model.GalleryCollection {
	version := collection.Version.Int()
	hidden := collection.Hidden.Bool()

	// TODO: Should we be filling this collection's gallery out, or leaving it to a resolver?
	// The Gallery->Collections path currently fills out the Gallery field on each Collection it returns,
	// and switching to a resolver here means switching to a resolver there. Not a big deal, just remember
	// to prime the "GalleryByCollectionId" cache with results from the "collections by gallery" lookup.
	return &model.GalleryCollection{
		ID:             collection.ID,
		Version:        &version,
		Name:           util.StringToPointer(collection.Name.String()),
		CollectorsNote: util.StringToPointer(collection.CollectorsNote.String()),
		Gallery:        nil, // handled by dedicated resolver
		Layout:         layoutToModel(ctx, collection.Layout),
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

	return model.MembershipOwner{
		ID:          membershipOwner.UserID,
		Address:     &membershipOwner.Address,
		User:        nil, // handled by dedicated resolver
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
