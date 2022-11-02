// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.13.0

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
	UserID              sql.NullString
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

type Backup struct {
	ID          persist.DBID
	Deleted     bool
	Version     sql.NullInt32
	CreatedAt   time.Time
	LastUpdated time.Time
	GalleryID   sql.NullString
	Gallery     pgtype.JSONB
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
	UserID       sql.NullString
	CollectionID sql.NullString
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
	Address string
}

type Event struct {
	ID             persist.DBID
	Version        int32
	ActorID        persist.DBID
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
	ExternalID     persist.NullString
	Caption        persist.NullString
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
	ForceEnabledUserIds []string
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
	Caption     persist.NullString
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
	Address            sql.NullString
	RequestHostAddress sql.NullString
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
	TokenID     sql.NullString
	Name        sql.NullString
	AssetUrl    sql.NullString
	Owners      persist.TokenHolderList
}

type NftEvent struct {
	ID          persist.DBID
	UserID      sql.NullString
	NftID       sql.NullString
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
	UserID      sql.NullString
	Address     sql.NullString
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
	TokenID              sql.NullString
	Quantity             sql.NullString
	OwnershipHistory     []pgtype.JSONB
	TokenMetadata        persist.TokenMetadata
	ExternalUrl          sql.NullString
	BlockNumber          sql.NullInt64
	OwnerUserID          persist.DBID
	OwnedByWallets       persist.DBIDList
	Chain                persist.Chain
	Contract             persist.DBID
	IsUserMarkedSpam     sql.NullBool
	IsProviderMarkedSpam sql.NullBool
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
}

type UserEvent struct {
	ID          persist.DBID
	UserID      sql.NullString
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
