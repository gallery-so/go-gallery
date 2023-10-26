package dataloader

import (
	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

func (*CountAdmiresByFeedEventIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountAdmiresByPostIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountAdmiresByTokenIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountCommentsByFeedEventIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountCommentsByPostIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*CountRepliesByCommentIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetAdmireByActorIDAndFeedEventID) getNotFoundError(coredb.GetAdmireByActorIDAndFeedEventIDParams) error {
	return pgx.ErrNoRows
}

func (*GetAdmireByActorIDAndPostID) getNotFoundError(coredb.GetAdmireByActorIDAndPostIDParams) error {
	return pgx.ErrNoRows
}

func (*GetAdmireByActorIDAndTokenID) getNotFoundError(coredb.GetAdmireByActorIDAndTokenIDParams) error {
	return pgx.ErrNoRows
}

func (*GetAdmireByAdmireIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetCollectionByIdBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetCommentByCommentIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetContractByChainAddressBatch) getNotFoundError(coredb.GetContractByChainAddressBatchParams) error {
	return pgx.ErrNoRows
}

func (*GetEventByIdBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetGalleryByCollectionIdBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetGalleryByIdBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetMediaByMediaIDIgnoringStatus) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetMembershipByMembershipIdBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetNotificationByIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetPostByIdBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetProfileImageByID) getNotFoundError(coredb.GetProfileImageByIDParams) error {
	return pgx.ErrNoRows
}

func (*GetTokenByIdBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetTokenByIdIgnoreDisplayableBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetTokenByUserTokenIdentifiersBatch) getNotFoundError(coredb.GetTokenByUserTokenIdentifiersBatchParams) error {
	return pgx.ErrNoRows
}

func (*GetTokenOwnerByIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetUserByAddressAndL1Batch) getNotFoundError(coredb.GetUserByAddressAndL1BatchParams) error {
	return pgx.ErrNoRows
}

func (*GetUserByIdBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetUserByUsernameBatch) getNotFoundError(string) error {
	return pgx.ErrNoRows
}

func (*GetWalletByIDBatch) getNotFoundError(persist.DBID) error {
	return pgx.ErrNoRows
}

func (*GetContractCreatorsByIds) getNotFoundError(string) error {
	return pgx.ErrNoRows
}

func (*GetContractsByIDs) getNotFoundError(string) error {
	return pgx.ErrNoRows
}
