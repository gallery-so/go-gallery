package wlta

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	// "sync"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/reservoir"
	"github.com/mikeydub/go-gallery/service/multichain/zora"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type Submission struct {
	ID                string
	Link              string
	Domain            Domain
	Category          Category
	Chain             persist.Chain
	Contract          persist.Address
	IsNSFW            bool
	Title             string
	Description       string
	AuthorName        string
	AuthorTwitter     string
	AuthorBio         string
	AuthorWallet      persist.Address
	IsMissingContract bool
	IsMissingData     bool
}

type fetchRequirements struct {
	Submission     Submission
	Token          persist.TokenIdentifiers
	Contract       persist.ContractIdentifiers
	RequireTokenID persist.TokenID // only used for zora pre-mint
	RequireOne     bool            // require at least one token
}

type Category int
type Domain int

const (
	GenreOneOfOnes Category = iota
	GenreAi
	GenreGenArt
	GenreMusic
)

const (
	DomainZora              Domain = iota // 0
	DomainOpensea                         // 1
	DomainManifold                        // 2
	DomainDecent                          // 3
	DomainMintFun                         // 4
	DomainZoraEnergy                      // 5
	DomainOptimismEtherscan               // 6
	DomainSuperCollector                  // 7
	DomainOptimismMan                     // 8
	DomainLoogies                         // 9
	DomainZonic                           // 10
	DomainWeLoveTheArt                    // 11
	DomainHighlight                       // 12
	DomainSound                           // 13
	DomainHolograph                       // 14
	DomainTitles                          // 15
	DomainMirror                          // 16
	DomainCurate                          // 17
	DomainUnknown                         // 18
	DomainCoopdville                      // 19
	DomainBonfire                         // 20
)

type Provider struct {
	Zora         *zora.Provider
	Base         *reservoir.Provider
	Optimism     *reservoir.Provider
	requirements []fetchRequirements
}

func NewProvider() *Provider {
	submissions := readSubmissions("./service/multichain/wlta/submissions_clean.csv")
	requirements := util.MapWithoutError(submissions, func(s Submission) fetchRequirements { return handleSubmission(s) })
	return &Provider{
		Zora:         zora.NewProvider(http.DefaultClient),
		Base:         reservoir.NewProvider(http.DefaultClient, persist.ChainBase),
		Optimism:     reservoir.NewProvider(http.DefaultClient, persist.ChainOptimism),
		requirements: requirements,
	}
}

func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts, 100)
	errCh := make(chan error, 100)
	go func() {
		defer close(recCh)
		defer close(errCh)
		p.FullfillRequirements(ctx, address, recCh, errCh)
	}()
	return recCh, errCh
}

func (p *Provider) FullfillRequirements(
	ctx context.Context,
	address persist.Address,
	recCh chan<- multichain.ChainAgnosticTokensAndContracts,
	errCh chan<- error,
) {
	tokenToSubmission := make(map[persist.TokenIdentifiers]Submission)
	submissionErrors := make(map[Submission]error)

	optResults := multichain.ChainAgnosticTokensAndContracts{
		Tokens:      []multichain.ChainAgnosticToken{},
		Contracts:   []multichain.ChainAgnosticContract{},
		ActualChain: persist.ChainOptimism,
	}
	baseResults := multichain.ChainAgnosticTokensAndContracts{
		Tokens:      []multichain.ChainAgnosticToken{},
		Contracts:   []multichain.ChainAgnosticContract{},
		ActualChain: persist.ChainBase,
	}
	zoraResults := multichain.ChainAgnosticTokensAndContracts{
		Tokens:      []multichain.ChainAgnosticToken{},
		Contracts:   []multichain.ChainAgnosticContract{},
		ActualChain: persist.ChainZora,
	}

	// var tokenMu sync.Mutex
	// var errMu sync.Mutex

	for _, req := range p.requirements {
		r, err := p.fullfillRequirement(ctx, req)

		// swallow the error for debugging
		if err != nil {
			// errMu.Lock()
			submissionErrors[req.Submission] = err
			// errMu.Unlock()
			continue
		}

		// override owner with address
		r.Tokens = util.MapWithoutError(r.Tokens, func(t multichain.ChainAgnosticToken) multichain.ChainAgnosticToken {
			return overrideTokenOwner(t, address)
		})

		// append to mapping file of token identifiers to category
		// tokenMu.Lock()
		for _, t := range r.Tokens {
			tokenToSubmission[persist.TokenIdentifiers{
				TokenID:         t.TokenID,
				ContractAddress: t.ContractAddress,
				Chain:           r.ActualChain,
			}] = req.Submission
		}
		// tokenMu.Unlock()

		if r.ActualChain == persist.ChainOptimism {
			optResults.Tokens = append(optResults.Tokens, r.Tokens...)
			optResults.Contracts = append(optResults.Contracts, r.Contracts...)
			continue
		}

		if r.ActualChain == persist.ChainBase {
			baseResults.Tokens = append(baseResults.Tokens, r.Tokens...)
			baseResults.Contracts = append(baseResults.Contracts, r.Contracts...)
			continue
		}

		if r.ActualChain == persist.ChainZora {
			zoraResults.Tokens = append(zoraResults.Tokens, r.Tokens...)
			zoraResults.Contracts = append(zoraResults.Contracts, r.Contracts...)
			continue
		}

		panic(fmt.Errorf("unknown chain :%d", r.ActualChain))

		// recCh <- r
	}

	writeMappingFile(tokenToSubmission)
	writeErrorsFile(submissionErrors)
	recCh <- optResults
	recCh <- baseResults
	recCh <- zoraResults
}

func writeMappingFile(m map[persist.TokenIdentifiers]Submission) {
	f, err := os.Create(fmt.Sprintf("./service/multichain/wlta/id_to_category_%s.csv", env.GetString("ENV")))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()
	for t, s := range m {
		w.Write([]string{
			fmt.Sprintf("%d", t.Chain),    // chain
			t.ContractAddress.String(),    // contract_address
			t.TokenID.String(),            // token_id
			s.ID,                          // submission_id
			fmt.Sprintf("%d", s.Category), // category
			strconv.FormatBool(s.IsNSFW),  // is_nsfw
		})
	}
}

func writeErrorsFile(m map[Submission]error) {
	f, err := os.Create(fmt.Sprintf("./service/multichain/wlta/submission_errors_%s.csv", env.GetString("ENV")))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()
	for s, e := range m {
		w.Write([]string{
			s.ID,                       // submission_id
			s.Link,                     // link
			fmt.Sprintf("%d", s.Chain), // chain
			s.Contract.String(),        // contract_address
			e.Error(),                  // error
		})
	}
}

func overrideTokenOwner(t multichain.ChainAgnosticToken, a persist.Address) multichain.ChainAgnosticToken {
	t.OwnerAddress = a
	return t
}

func (p *Provider) fullfillRequirement(ctx context.Context, req fetchRequirements) (multichain.ChainAgnosticTokensAndContracts, error) {
	fmt.Printf("[%s] handling submission\n", req.Submission.ID)
	if req.Submission.IsMissingContract {
		return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("missing contract data")
	}
	// require exact token
	if req.Token != (persist.TokenIdentifiers{}) {
		tID := multichain.ChainAgnosticIdentifiers{
			TokenID:         req.Token.TokenID,
			ContractAddress: req.Token.ContractAddress,
		}
		if req.Token.Chain == persist.ChainBase {
			token, contract, err := p.Base.GetTokenByTokenIdentifiers(ctx, tID)
			if err != nil {
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("failed exact token requirement: %s", err)
			}
			return multichain.ChainAgnosticTokensAndContracts{
				Tokens:      []multichain.ChainAgnosticToken{token},
				Contracts:   []multichain.ChainAgnosticContract{contract},
				ActualChain: persist.ChainBase,
			}, nil
		}
		if req.Token.Chain == persist.ChainOptimism {
			token, contract, err := p.Optimism.GetTokenByTokenIdentifiers(ctx, tID)
			if err != nil {
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("failed exact token requirement: %s", err)
			}
			return multichain.ChainAgnosticTokensAndContracts{
				Tokens:      []multichain.ChainAgnosticToken{token},
				Contracts:   []multichain.ChainAgnosticContract{contract},
				ActualChain: persist.ChainOptimism,
			}, nil
		}
		if req.Token.Chain == persist.ChainZora {
			token, contract, err := p.Zora.GetTokenByTokenIdentifiers(ctx, tID)
			if err != nil {
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("failed exact token requirement: %s", err)
			}
			return multichain.ChainAgnosticTokensAndContracts{
				Tokens:      []multichain.ChainAgnosticToken{token},
				Contracts:   []multichain.ChainAgnosticContract{contract},
				ActualChain: persist.ChainZora,
			}, nil
		}
		panic(fmt.Errorf("invalid chain: %d", req.Token.Chain))
	}
	// require matching token (only used for zora pre-mint)
	if req.RequireTokenID != "" {
		if req.Contract.Chain == persist.ChainZora {
			tokens, contract, err := p.Zora.GetTokensByContractAddress(ctx, req.Contract.ContractAddress, 0, 0)
			if err != nil {
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("failed require matching token requirements: %s", err)
			}
			for _, t := range tokens {
				if t.TokenID == req.RequireTokenID {
					return multichain.ChainAgnosticTokensAndContracts{
						Tokens:      []multichain.ChainAgnosticToken{t},
						Contracts:   []multichain.ChainAgnosticContract{contract},
						ActualChain: persist.ChainZora,
					}, nil
				}
			}
			if len(tokens) == 1 {
				return multichain.ChainAgnosticTokensAndContracts{
					Tokens:      tokens,
					Contracts:   []multichain.ChainAgnosticContract{contract},
					ActualChain: persist.ChainZora,
				}, nil
			}
			return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("failed require matching token requirements")
		}
		panic(fmt.Errorf("[%s] invalid chain: %d", req.Submission.ID, req.Token.Chain))
	}
	// require at least one token
	if req.RequireOne {
		var tokens []multichain.ChainAgnosticToken
		var contract multichain.ChainAgnosticContract
		var err error
		if req.Contract.Chain == persist.ChainBase {
			tokens, contract, err = p.Base.GetTokensByContractAddress(ctx, req.Contract.ContractAddress, 0, 0)
			if err != nil {
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("failed require one token requirements: %s", err)
			}
			return multichain.ChainAgnosticTokensAndContracts{
				Tokens:      tokens[:1],
				Contracts:   []multichain.ChainAgnosticContract{contract},
				ActualChain: persist.ChainBase,
			}, nil
		}
		if req.Contract.Chain == persist.ChainOptimism {
			tokens, contract, err = p.Optimism.GetTokensByContractAddress(ctx, req.Contract.ContractAddress, 0, 0)
			if err != nil {
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("failed require at least one token requirements: %s", err)
			}
			return multichain.ChainAgnosticTokensAndContracts{
				Tokens:      tokens[:1],
				Contracts:   []multichain.ChainAgnosticContract{contract},
				ActualChain: persist.ChainOptimism,
			}, nil
		}
		if req.Contract.Chain == persist.ChainZora {
			tokens, contract, err = p.Zora.GetTokensByContractAddress(ctx, req.Contract.ContractAddress, 0, 0)
			if err != nil {
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("failed require at least one token requirements: %s", err)
			}
			return multichain.ChainAgnosticTokensAndContracts{
				Tokens:      tokens[:1],
				Contracts:   []multichain.ChainAgnosticContract{contract},
				ActualChain: persist.ChainZora,
			}, nil
		}
		if len(tokens) == 0 {
			return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("failed require at least one token requirements: %s", err)
		}
		panic(fmt.Errorf("invalid chain: %d", req.Contract.Chain))
	}
	panic(fmt.Errorf("invalid requirements: %+v", req))
}

func handleSubmission(s Submission) fetchRequirements {
	parts := URLParts(s.Link)

	if s.IsMissingContract {
		cID := persist.ContractIdentifiers{
			ContractAddress: s.Contract,
			Chain:           s.Chain,
		}
		return fetchRequirements{
			Submission: s,
			Contract:   cID,
			RequireOne: true,
		}
	}

	if s.Domain == DomainZora {
		// zora collect url with pre-mint
		if len(parts) == 3 && parts[0] == "collect" && strings.HasPrefix(parts[2], "premint") {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission:     s,
				Contract:       cID,
				RequireTokenID: persist.MustTokenID(strings.TrimPrefix(parts[2], "premint-")),
			}
		}

		// zora collect url with token ID
		if len(parts) == 3 && parts[0] == "collect" {
			tID := persist.TokenIdentifiers{
				TokenID:         persist.MustTokenID(parts[2]),
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Token:      tID,
			}
		}

		// zora collect url no token ID
		if len(parts) == 2 && parts[0] == "collect" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if len(parts) == 1 && strings.HasSuffix(parts[0], ".eth") {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if len(parts) == 1 && strings.HasPrefix(parts[0], "0x") {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainOpensea {
		// opensea asset url with token ID
		if len(parts) == 4 && parts[0] == "assets" {
			tID := persist.TokenIdentifiers{
				TokenID:         persist.MustTokenID(parts[3]),
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Token:      tID,
				RequireOne: true,
			}
		}

		if len(parts) == 3 && parts[0] == "collection" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if len(parts) == 5 && parts[1] == "assets" {
			tID := persist.TokenIdentifiers{
				TokenID:         persist.MustTokenID(parts[4]),
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Token:      tID,
				RequireOne: true,
			}
		}

		// bad URL
		if s.ID == "12562" && len(parts) == 10 {
			p := strings.Split(s.Link, "https://")
			link := fmt.Sprintf("https://%s", p[1])
			parts = URLParts(link)
			if len(parts) == 4 && parts[0] == "assets" {
				tID := persist.TokenIdentifiers{
					TokenID:         persist.MustTokenID(parts[3]),
					ContractAddress: s.Contract,
					Chain:           s.Chain,
				}
				return fetchRequirements{
					Submission: s,
					Token:      tID,
				}
			}
		}

		if len(parts) == 5 && parts[0] == "assets" {
			tID := persist.TokenIdentifiers{
				TokenID:         persist.MustTokenID(parts[3]),
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Token:      tID,
			}
		}

		if len(parts) == 1 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainManifold {
		if len(parts) == 1 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if len(parts) == 3 && parts[2] == "edit" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		// manifold asset url with token ID
		if len(parts) == 3 {
			tID := persist.TokenIdentifiers{
				TokenID:         persist.MustTokenID(parts[2]),
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Token:      tID,
			}
		}
	}

	if s.Domain == DomainDecent {
		// decent asset url no token ID
		cID := persist.ContractIdentifiers{
			ContractAddress: s.Contract,
			Chain:           s.Chain,
		}
		return fetchRequirements{
			Submission: s,
			Contract:   cID,
			RequireOne: true,
		}
	}

	if s.Domain == DomainOpensea {
		if len(parts) == 2 && parts[0] == "collection" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainMintFun {
		// mint.fun asset url no token ID
		cID := persist.ContractIdentifiers{
			ContractAddress: s.Contract,
			Chain:           s.Chain,
		}
		if len(parts) == 2 {
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
		if len(parts) == 3 && parts[2] == "created" {
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
		if len(parts) == 4 {
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainManifold {
		// manifold asset url with no token ID
		if len(parts) == 2 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if len(parts) == 4 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainOptimismEtherscan {
		if len(parts) == 2 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainOptimismMan {
		if len(parts) == 1 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainSuperCollector {
		if len(parts) == 1 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainLoogies {
		if len(parts) == 1 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainZonic {
		if len(parts) == 2 && parts[0] == "collection" && parts[1] == "omniwonders" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainWeLoveTheArt {
		cID := persist.ContractIdentifiers{
			ContractAddress: s.Contract,
			Chain:           s.Chain,
		}
		return fetchRequirements{
			Submission: s,
			Contract:   cID,
			RequireOne: true,
		}
	}

	if s.Domain == DomainHighlight {
		if len(parts) == 2 && parts[0] == "mint" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if len(parts) == 3 && parts[0] == "mint" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if len(parts) == 4 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainHolograph {
		if len(parts) == 2 && parts[0] == "collections" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if s.ID == "11020" {
			tID := persist.TokenIdentifiers{
				TokenID:         persist.MustTokenID("188719626670054478562669105609137414715460010957784007367725272453685"),
				ContractAddress: "0x8c531f965c05fab8135d951e2ad0ad75cf3405a2",
				Chain:           persist.ChainOptimism,
			}
			return fetchRequirements{
				Submission: s,
				Token:      tID,
			}
		}

		if len(parts) == 4 && parts[0] == "collections" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if len(parts) == 4 && parts[0] == "collector" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainSound {
		if len(parts) == 2 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}

		if len(parts) == 1 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainTitles {
		if len(parts) == 3 && parts[0] == "collect" {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	if s.Domain == DomainCurate {
		cID := persist.ContractIdentifiers{
			ContractAddress: s.Contract,
			Chain:           s.Chain,
		}
		return fetchRequirements{
			Submission: s,
			Contract:   cID,
			RequireOne: true,
		}
	}

	if s.Domain == DomainMirror {
		if len(parts) == 2 {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission: s,
				Contract:   cID,
				RequireOne: true,
			}
		}
	}

	fmt.Printf("submissionID: %s; link: %s; parts: %d\n", s.ID, s.Link, len(parts))
	for i, p := range parts {
		fmt.Printf("part %d: %s\n", i, p)
	}
	panic("not implemented")
}

func URLParts(u string) []string {
	u = strings.Trim(RemoveQueryParams(u).Path, "/")
	return strings.Split(u, "/")
}

func RemoveQueryParams(link string) *url.URL {
	u, err := url.Parse(link)
	if err != nil {
		panic(err)
	}
	u.RawQuery = ""
	return u
}

func readSubmissions(f string) []Submission {
	byt, err := os.ReadFile(f)
	if err != nil {
		panic(err)
	}

	r := bytes.NewReader(byt)
	c := csv.NewReader(r)

	submissions := make([]Submission, 0)

	for {
		record, err := c.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			panic(err)
		}

		submissions = append(submissions, rowToSubmission(record))
	}

	return submissions
}

func rowToSubmission(r []string) Submission {
	chainRaw, err := strconv.Atoi(r[4])
	if err != nil {
		panic(err)
	}

	chain, ok := map[int]persist.Chain{
		3: persist.ChainOptimism,
		6: persist.ChainZora,
		7: persist.ChainBase,
		8: persist.ChainUnknown,
	}[chainRaw]
	if !ok {
		panic(chainRaw)
	}

	domainRaw, err := strconv.Atoi(r[2])
	if err != nil {
		panic(err)
	}

	domain, ok := map[int]Domain{
		0:  DomainZora,
		1:  DomainOpensea,
		2:  DomainManifold,
		3:  DomainDecent,
		4:  DomainMintFun,
		5:  DomainZoraEnergy,
		6:  DomainOptimismEtherscan,
		7:  DomainSuperCollector,
		8:  DomainOptimismMan,
		9:  DomainLoogies,
		10: DomainZonic,
		11: DomainWeLoveTheArt,
		12: DomainHighlight,
		13: DomainSound,
		14: DomainHolograph,
		15: DomainTitles,
		16: DomainMirror,
		17: DomainCurate,
		18: DomainUnknown,
		19: DomainCoopdville,
		20: DomainBonfire,
	}[domainRaw]
	if !ok {
		panic(domainRaw)
	}

	categoryRaw, err := strconv.Atoi(r[3])
	if err != nil {
		panic(err)
	}

	category, ok := map[int]Category{
		0: GenreOneOfOnes,
		1: GenreAi,
		2: GenreGenArt,
		3: GenreMusic,
	}[categoryRaw]
	if !ok {
		panic(categoryRaw)
	}

	isNSFW, err := strconv.ParseBool(r[6])
	if err != nil {
		panic(err)
	}

	isMissingContract, err := strconv.ParseBool(r[13])
	if err != nil {
		panic(err)
	}

	isMissingData, err := strconv.ParseBool(r[14])
	if err != nil {
		panic(err)
	}

	contract := persist.Address(r[5])
	if contract == "" && !isMissingContract {
		panic(fmt.Errorf("empty address: %s", r[0]))
	}

	return Submission{
		ID:                r[0],
		Link:              r[1],
		Domain:            domain,
		Category:          category,
		Chain:             chain,
		Contract:          persist.Address(r[5]),
		IsNSFW:            isNSFW,
		Title:             r[7],
		Description:       r[8],
		AuthorName:        r[9],
		AuthorTwitter:     r[10],
		AuthorBio:         r[11],
		AuthorWallet:      persist.Address(r[12]),
		IsMissingContract: isMissingContract,
		IsMissingData:     isMissingData,
	}
}

func ValidateTokenID(s string) {
	if _, err := strconv.Atoi(s); err != nil {
		panic(fmt.Sprintf("invalid token id: %s", s))
	}
}

func ValidateChainAndAddress(s string) {
	if len(strings.Split(s, ":")) != 2 {
		panic(fmt.Sprintf("invalid chain and address: %s", s))
	}
}
