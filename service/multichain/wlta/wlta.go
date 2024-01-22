package wlta

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/reservoir"
	"github.com/mikeydub/go-gallery/service/multichain/zora"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type submission struct {
	ID       int
	Link     string
	Domain   domain
	Category Category
	Chain    persist.Chain
	Contract persist.Address
}

type fetchRequirements struct {
	Submission     submission
	Token          persist.TokenIdentifiers
	Contract       persist.ContractIdentifiers
	RequireTokenID persist.TokenID // only used for zora pre-mint
	RequireOne     bool            // require at least one token
}

type Category int
type domain int

const (
	GenreOneOfOnes Category = iota
	GenreAi
	GenreGenArt
	GenreMusic
)

const (
	domainZora domain = iota
	domainOpensea
	domainManifold
	domainDecent
	domainMintFun
)

type Provider struct {
	Zora         *zora.Provider
	Base         *reservoir.Provider
	Optimism     *reservoir.Provider
	requirements []fetchRequirements
}

func NewProvider() *Provider {
	submissions := readSubmissions("./service/multichain/wlta/tokens.csv")
	requirements := util.MapWithoutError(submissions, func(s submission) fetchRequirements { return handleSubmission(s) })
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
		p.fulfillRequirements(ctx, address, recCh, errCh)
	}()
	return recCh, errCh
}

func (p *Provider) fulfillRequirements(
	ctx context.Context,
	address persist.Address,
	recCh chan<- multichain.ChainAgnosticTokensAndContracts,
	errCh chan<- error,
) {
	tokenToSubmission := make(map[persist.TokenIdentifiers]submission)
	submissionErrors := make(map[submission]error)

	var tokenMu sync.Mutex
	var errMu sync.Mutex

	for _, req := range p.requirements {
		r, err := p.fullfillRequirement(ctx, req)

		// swallow the error for debugging
		if err != nil {
			errMu.Lock()
			submissionErrors[req.Submission] = err
			errMu.Unlock()
			continue
		}

		// override owner with address
		r.Tokens = util.MapWithoutError(r.Tokens, func(t multichain.ChainAgnosticToken) multichain.ChainAgnosticToken {
			return overrideTokenOwner(t, address)
		})

		// append to mapping file of token identifiers to category
		tokenMu.Lock()
		for _, t := range r.Tokens {
			tokenToSubmission[persist.TokenIdentifiers{
				TokenID:         t.TokenID,
				ContractAddress: t.ContractAddress,
				Chain:           r.ActualChain,
			}] = req.Submission
		}
		tokenMu.Unlock()

		recCh <- r
	}

	writeMappingFile(tokenToSubmission)
	writeErrorsFile(submissionErrors)
}

func writeMappingFile(m map[persist.TokenIdentifiers]submission) {
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
			fmt.Sprintf("%d", s.ID),       // submission_id
			fmt.Sprintf("%d", s.Category), // category
		})
	}
}

func writeErrorsFile(m map[submission]error) {
	f, err := os.Create(fmt.Sprintf("./service/multichain/wlta/submission_errors_%s.csv", env.GetString("ENV")))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()
	for s, e := range m {
		w.Write([]string{
			fmt.Sprintf("%d", s.ID),    // submission_id
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
	fmt.Printf("[%d] handling submission\n", req.Submission.ID)
	// require exact token
	if req.Token != (persist.TokenIdentifiers{}) {
		tID := multichain.ChainAgnosticIdentifiers{
			TokenID:         req.Token.TokenID,
			ContractAddress: req.Token.ContractAddress,
		}
		if req.Token.Chain == persist.ChainBase {
			token, contract, err := p.Base.GetTokenByTokenIdentifiers(ctx, tID)
			if err != nil {
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("[%d] failed exact token requirement: %s", req.Submission.ID, err)
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
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("[%d] failed exact token requirement: %s", req.Submission.ID, err)
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
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("[%d] failed exact token requirement: %s", req.Submission.ID, err)
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
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("[%d] failed require match token requirements: %s", req.Submission.ID, err)
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
			return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("[%d] failed require match token requirements", req.Submission.ID)
		}
		panic(fmt.Errorf("[%d] invalid chain: %d", req.Submission.ID, req.Token.Chain))
	}
	// require at least one token
	if req.RequireOne {
		var tokens []multichain.ChainAgnosticToken
		var contract multichain.ChainAgnosticContract
		var err error
		if req.Contract.Chain == persist.ChainBase {
			tokens, contract, err = p.Base.GetTokensByContractAddress(ctx, req.Contract.ContractAddress, 0, 0)
			if err != nil {
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("[%d] failed require one token requirements: %s", req.Submission.ID, err)
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
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("[%d] failed require one token requirements: %s", req.Submission.ID, err)
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
				return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("[%d] failed require one token requirements: %s", req.Submission.ID, err)
			}
			return multichain.ChainAgnosticTokensAndContracts{
				Tokens:      tokens[:1],
				Contracts:   []multichain.ChainAgnosticContract{contract},
				ActualChain: persist.ChainZora,
			}, nil
		}
		if len(tokens) == 0 {
			return multichain.ChainAgnosticTokensAndContracts{}, fmt.Errorf("[%d] failed require one token requirements: %s", req.Submission.ID, errors.New("no tokens found"))
		}
		panic(fmt.Errorf("invalid chain: %d", req.Contract.Chain))
	}
	panic(fmt.Errorf("invalid requirements: %+v", req))
}

func handleSubmission(s submission) fetchRequirements {
	parts := urlParts(s.Link)

	if s.Domain == domainZora {
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

		// zora manage url with pre-mint
		if len(parts) == 4 && parts[0] == "manage" && strings.HasPrefix(parts[3], "premint") {
			cID := persist.ContractIdentifiers{
				ContractAddress: s.Contract,
				Chain:           s.Chain,
			}
			return fetchRequirements{
				Submission:     s,
				Contract:       cID,
				RequireTokenID: persist.MustTokenID(strings.TrimPrefix(parts[3], "premint-")),
			}
		}

		// zora manage url with token ID
		if len(parts) == 4 && parts[0] == "manage" {
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

		// zora manage url no token ID
		if len(parts) == 3 && parts[0] == "manage" {
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

	}

	if s.Domain == domainOpensea {
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
			}
		}
	}

	if s.Domain == domainManifold {
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

	if s.Domain == domainDecent {
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

	if s.Domain == domainMintFun {
		// mint.fun asset url no token ID
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

	if s.Domain == domainManifold {
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
	}

	panic(fmt.Errorf("%+v", s))
}

func urlParts(u string) []string {
	u = strings.Trim(removeQueryParams(u).Path, "/")
	return strings.Split(u, "/")
}

func removeQueryParams(link string) *url.URL {
	u, err := url.Parse(link)
	if err != nil {
		panic(err)
	}
	u.RawQuery = ""
	return u
}

func readSubmissions(f string) []submission {
	byt, err := os.ReadFile(f)
	if err != nil {
		panic(err)
	}

	r := bytes.NewReader(byt)
	c := csv.NewReader(r)

	submissions := make([]submission, 0)

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

func rowToSubmission(r []string) submission {
	id, err := strconv.Atoi(r[0])
	if err != nil {
		panic(err)
	}

	chain, ok := map[string]persist.Chain{
		"base":     persist.ChainBase,
		"zora":     persist.ChainZora,
		"optimism": persist.ChainOptimism,
	}[r[4]]
	if !ok {
		panic(r[4])
	}

	category, ok := map[string]Category{
		"1of1s":          GenreOneOfOnes,
		"AI Art":         GenreAi,
		"Generative Art": GenreGenArt,
		"Music":          GenreMusic,
	}[r[3]]
	if !ok {
		panic(category)
	}

	domain, ok := map[string]domain{
		"zora.co":              domainZora,
		"opensea.io":           domainOpensea,
		"gallery.manifold.xyz": domainManifold,
		"hq.decent.xyz":        domainDecent,
		"mint.fun":             domainMintFun,
	}[r[2]]
	if !ok {
		panic(r[2])
	}

	contract := persist.Address(r[5])
	if contract == "" {
		panic(fmt.Errorf("empty address: %d", id))
	}

	return submission{
		ID:       id,
		Link:     strings.TrimSpace(r[1]),
		Domain:   domain,
		Category: category,
		Chain:    persist.Chain(chain),
		Contract: persist.Address(chain.NormalizeAddress(contract)),
	}
}
