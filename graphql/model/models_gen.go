// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

type AddUserWalletPayloadOrError interface {
	IsAddUserWalletPayloadOrError()
}

type AuthorizationError interface {
	IsAuthorizationError()
}

type CollectionByIDOrError interface {
	IsCollectionByIDOrError()
}

type CollectionTokenByIDOrError interface {
	IsCollectionTokenByIDOrError()
}

type CommunityByAddressOrError interface {
	IsCommunityByAddressOrError()
}

type CreateCollectionPayloadOrError interface {
	IsCreateCollectionPayloadOrError()
}

type CreateUserPayloadOrError interface {
	IsCreateUserPayloadOrError()
}

type DeleteCollectionPayloadOrError interface {
	IsDeleteCollectionPayloadOrError()
}

type Error interface {
	IsError()
}

type FeedEventByIDOrError interface {
	IsFeedEventByIDOrError()
}

type FeedEventData interface {
	IsFeedEventData()
}

type FeedEventOrError interface {
	IsFeedEventOrError()
}

type FollowUserPayloadOrError interface {
	IsFollowUserPayloadOrError()
}

type GalleryUserOrAddress interface {
	IsGalleryUserOrAddress()
}

type GalleryUserOrWallet interface {
	IsGalleryUserOrWallet()
}

type GetAuthNoncePayloadOrError interface {
	IsGetAuthNoncePayloadOrError()
}

type LoginPayloadOrError interface {
	IsLoginPayloadOrError()
}

type Media interface {
	IsMedia()
}

type MediaSubtype interface {
	IsMediaSubtype()
}

type Node interface {
	IsNode()
}

type RefreshContractPayloadOrError interface {
	IsRefreshContractPayloadOrError()
}

type RefreshTokenPayloadOrError interface {
	IsRefreshTokenPayloadOrError()
}

type RemoveUserWalletsPayloadOrError interface {
	IsRemoveUserWalletsPayloadOrError()
}

type SyncTokensPayloadOrError interface {
	IsSyncTokensPayloadOrError()
}

type TokenByIDOrError interface {
	IsTokenByIDOrError()
}

type UnfollowUserPayloadOrError interface {
	IsUnfollowUserPayloadOrError()
}

type UpdateCollectionHiddenPayloadOrError interface {
	IsUpdateCollectionHiddenPayloadOrError()
}

type UpdateCollectionInfoPayloadOrError interface {
	IsUpdateCollectionInfoPayloadOrError()
}

type UpdateCollectionTokensPayloadOrError interface {
	IsUpdateCollectionTokensPayloadOrError()
}

type UpdateGalleryCollectionsPayloadOrError interface {
	IsUpdateGalleryCollectionsPayloadOrError()
}

type UpdateTokenInfoPayloadOrError interface {
	IsUpdateTokenInfoPayloadOrError()
}

type UpdateUserInfoPayloadOrError interface {
	IsUpdateUserInfoPayloadOrError()
}

type UserByIDOrError interface {
	IsUserByIDOrError()
}

type UserByUsernameOrError interface {
	IsUserByUsernameOrError()
}

type ViewerOrError interface {
	IsViewerOrError()
}

type AddUserWalletPayload struct {
	Viewer *Viewer `json:"viewer"`
}

func (AddUserWalletPayload) IsAddUserWalletPayloadOrError() {}

type AudioMedia struct {
	PreviewURLs      *PreviewURLSet `json:"previewURLs"`
	MediaURL         *string        `json:"mediaURL"`
	MediaType        *string        `json:"mediaType"`
	ContentRenderURL *string        `json:"contentRenderURL"`
}

func (AudioMedia) IsMediaSubtype() {}
func (AudioMedia) IsMedia()        {}

type AuthMechanism struct {
	Eoa        *EoaAuth        `json:"eoa"`
	GnosisSafe *GnosisSafeAuth `json:"gnosisSafe"`
	Debug      *DebugAuth      `json:"debug"`
}

type AuthNonce struct {
	Nonce      *string `json:"nonce"`
	UserExists *bool   `json:"userExists"`
}

func (AuthNonce) IsGetAuthNoncePayloadOrError() {}

type Collection struct {
	Dbid           persist.DBID       `json:"dbid"`
	Version        *int               `json:"version"`
	Name           *string            `json:"name"`
	CollectorsNote *string            `json:"collectorsNote"`
	Gallery        *Gallery           `json:"gallery"`
	Layout         *CollectionLayout  `json:"layout"`
	Hidden         *bool              `json:"hidden"`
	Tokens         []*CollectionToken `json:"tokens"`
}

func (Collection) IsNode()                  {}
func (Collection) IsCollectionByIDOrError() {}

type CollectionCreatedFeedEventData struct {
	EventTime  *time.Time      `json:"eventTime"`
	Owner      *GalleryUser    `json:"owner"`
	Action     *persist.Action `json:"action"`
	Collection *Collection     `json:"collection"`
}

func (CollectionCreatedFeedEventData) IsFeedEventData() {}

type CollectionLayout struct {
	Columns    *int   `json:"columns"`
	Whitespace []*int `json:"whitespace"`
}

type CollectionLayoutInput struct {
	Columns    int   `json:"columns"`
	Whitespace []int `json:"whitespace"`
}

type CollectionToken struct {
	HelperCollectionTokenData
	Token      *Token      `json:"token"`
	Collection *Collection `json:"collection"`
}

func (CollectionToken) IsNode()                       {}
func (CollectionToken) IsCollectionTokenByIDOrError() {}

type CollectorsNoteAddedToCollectionFeedEventData struct {
	EventTime         *time.Time      `json:"eventTime"`
	Owner             *GalleryUser    `json:"owner"`
	Action            *persist.Action `json:"action"`
	Collection        *Collection     `json:"collection"`
	NewCollectorsNote *string         `json:"newCollectorsNote"`
}

func (CollectorsNoteAddedToCollectionFeedEventData) IsFeedEventData() {}

type CollectorsNoteAddedToTokenFeedEventData struct {
	EventTime         *time.Time       `json:"eventTime"`
	Owner             *GalleryUser     `json:"owner"`
	Action            *persist.Action  `json:"action"`
	Token             *CollectionToken `json:"token"`
	NewCollectorsNote *string          `json:"newCollectorsNote"`
}

func (CollectorsNoteAddedToTokenFeedEventData) IsFeedEventData() {}

type Community struct {
	LastUpdated     *time.Time            `json:"lastUpdated"`
	ContractAddress *persist.ChainAddress `json:"contractAddress"`
	CreatorAddress  *persist.ChainAddress `json:"creatorAddress"`
	Chain           *persist.Chain        `json:"chain"`
	Name            *string               `json:"name"`
	Description     *string               `json:"description"`
	PreviewImage    *string               `json:"previewImage"`
	Owners          []*TokenHolder        `json:"owners"`
}

func (Community) IsNode()                      {}
func (Community) IsCommunityByAddressOrError() {}

type Contract struct {
	Dbid            persist.DBID          `json:"dbid"`
	LastUpdated     *time.Time            `json:"lastUpdated"`
	ContractAddress *persist.ChainAddress `json:"contractAddress"`
	CreatorAddress  *persist.ChainAddress `json:"creatorAddress"`
	Chain           *persist.Chain        `json:"chain"`
	Name            *string               `json:"name"`
}

func (Contract) IsNode() {}

type CreateCollectionInput struct {
	GalleryID      persist.DBID           `json:"galleryId"`
	Name           string                 `json:"name"`
	CollectorsNote string                 `json:"collectorsNote"`
	Tokens         []persist.DBID         `json:"tokens"`
	Layout         *CollectionLayoutInput `json:"layout"`
}

type CreateCollectionPayload struct {
	Collection *Collection `json:"collection"`
}

func (CreateCollectionPayload) IsCreateCollectionPayloadOrError() {}

type CreateUserPayload struct {
	UserID    *persist.DBID `json:"userId"`
	GalleryID *persist.DBID `json:"galleryId"`
	Viewer    *Viewer       `json:"viewer"`
}

func (CreateUserPayload) IsCreateUserPayloadOrError() {}

type DebugAuth struct {
	UserID         *persist.DBID           `json:"userId"`
	ChainAddresses []*persist.ChainAddress `json:"chainAddresses"`
}

type DeleteCollectionPayload struct {
	Gallery *Gallery `json:"gallery"`
}

func (DeleteCollectionPayload) IsDeleteCollectionPayloadOrError() {}

type EoaAuth struct {
	ChainAddress *persist.ChainAddress `json:"chainAddress"`
	Nonce        string                `json:"nonce"`
	Signature    string                `json:"signature"`
}

type ErrAddressOwnedByUser struct {
	Message string `json:"message"`
}

func (ErrAddressOwnedByUser) IsAddUserWalletPayloadOrError() {}
func (ErrAddressOwnedByUser) IsError()                       {}

type ErrAuthenticationFailed struct {
	Message string `json:"message"`
}

func (ErrAuthenticationFailed) IsAddUserWalletPayloadOrError() {}
func (ErrAuthenticationFailed) IsError()                       {}
func (ErrAuthenticationFailed) IsLoginPayloadOrError()         {}
func (ErrAuthenticationFailed) IsCreateUserPayloadOrError()    {}
func (ErrAuthenticationFailed) IsFollowUserPayloadOrError()    {}
func (ErrAuthenticationFailed) IsUnfollowUserPayloadOrError()  {}

type ErrCollectionNotFound struct {
	Message string `json:"message"`
}

func (ErrCollectionNotFound) IsError()                          {}
func (ErrCollectionNotFound) IsCollectionByIDOrError()          {}
func (ErrCollectionNotFound) IsCollectionTokenByIDOrError()     {}
func (ErrCollectionNotFound) IsDeleteCollectionPayloadOrError() {}

type ErrCommunityNotFound struct {
	Message string `json:"message"`
}

func (ErrCommunityNotFound) IsCommunityByAddressOrError() {}
func (ErrCommunityNotFound) IsError()                     {}

type ErrDoesNotOwnRequiredToken struct {
	Message string `json:"message"`
}

func (ErrDoesNotOwnRequiredToken) IsGetAuthNoncePayloadOrError() {}
func (ErrDoesNotOwnRequiredToken) IsAuthorizationError()         {}
func (ErrDoesNotOwnRequiredToken) IsError()                      {}
func (ErrDoesNotOwnRequiredToken) IsLoginPayloadOrError()        {}
func (ErrDoesNotOwnRequiredToken) IsCreateUserPayloadOrError()   {}

type ErrFeedEventNotFound struct {
	Message string `json:"message"`
}

func (ErrFeedEventNotFound) IsError()                {}
func (ErrFeedEventNotFound) IsFeedEventOrError()     {}
func (ErrFeedEventNotFound) IsFeedEventByIDOrError() {}

type ErrInvalidInput struct {
	Message    string   `json:"message"`
	Parameters []string `json:"parameters"`
	Reasons    []string `json:"reasons"`
}

func (ErrInvalidInput) IsUserByUsernameOrError()                  {}
func (ErrInvalidInput) IsUserByIDOrError()                        {}
func (ErrInvalidInput) IsCommunityByAddressOrError()              {}
func (ErrInvalidInput) IsCreateCollectionPayloadOrError()         {}
func (ErrInvalidInput) IsDeleteCollectionPayloadOrError()         {}
func (ErrInvalidInput) IsUpdateCollectionInfoPayloadOrError()     {}
func (ErrInvalidInput) IsUpdateCollectionTokensPayloadOrError()   {}
func (ErrInvalidInput) IsUpdateCollectionHiddenPayloadOrError()   {}
func (ErrInvalidInput) IsUpdateGalleryCollectionsPayloadOrError() {}
func (ErrInvalidInput) IsUpdateTokenInfoPayloadOrError()          {}
func (ErrInvalidInput) IsAddUserWalletPayloadOrError()            {}
func (ErrInvalidInput) IsRemoveUserWalletsPayloadOrError()        {}
func (ErrInvalidInput) IsUpdateUserInfoPayloadOrError()           {}
func (ErrInvalidInput) IsRefreshTokenPayloadOrError()             {}
func (ErrInvalidInput) IsRefreshContractPayloadOrError()          {}
func (ErrInvalidInput) IsError()                                  {}
func (ErrInvalidInput) IsFollowUserPayloadOrError()               {}
func (ErrInvalidInput) IsUnfollowUserPayloadOrError()             {}

type ErrInvalidToken struct {
	Message string `json:"message"`
}

func (ErrInvalidToken) IsAuthorizationError() {}
func (ErrInvalidToken) IsError()              {}

type ErrNoCookie struct {
	Message string `json:"message"`
}

func (ErrNoCookie) IsAuthorizationError() {}
func (ErrNoCookie) IsError()              {}

type ErrNotAuthorized struct {
	Message string             `json:"message"`
	Cause   AuthorizationError `json:"cause"`
}

func (ErrNotAuthorized) IsViewerOrError()                          {}
func (ErrNotAuthorized) IsCreateCollectionPayloadOrError()         {}
func (ErrNotAuthorized) IsDeleteCollectionPayloadOrError()         {}
func (ErrNotAuthorized) IsUpdateCollectionInfoPayloadOrError()     {}
func (ErrNotAuthorized) IsUpdateCollectionTokensPayloadOrError()   {}
func (ErrNotAuthorized) IsUpdateCollectionHiddenPayloadOrError()   {}
func (ErrNotAuthorized) IsUpdateGalleryCollectionsPayloadOrError() {}
func (ErrNotAuthorized) IsUpdateTokenInfoPayloadOrError()          {}
func (ErrNotAuthorized) IsAddUserWalletPayloadOrError()            {}
func (ErrNotAuthorized) IsRemoveUserWalletsPayloadOrError()        {}
func (ErrNotAuthorized) IsUpdateUserInfoPayloadOrError()           {}
func (ErrNotAuthorized) IsSyncTokensPayloadOrError()               {}
func (ErrNotAuthorized) IsError()                                  {}

type ErrOpenSeaRefreshFailed struct {
	Message string `json:"message"`
}

func (ErrOpenSeaRefreshFailed) IsSyncTokensPayloadOrError()      {}
func (ErrOpenSeaRefreshFailed) IsRefreshTokenPayloadOrError()    {}
func (ErrOpenSeaRefreshFailed) IsRefreshContractPayloadOrError() {}
func (ErrOpenSeaRefreshFailed) IsError()                         {}

type ErrTokenNotFound struct {
	Message string `json:"message"`
}

func (ErrTokenNotFound) IsTokenByIDOrError()           {}
func (ErrTokenNotFound) IsError()                      {}
func (ErrTokenNotFound) IsCollectionTokenByIDOrError() {}

type ErrUnknownAction struct {
	Message string `json:"message"`
}

func (ErrUnknownAction) IsError()                {}
func (ErrUnknownAction) IsFeedEventOrError()     {}
func (ErrUnknownAction) IsFeedEventByIDOrError() {}

type ErrUserAlreadyExists struct {
	Message string `json:"message"`
}

func (ErrUserAlreadyExists) IsUpdateUserInfoPayloadOrError() {}
func (ErrUserAlreadyExists) IsError()                        {}
func (ErrUserAlreadyExists) IsCreateUserPayloadOrError()     {}

type ErrUserNotFound struct {
	Message string `json:"message"`
}

func (ErrUserNotFound) IsUserByUsernameOrError()      {}
func (ErrUserNotFound) IsUserByIDOrError()            {}
func (ErrUserNotFound) IsError()                      {}
func (ErrUserNotFound) IsLoginPayloadOrError()        {}
func (ErrUserNotFound) IsFollowUserPayloadOrError()   {}
func (ErrUserNotFound) IsUnfollowUserPayloadOrError() {}

type FeedConnection struct {
	HelperFeedConnectionData
	Edges    []*FeedEdge `json:"edges"`
	PageInfo *PageInfo   `json:"pageInfo"`
}

type FeedEdge struct {
	Node   FeedEventOrError `json:"node"`
	Cursor string           `json:"cursor"`
}

type FeedEvent struct {
	Dbid      persist.DBID  `json:"dbid"`
	EventData FeedEventData `json:"eventData"`
}

func (FeedEvent) IsNode()                 {}
func (FeedEvent) IsFeedEventOrError()     {}
func (FeedEvent) IsFeedEventByIDOrError() {}

type FollowInfo struct {
	User         *GalleryUser `json:"user"`
	FollowedBack *bool        `json:"followedBack"`
}

type FollowUserPayload struct {
	Viewer *Viewer      `json:"viewer"`
	User   *GalleryUser `json:"user"`
}

func (FollowUserPayload) IsFollowUserPayloadOrError() {}

type Gallery struct {
	Dbid        persist.DBID  `json:"dbid"`
	Owner       *GalleryUser  `json:"owner"`
	Collections []*Collection `json:"collections"`
}

func (Gallery) IsNode() {}

type GalleryUser struct {
	Dbid                persist.DBID   `json:"dbid"`
	Username            *string        `json:"username"`
	Bio                 *string        `json:"bio"`
	Tokens              []*Token       `json:"tokens"`
	Wallets             []*Wallet      `json:"wallets"`
	Galleries           []*Gallery     `json:"galleries"`
	IsAuthenticatedUser *bool          `json:"isAuthenticatedUser"`
	Followers           []*GalleryUser `json:"followers"`
	Following           []*GalleryUser `json:"following"`
}

func (GalleryUser) IsNode()                  {}
func (GalleryUser) IsGalleryUserOrWallet()   {}
func (GalleryUser) IsGalleryUserOrAddress()  {}
func (GalleryUser) IsUserByUsernameOrError() {}
func (GalleryUser) IsUserByIDOrError()       {}

type GltfMedia struct {
	PreviewURLs      *PreviewURLSet `json:"previewURLs"`
	MediaURL         *string        `json:"mediaURL"`
	MediaType        *string        `json:"mediaType"`
	ContentRenderURL *string        `json:"contentRenderURL"`
}

func (GltfMedia) IsMediaSubtype() {}
func (GltfMedia) IsMedia()        {}

type GnosisSafeAuth struct {
	Address persist.Address `json:"address"`
	Nonce   string          `json:"nonce"`
}

type HTMLMedia struct {
	PreviewURLs      *PreviewURLSet `json:"previewURLs"`
	MediaURL         *string        `json:"mediaURL"`
	MediaType        *string        `json:"mediaType"`
	ContentRenderURL *string        `json:"contentRenderURL"`
}

func (HTMLMedia) IsMediaSubtype() {}
func (HTMLMedia) IsMedia()        {}

type ImageMedia struct {
	PreviewURLs      *PreviewURLSet `json:"previewURLs"`
	MediaURL         *string        `json:"mediaURL"`
	MediaType        *string        `json:"mediaType"`
	ContentRenderURL *string        `json:"contentRenderURL"`
}

func (ImageMedia) IsMediaSubtype() {}
func (ImageMedia) IsMedia()        {}

type InvalidMedia struct {
	PreviewURLs      *PreviewURLSet `json:"previewURLs"`
	MediaURL         *string        `json:"mediaURL"`
	MediaType        *string        `json:"mediaType"`
	ContentRenderURL *string        `json:"contentRenderURL"`
}

func (InvalidMedia) IsMediaSubtype() {}
func (InvalidMedia) IsMedia()        {}

type JSONMedia struct {
	PreviewURLs      *PreviewURLSet `json:"previewURLs"`
	MediaURL         *string        `json:"mediaURL"`
	MediaType        *string        `json:"mediaType"`
	ContentRenderURL *string        `json:"contentRenderURL"`
}

func (JSONMedia) IsMediaSubtype() {}
func (JSONMedia) IsMedia()        {}

type LoginPayload struct {
	UserID *persist.DBID `json:"userId"`
	Viewer *Viewer       `json:"viewer"`
}

func (LoginPayload) IsLoginPayloadOrError() {}

type LogoutPayload struct {
	Viewer *Viewer `json:"viewer"`
}

type MembershipTier struct {
	Dbid     persist.DBID   `json:"dbid"`
	Name     *string        `json:"name"`
	AssetURL *string        `json:"assetUrl"`
	TokenID  *string        `json:"tokenId"`
	Owners   []*TokenHolder `json:"owners"`
}

func (MembershipTier) IsNode() {}

type OwnerAtBlock struct {
	Owner       GalleryUserOrAddress `json:"owner"`
	BlockNumber *string              `json:"blockNumber"`
}

type PageInfo struct {
	Size            int    `json:"size"`
	HasPreviousPage bool   `json:"hasPreviousPage"`
	HasNextPage     bool   `json:"hasNextPage"`
	StartCursor     string `json:"startCursor"`
	EndCursor       string `json:"endCursor"`
}

type PreviewURLSet struct {
	Raw       *string `json:"raw"`
	Thumbnail *string `json:"thumbnail"`
	Small     *string `json:"small"`
	Medium    *string `json:"medium"`
	Large     *string `json:"large"`
	SrcSet    *string `json:"srcSet"`
}

type RefreshContractPayload struct {
	Contract *Contract `json:"contract"`
}

func (RefreshContractPayload) IsRefreshContractPayloadOrError() {}

type RefreshTokenPayload struct {
	Token *Token `json:"token"`
}

func (RefreshTokenPayload) IsRefreshTokenPayloadOrError() {}

type RemoveUserWalletsPayload struct {
	Viewer *Viewer `json:"viewer"`
}

func (RemoveUserWalletsPayload) IsRemoveUserWalletsPayloadOrError() {}

type SyncTokensPayload struct {
	Viewer *Viewer `json:"viewer"`
}

func (SyncTokensPayload) IsSyncTokensPayloadOrError() {}

type TextMedia struct {
	PreviewURLs      *PreviewURLSet `json:"previewURLs"`
	MediaURL         *string        `json:"mediaURL"`
	MediaType        *string        `json:"mediaType"`
	ContentRenderURL *string        `json:"contentRenderURL"`
}

func (TextMedia) IsMediaSubtype() {}
func (TextMedia) IsMedia()        {}

type Token struct {
	Dbid                  persist.DBID          `json:"dbid"`
	CreationTime          *time.Time            `json:"creationTime"`
	LastUpdated           *time.Time            `json:"lastUpdated"`
	CollectorsNote        *string               `json:"collectorsNote"`
	Media                 MediaSubtype          `json:"media"`
	TokenType             *TokenType            `json:"tokenType"`
	Chain                 *persist.Chain        `json:"chain"`
	Name                  *string               `json:"name"`
	Description           *string               `json:"description"`
	TokenURI              *string               `json:"tokenUri"`
	TokenID               *string               `json:"tokenId"`
	Quantity              *string               `json:"quantity"`
	Owner                 *GalleryUser          `json:"owner"`
	OwnedByWallets        []*Wallet             `json:"ownedByWallets"`
	OwnershipHistory      []*OwnerAtBlock       `json:"ownershipHistory"`
	TokenMetadata         *string               `json:"tokenMetadata"`
	Contract              *Contract             `json:"contract"`
	ExternalURL           *string               `json:"externalUrl"`
	BlockNumber           *string               `json:"blockNumber"`
	CreatorAddress        *persist.ChainAddress `json:"creatorAddress"`
	OpenseaCollectionName *string               `json:"openseaCollectionName"`
	OpenseaID             *int                  `json:"openseaId"`
}

func (Token) IsNode()             {}
func (Token) IsTokenByIDOrError() {}

type TokenHolder struct {
	HelperTokenHolderData
	Wallets       []*Wallet    `json:"wallets"`
	User          *GalleryUser `json:"user"`
	PreviewTokens []*string    `json:"previewTokens"`
}

type TokensAddedToCollectionFeedEventData struct {
	HelperTokensAddedToCollectionFeedEventDataData
	EventTime  *time.Time         `json:"eventTime"`
	Owner      *GalleryUser       `json:"owner"`
	Collection *Collection        `json:"collection"`
	Action     *persist.Action    `json:"action"`
	NewTokens  []*CollectionToken `json:"newTokens"`
	IsPreFeed  *bool              `json:"isPreFeed"`
}

func (TokensAddedToCollectionFeedEventData) IsFeedEventData() {}

type UnfollowUserPayload struct {
	Viewer *Viewer      `json:"viewer"`
	User   *GalleryUser `json:"user"`
}

func (UnfollowUserPayload) IsUnfollowUserPayloadOrError() {}

type UnknownMedia struct {
	PreviewURLs      *PreviewURLSet `json:"previewURLs"`
	MediaURL         *string        `json:"mediaURL"`
	MediaType        *string        `json:"mediaType"`
	ContentRenderURL *string        `json:"contentRenderURL"`
}

func (UnknownMedia) IsMediaSubtype() {}
func (UnknownMedia) IsMedia()        {}

type UpdateCollectionHiddenInput struct {
	CollectionID persist.DBID `json:"collectionId"`
	Hidden       bool         `json:"hidden"`
}

type UpdateCollectionHiddenPayload struct {
	Collection *Collection `json:"collection"`
}

func (UpdateCollectionHiddenPayload) IsUpdateCollectionHiddenPayloadOrError() {}

type UpdateCollectionInfoInput struct {
	CollectionID   persist.DBID `json:"collectionId"`
	Name           string       `json:"name"`
	CollectorsNote string       `json:"collectorsNote"`
}

type UpdateCollectionInfoPayload struct {
	Collection *Collection `json:"collection"`
}

func (UpdateCollectionInfoPayload) IsUpdateCollectionInfoPayloadOrError() {}

type UpdateCollectionTokensInput struct {
	CollectionID persist.DBID           `json:"collectionId"`
	Tokens       []persist.DBID         `json:"tokens"`
	Layout       *CollectionLayoutInput `json:"layout"`
}

type UpdateCollectionTokensPayload struct {
	Collection *Collection `json:"collection"`
}

func (UpdateCollectionTokensPayload) IsUpdateCollectionTokensPayloadOrError() {}

type UpdateGalleryCollectionsInput struct {
	GalleryID   persist.DBID   `json:"galleryId"`
	Collections []persist.DBID `json:"collections"`
}

type UpdateGalleryCollectionsPayload struct {
	Gallery *Gallery `json:"gallery"`
}

func (UpdateGalleryCollectionsPayload) IsUpdateGalleryCollectionsPayloadOrError() {}

type UpdateTokenInfoInput struct {
	TokenID        persist.DBID  `json:"tokenId"`
	CollectorsNote string        `json:"collectorsNote"`
	CollectionID   *persist.DBID `json:"collectionId"`
}

type UpdateTokenInfoPayload struct {
	Token *Token `json:"token"`
}

func (UpdateTokenInfoPayload) IsUpdateTokenInfoPayloadOrError() {}

type UpdateUserInfoInput struct {
	Username string `json:"username"`
	Bio      string `json:"bio"`
}

type UpdateUserInfoPayload struct {
	Viewer *Viewer `json:"viewer"`
}

func (UpdateUserInfoPayload) IsUpdateUserInfoPayloadOrError() {}

type UserCreatedFeedEventData struct {
	EventTime *time.Time      `json:"eventTime"`
	Owner     *GalleryUser    `json:"owner"`
	Action    *persist.Action `json:"action"`
}

func (UserCreatedFeedEventData) IsFeedEventData() {}

type UserFollowedUsersFeedEventData struct {
	EventTime *time.Time      `json:"eventTime"`
	Owner     *GalleryUser    `json:"owner"`
	Action    *persist.Action `json:"action"`
	Followed  []*FollowInfo   `json:"followed"`
}

func (UserFollowedUsersFeedEventData) IsFeedEventData() {}

type VideoMedia struct {
	PreviewURLs       *PreviewURLSet `json:"previewURLs"`
	MediaURL          *string        `json:"mediaURL"`
	MediaType         *string        `json:"mediaType"`
	ContentRenderURLs *VideoURLSet   `json:"contentRenderURLs"`
}

func (VideoMedia) IsMediaSubtype() {}
func (VideoMedia) IsMedia()        {}

type VideoURLSet struct {
	Raw    *string `json:"raw"`
	Small  *string `json:"small"`
	Medium *string `json:"medium"`
	Large  *string `json:"large"`
}

type Viewer struct {
	User            *GalleryUser     `json:"user"`
	ViewerGalleries []*ViewerGallery `json:"viewerGalleries"`
}

func (Viewer) IsViewerOrError() {}

type ViewerGallery struct {
	Gallery *Gallery `json:"gallery"`
}

type Wallet struct {
	Dbid         persist.DBID          `json:"dbid"`
	ChainAddress *persist.ChainAddress `json:"chainAddress"`
	Chain        *persist.Chain        `json:"chain"`
	WalletType   *persist.WalletType   `json:"walletType"`
	Tokens       []*Token              `json:"tokens"`
}

func (Wallet) IsNode()                {}
func (Wallet) IsGalleryUserOrWallet() {}

type TokenType string

const (
	TokenTypeErc721  TokenType = "ERC721"
	TokenTypeErc1155 TokenType = "ERC1155"
	TokenTypeErc20   TokenType = "ERC20"
)

var AllTokenType = []TokenType{
	TokenTypeErc721,
	TokenTypeErc1155,
	TokenTypeErc20,
}

func (e TokenType) IsValid() bool {
	switch e {
	case TokenTypeErc721, TokenTypeErc1155, TokenTypeErc20:
		return true
	}
	return false
}

func (e TokenType) String() string {
	return string(e)
}

func (e *TokenType) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = TokenType(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid TokenType", str)
	}
	return nil
}

func (e TokenType) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}
