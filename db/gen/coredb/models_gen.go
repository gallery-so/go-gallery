// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.15.0

package coredb

import (
	"database/sql"
	"time"

	"github.com/jackc/pgtype"
	"github.com/mikeydub/go-gallery/service/persist"
)

type Access struct {
	ID                  persist.DBID
	Deleted             bool
	Version             sql.NullInt32
	CreatedAt           time.Time
	LastUpdated         time.Time
	UserID              persist.DBID
	MostRecentBlock     sql.NullInt64
	RequiredTokensOwned pgtype.JSONB
	IsAdmin             sql.NullBool
}

type Admire struct {
	ID          persist.DBID
	Version     int32
	FeedEventID persist.DBID
	ActorID     persist.DBID
	Deleted     bool
	CreatedAt   time.Time
	LastUpdated time.Time
}

type Collection struct {
	ID             persist.DBID
	Deleted        bool
	OwnerUserID    persist.DBID
	Nfts           persist.DBIDList
	Version        sql.NullInt32
	LastUpdated    time.Time
	CreatedAt      time.Time
	Hidden         bool
	CollectorsNote sql.NullString
	Name           sql.NullString
	Layout         persist.TokenLayout
	TokenSettings  map[persist.DBID]persist.CollectionTokenSettings
}

type CollectionEvent struct {
	ID           persist.DBID
	UserID       persist.DBID
	CollectionID persist.DBID
	Version      sql.NullInt32
	EventCode    sql.NullInt32
	CreatedAt    time.Time
	LastUpdated  time.Time
	Data         pgtype.JSONB
	Sent         sql.NullBool
}

type Comment struct {
	ID          persist.DBID
	Version     int32
	FeedEventID persist.DBID
	ActorID     persist.DBID
	ReplyTo     persist.DBID
	Comment     string
	Deleted     bool
	CreatedAt   time.Time
	LastUpdated time.Time
}

type Contract struct {
	ID               persist.DBID
	Deleted          bool
	Version          sql.NullInt32
	CreatedAt        time.Time
	LastUpdated      time.Time
	Name             sql.NullString
	Symbol           sql.NullString
	Address          persist.Address
	CreatorAddress   persist.Address
	Chain            persist.Chain
	ProfileBannerUrl sql.NullString
	ProfileImageUrl  sql.NullString
	BadgeUrl         sql.NullString
	Description      sql.NullString
}

type EarlyAccess struct {
	Address persist.Address
}

type Event struct {
	ID             persist.DBID
	Version        int32
	ActorID        sql.NullString
	ResourceTypeID persist.ResourceType
	SubjectID      persist.DBID
	UserID         persist.DBID
	TokenID        persist.DBID
	CollectionID   persist.DBID
	Action         persist.Action
	Data           persist.EventData
	Deleted        bool
	LastUpdated    time.Time
	CreatedAt      time.Time
	GalleryID      persist.DBID
	CommentID      persist.DBID
	AdmireID       persist.DBID
	FeedEventID    persist.DBID
	ExternalID     sql.NullString
	Caption        sql.NullString
}

type Feature struct {
	ID                  persist.DBID
	Deleted             bool
	Version             sql.NullInt32
	LastUpdated         time.Time
	CreatedAt           time.Time
	RequiredToken       sql.NullString
	RequiredAmount      sql.NullInt64
	TokenType           sql.NullString
	Name                sql.NullString
	IsEnabled           sql.NullBool
	AdminOnly           sql.NullBool
	ForceEnabledUserIds persist.DBIDList
}

type FeedBlocklist struct {
	ID          persist.DBID
	UserID      persist.DBID
	Action      persist.Action
	LastUpdated time.Time
	CreatedAt   time.Time
	Deleted     bool
}

type FeedEvent struct {
	ID          persist.DBID
	Version     int32
	OwnerID     persist.DBID
	Action      persist.Action
	Data        persist.FeedEventData
	EventTime   time.Time
	EventIds    persist.DBIDList
	Deleted     bool
	LastUpdated time.Time
	CreatedAt   time.Time
	Caption     sql.NullString
}

type Follow struct {
	ID          persist.DBID
	Follower    persist.DBID
	Followee    persist.DBID
	Deleted     bool
	CreatedAt   time.Time
	LastUpdated time.Time
}

type Gallery struct {
	ID          persist.DBID
	Deleted     bool
	LastUpdated time.Time
	CreatedAt   time.Time
	Version     sql.NullInt32
	OwnerUserID persist.DBID
	Collections persist.DBIDList
}

type LoginAttempt struct {
	ID                 persist.DBID
	Deleted            bool
	Version            sql.NullInt32
	CreatedAt          time.Time
	LastUpdated        time.Time
	Address            persist.Address
	RequestHostAddress persist.Address
	UserExists         sql.NullBool
	Signature          sql.NullString
	SignatureValid     sql.NullBool
	RequestHeaders     pgtype.JSONB
	NonceValue         sql.NullString
}

type Membership struct {
	ID          persist.DBID
	Deleted     bool
	Version     sql.NullInt32
	CreatedAt   time.Time
	LastUpdated time.Time
	TokenID     persist.DBID
	Name        sql.NullString
	AssetUrl    sql.NullString
	Owners      persist.TokenHolderList
}

type NftEvent struct {
	ID          persist.DBID
	UserID      persist.DBID
	NftID       persist.DBID
	Version     sql.NullInt32
	EventCode   sql.NullInt32
	CreatedAt   time.Time
	LastUpdated time.Time
	Data        pgtype.JSONB
	Sent        sql.NullBool
}

type Nonce struct {
	ID          persist.DBID
	Deleted     bool
	Version     sql.NullInt32
	LastUpdated time.Time
	CreatedAt   time.Time
	UserID      persist.DBID
	Address     persist.Address
	Value       sql.NullString
	Chain       persist.Chain
}

type Notification struct {
	ID          persist.DBID
	Deleted     bool
	OwnerID     persist.DBID
	Version     sql.NullInt32
	LastUpdated time.Time
	CreatedAt   time.Time
	Action      persist.Action
	Data        persist.NotificationData
	EventIds    persist.DBIDList
	FeedEventID persist.DBID
	CommentID   persist.DBID
	GalleryID   persist.DBID
	Seen        bool
	Amount      int32
}

type Token struct {
	ID                   persist.DBID
	Deleted              bool
	Version              sql.NullInt32
	CreatedAt            time.Time
	LastUpdated          time.Time
	Name                 sql.NullString
	Description          sql.NullString
	CollectorsNote       sql.NullString
	Media                persist.Media
	TokenUri             sql.NullString
	TokenType            sql.NullString
	TokenID              persist.TokenID
	Quantity             sql.NullString
	OwnershipHistory     persist.AddressAtBlockList
	TokenMetadata        persist.TokenMetadata
	ExternalUrl          sql.NullString
	BlockNumber          sql.NullInt64
	OwnerUserID          persist.DBID
	OwnedByWallets       persist.DBIDList
	Chain                persist.Chain
	Contract             persist.DBID
	IsUserMarkedSpam     sql.NullBool
	IsProviderMarkedSpam sql.NullBool
	LastSynced           time.Time
}

type User struct {
	ID                   persist.DBID
	Deleted              bool
	Version              sql.NullInt32
	LastUpdated          time.Time
	CreatedAt            time.Time
	Username             sql.NullString
	UsernameIdempotent   sql.NullString
	Wallets              persist.WalletList
	Bio                  sql.NullString
	Traits               pgtype.JSONB
	Universal            bool
	NotificationSettings persist.UserNotificationSettings
	Email                persist.NullString
	EmailVerified        persist.EmailVerificationStatus
	EmailUnsubscriptions persist.EmailUnsubscriptions
}

type UserEvent struct {
	ID          persist.DBID
	UserID      persist.DBID
	Version     sql.NullInt32
	EventCode   sql.NullInt32
	CreatedAt   time.Time
	LastUpdated time.Time
	Data        pgtype.JSONB
	Sent        sql.NullBool
}

type Wallet struct {
	ID          persist.DBID
	CreatedAt   time.Time
	LastUpdated time.Time
	Deleted     bool
	Version     sql.NullInt32
	Address     persist.Address
	WalletType  persist.WalletType
	Chain       persist.Chain
}
