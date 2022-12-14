package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	emailService "github.com/mikeydub/go-gallery/emails"
	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/emails"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
)

func (r *admireResolver) Admirer(ctx context.Context, obj *model.Admire) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Admirer.Dbid)
}

func (r *admireFeedEventPayloadResolver) Admire(ctx context.Context, obj *model.AdmireFeedEventPayload) (*model.Admire, error) {
	return resolveAdmireByAdmireID(ctx, obj.Admire.Dbid)
}

func (r *admireFeedEventPayloadResolver) FeedEvent(ctx context.Context, obj *model.AdmireFeedEventPayload) (*model.FeedEvent, error) {
	admire, err := publicapi.For(ctx).Interaction.GetAdmireByID(ctx, obj.Admire.Dbid)
	if err != nil {
		return nil, err
	}
	return resolveFeedEventByEventID(ctx, admire.FeedEventID)
}

func (r *collectionResolver) Gallery(ctx context.Context, obj *model.Collection) (*model.Gallery, error) {
	gallery, err := publicapi.For(ctx).Gallery.GetGalleryByCollectionId(ctx, obj.Dbid)

	if err != nil {
		return nil, err
	}

	return galleryToModel(ctx, *gallery), nil
}

func (r *collectionResolver) Tokens(ctx context.Context, obj *model.Collection, limit *int) ([]*model.CollectionToken, error) {
	tokens, err := publicapi.For(ctx).Token.GetTokensByCollectionId(ctx, obj.Dbid, limit)

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
			Token:         tokenToModel(ctx, token),
			Collection:    obj,
			TokenSettings: nil, // handled by dedicated resolver
		}
	}

	return output, nil
}

func (r *collectionCreatedFeedEventDataResolver) Owner(ctx context.Context, obj *model.CollectionCreatedFeedEventData) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Owner.Dbid)
}

func (r *collectionCreatedFeedEventDataResolver) Collection(ctx context.Context, obj *model.CollectionCreatedFeedEventData) (*model.Collection, error) {
	return resolveCollectionByCollectionID(ctx, obj.Collection.Dbid)
}

func (r *collectionCreatedFeedEventDataResolver) NewTokens(ctx context.Context, obj *model.CollectionCreatedFeedEventData) ([]*model.CollectionToken, error) {
	return resolveNewTokensByEventID(ctx, obj.FeedEventID)
}

func (r *collectionTokenResolver) TokenSettings(ctx context.Context, obj *model.CollectionToken) (*model.CollectionTokenSettings, error) {
	return resolveTokenSettingsByIDs(ctx, obj.TokenId, obj.CollectionId)
}

func (r *collectionUpdatedFeedEventDataResolver) Owner(ctx context.Context, obj *model.CollectionUpdatedFeedEventData) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Owner.Dbid)
}

func (r *collectionUpdatedFeedEventDataResolver) Collection(ctx context.Context, obj *model.CollectionUpdatedFeedEventData) (*model.Collection, error) {
	return resolveCollectionByCollectionID(ctx, obj.Collection.Dbid)
}

func (r *collectionUpdatedFeedEventDataResolver) NewTokens(ctx context.Context, obj *model.CollectionUpdatedFeedEventData) ([]*model.CollectionToken, error) {
	return resolveNewTokensByEventID(ctx, obj.FeedEventID)
}

func (r *collectorsNoteAddedToCollectionFeedEventDataResolver) Owner(ctx context.Context, obj *model.CollectorsNoteAddedToCollectionFeedEventData) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Owner.Dbid)
}

func (r *collectorsNoteAddedToCollectionFeedEventDataResolver) Collection(ctx context.Context, obj *model.CollectorsNoteAddedToCollectionFeedEventData) (*model.Collection, error) {
	return resolveCollectionByCollectionID(ctx, obj.Collection.Dbid)
}

func (r *collectorsNoteAddedToTokenFeedEventDataResolver) Owner(ctx context.Context, obj *model.CollectorsNoteAddedToTokenFeedEventData) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Owner.Dbid)
}

func (r *collectorsNoteAddedToTokenFeedEventDataResolver) Token(ctx context.Context, obj *model.CollectorsNoteAddedToTokenFeedEventData) (*model.CollectionToken, error) {
	return resolveCollectionTokenByIDs(ctx, obj.Token.Token.Dbid, obj.Token.Collection.Dbid)
}

func (r *commentResolver) ReplyTo(ctx context.Context, obj *model.Comment) (*model.Comment, error) {
	return resolveCommentByCommentID(ctx, obj.ReplyTo.Dbid)
}

func (r *commentResolver) Commenter(ctx context.Context, obj *model.Comment) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Commenter.Dbid)
}

func (r *commentOnFeedEventPayloadResolver) Comment(ctx context.Context, obj *model.CommentOnFeedEventPayload) (*model.Comment, error) {
	return resolveCommentByCommentID(ctx, obj.Comment.Dbid)
}

func (r *commentOnFeedEventPayloadResolver) ReplyToComment(ctx context.Context, obj *model.CommentOnFeedEventPayload) (*model.Comment, error) {
	return resolveCommentByCommentID(ctx, obj.ReplyToComment.Dbid)
}

func (r *commentOnFeedEventPayloadResolver) FeedEvent(ctx context.Context, obj *model.CommentOnFeedEventPayload) (*model.FeedEvent, error) {
	return resolveFeedEventByEventID(ctx, obj.FeedEvent.Dbid)
}

func (r *communityResolver) TokensInCommunity(ctx context.Context, obj *model.Community, before *string, after *string, first *int, last *int, onlyGalleryUsers *bool) (*model.TokensConnection, error) {
	if onlyGalleryUsers == nil || (onlyGalleryUsers != nil && !*onlyGalleryUsers) {
		refresh := false
		if obj.ForceRefresh != nil {
			refresh = *obj.ForceRefresh
		}
		err := refreshTokensInContractAsync(ctx, obj.Dbid, refresh)
		if err != nil {
			return nil, err
		}
	}
	return resolveTokensByContractIDWithPagination(ctx, obj.Dbid, before, after, first, last, onlyGalleryUsers)
}

func (r *communityResolver) Owners(ctx context.Context, obj *model.Community, before *string, after *string, first *int, last *int, onlyGalleryUsers *bool) (*model.TokenHoldersConnection, error) {
	if onlyGalleryUsers == nil || (onlyGalleryUsers != nil && !*onlyGalleryUsers) {
		refresh := false
		if obj.ForceRefresh != nil {
			refresh = *obj.ForceRefresh
		}
		err := refreshTokensInContractAsync(ctx, obj.Dbid, refresh)
		if err != nil {
			return nil, err
		}
	}

	return resolveCommunityOwnersByContractID(ctx, obj.Dbid, before, after, first, last, onlyGalleryUsers)
}

func (r *createCollectionPayloadResolver) FeedEvent(ctx context.Context, obj *model.CreateCollectionPayload) (*model.FeedEvent, error) {
	if obj.FeedEvent.Dbid == "" {
		return nil, nil
	}
	return resolveFeedEventByEventID(ctx, obj.FeedEvent.Dbid)
}

func (r *feedEventResolver) EventData(ctx context.Context, obj *model.FeedEvent) (model.FeedEventData, error) {
	return resolveFeedEventDataByEventID(ctx, obj.Dbid)
}

func (r *feedEventResolver) Admires(ctx context.Context, obj *model.FeedEvent, before *string, after *string, first *int, last *int) (*model.FeedEventAdmiresConnection, error) {
	admires, pageInfo, err := publicapi.For(ctx).Interaction.PaginateAdmiresByFeedEventID(ctx, obj.Dbid, before, after, first, last)
	if err != nil {
		return nil, err
	}

	var edges []*model.FeedEventAdmireEdge
	for _, admire := range admires {
		edges = append(edges, &model.FeedEventAdmireEdge{
			Node:  admireToModel(ctx, admire),
			Event: obj,
		})
	}

	return &model.FeedEventAdmiresConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil
}

func (r *feedEventResolver) Comments(ctx context.Context, obj *model.FeedEvent, before *string, after *string, first *int, last *int) (*model.FeedEventCommentsConnection, error) {
	comments, pageInfo, err := publicapi.For(ctx).Interaction.PaginateCommentsByFeedEventID(ctx, obj.Dbid, before, after, first, last)
	if err != nil {
		return nil, err
	}

	var edges []*model.FeedEventCommentEdge
	for _, comment := range comments {
		edges = append(edges, &model.FeedEventCommentEdge{
			Node:  commentToModel(ctx, comment),
			Event: obj,
		})
	}

	return &model.FeedEventCommentsConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil
}

func (r *feedEventResolver) Interactions(ctx context.Context, obj *model.FeedEvent, before *string, after *string, first *int, last *int, typeFilter []persist.InteractionType) (*model.FeedEventInteractionsConnection, error) {
	interactions, pageInfo, err := publicapi.For(ctx).Interaction.PaginateInteractionsByFeedEventID(ctx, obj.Dbid, before, after, first, last, typeFilter)
	if err != nil {
		return nil, err
	}

	var edges []*model.FeedEventInteractionsEdge
	for _, interaction := range interactions {
		edge := &model.FeedEventInteractionsEdge{
			Event: obj,
		}
		if admire, ok := interaction.(coredb.Admire); ok {
			edge.Node = admireToModel(ctx, admire)
		} else if comment, ok := interaction.(coredb.Comment); ok {
			edge.Node = commentToModel(ctx, comment)
		}
		edges = append(edges, edge)
	}

	return &model.FeedEventInteractionsConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil
}

func (r *feedEventResolver) ViewerAdmire(ctx context.Context, obj *model.FeedEvent) (*model.Admire, error) {
	api := publicapi.For(ctx)

	// If the user isn't logged in, there is no viewer
	if !api.User.IsUserLoggedIn(ctx) {
		return nil, nil
	}

	userID := api.User.GetLoggedInUserId(ctx)

	admire, err := api.Interaction.GetAdmireByActorIDAndFeedEventID(ctx, userID, obj.Dbid)
	if err != nil {
		// If getting the admire fails for any reason, just return nil. This resolver doesn't
		// return error types -- it just returns an admire (if it can find one) or nil.
		return nil, nil
	}

	return admireToModel(ctx, *admire), nil
}

func (r *feedEventResolver) HasViewerAdmiredEvent(ctx context.Context, obj *model.FeedEvent) (*bool, error) {
	// Trivial implementation for backward compatibility; this will be removed soon
	f := false
	return &f, nil
}

func (r *followInfoResolver) User(ctx context.Context, obj *model.FollowInfo) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.User.Dbid)
}

func (r *followUserPayloadResolver) User(ctx context.Context, obj *model.FollowUserPayload) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.User.Dbid)
}

func (r *galleryResolver) TokenPreviews(ctx context.Context, obj *model.Gallery) ([]*string, error) {
	return resolveTokenPreviewsByGalleryID(ctx, obj.Dbid)
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

func (r *galleryUserResolver) Roles(ctx context.Context, obj *model.GalleryUser) ([]*persist.Role, error) {
	dbRoles, err := publicapi.For(ctx).Admin.GetUserRolesByUserID(ctx, obj.Dbid)
	if err != nil {
		return nil, err
	}

	roles := make([]*persist.Role, len(dbRoles))
	for i, role := range dbRoles {
		r := role
		roles[i] = &r
	}

	return roles, nil
}

func (r *galleryUserResolver) Tokens(ctx context.Context, obj *model.GalleryUser) ([]*model.Token, error) {
	return resolveTokensByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) TokensByChain(ctx context.Context, obj *model.GalleryUser, chain persist.Chain) (*model.ChainTokens, error) {
	tokens, err := publicapi.For(ctx).Token.GetTokensByUserIDAndChain(ctx, obj.Dbid, chain)
	if err != nil {
		return nil, err
	}

	return &model.ChainTokens{
		Chain:  &chain,
		Tokens: tokensToModel(ctx, tokens),
	}, nil
}

func (r *galleryUserResolver) Wallets(ctx context.Context, obj *model.GalleryUser) ([]*model.Wallet, error) {
	return resolveWalletsByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) FeaturedGallery(ctx context.Context, obj *model.GalleryUser) (*model.Gallery, error) {
	if obj.HelperGalleryUserData.FeaturedGalleryID == nil {
		return nil, nil
	}

	return resolveGalleryByGalleryID(ctx, *obj.HelperGalleryUserData.FeaturedGalleryID)
}

func (r *galleryUserResolver) Galleries(ctx context.Context, obj *model.GalleryUser) ([]*model.Gallery, error) {
	return resolveGalleriesByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) Badges(ctx context.Context, obj *model.GalleryUser) ([]*model.Badge, error) {
	return resolveBadgesByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) Followers(ctx context.Context, obj *model.GalleryUser) ([]*model.GalleryUser, error) {
	return resolveFollowersByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) Following(ctx context.Context, obj *model.GalleryUser) ([]*model.GalleryUser, error) {
	return resolveFollowingByUserID(ctx, obj.Dbid)
}

func (r *galleryUserResolver) Feed(ctx context.Context, obj *model.GalleryUser, before *string, after *string, first *int, last *int) (*model.FeedConnection, error) {
	events, pageInfo, err := publicapi.For(ctx).Feed.PaginateUserFeed(ctx, obj.Dbid, before, after, first, last)
	if err != nil {
		return nil, err
	}

	edges, err := eventsToFeedEdges(events)
	if err != nil {
		return nil, err
	}

	return &model.FeedConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil
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

	sectionLayout := make([]persist.CollectionSectionLayout, len(input.Layout.SectionLayout))
	for i, layout := range input.Layout.SectionLayout {
		sectionLayout[i] = persist.CollectionSectionLayout{
			Columns:    persist.NullInt32(layout.Columns),
			Whitespace: layout.Whitespace,
		}
	}

	layout := persist.TokenLayout{
		Sections:      persist.StandardizeCollectionSections(input.Layout.Sections),
		SectionLayout: sectionLayout,
	}

	settings := make(map[persist.DBID]persist.CollectionTokenSettings)
	for _, tokenSetting := range input.TokenSettings {
		settings[tokenSetting.TokenID] = persist.CollectionTokenSettings{RenderLive: tokenSetting.RenderLive}
	}

	collection, feedEvent, err := api.Collection.CreateCollection(ctx, input.GalleryID, input.Name, input.CollectorsNote, input.Tokens, layout, settings, input.Caption)
	if err != nil {
		return nil, err
	}

	var eventModel model.FeedEvent
	if feedEvent != nil {
		eventModel = model.FeedEvent{Dbid: feedEvent.ID}
	}

	output := model.CreateCollectionPayload{
		Collection: collectionToModel(ctx, *collection),
		FeedEvent:  &eventModel,
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

	sectionLayout := make([]persist.CollectionSectionLayout, len(input.Layout.SectionLayout))
	for i, layout := range input.Layout.SectionLayout {
		sectionLayout[i] = persist.CollectionSectionLayout{
			Columns:    persist.NullInt32(layout.Columns),
			Whitespace: layout.Whitespace,
		}
	}

	layout := persist.TokenLayout{
		Sections:      persist.StandardizeCollectionSections(input.Layout.Sections),
		SectionLayout: sectionLayout,
	}

	settings := make(map[persist.DBID]persist.CollectionTokenSettings)
	for _, tokenSetting := range input.TokenSettings {
		settings[tokenSetting.TokenID] = persist.CollectionTokenSettings{RenderLive: tokenSetting.RenderLive}
	}

	feedEvent, err := api.Collection.UpdateCollectionTokens(ctx, input.CollectionID, input.Tokens, layout, settings, input.Caption)
	if err != nil {
		return nil, err
	}

	collection, err := api.Collection.GetCollectionById(ctx, input.CollectionID)
	if err != nil {
		return nil, err
	}

	var eventModel model.FeedEvent
	if feedEvent != nil {
		eventModel = model.FeedEvent{Dbid: feedEvent.ID}
	}

	output := &model.UpdateCollectionTokensPayload{
		Collection: collectionToModel(ctx, *collection),
		FeedEvent:  &eventModel,
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

func (r *mutationResolver) SetSpamPreference(ctx context.Context, input model.SetSpamPreferenceInput) (model.SetSpamPreferencePayloadOrError, error) {
	err := publicapi.For(ctx).Token.SetSpamPreference(ctx, input.Tokens, input.IsSpam)
	if err != nil {
		return nil, err
	}

	tokens := make([]*model.Token, len(input.Tokens))
	for i, tokenID := range input.Tokens {
		tokens[i] = &model.Token{Dbid: tokenID} // Remaining fields handled by dedicated resolver
	}

	return model.SetSpamPreferencePayload{Tokens: tokens}, nil
}

func (r *mutationResolver) SyncTokens(ctx context.Context, chains []persist.Chain, userID *persist.DBID) (model.SyncTokensPayloadOrError, error) {
	api := publicapi.For(ctx)

	if chains == nil || len(chains) == 0 {
		chains = []persist.Chain{persist.ChainETH}
	}
	err := api.Token.SyncTokens(ctx, chains, userID)
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

func (r *mutationResolver) RefreshCollection(ctx context.Context, collectionID persist.DBID) (model.RefreshCollectionPayloadOrError, error) {
	api := publicapi.For(ctx)

	err := api.Token.RefreshCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	collection, err := resolveCollectionByCollectionID(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	output := &model.RefreshCollectionPayload{
		Collection: collection,
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

func (r *mutationResolver) DeepRefresh(ctx context.Context, input model.DeepRefreshInput) (model.DeepRefreshPayloadOrError, error) {
	err := publicapi.For(ctx).Token.DeepRefreshByChain(ctx, input.Chain)
	if err != nil {
		return nil, err
	}
	return model.DeepRefreshPayload{
		Chain:     &input.Chain,
		Submitted: util.BoolToPointer(true),
	}, nil
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

func (r *mutationResolver) CreateUser(ctx context.Context, authMechanism model.AuthMechanism, input model.CreateUserInput) (model.CreateUserPayloadOrError, error) {
	authenticator, err := r.authMechanismToAuthenticator(ctx, authMechanism)
	if err != nil {
		return nil, err
	}

	bioStr := ""
	if input.Bio != nil {
		bioStr = *input.Bio
	}

	var email *persist.Email
	if input.Email != nil {
		it := persist.Email(*input.Email)
		email = &it
	}

	userID, galleryID, err := publicapi.For(ctx).User.CreateUser(ctx, authenticator, input.Username, email, bioStr, input.GalleryName, input.GalleryDescription, input.GalleryPosition)
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

func (r *mutationResolver) UpdateEmail(ctx context.Context, input model.UpdateEmailInput) (model.UpdateEmailPayloadOrError, error) {
	return updateUserEmail(ctx, input.Email)
}

func (r *mutationResolver) ResendVerificationEmail(ctx context.Context) (model.ResendVerificationEmailPayloadOrError, error) {
	return resendEmailVerification(ctx)
}

func (r *mutationResolver) UpdateEmailNotificationSettings(ctx context.Context, input model.UpdateEmailNotificationSettingsInput) (model.UpdateEmailNotificationSettingsPayloadOrError, error) {
	return updateUserEmailNotificationSettings(ctx, input)
}

func (r *mutationResolver) UnsubscribeFromEmailType(ctx context.Context, input model.UnsubscribeFromEmailTypeInput) (model.UnsubscribeFromEmailTypePayloadOrError, error) {
	return unsubscribeFromEmailType(ctx, input)
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

func (r *mutationResolver) AdmireFeedEvent(ctx context.Context, feedEventID persist.DBID) (model.AdmireFeedEventPayloadOrError, error) {
	id, err := publicapi.For(ctx).Interaction.AdmireFeedEvent(ctx, feedEventID)
	if err != nil {
		return nil, err
	}
	output := &model.AdmireFeedEventPayload{
		Viewer:    resolveViewer(ctx),
		Admire:    &model.Admire{Dbid: id},
		FeedEvent: &model.FeedEvent{Dbid: feedEventID},
	}
	return output, nil
}

func (r *mutationResolver) RemoveAdmire(ctx context.Context, admireID persist.DBID) (model.RemoveAdmirePayloadOrError, error) {
	feedEventID, err := publicapi.For(ctx).Interaction.RemoveAdmire(ctx, admireID)
	if err != nil {
		return nil, err
	}

	output := &model.RemoveAdmirePayload{
		Viewer:   resolveViewer(ctx),
		AdmireID: &admireID,
		FeedEvent: &model.FeedEvent{
			Dbid: feedEventID, // remaining fields handled by dedicated resolver
		},
	}
	return output, nil
}

func (r *mutationResolver) CommentOnFeedEvent(ctx context.Context, feedEventID persist.DBID, replyToID *persist.DBID, comment string) (model.CommentOnFeedEventPayloadOrError, error) {
	id, err := publicapi.For(ctx).Interaction.CommentOnFeedEvent(ctx, feedEventID, replyToID, comment)
	if err != nil {
		return nil, err
	}

	output := &model.CommentOnFeedEventPayload{
		Viewer: resolveViewer(ctx),
		Comment: &model.Comment{
			Dbid: id,
		},
		FeedEvent: &model.FeedEvent{
			Dbid: feedEventID, // remaining fields handled by dedicated resolver
		},
	}
	if replyToID != nil {
		output.ReplyToComment = &model.Comment{
			Dbid: *replyToID, // remaining fields handled by dedicated resolver
		}
	}
	return output, nil
}

func (r *mutationResolver) RemoveComment(ctx context.Context, commentID persist.DBID) (model.RemoveCommentPayloadOrError, error) {
	feedEvent, err := publicapi.For(ctx).Interaction.RemoveComment(ctx, commentID)
	if err != nil {
		return nil, err
	}
	output := &model.RemoveCommentPayload{
		Viewer: resolveViewer(ctx),
		FeedEvent: &model.FeedEvent{
			Dbid: feedEvent,
		},
	}
	return output, nil
}

func (r *mutationResolver) ViewGallery(ctx context.Context, galleryID persist.DBID) (model.ViewGalleryPayloadOrError, error) {
	gallery, err := publicapi.For(ctx).Gallery.ViewGallery(ctx, galleryID)
	if err != nil {
		return nil, err
	}

	output := &model.ViewGalleryPayload{
		Gallery: galleryToModel(ctx, gallery),
	}

	return output, nil
}

func (r *mutationResolver) CreateGallery(ctx context.Context, input model.CreateGalleryInput) (model.CreateGalleryPayloadOrError, error) {
	gallery, err := publicapi.For(ctx).Gallery.CreateGallery(ctx, input.Name, input.Description, input.Position)
	if err != nil {
		return nil, err
	}

	output := &model.CreateGalleryPayload{
		Gallery: galleryToModel(ctx, gallery),
	}

	return output, nil
}

func (r *mutationResolver) UpdateGalleryHidden(ctx context.Context, input model.UpdateGalleryHiddenInput) (model.UpdateGalleryHiddenPayloadOrError, error) {
	err := publicapi.For(ctx).Gallery.UpdateGalleryHidden(ctx, input.ID, input.Hidden)
	if err != nil {
		return nil, err
	}

	gallery, err := resolveGalleryByGalleryID(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	output := &model.UpdateGalleryHiddenPayload{
		Gallery: gallery,
	}

	return output, nil
}

func (r *mutationResolver) DeleteGallery(ctx context.Context, galleryID persist.DBID) (model.DeleteGalleryPayloadOrError, error) {
	gallery, err := publicapi.For(ctx).Gallery.DeleteGallery(ctx, galleryID)
	if err != nil {
		return nil, err
	}

	output := &model.DeleteGalleryPayload{
		Gallery: galleryToModel(ctx, gallery),
	}

	return output, nil
}

func (r *mutationResolver) UpdateGalleryOrder(ctx context.Context, input model.UpdateGalleryOrderInput) (model.UpdateGalleryOrderPayloadOrError, error) {
	err := publicapi.For(ctx).Gallery.UpdateGalleryPositions(ctx, input.Positions)
	if err != nil {
		return nil, err
	}

	output := &model.UpdateGalleryOrderPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) UpdateGalleryInfo(ctx context.Context, input model.UpdateGalleryInfoInput) (model.UpdateGalleryInfoPayloadOrError, error) {
	err := publicapi.For(ctx).Gallery.UpdateGalleryInfo(ctx, input.ID, input.Name, input.Description)
	if err != nil {
		return nil, err
	}

	gallery, err := resolveGalleryByGalleryID(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	output := &model.UpdateGalleryInfoPayload{
		Gallery: gallery,
	}

	return output, nil
}

func (r *mutationResolver) UpdateFeaturedGallery(ctx context.Context, galleryID persist.DBID) (model.UpdateFeaturedGalleryPayloadOrError, error) {
	err := publicapi.For(ctx).User.UpdateFeaturedGallery(ctx, galleryID)
	if err != nil {
		return nil, err
	}

	output := &model.UpdateFeaturedGalleryPayload{
		Viewer: resolveViewer(ctx),
	}

	return output, nil
}

func (r *mutationResolver) ClearAllNotifications(ctx context.Context) (*model.ClearAllNotificationsPayload, error) {
	notifications, err := publicapi.For(ctx).Notifications.ClearUserNotifications(ctx)
	if err != nil {
		return nil, err
	}

	models := make([]model.Notification, len(notifications))
	for i, n := range notifications {
		model, err := notificationToModel(n)
		if err != nil {
			return nil, err
		}
		models[i] = model
	}

	output := &model.ClearAllNotificationsPayload{
		Notifications: models,
	}
	return output, nil
}

func (r *mutationResolver) UpdateNotificationSettings(ctx context.Context, settings *model.NotificationSettingsInput) (*model.NotificationSettings, error) {
	err := publicapi.For(ctx).User.UpdateUserNotificationSettings(ctx, persist.UserNotificationSettings{
		SomeoneFollowedYou:           settings.SomeoneFollowedYou,
		SomeoneAdmiredYourUpdate:     settings.SomeoneAdmiredYourUpdate,
		SomeoneCommentedOnYourUpdate: settings.SomeoneCommentedOnYourUpdate,
		SomeoneViewedYourGallery:     settings.SomeoneViewedYourGallery,
	})
	if err != nil {
		return nil, err
	}
	return resolveViewerNotificationSettings(ctx)
}

func (r *mutationResolver) PreverifyEmail(ctx context.Context, input model.PreverifyEmailInput) (model.PreverifyEmailPayloadOrError, error) {
	// todo we could have the frontend send the source? right now I don't see any other sources of verification other than signing up
	result, err := emails.PreverifyEmail(ctx, input.Email, "signup")
	if err != nil {
		return nil, err
	}

	var modelResult model.PreverifyEmailResult

	switch result.Result {
	case emailService.PreverifyEmailResultValid:
		modelResult = model.PreverifyEmailResultValid
	case emailService.PreverifyEmailResultInvalid:
		modelResult = model.PreverifyEmailResultInvalid
	case emailService.PreverifyEmailResultRisky:
		modelResult = model.PreverifyEmailResultRisky
	default:
		return nil, fmt.Errorf("unknown preverify result: %d", result.Result)
	}

	return model.PreverifyEmailPayload{
		Email:  input.Email,
		Result: modelResult,
	}, nil
}

func (r *mutationResolver) VerifyEmail(ctx context.Context, input model.VerifyEmailInput) (model.VerifyEmailPayloadOrError, error) {
	return verifyEmail(ctx, input.Token)
}

func (r *mutationResolver) RedeemMerch(ctx context.Context, input model.RedeemMerchInput) (model.RedeemMerchPayloadOrError, error) {
	tokenIDList := make([]persist.TokenID, len(input.TokenIds))
	for i, id := range input.TokenIds {
		tokenIDList[i] = persist.TokenID(id)
	}
	if input.Address == nil {
		return nil, fmt.Errorf("address is required")
	}
	tokens, err := publicapi.For(ctx).Merch.RedeemMerchItems(ctx, tokenIDList, *input.Address, input.Signature, input.WalletType)
	if err != nil {
		return nil, err
	}

	output := &model.RedeemMerchPayload{
		Tokens: tokens,
	}
	return output, nil
}

func (r *mutationResolver) AddRolesToUser(ctx context.Context, username string, roles []*persist.Role) (model.AddRolesToUserPayloadOrError, error) {
	user, err := publicapi.For(ctx).Admin.AddRolesToUser(ctx, username, roles)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, *user), nil
}

func (r *mutationResolver) RevokeRolesFromUser(ctx context.Context, username string, roles []*persist.Role) (model.RevokeRolesFromUserPayloadOrError, error) {
	user, err := publicapi.For(ctx).Admin.RemoveRolesFromUser(ctx, username, roles)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, *user), nil
}

func (r *mutationResolver) SyncTokensForUsername(ctx context.Context, username string, chains []persist.Chain) (model.SyncTokensForUsernamePayloadOrError, error) {
	api := publicapi.For(ctx)

	user, err := api.User.GetUserByUsername(ctx, username)

	if err != nil {
		return nil, err
	}

	if chains == nil || len(chains) == 0 {
		chains = []persist.Chain{persist.ChainETH}
	}

	err = api.Token.SyncTokens(ctx, chains, &user.ID)
	if err != nil {
		return nil, err
	}

	output := &model.SyncTokensForUsernamePayload{
		Message: "Successfully synced tokens",
	}

	return output, nil
}

func (r *mutationResolver) BanUserFromFeed(ctx context.Context, username string, action string) (model.BanUserFromFeedPayloadOrError, error) {
	user, err := publicapi.For(ctx).User.GetUserByUsername(ctx, username)

	if err != nil {
		return nil, err
	}

	err = publicapi.For(ctx).Feed.BlockUser(ctx, user.ID, persist.Action(action))

	if err != nil {
		return nil, err
	}

	return model.BanUserFromFeedPayload{User: userToModel(ctx, *user)}, nil
}

func (r *mutationResolver) UploadPersistedQueries(ctx context.Context, input *model.UploadPersistedQueriesInput) (model.UploadPersistedQueriesPayloadOrError, error) {
	err := publicapi.For(ctx).APQ.UploadPersistedQueries(ctx, *input.PersistedQueries)

	if err != nil {
		return nil, err
	}

	message := "Persisted queries uploaded successfully"

	return model.UploadPersistedQueriesPayload{Message: &message}, nil
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

func (r *queryResolver) UserByAddress(ctx context.Context, chainAddress persist.ChainAddress) (model.UserByAddressOrError, error) {
	return resolveGalleryUserByAddress(ctx, chainAddress)
}

func (r *queryResolver) UsersWithTrait(ctx context.Context, trait string) ([]*model.GalleryUser, error) {
	return resolveGalleryUsersWithTrait(ctx, trait)
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

func (r *queryResolver) CollectionsByIds(ctx context.Context, ids []persist.DBID) ([]model.CollectionByIDOrError, error) {
	collections, errs := resolveCollectionsByCollectionIDs(ctx, ids)

	models := make([]model.CollectionByIDOrError, len(ids))

	// TODO: Figure out how to handle errors for slice returns automatically, the same way we handle typical resolvers.
	//       Without that kind of handling, errors must be checked manually (as is happening below).
	for i, err := range errs {
		if err == nil {
			models[i] = collections[i]
		} else if notFoundErr, ok := err.(persist.ErrCollectionNotFoundByID); ok {
			models[i] = model.ErrCollectionNotFound{Message: notFoundErr.Error()}
		} else if validationErr, ok := err.(publicapi.ErrInvalidInput); ok {
			models[i] = model.ErrInvalidInput{Message: validationErr.Error(), Parameters: validationErr.Parameters, Reasons: validationErr.Reasons}
		} else {
			// Unhandled error -- add it to the unhandled error stack, but don't fail the whole operation
			// because one collection hit an unexpected error. Consider making "unhandled error" an expected type,
			// such that it can be returned as part of a model "OrError" union instead of returning null.
			graphql.AddError(ctx, err)
		}
	}

	return models, nil
}

func (r *queryResolver) TokenByID(ctx context.Context, id persist.DBID) (model.TokenByIDOrError, error) {
	return resolveTokenByTokenID(ctx, id)
}

func (r *queryResolver) CollectionTokenByID(ctx context.Context, tokenID persist.DBID, collectionID persist.DBID) (model.CollectionTokenByIDOrError, error) {
	return resolveCollectionTokenByIDs(ctx, tokenID, collectionID)
}

func (r *queryResolver) CommunityByAddress(ctx context.Context, communityAddress persist.ChainAddress, forceRefresh *bool) (model.CommunityByAddressOrError, error) {
	return resolveCommunityByContractAddress(ctx, communityAddress, forceRefresh)
}

func (r *queryResolver) GeneralAllowlist(ctx context.Context) ([]*persist.ChainAddress, error) {
	return resolveGeneralAllowlist(ctx)
}

func (r *queryResolver) GalleryOfTheWeekWinners(ctx context.Context) ([]*model.GalleryUser, error) {
	winners, err := publicapi.For(ctx).Misc.GetGalleryOfTheWeekWinners(ctx)

	var output = make([]*model.GalleryUser, len(winners))
	for i, user := range winners {
		output[i] = userToModel(ctx, user)
	}

	return output, err
}

func (r *queryResolver) GlobalFeed(ctx context.Context, before *string, after *string, first *int, last *int) (*model.FeedConnection, error) {
	events, pageInfo, err := publicapi.For(ctx).Feed.PaginateGlobalFeed(ctx, before, after, first, last)
	if err != nil {
		return nil, err
	}

	edges, err := eventsToFeedEdges(events)
	if err != nil {
		return nil, err
	}

	return &model.FeedConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil
}

func (r *queryResolver) FeedEventByID(ctx context.Context, id persist.DBID) (model.FeedEventByIDOrError, error) {
	return resolveFeedEventByEventID(ctx, id)
}

func (r *queryResolver) GetMerchTokens(ctx context.Context, wallet persist.Address) (model.MerchTokensPayloadOrError, error) {
	tokens, err := publicapi.For(ctx).Merch.GetMerchTokens(ctx, wallet)
	if err != nil {
		return nil, err
	}

	output := &model.MerchTokensPayload{
		Tokens: tokens,
	}
	return output, nil
}

func (r *queryResolver) UsersByRole(ctx context.Context, role persist.Role, before *string, after *string, first *int, last *int) (*model.UsersConnection, error) {
	users, pageInfo, err := publicapi.For(ctx).User.PaginateUsersWithRole(ctx, role, before, after, first, last)
	if err != nil {
		return nil, err
	}

	edges := make([]*model.UserEdge, len(users))
	for i, user := range users {
		edges[i] = &model.UserEdge{
			Node:   userToModel(ctx, user),
			Cursor: nil, // not used by relay, but relay will complain without this field existing
		}
	}

	return &model.UsersConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil
}

func (r *removeAdmirePayloadResolver) FeedEvent(ctx context.Context, obj *model.RemoveAdmirePayload) (*model.FeedEvent, error) {
	return resolveFeedEventByEventID(ctx, obj.FeedEvent.Dbid)
}

func (r *removeCommentPayloadResolver) FeedEvent(ctx context.Context, obj *model.RemoveCommentPayload) (*model.FeedEvent, error) {
	return resolveFeedEventByEventID(ctx, obj.FeedEvent.Dbid)
}

func (r *setSpamPreferencePayloadResolver) Tokens(ctx context.Context, obj *model.SetSpamPreferencePayload) ([]*model.Token, error) {
	tokenIDs := make([]persist.DBID, len(obj.Tokens))
	for i, token := range obj.Tokens {
		tokenIDs[i] = token.Dbid
	}

	tokens, errors := publicapi.For(ctx).Token.GetTokensByTokenIDs(ctx, tokenIDs)

	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	return tokensToModel(ctx, tokens), nil
}

func (r *someoneAdmiredYourFeedEventNotificationResolver) FeedEvent(ctx context.Context, obj *model.SomeoneAdmiredYourFeedEventNotification) (*model.FeedEvent, error) {
	return resolveFeedEventByEventID(ctx, obj.FeedEventID)
}

func (r *someoneAdmiredYourFeedEventNotificationResolver) Admirers(ctx context.Context, obj *model.SomeoneAdmiredYourFeedEventNotification, before *string, after *string, first *int, last *int) (*model.GroupNotificationUsersConnection, error) {
	return resolveGroupNotificationUsersConnectionByUserIDs(ctx, obj.NotificationData.AdmirerIDs, before, after, first, last)
}

func (r *someoneCommentedOnYourFeedEventNotificationResolver) Comment(ctx context.Context, obj *model.SomeoneCommentedOnYourFeedEventNotification) (*model.Comment, error) {
	return resolveCommentByCommentID(ctx, obj.CommentID)
}

func (r *someoneCommentedOnYourFeedEventNotificationResolver) FeedEvent(ctx context.Context, obj *model.SomeoneCommentedOnYourFeedEventNotification) (*model.FeedEvent, error) {
	return resolveFeedEventByEventID(ctx, obj.HelperSomeoneCommentedOnYourFeedEventNotificationData.FeedEventID)
}

func (r *someoneFollowedYouBackNotificationResolver) Followers(ctx context.Context, obj *model.SomeoneFollowedYouBackNotification, before *string, after *string, first *int, last *int) (*model.GroupNotificationUsersConnection, error) {
	return resolveGroupNotificationUsersConnectionByUserIDs(ctx, obj.NotificationData.FollowerIDs, before, after, first, last)
}

func (r *someoneFollowedYouNotificationResolver) Followers(ctx context.Context, obj *model.SomeoneFollowedYouNotification, before *string, after *string, first *int, last *int) (*model.GroupNotificationUsersConnection, error) {
	return resolveGroupNotificationUsersConnectionByUserIDs(ctx, obj.NotificationData.FollowerIDs, before, after, first, last)
}

func (r *someoneViewedYourGalleryNotificationResolver) UserViewers(ctx context.Context, obj *model.SomeoneViewedYourGalleryNotification, before *string, after *string, first *int, last *int) (*model.GroupNotificationUsersConnection, error) {
	return resolveGroupNotificationUsersConnectionByUserIDs(ctx, obj.NotificationData.AuthedViewerIDs, before, after, first, last)
}

func (r *someoneViewedYourGalleryNotificationResolver) Gallery(ctx context.Context, obj *model.SomeoneViewedYourGalleryNotification) (*model.Gallery, error) {
	return resolveGalleryByGalleryID(ctx, obj.GalleryID)
}

func (r *subscriptionResolver) NewNotification(ctx context.Context) (<-chan model.Notification, error) {
	return resolveNewNotificationSubscription(ctx), nil
}

func (r *subscriptionResolver) NotificationUpdated(ctx context.Context) (<-chan model.Notification, error) {
	return resolveUpdatedNotificationSubscription(ctx), nil
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

func (r *tokensAddedToCollectionFeedEventDataResolver) Owner(ctx context.Context, obj *model.TokensAddedToCollectionFeedEventData) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Owner.Dbid)
}

func (r *tokensAddedToCollectionFeedEventDataResolver) Collection(ctx context.Context, obj *model.TokensAddedToCollectionFeedEventData) (*model.Collection, error) {
	return resolveCollectionByCollectionID(ctx, obj.Collection.Dbid)
}

func (r *tokensAddedToCollectionFeedEventDataResolver) NewTokens(ctx context.Context, obj *model.TokensAddedToCollectionFeedEventData) ([]*model.CollectionToken, error) {
	return resolveNewTokensByEventID(ctx, obj.FeedEventID)
}

func (r *unfollowUserPayloadResolver) User(ctx context.Context, obj *model.UnfollowUserPayload) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.User.Dbid)
}

func (r *updateCollectionTokensPayloadResolver) FeedEvent(ctx context.Context, obj *model.UpdateCollectionTokensPayload) (*model.FeedEvent, error) {
	if obj.FeedEvent.Dbid == "" {
		return nil, nil
	}
	return resolveFeedEventByEventID(ctx, obj.FeedEvent.Dbid)
}

func (r *userCreatedFeedEventDataResolver) Owner(ctx context.Context, obj *model.UserCreatedFeedEventData) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Owner.Dbid)
}

func (r *userFollowedUsersFeedEventDataResolver) Owner(ctx context.Context, obj *model.UserFollowedUsersFeedEventData) (*model.GalleryUser, error) {
	return resolveGalleryUserByUserID(ctx, obj.Owner.Dbid)
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

func (r *viewerResolver) Feed(ctx context.Context, obj *model.Viewer, before *string, after *string, first *int, last *int) (*model.FeedConnection, error) {
	events, pageInfo, err := publicapi.For(ctx).Feed.PaginatePersonalFeed(ctx, before, after, first, last)
	if err != nil {
		return nil, err
	}

	edges, err := eventsToFeedEdges(events)
	if err != nil {
		return nil, err
	}

	return &model.FeedConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil
}

func (r *viewerResolver) Email(ctx context.Context, obj *model.Viewer) (*model.UserEmail, error) {
	return resolveViewerEmail(ctx), nil
}

func (r *viewerResolver) Notifications(ctx context.Context, obj *model.Viewer, before *string, after *string, first *int, last *int) (*model.NotificationsConnection, error) {
	return resolveViewerNotifications(ctx, before, after, first, last)
}

func (r *viewerResolver) NotificationSettings(ctx context.Context, obj *model.Viewer) (*model.NotificationSettings, error) {
	return resolveViewerNotificationSettings(ctx)
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

func (r *chainPubKeyInputResolver) PubKey(ctx context.Context, obj *persist.ChainPubKey, data persist.PubKey) error {
	return obj.GQLSetPubKeyFromResolver(data)
}

func (r *chainPubKeyInputResolver) Chain(ctx context.Context, obj *persist.ChainPubKey, data persist.Chain) error {
	return obj.GQLSetChainFromResolver(data)
}

// Admire returns generated.AdmireResolver implementation.
func (r *Resolver) Admire() generated.AdmireResolver { return &admireResolver{r} }

// AdmireFeedEventPayload returns generated.AdmireFeedEventPayloadResolver implementation.
func (r *Resolver) AdmireFeedEventPayload() generated.AdmireFeedEventPayloadResolver {
	return &admireFeedEventPayloadResolver{r}
}

// Collection returns generated.CollectionResolver implementation.
func (r *Resolver) Collection() generated.CollectionResolver { return &collectionResolver{r} }

// CollectionCreatedFeedEventData returns generated.CollectionCreatedFeedEventDataResolver implementation.
func (r *Resolver) CollectionCreatedFeedEventData() generated.CollectionCreatedFeedEventDataResolver {
	return &collectionCreatedFeedEventDataResolver{r}
}

// CollectionToken returns generated.CollectionTokenResolver implementation.
func (r *Resolver) CollectionToken() generated.CollectionTokenResolver {
	return &collectionTokenResolver{r}
}

// CollectionUpdatedFeedEventData returns generated.CollectionUpdatedFeedEventDataResolver implementation.
func (r *Resolver) CollectionUpdatedFeedEventData() generated.CollectionUpdatedFeedEventDataResolver {
	return &collectionUpdatedFeedEventDataResolver{r}
}

// CollectorsNoteAddedToCollectionFeedEventData returns generated.CollectorsNoteAddedToCollectionFeedEventDataResolver implementation.
func (r *Resolver) CollectorsNoteAddedToCollectionFeedEventData() generated.CollectorsNoteAddedToCollectionFeedEventDataResolver {
	return &collectorsNoteAddedToCollectionFeedEventDataResolver{r}
}

// CollectorsNoteAddedToTokenFeedEventData returns generated.CollectorsNoteAddedToTokenFeedEventDataResolver implementation.
func (r *Resolver) CollectorsNoteAddedToTokenFeedEventData() generated.CollectorsNoteAddedToTokenFeedEventDataResolver {
	return &collectorsNoteAddedToTokenFeedEventDataResolver{r}
}

// Comment returns generated.CommentResolver implementation.
func (r *Resolver) Comment() generated.CommentResolver { return &commentResolver{r} }

// CommentOnFeedEventPayload returns generated.CommentOnFeedEventPayloadResolver implementation.
func (r *Resolver) CommentOnFeedEventPayload() generated.CommentOnFeedEventPayloadResolver {
	return &commentOnFeedEventPayloadResolver{r}
}

// Community returns generated.CommunityResolver implementation.
func (r *Resolver) Community() generated.CommunityResolver { return &communityResolver{r} }

// CreateCollectionPayload returns generated.CreateCollectionPayloadResolver implementation.
func (r *Resolver) CreateCollectionPayload() generated.CreateCollectionPayloadResolver {
	return &createCollectionPayloadResolver{r}
}

// FeedEvent returns generated.FeedEventResolver implementation.
func (r *Resolver) FeedEvent() generated.FeedEventResolver { return &feedEventResolver{r} }

// FollowInfo returns generated.FollowInfoResolver implementation.
func (r *Resolver) FollowInfo() generated.FollowInfoResolver { return &followInfoResolver{r} }

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

// RemoveAdmirePayload returns generated.RemoveAdmirePayloadResolver implementation.
func (r *Resolver) RemoveAdmirePayload() generated.RemoveAdmirePayloadResolver {
	return &removeAdmirePayloadResolver{r}
}

// RemoveCommentPayload returns generated.RemoveCommentPayloadResolver implementation.
func (r *Resolver) RemoveCommentPayload() generated.RemoveCommentPayloadResolver {
	return &removeCommentPayloadResolver{r}
}

// SetSpamPreferencePayload returns generated.SetSpamPreferencePayloadResolver implementation.
func (r *Resolver) SetSpamPreferencePayload() generated.SetSpamPreferencePayloadResolver {
	return &setSpamPreferencePayloadResolver{r}
}

// SomeoneAdmiredYourFeedEventNotification returns generated.SomeoneAdmiredYourFeedEventNotificationResolver implementation.
func (r *Resolver) SomeoneAdmiredYourFeedEventNotification() generated.SomeoneAdmiredYourFeedEventNotificationResolver {
	return &someoneAdmiredYourFeedEventNotificationResolver{r}
}

// SomeoneCommentedOnYourFeedEventNotification returns generated.SomeoneCommentedOnYourFeedEventNotificationResolver implementation.
func (r *Resolver) SomeoneCommentedOnYourFeedEventNotification() generated.SomeoneCommentedOnYourFeedEventNotificationResolver {
	return &someoneCommentedOnYourFeedEventNotificationResolver{r}
}

// SomeoneFollowedYouBackNotification returns generated.SomeoneFollowedYouBackNotificationResolver implementation.
func (r *Resolver) SomeoneFollowedYouBackNotification() generated.SomeoneFollowedYouBackNotificationResolver {
	return &someoneFollowedYouBackNotificationResolver{r}
}

// SomeoneFollowedYouNotification returns generated.SomeoneFollowedYouNotificationResolver implementation.
func (r *Resolver) SomeoneFollowedYouNotification() generated.SomeoneFollowedYouNotificationResolver {
	return &someoneFollowedYouNotificationResolver{r}
}

// SomeoneViewedYourGalleryNotification returns generated.SomeoneViewedYourGalleryNotificationResolver implementation.
func (r *Resolver) SomeoneViewedYourGalleryNotification() generated.SomeoneViewedYourGalleryNotificationResolver {
	return &someoneViewedYourGalleryNotificationResolver{r}
}

// Subscription returns generated.SubscriptionResolver implementation.
func (r *Resolver) Subscription() generated.SubscriptionResolver { return &subscriptionResolver{r} }

// Token returns generated.TokenResolver implementation.
func (r *Resolver) Token() generated.TokenResolver { return &tokenResolver{r} }

// TokenHolder returns generated.TokenHolderResolver implementation.
func (r *Resolver) TokenHolder() generated.TokenHolderResolver { return &tokenHolderResolver{r} }

// TokensAddedToCollectionFeedEventData returns generated.TokensAddedToCollectionFeedEventDataResolver implementation.
func (r *Resolver) TokensAddedToCollectionFeedEventData() generated.TokensAddedToCollectionFeedEventDataResolver {
	return &tokensAddedToCollectionFeedEventDataResolver{r}
}

// UnfollowUserPayload returns generated.UnfollowUserPayloadResolver implementation.
func (r *Resolver) UnfollowUserPayload() generated.UnfollowUserPayloadResolver {
	return &unfollowUserPayloadResolver{r}
}

// UpdateCollectionTokensPayload returns generated.UpdateCollectionTokensPayloadResolver implementation.
func (r *Resolver) UpdateCollectionTokensPayload() generated.UpdateCollectionTokensPayloadResolver {
	return &updateCollectionTokensPayloadResolver{r}
}

// UserCreatedFeedEventData returns generated.UserCreatedFeedEventDataResolver implementation.
func (r *Resolver) UserCreatedFeedEventData() generated.UserCreatedFeedEventDataResolver {
	return &userCreatedFeedEventDataResolver{r}
}

// UserFollowedUsersFeedEventData returns generated.UserFollowedUsersFeedEventDataResolver implementation.
func (r *Resolver) UserFollowedUsersFeedEventData() generated.UserFollowedUsersFeedEventDataResolver {
	return &userFollowedUsersFeedEventDataResolver{r}
}

// Viewer returns generated.ViewerResolver implementation.
func (r *Resolver) Viewer() generated.ViewerResolver { return &viewerResolver{r} }

// Wallet returns generated.WalletResolver implementation.
func (r *Resolver) Wallet() generated.WalletResolver { return &walletResolver{r} }

// ChainAddressInput returns generated.ChainAddressInputResolver implementation.
func (r *Resolver) ChainAddressInput() generated.ChainAddressInputResolver {
	return &chainAddressInputResolver{r}
}

// ChainPubKeyInput returns generated.ChainPubKeyInputResolver implementation.
func (r *Resolver) ChainPubKeyInput() generated.ChainPubKeyInputResolver {
	return &chainPubKeyInputResolver{r}
}

type admireResolver struct{ *Resolver }
type admireFeedEventPayloadResolver struct{ *Resolver }
type collectionResolver struct{ *Resolver }
type collectionCreatedFeedEventDataResolver struct{ *Resolver }
type collectionTokenResolver struct{ *Resolver }
type collectionUpdatedFeedEventDataResolver struct{ *Resolver }
type collectorsNoteAddedToCollectionFeedEventDataResolver struct{ *Resolver }
type collectorsNoteAddedToTokenFeedEventDataResolver struct{ *Resolver }
type commentResolver struct{ *Resolver }
type commentOnFeedEventPayloadResolver struct{ *Resolver }
type communityResolver struct{ *Resolver }
type createCollectionPayloadResolver struct{ *Resolver }
type feedEventResolver struct{ *Resolver }
type followInfoResolver struct{ *Resolver }
type followUserPayloadResolver struct{ *Resolver }
type galleryResolver struct{ *Resolver }
type galleryUserResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type ownerAtBlockResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type removeAdmirePayloadResolver struct{ *Resolver }
type removeCommentPayloadResolver struct{ *Resolver }
type setSpamPreferencePayloadResolver struct{ *Resolver }
type someoneAdmiredYourFeedEventNotificationResolver struct{ *Resolver }
type someoneCommentedOnYourFeedEventNotificationResolver struct{ *Resolver }
type someoneFollowedYouBackNotificationResolver struct{ *Resolver }
type someoneFollowedYouNotificationResolver struct{ *Resolver }
type someoneViewedYourGalleryNotificationResolver struct{ *Resolver }
type subscriptionResolver struct{ *Resolver }
type tokenResolver struct{ *Resolver }
type tokenHolderResolver struct{ *Resolver }
type tokensAddedToCollectionFeedEventDataResolver struct{ *Resolver }
type unfollowUserPayloadResolver struct{ *Resolver }
type updateCollectionTokensPayloadResolver struct{ *Resolver }
type userCreatedFeedEventDataResolver struct{ *Resolver }
type userFollowedUsersFeedEventDataResolver struct{ *Resolver }
type viewerResolver struct{ *Resolver }
type walletResolver struct{ *Resolver }
type chainAddressInputResolver struct{ *Resolver }
type chainPubKeyInputResolver struct{ *Resolver }
