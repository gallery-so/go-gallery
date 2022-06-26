package graphql

// schema.resolvers.go gets updated when generating gqlgen bindings and should not contain
// helper functions. schema.resolvers.helpers.go is a companion file that can contain
// helper functions without interfering with code generation.

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/graphql/model/cursor"
	"github.com/mikeydub/go-gallery/service/mediamapper"

	"github.com/mikeydub/go-gallery/debugtools"
	"github.com/spf13/viper"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var errNoAuthMechanismFound = fmt.Errorf("no auth mechanism found")

var nodeFetcher = model.NodeFetcher{
	OnGallery:                    resolveGalleryByGalleryID,
	OnCollection:                 resolveCollectionByCollectionID,
	OnGalleryUser:                resolveGalleryUserByUserID,
	OnMembershipTier:             resolveMembershipTierByMembershipId,
	OnToken:                      resolveTokenByTokenID,
	OnWallet:                     resolveWalletByAddress,
	OnContract:                   resolveContractByContractID,
	OnCollectionCreatedFeedEvent: resolveCollectionCreatedFeedEventByEventID,
	OnCollectorsNoteAddedToCollectionFeedEvent: resolveCollectorsNoteAddedToCollectionFeedEventByEventID,
	OnCollectorsNoteAddedToTokenFeedEvent:      resolveCollectorsNoteAddedToTokenFeedEventByEventID,
	OnTokensAddedToCollectionFeedEvent:         resolveTokensAddedToCollectionFeedEventByEventID,
	OnUserCreatedFeedEvent:                     resolveUserCreatedFeedEventByEventID,
	OnUserFollowedUsersFeedEvent:               resolveUserFollowedUsersFeedEventByEventID,

	OnCollectionToken: func(ctx context.Context, tokenId string, collectionId string) (*model.CollectionToken, error) {
		return resolveCollectionTokenByIDs(ctx, persist.DBID(tokenId), persist.DBID(collectionId))
	},

	OnCommunity: func(ctx context.Context, contractAddress string, chain string) (*model.Community, error) {
		if parsed, err := strconv.Atoi(chain); err == nil {
			return resolveCommunityByContractAddress(ctx, persist.NewChainAddress(persist.Address(contractAddress), persist.Chain(parsed)), false)
		} else {
			return nil, err
		}
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
		mappedErr = model.ErrDoesNotOwnRequiredToken{Message: message}
	case persist.ErrUserNotFound:
		mappedErr = model.ErrUserNotFound{Message: message}
	case persist.ErrUserAlreadyExists:
		mappedErr = model.ErrUserAlreadyExists{Message: message}
	case persist.ErrCollectionNotFoundByID:
		mappedErr = model.ErrCollectionNotFound{Message: message}
	case persist.ErrTokenNotFoundByID:
		mappedErr = model.ErrTokenNotFound{Message: message}
	case persist.ErrCommunityNotFound:
		mappedErr = model.ErrCommunityNotFound{Message: message}
	case persist.ErrAddressOwnedByUser:
		mappedErr = model.ErrAddressOwnedByUser{Message: message}
	case publicapi.ErrTokenRefreshFailed:
		mappedErr = model.ErrOpenSeaRefreshFailed{Message: message}
	case publicapi.ErrInvalidInput:
		validationErr, _ := err.(publicapi.ErrInvalidInput)
		mappedErr = model.ErrInvalidInput{Message: message, Parameters: validationErr.Parameters, Reasons: validationErr.Reasons}
	case persist.ErrFeedEventNotFoundByID:
		mappedErr = model.ErrFeedEventNotFoundByID{Message: message}
	case persist.ErrUnknownAction:
		mappedErr = model.ErrUnknownAction{Message: message}
	}

	if mappedErr != nil {
		if converted, ok := model.ConvertToModelType(mappedErr, gqlTypeName); ok {
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
			return debugtools.NewDebugAuthenticator(userID, chainAddressPointersToChainAddresses(m.Debug.ChainAddresses)), nil
		}
	}

	if m.Eoa != nil && m.Eoa.ChainAddress != nil {
		return authApi.NewNonceAuthenticator(*m.Eoa.ChainAddress, m.Eoa.Nonce, m.Eoa.Signature, persist.WalletTypeEOA), nil
	}

	if m.GnosisSafe != nil {
		// GnosisSafe passes an empty signature
		return authApi.NewNonceAuthenticator(persist.NewChainAddress(m.GnosisSafe.Address, persist.ChainETH), m.Eoa.Nonce, "0x", persist.WalletTypeGnosis), nil
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

func resolveFollowersByUserID(ctx context.Context, userID persist.DBID) ([]*model.GalleryUser, error) {
	followers, err := publicapi.For(ctx).User.GetFollowersByUserId(ctx, userID)

	if err != nil {
		return nil, err
	}

	var output = make([]*model.GalleryUser, len(followers))
	for i, user := range followers {
		output[i] = userToModel(ctx, user)
	}

	return output, nil
}

func resolveFollowingByUserID(ctx context.Context, userID persist.DBID) ([]*model.GalleryUser, error) {
	following, err := publicapi.For(ctx).User.GetFollowingByUserId(ctx, userID)

	if err != nil {
		return nil, err
	}

	var output = make([]*model.GalleryUser, len(following))
	for i, user := range following {
		output[i] = userToModel(ctx, user)
	}

	return output, nil
}

func resolveGalleryUserByUsername(ctx context.Context, username string) (*model.GalleryUser, error) {
	user, err := publicapi.For(ctx).User.GetUserByUsername(ctx, username)

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

func resolveCollectionTokenByIDs(ctx context.Context, tokenID persist.DBID, collectionID persist.DBID) (*model.CollectionToken, error) {
	token, err := resolveTokenByTokenID(ctx, tokenID)
	if err != nil {
		return nil, err
	}

	collection, err := resolveCollectionByCollectionID(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	collectionToken := &model.CollectionToken{
		HelperCollectionTokenData: model.HelperCollectionTokenData{
			TokenId:      tokenID,
			CollectionId: collectionID,
		},
		Token:      token,
		Collection: collection,
	}

	return collectionToken, nil
}

func resolveGalleryByGalleryID(ctx context.Context, galleryID persist.DBID) (*model.Gallery, error) {
	gallery := &model.Gallery{
		Dbid:        galleryID,
		Owner:       nil, // handled by dedicated resolver
		Collections: nil, // handled by dedicated resolver
	}

	return gallery, nil
}

func resolveTokenByTokenID(ctx context.Context, tokenID persist.DBID) (*model.Token, error) {
	token, err := publicapi.For(ctx).Token.GetTokenById(ctx, tokenID)

	if err != nil {
		return nil, err
	}

	return tokenToModel(ctx, *token), nil
}

func resolveTokensByWalletID(ctx context.Context, walletID persist.DBID) ([]*model.Token, error) {
	tokens, err := publicapi.For(ctx).Token.GetTokensByWalletID(ctx, walletID)

	if err != nil {
		return nil, err
	}

	return tokensToModel(ctx, tokens), nil
}

func resolveTokensByUserID(ctx context.Context, userID persist.DBID) ([]*model.Token, error) {
	tokens, err := publicapi.For(ctx).Token.GetTokensByUserID(ctx, userID)

	if err != nil {
		return nil, err
	}

	return tokensToModel(ctx, tokens), nil
}

func resolveTokenOwnerByTokenID(ctx context.Context, tokenID persist.DBID) (*model.GalleryUser, error) {
	token, err := publicapi.For(ctx).Token.GetTokenById(ctx, tokenID)

	if err != nil {
		return nil, err
	}

	return resolveGalleryUserByUserID(ctx, token.OwnerUserID)
}

func resolveContractByTokenID(ctx context.Context, tokenID persist.DBID) (*model.Contract, error) {
	token, err := publicapi.For(ctx).Token.GetTokenById(ctx, tokenID)

	if err != nil {
		return nil, err
	}

	return resolveContractByContractID(ctx, token.Contract)
}

func resolveContractByContractID(ctx context.Context, contractID persist.DBID) (*model.Contract, error) {
	contract, err := publicapi.For(ctx).Contract.GetContractByID(ctx, contractID)

	if err != nil {
		return nil, err
	}

	return contractToModel(ctx, *contract), nil
}

func resolveWalletByWalletID(ctx context.Context, walletID persist.DBID) (*model.Wallet, error) {
	wallet, err := publicapi.For(ctx).Wallet.GetWalletByID(ctx, walletID)
	if err != nil {
		return nil, err
	}

	return walletToModelSqlc(ctx, *wallet), nil
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

func resolveCommunityByContractAddress(ctx context.Context, contractAddress persist.ChainAddress, forceRefresh bool) (*model.Community, error) {
	community, err := publicapi.For(ctx).User.GetCommunityByContractAddress(ctx, contractAddress, forceRefresh)

	if err != nil {
		return nil, err
	}

	return communityToModel(ctx, *community), nil
}

func resolveGeneralAllowlist(ctx context.Context) ([]*persist.ChainAddress, error) {
	addresses, err := publicapi.For(ctx).Misc.GetGeneralAllowlist(ctx)

	if err != nil {
		return nil, err
	}

	output := make([]*persist.ChainAddress, 0, len(addresses))

	for _, address := range addresses {
		chainAddress := persist.NewChainAddress(persist.Address(address), persist.ChainETH)
		output = append(output, &chainAddress)
	}

	return output, nil
}

func resolveWalletsByUserID(ctx context.Context, userID persist.DBID) ([]*model.Wallet, error) {
	wallets, err := publicapi.For(ctx).Wallet.GetWalletsByUserID(ctx, userID)

	if err != nil {
		return nil, err
	}

	output := make([]*model.Wallet, 0, len(wallets))

	for _, wallet := range wallets {
		output = append(output, walletToModelSqlc(ctx, wallet))
	}

	return output, nil
}

func resolveCollectionCreatedFeedEventByEventID(ctx context.Context, eventID persist.DBID) (*model.CollectionCreatedFeedEvent, error) {
	evt, err := publicapi.For(ctx).Feed.GetEventById(ctx, eventID)

	if err != nil {
		return nil, err
	}

	event := eventToCollectionCreatedFeedEventModel(evt)

	return &event, err
}

func resolveCollectorsNoteAddedToCollectionFeedEventByEventID(ctx context.Context, eventID persist.DBID) (*model.CollectorsNoteAddedToCollectionFeedEvent, error) {
	evt, err := publicapi.For(ctx).Feed.GetEventById(ctx, eventID)

	if err != nil {
		return nil, err
	}

	event := eventToCollectorsNoteAddedToCollectionFeedEventModel(evt)

	return &event, err
}

func resolveCollectorsNoteAddedToTokenFeedEventByEventID(ctx context.Context, eventID persist.DBID) (*model.CollectorsNoteAddedToTokenFeedEvent, error) {
	evt, err := publicapi.For(ctx).Feed.GetEventById(ctx, eventID)

	if err != nil {
		return nil, err
	}

	event := eventToCollectorsNoteAddedToTokenFeedEventModel(evt)

	return &event, err
}

func resolveTokensAddedToCollectionFeedEventByEventID(ctx context.Context, eventID persist.DBID) (*model.TokensAddedToCollectionFeedEvent, error) {
	evt, err := publicapi.For(ctx).Feed.GetEventById(ctx, eventID)

	if err != nil {
		return nil, err
	}

	event := eventToTokensAddedToCollectionFeedEventModel(evt)

	return &event, err
}

func resolveUserCreatedFeedEventByEventID(ctx context.Context, eventID persist.DBID) (*model.UserCreatedFeedEvent, error) {
	evt, err := publicapi.For(ctx).Feed.GetEventById(ctx, eventID)

	if err != nil {
		return nil, err
	}

	event := eventToUserCreatedFeedEventModel(evt)

	return &event, err
}

func resolveUserFollowedUsersFeedEventByEventID(ctx context.Context, eventID persist.DBID) (*model.UserFollowedUsersFeedEvent, error) {
	evt, err := publicapi.For(ctx).Feed.GetEventById(ctx, eventID)

	if err != nil {
		return nil, err
	}

	event := eventToUserFollowedUsersFeedEventModel(evt)

	return &event, err
}

func resolveViewerFeed(ctx context.Context, before *string, after *string, first *int, last *int) (*model.FeedConnection, error) {
	beforeToken, err := cursor.DecodeToDBID(before)
	if err != nil {
		return nil, err
	}

	afterToken, err := cursor.DecodeToDBID(after)
	if err != nil {
		return nil, err
	}

	events, err := publicapi.For(ctx).Feed.ViewerFeed(ctx, beforeToken, afterToken, first, last)

	if err != nil {
		return nil, err
	}

	return eventsToFeed(events, first, last)
}

func resolveGlobalFeed(ctx context.Context, before *string, after *string, first *int, last *int) (*model.FeedConnection, error) {
	beforeToken, err := cursor.DecodeToDBID(before)
	if err != nil {
		return nil, err
	}

	afterToken, err := cursor.DecodeToDBID(after)
	if err != nil {
		return nil, err
	}

	events, err := publicapi.For(ctx).Feed.GlobalFeed(ctx, beforeToken, afterToken, first, last)

	if err != nil {
		return nil, err
	}

	return eventsToFeed(events, first, last)
}

func resolveNewTokensByEventID(ctx context.Context, eventID persist.DBID) ([]*model.CollectionToken, error) {
	event, err := publicapi.For(ctx).Feed.GetEventById(ctx, eventID)

	if err != nil {
		return nil, err
	}

	tokens, err := publicapi.For(ctx).Token.GetNewTokensByFeedEventID(ctx, eventID)

	if err != nil {
		return nil, err
	}

	newTokens := make([]*model.CollectionToken, len(tokens))

	for i, tkn := range tokens {
		token, err := resolveCollectionTokenByIDs(ctx, tkn.ID, event.Data.CollectionID)

		if err != nil {
			return nil, err
		}

		newTokens[i] = token
	}

	return newTokens, nil
}

func feedEventToEdge(event *sqlc.FeedEvent) (*model.FeedEdge, error) {
	switch event.Action {
	case persist.ActionUserCreated:
		node := eventToUserCreatedFeedEventModel(event)
		return &model.FeedEdge{Node: node, Cursor: cursor.DBIDEncodeToCursor(node.Dbid)}, nil
	case persist.ActionUserFollowedUsers:
		node := eventToUserFollowedUsersFeedEventModel(event)
		return &model.FeedEdge{Node: node, Cursor: cursor.DBIDEncodeToCursor(node.Dbid)}, nil
	case persist.ActionCollectorsNoteAddedToToken:
		node := eventToCollectorsNoteAddedToTokenFeedEventModel(event)
		return &model.FeedEdge{Node: node, Cursor: cursor.DBIDEncodeToCursor(node.Dbid)}, nil
	case persist.ActionCollectionCreated:
		node := eventToCollectionCreatedFeedEventModel(event)
		return &model.FeedEdge{Node: node, Cursor: cursor.DBIDEncodeToCursor(node.Dbid)}, nil
	case persist.ActionCollectorsNoteAddedToCollection:
		node := eventToCollectionCreatedFeedEventModel(event)
		return &model.FeedEdge{Node: node, Cursor: cursor.DBIDEncodeToCursor(node.Dbid)}, nil
	case persist.ActionTokensAddedToCollection:
		node := eventToTokensAddedToCollectionFeedEventModel(event)
		return &model.FeedEdge{Node: node, Cursor: cursor.DBIDEncodeToCursor(node.Dbid)}, nil
	default:
		return nil, persist.ErrUnknownAction{Action: event.Action}
	}
}

func eventToUserCreatedFeedEventModel(event *sqlc.FeedEvent) model.UserCreatedFeedEvent {
	return model.UserCreatedFeedEvent{
		Dbid:      event.ID,
		EventTime: event.EventTime,
		Owner:     &model.GalleryUser{Dbid: event.OwnerID}, // remaining fields handled by dedicated resolver
		Action:    event.Action,
	}
}

func eventToUserFollowedUsersFeedEventModel(event *sqlc.FeedEvent) model.UserFollowedUsersFeedEvent {
	followed := make([]*model.FollowInfo, len(event.Data.UserFollowedIDs))

	for i, userID := range event.Data.UserFollowedIDs {
		followed[i] = &model.FollowInfo{
			User:         &model.GalleryUser{Dbid: userID}, // remaining fields handled by dedicated resolver
			FollowedBack: util.BoolToPointer(event.Data.UserFollowedBack[i]),
		}
	}

	return model.UserFollowedUsersFeedEvent{
		Dbid:      event.ID,
		EventTime: event.EventTime,
		Owner:     &model.GalleryUser{Dbid: event.OwnerID}, // remaining fields handled by dedicated resolver
		Action:    event.Action,
		Followed:  followed,
	}
}

func eventToCollectorsNoteAddedToTokenFeedEventModel(event *sqlc.FeedEvent) model.CollectorsNoteAddedToTokenFeedEvent {
	return model.CollectorsNoteAddedToTokenFeedEvent{
		Dbid:      event.ID,
		EventTime: event.EventTime,
		Owner:     &model.GalleryUser{Dbid: event.OwnerID}, // remaining fields handled by dedicated resolver
		Token: &model.CollectionToken{
			Token:      &model.Token{Dbid: event.Data.TokenID},                // remaining fields handled by dedicated resolver
			Collection: &model.Collection{Dbid: event.Data.TokenCollectionID}, // remaining fields handled by dedicated resolver
		},
		Action:            event.Action,
		NewCollectorsNote: util.StringToPointer(event.Data.TokenNewCollectorsNote),
	}
}

func eventToCollectionCreatedFeedEventModel(event *sqlc.FeedEvent) model.CollectionCreatedFeedEvent {
	return model.CollectionCreatedFeedEvent{
		Dbid:       event.ID,
		EventTime:  event.EventTime,
		Owner:      &model.GalleryUser{Dbid: event.OwnerID},          // remaining fields handled by dedicated resolver
		Collection: &model.Collection{Dbid: event.Data.CollectionID}, // remaining fields handled by dedicated resolver
		Action:     event.Action,
	}
}

func eventToCollectorsNoteAddedToCollectionFeedEventModel(event *sqlc.FeedEvent) model.CollectorsNoteAddedToCollectionFeedEvent {
	return model.CollectorsNoteAddedToCollectionFeedEvent{
		Dbid:              event.ID,
		EventTime:         event.EventTime,
		Owner:             &model.GalleryUser{Dbid: event.OwnerID},          // remaining fields handled by dedicated resolver
		Collection:        &model.Collection{Dbid: event.Data.CollectionID}, // remaining fields handled by dedicated resolver
		Action:            event.Action,
		NewCollectorsNote: util.StringToPointer(event.Data.CollectionNewCollectorsNote),
	}
}

func eventToTokensAddedToCollectionFeedEventModel(event *sqlc.FeedEvent) model.TokensAddedToCollectionFeedEvent {
	return model.TokensAddedToCollectionFeedEvent{
		Dbid:       event.ID,
		EventTime:  event.EventTime,
		Owner:      &model.GalleryUser{Dbid: event.OwnerID},          // remaining fields handled by dedicated resolver
		Collection: &model.Collection{Dbid: event.Data.CollectionID}, // remaining fields handled by dedicated resolver
		Action:     event.Action,
		NewTokens:  nil, // handled by dedicated resolver
	}
}

func eventsToFeed(events []sqlc.FeedEvent, first *int, last *int) (*model.FeedConnection, error) {
	evts := make([]*model.FeedEdge, len(events))

	var pageInfo model.PageInfo

	if first != nil && len(events) > *first {
		events = events[:*first]
		pageInfo.HasNextPage = true
	}

	if last != nil && len(events) > *last {
		events = events[len(events)-*last:]
		pageInfo.HasPreviousPage = true
	}

	for i, e := range events {
		evt, err := feedEventToEdge(&e)
		if err != nil {
			return nil, err
		}
		evts[i] = evt
	}

	if len(events) > 0 {
		pageInfo.StartCursor = evts[0].Cursor
		pageInfo.EndCursor = evts[len(evts)-1].Cursor
	}

	return &model.FeedConnection{Edges: evts, PageInfo: &pageInfo}, nil
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

	wallets := make([]*model.Wallet, len(user.Wallets))
	for i, wallet := range user.Wallets {
		wallets[i] = walletToModelPersist(ctx, wallet)
	}

	return &model.GalleryUser{
		Dbid:     user.ID,
		Username: &user.Username.String,
		Bio:      &user.Bio.String,
		Wallets:  wallets,

		// each handled by dedicated resolver
		Galleries: nil,
		Followers: nil,
		Following: nil,

		IsAuthenticatedUser: &isAuthenticatedUser,
	}
}

func walletToModelPersist(ctx context.Context, wallet persist.Wallet) *model.Wallet {
	chainAddress := persist.NewChainAddress(wallet.Address, wallet.Chain)

	return &model.Wallet{
		Dbid:         wallet.ID,
		WalletType:   &wallet.WalletType,
		ChainAddress: &chainAddress,
		Chain:        &wallet.Chain,
		Tokens:       nil, // handled by dedicated resolver
	}
}

func walletToModelSqlc(ctx context.Context, wallet sqlc.Wallet) *model.Wallet {
	chain := persist.Chain(wallet.Chain.Int32)
	chainAddress := persist.NewChainAddress(wallet.Address, chain)

	return &model.Wallet{
		Dbid:         wallet.ID,
		WalletType:   &wallet.WalletType,
		ChainAddress: &chainAddress,
		Chain:        &chain,
		Tokens:       nil, // handled by dedicated resolver
	}
}

func contractToModel(ctx context.Context, contract sqlc.Contract) *model.Contract {
	chain := persist.Chain(contract.Chain.Int32)
	addr := persist.NewChainAddress(contract.Address, chain)
	creator := persist.NewChainAddress(contract.CreatorAddress, chain)

	return &model.Contract{
		Dbid:            contract.ID,
		ContractAddress: &addr,
		CreatorAddress:  &creator,
		Chain:           &chain,
		Name:            &contract.Name.String,
		LastUpdated:     &contract.LastUpdated,
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
		Tokens:         nil, // handled by dedicated resolver
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
	previewTokens := make([]*string, len(tokenHolder.PreviewTokens))
	for i, token := range tokenHolder.PreviewTokens {
		previewTokens[i] = util.StringToPointer(token.String())
	}

	return &model.TokenHolder{
		HelperTokenHolderData: model.HelperTokenHolderData{UserId: tokenHolder.UserID, WalletIds: tokenHolder.WalletIDs},
		User:                  nil, // handled by dedicated resolver
		Wallets:               nil, // handled by dedicated resolver
		PreviewTokens:         previewTokens,
	}
}

func tokenToModel(ctx context.Context, token sqlc.Token) *model.Token {
	chain := persist.Chain(token.Chain.Int32)
	metadata, _ := token.TokenMetadata.MarshalJSON()
	metadataString := string(metadata)
	blockNumber := fmt.Sprint(token.BlockNumber.Int64)
	tokenType := model.TokenType(token.TokenType.String)
	return &model.Token{
		Dbid:             token.ID,
		CreationTime:     &token.CreatedAt,
		LastUpdated:      &token.LastUpdated,
		CollectorsNote:   &token.CollectorsNote.String,
		Media:            getMediaForToken(ctx, token),
		TokenType:        &tokenType,
		Chain:            &chain,
		Name:             &token.Name.String,
		Description:      &token.Description.String,
		OwnedByWallets:   nil, // handled by dedicated resolver
		TokenURI:         &token.TokenUri.String,
		TokenID:          &token.TokenID.String,
		Quantity:         &token.Quantity.String,
		Owner:            nil, // handled by dedicated resolver
		OwnershipHistory: nil, // TODO: later
		TokenMetadata:    &metadataString,
		Contract:         nil, // handled by dedicated resolver
		ExternalURL:      &token.ExternalUrl.String,
		BlockNumber:      &blockNumber, // TODO: later

		// These are legacy mappings that will likely end up elsewhere when we pull data from the indexer
		OpenseaCollectionName: nil, // TODO: later
	}
}

func tokensToModel(ctx context.Context, token []sqlc.Token) []*model.Token {
	res := make([]*model.Token, len(token))
	for i, token := range token {
		res[i] = tokenToModel(ctx, token)
	}
	return res
}

func communityToModel(ctx context.Context, community persist.Community) *model.Community {
	lastUpdated := community.LastUpdated.Time()
	contractAddress := persist.NewChainAddress(community.ContractAddress, community.Chain)
	creatorAddress := persist.NewChainAddress(community.CreatorAddress, community.Chain)

	owners := make([]*model.TokenHolder, len(community.Owners))
	for i, owner := range community.Owners {
		owners[i] = tokenHolderToModel(ctx, owner)
	}

	return &model.Community{
		LastUpdated:     &lastUpdated,
		ContractAddress: &contractAddress,
		CreatorAddress:  &creatorAddress,
		Name:            util.StringToPointer(community.Name.String()),
		Description:     util.StringToPointer(community.Description.String()),
		PreviewImage:    util.StringToPointer(community.PreviewImage.String()),
		Owners:          owners,
	}
}

func getUrlExtension(url string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(url), "."))
}

func getMediaForToken(ctx context.Context, token sqlc.Token) model.MediaSubtype {
	var med persist.Media
	err := token.Media.AssignTo(&med)
	if err != nil {
		return getInvalidMedia(ctx, med)
	}

	switch med.MediaType {
	case persist.MediaTypeImage, persist.MediaTypeGIF:
		return getImageMedia(ctx, med)
	case persist.MediaTypeVideo:
		return getVideoMedia(ctx, med)
	case persist.MediaTypeAudio:
		return getAudioMedia(ctx, med)
	case persist.MediaTypeHTML:
		return getHtmlMedia(ctx, med)
	case persist.MediaTypeAnimation:
		return getGltfMedia(ctx, med)
	case persist.MediaTypeJSON, persist.MediaTypeBase64JSON:
		return getJsonMedia(ctx, med)
	case persist.MediaTypeSVG, persist.MediaTypeText, persist.MediaTypeBase64SVG, persist.MediaTypeBase64Text:
		return getTextMedia(ctx, med)
	default:
		return getUnknownMedia(ctx, med)
	}

}

func getPreviewUrls(ctx context.Context, media persist.Media) *model.PreviewURLSet {
	preview := remapLargeImageUrls(media.ThumbnailURL.String())
	mm := mediamapper.For(ctx)

	return &model.PreviewURLSet{
		Raw:       &preview,
		Thumbnail: util.StringToPointer(mm.GetThumbnailImageUrl(preview)),
		Small:     util.StringToPointer(mm.GetSmallImageUrl(preview)),
		Medium:    util.StringToPointer(mm.GetMediumImageUrl(preview)),
		Large:     util.StringToPointer(mm.GetLargeImageUrl(preview)),
		SrcSet:    util.StringToPointer(mm.GetSrcSet(preview)),
	}
}

func getImageMedia(ctx context.Context, media persist.Media) model.ImageMedia {
	url := remapLargeImageUrls(media.MediaURL.String())

	return model.ImageMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.StringToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: &url,
	}
}

// Temporary method for handling the large "dead ringers" NFT image. This remapping
// step should actually happen as part of generating resized images with imgix.
func remapLargeImageUrls(url string) string {
	if url == "https://storage.opensea.io/files/33ab86c2a565430af5e7fb8399876960.png" || url == "https://openseauserdata.com/files/33ab86c2a565430af5e7fb8399876960.png" {
		return "https://lh3.googleusercontent.com/pw/AM-JKLVsudnwN97ULF-DgJC1J_AZ8i-1pMjLCVUqswF1_WShId30uP_p_jSRkmVx-XNgKNIGFSglgRojZQrsLOoCM2pVNJwgx5_E4yeYRsMvDQALFKbJk0_6wj64tjLhSIINwGpdNw0MhtWNehKCipDKNeE"
	}

	return url
}

func getVideoMedia(ctx context.Context, media persist.Media) model.VideoMedia {
	asString := media.MediaURL.String()
	videoUrls := model.VideoURLSet{
		Raw:    &asString,
		Small:  &asString,
		Medium: &asString,
		Large:  &asString,
	}

	return model.VideoMedia{
		PreviewURLs:       getPreviewUrls(ctx, media),
		MediaURL:          util.StringToPointer(media.MediaURL.String()),
		MediaType:         (*string)(&media.MediaType),
		ContentRenderURLs: &videoUrls,
	}
}

func getAudioMedia(ctx context.Context, media persist.Media) model.AudioMedia {
	return model.AudioMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.StringToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getTextMedia(ctx context.Context, media persist.Media) model.TextMedia {
	return model.TextMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.StringToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getHtmlMedia(ctx context.Context, media persist.Media) model.HTMLMedia {
	return model.HTMLMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.StringToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getJsonMedia(ctx context.Context, media persist.Media) model.JSONMedia {
	return model.JSONMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.StringToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getGltfMedia(ctx context.Context, media persist.Media) model.GltfMedia {
	return model.GltfMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.StringToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getUnknownMedia(ctx context.Context, media persist.Media) model.UnknownMedia {
	return model.UnknownMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.StringToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func getInvalidMedia(ctx context.Context, media persist.Media) model.InvalidMedia {
	return model.InvalidMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.StringToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
	}
}

func chainAddressPointersToChainAddresses(chainAddresses []*persist.ChainAddress) []persist.ChainAddress {
	addresses := make([]persist.ChainAddress, 0, len(chainAddresses))

	for _, address := range chainAddresses {
		if address != nil {
			addresses = append(addresses, *address)
		}
	}

	return addresses
}
