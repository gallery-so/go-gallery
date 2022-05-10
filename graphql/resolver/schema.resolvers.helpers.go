package graphql

// schema.resolvers.go gets updated when generating gqlgen bindings and should not contain
// helper functions. schema.resolvers.helpers.go is a companion file that can contain
// helper functions without interfering with code generation.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/debugtools"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

var errNoAuthMechanismFound = fmt.Errorf("no auth mechanism found")

var nodeFetcher = model.NodeFetcher{
	OnGallery:        resolveGalleryByGalleryID,
	OnCollection:     resolveCollectionByCollectionID,
	OnGalleryUser:    resolveGalleryUserByUserID,
	OnMembershipTier: resolveMembershipTierByMembershipId,
	OnNft:            resolveNftByNftID,
	OnWallet:         resolveWalletByAddress,
	OnCommunity:      resolveCommunityByID,
	OnAddress:        resolveAddressByID,

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
	case publicapi.ErrTokenRefreshFailed:
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
		if viper.GetString("ENV") == "local" && m.Debug != nil {
			userID := persist.DBID("")
			if m.Debug.UserID != nil {
				userID = *m.Debug.UserID
			}
			return debugtools.NewDebugAuthenticator(userID, m.Debug.Addresses, m.Debug.Chains), nil
		}
	}

	if m.Eoa != nil {
		return authApi.NewNonceAuthenticator(m.Eoa.Address, m.Eoa.Chain, m.Eoa.Nonce, m.Eoa.Signature, persist.WalletTypeEOA), nil
	}

	if m.GnosisSafe != nil {
		// GnosisSafe passes an empty signature
		return authApi.NewNonceAuthenticator(m.Eoa.Address, persist.ChainETH, m.Eoa.Nonce, "0x", persist.WalletTypeGnosis), nil
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

func resolveGalleryUserByAddress(ctx context.Context, address persist.DBID) (*model.GalleryUser, error) {
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

func resolveNftsByUserID(ctx context.Context, userId persist.DBID) ([]*model.Nft, error) {
	nfts, err := publicapi.For(ctx).Nft.GetNftsByUserID(ctx, userId)

	if err != nil {
		return nil, err
	}

	return nftsToModel(ctx, nfts), nil
}
func resolveNftOwnerByNftID(ctx context.Context, nftID persist.DBID) (*model.GalleryUser, error) {
	nft, err := publicapi.For(ctx).Nft.GetNftById(ctx, nftID)

	if err != nil {
		return nil, err
	}

	return resolveGalleryUserByUserID(ctx, nft.OwnerUserID)
}

func resolveWalletByAddress(ctx context.Context, address persist.DBID) (*model.Wallet, error) {

	wallet := model.Wallet{
		// TODO
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

func resolveCommunityByContractAddress(ctx context.Context, contractAddress persist.AddressValue, chain persist.Chain) (*model.Community, error) {
	community, err := publicapi.For(ctx).User.GetCommunityByContractAddress(ctx, contractAddress, chain)

	if err != nil {
		return nil, err
	}

	return communityToModel(ctx, *community), nil
}

func resolveCommunityByID(ctx context.Context, communityAddressID string) (*model.Community, error) {
	address, err := publicapi.For(ctx).Address.GetAddressById(ctx, persist.DBID(communityAddressID))

	if err != nil {
		return nil, err
	}

	community, err := publicapi.For(ctx).User.GetCommunityByContractAddress(ctx, address.AddressValue, address.Chain)

	if err != nil {
		return nil, err
	}

	return communityToModel(ctx, *community), nil
}

func resolveGeneralAllowlist(ctx context.Context) ([]*model.Wallet, error) {
	addresses, err := publicapi.For(ctx).Misc.GetGeneralAllowlist(ctx)

	if err != nil {
		return nil, err
	}

	output := make([]*model.Wallet, 0, len(addresses))

	for _, address := range addresses {
		output = append(output, ethAddressToWalletModel(ctx, address))
	}

	return output, nil
}

func resolveAddressByID(ctx context.Context, addressID persist.DBID) (*model.Address, error) {
	address, err := publicapi.For(ctx).Address.GetAddressById(ctx, persist.DBID(addressID))

	if err != nil {
		return nil, err
	}

	return addressToModelSqlc(ctx, address), nil
}

func resolveAddressByWalletID(ctx context.Context, addressID persist.DBID) (*model.Address, error) {
	address, err := publicapi.For(ctx).Address.GetAddressByWalletID(ctx, persist.DBID(addressID))

	if err != nil {
		return nil, err
	}

	return addressToModelSqlc(ctx, address), nil
}
func resolveWalletsByUserID(ctx context.Context, userID persist.DBID) ([]*model.Wallet, error) {
	addresses, err := publicapi.For(ctx).Wallet.GetWalletsByUserID(ctx, userID)

	if err != nil {
		return nil, err
	}

	output := make([]*model.Wallet, 0, len(addresses))

	for _, address := range addresses {
		output = append(output, walletToModelSqlc(ctx, address))
	}

	return output, nil
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
		wallets[i] = walletToModelPersist(ctx, address)
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

func walletToModelPersist(ctx context.Context, wallet persist.Wallet) *model.Wallet {
	return &model.Wallet{
		Dbid:       wallet.ID,
		WalletType: &wallet.WalletType,
		Address:    addressToModelPersist(ctx, wallet.Address),
		Nfts:       nil, // handled by dedicated resolver
	}
}

func walletToModelSqlc(ctx context.Context, wallet sqlc.Wallet) *model.Wallet {
	return &model.Wallet{
		Dbid:       wallet.ID,
		WalletType: &wallet.WalletType,
		Address:    nil, // handled by dedicated resolver
		Nfts:       nil, // handled by dedicated resolver
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
	owners := make([]*model.MembershipOwner, 0, len(membershipTier.Owners))
	for _, owner := range membershipTier.Owners {
		if owner.UserID != "" {
			owners = append(owners, persistMembershipOwnerToModel(ctx, owner))
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
	owners := make([]*model.MembershipOwner, 0, len(membershipTier.Owners))
	for _, owner := range membershipTier.Owners {
		if owner.UserID != "" {
			owners = append(owners, persistMembershipOwnerToModel(ctx, owner))
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

func persistMembershipOwnerToModel(ctx context.Context, membershipOwner persist.MembershipOwner) *model.MembershipOwner {
	previewNfts := make([]*string, len(membershipOwner.PreviewNFTs))
	for i, nft := range membershipOwner.PreviewNFTs {
		previewNfts[i] = util.StringToPointer(nft.String())
	}
	return &model.MembershipOwner{
		Dbid:        membershipOwner.UserID,
		Address:     &membershipOwner.Address,
		User:        nil, // handled by dedicated resolver
		PreviewNfts: previewNfts,
	}
}

func nftToModel(ctx context.Context, nft sqlc.Token) *model.Nft {
	chainEthereum := persist.ChainETH

	return &model.Nft{
		Dbid:             nft.ID,
		CreationTime:     &nft.CreatedAt,
		LastUpdated:      &nft.LastUpdated,
		CollectorsNote:   &nft.CollectorsNote.String,
		Media:            getMediaForToken(nft),
		TokenType:        nil,            // TODO: later
		Chain:            &chainEthereum, // Everything's Ethereum right now
		Name:             &nft.Name.String,
		Description:      &nft.Description.String,
		OwnerAddresses:   nil, // handled by dedicated resolver
		TokenURI:         nil, // TODO: later
		TokenID:          &nft.TokenID.String,
		Quantity:         nil, // TODO: later
		Owner:            nil, // handled by dedicated resolver
		OwnershipHistory: nil, // TODO: later
		TokenMetadata:    nil, // TODO: later
		ContractAddress:  nil, // handled by dedicated resolver
		ExternalURL:      &nft.ExternalUrl.String,
		BlockNumber:      nil, // TODO: later

		// These are legacy mappings that will likely end up elsewhere when we pull data from the indexer
		CreatorAddress:        nil,                        // handled by dedicated resolver
		OpenseaCollectionName: &nft.CollectionName.String, // how do we get this?
	}
}

func nftsToModel(ctx context.Context, nft []sqlc.Token) []*model.Nft {
	res := make([]*model.Nft, len(nft))
	for i, nft := range nft {
		res[i] = nftToModel(ctx, nft)
	}
	return res
}

func communityToModel(ctx context.Context, community persist.Community) *model.Community {
	lastUpdated := community.LastUpdated.Time()

	owners := make([]*model.CommunityOwner, len(community.Owners))
	for i := range community.Owners {
		owners[i] = &model.CommunityOwner{
			Address:  walletToModelPersist(ctx, community.Owners[i].Wallet),
			Username: util.StringToPointer(community.Owners[i].Username.String()),
		}
	}

	return &model.Community{
		LastUpdated:     &lastUpdated,
		ContractAddress: addressToModelPersist(ctx, community.ContractAddress),
		CreatorAddress:  addressToModelPersist(ctx, community.CreatorAddress),
		Name:            util.StringToPointer(community.Name.String()),
		Description:     util.StringToPointer(community.Description.String()),
		PreviewImage:    util.StringToPointer(community.PreviewImage.String()),
		Owners:          owners,
	}
}

func addressToModelPersist(ctx context.Context, address persist.Address) *model.Address {
	return &model.Address{
		Dbid:    address.ID,
		Address: &address.AddressValue,
		Chain:   &address.Chain,
	}
}

func addressToModelSqlc(ctx context.Context, address *sqlc.Address) *model.Address {
	return &model.Address{
		Dbid:    address.ID,
		Address: &address.AddressValue,
		Chain:   &address.Chain,
	}
}
func ethAddressToWalletModel(ctx context.Context, address persist.EthereumAddress) *model.Wallet {
	dbWallet, _ := publicapi.For(ctx).Wallet.GetWalletByDetails(ctx, persist.AddressValue(address.String()), persist.ChainETH)
	dbAddr, _ := publicapi.For(ctx).Address.GetAddressByDetails(ctx, persist.AddressValue(address.String()), persist.ChainETH)
	return &model.Wallet{
		Dbid:       dbWallet.ID,
		WalletType: &dbWallet.WalletType,
		Address: &model.Address{
			Dbid:    dbAddr.ID,
			Address: &dbAddr.AddressValue,
			Chain:   &dbAddr.Chain,
		},
		Nfts: nil, // handled by dedicated resolver
	}
}

func getUrlExtension(url string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(url), "."))
}

func getMediaForToken(token sqlc.Token) model.MediaSubtype {
	var med persist.Media
	err := token.Media.AssignTo(&med)
	if err != nil {
		return getInvalidMedia(med)
	}

	switch med.MediaType {
	case persist.MediaTypeImage, persist.MediaTypeGIF:
		return getImageMedia(med)
	case persist.MediaTypeVideo:
		return getVideoMedia(med)
	case persist.MediaTypeAudio:
		return getAudioMedia(med)
	case persist.MediaTypeHTML:
		return getHtmlMedia(med)
	case persist.MediaTypeAnimation:
		return getGltfMedia(med)
	case persist.MediaTypeJSON, persist.MediaTypeBase64JSON:
		return getJsonMedia(med)
	case persist.MediaTypeSVG, persist.MediaTypeText, persist.MediaTypeBase64SVG, persist.MediaTypeBase64Text:
		return getTextMedia(med)
	default:
		return getUnknownMedia(med)
	}

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

func getPreviewUrls(media persist.Media) *model.PreviewURLSet {
	return &model.PreviewURLSet{
		Raw:    remapLargeImageUrls(getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String())),
		Small:  remapLargeImageUrls(getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String())),
		Medium: remapLargeImageUrls(getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String())),
		Large:  remapLargeImageUrls(getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String())),
	}
}

func getImageMedia(media persist.Media) model.ImageMedia {
	imageUrls := model.ImageURLSet{
		Raw:    remapLargeImageUrls(getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String())),
		Small:  remapLargeImageUrls(getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String())),
		Medium: remapLargeImageUrls(getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String())),
		Large:  remapLargeImageUrls(getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String())),
	}

	return model.ImageMedia{
		PreviewURLs:       getPreviewUrls(media),
		MediaURL:          getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String()),
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

func getVideoMedia(media persist.Media) model.VideoMedia {
	asString := media.MediaURL.String()
	videoUrls := model.VideoURLSet{
		Raw:    &asString,
		Small:  &asString,
		Medium: &asString,
		Large:  &asString,
	}

	return model.VideoMedia{
		PreviewURLs:       getPreviewUrls(media),
		MediaURL:          getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String()),
		MediaType:         nil,
		ContentRenderURLs: &videoUrls,
	}
}

func getAudioMedia(media persist.Media) model.AudioMedia {
	return model.AudioMedia{
		PreviewURLs:      getPreviewUrls(media),
		MediaURL:         getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String()),
		MediaType:        nil,
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getTextMedia(media persist.Media) model.TextMedia {
	return model.TextMedia{
		PreviewURLs:      getPreviewUrls(media),
		MediaURL:         getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String()),
		MediaType:        nil,
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getHtmlMedia(media persist.Media) model.HTMLMedia {
	return model.HTMLMedia{
		PreviewURLs:      getPreviewUrls(media),
		MediaURL:         getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String()),
		MediaType:        nil,
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getJsonMedia(media persist.Media) model.JSONMedia {
	return model.JSONMedia{
		PreviewURLs:      getPreviewUrls(media),
		MediaURL:         getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String()),
		MediaType:        nil,
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getGltfMedia(media persist.Media) model.GltfMedia {
	return model.GltfMedia{
		PreviewURLs:      getPreviewUrls(media),
		MediaURL:         getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String()),
		MediaType:        nil,
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getUnknownMedia(media persist.Media) model.UnknownMedia {
	return model.UnknownMedia{
		PreviewURLs:      getPreviewUrls(media),
		MediaURL:         getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String()),
		MediaType:        nil,
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getInvalidMedia(media persist.Media) model.InvalidMedia {
	return model.InvalidMedia{
		PreviewURLs:      getPreviewUrls(media),
		MediaURL:         getFirstNonEmptyString(media.MediaURL.String(), media.ThumbnailURL.String()),
		MediaType:        nil,
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}
