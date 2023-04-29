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

	"github.com/gammazero/workerpool"
	"github.com/magiclabs/magic-admin-go/token"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/emails"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/mediamapper"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/socialauth"
	"github.com/mikeydub/go-gallery/service/twitter"
	"github.com/mikeydub/go-gallery/validate"

	"github.com/mikeydub/go-gallery/debugtools"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
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

	OnCollectionToken: func(ctx context.Context, tokenId string, collectionId string) (*model.CollectionToken, error) {
		return resolveCollectionTokenByID(ctx, persist.DBID(tokenId), persist.DBID(collectionId))
	},

	OnCommunity: func(ctx context.Context, contractAddress string, chain string) (*model.Community, error) {
		if parsed, err := strconv.Atoi(chain); err == nil {
			return resolveCommunityByContractAddress(ctx, persist.NewChainAddress(persist.Address(contractAddress), persist.Chain(parsed)), util.ToPointer(false))
		} else {
			return nil, err
		}
	},
	OnSomeoneAdmiredYourFeedEventNotification: func(ctx context.Context, dbid persist.DBID) (*model.SomeoneAdmiredYourFeedEventNotification, error) {
		notif, err := resolveNotificationByID(ctx, dbid)
		if err != nil {
			return nil, err
		}

		notifConverted := notif.(model.SomeoneAdmiredYourFeedEventNotification)

		return &notifConverted, nil
	},
	OnSomeoneCommentedOnYourFeedEventNotification: func(ctx context.Context, dbid persist.DBID) (*model.SomeoneCommentedOnYourFeedEventNotification, error) {
		notif, err := resolveNotificationByID(ctx, dbid)
		if err != nil {
			return nil, err
		}

		notifConverted := notif.(model.SomeoneCommentedOnYourFeedEventNotification)

		return &notifConverted, nil
	},
	OnSomeoneFollowedYouBackNotification: func(ctx context.Context, dbid persist.DBID) (*model.SomeoneFollowedYouBackNotification, error) {
		notif, err := resolveNotificationByID(ctx, dbid)
		if err != nil {
			return nil, err
		}

		notifConverted := notif.(model.SomeoneFollowedYouBackNotification)

		return &notifConverted, nil
	},
	OnSomeoneFollowedYouNotification: func(ctx context.Context, dbid persist.DBID) (*model.SomeoneFollowedYouNotification, error) {
		notif, err := resolveNotificationByID(ctx, dbid)
		if err != nil {
			return nil, err
		}

		notifConverted := notif.(model.SomeoneFollowedYouNotification)

		return &notifConverted, nil
	},
	OnSomeoneViewedYourGalleryNotification: func(ctx context.Context, dbid persist.DBID) (*model.SomeoneViewedYourGalleryNotification, error) {
		notif, err := resolveNotificationByID(ctx, dbid)
		if err != nil {
			return nil, err
		}

		notifConverted := notif.(model.SomeoneViewedYourGalleryNotification)

		return &notifConverted, nil
	},
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

	switch err.(type) {
	case auth.ErrAuthenticationFailed:
		mappedErr = model.ErrAuthenticationFailed{Message: message}
	case auth.ErrDoesNotOwnRequiredNFT:
		mappedErr = model.ErrDoesNotOwnRequiredToken{Message: message}
	case persist.ErrUserNotFound:
		mappedErr = model.ErrUserNotFound{Message: message}
	case persist.ErrUserAlreadyExists:
		mappedErr = model.ErrUserAlreadyExists{Message: message}
	case persist.ErrUsernameNotAvailable:
		mappedErr = model.ErrUsernameNotAvailable{Message: message}
	case persist.ErrCollectionNotFoundByID:
		mappedErr = model.ErrCollectionNotFound{Message: message}
	case persist.ErrTokenNotFoundByID:
		mappedErr = model.ErrTokenNotFound{Message: message}
	case persist.ErrContractNotFoundByAddress:
		mappedErr = model.ErrCommunityNotFound{Message: message}
	case persist.ErrAddressOwnedByUser:
		mappedErr = model.ErrAddressOwnedByUser{Message: message}
	case persist.ErrAdmireNotFound:
		mappedErr = model.ErrAdmireNotFound{Message: message}
	case persist.ErrAdmireAlreadyExists:
		mappedErr = model.ErrAdmireAlreadyExists{Message: message}
	case persist.ErrCommentNotFound:
		mappedErr = model.ErrCommentNotFound{Message: message}
	case publicapi.ErrTokenRefreshFailed:
		mappedErr = model.ErrSyncFailed{Message: message}
	case validate.ErrInvalidInput:
		validationErr, _ := err.(validate.ErrInvalidInput)
		mappedErr = model.ErrInvalidInput{Message: message, Parameters: validationErr.Parameters, Reasons: validationErr.Reasons}
	case persist.ErrFeedEventNotFoundByID:
		mappedErr = model.ErrFeedEventNotFound{Message: message}
	case persist.ErrUnknownAction:
		mappedErr = model.ErrUnknownAction{Message: message}
	case persist.ErrGalleryNotFound:
		mappedErr = model.ErrGalleryNotFound{Message: message}
	case twitter.ErrInvalidRefreshToken:
		mappedErr = model.ErrNeedsToReconnectSocial{SocialAccountType: persist.SocialProviderTwitter, Message: message}
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

	if m.Twitter != nil {
		authedUserID := publicapi.For(ctx).User.GetLoggedInUserId(ctx)
		return publicapi.For(ctx).Social.NewTwitterAuthenticator(authedUserID, m.Twitter.Code), nil
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

func resolveBadgesByUserID(ctx context.Context, userID persist.DBID) ([]*model.Badge, error) {
	contracts, err := publicapi.For(ctx).Contract.GetContractsDisplayedByUserID(ctx, userID)

	if err != nil {
		return nil, err
	}

	var result []*model.Badge
	for _, contract := range contracts {
		result = append(result, contractToBadgeModel(ctx, contract))
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

	return util.Map(medias, func(token persist.Media) (*model.PreviewURLSet, error) {
		return getPreviewUrls(ctx, token), nil
	})
}

func resolveCollectionTokenByID(ctx context.Context, tokenID persist.DBID, collectionID persist.DBID) (*model.CollectionToken, error) {
	token, err := resolveTokenByTokenID(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	return tokenCollectionToModel(ctx, token, collectionID), nil
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

func resolveTokensByUserIDAndContractID(ctx context.Context, userID, contractID persist.DBID) ([]*model.Token, error) {

	tokens, err := publicapi.For(ctx).Token.GetTokensByUserIDAndContractID(ctx, userID, contractID)
	if err != nil {
		return nil, err
	}

	return tokensToModel(ctx, tokens), nil
}

func resolveTokensByContractID(ctx context.Context, contractID persist.DBID) ([]*model.Token, error) {

	tokens, err := publicapi.For(ctx).Token.GetTokensByContractId(ctx, contractID)
	if err != nil {
		return nil, err
	}

	return tokensToModel(ctx, tokens), nil
}

func resolveTokensByContractIDWithPagination(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int, onlyGalleryUsers *bool) (*model.TokensConnection, error) {

	tokens, pageInfo, err := publicapi.For(ctx).Token.GetTokensByContractIdPaginate(ctx, contractID, before, after, first, last, onlyGalleryUsers)
	if err != nil {
		return nil, err
	}

	edges := make([]*model.TokenEdge, len(tokens))
	for i, token := range tokens {
		edges[i] = &model.TokenEdge{
			Node:   tokenToModel(ctx, token),
			Cursor: nil, // not used by relay, but relay will complain without this field existing
		}
	}

	return &model.TokensConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil
}

func refreshTokensInContractAsync(ctx context.Context, contractID persist.DBID, forceRefresh bool) error {
	return publicapi.For(ctx).Contract.RefreshOwnersAsync(ctx, contractID, forceRefresh)
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

func resolveCommunityByContractAddress(ctx context.Context, contractAddress persist.ChainAddress, forceRefresh *bool) (*model.Community, error) {
	community, err := publicapi.For(ctx).Contract.GetContractByAddress(ctx, contractAddress)

	if err != nil {
		return nil, err
	}

	return communityToModel(ctx, *community, forceRefresh), nil
}

func resolveCommunityOwnersByContractID(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int, onlyGalleryUsers *bool) (*model.TokenHoldersConnection, error) {
	contract, err := publicapi.For(ctx).Contract.GetContractByID(ctx, contractID)
	if err != nil {
		return nil, err
	}
	owners, pageInfo, err := publicapi.For(ctx).Contract.GetCommunityOwnersByContractAddress(ctx, persist.NewChainAddress(contract.Address, contract.Chain), before, after, first, last, onlyGalleryUsers)
	if err != nil {
		return nil, err
	}
	edges := make([]*model.TokenHolderEdge, len(owners))
	for i, owner := range owners {
		edges[i] = &model.TokenHolderEdge{
			Node:   owner,
			Cursor: nil, // not used by relay, but relay will complain without this field existing
		}
	}

	return &model.TokenHoldersConnection{
		Edges:    edges,
		PageInfo: pageInfoToModel(ctx, pageInfo),
	}, nil

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
		HelperGroupNotificationUsersConnectionData: model.HelperGroupNotificationUsersConnectionData{
			UserIDs: userIDs,
		},
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
		token := tokenToModel(ctx, t)
		newTokens[tokenIDToPosition[t.ID]] = tokenCollectionToModel(ctx, token, collectionID)
	}

	return newTokens, nil
}

func resolveTokenSettingsByIDs(ctx context.Context, tokenID, collectionID persist.DBID) (*model.CollectionTokenSettings, error) {
	collection, err := publicapi.For(ctx).Collection.GetCollectionById(ctx, collectionID)

	if err != nil {
		return nil, err
	}

	if settings, ok := collection.TokenSettings[tokenID]; ok {
		return &model.CollectionTokenSettings{RenderLive: &settings.RenderLive}, nil
	}

	return &model.CollectionTokenSettings{RenderLive: &defaultTokenSettings.RenderLive}, nil
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

func rawGalleryEventToFeedEventDataModel(event *db.Event) (model.FeedEventData, error) {
	switch event.Action {
	case persist.ActionCollectorsNoteAddedToToken:
		return rawEventToCollectorsNoteAddedToTokenFeedEventData(event), nil
	case persist.ActionCollectionCreated:
		return rawEventToCollectionCreatedFeedEventData(event), nil
	case persist.ActionCollectorsNoteAddedToCollection:
		return rawEventToCollectorsNoteAddedToCollectionFeedEventData(event), nil
	case persist.ActionTokensAddedToCollection:
		return rawEventToTokensAddedToCollectionFeedEventData(event), nil
	case persist.ActionGalleryInfoUpdated:
		return rawEventToGalleryInfoUpdatedFeedEventData(event), nil
	default:
		return nil, persist.ErrUnknownAction{Action: event.Action}
	}
}

func feedEventToModel(event *db.FeedEvent) (*model.FeedEvent, error) {
	data, err := feedEventToDataModel(event)
	if err != nil {
		return nil, err
	}

	// Value always returns a nil error so we can safely ignore it.
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
		Token:             &model.CollectionToken{Token: &model.Token{Dbid: event.Data.TokenID}, Collection: &model.Collection{Dbid: event.Data.TokenCollectionID}, HelperCollectionTokenData: model.HelperCollectionTokenData{TokenId: event.Data.TokenID, CollectionId: event.Data.TokenCollectionID}},
		Action:            &event.Action,
		NewCollectorsNote: util.ToPointer(event.Data.TokenNewCollectorsNote),
	}
}

func rawEventToCollectorsNoteAddedToTokenFeedEventData(event *db.Event) model.FeedEventData {
	return model.CollectorsNoteAddedToTokenFeedEventData{
		EventTime:         &event.CreatedAt,
		Owner:             &model.GalleryUser{Dbid: persist.DBID(event.ActorID.String)}, // remaining fields handled by dedicated resolver
		Token:             &model.CollectionToken{Token: &model.Token{Dbid: event.TokenID}, Collection: &model.Collection{Dbid: event.CollectionID}, HelperCollectionTokenData: model.HelperCollectionTokenData{TokenId: event.TokenID, CollectionId: event.CollectionID}},
		Action:            &event.Action,
		NewCollectorsNote: util.ToPointer(event.Data.TokenCollectorsNote),
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

func rawEventToCollectionCreatedFeedEventData(event *db.Event) model.FeedEventData {
	return model.CollectionCreatedFeedEventData{
		EventTime:  &event.CreatedAt,
		Owner:      &model.GalleryUser{Dbid: persist.DBID(event.ActorID.String)}, // remaining fields handled by dedicated resolver
		Collection: &model.Collection{Dbid: event.CollectionID},                  // remaining fields handled by dedicated resolver
		Action:     &event.Action,
		NewTokens:  nil, // handled by dedicated resolver
		HelperCollectionCreatedFeedEventDataData: model.HelperCollectionCreatedFeedEventDataData{
			TokenIDs:     event.Data.CollectionTokenIDs,
			CollectionID: event.CollectionID,
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

func rawEventToCollectorsNoteAddedToCollectionFeedEventData(event *db.Event) model.FeedEventData {
	return model.CollectorsNoteAddedToCollectionFeedEventData{
		EventTime:         &event.CreatedAt,
		Owner:             &model.GalleryUser{Dbid: persist.DBID(event.ActorID.String)}, // remaining fields handled by dedicated resolver
		Collection:        &model.Collection{Dbid: event.CollectionID},                  // remaining fields handled by dedicated resolver
		Action:            &event.Action,
		NewCollectorsNote: util.ToPointer(event.Data.CollectionCollectorsNote),
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

func rawEventToTokensAddedToCollectionFeedEventData(event *db.Event) model.FeedEventData {
	logger.For(nil).Infof("here it is before: coll id: %s - token ids: %+v", event.CollectionID, event.Data.CollectionTokenIDs)
	return model.TokensAddedToCollectionFeedEventData{
		EventTime:  &event.CreatedAt,
		Owner:      &model.GalleryUser{Dbid: persist.DBID(event.ActorID.String)}, // remaining fields handled by dedicated resolver
		Collection: &model.Collection{Dbid: event.CollectionID},                  // remaining fields handled by dedicated resolver
		Action:     &event.Action,
		NewTokens:  nil, // handled by dedicated resolver
		HelperTokensAddedToCollectionFeedEventDataData: model.HelperTokensAddedToCollectionFeedEventDataData{
			TokenIDs:     event.Data.CollectionTokenIDs,
			CollectionID: event.CollectionID,
		},
	}
}

func rawEventToGalleryInfoUpdatedFeedEventData(event *db.Event) model.FeedEventData {
	return model.GalleryInfoUpdatedFeedEventData{
		EventTime:      &event.CreatedAt,
		Owner:          &model.GalleryUser{Dbid: persist.DBID(event.ActorID.String)}, // remaining fields handled by dedicated resolver
		Action:         &event.Action,
		NewName:        event.Data.GalleryName,
		NewDescription: event.Data.GalleryDescription,
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
					Token: &model.CollectionToken{Token: &model.Token{Dbid: tokenID}, Collection: &model.Collection{Dbid: collectionID}, HelperCollectionTokenData: model.HelperCollectionTokenData{
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

func eventsToFeedEdges(events []db.FeedEvent) ([]*model.FeedEdge, error) {
	edges := make([]*model.FeedEdge, len(events))

	for i, evt := range events {
		var node model.FeedEventOrError
		node, err := feedEventToModel(&evt)

		if e, ok := err.(*persist.ErrUnknownAction); ok {
			node = model.ErrUnknownAction{Message: e.Error()}
		} else if err != nil {
			return nil, err
		}

		edges[i] = &model.FeedEdge{Node: node}
	}

	return edges, nil
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

func galleriesToModels(ctx context.Context, galleries []db.Gallery) []*model.Gallery {
	models := make([]*model.Gallery, len(galleries))
	for i, gallery := range galleries {
		models[i] = galleryToModel(ctx, gallery)
	}

	return models
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

	return &model.GalleryUser{
		HelperGalleryUserData: model.HelperGalleryUserData{
			UserID:            user.ID,
			FeaturedGalleryID: user.FeaturedGallery,
		},
		Dbid:      user.ID,
		Username:  &user.Username.String,
		Bio:       &user.Bio.String,
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

func usersToModels(ctx context.Context, users []db.User) []*model.GalleryUser {
	models := make([]*model.GalleryUser, len(users))
	for i, user := range users {
		models[i] = userToModel(ctx, user)
	}

	return models
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

	return &model.Admire{
		Dbid:         admire.ID,
		CreationTime: &admire.CreatedAt,
		LastUpdated:  &admire.LastUpdated,
		Admirer:      &model.GalleryUser{Dbid: admire.ActorID}, // remaining fields handled by dedicated resolver
	}
}

// admireToModel converts a db.Admire to a model.Admire
func admiresToModels(ctx context.Context, admires []db.Admire) []*model.Admire {
	result := make([]*model.Admire, len(admires))
	for i, admire := range admires {
		result[i] = admireToModel(ctx, admire)
	}
	return result
}

// commentToModel converts a db.Admire to a model.Admire
func commentToModel(ctx context.Context, comment db.Comment) *model.Comment {

	return &model.Comment{
		Dbid:         comment.ID,
		CreationTime: &comment.CreatedAt,
		LastUpdated:  &comment.LastUpdated,
		Comment:      &comment.Comment,
		Commenter:    &model.GalleryUser{Dbid: comment.ActorID}, // remaining fields handled by dedicated resolver
	}
}

// commentToModel converts a db.Admire to a model.Admire
func commentsToModels(ctx context.Context, comment []db.Comment) []*model.Comment {

	result := make([]*model.Comment, len(comment))
	for i, comment := range comment {
		result[i] = commentToModel(ctx, comment)
	}
	return result
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
	creator := persist.NewChainAddress(contract.OwnerAddress, chain)

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

func multichainTokenHolderToModel(ctx context.Context, tokenHolder multichain.TokenHolder, contractID persist.DBID) *model.TokenHolder {
	previewTokens := make([]*string, len(tokenHolder.PreviewTokens))
	for i, token := range tokenHolder.PreviewTokens {
		previewTokens[i] = util.ToPointer(token)
	}

	return &model.TokenHolder{
		HelperTokenHolderData: model.HelperTokenHolderData{UserId: tokenHolder.UserID, WalletIds: tokenHolder.WalletIDs},
		DisplayName:           &tokenHolder.DisplayName,
		User:                  nil, // handled by dedicated resolver
		Wallets:               nil, // handled by dedicated resolver
		PreviewTokens:         previewTokens,
	}
}

func tokenToModel(ctx context.Context, token db.Token) *model.Token {
	chain := token.Chain
	metadata, _ := token.TokenMetadata.MarshalJSON()
	metadataString := string(metadata)
	blockNumber := fmt.Sprint(token.BlockNumber.Int64)
	tokenType := model.TokenType(token.TokenType.String)

	var isSpamByUser *bool
	if token.IsUserMarkedSpam.Valid {
		isSpamByUser = &token.IsUserMarkedSpam.Bool
	}

	var isSpamByProvider *bool
	if token.IsProviderMarkedSpam.Valid {
		isSpamByProvider = &token.IsProviderMarkedSpam.Bool
	}

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
		TokenID:          util.ToPointer(token.TokenID.String()),
		Quantity:         &token.Quantity.String,
		Owner:            nil, // handled by dedicated resolver
		OwnershipHistory: nil, // TODO: later
		TokenMetadata:    &metadataString,
		Contract:         nil, // handled by dedicated resolver
		ExternalURL:      &token.ExternalUrl.String,
		BlockNumber:      &blockNumber, // TODO: later
		IsSpamByUser:     isSpamByUser,
		IsSpamByProvider: isSpamByProvider,

		// These are legacy mappings that will likely end up elsewhere when we pull data from the indexer
		OpenseaCollectionName: nil, // TODO: later
	}
}

func tokensToModel(ctx context.Context, token []db.Token) []*model.Token {
	res := make([]*model.Token, len(token))
	for i, token := range token {
		res[i] = tokenToModel(ctx, token)
	}
	return res
}

func tokenCollectionToModel(ctx context.Context, token *model.Token, collectionID persist.DBID) *model.CollectionToken {
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
	creatorAddress := persist.NewChainAddress(community.OwnerAddress, community.Chain)
	chain := community.Chain
	return &model.Community{
		HelperCommunityData: model.HelperCommunityData{
			ForceRefresh: forceRefresh,
		},
		Dbid:            community.ID,
		LastUpdated:     &lastUpdated,
		Contract:        contractToModel(ctx, community),
		ContractAddress: &contractAddress,
		CreatorAddress:  &creatorAddress,
		Name:            util.ToPointer(community.Name.String),
		Description:     util.ToPointer(community.Description.String),
		// PreviewImage:     util.ToPointer(community.Pr.String()), // TODO do we still need this with the new image fields?
		Chain:            &chain,
		ProfileImageURL:  util.ToPointer(community.ProfileImageUrl.String),
		ProfileBannerURL: util.ToPointer(community.ProfileBannerUrl.String),
		BadgeURL:         util.ToPointer(community.BadgeUrl.String),
		Owners:           nil, // handled by dedicated resolver
	}
}

func communitiesToModels(ctx context.Context, communities []db.Contract, forceRefresh *bool) []*model.Community {
	models := make([]*model.Community, len(communities))
	for i, community := range communities {
		models[i] = communityToModel(ctx, community, forceRefresh)
	}

	return models
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

func getUrlExtension(url string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(url), "."))
}

func getMediaForToken(ctx context.Context, token db.Token) model.MediaSubtype {
	med := token.Media

	var fallbackMedia *model.FallbackMedia
	if !med.IsServable() && token.FallbackMedia.IsServable() {
		fallbackMedia = getFallbackMedia(ctx, token.FallbackMedia)
	}

	switch med.MediaType {
	case persist.MediaTypeImage, persist.MediaTypeSVG:
		return getImageMedia(ctx, med, fallbackMedia)
	case persist.MediaTypeGIF:
		return getGIFMedia(ctx, med, fallbackMedia)
	case persist.MediaTypeVideo:
		return getVideoMedia(ctx, med, fallbackMedia)
	case persist.MediaTypeAudio:
		return getAudioMedia(ctx, med, fallbackMedia)
	case persist.MediaTypeHTML:
		return getHtmlMedia(ctx, med, fallbackMedia)
	case persist.MediaTypeAnimation:
		return getGltfMedia(ctx, med, fallbackMedia)
	case persist.MediaTypeJSON:
		return getJsonMedia(ctx, med, fallbackMedia)
	case persist.MediaTypeText, persist.MediaTypeBase64Text:
		return getTextMedia(ctx, med, fallbackMedia)
	case persist.MediaTypePDF:
		return getPdfMedia(ctx, med, fallbackMedia)
	case persist.MediaTypeUnknown:
		return getUnknownMedia(ctx, med, fallbackMedia)
	case persist.MediaTypeSyncing:
		return getSyncingMedia(ctx, med, fallbackMedia)
	default:
		return getInvalidMedia(ctx, med, fallbackMedia)
	}

}

func getPreviewUrls(ctx context.Context, media persist.Media, options ...mediamapper.Option) *model.PreviewURLSet {
	url := media.ThumbnailURL.String()
	if (media.MediaType == persist.MediaTypeImage || media.MediaType == persist.MediaTypeSVG || media.MediaType == persist.MediaTypeGIF) && url == "" {
		url = media.MediaURL.String()
	}
	preview := remapLargeImageUrls(url)
	mm := mediamapper.For(ctx)

	live := media.LivePreviewURL.String()
	if media.LivePreviewURL == "" {
		live = media.MediaURL.String()
	}

	return &model.PreviewURLSet{
		Raw:        &preview,
		Thumbnail:  util.ToPointer(mm.GetThumbnailImageUrl(preview, options...)),
		Small:      util.ToPointer(mm.GetSmallImageUrl(preview, options...)),
		Medium:     util.ToPointer(mm.GetMediumImageUrl(preview, options...)),
		Large:      util.ToPointer(mm.GetLargeImageUrl(preview, options...)),
		SrcSet:     util.ToPointer(mm.GetSrcSet(preview, options...)),
		LiveRender: &live,
	}
}

func getImageMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.ImageMedia {
	url := remapLargeImageUrls(media.MediaURL.String())

	return model.ImageMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: &url,
		Dimensions:       mediaToDimensions(media.Dimensions),
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

func getGIFMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.GIFMedia {
	url := remapLargeImageUrls(media.MediaURL.String())

	return model.GIFMedia{
		PreviewURLs:       getPreviewUrls(ctx, media),
		StaticPreviewURLs: getPreviewUrls(ctx, media, mediamapper.WithStaticImage()),
		MediaURL:          util.ToPointer(media.MediaURL.String()),
		MediaType:         (*string)(&media.MediaType),
		ContentRenderURL:  &url,
		Dimensions:        mediaToDimensions(media.Dimensions),
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

func getVideoMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.VideoMedia {
	asString := media.MediaURL.String()
	videoUrls := model.VideoURLSet{
		Raw:    &asString,
		Small:  &asString,
		Medium: &asString,
		Large:  &asString,
	}

	return model.VideoMedia{
		PreviewURLs:       getPreviewUrls(ctx, media),
		MediaURL:          util.ToPointer(media.MediaURL.String()),
		MediaType:         (*string)(&media.MediaType),
		ContentRenderURLs: &videoUrls,
		Dimensions:        mediaToDimensions(media.Dimensions),
		FallbackMedia:     fallbackMedia,
	}
}

func getAudioMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.AudioMedia {
	return model.AudioMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
		Dimensions:       mediaToDimensions(media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getTextMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.TextMedia {
	return model.TextMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
		Dimensions:       mediaToDimensions(media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getPdfMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.PDFMedia {
	return model.PDFMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
		Dimensions:       mediaToDimensions(media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getHtmlMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.HTMLMedia {
	return model.HTMLMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
		Dimensions:       mediaToDimensions(media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getJsonMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.JSONMedia {
	return model.JSONMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
		Dimensions:       mediaToDimensions(media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getGltfMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.GltfMedia {
	return model.GltfMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
		Dimensions:       mediaToDimensions(media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getUnknownMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.UnknownMedia {
	return model.UnknownMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
		Dimensions:       mediaToDimensions(media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getSyncingMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.SyncingMedia {
	return model.SyncingMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
		Dimensions:       mediaToDimensions(media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func getInvalidMedia(ctx context.Context, media persist.Media, fallbackMedia *model.FallbackMedia) model.InvalidMedia {
	return model.InvalidMedia{
		PreviewURLs:      getPreviewUrls(ctx, media),
		MediaURL:         util.ToPointer(media.MediaURL.String()),
		MediaType:        (*string)(&media.MediaType),
		ContentRenderURL: (*string)(&media.MediaURL),
		Dimensions:       mediaToDimensions(media.Dimensions),
		FallbackMedia:    fallbackMedia,
	}
}

func mediaToDimensions(dimensions persist.Dimensions) *model.MediaDimensions {
	var aspect float64
	if dimensions.Height > 0 && dimensions.Width > 0 {
		aspect = float64(dimensions.Width) / float64(dimensions.Height)
	}

	return &model.MediaDimensions{
		Width:       &dimensions.Height,
		Height:      &dimensions.Width,
		AspectRatio: &aspect,
	}
}
