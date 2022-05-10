package graphql

// schema.resolvers.go gets updated when generating gqlgen bindings and should not contain
// helper functions. schema.resolvers.helpers.go is a companion file that can contain
// helper functions without interfering with code generation.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mikeydub/go-gallery/debugtools"
	"github.com/spf13/viper"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var errNoAuthMechanismFound = fmt.Errorf("no auth mechanism found")

var nodeFetcher = model.NodeFetcher{
	OnGallery:        resolveGalleryByGalleryID,
	OnCollection:     resolveCollectionByCollectionID,
	OnGalleryUser:    resolveGalleryUserByUserID,
	OnMembershipTier: resolveMembershipTierByMembershipId,
	OnNft:            resolveNftByNftID,
	OnWallet:         resolveWalletByAddress,
	OnCommunity:      resolveCommunityByContractAddress,

	OnCollectionNft: func(ctx context.Context, nftId string, collectionId string) (*model.CollectionNft, error) {
		return resolveCollectionNftByIDs(ctx, persist.DBID(nftId), persist.DBID(collectionId))
	},
}

func init() {
	nodeFetcher.ValidateHandlers()
}

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
	case persist.ErrUserAlreadyExists:
		mappedErr = model.ErrUserAlreadyExists{Message: message}
	case persist.ErrCollectionNotFoundByID:
		mappedErr = model.ErrCollectionNotFound{Message: message}
	case persist.ErrNFTNotFoundByID:
		mappedErr = model.ErrNftNotFound{Message: message}
	case persist.ErrCommunityNotFound:
		mappedErr = model.ErrCommunityNotFound{Message: message}
	case publicapi.ErrOpenSeaRefreshFailed:
		mappedErr = model.ErrOpenSeaRefreshFailed{Message: message}
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
func (r *Resolver) authMechanismToAuthenticator(ctx context.Context, m model.AuthMechanism) (auth.Authenticator, error) {
	authApi := publicapi.For(ctx).Auth

	if debugtools.Enabled {
		if viper.GetString("ENV") == "local" && m.DebugAuth != nil {
			userID := persist.DBID("")
			if m.DebugAuth.UserID != nil {
				userID = *m.DebugAuth.UserID
			}
			return debugtools.NewDebugAuthenticator(userID, m.DebugAuth.Addresses), nil
		}
	}

	if m.EthereumEoa != nil {
		return authApi.NewEthereumNonceAuthenticator(m.EthereumEoa.Address, m.EthereumEoa.Nonce, m.EthereumEoa.Signature, auth.WalletTypeEOA), nil
	}

	if m.GnosisSafe != nil {
		// GnosisSafe passes an empty signature
		return authApi.NewEthereumNonceAuthenticator(m.GnosisSafe.Address, m.GnosisSafe.Nonce, "0x", auth.WalletTypeGnosis), nil
	}

	return nil, errNoAuthMechanismFound
}

func resolveGalleryUserByUserID(ctx context.Context, userID persist.DBID) (*model.GalleryUser, error) {
	user, err := publicapi.For(ctx).User.GetUserById(ctx, userID)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, *user), nil
}

func resolveGalleryUserByUsername(ctx context.Context, username string) (*model.GalleryUser, error) {
	user, err := publicapi.For(ctx).User.GetUserByUsername(ctx, username)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, *user), nil
}

func resolveGalleryUserByAddress(ctx context.Context, address persist.Address) (*model.GalleryUser, error) {
	user, err := publicapi.For(ctx).User.GetUserByAddress(ctx, address)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, *user), nil
}

func resolveGalleriesByUserID(ctx context.Context, userID persist.DBID) ([]*model.Gallery, error) {
	galleries, err := publicapi.For(ctx).Gallery.GetGalleriesByUserId(ctx, userID)

	if err != nil {
		return nil, err
	}

	var output = make([]*model.Gallery, len(galleries))
	for i, gallery := range galleries {
		output[i] = galleryToModel(ctx, gallery)
	}

	return output, nil
}

func resolveCollectionByCollectionID(ctx context.Context, collectionID persist.DBID) (*model.Collection, error) {
	collection, err := publicapi.For(ctx).Collection.GetCollectionById(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	return collectionToModel(ctx, *collection), nil
}

func resolveCollectionsByGalleryID(ctx context.Context, galleryID persist.DBID) ([]*model.Collection, error) {
	collections, err := publicapi.For(ctx).Collection.GetCollectionsByGalleryId(ctx, galleryID)
	if err != nil {
		return nil, err
	}

	var output = make([]*model.Collection, len(collections))
	for i, collection := range collections {
		output[i] = collectionToModel(ctx, collection)
	}

	return output, nil
}

func resolveCollectionNftByIDs(ctx context.Context, nftID persist.DBID, collectionID persist.DBID) (*model.CollectionNft, error) {
	nft, err := resolveNftByNftID(ctx, nftID)
	if err != nil {
		return nil, err
	}

	collection, err := resolveCollectionByCollectionID(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	collectionNft := &model.CollectionNft{
		HelperCollectionNftData: model.HelperCollectionNftData{
			NftId:        nftID,
			CollectionId: collectionID,
		},
		Nft:        nft,
		Collection: collection,
	}

	return collectionNft, nil
}

func resolveGalleryByGalleryID(ctx context.Context, galleryID persist.DBID) (*model.Gallery, error) {
	gallery := &model.Gallery{
		Dbid:        galleryID,
		Owner:       nil, // handled by dedicated resolver
		Collections: nil, // handled by dedicated resolver
	}

	return gallery, nil
}

func resolveNftByNftID(ctx context.Context, nftID persist.DBID) (*model.Nft, error) {
	nft, err := publicapi.For(ctx).Nft.GetNftById(ctx, nftID)

	if err != nil {
		return nil, err
	}

	return nftToModel(ctx, *nft), nil
}

func resolveNftOwnerByNftID(ctx context.Context, nftID persist.DBID) (model.GalleryUserOrWallet, error) {
	nft, err := publicapi.For(ctx).Nft.GetNftById(ctx, nftID)

	if err != nil {
		return nil, err
	}

	return resolveGalleryUserOrWalletByAddress(ctx, nft.OwnerAddress)
}

func resolveGalleryUserOrWalletByAddress(ctx context.Context, address persist.Address) (model.GalleryUserOrWallet, error) {
	owner, err := publicapi.For(ctx).User.GetUserByAddress(ctx, address)

	if err == nil {
		return userToModel(ctx, *owner), nil
	}

	if _, ok := err.(persist.ErrUserNotFound); ok {
		return resolveWalletByAddress(ctx, address)
	}

	return nil, err
}

func resolveWalletByAddress(ctx context.Context, address persist.Address) (*model.Wallet, error) {
	wallet := model.Wallet{
		Address: &address,
		Nfts:    nil, // handled by dedicated resolver
	}

	return &wallet, nil
}

func resolveViewer(ctx context.Context) *model.Viewer {
	if !publicapi.For(ctx).User.IsUserLoggedIn(ctx) {
		return nil
	}

	viewer := &model.Viewer{
		User:            nil, // handled by dedicated resolver
		ViewerGalleries: nil, // handled by dedicated resolver
	}

	return viewer
}

func resolveMembershipTierByMembershipId(ctx context.Context, id persist.DBID) (*model.MembershipTier, error) {
	tier, err := publicapi.For(ctx).User.GetMembershipByMembershipId(ctx, id)

	if err != nil {
		return nil, err
	}

	return membershipToModel(ctx, *tier), nil
}

func resolveCommunityByContractAddressWithRefresh(ctx context.Context, contractAddress persist.Address, forceRefresh bool) (*model.Community, error) {
	community, err := publicapi.For(ctx).User.GetCommunityByContractAddress(ctx, contractAddress, forceRefresh)

	if err != nil {
		return nil, err
	}

	return communityToModel(ctx, *community), nil
}

func resolveCommunityByContractAddress(ctx context.Context, contractAddress persist.Address) (*model.Community, error) {
	community, err := publicapi.For(ctx).User.GetCommunityByContractAddress(ctx, contractAddress, false)

	if err != nil {
		return nil, err
	}

	return communityToModel(ctx, *community), nil
}

func galleryToModel(ctx context.Context, gallery sqlc.Gallery) *model.Gallery {
	return &model.Gallery{
		Dbid:        gallery.ID,
		Owner:       nil, // handled by dedicated resolver
		Collections: nil, // handled by dedicated resolver
	}
}

func layoutToModel(ctx context.Context, layout sqlc.TokenLayout) *model.CollectionLayout {
	whitespace := make([]*int, len(layout.Whitespace))
	for i, w := range layout.Whitespace {
		w := w
		whitespace[i] = &w
	}

	return &model.CollectionLayout{
		Columns:    &layout.Columns,
		Whitespace: whitespace,
	}
}

// userToModel converts a sqlc.User to a model.User
func userToModel(ctx context.Context, user sqlc.User) *model.GalleryUser {
	userApi := publicapi.For(ctx).User
	isAuthenticatedUser := userApi.IsUserLoggedIn(ctx) && userApi.GetLoggedInUserId(ctx) == user.ID

	wallets := make([]*model.Wallet, len(user.Addresses))
	for i, address := range user.Addresses {
		wallets[i] = addressToModel(ctx, address)
	}

	return &model.GalleryUser{
		Dbid:                user.ID,
		Username:            &user.Username.String,
		Bio:                 &user.Bio.String,
		Wallets:             wallets,
		Galleries:           nil, // handled by dedicated resolver
		IsAuthenticatedUser: &isAuthenticatedUser,
	}
}

func addressToModel(ctx context.Context, address persist.Address) *model.Wallet {
	return &model.Wallet{
		Address: &address,
		Nfts:    nil, // handled by dedicated resolver
	}
}

func collectionToModel(ctx context.Context, collection sqlc.Collection) *model.Collection {
	version := int(collection.Version.Int32)

	return &model.Collection{
		Dbid:           collection.ID,
		Version:        &version,
		Name:           &collection.Name.String,
		CollectorsNote: &collection.CollectorsNote.String,
		Gallery:        nil, // handled by dedicated resolver
		Layout:         layoutToModel(ctx, collection.Layout),
		Hidden:         &collection.Hidden,
		Nfts:           nil, // handled by dedicated resolver
	}
}

func membershipToModel(ctx context.Context, membershipTier sqlc.Membership) *model.MembershipTier {
	owners := make([]*model.TokenHolder, 0, len(membershipTier.Owners))
	for _, owner := range membershipTier.Owners {
		if owner.UserID != "" {
			owners = append(owners, tokenHolderToModel(ctx, owner))
		}
	}

	return &model.MembershipTier{
		Dbid:     membershipTier.ID,
		Name:     &membershipTier.Name.String,
		AssetURL: &membershipTier.AssetUrl.String,
		TokenID:  &membershipTier.TokenID.String,
		Owners:   owners,
	}
}

func persistMembershipTierToModel(ctx context.Context, membershipTier persist.MembershipTier) *model.MembershipTier {
	owners := make([]*model.TokenHolder, 0, len(membershipTier.Owners))
	for _, owner := range membershipTier.Owners {
		if owner.UserID != "" {
			owners = append(owners, tokenHolderToModel(ctx, owner))
		}
	}

	return &model.MembershipTier{
		Dbid:     membershipTier.ID,
		Name:     util.StringToPointer(membershipTier.Name.String()),
		AssetURL: util.StringToPointer(membershipTier.AssetURL.String()),
		TokenID:  util.StringToPointer(membershipTier.TokenID.String()),
		Owners:   owners,
	}
}

func tokenHolderToModel(ctx context.Context, tokenHolder persist.TokenHolder) *model.TokenHolder {
	previewNfts := make([]*string, len(tokenHolder.PreviewNFTs))
	for i, nft := range tokenHolder.PreviewNFTs {
		previewNfts[i] = util.StringToPointer(nft.String())
	}

	addresses := make([]*persist.Address, len(tokenHolder.Addresses))
	for i, address := range tokenHolder.Addresses {
		address := address
		addresses[i] = &address
	}

	return &model.TokenHolder{
		Dbid:        tokenHolder.UserID,
		Addresses:   addresses,
		User:        nil, // handled by dedicated resolver
		PreviewNfts: previewNfts,
	}
}

func nftToModel(ctx context.Context, nft sqlc.Nft) *model.Nft {
	chainEthereum := model.ChainEthereum

	return &model.Nft{
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

		// These are legacy mappings that will likely end up elsewhere when we pull data from the indexer
		CreatorAddress:        &nft.CreatorAddress,
		OpenseaCollectionName: &nft.TokenCollectionName.String,
	}
}

func communityToModel(ctx context.Context, community persist.Community) *model.Community {
	lastUpdated := community.LastUpdated.Time()

	owners := make([]*model.TokenHolder, len(community.Owners))
	for _, owner := range community.Owners {
		if owner.UserID != "" {
			owners = append(owners, tokenHolderToModel(ctx, owner))
		}
	}

	return &model.Community{
		LastUpdated:     &lastUpdated,
		ContractAddress: &community.ContractAddress,
		CreatorAddress:  &community.CreatorAddress,
		Name:            util.StringToPointer(community.Name.String()),
		Description:     util.StringToPointer(community.Description.String()),
		PreviewImage:    util.StringToPointer(community.PreviewImage.String()),
		Owners:          owners,
	}
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
		fallthrough
	case "gif":
		fallthrough
	case "jpg":
		fallthrough
	case "jpeg":
		fallthrough
	case "png":
		return getImageMedia(nft)
	case "mp4":
		return getVideoMedia(nft)
	case "mp3":
		fallthrough
	case "wav":
		return getAudioMedia(nft)
	case "html":
		return getHtmlMedia(nft)
	case "glb":
		fallthrough
	case "gltf":
		return getGltfMedia(nft)
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
		Raw:    remapLargeImageUrls(getFirstNonEmptyString(nft.ImageOriginalUrl.String, nft.AnimationUrl.String)),
		Small:  remapLargeImageUrls(getFirstNonEmptyString(nft.ImageThumbnailUrl.String, nft.AnimationUrl.String)),
		Medium: remapLargeImageUrls(getFirstNonEmptyString(nft.ImagePreviewUrl.String, nft.AnimationUrl.String)),
		Large:  remapLargeImageUrls(getFirstNonEmptyString(nft.ImageUrl.String, nft.AnimationUrl.String)),
	}
}

func getImageMedia(nft sqlc.Nft) model.ImageMedia {
	imageUrls := model.ImageURLSet{
		Raw:    remapLargeImageUrls(getFirstNonEmptyString(nft.ImageOriginalUrl.String, nft.AnimationUrl.String)),
		Small:  remapLargeImageUrls(getFirstNonEmptyString(nft.ImageThumbnailUrl.String, nft.AnimationUrl.String)),
		Medium: remapLargeImageUrls(getFirstNonEmptyString(nft.ImagePreviewUrl.String, nft.AnimationUrl.String)),
		Large:  remapLargeImageUrls(getFirstNonEmptyString(nft.ImageUrl.String, nft.AnimationUrl.String)),
	}

	return model.ImageMedia{
		PreviewURLs:       getPreviewUrls(nft),
		MediaURL:          getFirstNonEmptyString(nft.ImageOriginalUrl.String, nft.ImageUrl.String),
		MediaType:         nil,
		ContentRenderURLs: &imageUrls,
	}
}

// Temporary method for handling the large "dead ringers" NFT image. This remapping
// step should actually happen as part of generating resized images with imgix.
func remapLargeImageUrls(url *string) *string {
	if url == nil || (*url != "https://storage.opensea.io/files/33ab86c2a565430af5e7fb8399876960.png" && *url != "https://openseauserdata.com/files/33ab86c2a565430af5e7fb8399876960.png") {
		return url
	}

	remapped := "https://lh3.googleusercontent.com/pw/AM-JKLVsudnwN97ULF-DgJC1J_AZ8i-1pMjLCVUqswF1_WShId30uP_p_jSRkmVx-XNgKNIGFSglgRojZQrsLOoCM2pVNJwgx5_E4yeYRsMvDQALFKbJk0_6wj64tjLhSIINwGpdNw0MhtWNehKCipDKNeE"
	return &remapped
}

func getVideoMedia(nft sqlc.Nft) model.VideoMedia {
	videoUrls := model.VideoURLSet{
		Raw:    &nft.AnimationOriginalUrl.String,
		Small:  &nft.AnimationUrl.String,
		Medium: &nft.AnimationUrl.String,
		Large:  &nft.AnimationUrl.String,
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
		ContentRenderURL: &nft.AnimationUrl.String,
	}
}

func getTextMedia(nft sqlc.Nft) model.TextMedia {
	return model.TextMedia{
		PreviewURLs:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:        nil,
		ContentRenderURL: &nft.AnimationUrl.String,
	}
}

func getHtmlMedia(nft sqlc.Nft) model.HTMLMedia {
	return model.HTMLMedia{
		PreviewURLs:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:        nil,
		ContentRenderURL: &nft.AnimationUrl.String,
	}
}

func getJsonMedia(nft sqlc.Nft) model.JSONMedia {
	return model.JSONMedia{
		PreviewURLs:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:        nil,
		ContentRenderURL: &nft.AnimationUrl.String,
	}
}

func getGltfMedia(nft sqlc.Nft) model.GltfMedia {
	return model.GltfMedia{
		PreviewURLs:      getPreviewUrls(nft),
		MediaURL:         getFirstNonEmptyString(nft.AnimationOriginalUrl.String, nft.AnimationUrl.String),
		MediaType:        nil,
		ContentRenderURL: &nft.AnimationUrl.String,
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
