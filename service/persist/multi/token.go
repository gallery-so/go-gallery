package multi

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

// TokenRepository is the repository for interacting with tokens from multiple different other repositories
type TokenRepository struct {
	ReadRepo   persist.TokenRepository
	WriteRepos []persist.TokenRepository
}

// NewTokenRepository returns a new TokenRepository
func NewTokenRepository(readRepo persist.TokenRepository, writeRepos ...persist.TokenRepository) *TokenRepository {
	return &TokenRepository{ReadRepo: readRepo, WriteRepos: writeRepos}
}

// CreateBulk creates a many tokens
func (t *TokenRepository) CreateBulk(pCtx context.Context, pTokens []persist.Token) ([]persist.DBID, error) {
	errChan := make(chan error)
	for _, repo := range t.WriteRepos {
		go func(repo persist.TokenRepository) {
			_, err := repo.CreateBulk(pCtx, pTokens)
			errChan <- err
		}(repo)
	}
	for range t.WriteRepos {
		if err := <-errChan; err != nil {
			return nil, err
		}
	}
	return t.ReadRepo.CreateBulk(pCtx, pTokens)
}

// Create creates a new token
func (t *TokenRepository) Create(pCtx context.Context, pToken persist.Token) (persist.DBID, error) {
	errChan := make(chan error)
	for _, repo := range t.WriteRepos {
		go func(repo persist.TokenRepository) {
			_, err := repo.Create(pCtx, pToken)
			errChan <- err
		}(repo)
	}
	for range t.WriteRepos {
		if err := <-errChan; err != nil {
			return "", err
		}
	}
	return t.ReadRepo.Create(pCtx, pToken)
}

// GetByWallet returns all tokens associated with a wallet
func (t *TokenRepository) GetByWallet(pCtx context.Context, pAddress persist.Address, limit int64, page int64) ([]persist.Token, error) {
	return t.ReadRepo.GetByWallet(pCtx, pAddress, limit, page)
}

// GetByUserID returns all tokens associated with a user
func (t *TokenRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, limit int64, offset int64) ([]persist.Token, error) {
	return t.ReadRepo.GetByUserID(pCtx, pUserID, limit, offset)
}

// GetByContract returns all tokens associated with a contract
func (t *TokenRepository) GetByContract(pCtx context.Context, pContractAddress persist.Address, limit int64, offset int64) ([]persist.Token, error) {
	return t.ReadRepo.GetByContract(pCtx, pContractAddress, limit, offset)
}

// GetByTokenIdentifiers returns all tokens associated with a token identifier
func (t *TokenRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, limit int64, offset int64) ([]persist.Token, error) {
	return t.ReadRepo.GetByTokenIdentifiers(pCtx, pTokenID, pContractAddress, limit, offset)
}

// GetByID returns a token by its ID
func (t *TokenRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.Token, error) {
	return t.ReadRepo.GetByID(pCtx, pID)
}

// BulkUpsert creates a many tokens
func (t *TokenRepository) BulkUpsert(pCtx context.Context, pTokens []persist.Token) error {
	errChan := make(chan error)
	for _, repo := range t.WriteRepos {
		go func(repo persist.TokenRepository) {
			errChan <- repo.BulkUpsert(pCtx, pTokens)
		}(repo)
	}
	go func() {
		errChan <- t.ReadRepo.BulkUpsert(pCtx, pTokens)
	}()
	for i := 0; i < len(t.WriteRepos)+1; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil
}

// Upsert creates or updates a token
func (t *TokenRepository) Upsert(pCtx context.Context, pToken persist.Token) error {
	errChan := make(chan error)
	for _, repo := range t.WriteRepos {
		go func(repo persist.TokenRepository) {
			errChan <- repo.Upsert(pCtx, pToken)
		}(repo)
	}
	go func() {
		errChan <- t.ReadRepo.Upsert(pCtx, pToken)
	}()
	for i := 0; i < len(t.WriteRepos)+1; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil

}

// UpdateByIDUnsafe updates a token by its ID
func (t *TokenRepository) UpdateByIDUnsafe(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {
	errChan := make(chan error)
	for _, repo := range t.WriteRepos {
		go func(repo persist.TokenRepository) {
			errChan <- repo.UpdateByIDUnsafe(pCtx, pID, pUpdate)
		}(repo)
	}
	go func() {
		errChan <- t.ReadRepo.UpdateByIDUnsafe(pCtx, pID, pUpdate)
	}()
	for i := 0; i < len(t.WriteRepos)+1; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil
}

// UpdateByID updates a token by its ID
func (t *TokenRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	errChan := make(chan error)
	for _, repo := range t.WriteRepos {
		go func(repo persist.TokenRepository) {
			errChan <- repo.UpdateByID(pCtx, pID, pUserID, pUpdate)
		}(repo)
	}
	go func() {
		errChan <- t.ReadRepo.UpdateByID(pCtx, pID, pUserID, pUpdate)
	}()
	for i := 0; i < len(t.WriteRepos)+1; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil
}

// UpdateByTokenIdentifiersUnsafe updates a token by its token identifiers
func (t *TokenRepository) UpdateByTokenIdentifiersUnsafe(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, pUpdate interface{}) error {
	errChan := make(chan error)
	for _, repo := range t.WriteRepos {
		go func(repo persist.TokenRepository) {
			errChan <- repo.UpdateByTokenIdentifiersUnsafe(pCtx, pTokenID, pContractAddress, pUpdate)
		}(repo)
	}
	go func() {
		errChan <- t.ReadRepo.UpdateByTokenIdentifiersUnsafe(pCtx, pTokenID, pContractAddress, pUpdate)
	}()
	for i := 0; i < len(t.WriteRepos)+1; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil
}

// MostRecentBlock returns the most recent block
func (t *TokenRepository) MostRecentBlock(pCtx context.Context) (persist.BlockNumber, error) {
	return t.ReadRepo.MostRecentBlock(pCtx)
}

// Count returns the number of tokens
func (t *TokenRepository) Count(pCtx context.Context, pTokenCountType persist.TokenCountType) (int64, error) {
	return t.ReadRepo.Count(pCtx, pTokenCountType)
}
