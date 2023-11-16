package graphql

// schema.resolvers.go gets updated when generating gqlgen bindings and should not contain
// helper functions. schema.resolvers.helpers.go is a companion file that can contain
// helper functions without interfering with code generation.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/gammazero/workerpool"
	"github.com/magiclabs/magic-admin-go/token"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/debugtools"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/emails"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/mediamapper"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc/ipfs"
	"github.com/mikeydub/go-gallery/service/socialauth"
	"github.com/mikeydub/go-gallery/service/twitter"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

var errNoAuthMechanismFound = fmt.Errorf("no auth mechanism found")

var nodeFetcher = model.NodeFetcher{
	OnGallery:          resolveGalleryByGalleryID,
	OnCollection:       resolveCollectionByCollectionID,
	OnGalleryUser:      resolveGalleryUserByUserID,
	OnMembershipTier:   resolveMembershipTierByMembershipId,
	OnToken:            resolveTokenByTokenID,
	OnWallet:           resolveWalletByAddress,
	OnContract:         resolveContractByContractID,
	OnFeedEvent:        resolveFeedEventByEventID,
	OnAdmire:           resolveAdmireByAdmireID,
	OnComment:          resolveCommentByCommentID,
	OnMerchToken:       resolveMerchTokenByTokenID,
	OnViewer:           resolveViewerByID,
	OnDeletedNode:      resolveDeletedNodeByID,
	OnSocialConnection: resolveSocialConnectionByIdentifiers,
	OnPost:             resolvePostByPostID,
	OnTokenDefinition:  resolveTokenDefinitionByID,

	OnCollectionToken: func(ctx context.Context, tokenId string, collectionId string) (*model.CollectionToken, error) {
		return resolveCollectionTokenByID(ctx, persist.DBID(tokenId), persist.DBID(collectionId))
	},

	OnCommunity: func(ctx context.Context, dbid persist.DBID) (*model.Community, error) {
		return resolveCommunityByID(ctx, dbid)
	},
	OnSomeoneAdmiredYourFeedEventNotification:          fetchNotificationByID[model.SomeoneAdmiredYourFeedEventNotification],
	OnSomeoneCommentedOnYourFeedEventNotification:      fetchNotificationByID[model.SomeoneCommentedOnYourFeedEventNotification],
	OnSomeoneAdmiredYourPostNotification:               fetchNotificationByID[model.SomeoneAdmiredYourPostNotification],
	OnSomeoneCommentedOnYourPostNotification:           fetchNotificationByID[model.SomeoneCommentedOnYourPostNotification],
	OnSomeoneFollowedYouBackNotification:               fetchNotificationByID[model.SomeoneFollowedYouBackNotification],
	OnSomeoneFollowedYouNotification:                   fetchNotificationByID[model.SomeoneFollowedYouNotification],
	OnSomeoneViewedYourGalleryNotification:             fetchNotificationByID[model.SomeoneViewedYourGalleryNotification],
	OnNewTokensNotification:                            fetchNotificationByID[model.NewTokensNotification],
	OnSomeoneMentionedYouNotification:                  fetchNotificationByID[model.SomeoneMentionedYouNotification],
	OnSomeoneMentionedYourCommunityNotification:        fetchNotificationByID[model.SomeoneMentionedYourCommunityNotification],
	OnSomeoneRepliedToYourCommentNotification:          fetchNotificationByID[model.SomeoneRepliedToYourCommentNotification],
	OnSomeoneAdmiredYourTokenNotification:              fetchNotificationByID[model.SomeoneAdmiredYourTokenNotification],
	OnSomeonePostedYourWorkNotification:                fetchNotificationByID[model.SomeonePostedYourWorkNotification],
	OnSomeoneYouFollowPostedTheirFirstPostNotification: fetchNotificationByID[model.SomeoneYouFollowPostedTheirFirstPostNotification],
}

// T any is a notification type, will panic if it is not a notification type
func fetchNotificationByID[T any](ctx context.Context, dbid persist.DBID) (*T, error) {
	notif, err := resolveNotificationByID(ctx, dbid)
	if err != nil {
		return nil, err
	}

	notifConverted := notif.(T)

	return &notifConverted, nil
}

var defaultTokenSettings = persist.CollectionTokenSettings{}

func init() {
	nodeFetcher.ValidateHandlers()
}

// errorToGraphqlType converts a golang error to its matching type from our GraphQL schema.
// If no matching type is found, ok will return false
func errorToGraphqlType(ctx context.Context, err error, gqlTypeName string) (gqlModel interface{}, ok bool) {
	message := err.Error()
	var mappedErr model.Error = nil

	// TODO: Add model.ErrNotAuthorized mapping once auth handling is moved to the publicapi layer

	switch {
	case util.ErrorAs[auth.ErrAuthenticationFailed](err):
		mappedErr = model.ErrAuthenticationFailed{Message: message}
	case util.ErrorAs[auth.ErrDoesNotOwnRequiredNFT](err):
		mappedErr = model.ErrDoesNotOwnRequiredToken{Message: message}
	case util.ErrorAs[persist.ErrUserNotFound](err):
		mappedErr = model.ErrUserNotFound{Message: message}
	case util.ErrorAs[persist.ErrUserAlreadyExists](err):
		mappedErr = model.ErrUserAlreadyExists{Message: message}
	case util.ErrorAs[persist.ErrUsernameNotAvailable](err):
		mappedErr = model.ErrUsernameNotAvailable{Message: message}
	case util.ErrorAs[persist.ErrCollectionNotFoundByID](err):
		mappedErr = model.ErrCollectionNotFound{Message: message}
	case util.ErrorAs[persist.ErrTokenNotFound](err) || util.ErrorAs[persist.ErrTokenDefinitionNotFound](err):
		mappedErr = model.ErrTokenNotFound{Message: message}
	case util.ErrorAs[persist.ErrContractNotFound](err):
		mappedErr = model.ErrCommunityNotFound{Message: message}
	case util.ErrorAs[persist.ErrAddressOwnedByUser](err):
		mappedErr = model.ErrAddressOwnedByUser{Message: message}
	case util.ErrorAs[persist.ErrAdmireNotFound](err) || util.ErrorAs[persist.ErrAdmireFeedEventNotFound](err) || util.ErrorAs[persist.ErrAdmirePostNotFound](err) || util.ErrorAs[persist.ErrAdmireTokenNotFound](err):
		mappedErr = model.ErrAdmireNotFound{Message: message}
	case util.ErrorAs[persist.ErrAdmireAlreadyExists](err):
		mappedErr = model.ErrAdmireAlreadyExists{Message: message}
	case util.ErrorAs[persist.ErrCommentNotFound](err):
		mappedErr = model.ErrCommentNotFound{Message: message}
	case util.ErrorAs[publicapi.ErrTokenRefreshFailed](err):
		mappedErr = model.ErrSyncFailed{Message: message}
	case util.ErrorAs[validate.ErrInvalidInput](err):
		errTyp := err.(validate.ErrInvalidInput)
		mappedErr = model.ErrInvalidInput{Message: message, Parameters: errTyp.Parameters, Reasons: errTyp.Reasons}
	case util.ErrorAs[persist.ErrFeedEventNotFoundByID](err):
		mappedErr = model.ErrFeedEventNotFound{Message: message}
	case util.ErrorAs[persist.ErrUnknownAction](err):
		mappedErr = model.ErrUnknownAction{Message: message}
	case util.ErrorAs[persist.ErrGalleryNotFound](err):
		mappedErr = model.ErrGalleryNotFound{Message: message}
	case util.ErrorAs[twitter.ErrInvalidRefreshToken](err):
		mappedErr = model.ErrNeedsToReconnectSocial{SocialAccountType: persist.SocialProviderTwitter, Message: message}
	case util.ErrorAs[persist.ErrPushTokenBelongsToAnotherUser](err):
		mappedErr = model.ErrPushTokenBelongsToAnotherUser{Message: message}
	case errors.Is(err, publicapi.ErrProfileImageTooManySources) || errors.Is(err, publicapi.ErrProfileImageUnknownSource):
		mappedErr = model.ErrInvalidInput{Message: message}
	case errors.Is(err, publicapi.ErrProfileImageNotTokenOwner) || errors.Is(err, publicapi.ErrProfileImageNotWalletOwner):
		mappedErr = model.ErrNotAuthorized{Message: message}
	case errors.Is(err, auth.ErrEmailUnverified):
		mappedErr = model.ErrEmailUnverified{Message: message}
	case errors.Is(err, auth.ErrEmailAlreadyUsed):
		mappedErr = model.ErrEmailAlreadyUsed{Message: message}
	case errors.Is(err, eth.ErrNoAvatarRecord) || errors.Is(err, eth.ErrNoResolution):
		mappedErr = model.ErrNoAvatarRecordSet{Message: message}
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
		if debugtools.IsDebugEnv() && m.Debug != nil {
			return authApi.NewDebugAuthenticator(ctx, *m.Debug)
		}
	}

	if m.Eoa != nil && m.Eoa.ChainPubKey != nil {
		return authApi.NewNonceAuthenticator(*m.Eoa.ChainPubKey, m.Eoa.Nonce, m.Eoa.Signature, persist.WalletTypeEOA), nil
	}

	if m.GnosisSafe != nil {
		// GnosisSafe passes an empty signature
		return authApi.NewNonceAuthenticator(persist.NewChainPubKey(persist.PubKey(m.GnosisSafe.Address), persist.ChainETH), m.GnosisSafe.Nonce, "0x", persist.WalletTypeGnosis), nil
	}

	if m.MagicLink != nil && m.MagicLink.Token != "" {
		t, err := token.NewToken(m.MagicLink.Token)
		if err != nil {
			return nil, err
		}
		return authApi.NewMagicLinkAuthenticator(*t), nil
	}

	if m.OneTimeLoginToken != nil && m.OneTimeLoginToken.Token != "" {
		return authApi.NewOneTimeLoginTokenAuthenticator(m.OneTimeLoginToken.Token), nil
	}

	return nil, errNoAuthMechanismFound
}

// authMechanismToAuthenticator takes a GraphQL AuthMechanism and returns an Authenticator that can be used for auth
func (r *Resolver) socialAuthMechanismToAuthenticator(ctx context.Context, m model.SocialAuthMechanism) (socialauth.Authenticator, error) {

	if debugtools.Enabled {
		if debugtools.IsDebugEnv() && m.Debug != nil {
			password := util.FromPointer(m.Debug.DebugToolsPassword)
			return debugtools.NewDebugSocialAuthenticator(m.Debug.Provider, m.Debug.ID, map[string]interface{}{"username": m.Debug.Username}, password), nil
		}
	}

	authedUserID := publicapi.For(ctx).User.GetLoggedInUserId(ctx)
	if m.Twitter != nil {
		return publicapi.For(ctx).Social.NewTwitterAuthenticator(authedUserID, m.Twitter.Code), nil
	}

	if m.Farcaster != nil {
		return publicapi.For(ctx).Social.NewFarcasterAuthenticator(authedUserID, m.Farcaster.Address, util.FromPointer(m.Farcaster.WithSigner)), nil
	}

	if m.Lens != nil {
		return publicapi.For(ctx).Social.NewLensAuthenticator(authedUserID, m.Lens.Address, util.FromPointer(m.Lens.Signature)), nil
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

func resolveGalleryUserByAddress(ctx context.Context, chainAddress persist.ChainAddress) (*model.GalleryUser, error) {
	user, err := publicapi.For(ctx).User.GetUserByAddress(ctx, chainAddress)

	if err != nil {
		return nil, err
	}

	return userToModel(ctx, *user), nil
}

func resolveGalleryUsersWithTrait(ctx context.Context, trait string) ([]*model.GalleryUser, error) {
	users, err := publicapi.For(ctx).User.GetUsersWithTrait(ctx, trait)

	if err != nil {
		return nil, err
	}

	models := make([]*model.GalleryUser, len(users))
	for i, user := range users {
		models[i] = userToModel(ctx, user)
	}

	return models, nil
}

const top100ActivityImageURL = "https://storage.googleapis.com/prod-token-content/top_100.png"

func resolveBadgesByUserID(ctx context.Context, userID persist.DBID, traits persist.Traits) ([]*model.Badge, error) {
	contracts, err := publicapi.For(ctx).Contract.GetContractsDisplayedByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var result []*model.Badge
	for _, contract := range contracts {
		result = append(result, contractToBadgeModel(ctx, contract))
	}

	if _, ok := traits[persist.TraitTypeTop100ActiveUser]; ok {

		result = append(result, &model.Badge{
			Name:     util.ToPointer("Top 100 Active User"),
			ImageURL: top100ActivityImageURL,
		})
	}

	return result, nil
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

func resolveCollectionsByCollectionIDs(ctx context.Context, collectionIDs []persist.DBID) ([]*model.Collection, []error) {
	models := make([]*model.Collection, len(collectionIDs))
	errors := make([]error, len(collectionIDs))

	collections, collectionErrs := publicapi.For(ctx).Collection.GetCollectionsByIds(ctx, collectionIDs)

	for i, err := range collectionErrs {
		if err != nil {
			errors[i] = err
		} else {
			models[i] = collectionToModel(ctx, *collections[i])
		}
	}

	return models, errors
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

func resolveTokenPreviewsByGalleryID(ctx context.Context, galleryID persist.DBID) ([]*model.PreviewURLSet, error) {
	medias, err := publicapi.For(ctx).Gallery.GetTokenPreviewsByGalleryID(ctx, galleryID)
	if err != nil {
		return nil, err
	}

	return util.Map(medias, func(t db.TokenMedia) (*model.PreviewURLSet, error) {
		return previewURLsFromTokenMedia(ctx, t), nil
	})
}

func resolveCollectionTokenByID(ctx context.Context, tokenID persist.DBID, collectionID persist.DBID) (*model.CollectionToken, error) {
	token, err := resolveTokenByTokenIDCollectionID(ctx, tokenID, collectionID)
	if err != nil {
		return nil, err
	}
	return collectionTokenToModel(ctx, token, collectionID), nil
}

func resolveGalleryByGalleryID(ctx context.Context, galleryID persist.DBID) (*model.Gallery, error) {
	dbGal, err := publicapi.For(ctx).Gallery.GetGalleryById(ctx, galleryID)
	if err != nil {
		return nil, err
	}
	gallery := &model.Gallery{
		Dbid:          galleryID,
		Name:          &dbGal.Name,
		Description:   &dbGal.Description,
		Position:      &dbGal.Position,
		Hidden:        &dbGal.Hidden,
		TokenPreviews: nil, // handled by dedicated resolver
		Owner:         nil, // handled by dedicated resolver
		Collections:   nil, // handled by dedicated resolver
	}

	return gallery, nil
}

func resolveViewerGalleryByGalleryID(ctx context.Context, galleryID persist.DBID) (*model.ViewerGallery, error) {
	gallery, err := publicapi.For(ctx).Gallery.GetViewerGalleryById(ctx, galleryID)

	if err != nil {
		return nil, err
	}

	return &model.ViewerGallery{
		Gallery: galleryToModel(ctx, *gallery),
	}, nil
}

func resolveViewerExperiencesByUserID(ctx context.Context, userID persist.DBID) ([]*model.UserExperience, error) {
	return publicapi.For(ctx).User.GetUserExperiences(ctx, userID)
}

func resolveViewerSocialsByUserID(ctx context.Context, userID persist.DBID) (*model.SocialAccounts, error) {
	return publicapi.For(ctx).User.GetSocials(ctx, userID)
}

func resolveUserSocialsByUserID(ctx context.Context, userID persist.DBID) (*model.SocialAccounts, error) {
	return publicapi.For(ctx).User.GetDisplayedSocials(ctx, userID)
}

func resolveTokenByTokenIDCollectionID(ctx context.Context, tokenID persist.DBID, collectionID persist.DBID) (*model.Token, error) {
	token, err := publicapi.For(ctx).Token.GetTokenById(ctx, tokenID)
	if err != nil {
		return nil, err
	}

	return tokenToModel(ctx, *token, &collectionID), nil
}

func resolveTokenByTokenID(ctx context.Context, tokenID persist.DBID) (*model.Token, error) {
	token, err := publicapi.For(ctx).Token.GetTokenById(ctx, tokenID)
	if err != nil {
		return nil, err
	}

	return tokenToModel(ctx, *token, nil), nil
}

func resolveTokensByWalletID(ctx context.Context, walletID persist.DBID) ([]*model.Token, error) {
	tokens, err := publicapi.For(ctx).Token.GetTokensByWalletID(ctx, walletID)

	if err != nil {
		return nil, err
	}

	return tokensToModel(ctx, tokens), nil
}

func resolveTokensByContractIDWithPagination(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int, onlyGalleryUsers bool) (*model.TokensConnection, error) {
	tokens, pageInfo, err := publicapi.For(ctx).Token.GetTokensByContractIdPaginate(ctx, contractID, before, after, first, last, onlyGalleryUsers)
	if err != nil {
		return nil, err
	}
	connection := tokensToConnection(ctx, tokens, pageInfo)
	return &connection, nil
}

func tokensToConnection(ctx context.Context, tokens []db.Token, pageInfo publicapi.PageInfo) model.TokensConnection {
	edges := make([]*model.TokenEdge, len(tokens))
	for i, token := range tokens {
		edges[i] = &model.TokenEdge{
			Node:   tokenToModel(ctx, token, nil),
			Cursor: nil, // not used by relay, but relay will complain without this field existing
		}
	}
	return model.TokensConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}
}

func refreshTokensInContractAsync(ctx context.Context, contractID persist.DBID, forceRefresh bool) error {
	return publicapi.For(ctx).Contract.RefreshOwnersAsync(ctx, contractID, forceRefresh)
}

func resolveTokenOwnerByTokenID(ctx context.Context, tokenID persist.DBID) (*model.GalleryUser, error) {
	token, err := publicapi.For(ctx).Token.GetTokenById(ctx, tokenID)

	if err != nil {
		return nil, err
	}

	return resolveGalleryUserByUserID(ctx, token.OwnerUserID)
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

	userID := publicapi.For(ctx).User.GetLoggedInUserId(ctx)

	viewer := &model.Viewer{
		HelperViewerData: model.HelperViewerData{
			UserId: userID,
		},
		User:            nil, // handled by dedicated resolver
		ViewerGalleries: nil, // handled by dedicated resolver
	}

	return viewer
}

func resolveViewerEmail(ctx context.Context) *model.UserEmail {
	userWithPII, err := publicapi.For(ctx).User.GetUserWithPII(ctx)
	if err != nil {
		return nil
	}

	return userWithPIIToEmailModel(userWithPII)
}

func userWithPIIToEmailModel(user *db.PiiUserView) *model.UserEmail {

	return &model.UserEmail{
		Email:              &user.PiiEmailAddress,
		VerificationStatus: &user.EmailVerified,
		EmailNotificationSettings: &model.EmailNotificationSettings{
			UnsubscribedFromAll:           user.EmailUnsubscriptions.All.Bool(),
			UnsubscribedFromNotifications: user.EmailUnsubscriptions.Notifications.Bool(),
		},
	}

}

func resolveMembershipTierByMembershipId(ctx context.Context, id persist.DBID) (*model.MembershipTier, error) {
	tier, err := publicapi.For(ctx).User.GetMembershipByMembershipId(ctx, id)

	if err != nil {
		return nil, err
	}

	return membershipToModel(ctx, *tier), nil
}

func resolveCommunityByID(ctx context.Context, id persist.DBID) (*model.Community, error) {
	community, err := publicapi.For(ctx).Contract.GetContractByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return communityToModel(ctx, *community, nil), nil
}

func resolveCommunityByContractAddress(ctx context.Context, contractAddress persist.ChainAddress, forceRefresh *bool) (*model.Community, error) {
	community, err := publicapi.For(ctx).Contract.GetContractByAddress(ctx, contractAddress)

	if err != nil {
		return nil, err
	}

	return communityToModel(ctx, *community, forceRefresh), nil
}

func resolveCommunityOwnersByContractID(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int, onlyGalleryUsers bool) (*model.TokenHoldersConnection, error) {
	contract, err := publicapi.For(ctx).Contract.GetContractByID(ctx, contractID)
	if err != nil {
		return nil, err
	}
	owners, pageInfo, err := publicapi.For(ctx).Contract.GetCommunityOwnersByContractAddress(ctx, persist.NewChainAddress(contract.Address, contract.Chain), before, after, first, last, onlyGalleryUsers)
	if err != nil {
		return nil, err
	}
	connection := ownersToConnection(ctx, owners, contractID, pageInfo)
	return &connection, nil
}

func resolveCommunityPostsByContractID(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int) (*model.PostsConnection, error) {
	posts, pageInfo, err := publicapi.For(ctx).Contract.GetCommunityPostsByContractID(ctx, contractID, before, after, first, last)
	if err != nil {
		return nil, err
	}
	connection := postsToConnection(ctx, posts, contractID, pageInfo)
	return &connection, nil
}

func ownersToConnection(ctx context.Context, owners []db.User, contractID persist.DBID, pageInfo publicapi.PageInfo) model.TokenHoldersConnection {
	edges := make([]*model.TokenHolderEdge, len(owners))
	for i, owner := range owners {
		walletIDs := make([]persist.DBID, len(owner.Wallets))
		for j, wallet := range owner.Wallets {
			walletIDs[j] = wallet.ID
		}
		edges[i] = &model.TokenHolderEdge{
			Node: &model.TokenHolder{
				HelperTokenHolderData: model.HelperTokenHolderData{
					UserId:     owner.ID,
					WalletIds:  walletIDs,
					ContractId: contractID,
				},
				DisplayName:   &owner.Username.String,
				Wallets:       nil, // handled by a dedicated resolver
				User:          nil, // handled by a dedicated resolver
				PreviewTokens: nil, // handled by dedicated resolver
			},
			Cursor: nil, // not used by relay, but relay will complain without this field existing
		}
	}
	return model.TokenHoldersConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}
}

func postsToConnection(ctx context.Context, posts []db.Post, contractID persist.DBID, pageInfo publicapi.PageInfo) model.PostsConnection {
	edges := make([]*model.PostEdge, len(posts))
	for i, post := range posts {

		po := post

		cval, _ := po.Caption.Value()

		var caption *string
		if cval != nil {
			caption = util.ToPointer(cval.(string))
		}

		edges[i] = &model.PostEdge{
			Node: &model.Post{
				HelperPostData: model.HelperPostData{
					TokenIDs: po.TokenIds,
					AuthorID: po.ActorID,
				},
				CreationTime: &po.CreatedAt,
				Dbid:         po.ID,
				Tokens:       nil, // handled by dedicated resolver
				Caption:      caption,
				Admires:      nil, // handled by dedicated resolver
				Comments:     nil, // handled by dedicated resolver
				Interactions: nil, // handled by dedicated resolver
				ViewerAdmire: nil, // handled by dedicated resolver

			},
			Cursor: nil, // not used by relay, but relay will complain without this field existing
		}
	}
	return model.PostsConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}
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

func resolvePrimaryWalletByUserID(ctx context.Context, userID persist.DBID) (*model.Wallet, error) {

	user, err := publicapi.For(ctx).User.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user.PrimaryWalletID == "" {
		return nil, nil
	}

	wallet, err := publicapi.For(ctx).Wallet.GetWalletByID(ctx, user.PrimaryWalletID)
	if err != nil {
		return nil, err
	}

	return walletToModelSqlc(ctx, *wallet), nil
}

func resolveFeedEventByEventID(ctx context.Context, eventID persist.DBID) (*model.FeedEvent, error) {
	event, err := publicapi.For(ctx).Feed.GetFeedEventById(ctx, eventID)
	if err != nil {
		return nil, err
	}

	return feedEventToModel(event)
}

func resolvePostByPostID(ctx context.Context, postID persist.DBID) (*model.Post, error) {
	post, err := publicapi.For(ctx).Feed.GetPostById(ctx, postID)
	if err != nil {
		return nil, err
	}

	return postToModel(post)
}

func resolveTokenDefinitionByID(ctx context.Context, dbid persist.DBID) (*model.TokenDefinition, error) {
	td, err := publicapi.For(ctx).Token.GetTokenDefinitionByID(ctx, dbid)
	if err != nil {
		return nil, err
	}
	return tokenDefinitionToModel(td), nil
}

func resolveMentionsByCommentID(ctx context.Context, commentID persist.DBID) ([]*model.Mention, error) {
	mentions, err := publicapi.For(ctx).Interaction.GetMentionsByCommentID(ctx, commentID)
	if err != nil {
		return nil, err
	}

	return mentionsToModel(ctx, mentions)
}

func resolveMentionsByPostID(ctx context.Context, postID persist.DBID) ([]*model.Mention, error) {
	mentions, err := publicapi.For(ctx).Interaction.GetMentionsByPostID(ctx, postID)
	if err != nil {
		return nil, err
	}

	return mentionsToModel(ctx, mentions)
}

func mentionsToModel(ctx context.Context, mentions []db.Mention) ([]*model.Mention, error) {
	result := make([]*model.Mention, len(mentions))

	for i, mention := range mentions {
		result[i] = mentionToModel(ctx, mention)
	}

	return result, nil
}

func resolveViewerNotifications(ctx context.Context, before *string, after *string, first *int, last *int) (*model.NotificationsConnection, error) {

	notifs, pageInfo, unseen, err := publicapi.For(ctx).Notifications.GetViewerNotifications(ctx, before, after, first, last)

	if err != nil {
		return nil, err
	}

	edges, err := notificationsToEdges(notifs)

	if err != nil {
		return nil, err
	}

	return &model.NotificationsConnection{
		Edges:       edges,
		PageInfo:    pageInfoToModel(ctx, pageInfo),
		UnseenCount: &unseen,
	}, nil
}

func notificationsToEdges(notifs []db.Notification) ([]*model.NotificationEdge, error) {
	edges := make([]*model.NotificationEdge, len(notifs))

	for i, notif := range notifs {

		node, err := notificationToModel(notif)
		if err != nil {
			return nil, err
		}

		edges[i] = &model.NotificationEdge{
			Node: node,
		}
	}

	return edges, nil
}

func notificationToModel(notif db.Notification) (model.Notification, error) {
	amount := int(notif.Amount)
	switch notif.Action {
	case persist.ActionAdmiredFeedEvent:
		return model.SomeoneAdmiredYourFeedEventNotification{
			HelperSomeoneAdmiredYourFeedEventNotificationData: model.HelperSomeoneAdmiredYourFeedEventNotificationData{
				OwnerID:          notif.OwnerID,
				FeedEventID:      notif.FeedEventID,
				NotificationData: notif.Data,
			},
			Dbid:         notif.ID,
			Seen:         &notif.Seen,
			CreationTime: &notif.CreatedAt,
			UpdatedTime:  &notif.LastUpdated,
			Count:        &amount,
			FeedEvent:    nil, // handled by dedicated resolver
			Admirers:     nil, // handled by dedicated resolver
		}, nil
	case persist.ActionCommentedOnFeedEvent:
		return model.SomeoneCommentedOnYourFeedEventNotification{
			HelperSomeoneCommentedOnYourFeedEventNotificationData: model.HelperSomeoneCommentedOnYourFeedEventNotificationData{
				OwnerID:          notif.OwnerID,
				FeedEventID:      notif.FeedEventID,
				CommentID:        notif.CommentID,
				NotificationData: notif.Data,
			},
			Dbid:         notif.ID,
			Seen:         &notif.Seen,
			CreationTime: &notif.CreatedAt,
			UpdatedTime:  &notif.LastUpdated,
			FeedEvent:    nil, // handled by dedicated resolver
			Comment:      nil, // handled by dedicated resolver
		}, nil
	case persist.ActionAdmiredPost:
		return model.SomeoneAdmiredYourPostNotification{
			HelperSomeoneAdmiredYourPostNotificationData: model.HelperSomeoneAdmiredYourPostNotificationData{
				OwnerID:          notif.OwnerID,
				PostID:           notif.PostID,
				NotificationData: notif.Data,
			},
			Dbid:         notif.ID,
			Seen:         &notif.Seen,
			CreationTime: &notif.CreatedAt,
			UpdatedTime:  &notif.LastUpdated,
			Count:        &amount,
			Post:         nil, // handled by dedicated resolver
			Admirers:     nil, // handled by dedicated resolver
		}, nil
	case persist.ActionAdmiredToken:
		return model.SomeoneAdmiredYourTokenNotification{
			HelperSomeoneAdmiredYourTokenNotificationData: model.HelperSomeoneAdmiredYourTokenNotificationData{
				OwnerID:          notif.OwnerID,
				TokenID:          notif.TokenID,
				NotificationData: notif.Data,
			},
			Dbid:         notif.ID,
			Seen:         &notif.Seen,
			CreationTime: &notif.CreatedAt,
			UpdatedTime:  &notif.LastUpdated,
			Count:        &amount,
			Token:        nil, // handled by dedicated resolver
			Admirers:     nil, // handled by dedicated resolver
		}, nil
	case persist.ActionCommentedOnPost:
		return model.SomeoneCommentedOnYourPostNotification{
			HelperSomeoneCommentedOnYourPostNotificationData: model.HelperSomeoneCommentedOnYourPostNotificationData{
				OwnerID:          notif.OwnerID,
				PostID:           notif.PostID,
				CommentID:        notif.CommentID,
				NotificationData: notif.Data,
			},
			Dbid:         notif.ID,
			Seen:         &notif.Seen,
			CreationTime: &notif.CreatedAt,
			UpdatedTime:  &notif.LastUpdated,
			Post:         nil, // handled by dedicated resolver
			Comment:      nil, // handled by dedicated resolver
		}, nil
	case persist.ActionUserFollowedUsers:
		if !notif.Data.FollowedBack {
			return model.SomeoneFollowedYouNotification{
				HelperSomeoneFollowedYouNotificationData: model.HelperSomeoneFollowedYouNotificationData{
					OwnerID:          notif.OwnerID,
					NotificationData: notif.Data,
				},
				Dbid:         notif.ID,
				Seen:         &notif.Seen,
				CreationTime: &notif.CreatedAt,
				UpdatedTime:  &notif.LastUpdated,
				Count:        &amount,
				Followers:    nil, // handled by dedicated resolver
			}, nil
		}
		return model.SomeoneFollowedYouBackNotification{
			HelperSomeoneFollowedYouBackNotificationData: model.HelperSomeoneFollowedYouBackNotificationData{
				OwnerID:          notif.OwnerID,
				NotificationData: notif.Data,
			},
			Dbid:         notif.ID,
			Seen:         &notif.Seen,
			CreationTime: &notif.CreatedAt,
			UpdatedTime:  &notif.LastUpdated,
			Count:        &amount,
			Followers:    nil, // handled by dedicated resolver
		}, nil
	case persist.ActionViewedGallery:
		nonCount := len(notif.Data.UnauthedViewerIDs)
		return model.SomeoneViewedYourGalleryNotification{
			HelperSomeoneViewedYourGalleryNotificationData: model.HelperSomeoneViewedYourGalleryNotificationData{
				OwnerID:          notif.OwnerID,
				GalleryID:        notif.GalleryID,
				NotificationData: notif.Data,
			},
			Dbid:               notif.ID,
			Seen:               &notif.Seen,
			CreationTime:       &notif.CreatedAt,
			UpdatedTime:        &notif.LastUpdated,
			Count:              &amount,
			UserViewers:        nil, // handled by dedicated resolver
			Gallery:            nil, // handled by dedicated resolver
			NonUserViewerCount: &nonCount,
		}, nil
	case persist.ActionNewTokensReceived:
		return model.NewTokensNotification{
			HelperNewTokensNotificationData: model.HelperNewTokensNotificationData{
				OwnerID:          notif.OwnerID,
				NotificationData: notif.Data,
			},
			Dbid:         notif.ID,
			Seen:         &notif.Seen,
			CreationTime: &notif.CreatedAt,
			UpdatedTime:  &notif.LastUpdated,
			Count:        &amount,
			Token:        nil, // handled by dedicated resolver
		}, nil
	case persist.ActionReplyToComment:
		return model.SomeoneRepliedToYourCommentNotification{
			HelperSomeoneRepliedToYourCommentNotificationData: model.HelperSomeoneRepliedToYourCommentNotificationData{
				OwnerID:          notif.OwnerID,
				CommentID:        notif.CommentID,
				NotificationData: notif.Data,
			},
			Dbid:            notif.ID,
			Seen:            &notif.Seen,
			CreationTime:    &notif.CreatedAt,
			UpdatedTime:     &notif.LastUpdated,
			Comment:         nil, // handled by dedicated resolver
			OriginalComment: nil, // handled by dedicated resolver
		}, nil
	case persist.ActionMentionUser:
		var postID *persist.DBID
		var commentID *persist.DBID

		if notif.PostID != "" {
			postID = &notif.PostID
		}
		if notif.CommentID != "" {
			commentID = &notif.CommentID
		}
		return model.SomeoneMentionedYouNotification{
			HelperSomeoneMentionedYouNotificationData: model.HelperSomeoneMentionedYouNotificationData{

				PostID:    postID,
				CommentID: commentID,
			},
			Dbid:          notif.ID,
			Seen:          &notif.Seen,
			CreationTime:  &notif.CreatedAt,
			UpdatedTime:   &notif.LastUpdated,
			MentionSource: nil, // handled by dedicated resolver
		}, nil

	case persist.ActionMentionCommunity:
		var postID *persist.DBID
		var commentID *persist.DBID

		if notif.PostID != "" {
			postID = &notif.PostID
		}
		if notif.CommentID != "" {
			commentID = &notif.CommentID
		}
		return model.SomeoneMentionedYourCommunityNotification{
			HelperSomeoneMentionedYourCommunityNotificationData: model.HelperSomeoneMentionedYourCommunityNotificationData{

				ContractID: notif.ContractID,
				PostID:     postID,
				CommentID:  commentID,
			},
			Dbid:          notif.ID,
			Seen:          &notif.Seen,
			CreationTime:  &notif.CreatedAt,
			UpdatedTime:   &notif.LastUpdated,
			Community:     nil, // handled by dedicated resolver
			MentionSource: nil, // handled by dedicated resolver
		}, nil
	case persist.ActionUserPostedYourWork:
		return model.SomeonePostedYourWorkNotification{
			HelperSomeonePostedYourWorkNotificationData: model.HelperSomeonePostedYourWorkNotificationData{

				ContractID: notif.ContractID,
				PostID:     notif.PostID,
			},
			Dbid:         notif.ID,
			Seen:         &notif.Seen,
			CreationTime: &notif.CreatedAt,
			UpdatedTime:  &notif.LastUpdated,
			Community:    nil, // handled by dedicated resolver
			Post:         nil, // handled by dedicated resolver
		}, nil
	case persist.ActionUserPostedFirstPost:
		return model.SomeoneYouFollowPostedTheirFirstPostNotification{
			HelperSomeoneYouFollowPostedTheirFirstPostNotificationData: model.HelperSomeoneYouFollowPostedTheirFirstPostNotificationData{
				PostID: notif.PostID,
			},
			Dbid:         notif.ID,
			Seen:         &notif.Seen,
			CreationTime: &notif.CreatedAt,
			UpdatedTime:  &notif.LastUpdated,
			Post:         nil, // handled by dedicated resolver
		}, nil

	default:
		return nil, fmt.Errorf("unknown notification action: %s", notif.Action)
	}
}

func resolveViewerNotificationSettings(ctx context.Context) (*model.NotificationSettings, error) {

	userID := publicapi.For(ctx).User.GetLoggedInUserId(ctx)

	user, err := publicapi.For(ctx).User.GetUserById(ctx, userID)

	if err != nil {
		return nil, err
	}

	return notificationSettingsToModel(ctx, user), nil

}

func notificationSettingsToModel(ctx context.Context, user *db.User) *model.NotificationSettings {
	settings := user.NotificationSettings
	return &model.NotificationSettings{
		SomeoneFollowedYou:           settings.SomeoneFollowedYou,
		SomeoneAdmiredYourUpdate:     settings.SomeoneAdmiredYourUpdate,
		SomeoneCommentedOnYourUpdate: settings.SomeoneCommentedOnYourUpdate,
		SomeoneViewedYourGallery:     settings.SomeoneViewedYourGallery,
	}
}

func resolveNewNotificationSubscription(ctx context.Context) <-chan model.Notification {
	userID := publicapi.For(ctx).User.GetLoggedInUserId(ctx)
	notifDispatcher := notifications.For(ctx)
	notifs := notifDispatcher.GetNewNotificationsForUser(userID)
	logger.For(ctx).Info("new notification subscription for ", userID)

	result := make(chan model.Notification)

	go func() {
		for notif := range notifs {
			// use async to prevent blocking the dispatcher
			asModel, err := notificationToModel(notif)
			if err != nil {
				logger.For(nil).Errorf("error converting notification to model: %v", err)
				return
			}
			select {
			case result <- asModel:
				logger.For(nil).Debug("sent new notification to subscription")
			default:
				logger.For(nil).Errorf("notification subscription channel full, dropping notification")
				notifDispatcher.UnsubscribeNewNotificationsForUser(userID)
			}
		}
	}()

	return result
}

func resolveUpdatedNotificationSubscription(ctx context.Context) <-chan model.Notification {
	userID := publicapi.For(ctx).User.GetLoggedInUserId(ctx)
	notifDispatcher := notifications.For(ctx)
	notifs := notifDispatcher.GetUpdatedNotificationsForUser(userID)

	result := make(chan model.Notification)

	wp := workerpool.New(10)

	go func() {
		for notif := range notifs {
			n := notif
			wp.Submit(func() {
				asModel, err := notificationToModel(n)
				if err != nil {
					logger.For(nil).Errorf("error converting notification to model: %v", err)
					return
				}
				select {
				case result <- asModel:
					logger.For(nil).Debug("sent updated notification to subscription")
				default:
					logger.For(nil).Errorf("notification subscription channel full, dropping notification")
					notifDispatcher.UnsubscribeUpdatedNotificationsForUser(userID)
				}
			})
		}
		wp.StopWait()
	}()

	return result
}

func resolveGroupNotificationUsersConnectionByUserIDs(ctx context.Context, userIDs persist.DBIDList, before *string, after *string, first *int, last *int) (*model.GroupNotificationUsersConnection, error) {
	if len(userIDs) == 0 {
		return &model.GroupNotificationUsersConnection{
			Edges:    []*model.GroupNotificationUserEdge{},
			PageInfo: &model.PageInfo{},
		}, nil
	}
	users, pageInfo, err := publicapi.For(ctx).User.GetUsersByIDs(ctx, userIDs, before, after, first, last)
	if err != nil {
		return nil, err
	}

	edges := make([]*model.GroupNotificationUserEdge, len(users))

	for i, user := range users {
		edges[i] = &model.GroupNotificationUserEdge{
			Node:   userToModel(ctx, user),
			Cursor: nil,
		}
	}

	return &model.GroupNotificationUsersConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil
}

func resolveFeedEventDataByEventID(ctx context.Context, eventID persist.DBID) (model.FeedEventData, error) {
	event, err := publicapi.For(ctx).Feed.GetFeedEventById(ctx, eventID)

	if err != nil {
		return nil, err
	}

	return feedEventToDataModel(event)
}

func resolveCollectionTokensByTokenIDs(ctx context.Context, collectionID persist.DBID, tokenIDs persist.DBIDList) ([]*model.CollectionToken, error) {
	tokens, err := publicapi.For(ctx).Token.GetTokensByIDs(ctx, tokenIDs)
	if err != nil {
		return nil, err
	}

	newTokens := make([]*model.CollectionToken, len(tokenIDs))

	tokenIDToPosition := make(map[persist.DBID]int)
	for i, tokenID := range tokenIDs {
		tokenIDToPosition[tokenID] = i
	}

	// Fill in the data for tokens that still exist.
	// Tokens that have since been deleted will be nil.
	for _, t := range tokens {
		token := tokenToModel(ctx, t, &collectionID)
		newTokens[tokenIDToPosition[t.ID]] = collectionTokenToModel(ctx, token, collectionID)
	}

	return newTokens, nil
}

func resolveTokenSettingsByIDs(ctx context.Context, tokenID, collectionID persist.DBID) (*model.CollectionTokenSettings, error) {
	collection, err := publicapi.For(ctx).Collection.GetCollectionById(ctx, collectionID)

	if err != nil {
		return nil, err
	}

	if settings, ok := collection.TokenSettings[tokenID]; ok {
		return &model.CollectionTokenSettings{RenderLive: &settings.RenderLive, HighDefinition: &settings.HighDefinition}, nil
	}

	return &model.CollectionTokenSettings{RenderLive: &defaultTokenSettings.RenderLive, HighDefinition: &defaultTokenSettings.HighDefinition}, nil
}

func resolveNotificationByID(ctx context.Context, id persist.DBID) (model.Notification, error) {
	notification, err := publicapi.For(ctx).Notifications.GetByID(ctx, id)

	if err != nil {
		return nil, err
	}

	return notificationToModel(notification)
}

func resolveAdmireByAdmireID(ctx context.Context, admireID persist.DBID) (*model.Admire, error) {
	admire, err := publicapi.For(ctx).Interaction.GetAdmireByID(ctx, admireID)

	if err != nil {
		return nil, err
	}

	return admireToModel(ctx, *admire), nil
}

func resolveCommentByCommentID(ctx context.Context, commentID persist.DBID) (*model.Comment, error) {
	comment, err := publicapi.For(ctx).Interaction.GetCommentByID(ctx, commentID)

	if err != nil {
		return nil, err
	}

	return commentToModel(ctx, *comment), nil
}

func resolveMerchTokenByTokenID(ctx context.Context, tokenID string) (*model.MerchToken, error) {
	token, err := publicapi.For(ctx).Merch.GetMerchTokenByTokenID(ctx, persist.TokenID(tokenID))

	if err != nil {
		return nil, err
	}

	return token, nil
}

func resolveViewerByID(ctx context.Context, id string) (*model.Viewer, error) {

	if !publicapi.For(ctx).User.IsUserLoggedIn(ctx) {
		return nil, nil
	}
	userID := publicapi.For(ctx).User.GetLoggedInUserId(ctx)

	if userID.String() != id {
		return nil, nil
	}

	return &model.Viewer{
		HelperViewerData: model.HelperViewerData{
			UserId: userID,
		},
		User:            nil, // handled by dedicated resolver
		ViewerGalleries: nil, // handled by dedicated resolver
	}, nil
}

func resolveDeletedNodeByID(ctx context.Context, id persist.DBID) (*model.DeletedNode, error) {
	return &model.DeletedNode{
		Dbid: id,
	}, nil
}

func resolveSocialConnectionByIdentifiers(ctx context.Context, socialId string, socialType persist.SocialProvider) (*model.SocialConnection, error) {
	return &model.SocialConnection{
		SocialID:   socialId,
		SocialType: socialType,
	}, nil
}

func verifyEmail(ctx context.Context, token string) (*model.VerifyEmailPayload, error) {
	output, err := emails.VerifyEmail(ctx, token)
	if err != nil {
		return nil, err
	}

	return &model.VerifyEmailPayload{
		Email: output.Email,
	}, nil

}

func updateUserEmail(ctx context.Context, email persist.Email) (*model.UpdateEmailPayload, error) {
	err := publicapi.For(ctx).User.UpdateUserEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	return &model.UpdateEmailPayload{
		Viewer: resolveViewer(ctx),
	}, nil

}

func resendEmailVerification(ctx context.Context) (*model.ResendVerificationEmailPayload, error) {
	err := publicapi.For(ctx).User.ResendEmailVerification(ctx)
	if err != nil {
		return nil, err
	}

	return &model.ResendVerificationEmailPayload{
		Viewer: resolveViewer(ctx),
	}, nil

}

func updateUserEmailNotificationSettings(ctx context.Context, input model.UpdateEmailNotificationSettingsInput) (*model.UpdateEmailNotificationSettingsPayload, error) {
	err := publicapi.For(ctx).User.UpdateUserEmailNotificationSettings(ctx, persist.EmailUnsubscriptions{
		All:           persist.NullBool(input.UnsubscribedFromAll),
		Notifications: persist.NullBool(input.UnsubscribedFromNotifications),
	})
	if err != nil {
		return nil, err
	}

	return &model.UpdateEmailNotificationSettingsPayload{
		Viewer: resolveViewer(ctx),
	}, nil

}

func unsubscribeFromEmailType(ctx context.Context, input model.UnsubscribeFromEmailTypeInput) (*model.UnsubscribeFromEmailTypePayload, error) {

	if err := emails.UnsubscribeByJWT(ctx, input.Token, []model.EmailUnsubscriptionType{input.Type}); err != nil {
		return nil, err
	}

	return &model.UnsubscribeFromEmailTypePayload{
		Viewer: resolveViewer(ctx),
	}, nil

}

func feedEventToDataModel(event *db.FeedEvent) (model.FeedEventData, error) {
	switch event.Action {
	case persist.ActionUserCreated:
		return feedEventToUserCreatedFeedEventData(event), nil
	case persist.ActionUserFollowedUsers:
		return feedEventToUserFollowedUsersFeedEventData(event), nil
	case persist.ActionCollectorsNoteAddedToToken:
		return feedEventToCollectorsNoteAddedToTokenFeedEventData(event), nil
	case persist.ActionCollectionCreated:
		return feedEventToCollectionCreatedFeedEventData(event), nil
	case persist.ActionCollectorsNoteAddedToCollection:
		return feedEventToCollectorsNoteAddedToCollectionFeedEventData(event), nil
	case persist.ActionTokensAddedToCollection:
		return feedEventToTokensAddedToCollectionFeedEventData(event), nil
	case persist.ActionCollectionUpdated:
		return feedEventToCollectionUpdatedFeedEventData(event), nil
	case persist.ActionGalleryUpdated:
		return feedEventToGalleryUpdatedFeedEventData(event), nil
	default:
		return nil, persist.ErrUnknownAction{Action: event.Action}
	}
}

func feedEntityToModel(event any) (model.FeedEventOrError, error) {
	// Value always returns a nil error so we can safely ignore it.

	switch event := event.(type) {
	case db.Post:
		caption, _ := event.Caption.Value()

		var captionVal *string
		if caption != nil {
			captionVal = util.ToPointer(caption.(string))
		}
		return &model.Post{
			HelperPostData: model.HelperPostData{
				TokenIDs: event.TokenIds,
				AuthorID: event.ActorID,
			},
			CreationTime: &event.CreatedAt,
			Dbid:         event.ID,
			Caption:      captionVal,
		}, nil
	case db.FeedEvent:
		var groupID sql.NullString
		if event.GroupID.String != "" {
			groupID = sql.NullString{
				String: event.GroupID.String,
				Valid:  true,
			}
		}
		data, err := feedEventToDataModel(&db.FeedEvent{
			ID:          event.ID,
			Version:     event.Version,
			OwnerID:     event.OwnerID,
			Action:      event.Action,
			Data:        event.Data,
			EventTime:   event.EventTime,
			EventIds:    event.EventIds,
			Deleted:     event.Deleted,
			LastUpdated: event.LastUpdated,
			CreatedAt:   event.CreatedAt,
			Caption:     event.Caption,
			GroupID:     groupID,
		})
		if err != nil {
			return nil, err
		}

		caption, _ := event.Caption.Value()

		var captionVal *string
		if caption != nil {
			captionVal = util.ToPointer(caption.(string))
		}

		return &model.FeedEvent{
			Dbid:      event.ID,
			Caption:   captionVal,
			EventData: data,
		}, nil
	default:
		panic(fmt.Sprintf("unknown type: %T", event))
	}
}

func feedEventToModel(event *db.FeedEvent) (*model.FeedEvent, error) {
	// Value always returns a nil error so we can safely ignore it.
	caption, _ := event.Caption.Value()

	var captionVal *string
	if caption != nil {
		captionVal = util.ToPointer(caption.(string))
	}

	data, err := feedEventToDataModel(event)
	if err != nil {
		return nil, err
	}

	return &model.FeedEvent{
		Dbid:      event.ID,
		Caption:   captionVal,
		EventData: data,
	}, nil

}

func feedEventToUserCreatedFeedEventData(event *db.FeedEvent) model.FeedEventData {
	return model.UserCreatedFeedEventData{
		EventTime: &event.EventTime,
		Owner:     &model.GalleryUser{Dbid: event.OwnerID}, // remaining fields handled by dedicated resolver
		Action:    &event.Action,
	}
}

func feedEventToUserFollowedUsersFeedEventData(event *db.FeedEvent) model.FeedEventData {
	followed := make([]*model.FollowInfo, len(event.Data.UserFollowedIDs))

	for i, userID := range event.Data.UserFollowedIDs {
		followed[i] = &model.FollowInfo{
			User:         &model.GalleryUser{Dbid: userID}, // remaining fields handled by dedicated resolver
			FollowedBack: util.ToPointer(event.Data.UserFollowedBack[i]),
		}
	}

	return model.UserFollowedUsersFeedEventData{
		EventTime: &event.EventTime,
		Owner:     &model.GalleryUser{Dbid: event.OwnerID}, // remaining fields handled by dedicated resolver
		Action:    &event.Action,
		Followed:  followed,
	}
}

func feedEventToCollectorsNoteAddedToTokenFeedEventData(event *db.FeedEvent) model.FeedEventData {
	return model.CollectorsNoteAddedToTokenFeedEventData{
		EventTime:         &event.EventTime,
		Owner:             &model.GalleryUser{Dbid: event.OwnerID}, // remaining fields handled by dedicated resolver
		Token:             &model.CollectionToken{Token: &model.Token{Dbid: event.Data.TokenID, HelperTokenData: model.HelperTokenData{CollectionID: (*persist.DBID)(util.StringToPointerIfNotEmpty(string(event.Data.TokenCollectionID)))}}, Collection: &model.Collection{Dbid: event.Data.TokenCollectionID}, HelperCollectionTokenData: model.HelperCollectionTokenData{TokenId: event.Data.TokenID, CollectionId: event.Data.TokenCollectionID}},
		Action:            &event.Action,
		NewCollectorsNote: util.ToPointer(event.Data.TokenNewCollectorsNote),
	}
}

func feedEventToCollectionCreatedFeedEventData(event *db.FeedEvent) model.FeedEventData {
	return model.CollectionCreatedFeedEventData{
		EventTime:  &event.EventTime,
		Owner:      &model.GalleryUser{Dbid: event.OwnerID},          // remaining fields handled by dedicated resolver
		Collection: &model.Collection{Dbid: event.Data.CollectionID}, // remaining fields handled by dedicated resolver
		Action:     &event.Action,
		NewTokens:  nil, // handled by dedicated resolver
		HelperCollectionCreatedFeedEventDataData: model.HelperCollectionCreatedFeedEventDataData{
			TokenIDs:     event.Data.CollectionTokenIDs,
			CollectionID: event.Data.CollectionID,
		},
	}
}

func feedEventToCollectorsNoteAddedToCollectionFeedEventData(event *db.FeedEvent) model.FeedEventData {
	return model.CollectorsNoteAddedToCollectionFeedEventData{
		EventTime:         &event.EventTime,
		Owner:             &model.GalleryUser{Dbid: event.OwnerID},          // remaining fields handled by dedicated resolver
		Collection:        &model.Collection{Dbid: event.Data.CollectionID}, // remaining fields handled by dedicated resolver
		Action:            &event.Action,
		NewCollectorsNote: util.ToPointer(event.Data.CollectionNewCollectorsNote),
	}
}

func feedEventToTokensAddedToCollectionFeedEventData(event *db.FeedEvent) model.FeedEventData {
	return model.TokensAddedToCollectionFeedEventData{
		EventTime:  &event.EventTime,
		Owner:      &model.GalleryUser{Dbid: event.OwnerID},          // remaining fields handled by dedicated resolver
		Collection: &model.Collection{Dbid: event.Data.CollectionID}, // remaining fields handled by dedicated resolver
		Action:     &event.Action,
		NewTokens:  nil, // handled by dedicated resolver
		IsPreFeed:  util.ToPointer(event.Data.CollectionIsPreFeed),
		HelperTokensAddedToCollectionFeedEventDataData: model.HelperTokensAddedToCollectionFeedEventDataData{
			TokenIDs:     event.Data.CollectionTokenIDs,
			CollectionID: event.Data.CollectionID,
		},
	}
}

func feedEventToCollectionUpdatedFeedEventData(event *db.FeedEvent) model.FeedEventData {
	return model.CollectionUpdatedFeedEventData{
		EventTime:         &event.EventTime,
		Owner:             &model.GalleryUser{Dbid: event.OwnerID},          // remaining fields handled by dedicated resolver
		Collection:        &model.Collection{Dbid: event.Data.CollectionID}, // remaining fields handled by dedicated resolver
		Action:            &event.Action,
		NewTokens:         nil, // handled by dedicated resolver
		NewCollectorsNote: util.ToPointer(event.Data.CollectionNewCollectorsNote),
		HelperCollectionUpdatedFeedEventDataData: model.HelperCollectionUpdatedFeedEventDataData{
			TokenIDs:     event.Data.CollectionTokenIDs,
			CollectionID: event.Data.CollectionID,
		},
	}
}

func feedEventToGalleryUpdatedFeedEventData(event *db.FeedEvent) model.FeedEventData {

	return model.GalleryUpdatedFeedEventData{
		EventTime:      &event.EventTime,
		Owner:          &model.GalleryUser{Dbid: event.OwnerID},    // remaining fields handled by dedicated resolver
		Gallery:        &model.Gallery{Dbid: event.Data.GalleryID}, // remaining fields handled by dedicated resolver
		Action:         &event.Action,
		SubEventDatas:  nil, // handled by dedicated resolver
		NewName:        util.StringToPointerIfNotEmpty(event.Data.GalleryName),
		NewDescription: util.StringToPointerIfNotEmpty(event.Data.GalleryDescription),
		HelperGalleryUpdatedFeedEventDataData: model.HelperGalleryUpdatedFeedEventDataData{
			FeedEventID: event.ID,
		},
	}
}

func resolveSubEventDatasByFeedEventID(ctx context.Context, feedEventID persist.DBID) ([]model.FeedEventData, error) {
	feedEvent, err := publicapi.For(ctx).Feed.GetFeedEventById(ctx, feedEventID)
	if err != nil {
		return nil, err
	}

	return feedEventToSubEventDatas(ctx, *feedEvent)

}

func feedEventToSubEventDatas(ctx context.Context, event db.FeedEvent) ([]model.FeedEventData, error) {
	result := make([]model.FeedEventData, 0, 5)
	if event.Data.GalleryName != "" || event.Data.GalleryDescription != "" {
		result = append(result, model.GalleryInfoUpdatedFeedEventData{
			EventTime:      &event.CreatedAt,
			Owner:          &model.GalleryUser{Dbid: persist.DBID(event.OwnerID)}, // remaining fields handled by dedicated resolver
			Action:         util.ToPointer(persist.ActionGalleryInfoUpdated),
			NewName:        util.StringToPointerIfNotEmpty(event.Data.GalleryName),
			NewDescription: util.StringToPointerIfNotEmpty(event.Data.GalleryDescription),
		})
	}

	handledNew := make(map[persist.DBID]bool)

	if event.Data.GalleryNewCollections != nil && len(event.Data.GalleryNewCollections) > 0 {
		for _, collectionID := range event.Data.GalleryNewCollections {
			var collectorsNote *string
			if note, ok := event.Data.GalleryNewCollectionCollectorsNotes[collectionID]; ok {
				collectorsNote = &note
			}
			handledNew[collectionID] = true
			if collectorsNote == nil && (event.Data.GalleryNewCollectionTokenIDs[collectionID] == nil || len(event.Data.GalleryNewCollectionTokenIDs[collectionID]) == 0) {
				continue
			}
			result = append(result, model.CollectionCreatedFeedEventData{
				EventTime:         &event.CreatedAt,
				Owner:             &model.GalleryUser{Dbid: persist.DBID(event.OwnerID)}, // remaining fields handled by dedicated resolver
				Collection:        &model.Collection{Dbid: collectionID},                 // remaining fields handled by dedicated resolver
				Action:            util.ToPointer(persist.ActionCollectionCreated),
				NewTokens:         nil, // handled by dedicated resolver
				NewCollectorsNote: collectorsNote,
				HelperCollectionCreatedFeedEventDataData: model.HelperCollectionCreatedFeedEventDataData{
					CollectionID: collectionID,
					TokenIDs:     event.Data.GalleryNewCollectionTokenIDs[collectionID],
				},
			})

		}
	}

	if event.Data.GalleryNewCollectionTokenIDs != nil && len(event.Data.GalleryNewCollectionTokenIDs) > 0 {
		for collectionID, tokenIDs := range event.Data.GalleryNewCollectionTokenIDs {
			if handledNew[collectionID] {
				continue
			}
			result = append(result, model.TokensAddedToCollectionFeedEventData{
				EventTime:  &event.CreatedAt,
				Owner:      &model.GalleryUser{Dbid: persist.DBID(event.OwnerID)}, // remaining fields handled by dedicated resolver
				Collection: &model.Collection{Dbid: collectionID},                 // remaining fields handled by dedicated resolver
				Action:     util.ToPointer(persist.ActionCollectionUpdated),
				NewTokens:  nil, // handled by dedicated resolver
				HelperTokensAddedToCollectionFeedEventDataData: model.HelperTokensAddedToCollectionFeedEventDataData{
					TokenIDs:     tokenIDs,
					CollectionID: collectionID,
				},
			})
		}
	}

	if event.Data.GalleryNewCollectionCollectorsNotes != nil && len(event.Data.GalleryNewCollectionCollectorsNotes) > 0 {
		for collectionID, collectorsNote := range event.Data.GalleryNewCollectionCollectorsNotes {
			if handledNew[collectionID] {
				continue
			}
			result = append(result, model.CollectorsNoteAddedToCollectionFeedEventData{
				EventTime:         &event.CreatedAt,
				Owner:             &model.GalleryUser{Dbid: persist.DBID(event.OwnerID)}, // remaining fields handled by dedicated resolver
				Collection:        &model.Collection{Dbid: collectionID},                 // remaining fields handled by dedicated resolver
				Action:            util.ToPointer(persist.ActionCollectionUpdated),
				NewCollectorsNote: util.StringToPointerIfNotEmpty(collectorsNote),
			})
		}
	}

	if event.Data.GalleryNewCollectionTokenCollectorsNotes != nil && len(event.Data.GalleryNewCollectionTokenCollectorsNotes) > 0 {
		for collectionID, newNotes := range event.Data.GalleryNewCollectionTokenCollectorsNotes {
			for tokenID, note := range newNotes {
				result = append(result, model.CollectorsNoteAddedToTokenFeedEventData{
					EventTime: &event.CreatedAt,
					Owner:     &model.GalleryUser{Dbid: persist.DBID(event.OwnerID)}, // remaining fields handled by dedicated resolver
					Token: &model.CollectionToken{Token: &model.Token{Dbid: tokenID, HelperTokenData: model.HelperTokenData{CollectionID: (*persist.DBID)(util.StringToPointerIfNotEmpty(string(collectionID)))}}, Collection: &model.Collection{Dbid: collectionID}, HelperCollectionTokenData: model.HelperCollectionTokenData{
						TokenId:      tokenID,
						CollectionId: collectionID,
					}}, // remaining fields handled by dedicated resolver
					Action:            util.ToPointer(persist.ActionCollectorsNoteAddedToToken),
					NewCollectorsNote: util.StringToPointerIfNotEmpty(note),
				})
			}
		}
	}

	return result, nil
}

func entitiesToFeedEdges(events []any) ([]*model.FeedEdge, error) {
	edges := make([]*model.FeedEdge, len(events))

	for i, evt := range events {
		var node model.FeedEventOrError
		node, err := feedEntityToModel(evt)

		if e, ok := err.(*persist.ErrUnknownAction); ok {
			node = model.ErrUnknownAction{Message: e.Error()}
		} else if err != nil {
			return nil, err
		}

		edges[i] = &model.FeedEdge{Node: node}
	}

	return edges, nil
}

func postToModel(event *db.Post) (*model.Post, error) {
	// Value always returns a nil error so we can safely ignore it.
	caption, _ := event.Caption.Value()

	var captionVal *string
	if caption != nil {
		captionVal = util.ToPointer(html.UnescapeString(caption.(string)))
	}

	return &model.Post{
		HelperPostData: model.HelperPostData{
			TokenIDs: event.TokenIds,
			AuthorID: event.ActorID,
		},
		Dbid:         event.ID,
		CreationTime: &event.CreatedAt,
		Caption:      captionVal,
	}, nil

}

func galleryToModel(ctx context.Context, gallery db.Gallery) *model.Gallery {

	return &model.Gallery{
		Dbid:        gallery.ID,
		Name:        &gallery.Name,
		Description: &gallery.Description,
		Position:    &gallery.Position,
		Hidden:      &gallery.Hidden,
		Owner:       nil, // handled by dedicated resolver
		Collections: nil, // handled by dedicated resolver
	}
}

func layoutToModel(ctx context.Context, layout persist.TokenLayout, version int) *model.CollectionLayout {
	if version == 0 {
		// Some older collections predate configurable columns; the default back then was 3
		if layout.Columns == 0 {
			layout.Columns = 3
		}

		// Treat the original collection as a single section.
		return &model.CollectionLayout{
			Sections: []*int{util.ToPointer(0)},
			SectionLayout: []*model.CollectionSectionLayout{
				{
					Columns:    util.ToPointer(layout.Columns),
					Whitespace: util.ToPointerSlice(layout.Whitespace),
				},
			},
		}
	}

	layouts := make([]*model.CollectionSectionLayout, 0)
	for _, l := range layout.SectionLayout {
		layouts = append(layouts, &model.CollectionSectionLayout{
			Columns:    util.ToPointer(l.Columns.Int()),
			Whitespace: util.ToPointerSlice(l.Whitespace),
		})
	}

	return &model.CollectionLayout{
		Sections:      util.ToPointerSlice(layout.Sections),
		SectionLayout: layouts,
	}
}

// userToModel converts a db.User to a model.User
func userToModel(ctx context.Context, user db.User) *model.GalleryUser {
	userApi := publicapi.For(ctx).User
	isAuthenticatedUser := userApi.IsUserLoggedIn(ctx) && userApi.GetLoggedInUserId(ctx) == user.ID

	wallets := make([]*model.Wallet, len(user.Wallets))
	for i, wallet := range user.Wallets {
		wallets[i] = walletToModelPersist(ctx, wallet)
	}

	var traits persist.Traits
	user.Traits.AssignTo(&traits)

	return &model.GalleryUser{
		HelperGalleryUserData: model.HelperGalleryUserData{
			UserID:            user.ID,
			FeaturedGalleryID: user.FeaturedGallery,
			Traits:            traits,
		},
		Dbid:      user.ID,
		Username:  &user.Username.String,
		Bio:       util.ToPointer(html.UnescapeString(user.Bio.String)),
		Wallets:   wallets,
		Universal: &user.Universal,

		// each handled by dedicated resolver
		Galleries: nil,
		Followers: nil,
		Following: nil,
		Tokens:    nil,
		Badges:    nil,
		Roles:     nil,

		IsAuthenticatedUser: &isAuthenticatedUser,
	}
}

func usersToEdges(ctx context.Context, users []db.User) []*model.UserEdge {
	edges := make([]*model.UserEdge, len(users))
	for i, user := range users {
		edges[i] = &model.UserEdge{
			Node:   userToModel(ctx, user),
			Cursor: nil, // not used by relay, but relay will complain without this field existing
		}
	}
	return edges
}

// admireToModel converts a db.Admire to a model.Admire
func admireToModel(ctx context.Context, admire db.Admire) *model.Admire {

	var postID, feedEventID *persist.DBID
	if admire.PostID != "" {
		postID = &admire.PostID
	}
	if admire.FeedEventID != "" {
		feedEventID = &admire.FeedEventID
	}

	return &model.Admire{
		HelperAdmireData: model.HelperAdmireData{
			PostID:      postID,
			FeedEventID: feedEventID,
		},
		Dbid:         admire.ID,
		CreationTime: &admire.CreatedAt,
		LastUpdated:  &admire.LastUpdated,
		Admirer:      &model.GalleryUser{Dbid: admire.ActorID}, // remaining fields handled by dedicated resolver
	}
}

// commentToModel converts a db.Admire to a model.Admire
func commentToModel(ctx context.Context, comment db.Comment) *model.Comment {

	var postID, feedEventID, replyToID *persist.DBID
	if comment.PostID != "" {
		postID = &comment.PostID
	}
	if comment.FeedEventID != "" {
		feedEventID = &comment.FeedEventID
	}
	if comment.ReplyTo != "" {
		replyToID = &comment.ReplyTo
	}
	return &model.Comment{
		HelperCommentData: model.HelperCommentData{
			PostID:      postID,
			FeedEventID: feedEventID,
			ReplyToID:   replyToID,
		},
		Dbid:         comment.ID,
		CreationTime: &comment.CreatedAt,
		LastUpdated:  &comment.LastUpdated,
		Comment:      util.ToPointer(html.UnescapeString(comment.Comment)),
		Commenter:    &model.GalleryUser{Dbid: comment.ActorID}, // remaining fields handled by dedicated resolver
		ReplyTo:      nil,                                       // handled by dedicated resolver
		Replies:      nil,                                       // handled by dedicated resolver
		Source:       nil,                                       // handled by dedicated resolver
		Deleted:      &comment.Removed,
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

func walletToModelSqlc(ctx context.Context, wallet db.Wallet) *model.Wallet {
	chain := wallet.Chain
	chainAddress := persist.NewChainAddress(wallet.Address, chain)

	return &model.Wallet{
		Dbid:         wallet.ID,
		WalletType:   &wallet.WalletType,
		ChainAddress: &chainAddress,
		Chain:        &wallet.Chain,
		Tokens:       nil, // handled by dedicated resolver
	}
}

func contractToModel(ctx context.Context, contract db.Contract) *model.Contract {
	chain := contract.Chain
	addr := persist.NewChainAddress(contract.Address, chain)
	creatorAddress, _ := util.FindFirst([]persist.Address{contract.OwnerAddress, contract.CreatorAddress}, func(a persist.Address) bool {
		return a != ""
	})
	creator := persist.NewChainAddress(creatorAddress, chain)

	return &model.Contract{
		Dbid:             contract.ID,
		ContractAddress:  &addr,
		CreatorAddress:   &creator,
		Chain:            &contract.Chain,
		Name:             &contract.Name.String,
		LastUpdated:      &contract.LastUpdated,
		ProfileImageURL:  &contract.ProfileImageUrl.String,
		ProfileBannerURL: &contract.ProfileBannerUrl.String,
		BadgeURL:         &contract.BadgeUrl.String,
		IsSpam:           &contract.IsProviderMarkedSpam,
	}
}

func contractToBadgeModel(ctx context.Context, contract db.Contract) *model.Badge {
	return &model.Badge{
		Contract: contractToModel(ctx, contract),
		Name:     &contract.Name.String,
		ImageURL: contract.BadgeUrl.String,
	}
}
func collectionToModel(ctx context.Context, collection db.Collection) *model.Collection {
	version := int(collection.Version.Int32)

	return &model.Collection{
		Dbid:           collection.ID,
		Version:        &version,
		Name:           &collection.Name.String,
		CollectorsNote: &collection.CollectorsNote.String,
		Gallery:        nil, // handled by dedicated resolver
		Layout:         layoutToModel(ctx, collection.Layout, version),
		Hidden:         &collection.Hidden,
		Tokens:         nil, // handled by dedicated resolver
	}
}

func membershipToModel(ctx context.Context, membershipTier db.Membership) *model.MembershipTier {
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
		TokenID:  util.ToPointer(membershipTier.TokenID.String()),
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
		Name:     util.ToPointer(membershipTier.Name.String()),
		AssetURL: util.ToPointer(membershipTier.AssetURL.String()),
		TokenID:  util.ToPointer(membershipTier.TokenID.String()),
		Owners:   owners,
	}
}

func tokenHolderToModel(ctx context.Context, tokenHolder persist.TokenHolder) *model.TokenHolder {
	previewTokens := make([]*string, len(tokenHolder.PreviewTokens))
	for i, token := range tokenHolder.PreviewTokens {
		previewTokens[i] = util.ToPointer(token.String())
	}

	return &model.TokenHolder{
		HelperTokenHolderData: model.HelperTokenHolderData{UserId: tokenHolder.UserID, WalletIds: tokenHolder.WalletIDs},
		User:                  nil, // handled by dedicated resolver
		Wallets:               nil, // handled by dedicated resolver
		PreviewTokens:         previewTokens,
	}
}

func tokenDefinitionToModel(td db.TokenDefinition) *model.TokenDefinition {
	return &model.TokenDefinition{
		HelperTokenDefinitionData: model.HelperTokenDefinitionData{Definition: td},
		Dbid:                      td.ID,
		CreationTime:              &td.CreatedAt,
		LastUpdated:               &td.LastUpdated,
		Media:                     nil, // handled by dedicated resolver
		TokenType:                 util.ToPointer(model.TokenType(td.TokenType)),
		Chain:                     &td.Chain,
		Name:                      &td.Name.String,
		Description:               &td.Description.String,
		TokenID:                   util.ToPointer(td.TokenID.String()),
		Community:                 nil, // handled by dedicated resolver
		ExternalURL:               &td.ExternalUrl.String,
	}
}

func tokenToModel(ctx context.Context, token db.Token, collectionID *persist.DBID) *model.Token {
	var isSpamByUser *bool
	if token.IsUserMarkedSpam.Valid {
		isSpamByUser = &token.IsUserMarkedSpam.Bool
	}
	return &model.Token{
		HelperTokenData: model.HelperTokenData{Token: token, CollectionID: collectionID},
		Dbid:            token.ID,
		CreationTime:    &token.CreatedAt,
		LastUpdated:     &token.LastUpdated,
		CollectorsNote:  util.ToPointer(html.UnescapeString(token.CollectorsNote.String)),
		Quantity:        util.ToPointer(token.Quantity.String()),
		Owner:           nil, // handled by dedicated resolver
		OwnerIsHolder:   &token.IsHolderToken,
		OwnerIsCreator:  &token.IsCreatorToken,
		IsSpamByUser:    isSpamByUser,
		Definition:      nil, // handled by dedicated resolver
		// Fields to be deprecated
		Media:                 nil, // handled by dedicated resolver
		TokenType:             nil, // handled by dedicated resolver
		Chain:                 nil, // handled by dedicated resolver
		Name:                  nil, // handled by dedicated resolver
		Description:           nil, // handled by dedicated resolver
		TokenID:               nil, // handled by dedicated resolver
		TokenMetadata:         nil, // handled by dedicated resolver
		Contract:              nil, // handled by dedicated resolver
		ExternalURL:           nil, // handled by dedicated resolver
		IsSpamByProvider:      nil, // handled by dedicated resolver
		OwnedByWallets:        nil, // handled by dedicated resolver
		BlockNumber:           nil,
		OwnershipHistory:      nil, // TODO: later
		OpenseaCollectionName: nil, // TODO: later
	}
}

func tokensToModel(ctx context.Context, token []db.Token) []*model.Token {
	res := make([]*model.Token, len(token))
	for i, token := range token {
		res[i] = tokenToModel(ctx, token, nil)
	}
	return res
}

func collectionTokenToModel(ctx context.Context, token *model.Token, collectionID persist.DBID) *model.CollectionToken {
	return &model.CollectionToken{
		HelperCollectionTokenData: model.HelperCollectionTokenData{
			TokenId:      token.Dbid,
			CollectionId: collectionID,
		},
		Token:         token,
		Collection:    nil, // handled by dedicated resolver
		TokenSettings: nil, // handled by dedicated resolver
	}
}

func communityToModel(ctx context.Context, community db.Contract, forceRefresh *bool) *model.Community {
	lastUpdated := community.LastUpdated
	contractAddress := persist.NewChainAddress(community.Address, community.Chain)
	chain := community.Chain

	var creatorAddress *persist.ChainAddress
	if community.OwnerAddress != "" {
		creator, _ := util.FindFirst([]persist.Address{community.OwnerAddress, community.CreatorAddress}, func(a persist.Address) bool {
			return a != ""
		})
		chainAddress := persist.NewChainAddress(creator, chain)
		creatorAddress = &chainAddress
	}

	return &model.Community{
		HelperCommunityData: model.HelperCommunityData{
			ForceRefresh: forceRefresh,
		},
		Dbid:            community.ID,
		LastUpdated:     &lastUpdated,
		Contract:        contractToModel(ctx, community),
		ContractAddress: &contractAddress,
		CreatorAddress:  creatorAddress,
		Name:            util.ToPointer(html.UnescapeString(community.Name.String)),
		Description:     util.ToPointer(html.UnescapeString(community.Description.String)),
		// PreviewImage:     util.ToPointer(community.Pr.String()), // TODO do we still need this with the new image fields?
		Chain:             &chain,
		ProfileImageURL:   util.ToPointer(community.ProfileImageUrl.String),
		ProfileBannerURL:  util.ToPointer(community.ProfileBannerUrl.String),
		BadgeURL:          util.ToPointer(community.BadgeUrl.String),
		Owners:            nil, // handled by dedicated resolver
		Creator:           nil, // handled by dedicated resolver
		ParentCommunity:   nil, // handled by dedicated resolver
		SubCommunities:    nil, // handled by dedicated resolver
		TokensInCommunity: nil, // handled by dedicated resolver
	}
}

func pageInfoToModel(ctx context.Context, pageInfo publicapi.PageInfo) *model.PageInfo {
	return &model.PageInfo{
		Total:           pageInfo.Total,
		Size:            pageInfo.Size,
		HasPreviousPage: pageInfo.HasPreviousPage,
		HasNextPage:     pageInfo.HasNextPage,
		StartCursor:     pageInfo.StartCursor,
		EndCursor:       pageInfo.EndCursor,
	}
}

func resolveTokenMedia(ctx context.Context, td db.TokenDefinition, tokenMedia db.TokenMedia, highDef bool) model.MediaSubtype {
	// Rewrite fallback IPFS and Arweave URLs to HTTP
	if fallback := td.FallbackMedia.ImageURL.String(); strings.HasPrefix(fallback, "ipfs://") {
		td.FallbackMedia.ImageURL = persist.NullString(ipfs.DefaultGatewayFrom(fallback))
	} else if strings.HasPrefix(fallback, "ar://") {
		td.FallbackMedia.ImageURL = persist.NullString(fmt.Sprintf("https://arweave.net/%s", util.GetURIPath(fallback, false)))
	}

	// Media is found and is active.
	if tokenMedia.ID != "" && tokenMedia.Active {
		return mediaToModel(ctx, tokenMedia, td.FallbackMedia, highDef)
	}

	// If there is no media for a token, assume that the token is still being synced.
	if tokenMedia.ID == "" {
		tokenMedia.Media.MediaType = persist.MediaTypeSyncing
		// In the worse case the processing message was dropped and the token never gets handled. To address that,
		// we compare when the token was created to the current time. If it's longer than the grace period, we assume that the
		// message was lost and set the media to invalid so it could be refreshed manually.
		if inFlight, err := publicapi.For(ctx).Token.GetProcessingStateByTokenDefinitionID(ctx, td.ID); !inFlight || err != nil {
			if time.Since(td.CreatedAt) > time.Duration(1*time.Hour) {
				tokenMedia.Media.MediaType = persist.MediaTypeInvalid
			}
		}
		return mediaToModel(ctx, tokenMedia, td.FallbackMedia, highDef)
	}

	// If the media isn't valid, check if its still up for processing. If so, set the media as syncing.
	if tokenMedia.Media.MediaType != persist.MediaTypeSyncing && !tokenMedia.Media.MediaType.IsValid() {
		if inFlight, _ := publicapi.For(ctx).Token.GetProcessingStateByTokenDefinitionID(ctx, td.ID); inFlight {
			tokenMedia.Media.MediaType = persist.MediaTypeSyncing
		}
	}

	return mediaToModel(ctx, tokenMedia, td.FallbackMedia, highDef)
}

func mediaToModel(ctx context.Context, tokenMedia db.TokenMedia, fallback persist.FallbackMedia, highDef bool) model.MediaSubtype {
	fallbackMedia := getFallbackMedia(ctx, fallback)

	switch media := tokenMedia.Media; media.MediaType {
	case persist.MediaTypeImage, persist.MediaTypeSVG:
		return getImageMedia(ctx, tokenMedia, fallbackMedia, highDef)
	case persist.MediaTypeGIF:
		return getGIFMedia(ctx, tokenMedia, fallbackMedia, highDef)
	case persist.MediaTypeVideo:
		return getVideoMedia(ctx, tokenMedia, fallbackMedia, highDef)
	case persist.MediaTypeAudio:
		return getAudioMedia(ctx, tokenMedia, fallbackMedia)
	case persist.MediaTypeHTML:
		return getHtmlMedia(ctx, tokenMedia, fallbackMedia)
	case persist.MediaTypeAnimation:
		return getGltfMedia(ctx, tokenMedia, fallbackMedia)
	case persist.MediaTypeJSON:
		return getJsonMedia(ctx, tokenMedia, fallbackMedia)
	case persist.MediaTypeText:
		return getTextMedia(ctx, tokenMedia, fallbackMedia)
	case persist.MediaTypePDF:
		return getPdfMedia(ctx, tokenMedia, fallbackMedia)
	case persist.MediaTypeUnknown:
		return getUnknownMedia(ctx, tokenMedia, fallbackMedia)
	case persist.MediaTypeSyncing:
		return getSyncingMedia(ctx, tokenMedia, fallbackMedia)
	default:
		return getInvalidMedia(ctx, tokenMedia, fallbackMedia)
	}
}

func profileImageToModel(ctx context.Context, pfp db.ProfileImage) (model.ProfileImage, error) {
	// PFP isn't set or we were unable to retrieve it
	if pfp.ID == "" {
		return nil, nil
	}
	switch pfp.SourceType {
	case persist.ProfileImageSourceToken:
		token, err := publicapi.For(ctx).Token.GetTokenByIdIgnoreDisplayable(ctx, pfp.TokenID)
		if err != nil {
			return nil, err
		}
		return &model.TokenProfileImage{Token: tokenToModel(ctx, *token, nil)}, nil
	case persist.ProfileImageSourceENS:
		return ensProfileImageToModel(ctx, pfp.UserID, pfp.WalletID, pfp.EnsAvatarUri.String, pfp.EnsDomain.String)
	default:
		return nil, publicapi.ErrProfileImageUnknownSource
	}
}

func ensProfileImageToModel(ctx context.Context, userID, walletID persist.DBID, url, domain string) (*model.EnsProfileImage, error) {
	api := publicapi.For(ctx).Token
	// Use the token's profile image if the token exists
	if token, err := api.GetTokenByEnsDomain(ctx, userID, domain); err == nil {
		// This should be free because the definition is cached from the call above
		tDef, err := api.GetTokenDefinitionByID(ctx, token.TokenDefinitionID)
		if err != nil {
			return nil, err
		}
		if tokenMedia, err := api.GetMediaByMediaID(ctx, tDef.TokenMediaID); err == nil {
			if tokenMedia.Media.ProfileImageURL != "" {
				url = string(tokenMedia.Media.ProfileImageURL)
			}
		}
	}

	var pfp *model.HTTPSProfileImage = nil

	if strings.HasPrefix(url, "data:image/svg") {
		previewURL := util.ToPointer(url)
		pfp = &model.HTTPSProfileImage{
			PreviewURLs: &model.PreviewURLSet{
				Raw:       &url,
				Thumbnail: previewURL,
				Small:     previewURL,
				Medium:    previewURL,
				Large:     previewURL,
				SrcSet:    previewURL,
			},
		}
	} else {
		pfp = &model.HTTPSProfileImage{PreviewURLs: previewURLs(ctx, url, nil)}
	}

	return &model.EnsProfileImage{
		ProfileImage: pfp,
		Wallet:       nil, // handled by dedicated resolver
		Token:        nil, // handled by dedicated resolver, resolving this token should be free as it would be cached from the call above
		HelperEnsProfileImageData: model.HelperEnsProfileImageData{
			UserID:    userID,
			WalletID:  walletID,
			EnsDomain: domain,
		},
	}, nil
}

func resolveTokenByEnsDomain(ctx context.Context, userID persist.DBID, domain string) (*model.Token, error) {
	token, err := publicapi.For(ctx).Token.GetTokenByEnsDomain(ctx, userID, domain)
	if err != nil {
		return nil, err
	}
	return tokenToModel(ctx, token, nil), nil
}

func previewURLsFromTokenMedia(ctx context.Context, tokenMedia db.TokenMedia, options ...mediamapper.Option) *model.PreviewURLSet {
	url := tokenMedia.Media.ThumbnailURL.String()
	if (tokenMedia.Media.MediaType == persist.MediaTypeImage || tokenMedia.Media.MediaType == persist.MediaTypeSVG || tokenMedia.Media.MediaType == persist.MediaTypeGIF) && url == "" {
		url = tokenMedia.Media.MediaURL.String()
	}

	preview := remapLargeImageUrls(url)

	// Add timestamp to options
	options = append(options, mediamapper.WithTimestamp(tokenMedia.LastUpdated))

	// Add live render
	live := tokenMedia.Media.LivePreviewURL.String()
	if tokenMedia.Media.LivePreviewURL == "" {
		live = tokenMedia.Media.MediaURL.String()
	}

	return previewURLs(ctx, preview, &live, options...)
}

func previewURLs(ctx context.Context, url string, liveRender *string, options ...mediamapper.Option) *model.PreviewURLSet {
	mm := mediamapper.For(ctx)
	return &model.PreviewURLSet{
		Raw:        &url,
		Thumbnail:  util.ToPointer(mm.GetThumbnailImageUrl(url, options...)),
		Small:      util.ToPointer(mm.GetSmallImageUrl(url, options...)),
		Medium:     util.ToPointer(mm.GetMediumImageUrl(url, options...)),
		Large:      util.ToPointer(mm.GetLargeImageUrl(url, options...)),
		SrcSet:     util.ToPointer(mm.GetSrcSet(url, options...)),
		LiveRender: liveRender,
	}
}

func getImageMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia, highDef bool) model.ImageMedia {
	url := remapLargeImageUrls(tokenMedia.Media.MediaURL.String())

	options := []mediamapper.Option{}
	if highDef {
		options = append(options, mediamapper.WithQuality(100))
	}
	return model.ImageMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia, options...),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: &url,
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getFallbackMedia(ctx context.Context, media persist.FallbackMedia) *model.FallbackMedia {
	url := remapLargeImageUrls(media.ImageURL.String())
	medType := persist.MediaTypeFallback
	return &model.FallbackMedia{
		MediaURL:  util.ToPointer(url),
		MediaType: (*string)(&medType),
	}
}

func getGIFMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia, highDef bool) model.GIFMedia {
	url := remapLargeImageUrls(tokenMedia.Media.MediaURL.String())

	options := []mediamapper.Option{}
	if highDef {
		options = append(options, mediamapper.WithQuality(100))
	}
	return model.GIFMedia{
		PreviewURLs:       previewURLsFromTokenMedia(ctx, tokenMedia, options...),
		StaticPreviewURLs: previewURLsFromTokenMedia(ctx, tokenMedia, mediamapper.WithStaticImage()),
		MediaURL:          util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:         (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL:  &url,
		Dimensions:        mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:     fallbackMedia,
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

func getVideoMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia, highDef bool) model.VideoMedia {
	asString := tokenMedia.Media.MediaURL.String()
	videoUrls := model.VideoURLSet{
		Raw:    &asString,
		Small:  &asString,
		Medium: &asString,
		Large:  &asString,
	}

	options := []mediamapper.Option{}
	if highDef {
		options = append(options, mediamapper.WithQuality(100))
	}

	return model.VideoMedia{
		PreviewURLs:       previewURLsFromTokenMedia(ctx, tokenMedia, options...),
		MediaURL:          util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:         (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURLs: &videoUrls,
		Dimensions:        mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:     fallbackMedia,
	}
}

func getAudioMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia) model.AudioMedia {
	return model.AudioMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: (*string)(&tokenMedia.Media.MediaURL),
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getTextMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia) model.TextMedia {
	return model.TextMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: (*string)(&tokenMedia.Media.MediaURL),
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getPdfMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia) model.PDFMedia {
	return model.PDFMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: (*string)(&tokenMedia.Media.MediaURL),
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getHtmlMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia) model.HTMLMedia {
	return model.HTMLMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: (*string)(&tokenMedia.Media.MediaURL),
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getJsonMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia) model.JSONMedia {
	return model.JSONMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: (*string)(&tokenMedia.Media.MediaURL),
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getGltfMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia) model.GltfMedia {
	return model.GltfMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: (*string)(&tokenMedia.Media.MediaURL),
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getUnknownMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia) model.UnknownMedia {
	return model.UnknownMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: (*string)(&tokenMedia.Media.MediaURL),
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getSyncingMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia) model.SyncingMedia {
	return model.SyncingMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: (*string)(&tokenMedia.Media.MediaURL),
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getInvalidMedia(ctx context.Context, tokenMedia db.TokenMedia, fallbackMedia *model.FallbackMedia) model.InvalidMedia {
	return model.InvalidMedia{
		PreviewURLs:      previewURLsFromTokenMedia(ctx, tokenMedia),
		MediaURL:         util.ToPointer(tokenMedia.Media.MediaURL.String()),
		MediaType:        (*string)(&tokenMedia.Media.MediaType),
		ContentRenderURL: (*string)(&tokenMedia.Media.MediaURL),
		Dimensions:       mediaToDimensions(tokenMedia.Media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func mediaToDimensions(dimensions persist.Dimensions) *model.MediaDimensions {
	var aspect float64
	if dimensions.Height > 0 && dimensions.Width > 0 {
		aspect = float64(dimensions.Width) / float64(dimensions.Height)
	}

	return &model.MediaDimensions{
		Height:      &dimensions.Height,
		Width:       &dimensions.Width,
		AspectRatio: &aspect,
	}
}

func mentionToModel(ctx context.Context, mention db.Mention) *model.Mention {
	m := &model.Mention{}
	if mention.Start.Valid {
		m.Interval = &model.Interval{
			Start:  int(mention.Start.Int32),
			Length: int(mention.Length.Int32),
		}
	}

	switch {
	case mention.UserID != "":
		m.HelperMentionData.UserID = &mention.UserID
	case mention.ContractID != "":
		m.HelperMentionData.CommunityID = &mention.ContractID
	default:
		panic(fmt.Sprintf("unknown mention type: %+v", mention))
	}

	return m
}
