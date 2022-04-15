package indexer

import "github.com/mikeydub/go-gallery/service/persist"

// GetContractOutput is the response for getting a single smart contract
type GetContractOutput struct {
	Contract persist.Contract `json:"contract"`
}
