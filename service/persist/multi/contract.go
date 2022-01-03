package multi

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

// ContractRepository is a contract repository for interacting with contracts from multiple other repositories
type ContractRepository struct {
	ReadRepo   persist.ContractRepository
	WriteRepos []persist.ContractRepository
}

// NewContractRepository returns a new ContractRepository
func NewContractRepository(readRepo persist.ContractRepository, writeRepos ...persist.ContractRepository) *ContractRepository {
	return &ContractRepository{ReadRepo: readRepo, WriteRepos: writeRepos}
}

// GetByAddress returns a contract by address
func (c *ContractRepository) GetByAddress(pCtx context.Context, pAddress persist.Address) (persist.Contract, error) {
	return c.ReadRepo.GetByAddress(pCtx, pAddress)
}

// UpsertByAddress is an upsert for a contract by address
func (c *ContractRepository) UpsertByAddress(pCtx context.Context, pAddress persist.Address, pContract persist.Contract) error {
	errChan := make(chan error)
	for _, repo := range c.WriteRepos {
		go func(repo persist.ContractRepository) {
			errChan <- repo.UpsertByAddress(pCtx, pAddress, pContract)
		}(repo)
	}
	go func() {
		errChan <- c.ReadRepo.UpsertByAddress(pCtx, pAddress, pContract)
	}()
	for i := 0; i < len(c.WriteRepos)+1; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil
}

// BulkUpsert is a bulk upsert for contracts
func (c *ContractRepository) BulkUpsert(pCtx context.Context, pContracts []persist.Contract) error {
	errChan := make(chan error)
	for _, repo := range c.WriteRepos {
		go func(repo persist.ContractRepository) {
			errChan <- repo.BulkUpsert(pCtx, pContracts)
		}(repo)
	}
	go func() {
		errChan <- c.ReadRepo.BulkUpsert(pCtx, pContracts)
	}()
	for i := 0; i < len(c.WriteRepos)+1; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil
}
