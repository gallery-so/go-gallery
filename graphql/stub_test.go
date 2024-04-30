package graphql_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/service/recommend/userpref"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/mock"
)

// stubProvider returns a canned set of tokens and contracts
type stubProvider struct {
	Contracts     []common.ChainAgnosticContract
	Tokens        []common.ChainAgnosticToken
	FetchMetadata func() (persist.TokenMetadata, error)
	RetErr        error
}

func (p stubProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address) ([]common.ChainAgnosticToken, []common.ChainAgnosticContract, error) {
	return p.Tokens, p.Contracts, p.RetErr
}

func (p stubProvider) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan common.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)
	go func() {
		defer close(recCh)
		defer close(errCh)
		if p.RetErr != nil {
			errCh <- p.RetErr
			return
		}
		recCh <- common.ChainAgnosticTokensAndContracts{
			Tokens:    p.Tokens,
			Contracts: p.Contracts,
		}
	}()
	return recCh, errCh
}

func (p stubProvider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	return p.FetchMetadata()
}

func (p stubProvider) GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p stubProvider) GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit, offset int) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p stubProvider) GetTokenByTokenIdentifiersAndOwner(context.Context, common.ChainAgnosticIdentifiers, persist.Address) (common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	panic("not implemented")
}

type providerOpt func(*stubProvider)

func newStubProvider(opts ...providerOpt) stubProvider {
	provider := stubProvider{}
	for _, opt := range opts {
		opt(&provider)
	}
	return provider
}

// withContracts configures the stubProvider to return a canned set of contracts
func withContracts(contracts []common.ChainAgnosticContract) providerOpt {
	return func(p *stubProvider) {
		p.Contracts = contracts
	}
}

// withTokens configures the stubProvider to return a canned set of tokens
func withTokens(tokens []common.ChainAgnosticToken) providerOpt {
	return func(p *stubProvider) {
		p.Tokens = tokens
	}
}

// withReturnError configures the stubProvider to return an error
func withReturnError(err error) providerOpt {
	return func(p *stubProvider) {
		p.RetErr = err
	}
}

// withDummyTokenN will generate n dummy tokens from the provided contract
func withDummyTokenN(contract common.ChainAgnosticContract, ownerAddress persist.Address, n int) providerOpt {
	start := 1337
	return func(p *stubProvider) {
		tokens := []common.ChainAgnosticToken{}
		for i := start; i < start+n; i++ {
			tokenID := persist.DecimalTokenID(fmt.Sprint(i))
			token := dummyTokenIDContract(ownerAddress, contract.Address, tokenID.ToHexTokenID())
			tokens = append(tokens, token)
		}
		withContracts([]common.ChainAgnosticContract{contract})(p)
		withTokens(tokens)(p)
	}
}

// withDummyTokenID will generate a token with the provided token ID
func withDummyTokenID(ownerAddress persist.Address, tokenID persist.HexTokenID) providerOpt {
	c := common.ChainAgnosticContract{Address: "0x123"}
	return func(p *stubProvider) {
		withContracts([]common.ChainAgnosticContract{c})(p)
		withTokens([]common.ChainAgnosticToken{dummyTokenIDContract(ownerAddress, c.Address, tokenID)})(p)
	}
}

// withFetchMetadata configures how the provider will retrieve metadata
func withFetchMetadata(f func() (persist.TokenMetadata, error)) providerOpt {
	return func(p *stubProvider) {
		p.FetchMetadata = f
	}
}

// defaultStubProvider returns a stubProvider that returns dummy tokens
func defaultStubProvider(ownerAddress persist.Address) stubProvider {
	contract := common.ChainAgnosticContract{Address: "0x123", Descriptors: common.ChainAgnosticContractDescriptors{Name: "testContract"}}
	return newStubProvider(withDummyTokenN(contract, ownerAddress, 10))
}

// newStubRecommender returns a recommender that returns a canned set of recommendations
func newStubRecommender(t *testing.T, userIDs []persist.DBID) *recommend.Recommender {
	return &recommend.Recommender{
		LoadFunc:      func(context.Context) {},
		BootstrapFunc: func(context.Context) ([]persist.DBID, error) { return userIDs, nil },
	}
}

// newStubPersonalization returns a stub of the personalization service
func newStubPersonalization(t *testing.T) *userpref.Personalization {
	return &userpref.Personalization{}
}

// noopSubmitter is useful when the code under test doesn't require tokenprocessing
type noopSubmitter struct{}

func (n *noopSubmitter) SubmitNewTokens(context.Context, []persist.DBID) error { return nil }
func (n *noopSubmitter) SubmitTokenForRetry(context.Context, persist.DBID, int, time.Duration) error {
	return nil
}

// recorderSubmitter records tokens sent to tokenprocessing
type recorderSubmitter struct{ mock.Mock }

func (r *recorderSubmitter) SubmitNewTokens(ctx context.Context, tokenDefinitionIDs []persist.DBID) error {
	r.Called(ctx, tokenDefinitionIDs)
	return nil
}

func (r *recorderSubmitter) SubmitTokenForRetry(context.Context, persist.DBID, int, time.Duration) error {
	panic("not implemented") // shouldn't be needed in syncing tests
}

// httpSubmitter processes tokens synchronously via HTTP
type httpSubmitter struct {
	Handler  http.Handler
	Method   string
	Endpoint string
}

func (h *httpSubmitter) SubmitNewTokens(ctx context.Context, tokenDefinitionIDs []persist.DBID) error {
	m := task.TokenProcessingBatchMessage{BatchID: persist.GenerateID(), TokenDefinitionIDs: tokenDefinitionIDs}
	byt, _ := json.Marshal(m)
	r := bytes.NewReader(byt)
	req := httptest.NewRequest(h.Method, h.Endpoint, r)
	w := httptest.NewRecorder()
	h.Handler.ServeHTTP(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		return util.BodyAsError(res)
	}
	return nil
}

func (h *httpSubmitter) SubmitTokenForRetry(context.Context, persist.DBID, int, time.Duration) error {
	panic("not implemented") // shouldn't be needed in syncing tests
}

// fetchFromDummyEndpoint fetches metadata from the given endpoint
func fetchFromDummyEndpoint(url, endpoint string) func() (persist.TokenMetadata, error) {
	return func() (persist.TokenMetadata, error) {
		r := httptest.NewRequest(http.MethodGet, url+endpoint, nil)
		r.RequestURI = ""

		res, err := http.DefaultClient.Do(r)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		// Don't try to JSON parse base64 encoded data
		if strings.HasPrefix(string(body), "data:application/json;base64,") {
			body = []byte(strings.Split(string(body), ",")[1])
			body, err = util.Base64Decode(string(body), base64.StdEncoding, base64.RawStdEncoding)
			if err != nil {
				return nil, err
			}
		}

		var meta persist.TokenMetadata
		err = json.Unmarshal(body, &meta)

		return meta, err
	}
}
