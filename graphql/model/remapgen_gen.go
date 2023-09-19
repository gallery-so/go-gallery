// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

var typeConversionMap = map[string]func(object interface{}) (objectAsType interface{}, ok bool){
	"AddRolesToUserPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(AddRolesToUserPayloadOrError)
		return obj, ok
	},

	"AddUserWalletPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(AddUserWalletPayloadOrError)
		return obj, ok
	},

	"AdminAddWalletPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(AdminAddWalletPayloadOrError)
		return obj, ok
	},

	"AdmireFeedEventPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(AdmireFeedEventPayloadOrError)
		return obj, ok
	},

	"AdmirePostPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(AdmirePostPayloadOrError)
		return obj, ok
	},

	"AdmireTokenPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(AdmireTokenPayloadOrError)
		return obj, ok
	},

	"AuthorizationError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(AuthorizationError)
		return obj, ok
	},

	"BanUserFromFeedPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(BanUserFromFeedPayloadOrError)
		return obj, ok
	},

	"CollectionByIdOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CollectionByIDOrError)
		return obj, ok
	},

	"CollectionTokenByIdOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CollectionTokenByIDOrError)
		return obj, ok
	},

	"CommentOnFeedEventPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CommentOnFeedEventPayloadOrError)
		return obj, ok
	},

	"CommentOnPostPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CommentOnPostPayloadOrError)
		return obj, ok
	},

	"CommunityByAddressOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CommunityByAddressOrError)
		return obj, ok
	},

	"ConnectSocialAccountPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(ConnectSocialAccountPayloadOrError)
		return obj, ok
	},

	"CreateCollectionPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CreateCollectionPayloadOrError)
		return obj, ok
	},

	"CreateGalleryPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CreateGalleryPayloadOrError)
		return obj, ok
	},

	"CreateUserPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CreateUserPayloadOrError)
		return obj, ok
	},

	"DeleteCollectionPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(DeleteCollectionPayloadOrError)
		return obj, ok
	},

	"DeleteGalleryPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(DeleteGalleryPayloadOrError)
		return obj, ok
	},

	"DeletePostPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(DeletePostPayloadOrError)
		return obj, ok
	},

	"DisconnectSocialAccountPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(DisconnectSocialAccountPayloadOrError)
		return obj, ok
	},

	"Error": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(Error)
		return obj, ok
	},

	"FeedEventByIdOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(FeedEventByIDOrError)
		return obj, ok
	},

	"FeedEventData": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(FeedEventData)
		return obj, ok
	},

	"FeedEventOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(FeedEventOrError)
		return obj, ok
	},

	"FollowAllSocialConnectionsPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(FollowAllSocialConnectionsPayloadOrError)
		return obj, ok
	},

	"FollowUserPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(FollowUserPayloadOrError)
		return obj, ok
	},

	"GalleryByIdPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(GalleryByIDPayloadOrError)
		return obj, ok
	},

	"GalleryUserOrAddress": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(GalleryUserOrAddress)
		return obj, ok
	},

	"GalleryUserOrWallet": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(GalleryUserOrWallet)
		return obj, ok
	},

	"GenerateQRCodeLoginTokenPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(GenerateQRCodeLoginTokenPayloadOrError)
		return obj, ok
	},

	"GetAuthNoncePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(GetAuthNoncePayloadOrError)
		return obj, ok
	},

	"GroupedNotification": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(GroupedNotification)
		return obj, ok
	},

	"Interaction": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(Interaction)
		return obj, ok
	},

	"LoginPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(LoginPayloadOrError)
		return obj, ok
	},

	"Media": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(Media)
		return obj, ok
	},

	"MediaSubtype": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(MediaSubtype)
		return obj, ok
	},

	"MerchTokensPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(MerchTokensPayloadOrError)
		return obj, ok
	},

	"MintPremiumCardToWalletPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(MintPremiumCardToWalletPayloadOrError)
		return obj, ok
	},

	"MoveCollectionToGalleryPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(MoveCollectionToGalleryPayloadOrError)
		return obj, ok
	},

	"Node": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(Node)
		return obj, ok
	},

	"Notification": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(Notification)
		return obj, ok
	},

	"OptInForRolesPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(OptInForRolesPayloadOrError)
		return obj, ok
	},

	"OptOutForRolesPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(OptOutForRolesPayloadOrError)
		return obj, ok
	},

	"PostComposerDraftDetailsPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(PostComposerDraftDetailsPayloadOrError)
		return obj, ok
	},

	"PostOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(PostOrError)
		return obj, ok
	},

	"PostTokensPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(PostTokensPayloadOrError)
		return obj, ok
	},

	"PreverifyEmailPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(PreverifyEmailPayloadOrError)
		return obj, ok
	},

	"ProfileImage": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(ProfileImage)
		return obj, ok
	},

	"PublishGalleryPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(PublishGalleryPayloadOrError)
		return obj, ok
	},

	"RedeemMerchPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RedeemMerchPayloadOrError)
		return obj, ok
	},

	"ReferralPostPreflightPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(ReferralPostPreflightPayloadOrError)
		return obj, ok
	},

	"ReferralPostTokenPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(ReferralPostTokenPayloadOrError)
		return obj, ok
	},

	"RefreshCollectionPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RefreshCollectionPayloadOrError)
		return obj, ok
	},

	"RefreshContractPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RefreshContractPayloadOrError)
		return obj, ok
	},

	"RefreshTokenPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RefreshTokenPayloadOrError)
		return obj, ok
	},

	"RegisterUserPushTokenPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RegisterUserPushTokenPayloadOrError)
		return obj, ok
	},

	"RemoveAdmirePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RemoveAdmirePayloadOrError)
		return obj, ok
	},

	"RemoveCommentPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RemoveCommentPayloadOrError)
		return obj, ok
	},

	"RemoveProfileImagePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RemoveProfileImagePayloadOrError)
		return obj, ok
	},

	"RemoveUserWalletsPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RemoveUserWalletsPayloadOrError)
		return obj, ok
	},

	"ResendVerificationEmailPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(ResendVerificationEmailPayloadOrError)
		return obj, ok
	},

	"RevokeRolesFromUserPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RevokeRolesFromUserPayloadOrError)
		return obj, ok
	},

	"SearchCommunitiesPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SearchCommunitiesPayloadOrError)
		return obj, ok
	},

	"SearchGalleriesPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SearchGalleriesPayloadOrError)
		return obj, ok
	},

	"SearchUsersPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SearchUsersPayloadOrError)
		return obj, ok
	},

	"SetCommunityOverrideCreatorPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SetCommunityOverrideCreatorPayloadOrError)
		return obj, ok
	},

	"SetProfileImagePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SetProfileImagePayloadOrError)
		return obj, ok
	},

	"SetSpamPreferencePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SetSpamPreferencePayloadOrError)
		return obj, ok
	},

	"SocialAccount": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SocialAccount)
		return obj, ok
	},

	"SocialConnectionsOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SocialConnectionsOrError)
		return obj, ok
	},

	"SocialQueriesOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SocialQueriesOrError)
		return obj, ok
	},

	"SyncCreatedTokensForExistingContractPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SyncCreatedTokensForExistingContractPayloadOrError)
		return obj, ok
	},

	"SyncCreatedTokensForNewContractsPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SyncCreatedTokensForNewContractsPayloadOrError)
		return obj, ok
	},

	"SyncCreatedTokensForUsernameAndExistingContractPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SyncCreatedTokensForUsernameAndExistingContractPayloadOrError)
		return obj, ok
	},

	"SyncCreatedTokensForUsernamePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SyncCreatedTokensForUsernamePayloadOrError)
		return obj, ok
	},

	"SyncTokensForUsernamePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SyncTokensForUsernamePayloadOrError)
		return obj, ok
	},

	"SyncTokensPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(SyncTokensPayloadOrError)
		return obj, ok
	},

	"TokenByIdOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(TokenByIDOrError)
		return obj, ok
	},

	"TrendingUsersPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(TrendingUsersPayloadOrError)
		return obj, ok
	},

	"UnbanUserFromFeedPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UnbanUserFromFeedPayloadOrError)
		return obj, ok
	},

	"UnfollowUserPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UnfollowUserPayloadOrError)
		return obj, ok
	},

	"UnregisterUserPushTokenPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UnregisterUserPushTokenPayloadOrError)
		return obj, ok
	},

	"UnsubscribeFromEmailTypePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UnsubscribeFromEmailTypePayloadOrError)
		return obj, ok
	},

	"UpdateCollectionHiddenPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateCollectionHiddenPayloadOrError)
		return obj, ok
	},

	"UpdateCollectionInfoPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateCollectionInfoPayloadOrError)
		return obj, ok
	},

	"UpdateCollectionTokensPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateCollectionTokensPayloadOrError)
		return obj, ok
	},

	"UpdateEmailNotificationSettingsPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateEmailNotificationSettingsPayloadOrError)
		return obj, ok
	},

	"UpdateEmailPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateEmailPayloadOrError)
		return obj, ok
	},

	"UpdateFeaturedGalleryPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateFeaturedGalleryPayloadOrError)
		return obj, ok
	},

	"UpdateGalleryCollectionsPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateGalleryCollectionsPayloadOrError)
		return obj, ok
	},

	"UpdateGalleryHiddenPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateGalleryHiddenPayloadOrError)
		return obj, ok
	},

	"UpdateGalleryInfoPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateGalleryInfoPayloadOrError)
		return obj, ok
	},

	"UpdateGalleryOrderPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateGalleryOrderPayloadOrError)
		return obj, ok
	},

	"UpdateGalleryPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateGalleryPayloadOrError)
		return obj, ok
	},

	"UpdatePrimaryWalletPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdatePrimaryWalletPayloadOrError)
		return obj, ok
	},

	"UpdateSocialAccountDisplayedPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateSocialAccountDisplayedPayloadOrError)
		return obj, ok
	},

	"UpdateTokenInfoPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateTokenInfoPayloadOrError)
		return obj, ok
	},

	"UpdateUserExperiencePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateUserExperiencePayloadOrError)
		return obj, ok
	},

	"UpdateUserInfoPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateUserInfoPayloadOrError)
		return obj, ok
	},

	"UploadPersistedQueriesPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UploadPersistedQueriesPayloadOrError)
		return obj, ok
	},

	"UserByAddressOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UserByAddressOrError)
		return obj, ok
	},

	"UserByIdOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UserByIDOrError)
		return obj, ok
	},

	"UserByUsernameOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UserByUsernameOrError)
		return obj, ok
	},

	"VerifyEmailMagicLinkPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(VerifyEmailMagicLinkPayloadOrError)
		return obj, ok
	},

	"VerifyEmailPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(VerifyEmailPayloadOrError)
		return obj, ok
	},

	"ViewGalleryPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(ViewGalleryPayloadOrError)
		return obj, ok
	},

	"ViewTokenPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(ViewTokenPayloadOrError)
		return obj, ok
	},

	"ViewerGalleryByIdPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(ViewerGalleryByIDPayloadOrError)
		return obj, ok
	},

	"ViewerOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(ViewerOrError)
		return obj, ok
	},
}

func ConvertToModelType(object interface{}, gqlTypeName string) (objectAsType interface{}, ok bool) {
	if conversionFunc, ok := typeConversionMap[gqlTypeName]; ok {
		if convertedObj, ok := conversionFunc(object); ok {
			return convertedObj, true
		}
	}

	return nil, false
}
