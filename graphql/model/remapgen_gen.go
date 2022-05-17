// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

var typeConversionMap = map[string]func(object interface{}) (objectAsType interface{}, ok bool){
	"AddUserAddressPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(AddUserAddressPayloadOrError)
		return obj, ok
	},

	"AuthorizationError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(AuthorizationError)
		return obj, ok
	},

	"CollectionByIdOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CollectionByIDOrError)
		return obj, ok
	},

	"CollectionNftByIdOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CollectionNftByIDOrError)
		return obj, ok
	},

	"CommunityByAddressOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CommunityByAddressOrError)
		return obj, ok
	},

	"CreateCollectionPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(CreateCollectionPayloadOrError)
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

	"Error": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(Error)
		return obj, ok
	},

	"FollowUserPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(FollowUserPayloadOrError)
		return obj, ok
	},

	"GalleryUserOrWallet": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(GalleryUserOrWallet)
		return obj, ok
	},

	"GetAuthNoncePayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(GetAuthNoncePayloadOrError)
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

	"NftByIdOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(NftByIDOrError)
		return obj, ok
	},

	"Node": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(Node)
		return obj, ok
	},

	"RefreshOpenSeaNftsPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RefreshOpenSeaNftsPayloadOrError)
		return obj, ok
	},

	"RemoveUserAddressesPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(RemoveUserAddressesPayloadOrError)
		return obj, ok
	},

	"UnfollowUserPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UnfollowUserPayloadOrError)
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

	"UpdateCollectionNftsPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateCollectionNftsPayloadOrError)
		return obj, ok
	},

	"UpdateGalleryCollectionsPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateGalleryCollectionsPayloadOrError)
		return obj, ok
	},

	"UpdateNftInfoPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateNftInfoPayloadOrError)
		return obj, ok
	},

	"UpdateUserInfoPayloadOrError": func(object interface{}) (interface{}, bool) {
		obj, ok := object.(UpdateUserInfoPayloadOrError)
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
