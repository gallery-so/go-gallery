package dataloader

import (
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

func (*GetUserByUsernameBatch) getKeyForResult(user coredb.User) string {
	return user.Username.String
}

func (*GetGalleryByCollectionIdBatch) getKeysForResult(gallery coredb.Gallery) []persist.DBID {
	return gallery.Collections
}

func (*GetContractsByIDs) getKeyForResult(contract coredb.Contract) string {
	return contract.ID.String()
}

func (*GetContractByChainAddressBatch) getKeyForResult(contract coredb.Contract) coredb.GetContractByChainAddressBatchParams {
	return coredb.GetContractByChainAddressBatchParams{Address: contract.Address, Chain: contract.Chain}
}
