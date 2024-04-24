package multichain

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/platform"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	op "github.com/mikeydub/go-gallery/service/multichain/operation"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/mikeydub/go-gallery/validate"
)

func init() {
	env.RegisterValidation("TOKEN_PROCESSING_URL", "required")
}

var unknownContractNames = map[string]bool{
	"unidentified contract": true,
	"unknown contract":      true,
	"unknown":               true,
}

const maxCommunitySize = 1000

type Provider struct {
	Repos     *postgres.Repositories
	Queries   *db.Queries
	Chains    ProviderLookup
	Submitter tokenmanage.Submitter
}

// ChainAgnosticToken is a token that is agnostic to the chain it is on
type ChainAgnosticToken struct {
	Descriptors     ChainAgnosticTokenDescriptors `json:"descriptors"`
	TokenType       persist.TokenType             `json:"token_type"`
	TokenURI        persist.TokenURI              `json:"token_uri"`
	TokenID         persist.HexTokenID            `json:"token_id"`
	Quantity        persist.HexString             `json:"quantity"`
	OwnerAddress    persist.Address               `json:"owner_address"`
	TokenMetadata   persist.TokenMetadata         `json:"metadata"`
	ContractAddress persist.Address               `json:"contract_address"`
	FallbackMedia   persist.FallbackMedia         `json:"fallback_media"`
	ExternalURL     string                        `json:"external_url"`
	BlockNumber     persist.BlockNumber           `json:"block_number"`
	IsSpam          *bool                         `json:"is_spam"`
}

// ChainAgnosticContract is a contract that is agnostic to the chain it is on
type ChainAgnosticContract struct {
	Descriptors ChainAgnosticContractDescriptors `json:"descriptors"`
	Address     persist.Address                  `json:"address"`
	IsSpam      *bool                            `json:"is_spam"`
	LatestBlock persist.BlockNumber              `json:"latest_block"`
}

type ChainAgnosticTokensAndContracts struct {
	Tokens    []ChainAgnosticToken    `json:"tokens"`
	Contracts []ChainAgnosticContract `json:"contracts"`
}

// ChainAgnosticTokenDescriptors are the fields that describe a token but cannot be used to uniquely identify it
type ChainAgnosticTokenDescriptors struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ChainAgnosticContractDescriptors struct {
	Symbol          string          `json:"symbol"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	ProfileImageURL string          `json:"profile_image_url"`
	OwnerAddress    persist.Address `json:"owner_address"`
}

// ChainAgnosticIdentifiers identify tokens despite their chain
type ChainAgnosticIdentifiers struct {
	ContractAddress persist.Address    `json:"contract_address"`
	TokenID         persist.HexTokenID `json:"token_id"`
}

func (t ChainAgnosticIdentifiers) String() string {
	return fmt.Sprintf("token(address=%s, tokenID=%s)", t.ContractAddress, t.TokenID.ToDecimalTokenID())
}

type ErrProviderFailed struct{ Err error }

func (e ErrProviderFailed) Unwrap() error { return e.Err }
func (e ErrProviderFailed) Error() string { return fmt.Sprintf("calling provider failed: %s", e.Err) }

type ErrProviderContractNotFound struct {
	Contract persist.Address
	Chain    persist.Chain
	Err      error
}

func (e ErrProviderContractNotFound) Unwrap() error { return e.Err }
func (e ErrProviderContractNotFound) Error() string {
	return fmt.Sprintf("provider did not find contract: %s", e.Contract.String())
}

type communityInfo interface {
	GetKey() persist.CommunityKey
	GetName() string
	GetDescription() string
	GetProfileImageURL() string
	GetCreatorAddresses() []persist.ChainAddress
	GetWebsiteURL() string
}

// Verifier can verify that a signature is signed by a given key
type Verifier interface {
	VerifySignature(ctx context.Context, pubKey persist.PubKey, walletType persist.WalletType, nonce string, sig string) (bool, error)
}

type TokenIdentifierOwnerFetcher interface {
	GetTokenByTokenIdentifiersAndOwner(context.Context, ChainAgnosticIdentifiers, persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error)
}

type TokensByContractWalletFetcher interface {
	GetTokensByContractWallet(ctx context.Context, contract persist.ChainAddress, wallet persist.Address) ([]ChainAgnosticToken, ChainAgnosticContract, error)
}

type TokensByTokenIdentifiersFetcher interface {
	GetTokensByTokenIdentifiers(context.Context, ChainAgnosticIdentifiers) ([]ChainAgnosticToken, ChainAgnosticContract, error)
}

// TokensIncrementalOwnerFetcher supports fetching tokens for syncing incrementally
type TokensIncrementalOwnerFetcher interface {
	// NOTE: implementation MUST close the rec channel
	GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan ChainAgnosticTokensAndContracts, <-chan error)
}

// TokensIncrementalContractFetcher supports fetching tokens by contract for syncing incrementally
type TokensIncrementalContractFetcher interface {
	// NOTE: implementations MUST close the rec channel
	// maxLimit is not for pagination, it is to make sure we don't fetch a bajilion tokens from an omnibus contract
	GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan ChainAgnosticTokensAndContracts, <-chan error)
}

type ContractFetcher interface {
	GetContractByAddress(ctx context.Context, contract persist.Address) (ChainAgnosticContract, error)
}

type ContractsCreatorFetcher interface {
	GetContractsByCreatorAddress(ctx context.Context, owner persist.Address) ([]ChainAgnosticContract, error)
}

// TokenMetadataFetcher supports fetching token metadata
type TokenMetadataFetcher interface {
	GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti ChainAgnosticIdentifiers) (persist.TokenMetadata, error)
}

type TokenMetadataBatcher interface {
	GetTokenMetadataByTokenIdentifiersBatch(ctx context.Context, tIDs []ChainAgnosticIdentifiers) ([]persist.TokenMetadata, error)
}

type TokenDescriptorsFetcher interface {
	GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti ChainAgnosticIdentifiers) (ChainAgnosticTokenDescriptors, ChainAgnosticContractDescriptors, error)
}

type chainTokensAndContracts struct {
	Chain     persist.Chain
	Tokens    []ChainAgnosticToken
	Contracts []ChainAgnosticContract
}

// SyncTokensByUserID updates the media for all tokens for a user
func (p *Provider) SyncTokensByUserID(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"user_id": userID, "chains": chains})

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	chainsToAddresses := p.matchingWallets(user.Wallets, chains)
	if len(chainsToAddresses) == 0 {
		return nil
	}

	recCh := make(chan chainTokensAndContracts, len(chains)*len(chainsToAddresses)*8)
	errCh := make(chan error)
	wg := &conc.WaitGroup{}

	for c, a := range chainsToAddresses {
		fetcher, ok := p.Chains[c].(TokensIncrementalOwnerFetcher)
		if !ok {
			continue
		}
		for _, addr := range a {
			addr := addr
			chain := c
			wg.Go(func() {
				logger.For(ctx).Infof("syncing chain=%d; user=%s; wallet=%s", chain, user.Username.String(), addr)
				pageCh, pageErrCh := fetcher.GetTokensIncrementallyByWalletAddress(ctx, addr)
				for {
					select {
					case page, ok := <-pageCh:
						if !ok {
							return
						}
						recCh <- chainTokensAndContracts{
							Chain:     chain,
							Tokens:    page.Tokens,
							Contracts: page.Contracts,
						}
					case err, ok := <-pageErrCh:
						if !ok {
							return
						}
						errCh <- ErrProviderFailed{Err: err}
						return
					}
				}
			})
		}
	}

	go func() {
		defer close(recCh)
		defer close(errCh)
		wg.Wait()
	}()

	_, _, err = p.addHolderTokensForUser(ctx, user, chains, recCh, errCh)
	return err
}

// SyncCreatedTokensForNewContracts syncs tokens for contracts that the user created but does not currently have any tokens for.
func (p *Provider) SyncCreatedTokensForNewContracts(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"user_id": userID, "chains": chains})

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	chainsToAddresses := p.matchingWallets(user.Wallets, chains)
	if len(chainsToAddresses) == 0 {
		return nil
	}

	recCh := make(chan chainTokensAndContracts, len(chains)*len(chainsToAddresses)*8)
	errCh := make(chan error)

	wg := &conc.WaitGroup{}
	for c, a := range chainsToAddresses {
		contractFetcher, contractOK := p.Chains[c].(ContractsCreatorFetcher)
		tokenFetcher, tokenOK := p.Chains[c].(TokensIncrementalContractFetcher)

		if !contractOK || !tokenOK {
			continue
		}

		for _, addr := range a {
			addr := addr
			chain := c

			wg.Go(func() {
				innerWg := &conc.WaitGroup{}

				contracts, err := contractFetcher.GetContractsByCreatorAddress(ctx, addr)
				if err != nil {
					errCh <- ErrProviderFailed{Err: err}
					return
				}

				for _, contract := range contracts {
					c := contract
					innerWg.Go(func() {
						logger.For(ctx).Infof("syncing chain=%d; user=%s; contract=%s", chain, user.Username.String(), c.Address.String())
						pageCh, pageErrCh := tokenFetcher.GetTokensIncrementallyByContractAddress(ctx, c.Address, maxCommunitySize)
						for {
							select {
							case page, ok := <-pageCh:
								if !ok {
									return
								}
								recCh <- chainTokensAndContracts{
									Chain:     chain,
									Tokens:    page.Tokens,
									Contracts: page.Contracts,
								}
							case err, ok := <-pageErrCh:
								if !ok {
									return
								}
								errCh <- ErrProviderFailed{Err: err}
								return
							}
						}
					})
				}
				innerWg.Wait()
			})
		}
	}

	go func() {
		defer close(recCh)
		defer close(errCh)
		wg.Wait()
	}()

	_, _, err = p.addCreatorTokensForUser(ctx, user, chains, recCh, errCh)
	if err != nil {
		return err
	}

	// Remove creator status from any tokens this user is no longer the creator of
	return p.Queries.RemoveStaleCreatorStatusFromTokens(ctx, userID)
}

// AddTokensToUserUnchecked adds tokens to a user with the requested quantities. AddTokensToUserUnchecked does not make any effort to validate
// that the user owns the tokens, only that the tokens exist and are fetchable on chain. This is useful for adding tokens to a user when it's
// already known beforehand that the user owns the token via a trusted source, skipping the potentially expensive operation of fetching a token by its owner.
func (p *Provider) AddTokensToUserUnchecked(ctx context.Context, userID persist.DBID, tIDs []persist.TokenUniqueIdentifiers, newQuantities []persist.HexString) ([]op.TokenFullDetails, error) {
	// Validate
	err := validate.Validate(validate.ValidationMap{
		"userID":        validate.WithTag(userID, "required"),
		"tokensToAdd":   validate.WithTag(tIDs, "required,gt=0,unique"),
		"newQuantities": validate.WithTag(newQuantities, fmt.Sprintf("len=%d,dive,gt=0", len(tIDs))),
	})
	if err != nil {
		return nil, err
	}

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Group tokens by chain
	chainPages := make(map[persist.Chain]chainTokensAndContracts)

outer:
	for _, t := range tIDs {
		// Validate that the chain is supported
		_, ok := p.Chains[t.Chain].(TokensByTokenIdentifiersFetcher)
		if !ok {
			err = fmt.Errorf("multichain is not configured to fetch unchecked tokens for chain=%d", t.Chain)
			logger.For(ctx).Error(err)
			return nil, err
		}
		// Validate that the requested owner address is a wallet owned by the user
		for _, w := range user.Wallets {
			if (w.Address == t.OwnerAddress) && (w.Chain.L1Chain() == t.Chain.L1Chain()) {
				if _, ok := chainPages[t.Chain]; !ok {
					chainPages[t.Chain] = chainTokensAndContracts{
						Chain:     t.Chain,
						Tokens:    make([]ChainAgnosticToken, 0),
						Contracts: make([]ChainAgnosticContract, 0),
					}
				}
				continue outer
			}
		}
		// Return an error if the requested owner address is not owned by the user
		err := fmt.Errorf("token(chain=%d, contract=%s; tokenID=%s) requested owner address=%s, but address is not owned by user", t.Chain, t.ContractAddress, t.TokenID, t.OwnerAddress)
		logger.For(ctx).Error(err)
		return nil, err
	}

	for i, t := range tIDs {
		tokenID := ChainAgnosticIdentifiers{ContractAddress: t.ContractAddress, TokenID: t.TokenID}

		tokens, contract, err := p.Chains[t.Chain].(TokensByTokenIdentifiersFetcher).GetTokensByTokenIdentifiers(ctx, tokenID)
		// Exit early if a token in the batch is not found
		if err != nil {
			err := fmt.Errorf("failed to fetch token(chain=%d, contract=%s, tokenID=%s): %s", t.Chain, t.ContractAddress, t.TokenID, err)
			logger.For(ctx).Error(err)
			return nil, err
		}

		if len(tokens) == 0 {
			err := fmt.Errorf("failed to fetch token(chain=%d, contract=%s, tokenID=%s)", t.Chain, t.ContractAddress, t.TokenID)
			logger.For(ctx).Error(err)
			return nil, err
		}

		// Handle overrides
		token := tokens[0]
		token.OwnerAddress = tIDs[i].OwnerAddress       // Override the owner with the requested owner
		token.Quantity = newQuantities[i]               // Override the quantity
		if token.TokenType == persist.TokenTypeERC721 { // Ignore the requested quantity for ERC721s
			token.Quantity = persist.MustHexString("1")
		}

		chainPage := chainPages[t.Chain]
		chainPage.Tokens = append(chainPage.Tokens, token)
		chainPage.Contracts = append(chainPage.Contracts, contract)
		chainPages[t.Chain] = chainPage
	}

	// Add tokens to the user
	recCh := make(chan chainTokensAndContracts, len(chainPages))
	errCh := make(chan error)
	go func() {
		defer close(recCh)
		defer close(errCh)
		for c, page := range chainPages {
			logger.For(ctx).Infof("adding %d unchecked token(s) to chain=%d for user=%s", len(page.Tokens), c, userID)
			recCh <- page
		}
	}()

	newTokens, _, err := p.addHolderTokensForUser(ctx, user, util.MapKeys(chainPages), recCh, errCh)
	return newTokens, err
}

// SyncTokensByUserIDAndTokenIdentifiers updates the media for specific tokens for a user
func (p *Provider) SyncTokensByUserIDAndTokenIdentifiers(ctx context.Context, userID persist.DBID, tokenIdentifiers []persist.TokenUniqueIdentifiers) ([]op.TokenFullDetails, error) {
	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	chains, _ := util.Map(tokenIdentifiers, func(i persist.TokenUniqueIdentifiers) (persist.Chain, error) { return i.Chain, nil })
	chains = util.Dedupe(chains, false)

	matchingWallets := p.matchingWallets(user.Wallets, chains)

	chainAddresses := map[persist.ChainAddress]bool{}
	for chain, addresses := range matchingWallets {
		for _, address := range addresses {
			chainAddresses[persist.NewChainAddress(address, chain)] = true
		}
	}

	chainsToTokenIdentifiers := make(map[persist.Chain][]persist.TokenUniqueIdentifiers)
	for _, tid := range tokenIdentifiers {
		// check if the user owns the wallet that owns the token
		if !chainAddresses[persist.NewChainAddress(tid.OwnerAddress, tid.Chain)] {
			continue
		}
		chainsToTokenIdentifiers[tid.Chain] = append(chainsToTokenIdentifiers[tid.Chain], tid)
	}

	for c, a := range chainsToTokenIdentifiers {
		chainsToTokenIdentifiers[c] = util.Dedupe(a, false)
	}

	recCh := make(chan chainTokensAndContracts, len(tokenIdentifiers))
	errCh := make(chan error)

	wg := &conc.WaitGroup{}

	for c, t := range chainsToTokenIdentifiers {
		chain := c
		tids := t

		fetcher, ok := p.Chains[chain].(TokenIdentifierOwnerFetcher)
		if !ok {
			continue
		}

		for _, tid := range tids {
			tid := tid
			wg.Go(func() {
				logger.For(ctx).Infof("syncing chain=%d; user=%s; token=%s", chain, user.Username.String(), tid)
				id := ChainAgnosticIdentifiers{ContractAddress: tid.ContractAddress, TokenID: tid.TokenID}
				token, contract, err := fetcher.GetTokenByTokenIdentifiersAndOwner(ctx, id, tid.OwnerAddress)
				if err != nil {
					errCh <- ErrProviderFailed{Err: err}
					return
				}
				recCh <- chainTokensAndContracts{
					Chain:     chain,
					Tokens:    []ChainAgnosticToken{token},
					Contracts: []ChainAgnosticContract{contract},
				}
			})
		}
	}

	go func() {
		defer close(recCh)
		defer close(errCh)
		wg.Wait()
	}()

	newTokens, _, err := p.addHolderTokensForUser(ctx, user, chains, recCh, errCh)
	return newTokens, err
}

// TokenExists checks if a token exists according to any provider by its identifiers. It returns nil if the token exists.
// If a token exists, it will also update its contract and its descriptors in the database.
func (p *Provider) TokenExists(ctx context.Context, token persist.TokenIdentifiers, r retry.Retry) (td db.TokenDefinition, err error) {
	searchF := func(ctx context.Context) error {
		td, err = p.RefreshTokenDescriptorsByTokenIdentifiers(ctx, persist.TokenIdentifiers{
			TokenID:         token.TokenID,
			Chain:           token.Chain,
			ContractAddress: token.ContractAddress,
		})
		return err
	}

	logRetryF := func(err error) bool {
		logger.For(ctx).Errorf("polling for token: %s: retrying on error: %s", token.String(), err.Error())
		return true
	}

	retry.RetryFunc(ctx, searchF, logRetryF, r)

	return td, err
}

// SyncTokenByUserWalletsAndTokenIdentifiersRetry attempts to sync a token for a user by their wallets and token identifiers.
func (p *Provider) SyncTokenByUserWalletsAndTokenIdentifiersRetry(ctx context.Context, userID persist.DBID, t persist.TokenIdentifiers, r retry.Retry) (token op.TokenFullDetails, err error) {
	user, err := p.Queries.GetUserById(ctx, userID)
	if err != nil {
		return op.TokenFullDetails{}, err
	}

	searchF := func(ctx context.Context) error {
		_, err := p.Queries.GetTokenByUserTokenIdentifiers(ctx, db.GetTokenByUserTokenIdentifiersParams{
			OwnerID:         user.ID,
			TokenID:         t.TokenID,
			ContractAddress: t.ContractAddress,
			Chain:           t.Chain,
		})
		// Token alrady exists, do nothing
		if err == nil {
			return nil
		}
		// Unexpected error
		if err != nil && !util.ErrorIs[persist.ErrTokenNotFoundByUserTokenIdentifers](err) {
			return err
		}
		wg := sync.WaitGroup{}
		for _, w := range p.matchingWalletsChain(user.Wallets, t.Chain) {
			w := w
			searchWallet := persist.TokenUniqueIdentifiers{
				Chain:           t.Chain,
				ContractAddress: t.ContractAddress,
				TokenID:         t.TokenID,
				OwnerAddress:    w,
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				p.SyncTokensByUserIDAndTokenIdentifiers(ctx, user.ID, []persist.TokenUniqueIdentifiers{searchWallet})
			}()
		}
		wg.Wait()
		token, err = op.TokenFullDetailsByUserTokenIdentifiers(ctx, p.Queries, user.ID, t)
		return err
	}

	logRetryF := func(err error) bool {
		logger.For(ctx).Errorf("polling for token for user=%s: polling for token=%s: retrying on error: %s", user.ID, t.String(), err.Error())
		return true
	}

	err = retry.RetryFunc(ctx, searchF, logRetryF, r)

	return token, err
}

func (p *Provider) SyncCreatedTokensForExistingContract(ctx context.Context, userID persist.DBID, contractID persist.DBID) error {
	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	contract, err := p.Queries.GetContractByID(ctx, contractID)
	if err != nil {
		return err
	}

	f, ok := p.Chains[contract.Chain].(TokensIncrementalContractFetcher)
	if !ok {
		return fmt.Errorf("no tokens contract fetcher for chain: %d", contract.Chain)
	}

	recCh := make(chan chainTokensAndContracts, 8)
	errCh := make(chan error)

	go func() {
		defer close(recCh)
		defer close(errCh)
		pageCh, pageErrCh := f.GetTokensIncrementallyByContractAddress(ctx, contract.Address, maxCommunitySize)
		for {
			select {
			case page, ok := <-pageCh:
				if !ok {
					return
				}
				recCh <- chainTokensAndContracts{
					Chain:     contract.Chain,
					Tokens:    page.Tokens,
					Contracts: page.Contracts,
				}
			case err, ok := <-pageErrCh:
				if !ok {
					continue
				}
				errCh <- ErrProviderFailed{Err: err}
				return
			}
		}
	}()

	_, _, err = p.addCreatorTokensForUser(ctx, user, []persist.Chain{contract.Chain}, recCh, errCh)
	return err
}

func (p *Provider) processCommunities(ctx context.Context, contracts []db.Contract, tokens []op.TokenFullDetails) error {
	knownProviders, err := p.Queries.GetCommunityContractProviders(ctx, util.MapWithoutError(contracts, func(c db.Contract) persist.DBID { return c.ID }))
	if err != nil {
		return fmt.Errorf("failed to retrieve contract community types: %w", err)
	}

	// TODO: Make this more flexible, allow other providers, etc (possibly via wire)
	return p.processArtBlocksCommunityTokens(ctx, knownProviders, tokens)
}

func (p *Provider) processTokensForUsers(ctx context.Context, chain persist.Chain, users map[persist.DBID]persist.User, chainTokensForUsers map[persist.DBID][]ChainAgnosticToken, contracts []db.Contract, upsertParams op.TokenUpsertParams) (newUserTokens map[persist.DBID][]op.TokenFullDetails, err error) {
	definitionsToAdd := make([]db.TokenDefinition, 0)
	tokensToAdd := make([]op.UpsertToken, 0)

	for userID, user := range users {
		tokens := chainTokensToUpsertableTokens(chain, chainTokensForUsers[userID], contracts, user)
		definitions := chainTokensToUpsertableTokenDefinitions(ctx, chain, chainTokensForUsers[userID], contracts)
		tokensToAdd = append(tokensToAdd, tokens...)
		definitionsToAdd = append(definitionsToAdd, definitions...)
	}

	// Insert token definitions
	definitionsToAdd = dedupeTokenDefinitions(definitionsToAdd)
	addedDefinitions, isNewDefinitions, err := op.InsertTokenDefinitions(ctx, p.Queries, definitionsToAdd)
	if err != nil {
		logger.For(ctx).Errorf("error in bulk upsert of token definitions: %s", err)
		return nil, err
	}

	tokenToDefinitionID := map[persist.TokenIdentifiers]persist.DBID{}
	membershipsToAdd := make([]db.TokenCommunityMembership, len(addedDefinitions))
	membershipContractIDs := make([]persist.DBID, len(addedDefinitions))
	definitionsToSendToTokenProcessing := make([]persist.DBID, 0, len(addedDefinitions))

	for i, t := range addedDefinitions {
		membershipsToAdd[i] = db.TokenCommunityMembership{
			TokenDefinitionID: t.ID,
			TokenID:           t.TokenID.ToDecimalTokenID(),
		}
		membershipContractIDs[i] = t.ContractID

		tID := persist.TokenIdentifiers{
			TokenID:         t.TokenID,
			ContractAddress: t.ContractAddress,
			Chain:           t.Chain,
		}
		tokenToDefinitionID[tID] = t.ID

		// Compare when the token was last updated to when it was created to determine if a token is new
		// Add a fudge factor of a second to account for difference in clock times
		if isNewDefinitions[i] {
			logger.For(ctx).Infof("%s is new (dbid=%s); adding to tokenprocessing batch", tID, t.ID)
			definitionsToSendToTokenProcessing = append(definitionsToSendToTokenProcessing, addedDefinitions[i].ID)
		} else {
			logger.For(ctx).Infof("%s was already in db (dbid=%s); not adding to tokenprocessing batch", tID, t.ID)
		}
	}

	// Send definitions to tokenprocessing
	if err := p.Submitter.SubmitNewTokens(ctx, definitionsToSendToTokenProcessing); err != nil {
		logger.For(ctx).Errorf("failed to submit batch: %s", err)
		sentryutil.ReportError(ctx, err)
	}

	// Insert token memberships
	_, err = op.InsertTokenCommunityMemberships(ctx, p.Queries, membershipsToAdd, membershipContractIDs)
	if err != nil {
		logger.For(ctx).Errorf("error in bulk upsert of token communities: %s", err)
		return nil, err
	}

	// Insert tokens
	tokensToAdd = dedupeTokenInstances(tokensToAdd)
	for i := range tokensToAdd {
		tokensToAdd[i].Token.TokenDefinitionID = tokenToDefinitionID[tokensToAdd[i].Identifiers]
	}

	upsertTime, addedTokens, err := op.InsertTokens(ctx, p.Queries, tokensToAdd, upsertParams)
	if err != nil {
		logger.For(ctx).Errorf("error in bulk upsert of tokens: %s", err)
		return nil, err
	}

	// TODO: Consider tracking (token_definition_id, community_type) in a table so we'd know whether we've already
	// evaluated a token for a given community type and can avoid checking it again.
	communityTokens := addedTokens
	err = p.processCommunities(ctx, contracts, communityTokens)
	if err != nil {
		// Report errors, but don't return. We can retry token community memberships at some point, but the whole
		// sync shouldn't fail because a community provider's API was unavailable.
		err = fmt.Errorf("failed to process token communities: %w", err)
		logger.For(ctx).WithError(err).Error(err)
		sentryutil.ReportError(ctx, err)
	}

	if upsertParams.OptionalDelete != nil {
		numAffectedRows, err := p.Queries.DeleteTokensBeforeTimestamp(ctx, upsertParams.OptionalDelete.ToParams(upsertTime))
		if err != nil {
			return nil, fmt.Errorf("failed to delete tokens: %w", err)
		}
		logger.For(ctx).Infof("deleted %d tokens", numAffectedRows)
	}

	newUserTokens = make(map[persist.DBID][]op.TokenFullDetails, len(users))
	for _, t := range addedTokens {
		newUserTokens[t.Instance.OwnerUserID] = append(newUserTokens[t.Instance.OwnerUserID], t)
	}

	return newUserTokens, err
}

type addTokensFunc func(context.Context, persist.User, persist.Chain, []ChainAgnosticToken, []db.Contract) (newTokens []op.TokenFullDetails, err error)

func (p *Provider) addCreatorTokensOfContractsToUser(ctx context.Context, user persist.User, chain persist.Chain, tokens []ChainAgnosticToken, contracts []db.Contract) (newTokens []op.TokenFullDetails, err error) {
	return p.processTokensForUser(ctx, user, chain, tokens, contracts, op.TokenUpsertParams{
		SetCreatorFields: true,
		SetHolderFields:  false,
		OptionalDelete:   nil,
	})
}

func (p *Provider) addHolderTokensToUser(ctx context.Context, user persist.User, chain persist.Chain, tokens []ChainAgnosticToken, contracts []db.Contract) (newTokens []op.TokenFullDetails, err error) {
	return p.processTokensForUser(ctx, user, chain, tokens, contracts, op.TokenUpsertParams{
		SetCreatorFields: false,
		SetHolderFields:  true,
		OptionalDelete:   nil,
	})
}

func (p *Provider) processTokensForUser(ctx context.Context, user persist.User, chain persist.Chain, tokens []ChainAgnosticToken, contracts []db.Contract, upsertParams op.TokenUpsertParams) (newTokens []op.TokenFullDetails, error error) {
	userMap := map[persist.DBID]persist.User{user.ID: user}
	providerTokenMap := map[persist.DBID][]ChainAgnosticToken{user.ID: tokens}
	newUserTokens, err := p.processTokensForUsers(ctx, chain, userMap, providerTokenMap, contracts, upsertParams)
	if err != nil {
		return nil, err
	}
	return newUserTokens[user.ID], nil
}

func (p *Provider) receiveProviderData(ctx context.Context, user persist.User, recCh <-chan chainTokensAndContracts, errCh <-chan error, addTokensF addTokensFunc) ([]op.TokenFullDetails, []db.Contract, error) {
	var newTokens []op.TokenFullDetails
	var currentContracts []db.Contract
	var err error
	for {
		select {
		case page, ok := <-recCh:
			if !ok {
				return newTokens, currentContracts, nil
			}

			contracts, err := p.processContracts(ctx, page.Chain, page.Contracts, false)
			if err != nil {
				return nil, nil, err
			}

			addedTokens, err := addTokensF(ctx, user, page.Chain, page.Tokens, contracts)
			if err != nil {
				return nil, nil, err
			}

			newTokens = append(newTokens, addedTokens...)
		case <-ctx.Done():
			err = ctx.Err()
			return nil, nil, err
		case err, ok := <-errCh:
			if ok {
				return nil, nil, err
			}
		}
	}
}

func (p *Provider) addHolderTokensForUser(ctx context.Context, user persist.User, chains []persist.Chain, recCh <-chan chainTokensAndContracts, errCh <-chan error) (
	newTokens []op.TokenFullDetails,
	currentContracts []db.Contract,
	err error,
) {
	return p.receiveProviderData(ctx, user, recCh, errCh, p.addHolderTokensToUser)
}

func (p *Provider) addCreatorTokensForUser(ctx context.Context, user persist.User, chains []persist.Chain, recCh <-chan chainTokensAndContracts, errCh <-chan error) (
	newTokens []op.TokenFullDetails,
	currentContracts []db.Contract,
	err error,
) {
	return p.receiveProviderData(ctx, user, recCh, errCh, p.addCreatorTokensOfContractsToUser)
}

// replaceCreatorTokensForUser adds new creator tokens to a user and deletes old creator tokens. If onlyForContractIDs is not empty,
// only tokens for the specified contracts will be deleted.
func (p *Provider) replaceCreatorTokensForUser(ctx context.Context, user persist.User, onlyForContractIDs []persist.DBID, chains []persist.Chain, recCh <-chan chainTokensAndContracts, errCh <-chan error) (
	newTokens []op.TokenFullDetails,
	currentContracts []db.Contract,
	err error,
) {
	now := time.Now()
	newTokens, currentContracts, err = p.receiveProviderData(ctx, user, recCh, errCh, p.addCreatorTokensOfContractsToUser)
	if err != nil {
		return
	}

	var contractIDs []string
	if len(onlyForContractIDs) > 0 {
		contractIDs = util.MapWithoutError(onlyForContractIDs, func(id persist.DBID) string { return id.String() })
	} else {
		contractIDs = util.MapWithoutError(currentContracts, func(c db.Contract) string { return c.ID.String() })
	}

	_, err = p.Queries.DeleteTokensBeforeTimestamp(ctx, db.DeleteTokensBeforeTimestampParams{
		RemoveHolderStatus:  false,
		RemoveCreatorStatus: true,
		OnlyFromUserID:      util.ToNullString(user.ID.String(), true),
		OnlyFromContractIds: contractIDs,
		OnlyFromChains:      util.MapWithoutError(chains, func(c persist.Chain) int32 { return int32(c) }),
		Timestamp:           now,
	})
	if err != nil {
		return
	}
	return
}

func (p *Provider) replaceHolderTokensForUser(ctx context.Context, user persist.User, chains []persist.Chain, recCh <-chan chainTokensAndContracts, errCh <-chan error) (
	currentTokens []op.TokenFullDetails,
	newTokens []op.TokenFullDetails,
	currentContracts []db.Contract,
	err error,
) {
	now := time.Now()
	newTokens, currentContracts, err = p.receiveProviderData(ctx, user, recCh, errCh, p.addHolderTokensToUser)
	if err != nil {
		return
	}
	_, err = p.Queries.DeleteTokensBeforeTimestamp(ctx, db.DeleteTokensBeforeTimestampParams{
		RemoveHolderStatus:  true,
		RemoveCreatorStatus: false,
		OnlyFromUserID:      util.ToNullString(user.ID.String(), true),
		OnlyFromContractIds: util.MapWithoutError(currentContracts, func(c db.Contract) string { return c.ID.String() }),
		OnlyFromChains:      util.MapWithoutError(chains, func(c persist.Chain) int32 { return int32(c) }),
		Timestamp:           now,
	})
	if err != nil {
		return
	}
	return
}

func (p *Provider) GetTokensOfContractForWallet(ctx context.Context, contractAddress persist.ChainAddress, wallet persist.L1ChainAddress) ([]op.TokenFullDetails, error) {
	user, err := p.Repos.UserRepository.GetByChainAddress(ctx, wallet)
	if err != nil {
		return nil, err
	}

	var walletID persist.DBID

	for _, w := range user.Wallets {
		if w.Chain.L1Chain() == wallet.L1Chain() && w.Address == wallet.Address() {
			walletID = w.ID
		}
	}

	if walletID == "" {
		return nil, fmt.Errorf("user does not own wallet with address: %s", wallet)
	}

	f, ok := p.Chains[contractAddress.Chain()].(TokensByContractWalletFetcher)
	if !ok {
		return nil, fmt.Errorf("no tokens owner fetcher for chain: %d", contractAddress.Chain())
	}

	tokens, contract, err := f.GetTokensByContractWallet(ctx, contractAddress, wallet.Address())
	if err != nil {
		return nil, err
	}

	outCh := make(chan chainTokensAndContracts)
	outErr := make(chan error)

	go func() {
		defer close(outCh)
		defer close(outErr)
		outCh <- chainTokensAndContracts{
			Chain:     contractAddress.Chain(),
			Tokens:    tokens,
			Contracts: []ChainAgnosticContract{contract},
		}
	}()

	_, _, err = p.addHolderTokensForUser(ctx, user, []persist.Chain{contractAddress.Chain()}, outCh, outErr)
	if err != nil {
		return nil, err
	}

	t, err := p.Queries.GetTokensByContractAddressUserId(ctx, db.GetTokensByContractAddressUserIdParams{
		OwnerUserID:     user.ID,
		ContractAddress: contractAddress.Address(),
		Chain:           contractAddress.Chain(),
		WalletID:        walletID.String(),
	})
	if err != nil {
		return nil, err
	}

	ownedTokens := make([]op.TokenFullDetails, len(t))
	for i := range t {
		ownedTokens[i] = op.TokenFullDetails{
			Instance:   t[i].Token,
			Contract:   t[i].Contract,
			Definition: t[i].TokenDefinition,
		}
	}

	return ownedTokens, nil
}

func (p *Provider) GetTokenMetadataByTokenIdentifiersBatch(ctx context.Context, chain persist.Chain, tIDs []ChainAgnosticIdentifiers) ([]persist.TokenMetadata, error) {
	f, ok := p.Chains[chain].(TokenMetadataBatcher)
	if !ok {
		return nil, fmt.Errorf("no metadata batchers for chain %d", chain)
	}
	return f.GetTokenMetadataByTokenIdentifiersBatch(ctx, tIDs)
}

func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, contractAddress persist.Address, tokenID persist.HexTokenID, chain persist.Chain) (persist.TokenMetadata, error) {
	fetcher, ok := p.Chains[chain].(TokenMetadataFetcher)
	if !ok {
		return nil, fmt.Errorf("no metadata fetchers for chain %d", chain)
	}
	return fetcher.GetTokenMetadataByTokenIdentifiers(ctx, ChainAgnosticIdentifiers{ContractAddress: contractAddress, TokenID: tokenID})
}

// VerifySignature verifies a signature for a wallet address
func (p *Provider) VerifySignature(ctx context.Context, pSig string, pMessage string, pChainAddress persist.ChainPubKey, pWalletType persist.WalletType) (bool, error) {
	if verifier, ok := p.Chains[pChainAddress.Chain()].(Verifier); ok {
		if valid, err := verifier.VerifySignature(ctx, pChainAddress.PubKey(), pWalletType, pMessage, pSig); err != nil || !valid {
			return false, err
		}
	}
	return true, nil
}

// RefreshToken refreshes a token on the given chain using the chain provider for that chain
func (p *Provider) RefreshToken(ctx context.Context, ti persist.TokenIdentifiers) error {
	body, err := json.Marshal(map[string]any{
		"token_id":         ti.TokenID,
		"contract_address": ti.ContractAddress,
		"chain":            ti.Chain,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/media/process/token", env.GetString("TOKEN_PROCESSING_URL"))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return util.BodyAsError(resp)
	}

	_, err = p.RefreshTokenDescriptorsByTokenIdentifiers(ctx, ti)
	return err
}

// RefreshTokenDescriptorsByTokenIdentifiers will refresh the token descriptors for a token by its identifiers.
func (p *Provider) RefreshTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti persist.TokenIdentifiers) (db.TokenDefinition, error) {
	fetcher, ok := p.Chains[ti.Chain].(TokenDescriptorsFetcher)
	if !ok {
		return db.TokenDefinition{}, fmt.Errorf("no token descriptor fetchers for chain %d", ti.Chain)
	}

	id := ChainAgnosticIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}

	var t db.TokenDefinition
	var c db.Contract

	tDesc, cDesc, err := fetcher.GetTokenDescriptorsByTokenIdentifiers(ctx, id)
	if err != nil {
		return db.TokenDefinition{}, persist.ErrTokenNotFoundByTokenIdentifiers{Token: ti}

	}

	t = mergeTokenDefinitions(t, db.TokenDefinition{
		Name:        util.ToNullString(tDesc.Name, true),
		Description: util.ToNullString(tDesc.Description, true),
	})

	c = mergeContracts(c, db.Contract{
		Name:            util.ToNullString(cDesc.Name, true),
		Description:     util.ToNullString(cDesc.Description, true),
		Symbol:          util.ToNullString(cDesc.Symbol, true),
		OwnerAddress:    cDesc.OwnerAddress,
		ProfileImageUrl: util.ToNullString(cDesc.ProfileImageURL, true),
	})

	c.Address = persist.Address(ti.Chain.NormalizeAddress(ti.ContractAddress))
	c.Chain = ti.Chain

	c, err = p.Repos.ContractRepository.Upsert(ctx, c, true)
	if err != nil {
		return db.TokenDefinition{}, err
	}

	return p.Queries.UpdateTokenMetadataFieldsByTokenIdentifiers(ctx, db.UpdateTokenMetadataFieldsByTokenIdentifiersParams{
		Name:        t.Name,
		Description: t.Description,
		TokenID:     ti.TokenID,
		ContractID:  c.ID,
		Chain:       ti.Chain,
	})
}

// RefreshContract refreshes a contract on the given chain using the chain provider for that chain
func (p *Provider) RefreshContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	var contracts []ChainAgnosticContract

	if fetcher, ok := p.Chains[ci.Chain].(ContractFetcher); ok {
		c, err := fetcher.GetContractByAddress(ctx, ci.ContractAddress)
		if err != nil {
			return err
		}
		contracts = append(contracts, c)
	}

	_, err := p.processContracts(ctx, ci.Chain, contracts, false)
	return err
}

type ContractOwnerResult struct {
	Contracts []ChainAgnosticContract
	Chain     persist.Chain
}

// matchingWallets returns wallet addresses that belong to any of the passed chains
func (p *Provider) matchingWallets(wallets []persist.Wallet, chains []persist.Chain) map[persist.Chain][]persist.Address {
	matches := make(map[persist.Chain][]persist.Address)
	for _, chain := range chains {
		for _, wallet := range wallets {
			if wallet.Chain == chain {
				matches[chain] = append(matches[chain], wallet.Address)
			} else if overrides := chain.L1ChainGroup(); util.Contains(overrides, wallet.Chain) {
				matches[chain] = append(matches[chain], wallet.Address)
			}
		}
	}
	for chain, addresses := range matches {
		matches[chain] = util.Dedupe(addresses, true)
	}
	return matches
}

// matchingWalletsChain returns a list of wallets that match the given chain
func (p *Provider) matchingWalletsChain(wallets []persist.Wallet, chain persist.Chain) []persist.Address {
	return p.matchingWallets(wallets, []persist.Chain{chain})[chain]
}

// processContractCommunities ensures that every contract has a corresponding "contract community" in the database.
// This is the most basic community type, and every token belongs to a contract community (because every token belongs
// to a contract, and every contract belongs to a contract community).
func (d *Provider) processContractCommunities(ctx context.Context, contracts []db.Contract) ([]db.Community, error) {
	communities := make([]db.Community, 0, len(contracts))
	for _, contract := range contracts {
		// No need to fill in createdAt, lastUpdated, or deleted. They'll be handled by the upsert.
		communities = append(communities, db.Community{
			ID:              persist.GenerateID(),
			Version:         0,
			Name:            contract.Name.String,
			Description:     contract.Description.String,
			CommunityType:   persist.CommunityTypeContract,
			Key1:            fmt.Sprintf("%d", contract.Chain),
			Key2:            contract.Address.String(),
			ContractID:      contract.ID,
			ProfileImageUrl: contract.ProfileImageUrl,
		})
	}

	communities, err := d.Repos.CommunityRepository.UpsertCommunities(ctx, communities)
	if err != nil {
		return nil, err
	}

	communityByContractID := make(map[persist.DBID]db.Community)
	communityByKey := make(map[persist.CommunityKey]db.Community)

	for _, community := range communities {
		communityByContractID[community.ContractID] = community
		communityByKey[persist.CommunityKey{
			Type: community.CommunityType,
			Key1: community.Key1,
			Key2: community.Key2,
		}] = community
	}

	upsertCreatorsParams := db.UpsertCommunityCreatorsParams{}
	for _, contract := range contracts {
		creatorAddress, ok := util.FindFirst([]persist.Address{contract.OwnerAddress, contract.CreatorAddress}, func(a persist.Address) bool {
			return a != ""
		})

		if !ok {
			continue
		}

		if community, ok := communityByContractID[contract.ID]; ok {
			upsertCreatorsParams.Ids = append(upsertCreatorsParams.Ids, persist.GenerateID().String())
			upsertCreatorsParams.CreatorType = append(upsertCreatorsParams.CreatorType, int32(persist.CommunityCreatorTypeProvider))
			upsertCreatorsParams.CommunityID = append(upsertCreatorsParams.CommunityID, community.ID.String())
			upsertCreatorsParams.CreatorAddress = append(upsertCreatorsParams.CreatorAddress, creatorAddress.String())
			upsertCreatorsParams.CreatorAddressChain = append(upsertCreatorsParams.CreatorAddressChain, int32(contract.Chain))
			upsertCreatorsParams.CreatorAddressL1Chain = append(upsertCreatorsParams.CreatorAddressL1Chain, int32(contract.Chain.L1Chain()))
		}
	}

	if len(upsertCreatorsParams.Ids) > 0 {
		_, err = d.Queries.UpsertCommunityCreators(ctx, upsertCreatorsParams)
		if err != nil {
			err := fmt.Errorf("failed to upsert contract community creators: %w", err)
			logger.For(ctx).WithError(err).Error(err)
			sentryutil.ReportError(ctx, err)
		}
	}

	params := db.UpsertContractCommunityMembershipsParams{}

	for _, contract := range contracts {
		key := persist.CommunityKey{
			Type: persist.CommunityTypeContract,
			Key1: fmt.Sprintf("%d", contract.Chain),
			Key2: contract.Address.String(),
		}

		community, ok := communityByKey[key]
		if !ok {
			// This shouldn't happen. By this point, we've successfully upserted communities,
			// so we should be able to find one matching every contract we have.
			err = fmt.Errorf("couldn't find community with type: %d, key: %s for contract ID: %s", key.Type, key, contract.ID)
			sentryutil.ReportError(ctx, err)
			return nil, err
		}

		params.Ids = append(params.Ids, persist.GenerateID().String())
		params.ContractID = append(params.ContractID, contract.ID.String())
		params.CommunityID = append(params.CommunityID, community.ID.String())
	}

	_, err = d.Queries.UpsertContractCommunityMemberships(ctx, params)
	if err != nil {
		return nil, err
	}

	return communities, nil
}

// processContracts deduplicates contracts and upserts them into the database. If canOverwriteOwnerAddress is true, then
// the owner address of an existing contract will be overwritten if the new contract provides a non-empty owner address.
// An empty owner address will never overwrite an existing address, even if canOverwriteOwnerAddress is true.
func (d *Provider) processContracts(ctx context.Context, chain persist.Chain, contracts []ChainAgnosticContract, canOverwriteOwnerAddress bool) (newContracts []db.Contract, err error) {
	contractsToAdd := chainContractsToUpsertableContracts(chain, contracts)

	addedContracts, err := d.Repos.ContractRepository.BulkUpsert(ctx, contractsToAdd, canOverwriteOwnerAddress)
	if err != nil {
		return nil, err
	}

	_, err = d.processContractCommunities(ctx, addedContracts)
	if err != nil {
		return nil, err
	}

	return addedContracts, nil
}

// chainTokensToUpsertableTokenDefinitions returns a slice of token definitions that are ready to be upserted into the database from a slice of chainTokens.
func chainTokensToUpsertableTokenDefinitions(ctx context.Context, chain persist.Chain, tokens []ChainAgnosticToken, existingContracts []db.Contract) []db.TokenDefinition {
	result := make(map[persist.TokenIdentifiers]db.TokenDefinition)
	contracts := make(map[persist.ChainAddress]db.Contract)
	for _, c := range existingContracts {
		ca := persist.NewChainAddress(c.Address, c.Chain)
		contracts[ca] = c
	}
	for _, t := range tokens {
		normalizedAddress := persist.Address(chain.NormalizeAddress(t.ContractAddress))
		ti := persist.NewTokenIdentifiers(normalizedAddress, t.TokenID, chain)
		ca := persist.NewChainAddress(normalizedAddress, chain)
		c, ok := contracts[ca]
		if !ok {
			panic(fmt.Sprintf("contract %s should have already been inserted", ca))
		}
		tDef, ok := result[ti]
		if !ok {
			result[ti] = tokenToTokenDefinitionDB(t, c)
			continue
		}
		result[ti] = mergeTokenDefinitions(tDef, tokenToTokenDefinitionDB(t, c))
	}
	return util.MapValues(result)
}

func tokenToTokenDefinitionDB(t ChainAgnosticToken, c db.Contract) db.TokenDefinition {
	return db.TokenDefinition{
		Name:            util.ToNullString(t.Descriptors.Name, true),
		Description:     util.ToNullString(t.Descriptors.Description, true),
		TokenID:         t.TokenID,
		TokenType:       t.TokenType,
		ExternalUrl:     util.ToNullString(t.ExternalURL, true),
		Chain:           c.Chain,
		Metadata:        t.TokenMetadata,
		FallbackMedia:   t.FallbackMedia,
		ContractAddress: persist.Address(c.Chain.NormalizeAddress(t.ContractAddress)),
		ContractID:      c.ID,
		TokenMediaID:    "", // Upsert will handle this in the db if the definition already exists
		IsFxhash:        platform.IsFxhash(c),
	}
}

func mergeTokenDefinitions(a db.TokenDefinition, b db.TokenDefinition) db.TokenDefinition {
	a.Name = util.ToNullString(util.FirstNonEmptyString(a.Name.String, b.Name.String), true)
	a.Description = util.ToNullString(util.FirstNonEmptyString(a.Description.String, b.Description.String), true)
	a.ExternalUrl = util.ToNullString(util.FirstNonEmptyString(a.ExternalUrl.String, b.ExternalUrl.String), true)
	a.FallbackMedia, _ = util.FindFirst([]persist.FallbackMedia{a.FallbackMedia, b.FallbackMedia}, func(m persist.FallbackMedia) bool { return m.IsServable() })
	a.IsFxhash = a.IsFxhash || b.IsFxhash
	a.Metadata, _ = util.FindFirst([]persist.TokenMetadata{a.Metadata, b.Metadata}, func(m persist.TokenMetadata) bool {
		_, _, err := media.FindMediaURLsChain(m, a.Chain)
		return err == nil
	})
	return a
}

// chainTokensToUpsertableTokens returns a unique slice of tokens that are ready to be upserted into the database.
func chainTokensToUpsertableTokens(chain persist.Chain, tokens []ChainAgnosticToken, existingContracts []db.Contract, ownerUser persist.User) []op.UpsertToken {
	addressToContract := make(map[string]db.Contract)

	util.Map(existingContracts, func(c db.Contract) (any, error) {
		addr := c.Chain.NormalizeAddress(c.Address)
		addressToContract[addr] = c
		return nil, nil
	})

	seenTokens := make(map[persist.TokenIdentifiers]op.UpsertToken)
	seenWallets := make(map[persist.TokenIdentifiers][]persist.Wallet)
	seenQuantities := make(map[persist.TokenIdentifiers]persist.HexString)
	addressToWallets := make(map[string]persist.Wallet)
	createdContracts := make(map[persist.Address]bool)

	for _, wallet := range ownerUser.Wallets {
		// could a normalized address ever overlap with the normalized address of another chain?
		normalizedAddress := wallet.Chain.NormalizeAddress(wallet.Address)
		addressToWallets[normalizedAddress] = wallet
	}

	for _, contract := range existingContracts {
		// If the contract has an override creator, use that to determine whether this user is the contract's creator
		contractAddress := persist.Address(contract.Chain.NormalizeAddress(contract.Address))
		if contract.OverrideCreatorUserID != "" {
			createdContracts[contractAddress] = contract.OverrideCreatorUserID == ownerUser.ID
			continue
		}

		// If the contract doesn't have an override creator, use the first non-empty value in
		// (ownerAddress, creatorAddress) to determine the creator address. If this user
		// has a wallet matching the creator address, they're the creator of the contract.
		creatorAddress := contract.OwnerAddress
		if creatorAddress == "" {
			creatorAddress = contract.CreatorAddress
		}

		if wallet, ok := addressToWallets[contract.Chain.NormalizeAddress(creatorAddress)]; ok {
			createdContracts[contractAddress] = wallet.L1Chain == contract.L1Chain
		} else {
			createdContracts[contractAddress] = false
		}
	}

	for _, token := range tokens {

		if token.Quantity.BigInt().Cmp(big.NewInt(0)) == 0 {
			continue
		}

		normalizedAddress := chain.NormalizeAddress(token.ContractAddress)
		ti := persist.NewTokenIdentifiers(persist.Address(normalizedAddress), token.TokenID, chain)
		contract, ok := addressToContract[normalizedAddress]
		if !ok {
			panic(fmt.Sprintf("no persisted contract for chain=%d, address=%s", chain, normalizedAddress))
		}

		// Duplicate tokens will have the same values for these fields, so we only need to set them once
		if _, ok := seenTokens[ti]; !ok {
			seenTokens[ti] = op.UpsertToken{
				Identifiers: ti,
				Token: db.Token{
					OwnerUserID:    ownerUser.ID,
					BlockNumber:    sql.NullInt64{Int64: token.BlockNumber.BigInt().Int64(), Valid: true},
					IsCreatorToken: createdContracts[persist.Address(normalizedAddress)],
					ContractID:     contract.ID,
				},
			}
		}

		var found bool
		for _, wallet := range seenWallets[ti] {
			if wallet.Address == token.OwnerAddress {
				found = true
			}
		}
		if !found {
			if q, ok := seenQuantities[ti]; ok {
				seenQuantities[ti] = q.Add(token.Quantity)
			} else {
				seenQuantities[ti] = token.Quantity
			}
		}

		if w, ok := addressToWallets[chain.NormalizeAddress(token.OwnerAddress)]; ok {
			seenWallets[ti] = append(seenWallets[ti], w)
			seenWallets[ti] = dedupeWallets(seenWallets[ti])
		}

		finalSeenToken := seenTokens[ti]
		finalSeenToken.Token.OwnedByWallets = util.MapWithoutError(seenWallets[ti], func(w persist.Wallet) persist.DBID { return w.ID })
		finalSeenToken.Token.Quantity = seenQuantities[ti]
		seenTokens[ti] = finalSeenToken
	}

	res := make([]op.UpsertToken, len(seenTokens))

	i := 0
	for _, t := range seenTokens {
		res[i] = t
		i++
	}

	return res
}

// contractsToUpsertableContracts returns a unique slice of contracts that are ready to be upserted into the database.
func chainContractsToUpsertableContracts(chain persist.Chain, contracts []ChainAgnosticContract) []db.Contract {
	result := map[persist.Address]db.Contract{}
	for _, c := range contracts {
		normalizedAddress := persist.Address(chain.NormalizeAddress(c.Address))
		result[normalizedAddress] = mergeContracts(result[normalizedAddress], db.Contract{
			Symbol:               util.ToNullStringEmptyNull(c.Descriptors.Symbol),
			Name:                 util.ToNullStringEmptyNull(c.Descriptors.Name),
			OwnerAddress:         persist.Address(chain.NormalizeAddress(c.Descriptors.OwnerAddress)),
			ProfileImageUrl:      util.ToNullStringEmptyNull(c.Descriptors.ProfileImageURL),
			Description:          util.ToNullStringEmptyNull(c.Descriptors.Description),
			IsProviderMarkedSpam: util.FromPointer(c.IsSpam),
		})
	}
	r := make([]db.Contract, 0, len(result))
	for address, c := range result {
		r = append(r, db.Contract{
			Chain:                chain,
			L1Chain:              chain.L1Chain(),
			Address:              address,
			Symbol:               c.Symbol,
			Name:                 c.Name,
			ProfileImageUrl:      c.ProfileImageUrl,
			OwnerAddress:         c.OwnerAddress,
			Description:          c.Description,
			IsProviderMarkedSpam: c.IsProviderMarkedSpam,
		})
	}
	return r
}

func mergeContracts(a db.Contract, b db.Contract) db.Contract {
	a.Name, _ = util.FindFirst([]sql.NullString{a.Name, b.Name}, func(s sql.NullString) bool { return s.String != "" && !unknownContractNames[strings.ToLower(s.String)] })
	a.Symbol = util.ToNullString(util.FirstNonEmptyString(a.Symbol.String, b.Symbol.String), true)
	a.OwnerAddress = persist.Address(util.FirstNonEmptyString(a.OwnerAddress.String(), b.OwnerAddress.String()))
	a.Description = util.ToNullString(util.FirstNonEmptyString(a.Description.String, b.Description.String), true)
	a.ProfileImageUrl = util.ToNullString(util.FirstNonEmptyString(a.ProfileImageUrl.String, b.ProfileImageUrl.String), true)
	a.IsProviderMarkedSpam = a.IsProviderMarkedSpam || b.IsProviderMarkedSpam
	return a
}

func dedupeWallets(wallets []persist.Wallet) []persist.Wallet {
	return util.DedupeWithTranslate(wallets, false, func(w persist.Wallet) string {
		return fmt.Sprintf("%d:%s", w.Chain, w.Address)
	})
}

func dedupeTokenDefinitions(tDefs []db.TokenDefinition) (uniqueDefs []db.TokenDefinition) {
	type Key struct {
		Val db.TokenDefinition
		ID  string
	}

	keys := util.MapWithoutError(tDefs, func(tDef db.TokenDefinition) Key {
		return Key{
			Val: tDef,
			ID:  fmt.Sprintf("%d:%s:%s", tDef.Chain, tDef.ContractID, tDef.TokenID),
		}
	})

	t := util.DedupeWithTranslate(keys, false, func(k Key) string { return k.ID })
	return util.MapWithoutError(t, func(k Key) db.TokenDefinition { return k.Val })
}

func dedupeTokenInstances(tokens []op.UpsertToken) (uniqueTokens []op.UpsertToken) {
	type Key struct {
		Val op.UpsertToken
		ID  string
	}

	keys := util.MapWithoutError(tokens, func(t op.UpsertToken) Key {
		return Key{
			Val: t,
			ID:  fmt.Sprintf("%d:%s:%s:%s", t.Identifiers.Chain, t.Token.ContractID, t.Identifiers.TokenID, t.Token.OwnerUserID),
		}
	})

	t := util.DedupeWithTranslate(keys, false, func(k Key) string { return k.ID })
	return util.MapWithoutError(t, func(k Key) op.UpsertToken { return k.Val })
}
