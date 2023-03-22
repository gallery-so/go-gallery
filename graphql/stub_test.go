package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/tests/integration/dummymetadata"
	"github.com/mikeydub/go-gallery/tokenprocessing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/mock"
)

// stubTokenMetadataFetcher returns a canned metadata response
type stubTokenMetadataFetcher struct {
	FetchMetadata func() (persist.TokenMetadata, error)
}

func (p *stubTokenMetadataFetcher) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti ChainAgnosticIdentifiers, ownerAddress persist.Address) (persist.TokenMetadata, error) {
	return p.FetchMetadata()
}

// fetchMetadataFromDummyMetadata returns static metadata from the dummymetadata server
func fetchMetadataFromDummyMetadata(endpoint string) (persist.TokenMetadata, error) {
	handler := dummymetadata.CoreInitServer()
	req := httptest.NewRequest(http.MethodGet, endpoint, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, util.BodyAsError(res)
	}

	var data persist.TokenMetadata
	json.Unmarshal(w.Body.Bytes(), &data)
	return data, nil
}

// fetchFromDummyEndpoint fetches metadata from the given endpoint
func fetchFromDummyEndpoint(endpoint string) func() (persist.TokenMetadata, error) {
	return func() (persist.TokenMetadata, error) {
		return fetchMetadataFromDummyMetadata(endpoint)
	}
}

// stubTokensFetcher returns a canned set of tokens and contracts
type stubTokensFetcher struct {
	Contracts []multichain.ChainAgnosticContract
	Tokens    []multichain.ChainAgnosticToken
}

func (p *stubTokensFetcher) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	return p.Tokens, p.Contracts, nil
}

func (p *stubTokensFetcher) GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p *stubTokensFetcher) GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p *stubTokensFetcher) GetTokensByTokenIdentifiersAndOwner(context.Context, multichain.ChainAgnosticIdentifiers, persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

// stubProvider returns a canned set of tokens and contracts
type stubProvider struct {
	stubTokensFetcher
	stubTokenMetadataFetcher
}

type providerOpt func(*stubProvider)

func newStubProvider(opts ...providerOpt) *stubProvider {
	provider := &stubProvider{}
	for _, opt := range opts {
		opt(provider)
	}
	return provider
}

// withContracts configures the stubProvider to return a canned set of contracts
func withContracts(contracts []multichain.ChainAgnosticContract) providerOpt {
	return func(p *stubProvider) {
		p.Contracts = contracts
	}
}

// withTokens configures the stubProvider to return a canned set of tokens
func withTokens(tokens []multichain.ChainAgnosticToken) providerOpt {
	return func(p *stubProvider) {
		p.Tokens = tokens
	}
}

// withContractTokens will generate n dummy tokens from the provided contract
func withContractTokens(contract multichain.ChainAgnosticContract, address string, n int) providerOpt {
	return func(p *stubProvider) {
		tokens := []multichain.ChainAgnosticToken{}
		for i := 0; i < n; i++ {
			tokens = append(tokens, multichain.ChainAgnosticToken{
				Name:            fmt.Sprintf("%s_testToken%d", contract.Name, i),
				TokenID:         persist.TokenID(fmt.Sprintf("%X", i)),
				Quantity:        "1",
				ContractAddress: contract.Address,
				OwnerAddress:    persist.Address(address),
			})
		}
		withContracts([]multichain.ChainAgnosticContract{contract})(p)
		withTokens(tokens)(p)
	}
}

// withFetchMetadata configures how the provider will retrieve metadata
func withFetchMetadata(f func() (persist.TokenMetadata, error)) providerOpt {
	return func(p *stubProvider) {
		p.FetchMetadata = f
	}
}

// defaultStubProvider returns a stubProvider that returns dummy tokens
func defaultStubProvider(address string) *stubProvider {
	contract := multichain.ChainAgnosticContract{Address: "0x123", Name: "testContract"}
	return newStubProvider(withContractTokens(contract, address, 10))
}

// newStubRecommender returns a recommender that returns a canned set of recommendations
func newStubRecommender(t *testing.T, userIDs []persist.DBID) *recommend.Recommender {
	return &recommend.Recommender{
		LoadFunc:      func(context.Context) {},
		BootstrapFunc: func(context.Context) ([]persist.DBID, error) { return userIDs, nil },
	}
}

// sendTokensRecorder records tokenprocessing messages
type sendTokensRecorder struct {
	mock.Mock
	SendTokens multichain.SendTokens
	Tasks      []task.TokenProcessingUserMessage
}

func (r *sendTokensRecorder) Send(ctx context.Context, t task.TokenProcessingUserMessage) error {
	r.Called(ctx, t)
	r.Tasks = append(r.Tasks, t)
	return nil
}

// sendTokensNOOP is useful when the code under test doesn't require tokenprocessing
func sendTokensNOOP(context.Context, task.TokenProcessingUserMessage) error {
	return nil
}

// handler should be configured with a provider that supports fetching tokenmetadata
// the provider should be able to get configured to call a specific endpoint from	tokenprocessing
func sendTokensToHTTPHandler(handler http.Handler, method, endpoint string) multichain.SendTokens {
	return func(ctx context.Context, t task.TokenProcessingUserMessage) error {
		byt, _ := json.Marshal(t)
		r := bytes.NewReader(byt)
		req := httptest.NewRequest(method, endpoint, r)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return nil
	}
}

// sendTokensToTokenProcessing processes a batch of tokens synchronously through tokenprocessing
func sendTokensToTokenProcessing(c *server.Clients, provider *multichain.Provider) multichain.SendTokens {
	return func(context.Context, task.TokenProcessingUserMessage) error {
		h := tokenprocessing.CoreInitServer(c, provider)
		sendTokensToHTTPHandler(h, http.MethodPost, "/media/process")
		return nil
	}
}
