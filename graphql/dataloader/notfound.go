package dataloader

import (
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

func (*CountAdmiresByFeedEventIDBatch) getNotFoundError(key persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountAdmiresByPostIDBatch) getNotFoundError(key persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountAdmiresByTokenIDBatch) getNotFoundError(key persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountAdmiresByCommentIDBatch) getNotFoundError(key persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountCommentsByFeedEventIDBatch) getNotFoundError(key persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountCommentsByPostIDBatch) getNotFoundError(key persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountRepliesByCommentIDBatch) getNotFoundError(key persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetAdmireByActorIDAndFeedEventID) getNotFoundError(key coredb.GetAdmireByActorIDAndFeedEventIDParams) error {
	return persist.ErrAdmireNotFoundByActorIDFeedEventID{ActorID: key.ActorID, FeedEventID: key.FeedEventID}
}

func (*GetAdmireByActorIDAndPostID) getNotFoundError(key coredb.GetAdmireByActorIDAndPostIDParams) error {
	return persist.ErrAdmireNotFoundByActorIDPostID{ActorID: key.ActorID, PostID: key.PostID}
}

func (*GetAdmireByActorIDAndTokenID) getNotFoundError(key coredb.GetAdmireByActorIDAndTokenIDParams) error {
	return persist.ErrAdmireNotFoundByActorIDTokenID{ActorID: key.ActorID, TokenID: key.TokenID}
}

func (*GetAdmireByActorIDAndCommentID) getNotFoundError(key coredb.GetAdmireByActorIDAndCommentIDParams) error {
	return persist.ErrAdmireNotFoundByActorIDCommentID{ActorID: key.ActorID, CommentID: key.CommentID}
}

func (*GetAdmireByAdmireIDBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrAdmireNotFoundByID{ID: key}
}

func (*GetCollectionByIdBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrCollectionNotFoundByID{ID: key}
}

func (*GetCommentByCommentIDBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrCommentNotFoundByID{ID: key}
}

func (*GetContractByChainAddressBatch) getNotFoundError(key coredb.GetContractByChainAddressBatchParams) error {
	return persist.ErrContractNotFoundByAddress{Address: key.Address, Chain: key.Chain}
}

func (*GetEventByIdBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrFeedEventNotFoundByID{ID: key}
}

func (*GetGalleryByCollectionIdBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrGalleryNotFound{CollectionID: key}
}

func (*GetGalleryByIdBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrGalleryNotFound{ID: key}
}

func (*GetMembershipByMembershipIdBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrMembershipNotFoundByID{ID: key}
}

func (*GetNotificationByIDBatch) getNotFoundError(key persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetPostByIdBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrPostNotFoundByID{ID: key}
}

func (*GetProfileImageByIdBatch) getNotFoundError(key coredb.GetProfileImageByIdBatchParams) error {
	return persist.ErrProfileImageNotFound{Err: pgx.ErrNoRows, ProfileImageID: key.ID}
}

func (*GetTokenByIdBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrTokenNotFoundByID{ID: key}
}

func (*GetTokenByIdIgnoreDisplayableBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrTokenNotFoundByID{ID: key}
}

func (*GetTokenByUserTokenIdentifiersBatch) getNotFoundError(key coredb.GetTokenByUserTokenIdentifiersBatchParams) error {
	return persist.ErrTokenNotFoundByUserTokenIdentifers{
		UserID: key.OwnerID,
		Token: persist.TokenIdentifiers{
			TokenID:         key.TokenID,
			ContractAddress: key.ContractAddress,
			Chain:           key.Chain,
		},
	}
}

func (*GetTokenByUserTokenIdentifiersIgnoreDisplayableBatch) getNotFoundError(key coredb.GetTokenByUserTokenIdentifiersIgnoreDisplayableBatchParams) error {
	return persist.ErrTokenNotFoundByUserTokenIdentifers{
		UserID: key.OwnerID,
		Token: persist.TokenIdentifiers{
			TokenID:         key.TokenID,
			ContractAddress: key.ContractAddress,
			Chain:           key.Chain,
		},
	}
}

func (*GetUserByAddressAndL1Batch) getNotFoundError(key coredb.GetUserByAddressAndL1BatchParams) error {
	return persist.ErrUserNotFound{L1ChainAddress: persist.NewL1ChainAddress(key.Address, persist.Chain(key.L1Chain))}
}

func (*GetUserByIdBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrUserNotFound{UserID: key}
}

func (*GetUserByUsernameBatch) getNotFoundError(key string) error {
	return persist.ErrUserNotFound{Username: key}
}

func (*GetWalletByIDBatch) getNotFoundError(key persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetContractCreatorsByIds) getNotFoundError(key string) error {
	return persist.ErrContractCreatorNotFound{ContractID: persist.DBID(key)}
}

func (*GetContractsByIDs) getNotFoundError(key string) error {
	return pgx.ErrNoRows
}

func (*GetMediaByMediaIdIgnoringStatusBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrMediaNotFoundByID{ID: key}
}

func (*GetTokenDefinitionByIdBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrTokenDefinitionNotFoundByID{ID: key}
}

func (*GetCommunityByKey) getNotFoundError(key coredb.GetCommunityByKeyParams) error {
	return persist.ErrCommunityNotFound{Key: persist.CommunityKey{
		Type: persist.CommunityType(key.Type),
		Key1: key.Key1,
		Key2: key.Key2,
		Key3: key.Key3,
		Key4: key.Key4,
	}}
}

func (*GetCommunityByIDBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrCommunityNotFound{ID: key}
}

func (*CountGalleriesDisplayingCommunityIDBatch) getNotFoundError(key persist.DBID) error {
	return persist.ErrGalleryNotFound{ID: key}
}

func (*GetTokenDefinitionByTokenDbidBatch) getNotFoundError(key persist.DBID) error {
	return fmt.Errorf("tokenDefinition not found by tokenDBID=%s", key)
}
