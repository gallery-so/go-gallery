package graphql

// schema.resolvers.go gets updated when generating gqlgen bindings and should not contain
// helper functions. schema.resolvers.helpers.go is a companion file that can contain
// helper functions without interfering with code generation.

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/db/sqlc"
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
func errorToGraphqlType(ctx context.Context, err error, gqlTypeName string) (gqlModel interface{}, ok bool) {
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
	case persist.ErrCollectionNotFoundByID:
		mappedErr = model.ErrCollectionNotFound{Message: message}
	case publicapi.ErrInvalidInput:
		validationErr, _ := err.(publicapi.ErrInvalidInput)
		mappedErr = model.ErrInvalidInput{Message: message, Parameters: validationErr.Parameters, Reasons: validationErr.Reasons}
	}

	if mappedErr != nil {
		if converted, ok := model.ConvertToModelType(mappedErr, gqlTypeName); ok {
			addError(ctx, err, converted)
			return converted, true
		}
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
	user, err := publicapi.For(ctx).User.GetUserById(ctx, userID)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, r, *user)
}

func resolveGalleryUserByUsername(ctx context.Context, r *Resolver, username string) (*model.GalleryUser, error) {
	user, err := publicapi.For(ctx).User.GetUserByUsername(ctx, username)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, r, *user)
}

func resolveGalleryUserByAddress(ctx context.Context, r *Resolver, address persist.Address) (*model.GalleryUser, error) {
	user, err := publicapi.For(ctx).User.GetUserByAddress(ctx, address)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, r, *user)
}

func resolveGalleriesByUserID(ctx context.Context, r *Resolver, userID persist.DBID) ([]*model.Gallery, error) {
	galleries, err := publicapi.For(ctx).Gallery.GetGalleriesByUserId(ctx, userID)

	if err != nil {
		return nil, err
	}

	var output = make([]*model.Gallery, len(galleries))
	for i, gallery := range galleries {
		output[i] = galleryToModel(gallery)
	}

	return output, nil
}

func resolveCollectionsByGalleryID(ctx context.Context, r *Resolver, galleryID persist.DBID) ([]*model.GalleryCollection, error) {
	collections, err := publicapi.For(ctx).Collection.GetCollectionsByGalleryId(ctx, galleryID)
	if err != nil {
		return nil, err
	}

	var output = make([]*model.GalleryCollection, len(collections))
	for i, collection := range collections {
		version := int(collection.Version.Int32)
		hidden := collection.Hidden

		output[i] = &model.GalleryCollection{
			Dbid:           collection.ID,
			Version:        &version,
			Name:           util.StringToPointer(collection.Name.String),
			CollectorsNote: util.StringToPointer(collection.CollectorsNote.String),
			Gallery:        galleryIDToGalleryModel(galleryID),
			Layout:         layoutToModel(ctx, collection.Layout),
			Hidden:         &hidden,
			Nfts:           nil, // handled by dedicated resolver
		}
	}

	return output, nil
}

func galleryToModel(gallery sqlc.Gallery) *model.Gallery {
	return galleryIDToGalleryModel(gallery.ID)
}

func galleryIDToGalleryModel(galleryID persist.DBID) *model.Gallery {
	return &model.Gallery{
		Dbid:        galleryID,
		Owner:       nil, // handled by dedicated resolver
		Collections: nil, // handled by dedicated resolver
	}
}

func layoutToModel(ctx context.Context, layout sqlc.TokenLayout) *model.GalleryCollectionLayout {
	whitespace := make([]*int, len(layout.Whitespace))
	for i, w := range layout.Whitespace {
		whitespace[i] = &w
	}

	output := model.GalleryCollectionLayout{
		Columns:    &layout.Columns,
		Whitespace: whitespace,
	}

	return &output
}

// userToModel converts a sqlc.User to a model.User
func userToModel(ctx context.Context, r *Resolver, user sqlc.User) (*model.GalleryUser, error) {
	gc := util.GinContextFromContext(ctx)
	isAuthenticated := auth.GetUserAuthedFromCtx(gc)

	galleryUser := &model.GalleryUser{
		Dbid:                user.ID,
		Username:            &user.Username.String,
		Bio:                 &user.Bio.String,
		Wallets:             addressesToModels(ctx, r, user.Addresses),
		Galleries:           nil, // handled by dedicated resolver
		IsAuthenticatedUser: &isAuthenticated,
	}

	return galleryUser, nil
}

// addressesToModels converts a slice of persist.Address to a slice of model.Wallet
func addressesToModels(ctx context.Context, r *Resolver, addresses []persist.Address) []*model.Wallet {
	wallets := make([]*model.Wallet, len(addresses))
	for i, address := range addresses {
		wallets[i] = &model.Wallet{
			Address: &address,
			Nfts:    nil, // handled by dedicated resolver
		}
	}

	return wallets
}

func resolveNftOwnerByNftId(ctx context.Context, r *Resolver, nftId persist.DBID) (model.GalleryUserOrWallet, error) {
	nft, err := publicapi.For(ctx).Nft.GetNftById(ctx, nftId)

	if err != nil {
		return nil, err
	}

	return resolveGalleryUserOrWalletByAddress(ctx, r, nft.OwnerAddress)
}

func resolveGalleryUserOrWalletByAddress(ctx context.Context, r *Resolver, address persist.Address) (model.GalleryUserOrWallet, error) {
	owner, err := publicapi.For(ctx).User.GetUserByAddress(ctx, address)

	if err == nil {
		return userToModel(ctx, r, *owner)
	}

	if _, ok := err.(persist.ErrUserNotFound); ok {
		wallet := model.Wallet{
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

func getMediaForNft(nft sqlc.Nft) model.MediaSubtype {
	// Extension/URL checking based on the existing frontend methodology
	ext := getUrlExtension(nft.ImageUrl.String)
	if ext == "mp4" {
		return getVideoMedia(nft)
	}

	if nft.AnimationUrl.String == "" {
		return getImageMedia(nft)
	}

	ext = getUrlExtension(nft.AnimationUrl.String)

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

func getPreviewUrls(nft sqlc.Nft) *model.PreviewURLSet {
	return &model.PreviewURLSet{
		Raw:    getFirstNonEmptyString(nft.ImageOriginalUrl.String, nft.AnimationUrl.String),
		Small:  getFirstNonEmptyString(nft.ImageThumbnailUrl.String, nft.AnimationUrl.String),
		Medium: getFirstNonEmptyString(nft.ImagePreviewUrl.String, nft.AnimationUrl.String),
		Large:  getFirstNonEmptyString(nft.ImageUrl.String, nft.AnimationUrl.String),
	}
}

func getImageMedia(nft sqlc.Nft) model.ImageMedia {
	imageUrls := model.ImageURLSet{
		Raw:    getFirstNonEmptyString(nft.ImageOriginalUrl.String, nft.AnimationUrl.String),
		Small:  getFirstNonEmptyString(nft.ImageThumbnailUrl.String, nft.AnimationUrl.String),
		Medium: getFirstNonEmptyString(nft.ImagePreviewUrl.String, nft.AnimationUrl.String),
		Large:  getFirstNonEmptyString(nft.ImageUrl.String, nft.AnimationUrl.String),
	}

	return model.ImageMedia{
		PreviewURLs:       getPreviewUrls(nft),
		MediaURL:          getFirstNonEmptyString(nft.ImageOriginalUrl.String, nft.ImageUrl.String),
		MediaType:         nil,
		ContentRenderURLs: &imageUrls,
	}
}

func getVideoMedia(nft sqlc.Nft) model.VideoMedia {
	videoUrls := model.VideoURLSet{
		Raw:    util.StringToPointer(nft.AnimationOriginalUrl.String),
		Small:  util.StringToPointer(nft.AnimationUrl.String),
		Medium: util.StringToPointer(nft.AnimationUrl.String),
		Large:  util.StringToPointer(nft.AnimationUrl.String),
	}

	return model.VideoMedia{
		PreviewURLs:       getPreviewUrls(nft),
		MediaURL:          getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:         nil,
		ContentRenderURLs: &videoUrls,
	}
}

func getAudioMedia(nft sqlc.Nft) model.AudioMedia {
	return model.AudioMedia{
		PreviewURLs:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:        nil,
		ContentRenderURL: util.StringToPointer(nft.AnimationUrl.String),
	}
}

func getTextMedia(nft sqlc.Nft) model.TextMedia {
	return model.TextMedia{
		PreviewURLs:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:        nil,
		ContentRenderURL: util.StringToPointer(nft.AnimationUrl.String),
	}
}

func getHtmlMedia(nft sqlc.Nft) model.HTMLMedia {
	return model.HTMLMedia{
		PreviewURLs:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:        nil,
		ContentRenderURL: util.StringToPointer(nft.AnimationUrl.String),
	}
}

func getJsonMedia(nft sqlc.Nft) model.JSONMedia {
	return model.JSONMedia{
		PreviewURLs:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:        nil,
		ContentRenderURL: util.StringToPointer(nft.AnimationUrl.String),
	}
}

func getUnknownMedia(nft sqlc.Nft) model.UnknownMedia {
	return model.UnknownMedia{
		PreviewURLs:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:        nil,
		ContentRenderURL: &nft.AnimationUrl.String,
	}
}

func getInvalidMedia(nft sqlc.Nft) model.InvalidMedia {
	return model.InvalidMedia{}
}

func nftToModel(ctx context.Context, r *Resolver, nft sqlc.Nft) model.Nft {
	chainEthereum := model.ChainEthereum

	return model.Nft{
		Dbid:             nft.ID,
		CreationTime:     &nft.CreatedAt,
		LastUpdated:      &nft.LastUpdated,
		CollectorsNote:   &nft.CollectorsNote.String,
		Media:            getMediaForNft(nft),
		TokenType:        nil,            // TODO: later
		Chain:            &chainEthereum, // Everything's Ethereum right now
		Name:             &nft.Name.String,
		Description:      &nft.Description.String,
		TokenURI:         nil, // TODO: later
		TokenID:          &nft.OpenseaTokenID.String,
		Quantity:         nil, // TODO: later
		Owner:            nil, // handled by dedicated resolver
		OwnershipHistory: nil, // TODO: later
		TokenMetadata:    nil, // TODO: later
		ContractAddress:  &nft.Contract.ContractAddress,
		ExternalURL:      &nft.ExternalUrl.String,
		BlockNumber:      nil, // TODO: later
	}
}

func collectionToModel(ctx context.Context, collection sqlc.Collection) *model.GalleryCollection {
	version := int(collection.Version.Int32)
	hidden := collection.Hidden

	// TODO: Should we be filling this collection's gallery out, or leaving it to a resolver?
	// The Gallery->Collections path currently fills out the Gallery field on each Collection it returns,
	// and switching to a resolver here means switching to a resolver there. Not a big deal, just remember
	// to prime the "GalleryByCollectionId" cache with results from the "collections by gallery" lookup.
	return &model.GalleryCollection{
		Dbid:           collection.ID,
		Version:        &version,
		Name:           util.StringToPointer(collection.Name.String),
		CollectorsNote: util.StringToPointer(collection.CollectorsNote.String),
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
		Dbid:     membershipTier.ID,
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
		Dbid:        membershipOwner.UserID,
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
